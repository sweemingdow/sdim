package routers

import (
	"github.com/gofiber/fiber/v2"
	"github.com/lesismal/arpc"
	"github.com/sweemingdow/gmicro_pkg/pkg/routebinder"
	"github.com/sweemingdow/sdim/micros/msgsrv/internal/handlers/hhttp"
	"github.com/sweemingdow/sdim/micros/msgsrv/internal/handlers/hrpc"
	"github.com/sweemingdow/sdim/micros/msgsrv/internal/routers/rhttp"
	"github.com/sweemingdow/sdim/micros/msgsrv/internal/routers/rrpc"
)

type msgServerRouterBinder struct {
	msgHandler        *hrpc.MsgHandler
	historyMsgHandler *hhttp.HistoryMsgHandler
}

func NewMsgServerRouterBinder(
	msgHandler *hrpc.MsgHandler,
	historyMsgHandler *hhttp.HistoryMsgHandler,
) routebinder.AppRouterBinder {
	return &msgServerRouterBinder{
		msgHandler:        msgHandler,
		historyMsgHandler: historyMsgHandler,
	}
}

func (msr msgServerRouterBinder) BindFiber(fa *fiber.App) {
	rhttp.ConfigureHistoryMsgRouter(fa, msr.historyMsgHandler)
}

func (msr msgServerRouterBinder) BindArpc(srv *arpc.Server) {
	rrpc.ConfigureMsgRouter(srv, msr.msgHandler)
}
