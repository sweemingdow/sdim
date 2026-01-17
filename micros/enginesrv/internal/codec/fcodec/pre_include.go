package fcodec

import (
	"github.com/sweemingdow/sdim/external/eglobal/chatconst"
	"github.com/sweemingdow/sdim/external/emodel/msgmodel"
	"github.com/sweemingdow/sdim/pkg/constt"
)

type (
	ConnFrame struct {
		Uid     string            `json:"uid,omitempty"`
		CType   constt.ClientType `json:"ctype,omitempty"`
		TsMills int64             `json:"tsMills,omitempty"`
	}

	ConnAckFrame struct {
		ErrCode  ErrCode `json:"errCode,omitempty"`
		ErrDesc  string  `json:"errDesc,omitempty"`
		TimeDiff int64   `json:"timeDiff"` // 客户端和服务器之间的时差
	}
)

type (
	ErrCode uint32

	SendFrame struct {
		Sender         string               `json:"sender,omitempty"`   // 发送者uid
		Receiver       string               `json:"receiver,omitempty"` // 接收者, 单聊是对方的uid, 群聊是群id
		ChatType       chatconst.ChatType   `json:"chatType,omitempty"`
		SendMills      int64                `json:"sendMills,omitempty"`
		Sign           string               `json:"sign,omitempty"`           // 消息签名, 防纂改
		Ttl            int32                `json:"ttl,omitempty"`            // 消息过期时间(sec), -1:阅后即焚,0:不过期
		ClientUniqueId string               `json:"clientUniqueId,omitempty"` // 客户端唯一id
		MsgContent     *msgmodel.MsgContent `json:"msgContent,omitempty"`
	}

	SendFrameAck struct {
		ErrCode ErrCode          `json:"errCode,omitempty"`
		ErrDesc string           `json:"errDesc,omitempty"`
		Data    SendFrameAckBody `json:"data,omitempty"`
	}

	SendFrameAckBody struct {
		ClientUniqueId string `json:"clientUniqueId,omitempty"` // 客户端唯一id
		MsgId          int64  `json:"msgId,omitempty"`          // 消息id
		ConvId         string `json:"convId,omitempty"`         // 会话id
		MsgSeq         int64  `json:"msgSeq,omitempty"`         // 消息序列号
		SendTs         int64  `json:"sendTs,omitempty"`         // 服务器时间戳
	}
)

type (
	ForwardFrameBody struct {
		ConvId           string               `json:"convId,omitempty"`
		ConvLastActiveTs int64                `json:"convLastActiveTs,omitempty"`
		MsgId            int64                `json:"msgId,omitempty"`
		ClientUniqueId   string               `json:"clientUniqueId,omitempty"` // 客户端唯一id
		MsgSeq           int64                `json:"msgSeq,omitempty"`
		ChatType         chatconst.ChatType   `json:"chatType,omitempty"`
		Sender           string               `json:"sender,omitempty"`
		Receiver         string               `json:"receiver,omitempty"`
		ToUid            string               `json:"toUid,omitempty"`
		SendTs           int64                `json:"sendTs,omitempty"`
		MsgContent       *msgmodel.MsgContent `json:"msgContent,omitempty"`
		SenderInfo       msgmodel.SenderInfo  `json:"senderInfo,omitempty"`
	}

	ForwardFrameAckBody struct {
	}
)

type (
	ConvUpdateFrame struct {
		ConvId string         `json:"convId"`
		Type   string         `json:"type"` // 会话更新类型
		Data   map[string]any `json:"data"` // 携带的数据
	}
)

const (
	OK         ErrCode = 0
	BizErr             = 1000
	ServerErr  ErrCode = 2000
	RpcRespErr ErrCode = 3000
)

type ErrItem struct {
	ErrCode ErrCode
	Desc    string
}

var errCode2desc = map[ErrCode]string{
	ServerErr:  "Server Internal Error",
	RpcRespErr: "Inner Response Error",
}

func TransferDesc(ec ErrCode) string {
	return errCode2desc[ec]
}
