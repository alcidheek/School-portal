package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alcidheek/school-portal/internal/model"
	"github.com/alcidheek/school-portal/internal/repository"
	"github.com/google/uuid"
	"github.com/lib/pq"
)

// A unique schema and pool-wide search_path let concurrent connections see the
// same test tables without touching application tables or migration history.
func academicServiceFixture(t *testing.T) (*AcademicSessionService, *sql.DB, context.Context) {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to run PostgreSQL academic service tests")
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
	schema := "academic_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	quoted := pq.QuoteIdentifier(schema)
	if _, err := admin.Exec("CREATE SCHEMA " + quoted); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec("DROP SCHEMA " + quoted + " CASCADE"); err != nil {
			t.Errorf("cleanup schema: %v", err)
		}
		admin.Close()
	})
	db, err := sql.Open("postgres", dsn+" search_path="+schema)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(32)
	t.Cleanup(func() { db.Close() })
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	for _, name := range []string{"002_academic_section.sql", "003_terms.sql", "012_academic_lifecycle.sql"} {
		content, err := os.ReadFile(filepath.Join("..", "..", "migrations", name))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(string(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	return NewAcademicSessionService(repository.NewAcademicSessionRepository(db), repository.NewTermRepository(db)), db, ctx
}

func createAcademic(t *testing.T, s *AcademicSessionService, ctx context.Context, name string, active bool) *model.AcademicSession {
	t.Helper()
	session, err := s.Create(ctx, name, "2026-01-01", "2026-12-31", active, "FIRST_TERM", "2026-01-01", "2026-04-01")
	if err != nil {
		t.Fatal(err)
	}
	return session
}

func assertAcademicState(t *testing.T, db *sql.DB, sessionID, termID uuid.UUID) {
	t.Helper()
	var sessions, terms, bad int
	if err := db.QueryRow(`SELECT (SELECT count(*) FROM academic_sessions WHERE is_active),
		(SELECT count(*) FROM terms WHERE is_active),
		(SELECT count(*) FROM terms t JOIN academic_sessions s ON s.id=t.academic_session_id
		WHERE t.is_active AND (NOT s.is_active OR s.id<>$1 OR t.id<>$2 OR s.last_active_term_id IS DISTINCT FROM t.id))`, sessionID, termID).Scan(&sessions, &terms, &bad); err != nil {
		t.Fatal(err)
	}
	if sessions != 1 || terms != 1 || bad != 0 {
		t.Fatalf("invalid active state: sessions=%d terms=%d mismatches=%d", sessions, terms, bad)
	}
}

func TestAcademicSessionLifecycle(t *testing.T) {
	s, db, ctx := academicServiceFixture(t)
	a := createAcademic(t, s, ctx, "A", true)
	first, err := s.GetActiveTerm(ctx, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	assertAcademicState(t, db, a.ID, first.ID)
	second, err := s.SwitchTerm(ctx, a.ID, "SECOND_TERM", "2026-05-01", "2026-08-01")
	if err != nil {
		t.Fatal(err)
	}
	back, err := s.SwitchTerm(ctx, a.ID, "FIRST_TERM", "2026-01-01", "2026-04-01")
	if err != nil || back.ID != first.ID || !back.StartDate.Equal(first.StartDate) || !back.EndDate.Equal(first.EndDate) {
		t.Fatalf("first term reuse: %v %v", back, err)
	}
	back, err = s.SwitchTerm(ctx, a.ID, "SECOND_TERM", "2026-05-01", "2026-08-01")
	if err != nil || back.ID != second.ID {
		t.Fatalf("second term reuse: %v %v", back, err)
	}
	b := createAcademic(t, s, ctx, "B", false)
	if b.LastActiveTermID.Valid {
		t.Fatal("inactive creation invented activity history")
	}
	if err := s.Activate(ctx, b.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetActiveTerm(ctx, a.ID); !errors.Is(err, ErrActiveTermNotFound) {
		t.Fatalf("inactive term lookup: %v", err)
	}
	if err := s.Activate(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	assertAcademicState(t, db, a.ID, second.ID)
	for i := 0; i < 3; i++ {
		if err := s.Activate(ctx, a.ID); err != nil {
			t.Fatal(err)
		}
	}
	assertAcademicState(t, db, a.ID, second.ID)
	// New active creation must preserve the outgoing term as well.
	c := createAcademic(t, s, ctx, "C", true)
	if err := s.Activate(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	assertAcademicState(t, db, a.ID, second.ID)
	if _, err := s.SwitchTerm(ctx, c.ID, "SECOND_TERM", "2026-05-01", "2026-08-01"); !errors.Is(err, ErrAcademicSessionInactive) {
		t.Fatalf("inactive switch: %v", err)
	}
	// Recover an already-active session whose term activity was lost.
	if _, err := db.Exec(`UPDATE terms SET is_active=FALSE`); err != nil {
		t.Fatal(err)
	}
	if err := s.Activate(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	assertAcademicState(t, db, a.ID, second.ID)
	// Null memory falls back by term sequence, not insertion date or UUID.
	if _, err := db.Exec(`UPDATE terms SET is_active=FALSE; UPDATE academic_sessions SET last_active_term_id=NULL WHERE name='A'`); err != nil {
		t.Fatal(err)
	}
	if err := s.Activate(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	assertAcademicState(t, db, a.ID, first.ID)
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM terms WHERE academic_session_id=$1`, a.ID).Scan(&count); err != nil || count != 2 {
		t.Fatalf("duplicate records: %d %v", count, err)
	}
}

func TestAcademicInvalidRequests(t *testing.T) {
	s, db, ctx := academicServiceFixture(t)
	a := createAcademic(t, s, ctx, "A", true)
	first, err := s.GetActiveTerm(ctx, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, term, start, end string
		want                   error
	}{
		{"invalid name", "FOURTH_TERM", "2026-05-01", "2026-08-01", ErrInvalidAcademicInput},
		{"malformed start", "SECOND_TERM", "bad", "2026-08-01", ErrInvalidAcademicInput},
		{"malformed end", "SECOND_TERM", "2026-05-01", "bad", ErrInvalidAcademicInput},
		{"reversed", "SECOND_TERM", "2026-08-01", "2026-05-01", ErrInvalidAcademicInput},
		{"equal", "SECOND_TERM", "2026-05-01", "2026-05-01", ErrInvalidAcademicInput},
		{"before session", "SECOND_TERM", "2025-12-31", "2026-08-01", ErrInvalidAcademicInput},
		{"after session", "SECOND_TERM", "2026-05-01", "2027-01-01", ErrInvalidAcademicInput},
		{"existing date conflict", "FIRST_TERM", "2026-01-02", "2026-04-01", ErrTermDatesConflict},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := s.SwitchTerm(ctx, a.ID, tc.term, tc.start, tc.end); !errors.Is(err, tc.want) {
				t.Fatalf("got %v want %v", err, tc.want)
			}
			assertAcademicState(t, db, a.ID, first.ID)
		})
	}
	missing := uuid.New()
	if err := s.Activate(ctx, missing); !errors.Is(err, ErrAcademicSessionNotFound) {
		t.Fatalf("missing activate: %v", err)
	}
	if _, err := s.SwitchTerm(ctx, missing, "FIRST_TERM", "2026-01-01", "2026-04-01"); !errors.Is(err, ErrAcademicSessionNotFound) {
		t.Fatalf("missing switch: %v", err)
	}
	if _, err := s.GetActiveTerm(ctx, missing); !errors.Is(err, ErrAcademicSessionNotFound) {
		t.Fatalf("missing lookup: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO academic_sessions (name,start_date,end_date) VALUES ('Empty','2026-01-01','2026-12-31')`); err != nil {
		t.Fatal(err)
	}
	var empty uuid.UUID
	if err := db.QueryRow(`SELECT id FROM academic_sessions WHERE name='Empty'`).Scan(&empty); err != nil {
		t.Fatal(err)
	}
	if err := s.Activate(ctx, empty); !errors.Is(err, ErrNoAvailableTerm) {
		t.Fatalf("no terms: %v", err)
	}
	assertAcademicState(t, db, a.ID, first.ID)
	// Bad stored dates must not be restored, including an already-active term.
	if _, err := db.Exec(`UPDATE terms SET end_date='2027-01-01' WHERE id=$1`, first.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.Activate(ctx, a.ID); !errors.Is(err, ErrInvalidAcademicInput) {
		t.Fatalf("invalid stored dates: %v", err)
	}
	if _, err := db.Exec(`UPDATE terms SET end_date='2026-04-01',is_active=FALSE WHERE id=$1`, first.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetActiveTerm(ctx, a.ID); !errors.Is(err, ErrActiveTermNotFound) {
		t.Fatalf("missing active term: %v", err)
	}
	if err := s.Activate(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	// Boundary equality is valid for a new term.
	if _, err := s.SwitchTerm(ctx, a.ID, "THIRD_TERM", "2026-01-01", "2026-12-31"); err != nil {
		t.Fatal(err)
	}
}

func academicSnapshot(t *testing.T, db *sql.DB) string {
	t.Helper()
	var state string
	if err := db.QueryRow(`SELECT json_build_array((SELECT json_agg(s ORDER BY id) FROM academic_sessions s),
		(SELECT json_agg(t ORDER BY id) FROM terms t))::text`).Scan(&state); err != nil {
		t.Fatal(err)
	}
	return state
}

func TestAcademicTransactionRollback(t *testing.T) {
	s, db, ctx := academicServiceFixture(t)
	a := createAcademic(t, s, ctx, "A", true)
	b := createAcademic(t, s, ctx, "B", false)
	// A failed session insert occurs after outgoing memory and flags were updated.
	before := academicSnapshot(t, db)
	if _, err := s.Create(ctx, "A", "2026-01-01", "2026-12-31", true, "FIRST_TERM", "2026-01-01", "2026-04-01"); !errors.Is(err, ErrAcademicConflict) {
		t.Fatalf("duplicate session: %v", err)
	}
	if after := academicSnapshot(t, db); after != before {
		t.Fatal("failed create changed lifecycle")
	}
	if _, err := db.Exec(`ALTER TABLE terms ADD CONSTRAINT reject_second CHECK (name <> 'SECOND_TERM')`); err != nil {
		t.Fatal(err)
	}
	before = academicSnapshot(t, db)
	if _, err := s.SwitchTerm(ctx, a.ID, "SECOND_TERM", "2026-05-01", "2026-08-01"); err == nil {
		t.Fatal("expected insert failure")
	}
	if after := academicSnapshot(t, db); after != before {
		t.Fatal("failed switch changed lifecycle")
	}
	// Activation failure after outgoing deactivation must restore both sessions.
	if _, err := db.Exec(`ALTER TABLE academic_sessions ADD CONSTRAINT reject_b CHECK (name<>'B' OR NOT is_active)`); err != nil {
		t.Fatal(err)
	}
	before = academicSnapshot(t, db)
	if err := s.Activate(ctx, b.ID); err == nil {
		t.Fatal("expected activation failure")
	}
	if after := academicSnapshot(t, db); after != before {
		t.Fatal("failed activation changed lifecycle")
	}
	// Commit-time failures also roll back the inserted records and all flags.
	if _, err := db.Exec(`CREATE FUNCTION reject_commit() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN IF NEW.name='CommitFail' THEN RAISE EXCEPTION 'private commit failure'; END IF; RETURN NEW; END $$;
		CREATE CONSTRAINT TRIGGER fail_commit AFTER INSERT ON academic_sessions DEFERRABLE INITIALLY DEFERRED
		FOR EACH ROW EXECUTE FUNCTION reject_commit()`); err != nil {
		t.Fatal(err)
	}
	before = academicSnapshot(t, db)
	if _, err := s.Create(ctx, "CommitFail", "2026-01-01", "2026-12-31", true, "FIRST_TERM", "2026-01-01", "2026-04-01"); err == nil {
		t.Fatal("expected commit failure")
	}
	if after := academicSnapshot(t, db); after != before {
		t.Fatal("commit failure changed lifecycle")
	}
}

func TestAcademicConcurrentOperations(t *testing.T) {
	s, db, ctx := academicServiceFixture(t)
	a := createAcademic(t, s, ctx, "A", false)
	b := createAcademic(t, s, ctx, "B", false)
	start := make(chan struct{})
	errs := make(chan error, 24)
	var wg sync.WaitGroup
	for i := 0; i < 24; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			id := a.ID
			if i%2 == 1 {
				id = b.ID
			}
			errs <- s.Activate(ctx, id)
		}(i)
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var active, term uuid.UUID
	if err := db.QueryRow(`SELECT s.id,t.id FROM academic_sessions s JOIN terms t ON t.academic_session_id=s.id WHERE s.is_active AND t.is_active`).Scan(&active, &term); err != nil {
		t.Fatal(err)
	}
	assertAcademicState(t, db, active, term)
	// Concurrent requests for the same absent term must insert once, then reuse it.
	start = make(chan struct{})
	results := make(chan *model.Term, 16)
	errs = make(chan error, 16)
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			term, err := s.SwitchTerm(ctx, active, "SECOND_TERM", "2026-05-01", "2026-08-01")
			results <- term
			errs <- err
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var chosen uuid.UUID
	for term := range results {
		if chosen == uuid.Nil {
			chosen = term.ID
		}
		if term.ID != chosen {
			t.Fatal("concurrent switch created duplicates")
		}
	}
	assertAcademicState(t, db, active, chosen)
	// Active creation uses the same lock as activation.
	start = make(chan struct{})
	errs = make(chan error, 16)
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			if i%2 == 0 {
				errs <- s.Activate(ctx, a.ID)
			} else {
				_, err := s.Create(ctx, fmt.Sprintf("Concurrent-%d", i), "2026-01-01", "2026-12-31", true, "FIRST_TERM", "2026-01-01", "2026-04-01")
				errs <- err
			}
		}(i)
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := db.QueryRow(`SELECT s.id,t.id FROM academic_sessions s JOIN terms t ON t.academic_session_id=s.id WHERE s.is_active AND t.is_active`).Scan(&active, &term); err != nil {
		t.Fatal(err)
	}
	assertAcademicState(t, db, active, term)
}

func TestAcademicLockCancellation(t *testing.T) {
	s, db, ctx := academicServiceFixture(t)
	a := createAcademic(t, s, ctx, "A", true)
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, academicLifecycleLockKey); err != nil {
		t.Fatal(err)
	}
	before := academicSnapshot(t, db)
	for _, operation := range []struct {
		name string
		call func(context.Context) error
	}{
		{"activate", func(c context.Context) error { return s.Activate(c, a.ID) }},
		{"switch", func(c context.Context) error {
			_, err := s.SwitchTerm(c, a.ID, "SECOND_TERM", "2026-05-01", "2026-08-01")
			return err
		}},
		{"create", func(c context.Context) error {
			_, err := s.Create(c, "Blocked", "2026-01-01", "2026-12-31", true, "FIRST_TERM", "2026-01-01", "2026-04-01")
			return err
		}},
	} {
		t.Run(operation.name, func(t *testing.T) {
			limited, cancel := context.WithTimeout(ctx, 150*time.Millisecond)
			defer cancel()
			if err := operation.call(limited); err == nil {
				t.Fatal("operation bypassed held lifecycle lock")
			}
			if limited.Err() != context.DeadlineExceeded {
				t.Fatal("operation failed before waiting on lock")
			}
		})
	}
	if after := academicSnapshot(t, db); after != before {
		t.Fatal("canceled lock wait changed state")
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if err := s.Activate(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
}

func TestAcademicCreateValidation(t *testing.T) {
	// Validation must fail before even opening a lifecycle transaction.
	s := NewAcademicSessionService(nil, nil)
	for _, tc := range []struct {
		name  string
		field int
		value string
	}{
		{"blank name", 0, " "}, {"session start", 1, "bad"}, {"session end", 2, "bad"},
		{"session ordering", 2, "2026-01-01"}, {"term name", 3, "FOURTH_TERM"},
		{"term start", 4, "bad"}, {"term end", 5, "bad"}, {"term ordering", 5, "2026-01-01"},
		{"term before session", 4, "2025-12-31"}, {"term after session", 5, "2027-01-01"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := []string{"A", "2026-01-01", "2026-12-31", "FIRST_TERM", "2026-01-01", "2026-04-01"}
			args[tc.field] = tc.value
			if _, err := s.Create(context.Background(), args[0], args[1], args[2], true, args[3], args[4], args[5]); !errors.Is(err, ErrInvalidAcademicInput) {
				t.Fatalf("got %v", err)
			}
		})
	}
}

func TestAcademicRejectsCorruptRememberedTerm(t *testing.T) {
	s, db, ctx := academicServiceFixture(t)
	a := createAcademic(t, s, ctx, "A", true)
	b := createAcademic(t, s, ctx, "B", false)
	term, err := s.GetActiveTerm(ctx, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	// The real FK prevents corruption (repository tests cover it). Remove it only
	// in this isolated schema to exercise the service's defensive ownership check.
	if _, err := db.Exec(`ALTER TABLE academic_sessions DROP CONSTRAINT academic_sessions_last_active_term_fk`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE academic_sessions SET last_active_term_id=$1 WHERE id=$2`, term.ID, b.ID); err != nil {
		t.Fatal(err)
	}
	before := academicSnapshot(t, db)
	if err := s.Activate(ctx, b.ID); !errors.Is(err, ErrTermSessionMismatch) {
		t.Fatalf("cross-session history: %v", err)
	}
	if after := academicSnapshot(t, db); after != before {
		t.Fatal("cross-session rejection changed state")
	}
	if _, err := db.Exec(`UPDATE academic_sessions SET last_active_term_id=$1 WHERE id=$2`, uuid.New(), b.ID); err != nil {
		t.Fatal(err)
	}
	before = academicSnapshot(t, db)
	if err := s.Activate(ctx, b.ID); !errors.Is(err, ErrNoAvailableTerm) {
		t.Fatalf("missing remembered term: %v", err)
	}
	if after := academicSnapshot(t, db); after != before {
		t.Fatal("missing history rejection changed state")
	}
}
