package routers

import (
	"github.com/gofiber/fiber/v2"
	"github.com/lesismal/arpc"
	"github.com/sweemingdow/gmicro_pkg/pkg/routebinder"
	"github.com/sweemingdow/sdim/micros/usersrv/internal/handlers/hrpc"
	"github.com/sweemingdow/sdim/micros/usersrv/internal/routers/rrpc"
)

type userServerRouteBinder struct {
	userInfoHandler *hrpc.UserInfoHandler
}

func (tsr *userServerRouteBinder) BindFiber(_ *fiber.App) {

}

func (tsr *userServerRouteBinder) BindArpc(srv *arpc.Server) {
	rrpc.ConfigureUserInfoRouter(srv, tsr.userInfoHandler)
}

func NewUserServerRouteBinder(userInfoHandler *hrpc.UserInfoHandler) routebinder.AppRouterBinder {
	return &userServerRouteBinder{
		userInfoHandler: userInfoHandler,
	}
}
