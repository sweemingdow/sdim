package inforepo

import (
	"github.com/gocraft/dbr/v2"
	"github.com/sweemingdow/gmicro_pkg/pkg/component/credis"
	"github.com/sweemingdow/gmicro_pkg/pkg/component/csql"
	"github.com/sweemingdow/gmicro_pkg/pkg/utils/usli"
	"github.com/sweemingdow/sdim/external/emodel/usermodel"
)

type (
	UserUnitInfo struct {
		Uid      string `db:"uid" json:"uid,omitempty"`
		Nickname string `db:"nickname" json:"nickname,omitempty"`
		Avatar   string `db:"avatar" json:"avatar,omitempty"`
	}
)

type (
	UserInfoRepository interface {
		FindUserState(uid string) (usermodel.UserState, error)

		FindUsersUnitInfo(uids []string) (map[string]*UserUnitInfo, error)

		FindUserUnitInfo(uid string) (*UserUnitInfo, error)
	}

	userInfoRepository struct {
		sc *csql.SqlClient
		rc *credis.RedisClient
	}
)

func (uir *userInfoRepository) FindUsersUnitInfoInRemoteCache(uids []string) ([]UserUnitInfo, error) {
	//TODO implement me
	panic("implement me")
}

func NewUserInfoRepository(sc *csql.SqlClient, rc *credis.RedisClient) UserInfoRepository {
	return &userInfoRepository{
		sc: sc,
		rc: rc,
	}
}

func (uir *userInfoRepository) FindUserState(uid string) (usermodel.UserState, error) {
	var state int8
	err := uir.sc.WithSess(func(sess *dbr.Session) error {
		return sess.Select("state").From("t_user").Where("uid = ?", uid).LoadOne(&state)
	})

	if err != nil {
		return 0, err
	}

	return usermodel.UserState(state), nil
}

func (uir *userInfoRepository) FindUsersUnitInfo(uids []string) (map[string]*UserUnitInfo, error) {
	var infoItems []*UserUnitInfo
	err := uir.sc.WithSess(func(sess *dbr.Session) error {
		_, e := sess.Select("uid, nickname, avatar").From("t_user").Where("uid in ?", uids).Load(&infoItems)
		if e != nil {
			return e
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return usli.ToItMap(infoItems, func(item *UserUnitInfo) string {
		return item.Uid
	}), nil
}

func (uir *userInfoRepository) FindUserUnitInfo(uid string) (*UserUnitInfo, error) {
	var uui UserUnitInfo
	err := uir.sc.WithSess(func(sess *dbr.Session) error {
		return sess.Select("uid, nickname, avatar").From("t_user").Where("uid = ?", uid).LoadOne(&uui)
	})

	if err != nil {
		return nil, err
	}

	return &uui, nil
}
