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
	"github.com/sweemingdow/sdim/external/erpc/rpcuser"
	"github.com/sweemingdow/sdim/micros/msgsrv/internal/config/msncfg"
	"github.com/sweemingdow/sdim/micros/msgsrv/internal/handlers/hmq/msgreceive"
	"github.com/sweemingdow/sdim/micros/msgsrv/internal/handlers/hrpc"
	"github.com/sweemingdow/sdim/micros/msgsrv/internal/repostories/msgrepo"
	"github.com/sweemingdow/sdim/micros/msgsrv/internal/routers"
)

func main() {
	booter := boot.NewBooter()

	booter.AddConfigStageOption(boot.WithLogger(nil))

	booter.AddComponentStageOption(boot.WithNacosClient())

	booter.AddComponentStageOption(boot.WithNacosConfig(msncfg.NewMsgSrvConfigurationReceiver()))

	booter.AddComponentStageOption(boot.WithNacosRegistry())

	booter.AddComponentStageOption(boot.WithRpcClientFactory(nil))

	// http服务
	booter.AddServerOption(boot.WithHttpServer(func(c *fiber.Ctx, err error) error {
		lg := mylog.AppLogger()
		lg.Error().Stack().Err(err).Msgf("fiber handle faield")
		return c.SendString(err.Error())
	}))

	// rpc服务
	booter.AddServerOption(boot.WithRpcServer())

	booter.StartAndServe(func(ac *boot.AppContext) (routebinder.AppRouterBinder, error) {
		staticCfgVal, ok := ac.GetConfigureReceiver().RecentlyConfigure(dnacos.StaticConfigName)
		if !ok {
			return nil, fmt.Errorf("%s content not found", dnacos.StaticConfigName)
		}

		staticCfg := staticCfgVal.(msncfg.StaticConfig)
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
			ac.GetEc() <- err
			return nil, nil
		}
		ac.CollectLifecycle(cnsq.ProducerLifetimeTag, nsqPd)

		mr := msgrepo.NewMsgRepository(sc)
		userProvider := rpcuser.NewUserInfoRpcProvider(ac.GetArpcClientFactory())

		nsqFactory := cnsq.NewStaticNsqMsgConsumeFactory()
		nsqFactory.Register(nsqconst.MsgReceiveTopic, msgreceive.NewMsgReceiveHandler(mr, userProvider, nsqPd))
		nsqCs, err := cnsq.NewNsqConsumer(csCfg, nsqFactory)

		ac.CollectLifecycle(cnsq.ConsumerLifetimeTag, nsqCs)

		if err != nil {
			ac.GetEc() <- err
			return nil, nil
		}

		msgHandler := hrpc.NewMsgHandler(mr)

		return routers.NewMsgServerRouterBinder(msgHandler), nil
	})
}
