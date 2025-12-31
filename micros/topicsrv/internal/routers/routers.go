package routers

import (
	"github.com/gofiber/fiber/v2"
	"github.com/lesismal/arpc"
	"github.com/sweemingdow/gmicro_pkg/pkg/routebinder"
	"github.com/sweemingdow/sdim/micros/topicsrv/internal/handlers/hrpc"
	"github.com/sweemingdow/sdim/micros/topicsrv/internal/routers/rrpc"
)

type topicServerRouteBinder struct {
	topicHandler *hrpc.TopicHandler
}

func (tsr *topicServerRouteBinder) BindFiber(_ *fiber.App) {

}

func (tsr *topicServerRouteBinder) BindArpc(srv *arpc.Server) {
	rrpc.ConfigureTopicRouter(srv, tsr.topicHandler)
}

func NewTopicServerRouteBinder(topicHandler *hrpc.TopicHandler) routebinder.AppRouterBinder {
	return &topicServerRouteBinder{
		topicHandler: topicHandler,
	}
}
