package chatmodel

type SenderType uint8

const SysAutoSend = "sys:auto:send"

const (
	UserSenderCompatible SenderType = 0
	UserSender           SenderType = 1
	SysCmdSender         SenderType = 10
)

func IsUserSend(st SenderType) bool {
	// 保持兼容
	return st == UserSenderCompatible || st == UserSender
}
