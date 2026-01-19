package core

import (
	"github.com/sweemingdow/gmicro_pkg/pkg/guc"
	"github.com/sweemingdow/sdim/external/emodel/chatmodel"
	"time"
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
	Role        chatmodel.GroupRole
	State       chatmodel.GroupMebState
	ForbiddenAt int64
	Cts         int64
	Uts         int64
}

type GroupManager interface {
	OnGroupCreated(grpNo, creator string, uid2role map[string]chatmodel.GroupRole)
}

type groupManager struct {
	segLock    *guc.SegmentRwLock[string]
	grpNo2info map[string]*GroupInfo
}

func NewGroupManager(strip int) GroupManager {
	gm := &groupManager{
		segLock:    guc.NewSegmentRwLock[string](strip, nil),
		grpNo2info: make(map[string]*GroupInfo, strip*2),
	}

	return gm
}

func (gm *groupManager) OnGroupCreated(grpNo, creator string, uid2role map[string]chatmodel.GroupRole) {
	mills := time.Now().UnixMilli()

	_, _ = gm.segLock.WithLock(
		grpNo,
		func() (any, error) {
			grpInfo := &GroupInfo{
				GroupNo: grpNo,
				State:   chatmodel.GrpNormal,
				Creator: creator,
				Owner:   creator,
				Cts:     mills,
				Uts:     mills,
			}

			grpMebsInfo := make(map[string]*GroupMebItem, len(uid2role))
			for mebUid, role := range uid2role {
				grpMebsInfo[mebUid] = &GroupMebItem{
					Role:        role,
					State:       chatmodel.GrpMebNormal,
					ForbiddenAt: 0,
					Cts:         mills,
					Uts:         mills,
				}
			}

			grpInfo.Meb2item = grpMebsInfo

			gm.grpNo2info[grpNo] = grpInfo

			return nil, nil
		},
	)
}
