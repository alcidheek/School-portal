package service

import (
	"context"
	"errors"
	"testing"

	"github.com/alcidheek/school-portal/internal/model"
	"github.com/alcidheek/school-portal/internal/repository"
	"github.com/google/uuid"
)

func TestAuthenticationCleanupEligibility(t *testing.T) {
	s, db, ctx := userSecurityFixture(t, true)
	expired := seedAccount(t, db, "expired@example.com", model.RoleTeacher, true)
	cooldown := seedAccount(t, db, "cooldown@example.com", model.RoleTeacher, true)
	unexpired := seedAccount(t, db, "unexpired@example.com", model.RoleTeacher, true)
	budget := seedAccount(t, db, "budget@example.com", model.RoleTeacher, true)
	for _, entry := range []struct {
		id             uuid.UUID
		window, issued string
		attempts       int
	}{
		{expired, "-1 minute", "-2 minutes", 5}, {cooldown, "-1 second", "-10 seconds", 3},
		{unexpired, "-1 minute", "-2 minutes", 4}, {budget, "15 minutes", "-2 minutes", 5},
	} {
		if _, err := db.Exec(`INSERT INTO password_reset_state(user_id,window_expires_at,last_issued_at,failed_attempts) VALUES($1,clock_timestamp()+$2::interval,clock_timestamp()+$3::interval,$4)`, entry.id, entry.window, entry.issued, entry.attempts); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`INSERT INTO password_reset_codes(user_id,code_hash,expires_at,used_at) VALUES($1,'expired-unused',clock_timestamp()-interval '1 minute',NULL),($1,'expired-used',clock_timestamp()-interval '1 minute',clock_timestamp()),($2,'unexpired-used',clock_timestamp()+interval '15 minutes',clock_timestamp()),($3,'unexpired-unused',clock_timestamp()+interval '15 minutes',NULL)`, expired, unexpired, budget); err != nil {
		t.Fatal(err)
	}
	expiredToken, activeToken := uuid.New(), uuid.New()
	if _, err := db.Exec(`INSERT INTO revoked_access_tokens(token_id,user_id,expires_at) VALUES($1,$3,clock_timestamp()-interval '1 minute'),($2,$3,clock_timestamp()+interval '1 hour')`, expiredToken, activeToken, expired); err != nil {
		t.Fatal(err)
	}
	cleanup := NewAuthenticationCleanupService(repository.NewAuthenticationCleanupRepository(db))
	counts, err := cleanup.Cleanup(ctx)
	if err != nil || counts.Skipped || counts.ResetCodes != 2 || counts.RevokedTokens != 1 || counts.ResetStates != 1 {
		t.Fatalf("cleanup counts %+v error %v", counts, err)
	}
	var codes, states, tokens, attempts int
	if err = db.QueryRow(`SELECT count(*) FROM password_reset_codes`).Scan(&codes); err != nil || codes != 2 {
		t.Fatal("unexpired codes changed", err)
	}
	if err = db.QueryRow(`SELECT count(*) FROM password_reset_state`).Scan(&states); err != nil || states != 3 {
		t.Fatal("protected states changed", err)
	}
	if err = db.QueryRow(`SELECT count(*) FROM revoked_access_tokens WHERE token_id=$1`, activeToken).Scan(&tokens); err != nil || tokens != 1 {
		t.Fatal("unexpired revocation deleted", err)
	}
	if err = db.QueryRow(`SELECT failed_attempts FROM password_reset_state WHERE user_id=$1`, budget).Scan(&attempts); err != nil || attempts != 5 {
		t.Fatal("attempt budget lost", err)
	}
	s.resetSender = &capturedResetSender{}
	if err = s.ForgotPassword(ctx, "budget@example.com"); err != nil {
		t.Fatal(err)
	}
	if err = s.ForgotPassword(ctx, "cooldown@example.com"); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(`SELECT count(*) FROM password_reset_codes`).Scan(&codes); err != nil || codes != 2 {
		t.Fatal("cleanup bypassed budget/cooldown", err)
	}
}

func TestAuthenticationCleanupLockAndConcurrentRuns(t *testing.T) {
	_, db, ctx := userSecurityFixture(t, true)
	id := seedAccount(t, db, "user@example.com", model.RoleTeacher, true)
	if _, err := db.Exec(`INSERT INTO password_reset_codes(user_id,code_hash,expires_at) VALUES($1,'expired',clock_timestamp()-interval '1 minute');`, id); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO password_reset_state(user_id,window_expires_at) VALUES($1,clock_timestamp()-interval '1 minute')`, id); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO revoked_access_tokens(token_id,user_id,expires_at) VALUES($1,$2,clock_timestamp()-interval '1 minute')`, uuid.New(), id); err != nil {
		t.Fatal(err)
	}
	cleanup := NewAuthenticationCleanupService(repository.NewAuthenticationCleanupRepository(db))
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, accountManagementLock); err != nil {
		t.Fatal(err)
	}
	// Models an in-flight reset transaction holding the exact same lifecycle lock.
	if counts, err := cleanup.Cleanup(ctx); err != nil || !counts.Skipped {
		t.Fatal("cleanup competed with active reset", err)
	}
	tx.Rollback()
	type result struct {
		counts repository.AuthenticationCleanupCounts
		err    error
	}
	results := make(chan result, 12)
	for i := 0; i < 12; i++ {
		go func() {
			other := NewAuthenticationCleanupService(repository.NewAuthenticationCleanupRepository(db))
			counts, err := other.Cleanup(ctx)
			results <- result{counts, err}
		}()
	}
	var total repository.AuthenticationCleanupCounts
	for i := 0; i < 12; i++ {
		r := <-results
		if r.err != nil {
			t.Fatal(r.err)
		}
		total.ResetCodes += r.counts.ResetCodes
		total.ResetStates += r.counts.ResetStates
		total.RevokedTokens += r.counts.RevokedTokens
	}
	if total.ResetCodes != 1 || total.ResetStates != 1 || total.RevokedTokens != 1 {
		t.Fatalf("concurrent cleanup %+v", total)
	}
}

func TestAuthenticationCleanupRollbackAndDatabaseFailure(t *testing.T) {
	_, db, ctx := userSecurityFixture(t, true)
	id := seedAccount(t, db, "user@example.com", model.RoleTeacher, true)
	if _, err := db.Exec(`INSERT INTO password_reset_codes(user_id,code_hash,expires_at) VALUES($1,'expired',clock_timestamp()-interval '1 minute')`, id); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO password_reset_state(user_id,window_expires_at) VALUES($1,clock_timestamp()-interval '1 minute')`, id); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE FUNCTION reject_cleanup() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'private test diagnostic'; END $$; CREATE TRIGGER reject_cleanup BEFORE DELETE ON password_reset_state FOR EACH ROW EXECUTE FUNCTION reject_cleanup()`); err != nil {
		t.Fatal(err)
	}
	cleanup := NewAuthenticationCleanupService(repository.NewAuthenticationCleanupRepository(db))
	if counts, err := cleanup.Cleanup(ctx); err == nil || counts.ResetCodes != 0 {
		t.Fatal("expected transactional failure")
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM password_reset_codes`).Scan(&count); err != nil || count != 1 {
		t.Fatal("cleanup was not rolled back", err)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := cleanup.Cleanup(cancelled); !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation ignored", err)
	}
	db.Close()
	if _, err := cleanup.Cleanup(ctx); err == nil {
		t.Fatal("closed database accepted")
	}
}
