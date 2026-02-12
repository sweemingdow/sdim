package hrpc

import (
	"github.com/sweemingdow/gmicro_pkg/pkg/mylog"
	"github.com/sweemingdow/gmicro_pkg/pkg/server/srpc/rpccall"
	"github.com/sweemingdow/gmicro_pkg/pkg/utils/umap"
	"github.com/sweemingdow/sdim/external/emodel/usermodel"
	"github.com/sweemingdow/sdim/external/erespcode"
	"github.com/sweemingdow/sdim/external/erpc/rpcuser"
	"github.com/sweemingdow/sdim/micros/usersrv/internal/repostories/inforepo"
	"github.com/sweemingdow/sdim/micros/usersrv/internal/services/infosrv"
)

const (
	UserInfoLogger = "userInfoLogger"
)

type UserInfoHandler struct {
	uis infosrv.UserInfoService
	dl  *mylog.DecoLogger
}

func NewUserInfoHandler(uis infosrv.UserInfoService) *UserInfoHandler {
	return &UserInfoHandler{
		uis: uis,
		dl:  mylog.NewDecoLogger("userInfoHandlerLogger"),
	}
}

func (h *UserInfoHandler) HandleUserState(req rpccall.RpcReqWrapper[rpcuser.UserStateReq]) (usermodel.UserState, error) {
	lg := req.BindLogger(h.dl)
	lg.Debug().Any("req", req).Msg("handle user state start")

	state, err := h.uis.UserState(req.Req.Uid)

	if err != nil {
		if err == infosrv.UserNotExistsErr {
			return 0, erespcode.NewUserNotExistsErr()
		}

		lg.Error().Err(err).Msg("handle user state failed")

		return 0, err
	}

	if state == usermodel.Frozen {
		return state, erespcode.NewUserFrozenErr()
	} else if state == usermodel.Unregister {
		return state, erespcode.NewUserUnregistersErr()
	}

	return state, nil
}

func (h *UserInfoHandler) HandleUsersUnitInfo(req rpccall.RpcReqWrapper[rpcuser.UsersUnitInfoReq]) (map[string]*rpcuser.UnitInfoRespItem, error) {
	lg := req.BindLogger(h.dl)
	lg.Debug().Any("req", req).Msg("handle users unit info start")

	uid2item, err := h.uis.UsersUnitInfo(req.Req.Uids)

	if err != nil {
		lg.Error().Err(err).Msg("handle users unit info failed")

		return nil, rpccall.NewRpcUnpredictableErr(err)
	}

	return umap.Map(
		uid2item,
		func(_ string, val *inforepo.UserUnitInfo) *rpcuser.UnitInfoRespItem {
			return &rpcuser.UnitInfoRespItem{
				Uid:      val.Uid,
				Nickname: val.Nickname,
				Avatar:   val.Avatar,
			}
		},
	), nil
}

func (h *UserInfoHandler) HandleUserUnitInfo(req rpccall.RpcReqWrapper[rpcuser.UserUnitInfoReq]) (rpcuser.UnitInfoRespItem, error) {
	lg := req.BindLogger(h.dl)
	lg.Debug().Any("req", req).Msg("handle user unit info start")

	info, err := h.uis.UserUnitInfo(req.Req.Uid)

	if err != nil {
		var zero rpcuser.UnitInfoRespItem
		if err == infosrv.UserNotExistsErr {
			return zero, nil
		}

		lg.Error().Err(err).Msg("handle user unit info failed")

		return zero, err
	}

	item := rpcuser.UnitInfoRespItem{
		Uid:      info.Uid,
		Nickname: info.Nickname,
		Avatar:   info.Avatar,
	}

	return item, nil
}
