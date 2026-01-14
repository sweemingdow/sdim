package rhttp

import (
	"github.com/gofiber/fiber/v2"
	"github.com/sweemingdow/sdim/micros/msgsrv/internal/handlers/hhttp"
)

func ConfigureHistoryMsgRouter(fa *fiber.App, handler *hhttp.HistoryMsgHandler) {
	msgGrp := fa.Group("/msg")
	msgGrp.Get("/conv_history_msgs", handler.HandleConvHistoryMsgs)
}
