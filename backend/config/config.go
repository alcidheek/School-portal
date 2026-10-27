package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	MinIOEndpoint        string
	MinIOBucket          string
	MinIOUseSSL          bool
	MinIOAccessKey       string
	MinIOSecretKey       string
	SMTPHost             string
	SMTPPort             string
	SMTPEmail            string
	SMTPAppPassword      string
	DatabaseURL          string
	JWTSecret            string
	InitialAdminFullName string
	InitialAdminEmail    string
	InitialAdminPhone    string
	InitialAdminPassword string
}

func Load() (*Config, error) {
	if err := godotenv.Load(); err != nil {
		return nil, fmt.Errorf("load .env: %w", err)

	}

	databaseURL := os.Getenv("DATABASE_URL")
	initialAdminFullName := os.Getenv("INITIAL_ADMIN_FULL_NAME")
	initialAdminEmail := os.Getenv("INITIAL_ADMIN_EMAIL")
	initialAdminPhone := os.Getenv("INITIAL_ADMIN_PHONE")
	initialAdminPassword := os.Getenv("INITIAL_ADMIN_PASSWORD")

	if databaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is not set")
	}

	jwtSecret := os.Getenv("JWT_SECRET")

	if jwtSecret == "" {
		return nil, fmt.Errorf("JWT_SECRET is not set")
	}
	if len(jwtSecret) < 32 {
		return nil, fmt.Errorf("JWT_SECRET must contain at least 32 bytes; use a cryptographically random secret")
	}

	minioValues := make(map[string]string)
	for _, name := range []string{"MINIO_ENDPOINT", "MINIO_BUCKET", "MINIO_ACCESS_KEY", "MINIO_SECRET_KEY", "MINIO_USE_SSL"} {
		value := os.Getenv(name)
		if strings.TrimSpace(value) == "" {
			return nil, fmt.Errorf("%s is not set", name)
		}
		minioValues[name] = value
	}
	useSSL, err := strconv.ParseBool(strings.TrimSpace(minioValues["MINIO_USE_SSL"]))
	if err != nil {
		return nil, fmt.Errorf("MINIO_USE_SSL must be a boolean")
	}

	return &Config{
		MinIOEndpoint:        strings.TrimSpace(minioValues["MINIO_ENDPOINT"]),
		MinIOBucket:          strings.TrimSpace(minioValues["MINIO_BUCKET"]),
		MinIOUseSSL:          useSSL,
		MinIOAccessKey:       minioValues["MINIO_ACCESS_KEY"],
		MinIOSecretKey:       minioValues["MINIO_SECRET_KEY"],
		SMTPHost:             os.Getenv("SMTP_HOST"),
		SMTPPort:             os.Getenv("SMTP_PORT"),
		SMTPEmail:            os.Getenv("SMTP_EMAIL"),
		SMTPAppPassword:      os.Getenv("SMTP_APP_PASSWORD"),
		DatabaseURL:          databaseURL,
		JWTSecret:            jwtSecret,
		InitialAdminFullName: initialAdminFullName,
		InitialAdminEmail:    initialAdminEmail,
		InitialAdminPhone:    initialAdminPhone,
		InitialAdminPassword: initialAdminPassword,
	}, nil
}
