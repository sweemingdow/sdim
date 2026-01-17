package convrepo

import (
	"context"
	"github.com/gocraft/dbr/v2"
	"github.com/redis/go-redis/v9"
	"github.com/sweemingdow/gmicro_pkg/pkg/component/credis"
	"github.com/sweemingdow/gmicro_pkg/pkg/component/csql"
	"github.com/sweemingdow/gmicro_pkg/pkg/utils"
	"github.com/sweemingdow/gmicro_pkg/pkg/utils/umap"
	"github.com/sweemingdow/gmicro_pkg/pkg/utils/usli"
	"github.com/sweemingdow/sdim/external/emodel/chatmodel/chatpojo"
	"strconv"
	"time"
)

type (
	Conv struct {
		ConvId       string
		MsgSeq       int64 // 会话全局序列号
		LastMsgId    int64 // 最后一条消息id
		LastActiveTs int64
		ConvType     int8 // 会话类型
		Cts          int64
		Items        []*ConvItem // upsert时用
	}

	ConvItem struct {
		ConvId       string
		OwnerUid     string // 会话的所有者
		RelationId   string // 会话的关联者
		ConvType     int8
		Icon         string
		Title        string
		Remark       string
		BrowseMsgSeq int64 // 所有者浏览到的seq
		PinTop       bool
		NoDisturb    bool
		Cts          int64
		Uts          int64
	}
)

const convTreeLuaScript = `
local hk = KEYS[1]
local seq = redis.call('HINCRBY', hk, 'msg_seq', 1)
redis.call('HMSET', hk, ARGV[1], seq, 'last_msg_id', ARGV[2], 'last_active_ts', ARGV[3])
return seq
`

const clearUnreadLuaScript = `
local hk = KEYS[1]
local seq = redis.call('HGET', hk, 'msg_seq')
redis.call('HSET', hk, ARGV[1], seq)
return seq
`

const (
	convTreeKeyPrefix  = "conv_tree:"
	BrowseSeqKeyPrefix = "browse_seq:uid:"
	LastMsgIdKey       = "last_msg_id"
	LastActiveTs       = "last_active_ts"
	MsgSeqKey          = "msg_seq"
	UserBrowseMsgSeq   = "user_browse_msg_seq"
)

/*
会话在redis存储的是个hash结构
hashKey: conv_tree:{convId}
sub-fields:

	msgSeq: 全局序列号
	lastMsgId: 最后一条消息id
	u:{uid}: 用户在会话内的浏览位置
	... other users
*/
type (
	ConvRepository interface {
		FindConvItems(ctx context.Context, convId string) (*Conv, error)

		FindConv(ctx context.Context, convId string) (*Conv, error)

		FindConvOneItem(ctx context.Context, convId, uid string) (*ConvItem, error)

		// 获取用户的所有会话
		FindUserConvItems(ctx context.Context, uid string) ([]*ConvItem, error)

		UpsertConv(ctx context.Context, conv *Conv) error

		ModifyConvTree(ctx context.Context, sender, convId string, msgId int64) (int64, int64, error)

		GetConvTree(ctx context.Context, convId string, uids []string) (map[string]string, error)

		GetConvTreeWithP2p(ctx context.Context, convId string) (map[string]string, error)

		GetConvTreesWithP2p(ctx context.Context, uid string, convIds []string) (map[string]map[string]string, error)

		// 清除未读
		ClearUnread(ctx context.Context, convId, uid string) (int64, error)
	}

	convRepository struct {
		rc *credis.RedisClient

		sc *csql.SqlClient

		ctScript *redis.Script

		clearUnreadScript *redis.Script
	}
)

func NewConvRepository(rc *credis.RedisClient, sc *csql.SqlClient) ConvRepository {
	return &convRepository{
		rc:                rc,
		sc:                sc,
		ctScript:          redis.NewScript(convTreeLuaScript),
		clearUnreadScript: redis.NewScript(clearUnreadLuaScript),
	}
}

