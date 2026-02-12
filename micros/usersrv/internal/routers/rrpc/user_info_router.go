package rrpc

import (
	"github.com/lesismal/arpc"
	"github.com/sweemingdow/gmicro_pkg/pkg/middleware/arpcmw"
	"github.com/sweemingdow/sdim/micros/usersrv/internal/handlers/hrpc"
)

func ConfigureUserInfoRouter(srv *arpc.Server, handler *hrpc.UserInfoHandler) {
	srv.Handler.Handle("/user_state", arpcmw.BindAndWrite(handler.HandleUserState))
	srv.Handler.Handle("/users_unit_info", arpcmw.BindAndWrite(handler.HandleUsersUnitInfo))
	srv.Handler.Handle("/user_unit_info", arpcmw.BindAndWrite(handler.HandleUserUnitInfo))
}
