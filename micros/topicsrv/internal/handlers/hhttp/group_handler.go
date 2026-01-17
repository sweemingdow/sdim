package hhttp

import (
	"context"
	"errors"
	"github.com/gofiber/fiber/v2"
	"github.com/sweemingdow/gmicro_pkg/pkg/mylog"
	"github.com/sweemingdow/gmicro_pkg/pkg/server/srpc/rpccall"
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
	var sgc StartGroupChatReq
	err := c.BodyParser(&sgc)
	if err != nil {
		return err
	}

	if len(sgc.Members) == 0 {
		return errors.New("can not create group without any members")
	}

	newMebs := usli.Distinct(sgc.Members)
	if len(newMebs) == 1 {
		if newMebs[0] == sgc.OwnerUid {
			return errors.New("can not create group with self")
		}
	}

	lg := mylog.AppLogger()

	lg.Debug().Msgf("handle start group chat, req=%+v", sgc)

	// rpc查用户信息
	rpcResp, err := ghh.userProvider.UsersUnitInfo(rpccall.CreateReq(rpcuser.UsersUnitInfoReq{Uids: append([]string{sgc.OwnerUid}, newMebs...)}))
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

	_, err = ghh.gr.CreateGroupChat(
		ctx,
		grouprepo.CreateGroupChatParam{
			GroupName:   sgc.GroupName,
			Avatar:      sgc.Avatar,
			OwnerUid:    sgc.OwnerUid,
			LimitedNum:  sgc.LimitedNum,
			MembersInfo: mebInfo,
		})

	if err != nil {
		return err
	}

	// 同步会话到内存
	return nil
}
