package convupdate

import (
	"github.com/nsqio/go-nsq"
	"github.com/sweemingdow/gmicro_pkg/pkg/mylog"
	"github.com/sweemingdow/gmicro_pkg/pkg/parser/json"
	"github.com/sweemingdow/sdim/external/eglobal/chatconst"
	"github.com/sweemingdow/sdim/external/eglobal/nsqconst/payload/convpd"
	"github.com/sweemingdow/sdim/micros/enginesrv/internal/codec/fcodec"
	"github.com/sweemingdow/sdim/micros/enginesrv/internal/core"
)

type convUpdateHandler struct {
	connMgr core.ConnManager
	frCodec fcodec.FrameCodec
	dl      *mylog.DecoLogger
}

func NewConvUpdateHandler(connMgr core.ConnManager, frCodec fcodec.FrameCodec) nsq.Handler {
	mfh := &convUpdateHandler{
		connMgr: connMgr,
		frCodec: frCodec,
		dl:      mylog.NewDecoLogger("convUpdateHandlerLogger"),
	}

	return mfh
}

func (mfh *convUpdateHandler) HandleMessage(message *nsq.Message) error {
	var cup convpd.ConvLastMsgUpdateEventPayload
	err := json.Parse(message.Body, &cup)
	if err != nil {
		mfh.dl.Error().Stack().Err(err).Msg("parse conv update payload failed")
		// give up
		return nil
	}

	lg := mfh.dl.GetLogger().With().
		Str("conv_id", cup.ConvId).
		Logger()

	lg.Debug().Msgf("receive conv update event for push, payload:%+v", cup)

	uid2conns := mfh.connMgr.GetUsersConns(cup.Members)

	for _, mebUid := range cup.Members {
		dataMap := make(map[string]any, 3)
		dataMap["lastActiveTs"] = cup.LastActiveTs
		dataMap["lastMsg"] = cup.LastMsg
		dataMap["unreadCount"] = cup.Uid2UnreadCount[mebUid]

		cuf := fcodec.ConvUpdateFrame{
			ConvId: cup.ConvId,
			Type:   chatconst.ConvLastMsgUpdated,
			Data:   dataMap,
		}

		bodies, err := json.Fmt(cuf)
		if err != nil {
			lg.Error().Stack().Str("uid", mebUid).Err(err).Msg("format conv update frame body failed")
			// ignore it
			return nil
		}

		if conns, ok := uid2conns[mebUid]; ok {
			lg.Trace().Str("uid", mebUid).Msgf("conv update event pushing, connSize:%d", len(conns))

			for _, conn := range conns {
				fr := fcodec.NewConvUpdateFrame(bodies)
				b, _ := mfh.frCodec.Encode(fr)
				_, err = conn.Write(b)
				if err != nil {
					lg.Error().Str("uid", mebUid).Stack().Err(err).Msg("write conv update event for pushing failed")
					break
				}
			}
		}
	}

	return nil
}
