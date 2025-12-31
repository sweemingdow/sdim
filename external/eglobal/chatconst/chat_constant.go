package chatconst

type ChatType uint8

const (
	P2pChat      ChatType = 1
	GroupChat    ChatType = 2
	CustomerChat ChatType = 3
	Danmaku      ChatType = 4
)

type ConvType uint8

const (
	P2pConv      ConvType = 1
	GroupConv    ConvType = 2
	CustomerConv ConvType = 3
)

var (
	chatType2convType = map[ChatType]ConvType{
		P2pChat:      P2pConv,
		GroupChat:    GroupConv,
		CustomerChat: CustomerConv,
	}
)

func GetConvTypeWithChatType(ct ChatType) ConvType {
	return chatType2convType[ct]
}

func IsValidConvType(ct ChatType) bool {
	_, ok := chatType2convType[ct]
	return ok
}

func IsDanmakuType(chatType ChatType) bool {
	return chatType == Danmaku
}