func TransferConvTreeForBatch(convTree map[string]string) (int64, int64, int64, int64) {
	var (
		msgSeq       int64
		lastMsgId    int64
		lastActiveTs int64
		browseSeq    int64
	)

	val, ok := convTree[MsgSeqKey]
	if ok {
		msgSeq = utils.MustParseI64(val)
	}

	val, ok = convTree[LastMsgIdKey]
	if ok {
		lastMsgId = utils.MustParseI64(val)
	}

	val, ok = convTree[LastActiveTs]
	if ok {
		lastActiveTs = utils.MustParseI64(val)
	}

	val, ok = convTree[UserBrowseMsgSeq]
	if ok {
		browseSeq = utils.MustParseI64(val)
	}

	return msgSeq, lastMsgId, lastActiveTs, browseSeq
}

func TransferConvTreeForOrigin(convTree map[string]string, members []string) (int64, int64, int64, map[string]int64) {
	var (
		msgSeq        int64
		lastMsgId     int64
		lastActiveTs  int64
		uid2browseSeq = make(map[string]int64, len(members))
	)

	val, ok := convTree[MsgSeqKey]
	if ok {
		msgSeq = utils.MustParseI64(val)
	}

	val, ok = convTree[LastMsgIdKey]
	if ok {
		lastMsgId = utils.MustParseI64(val)
	}

	val, ok = convTree[LastActiveTs]
	if ok {
		lastActiveTs = utils.MustParseI64(val)
	}

	for _, mebUid := range members {
		val, ok = convTree[PkgBrowseSeqKey(mebUid)]
		if ok {
			browseSeq := utils.MustParseI64(val)
			uid2browseSeq[mebUid] = browseSeq
		}
	}

	return msgSeq, lastMsgId, lastActiveTs, uid2browseSeq
}

func (cr *convRepository) FindConvItems(ctx context.Context, convId string) (*Conv, error) {
	var items []chatpojo.ConvItem
	err := cr.sc.WithSess(func(sess *dbr.Session) error {
		_, err := sess.Select("*").From(chatpojo.ConvItem{}.TableName()).Where("conv_id = ?", convId).LoadContext(ctx, &items)
		return err
	})

	if err != nil {
		return nil, err
	}

	if len(items) == 0 {
		return nil, nil
	}

	conv := &Conv{
		ConvId: convId,
		Items:  make([]*ConvItem, len(items)),
	}

	ctsSetting := false
	for i, item := range items {
		conv.Items[i] = &ConvItem{
			ConvId:     item.ConvId,
			OwnerUid:   item.OwnerUid,
			RelationId: item.RelationId,
			ConvType:   item.ConvType,
			Title:      item.ConvTitle.String,
			Icon:       item.ConvIcon.String,
			Remark:     item.ConvRemark.String,
			PinTop:     item.PinTop.Int16 == 1,
			NoDisturb:  item.NoDisturb.Int16 == 1,
			Cts:        item.Cts,
			Uts:        item.Uts.Int64,
		}

		if !ctsSetting {
			ctsSetting = true
			conv.Cts = item.Cts
		}
	}

	return conv, nil
}

func (cr *convRepository) FindConv(ctx context.Context, convId string) (*Conv, error) {
	var pojo chatpojo.Conv
	err := cr.sc.WithSess(func(sess *dbr.Session) error {
		ie := sess.Select("conv_id, conv_type").
			From("t_conv").
			Where("conv_id = ?", convId).
			LoadOneContext(ctx, &pojo)
		return ie
	})

	if err != nil {
		return nil, err
	}

	return &Conv{
		ConvId:   convId,
		ConvType: pojo.ConvType,
		Items:    make([]*ConvItem, 0),
	}, nil
}

func pojoItem2memItem(item *chatpojo.ConvItem) *ConvItem {
	return &ConvItem{
		ConvId:     item.ConvId,
		OwnerUid:   item.OwnerUid,
		RelationId: item.RelationId,
		ConvType:   item.ConvType,
		Title:      item.ConvTitle.String,
		Icon:       item.ConvIcon.String,
		Remark:     item.ConvRemark.String,
		PinTop:     item.PinTop.Int16 == 1,
		NoDisturb:  item.NoDisturb.Int16 == 1,
		Cts:        item.Cts,
		Uts:        item.Uts.Int64,
	}
}

