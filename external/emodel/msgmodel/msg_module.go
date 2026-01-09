package msgmodel

import (
	"github.com/sweemingdow/sdim/external/eglobal/chatconst"
)

type MsgType uint16

const (
	TextType MsgType = 1
)

type SenderInfo struct {
	Nickname string `json:"nickname,omitempty"`
	Avatar   string `json:"avatar,omitempty"`
}

type MsgItemInMem struct {
	MsgId      int64              `json:"msgId"`
	ConvId     string             `json:"convId"`
	Sender     string             `json:"sender"`
	Receiver   string             `json:"receiver"`
	ChatType   chatconst.ChatType `json:"chatType"`
	MsgType    MsgType            `json:"msgType"`
	Content    *MsgContent        `json:"content"`
	SenderInfo SenderInfo         `json:"senderInfo"`
	MegSeq     int64              `json:"megSeq"`
	Cts        int64              `json:"cts"`
}

type MsgContent struct {
	Type    MsgType        `json:"type"`    // 消息类型
	Content map[string]any `json:"content"` // 消息内容
	Custom  map[string]any `json:"custom"`  // 自定义内容
	Extra   map[string]any `json:"extra"`   // extra内容
}

func ValidateMsgContent(mc *MsgContent) error {
	return nil
}
