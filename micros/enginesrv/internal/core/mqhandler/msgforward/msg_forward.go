package msgforward

import (
	"github.com/nsqio/go-nsq"
	"github.com/sweemingdow/gmicro_pkg/pkg/mylog"
	"github.com/sweemingdow/gmicro_pkg/pkg/parser/json"
	"github.com/sweemingdow/sdim/external/eglobal/nsqconst/payload/msgpd"
	"github.com/sweemingdow/sdim/micros/enginesrv/internal/codec/fcodec"
	"github.com/sweemingdow/sdim/micros/enginesrv/internal/core"
)

type msgForwardHandler struct {
	connMgr core.ConnManager
	frCodec fcodec.FrameCodec
}

func NewMsgForwardHandler(connMgr core.ConnManager, frCodec fcodec.FrameCodec) nsq.Handler {
	mfh := &msgForwardHandler{
		connMgr: connMgr,
		frCodec: frCodec,
	}

	return mfh
}

func (mfh *msgForwardHandler) HandleMessage(message *nsq.Message) error {
	lg := mylog.AppLogger()

	var msp msgpd.MsgForwardPayload

	err := json.Parse(message.Body, &msp)
	if err != nil {
		lg.Error().Stack().Err(err).Msg("parse forward msg payload failed")
		// give up
		return nil
	}

	bodies, err := json.Fmt(msp.Msg)
	if err != nil {
		lg.Error().Stack().Err(err).Msg("parse msg body failed")
		// give up
		return nil
	}

	lg = lg.With().
		Str("req_id", msp.ReqId).
		Str("conv_id", msp.ConvId).
		Int64("msg_id", msp.MsgId).
		Logger()

	lg.Debug().Msgf("receive msg for forward, msgBody:%+v", *msp.Msg)

	uid2conns := mfh.connMgr.GetUsersConns(msp.Members)

	for _, mebUid := range msp.Members {
		if conns, ok := uid2conns[mebUid]; ok {
			lg.Trace().Str("uid", mebUid).Msgf("forwarding msg, connSize:%d", len(conns))
			for _, conn := range conns {
				fr, _ := fcodec.NewForwardFrame(msp.ReqId, bodies)

				b, _ := mfh.frCodec.Encode(fr)
				_, err = conn.Write(b)
				if err != nil {
					lg.Error().Str("uid", mebUid).Stack().Err(err).Msg("write msg for forward failed")
					break
				}
			}
		} else {
			lg.Debug().Str("uid", mebUid).Msg("user offline while forward msg")
		}
	}

	return nil
}
