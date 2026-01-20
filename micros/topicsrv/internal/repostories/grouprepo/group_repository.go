package grouprepo

import (
	"context"
	"github.com/gocraft/dbr/v2"
	"github.com/redis/go-redis/v9"
	"github.com/sweemingdow/gmicro_pkg/pkg/component/credis"
	"github.com/sweemingdow/gmicro_pkg/pkg/component/csql"
	"github.com/sweemingdow/gmicro_pkg/pkg/utils/umap"
	"github.com/sweemingdow/sdim/external/eglobal/chatconst"
	"github.com/sweemingdow/sdim/external/emodel/chatmodel"
	"github.com/sweemingdow/sdim/external/emodel/chatmodel/chatpojo"
	"github.com/sweemingdow/sdim/external/emodel/usermodel"
	"strconv"
	"time"
)

type CreateGroupChatParam struct {
	GroupNo     string
	GroupName   string
	Avatar      string
	OwnerUid    string
	LimitedNum  int
	MembersInfo []usermodel.UserUnitInfo
}

const (
	groupTreeKeyPrefix = "group_tree:"
	GroupTitleKey      = "group_title"
	GroupAvatarKey     = "group_avatar"
	GroupStateKey      = "group_state"
	GroupLimitedNum    = "group_limited_num"
	GroupCts           = "group_cts"
	GroupUts           = "group_uts"
)

type GroupRepository interface {
	CreateGroupChat(ctx context.Context, param CreateGroupChatParam) (string, map[string]chatmodel.GroupRole, int64, error)

	FindGroupInfo(ctx context.Context, groupNo string) (*chatpojo.Group, error)

	FindGroupItems(ctx context.Context, groupNo string) ([]*chatpojo.GroupItem, error)
}

type groupRepository struct {
	sc *csql.SqlClient
	rc *credis.RedisClient
}

func NewGroupRepository(sc *csql.SqlClient, rc *credis.RedisClient) GroupRepository {
	return &groupRepository{
		sc: sc,
		rc: rc,
	}
}

func (gr *groupRepository) CreateGroupChat(ctx context.Context, param CreateGroupChatParam) (string, map[string]chatmodel.GroupRole, int64, error) {
	cts := time.Now().UnixMilli()
	convId := chatmodel.GenerateGroupChatConvId(param.GroupNo)

	uid2role := make(map[string]chatmodel.GroupRole, len(param.MembersInfo))
	var grpMebCount int64
	err := gr.sc.WithTransCtx(
		ctx,
		func(_ context.Context, tx *dbr.Tx) error {
			_, ie := tx.InsertBySql(
				`insert into t_group (group_no, creator, group_name, group_avatar, limited_num, cts, uts) values(?,?,?,?,?,?,?)`,
				param.GroupNo,
				param.OwnerUid,
				param.GroupName,
				param.Avatar,
				param.LimitedNum,
				cts,
				cts,
			).Exec()
			if ie != nil {
				return ie
			}

			var role chatmodel.GroupRole
			for _, mebInfo := range param.MembersInfo {
				if mebInfo.Uid == param.OwnerUid {
					role = chatmodel.Owner
				} else {
					role = chatmodel.OrdinaryMeb
				}

				uid2role[mebInfo.Uid] = role

				_, ie = tx.InsertBySql(
					`insert into t_group_item (group_no, uid, role, meb_avatar, meb_nickname, cts, uts) values(?,?,?,?,?,?,?) on duplicate key update meb_avatar = values(meb_avatar), meb_nickname = values(meb_nickname), uts = values(uts)`,
					param.GroupNo,
					mebInfo.Uid,
					role,
					mebInfo.Avatar,
					mebInfo.Nickname,
					cts,
					cts,
				).Exec()
				if ie != nil {
					return ie
				}
			}

			_, ie = tx.InsertBySql(
				`insert into t_conv (conv_id, conv_type, msg_seq, cts, uts) values (?,?,?,?,?)`,
				convId,
				chatconst.GroupConv,
				0,
				cts,
				cts,
			).Exec()

			if ie != nil {
				return ie
			}

			for _, mebInfo := range param.MembersInfo {
				_, ie = tx.InsertBySql(
					`insert into t_conv_item (conv_id, conv_type, owner_uid, relation_id, conv_icon, conv_title, cts, uts) values (?,?,?,?,?,?,?,?) on duplicate key update conv_icon = values(conv_icon), conv_title = values(conv_title), uts = values(uts)`,
					convId,
					chatconst.GroupConv,
					mebInfo.Uid,
					param.GroupNo,
					param.Avatar,
					param.GroupName,
					cts,
					cts,
				).Exec()

				if ie != nil {
					return ie
				}
			}

			ie = tx.Select("count(1)").From("t_group_item").Where("group_no = ? and state = ?",
				param.GroupNo,
				chatmodel.GrpMebNormal,
			).LoadOneContext(ctx, &grpMebCount)

			if ie != nil {
				return ie
			}

			return nil
		},
	)

	if err != nil {
		return "", nil, 0, err
	}

	groupTree := make(map[string]string, 5)
	groupTree[GroupTitleKey] = param.GroupName
	groupTree[GroupAvatarKey] = param.Avatar
	groupTree[GroupStateKey] = strconv.FormatInt(int64(chatmodel.GrpNormal), 10)
	groupTree[GroupLimitedNum] = strconv.FormatInt(int64(param.LimitedNum), 10)
	groupTree[GroupCts] = strconv.FormatInt(time.Now().UnixMilli(), 10)

	err = gr.rc.With(func(cli redis.UniversalClient) error {
		_, ie := cli.HMSet(
			ctx,
			PkgGroupTreeKey(param.GroupNo),
			umap.Flat(groupTree),
		).Result()

		return ie
	})

	if err != nil {
		return "", nil, 0, err
	}

	return convId, uid2role, grpMebCount, nil
}

func (gr *groupRepository) FindGroupInfo(ctx context.Context, groupNo string) (*chatpojo.Group, error) {
	var pojo chatpojo.Group
	err := gr.sc.WithSess(func(sess *dbr.Session) error {
		return sess.Select("*").From("t_group").Where("group_no = ?", groupNo).LoadOneContext(ctx, &pojo)
	})

	if err != nil {
		return nil, err
	}

	return &pojo, nil
}

func (gr *groupRepository) FindGroupItems(ctx context.Context, groupNo string) ([]*chatpojo.GroupItem, error) {
	var pojos []*chatpojo.GroupItem
	err := gr.sc.WithSess(func(sess *dbr.Session) error {
		_, ie := sess.Select("*").From("t_group_item").Where("group_no = ?", groupNo).LoadContext(ctx, &pojos)
		if ie != nil {
			return ie
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	return pojos, nil
}

func PkgGroupTreeKey(groupNo string) string {
	return groupTreeKeyPrefix + groupNo
}
