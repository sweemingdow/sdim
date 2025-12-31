package usncfg

import (
	"github.com/sweemingdow/gmicro_pkg/pkg/decorate/dnacos"
	"github.com/sweemingdow/gmicro_pkg/pkg/parser/yaml"
)

type userSrvConfigurationReceiver struct {
	cs *dnacos.ConfigureStorage
}

func NewUserSrvConfigurationReceiver() dnacos.ConfigurationReceiver {
	return &userSrvConfigurationReceiver{
		cs: dnacos.NewConfigureStorage(),
	}
}

func (scr *userSrvConfigurationReceiver) OnReceiveStatic(dataId, groupName, data string) {
	lg := dnacos.LogWhenReceived(dataId, groupName, data, true, false)

	if dnacos.IsDefaultStaticConfig(dataId) {
		var cfg StaticConfig
		if err := yaml.Parse([]byte(data), &cfg); err != nil {
			lg.Error().Stack().Err(err).Msg("parse static config failed")
			return
		}

		scr.cs.Store(dataId, cfg)
	}

}

func (scr *userSrvConfigurationReceiver) OnReceiveDynamic(dataId, groupName, data string, firstLoad bool) {
	_ = dnacos.LogWhenReceived(dataId, groupName, data, false, firstLoad)

}

func (scr *userSrvConfigurationReceiver) RecentlyConfigure(dataId string) (any, bool) {
	return scr.cs.Get(dataId)
}
