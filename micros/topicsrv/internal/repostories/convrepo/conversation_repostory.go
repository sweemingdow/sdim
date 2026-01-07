package convrepo

import (
	"context"
	"github.com/gocraft/dbr/v2"
	"github.com/redis/go-redis/v9"
	"github.com/sweemingdow/gmicro_pkg/pkg/component/credis"
	"github.com/sweemingdow/gmicro_pkg/pkg/component/csql"
	"github.com/sweemingdow/gmicro_pkg/pkg/utils/umap"
	"github.com/sweemingdow/gmicro_pkg/pkg/utils/usli"
	"github.com/sweemingdow/sdim/external/emodel/chatmodel/chatpojo"
	"strconv"
	"time"
)

type (
	Conv struct {
		ConvId    string
		MsgSeq    int64 // 会话全局序列号
		LastMsgId int64 // 最后一条消息id
		ConvType  int8  // 会话类型
		Cts       int64
		Items     []*ConvItem
	}

	ConvItem struct {
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

const (
	convTreeKeyPrefix  = "conv_tree:"
	BrowseSeqKeyPrefix = "browse_seq:uid:"
	LastMsgIdKey       = "last_msg_id"
	LastActiveTs       = "last_active_ts"
	MsgSeqKey          = "msg_seq"
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

		UpsertConv(ctx context.Context, conv *Conv) error

		ModifyConvTree(ctx context.Context, sender, convId string, msgId int64) (int64, int64, error)

		GetConvTree(ctx context.Context, convId string, uids []string) (map[string]string, error)
	}

	convRepository struct {
		rc *credis.RedisClient

		sc *csql.SqlClient

		ctScript *redis.Script
	}
)

func NewConvRepository(rc *credis.RedisClient, sc *csql.SqlClient) ConvRepository {
	return &convRepository{
		rc:       rc,
		sc:       sc,
		ctScript: redis.NewScript(convTreeLuaScript),
	}
}

func (cr *convRepository) FindConvItems(ctx context.Context, convId string) (*Conv, error) {
	var items []chatpojo.ConvItem
	err := cr.sc.WithSess(func(sess *dbr.Session) error {
		_, err := sess.Select("*").From(chatpojo.ConvItem{}.TableName()).Where("conv_id = ?", convId).Load(&items)
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

	var convTree map[string]string
	err = cr.rc.With(func(cli redis.UniversalClient) error {
		m, e := cli.HGetAll(ctx, pkgConvTreeKey(convId)).Result()
		if e != nil {
			return e
		}

		convTree = m
		return nil
	})

	if err != nil {
		return conv, nil
	}

	if len(convTree) > 0 {
		if val, ok := convTree[MsgSeqKey]; ok {
			seq, _ := strconv.ParseInt(val, 10, 64)
			conv.MsgSeq = seq
		}

		if val, ok := convTree[LastMsgIdKey]; ok {
			msgId, _ := strconv.ParseInt(val, 10, 64)
			conv.LastMsgId = msgId
		}

		for _, item := range conv.Items {
			if val, ok := convTree[PkgBrowseSeqKey(item.OwnerUid)]; ok {
				seq, _ := strconv.ParseInt(val, 10, 64)
				item.BrowseMsgSeq = seq
			}
		}
	}

	return conv, nil
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

func pkgConvTreeKey(convId string) string {
	return convTreeKeyPrefix + convId
}

func PkgBrowseSeqKey(uid string) string {
	return BrowseSeqKeyPrefix + uid
}
