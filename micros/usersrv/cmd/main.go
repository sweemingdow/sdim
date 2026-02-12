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
	usncfg2 "github.com/sweemingdow/sdim/micros/usersrv/internal/config/usncfg"
	"github.com/sweemingdow/sdim/micros/usersrv/internal/handlers/hhttp"
	"github.com/sweemingdow/sdim/micros/usersrv/internal/handlers/hrpc"
	"github.com/sweemingdow/sdim/micros/usersrv/internal/repostories/inforepo"
	"github.com/sweemingdow/sdim/micros/usersrv/internal/routers"
	"github.com/sweemingdow/sdim/micros/usersrv/internal/services/infosrv"
	"github.com/sweemingdow/sdim/pkg/wrapper"
)

func main() {
	booter := boot.NewBooter()

	booter.AddConfigStageOption(boot.WithLogger(0, 5, nil))

	booter.AddComponentStageOption(boot.WithNacosClient())

	booter.AddComponentStageOption(boot.WithNacosConfig(usncfg2.NewUserSrvConfigurationReceiver()))

	booter.AddComponentStageOption(boot.WithNacosRegistry())

	booter.AddServerOption(boot.WithRpcServer())

	booter.AddServerOption(boot.WithHttpServer(func(c *fiber.Ctx, err error) error {
		lg := mylog.GetDecoLogger()
		lg.Error().Stack().Err(err).Msgf("fiber handle faield")

		return c.JSON(wrapper.GeneralErr(err))
	}))

	booter.StartAndServe(func(ac *boot.AppContext) (routebinder.AppRouterBinder, error) {
		staticCfgVal, ok := ac.GetConfigureReceiver().RecentlyConfigure(dnacos.StaticConfigName)
		if !ok {
			return nil, fmt.Errorf("%s content not found", dnacos.StaticConfigName)
		}

		staticCfg := staticCfgVal.(usncfg2.StaticConfig)
		sc, err := csql.NewSqlClient(econfig.SqlConfigConvert(staticCfg.SqlCfg))
		if err != nil {
			return nil, err
		}

		ac.CollectLifecycle(csql.SqlLifetimeTag, sc)

		rc := credis.NewRedisClient(econfig.RedisConfigConvert(staticCfg.RedisCfg))
		ac.CollectLifecycle(credis.RedisLifetimeTag, rc)

		pdCfg, _ := econfig.NsqCfgConvert(staticCfg.NsqCfg)

		nsqPd, err := cnsq.NewNsqProducer(pdCfg)
		if err != nil {
			return nil, err
		}

		ac.CollectLifecycle(cnsq.ProducerLifetimeTag, nsqPd)

		uir := inforepo.NewUserInfoRepository(sc, rc)

		return routers.NewUserServerRouteBinder(
			hrpc.NewUserInfoHandler(infosrv.NewUserInfoService(uir)),
			hhttp.NewUserProfileHandler(uir),
		), nil
	})
}
