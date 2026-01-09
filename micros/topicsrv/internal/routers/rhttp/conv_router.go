package rhttp

import (
	"github.com/gofiber/fiber/v2"
	"github.com/sweemingdow/sdim/micros/topicsrv/internal/handlers/hhttp"
)

func ConfigConvRouter(fa *fiber.App, handler *hhttp.ConvHttpHandler) {
	convGrp := fa.Group("/conv")
	convGrp.
		Get("/recently_list", handler.HandleRecentlyConvList).
		Get("/sync/hot_list", handler.HandleSyncHotConvList).
		Post("/clear_unread_count", handler.HandleClearUnreadCount)

}
