package hhttp

import (
	"context"
	"errors"
	"github.com/gofiber/fiber/v2"
	"github.com/nsqio/go-nsq"
	"github.com/sweemingdow/gmicro_pkg/pkg/component/cid/sfid"
	"github.com/sweemingdow/gmicro_pkg/pkg/mylog"
	"github.com/sweemingdow/gmicro_pkg/pkg/parser/json"
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
func (ghh *GroupHttpHandler) HandleStartGroupChat(c *fiber.Ctx) error {
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

	newMebs = append(newMebs, req.OwnerUid)

	if req.LimitedNum > 0 {
		if len(newMebs) > req.LimitedNum {
			return errors.New("group members over limited num")
		}
	}

	lg := mylog.AppLogger()

	lg.Debug().Msgf("handle start group chat, req=%+v", req)

	// rpc查用户信息
	rpcResp, err := ghh.userProvider.UsersUnitInfo(rpccall.CreateReq(rpcuser.UsersUnitInfoReq{Uids: newMebs}))
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

	convId, uid2role, grpMebCnt, err := ghh.gr.CreateGroupChat(
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
	ghh.cm.UpsertGroupChatConv(convId, grpNo, req.Avatar, req.Avatar, newMebs)

	// 群组管理
	ghh.gm.OnGroupCreated(grpNo, req.OwnerUid, uid2role)

	inviteMsg := &msgmodel.LastMsg{
		MsgId: sfid.Next(),
		SenderInfo: msgmodel.SenderInfo{
			SenderType: chatmodel.SysCmdSender,
		},
		Content: msgmodel.BuildGroupInvitedCmdMsg(
			map[string]any{
				"inviteInfo": map[string]any{
					"uid":      req.OwnerUid,
					"nickname": uid2info[req.OwnerUid].Nickname,
				},
				"groupMebCount": grpMebCnt,
				"inviteHint":    ghh.buildInviteHint(uid2info, req.Members),
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
		Sender:         chatmodel.SysSendUser,
		Receiver:       grpNo,
		RelationId:     grpNo,
		FollowMsg:      inviteMsg,
	}

	lg = lg.With().Str("conv_id", convId).Logger()

	lg.Debug().Msgf("group created, conv add event will be published, pd=%+v", pd)

	ghh.ms.SendConvAddEvent(pd, lg)

	err = ghh.ms.SendMsg(
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
		ghh.doneChan,
		[]any{
			convId,
			inviteMsg.MsgId,
			mills,
		},
	)

	if err != nil {
		return err
	}

	resp := wrapper.JustOk()
	bodies, err := json.Fmt(resp)
	if err != nil {
		return err
	}

	return c.Send(bodies)
}

func (ghh *GroupHttpHandler) buildInviteHint(uid2info map[string]*rpcuser.UnitInfoRespItem, members []string) string {
	var appender strings.Builder
	appender.WriteString("{0}邀请")
	for idx, mebUid := range members {
		if info, ok := uid2info[mebUid]; ok {
			appender.WriteString(info.Nickname)

			if idx != len(members)-1 {
				appender.WriteString("丶")
			}
		}
	}
	appender.WriteString("加入了群聊")
	return appender.String()
}

func (ghh *GroupHttpHandler) OnCreated(_ chan<- error) {

}

func (ghh *GroupHttpHandler) OnDispose(_ context.Context) error {
	if !ghh.closed.CompareAndSwap(false, true) {
		return nil
	}

	lg := mylog.AppLogger()
	lg.Debug().Msg("group http handler stop now")

	close(ghh.done)

	lg.Info().Msg("group http handler stopped successfully")

	return nil
}

func (ghh *GroupHttpHandler) receiveMqSendAsyncResult() {
	for {
		select {
		case <-ghh.done:
			return
		case pt, ok := <-ghh.doneChan:
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

			ghh.cm.UpdateGroupChatAfterCreatedEventSent(convId, msgId, lastTs)

			lg.Trace().Str("conv_id", convId).Int64("msg_id", msgId).Msg("group created, mq msg async send success")
		}
	}
}
