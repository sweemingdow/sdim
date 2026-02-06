package msgmodel

import (
	"github.com/sweemingdow/sdim/external/eglobal/chatconst"
	"github.com/sweemingdow/sdim/external/emodel/chatmodel"
)

type MsgType uint16

const (
	// chat
	// text
	TextType MsgType = 1

	// image
	ImageType MsgType = 2

	// custom
	CustomType MsgType = 100

	// cmd
	CmdType MsgType = 1000
)

type SubCmdType uint16

const (
	// 邀请入群
	SubCmdGroupInvited SubCmdType = 1001

	// 设置群名称
	SubCmdGroupSettingName SubCmdType = 1002

	// 添加到群聊
	SubCmdGroupAddMembers SubCmdType = 1003

	// 移出群聊
	SubCmdGroupRemoveMembers SubCmdType = 1004
)

type SenderInfo struct {
	SenderType chatmodel.SenderType `json:"senderType"`
	Nickname   string               `json:"nickname,omitempty"`
	Avatar     string               `json:"avatar,omitempty"`
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

type LastMsg struct {
	MsgId      int64       `json:"msgId"`
	SenderInfo SenderInfo  `json:"senderInfo"`
	Content    *MsgContent `json:"content"`
}

func ValidateMsgContent(mc *MsgContent) error {
	return nil
}

func BuildGroupInvitedCmdMsg(inviteContent map[string]any) *MsgContent {
	return &MsgContent{
		Type: CmdType,
		Content: map[string]any{
			"subCmd":        SubCmdGroupInvited,
			"inviteContent": inviteContent,
		},
	}
}

func BuildGroupSettingCmdMsg(settingContent map[string]any) *MsgContent {
	return &MsgContent{
		Type: CmdType,
		Content: map[string]any{
			"subCmd":         SubCmdGroupSettingName,
			"settingContent": settingContent,
		},
	}
}

func BuildGroupRemoveCmdMsg(removeContent map[string]any) *MsgContent {
	return &MsgContent{
		Type: CmdType,
		Content: map[string]any{
			"subCmd":        SubCmdGroupRemoveMembers,
			"removeContent": removeContent,
		},
	}
}
