package hhttp

import (
	"errors"
	"fmt"
	"github.com/gofiber/fiber/v2"
	"github.com/sweemingdow/gmicro_pkg/pkg/parser/json"
	"github.com/sweemingdow/sdim/micros/usersrv/internal/repostories/inforepo"
	"github.com/sweemingdow/sdim/pkg/wrapper"
)

type UserProfile struct {
	Uid      string `json:"uid,omitempty"`
	Nickname string `json:"nickname,omitempty"`
	Avatar   string `json:"avatar,omitempty"`
}

type UserProfileHandler struct {
	uir inforepo.UserInfoRepository
}

func NewUserProfileHandler(uir inforepo.UserInfoRepository) *UserProfileHandler {
	return &UserProfileHandler{
		uir: uir,
	}
}

func (uph *UserProfileHandler) Profile(c *fiber.Ctx) error {
	uid := c.Query("uid")
	if uid == "" {
		return fmt.Errorf("uid is required")
	}

	uid2info, err := uph.uir.FindUsersUnitInfo([]string{uid})
	if err != nil {
		return err
	}

	info, ok := uid2info[uid]
	if !ok {
		return errors.New("user not found")
	}

	var up = UserProfile{
		Uid:      uid,
		Nickname: info.Nickname,
		Avatar:   info.Avatar,
	}
	resp := wrapper.RespOk(up)

	contents, err := json.Fmt(resp)
	if err != nil {
		return err
	}

	return c.Send(contents)
}
