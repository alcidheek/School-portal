package handler

import (
	"encoding/json"
	"net/http"
)

func (h *UserHandler) Me(w http.ResponseWriter, r *http.Request) {

	userID, ok := authenticatedUserID(w, r)
	if !ok {
		return
	}
	user, err := h.userService.CurrentUser(r.Context(), userID)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	response := map[string]interface{}{
		"user_id": userID,
		"email":   user.Email,
		"role":    user.Role,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (h *UserHandler) AdminTest(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("welcome admin"))
}
