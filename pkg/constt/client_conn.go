package constt

// 客户端类型
type ClientType uint8

const (
	Android ClientType = 1
	IOS     ClientType = 2
	Pc      ClientType = 3
	Web     ClientType = 4
)

/*
客户端级别:
同一时刻只允许一台master设备和多台slave设备同时在线
多台master设备同时登录, 会互T

one master + n salves
*/
type ClientLevel uint8

const (
	Master ClientLevel = 1 //
	Slave  ClientLevel = 2 //
)

func IsMasterLevel(cl ClientLevel) bool {
	return cl == Master
}

/*
默认一个用户同时在线的总设备数(cnt)
可以是:
(one master + n slaves) <= cnt
(n slaves) <= cnt
one master
*/
const DefaultOnlineDeviceCount = 5

// 连接状态
type ConnState uint8

const (
	// 新建
	Created ConnState = 1

	// 待删除
	BeRemove ConnState = 2
)
