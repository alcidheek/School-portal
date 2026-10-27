package repository

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alcidheek/school-portal/internal/model"
	"github.com/google/uuid"
	"github.com/lib/pq"
)

// Every test owns a connection-local temporary schema. No application table or
// schema_migrations row is changed, even when TEST_DATABASE_URL is configured.
func academicTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to run PostgreSQL lifecycle tests")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	for _, name := range []string{"002_academic_section.sql", "003_terms.sql"} {
		content, err := os.ReadFile(filepath.Join("..", "..", "migrations", name))
		if err != nil {
			t.Fatal(err)
		}
		// Only the original CREATE TABLE becomes temporary; its constraints are unchanged.
		query := "CREATE TEMP TABLE" + string(content[len("CREATE TABLE"):])
		if _, err := db.Exec(query); err != nil {
			t.Fatal(err)
		}
	}
	return db
}

func applyAcademicMigration(db *sql.DB) error {
	content, err := os.ReadFile(filepath.Join("..", "..", "migrations", "012_academic_lifecycle.sql"))
	if err != nil {
		return err
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(string(content)); err != nil {
		return err
	}
	return tx.Commit()
}

func seedAcademic(t *testing.T, db *sql.DB) (uuid.UUID, uuid.UUID, uuid.UUID) {
	t.Helper()
	s1, s2, term := uuid.New(), uuid.New(), uuid.New()
	if _, err := db.Exec(`INSERT INTO academic_sessions (id,name,start_date,end_date,is_active)
		VALUES ($1,'A','2026-01-01','2026-12-31',TRUE), ($2,'B','2026-01-01','2026-12-31',FALSE)`, s1, s2); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO terms (id,academic_session_id,name,start_date,end_date,is_active)
		VALUES ($1,$2,'SECOND_TERM','2026-05-01','2026-08-01',TRUE)`, term, s1); err != nil {
		t.Fatal(err)
	}
	return s1, s2, term
}

func requirePQCode(t *testing.T, err error, code string) {
	t.Helper()
	var pgErr *pq.Error
	if !errors.As(err, &pgErr) || string(pgErr.Code) != code {
		t.Fatalf("want SQLSTATE %s, got %v", code, err)
	}
}

func TestAcademicLifecycleRepository(t *testing.T) {
	db := academicTestDB(t)
	s1, s2, termID := seedAcademic(t, db)
	if err := applyAcademicMigration(db); err != nil {
		t.Fatal(err)
	}
	sessions, terms := NewAcademicSessionRepository(db), NewTermRepository(db)
	ctx := context.Background()
	tx, err := sessions.BeginTx(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	s, err := sessions.GetByIDTx(ctx, tx, s1)
	if err != nil || !s.LastActiveTermID.Valid || s.LastActiveTermID.UUID != termID {
		t.Fatalf("backfill: %v, %v", s, err)
	}
	active, err := sessions.GetActiveTx(ctx, tx)
	if err != nil || active.ID != s1 {
		t.Fatalf("active session: %v, %v", active, err)
	}
	original, err := terms.GetBySessionAndNameTx(ctx, tx, s1, model.SecondTerm)
	if err != nil {
		t.Fatal(err)
	}
	if err := terms.DeactivateAllActiveTx(ctx, tx); err != nil {
		t.Fatal(err)
	}
	if err := terms.ActivateTx(ctx, tx, s2, termID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("cross-session activation: %v", err)
	}
	if err := terms.ActivateTx(ctx, tx, s1, termID); err != nil {
		t.Fatal(err)
	}
	restored, err := terms.GetActiveBySessionIDTx(ctx, tx, s1)
	if err != nil || restored.ID != original.ID || !restored.StartDate.Equal(original.StartDate) || !restored.EndDate.Equal(original.EndDate) {
		t.Fatalf("reactivation changed record: %v, %v", restored, err)
	}
	firstID := uuid.New()
	first := &model.Term{ID: firstID, AcademicSessionID: s1, Name: model.FirstTerm, StartDate: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), EndDate: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)}
	if err := terms.CreateTx(ctx, tx, first); err != nil {
		t.Fatal(err)
	}
	fallback, err := terms.GetFirstBySessionIDTx(ctx, tx, s1)
	if err != nil || fallback.ID != firstID {
		t.Fatalf("academic ordering: %v, %v", fallback, err)
	}
	if err := sessions.SetLastActiveTermTx(ctx, tx, s1, uuid.NullUUID{UUID: firstID, Valid: true}); err != nil {
		t.Fatal(err)
	}
	if err := terms.DeactivateTx(ctx, tx, s1, termID); err != nil {
		t.Fatal(err)
	}
	if err := sessions.DeactivateTx(ctx, tx, s1); err != nil {
		t.Fatal(err)
	}
	if err := terms.ActivateTx(ctx, tx, s1, firstID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("inactive parent activation: %v", err)
	}
	if err := sessions.ActivateTx(ctx, tx, s2); err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	s, err = sessions.GetByID(ctx, s1)
	if err != nil || !s.IsActive || s.LastActiveTermID.UUID != termID {
		t.Fatalf("rollback lost original session: %v, %v", s, err)
	}
	list, err := sessions.List(ctx)
	if err != nil || len(list) != 2 {
		t.Fatalf("list: %v, %v", list, err)
	}
	if _, err := terms.GetByID(ctx, firstID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("rollback retained inserted term: %v", err)
	}
	tx, err = sessions.BeginTx(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	other, err := sessions.GetByIDTx(ctx, tx, s2)
	if err != nil || other.LastActiveTermID.Valid {
		t.Fatalf("nullable remembered term: %v, %v", other, err)
	}
	if _, err := terms.GetFirstBySessionIDTx(ctx, tx, s2); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("empty fallback: %v", err)
	}
	if _, err := terms.GetBySessionAndNameTx(ctx, tx, s1, model.ThirdTerm); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("missing term: %v", err)
	}
	if err := sessions.SetLastActiveTermTx(ctx, tx, uuid.New(), uuid.NullUUID{}); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("missing session update: %v", err)
	}
	if err := sessions.SetLastActiveTermTx(ctx, tx, s1, uuid.NullUUID{}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	s, err = sessions.GetByID(ctx, s1)
	if err != nil || s.LastActiveTermID.Valid {
		t.Fatalf("clear remembered term: %v, %v", s, err)
	}
}

func TestAcademicMigrationEmptyDatabase(t *testing.T) {
	db := academicTestDB(t)
	if err := applyAcademicMigration(db); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	sessions, terms := NewAcademicSessionRepository(db), NewTermRepository(db)
	tx, err := sessions.BeginTx(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)
	s := &model.AcademicSession{ID: uuid.New(), Name: "New", StartDate: start, EndDate: end, IsActive: true}
	if err := sessions.CreateTx(ctx, tx, s); err != nil {
		t.Fatal(err)
	}
	term := &model.Term{ID: uuid.New(), AcademicSessionID: s.ID, Name: model.FirstTerm, StartDate: start, EndDate: end, IsActive: true}
	if err := terms.CreateTx(ctx, tx, term); err != nil {
		t.Fatal(err)
	}
	if err := sessions.SetLastActiveTermTx(ctx, tx, s.ID, uuid.NullUUID{UUID: term.ID, Valid: true}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	stored, err := sessions.GetByID(ctx, s.ID)
	if err != nil || stored.LastActiveTermID.UUID != term.ID {
		t.Fatalf("create with remembered term: %v, %v", stored, err)
	}
}

func TestAcademicLifecycleConstraints(t *testing.T) {
	db := academicTestDB(t)
	s1, s2, termID := seedAcademic(t, db)
	if err := applyAcademicMigration(db); err != nil {
		t.Fatal(err)
	}
	_, err := db.Exec(`UPDATE academic_sessions SET is_active=TRUE WHERE id=$1`, s2)
	requirePQCode(t, err, "23505")
	_, err = db.Exec(`INSERT INTO terms (academic_session_id,name,start_date,end_date,is_active) VALUES ($1,'FIRST_TERM','2026-01-01','2026-04-01',TRUE)`, s1)
	requirePQCode(t, err, "23505")
	ctx := context.Background()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := NewAcademicSessionRepository(db).SetLastActiveTermTx(ctx, tx, s2, uuid.NullUUID{UUID: termID, Valid: true}); err != nil {
		t.Fatal(err)
	}
	requirePQCode(t, tx.Commit(), "23503")
	_, err = db.Exec(`DELETE FROM terms WHERE id=$1`, termID)
	requirePQCode(t, err, "23503")
	if _, err := db.Exec(`DELETE FROM academic_sessions WHERE id=$1`, s1); err != nil {
		t.Fatalf("circular FK blocks cascade: %v", err)
	}
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM terms WHERE academic_session_id=$1`, s1).Scan(&n); err != nil || n != 0 {
		t.Fatalf("cascade: %d, %v", n, err)
	}
}

