package hhttp

import (
	"context"
	"errors"
	"github.com/gofiber/fiber/v2"
	"github.com/sweemingdow/gmicro_pkg/pkg/mylog"
	"github.com/sweemingdow/gmicro_pkg/pkg/server/srpc/rpccall"
	"github.com/sweemingdow/gmicro_pkg/pkg/utils"
	"github.com/sweemingdow/gmicro_pkg/pkg/utils/umap"
	"github.com/sweemingdow/gmicro_pkg/pkg/utils/usli"
	"github.com/sweemingdow/sdim/external/emodel/usermodel"
	"github.com/sweemingdow/sdim/external/erpc/rpcuser"
	"github.com/sweemingdow/sdim/micros/topicsrv/internal/core"
	"github.com/sweemingdow/sdim/micros/topicsrv/internal/repostories/grouprepo"
	"time"
)

type GroupHttpHandler struct {
	cm           core.ConvManager
	gr           grouprepo.GroupRepository
	userProvider rpcuser.UserInfoRpcProvider
}

func NewGroupHttpHandler(
	cm core.ConvManager,
	gr grouprepo.GroupRepository,
	userProvider rpcuser.UserInfoRpcProvider,
) *GroupHttpHandler {
	return &GroupHttpHandler{
		cm:           cm,
		gr:           gr,
		userProvider: userProvider,
	}
}

type StartGroupChatReq struct {
	GroupName  string   `json:"groupName"`
	Avatar     string   `json:"avatar"`
	OwnerUid   string   `json:"ownerUid"`
	LimitedNum int      `json:"limitedNum"`
	Members    []string `json:"members"`
}

// 发起群聊(创建并加入群聊)
func (ghh *GroupHttpHandler) HandleStartGroupChat(c *fiber.Ctx) error {
	var req StartGroupChatReq
	err := c.BodyParser(&req)
	if err != nil {
		return err
	}

	if len(req.Members) == 0 {
		return errors.New("can not create group without any members")
	}

	newMebs := usli.Distinct(req.Members)
	if len(newMebs) == 1 {
		if newMebs[0] == req.OwnerUid {
			return errors.New("can not create group with self")
		}
	}

	newMebs = append(newMebs, req.OwnerUid)

	if req.LimitedNum > 0 {
		if len(newMebs) > req.LimitedNum {
			return errors.New("group members over limited num")
		}
	}

	lg := mylog.AppLogger()

	lg.Debug().Msgf("handle start group chat, req=%+v", req)

	// rpc查用户信息
	rpcResp, err := ghh.userProvider.UsersUnitInfo(rpccall.CreateReq(rpcuser.UsersUnitInfoReq{Uids: newMebs}))
	if err != nil {
		return err
	}

	uid2info, err := rpcResp.OkOrErr()
	if err != nil {
		return err
	}

	mebInfo := umap.ToSliWithMap(
		uid2info,
		func(_ string, val *rpcuser.UnitInfoRespItem) usermodel.UserUnitInfo {
			return usermodel.UserUnitInfo{
				Uid:      val.Uid,
				Nickname: val.Nickname,
				Avatar:   val.Avatar,
			}
		},
	)

	ctx, cancel := context.WithTimeout(context.Background(), 3000*time.Millisecond)
	defer cancel()

	grpNo := utils.RandStr(32)

	convId, err := ghh.gr.CreateGroupChat(
		ctx,
		grouprepo.CreateGroupChatParam{
			GroupNo:     grpNo,
			GroupName:   req.GroupName,
			Avatar:      req.Avatar,
			OwnerUid:    req.OwnerUid,
			LimitedNum:  req.LimitedNum,
			MembersInfo: mebInfo,
		})

	if err != nil {
		return err
	}

	// 同步会话到内存
	ghh.cm.UpsertGroupChatConv(convId, grpNo, req.Avatar, req.Avatar, req.Members)

	// todo

	return nil
}
