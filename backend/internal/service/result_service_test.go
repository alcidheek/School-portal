package service

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/alcidheek/school-portal/internal/model"
	"github.com/alcidheek/school-portal/internal/repository"
	"github.com/google/uuid"
)

// A scripted SQL connection exercises the real repositories without a database
// server or additional dependencies. Unexpected queries fail the test.
type resultQueryStep struct {
	contains string
	args     []driver.Value
	rows     [][]driver.Value
	err      error
}

type resultTestDB struct {
	t     *testing.T
	steps []resultQueryStep
}

func (d *resultTestDB) Connect(context.Context) (driver.Conn, error) { return d, nil }
func (d *resultTestDB) Driver() driver.Driver                        { return d }
func (d *resultTestDB) Open(string) (driver.Conn, error)             { return d, nil }
func (d *resultTestDB) Close() error                                 { return nil }
func (d *resultTestDB) Begin() (driver.Tx, error) {
	return nil, errors.New("unexpected transaction")
}
func (d *resultTestDB) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("unexpected prepared statement")
}
func (d *resultTestDB) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	d.t.Helper()
	if len(d.steps) == 0 {
		d.t.Errorf("unexpected query: %s", query)
		return nil, errors.New("unexpected query")
	}
	step := d.steps[0]
	d.steps = d.steps[1:]
	if !strings.Contains(strings.Join(strings.Fields(query), " "), step.contains) {
		d.t.Errorf("query %q does not contain %q", query, step.contains)
	}
	got := make([]driver.Value, len(args))
	for i, arg := range args {
		got[i] = arg.Value
	}
	if !reflect.DeepEqual(got, step.args) {
		d.t.Errorf("query arguments = %v, want %v", got, step.args)
	}
	if step.err != nil {
		return nil, step.err
	}
	return &resultTestRows{rows: step.rows}, nil
}

type resultTestRows struct {
	rows [][]driver.Value
}

func (r *resultTestRows) Columns() []string {
	count := 8 // Term lookup with no rows still has eight columns.
	if len(r.rows) > 0 {
		count = len(r.rows[0])
	}
	columns := make([]string, count)
	for i := range columns {
		columns[i] = fmt.Sprintf("column_%d", i)
	}
	return columns
}
func (r *resultTestRows) Close() error { return nil }
func (r *resultTestRows) Next(dest []driver.Value) error {
	if len(r.rows) == 0 {
		return io.EOF
	}
	copy(dest, r.rows[0])
	r.rows = r.rows[1:]
	return nil
}

