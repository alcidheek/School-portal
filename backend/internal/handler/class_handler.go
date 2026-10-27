package handler

import (
	"encoding/json"
	"github.com/alcidheek/school-portal/internal/httpresponse"
	"net/http"

	"github.com/alcidheek/school-portal/internal/service"
	"github.com/google/uuid"
)

type ClassHandler struct {
	classService *service.ClassService
}

// NewClassHandler creates a new class handler.
func NewClassHandler(
	classService *service.ClassService,
) *ClassHandler {
	return &ClassHandler{
		classService: classService,
	}
}

// Create creates a new school class.
func (h *ClassHandler) Create(
	w http.ResponseWriter,
	r *http.Request,
) {
	// Decode the incoming JSON request.
	var req CreateClassRequest

	if err := httpresponse.Decode(w, r, &req); err != nil {
		httpresponse.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// Pass plain values to the service layer.
	class, err := h.classService.Create(
		r.Context(),
		req.Name,
		req.Level,
	)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	// Return the newly created class.
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(class)
}

// GetAll returns all available school classes.
func (h *ClassHandler) GetAll(
	w http.ResponseWriter,
	r *http.Request,
) {
	// Ask the service for all available classes.
	classes, err := h.classService.GetAll(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}

	// Return the classes as JSON.
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(classes)
}

type UpdateClassRequest struct {
	Name  string `json:"name"`
	Level string `json:"level"`
}

// Update requires both the class name and level and returns the saved class.
func (handler *ClassHandler) Update(response http.ResponseWriter, request *http.Request) {
	classID, err := uuid.Parse(request.PathValue("id"))
	if err != nil || classID == uuid.Nil {
		httpresponse.Error(response, "invalid class id", http.StatusBadRequest)
		return
	}

	var updateRequest UpdateClassRequest
	if err := httpresponse.DecodeStrict(response, request, &updateRequest); err != nil {
		httpresponse.Error(response, "invalid request body", http.StatusBadRequest)
		return
	}

	class, err := handler.classService.Update(request.Context(), classID, updateRequest.Name, updateRequest.Level)
	if err != nil {
		writeServiceError(response, err)
		return
	}

	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(http.StatusOK)
	json.NewEncoder(response).Encode(class)
}

// Delete removes an unreferenced class without deleting related academic data.
func (handler *ClassHandler) Delete(response http.ResponseWriter, request *http.Request) {
	classID, err := uuid.Parse(request.PathValue("id"))
	if err != nil || classID == uuid.Nil {
		httpresponse.Error(response, "invalid class id", http.StatusBadRequest)
		return
	}

	if err := handler.classService.Delete(request.Context(), classID); err != nil {
		writeServiceError(response, err)
		return
	}

	response.WriteHeader(http.StatusNoContent)
}
