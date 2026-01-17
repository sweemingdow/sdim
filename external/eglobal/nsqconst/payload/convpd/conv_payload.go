package convpd

import (
	"github.com/sweemingdow/sdim/external/eglobal/chatconst"
	"github.com/sweemingdow/sdim/external/emodel/msgmodel"
)

type ConvLastMsgUpdateEventPayload struct {
	ConvId          string            `json:"convId,omitempty"`
	Members         []string          `json:"members,omitempty"`
	LastMsg         *msgmodel.LastMsg `json:"lastMsg,omitempty"`
	LastActiveTs    int64             `json:"lastActiveTs,omitempty"`
	Uid2UnreadCount map[string]int64  `json:"uid2unreadCount,omitempty"`
}

type MemberUnitInfo struct {
	Icon  string
	Title string
}

type ConvAddEventPayload struct {
	ConvId         string                    `json:"convId,omitempty"`
	ConvType       chatconst.ConvType        `json:"convType,omitempty"`
	ChatType       chatconst.ChatType        `json:"chatType,omitempty"`
	Icon           string                    `json:"icon,omitempty"`  // 公共的(群)
	Title          string                    `json:"title,omitempty"` // 公共的(群)
	Ts             int64                     `json:"ts,omitempty"`
	Sender         string                    `json:"sender,omitempty"`
	Receiver       string                    `json:"receiver,omitempty"`
	RelationId     string                    `json:"relationId,omitempty"` // 公共的(群)
	Members        []string                  `json:"members,omitempty"`
	MebId2UnitInfo map[string]MemberUnitInfo `json:"mebId2UnitInfo,omitempty"` // 私有的(p2p)
	FollowMsg      *msgmodel.LastMsg         `json:"followMsg,omitempty"`
}
