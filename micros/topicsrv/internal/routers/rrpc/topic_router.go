package rrpc

import (
	"github.com/lesismal/arpc"
	"github.com/sweemingdow/gmicro_pkg/pkg/middleware/arpcmw"
	"github.com/sweemingdow/sdim/micros/topicsrv/internal/handlers/hrpc"
)

func ConfigureTopicRouter(srv *arpc.Server, handler *hrpc.TopicHandler) {
	srv.Handler.Use(func(c *arpc.Context) {
		c.Next()
	})

	srv.Handler.Handle("/msg_coming", arpcmw.BindAndWrite(handler.HandleMsgComing))
}
