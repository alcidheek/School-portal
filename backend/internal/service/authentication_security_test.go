package service

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alcidheek/school-portal/internal/model"
	"github.com/alcidheek/school-portal/internal/repository"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

func loginSecurityClaims(t *testing.T, s *UserService, ctx context.Context, email, password string) (uuid.UUID, uuid.UUID, int64, time.Time) {
	t.Helper()
	text, user, err := s.Login(ctx, LoginRequest{Email: email, Password: password})
	if err != nil {
		t.Fatal(err)
	}
	token, err := jwt.Parse(text, func(*jwt.Token) (any, error) { return []byte(s.jwtSecret), nil })
	if err != nil {
		t.Fatal(err)
	}
	claims := token.Claims.(jwt.MapClaims)
	expires, _ := claims.GetExpirationTime()
	return user.ID, uuid.MustParse(claims["jti"].(string)), int64(claims["token_version"].(float64)), expires.Time
}

func TestPersistentTokenRevocation(t *testing.T) {
	s, db, ctx := userSecurityFixture(t, true)
	id := seedAccount(t, db, "admin@example.com", model.RoleAdmin, true)
	_, one, version, expiry := loginSecurityClaims(t, s, ctx, "admin@example.com", accountPassword)
	_, two, _, _ := loginSecurityClaims(t, s, ctx, "admin@example.com", accountPassword)
	if err := s.AuthorizeToken(ctx, id, model.RoleAdmin, one, version); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := s.Logout(ctx, id, one, expiry); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.AuthorizeToken(ctx, id, model.RoleAdmin, one, version); !errors.Is(err, ErrAuthentication) {
		t.Fatal("logout failed", err)
	}
	if err := s.AuthorizeToken(ctx, id, model.RoleAdmin, two, version); err != nil {
		t.Fatal("other session revoked", err)
	}
	// Dispose pooled connections and reconstruct services: no in-memory revocation.
	db.SetMaxIdleConns(0)
	restarted := NewUserService(repository.NewUserRepository(db), s.jwtSecret)
	if err := restarted.AuthorizeToken(ctx, id, model.RoleAdmin, one, version); !errors.Is(err, ErrAuthentication) {
		t.Fatal("revocation not persistent", err)
	}
	if err := s.ChangePassword(ctx, id, accountPassword, "NewStrongPassword2!", "NewStrongPassword2!"); err != nil {
		t.Fatal(err)
	}
	if err := s.AuthorizeToken(ctx, id, model.RoleAdmin, two, version); !errors.Is(err, ErrAuthentication) {
		t.Fatal("password change did not revoke", err)
	}
	_, fresh, freshVersion, _ := loginSecurityClaims(t, s, ctx, "admin@example.com", "NewStrongPassword2!")
	if err := s.AuthorizeToken(ctx, id, model.RoleAdmin, fresh, freshVersion); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE users SET is_active=FALSE WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	if err := s.AuthorizeToken(ctx, id, model.RoleAdmin, fresh, freshVersion); !errors.Is(err, ErrInactiveAccount) {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE users SET is_active=TRUE WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	if err := s.AuthorizeToken(ctx, id, model.RoleAdmin, fresh, freshVersion); !errors.Is(err, ErrAuthentication) {
		t.Fatal("reactivation revived token", err)
	}
	_, afterReactivate, newVersion, _ := loginSecurityClaims(t, s, ctx, "admin@example.com", "NewStrongPassword2!")
	if _, err := db.Exec(`UPDATE users SET role='TEACHER' WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	if err := s.AuthorizeToken(ctx, id, model.RoleAdmin, afterReactivate, newVersion); !errors.Is(err, ErrAuthentication) {
		t.Fatal("stale role accepted", err)
	}
}

func TestConcurrentLogoutAndValidation(t *testing.T) {
	s, db, ctx := userSecurityFixture(t, true)
	seedAccount(t, db, "teacher@example.com", model.RoleTeacher, true)
	id, token, version, expiry := loginSecurityClaims(t, s, ctx, "teacher@example.com", accountPassword)
	var wg sync.WaitGroup
	errs := make(chan error, 24)
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); errs <- s.Logout(ctx, id, token, expiry) }()
	}
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := s.AuthorizeToken(ctx, id, model.RoleTeacher, token, version)
			if errors.Is(err, ErrAuthentication) {
				err = nil
			}
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM revoked_access_tokens WHERE token_id=$1`, token).Scan(&count); err != nil || count != 1 {
		t.Fatalf("revocations %d %v", count, err)
	}
	if err := s.AuthorizeToken(ctx, id, model.RoleTeacher, token, version); !errors.Is(err, ErrAuthentication) {
		t.Fatal("accepted after committed logout", err)
	}
}

func resetFixture(t *testing.T, role model.Role) (*UserService, *sql.DB, context.Context, uuid.UUID, *capturedResetSender) {
	t.Helper()
	s, db, ctx := userSecurityFixture(t, true)
	id := seedAccount(t, db, "user@example.com", role, true)
	sender := &capturedResetSender{}
	s.resetSender = sender
	if err := s.ForgotPassword(ctx, "user@example.com"); err != nil {
		t.Fatal(err)
	}
	return s, db, ctx, id, sender
}

func TestResetCodeSecurityAndAllSessionRevocation(t *testing.T) {
	for _, role := range []model.Role{model.RoleAdmin, model.RoleTeacher, model.RoleStudent} {
		t.Run(string(role), func(t *testing.T) {
			s, db, ctx, id, sender := resetFixture(t, role)
			_, token, version, _ := loginSecurityClaims(t, s, ctx, "user@example.com", accountPassword)
			var hash string
			var ttl float64
			if err := db.QueryRow(`SELECT code_hash,EXTRACT(EPOCH FROM(expires_at-created_at)) FROM password_reset_codes WHERE user_id=$1 AND used_at IS NULL`, id).Scan(&hash, &ttl); err != nil {
				t.Fatal(err)
			}
			if hash == sender.code || !strings.HasPrefix(hash, "$2") || ttl < 895 || ttl > 901 {
				t.Fatalf("unsafe hash or expiry: TTL %f", ttl)
			}
			if err := bcrypt.CompareHashAndPassword([]byte(hash), s.resetCodeMaterial(id, sender.code)); err != nil {
				t.Fatal(err)
			}
			seedAccount(t, db, "other@example.com", model.RoleTeacher, true)
			if err := s.ResetPassword(ctx, "other@example.com", sender.code, "ResetStrongPassword2!"); !errors.Is(err, repository.ErrInvalidResetCode) {
				t.Fatal("wrong-account code accepted", err)
			}
			if err := s.ResetPassword(ctx, "user@example.com", sender.code, "ResetStrongPassword2!"); err != nil {
				t.Fatal(err)
			}
			if err := s.AuthorizeToken(ctx, id, role, token, version); !errors.Is(err, ErrAuthentication) {
				t.Fatal("reset failed to revoke", err)
			}
			if err := s.ResetPassword(ctx, "user@example.com", sender.code, "AnotherStrongPassword3!"); !errors.Is(err, repository.ErrInvalidResetCode) {
				t.Fatal("replay accepted", err)
			}
			var outstanding int
			if err := db.QueryRow(`SELECT count(*) FROM password_reset_codes WHERE user_id=$1 AND used_at IS NULL`, id).Scan(&outstanding); err != nil || outstanding != 0 {
				t.Fatal("outstanding codes remain", err)
			}
			if _, _, err := s.Login(ctx, LoginRequest{Email: "user@example.com", Password: "ResetStrongPassword2!"}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestResetAttemptBudgetCooldownAndExpiry(t *testing.T) {
	s, db, ctx, id, sender := resetFixture(t, model.RoleTeacher)
	first := sender.code
	if err := s.ForgotPassword(ctx, "user@example.com"); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM password_reset_codes WHERE user_id=$1`, id).Scan(&count); err != nil || count != 1 {
		t.Fatal("cooldown bypass", err)
	}
	bad := "000000"
	if bad == first {
		bad = "000001"
	}
	for i := 0; i < 2; i++ {
		if err := s.ResetPassword(ctx, "user@example.com", bad, "ResetStrongPassword2!"); !errors.Is(err, repository.ErrInvalidResetCode) {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`UPDATE password_reset_state SET last_issued_at=clock_timestamp()-interval '61 seconds' WHERE user_id=$1`, id); err != nil {
		t.Fatal(err)
	}
	if err := s.ForgotPassword(ctx, "user@example.com"); err != nil {
		t.Fatal(err)
	}
	var attempts int
	if err := db.QueryRow(`SELECT failed_attempts FROM password_reset_state WHERE user_id=$1`, id).Scan(&attempts); err != nil || attempts != 2 {
		t.Fatal("resend replenished budget", err)
	}
	bad = "000000"
	if bad == sender.code {
		bad = "000001"
	}
	for i := 0; i < 3; i++ {
		if err := s.ResetPassword(ctx, "user@example.com", bad, "ResetStrongPassword2!"); !errors.Is(err, repository.ErrInvalidResetCode) {
			t.Fatal(err)
		}
	}
	if err := s.ResetPassword(ctx, "user@example.com", sender.code, "ResetStrongPassword2!"); !errors.Is(err, repository.ErrInvalidResetCode) {
		t.Fatal("limit bypass", err)
	}
	if _, err := db.Exec(`UPDATE password_reset_state SET last_issued_at=clock_timestamp()-interval '61 seconds' WHERE user_id=$1`, id); err != nil {
		t.Fatal(err)
	}
	if err := s.ForgotPassword(ctx, "user@example.com"); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT count(*) FROM password_reset_codes WHERE user_id=$1`, id).Scan(&count); err != nil || count != 2 {
		t.Fatal("blocked resend issued code", err)
	}
	if _, err := db.Exec(`UPDATE password_reset_state SET window_expires_at=clock_timestamp()-interval '1 second' WHERE user_id=$1`, id); err != nil {
		t.Fatal(err)
	}
	if err := s.ResetPassword(ctx, "user@example.com", sender.code, "ResetStrongPassword2!"); !errors.Is(err, repository.ErrInvalidResetCode) {
		t.Fatal("expired lifecycle accepted", err)
	}
	if err := s.ForgotPassword(ctx, "user@example.com"); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT failed_attempts FROM password_reset_state WHERE user_id=$1`, id).Scan(&attempts); err != nil || attempts != 0 {
		t.Fatal("new lifecycle not available", err)
	}
	if _, err := db.Exec(`UPDATE password_reset_codes SET expires_at=clock_timestamp()-interval '1 second' WHERE user_id=$1`, id); err != nil {
		t.Fatal(err)
	}
	if err := s.ResetPassword(ctx, "user@example.com", sender.code, "ResetStrongPassword2!"); !errors.Is(err, repository.ErrInvalidResetCode) {
		t.Fatal("expired code accepted", err)
	}
}

func TestConcurrentResetSingleUseAndAttempts(t *testing.T) {
	s, db, ctx, id, sender := resetFixture(t, model.RoleStudent)
	results := make(chan error, 8)
	for i := 0; i < 8; i++ {
		go func() { results <- s.ResetPassword(ctx, "user@example.com", sender.code, "ResetStrongPassword2!") }()
	}
	successes := 0
	for i := 0; i < 8; i++ {
		err := <-results
		if err == nil {
			successes++
		} else if !errors.Is(err, repository.ErrInvalidResetCode) {
			t.Fatal(err)
		}
	}
	if successes != 1 {
		t.Fatalf("successful concurrent redemptions %d", successes)
	}
	if _, err := db.Exec(`UPDATE password_reset_state SET last_issued_at=clock_timestamp()-interval '61 seconds' WHERE user_id=$1`, id); err != nil {
		t.Fatal(err)
	}
	if err := s.ForgotPassword(ctx, "user@example.com"); err != nil {
		t.Fatal(err)
	}
	bad := "000000"
	if bad == sender.code {
		bad = "000001"
	}
	for i := 0; i < 8; i++ {
		go func() { results <- s.ResetPassword(ctx, "user@example.com", bad, "ResetStrongPassword2!") }()
	}
	for i := 0; i < 8; i++ {
		if err := <-results; !errors.Is(err, repository.ErrInvalidResetCode) {
			t.Fatal(err)
		}
	}
	var attempts int
	if err := db.QueryRow(`SELECT failed_attempts FROM password_reset_state WHERE user_id=$1`, id).Scan(&attempts); err != nil || attempts != 5 {
		t.Fatalf("lost attempts %d %v", attempts, err)
	}
}

func TestConcurrentResetIssuance(t *testing.T) {
	s, db, ctx := userSecurityFixture(t, true)
	seedAccount(t, db, "user@example.com", model.RoleTeacher, true)
	s.resetSender = &capturedResetSender{}
	results := make(chan error, 8)
	for i := 0; i < 8; i++ {
		go func() { results <- s.ForgotPassword(ctx, "user@example.com") }()
	}
	for i := 0; i < 8; i++ {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM password_reset_codes WHERE used_at IS NULL`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("concurrent issuance %d %v", count, err)
	}
}

type failedResetSender struct{}

func (failedResetSender) SendResetCode(context.Context, string, string) error {
	return errors.New("private SMTP diagnostics")
}
func TestResetEnumerationAndDeliveryFailure(t *testing.T) {
	s, db, ctx := userSecurityFixture(t, true)
	seedAccount(t, db, "active@example.com", model.RoleTeacher, true)
	seedAccount(t, db, "inactive@example.com", model.RoleTeacher, false)
	s.resetSender = failedResetSender{}
	for _, email := range []string{"missing@example.com", "inactive@example.com", "active@example.com"} {
		if err := s.ForgotPassword(ctx, email); err != nil {
			t.Fatal("enumeration through error", err)
		}
	}
	var pending int
	if err := db.QueryRow(`SELECT count(*) FROM password_reset_codes`).Scan(&pending); err != nil || pending != 0 {
		t.Fatal("undelivered code retained", err)
	}
}

func TestResetRollbackIncludesTokenVersion(t *testing.T) {
	s, db, ctx, id, sender := resetFixture(t, model.RoleAdmin)
	_, token, version, _ := loginSecurityClaims(t, s, ctx, "user@example.com", accountPassword)
	if _, err := db.Exec(`CREATE FUNCTION reject_reset() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'test failure'; END $$; CREATE TRIGGER reject_reset BEFORE UPDATE ON password_reset_codes FOR EACH ROW EXECUTE FUNCTION reject_reset()`); err != nil {
		t.Fatal(err)
	}
	if err := s.ResetPassword(ctx, "user@example.com", sender.code, "ResetStrongPassword2!"); err == nil {
		t.Fatal("expected failure")
	}
	if err := s.AuthorizeToken(ctx, id, model.RoleAdmin, token, version); err != nil {
		t.Fatal("rollback revoked token", err)
	}
	if _, _, err := s.Login(ctx, LoginRequest{Email: "user@example.com", Password: accountPassword}); err != nil {
		t.Fatal("rollback changed password", err)
	}
}

func TestAuthenticationMigrationPreservesAccountsAndInvalidatesLegacyCodes(t *testing.T) {
	for _, legacyHash := range []bool{false, true} {
		t.Run(map[bool]string{false: "plaintext", true: "legacy hash"}[legacyHash], func(t *testing.T) {
			s, db, ctx := userSecurityFixture(t, true)
			id := seedAccount(t, db, "user@example.com", model.RoleTeacher, true)
			// Reconstruct the previous reset-code shape in this private schema only.
			_, err := db.Exec(`DROP TABLE revoked_access_tokens,password_reset_state; DROP TRIGGER users_invalidate_tokens ON users; ALTER TABLE users DROP COLUMN token_version; ALTER TABLE password_reset_codes DROP CONSTRAINT reset_code_hash_required; ALTER TABLE password_reset_codes ADD COLUMN code VARCHAR(6);`)
			if err != nil {
				t.Fatal(err)
			}
			if legacyHash {
				_, err = db.Exec(`ALTER TABLE password_reset_codes ALTER COLUMN code_hash SET NOT NULL`)
			} else {
				_, err = db.Exec(`ALTER TABLE password_reset_codes DROP COLUMN code_hash`)
			}
			if err != nil {
				t.Fatal(err)
			}
			if legacyHash {
				_, err = db.Exec(`INSERT INTO password_reset_codes(user_id,code,code_hash) VALUES($1,'123456','legacy-hash')`, id)
			} else {
				_, err = db.Exec(`INSERT INTO password_reset_codes(user_id,code) VALUES($1,'123456')`, id)
			}
			if err != nil {
				t.Fatal(err)
			}
			body, err := os.ReadFile(filepath.Join("..", "..", "migrations", "014_authentication_security.sql"))
			if err != nil {
				t.Fatal(err)
			}
			tx, err := db.BeginTx(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = tx.Exec(string(body)); err != nil {
				tx.Rollback()
				t.Fatal(err)
			}
			if err = tx.Commit(); err != nil {
				t.Fatal(err)
			}
			if _, _, err = s.Login(ctx, LoginRequest{Email: "user@example.com", Password: accountPassword}); err != nil {
				t.Fatal("migration broke password", err)
			}
			var active int
			if err = db.QueryRow(`SELECT count(*) FROM password_reset_codes WHERE used_at IS NULL`).Scan(&active); err != nil || active != 0 {
				t.Fatal("legacy code still valid", err)
			}
			if _, err = db.Exec(`SELECT code FROM password_reset_codes`); err == nil {
				t.Fatal("plaintext column retained")
			}
			if _, err = db.Exec(`INSERT INTO password_reset_codes(user_id) VALUES($1)`, id); err == nil {
				t.Fatal("unhashed active code accepted")
			}
		})
	}
}
