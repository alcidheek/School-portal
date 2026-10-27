package repository

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
)

func TestResetPasswordTransaction(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to an isolated PostgreSQL database")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	_, err = db.Exec(`CREATE TEMP TABLE users(id UUID PRIMARY KEY,password_hash TEXT,updated_at TIMESTAMPTZ);
 CREATE TEMP TABLE password_reset_codes(id UUID PRIMARY KEY DEFAULT gen_random_uuid(),user_id UUID,code_hash TEXT,expires_at TIMESTAMPTZ,used_at TIMESTAMPTZ,created_at TIMESTAMPTZ DEFAULT clock_timestamp());
 CREATE TEMP TABLE password_reset_state(user_id UUID PRIMARY KEY,window_expires_at TIMESTAMPTZ,failed_attempts INTEGER DEFAULT 0,last_issued_at TIMESTAMPTZ);`)
	if err != nil {
		t.Fatal(err)
	}
	id := uuid.New()
	ctx := context.Background()
	repo := NewUserRepository(db)
	if _, err = db.Exec(`INSERT INTO users VALUES($1,'old',NOW())`, id); err != nil {
		t.Fatal(err)
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	_, err = repo.ResetStateTx(ctx, tx, id, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = repo.CreateResetCodeTx(ctx, tx, id, "stored-hash"); err != nil {
		t.Fatal(err)
	}
	if hash, err := repo.ResetCodeHashTx(ctx, tx, id); err != nil || hash != "stored-hash" {
		t.Fatalf("hash retrieval: %v", err)
	}
	if err = repo.FailedResetAttemptTx(ctx, tx, id); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	// Fail after password UPDATE; service rollback preserves password and codes.
	if _, err = db.Exec(`ALTER TABLE password_reset_codes ADD CHECK(used_at IS NULL)`); err != nil {
		t.Fatal(err)
	}
	tx, err = db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err = repo.CompleteResetTx(ctx, tx, id, "new"); err == nil {
		t.Fatal("expected invalidation failure")
	}
	tx.Rollback()
	var hash string
	var attempts int
	if err = db.QueryRow(`SELECT password_hash FROM users`).Scan(&hash); err != nil || hash != "old" {
		t.Fatalf("rollback: %v", err)
	}
	if err = db.QueryRow(`SELECT failed_attempts FROM password_reset_state`).Scan(&attempts); err != nil || attempts != 1 {
		t.Fatalf("attempt persistence: %d %v", attempts, err)
	}
}
