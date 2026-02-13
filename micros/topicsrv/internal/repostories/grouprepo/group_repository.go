package grouprepo

import (
	"context"
	"fmt"
	"github.com/gocraft/dbr/v2"
	"github.com/pkg/errors"
	"github.com/redis/go-redis/v9"
	"github.com/sweemingdow/gmicro_pkg/pkg/component/credis"
	"github.com/sweemingdow/gmicro_pkg/pkg/component/csql"
	"github.com/sweemingdow/gmicro_pkg/pkg/utils/umap"
	"github.com/sweemingdow/sdim/external/eglobal/chatconst"
	"github.com/sweemingdow/sdim/external/emodel/chatmodel"
	"github.com/sweemingdow/sdim/external/emodel/chatmodel/chatpojo"
	"github.com/sweemingdow/sdim/external/emodel/usermodel"
	"strconv"
	"strings"
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

	FindGroupMebUids(ctx context.Context, groupNo string) ([]string, error)

	SettingGroupName(ctx context.Context, groupNo, groupName string) (bool, error)

	SettingGroupBak(ctx context.Context, uid, groupNo, groupBak string) (bool, error)

	SettingGroupNickname(ctx context.Context, uid, groupNo, nickname string) (bool, error)

	AddMembers(ctx context.Context, tx *dbr.Tx, uid, groupNo string, members []usermodel.UserUnitInfo) (bool, error)

	GetRoleInGroup(ctx context.Context, groupNo string, uid string, tx *dbr.Tx) (chatmodel.GroupRole, error)

	RemoveMembers(ctx context.Context, tx *dbr.Tx, groupNo string, members []string) ([]string, error)
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
		//return nil, errors.WithStack(err)
		return nil, errors.Wrapf(err, "find group failed, groupNo=%s", groupNo)
	}

	return &pojo, nil
}

func (gr *groupRepository) FindGroupItems(ctx context.Context, groupNo string) ([]*chatpojo.GroupItem, error) {
	var pojos []*chatpojo.GroupItem
	err := gr.sc.WithSess(func(sess *dbr.Session) error {
		_, ie := sess.Select("*").From("t_group_item").Where("group_no = ? and state = ?", groupNo, chatmodel.GrpMebNormal).LoadContext(ctx, &pojos)
		if ie != nil {
			return ie
		}
		return nil
	})

	if err != nil {
		return nil, errors.Wrapf(err, "find group items failed, groupNo=%s", groupNo)
	}

	return pojos, nil
}

func (gr *groupRepository) SettingGroupName(ctx context.Context, groupNo, groupName string) (bool, error) {
	var suc bool
	err := gr.sc.WithTransCtx(
		ctx,
		func(_ context.Context, tx *dbr.Tx) error {
			rst, ie := tx.Update(chatpojo.Group{}.TableName()).Set("group_name", groupName).Where("group_no = ?", groupNo).Exec()
			if ie != nil {
				return ie
			}
			rows, _ := rst.RowsAffected()
			suc = rows == 1
			if !suc {
				return errors.New("modify group name failed, groupNo=" + groupNo)
			}

			//rst, ie = tx.Update(chatpojo.ConvItem{}.TableName()).Set("conv_title", groupName).Where("relation_id = ?", groupNo).Exec()
			//if ie != nil {
			//	return ie
			//}
			//
			//rows, _ = rst.RowsAffected()
			//suc = rows == 1

			rst, ie = tx.UpdateBySql(
				`update t_conv_item ci join t_group_item gi on ci.relation_id = gi.group_no and ci.owner_uid = gi.uid set ci.conv_title = ? where gi.group_no = ? and (gi.remark is null or gi.remark = '')`,
				groupName, groupNo,
			).Exec()

			if ie != nil {
				return ie
			}

			return nil
		})

	if err != nil {
		return suc, err
	}

	err = gr.rc.With(func(cli redis.UniversalClient) error {
		return cli.HSet(ctx, PkgGroupTreeKey(groupNo), GroupTitleKey, groupName).Err()
	})

	if err != nil {
		suc = false
		return suc, err
	}

	return suc, nil
}

