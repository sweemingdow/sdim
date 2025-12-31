package econfig

import (
	"github.com/sweemingdow/gmicro_pkg/pkg/component/csql"
	"time"
)

type SqlConfig struct {
	Schema      string        `yaml:"schema"`
	Host        string        `yaml:"host"`
	Port        int           `yaml:"port"`
	Database    string        `yaml:"database"`
	Username    string        `yaml:"username"`
	Password    string        `yaml:"password"`
	MaximumIdle int           `yaml:"maximum-idle"`
	Maximum     int           `yaml:"maximum"`
	MaxLifeTime time.Duration `yaml:"max-life-time"`
	MaxIdleTime time.Duration `yaml:"max-idle-time"`
	PingTimeout time.Duration `yaml:"ping-timeout"`
}

func SqlConfigConvert(cfg SqlConfig) csql.SqlCfg {
	return csql.SqlCfg{
		Schema:      cfg.Schema,
		Host:        cfg.Host,
		Port:        cfg.Port,
		Database:    cfg.Database,
		Username:    cfg.Username,
		Password:    cfg.Password,
		MaximumIdle: cfg.MaximumIdle,
		Maximum:     cfg.Maximum,
		MaxLifeTime: cfg.MaxLifeTime,
		MaxIdleTime: cfg.MaxIdleTime,
		PingTimeout: cfg.PingTimeout,
	}
}
