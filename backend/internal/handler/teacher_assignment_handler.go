package handler

import (
	"encoding/json"
	"errors"
	"github.com/alcidheek/school-portal/internal/httpresponse"
	"net/http"

	"github.com/alcidheek/school-portal/internal/service"
	"github.com/google/uuid"
)

type TeacherAssignmentHandler struct {
	assignmentService *service.TeacherAssignmentService
}

func NewTeacherAssignmentHandler(
	assignmentService *service.TeacherAssignmentService,
) *TeacherAssignmentHandler {
	return &TeacherAssignmentHandler{assignmentService: assignmentService}
}

func (h *TeacherAssignmentHandler) AssignClassTeacher(w http.ResponseWriter, r *http.Request) {
	var req AssignClassTeacherRequest
	if err := httpresponse.Decode(w, r, &req); err != nil {
		httpresponse.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.TeacherID == uuid.Nil || req.ClassID == uuid.Nil || req.AcademicSessionID == uuid.Nil {
		httpresponse.Error(w, "teacher_id, class_id and academic_session_id are required", http.StatusBadRequest)
		return
	}

	details, err := h.assignmentService.AssignClassTeacher(
		r.Context(), req.TeacherID, req.ClassID, req.AcademicSessionID, req.TermID,
	)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(details)
}

func (h *TeacherAssignmentHandler) AssignSubjectTeacher(w http.ResponseWriter, r *http.Request) {
	var req AssignSubjectTeacherRequest
	if err := httpresponse.Decode(w, r, &req); err != nil {
		httpresponse.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.TeacherID == uuid.Nil || req.SubjectID == uuid.Nil ||
		req.ClassID == uuid.Nil || req.AcademicSessionID == uuid.Nil {
		httpresponse.Error(w, "teacher_id, subject_id, class_id and academic_session_id are required", http.StatusBadRequest)
		return
	}

	details, err := h.assignmentService.AssignSubjectTeacher(
		r.Context(), req.TeacherID, req.SubjectID, req.ClassID, req.AcademicSessionID, req.TermID,
	)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(details)
}

func (h *TeacherAssignmentHandler) GetClassTeacher(w http.ResponseWriter, r *http.Request) {
	classID, sessionID, ok := assignmentPathAndSession(w, r, "class_id")
	if !ok {
		return
	}

	actor, ok := authenticatedUserID(w, r)
	if !ok {
		return
	}
	details, err := h.assignmentService.GetClassTeacherForActor(r.Context(), actor, classID, sessionID, assignmentTerm(r))
	if errors.Is(err, service.ErrClassTeacherNotFound) {
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

func (h *TeacherAssignmentHandler) GetTeacherSubjectAssignments(w http.ResponseWriter, r *http.Request) {
	teacherID, sessionID, ok := assignmentPathAndSession(w, r, "teacher_id")
	if !ok {
		return
	}

	actor, ok := authenticatedUserID(w, r)
	if !ok {
		return
	}
	assignments, err := h.assignmentService.GetTeacherAssignmentsForActor(
		r.Context(), actor, teacherID, sessionID, assignmentTerm(r),
	)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(assignments)
}

func assignmentPathAndSession(
	w http.ResponseWriter,
	r *http.Request,
	pathKey string,
) (uuid.UUID, uuid.UUID, bool) {
	entityID, err := uuid.Parse(r.PathValue(pathKey))
	if err != nil || entityID == uuid.Nil {
		httpresponse.Error(w, "invalid "+pathKey, http.StatusBadRequest)
		return uuid.Nil, uuid.Nil, false
	}
	sessionID, err := uuid.Parse(r.URL.Query().Get("academic_session_id"))
	if err != nil || sessionID == uuid.Nil || len(r.URL.Query()["academic_session_id"]) != 1 {
		httpresponse.Error(w, "valid academic_session_id query parameter is required", http.StatusBadRequest)
		return uuid.Nil, uuid.Nil, false
	}
	return entityID, sessionID, true
}

func (h *TeacherAssignmentHandler) GetMySubjectAssignments(w http.ResponseWriter, r *http.Request) {
	actor, ok := authenticatedUserID(w, r)
	if !ok {
		return
	}
	session, err := uuid.Parse(r.URL.Query().Get("academic_session_id"))
	if err != nil || session == uuid.Nil || len(r.URL.Query()["academic_session_id"]) != 1 {
		httpresponse.Error(w, "valid academic_session_id query parameter is required", 400)
		return
	}
	assignments, err := h.assignmentService.GetTeacherAssignmentsForActor(r.Context(), actor, actor, session, assignmentTerm(r))
	attendanceResponse(w, assignments, err, http.StatusOK)
}
func (h *TeacherAssignmentHandler) GetClassGuardians(w http.ResponseWriter, r *http.Request) {
	actor, ok := authenticatedUserID(w, r)
	if !ok {
		return
	}
	class, session, ok := assignmentPathAndSession(w, r, "class_id")
	if !ok {
		return
	}
	guardians, err := h.assignmentService.GetClassGuardians(r.Context(), actor, class, session, assignmentTerm(r))
	attendanceResponse(w, map[string]any{"students": guardians}, err, http.StatusOK)
}
