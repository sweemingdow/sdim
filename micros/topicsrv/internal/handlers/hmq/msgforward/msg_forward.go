package msgforward

import (
	"github.com/nsqio/go-nsq"
	"github.com/sweemingdow/gmicro_pkg/pkg/component/cnsq"
	"github.com/sweemingdow/gmicro_pkg/pkg/mylog"
	"github.com/sweemingdow/gmicro_pkg/pkg/parser/json"
	"github.com/sweemingdow/sdim/external/eglobal/nsqconst"
	"github.com/sweemingdow/sdim/external/eglobal/nsqconst/payload/convpd"
	"github.com/sweemingdow/sdim/external/eglobal/nsqconst/payload/msgpd"
	"github.com/sweemingdow/sdim/micros/topicsrv/internal/core"
)

type msgForwardHandler struct {
	cm    core.ConvManager
	nsqPd *cnsq.NsqProducer
}

func NewMsgForwardHandler(cm core.ConvManager, nsqPd *cnsq.NsqProducer) nsq.Handler {
	mfh := &msgForwardHandler{
		cm:    cm,
		nsqPd: nsqPd,
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

	lg.Debug().Msgf("receive msg for forward, msg=%+v, conent=%+v", *msp.Msg, *msp.Msg.Content)

	msr := mfh.cm.OnMsgStored(&msp)

	lg.Trace().Msgf("on msg stored completed, msr%+v", msr)

	// mq到engine_server, 推ConvUpdateFrame帧到客户端
	pd := convpd.ConvLastMsgUpdateEventPayload{
		ConvId:          msp.ConvId,
		ConvType:        msp.ConvType,
		Members:         append([]string{msp.Sender}, msp.Members...),
		LastMsg:         msr.LastMsg,
		LastActiveTs:    msr.LastActiveTs,
		Uid2UnreadCount: msr.Uid2UnreadCount,
	}

	payloads, err := json.Fmt(pd)
	if err != nil {
		lg.Error().Stack().Err(err).Msg("format payload failed")

		// ignore it
		return nil
	}

	err = mfh.nsqPd.PublishAsync(
		cnsq.PublishParam{
			Topic:   nsqconst.ConvUpdateTopic,
			Payload: payloads,
		},
		nil,
		nil,
	)

	if err != nil {
		lg.Error().Stack().Err(err).Msgf("publish to %s failed, content=%+v", nsqconst.ConvUpdateTopic, *msp.Msg.Content)
		// ignore it
		return nil
	}

	return nil
}
