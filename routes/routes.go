package routes

import (
	"net/http"

	"github.com/alcidheek/school-portal/internal/handler"
)

func SetupRoutes(
	userHandler *handler.UserHandler,
) *http.ServeMux {

	mux := http.NewServeMux()

	// User routes
	mux.HandleFunc(
		"POST /auth/register",
		userHandler.RegisterUser,
	)

	//login routes
	mux.HandleFunc("POST /auth/login", userHandler.Login)

	return mux
}
