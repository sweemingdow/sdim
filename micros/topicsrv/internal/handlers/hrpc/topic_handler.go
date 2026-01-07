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

type TopicHandler struct {
	cm core.ConvManager
}

func NewTopicHandler(cm core.ConvManager) *TopicHandler {
	return &TopicHandler{
		cm: cm,
	}
}

func (th *TopicHandler) HandleMsgComing(c *arpc.Context) {
	lg := mylog.AppLogger()

	var req rpccall.RpcReqWrapper[rpctopic.MsgComingReq]
	if ok := srpc.BindAndWriteLoggedIfError(c, &req); !ok {
		return
	}

	qlg := rpccall.LoggerWrapWithReq(req, lg)
	qlg.Debug().Msg("receive msg coming")

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

	param := core.MsgComingParamFrom(rr, req.ReqId)
	param.IsConvType = isConv

	rst := th.cm.OnMsgComing(param)

	if rst.Err != nil {
		plg := rpccall.LoggerWrapWithReq(req, lg)
		plg.Error().Stack().Err(rst.Err).Msg("msg coming handle failed")

		var resp = rpccall.SimpleErrDesc(rst.Err.Error())
		srpc.WriteLoggedIfError(c, resp)
	} else {
		var resp = rpccall.Ok(core.MsgComingResultTo(rst, param.ClientUniqueId))

		if ok := srpc.WriteLoggedIfError(c, resp); ok {
			plg := rpccall.LoggerWrapWithResp(req.ReqId, resp, lg)
			plg.Trace().Msg("msg coming handle completed")
		}
	}

}
