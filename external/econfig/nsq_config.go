package econfig

import (
	"github.com/sweemingdow/gmicro_pkg/pkg/component/cnsq"
	"github.com/sweemingdow/gmicro_pkg/pkg/utils/usli"
	"time"
)

type NsqConfig struct {
	Producer Producer `yaml:"producer"`
	Consumer Consumer `yaml:"consumer"`
}

type Producer struct {
	NsqdAddr string `yaml:"nsqd-addr"`
}

type ConsumeItems struct {
	Topic                 string        `yaml:"topic"`
	Channel               string        `yaml:"channel"`
	Concurrency           int           `yaml:"concurrency"`
	MaxAttempts           int           `yaml:"max-attempts"`
	MsgTimeout            time.Duration `yaml:"msg-timeout"`
	RequeueDelayWhenRetry time.Duration `yaml:"requeue-delay-when-retry"`
}

type Consumer struct {
	NsqdAddresses     []string       `yaml:"nsqd-addresses"`
	LookupdAddresses  []string       `yaml:"lookupd-addresses"`
	HeartbeatInterval time.Duration  `yaml:"heartbeat-interval"`
	ConsumeItems      []ConsumeItems `yaml:"consume-items"`
}

func NsqCfgConvert(cfg NsqConfig) (cnsq.NsqPdConfig, cnsq.NsqCsConfig) {
	pdCfg := cnsq.NsqPdConfig{
		NsqdAddr: cfg.Producer.NsqdAddr,
	}

	cs := cfg.Consumer
	csCfg := cnsq.NsqCsConfig{
		NsqdDirectAddr:    cs.NsqdAddresses,
		NsqLookupdAddr:    cs.LookupdAddresses,
		HeartbeatInterval: cs.HeartbeatInterval,
		Items: usli.Conv(
			cs.ConsumeItems,
			func(ci ConsumeItems) cnsq.ConsumerItem {
				return cnsq.ConsumerItem{
					Topic:                 ci.Topic,
					Channel:               ci.Channel,
					Concurrency:           ci.Concurrency,
					MaxAttempts:           uint16(ci.MaxAttempts),
					MsgTimeout:            ci.MsgTimeout,
					RequeueDelayWhenRetry: ci.RequeueDelayWhenRetry,
				}
			},
		),
	}

	return pdCfg, csCfg
}
