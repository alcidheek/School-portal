package handler

import (
	"encoding/json"
	"github.com/alcidheek/school-portal/internal/httpresponse"
	"net/http"

	"github.com/alcidheek/school-portal/internal/service"
	"github.com/google/uuid"
)

type SubjectHandler struct {
	subjectService *service.SubjectService
}

func NewSubjectHandler(subjectService *service.SubjectService) *SubjectHandler {
	return &SubjectHandler{subjectService: subjectService}
}

func (h *SubjectHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req CreateSubjectRequest
	if err := httpresponse.Decode(w, r, &req); err != nil {
		httpresponse.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	subject, err := h.subjectService.Create(r.Context(), req.Name)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(subject)
}

func (h *SubjectHandler) GetAll(w http.ResponseWriter, r *http.Request) {
	subjects, err := h.subjectService.GetAll(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(subjects)
}

type UpdateSubjectRequest struct {
	Name string `json:"name"`
}

// Update requires a subject name and returns the saved subject.
func (handler *SubjectHandler) Update(response http.ResponseWriter, request *http.Request) {
	subjectID, err := uuid.Parse(request.PathValue("id"))
	if err != nil || subjectID == uuid.Nil {
		httpresponse.Error(response, "invalid subject id", http.StatusBadRequest)
		return
	}

	var updateRequest UpdateSubjectRequest
	if err := httpresponse.DecodeStrict(response, request, &updateRequest); err != nil {
		httpresponse.Error(response, "invalid request body", http.StatusBadRequest)
		return
	}

	subject, err := handler.subjectService.Update(request.Context(), subjectID, updateRequest.Name)
	if err != nil {
		writeServiceError(response, err)
		return
	}

	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(http.StatusOK)
	json.NewEncoder(response).Encode(subject)
}

// Delete removes an unreferenced subject without deleting related academic data.
func (handler *SubjectHandler) Delete(response http.ResponseWriter, request *http.Request) {
	subjectID, err := uuid.Parse(request.PathValue("id"))
	if err != nil || subjectID == uuid.Nil {
		httpresponse.Error(response, "invalid subject id", http.StatusBadRequest)
		return
	}

	if err := handler.subjectService.Delete(request.Context(), subjectID); err != nil {
		writeServiceError(response, err)
		return
	}

	response.WriteHeader(http.StatusNoContent)
}
