package msgforward

import (
	"github.com/nsqio/go-nsq"
	"github.com/sweemingdow/gmicro_pkg/pkg/mylog"
	"github.com/sweemingdow/gmicro_pkg/pkg/parser/json"
	"github.com/sweemingdow/sdim/external/eglobal/nsqconst/payload/msgpd"
	"github.com/sweemingdow/sdim/micros/topicsrv/internal/core"
)

type msgForwardHandler struct {
	cm core.ConvManager
}

func NewMsgForwardHandler(cm core.ConvManager) nsq.Handler {
	mfh := &msgForwardHandler{
		cm: cm,
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

	lg = lg.With().
		Str("req_id", msp.ReqId).
		Str("conv_id", msp.ConvId).
		Int64("msg_id", msp.MsgId).
		Logger()

	lg.Debug().Msgf("receive msg for forward, msg:%+v, conent:%+v", *msp.Msg, *msp.Msg.Content)

	mfh.cm.OnMsgStored(&msp)

	return nil
}
