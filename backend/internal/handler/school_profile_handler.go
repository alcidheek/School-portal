package handler

import (
	"encoding/json"
	"net/http"

	"github.com/alcidheek/school-portal/internal/httpresponse"
	"github.com/alcidheek/school-portal/internal/model"
	"github.com/alcidheek/school-portal/internal/service"
)

// SchoolProfileHandler handles school profile requests.
type SchoolProfileHandler struct {
	schoolProfileService *service.SchoolProfileService
}

// NewSchoolProfileHandler creates the school profile handler.
func NewSchoolProfileHandler(
	schoolProfileService *service.SchoolProfileService,
) *SchoolProfileHandler {
	return &SchoolProfileHandler{
		schoolProfileService: schoolProfileService,
	}
}

// UpdateSchoolProfileRequest represents all editable
// school profile fields.
//
// Pointers allow the handler to distinguish missing
// JSON fields from explicitly supplied empty strings.
type UpdateSchoolProfileRequest struct {
	Name    *string `json:"name"`
	Motto   *string `json:"motto"`
	Email   *string `json:"email"`
	Phone   *string `json:"phone"`
	Address *string `json:"address"`
	Website *string `json:"website"`
}

// Get returns the currently configured school profile.
func (handler *SchoolProfileHandler) Get(
	response http.ResponseWriter,
	request *http.Request,
) {
	profile, err := handler.schoolProfileService.Get(
		request.Context(),
	)

	if err != nil {
		writeServiceError(response, err)
		return
	}

	response.Header().Set(
		"Content-Type",
		"application/json",
	)

	response.WriteHeader(http.StatusOK)

	_ = json.NewEncoder(response).Encode(profile)
}

// Update creates or updates the singleton school profile.
func (handler *SchoolProfileHandler) Update(
	response http.ResponseWriter,
	request *http.Request,
) {
	var updateRequest UpdateSchoolProfileRequest

	if err := httpresponse.DecodeStrict(
		response,
		request,
		&updateRequest,
	); err != nil {
		httpresponse.Error(
			response,
			"invalid request body",
			http.StatusBadRequest,
		)
		return
	}

	// Every field must be present in the PUT request.
	// Empty strings are allowed for optional fields.
	if updateRequest.Name == nil ||
		updateRequest.Motto == nil ||
		updateRequest.Email == nil ||
		updateRequest.Phone == nil ||
		updateRequest.Address == nil ||
		updateRequest.Website == nil {
		httpresponse.Error(
			response,
			"all school profile fields must be supplied",
			http.StatusBadRequest,
		)
		return
	}

	profile := &model.SchoolProfile{
		Name:    *updateRequest.Name,
		Motto:   *updateRequest.Motto,
		Email:   *updateRequest.Email,
		Phone:   *updateRequest.Phone,
		Address: *updateRequest.Address,
		Website: *updateRequest.Website,
	}

	savedProfile, err := handler.schoolProfileService.Update(
		request.Context(),
		profile,
	)

	if err != nil {
		writeServiceError(response, err)
		return
	}

	response.Header().Set(
		"Content-Type",
		"application/json",
	)

	response.WriteHeader(http.StatusOK)

	_ = json.NewEncoder(response).Encode(savedProfile)
}
