package grpmgr

import (
	"context"
	"github.com/sweemingdow/gmicro_pkg/pkg/guc"
	"github.com/sweemingdow/gmicro_pkg/pkg/utils/umap"
	"github.com/sweemingdow/sdim/external/emodel/chatmodel"
	"github.com/sweemingdow/sdim/micros/topicsrv/internal/core"
	"github.com/sweemingdow/sdim/micros/topicsrv/internal/repostories/grouprepo"
	"sync"
	"time"
)

type groupManager struct {
	segLock    *guc.SegmentRwLock[string]
	grpNo2info map[string]*core.GroupInfo
	gr         grouprepo.GroupRepository
}

func NewGroupManager(
	strip int,
	gr grouprepo.GroupRepository) core.GroupManager {
	gm := &groupManager{
		segLock:    guc.NewSegmentRwLock[string](strip, nil),
		grpNo2info: make(map[string]*core.GroupInfo, strip*2),
		gr:         gr,
	}

	return gm
}

func (gm *groupManager) OnGroupCreated(grpNo, creator string, uid2role map[string]chatmodel.GroupRole) {
	mills := time.Now().UnixMilli()

	_, _ = gm.segLock.WithLock(
		grpNo,
		func() (any, error) {
			grpInfo := &core.GroupInfo{
				GroupNo: grpNo,
				State:   chatmodel.GrpNormal,
				Owner:   creator,
				Cts:     mills,
				Uts:     mills,
			}

			grpMebsInfo := make(map[string]*core.GroupMebItem, len(uid2role))
			for mebUid, role := range uid2role {
				grpMebsInfo[mebUid] = &core.GroupMebItem{
					Role:  role,
					State: chatmodel.GrpMebNormal,
					Cts:   mills,
					Uts:   mills,
				}
			}

			grpInfo.Meb2item = grpMebsInfo

			gm.grpNo2info[grpNo] = grpInfo

			return nil, nil
		},
	)
}

func (gm *groupManager) OnSendMsgInGroup(ctx context.Context, grpNo, sender string) (core.CanSendInfo, error) {
	grpInfo, err := gm.initGroupInfoLazy(ctx, grpNo)
	if err != nil {
		var info core.CanSendInfo
		return info, err
	}

	return gm.groupIno2canSendInfo(grpInfo, sender), nil
}

func (gm *groupManager) initGroupInfoLazy(ctx context.Context, grpNo string) (*core.GroupInfo, error) {
	infoVal, err := gm.segLock.WithLockManual(
		grpNo,
		func(lock *sync.RWMutex) (any, error) {
			lock.RLock()
			grpInfo, ok := gm.grpNo2info[grpNo]
			lock.RUnlock()

			if ok {
				return grpInfo, nil
			}

			lock.Lock()
			defer lock.Unlock()

			grpInfo, ok = gm.grpNo2info[grpNo]

			if ok {
				return grpInfo, nil
			}

			_grpInfo, ie := gm.gr.FindGroupInfo(ctx, grpNo)
			if ie != nil {
				return nil, ie
			}
			grpMebItems, ie := gm.gr.FindGroupItems(ctx, grpNo)
			if ie != nil {
				return nil, ie
			}

			// 构造一个新的grpInfo
			grpInfo = &core.GroupInfo{
				GroupNo:  grpNo,
				State:    chatmodel.GroupState(_grpInfo.State),
				Creator:  _grpInfo.Creator,
				Meb2item: make(map[string]*core.GroupMebItem, len(grpMebItems)),
				Cts:      _grpInfo.Cts,
				Uts:      _grpInfo.Uts.Int64,
			}

			var ownerUid string
			for _, item := range grpMebItems {
				role := chatmodel.GroupRole(item.Role)
				if role == chatmodel.Owner {
					if ownerUid == "" {
						ownerUid = item.Uid
					}
				}

				grpMebItem := &core.GroupMebItem{
					Role:         chatmodel.GroupRole(item.Role),
					State:        chatmodel.GroupMebState(item.State),
					ForbiddenSec: item.ForbidenDur,
					ForbiddenAt:  item.ForbiddenAt,
					Cts:          item.Cts,
					Uts:          item.Uts.Int64,
				}

				grpInfo.Meb2item[item.Uid] = grpMebItem
			}

			grpInfo.Owner = ownerUid
			gm.grpNo2info[grpNo] = grpInfo

			return grpInfo, nil
		},
	)

	if err != nil {
		return nil, err
	}

	return infoVal.(*core.GroupInfo), nil
}

func (gm *groupManager) GetGroupMebUids(ctx context.Context, grpNo string) []string {
	_, err := gm.initGroupInfoLazy(ctx, grpNo)
	if err != nil {
		return make([]string, 0)
	}

	uidsVal, _ := gm.segLock.WithLock(
		grpNo,
		func() (any, error) {
			// copy
			info, ok := gm.grpNo2info[grpNo]
			if !ok {
				return nil, nil
			}

			return umap.KeyToSli(info.Meb2item), nil
		})

	if uids, ok := uidsVal.([]string); ok {
		return uids
	}

	return make([]string, 0)
}

func (gm *groupManager) groupIno2canSendInfo(grpInfo *core.GroupInfo, sender string) core.CanSendInfo {
	if grpInfo.State != chatmodel.GrpNormal {
		return core.CanSendInfo{
			GrpState: grpInfo.State,
		}
	}

	if item, ok := grpInfo.Meb2item[sender]; !ok {
		return core.CanSendInfo{
			MebNotInGrp: true,
		}
	} else {
		members := make([]string, 0, len(grpInfo.Meb2item)-1)
		for mebUid, mebItem := range grpInfo.Meb2item {
			if mebUid == sender {
				continue
			}
			if mebItem.State != chatmodel.GrpMebNormal {
				continue
			}

			members = append(members, mebUid)
		}

		return core.CanSendInfo{
			GrpState:       grpInfo.State,
			MebNotInGrp:    false,
			GrpMebState:    item.State,
			ForbiddenSec:   item.ForbiddenSec,
			ForbiddenAt:    item.ForbiddenAt,
			ForwardMembers: members,
		}
	}
}
