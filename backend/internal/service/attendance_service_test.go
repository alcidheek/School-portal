package service

import (
	"context"
	"errors"
	"github.com/alcidheek/school-portal/internal/model"
	"github.com/google/uuid"
	"testing"
)

type attendanceFake struct {
	teacher, student, session uuid.UUID
	saved                     []model.Attendance
	filter                    model.AttendanceFilter
}

func (f *attendanceFake) CreateBatch(_ context.Context, a []model.Attendance) error {
	f.saved = a
	return nil
}

func (f *attendanceFake) GetUserByID(_ context.Context, id uuid.UUID) (*model.User, error) {
	role := model.RoleTeacher
	if id == f.student {
		role = model.RoleStudent
	}
	return &model.User{ID: id, Role: role, IsActive: true}, nil
}
func (f *attendanceFake) AcademicSessionExists(context.Context, uuid.UUID) (bool, error) {
	return true, nil
}

// Record the lookup arguments so tests can verify student access restrictions.
func (f *attendanceFake) GetByStudent(_ context.Context, student uuid.UUID, q model.AttendanceFilter) ([]model.Attendance, error) {
	q.StudentID = student
	f.filter = q
	return []model.Attendance{}, nil
}

func (f *attendanceFake) GetByClass(_ context.Context, class, session, term uuid.UUID, date string) ([]model.Attendance, error) {
	f.filter = model.AttendanceFilter{ClassID: class, AcademicSessionID: session, TermID: term, Date: date}
	return []model.Attendance{}, nil
}

func (f *attendanceFake) GetForAdmin(_ context.Context, q model.AttendanceFilter) ([]model.Attendance, error) {
	f.filter = q
	return []model.Attendance{}, nil
}
func (f *attendanceFake) GetStudentsByClass(context.Context, uuid.UUID, uuid.UUID, ...uuid.UUID) ([]model.User, error) {
	return []model.User{{ID: f.student, FullName: "Student"}}, nil
}
func (f *attendanceFake) GetClassTeacher(context.Context, uuid.UUID, uuid.UUID, ...uuid.UUID) (*model.ClassTeacherAssignmentDetails, error) {
	return &model.ClassTeacherAssignmentDetails{Teacher: model.TeacherContact{ID: f.teacher}}, nil
}
func (f *attendanceFake) GetTeacherClasses(context.Context, uuid.UUID, uuid.UUID, ...uuid.UUID) ([]model.EntityReference, error) {
	return nil, nil
}
func (f *attendanceFake) GetByID(context.Context, uuid.UUID) (*model.Term, error) {
	return &model.Term{AcademicSessionID: f.session}, nil
}

func TestAttendanceSubmission(t *testing.T) {
	for _, name := range []string{"valid", "unassigned", "unenrolled", "missing student", "duplicate student", "invalid status", "invalid date", "wrong session"} {
		t.Run(name, func(t *testing.T) {
			f := &attendanceFake{teacher: uuid.New(), student: uuid.New(), session: uuid.New()}
			s := NewAttendanceService(f, f, f, f, f)
			teacher := f.teacher
			req := model.AttendanceBatch{ClassID: uuid.New(), AcademicSessionID: f.session, TermID: uuid.New(), Date: "2026-09-10", Students: []model.AttendanceMark{{StudentID: f.student, Status: "PRESENT"}}}
			want := ErrInvalidAttendance
			switch name {
			case "valid":
				want = nil
			case "unassigned":
				teacher = uuid.New()
				want = ErrAttendanceForbidden
			case "unenrolled":
				req.Students = append(req.Students, model.AttendanceMark{StudentID: uuid.New(), Status: "ABSENT"})
			case "missing student":
				req.Students = nil
			case "duplicate student":
				req.Students = append(req.Students, req.Students[0])
			case "invalid status":
				req.Students[0].Status = "LATE"
			case "invalid date":
				req.Date = "2026-02-30"
			case "wrong session":
				req.AcademicSessionID = uuid.New()
			}
			_, err := s.Create(context.Background(), teacher, req)
			if !errors.Is(err, want) {
				t.Fatalf("got %v, want %v", err, want)
			}
			if want != nil && len(f.saved) != 0 {
				t.Fatal("invalid request persisted attendance")
			}
			if want == nil && (len(f.saved) != 1 || f.saved[0].RecordedBy != teacher) {
				t.Fatal("missing attendance or authenticated recorder")
			}
		})
	}
}
func TestAttendanceStudentScope(t *testing.T) {
	student := uuid.New()
	f := &attendanceFake{student: student}
	s := NewAttendanceService(f, f, f, f, f)
	_, err := s.StudentRecords(context.Background(), student, model.AttendanceFilter{StudentID: uuid.New()})
	if err != nil || f.filter.StudentID != student {
		t.Fatal("student filter did not enforce authenticated identity")
	}
	_, err = s.StudentRecords(context.Background(), uuid.Nil, model.AttendanceFilter{})
	if !errors.Is(err, ErrAttendanceForbidden) {
		t.Fatal("missing identity accepted")
	}
}
