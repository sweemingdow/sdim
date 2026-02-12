package core

import (
	"context"
	"github.com/sweemingdow/sdim/external/emodel/chatmodel"
)

type GroupInfo struct {
	GroupNo  string
	State    chatmodel.GroupState
	Creator  string
	Owner    string
	Meb2item map[string]*GroupMebItem
	Cts      int64
	Uts      int64
}

type GroupMebItem struct {
	Role         chatmodel.GroupRole
	State        chatmodel.GroupMebState
	ForbiddenSec int32
	ForbiddenAt  int64
	Cts          int64
	Uts          int64
}

type CanSendInfo struct {
	GrpState       chatmodel.GroupState
	MebNotInGrp    bool
	GrpMebState    chatmodel.GroupMebState
	ForbiddenSec   int32
	ForbiddenAt    int64
	ClientMsgId    string
	ConvId         string
	ForwardMembers []string
}

type GroupManager interface {
	OnGroupCreated(grpNo, creator string, uid2role map[string]chatmodel.GroupRole)

	OnSendMsgInGroup(ctx context.Context, grpNo, sender, convId, msgClientId string) (CanSendInfo, error)

	GetGroupMebUids(ctx context.Context, grpNo string) []string

	OnGroupMebRemoved(grpNo string, remUids []string)
}
