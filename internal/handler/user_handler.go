package handler

import (
	"encoding/json"
	"net/http"

	"github.com/alcidheek/school-portal/internal/model"
	"github.com/alcidheek/school-portal/internal/service"
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

type RegisterUserRequest struct {
	FullName string     `json:"full_name"`
	Email    string     `json:"email"`
	Phone    string     `json:"phone"`
	Password string     `json:"password"`
	Role     model.Role `json:"role"`
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (h *UserHandler) RegisterUser(
	w http.ResponseWriter,
	r *http.Request,
) {
	var req RegisterUserRequest

	// Decode request body
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(
			w,
			"invalid request body",
			http.StatusBadRequest,
		)
		return
	}

	// Call service
	user, err := h.userService.RegisterUser(
		r.Context(),
		service.RegisterUserRequest{
			FullName: req.FullName,
			Email:    req.Email,
			Phone:    req.Phone,
			Password: req.Password,
			Role:     req.Role,
		},
	)

	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
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

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
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
		http.Error(w, err.Error(), http.StatusUnauthorized)
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
