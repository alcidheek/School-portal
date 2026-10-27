package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/alcidheek/school-portal/internal/worker"

	"github.com/alcidheek/school-portal/internal/middleware"

	"github.com/alcidheek/school-portal/config"
	"github.com/alcidheek/school-portal/database"
	"github.com/alcidheek/school-portal/internal/handler"
	"github.com/alcidheek/school-portal/internal/mail"
	"github.com/alcidheek/school-portal/internal/repository"
	"github.com/alcidheek/school-portal/internal/service"
	"github.com/alcidheek/school-portal/internal/storage"
	"github.com/alcidheek/school-portal/routes"
)

func main() {

	// Load configuration
	log.Println("Loading config...")

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	// Verify private media storage before initializing application dependencies.
	mediaStorage, err := storage.NewMinIOStorage(context.Background(),
		cfg.MinIOEndpoint, cfg.MinIOAccessKey, cfg.MinIOSecretKey,
		cfg.MinIOBucket, cfg.MinIOUseSSL)
	if err != nil {
		log.Fatalf("initialize media storage: %v", err)
	}
	defer mediaStorage.Close()
	log.Println("MinIO bucket access verified")

	// Connect to database
	log.Println("Connecting to database...")

	db := database.Connect(cfg)
	defer db.Close()

	log.Println("Database connected successfully")

	// Run database migrations
	log.Println("Running database migrations...")

	if err := database.RunMigrations(db); err != nil {
		log.Fatalf("run migrations: %v", err)
	}

	log.Println("Database migrations completed")

	// Repository
	userRepo := repository.NewUserRepository(db)
	academicSessionRepo := repository.NewAcademicSessionRepository(db)
	termRepo := repository.NewTermRepository(db)
	classRepo := repository.NewClassRepository(db)
	subjectRepo := repository.NewSubjectRepository(db)
	studentEnrollmentRepo := repository.NewStudentEnrollmentRepository(db)
	teacherAssignmentRepo := repository.NewTeacherAssignmentRepository(db)
	resultRepo := repository.NewResultRepository(db)
	attendanceRepo := repository.NewAttendanceRepository(db)
	schoolProfileRepo := repository.NewSchoolProfileRepository(db)

	// Service
	resetSender, err := mail.NewSMTPSender(cfg.SMTPHost, cfg.SMTPPort, cfg.SMTPEmail, cfg.SMTPAppPassword)
	if err != nil {
		log.Fatalf("configure SMTP: %v", err)
	}
	userService := service.NewUserService(userRepo, cfg.JWTSecret, resetSender)
	academicSessionService := service.NewAcademicSessionService(academicSessionRepo, termRepo)
	classService := service.NewClassService(classRepo)
	subjectService := service.NewSubjectService(subjectRepo)
	schoolProfileService := service.NewSchoolProfileService(schoolProfileRepo)
	mediaService := service.NewMediaService(mediaStorage, schoolProfileRepo, userRepo)
	studentEnrollmentService := service.NewStudentEnrollmentService(
		studentEnrollmentRepo,
		userRepo,
		classRepo,
		academicSessionRepo,
	)
	teacherAssignmentService := service.NewTeacherAssignmentService(
		teacherAssignmentRepo,
		userRepo,
		classRepo,
		subjectRepo,
		academicSessionRepo,
	)
	resultService := service.NewResultService(
		resultRepo,
		userRepo,
		subjectRepo,
		classRepo,
		academicSessionRepo,
		termRepo,
		studentEnrollmentRepo,
		teacherAssignmentRepo,
	)

	log.Println("Checking initial admin...")

	err = userService.BootstrapAdmin(
		context.Background(),
		cfg.InitialAdminFullName,
		cfg.InitialAdminEmail,
		cfg.InitialAdminPhone,
		cfg.InitialAdminPassword,
	)
	if err != nil {
		log.Fatalf("bootstrap admin: %v", err)
	}

	log.Println("Initial admin check completed")

	// Handler
	userHandler := handler.NewUserHandler(userService)
	academicSessionHandler := handler.NewAcademicSessionHandler(academicSessionService)
	classHandler := handler.NewClassHandler(classService)
	subjectHandler := handler.NewSubjectHandler(subjectService)
	schoolProfileHandler := handler.NewSchoolProfileHandler(schoolProfileService)
	mediaHandler := handler.NewMediaHandler(mediaService)
	studentEnrollmentHandler := handler.NewStudentEnrollmentHandler(studentEnrollmentService)
	teacherAssignmentHandler := handler.NewTeacherAssignmentHandler(teacherAssignmentService)
	resultHandler := handler.NewResultHandler(resultService)
	attendanceHandler := handler.NewAttendanceHandler(service.NewAttendanceService(
		attendanceRepo, studentEnrollmentRepo, teacherAssignmentRepo, termRepo, userRepo,
	))
	// Routes
	limiters := routes.NewAuthenticationRateLimiters()
	router := routes.SetupRoutesWithMedia(
		userHandler,
		academicSessionHandler,
		classHandler,
		subjectHandler,
		studentEnrollmentHandler,
		teacherAssignmentHandler,
		resultHandler,
		attendanceHandler,
		schoolProfileHandler,
		mediaHandler,
		cfg.JWTSecret,
		limiters,
	)

	// Start server
	log.Println("Server running on port 8080")

	// Allow the React frontend to communicate with the backend.
	corsRouter := middleware.CORSMiddleware(router)

	// Signals stop maintenance and drain HTTP requests before closing PostgreSQL.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	workerCtx, cancelWorker := context.WithCancel(ctx)
	rateCleanupDone := make(chan struct{})
	go func() {
		defer close(rateCleanupDone)
		middleware.RunRateLimiterCleanup(workerCtx, limiters.Login, limiters.ForgotPassword, limiters.ResetPassword)
	}()
	cleanup := worker.NewAuthenticationCleanup(service.NewAuthenticationCleanupService(
		repository.NewAuthenticationCleanupRepository(db)), log.Default())
	workerDone := make(chan struct{})
	go func() { defer close(workerDone); cleanup.Run(workerCtx) }()
	server := &http.Server{Addr: ":8080", Handler: corsRouter}
	serverErrors := make(chan error, 1)
	go func() { serverErrors <- server.ListenAndServe() }()
	var serverFailure error
	select {
	case <-ctx.Done():
		log.Println("Server shutdown requested")
	case err := <-serverErrors:
		if !errors.Is(err, http.ErrServerClosed) {
			serverFailure = err
			log.Println("HTTP server stopped unexpectedly")
		}
	}
	cancelWorker()
	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelShutdown()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Println("HTTP shutdown timed out or failed; closing remaining connections")
		server.Close()
	}
	<-workerDone
	<-rateCleanupDone
	log.Println("Server and authentication cleanup worker stopped")
	if serverFailure != nil {
		// Fatal exits skip defers: close PostgreSQL explicitly after the worker
		// has stopped, preserving the previous nonzero server-failure exit.
		db.Close()
		log.Fatal("HTTP server failed")
	}
}
