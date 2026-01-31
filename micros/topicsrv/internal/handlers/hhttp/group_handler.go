package hhttp

import (
	"context"
	"errors"
	"fmt"
	"github.com/gofiber/fiber/v2"
	"github.com/nsqio/go-nsq"
	"github.com/sweemingdow/gmicro_pkg/pkg/component/cid/sfid"
	"github.com/sweemingdow/gmicro_pkg/pkg/mylog"
	"github.com/sweemingdow/gmicro_pkg/pkg/server/srpc/rpccall"
	"github.com/sweemingdow/gmicro_pkg/pkg/utils"
	"github.com/sweemingdow/gmicro_pkg/pkg/utils/umap"
	"github.com/sweemingdow/gmicro_pkg/pkg/utils/usli"
	"github.com/sweemingdow/sdim/external/eglobal/chatconst"
	"github.com/sweemingdow/sdim/external/eglobal/nsqconst/payload/convpd"
	"github.com/sweemingdow/sdim/external/eglobal/nsqconst/payload/msgpd"
	"github.com/sweemingdow/sdim/external/emodel/chatmodel"
	"github.com/sweemingdow/sdim/external/emodel/msgmodel"
	"github.com/sweemingdow/sdim/external/emodel/usermodel"
	"github.com/sweemingdow/sdim/external/erpc/rpcuser"
	"github.com/sweemingdow/sdim/micros/topicsrv/internal/core"
	"github.com/sweemingdow/sdim/micros/topicsrv/internal/core/nsqsend"
	"github.com/sweemingdow/sdim/micros/topicsrv/internal/repostories/grouprepo"
	"github.com/sweemingdow/sdim/pkg/wrapper"
	"strings"
	"sync/atomic"
	"time"
)

type GroupHttpHandler struct {
	cm           core.ConvManager
	gm           core.GroupManager
	ms           *nsqsend.MsgSender
	gr           grouprepo.GroupRepository
	userProvider rpcuser.UserInfoRpcProvider
	done         chan struct{}
	doneChan     chan *nsq.ProducerTransaction
	closed       atomic.Bool
}

func NewGroupHttpHandler(
	cm core.ConvManager,
	gr grouprepo.GroupRepository,
	gm core.GroupManager,
	ms *nsqsend.MsgSender,
	userProvider rpcuser.UserInfoRpcProvider,
) *GroupHttpHandler {
	ghh := &GroupHttpHandler{
		cm:           cm,
		gm:           gm,
		gr:           gr,
		ms:           ms,
		userProvider: userProvider,
		doneChan:     make(chan *nsq.ProducerTransaction, 10),
		done:         make(chan struct{}),
	}

	go ghh.receiveMqSendAsyncResult()

	return ghh
}

type StartGroupChatReq struct {
	GroupName  string   `json:"groupName"`
	Avatar     string   `json:"avatar"`
	OwnerUid   string   `json:"ownerUid"`
	LimitedNum int      `json:"limitedNum"`
	Members    []string `json:"members"`
}

