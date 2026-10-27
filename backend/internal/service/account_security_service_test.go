package service

import (
	"context"
	"database/sql"
	"errors"
	"github.com/alcidheek/school-portal/internal/model"
	"github.com/alcidheek/school-portal/internal/repository"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"golang.org/x/crypto/bcrypt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func userSecurityFixture(t *testing.T, migrate bool) (*UserService, *sql.DB, context.Context) {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to an isolated PostgreSQL test database")
	}
	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		var err error
		dsn, err = pq.ParseURL(dsn)
		if err != nil {
			t.Fatal(err)
		}
	}
	admin, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := "account_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	quoted := pq.QuoteIdentifier(schema)
	if _, err = admin.Exec("CREATE SCHEMA " + quoted); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec("DROP SCHEMA " + quoted + " CASCADE"); err != nil {
			t.Error(err)
		}
		admin.Close()
	})
	db, err := sql.Open("postgres", dsn+" search_path="+schema)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(16)
	t.Cleanup(func() { db.Close() })
	names := []string{"001_create_users.sql", "009_password_reset_codes.sql"}
	if migrate {
		names = append(names, "013_user_email_normalization.sql")
	}
	names = append(names, "014_authentication_security.sql")
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	for _, name := range names {
		body, err := os.ReadFile(filepath.Join("..", "..", "migrations", name))
		if err != nil {
			t.Fatal(err)
		}
		query := strings.ReplaceAll(string(body), `CREATE EXTENSION IF NOT EXISTS "pgcrypto";`, "")
		if _, err = tx.Exec(query); err != nil {
			t.Fatal(err)
		}
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	t.Cleanup(cancel)
	return NewUserService(repository.NewUserRepository(db), "test-account-secret"), db, ctx
}

const accountPassword = "OriginalStrong1!"

func seedAccount(t *testing.T, db *sql.DB, email string, role model.Role, active bool) uuid.UUID {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(accountPassword), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	id := uuid.New()
	if _, err = db.Exec(`INSERT INTO users(id,full_name,email,phone,password_hash,role,is_active,guardian_name,guardian_phone) VALUES($1,'Original',$2,'+2348012345678',$3,$4::varchar,$5,CASE WHEN $4::varchar='STUDENT' THEN 'Guardian' ELSE NULL END,CASE WHEN $4::varchar='STUDENT' THEN '+2348012345678' ELSE NULL END)`, id, email, string(hash), role, active); err != nil {
		t.Fatal(err)
	}
	return id
}
func ptr[T any](value T) *T { return &value }

