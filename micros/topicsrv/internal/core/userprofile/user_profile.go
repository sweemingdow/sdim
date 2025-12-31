package userprofile

import (
	"github.com/sweemingdow/sdim/micros/topicsrv/internal/core"
)

type userProfileService struct {
}

func NewUserProfileService() core.UserProfileService {
	return nil
}

func (ups *userProfileService) UserOk(uid string) (bool, error) {
	//TODO implement me
	panic("implement me")
}
