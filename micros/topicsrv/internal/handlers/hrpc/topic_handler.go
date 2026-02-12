package hrpc

import (
	"fmt"
	"github.com/sweemingdow/gmicro_pkg/pkg/mylog"
	"github.com/sweemingdow/gmicro_pkg/pkg/server/srpc/rpccall"
	"github.com/sweemingdow/sdim/external/eglobal/chatconst"
	"github.com/sweemingdow/sdim/external/erpc/rpctopic"
	"github.com/sweemingdow/sdim/micros/topicsrv/internal/core"
)

const (
	topicHandleLoggerName = "topicHandleLogger"
)

type TopicHandler struct {
	cm core.ConvManager
	dl *mylog.DecoLogger
}

func NewTopicHandler(cm core.ConvManager) *TopicHandler {
	return &TopicHandler{
		cm: cm,
		dl: mylog.NewDecoLogger(topicHandleLoggerName),
	}
}

func (h *TopicHandler) HandleMsgComing(req rpccall.RpcReqWrapper[rpctopic.MsgComingReq]) (rpctopic.MsgComingResp, error) {
	lg := req.BindLogger(h.dl)
	lg.Debug().Msg("receive msg coming")

	var zero rpctopic.MsgComingResp
	var rr = req.Req
	var (
		isConv    bool
		validChat bool
	)
	if chatconst.IsValidConvType(rr.ChatType) {
		isConv = true
		validChat = true
	} else if chatconst.IsDanmakuType(rr.ChatType) {
		isConv = false
		validChat = true
	} else {
		isConv = false
		validChat = false
	}

	if !validChat {
		return zero, rpccall.NewRpcParamInvalidErr(fmt.Sprintf("invalid chat type:%d", rr.ChatType))
	}

	if rr.ChatType == chatconst.GroupChat {
		if rr.ConvId == "" {
			return zero, rpccall.NewRpcParamInvalidErr(fmt.Sprintf("invalid convId with chatType:%d", rr.ChatType))
		}
	}

	param := core.MsgComingParamFrom(rr, req.ReqId)
	param.IsConvType = isConv

	rst := h.cm.OnMsgComing(param)

	if rst.Err != nil {
		lg.Error().Stack().Err(rst.Err).Msg("msg coming handle failed")
		return zero, rst.Err
	} else {

		//if ok := srpc.WriteResp(c, resp); ok {
		//	lg = lg.With().Any("rpc_resp", resp).Logger()
		//	lg.Trace().Msg("msg coming handle completed")
		//}
		return core.MsgComingResultTo(rst, param.ClientUniqueId), nil
	}

}

/*func (h *TopicHandler) HandleMsgComing(c *arpc.Context) {
	var req rpccall.RpcReqWrapper[rpctopic.MsgComingReq]
	ok := srpc.ParseReq(c, &req)
	if !ok {
		return
	}

	lg := rpccall.LoggerBindReqId(req, h.dl)
	lg.Debug().Msg("receive msg coming")

	var rr = req.Req
	var (
		isConv    bool
		validChat bool
	)
	if chatconst.IsValidConvType(rr.ChatType) {
		isConv = true
		validChat = true
	} else if chatconst.IsDanmakuType(rr.ChatType) {
		isConv = false
		validChat = true
	} else {
		isConv = false
		validChat = false
	}

	if !validChat {
		srpc.WriteResp(c, rpccall.NewParamInvalidResp(fmt.Sprintf("invalid chat type:%d", rr.ChatType)))
		return
	}

	if rr.ChatType == chatconst.GroupChat {
		if rr.ConvId == "" {
			srpc.WriteResp(c, rpccall.NewParamInvalidResp(fmt.Sprintf("invalid convId with chatType:%d", rr.ChatType)))
			return
		}
	}

	param := core.MsgComingParamFrom(rr, req.ReqId)
	param.IsConvType = isConv

	rst := h.cm.OnMsgComing(param)

	if rst.Err != nil {
		lg.Error().Stack().Err(rst.Err).Msg("msg coming handle failed")

		var resp = rpccall.ErrGeneral(rst.Err.Error(), core.MsgComingResultTo(rst, param.ClientUniqueId))
		srpc.WriteResp(c, resp)
		c.Error()
	} else {
		var resp = rpccall.Ok(core.MsgComingResultTo(rst, param.ClientUniqueId))

		if ok := srpc.WriteResp(c, resp); ok {
			lg = lg.With().Any("rpc_resp", resp).Logger()
			lg.Trace().Msg("msg coming handle completed")
		}
	}

}*/
