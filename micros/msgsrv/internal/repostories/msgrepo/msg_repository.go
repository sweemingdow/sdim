package msgrepo

import (
	"context"
	"github.com/gocraft/dbr/v2"
	"github.com/sweemingdow/gmicro_pkg/pkg/component/csql"
	"github.com/sweemingdow/sdim/external/emodel/msgmodel/msgpojo"
	"time"
)

type (
	MsgRepository interface {
		UpsertMsg(timeout time.Duration, pojo *msgpojo.Msg) (int64, error)

		FindConvMsgs(ctx context.Context, convId string, lastMsgId int64, limit uint8) ([]*msgpojo.Msg, error)
	}

	msgRepository struct {
		sc *csql.SqlClient
	}
)

func NewMsgRepository(sc *csql.SqlClient) MsgRepository {
	return &msgRepository{
		sc: sc,
	}
}

func (mr *msgRepository) UpsertMsg(timeout time.Duration, pojo *msgpojo.Msg) (int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	var rows int64
	err := mr.sc.WithTransCtx(
		ctx,
		func(_ context.Context, tx *dbr.Tx) error {
			rst, ie := tx.InsertBySql(
				`insert ignore into t_msg (id, conv_id, chat_type, msg_type, sender, receiver, seq, msg_body, expired_at, cts, uts) values (?,?,?,?,?,?,?,?,?,?,?)`,
				pojo.Id,
				pojo.ConvId,
				pojo.ChatType,
				pojo.MsgType,
				pojo.Sender,
				pojo.Receiver,
				pojo.Seq,
				pojo.MsgBody,
				pojo.ExpiredAt,
				pojo.Cts,
				pojo.Uts,
			).Exec()

			if ie != nil {
				return ie
			}

			rows, _ = rst.RowsAffected()
			return nil
		})

	if err != nil {
		return 0, err
	}

	return rows, nil
}

func (mr *msgRepository) FindConvMsgs(ctx context.Context, convId string, lastMsgId int64, limit uint8) ([]*msgpojo.Msg, error) {
	var pojos []*msgpojo.Msg
	err := mr.sc.WithSess(func(sess *dbr.Session) error {
		_, ie := sess.Select("*").
			From("t_msg").
			Where("conv_id = ? and id < ?", convId, lastMsgId).
			OrderDesc("id").
			Limit(uint64(limit)).
			LoadContext(ctx, &pojos)

		return ie
	})

	if err != nil {
		return make([]*msgpojo.Msg, 0), err
	}

	return pojos, nil
}
