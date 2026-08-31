package config

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	DatabaseURL string
	JWTSecret   string
}

func Load() (*Config, error) {
	if err := godotenv.Load(); err != nil {
		return nil, fmt.Errorf("load .env: %w", err)

	}

	databaseURL := os.Getenv("DATABASE_URL")

	if databaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is not set")
	}

	jwtSecret := os.Getenv("JWT_SECRET")

	if jwtSecret == "" {
		return nil, fmt.Errorf("JWT_SECRET is not set")
	}

	return &Config{
		DatabaseURL: databaseURL,
		JWTSecret:   jwtSecret,
	}, nil
}
