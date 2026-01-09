package routers

import (
	"github.com/gofiber/fiber/v2"
	"github.com/lesismal/arpc"
	"github.com/sweemingdow/gmicro_pkg/pkg/routebinder"
	"github.com/sweemingdow/sdim/micros/topicsrv/internal/handlers/hhttp"
	"github.com/sweemingdow/sdim/micros/topicsrv/internal/handlers/hrpc"
	"github.com/sweemingdow/sdim/micros/topicsrv/internal/routers/rhttp"
	"github.com/sweemingdow/sdim/micros/topicsrv/internal/routers/rrpc"
)

type topicServerRouteBinder struct {
	topicHandler    *hrpc.TopicHandler
	convHttpHandler *hhttp.ConvHttpHandler
}

func NewTopicServerRouteBinder(
	topicHandler *hrpc.TopicHandler,
	convHttpHandler *hhttp.ConvHttpHandler,
) routebinder.AppRouterBinder {
	return &topicServerRouteBinder{
		topicHandler:    topicHandler,
		convHttpHandler: convHttpHandler,
	}
}

func (tsr *topicServerRouteBinder) BindFiber(fa *fiber.App) {
	rhttp.ConfigConvRouter(fa, tsr.convHttpHandler)
}

func (tsr *topicServerRouteBinder) BindArpc(srv *arpc.Server) {
	rrpc.ConfigureTopicRouter(srv, tsr.topicHandler)
}
