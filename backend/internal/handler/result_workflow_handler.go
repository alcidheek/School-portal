package handler

import (
	"encoding/json"
	"github.com/alcidheek/school-portal/internal/httpresponse"
	"github.com/alcidheek/school-portal/internal/middleware"
	"github.com/alcidheek/school-portal/internal/model"
	"github.com/alcidheek/school-portal/internal/service"
	"github.com/google/uuid"
	"net/http"
	"strings"
)

func resultActor(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	raw, _ := r.Context().Value(middleware.UserIDKey).(string)
	id, err := uuid.Parse(raw)
	if err != nil || id == uuid.Nil {
		httpresponse.Error(w, "Authentication is required", 401)
		return uuid.Nil, false
	}
	return id, true
}
func resultJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
func resultQuery(r *http.Request) (model.ResultPeriod, error) {
	var p model.ResultPeriod
	for _, f := range []struct {
		name string
		id   *uuid.UUID
	}{{"class_id", &p.ClassID}, {"academic_session_id", &p.AcademicSessionID}, {"term_id", &p.TermID}, {"subject_id", &p.SubjectID}} {
		if values, exists := r.URL.Query()[f.name]; exists && len(values) != 1 {
			return p, service.ErrInvalidInput
		}
		raw := r.URL.Query().Get(f.name)
		if raw == "" {
			continue
		}
		id, err := uuid.Parse(raw)
		if err != nil || id == uuid.Nil {
			return p, service.ErrInvalidInput
		}
		*f.id = id
	}
	return p, nil
}
func (h *ResultHandler) Offerings(w http.ResponseWriter, r *http.Request) {
	actor, ok := resultActor(w, r)
	if !ok {
		return
	}
	p, err := resultQuery(r)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	p.ClassID, err = uuid.Parse(r.PathValue("class_id"))
	if err != nil {
		writeServiceError(w, service.ErrInvalidInput)
		return
	}
	operation := "list"
	if r.Method == http.MethodPost {
		var req struct {
			SubjectID         uuid.UUID `json:"subject_id"`
			AcademicSessionID uuid.UUID `json:"academic_session_id"`
			TermID            uuid.UUID `json:"term_id"`
		}
		if httpresponse.DecodeStrict(w, r, &req) != nil {
			httpresponse.Error(w, "invalid request body", 400)
			return
		}
		p.SubjectID = req.SubjectID
		p.AcademicSessionID = req.AcademicSessionID
		p.TermID = req.TermID
		operation = "add"
	} else if r.Method == http.MethodDelete {
		p.SubjectID, err = uuid.Parse(r.PathValue("subject_id"))
		if err != nil {
			writeServiceError(w, service.ErrInvalidInput)
			return
		}
		operation = "remove"
	}
	list, err := h.resultService.ManageOfferings(r.Context(), actor, p, operation)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	if operation == "add" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(201)
	}
	resultJSON(w, map[string]any{"subjects": list})
}
func (h *ResultHandler) EditDraft(w http.ResponseWriter, r *http.Request) {
	actor, ok := resultActor(w, r)
	if !ok {
		return
	}
	id, err := uuid.Parse(r.PathValue("result_id"))
	if err != nil {
		writeServiceError(w, service.ErrInvalidInput)
		return
	}
	var req CreateResultRequest
	if httpresponse.DecodeStrict(w, r, &req) != nil {
		httpresponse.Error(w, "invalid request body", 400)
		return
	}
	_, err = h.resultService.EditDraft(r.Context(), actor, id, service.CreateResultRequest{StudentID: req.StudentID, SubjectID: req.SubjectID, ClassID: req.ClassID, AcademicSessionID: req.AcademicSessionID, TermID: req.TermID, CAScore: req.CAScore, ExamScore: req.ExamScore})
	if err != nil {
		writeServiceError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(204)
}
func (h *ResultHandler) TransitionBatch(w http.ResponseWriter, r *http.Request) {
	actor, ok := resultActor(w, r)
	if !ok {
		return
	}
	var req struct {
		model.ResultPeriod
		Reason string `json:"reason"`
	}
	if httpresponse.DecodeStrict(w, r, &req) != nil {
		httpresponse.Error(w, "invalid request body", 400)
		return
	}
	action := "submitted"
	if strings.HasSuffix(r.URL.Path, "/approve") {
		action = "approved"
	}
	if strings.HasSuffix(r.URL.Path, "/reject") {
		action = "rejected"
	}
	if err := h.resultService.TransitionBatch(r.Context(), actor, req.ResultPeriod, action, req.Reason); err != nil {
		writeServiceError(w, err)
		return
	}
	resultJSON(w, map[string]string{"status": action})
}
func (h *ResultHandler) Publish(w http.ResponseWriter, r *http.Request) {
	actor, ok := resultActor(w, r)
	if !ok {
		return
	}
	var p model.ResultPeriod
	if httpresponse.DecodeStrict(w, r, &p) != nil {
		httpresponse.Error(w, "invalid request body", 400)
		return
	}
	publication, err := h.resultService.PublishClassTerm(r.Context(), actor, p)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(201)
	resultJSON(w, publication)
}
func (h *ResultHandler) Review(w http.ResponseWriter, r *http.Request) {
	actor, ok := resultActor(w, r)
	if !ok {
		return
	}
	p, err := resultQuery(r)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	review, err := h.resultService.ReviewBatches(r.Context(), actor, p, strings.Contains(r.URL.Path, "/teacher/"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	resultJSON(w, review)
}

func (h *ResultHandler) Options(w http.ResponseWriter, r *http.Request) {
	actor, ok := resultActor(w, r)
	if !ok {
		return
	}
	role := model.RoleAdmin
	if strings.Contains(r.URL.Path, "/teacher/") {
		role = model.RoleTeacher
	}
	if strings.Contains(r.URL.Path, "/student/") {
		role = model.RoleStudent
	}
	options, err := h.resultService.WorkflowOptions(r.Context(), actor, role)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	resultJSON(w, options)
}

func (handler *ResultHandler) SavePromotionDecisions(writer http.ResponseWriter, request *http.Request) {
	administratorID, authenticated := resultActor(writer, request)
	if !authenticated {
		return
	}
	var promotionRequest model.SaveStudentPromotionDecisionsRequest
	if err := httpresponse.DecodeStrict(writer, request, &promotionRequest); err != nil {
		httpresponse.Error(writer, "invalid request body", http.StatusBadRequest)
		return
	}
	response, err := handler.resultService.SavePromotionDecisions(request.Context(), administratorID, promotionRequest)
	if err != nil {
		writeServiceError(writer, err)
		return
	}
	resultJSON(writer, response)
}

func (handler *ResultHandler) DeletePromotionDecision(writer http.ResponseWriter, request *http.Request) {
	administratorID, authenticated := resultActor(writer, request)
	if !authenticated {
		return
	}
	decisionID, err := uuid.Parse(request.PathValue("decision_id"))
	if err != nil || decisionID == uuid.Nil {
		writeServiceError(writer, service.ErrInvalidInput)
		return
	}
	if err := handler.resultService.DeletePromotionDecision(request.Context(), administratorID, decisionID); err != nil {
		writeServiceError(writer, err)
		return
	}
	writer.WriteHeader(http.StatusNoContent)
}

func promotionSourceQuery(request *http.Request) (model.PromotionSource, error) {
	var source model.PromotionSource
	query := request.URL.Query()
	for _, field := range []struct {
		name       string
		identifier *uuid.UUID
	}{
		{"source_class_id", &source.SourceClassID},
		{"source_academic_session_id", &source.SourceAcademicSessionID},
		{"source_term_id", &source.SourceTermID},
	} {
		values := query[field.name]
		if len(values) != 1 {
			return source, service.ErrInvalidInput
		}
		identifier, err := uuid.Parse(values[0])
		if err != nil || identifier == uuid.Nil {
			return source, service.ErrInvalidInput
		}
		*field.identifier = identifier
	}
	return source, nil
}

func (handler *ResultHandler) ListPromotionDecisions(writer http.ResponseWriter, request *http.Request) {
	administratorID, authenticated := resultActor(writer, request)
	if !authenticated {
		return
	}
	source, err := promotionSourceQuery(request)
	if err != nil {
		writeServiceError(writer, err)
		return
	}
	decisions, err := handler.resultService.ListPromotionDecisions(request.Context(), administratorID, source)
	if err != nil {
		writeServiceError(writer, err)
		return
	}
	resultJSON(writer, map[string]any{"decisions": decisions})
}

func (handler *ResultHandler) FinalizePromotions(writer http.ResponseWriter, request *http.Request) {
	administratorID, authenticated := resultActor(writer, request)
	if !authenticated {
		return
	}
	var promotionRequest model.FinalizeStudentPromotionsRequest
	if err := httpresponse.DecodeStrict(writer, request, &promotionRequest); err != nil {
		httpresponse.Error(writer, "invalid request body", http.StatusBadRequest)
		return
	}
	response, err := handler.resultService.FinalizePromotions(request.Context(), administratorID, promotionRequest)
	if err != nil {
		writeServiceError(writer, err)
		return
	}
	// A batch with failures returns 207, even if every student failed.
	// Clients must read each result; successful students have already committed.
	if response.Failed > 0 {
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusMultiStatus)
	}
	resultJSON(writer, response)
}
