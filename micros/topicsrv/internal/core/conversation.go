package core

import (
	"github.com/sweemingdow/sdim/external/eglobal/chatconst"
	"github.com/sweemingdow/sdim/external/emodel/msgmodel"
)

type (
	Conversation struct {
		Id           string             // 会话id
		Type         chatconst.ConvType // 会话类型
		LastActiveTs int64
		Members      []string // 会话的所有成员(包含自己)
		MsgSeq       int64    // 会话内的全局递增消息序列号
		LastMsgId    int64
		LastMsg      *msgmodel.MsgContent // 维护会话的最近1条实时消息
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

//func (c *Conversation) SetRecentlyMsg() {
//	c.msgSeq.Add(1)
//}
//
//func (c *Conversation) CurrMsqSeq() uint32 {
//	return c.msgSeq.Load()
//}
