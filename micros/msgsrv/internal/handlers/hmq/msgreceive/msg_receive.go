package msgreceive

import (
	"github.com/nsqio/go-nsq"
	"github.com/sweemingdow/gmicro_pkg/pkg/component/cnsq"
	"github.com/sweemingdow/gmicro_pkg/pkg/mylog"
	"github.com/sweemingdow/gmicro_pkg/pkg/parser/json"
	"github.com/sweemingdow/gmicro_pkg/pkg/server/srpc/rpccall"
	"github.com/sweemingdow/sdim/external/eglobal/nsqconst"
	"github.com/sweemingdow/sdim/external/eglobal/nsqconst/payload/msgpd"
	"github.com/sweemingdow/sdim/external/emodel/chatmodel"
	"github.com/sweemingdow/sdim/external/emodel/msgmodel"
	"github.com/sweemingdow/sdim/external/emodel/msgmodel/msgpojo"
	"github.com/sweemingdow/sdim/external/erpc/rpcuser"
	"github.com/sweemingdow/sdim/micros/msgsrv/internal/repostories/msgrepo"
	"github.com/sweemingdow/sdim/pkg/async"
	"time"
)

const (
	maxRetryTimes = 1
)

type msgReceiveHandler struct {
	mr           msgrepo.MsgRepository
	userProvider rpcuser.UserInfoRpcProvider
	ah           async.AsyncHandler
	nsqPd        *cnsq.NsqProducer
	dl           *mylog.DecoLogger
}

func NewMsgReceiveHandler(mr msgrepo.MsgRepository, userProvider rpcuser.UserInfoRpcProvider, nsqPd *cnsq.NsqProducer) nsq.Handler {
	return &msgReceiveHandler{
		mr:           mr,
		userProvider: userProvider,
		ah: async.NewCallerRunHandler(async.CallerRunOptions{
			CoreWorkers:      8,
			MaxWorkers:       128,
			MaxWaitQueueSize: 2048,
			MaxIdleTimeout:   30 * time.Second,
		}),
		nsqPd: nsqPd,
		dl:    mylog.NewDecoLogger("msgReceiveLogger"),
	}
}

func (mrh *msgReceiveHandler) HandleMessage(message *nsq.Message) error {
	message.DisableAutoResponse()

	var msgPd msgpd.MsgSendReceivePayload
	err := json.Parse(message.Body, &msgPd)
	if err != nil {
		mrh.dl.Error().Stack().Err(err).Msg("parse msg payload failed")
		// 不需要重试了
		message.Finish()
		return nil
	}

	lg := mrh.dl.GetLogger().With().
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

	err = mrh.ah.Submit(func() {
		var sendInfo msgmodel.SenderInfo
		if chatmodel.IsUserSend(msgPd.SenderType) {
			resp, ie := mrh.userProvider.UserUnitInfo(rpccall.CreateIdReq(msgPd.ReqId, rpcuser.UserUnitInfoReq{Uid: msgPd.Sender}))

			if ie != nil {
				lg.Error().Stack().Err(ie).Msgf("find user with rpc failed, uid:%s", msgPd.Sender)
				message.Requeue(500 * time.Millisecond)
				return
			}

			info, ie := resp.OkOrErr()
			if ie != nil {
				lg.Error().Stack().Err(ie).Msgf("find user with rpc response failed, uid:%s", msgPd.Sender)
				message.Requeue(100 * time.Millisecond)
				return
			}

			sendInfo = msgmodel.SenderInfo{
				SenderType: msgPd.SenderType,
				Nickname:   info.Nickname,
				Avatar:     info.Avatar,
			}
		} else {
			sendInfo = msgmodel.SenderInfo{
				SenderType: msgPd.SenderType,
			}
		}

		msg := &msgpd.Msg{
			SenderInfo: sendInfo,
			Content:    msgPd.MsgContent,
		}

		bodies, _ := json.Fmt(msg)

		pojo, tsMills, _ := mrh.msgPd2pojo(msgPd, bodies)

		_, ie := mrh.mr.UpsertMsg(300*time.Millisecond, pojo)

		if ie != nil {
			lg.Error().Stack().Err(ie).Msgf("upsert msg failed, msgPayload:%+v", msgPd)
			message.Requeue(100 * time.Millisecond)
			return
		}

		lg.Trace().Msg("upsert msg completed, publishing to engine server")

		fpd := msgpd.ReceivePd2forwardPd(&msgPd, msg, tsMills)
		data, _ := json.Fmt(fpd)

		// upsert成功
		// 发送到engine_server, 转发到对应客户端
		// 发送到topic_server, 更新会话
		ie = mrh.nsqPd.Publish(cnsq.PublishParam{
			Topic:   nsqconst.MsgForwardTopic,
			Payload: data,
		})

		if ie != nil {
			lg.Error().Stack().Err(ie).Msg("publishing to engine server failed")
			message.Requeue(100 * time.Millisecond)
		}

		message.Finish()
	})

	if err != nil {
		message.Requeue(500 * time.Millisecond)
		return err
	}

	return nil
}

func (mrh *msgReceiveHandler) msgPd2pojo(msgPd msgpd.MsgSendReceivePayload, bodies []byte) (*msgpojo.Msg, int64, error) {
	var (
		now      = time.Now()
		tsMills  = now.UnixMilli()
		tsSec    = now.Unix()
		expireAt int64
	)

	if msgPd.Ttl > 0 {
		expireAt = tsSec + int64(msgPd.Ttl)
	} else if msgPd.Ttl < 0 {
		expireAt = -1
	}

	var pojo = &msgpojo.Msg{
		Id:        msgPd.MsgId,
		ConvId:    msgPd.ConvId,
		ChatType:  int8(msgPd.ChatType),
		MsgType:   int32(msgPd.MsgContent.Type),
		Sender:    msgPd.Sender,
		Receiver:  msgPd.Receiver,
		Seq:       msgPd.MsgSeq,
		MsgBody:   string(bodies),
		ExpiredAt: expireAt,
		Cts:       tsMills,
		Uts:       tsMills,
	}

	return pojo, tsMills, nil
}
