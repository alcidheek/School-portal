package handler

import (
	"github.com/alcidheek/school-portal/internal/httpresponse"
	"github.com/alcidheek/school-portal/internal/model"
	"github.com/alcidheek/school-portal/internal/service"
	"github.com/google/uuid"
	"net/http"
)

func (h *ResultHandler) TermConfiguration(w http.ResponseWriter, r *http.Request) {
	actor, ok := resultActor(w, r)
	if !ok {
		return
	}
	p, err := resultQuery(r)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	var change *model.TermConfigurationChange
	if r.Method != "GET" {
		change = &model.TermConfigurationChange{}
		if httpresponse.DecodeStrict(w, r, change) != nil {
			httpresponse.Error(w, "invalid request body", 400)
			return
		}
		p = change.ResultPeriod
	}
	if raw := r.PathValue("class_id"); raw != "" {
		p.ClassID, err = uuid.Parse(raw)
		if err != nil {
			writeServiceError(w, service.ErrInvalidInput)
			return
		}
		if change != nil {
			change.ResultPeriod = p
		}
	}
	v, err := h.resultService.TermConfiguration(r.Context(), actor, p, change)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	resultJSON(w, v)
}
func (h *ResultHandler) InitializeTerm(w http.ResponseWriter, r *http.Request) {
	actor, ok := resultActor(w, r)
	if !ok {
		return
	}
	var req model.TermInitialization
	if httpresponse.DecodeStrict(w, r, &req) != nil {
		httpresponse.Error(w, "invalid request body", 400)
		return
	}
	copied, err := h.resultService.InitializeTerm(r.Context(), actor, req)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	resultJSON(w, map[string]any{"copied": copied})
}

// Compatibility creation routes use the same audited transaction as the new UI.
func (h *ResultHandler) EnrollTermStudent(w http.ResponseWriter, r *http.Request) {
	var req EnrollStudentRequest
	if httpresponse.DecodeStrict(w, r, &req) != nil {
		httpresponse.Error(w, "invalid request body", 400)
		return
	}
	h.createTermRelationship(w, r, model.TermConfigurationChange{ResultPeriod: model.ResultPeriod{ClassID: req.ClassID, AcademicSessionID: req.AcademicSessionID, TermID: req.TermID}, Category: "enrollment", Operation: "add", StudentID: req.StudentID})
}
func (h *ResultHandler) AssignTermClassTeacher(w http.ResponseWriter, r *http.Request) {
	var req AssignClassTeacherRequest
	if httpresponse.DecodeStrict(w, r, &req) != nil {
		httpresponse.Error(w, "invalid request body", 400)
		return
	}
	h.createTermRelationship(w, r, model.TermConfigurationChange{ResultPeriod: model.ResultPeriod{ClassID: req.ClassID, AcademicSessionID: req.AcademicSessionID, TermID: req.TermID}, Category: "class_teachers", Operation: "replace", TeacherID: req.TeacherID})
}
func (h *ResultHandler) AssignTermSubjectTeacher(w http.ResponseWriter, r *http.Request) {
	var req AssignSubjectTeacherRequest
	if httpresponse.DecodeStrict(w, r, &req) != nil {
		httpresponse.Error(w, "invalid request body", 400)
		return
	}
	h.createTermRelationship(w, r, model.TermConfigurationChange{ResultPeriod: model.ResultPeriod{ClassID: req.ClassID, AcademicSessionID: req.AcademicSessionID, TermID: req.TermID, SubjectID: req.SubjectID}, Category: "subject_teachers", Operation: "replace", TeacherID: req.TeacherID})
}
func (h *ResultHandler) createTermRelationship(w http.ResponseWriter, r *http.Request, c model.TermConfigurationChange) {
	actor, ok := resultActor(w, r)
	if !ok {
		return
	}
	v, err := h.resultService.TermConfiguration(r.Context(), actor, c.ResultPeriod, &c)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(201)
	resultJSON(w, v)
}
