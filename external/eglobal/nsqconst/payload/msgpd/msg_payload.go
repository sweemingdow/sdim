package msgpd

import (
	"github.com/sweemingdow/sdim/external/eglobal/chatconst"
	"github.com/sweemingdow/sdim/external/emodel/msgmodel"
)

type MsgSendReceivePayload struct {
	ConvId     string               `json:"convId,omitempty"`
	MsgId      int64                `json:"msgId,omitempty"`
	MsgSeq     int64                `json:"msgSeq,omitempty"`
	ChatType   chatconst.ChatType   `json:"chatType,omitempty"`
	Sender     string               `json:"sender,omitempty"`
	Receiver   string               `json:"receiver,omitempty"`
	Members    []string             `json:"members,omitempty"`
	ReqId      string               `json:"reqId,omitempty"`
	SendTs     int64                `json:"sendTs,omitempty"`
	Ttl        int32                `json:"ttl,omitempty"`
	MsgContent *msgmodel.MsgContent `json:"payload,omitempty"`
}

type MsgForwardPayload struct {
	ReqId    string             `json:"reqId,omitempty"`
	ConvId   string             `json:"convId,omitempty"`
	MsgId    int64              `json:"msgId,omitempty"`
	ChatType chatconst.ChatType `json:"chatType,omitempty"`
	Sender   string             `json:"sender,omitempty"`
	Members  []string           `json:"members,omitempty"`
	SendTs   int64              `json:"sendTs,omitempty"`
	Ttl      int32              `json:"ttl,omitempty"`
	MsgBody  *StorageMsgBody    `json:"payload,omitempty"`
}

type (
	SenderInfo struct {
		SendNickname string `json:"sendNickname,omitempty"`
		SendAvatar   string `json:"sendAvatar,omitempty"`
	}

	StorageMsgBody struct {
		SenderInfo SenderInfo           `json:"senderInfo,omitempty"`
		Content    *msgmodel.MsgContent `json:"payload,omitempty"`
	}
)

func ReceivePd2forwardPd(rpd *MsgSendReceivePayload, body *StorageMsgBody) *MsgForwardPayload {
	return &MsgForwardPayload{
		ReqId:    rpd.ReqId,
		ConvId:   rpd.ConvId,
		MsgId:    rpd.MsgId,
		ChatType: rpd.ChatType,
		Sender:   rpd.Sender,
		Members:  rpd.Members,
		SendTs:   rpd.SendTs,
		Ttl:      rpd.Ttl,
		MsgBody:  body,
	}
}
