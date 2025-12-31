package esncfg

import (
	"github.com/sweemingdow/sdim/external/econfig"
	"github.com/sweemingdow/sdim/pkg/constt"
	"time"
)

type StaticConfig struct {
	DriveEngine DriveEngineConfig `yaml:"drive-engine-config"`
	ConnManager ConnManagerConfig `yaml:"conn-manager-config"`
	NsqCfg      econfig.NsqConfig `yaml:"nsq-config"`
}

type DriveEngineConfig struct {
	TcpPort   int  `yaml:"tcp-port"`
	Multicore bool `yaml:"multicore"`
	BatchRead int  `yaml:"batch-read"`
}

type ConnManagerConfig struct {
	// 用户最大从设备数
	UserMaxSlaveConns int `yaml:"user-max-slave-conns"`
	// 主设备的客户端类型
	MasterClientTypes []constt.ClientType `yaml:"master-client-types"`

	ConcurrencyStrip int `yaml:"concurrency-strip"`

	EstimatedCapacity int `yaml:"estimated-capacity"`

	AuthTimeout time.Duration `yaml:"auth-timeout"`

	PingInterval time.Duration `yaml:"ping-interval"`

	// ping丢失次数
	PingLostTimes int `yaml:"ping-lost-times"`
}
