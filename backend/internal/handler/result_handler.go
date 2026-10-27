package handler

import (
	"encoding/json"
	"errors"
	"github.com/alcidheek/school-portal/internal/httpresponse"
	"net/http"

	"github.com/alcidheek/school-portal/internal/middleware"
	"github.com/alcidheek/school-portal/internal/service"
	"github.com/google/uuid"
)

type ResultHandler struct {
	resultService *service.ResultService
}

func NewResultHandler(resultService *service.ResultService) *ResultHandler {
	return &ResultHandler{resultService: resultService}
}

func (h *ResultHandler) Create(w http.ResponseWriter, r *http.Request) {
	teacherIDString, ok := r.Context().Value(middleware.UserIDKey).(string)
	if !ok {
		httpresponse.Error(w, "teacher identity not found", http.StatusUnauthorized)
		return
	}
	teacherID, err := uuid.Parse(teacherIDString)
	if err != nil {
		httpresponse.Error(w, "invalid teacher identity", http.StatusUnauthorized)
		return
	}

	var req CreateResultRequest
	if err := httpresponse.Decode(w, r, &req); err != nil {
		httpresponse.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.StudentID == uuid.Nil || req.SubjectID == uuid.Nil ||
		req.ClassID == uuid.Nil || req.AcademicSessionID == uuid.Nil || req.TermID == uuid.Nil {
		httpresponse.Error(w, "student_id, subject_id, class_id, academic_session_id and term_id are required", http.StatusBadRequest)
		return
	}

	result, err := h.resultService.Create(r.Context(), teacherID, service.CreateResultRequest{
		StudentID:         req.StudentID,
		SubjectID:         req.SubjectID,
		ClassID:           req.ClassID,
		AcademicSessionID: req.AcademicSessionID,
		TermID:            req.TermID,
		CAScore:           req.CAScore,
		ExamScore:         req.ExamScore,
	})
	if errors.Is(err, service.ErrDuplicateResult) {
		writeServiceError(w, err)
		return
	}
	if err != nil {
		writeServiceError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(result)
}

func (h *ResultHandler) GetTeacherResults(w http.ResponseWriter, r *http.Request) {
	teacherIDString, ok := r.Context().Value(middleware.UserIDKey).(string)
	if !ok {
		httpresponse.Error(w, "teacher identity not found", http.StatusUnauthorized)
		return
	}
	teacherID, err := uuid.Parse(teacherIDString)
	if err != nil || teacherID == uuid.Nil {
		httpresponse.Error(w, "invalid teacher identity", http.StatusUnauthorized)
		return
	}

	query := r.URL.Query()
	classID, err := uuid.Parse(query.Get("class_id"))
	if err != nil || classID == uuid.Nil {
		httpresponse.Error(w, "valid class_id query parameter is required", http.StatusBadRequest)
		return
	}
	subjectID, err := uuid.Parse(query.Get("subject_id"))
	if err != nil || subjectID == uuid.Nil {
		httpresponse.Error(w, "valid subject_id query parameter is required", http.StatusBadRequest)
		return
	}
	academicSessionID, err := uuid.Parse(query.Get("academic_session_id"))
	if err != nil || academicSessionID == uuid.Nil {
		httpresponse.Error(w, "valid academic_session_id query parameter is required", http.StatusBadRequest)
		return
	}
	termID, err := uuid.Parse(query.Get("term_id"))
	if err != nil || termID == uuid.Nil {
		httpresponse.Error(w, "valid term_id query parameter is required", http.StatusBadRequest)
		return
	}

	results, err := h.resultService.GetTeacherResults(
		r.Context(), teacherID, classID, subjectID, academicSessionID, termID,
	)
	if errors.Is(err, service.ErrTeacherNotAssigned) {
		writeServiceError(w, err)
		return
	}
	if err != nil {
		writeServiceError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"results": results,
	})
}

func (h *ResultHandler) GetMyResults(w http.ResponseWriter, r *http.Request) {
	studentIDString, ok := r.Context().Value(middleware.UserIDKey).(string)
	if !ok {
		httpresponse.Error(w, "student identity not found", http.StatusUnauthorized)
		return
	}
	studentID, err := uuid.Parse(studentIDString)
	if err != nil {
		httpresponse.Error(w, "invalid student identity", http.StatusUnauthorized)
		return
	}

	academicSessionID, err := uuid.Parse(r.URL.Query().Get("academic_session_id"))
	if err != nil {
		httpresponse.Error(w, "valid academic_session_id query parameter is required", http.StatusBadRequest)
		return
	}
	termID, err := uuid.Parse(r.URL.Query().Get("term_id"))
	if err != nil {
		httpresponse.Error(w, "valid term_id query parameter is required", http.StatusBadRequest)
		return
	}

	results, err := h.resultService.GetStudentResults(
		r.Context(),
		studentID,
		academicSessionID,
		termID,
	)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"results": results,
	})
}
