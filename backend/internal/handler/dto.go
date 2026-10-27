package handler

import (
	"github.com/alcidheek/school-portal/internal/model"
	"github.com/google/uuid"
)

type RegisterUserRequest struct {
	FullName      string     `json:"full_name"`
	Email         string     `json:"email"`
	Phone         string     `json:"phone"`
	Password      string     `json:"password"`
	Role          model.Role `json:"role"`
	GuardianName  string     `json:"guardian_name"`
	GuardianPhone string     `json:"guardian_phone"`
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type CreateAttendanceRequest struct {
	ClassID           uuid.UUID         `json:"class_id"`
	AcademicSessionID uuid.UUID         `json:"academic_session_id"`
	TermID            uuid.UUID         `json:"term_id"`
	Date              string            `json:"date"`
	Students          []AttendanceEntry `json:"students"`
}

type AttendanceEntry struct {
	StudentID uuid.UUID `json:"student_id"`
	Status    string    `json:"status"`
}

type CreateAcademicSessionRequest struct {
	// Academic session details.
	Name      string `json:"name"`
	StartDate string `json:"start_date"`
	EndDate   string `json:"end_date"`
	Activate  bool   `json:"activate"`

	// Current term details to create with the session.
	TermName      string `json:"term_name"`
	TermStartDate string `json:"term_start_date"`
	TermEndDate   string `json:"term_end_date"`
}

// CreateClassRequest represents the data required to create a class.
type CreateClassRequest struct {
	Name  string `json:"name"`
	Level string `json:"level"`
}

type CreateSubjectRequest struct {
	Name string `json:"name"`
}

type EnrollStudentRequest struct {
	TermID            uuid.UUID `json:"term_id"`
	StudentID         uuid.UUID `json:"student_id"`
	ClassID           uuid.UUID `json:"class_id"`
	AcademicSessionID uuid.UUID `json:"academic_session_id"`
}

type AssignClassTeacherRequest struct {
	TermID            uuid.UUID `json:"term_id"`
	TeacherID         uuid.UUID `json:"teacher_id"`
	ClassID           uuid.UUID `json:"class_id"`
	AcademicSessionID uuid.UUID `json:"academic_session_id"`
}

type AssignSubjectTeacherRequest struct {
	TermID            uuid.UUID `json:"term_id"`
	TeacherID         uuid.UUID `json:"teacher_id"`
	SubjectID         uuid.UUID `json:"subject_id"`
	ClassID           uuid.UUID `json:"class_id"`
	AcademicSessionID uuid.UUID `json:"academic_session_id"`
}

type CreateResultRequest struct {
	StudentID         uuid.UUID `json:"student_id"`
	SubjectID         uuid.UUID `json:"subject_id"`
	ClassID           uuid.UUID `json:"class_id"`
	AcademicSessionID uuid.UUID `json:"academic_session_id"`
	TermID            uuid.UUID `json:"term_id"`
	CAScore           float64   `json:"ca_score"`
	ExamScore         float64   `json:"exam_score"`
}
