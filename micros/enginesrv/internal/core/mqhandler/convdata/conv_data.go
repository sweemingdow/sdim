package convdata

import (
	"github.com/nsqio/go-nsq"
	"github.com/sweemingdow/gmicro_pkg/pkg/mylog"
	"github.com/sweemingdow/gmicro_pkg/pkg/parser/json"
	"github.com/sweemingdow/sdim/external/eglobal/chatconst"
	"github.com/sweemingdow/sdim/external/eglobal/nsqconst/payload/convpd"
	"github.com/sweemingdow/sdim/micros/enginesrv/internal/codec/fcodec"
	"github.com/sweemingdow/sdim/micros/enginesrv/internal/core"
)

type convUnitDataHandler struct {
	connMgr core.ConnManager
	frCodec fcodec.FrameCodec
	dl      *mylog.DecoLogger
}

func NewConvUnitDataHandler(connMgr core.ConnManager, frCodec fcodec.FrameCodec) nsq.Handler {
	mfh := &convUnitDataHandler{
		connMgr: connMgr,
		frCodec: frCodec,
		dl:      mylog.NewDecoLogger("convUnitDataHandlerLogger"),
	}

	return mfh
}

func (h *convUnitDataHandler) HandleMessage(message *nsq.Message) error {
	h.dl.Debug().Msg("receive conv unit data msg")

	var pd convpd.ConvUnitDataUpdatePayload
	err := json.Parse(message.Body, &pd)
	if err != nil {
		h.dl.Error().Stack().Err(err).Msg("parse conv unit data payload failed")
		// give up
		return nil
	}

	lg := h.dl.GetLogger().With().
		Str("conv_id", pd.ConvId).
		Logger()

	lg.Debug().Msgf("receive conv unit data event for push, payload=%+v", pd)

	uid2conns := h.connMgr.GetUsersConns(pd.Members)

	for _, mebUid := range pd.Members {
		var updateType string
		dataMap := make(map[string]any, 2)
		dataMap["updateReason"] = pd.UpdateReason
		if pd.Title != nil {
			dataMap["title"] = *pd.Title
			updateType = chatconst.ConvTitleChanged
		}
		if pd.Icon != nil {
			dataMap["icon"] = *pd.Icon
			updateType = chatconst.ConvIconChanged
		}
		if pd.PinTop != nil {
			dataMap["pinTop"] = *pd.PinTop
			updateType = chatconst.ConvPinTopChanged
		}
		if pd.NoDisturb != nil {
			dataMap["noDisturb"] = *pd.NoDisturb
			updateType = chatconst.ConvNoDisturbChanged
		}

		cuf := fcodec.ConvUpdateFrame{
			ConvId: pd.ConvId,
			Type:   updateType,
			Data:   dataMap,
		}

		bodies, err := json.Fmt(cuf)
		if err != nil {
			lg.Error().Stack().Str("uid", mebUid).Err(err).Msg("format conv unit data frame body failed")
			// ignore it
			break
		}

		if conns, ok := uid2conns[mebUid]; ok {
			lg.Trace().Str("uid", mebUid).Msgf("conv unit data event pushing, connSize:%d", len(conns))

			for _, conn := range conns {
				fr := fcodec.NewConvUpdateFrame(bodies)
				b, _ := h.frCodec.Encode(fr)
				_, err = conn.Write(b)
				if err != nil {
					lg.Error().Str("uid", mebUid).Stack().Err(err).Msg("write conv unit data event for pushing failed")
					break
				}
			}
		}
	}

	return nil
}
