package database

import (
	"database/sql"
	"log"

	_ "github.com/lib/pq"

	"github.com/alcidheek/school-portal/config"
)

func Connect(cfg *config.Config) *sql.DB {
	db, err := sql.Open("postgres", cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("database connection failed: %v", err)
	}

	if err := db.Ping(); err != nil {
		log.Fatalf("database ping failed: %v", err)
	}

	return db
}