// 发起群聊(创建并加入群聊)
func (h *GroupHttpHandler) HandleStartGroupChat(c *fiber.Ctx) error {
	var req StartGroupChatReq
	err := c.BodyParser(&req)
	if err != nil {
		return err
	}

	if len(req.Members) == 0 {
		return errors.New("can not create group without any members")
	}

	newMebs := usli.Distinct(req.Members)
	if len(newMebs) == 1 {
		if newMebs[0] == req.OwnerUid {
			return errors.New("can not create group with self")
		}
	}

	newMebs = append([]string{req.OwnerUid}, newMebs...)

	if req.LimitedNum > 0 {
		if len(newMebs) > req.LimitedNum {
			return errors.New("group members over limited num")
		}
	}

	lg := mylog.AppLogger()

	lg.Debug().Msgf("handle start group chat, req=%+v", req)

	// rpc查用户信息
	rpcResp, err := h.userProvider.UsersUnitInfo(rpccall.CreateReq(rpcuser.UsersUnitInfoReq{Uids: newMebs}))
	if err != nil {
		return err
	}

	uid2info, err := rpcResp.OkOrErr()
	if err != nil {
		return err
	}

	mebInfo := umap.ToSliWithMap(
		uid2info,
		func(_ string, val *rpcuser.UnitInfoRespItem) usermodel.UserUnitInfo {
			return usermodel.UserUnitInfo{
				Uid:      val.Uid,
				Nickname: val.Nickname,
				Avatar:   val.Avatar,
			}
		},
	)

	ctx, cancel := context.WithTimeout(context.Background(), 3000*time.Millisecond)
	defer cancel()

	grpNo := utils.RandStr(32)

	convId, uid2role, grpMebCnt, err := h.gr.CreateGroupChat(
		ctx,
		grouprepo.CreateGroupChatParam{
			GroupNo:     grpNo,
			GroupName:   req.GroupName,
			Avatar:      req.Avatar,
			OwnerUid:    req.OwnerUid,
			LimitedNum:  req.LimitedNum,
			MembersInfo: mebInfo,
		})

	if err != nil {
		return err
	}

	// 同步会话到内存
	h.cm.UpsertGroupChatConv(convId, grpNo, req.Avatar, req.Avatar, newMebs)

	// 群组管理
	h.gm.OnGroupCreated(grpNo, req.OwnerUid, uid2role)

	hintFmt, fmtItems := h.buildInviteInfoFmt(uid2info, newMebs, convId)

	inviteMsg := &msgmodel.LastMsg{
		MsgId: sfid.Next(),
		SenderInfo: msgmodel.SenderInfo{
			SenderType: chatmodel.SysCmdSender,
		},
		Content: msgmodel.BuildGroupInvitedCmdMsg(
			map[string]any{
				"inviteFmtItems": fmtItems,
				"groupMebCount":  grpMebCnt,
				"inviteHint":     hintFmt,
			},
		),
	}

	mills := time.Now().UnixMilli()
	// 通知会话更新
	// mq到engine_server, 通知客户端会话新增
	pd := convpd.ConvAddEventPayload{
		ConvId:         convId,
		ConvType:       chatconst.GroupConv,
		ChatType:       chatconst.GroupChat,
		Title:          req.GroupName,
		Ts:             mills,
		Members:        newMebs,
		MebId2UnitInfo: nil,
		Sender:         chatmodel.SysAutoSend,
		Receiver:       grpNo,
		RelationId:     grpNo,
		//FollowMsg:      inviteMsg,
	}

	lg = lg.With().Str("conv_id", convId).Logger()

	lg.Debug().Msgf("group created, conv add event will be published, pd=%+v", pd)

	h.ms.SendConvAddEvent(pd, lg)

	err = h.ms.SendMsg(
		&msgpd.MsgSendReceivePayload{
			ConvId:           convId,
			ConvLastActiveTs: mills,
			MsgId:            inviteMsg.MsgId,
			ClientUniqueId:   "",
			MsgSeq:           0,
			ChatType:         pd.ChatType,
			SenderType:       chatmodel.SysCmdSender,
			Sender:           pd.Sender,
			Receiver:         pd.Receiver,
			Members:          newMebs,
			ReqId:            "",
			SendTs:           mills,
			MsgContent:       inviteMsg.Content,
		},
		h.doneChan,
		[]any{
			convId,
			inviteMsg.MsgId,
			mills,
		},
	)

	if err != nil {
		return err
	}

	return c.JSON(wrapper.JustOk())
}

type GroupDataResp struct {
	GroupNo           string        `json:"groupNo,omitempty"`
	GroupName         string        `json:"groupName,omitempty"`
	GroupLimitedNum   int           `json:"groupLimitedNum,omitempty"`
	GroupMebCount     int           `json:"groupMebCount,omitempty"`
	GroupAnnouncement string        `json:"groupAnnouncement,omitempty"` // 群公告
	MembersInfo       []MebInfoItem `json:"membersInfo"`                 // 群成员信息
	GroupBak          string        `json:"groupBak,omitempty"`          // 群备注(仅自己可见)
	NicknameInGroup   string        `json:"nicknameInGroup,omitempty"`   // 在群中的昵称
}

type MebInfoItem struct {
	Id       int64  `json:"id,omitempty"`
	Uid      string `json:"uid,omitempty"`
	Nickname string `json:"nickname,omitempty"`
	Avatar   string `json:"avatar,omitempty"`
}

