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
