package rhttp

import (
	"github.com/gofiber/fiber/v2"
	"github.com/sweemingdow/sdim/micros/topicsrv/internal/handlers/hhttp"
)

func ConfigGroupRouter(fa *fiber.App, handler *hhttp.GroupHttpHandler) {
	convGrp := fa.Group("/group")
	convGrp.
		Post("/start_chat", handler.HandleStartGroupChat).
		Get("/fetch_group_data", handler.HandleFetchGroupData).
		Post("/setting_group_name", handler.HandlerSettingGroupName)

}
