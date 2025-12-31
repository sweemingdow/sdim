package core

const (
	UserProfileLogger = "userProfileLogger"
)

type UserProfileService interface {
	UserOk(uid string) (bool, error)
}
