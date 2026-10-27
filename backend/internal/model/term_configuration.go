package model

import "github.com/google/uuid"

type TermConfiguration struct {
	ResultPeriod
	Locked          bool                 `json:"locked"`
	Students        []EntityReference    `json:"students"`
	Subjects        []EntityReference    `json:"subjects"`
	ClassTeacher    *EntityReference     `json:"class_teacher"`
	SubjectTeachers []TermSubjectTeacher `json:"subject_teachers"`
	Candidates      []TermCandidate      `json:"candidates"`
}
type TermSubjectTeacher struct {
	SubjectID uuid.UUID       `json:"subject_id"`
	Teacher   EntityReference `json:"teacher"`
}
type TermCandidate struct {
	EntityReference
	Role Role `json:"role"`
}
type TermConfigurationChange struct {
	ResultPeriod
	Category  string    `json:"category"`
	Operation string    `json:"operation"`
	StudentID uuid.UUID `json:"student_id"`
	TeacherID uuid.UUID `json:"teacher_id"`
}
type TermInitialization struct {
	AcademicSessionID uuid.UUID `json:"academic_session_id"`
	TermID            uuid.UUID `json:"term_id"`
	SourceTermID      uuid.UUID `json:"source_term_id"`
	Categories        []string  `json:"categories"`
}
