package rrpc

import (
	"github.com/lesismal/arpc"
	"github.com/sweemingdow/sdim/micros/topicsrv/internal/handlers/hrpc"
)

func ConfigureTopicRouter(srv *arpc.Server, handler *hrpc.TopicHandler) {
	srv.Handler.Handle("/msg_coming", handler.HandleMsgComing)
}
