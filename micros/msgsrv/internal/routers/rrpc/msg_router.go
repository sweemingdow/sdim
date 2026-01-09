package rrpc

import (
	"github.com/lesismal/arpc"
	"github.com/sweemingdow/sdim/micros/msgsrv/internal/handlers/hrpc"
)

func ConfigureMsgRouter(srv *arpc.Server, handler *hrpc.MsgHandler) {
	srv.Handler.Handle("/batch_conv_recently_msgs", handler.HandlerBatchConvRecentlyMsgs)
}
