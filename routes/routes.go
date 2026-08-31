package routes

import (
	"net/http"

	"github.com/alcidheek/school-portal/internal/handler"
	"github.com/alcidheek/school-portal/internal/middleware"
	"github.com/alcidheek/school-portal/internal/model"
)

func SetupRoutes(
	userHandler *handler.UserHandler,
	jwtSecret string,
) *http.ServeMux {

	mux := http.NewServeMux()

	// User routes
	mux.HandleFunc(
		"POST /auth/register",
		userHandler.RegisterUser,
	)

	//login routes
	mux.HandleFunc("POST /auth/login", userHandler.Login)

	// authorization routes
	mux.Handle(
		"/api/v1/me",
		middleware.AuthMiddleware(jwtSecret)(
			http.HandlerFunc(userHandler.Me),
		),
	)

	//rbac admin role
	mux.Handle(
		"/api/v1/admin/test",
		middleware.AuthMiddleware(jwtSecret)(
			middleware.RequireRole(model.RoleAdmin)(
				http.HandlerFunc(userHandler.AdminTest),
			),
		),
	)

	return mux
}
