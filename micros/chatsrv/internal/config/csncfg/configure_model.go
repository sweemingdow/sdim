package csncfg

import (
	"github.com/sweemingdow/sdim/external/econfig"
)

type StaticConfig struct {
	SqlCfg econfig.SqlConfig `yaml:"sql-config"`

	RedisCfg econfig.RedisCfg `yaml:"redis-config"`
}