func (cr *convRepository) FindConvOneItem(ctx context.Context, convId, uid string) (*ConvItem, error) {
	var pojo chatpojo.ConvItem
	err := cr.sc.WithSess(func(sess *dbr.Session) error {
		ie := sess.Select("*").
			From("t_conv_item").
			Where("owner_uid = ?", uid).
			LoadOneContext(ctx, &pojo)
		return ie
	})

	if err != nil {
		return nil, err
	}

	return pojoItem2memItem(&pojo), nil
}

func (cr *convRepository) FindUserConvItems(ctx context.Context, uid string) ([]*ConvItem, error) {
	var pojoItems []chatpojo.ConvItem
	err := cr.sc.WithSess(func(sess *dbr.Session) error {
		_, ie := sess.Select("*").
			From("t_conv_item").
			Where("owner_uid = ?", uid).
			LoadContext(ctx, &pojoItems)
		return ie
	})

	if err != nil {
		return nil, err
	}

	if len(pojoItems) == 0 {
		return make([]*ConvItem, 0), nil
	}

	items := make([]*ConvItem, len(pojoItems))
	for i, item := range pojoItems {
		items[i] = pojoItem2memItem(&item)
	}

	return items, nil
}

func (cr *convRepository) UpsertConv(ctx context.Context, conv *Conv) error {
	// 写db
	err := cr.sc.WithTransCtx(ctx, func(_ context.Context, tx *dbr.Tx) error {
		_, e := tx.InsertBySql(
			`insert into t_conv (conv_id, conv_type, msg_seq, last_msg_id, cts, uts) values (?,?,?,?,?,?) on duplicate key update msg_seq = values(msg_seq), last_msg_id = values(last_msg_id), uts = values(uts)`,
			conv.ConvId,
			conv.ConvType,
			conv.MsgSeq,
			conv.LastMsgId,
			conv.Cts,
			conv.Cts,
		).Exec()

		if e != nil {
			return e
		}

		for _, item := range conv.Items {
			_, e = tx.InsertBySql(
				`insert into t_conv_item (conv_id, conv_type, owner_uid, relation_id, conv_icon, conv_title, browse_msg_seq, cts, uts) values (?,?,?,?,?,?,?,?,?) on duplicate key update conv_icon = values(conv_icon), conv_title = values(conv_title), browse_msg_seq = values(browse_msg_seq), uts = values(uts)`,
				conv.ConvId,
				conv.ConvType,
				item.OwnerUid,
				item.RelationId,
				item.Icon,
				item.Title,
				item.BrowseMsgSeq,
				item.Cts,
				item.Cts,
			).Exec()

			if e != nil {
				return e
			}
		}

		return nil
	})

	if err != nil {
		return err
	}

	// 写redis
	convTree := make(map[string]string, 4)
	convTree[MsgSeqKey] = strconv.FormatInt(conv.MsgSeq, 10)
	convTree[LastMsgIdKey] = strconv.FormatInt(conv.LastMsgId, 10)
	convTree[LastActiveTs] = strconv.FormatInt(conv.LastActiveTs, 10)

	for _, item := range conv.Items {
		convTree[PkgBrowseSeqKey(item.OwnerUid)] = "0"
	}

	err = cr.rc.With(func(cli redis.UniversalClient) error {
		return cli.HMSet(
			ctx,
			pkgConvTreeKey(conv.ConvId),
			umap.Flat(convTree),
		).Err()
	})

	if err != nil {
		return err
	}

	return nil
}

func (cr *convRepository) ModifyConvTree(ctx context.Context, sender, convId string, msgId int64) (int64, int64, error) {
	var seq int64
	var lastTs = time.Now().UnixMilli()
	err := cr.rc.With(func(cli redis.UniversalClient) error {
		val, e := cr.ctScript.Run(
			ctx,
			cli,
			[]string{pkgConvTreeKey(convId)},
			PkgBrowseSeqKey(sender),
			strconv.FormatInt(msgId, 10),
			lastTs,
		).Result()

		if e != nil {
			return e
		}

		seq = val.(int64)
		return nil
	})

	if err != nil {
		return 0, 0, err
	}

	return seq, lastTs, nil
}

