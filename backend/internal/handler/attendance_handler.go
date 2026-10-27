package handler

import (
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/alcidheek/school-portal/internal/httpresponse"
	"github.com/alcidheek/school-portal/internal/middleware"
	"github.com/alcidheek/school-portal/internal/model"
	"github.com/alcidheek/school-portal/internal/service"
	"github.com/google/uuid"
)

type AttendanceHandler struct{ service *service.AttendanceService }

func NewAttendanceHandler(s *service.AttendanceService) *AttendanceHandler {
	return &AttendanceHandler{service: s}
}
func attendanceIdentity(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	raw, _ := r.Context().Value(middleware.UserIDKey).(string)
	id, err := uuid.Parse(raw)
	if err != nil || id == uuid.Nil {
		httpresponse.Error(w, "invalid authenticated identity", http.StatusUnauthorized)
		return uuid.Nil, false
	}
	return id, true
}
func attendanceResponse(w http.ResponseWriter, value any, err error, status int) {
	if err != nil {
		writeServiceError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(value)
}
func attendanceFilter(r *http.Request) (model.AttendanceFilter, error) {
	var f model.AttendanceFilter
	for _, field := range []struct {
		name   string
		target *uuid.UUID
	}{{"class_id", &f.ClassID}, {"student_id", &f.StudentID}, {"academic_session_id", &f.AcademicSessionID}, {"term_id", &f.TermID}} {
		if raw, exists := r.URL.Query()[field.name]; exists {
			if len(raw) != 1 {
				return f, service.ErrInvalidAttendance
			}
			id, err := uuid.Parse(raw[0])
			if err != nil || id == uuid.Nil {
				return f, service.ErrInvalidAttendance
			}
			*field.target = id
		}
	}
	if raw, exists := r.URL.Query()["date"]; exists {
		if len(raw) != 1 {
			return f, service.ErrInvalidAttendance
		}
		if _, err := time.Parse(time.DateOnly, raw[0]); err != nil {
			return f, service.ErrInvalidAttendance
		}
		f.Date = raw[0]
	}
	return f, nil
}
func (h *AttendanceHandler) Create(w http.ResponseWriter, r *http.Request) {
	teacher, ok := attendanceIdentity(w, r)
	if !ok {
		return
	}
	var req CreateAttendanceRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		httpresponse.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		httpresponse.Error(w, "request must contain one JSON object", http.StatusBadRequest)
		return
	}
	marks := make([]model.AttendanceMark, len(req.Students))
	for i, entry := range req.Students {
		marks[i] = model.AttendanceMark{StudentID: entry.StudentID, Status: entry.Status}
	}
	records, err := h.service.Create(r.Context(), teacher, model.AttendanceBatch{
		ClassID: req.ClassID, AcademicSessionID: req.AcademicSessionID,
		TermID: req.TermID, Date: req.Date, Students: marks,
	})
	attendanceResponse(w, map[string]any{"attendance": records}, err, http.StatusCreated)
}
func (h *AttendanceHandler) GetTeacher(w http.ResponseWriter, r *http.Request) {
	id, ok := attendanceIdentity(w, r)
	if !ok {
		return
	}
	f, err := attendanceFilter(r)
	if err != nil {
		attendanceResponse(w, nil, err, 0)
		return
	}
	records, err := h.service.TeacherRecords(r.Context(), id, f)
	attendanceResponse(w, map[string]any{"attendance": records}, err, http.StatusOK)
}
func (h *AttendanceHandler) GetStudent(w http.ResponseWriter, r *http.Request) {
	id, ok := attendanceIdentity(w, r)
	if !ok {
		return
	}
	f, err := attendanceFilter(r)
	if err != nil {
		attendanceResponse(w, nil, err, 0)
		return
	}
	records, err := h.service.StudentRecords(r.Context(), id, f)
	attendanceResponse(w, map[string]any{"attendance": records}, err, http.StatusOK)
}
func (h *AttendanceHandler) GetAdmin(w http.ResponseWriter, r *http.Request) {
	id, ok := attendanceIdentity(w, r)
	if !ok {
		return
	}
	f, err := attendanceFilter(r)
	if err != nil {
		attendanceResponse(w, nil, err, 0)
		return
	}
	records, err := h.service.AdminRecordsForActor(r.Context(), id, f)
	attendanceResponse(w, map[string]any{"attendance": records}, err, http.StatusOK)
}
func (h *AttendanceHandler) GetRoster(w http.ResponseWriter, r *http.Request) {
	id, ok := attendanceIdentity(w, r)
	if !ok {
		return
	}
	f, err := attendanceFilter(r)
	if err != nil {
		attendanceResponse(w, nil, err, 0)
		return
	}
	class, err := uuid.Parse(r.PathValue("class_id"))
	if err != nil {
		attendanceResponse(w, nil, service.ErrInvalidAttendance, 0)
		return
	}
	students, err := h.service.Roster(r.Context(), id, class, f.AcademicSessionID, f.TermID)
	attendanceResponse(w, map[string]any{"students": students}, err, http.StatusOK)
}
func (h *AttendanceHandler) GetClasses(w http.ResponseWriter, r *http.Request) {
	id, ok := attendanceIdentity(w, r)
	if !ok {
		return
	}
	f, err := attendanceFilter(r)
	if err != nil {
		attendanceResponse(w, nil, err, 0)
		return
	}
	classes, err := h.service.Classes(r.Context(), id, f.AcademicSessionID, f.TermID)
	attendanceResponse(w, map[string]any{"classes": classes}, err, http.StatusOK)
}
