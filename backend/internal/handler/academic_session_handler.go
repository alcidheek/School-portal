package handler

import (
	"encoding/json"
	"github.com/alcidheek/school-portal/internal/httpresponse"

	"net/http"

	"github.com/google/uuid"

	"github.com/alcidheek/school-portal/internal/service"
)

type AcademicSessionHandler struct {
	sessionService *service.AcademicSessionService
}

func NewAcademicSessionHandler(
	sessionService *service.AcademicSessionService,
) *AcademicSessionHandler {
	return &AcademicSessionHandler{
		sessionService: sessionService,
	}
}

func (h *AcademicSessionHandler) Create(
	w http.ResponseWriter,
	r *http.Request,
) {
	var req CreateAcademicSessionRequest

	if err := httpresponse.Decode(w, r, &req); err != nil {
		httpresponse.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	session, err := h.sessionService.Create(r.Context(), req.Name, req.StartDate, req.EndDate, req.Activate, req.TermName, req.TermStartDate, req.TermEndDate)
	if err != nil {
		writeAcademicError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)

	json.NewEncoder(w).Encode(session)
}

type SwitchTermRequest struct {
	TermName  string `json:"term_name"`
	StartDate string `json:"start_date"`
	EndDate   string `json:"end_date"`
}

func (h *AcademicSessionHandler) SwitchTerm(
	w http.ResponseWriter,
	r *http.Request,
) {
	// Get the academic session ID from the URL.
	sessionID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		httpresponse.Error(w, "invalid academic session id", http.StatusBadRequest)
		return
	}

	// Decode the new term details from the request body.
	var req SwitchTermRequest

	if err := httpresponse.Decode(w, r, &req); err != nil {
		httpresponse.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// Pass the values to the service to switch the current term.
	term, err := h.sessionService.SwitchTerm(
		r.Context(),
		sessionID,
		req.TermName,
		req.StartDate,
		req.EndDate,
	)
	if err != nil {
		writeAcademicError(w, err)
		return
	}

	// Return the selected active term, whether created or reused.
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	json.NewEncoder(w).Encode(term)
}

func (h *AcademicSessionHandler) Activate(
	w http.ResponseWriter,
	r *http.Request,
) {
	// Get the academic session ID from the URL.
	sessionID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		httpresponse.Error(
			w,
			"invalid academic session id",
			http.StatusBadRequest,
		)
		return
	}

	// Ask the service to make this the active session.
	if err := h.sessionService.Activate(
		r.Context(),
		sessionID,
	); err != nil {
		writeAcademicError(w, err)
		return
	}

	// No response body is needed.
	w.WriteHeader(http.StatusNoContent)
}

func (h *AcademicSessionHandler) List(
	w http.ResponseWriter,
	r *http.Request,
) {
	// Get all academic sessions from the service.
	sessions, err := h.sessionService.List(r.Context())
	if err != nil {
		httpresponse.Error(
			w,
			"failed to fetch academic sessions",
			http.StatusInternalServerError,
		)
		return
	}

	// Return the sessions as JSON.
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	json.NewEncoder(w).Encode(sessions)
}

// GetActiveTerm returns the active term
// for a specific academic session.
func (h *AcademicSessionHandler) GetActiveTerm(
	w http.ResponseWriter,
	r *http.Request,
) {
	// Read the academic session ID from the URL.
	sessionID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		httpresponse.Error(
			w,
			"invalid academic session id",
			http.StatusBadRequest,
		)
		return
	}

	// Ask the service for the active term.
	term, err := h.sessionService.GetActiveTerm(
		r.Context(),
		sessionID,
	)
	if err != nil {
		writeAcademicError(w, err)
		return
	}

	// Return the active term as JSON.
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	json.NewEncoder(w).Encode(term)
}

func (handler *AcademicSessionHandler) ListTerms(response http.ResponseWriter, request *http.Request) {
	sessionID, err := uuid.Parse(request.PathValue("id"))
	if err != nil {
		httpresponse.Error(response, "invalid academic session id", http.StatusBadRequest)
		return
	}

	terms, err := handler.sessionService.ListTerms(request.Context(), sessionID)
	if err != nil {
		writeAcademicError(response, err)
		return
	}

	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(http.StatusOK)
	json.NewEncoder(response).Encode(terms)
}

func (handler *AcademicSessionHandler) Delete(response http.ResponseWriter, request *http.Request) {
	sessionID, err := uuid.Parse(request.PathValue("id"))
	if err != nil {
		httpresponse.Error(response, "invalid academic session id", http.StatusBadRequest)
		return
	}

	if err := handler.sessionService.Delete(request.Context(), sessionID); err != nil {
		writeAcademicError(response, err)
		return
	}

	response.WriteHeader(http.StatusNoContent)
}

func writeAcademicError(w http.ResponseWriter, err error) {
	writeServiceError(w, err)
}
