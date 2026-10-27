package repository

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"github.com/alcidheek/school-portal/internal/model"
	"github.com/lib/pq"
	"io"
	"strings"
	"testing"
	"time"
)

// Exercise transaction outcomes through database/sql without a running server.
type attendanceDB struct {
	inserts               int
	failure               error
	committed, rolledBack bool
}

func (d *attendanceDB) Connect(context.Context) (driver.Conn, error) { return d, nil }
func (d *attendanceDB) Driver() driver.Driver                        { return d }
func (d *attendanceDB) Open(string) (driver.Conn, error)             { return d, nil }
func (d *attendanceDB) Close() error                                 { return nil }
func (d *attendanceDB) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("unexpected prepare")
}
func (d *attendanceDB) Begin() (driver.Tx, error) { return d, nil }
func (d *attendanceDB) Commit() error             { d.committed = true; return nil }
func (d *attendanceDB) Rollback() error           { d.rolledBack = true; return nil }
func (d *attendanceDB) ExecContext(context.Context, string, []driver.NamedValue) (driver.Result, error) {
	return driver.RowsAffected(1), nil
}
func (d *attendanceDB) QueryContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	if strings.Contains(query, "count(*)") {
		return &attendanceCountRows{}, nil
	}
	d.inserts++
	if d.inserts == 2 && d.failure != nil {
		return nil, d.failure
	}
	return &attendanceRows{}, nil
}

type attendanceCountRows struct{ done bool }

func (r *attendanceCountRows) Columns() []string { return []string{"count"} }
func (r *attendanceCountRows) Close() error      { return nil }
func (r *attendanceCountRows) Next(v []driver.Value) error {
	if r.done {
		return io.EOF
	}
	r.done = true
	v[0] = int64(2)
	return nil
}

type attendanceRows struct{ done bool }

func (r *attendanceRows) Columns() []string { return []string{"created_at", "updated_at"} }
func (r *attendanceRows) Close() error      { return nil }
func (r *attendanceRows) Next(values []driver.Value) error {
	if r.done {
		return io.EOF
	}
	r.done = true
	values[0] = time.Now()
	values[1] = time.Now()
	return nil
}
func TestAttendanceBatchTransaction(t *testing.T) {
	for _, name := range []string{"commit", "duplicate", "insert failure"} {
		t.Run(name, func(t *testing.T) {
			d := &attendanceDB{}
			switch name {
			case "duplicate":
				d.failure = &pq.Error{Code: "23505", Constraint: "unique_student_attendance_date"}
			case "insert failure":
				d.failure = errors.New("insert failed")
			}
			db := sql.OpenDB(d)
			defer db.Close()
			err := NewAttendanceRepository(db).CreateBatch(context.Background(), make([]model.Attendance, 2))
			if name == "commit" {
				if err != nil || !d.committed || d.rolledBack {
					t.Fatalf("unexpected commit outcome: %v", err)
				}
			} else {
				if err == nil || d.committed || !d.rolledBack {
					t.Fatalf("failed insert did not roll back: %v", err)
				}
				if name == "duplicate" && !errors.Is(err, ErrDuplicateAttendance) {
					t.Fatalf("duplicate not mapped: %v", err)
				}
			}
		})
	}
}
