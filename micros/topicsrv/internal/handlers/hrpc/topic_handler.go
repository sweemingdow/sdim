package hrpc

import (
	"fmt"
	"github.com/lesismal/arpc"
	"github.com/sweemingdow/gmicro_pkg/pkg/mylog"
	"github.com/sweemingdow/gmicro_pkg/pkg/server/srpc"
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

func (h *TopicHandler) HandleMsgComing(c *arpc.Context) {
	var req rpccall.RpcReqWrapper[rpctopic.MsgComingReq]
	if ok := srpc.BindAndWriteLoggedIfError(c, &req); !ok {
		return
	}

	lg := rpccall.LoggerWrapWithReq(req, h.dl)
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
		srpc.WriteLoggedIfError(c, rpccall.SimpleParamValidateErr(fmt.Sprintf("invalid chat type:%d", rr.ChatType), ""))
		return
	}

	if rr.ChatType == chatconst.GroupChat {
		if rr.ConvId == "" {
			srpc.WriteLoggedIfError(c, rpccall.SimpleParamValidateErr(fmt.Sprintf("invalid convId with chatType:%d", rr.ChatType), ""))
			return
		}
	}

	param := core.MsgComingParamFrom(rr, req.ReqId)
	param.IsConvType = isConv

	rst := h.cm.OnMsgComing(param)

	if rst.Err != nil {
		lg.Error().Stack().Err(rst.Err).Msg("msg coming handle failed")

		var resp = rpccall.ErrGeneral(rst.Err.Error(), core.MsgComingResultTo(rst, param.ClientUniqueId))
		srpc.WriteLoggedIfError(c, resp)
	} else {
		var resp = rpccall.Ok(core.MsgComingResultTo(rst, param.ClientUniqueId))

		if ok := srpc.WriteLoggedIfError(c, resp); ok {
			lg = lg.With().Any("rpc_resp", resp).Logger()
			lg.Trace().Msg("msg coming handle completed")
		}
	}

}
