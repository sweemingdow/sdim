package rpcuser

import (
	"github.com/sweemingdow/gmicro_pkg/pkg/server/srpc/rpccall"
	"github.com/sweemingdow/sdim/external/emodel/usermodel"
)

type (
	UserStateReq struct {
		Uid string `json:"uid"`
	}

	UserUnitInfoReq struct {
		Uid string `json:"uid"`
	}

	UsersUnitInfoReq struct {
		Uids []string `json:"uids"`
	}

	UnitInfoRespItem struct {
		Uid      string `json:"uid,omitempty"`
		Nickname string `json:"nickname,omitempty"`
		Avatar   string `json:"avatar,omitempty"`
	}
)

// @rpc_server user_server
//
//go:generate rpcgen -type UserInfoRpcProvider
type UserInfoRpcProvider interface {
	// @path /user_state
	// @timeout 500ms
	UserState(req rpccall.RpcReqWrapper[UserStateReq]) (rpccall.RpcRespWrapper[usermodel.UserState], error)

	// @path /users_unit_info
	// @timeout 800ms
	UsersUnitInfo(req rpccall.RpcReqWrapper[UsersUnitInfoReq]) (rpccall.RpcRespWrapper[map[string]*UnitInfoRespItem], error)

	// @path /user_unit_info
	// @timeout 500ms
	UserUnitInfo(req rpccall.RpcReqWrapper[UserUnitInfoReq]) (rpccall.RpcRespWrapper[*UnitInfoRespItem], error)
}
