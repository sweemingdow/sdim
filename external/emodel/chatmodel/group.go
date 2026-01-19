package chatmodel

type GroupRole int8

const (
	Owner       GroupRole = 1
	Manager     GroupRole = 2
	OrdinaryMeb GroupRole = 3
)

type GroupState int8

const (
	GrpNormal    GroupState = 1 // ok
	GrpBan       GroupState = 2 // ban
	GrpDismissed GroupState = 3 // dismissed
)

type GroupMebState int8

const (
	GrpMebNormal GroupMebState = 1 // ok
	GrpMebKicked GroupMebState = 2 // be kicked
)

func GenerateGroupChatConvId(grpNo string) string {
	return "grp:" + grpNo
}
