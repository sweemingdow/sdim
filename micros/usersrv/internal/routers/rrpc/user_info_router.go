package rrpc

import (
	"github.com/lesismal/arpc"
	"github.com/sweemingdow/sdim/micros/usersrv/internal/handlers/hrpc"
)

func ConfigureUserInfoRouter(srv *arpc.Server, handler *hrpc.UserInfoHandler) {
	srv.Handler.Handle("/user_state", handler.HandleUserState)
	srv.Handler.Handle("/users_unit_info", handler.HandleUsersUnitInfo)
	srv.Handler.Handle("/user_unit_info", handler.HandleUserUnitInfo)
}
