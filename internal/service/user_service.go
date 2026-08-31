package service

import (
	"context"
	"errors"
	"log"
	"strings"

	"github.com/alcidheek/school-portal/internal/auth"

	"github.com/alcidheek/school-portal/internal/model"
	"github.com/alcidheek/school-portal/internal/repository"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

type UserService struct {
	userRepo  *repository.UserRepository
	jwtSecret string
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func NewUserService(userRepo *repository.UserRepository, jwtSecret string) *UserService {
	return &UserService{
		userRepo:  userRepo,
		jwtSecret: jwtSecret,
	}
}

type RegisterUserRequest struct {
	FullName string
	Email    string
	Phone    string
	Password string
	Role     model.Role
}

func (s *UserService) RegisterUser(
	ctx context.Context,
	req RegisterUserRequest,
) (*model.User, error) {

	// 1. Clean input
	fullName := strings.TrimSpace(req.FullName)
	email := strings.ToLower(strings.TrimSpace(req.Email))
	phone := strings.TrimSpace(req.Phone)

	// 2. Validate required fields
	if fullName == "" {
		return nil, errors.New("full name is required")
	}

	if email == "" {
		return nil, errors.New("email is required")
	}

	if phone == "" {
		return nil, errors.New("phone is required")
	}

	if req.Password == "" {
		return nil, errors.New("password is required")
	}

	if len(req.Password) < 6 {
		return nil, errors.New("password must be at least 6 characters")
	}

	role := model.Role(strings.ToUpper(strings.TrimSpace(string(req.Role))))
	log.Printf("ROLE RECEIVED: %q", role)

	// Validate role
	if role != model.RoleAdmin &&
		role != model.RoleTeacher &&
		role != model.RoleStudent {
		return nil, errors.New("invalid user role")
	}

	// 4. Check if email already exists
	exists, err := s.userRepo.EmailExists(ctx, email)
	if err != nil {
		return nil, err
	}

	if exists {
		return nil, errors.New("email already exists")
	}

	// 5. Hash password
	passwordHash, err := bcrypt.GenerateFromPassword(
		[]byte(req.Password),
		bcrypt.DefaultCost,
	)
	if err != nil {
		return nil, err
	}

	// 6. Create user
	user := &model.User{
		ID:           uuid.New(),
		FullName:     fullName,
		Email:        email,
		Phone:        phone,
		PasswordHash: string(passwordHash),
		Role:         role,
		IsActive:     true,
	}

	switch role {

	case model.RoleStudent:
		admissionNumber, err := s.userRepo.GenerateAdmissionNumber(ctx)
		if err != nil {
			return nil, err
		}

		user.AdmissionNumber = &admissionNumber

	case model.RoleTeacher, model.RoleAdmin:
		employeeNumber, err := s.userRepo.GenerateEmployeeNumber(ctx)
		if err != nil {
			return nil, err
		}

		user.EmployeeNumber = &employeeNumber
	}

	// 7. Save user
	if err := s.userRepo.CreateUser(ctx, user); err != nil {
		return nil, err
	}

	// Never return password hash
	user.PasswordHash = ""

	return user, nil
}

func (s *UserService) Login(
	ctx context.Context,
	req LoginRequest,
) (string, *model.User, error) {

	email := strings.TrimSpace(strings.ToLower(req.Email))

	if email == "" || req.Password == "" {
		return "", nil, errors.New("email and password are required")
	}

	user, err := s.userRepo.GetUserByEmail(ctx, email)
	if err != nil {
		return "", nil, err
	}

	if user == nil {
		return "", nil, errors.New("invalid email or password")
	}

	if !user.IsActive {
		return "", nil, errors.New("user account is inactive")
	}

	if err := bcrypt.CompareHashAndPassword(
		[]byte(user.PasswordHash),
		[]byte(req.Password),
	); err != nil {
		return "", nil, errors.New("invalid email or password")
	}

	token, err := auth.GenerateToken(user, s.jwtSecret)
	if err != nil {
		return "", nil, err
	}

	return token, user, nil
}
