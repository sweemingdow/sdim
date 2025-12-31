package econfig

import (
	"github.com/sweemingdow/gmicro_pkg/pkg/component/credis"
	"time"
)

type RedisCfg struct {
	Addresses      []string      `yaml:"addresses"`
	Database       int           `yaml:"database"`
	Username       string        `yaml:"username"`
	Password       string        `yaml:"password"`
	MinimumIdle    int           `yaml:"minimum-idle"`
	MaximumIdle    int           `yaml:"maximum-idle"`
	Maximum        int           `yaml:"maximum"`
	MaxIdleTime    time.Duration `yaml:"max-idle-time"`
	ReadTimeout    time.Duration `yaml:"read-timeout"`
	WriteTimeout   time.Duration `yaml:"write-timeout"`
	MaxWaitTimeout time.Duration `yaml:"max-wait-timeout"`
	PingTimeout    time.Duration `yaml:"ping-timeout"`
}

func RedisConfigConvert(cfg RedisCfg) credis.RedisCfg {
	return credis.RedisCfg{
		Addresses:      cfg.Addresses,
		Database:       cfg.Database,
		Username:       cfg.Username,
		Password:       cfg.Password,
		MinimumIdle:    cfg.MinimumIdle,
		MaximumIdle:    cfg.MaximumIdle,
		Maximum:        cfg.Maximum,
		MaxIdleTime:    cfg.MaxIdleTime,
		ReadTimeout:    cfg.ReadTimeout,
		WriteTimeout:   cfg.WriteTimeout,
		MaxWaitTimeout: cfg.MaxWaitTimeout,
		PingTimeout:    cfg.PingTimeout,
	}
}
