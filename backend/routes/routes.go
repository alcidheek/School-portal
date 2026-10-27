package routes

import (
	"net/http"

	"github.com/alcidheek/school-portal/internal/handler"
	"github.com/alcidheek/school-portal/internal/httpresponse"
	"github.com/alcidheek/school-portal/internal/middleware"
	"github.com/alcidheek/school-portal/internal/model"
)

type AuthenticationRateLimiters struct {
	Login, ForgotPassword, ResetPassword *middleware.RateLimiter
}

func NewAuthenticationRateLimiters() *AuthenticationRateLimiters {
	return &AuthenticationRateLimiters{
		Login:          middleware.NewRateLimiter(5, 5.0/60.0),
		ForgotPassword: middleware.NewRateLimiter(3, 3.0/60.0),
		ResetPassword:  middleware.NewRateLimiter(5, 5.0/60.0),
	}
}

func SetupRoutes(
	userHandler *handler.UserHandler,
	academicSessionHandler *handler.AcademicSessionHandler,
	classHandler *handler.ClassHandler,
	subjectHandler *handler.SubjectHandler,
	studentEnrollmentHandler *handler.StudentEnrollmentHandler,
	teacherAssignmentHandler *handler.TeacherAssignmentHandler,
	resultHandler *handler.ResultHandler,
	attendanceHandler *handler.AttendanceHandler,
	schoolProfileHandler *handler.SchoolProfileHandler,
	jwtSecret string,
	configuredLimiters ...*AuthenticationRateLimiters,
) http.Handler {
	return SetupRoutesWithMedia(userHandler, academicSessionHandler, classHandler,
		subjectHandler, studentEnrollmentHandler, teacherAssignmentHandler,
		resultHandler, attendanceHandler, schoolProfileHandler, nil, jwtSecret, configuredLimiters...)
}

