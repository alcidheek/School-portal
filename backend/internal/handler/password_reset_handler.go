package handler

import (
	"encoding/json"
	"errors"
	"github.com/alcidheek/school-portal/internal/httpresponse"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/alcidheek/school-portal/internal/repository"
	"github.com/alcidheek/school-portal/internal/service"
)

func decodeResetRequest(w http.ResponseWriter, r *http.Request, target any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		httpresponse.Error(w, "invalid request body", http.StatusBadRequest)
		return false
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		httpresponse.Error(w, "invalid request body", http.StatusBadRequest)
		return false
	}
	return true
}

func writeResetResponse(w http.ResponseWriter, message string, err error) {
	if err != nil {
		if errors.Is(err, service.ErrInvalidInput) || errors.Is(err, service.ErrResetEmail) || errors.Is(err, service.ErrResetCodeFormat) || errors.Is(err, service.ErrResetPassword) || errors.Is(err, repository.ErrInvalidResetCode) {
			writeServiceError(w, err)
		} else {
			log.Print("Password reset failed")
			httpresponse.Error(w, "unable to process password reset", http.StatusInternalServerError)
		}
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": message})
}

func (h *UserHandler) ForgotPassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email string `json:"email"`
	}
	if !decodeResetRequest(w, r, &req) {
		return
	}
	err := h.userService.ForgotPassword(r.Context(), req.Email)
	writeResetResponse(w, "If the account exists, a password reset code has been emailed.", err)
}

func (h *UserHandler) ResetPassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email       string `json:"email"`
		Code        string `json:"code"`
		ResetCode   string `json:"reset_code"`
		NewPassword string `json:"new_password"`
	}
	if !decodeResetRequest(w, r, &req) {
		return
	}
	code, resetCode := strings.TrimSpace(req.Code), strings.TrimSpace(req.ResetCode)
	if code != "" && resetCode != "" && code != resetCode {
		httpresponse.Error(w, "code and reset_code must match when both are provided", http.StatusBadRequest)
		return
	}
	if code == "" {
		code = resetCode
	}
	err := h.userService.ResetPassword(r.Context(), req.Email, code, req.NewPassword)
	writeResetResponse(w, "Password reset successfully.", err)
}
