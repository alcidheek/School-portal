package main

import (
	"log"
	"net/http"

	"github.com/alcidheek/school-portal/config"
	"github.com/alcidheek/school-portal/database"
	"github.com/alcidheek/school-portal/internal/handler"
	"github.com/alcidheek/school-portal/internal/repository"
	"github.com/alcidheek/school-portal/internal/service"
	"github.com/alcidheek/school-portal/routes"
)

func main() {

	// Load configuration
	log.Println("Loading config...")

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	// Connect to database
	log.Println("Connecting to database...")

	db := database.Connect(cfg)
	defer db.Close()

	log.Println("Database connected successfully")

	// Run database migrations
	log.Println("Running database migrations...")

	if err := database.RunMigrations(db); err != nil {
		log.Fatalf("run migrations: %v", err)
	}

	log.Println("Database migrations completed")

	// Repository
	userRepo := repository.NewUserRepository(db)

	// Service
	userService := service.NewUserService(userRepo, cfg.JWTSecret)

	// Handler
	userHandler := handler.NewUserHandler(userService)

	// Routes
	router := routes.SetupRoutes(userHandler, cfg.JWTSecret)

	// Start server
	log.Println("Server running on port 8080")

	if err := http.ListenAndServe(":8080", router); err != nil {
		log.Fatalf("server failed to start: %v", err)
	}
}