// SetupRoutesWithMedia preserves SetupRoutes for existing callers while enabling
// media endpoints through the same JWT, active-account and role guards.
func SetupRoutesWithMedia(
	userHandler *handler.UserHandler,
	academicSessionHandler *handler.AcademicSessionHandler,
	classHandler *handler.ClassHandler,
	subjectHandler *handler.SubjectHandler,
	studentEnrollmentHandler *handler.StudentEnrollmentHandler,
	teacherAssignmentHandler *handler.TeacherAssignmentHandler,
	resultHandler *handler.ResultHandler,
	attendanceHandler *handler.AttendanceHandler,
	schoolProfileHandler *handler.SchoolProfileHandler,
	mediaHandler *handler.MediaHandler,

	jwtSecret string,
	configuredLimiters ...*AuthenticationRateLimiters,
) http.Handler {

	mux := http.NewServeMux()

	// Authentication rate limiters.
	var limiters *AuthenticationRateLimiters
	if len(configuredLimiters) > 0 {
		limiters = configuredLimiters[0]
	}
	if limiters == nil {
		limiters = NewAuthenticationRateLimiters()
	}
	loginLimiter := limiters.Login
	forgotPasswordLimiter := limiters.ForgotPassword
	resetPasswordLimiter := limiters.ResetPassword

	authenticate := func(next http.Handler) http.Handler {
		return middleware.AuthMiddleware(jwtSecret)(userHandler.RequireActiveAccount(next))
	}
	if mediaHandler != nil {
		mux.Handle("PUT /api/v1/admin/school-profile/logo", authenticate(middleware.RequireRole(model.RoleAdmin)(http.HandlerFunc(mediaHandler.UploadSchoolLogo))))
		mux.Handle("DELETE /api/v1/admin/school-profile/logo", authenticate(middleware.RequireRole(model.RoleAdmin)(http.HandlerFunc(mediaHandler.DeleteSchoolLogo))))
		mux.Handle("GET /api/v1/school-profile/logo", authenticate(http.HandlerFunc(mediaHandler.DownloadSchoolLogo)))
		mux.Handle("PUT /api/v1/users/me/profile-picture", authenticate(http.HandlerFunc(mediaHandler.UploadProfilePicture)))
		mux.Handle("GET /api/v1/users/me/profile-picture", authenticate(http.HandlerFunc(mediaHandler.DownloadProfilePicture)))
		mux.Handle("DELETE /api/v1/users/me/profile-picture", authenticate(http.HandlerFunc(mediaHandler.DeleteProfilePicture)))
	}

	// User routes
	mux.Handle(
		"POST /auth/register",
		authenticate(
			middleware.RequireRole(model.RoleAdmin)(
				http.HandlerFunc(userHandler.RegisterUser),
			),
		),
	)

	// Authentication routes.
	// Login.
	mux.Handle(
		"POST /auth/login",
		loginLimiter.Middleware(
			http.HandlerFunc(userHandler.Login),
		),
	)
	// Logout — keep existing JWT protection.
	mux.Handle(
		"POST /auth/logout",
		middleware.AuthMiddleware(jwtSecret)(
			http.HandlerFunc(userHandler.Logout),
		),
	)
	// Forgot password.
	mux.Handle(
		"POST /auth/forgot-password",
		forgotPasswordLimiter.Middleware(
			http.HandlerFunc(userHandler.ForgotPassword),
		),
	)
	// Reset password.
	mux.Handle(
		"POST /auth/reset-password",
		resetPasswordLimiter.Middleware(
			http.HandlerFunc(userHandler.ResetPassword),
		),
	)

	// authorization routes
	mux.Handle(
		"GET /api/v1/me",
		authenticate(
			http.HandlerFunc(userHandler.Me)),
	)

	//rbac admin role
	for _, route := range []struct {
		pattern string
		handler http.HandlerFunc
	}{
		{"PATCH /api/v1/me/email", userHandler.ChangeEmail},
		{"PATCH /api/v1/me/password", userHandler.ChangePassword},
	} {
		mux.Handle(route.pattern, authenticate(middleware.RequireRole(model.RoleAdmin)(route.handler)))
	}

	//rbac admin role
	mux.Handle(
		"GET /api/v1/admin/test",
		authenticate(
			middleware.RequireRole(model.RoleAdmin)(
				http.HandlerFunc(userHandler.AdminTest))),
	)

	// List users route
	mux.Handle(
		"GET /api/v1/admin/users",
		authenticate(
			middleware.RequireRole(model.RoleAdmin)(
				http.HandlerFunc(userHandler.ListUsers))),
	)

	// Get user by ID route
	mux.Handle(
		"GET /api/v1/admin/users/{id}",
		authenticate(
			middleware.RequireRole(model.RoleAdmin)(
				http.HandlerFunc(userHandler.GetUserByID))),
	)

	// Update user route
	mux.Handle(
		"PATCH /api/v1/admin/users/{id}",
		authenticate(
			middleware.RequireRole(model.RoleAdmin)(
				http.HandlerFunc(userHandler.UpdateUser))),
	)

	// Academic session routes
	mux.Handle(
		"POST /api/v1/admin/academic-sessions",
		authenticate(
			middleware.RequireRole(model.RoleAdmin)(
				http.HandlerFunc(academicSessionHandler.Create))),
	)

	// School Profile Settings
	// Only authenticated, active administrators
	// can retrieve or update the school profile.
	for _, route := range []struct {
		pattern string
		handle  http.HandlerFunc
	}{
		{
			pattern: "GET /api/v1/admin/school-profile",
			handle:  schoolProfileHandler.Get,
		},
		{
			pattern: "PUT /api/v1/admin/school-profile",
			handle:  schoolProfileHandler.Update,
		},
	} {
		mux.Handle(
			route.pattern,
			authenticate(
				middleware.RequireRole(model.RoleAdmin)(
					route.handle,
				),
			),
		)
	}

	// Delete an unused, inactive academic session.
	mux.Handle(
		"DELETE /api/v1/admin/academic-sessions/{id}",
		authenticate(
			middleware.RequireRole(model.RoleAdmin)(
				http.HandlerFunc(academicSessionHandler.Delete),
			),
		),
	)

	// Make an existing academic session the active session.
	mux.Handle(
		"PATCH /api/v1/admin/academic-sessions/{id}/activate",
		authenticate(
			middleware.RequireRole(model.RoleAdmin)(
				http.HandlerFunc(academicSessionHandler.Activate),
			),
		),
	)

	// Get all academic sessions.
	mux.Handle(
		"GET /api/v1/admin/academic-sessions",
		authenticate(
			middleware.RequireRole(model.RoleAdmin)(
				http.HandlerFunc(academicSessionHandler.List),
			),
		),
	)

	// Get existing terms for an academic session.
	mux.Handle(
		"GET /api/v1/admin/academic-sessions/{id}/terms",
		authenticate(
			middleware.RequireRole(model.RoleAdmin)(
				http.HandlerFunc(academicSessionHandler.ListTerms),
			),
		),
	)

	// Get the active term for an academic session.
	mux.Handle(
		"GET /api/v1/admin/academic-sessions/{id}/active-term",
		authenticate(
			middleware.RequireRole(model.RoleAdmin)(
				http.HandlerFunc(
					academicSessionHandler.GetActiveTerm,
				),
			),
		),
	)

	// Switch to another term within an existing academic session.
	mux.Handle(
		"PATCH /api/v1/admin/academic-sessions/{id}/term",
		authenticate(
			middleware.RequireRole(model.RoleAdmin)(
				http.HandlerFunc(academicSessionHandler.SwitchTerm),
			),
		),
	)

	// Create a new school class.
	mux.Handle(
		"POST /api/v1/admin/classes",
		authenticate(
			middleware.RequireRole(model.RoleAdmin)(
				http.HandlerFunc(classHandler.Create),
			),
		),
	)

	// Get all available school classes.
	mux.Handle(
		"GET /api/v1/admin/classes",
		authenticate(
			middleware.RequireRole(model.RoleAdmin)(
				http.HandlerFunc(classHandler.GetAll),
			),
		),
	)

	// Update the name and level of an existing school class.
	mux.Handle(
		"PATCH /api/v1/admin/classes/{id}",
		authenticate(
			middleware.RequireRole(model.RoleAdmin)(
				http.HandlerFunc(classHandler.Update),
			),
		),
	)

	// Delete a school class only when no academic records reference it.
	mux.Handle(
		"DELETE /api/v1/admin/classes/{id}",
		authenticate(
			middleware.RequireRole(model.RoleAdmin)(
				http.HandlerFunc(classHandler.Delete),
			),
		),
	)

	// Create a reusable school subject.
	mux.Handle(
		"POST /api/v1/admin/subjects",
		authenticate(
			middleware.RequireRole(model.RoleAdmin)(
				http.HandlerFunc(subjectHandler.Create),
			),
		),
	)

	// Get all available school subjects.
	mux.Handle(
		"GET /api/v1/admin/subjects",
		authenticate(
			middleware.RequireRole(model.RoleAdmin)(
				http.HandlerFunc(subjectHandler.GetAll),
			),
		),
	)

	// Update the name of an existing school subject.
	mux.Handle(
		"PATCH /api/v1/admin/subjects/{id}",
		authenticate(
			middleware.RequireRole(model.RoleAdmin)(
				http.HandlerFunc(subjectHandler.Update),
			),
		),
	)

	// Delete a school subject only when no academic records reference it.
	mux.Handle(
		"DELETE /api/v1/admin/subjects/{id}",
		authenticate(
			middleware.RequireRole(model.RoleAdmin)(
				http.HandlerFunc(subjectHandler.Delete),
			),
		),
	)

	// Enroll a student in a class for an academic session.
	mux.Handle(
		"POST /api/v1/admin/student-enrollments",
		authenticate(
			middleware.RequireRole(model.RoleAdmin)(
				http.HandlerFunc(resultHandler.EnrollTermStudent),
			),
		),
	)

	// Get students in a class for an academic session.
	mux.Handle(
		"GET /api/v1/admin/classes/{class_id}/students",
		authenticate(
			middleware.RequireRole(model.RoleAdmin)(
				http.HandlerFunc(studentEnrollmentHandler.GetStudentsByClass),
			),
		),
	)

	// Get a student's enrollment for an academic session.
	mux.Handle(
		"GET /api/v1/admin/students/{student_id}/enrollment",
		authenticate(
			middleware.RequireRole(model.RoleAdmin)(
				http.HandlerFunc(studentEnrollmentHandler.GetStudentEnrollment),
			),
		),
	)

	// Assign a teacher as the class teacher for a session.
	mux.Handle(
		"POST /api/v1/admin/class-teacher-assignments",
		authenticate(
			middleware.RequireRole(model.RoleAdmin)(
				http.HandlerFunc(resultHandler.AssignTermClassTeacher),
			),
		),
	)

	// Assign a teacher to teach a subject in a class for a session.
	mux.Handle(
		"POST /api/v1/admin/teacher-subject-assignments",
		authenticate(
			middleware.RequireRole(model.RoleAdmin)(
				http.HandlerFunc(resultHandler.AssignTermSubjectTeacher),
			),
		),
	)

	// Get the class teacher for a class and session.
	mux.Handle(
		"GET /api/v1/admin/classes/{class_id}/class-teacher",
		authenticate(
			middleware.RequireRole(model.RoleAdmin, model.RoleTeacher, model.RoleStudent)(
				http.HandlerFunc(teacherAssignmentHandler.GetClassTeacher),
			),
		),
	)

	// Get all subjects and classes assigned to a teacher in a session.
	mux.Handle(
		"GET /api/v1/admin/teachers/{teacher_id}/subject-assignments",
		authenticate(
			middleware.RequireRole(model.RoleAdmin, model.RoleTeacher)(
				http.HandlerFunc(teacherAssignmentHandler.GetTeacherSubjectAssignments),
			),
		),
	)

	for _, route := range []struct {
		pattern string
		role    model.Role
		handle  http.HandlerFunc
	}{
		{"GET /api/v1/admin/classes/{class_id}/subjects", model.RoleAdmin, resultHandler.Offerings},
		{"GET /api/v1/admin/classes/{class_id}/term-configuration", model.RoleAdmin, resultHandler.TermConfiguration},
		{"PATCH /api/v1/admin/classes/{class_id}/term-configuration", model.RoleAdmin, resultHandler.TermConfiguration},
		{"POST /api/v1/admin/term-initializations", model.RoleAdmin, resultHandler.InitializeTerm},
		{"PUT /api/v1/admin/student-promotions/decisions", model.RoleAdmin, resultHandler.SavePromotionDecisions},
		{"GET /api/v1/admin/student-promotions/decisions", model.RoleAdmin, resultHandler.ListPromotionDecisions},
		{"DELETE /api/v1/admin/student-promotions/decisions/{decision_id}", model.RoleAdmin, resultHandler.DeletePromotionDecision},
		{"POST /api/v1/admin/student-promotions/finalize", model.RoleAdmin, resultHandler.FinalizePromotions},
		{"GET /api/v1/admin/result-options", model.RoleAdmin, resultHandler.Options},
		{"GET /api/v1/teacher/result-options", model.RoleTeacher, resultHandler.Options},
		{"GET /api/v1/student/result-options", model.RoleStudent, resultHandler.Options},
		{"POST /api/v1/admin/classes/{class_id}/subjects", model.RoleAdmin, resultHandler.Offerings},
		{"DELETE /api/v1/admin/classes/{class_id}/subjects/{subject_id}", model.RoleAdmin, resultHandler.Offerings},
		{"PATCH /api/v1/teacher/results/{result_id}", model.RoleTeacher, resultHandler.EditDraft},
		{"GET /api/v1/teacher/result-batches", model.RoleTeacher, resultHandler.Review},
		{"POST /api/v1/teacher/result-batches/submit", model.RoleTeacher, resultHandler.TransitionBatch},
		{"GET /api/v1/admin/result-batches", model.RoleAdmin, resultHandler.Review},
		{"POST /api/v1/admin/result-batches/approve", model.RoleAdmin, resultHandler.TransitionBatch},
		{"POST /api/v1/admin/result-batches/reject", model.RoleAdmin, resultHandler.TransitionBatch},
		{"POST /api/v1/admin/result-publications", model.RoleAdmin, resultHandler.Publish},
	} {
		mux.Handle(route.pattern, authenticate(middleware.RequireRole(route.role)(route.handle)))
	}
	// Allow an assigned teacher to record a student's draft result.
	mux.Handle(
		"POST /api/v1/teacher/results",
		authenticate(
			middleware.RequireRole(model.RoleTeacher)(
				http.HandlerFunc(resultHandler.Create),
			),
		),
	)

	// Allow a teacher to view results for their assigned class and subject.
	mux.Handle(
		"GET /api/v1/teacher/results",
		authenticate(
			middleware.RequireRole(model.RoleTeacher)(
				http.HandlerFunc(resultHandler.GetTeacherResults),
			),
		),
	)

	// Allow a logged-in student to view only their own results.
	mux.Handle(
		"GET /api/v1/student/results",
		authenticate(
			middleware.RequireRole(model.RoleStudent)(
				http.HandlerFunc(resultHandler.GetMyResults),
			),
		),
	)
	mux.Handle("GET /api/v1/teacher/subject-assignments", authenticate(middleware.RequireRole(model.RoleTeacher)(http.HandlerFunc(teacherAssignmentHandler.GetMySubjectAssignments))))
	mux.Handle("GET /api/v1/teacher/classes/{class_id}/guardians", authenticate(middleware.RequireRole(model.RoleTeacher)(http.HandlerFunc(teacherAssignmentHandler.GetClassGuardians))))
	mux.Handle("GET /api/v1/admin/classes/{class_id}/guardians", authenticate(middleware.RequireRole(model.RoleAdmin)(http.HandlerFunc(teacherAssignmentHandler.GetClassGuardians))))
	// Attendance uses the existing authentication and role guards.
	for _, route := range []struct {
		pattern string
		role    model.Role
		handle  http.HandlerFunc
	}{
		{"POST /api/v1/teacher/attendance", model.RoleTeacher, attendanceHandler.Create},
		{"GET /api/v1/teacher/attendance", model.RoleTeacher, attendanceHandler.GetTeacher},
		{"GET /api/v1/student/attendance", model.RoleStudent, attendanceHandler.GetStudent},
		{"GET /api/v1/admin/attendance", model.RoleAdmin, attendanceHandler.GetAdmin},
		{"GET /api/v1/teacher/classes", model.RoleTeacher, attendanceHandler.GetClasses},
		{"GET /api/v1/teacher/classes/{class_id}/students", model.RoleTeacher, attendanceHandler.GetRoster},
	} {
		mux.Handle(route.pattern, authenticate(middleware.RequireRole(route.role)(route.handle)))
	}
	return httpresponse.Router(mux)
}
