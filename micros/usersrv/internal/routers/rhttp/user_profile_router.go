package rhttp

import (
	"github.com/gofiber/fiber/v2"
	"github.com/sweemingdow/sdim/micros/usersrv/internal/handlers/hhttp"
)

func ConfigureUserProfileRouter(fa *fiber.App, userProfileHandler *hhttp.UserProfileHandler) {
	userGrp := fa.Group("/user")
	userGrp.Get("/profile", userProfileHandler.Profile)
}
