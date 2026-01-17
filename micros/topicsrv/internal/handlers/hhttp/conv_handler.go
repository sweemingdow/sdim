package hhttp

import (
	"fmt"
	"github.com/gofiber/fiber/v2"
	"github.com/sweemingdow/gmicro_pkg/pkg/parser/json"
	"github.com/sweemingdow/sdim/micros/topicsrv/internal/core"
	"github.com/sweemingdow/sdim/pkg/wrapper"
)

type ConvHttpHandler struct {
	cm core.ConvManager
}

func NewConvHttpHandler(cm core.ConvManager) *ConvHttpHandler {
	return &ConvHttpHandler{
		cm: cm,
	}
}

// 用户最近的会话列表
func (chh *ConvHttpHandler) HandleRecentlyConvList(c *fiber.Ctx) error {
	return chh.convList(c, chh.cm.RecentlyConvList)
}

// 用户最近的会话列表
func (chh *ConvHttpHandler) HandleSyncHotConvList(c *fiber.Ctx) error {
	return chh.convList(c, chh.cm.SyncHotConvList)
}

func (chh *ConvHttpHandler) convList(c *fiber.Ctx, listFunc func(uid string) []*core.ConvListItem) error {
	uid := c.Query("uid")
	if uid == "" {
		return fmt.Errorf("uid is required")
	}

	items := listFunc(uid)

	resp := wrapper.RespOk(items)

	contents, err := json.Fmt(resp)
	if err != nil {
		return err
	}

	return c.Send(contents)
}

// 清除会话未读数
func (chh *ConvHttpHandler) HandleClearUnreadCount(c *fiber.Ctx) error {
	convId := c.Query("conv_id")
	if convId == "" {
		return fmt.Errorf("convId is required")
	}

	uid := c.Query("uid")

	if uid == "" {
		return fmt.Errorf("uid is required")
	}

	err := chh.cm.ClearUnread(convId, uid)
	if err != nil {
		return err
	}

	bodies, _ := json.Fmt(wrapper.JustOk())
	return c.Send(bodies)
}
