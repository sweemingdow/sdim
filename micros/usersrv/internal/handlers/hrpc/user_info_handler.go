package hrpc

import (
	"github.com/lesismal/arpc"
	"github.com/sweemingdow/gmicro_pkg/pkg/mylog"
	"github.com/sweemingdow/gmicro_pkg/pkg/server/srpc"
	"github.com/sweemingdow/gmicro_pkg/pkg/server/srpc/rpccall"
	"github.com/sweemingdow/gmicro_pkg/pkg/utils/umap"
	"github.com/sweemingdow/sdim/external/emodel/usermodel"
	"github.com/sweemingdow/sdim/external/erpc/rpcuser"
	"github.com/sweemingdow/sdim/micros/usersrv/internal/repostories/inforepo"
	"github.com/sweemingdow/sdim/micros/usersrv/internal/services/infosrv"
)

const (
	UserInfoLogger = "userInfoLogger"
)

type UserInfoHandler struct {
	uis infosrv.UserInfoService
}

func NewUserInfoHandler(uis infosrv.UserInfoService) *UserInfoHandler {
	mylog.AddModuleLogger(UserInfoLogger)

	return &UserInfoHandler{
		uis: uis,
	}
}

func (uih *UserInfoHandler) HandleUserState(c *arpc.Context) {
	var req rpccall.RpcReqWrapper[rpcuser.UserStateReq]
	if ok := srpc.BindAndWriteLoggedIfError(c, &req); !ok {
		return
	}

	lg := rpccall.LoggerWrapWithReq(req, mylog.GetLogger(UserInfoLogger))
	lg.Debug().Any("req", req).Msg("handle user state start")

	state, err := uih.uis.UserState(req.Req.Uid)

	if err != nil {
		if err == infosrv.UserNotExistsErr {
			srpc.WriteLoggedIfError(c, rpccall.Ok(usermodel.NotExists))
			return
		}

		lg.Error().Stack().Err(err).Msg("handle user state failed")

		var resp = rpccall.SimpleUnpredictableErr(err)
		srpc.WriteLoggedIfError(c, resp)

		return
	}

	var resp = rpccall.Ok(state)

	if ok := srpc.WriteLoggedIfError(c, resp); ok {
		lg.Trace().Msg("handle user state completed")
	}
}

func (uih *UserInfoHandler) HandleUsersUnitInfo(c *arpc.Context) {
	var req rpccall.RpcReqWrapper[rpcuser.UsersUnitInfoReq]
	if ok := srpc.BindAndWriteLoggedIfError(c, &req); !ok {
		return
	}

	lg := rpccall.LoggerWrapWithReq(req, mylog.GetLogger(UserInfoLogger))
	lg.Debug().Any("req", req).Msg("handle users unit info start")

	uid2item, err := uih.uis.UsersUnitInfo(req.Req.Uids)

	if err != nil {
		lg.Error().Stack().Err(err).Msg("handle users unit info failed")

		var resp = rpccall.SimpleUnpredictableErr(err)
		srpc.WriteLoggedIfError(c, resp)

		return
	}

	var resp = rpccall.Ok(umap.Map(
		uid2item,
		func(_ string, val *inforepo.UserUnitInfo) *rpcuser.UnitInfoRespItem {
			return &rpcuser.UnitInfoRespItem{
				Uid:      val.Uid,
				Nickname: val.Nickname,
				Avatar:   val.Avatar,
			}
		},
	))

	if ok := srpc.WriteLoggedIfError(c, resp); ok {
		lg.Trace().Msg("handle users unit info completed")
	}
}

func (uih *UserInfoHandler) HandleUserUnitInfo(c *arpc.Context) {
	var req rpccall.RpcReqWrapper[rpcuser.UserUnitInfoReq]
	if ok := srpc.BindAndWriteLoggedIfError(c, &req); !ok {
		return
	}

	lg := rpccall.LoggerWrapWithReq(req, mylog.GetLogger(UserInfoLogger))
	lg.Debug().Any("req", req).Msg("handle user unit info start")

	info, err := uih.uis.UserUnitInfo(req.Req.Uid)

	if err != nil {
		if err == infosrv.UserNotExistsErr {
			srpc.WriteLoggedIfError(c, rpccall.SimpleErrDesc(err.Error()))
			return
		}

		lg.Error().Stack().Err(err).Msg("handle user unit info failed")

		var resp = rpccall.SimpleUnpredictableErr(err)
		srpc.WriteLoggedIfError(c, resp)

		return
	}

	item := rpcuser.UnitInfoRespItem{
		Uid:      info.Uid,
		Nickname: info.Nickname,
		Avatar:   info.Avatar,
	}

	var resp = rpccall.Ok(item)

	if ok := srpc.WriteLoggedIfError(c, resp); ok {
		lg.Trace().Msg("handle user unit info completed")
	}
}
