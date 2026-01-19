package nsqsend

import (
	"github.com/nsqio/go-nsq"
	"github.com/rs/zerolog"
	"github.com/sweemingdow/gmicro_pkg/pkg/component/cnsq"
	"github.com/sweemingdow/gmicro_pkg/pkg/parser/json"
	"github.com/sweemingdow/sdim/external/eglobal/nsqconst"
	"github.com/sweemingdow/sdim/external/eglobal/nsqconst/payload/convpd"
	"github.com/sweemingdow/sdim/external/eglobal/nsqconst/payload/msgpd"
)

type MsgSender struct {
	nsdPd *cnsq.NsqProducer
}

func NewMsgSender(nsdPd *cnsq.NsqProducer) *MsgSender {
	return &MsgSender{
		nsdPd: nsdPd,
	}
}

func (ms *MsgSender) SendMsg(pd *msgpd.MsgSendReceivePayload, doneChan chan *nsq.ProducerTransaction, args []any) error {
	b, err := json.Fmt(pd)
	if err != nil {
		return err
	}
	// 拿到会话, mq异步移到msg_srv
	err = ms.nsdPd.PublishAsync(
		cnsq.PublishParam{
			Topic:   nsqconst.MsgReceiveTopic,
			Payload: b,
		},
		doneChan,
		args,
	)

	return err
}

func (ms *MsgSender) SendConvAddEvent(pd convpd.ConvAddEventPayload, lg zerolog.Logger) {
	payloads, err := json.Fmt(pd)
	if err != nil {
		lg.Error().Stack().Err(err).Msgf("format conv add event payload failed, pd=%+v", pd)
		return
	}

	err = ms.nsdPd.PublishAsync(
		cnsq.PublishParam{
			Topic:   nsqconst.ConvAddTopic,
			Payload: payloads,
		},
		nil,
		nil,
	)

	if err != nil {
		lg.Error().Stack().Err(err).Msgf("publish conv add event payload failed, pd=%+v", pd)
	}
}
