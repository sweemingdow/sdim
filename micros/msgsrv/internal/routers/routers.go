package routers

import (
	"github.com/gofiber/fiber/v2"
	"github.com/lesismal/arpc"
	"github.com/sweemingdow/gmicro_pkg/pkg/routebinder"
	"github.com/sweemingdow/sdim/micros/msgsrv/internal/handlers/hrpc"
	"github.com/sweemingdow/sdim/micros/msgsrv/internal/routers/rrpc"
)

type msgServerRouterBinder struct {
	msgHandler *hrpc.MsgHandler
}

func NewMsgServerRouterBinder(
	msgHandler *hrpc.MsgHandler,
) routebinder.AppRouterBinder {
	return &msgServerRouterBinder{
		msgHandler: msgHandler,
	}
}

func (msr msgServerRouterBinder) BindFiber(fa *fiber.App) {
}

func (msr msgServerRouterBinder) BindArpc(srv *arpc.Server) {
	rrpc.ConfigureMsgRouter(srv, msr.msgHandler)
}