func (cr *convRepository) GetConvTree(ctx context.Context, convId string, uids []string) (map[string]string, error) {
	uids = usli.Conv(uids, func(uid string) string {
		return PkgBrowseSeqKey(uid)
	})

	fields := make([]string, 0, len(uids)+3)
	fields = append(fields, MsgSeqKey, LastMsgIdKey, LastActiveTs)
	fields = append(fields, uids...)

	var results []any
	err := cr.rc.With(func(cli redis.UniversalClient) error {
		_result, ie := cli.HMGet(ctx, pkgConvTreeKey(convId), fields...).Result()
		if ie != nil {
			return ie
		}

		results = _result

		return nil
	})

	if err != nil {
		return nil, err
	}

	rm := make(map[string]string, len(fields))
	for idx, field := range fields {
		v, ok := results[idx].(string)
		if !ok {
			v = ""
		}
		rm[field] = v
	}

	return rm, nil
}

func (cr *convRepository) GetConvTreeWithP2p(ctx context.Context, convId string) (map[string]string, error) {
	var convTree map[string]string
	err := cr.rc.With(func(cli redis.UniversalClient) error {
		m, e := cli.HGetAll(ctx, pkgConvTreeKey(convId)).Result()
		if e != nil {
			return e
		}

		convTree = m
		return nil
	})

	if err != nil {
		return nil, err
	}

	return convTree, nil
}

func (cr *convRepository) GetConvTreesWithP2p(ctx context.Context, uid string, convIds []string) (map[string]map[string]string, error) {
	var sliceCmds []*redis.SliceCmd
	err := cr.rc.With(func(cli redis.UniversalClient) error {
		cmds, ie := cli.Pipelined(ctx, func(pl redis.Pipeliner) error {
			for _, convId := range convIds {
				fields := []string{MsgSeqKey, LastMsgIdKey, LastActiveTs, PkgBrowseSeqKey(uid)}
				fields = append(fields)

				pl.HMGet(ctx, pkgConvTreeKey(convId), fields...)
			}
			return nil
		})

		if ie != nil {
			return ie
		}

		sliceCmds = make([]*redis.SliceCmd, len(cmds))
		for idx, cmd := range cmds {
			if cmd.Err() != nil {
				return cmd.Err()
			}

			sliceCmds[idx] = cmd.(*redis.SliceCmd)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	m := make(map[string]map[string]string, len(sliceCmds))

	for idx, convId := range convIds {
		m[convId] = make(map[string]string, 4)
		sliCmd := sliceCmds[idx]
		values := sliCmd.Val()
		if values[0] != nil {
			m[convId][MsgSeqKey] = values[0].(string)
		}

		if values[1] != nil {
			m[convId][LastMsgIdKey] = values[1].(string)
		}

		if values[2] != nil {
			m[convId][LastActiveTs] = values[2].(string)
		}

		if values[3] != nil {
			m[convId][UserBrowseMsgSeq] = values[3].(string)
		}
	}

	return m, nil
}

func (cr *convRepository) ClearUnread(ctx context.Context, convId, uid string) (int64, error) {
	var seqVal string
	err := cr.rc.With(func(cli redis.UniversalClient) error {
		val, e := cr.clearUnreadScript.Run(
			ctx,
			cli,
			[]string{pkgConvTreeKey(convId)},
			PkgBrowseSeqKey(uid),
		).Result()

		if e != nil {
			return e
		}

		seqVal = val.(string)
		return nil
	})

	if err != nil {
		return 0, err
	}

	return strconv.ParseInt(seqVal, 10, 64)
}

func pkgConvTreeKey(convId string) string {
	return convTreeKeyPrefix + convId
}

func PkgBrowseSeqKey(uid string) string {
	return BrowseSeqKeyPrefix + uid
}
