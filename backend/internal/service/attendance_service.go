package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/alcidheek/school-portal/internal/model"
	"github.com/google/uuid"
	"time"
)

var ErrAttendanceForbidden = errors.New("teacher is not assigned as class teacher for this academic term")
var ErrInvalidAttendance = errors.New("invalid attendance request")

type attendanceStore interface {
	CreateBatch(context.Context, []model.Attendance) error
	GetByClass(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, string) ([]model.Attendance, error)
	GetByStudent(context.Context, uuid.UUID, model.AttendanceFilter) ([]model.Attendance, error)
	GetForAdmin(context.Context, model.AttendanceFilter) ([]model.Attendance, error)
}
type attendanceEnrollmentStore interface {
	GetStudentsByClass(context.Context, uuid.UUID, uuid.UUID, ...uuid.UUID) ([]model.User, error)
}
type attendanceAssignmentStore interface {
	AcademicSessionExists(context.Context, uuid.UUID) (bool, error)
	GetClassTeacher(context.Context, uuid.UUID, uuid.UUID, ...uuid.UUID) (*model.ClassTeacherAssignmentDetails, error)
	GetTeacherClasses(context.Context, uuid.UUID, uuid.UUID, ...uuid.UUID) ([]model.EntityReference, error)
}
type attendanceTermStore interface {
	GetByID(context.Context, uuid.UUID) (*model.Term, error)
}
type AttendanceService struct {
	records     attendanceStore
	enrollments attendanceEnrollmentStore
	assignments attendanceAssignmentStore
	terms       attendanceTermStore
	users       academicUserStore
}

func NewAttendanceService(r attendanceStore, e attendanceEnrollmentStore, a attendanceAssignmentStore, t attendanceTermStore, users academicUserStore) *AttendanceService {
	return &AttendanceService{r, e, a, t, users}
}
func invalidAttendance(message string) error {
	return fmt.Errorf("%w: %s", ErrInvalidAttendance, message)
}
func (s *AttendanceService) authorize(ctx context.Context, teacher, class, session uuid.UUID, termIDs ...uuid.UUID) error {
	term, termErr := requiredTerm(termIDs)
	if termErr != nil {
		return termErr
	}
	if teacher == uuid.Nil {
		return ErrAttendanceForbidden
	}
	if class == uuid.Nil || session == uuid.Nil {
		return invalidAttendance("class_id and academic_session_id are required")
	}
	if _, err := activeAcademicActor(ctx, s.users, teacher, model.RoleTeacher); err != nil {
		return err
	}
	selected, err := s.terms.GetByID(ctx, term)
	if err != nil {
		return err
	}
	if selected.AcademicSessionID != session {
		return invalidAttendance("term does not belong to academic session")
	}
	assignment, err := s.assignments.GetClassTeacher(ctx, class, session, term)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrAttendanceForbidden
	}
	if err != nil {
		return err
	}
	if assignment.Teacher.ID != teacher {
		return ErrAttendanceForbidden
	}
	return nil
}
func (s *AttendanceService) Roster(ctx context.Context, teacher, class, session uuid.UUID, termIDs ...uuid.UUID) ([]model.EntityReference, error) {
	term, termErr := requiredTerm(termIDs)
	if termErr != nil {
		return nil, termErr
	}
	if err := s.authorize(ctx, teacher, class, session, term); err != nil {
		return nil, err
	}
	students, err := s.enrollments.GetStudentsByClass(ctx, class, session, term)
	if err != nil {
		return nil, err
	}
	roster := make([]model.EntityReference, 0, len(students))
	for _, student := range students {
		roster = append(roster, model.EntityReference{ID: student.ID, Name: student.FullName})
	}
	return roster, nil
}
func (s *AttendanceService) Classes(ctx context.Context, teacher, session uuid.UUID, termIDs ...uuid.UUID) ([]model.EntityReference, error) {
	term, termErr := requiredTerm(termIDs)
	if termErr != nil {
		return nil, termErr
	}
	if teacher == uuid.Nil {
		return nil, ErrAttendanceForbidden
	}
	if session == uuid.Nil {
		return nil, invalidAttendance("academic_session_id is required")
	}
	if _, err := activeAcademicActor(ctx, s.users, teacher, model.RoleTeacher); err != nil {
		return nil, err
	}
	exists, err := s.assignments.AcademicSessionExists(ctx, session)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, ErrNotFound
	}
	selected, err := s.terms.GetByID(ctx, term)
	if err != nil {
		return nil, err
	}
	if selected.AcademicSessionID != session {
		return nil, invalidAttendance("term does not belong to academic session")
	}
	classes, err := s.assignments.GetTeacherClasses(ctx, teacher, session, term)
	if err != nil {
		return nil, err
	}
	if classes == nil {
		classes = make([]model.EntityReference, 0)
	}
	return classes, nil
}
func (s *AttendanceService) Create(ctx context.Context, teacher uuid.UUID, req model.AttendanceBatch) ([]model.Attendance, error) {
	if req.TermID == uuid.Nil {
		return nil, invalidAttendance("term_id is required")
	}
	if _, err := time.Parse(time.DateOnly, req.Date); err != nil {
		return nil, invalidAttendance("date must be YYYY-MM-DD")
	}
	if len(req.Students) == 0 {
		return nil, invalidAttendance("students must not be empty")
	}
	seen := make(map[uuid.UUID]bool, len(req.Students))
	for _, entry := range req.Students {
		if entry.StudentID == uuid.Nil || seen[entry.StudentID] {
			return nil, invalidAttendance("student IDs must be valid and unique")
		}
		if entry.Status != "PRESENT" && entry.Status != "ABSENT" {
			return nil, invalidAttendance("status must be PRESENT or ABSENT")
		}
		seen[entry.StudentID] = true
	}
	roster, err := s.Roster(ctx, teacher, req.ClassID, req.AcademicSessionID, req.TermID)
	if err != nil {
		return nil, err
	}
	// Require exactly the enrolled roster so a class cannot be partially recorded.
	for _, student := range roster {
		if !seen[student.ID] {
			return nil, invalidAttendance("submit every enrolled student exactly once")
		}
	}
	if len(roster) != len(seen) {
		return nil, invalidAttendance("every submitted student must be enrolled in this class/session")
	}
	term, err := s.terms.GetByID(ctx, req.TermID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: term not found", ErrNotFound)
	}
	if err != nil {
		return nil, err
	}
	if term.AcademicSessionID != req.AcademicSessionID {
		return nil, invalidAttendance("term does not belong to academic session")
	}
	records := make([]model.Attendance, 0, len(req.Students))
	for _, entry := range req.Students {
		records = append(records, model.Attendance{ID: uuid.New(), StudentID: entry.StudentID, ClassID: req.ClassID, AcademicSessionID: req.AcademicSessionID, TermID: req.TermID, Date: req.Date, Status: entry.Status, RecordedBy: teacher})
	}
	if err := s.records.CreateBatch(ctx, records); err != nil {
		return nil, err
	}
	return records, nil
}

