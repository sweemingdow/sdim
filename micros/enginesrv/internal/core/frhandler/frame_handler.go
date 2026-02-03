package frhandler

import (
	"fmt"
	"github.com/google/uuid"
	"github.com/panjf2000/gnet/v2"
	"github.com/panjf2000/gnet/v2/pkg/pool/goroutine"
	"github.com/rs/zerolog"
	"github.com/sweemingdow/gmicro_pkg/pkg/app"
	"github.com/sweemingdow/gmicro_pkg/pkg/component/cnsq"
	"github.com/sweemingdow/gmicro_pkg/pkg/myerr"
	"github.com/sweemingdow/gmicro_pkg/pkg/mylog"
	"github.com/sweemingdow/gmicro_pkg/pkg/server/srpc/rpccall"
	"github.com/sweemingdow/gmicro_pkg/pkg/utils/usli"
	"github.com/sweemingdow/sdim/external/erpc/rpctopic"
	"github.com/sweemingdow/sdim/external/erpc/rpcuser"
	"github.com/sweemingdow/sdim/micros/enginesrv/internal/codec/fcodec"
	"github.com/sweemingdow/sdim/micros/enginesrv/internal/core"
	"time"
)

type asyncFrameHandler struct {
	pool             *goroutine.Pool
	cm               core.ConnManager
	frCodec          fcodec.FrameCodec
	nsqProducer      *cnsq.NsqProducer
	topicProvider    rpctopic.TopicRpcProvider
	userInfoProvider rpcuser.UserInfoRpcProvider
	dl               *mylog.DecoLogger
	isProd           bool
}

func NewAsyncFrameHandler(
	frCodec fcodec.FrameCodec,
	cm core.ConnManager,
	nsqProducer *cnsq.NsqProducer,
	topicProvider rpctopic.TopicRpcProvider,
	userInfoProvider rpcuser.UserInfoRpcProvider,
) core.FrameHandler {
	return &asyncFrameHandler{
		pool:             goroutine.Default(),
		frCodec:          frCodec,
		cm:               cm,
		nsqProducer:      nsqProducer,
		topicProvider:    topicProvider,
		userInfoProvider: userInfoProvider,
		dl:               mylog.NewDecoLogger("frameLogger"),
		isProd:           app.GetTheApp().IsProdProfile(),
	}
}

func (fh *asyncFrameHandler) Handle(connAuthed bool, c gnet.Conn, frs []fcodec.Frame) gnet.Action {
	ccCtx := core.GetConnCtx(c)
	lg := core.LoggerWithCcCtx(ccCtx, fh.dl)

	// 未认证连接：只处理单个 Conn 帧
	if !connAuthed {
		if len(frs) != 1 || frs[0].Header.Ftype != fcodec.Conn {
			lg.Warn().Int("frs_len", len(frs)).Str("frame_type", fcodec.GetFrameTypeDesc(frs[0].Header.Ftype)).Msg("unauthenticated conn sent invalid frames, will be close")
			return gnet.Close
		}

		err := fh.handleFrames(ccCtx.Id, c, frs, lg)

		if err != nil {
			lg.Error().Stack().Err(err).Msgf("handle %s frame failed", fcodec.GetFrameTypeDesc(frs[0].Header.Ftype))
			return gnet.Close
		}

		return gnet.None
	}

	idx2frs := usli.GroupByIt(frs, func(fr fcodec.Frame) int {
		// 分为2组, ping单独一组, 其他一组
		if fr.Header.Ftype == fcodec.Ping {
			return 0
		} else {
			return 1
		}
	})

	// ping组同步处理掉
	pingFrs := idx2frs[0]
	if len(pingFrs) > 0 {
		for _, fr := range pingFrs {
			lg.Trace().Msgf("start handle Ping frame data")

			if err := fh.handlePingFrame(c, fr); err != nil {
				lg.Error().Stack().Err(err).Msg("handle Ping frame failed")
				return gnet.Close
			} else {
				fh.cm.ModifyAfterPingSuccess(ccCtx.Id)
			}
		}
	}

	// 其他组异步处理
	otherFrs := idx2frs[1]
	if len(otherFrs) > 0 {
		if err := fh.handleFrames(ccCtx.Id, c, otherFrs, lg); err != nil {
			frsDesc := usli.Conv(frs, func(fr fcodec.Frame) string {
				return fcodec.GetFrameTypeDesc(fr.Header.Ftype)
			})

			lg.Error().Stack().Strs("frame_types", frsDesc).Err(err).Msg("handle  frames failed")
			return gnet.Close
		}
	}

	return gnet.None
}