func TestGetTeacherResults(t *testing.T) {
	teacherID, classID, subjectID := uuid.New(), uuid.New(), uuid.New()
	sessionID, termID, studentID, resultID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	now := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	dbErr := errors.New("database unavailable")
	assignment := resultQueryStep{
		contains: "WHERE teacher_id = $1 AND subject_id = $2 AND class_id = $3 AND academic_session_id = $4",
		args:     []driver.Value{teacherID.String(), subjectID.String(), classID.String(), sessionID.String(), termID.String()},
		rows:     [][]driver.Value{{true}},
	}
	term := resultQueryStep{
		contains: "FROM terms WHERE id = $1",
		args:     []driver.Value{termID.String()},
		rows:     [][]driver.Value{{termID.String(), sessionID.String(), "FIRST_TERM", now, now, true, now, now}},
	}
	results := resultQueryStep{
		contains: "WHERE r.class_id = $1 AND r.subject_id = $2 AND r.academic_session_id = $3 AND r.term_id = $4 AND EXISTS",
		args:     []driver.Value{classID.String(), subjectID.String(), sessionID.String(), termID.String(), teacherID.String()},
		rows:     [][]driver.Value{{resultID.String(), studentID.String(), "Ada Student", subjectID.String(), "Mathematics", classID.String(), "JSS 1", sessionID.String(), "2026/2027", termID.String(), "FIRST_TERM", float64(30), float64(50), float64(80), "A", "Excellent", now}},
	}
	denied := assignment
	denied.rows = [][]driver.Value{{false}}
	assignmentFailure := assignment
	assignmentFailure.err = dbErr
	missingTerm := term
	missingTerm.rows = nil
	wrongSession := term
	wrongSession.rows = [][]driver.Value{{termID.String(), uuid.New().String(), "FIRST_TERM", now, now, true, now, now}}
	termFailure := term
	termFailure.err = dbErr
	resultFailure := results
	resultFailure.err = dbErr
	emptyResults := results
	emptyResults.rows = nil

	for _, tc := range []struct {
		name    string
		steps   []resultQueryStep
		wantErr string
		wantIs  error
		count   int
	}{
		{name: "assigned teacher receives results", steps: []resultQueryStep{assignment, term, results}, count: 1},
		{name: "unassigned teacher cannot query results", steps: []resultQueryStep{denied}, wantIs: ErrTeacherNotAssigned},
		{name: "assignment failure stops retrieval", steps: []resultQueryStep{assignmentFailure}, wantIs: dbErr},
		{name: "missing term", steps: []resultQueryStep{assignment, missingTerm}, wantErr: "term not found", wantIs: ErrNotFound},
		{name: "term belongs to another session", steps: []resultQueryStep{assignment, wrongSession}, wantErr: "term does not belong to the academic session", wantIs: ErrInvalidInput},
		{name: "term lookup failure", steps: []resultQueryStep{assignment, termFailure}, wantIs: dbErr},
		{name: "results lookup failure", steps: []resultQueryStep{assignment, term, resultFailure}, wantIs: dbErr},
		{name: "no recorded results returns empty list", steps: []resultQueryStep{assignment, term, emptyResults}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			actor := resultQueryStep{contains: "FROM users WHERE id = $1", args: []driver.Value{teacherID.String()}, rows: [][]driver.Value{{teacherID.String(), "Teacher", "teacher@example.com", "1234567", "unused", "TEACHER", nil, nil, nil, nil, true, now, now}}}
			connector := &resultTestDB{t: t, steps: append([]resultQueryStep{actor}, tc.steps...)}
			db := sql.OpenDB(connector)
			defer db.Close()
			s := &ResultService{
				userRepo:       repository.NewUserRepository(db),
				assignmentRepo: repository.NewTeacherAssignmentRepository(db),
				termRepo:       repository.NewTermRepository(db),
				resultRepo:     repository.NewResultRepository(db),
			}
			got, err := s.GetTeacherResults(context.Background(), teacherID, classID, subjectID, sessionID, termID)
			if len(connector.steps) != 0 {
				t.Errorf("%d expected queries were not executed", len(connector.steps))
			}
			if tc.wantIs != nil || tc.wantErr != "" {
				if tc.wantIs != nil && !errors.Is(err, tc.wantIs) {
					t.Errorf("error = %v, want %v", err, tc.wantIs)
				}
				if tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)) {
					t.Errorf("error = %v, want %q", err, tc.wantErr)
				}
				if got != nil {
					t.Errorf("expected no results on failure, got %v", got)
				}
				return
			}
			if err != nil || got == nil || len(got) != tc.count {
				t.Fatalf("results = %v, error = %v; want non-nil list of %d results", got, err, tc.count)
			}
			if tc.count == 1 {
				want := model.ResultDetails{
					ID:              resultID,
					Student:         model.EntityReference{ID: studentID, Name: "Ada Student"},
					Subject:         model.EntityReference{ID: subjectID, Name: "Mathematics"},
					Class:           model.EntityReference{ID: classID, Name: "JSS 1"},
					AcademicSession: model.EntityReference{ID: sessionID, Name: "2026/2027"},
					Term:            model.EntityReference{ID: termID, Name: "FIRST_TERM"},
					CAScore:         30, ExamScore: 50, TotalScore: 80, Grade: "A", Remark: "Excellent", CreatedAt: now,
				}
				if !reflect.DeepEqual(got[0], want) {
					t.Errorf("result = %+v, want %+v", got[0], want)
				}
			}
		})
	}
}

func TestGetTeacherResultsRequiresIDs(t *testing.T) {
	for index, name := range []string{"teacher", "class", "subject", "session", "term"} {
		t.Run(name, func(t *testing.T) {
			ids := []uuid.UUID{uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()}
			ids[index] = uuid.Nil
			// No repositories: validation must reject the request before database access.
			s := &ResultService{}
			got, err := s.GetTeacherResults(context.Background(), ids[0], ids[1], ids[2], ids[3], ids[4])
			if err == nil || got != nil {
				t.Fatalf("results = %v, error = %v; want validation failure", got, err)
			}
			if index == 0 && !errors.Is(err, ErrTeacherNotAssigned) {
				t.Errorf("error = %v, want %v", err, ErrTeacherNotAssigned)
			}
		})
	}
}

func TestCalculateGrade(t *testing.T) {
	tests := []struct {
		name       string
		total      float64
		wantGrade  string
		wantRemark string
	}{
		{name: "maximum score", total: 100, wantGrade: "A", wantRemark: "Excellent"},
		{name: "A boundary", total: 70, wantGrade: "A", wantRemark: "Excellent"},
		{name: "B boundary", total: 60, wantGrade: "B", wantRemark: "Very Good"},
		{name: "C boundary", total: 50, wantGrade: "C", wantRemark: "Good"},
		{name: "D boundary", total: 45, wantGrade: "D", wantRemark: "Pass"},
		{name: "E boundary", total: 40, wantGrade: "E", wantRemark: "Fair"},
		{name: "F boundary", total: 39, wantGrade: "F", wantRemark: "Fail"},
		{name: "minimum score", total: 0, wantGrade: "F", wantRemark: "Fail"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			grade, remark := calculateGrade(tt.total)
			if grade != tt.wantGrade || remark != tt.wantRemark {
				t.Fatalf(
					"calculateGrade(%v) = (%q, %q), want (%q, %q)",
					tt.total, grade, remark, tt.wantGrade, tt.wantRemark,
				)
			}
		})
	}
}
