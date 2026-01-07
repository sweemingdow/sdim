package msgmodel

type MsgType uint16

const (
	TextType MsgType = 1
)

type MsgContent struct {
	Type    MsgType        `json:"type,omitempty"`    // 消息类型
	Content map[string]any `json:"content,omitempty"` // 消息内容
	Custom  map[string]any `json:"custom,omitempty"`  // 自定义内容
	Extra   map[string]any `json:"extra,omitempty"`   // extra内容
}

func ValidateMsgContent(mc *MsgContent) error {
	return nil
}