func (fh *asyncFrameHandler) handlePingFrame(c gnet.Conn, fr fcodec.Frame) error {
	pongFr := fcodec.NewPongFrame(fr.Payload)

	pongBytes, _ := fh.frCodec.Encode(pongFr)
	_, err := c.Write(pongBytes)
	return err
}

func (fh *asyncFrameHandler) handleFrames(connId string, c gnet.Conn, frs []fcodec.Frame, lg zerolog.Logger) error {
	if e := lg.Trace(); e.Enabled() {
		frsDesc := usli.Conv(frs, func(fr fcodec.Frame) string {
			return fcodec.GetFrameTypeDesc(fr.Header.Ftype)
		})

		e.Strs("frame_types", frsDesc).Msgf("start handle frames data")
	}

	if len(frs) == 1 && frs[0].Header.Ftype == fcodec.Conn {
		sn := time.Now().UnixMilli()
		connFr := frs[0]
		return fh.pool.Submit(func() {
			// 处理连接帧
			fh.handleConnFrame(connId, connFr, lg, c, sn)
		})
	}

	// 处理其他帧
	// 只能是Send帧或RecvAck帧
	err := fh.validExceptFrames(frs, c)
	if err != nil {
		return err
	}

	return fh.pool.Submit(func() {
		for _, fr := range frs {
			if fr.Header.Ftype == fcodec.Send { // Send帧
				var sendFrb fcodec.SendFrame
				derr := fcodec.DecodePayload(fr.Payload, &sendFrb)
				if derr != nil {
					lg.Error().Stack().Err(derr).Msg("decode send frame body failed, kick it")
					_ = c.Close()
					return
				}

				reqId := fh.extractReqId(fr)

				clg := lg.With().Str("req_id", reqId).Logger()

				clg.Debug().Msgf("handle send frame, frame:%+v, msgContent:%+v", sendFrb, *sendFrb.MsgContent)

				req := rpctopic.MsgComingReq{
					ConvId:         sendFrb.ConvId,
					Sender:         sendFrb.Sender,
					Receiver:       sendFrb.Receiver,
					ChatType:       sendFrb.ChatType,
					Ttl:            sendFrb.Ttl,
					ClientUniqueId: sendFrb.ClientUniqueId,
					MsgContent:     sendFrb.MsgContent,
				}

				// rpc: 调用TopicServer
				resp, re := fh.topicProvider.MsgComing(rpccall.CreateIdReq(reqId, req))

				if re != nil {
					clg.Error().Stack().Err(re).Msg("call rpc failed when msg coming")
					app.GetTheApp().IsDevProfile()

					fh.writeSendAckFrame(
						fcodec.SendFrameAck{
							ErrCode: fcodec.ServerErr,
							ErrDesc: re.Error(),
						},
						fr,
						c,
						clg,
					)
					return
				}

				r, e := resp.OkOrErr()
				if e != nil {
					clg.Error().Err(e).Msgf("rpc call resp invalid")

					rre, _ := myerr.DecodeRpcRespError(e)

					fh.writeSendAckFrame(
						fcodec.SendFrameAck{
							ErrCode: fcodec.RpcRespErr,
							ErrDesc: rre.ErrDesc,
						},
						fr,
						c,
						clg,
					)
					return
				}

				lg.Debug().Msgf("msg coming return resp:%+v", r)

				data := resp.Data
				fh.writeSendAckFrame(
					fcodec.SendFrameAck{
						ErrCode: fcodec.OK,
						Data: fcodec.SendFrameAckBody{
							MsgId:          data.MsgId,
							ClientUniqueId: data.ClientUniqueId,
							ConvId:         data.ConvId,
							MsgSeq:         data.MsgSeq,
							SendTs:         data.SendTs,
						},
					},
					fr,
					c,
					clg,
				)
			} else { // RecvAck帧

			}
		}
	})
}

func (fh *asyncFrameHandler) extractReqId(fr fcodec.Frame) string {
	var reqId string
	if uuidVal, e := uuid.FromBytes(fr.Payload.ReqId[:]); e != nil {
		reqId = string(fr.Payload.ReqId[:])
	} else {
		reqId = uuidVal.String()
	}
	return reqId
}

