package handler

import (
	"encoding/json"
	"github.com/alcidheek/school-portal/internal/httpresponse"
	"net/http"

	"github.com/alcidheek/school-portal/internal/model"
	"github.com/alcidheek/school-portal/internal/service"
	"github.com/google/uuid"
)

type UserHandler struct {
	userService *service.UserService
}

func NewUserHandler(
	userService *service.UserService,
) *UserHandler {
	return &UserHandler{
		userService: userService,
	}
}

func (h *UserHandler) RegisterUser(
	w http.ResponseWriter,
	r *http.Request,
) {
	actorID, ok := authenticatedUserID(w, r)
	if !ok {
		return
	}
	var req RegisterUserRequest

	// Decode request body
	if err := httpresponse.DecodeStrict(w, r, &req); err != nil {
		httpresponse.Error(
			w,
			"invalid request body",
			http.StatusBadRequest,
		)
		return
	}

	// Call service
	user, err := h.userService.RegisterUserAsAdmin(
		r.Context(),
		actorID,
		service.RegisterUserRequest{
			FullName:      req.FullName,
			Email:         req.Email,
			Phone:         req.Phone,
			Password:      req.Password,
			Role:          req.Role,
			GuardianName:  req.GuardianName,
			GuardianPhone: req.GuardianPhone,
		},
	)

	if err != nil {
		writeServiceError(w, err)
		return
	}

	response := map[string]interface{}{
		"message": "user successfully created",
	}

	if user.AdmissionNumber != nil {
		response["admission_number"] = *user.AdmissionNumber
	}

	if user.EmployeeNumber != nil {
		response["employee_number"] = *user.EmployeeNumber
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)

	json.NewEncoder(w).Encode(response)
}

func (h *UserHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest

	if err := httpresponse.Decode(w, r, &req); err != nil {
		httpresponse.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	token, user, err := h.userService.Login(
		r.Context(),
		service.LoginRequest{
			Email:    req.Email,
			Password: req.Password,
		},
	)

	if err != nil {
		writeServiceError(w, err)
		return
	}

	response := map[string]interface{}{
		"message": "login successful",
		"token":   token,
		"user": map[string]interface{}{
			"id":    user.ID,
			"email": user.Email,
			"role":  user.Role,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	json.NewEncoder(w).Encode(response)
}

func (h *UserHandler) ListUsers(w http.ResponseWriter, r *http.Request) {
	users, err := h.userService.ListUsers(r.Context())
	if err != nil {
		httpresponse.Error(w, "failed to fetch users", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	json.NewEncoder(w).Encode(map[string]interface{}{
		"users": users,
	})
}

func (h *UserHandler) GetUserByID(w http.ResponseWriter, r *http.Request) {
	userID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		httpresponse.Error(w, "invalid user id", http.StatusBadRequest)
		return
	}

	user, err := h.userService.GetUserByID(r.Context(), userID)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(user)
}

func (h *UserHandler) UpdateUser(w http.ResponseWriter, r *http.Request) {
	actorID, ok := authenticatedUserID(w, r)
	if !ok {
		return
	}
	userID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		httpresponse.Error(w, "invalid user id", http.StatusBadRequest)
		return
	}

	var req model.UpdateUserRequest
	if err := httpresponse.DecodeStrict(w, r, &req); err != nil {
		httpresponse.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if err := h.userService.UpdateUserAsAdmin(r.Context(), actorID, userID, req); err != nil {
		writeServiceError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
