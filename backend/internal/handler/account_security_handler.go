package handler

import (
	"encoding/json"
	"github.com/alcidheek/school-portal/internal/httpresponse"
	"github.com/alcidheek/school-portal/internal/middleware"
	"github.com/google/uuid"
	"net/http"
)

func authenticatedUserID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	value, ok := r.Context().Value(middleware.UserIDKey).(string)
	id, err := uuid.Parse(value)
	if !ok || err != nil || id == uuid.Nil {
		httpresponse.Error(w, "Authentication is required", http.StatusUnauthorized)
		return uuid.Nil, false
	}
	return id, true
}
func (h *UserHandler) ChangeEmail(w http.ResponseWriter, r *http.Request) {
	id, ok := authenticatedUserID(w, r)
	if !ok {
		return
	}
	var req struct {
		NewEmail        string `json:"new_email"`
		CurrentPassword string `json:"current_password"`
	}
	if err := httpresponse.DecodeStrict(w, r, &req); err != nil {
		httpresponse.Error(w, "Invalid request body", 400)
		return
	}
	token, user, err := h.userService.ChangeEmail(r.Context(), id, req.NewEmail, req.CurrentPassword)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"message": "Email updated successfully", "token": token, "user": map[string]any{"id": user.ID, "email": user.Email, "role": user.Role}})
}
func (h *UserHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	id, ok := authenticatedUserID(w, r)
	if !ok {
		return
	}
	var req struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
		ConfirmPassword string `json:"confirm_password"`
	}
	if err := httpresponse.DecodeStrict(w, r, &req); err != nil {
		httpresponse.Error(w, "Invalid request body", 400)
		return
	}
	if err := h.userService.ChangePassword(r.Context(), id, req.CurrentPassword, req.NewPassword, req.ConfirmPassword); err != nil {
		writeServiceError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "Password updated successfully. Please sign in again."})
}