func TestUserValidation(t *testing.T) {
	for _, email := range []string{"", "invalid", "Name <name@example.com>", "a b@example.com", "a@example.com\r\nBcc: b@example.com"} {
		if _, err := normalizeUserEmail(email); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("accepted %q", email)
		}
	}
	if value, err := normalizeUserEmail(" USER@Example.COM \t"); err != nil || value != "user@example.com" {
		t.Fatalf("normalization %q %v", value, err)
	}
	for _, phone := range []string{"", "123", "+234ABCDEFG", strings.Repeat("1", 16)} {
		if _, err := validatePhone(phone); !errors.Is(err, ErrInvalidInput) {
			t.Fatal("accepted phone", phone)
		}
	}
	for _, password := range []string{"weak", strings.Repeat("a", 12), "Strong12345678", strings.Repeat("Ab1!", 19)} {
		if !errors.Is(validateStrongPassword(password), ErrInvalidInput) {
			t.Fatal("accepted weak password")
		}
	}
	if err := validateStrongPassword(accountPassword); err != nil {
		t.Fatal(err)
	}
	s := &UserService{}
	for _, req := range []RegisterUserRequest{
		{FullName: " ", Email: "a@example.com", Phone: "1234567", Password: "example", Role: model.RoleTeacher},
		{FullName: "Name", Email: "bad", Phone: "1234567", Password: "example", Role: model.RoleTeacher},
		{FullName: "Name", Email: "a@example.com", Phone: "1234567", Password: "example", Role: model.RoleStudent},
		{FullName: "Name", Email: "a@example.com", Phone: "1234567", Password: "example", Role: model.RoleTeacher, GuardianName: "Guardian"},
		{FullName: "Name", Email: "a@example.com", Phone: "1234567", Password: "example", Role: model.RoleAdmin},
	} {
		if _, err := s.RegisterUser(context.Background(), req); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("validation: %v", err)
		}
	}
}
func TestEmailChangesAndLegacyLogin(t *testing.T) {
	s, db, ctx := userSecurityFixture(t, true)
	id := seedAccount(t, db, "\t\u00a0 OLD@Example.com \u3000\n", model.RoleAdmin, true)
	vID := seedAccount(t, db, "vanessa@example.gov", model.RoleTeacher, true)
	if user, err := s.userRepo.GetUserByEmail(ctx, "vanessa@example.gov"); err != nil || user.ID != vID {
		t.Fatal("whitespace trimming changed letter v", err)
	}
	seedAccount(t, db, "other@example.com", model.RoleTeacher, true)
	if _, user, err := s.Login(ctx, LoginRequest{Email: " old@EXAMPLE.COM ", Password: accountPassword}); err != nil || user.Email != "old@example.com" {
		t.Fatalf("legacy login: %v", err)
	}
	if _, _, err := s.ChangeEmail(ctx, id, "new@example.com", "wrong"); !errors.Is(err, ErrCurrentPassword) {
		t.Fatal(err)
	}
	if _, _, err := s.ChangeEmail(ctx, id, " OTHER@Example.com ", accountPassword); !errors.Is(err, ErrEmailConflict) {
		t.Fatal(err)
	}
	if _, _, err := s.ChangeEmail(ctx, id, "invalid", accountPassword); !errors.Is(err, ErrInvalidInput) {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO password_reset_codes(user_id,code_hash) VALUES($1,'123456')`, id); err != nil {
		t.Fatal(err)
	}
	token, user, err := s.ChangeEmail(ctx, id, " NEW@Example.com ", accountPassword)
	if err != nil || token == "" || user.ID != id || user.Email != "new@example.com" || user.PasswordHash != "" {
		t.Fatalf("email success: %+v %v", user, err)
	}
	var pending int
	if err = db.QueryRow(`SELECT count(*) FROM password_reset_codes WHERE user_id=$1 AND used_at IS NULL`, id).Scan(&pending); err != nil || pending != 0 {
		t.Fatalf("reset codes not invalidated %d %v", pending, err)
	}
	if _, _, err = s.Login(ctx, LoginRequest{Email: "new@example.com", Password: accountPassword}); err != nil {
		t.Fatal(err)
	}
	if _, _, err = s.Login(ctx, LoginRequest{Email: "old@example.com", Password: accountPassword}); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatal(err)
	}
	if _, _, err = s.ChangeEmail(ctx, id, "new@example.com", accountPassword); err != nil {
		t.Fatal("idempotent email", err)
	}
	teacher := seedAccount(t, db, "teacher@example.com", model.RoleTeacher, true)
	inactive := seedAccount(t, db, "inactive@example.com", model.RoleAdmin, false)
	for _, tc := range []struct {
		id   uuid.UUID
		want error
	}{{teacher, ErrForbiddenAccount}, {inactive, ErrInactiveAccount}, {uuid.New(), sql.ErrNoRows}} {
		if _, _, err = s.ChangeEmail(ctx, tc.id, "unused@example.com", accountPassword); !errors.Is(err, tc.want) {
			t.Fatal(err)
		}
	}
}
func TestPasswordChangesAndRollback(t *testing.T) {
	s, db, ctx := userSecurityFixture(t, true)
	id := seedAccount(t, db, "admin@example.com", model.RoleAdmin, true)
	for _, tc := range []struct {
		current, next, confirm string
		want                   error
	}{{"wrong", "NewStrongPassword2!", "NewStrongPassword2!", ErrCurrentPassword}, {accountPassword, "weak", "weak", ErrInvalidInput}, {accountPassword, "NewStrongPassword2!", "different", ErrInvalidInput}, {accountPassword, accountPassword, accountPassword, ErrInvalidInput}} {
		if err := s.ChangePassword(ctx, id, tc.current, tc.next, tc.confirm); !errors.Is(err, tc.want) {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`INSERT INTO password_reset_codes(user_id,code_hash) VALUES($1,'123456')`, id); err != nil {
		t.Fatal(err)
	}
	if err := s.ChangePassword(ctx, id, accountPassword, "NewStrongPassword2!", "NewStrongPassword2!"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Login(ctx, LoginRequest{Email: "admin@example.com", Password: accountPassword}); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatal(err)
	}
	if _, _, err := s.Login(ctx, LoginRequest{Email: "admin@example.com", Password: "NewStrongPassword2!"}); err != nil {
		t.Fatal(err)
	}
	if err := s.ResetPassword(ctx, "admin@example.com", "123456", "NewStrongPassword2!"); !errors.Is(err, repository.ErrInvalidResetCode) {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE FUNCTION reject_code_update() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'private test failure'; END $$; CREATE TRIGGER reject_code_update BEFORE UPDATE ON password_reset_codes FOR EACH ROW EXECUTE FUNCTION reject_code_update();`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO password_reset_codes(user_id,code_hash) VALUES($1,'234567')`, id); err != nil {
		t.Fatal(err)
	}
	if err := s.ChangePassword(ctx, id, "NewStrongPassword2!", "RollbackStrong3!", "RollbackStrong3!"); err == nil {
		t.Fatal("expected rollback")
	}
	if _, _, err := s.ChangeEmail(ctx, id, "rollback@example.com", "NewStrongPassword2!"); err == nil {
		t.Fatal("expected email rollback")
	}
	if _, _, err := s.Login(ctx, LoginRequest{Email: "admin@example.com", Password: "NewStrongPassword2!"}); err != nil {
		t.Fatal("password/email changed despite rollback", err)
	}
}
func TestUserPatchAndAdminSafeguard(t *testing.T) {
	s, db, ctx := userSecurityFixture(t, true)
	admin := seedAccount(t, db, "admin@example.com", model.RoleAdmin, true)
	student := seedAccount(t, db, "student@example.com", model.RoleStudent, true)
	for _, req := range []model.UpdateUserRequest{{FullName: ptr(" ")}, {Phone: ptr("invalid")}, {GuardianName: ptr(" ")}, {GuardianPhone: ptr("123")}, {Email: ptr("invalid")}} {
		if err := s.UpdateUser(ctx, student, req); !errors.Is(err, ErrInvalidInput) {
			t.Fatal(err)
		}
	}
	if err := s.UpdateUser(ctx, admin, model.UpdateUserRequest{GuardianName: ptr("Guardian")}); !errors.Is(err, ErrInvalidInput) {
		t.Fatal(err)
	}
	if err := s.UpdateUser(ctx, student, model.UpdateUserRequest{Email: ptr(" ADMIN@Example.com ")}); !errors.Is(err, ErrEmailConflict) {
		t.Fatal(err)
	}
	if err := s.UpdateUser(ctx, student, model.UpdateUserRequest{FullName: ptr(" Updated Student "), Email: ptr(" NEW.STUDENT@Example.com ")}); err != nil {
		t.Fatal(err)
	}
	user, err := s.GetUserByID(ctx, student)
	if err != nil || user.FullName != "Updated Student" || user.Email != "new.student@example.com" || user.GuardianName == nil || *user.GuardianName != "Guardian" || user.Phone != "+2348012345678" || !user.IsActive {
		t.Fatalf("PATCH lost fields: %+v %v", user, err)
	}
	if err := s.UpdateUser(ctx, admin, model.UpdateUserRequest{IsActive: ptr(false)}); !errors.Is(err, ErrFinalActiveAdmin) {
		t.Fatal(err)
	}
	if err := s.UpdateUserAsAdmin(ctx, admin, admin, model.UpdateUserRequest{Email: ptr("bypass@example.com")}); !errors.Is(err, ErrForbiddenAccount) {
		t.Fatal(err)
	}
	if err := s.UpdateUserAsAdmin(ctx, student, admin, model.UpdateUserRequest{}); !errors.Is(err, ErrForbiddenAccount) {
		t.Fatal(err)
	}
}
func TestConcurrentAdminDeactivation(t *testing.T) {
	s, db, ctx := userSecurityFixture(t, true)
	ids := []uuid.UUID{seedAccount(t, db, "one@example.com", model.RoleAdmin, true), seedAccount(t, db, "two@example.com", model.RoleAdmin, true)}
	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for _, id := range ids {
		wg.Add(1)
		go func(id uuid.UUID) {
			defer wg.Done()
			<-start
			results <- s.UpdateUserAsAdmin(ctx, id, id, model.UpdateUserRequest{IsActive: ptr(false)})
		}(id)
	}
	close(start)
	wg.Wait()
	close(results)
	success, rejected := 0, 0
	for err := range results {
		if err == nil {
			success++
		} else if errors.Is(err, ErrFinalActiveAdmin) {
			rejected++
		} else {
			t.Fatal(err)
		}
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM users WHERE role='ADMIN' AND is_active`).Scan(&count); err != nil || count != 1 || success != 1 || rejected != 1 {
		t.Fatalf("guard bypassed: %d %d %d %v", count, success, rejected, err)
	}
}
func TestBootstrapValidationAndPreservation(t *testing.T) {
	s, db, ctx := userSecurityFixture(t, true)
	for _, tc := range []struct{ name, email, phone, password string }{{" ", "admin@example.com", "1234567", accountPassword}, {"Admin", "bad", "1234567", accountPassword}, {"Admin", "admin@example.com", "bad", accountPassword}, {"Admin", "admin@example.com", "1234567", "weak"}} {
		if err := s.BootstrapAdmin(ctx, tc.name, tc.email, tc.phone, tc.password); !errors.Is(err, ErrInvalidInput) {
			t.Fatal(err)
		}
	}
	seedAccount(t, db, "taken@example.com", model.RoleTeacher, true)
	if err := s.BootstrapAdmin(ctx, "Admin", " TAKEN@example.com ", "1234567", accountPassword); !errors.Is(err, ErrEmailConflict) {
		t.Fatal(err)
	}
	if err := s.BootstrapAdmin(ctx, " Admin ", " ADMIN@Example.com ", "1234567", accountPassword); err != nil {
		t.Fatal(err)
	}
	before, err := s.userRepo.GetUserByEmail(ctx, "admin@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.BootstrapAdmin(ctx, "different", "different@example.com", "invalid", "weak"); err != nil {
		t.Fatal(err)
	}
	after, err := s.userRepo.GetUserByID(ctx, before.ID)
	if err != nil || before.Email != after.Email || before.PasswordHash != after.PasswordHash || before.EmployeeNumber == nil || *before.EmployeeNumber != *after.EmployeeNumber {
		t.Fatal("bootstrap overwrote existing admin", err)
	}
}
func TestNormalizedEmailMigrationCompatibility(t *testing.T) {
	_, db, _ := userSecurityFixture(t, false)
	id := seedAccount(t, db, " Legacy@Example.com ", model.RoleAdmin, true)
	other := seedAccount(t, db, "legacy@example.com", model.RoleTeacher, true)
	body, err := os.ReadFile(filepath.Join("..", "..", "migrations", "013_user_email_normalization.sql"))
	if err != nil {
		t.Fatal(err)
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(string(body))
	tx.Rollback()
	if err == nil {
		t.Fatal("migration accepted conflicting legacy rows")
	}
	var count int
	if err = db.QueryRow(`SELECT count(*) FROM users`).Scan(&count); err != nil || count != 2 {
		t.Fatal("migration changed records", count, err)
	}
	if _, err = db.Exec(`UPDATE users SET email='different@example.com' WHERE id=$1`, other); err != nil {
		t.Fatal(err)
	} // explicit test fixture decision, never automatic repair
	tx, err = db.Begin()
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
	var email string
	if err = db.QueryRow(`SELECT email FROM users WHERE id=$1`, id).Scan(&email); err != nil || email != " Legacy@Example.com " {
		t.Fatal("migration rewrote legacy email", email, err)
	}
	_, err = db.Exec(`INSERT INTO users(full_name,email,password_hash,role) VALUES('Duplicate','LEGACY@example.com','unused','TEACHER')`)
	var pg *pq.Error
	if !errors.As(err, &pg) || pg.Code != "23505" {
		t.Fatal("normalized uniqueness missing", err)
	}
}

func TestRegistrationAndConcurrentEmailChanges(t *testing.T) {
	s, db, ctx := userSecurityFixture(t, true)
	admin := seedAccount(t, db, "admin@example.com", model.RoleAdmin, true)
	student, err := s.RegisterUserAsAdmin(ctx, admin, RegisterUserRequest{FullName: " Student ", Email: " STUDENT@Example.COM ", Phone: "+2348012345678", Password: "student-secret", Role: model.RoleStudent, GuardianName: " Guardian ", GuardianPhone: "+2348012345678"})
	if err != nil || student.Email != "student@example.com" || student.AdmissionNumber == nil || student.EmployeeNumber != nil || student.GuardianName == nil || *student.GuardianName != "Guardian" || student.PasswordHash != "" {
		t.Fatalf("register student %+v %v", student, err)
	}
	if _, _, err = s.Login(ctx, LoginRequest{Email: " STUDENT@Example.com ", Password: "student-secret"}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.RegisterUserAsAdmin(ctx, admin, RegisterUserRequest{FullName: "Duplicate", Email: "STUDENT@example.com", Phone: "1234567", Password: "password", Role: model.RoleTeacher}); !errors.Is(err, ErrEmailConflict) {
		t.Fatal(err)
	}
	inactive := seedAccount(t, db, "inactive@example.com", model.RoleAdmin, false)
	if _, err = s.RegisterUserAsAdmin(ctx, inactive, RegisterUserRequest{FullName: "Teacher", Email: "teacher@example.com", Phone: "1234567", Password: "password", Role: model.RoleTeacher}); !errors.Is(err, ErrInactiveAccount) {
		t.Fatal(err)
	}
	second := seedAccount(t, db, "second@example.com", model.RoleAdmin, true)
	start := make(chan struct{})
	results := make(chan error, 2)
	for _, id := range []uuid.UUID{admin, second} {
		go func(id uuid.UUID) {
			<-start
			_, _, err := s.ChangeEmail(ctx, id, " SHARED@Example.COM ", accountPassword)
			results <- err
		}(id)
	}
	close(start)
	success, conflicts := 0, 0
	for range 2 {
		err := <-results
		if err == nil {
			success++
		} else if errors.Is(err, ErrEmailConflict) {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || conflicts != 1 {
		t.Fatalf("concurrent email: %d %d", success, conflicts)
	}
}

func TestAdminResetPasswordPolicyAndNormalization(t *testing.T) {
	s, db, ctx := userSecurityFixture(t, true)
	id := seedAccount(t, db, " ADMIN@Example.com ", model.RoleAdmin, true)
	hash, err := bcrypt.GenerateFromPassword(s.resetCodeMaterial(id, "123456"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO password_reset_state(user_id,window_expires_at) VALUES($1,clock_timestamp()+interval '15 minutes');`, id); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO password_reset_codes(user_id,code_hash) VALUES($1,$2)`, id, string(hash)); err != nil {
		t.Fatal(err)
	}
	if err := s.ResetPassword(ctx, " ADMIN@example.com ", "wrong", "weak"); !errors.Is(err, ErrResetCodeFormat) {
		t.Fatal(err)
	}
	if err := s.ResetPassword(ctx, " ADMIN@example.com ", "654321", "weakpass"); !errors.Is(err, repository.ErrInvalidResetCode) {
		t.Fatal(err)
	}
	if err := s.ResetPassword(ctx, " ADMIN@example.com ", "123456", "weakpass"); !errors.Is(err, ErrInvalidInput) {
		t.Fatal("reset bypassed admin policy", err)
	}
	if err := s.ResetPassword(ctx, " ADMIN@example.com ", "123456", "ResetStrongPassword2!"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Login(ctx, LoginRequest{Email: "admin@example.com", Password: "ResetStrongPassword2!"}); err != nil {
		t.Fatal(err)
	}
}

func TestConcurrentAdminBootstrap(t *testing.T) {
	s, db, ctx := userSecurityFixture(t, true)
	start := make(chan struct{})
	results := make(chan error, 2)
	for _, email := range []string{"first@example.com", "second@example.com"} {
		go func(email string) {
			<-start
			results <- s.BootstrapAdmin(ctx, "Admin", email, "1234567", accountPassword)
		}(email)
	}
	close(start)
	for range 2 {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM users WHERE role='ADMIN'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("duplicate bootstrap: %d %v", count, err)
	}
}

type capturedResetSender struct{ email, code string }

func (sender *capturedResetSender) SendResetCode(_ context.Context, email, code string) error {
	sender.email = email
	sender.code = code
	return nil
}
func TestForgotPasswordNormalizesLegacyRecipient(t *testing.T) {
	s, db, ctx := userSecurityFixture(t, true)
	seedAccount(t, db, " ADMIN@Example.com ", model.RoleAdmin, true)
	sender := &capturedResetSender{}
	s.resetSender = sender
	if err := s.ForgotPassword(ctx, " ADMIN@Example.COM "); err != nil {
		t.Fatal(err)
	}
	if sender.email != "admin@example.com" || len(sender.code) != 6 {
		t.Fatalf("invalid normalized recipient/code: %q", sender.email)
	}
	if err := s.ResetPassword(ctx, " admin@example.com ", sender.code, "ResetStrongPassword2!"); err != nil {
		t.Fatal(err)
	}
}
