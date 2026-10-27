package handler

import (
	"encoding/json"
	"errors"
	"github.com/alcidheek/school-portal/internal/httpresponse"
	"net/http"

	"github.com/alcidheek/school-portal/internal/service"
	"github.com/google/uuid"
)

type StudentEnrollmentHandler struct {
	enrollmentService *service.StudentEnrollmentService
}

func NewStudentEnrollmentHandler(
	enrollmentService *service.StudentEnrollmentService,
) *StudentEnrollmentHandler {
	return &StudentEnrollmentHandler{enrollmentService: enrollmentService}
}

func (h *StudentEnrollmentHandler) EnrollStudent(w http.ResponseWriter, r *http.Request) {
	var req EnrollStudentRequest
	if err := httpresponse.Decode(w, r, &req); err != nil {
		httpresponse.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.StudentID == uuid.Nil || req.ClassID == uuid.Nil || req.AcademicSessionID == uuid.Nil {
		httpresponse.Error(w, "student_id, class_id and academic_session_id are required", http.StatusBadRequest)
		return
	}

	enrollment, err := h.enrollmentService.EnrollStudent(
		r.Context(),
		req.StudentID,
		req.ClassID,
		req.AcademicSessionID, req.TermID,
	)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(enrollment)
}

func (h *StudentEnrollmentHandler) GetStudentsByClass(w http.ResponseWriter, r *http.Request) {
	classID, err := uuid.Parse(r.PathValue("class_id"))
	if err != nil {
		httpresponse.Error(w, "invalid class id", http.StatusBadRequest)
		return
	}

	academicSessionID, err := uuid.Parse(r.URL.Query().Get("academic_session_id"))
	if err != nil || academicSessionID == uuid.Nil || len(r.URL.Query()["academic_session_id"]) != 1 {
		httpresponse.Error(w, "valid academic_session_id query parameter is required", http.StatusBadRequest)
		return
	}

	students, err := h.enrollmentService.GetStudentsByClass(
		r.Context(),
		classID,
		academicSessionID, assignmentTerm(r),
	)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(students)
}

func (h *StudentEnrollmentHandler) GetStudentEnrollment(w http.ResponseWriter, r *http.Request) {
	studentID, err := uuid.Parse(r.PathValue("student_id"))
	if err != nil {
		httpresponse.Error(w, "invalid student id", http.StatusBadRequest)
		return
	}

	academicSessionID, err := uuid.Parse(r.URL.Query().Get("academic_session_id"))
	if err != nil || academicSessionID == uuid.Nil || len(r.URL.Query()["academic_session_id"]) != 1 {
		httpresponse.Error(w, "valid academic_session_id query parameter is required", http.StatusBadRequest)
		return
	}

	details, err := h.enrollmentService.GetStudentEnrollment(
		r.Context(),
		studentID,
		academicSessionID, assignmentTerm(r),
	)
	if errors.Is(err, service.ErrStudentEnrollmentNotFound) {
		writeServiceError(w, err)
		return
	}
	if err != nil {
		writeServiceError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(details)
}
