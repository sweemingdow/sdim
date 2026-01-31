package convadd

import (
	"github.com/nsqio/go-nsq"
	"github.com/sweemingdow/gmicro_pkg/pkg/mylog"
	"github.com/sweemingdow/gmicro_pkg/pkg/parser/json"
	"github.com/sweemingdow/gmicro_pkg/pkg/utils/usli"
	"github.com/sweemingdow/sdim/external/eglobal/chatconst"
	"github.com/sweemingdow/sdim/external/eglobal/nsqconst/payload/convpd"
	"github.com/sweemingdow/sdim/micros/enginesrv/internal/codec/fcodec"
	"github.com/sweemingdow/sdim/micros/enginesrv/internal/core"
)

type convAddHandler struct {
	connMgr core.ConnManager
	frCodec fcodec.FrameCodec
}

func NewConvAddHandler(connMgr core.ConnManager, frCodec fcodec.FrameCodec) nsq.Handler {
	mfh := &convAddHandler{
		connMgr: connMgr,
		frCodec: frCodec,
	}

	return mfh
}

func (mfh *convAddHandler) HandleMessage(message *nsq.Message) error {
	lg := mylog.AppLogger()

	var pd convpd.ConvAddEventPayload
	err := json.Parse(message.Body, &pd)
	if err != nil {
		lg.Error().Stack().Err(err).Msg("parse conv add payload failed")
		// give up
		return nil
	}

	lg = lg.With().
		Str("conv_id", pd.ConvId).
		Logger()

	lg.Debug().Msgf("receive conv add event for push, payload:%+v", pd)

	uid2conns := mfh.connMgr.GetUsersConns(pd.Members)

	var (
		icon       string
		title      string
		relationId string
	)

	for _, mebUid := range pd.Members {
		icon = pd.Icon
		if icon == "" {
			icon = pd.MebId2UnitInfo[mebUid].Icon
		}

		title = pd.Title
		if title == "" {
			title = pd.MebId2UnitInfo[mebUid].Title
		}

		dataMap := make(map[string]any, 10)
		dataMap["icon"] = icon
		dataMap["title"] = title
		dataMap["ts"] = pd.Ts
		dataMap["convType"] = pd.ConvType
		dataMap["chatType"] = pd.ChatType
		dataMap["sender"] = pd.Sender
		dataMap["receiver"] = pd.Receiver
		//dataMap["followMsg"] = pd.FollowMsg

		relationId = pd.RelationId
		if relationId == "" {
			relUid, ok := usli.FindFirstIf(pd.Members, func(val string) bool {
				return val != mebUid
			})

			if !ok {
				continue
			}

			relationId = relUid
		}

		dataMap["relationId"] = relationId

		cuf := fcodec.ConvUpdateFrame{
			ConvId: pd.ConvId,
			Type:   chatconst.ConvAdded,
			Data:   dataMap,
		}

		bodies, err := json.Fmt(cuf)
		if err != nil {
			lg.Error().Stack().Str("uid", mebUid).Err(err).Msg("format conv update frame body failed")
			// ignore it
			return nil
		}

		if conns, ok := uid2conns[mebUid]; ok {
			lg.Trace().Str("uid", mebUid).Msgf("conv add event pushing, connSize:%d", len(conns))

			for _, conn := range conns {
				fr := fcodec.NewConvUpdateFrame(bodies)
				b, _ := mfh.frCodec.Encode(fr)
				_, err = conn.Write(b)
				if err != nil {
					lg.Error().Str("uid", mebUid).Stack().Err(err).Msg("write conv add event for pushing failed")
					break
				}
			}
		}
	}

	return nil
}
