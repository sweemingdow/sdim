package msgreceive

import (
	"github.com/nsqio/go-nsq"
	"github.com/sweemingdow/gmicro_pkg/pkg/mylog"
	"github.com/sweemingdow/gmicro_pkg/pkg/parser/json"
	"github.com/sweemingdow/sdim/external/eglobal/nsqconst/payload/msgpd"
	"github.com/sweemingdow/sdim/micros/topicsrv/internal/core"
)

const (
	maxRetryTimes = 1
)

type msgReceiveHandler struct {
	cm core.ConvManager
}

func NewMsgReceiveHandler(cm core.ConvManager) nsq.Handler {
	return &msgReceiveHandler{
		cm: cm,
	}
}

func (mrh *msgReceiveHandler) HandleMessage(message *nsq.Message) error {
	message.DisableAutoResponse()

	lg := mylog.AppLogger()

	var msgPd msgpd.MsgSendReceivePayload
	err := json.Parse(message.Body, &msgPd)
	if err != nil {
		lg.Error().Stack().Err(err).Msg("parse msg payload failed")
		message.Finish()
		return nil
	}

	lg = lg.With().
		Str("conv_id", msgPd.ConvId).
		Int64("msg_id", msgPd.MsgId).
		Str("req_id", msgPd.ReqId).
		Logger()

	lg.Debug().Uint16("attempts", message.Attempts).Msgf("received msg:%+v", msgPd)

	if message.Attempts >= maxRetryTimes+1 {
		lg.Warn().Msg("msg handle reached max retry times, throw it")
		message.Finish()
		return nil
	}

	//mrh.cm.OnMsgComing()

	return nil
}
