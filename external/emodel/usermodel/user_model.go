package usermodel

type UserState uint8

const (
	NotExists  UserState = 0
	Normal     UserState = 1
	Frozen     UserState = 2
	Unregister UserState = 3
)

func (us UserState) IsOk() bool {
	return us == Normal
}

type UserUnitInfo struct {
	Uid      string `json:"uid,omitempty"`
	Nickname string `json:"nickname,omitempty"`
	Avatar   string `json:"avatar,omitempty"`
}
