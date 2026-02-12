package rhttp

import (
	"github.com/gofiber/fiber/v2"
	"github.com/sweemingdow/gmicro_pkg/pkg/middleware/fibermw"
	"github.com/sweemingdow/sdim/micros/topicsrv/internal/handlers/hhttp"
)

func ConfigGroupRouter(fa *fiber.App, handler *hhttp.GroupHttpHandler) {
	convGrp := fa.Group("/group")
	convGrp.
		Post("/start_chat", fibermw.BindAndWrite(handler.HandleStartGroupChat)).            // 发起群聊
		Get("/fetch_group_data", fibermw.QueryAndWrite(handler.HandleFetchGroupData)).      // 拉取群资料
		Post("/setting_group_name", fibermw.QueryAndWrite(handler.HandleSettingGroupName)). // 设置群名称
		Post("/setting_group_bak", handler.HandleSettingGroupBak).                          // 设置群备注
		Post("/setting_group_nickname", handler.HandleSettingGroupNickname).                // 设置群内昵称
		Post("/add_members", fibermw.BindAndWrite(handler.HandleAddMembers)).               // 添加群成员
		Post("/remove_members", fibermw.BindAndWrite(handler.HandleRemoveMembers))          // 移除群成员
}
