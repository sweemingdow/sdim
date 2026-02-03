package main

import (
	"fmt"
	"github.com/gofiber/fiber/v2"
	"github.com/sweemingdow/gmicro_pkg/pkg/boot"
	"github.com/sweemingdow/gmicro_pkg/pkg/component/cnsq"
	"github.com/sweemingdow/gmicro_pkg/pkg/component/credis"
	"github.com/sweemingdow/gmicro_pkg/pkg/component/csql"
	"github.com/sweemingdow/gmicro_pkg/pkg/decorate/dnacos"
	"github.com/sweemingdow/gmicro_pkg/pkg/mylog"
	"github.com/sweemingdow/gmicro_pkg/pkg/routebinder"
	"github.com/sweemingdow/sdim/external/econfig"
	"github.com/sweemingdow/sdim/external/eglobal/nsqconst"
	"github.com/sweemingdow/sdim/external/erpc/rpcmsg"
	"github.com/sweemingdow/sdim/external/erpc/rpcuser"
	"github.com/sweemingdow/sdim/micros/topicsrv/internal/config/tsncfg"
	"github.com/sweemingdow/sdim/micros/topicsrv/internal/core/convmgr"
	"github.com/sweemingdow/sdim/micros/topicsrv/internal/core/grpmgr"
	"github.com/sweemingdow/sdim/micros/topicsrv/internal/core/nsqsend"
	"github.com/sweemingdow/sdim/micros/topicsrv/internal/handlers/hhttp"
	"github.com/sweemingdow/sdim/micros/topicsrv/internal/handlers/hmq/msgforward"
	"github.com/sweemingdow/sdim/micros/topicsrv/internal/handlers/hrpc"
	"github.com/sweemingdow/sdim/micros/topicsrv/internal/repostories/convrepo"
	"github.com/sweemingdow/sdim/micros/topicsrv/internal/repostories/grouprepo"
	"github.com/sweemingdow/sdim/micros/topicsrv/internal/routers"
	"github.com/sweemingdow/sdim/pkg/wrapper"
)

func main() {
	booter := boot.NewBooter()

	booter.AddConfigStageOption(boot.WithLogger(func() string {
		return ""
	}))

	booter.AddComponentStageOption(boot.WithNacosClient())

	booter.AddComponentStageOption(boot.WithNacosConfig(tsncfg.NewTopicSrvConfigurationReceiver()))

	booter.AddComponentStageOption(boot.WithNacosRegistry())

	booter.AddServerOption(boot.WithHttpServer(func(c *fiber.Ctx, err error) error {
		lg := mylog.GetDecoLogger()
		lg.Error().Stack().Err(err).Msgf("fiber handle faield")

		return c.JSON(wrapper.GeneralErr(err))
	}))

	// 启动rpc服务
	booter.AddServerOption(boot.WithRpcServer())

	booter.AddComponentStageOption(boot.WithRpcClientFactory(nil))

	booter.StartAndServe(func(ac *boot.AppContext) (routebinder.AppRouterBinder, error) {
		staticCfgVal, ok := ac.GetConfigureReceiver().RecentlyConfigure(dnacos.StaticConfigName)
		if !ok {
			return nil, fmt.Errorf("%s content not found", dnacos.StaticConfigName)
		}

		staticCfg := staticCfgVal.(tsncfg.StaticConfig)
		sc, err := csql.NewSqlClient(econfig.SqlConfigConvert(staticCfg.SqlCfg))
		if err != nil {
			return nil, err
		}

		ac.CollectLifecycle(csql.SqlLifetimeTag, sc)

		rc := credis.NewRedisClient(econfig.RedisConfigConvert(staticCfg.RedisCfg))
		ac.CollectLifecycle(credis.RedisLifetimeTag, rc)

		pdCfg, csCfg := econfig.NsqCfgConvert(staticCfg.NsqCfg)

		nsqPd, err := cnsq.NewNsqProducer(pdCfg)
		if err != nil {
			return nil, err
		}

		ac.CollectLifecycle(cnsq.ProducerLifetimeTag, nsqPd)

		userProvider := rpcuser.NewUserInfoRpcProvider(ac.GetArpcClientFactory())
		ms := nsqsend.NewMsgSender(nsqPd)
		groupRepo := grouprepo.NewGroupRepository(sc, rc)
		groupMgr := grpmgr.NewGroupManager(100, groupRepo)
		cm := convmgr.NewConvManager(
			100,
			128,
			100,
			convrepo.NewConvRepository(rc, sc),
			groupRepo,
			nsqPd,
			userProvider,
			rpcmsg.NewMsgProvider(ac.GetArpcClientFactory()),
			ms,
			groupMgr,
		)

		nsqFactory := cnsq.NewStaticNsqMsgConsumeFactory()
		nsqFactory.Register(nsqconst.MsgForwardTopic, msgforward.NewMsgForwardHandler(cm, nsqPd))

		nsdCs, err := cnsq.NewNsqConsumer(csCfg, nsqFactory)
		if err != nil {
			return nil, err
		}

		ac.CollectLifecycle(cnsq.ConsumerLifetimeTag, nsdCs)

		topicHandler := hrpc.NewTopicHandler(cm)

		convHttpHandler := hhttp.NewConvHttpHandler(cm)

		groupHttpHandler := hhttp.NewGroupHttpHandler(cm, groupRepo, groupMgr, ms, userProvider)
		ac.CollectLifecycle("groupHttpHandler", groupHttpHandler)

		return routers.NewTopicServerRouteBinder(
			topicHandler,
			convHttpHandler,
			groupHttpHandler,
		), nil
	})
}
