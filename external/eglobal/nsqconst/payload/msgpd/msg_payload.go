package msgpd

import (
	"github.com/sweemingdow/sdim/external/eglobal/chatconst"
	"github.com/sweemingdow/sdim/external/emodel/msgmodel"
)

type MsgSendReceivePayload struct {
	ConvId           string               `json:"convId,omitempty"`
	MsgId            int64                `json:"msgId,omitempty"`
	ConvLastActiveTs int64                `json:"convLastActiveTs,omitempty"` // 会话最后的活跃时间(最后一条消息的时间)
	ClientUniqueId   string               `json:"clientUniqueId,omitempty"`   // 客户端唯一id
	MsgSeq           int64                `json:"msgSeq,omitempty"`
	ChatType         chatconst.ChatType   `json:"chatType,omitempty"`
	Sender           string               `json:"sender,omitempty"`
	Receiver         string               `json:"receiver,omitempty"`
	Members          []string             `json:"members,omitempty"`
	ReqId            string               `json:"reqId,omitempty"`
	SendTs           int64                `json:"sendTs,omitempty"`
	Ttl              int32                `json:"ttl,omitempty"`
	MsgContent       *msgmodel.MsgContent `json:"payload,omitempty"`
}

type MsgForwardPayload struct {
	ReqId            string             `json:"reqId,omitempty"`
	ConvId           string             `json:"convId,omitempty"`
	ConvLastActiveTs int64              `json:"convLastActiveTs,omitempty"`
	MsgId            int64              `json:"msgId,omitempty"`
	ClientUniqueId   string             `json:"clientUniqueId,omitempty"` // 客户端唯一id
	ChatType         chatconst.ChatType `json:"chatType,omitempty"`
	Sender           string             `json:"sender,omitempty"`
	Members          []string           `json:"members,omitempty"`
	SendTs           int64              `json:"sendTs,omitempty"`
	Ttl              int32              `json:"ttl,omitempty"`
	MsgBody          *StorageMsgBody    `json:"payload,omitempty"`
}

type (
	SenderInfo struct {
		Nickname string `json:"nickname,omitempty"`
		Avatar   string `json:"avatar,omitempty"`
	}

	StorageMsgBody struct {
		SenderInfo SenderInfo           `json:"senderInfo,omitempty"`
		Content    *msgmodel.MsgContent `json:"payload,omitempty"`
	}
)

func ReceivePd2forwardPd(rpd *MsgSendReceivePayload, body *StorageMsgBody) *MsgForwardPayload {
	return &MsgForwardPayload{
		ReqId:            rpd.ReqId,
		ConvId:           rpd.ConvId,
		ConvLastActiveTs: rpd.ConvLastActiveTs,
		MsgId:            rpd.MsgId,
		ClientUniqueId:   rpd.ClientUniqueId,
		ChatType:         rpd.ChatType,
		Sender:           rpd.Sender,
		Members:          rpd.Members,
		SendTs:           rpd.SendTs,
		Ttl:              rpd.Ttl,
		MsgBody:          body,
	}
}
