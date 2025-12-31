package main

import (
	"github.com/sweemingdow/gmicro_pkg/pkg/boot"
	"github.com/sweemingdow/gmicro_pkg/pkg/decorate/dnacos"
	"github.com/sweemingdow/sdim/micros/chatsrv/internal/config/csncfg"
)

func main() {
	booter := boot.NewBooter()

	booter.AddConfigStageOption(boot.WithLogger(nil))

	booter.AddComponentStageOption(boot.WithNacosClient())

	booter.AddComponentStageOption(boot.WithNacosConfig(csncfg.NewChatSrvConfigurationReceiver()))

	booter.AddComponentStageOption(boot.WithNacosRegistry())

	booter.AddComponentStageOption(boot.WithRpcClientFactory(nil))

	booter.AddComponentStageOption(boot.WithConfigureLoaded(
		func(ac *boot.AppContext) error {
			receiver := ac.GetConfigureReceiver()

			staticCfgVal, ok := receiver.RecentlyConfigure(dnacos.StaticConfigName)
			if ok {
				_ = staticCfgVal.(csncfg.StaticConfig)
			}
			return nil
		}))

	booter.StartAndServe(nil)
}