func (gr *groupRepository) SettingGroupBak(ctx context.Context, uid, groupNo, groupBak string) (bool, error) {
	var suc bool
	err := gr.sc.WithTransCtx(
		ctx,
		func(_ context.Context, tx *dbr.Tx) error {
			rst, ie := tx.Update(chatpojo.GroupItem{}.TableName()).Set("remark", groupBak).Where("group_no = ? and uid = ?", groupNo, uid).Exec()
			if ie != nil {
				return ie
			}
			rows, _ := rst.RowsAffected()
			suc = rows == 1
			if !suc {
				return errors.New("modify group bak failed, groupNo=" + groupNo)
			}

			rst, ie = tx.Update(chatpojo.ConvItem{}.TableName()).Set("conv_title", groupBak).Where("relation_id = ? and owner_uid = ?",
				groupNo, uid).Exec()
			if ie != nil {
				return ie
			}

			rows, _ = rst.RowsAffected()
			suc = rows == 1
			return nil
		})

	if err != nil {
		return suc, err
	}

	return suc, nil
}

func (gr *groupRepository) SettingGroupNickname(ctx context.Context, uid, groupNo, nickname string) (bool, error) {
	var suc bool
	err := gr.sc.WithTransCtx(
		ctx,
		func(_ context.Context, tx *dbr.Tx) error {
			rst, ie := tx.Update(chatpojo.GroupItem{}.TableName()).Set("meb_nickname", nickname).Where("group_no = ? and uid = ?", groupNo, uid).Exec()
			if ie != nil {
				return ie
			}
			rows, _ := rst.RowsAffected()
			suc = rows == 1
			if !suc {
				return errors.New("modify group nickname failed, groupNo=" + groupNo)
			}
			return nil
		})

	if err != nil {
		return suc, err
	}

	return suc, nil
}

func (gr *groupRepository) FindGroupMebUids(ctx context.Context, groupNo string) ([]string, error) {
	var uids []string
	err := gr.sc.WithSess(func(sess *dbr.Session) error {
		_, ie := sess.Select("uid").From(chatpojo.GroupItem{}.TableName()).Where("group_no = ?", groupNo).LoadContext(ctx, &uids)
		if ie != nil {
			return ie
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return uids, nil
}

func (gr *groupRepository) AddMembers(ctx context.Context, tx *dbr.Tx, uid, groupNo string, members []usermodel.UserUnitInfo) (bool, error) {
	mills := time.Now().UnixMilli()
	var (
		placeholders []string
		args         = make([]any, 0, len(members)*7)
	)

	for _, mebInfo := range members {
		placeholders = append(placeholders, "(?,?,?,?,?,?,?)")
		args = append(args,
			groupNo,
			mebInfo.Uid,
			chatmodel.OrdinaryMeb,
			mebInfo.Avatar,
			mebInfo.Nickname,
			mills,
			mills,
		)
	}

	sql := fmt.Sprintf(
		`INSERT INTO t_group_item (group_no, uid, role, meb_avatar, meb_nickname, cts, uts) 
     VALUES %s 
     ON DUPLICATE KEY UPDATE 
        meb_avatar = VALUES(meb_avatar),
        meb_nickname = VALUES(meb_nickname),
        uts = VALUES(uts)`,
		strings.Join(placeholders, ","),
	)

	_, err := tx.InsertBySql(sql, args...).ExecContext(ctx)
	if err != nil {
		return false, errors.WithStack(err)
	}

	return true, nil
}

func (gr *groupRepository) RemoveMembers(ctx context.Context, tx *dbr.Tx, groupNo string, members []string) ([]string, error) {
	var existsUids []string
	var pojo chatpojo.GroupItem
	// 查询当前已存在的成员
	_, err := tx.Select("uid").From(pojo.TableName()).
		Where("group_no = ? and uid in ? and state = ?", groupNo, members, chatmodel.GrpMebNormal).
		LoadContext(ctx, &existsUids)
	if err != nil {
		return existsUids, errors.WithStack(err)
	}

	if len(existsUids) == 0 {
		return existsUids, nil
	}

	_, err = tx.Update(pojo.TableName()).
		Set("state", chatmodel.GrpMebKicked).
		Where("group_no = ? and uid in ?", groupNo, existsUids).Exec()
	if err != nil {
		return existsUids, errors.WithStack(err)
	}

	return existsUids, nil
}

func (gr *groupRepository) GetRoleInGroup(ctx context.Context, groupNo string, uid string, tx *dbr.Tx) (chatmodel.GroupRole, error) {
	var role int8
	err := tx.Select("role").From(chatpojo.GroupItem{}.TableName()).
		Where("group_no = ? and uid = ? and state = ?", groupNo, uid, chatmodel.GrpMebNormal).
		LoadOneContext(ctx, &role)
	if err != nil {
		return 0, errors.WithStack(err)
	}

	return chatmodel.GroupRole(role), nil
}

func PkgGroupTreeKey(groupNo string) string {
	return groupTreeKeyPrefix + groupNo
}
