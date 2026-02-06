package notifyevent

import (
	"github.com/nsqio/go-nsq"
	"github.com/sweemingdow/gmicro_pkg/pkg/mylog"
	"github.com/sweemingdow/gmicro_pkg/pkg/parser/json"
	"github.com/sweemingdow/sdim/external/eglobal/nsqconst/payload/notifypd"
	"github.com/sweemingdow/sdim/micros/enginesrv/internal/codec/fcodec"
	"github.com/sweemingdow/sdim/micros/enginesrv/internal/core"
)

type notifyEventHandler struct {
	connMgr core.ConnManager
	frCodec fcodec.FrameCodec
	dl      *mylog.DecoLogger
}

func NewNotifyEventHandler(cm core.ConnManager, frCodec fcodec.FrameCodec) nsq.Handler {
	return &notifyEventHandler{
		connMgr: cm,
		frCodec: frCodec,
		dl:      mylog.NewDecoLogger("notifyEventHandler"),
	}
}

func (h *notifyEventHandler) HandleMessage(message *nsq.Message) error {
	h.dl.Debug().Msg("receive notify event")

	var pd notifypd.NotifyPayload
	err := json.Parse(message.Body, &pd)
	if err != nil {
		h.dl.Error().Stack().Err(err).Msg("parse notify msg payload failed")
		// give up
		return nil
	}

	uid2conns := h.connMgr.GetUsersConns(pd.Members)

	for _, mebUid := range pd.Members {
		frm := fcodec.NotifyFrame{
			NotifyType: pd.NotifyType,
			SubType:    pd.SubType,
			Data:       pd.Data,
		}

		bodies, err := json.Fmt(frm)
		if err != nil {
			h.dl.Error().Stack().Str("uid", mebUid).Err(err).Msg("format notify msg frame body failed")
			// ignore it
			break
		}

		if conns, ok := uid2conns[mebUid]; ok {
			for _, conn := range conns {
				fr := fcodec.NewNotifyFrame(bodies)
				b, _ := h.frCodec.Encode(fr)
				_, err = conn.Write(b)
				if err != nil {
					h.dl.Error().Str("uid", mebUid).Stack().Err(err).Msg("write notify data event for pushing failed")
					break
				}
			}
		}
	}

	return nil
}