func TestAcademicMigrationRejectsInconsistentData(t *testing.T) {
	for _, tc := range []struct{ name, query string }{
		{"multiple sessions", `UPDATE academic_sessions SET is_active=TRUE`},
		{"multiple terms", `INSERT INTO terms (academic_session_id,name,start_date,end_date,is_active) SELECT id,'FIRST_TERM','2026-01-01','2026-04-01',TRUE FROM academic_sessions WHERE name='A'`},
		{"inactive parent", `UPDATE academic_sessions SET is_active=FALSE`},
		{"outside dates", `UPDATE terms SET end_date='2027-01-01'`},
		{"no available term", `DELETE FROM terms`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := academicTestDB(t)
			seedAcademic(t, db)
			if _, err := db.Exec(tc.query); err != nil {
				t.Fatal(err)
			}
			var before, after string
			snapshot := `SELECT json_build_array((SELECT json_agg(s ORDER BY id) FROM academic_sessions s),(SELECT json_agg(t ORDER BY id) FROM terms t))::text`
			if err := db.QueryRow(snapshot).Scan(&before); err != nil {
				t.Fatal(err)
			}
			if err := applyAcademicMigration(db); err == nil {
				t.Fatal("inconsistent data accepted")
			}
			if err := db.QueryRow(snapshot).Scan(&after); err != nil {
				t.Fatal(err)
			}
			if before != after {
				t.Fatal("failed migration changed legacy rows")
			}
		})
	}
}