func (h *GroupHttpHandler) HandleFetchGroupData(c *fiber.Ctx) error {
	groupNo := c.Query("group_no")
	if groupNo == "" {
		return errors.New("group_no is required")
	}

	uid := c.Query("uid")
	if uid == "" {
		return errors.New("uid is required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	grp, err := h.gr.FindGroupInfo(ctx, groupNo)
	if err != nil {
		return err
	}

	items, err := h.gr.FindGroupItems(ctx, groupNo)
	if err != nil {
		return err
	}

	resp := &GroupDataResp{
		GroupNo:           grp.GroupNo,
		GroupName:         grp.GroupName,
		GroupLimitedNum:   int(grp.LimitedNum),
		GroupMebCount:     len(items),
		GroupAnnouncement: grp.Notice.String,
	}
	resp.MembersInfo = make([]MebInfoItem, 0, len(items))

	for _, item := range items {
		if item.Uid == uid {
			resp.GroupBak = item.Remark.String
			resp.NicknameInGroup = item.MebNickname.String
		}

		resp.MembersInfo = append(resp.MembersInfo, MebInfoItem{
			Id:       item.Id,
			Uid:      item.Uid,
			Nickname: item.MebNickname.String,
			Avatar:   item.MebAvatar.String,
		})
	}

	return c.JSON(wrapper.RespOk(resp))
}

func (h *GroupHttpHandler) HandlerSettingGroupName(c *fiber.Ctx) error {
	groupNo := c.Query("group_no")
	if groupNo == "" {
		return errors.New("groupNo is required")
	}

	groupName := c.Query("group_name")
	if groupName == "" {
		return errors.New("groupName is required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	ok, err := h.gr.SettingGroupName(ctx, groupNo, groupName)
	if err != nil {
		return err
	}

	// todo 修改会话title, 发送cmd消息, {0}修改群名称为:{groupName}

	if ok {
		return c.JSON(wrapper.JustOk())
	}

	return c.JSON(wrapper.JustGeneralErr())
}

func (h *GroupHttpHandler) buildInviteInfoFmt(uid2info map[string]*rpcuser.UnitInfoRespItem, members []string, convId string) (string, []chatmodel.UidNickname) {
	uidNicknames := make([]chatmodel.UidNickname, len(members))
	var appender strings.Builder
	appender.WriteString("{0}邀请")

	for idx, mebUid := range members {
		info, ok := uid2info[mebUid]
		nickname := info.Nickname
		uidNicknames[idx] = chatmodel.UidNickname{
			Uid:      mebUid,
			Nickname: nickname,
		}

		if ok && idx > 0 {
			appender.WriteString(fmt.Sprintf("{%d}", idx))
			if idx != len(members)-1 {
				appender.WriteString("丶")
			}
		}
	}
	appender.WriteString("加入了群聊")
	return appender.String(), uidNicknames
}

func (h *GroupHttpHandler) OnCreated(_ chan<- error) {

}

func (h *GroupHttpHandler) OnDispose(_ context.Context) error {
	if !h.closed.CompareAndSwap(false, true) {
		return nil
	}

	lg := mylog.AppLogger()
	lg.Debug().Msg("group http handler stop now")

	close(h.done)

	lg.Info().Msg("group http handler stopped successfully")

	return nil
}

func (h *GroupHttpHandler) receiveMqSendAsyncResult() {
	for {
		select {
		case <-h.done:
			return
		case pt, ok := <-h.doneChan:
			if !ok {
				return
			}

			convId, ok := pt.Args[0].(string)
			if !ok {
				continue
			}

			msgId, ok := pt.Args[1].(int64)
			if !ok {
				continue
			}

			lastTs, ok := pt.Args[2].(int64)
			if !ok {
				continue
			}

			lg := mylog.AppLogger()
			if pt.Error != nil {
				lg.Error().
					Stack().
					Str("conv_id", convId).
					Any("args", pt.Args).
					Err(pt.Error).
					Msg("group create, mq msg async send failed")
				continue
			}

			h.cm.UpdateGroupChatAfterCreatedEventSent(convId, msgId, lastTs)

			lg.Trace().Str("conv_id", convId).Int64("msg_id", msgId).Msg("group created, mq msg async send success")
		}
	}
}
