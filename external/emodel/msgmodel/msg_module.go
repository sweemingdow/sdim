package msgmodel

import "fmt"

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

func ParseMsgContent(mb any) (*MsgContent, error) {
	m, ok := mb.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("invalid msg:%+v", mb)
	}

	cm := new(MsgContent)

	if val, ok := m["type"]; !ok {
		return nil, fmt.Errorf("invaid msg, no type specified")
	} else {
		t, ok := val.(float64)
		if !ok {
			return nil, fmt.Errorf("invaid msg, type invalid:%v", val)
		}

		cm.Type = MsgType(t)
	}

	if val, ok := m["content"]; !ok {
		return nil, fmt.Errorf("invaid msg, no content specified")
	} else {
		c, ok := val.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("invaid msg, cotnent invalid:%v", val)
		}

		cm.Content = c
	}

	if val, ok := m["custom"]; ok {
		c, ok := val.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("invaid msg, custom invalid:%v", val)
		}

		cm.Custom = c
	}

	if val, ok := m["extra"]; ok {
		c, ok := val.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("invaid msg, extra invalid:%v", val)
		}

		cm.Extra = c
	}

	return cm, nil
}