func (fh *asyncFrameHandler) writeConnAckFrame(caf fcodec.ConnAckFrame, fr fcodec.Frame, c gnet.Conn, clg zerolog.Logger) {
	if fh.isProd && caf.ErrCode != fcodec.BizErr {
		caf.ErrDesc = fcodec.TransferDesc(caf.ErrCode)
	}

	sfr, err := fcodec.NewS2cFrame(
		fr.Payload,
		fcodec.ConnAck,
		caf,
	)

	if err != nil {
		clg.Error().Stack().Err(err).Msg("create conn ack frame failed, will be close")
		_ = c.Close()
		return
	}

	frBytes, err := fh.frCodec.Encode(sfr)
	if err != nil {
		clg.Error().Stack().Err(err).Msg("encode conn ack frame failed, will be close")
		_ = c.Close()
		return
	}

	_ = c.AsyncWrite(frBytes, func(c gnet.Conn, err error) error {
		if err != nil {
			clg.Error().Stack().Err(err).Msg("async write conn ack frame failed")
		}

		return nil
	})
}

func (fh *asyncFrameHandler) writeSendAckFrame(sfa fcodec.SendFrameAck, fr fcodec.Frame, c gnet.Conn, clg zerolog.Logger) {
	if fh.isProd {
		sfa.ErrDesc = fcodec.TransferDesc(sfa.ErrCode)
	}

	sfr, err := fcodec.NewS2cFrame(
		fr.Payload,
		fcodec.SendAck,
		sfa,
	)

	if err != nil {
		clg.Error().Stack().Err(err).Msg("create send ack frame failed, will be close")
		_ = c.Close()
		return
	}

	frBytes, err := fh.frCodec.Encode(sfr)
	if err != nil {
		clg.Error().Stack().Err(err).Msg("encode send ack frame failed, will be close")
		_ = c.Close()
		return
	}

	_ = c.AsyncWrite(frBytes, func(c gnet.Conn, err error) error {
		if err != nil {
			clg.Error().Stack().Err(err).Msg("async write send ack frame failed")
		}

		return nil
	})
}

func (fh *asyncFrameHandler) validExceptFrames(frs []fcodec.Frame, c gnet.Conn) error {
	for _, fr := range frs {
		if fr.Header.Ftype != fcodec.Send && fr.Header.Ftype != fcodec.ForwardAck {
			_ = c.Close()
			return fmt.Errorf("invalid frame:%s be handle, kick it", fcodec.GetFrameTypeDesc(fr.Header.Ftype))
		}
	}
	return nil
}

func (fh *asyncFrameHandler) handleConnFrame(connId string, connFr fcodec.Frame, lg zerolog.Logger, c gnet.Conn, sn int64) {
	var cf fcodec.ConnFrame
	err := fcodec.DecodePayload(connFr.Payload, &cf)
	if err != nil {
		lg.Error().Stack().Err(err).Msg("decode conn frame body failed, will be close")
		_ = c.Close()
		return
	}

	lg.Debug().Msgf("handle user conn frame start, body:%+v", cf)

	reqId := fh.extractReqId(connFr)
	req := rpccall.CreateIdReq(reqId, rpcuser.UserStateReq{Uid: cf.Uid})

	// rpc验证用户
	resp, err := fh.userInfoProvider.UserState(req)

	lg = lg.With().Str("uid", cf.Uid).Logger()

	if err != nil {
		lg.Error().Stack().Err(err).Msg("user state rpc failed")
		caf := fcodec.ConnAckFrame{
			ErrCode:  fcodec.ServerErr,
			ErrDesc:  err.Error(),
			TimeDiff: sn - cf.TsMills,
		}

		fh.writeConnAckFrame(caf, connFr, c, lg)

		return
	}

	lg.Trace().Any("resp", resp).Msg("user state rpc success")

	state, err := resp.OkOrErr()
	if err != nil {
		caf := fcodec.ConnAckFrame{
			ErrCode:  fcodec.RpcRespErr,
			ErrDesc:  err.Error(),
			TimeDiff: sn - cf.TsMills,
		}

		fh.writeConnAckFrame(caf, connFr, c, lg)

		return
	}

	if !state.IsOk() {
		caf := fcodec.ConnAckFrame{
			ErrCode:  fcodec.BizErr,
			ErrDesc:  fmt.Sprintf("user state invalid:%d", state),
			TimeDiff: sn - cf.TsMills,
		}

		fh.writeConnAckFrame(caf, connFr, c, lg)

		return
	}

	// 认证成功, 修改
	fh.cm.ModifyAfterAuthed(core.ConnModifyParam{
		ConnId: connId,
		Uid:    cf.Uid,
		CType:  cf.CType,
	})

	caf := fcodec.ConnAckFrame{
		ErrCode:  fcodec.OK,
		TimeDiff: sn - cf.TsMills,
	}

	fh.writeConnAckFrame(caf, connFr, c, lg)

	lg.Debug().Msgf("handle user conn frame completed, body:%+v", cf)
}
