package infosrv

import (
	"errors"
	"github.com/dgraph-io/ristretto/v2"
	"github.com/gocraft/dbr/v2"
	"github.com/sweemingdow/gmicro_pkg/pkg/utils"
	"github.com/sweemingdow/sdim/external/emodel/usermodel"
	"github.com/sweemingdow/sdim/micros/usersrv/internal/repostories/inforepo"
	"golang.org/x/sync/singleflight"
	"sort"
	"strings"
	"time"
)

var (
	UserNotExistsErr = errors.New("user not exits")
)

const (
	userLocalCacheMinTtl = 3
	userLocalCacheMaxTtl = 5
)

type (
	UserInfoService interface {
		// 用户状态
		UserState(uid string) (usermodel.UserState, error)

		UsersUnitInfo(uids []string) (map[string]*inforepo.UserUnitInfo, error)

		UserUnitInfo(uid string) (*inforepo.UserUnitInfo, error)
	}

	userInfoService struct {
		infoRepo inforepo.UserInfoRepository
		userLc   *ristretto.Cache[string, *inforepo.UserUnitInfo]
		sf       *singleflight.Group
	}
)

func NewUserInfoService(infoRepo inforepo.UserInfoRepository) UserInfoService {
	uis := &userInfoService{
		infoRepo: infoRepo,
		sf:       &singleflight.Group{},
	}

	lc, err := ristretto.NewCache(&ristretto.Config[string, *inforepo.UserUnitInfo]{
		NumCounters: 1e7,
		MaxCost:     1 << 30,
		BufferItems: 64,
		OnEvict:     uis.onUserInfoLocalCacheEvict,
	})

	uis.userLc = lc

	if err != nil {
		panic(err)
	}

	return uis
}
func (uis *userInfoService) UserState(uid string) (usermodel.UserState, error) {
	state, err := uis.infoRepo.FindUserState(uid)
	if err != nil {
		if err == dbr.ErrNotFound {
			return 0, UserNotExistsErr
		}

		return 0, err
	}

	return state, nil
}

func (uis *userInfoService) UsersUnitInfo(uids []string) (map[string]*inforepo.UserUnitInfo, error) {
	uidsLen := len(uids)

	if uidsLen == 0 {
		return map[string]*inforepo.UserUnitInfo{}, nil
	}

	uid2info := make(map[string]*inforepo.UserUnitInfo, uidsLen)
	notHitUids := make([]string, 0, uidsLen)

	for _, uid := range uids {
		if item, ok := uis.userLc.Get(uid); ok {
			uid2info[uid] = item
		} else {
			notHitUids = append(notHitUids, uid)
		}
	}

	if len(notHitUids) == 0 {
		return uid2info, nil
	}

	sort.Strings(notHitUids)
	batchKey := strings.Join(notHitUids, "|")

	m, err, _ := uis.sf.Do(batchKey, func() (interface{}, error) {
		return uis.infoRepo.FindUsersUnitInfo(notHitUids)
	})

	if err != nil {
		return nil, err
	}

	batchInfos := m.(map[string]*inforepo.UserUnitInfo)
	ttl := time.Duration(utils.RandInt(userLocalCacheMinTtl, userLocalCacheMaxTtl)) * time.Minute
	for uid, info := range batchInfos {
		uid2info[uid] = info
		uis.userLc.SetWithTTL(uid, info, 1, ttl)
	}

	return uid2info, nil
}

func (uis *userInfoService) UserUnitInfo(uid string) (*inforepo.UserUnitInfo, error) {
	info, err := uis.infoRepo.FindUserUnitInfo(uid)
	if err != nil {
		if err == dbr.ErrNotFound {
			return nil, UserNotExistsErr
		}

		return nil, err
	}

	return info, nil
}

func (uis *userInfoService) onUserInfoLocalCacheEvict(item *ristretto.Item[*inforepo.UserUnitInfo]) {
	uis.sf.Forget(item.Value.Uid)
}
