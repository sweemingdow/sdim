package hhttp

import (
	"context"
	"fmt"
	"github.com/gocraft/dbr/v2"
	"github.com/gofiber/fiber/v2"
	"github.com/nsqio/go-nsq"
	"github.com/pkg/errors"
	"github.com/sweemingdow/gmicro_pkg/pkg/component/cid/sfid"
	"github.com/sweemingdow/gmicro_pkg/pkg/component/cnsq"
	"github.com/sweemingdow/gmicro_pkg/pkg/component/csql"
	"github.com/sweemingdow/gmicro_pkg/pkg/executor"
	"github.com/sweemingdow/gmicro_pkg/pkg/myerr"
	"github.com/sweemingdow/gmicro_pkg/pkg/mylog"
	"github.com/sweemingdow/gmicro_pkg/pkg/server/shttp/apicall"
	"github.com/sweemingdow/gmicro_pkg/pkg/server/srpc/rpccall"
	"github.com/sweemingdow/gmicro_pkg/pkg/utils"
	"github.com/sweemingdow/gmicro_pkg/pkg/utils/umap"
	"github.com/sweemingdow/gmicro_pkg/pkg/utils/usli"
	"github.com/sweemingdow/sdim/external/eglobal/chatconst"
	"github.com/sweemingdow/sdim/external/eglobal/nsqconst"
	"github.com/sweemingdow/sdim/external/eglobal/nsqconst/payload/convpd"
	"github.com/sweemingdow/sdim/external/eglobal/nsqconst/payload/msgpd"
	"github.com/sweemingdow/sdim/external/eglobal/nsqconst/payload/notifypd"
	"github.com/sweemingdow/sdim/external/emodel/chatmodel"
	"github.com/sweemingdow/sdim/external/emodel/chatmodel/chatpojo"
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

const (
	groupHttpHandleLoggerName = "groupHttpHandleLogger"
)

type GroupHttpHandler struct {
	cm           core.ConvManager
	tm           csql.TransManager
	gm           core.GroupManager
	ms           *nsqsend.MsgSender
	gr           grouprepo.GroupRepository
	userProvider rpcuser.UserInfoRpcProvider
	done         chan struct{}
	doneChan     chan *nsq.ProducerTransaction
	closed       atomic.Bool
	executor     executor.Executor
	nsqPd        *cnsq.NsqProducer
	dl           *mylog.DecoLogger
}

func NewGroupHttpHandler(
	cm core.ConvManager,
	tm csql.TransManager,
	gr grouprepo.GroupRepository,
	gm core.GroupManager,
	ms *nsqsend.MsgSender,
	userProvider rpcuser.UserInfoRpcProvider,
	nsqPd *cnsq.NsqProducer,
) *GroupHttpHandler {
	ghh := &GroupHttpHandler{
		cm:           cm,
		tm:           tm,
		gm:           gm,
		gr:           gr,
		ms:           ms,
		userProvider: userProvider,
		doneChan:     make(chan *nsq.ProducerTransaction, 10),
		done:         make(chan struct{}),
		executor: executor.NewCallerRunExecutor(executor.CallerRunOptions{
			CoreWorkers:      2,
			MaxWorkers:       16,
			MaxWaitQueueSize: 512,
			MaxIdleTimeout:   30 * time.Second,
		}),
		dl:    mylog.NewDecoLogger(groupHttpHandleLoggerName),
		nsqPd: nsqPd,
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
func (h *GroupHttpHandler) HandleStartGroupChat(req StartGroupChatReq) (any, error) {
	if len(req.Members) == 0 {
		return nil, apicall.NewParamInvalidErr("can not create group without any members")
	}

	newMebs := usli.Distinct(req.Members)
	if len(newMebs) == 1 {
		if newMebs[0] == req.OwnerUid {
			return nil, apicall.NewParamInvalidErr("can not create group with self")
		}
	}

	newMebs = append([]string{req.OwnerUid}, newMebs...)

	if req.LimitedNum > 0 {
		if len(newMebs) > req.LimitedNum {
			return nil, apicall.NewParamInvalidErr("group members over limited num")
		}
	}

	h.dl.Debug().Msgf("handle start group chat, req=%+v", req)

	// rpc查用户信息
	rpcResp, err := h.userProvider.UsersUnitInfo(rpccall.CreateReq(rpcuser.UsersUnitInfoReq{Uids: newMebs}))
	if err != nil {
		return nil, err
	}

	uid2info, err := rpcResp.OkOrErr()
	if err != nil {
		return nil, err
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
		return nil, err
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

	lg := h.dl.GetLogger().With().Str("conv_id", convId).Logger()
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
		return nil, err
	}

	return nil, nil
}

type GroupDataResp struct {
	GroupNo           string              `json:"groupNo,omitempty"`
	GroupName         string              `json:"groupName,omitempty"`
	Role              chatmodel.GroupRole `json:"role,omitempty"`
	GroupLimitedNum   int                 `json:"groupLimitedNum,omitempty"`
	GroupMebCount     int                 `json:"groupMebCount,omitempty"`
	GroupAnnouncement string              `json:"groupAnnouncement,omitempty"` // 群公告
	MembersInfo       []MebInfoItem       `json:"membersInfo"`                 // 群成员信息
	GroupBak          string              `json:"groupBak,omitempty"`          // 群备注(仅自己可见)
	NicknameInGroup   string              `json:"nicknameInGroup,omitempty"`   // 在群中的昵称
}

type MebInfoItem struct {
	Id       int64               `json:"id,omitempty"`
	Uid      string              `json:"uid,omitempty"`
	Nickname string              `json:"nickname,omitempty"`
	Avatar   string              `json:"avatar,omitempty"`
	Role     chatmodel.GroupRole `json:"role,omitempty"`
}

func (h *GroupHttpHandler) HandleFetchGroupData(qm map[string]string) (*GroupDataResp, error) {
	groupNo := qm["group_no"]
	if groupNo == "" {
		return nil, apicall.NewParamInvalidErr("group_no is required")
	}

	uid := qm["uid"]
	if uid == "" {
		return nil, apicall.NewParamInvalidErr("uid is required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	grp, err := h.gr.FindGroupInfo(ctx, groupNo)
	if err != nil {
		if err == dbr.ErrNotFound {
			return nil, nil
		}

		return nil, err
	}

	items, err := h.gr.FindGroupItems(ctx, groupNo)
	if err != nil {
		return nil, err
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
			resp.Role = chatmodel.GroupRole(item.Role)
		}

		resp.MembersInfo = append(resp.MembersInfo, MebInfoItem{
			Id:       item.Id,
			Uid:      item.Uid,
			Nickname: item.MebNickname.String,
			Avatar:   item.MebAvatar.String,
			Role:     chatmodel.GroupRole(item.Role),
		})
	}

	return resp, nil
}

func (h *GroupHttpHandler) HandleSettingGroupName(qm map[string]string) (any, error) {
	groupNo := qm["group_no"]
	if groupNo == "" {
		return nil, apicall.NewParamInvalidErr("group_no is required")
	}

	groupName := qm["group_name"]
	if groupName == "" {
		return nil, apicall.NewParamInvalidErr("group_name is required")
	}

	uid := qm["uid"]
	if uid == "" {
		return nil, apicall.NewParamInvalidErr("uid is required")
	}

	lg := h.dl.GetLogger().With().Str("uid", uid).Logger()

	convId := chatmodel.GenerateGroupChatConvId(groupNo)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := h.gr.SettingGroupName(ctx, groupNo, groupName)
	if err != nil {
		return nil, err
	}

	groupUids, err := h.gr.FindGroupMebUids(ctx, groupNo)
	if err != nil {
		return nil, err
	}

	// 修改会话title
	h.cm.OnGroupDataChanged(core.OnGroupDataChangedParam{
		GroupNo:   groupNo,
		Uids:      groupUids,
		GroupName: &groupName,
	})

	err = h.executor.Submit(func() {
		resp, ie := h.userProvider.UserUnitInfo(rpccall.CreateReq(rpcuser.UserUnitInfoReq{Uid: uid}))
		if ie != nil {
			lg.Error().Err(ie).Msg("rpc qry user unit info failed")
			return
		}

		userInfo, ie := resp.OkOrErr()
		if ie != nil {
			lg.Error().Err(ie).Msg("rpc qry user unit info resp not ok")
			return
		}

		if userInfo.Uid == "" {
			lg.Warn().Msgf("user not exists, uid=%s", uid)
			return
		}

		mills := time.Now().UnixMilli()
		// 发送cmd消息, {0}修改群名称为:{groupName}
		groupSettingFmt, fmtItems := h.buildGroupSettingInfoFmt(*userInfo, groupName)
		settingMsg := &msgmodel.LastMsg{
			MsgId: sfid.Next(),
			SenderInfo: msgmodel.SenderInfo{
				SenderType: chatmodel.SysCmdSender,
			},
			Content: msgmodel.BuildGroupSettingCmdMsg(
				map[string]any{
					"settingFmtItems": fmtItems,
					"settingHint":     groupSettingFmt,
				},
			),
		}
		ie = h.ms.SendMsg(
			&msgpd.MsgSendReceivePayload{
				ConvId:           chatmodel.GenerateGroupChatConvId(groupNo),
				ConvLastActiveTs: mills,
				MsgId:            settingMsg.MsgId,
				MsgSeq:           0,
				ChatType:         chatconst.GroupChat,
				SenderType:       chatmodel.SysCmdSender,
				Sender:           chatmodel.SysAutoSend,
				Receiver:         groupNo,
				Members:          groupUids,
				SendTs:           mills,
				MsgContent:       settingMsg.Content,
			},
			h.doneChan,
			[]any{
				convId,
				settingMsg.MsgId,
				mills,
			},
		)

		if ie != nil {
			lg.Error().Err(ie).Msg("receive msg payload mq send failed")
			return
		}

		// 发送会话变更通知
		eventPd := convpd.ConvUnitDataUpdatePayload{
			ConvId:       convId,
			UpdateReason: chatconst.SomeOneModifyGroupName,
			Title:        &groupName,
			Members:      groupUids,
			Uts:          time.Now().UnixMilli(),
		}

		ie = h.nsqPd.JsonPublish(
			nsqconst.ConvUnitDataUpdateTopic,
			eventPd,
		)

		if ie != nil {
			lg.Error().Err(ie).Msg("conv unit data payload mq send failed")
			return
		}
	})

	return nil, nil
}

func (h *GroupHttpHandler) HandleSettingGroupBak(c *fiber.Ctx) error {
	groupNo := c.Query("group_no")
	if groupNo == "" {
		return errors.New("groupNo is required")
	}

	groupBak := c.Query("group_bak")
	if groupBak == "" {
		return errors.New("groupBak is required")
	}

	uid := c.Query("uid")
	if uid == "" {
		return errors.New("uid is required")
	}

	lg := h.dl.GetLogger().With().Str("uid", uid).Logger()

	convId := chatmodel.GenerateGroupChatConvId(groupNo)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	ok, err := h.gr.SettingGroupBak(ctx, uid, groupNo, groupBak)
	if err != nil {
		return err
	}

	mebUids := []string{uid}

	// 修改群资料
	h.cm.OnGroupDataChanged(core.OnGroupDataChangedParam{
		GroupNo: groupNo,
		Uids:    mebUids,
		Remark:  &groupBak,
	})

	err = h.executor.Submit(func() {
		// 发送会话变更通知
		eventPd := convpd.ConvUnitDataUpdatePayload{
			ConvId:       convId,
			Title:        &groupBak,
			UpdateReason: chatconst.UserActiveSettingGroupBak,
			Members:      mebUids,
			Uts:          time.Now().UnixMilli(),
		}

		ie := h.nsqPd.JsonPublish(
			nsqconst.ConvUnitDataUpdateTopic,
			eventPd,
		)

		if ie != nil {
			lg.Error().Err(ie).Msg("conv unit data payload mq send failed")
			return
		}
	})

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
		if ok {
			nickname := info.Nickname
			uidNicknames[idx] = chatmodel.UidNickname{
				Uid:      mebUid,
				Nickname: nickname,
			}
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

// {0}修改群名称为:{groupName}
func (h *GroupHttpHandler) buildGroupSettingInfoFmt(modifyUserInfo rpcuser.UnitInfoRespItem, groupName string) (string, []chatmodel.UidNickname) {
	uidNicknames := []chatmodel.UidNickname{
		{
			Uid:      modifyUserInfo.Uid,
			Nickname: modifyUserInfo.Nickname,
		},
	}

	fmtInfo := fmt.Sprintf("{0}修改群名称为 %s", groupName)

	return fmtInfo, uidNicknames
}

func (h *GroupHttpHandler) buildRemoveInfoFmt(uid2info map[string]*rpcuser.UnitInfoRespItem, members []string) (string, []chatmodel.UidNickname) {
	uidNicknames := make([]chatmodel.UidNickname, len(members))
	var appender strings.Builder
	appender.WriteString("{0}将")

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
	appender.WriteString("移出了群聊")
	return appender.String(), uidNicknames
}

func (h *GroupHttpHandler) OnCreated(_ chan<- error) {

}

func (h *GroupHttpHandler) OnDispose(ctx context.Context) error {
	if !h.closed.CompareAndSwap(false, true) {
		return nil
	}

	lg := mylog.GetStopMarkLogger()
	lg.Debug().Msg("group http handler stop now")

	close(h.done)

	err := h.executor.Shutdown(ctx)
	if err != nil {
		return err
	}

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

			if pt.Error != nil {
				h.dl.Error().
					Stack().
					Str("conv_id", convId).
					Any("args", pt.Args).
					Err(pt.Error).
					Msg("group create, mq msg async send failed")
				continue
			}

			h.cm.UpdateGroupChatAfterCreatedEventSent(convId, msgId, lastTs)

			//h.dl.Trace().Str("conv_id", convId).Int64("msg_id", msgId).Msg("group created, mq msg async send success")
		}
	}
}

func (h *GroupHttpHandler) HandleSettingGroupNickname(c *fiber.Ctx) error {
	groupNo := c.Query("group_no")
	if groupNo == "" {
		return errors.New("groupNo is required")
	}

	nickname := c.Query("nickname")

	uid := c.Query("uid")
	if uid == "" {
		return errors.New("uid is required")
	}

	lg := h.dl.GetLogger().With().Str("uid", uid).Logger()

	convId := chatmodel.GenerateGroupChatConvId(groupNo)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	ok, err := h.gr.SettingGroupNickname(ctx, uid, groupNo, nickname)
	if err != nil {
		return err
	}

	err = h.executor.Submit(func() {
		mebUids := h.gm.GetGroupMebUids(context.Background(), groupNo)

		if len(mebUids) == 0 {
			return
		}

		// 用户修改群内昵称通知
		pd := notifypd.NotifyPayload{
			NotifyType: chatconst.GroupNotifyType,
			SubType:    chatconst.SettingNicknameInGroup,
			Members:    mebUids,
			Data: map[string]any{
				"convId":      convId,
				"groupNo":     groupNo,
				"modifier":    uid,
				"newNickname": nickname,
			},
		}

		ie := h.nsqPd.JsonPublish(
			nsqconst.SrvNotifyTopic,
			pd,
		)

		if ie != nil {
			lg.Error().Err(ie).Msg("notify payload mq send failed")
			return
		}
	})

	if ok {
		return c.JSON(wrapper.JustOk())
	}

	return c.JSON(wrapper.JustGeneralErr())
}

type AddRemMembersReq struct {
	Uid     string   `json:"uid,omitempty"`
	GroupNo string   `json:"groupNo,omitempty"`
	Members []string `json:"members,omitempty"`
}

func (h *GroupHttpHandler) HandleAddMembers(req AddRemMembersReq) (any, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	h.dl.Info().Msgf("add group members, req=%+v", req)

	if len(req.Members) == 0 {
		return nil, apicall.NewParamInvalidErr("members is required")
	}

	// rpc查询用户信息
	newMebs := append([]string{req.Uid}, req.Members...)
	usersResp, err := h.userProvider.UsersUnitInfo(rpccall.CreateReq(rpcuser.UsersUnitInfoReq{Uids: newMebs}))
	if err != nil {
		return nil, myerr.NewRpcCallError(err)
	}

	rd, re := usersResp.OkOrErr()
	if re != nil {
		return nil, re
	}

	mebInfo := umap.ToSliWithMap(
		rd,
		func(_ string, val *rpcuser.UnitInfoRespItem) usermodel.UserUnitInfo {
			return usermodel.UserUnitInfo{
				Uid:      val.Uid,
				Nickname: val.Nickname,
				Avatar:   val.Avatar,
			}
		},
	)

	var groupInfo *chatpojo.Group
	err = h.tm.DoInTx(
		ctx,
		func(_ context.Context, tx *dbr.Tx) error {
			// 查询操作者是否有权限
			role, ie := h.gr.GetRoleInGroup(ctx, req.GroupNo, req.Uid, tx)
			if ie != nil {
				if errors.Is(ie, dbr.ErrNotFound) {
					return apicall.NewNoPermissionErr("")
				}

				return ie
			}
			if role > chatmodel.Manager {
				return apicall.NewNoPermissionErr("")
			}

			_, ie = h.gr.AddMembers(ctx, tx, req.Uid, req.GroupNo, mebInfo)
			if ie != nil {
				return ie
			}

			// todo 会话处理

			groupInfo, ie = h.gr.FindGroupInfo(ctx, req.GroupNo)
			if ie != nil {
				return ie
			}

			return nil
		})

	if err != nil {
		return nil, err
	}

	// 添加新成员
	groupMebCount, err := h.gm.OnGroupMebAdded(ctx, req.GroupNo, mebInfo)
	if err != nil {
		return nil, err
	}

	err = h.executor.Submit(func() {
		convId := chatmodel.GenerateGroupChatConvId(req.GroupNo)
		hintFmt, fmtItems := h.buildInviteInfoFmt(rd, newMebs, convId)

		inviteMsg := &msgmodel.LastMsg{
			MsgId: sfid.Next(),
			SenderInfo: msgmodel.SenderInfo{
				SenderType: chatmodel.SysCmdSender,
			},
			Content: msgmodel.BuildGroupInvitedCmdMsg(
				map[string]any{
					"inviteFmtItems": fmtItems,
					"groupMebCount":  groupMebCount,
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
			Title:          groupInfo.GroupName,
			Ts:             mills,
			Members:        newMebs,
			MebId2UnitInfo: nil,
			Sender:         chatmodel.SysAutoSend,
			Receiver:       req.GroupNo,
			RelationId:     req.GroupNo,
			//FollowMsg:      inviteMsg,
		}

		lg := h.dl.GetLogger().With().Str("conv_id", convId).Logger()
		lg.Debug().Msgf("group members added, conv add event will be published, pd=%+v", pd)

		h.ms.SendConvAddEvent(pd, lg)

		receivePd := &msgpd.MsgSendReceivePayload{
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
		}

		ie := h.nsqPd.JsonPublish(
			nsqconst.MsgReceiveTopic,
			receivePd,
		)

		if ie != nil {
			lg.Error().Err(ie).Msgf("publish receive payload failed after group meb removed, pd=%+v", *receivePd)
		}

		// 发送通知消息
		notifyPd := notifypd.NotifyPayload{
			NotifyType: chatconst.GroupNotifyType,
			SubType:    chatconst.GroupAddMembers,
			Members:    newMebs,
			Data: map[string]any{
				"convId":    chatmodel.GenerateGroupChatConvId(req.GroupNo),
				"groupNo":   req.GroupNo,
				"addedUids": req.Members,
			},
		}

		ie = h.nsqPd.JsonPublish(
			nsqconst.SrvNotifyTopic,
			notifyPd,
		)

		if ie != nil {
			h.dl.Error().Err(ie).Msgf("publish notify payload failed after group meb removed, pd=%+v", notifyPd)
			return
		}
	})

	if err != nil {
		h.dl.Error().Err(err).Msg("group add members task submit failed")
		// ignore this error
		return nil, nil
	}

	return nil, nil
}

func (h *GroupHttpHandler) HandleRemoveMembers(req AddRemMembersReq) (any, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var remUids []string
	err := h.tm.DoInTx(
		ctx,
		func(_ context.Context, tx *dbr.Tx) error {
			// 查询操作者是否有权限
			role, ie := h.gr.GetRoleInGroup(ctx, req.GroupNo, req.Uid, tx)
			if ie != nil {
				return ie
			}
			if role > chatmodel.Manager {
				return errors.New("no permission")
			}

			remUids, ie = h.gr.RemoveMembers(ctx, tx, req.GroupNo, req.Members)
			if ie != nil {
				return ie
			}

			return nil
		})

	if err != nil {
		return nil, err
	}

	if len(remUids) == 0 {
		return nil, nil
	}

	// 更新群内成员状态
	h.gm.OnGroupMebRemoved(req.GroupNo, remUids)

	err = h.executor.Submit(func() {
		newMebs := append([]string{req.Uid}, req.Members...)
		// rpc查用户信息
		rpcResp, ie := h.userProvider.UsersUnitInfo(rpccall.CreateReq(rpcuser.UsersUnitInfoReq{Uids: newMebs}))
		if ie != nil {
			h.dl.Error().Err(ie).Msg("rpc qry users unit info failed")
			return
		}

		uid2info, ie := rpcResp.OkOrErr()
		if ie != nil {
			h.dl.Error().Err(ie).Msg("rpc qry users unit info resp not ok")
			return
		}

		mills := time.Now().UnixMilli()
		// 发送cmd消息, {0}修改群名称为:{groupName}
		groupRemFmt, fmtItems := h.buildRemoveInfoFmt(uid2info, newMebs)
		remMsg := &msgmodel.LastMsg{
			MsgId: sfid.Next(),
			SenderInfo: msgmodel.SenderInfo{
				SenderType: chatmodel.SysCmdSender,
			},
			Content: msgmodel.BuildGroupRemoveCmdMsg(
				map[string]any{
					"groupRemItems": fmtItems,
					"remHint":       groupRemFmt,
				},
			),
		}
		// 发送receivePayload
		receivePd := &msgpd.MsgSendReceivePayload{
			ConvId:           chatmodel.GenerateGroupChatConvId(req.GroupNo),
			ConvLastActiveTs: mills,
			MsgId:            remMsg.MsgId,
			MsgSeq:           0,
			ChatType:         chatconst.GroupChat,
			SenderType:       chatmodel.SysCmdSender,
			Sender:           chatmodel.SysAutoSend,
			Receiver:         req.GroupNo,
			Members:          newMebs,
			SendTs:           mills,
			MsgContent:       remMsg.Content,
		}

		ie = h.nsqPd.JsonPublish(
			nsqconst.MsgReceiveTopic,
			receivePd,
		)

		if ie != nil {
			h.dl.Error().Err(ie).Msg("publish receive payload failed")
			return
		}

		// 发送通知消息
		pd := notifypd.NotifyPayload{
			NotifyType: chatconst.GroupNotifyType,
			SubType:    chatconst.GroupRemoveMembers,
			Members:    newMebs,
			Data: map[string]any{
				"convId":      chatmodel.GenerateGroupChatConvId(req.GroupNo),
				"groupNo":     req.GroupNo,
				"removedUids": remUids,
			},
		}

		ie = h.nsqPd.JsonPublish(
			nsqconst.SrvNotifyTopic,
			pd,
		)

		if ie != nil {
			h.dl.Error().Err(ie).Msg("publish notify payload failed")
			return
		}
	})

	if err != nil {
		h.dl.Error().Err(err).Msg("group remove members failed")
		return nil, err
	}

	return nil, nil
}
