package core

import (
	"github.com/gammazero/deque"
	"github.com/sweemingdow/gmicro_pkg/pkg/graceful"
	"github.com/sweemingdow/sdim/external/eglobal/chatconst"
	"github.com/sweemingdow/sdim/external/eglobal/nsqconst/payload/msgpd"
	"github.com/sweemingdow/sdim/external/emodel/msgmodel"
	"github.com/sweemingdow/sdim/external/erpc/rpctopic"
	"time"
)

type MsgComingParam struct {
	ConvId         string
	Sender         string
	Receiver       string
	ChatType       chatconst.ChatType
	ReqId          string
	Ttl            int32
	ClientUniqueId string
	MsgContent     *msgmodel.MsgContent
	IsConvType     bool
}

type MsgComingResult struct {
	MsgId  int64
	ConvId string
	MsgSeq int64
	Err    error
}

func MsgComingParamFrom(req rpctopic.MsgComingReq, reqId string) MsgComingParam {
	return MsgComingParam{
		ConvId:         req.ConvId,
		Sender:         req.Sender,
		Receiver:       req.Receiver,
		ChatType:       req.ChatType,
		ReqId:          reqId,
		Ttl:            req.Ttl,
		ClientUniqueId: req.ClientUniqueId,
		MsgContent:     req.MsgContent,
	}
}

func MsgComingResultTo(mcr MsgComingResult, clientUniqueId string) rpctopic.MsgComingResp {
	return rpctopic.MsgComingResp{
		MsgId:          mcr.MsgId,
		ClientUniqueId: clientUniqueId,
		ConvId:         mcr.ConvId,
		MsgSeq:         mcr.MsgSeq,
		SendTs:         time.Now().UnixMilli(),
	}
}

type (
	Conversation struct {
		Id           string             // 会话id
		Type         chatconst.ConvType // 会话类型
		LastActiveTs int64
		Members      map[string]struct{} // 会话的所有成员(包含自己)
		MsgSeq       int64               // 会话内的全局递增消息序列号
		LastMsgId    int64
		LastMsg      *msgmodel.LastMsg                    // 维护会话的最近1条实时消
		RecentlyMsgs *deque.Deque[*msgmodel.MsgItemInMem] // 会话的最近n条消息
	}

	// 用户会话
	MemberConv struct {
		Id           string // 会话id
		Type         chatconst.ConvType
		Icon         string // 会话icon(单聊是对方的avatar, 群聊则是群头像)
		Title        string
		RelationId   string
		Remark       string
		PinTop       bool
		NoDisturb    bool
		BrowseMsgSeq int64 // 浏览到的位置(计算未读数)
		UnreadCount  int64 // 用户在该会话的未读数
		Cts          int64
		Uts          int64
	}

	// 用户会话包装
	MemberConvWrap struct {
		ConvItems   []*MemberConv          // 用户的最近活跃会话(自然顺序)
		ConvId2Item map[string]*MemberConv // id映射, 快速查找
	}
)

type ConvListItem struct {
	ConvId       string                   `json:"convId"`
	ConvType     chatconst.ConvType       `json:"convType"`
	Icon         string                   `json:"icon"`
	Title        string                   `json:"title"`
	RelationId   string                   `json:"relationId"`
	Remark       string                   `json:"remark"`
	PinTop       bool                     `json:"pinTop"`
	NoDisturb    bool                     `json:"noDisturb"`
	MsgSeq       int64                    `json:"msgSeq"`
	LastMsg      *msgmodel.LastMsg        `json:"lastMsg"`
	BrowseMsgSeq int64                    `json:"browseMsgSeq"`
	UnreadCount  int64                    `json:"unreadCount"`
	Cts          int64                    `json:"cts"`
	Uts          int64                    `json:"uts"`
	RecentlyMsgs []*msgmodel.MsgItemInMem `json:"recentlyMsgs"`
}

type MsgStoredResult struct {
	LastMsg         *msgmodel.LastMsg
	Uid2UnreadCount map[string]int64
	LastActiveTs    int64
}

// 会话管理器
type ConvManager interface {
	graceful.Gracefully

	OnMsgComing(pa MsgComingParam) MsgComingResult

	OnMsgStored(pd *msgpd.MsgForwardPayload) MsgStoredResult

	// 获取用户最近会话列表(只获取列表, 不包含消息)
	RecentlyConvList(uid string) []*ConvListItem

	// 同步用户最近会话列表, 包含每条会话的最近N条消息
	SyncHotConvList(uid string) []*ConvListItem

	ClearUnread(convId, uid string) error

	// 群聊会话
	UpsertGroupChatConv(convId, groupNo, icon, title string, members []string) bool

	// 群聊创建发送消息成功后
	UpdateGroupChatAfterCreatedEventSent(convId string, msgId, lastActiveTs int64)
}
