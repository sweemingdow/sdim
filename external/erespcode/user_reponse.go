package erespcode

const (
	UserNotExists  = "100000"
	UserFrozen     = "100001"
	UserUnregister = "100002"
)

var userRespCode2text = map[string]string{
	UserNotExists:  "user not exits",
	UserFrozen:     "user frozen",
	UserUnregister: "user unregister",
}
