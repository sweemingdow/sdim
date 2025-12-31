package core

import (
	"github.com/sweemingdow/sdim/external/eglobal/chatconst"
	"github.com/sweemingdow/sdim/external/emodel/msgmodel"
	"sync/atomic"
)

type (
	Conversation struct {
		Id          string                              // 会话id
		Type        chatconst.ConvType                  // 会话类型
		Members     []string                            // 会话的所有成员(包含自己)
		msgSeq      atomic.Uint64                       // 会话内的全局递增消息序列号
		recentlyMsg atomic.Pointer[msgmodel.MsgContent] // 维护会话的最近1条实时消息
	}

	// 用户会话
	MemberConv struct {
		Id           string // 会话id
		Type         chatconst.ConvType
		Icon         string // 会话icon(单聊是对方的avatar, 群聊则是群头像)
		Title        string
		RelationUid  string
		Remark       string
		PinTop       bool
		NoDisturb    bool
		BrowseMsgSeq uint32 // 浏览到的位置(计算未读数)
		UnreadCount  uint32 // 用户在该会话的未读数
		Cts          int64
		Uts          int64
	}

	// 用户会话包装
	MemberConvWrap struct {
		ConvItems   []*MemberConv          // 用户的最近活跃会话(自然顺序)
		ConvId2Item map[string]*MemberConv // id映射, 快速查找
	}
)

func (c *Conversation) SetMsgSeq(seq int64) {
	c.msgSeq.Store(uint64(seq))
}

func (c *Conversation) CurrMsqSeq() uint64 {
	return c.msgSeq.Load()
}

func (c *Conversation) ResetMsgSeq(val uint64) {
	c.msgSeq.Store(val)
}

func (c *Conversation) SetRecentlyMsg(rcm *msgmodel.MsgContent) {
	c.recentlyMsg.Store(rcm)
}

func (c *Conversation) GetRecentlyMsg() *msgmodel.MsgContent {
	return c.recentlyMsg.Load()
}

//func (c *Conversation) SetRecentlyMsg() {
//	c.msgSeq.Add(1)
//}
//
//func (c *Conversation) CurrMsqSeq() uint32 {
//	return c.msgSeq.Load()
//}
