package erespcode

import "github.com/sweemingdow/gmicro_pkg/pkg/myerr"

func _newUserRespErr(code string) error {
	return myerr.NewRpcRespError(code, "", userRespCode2text[code], nil)
}

func NewUserNotExistsErr() error {
	return _newUserRespErr(UserNotExists)
}

func NewUserFrozenErr() error {
	return myerr.NewRpcRespError(UserFrozen, "", userRespCode2text[UserFrozen], nil)
}

func NewUserUnregistersErr() error {
	return myerr.NewRpcRespError(UserUnregister, "", userRespCode2text[UserUnregister], nil)
}