// TeacherRecords checks the assignment before fetching a class register.
func (s *AttendanceService) TeacherRecords(ctx context.Context, teacher uuid.UUID, f model.AttendanceFilter) ([]model.Attendance, error) {
	if err := s.authorize(ctx, teacher, f.ClassID, f.AcademicSessionID, f.TermID); err != nil {
		return nil, err
	}
	if f.TermID == uuid.Nil || f.Date == "" {
		return nil, invalidAttendance("term_id and date are required")
	}
	term, err := s.terms.GetByID(ctx, f.TermID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if term.AcademicSessionID != f.AcademicSessionID {
		return nil, invalidAttendance("term does not belong to academic session")
	}
	records, err := s.records.GetByClass(ctx, f.ClassID, f.AcademicSessionID, f.TermID, f.Date)
	if err != nil {
		return nil, err
	}
	// Optionally show one student from this class register.
	if f.StudentID != uuid.Nil {
		filtered := make([]model.Attendance, 0)
		for _, record := range records {
			if record.StudentID == f.StudentID {
				filtered = append(filtered, record)
			}
		}
		return filtered, nil
	}
	return records, nil
}

// StudentRecords reads attendance only for the authenticated student.
func (s *AttendanceService) StudentRecords(ctx context.Context, student uuid.UUID, f model.AttendanceFilter) ([]model.Attendance, error) {
	if student == uuid.Nil {
		return nil, ErrAttendanceForbidden
	}
	// Always override client filters with the authenticated student's identity.
	if _, err := activeAcademicActor(ctx, s.users, student, model.RoleStudent); err != nil {
		return nil, err
	}
	f.StudentID = student
	return s.records.GetByStudent(ctx, student, f)
}

// AdminRecords allows school-wide viewing with optional filters.
func (s *AttendanceService) AdminRecords(ctx context.Context, f model.AttendanceFilter) ([]model.Attendance, error) {
	return s.records.GetForAdmin(ctx, f)
}

func (s *AttendanceService) AdminRecordsForActor(ctx context.Context, actor uuid.UUID, f model.AttendanceFilter) ([]model.Attendance, error) {
	if _, err := activeAcademicActor(ctx, s.users, actor, model.RoleAdmin); err != nil {
		return nil, err
	}
	return s.AdminRecords(ctx, f)
}
