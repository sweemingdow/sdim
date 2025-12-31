package main

import (
	"github.com/sweemingdow/gmicro_pkg/pkg/boot"
	"github.com/sweemingdow/gmicro_pkg/pkg/decorate/dnacos"
	"github.com/sweemingdow/sdim/micros/enginesrv/internal/config/esncfg"
	"github.com/sweemingdow/sdim/micros/enginesrv/internal/csboot"
)

func main() {
	booter := boot.NewBooter()

	booter.AddConfigStageOption(boot.WithLogger(func() string {
		return ""
	}))

	booter.AddComponentStageOption(boot.WithNacosClient())

	booter.AddComponentStageOption(boot.WithNacosConfig(esncfg.NewEngineSrvConfigurationReceiver()))

	booter.AddComponentStageOption(boot.WithNacosRegistry())

	booter.AddComponentStageOption(boot.WithRpcClientFactory(nil))

	booter.AddComponentStageOption(boot.WithConfigureLoaded(
		func(ac *boot.AppContext) error {
			receiver := ac.GetConfigureReceiver()

			staticCfgVal, ok := receiver.RecentlyConfigure(dnacos.StaticConfigName)
			if ok {
				staticCfg := staticCfgVal.(esncfg.StaticConfig)
				csboot.StartConnDriveEngine(ac, staticCfg)
			}
			return nil
		}))

	//booter.AddServerOption(boot.WithShutdownPreHooks(csboot.ShutdownConnDriveEngine))

	booter.StartAndServe(nil)
}
