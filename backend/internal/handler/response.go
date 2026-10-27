package handler

import (
	"database/sql"
	"errors"
	"net/http"

	"github.com/alcidheek/school-portal/internal/httpresponse"
	"github.com/alcidheek/school-portal/internal/repository"
	"github.com/alcidheek/school-portal/internal/service"
	"github.com/lib/pq"
)

// Domain errors have safe messages. Unknown and database errors never use
// err.Error() in the response; wrapped pq errors are classified by SQLSTATE.
func writeServiceError(w http.ResponseWriter, err error) {
	status, message := http.StatusInternalServerError, "Internal server error"
	var pgErr *pq.Error
	switch {
	case errors.Is(err, repository.ErrTermRequired):
		status, message = http.StatusBadRequest, err.Error()
	case errors.Is(err, repository.ErrResultWorkflowForbidden):
		status, message = http.StatusForbidden, "Access to this resource is forbidden"
	case errors.Is(err, repository.ErrResultWorkflowConflict):
		status, message = http.StatusConflict, err.Error()
	case errors.Is(err, service.ErrAuthentication):
		status, message = http.StatusUnauthorized, service.ErrAuthentication.Error()
	case errors.Is(err, service.ErrResourceForbidden):
		status, message = http.StatusForbidden, service.ErrResourceForbidden.Error()
	case errors.Is(err, service.ErrCurrentPassword):
		status, message = http.StatusUnauthorized, service.ErrCurrentPassword.Error()
	case errors.Is(err, service.ErrForbiddenAccount):
		status, message = http.StatusForbidden, service.ErrForbiddenAccount.Error()
	case errors.Is(err, service.ErrFinalActiveAdmin):
		status, message = http.StatusConflict, service.ErrFinalActiveAdmin.Error()
	case errors.Is(err, service.ErrEmailConflict):
		status, message = http.StatusConflict, service.ErrEmailConflict.Error()
	case errors.Is(err, service.ErrInvalidCredentials):
		status, message = http.StatusUnauthorized, service.ErrInvalidCredentials.Error()
	case errors.Is(err, service.ErrInactiveAccount):
		status, message = http.StatusForbidden, service.ErrInactiveAccount.Error()
	case errors.Is(err, service.ErrInvalidInput), errors.Is(err, service.ErrInvalidAcademicInput), errors.Is(err, service.ErrInvalidAttendance):
		status, message = http.StatusBadRequest, err.Error()
	case errors.Is(err, service.ErrResetEmail):
		status, message = http.StatusBadRequest, service.ErrResetEmail.Error()
	case errors.Is(err, service.ErrResetCodeFormat):
		status, message = http.StatusBadRequest, service.ErrResetCodeFormat.Error()
	case errors.Is(err, service.ErrResetPassword):
		status, message = http.StatusBadRequest, service.ErrResetPassword.Error()
	case errors.Is(err, repository.ErrInvalidResetCode):
		status, message = http.StatusBadRequest, repository.ErrInvalidResetCode.Error()
	case errors.Is(err, service.ErrAcademicSessionNotFound):
		status, message = http.StatusNotFound, service.ErrAcademicSessionNotFound.Error()
	case errors.Is(err, service.ErrActiveTermNotFound):
		status, message = http.StatusNotFound, service.ErrActiveTermNotFound.Error()
	case errors.Is(err, service.ErrStudentEnrollmentNotFound):
		status, message = http.StatusNotFound, service.ErrStudentEnrollmentNotFound.Error()
	case errors.Is(err, service.ErrClassTeacherNotFound):
		status, message = http.StatusNotFound, service.ErrClassTeacherNotFound.Error()
	case errors.Is(err, service.ErrNotFound), errors.Is(err, sql.ErrNoRows):
		status, message = http.StatusNotFound, "Resource not found"
	case errors.Is(err, service.ErrTeacherNotAssigned):
		status, message = http.StatusForbidden, service.ErrTeacherNotAssigned.Error()
	case errors.Is(err, service.ErrAttendanceForbidden):
		status, message = http.StatusForbidden, service.ErrAttendanceForbidden.Error()
	case errors.Is(err, repository.ErrDuplicateResult):
		status, message = http.StatusConflict, repository.ErrDuplicateResult.Error()
	case errors.Is(err, repository.ErrDuplicateAttendance):
		status, message = http.StatusConflict, repository.ErrDuplicateAttendance.Error()
	case errors.Is(err, service.ErrAcademicSessionInactive):
		status, message = http.StatusConflict, service.ErrAcademicSessionInactive.Error()
	case errors.Is(err, service.ErrNoAvailableTerm):
		status, message = http.StatusConflict, service.ErrNoAvailableTerm.Error()
	case errors.Is(err, service.ErrTermDatesConflict):
		status, message = http.StatusConflict, service.ErrTermDatesConflict.Error()
	case errors.Is(err, service.ErrTermSessionMismatch):
		status, message = http.StatusConflict, service.ErrTermSessionMismatch.Error()
	case errors.Is(err, service.ErrAcademicConflict):
		status, message = http.StatusConflict, service.ErrAcademicConflict.Error()
	case errors.Is(err, service.ErrConflict):
		status, message = http.StatusConflict, service.ErrConflict.Error()
		// Service-authored conflict details are safe unless a database error is
		// also wrapped into the chain. Keep database diagnostics private.
		if !errors.As(err, &pgErr) {
			message = err.Error()
		}
	case errors.As(err, &pgErr):
		switch pgErr.Code {
		case "23505":
			status, message = http.StatusConflict, "Operation conflicts with existing data"
		case "23503":
			status, message = http.StatusNotFound, "Referenced resource not found"
		case "P6001":
			status, message = http.StatusConflict, "Class-term configuration is locked or has recorded academic data; initialize or edit an unlocked term"
		case "P6002":
			status, message = http.StatusForbidden, "Teacher is not assigned for this term"
		}
	}
	httpresponse.Error(w, message, status)
}
