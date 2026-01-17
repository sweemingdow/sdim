package chatmodel

type GroupRole int8

const (
	Owner       GroupRole = 1
	Manager     GroupRole = 2
	OrdinaryMeb GroupRole = 3
)

func GenerateGroupChatConvId(grpNo string) string {
	return "grp:" + grpNo
}
