package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/alcidheek/school-portal/internal/auth"

	"github.com/alcidheek/school-portal/internal/model"
	"github.com/alcidheek/school-portal/internal/repository"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

type UserService struct {
	userRepo    *repository.UserRepository
	jwtSecret   string
	resetSender ResetCodeSender
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func NewUserService(userRepo *repository.UserRepository, jwtSecret string, senders ...ResetCodeSender) *UserService {
	var sender ResetCodeSender
	if len(senders) > 0 {
		sender = senders[0]
	}
	return &UserService{
		resetSender: sender,
		userRepo:    userRepo,
		jwtSecret:   jwtSecret,
	}
}

type RegisterUserRequest struct {
	FullName      string
	Email         string
	Phone         string
	Password      string
	Role          model.Role
	GuardianName  string
	GuardianPhone string
}

func (s *UserService) RegisterUser(ctx context.Context, req RegisterUserRequest) (*model.User, error) {
	return s.registerUser(ctx, uuid.Nil, req)
}
func (s *UserService) RegisterUserAsAdmin(ctx context.Context, actor uuid.UUID, req RegisterUserRequest) (*model.User, error) {
	if actor == uuid.Nil {
		return nil, ErrForbiddenAccount
	}
	return s.registerUser(ctx, actor, req)
}
func (s *UserService) registerUser(ctx context.Context, actor uuid.UUID, req RegisterUserRequest) (*model.User, error) {
	fullName, err := validateName(req.FullName)
	if err != nil {
		return nil, err
	}
	email, err := normalizeUserEmail(req.Email)
	if err != nil {
		return nil, err
	}
	phone, err := validatePhone(req.Phone)
	if err != nil {
		return nil, err
	}
	role := model.Role(strings.ToUpper(strings.TrimSpace(string(req.Role))))
	if role != model.RoleAdmin && role != model.RoleTeacher && role != model.RoleStudent {
		return nil, fmt.Errorf("%w: invalid user role", ErrInvalidInput)
	}
	if len(req.Password) < 6 || len(req.Password) > 72 || strings.TrimSpace(req.Password) == "" {
		return nil, fmt.Errorf("%w: password must be 6 to 72 bytes", ErrInvalidInput)
	}
	if role == model.RoleAdmin {
		if err = validateStrongPassword(req.Password); err != nil {
			return nil, err
		}
	}
	user := &model.User{ID: uuid.New(), FullName: fullName, Email: email, Phone: phone, Role: role, IsActive: true}
	if role == model.RoleStudent {
		name, e := validateName(req.GuardianName)
		if e != nil {
			return nil, fmt.Errorf("%w: valid guardian name is required", ErrInvalidInput)
		}
		phone, e := validatePhone(req.GuardianPhone)
		if e != nil {
			return nil, fmt.Errorf("%w: valid guardian phone is required", ErrInvalidInput)
		}
		user.GuardianName = &name
		user.GuardianPhone = &phone
	} else if req.GuardianName != "" || req.GuardianPhone != "" {
		return nil, fmt.Errorf("%w: guardian details are only allowed for students", ErrInvalidInput)
	}
	tx, err := s.beginAccountTx(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if actor != uuid.Nil {
		if _, err = s.verifyAdminTx(ctx, tx, actor); err != nil {
			return nil, err
		}
	}
	exists, err := s.userRepo.EmailExistsTx(ctx, email, tx)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, ErrEmailConflict
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	user.PasswordHash = string(hash)
	if role == model.RoleStudent {
		number, e := s.userRepo.GenerateAdmissionNumberTx(ctx, tx)
		if e != nil {
			return nil, e
		}
		user.AdmissionNumber = &number
	} else {
		number, e := s.userRepo.GenerateEmployeeNumberTx(ctx, tx)
		if e != nil {
			return nil, e
		}
		user.EmployeeNumber = &number
	}
	if err = s.userRepo.CreateUserTx(ctx, user, tx); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	user.PasswordHash = ""
	return user, nil
}

func (s *UserService) Login(
	ctx context.Context,
	req LoginRequest,
) (string, *model.User, error) {

	email, validationErr := normalizeUserEmail(req.Email)

	if validationErr != nil {
		return "", nil, validationErr
	}
	if req.Password == "" {
		return "", nil, fmt.Errorf("%w: email and password are required", ErrInvalidInput)
	}

	user, err := s.userRepo.GetUserByEmail(ctx, email)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil, ErrInvalidCredentials
	}
	if err != nil {
		return "", nil, err
	}

	if user == nil {
		return "", nil, ErrInvalidCredentials
	}

	if err := bcrypt.CompareHashAndPassword(
		[]byte(user.PasswordHash),
		[]byte(req.Password),
	); err != nil {
		if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
			return "", nil, ErrInvalidCredentials
		}
		return "", nil, fmt.Errorf("verify password hash: %w", err)
	}
	// Reveal account status only after the supplied password has been verified.
	if !user.IsActive {
		return "", nil, ErrInactiveAccount
	}

	user.TokenVersion, err = s.userRepo.TokenVersionForLogin(ctx, user)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil, ErrInvalidCredentials
	}
	if err != nil {
		return "", nil, err
	}
	user.Email = strings.ToLower(strings.TrimSpace(user.Email))
	token, err := auth.GenerateToken(user, s.jwtSecret)
	if err != nil {
		return "", nil, err
	}

	return token, user, nil
}

func (s *UserService) BootstrapAdmin(ctx context.Context, fullName, email, phone, password string) error {
	tx, err := s.beginAccountTx(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	exists, err := s.userRepo.AdminExistsTx(ctx, tx)
	if err != nil {
		return err
	}
	if exists {
		return tx.Commit()
	}
	fullName, err = validateName(fullName)
	if err != nil {
		return err
	}
	email, err = normalizeUserEmail(email)
	if err != nil {
		return err
	}
	phone, err = validatePhone(phone)
	if err != nil {
		return err
	}
	if err = validateStrongPassword(password); err != nil {
		return err
	}
	exists, err = s.userRepo.EmailExistsTx(ctx, email, tx)
	if err != nil {
		return err
	}
	if exists {
		return ErrEmailConflict
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	number, err := s.userRepo.GenerateEmployeeNumberTx(ctx, tx)
	if err != nil {
		return err
	}
	admin := &model.User{ID: uuid.New(), FullName: fullName, Email: email, Phone: phone, PasswordHash: string(hash), Role: model.RoleAdmin, EmployeeNumber: &number, IsActive: true}
	if err = s.userRepo.CreateUserTx(ctx, admin, tx); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *UserService) ListUsers(ctx context.Context) ([]model.User, error) {
	users, err := s.userRepo.ListUsers(ctx)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}

	return users, nil
}

func (s *UserService) GetUserByID(
	ctx context.Context,
	userID uuid.UUID,
) (*model.User, error) {
	user, err := s.userRepo.GetUserByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get user by id: %w", err)
	}

	return user, nil
}

func (s *UserService) UpdateUser(ctx context.Context, id uuid.UUID, req model.UpdateUserRequest) error {
	return s.updateUser(ctx, uuid.Nil, id, req)
}

// The HTTP pathway checks the live administrator and prevents the general
// profile endpoint from bypassing current-password confirmation for self email.
func (s *UserService) UpdateUserAsAdmin(ctx context.Context, actor, id uuid.UUID, req model.UpdateUserRequest) error {
	if actor == uuid.Nil {
		return ErrForbiddenAccount
	}
	return s.updateUser(ctx, actor, id, req)
}
func (s *UserService) updateUser(ctx context.Context, actor, id uuid.UUID, req model.UpdateUserRequest) error {
	var err error
	for _, field := range []**string{&req.FullName, &req.GuardianName} {
		if *field != nil {
			value, e := validateName(**field)
			if e != nil {
				return e
			}
			*field = &value
		}
	}
	for _, field := range []**string{&req.Phone, &req.GuardianPhone} {
		if *field != nil {
			value, e := validatePhone(**field)
			if e != nil {
				return e
			}
			*field = &value
		}
	}
	if req.Email != nil {
		value, e := normalizeUserEmail(*req.Email)
		if e != nil {
			return e
		}
		req.Email = &value
	}
	tx, err := s.beginAccountTx(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if actor != uuid.Nil {
		if _, err = s.verifyAdminTx(ctx, tx, actor); err != nil {
			return err
		}
	}
	user, err := s.userRepo.GetUserByIDTx(ctx, id, tx)
	if err != nil {
		return err
	}
	if user.Role != model.RoleStudent && (req.GuardianName != nil || req.GuardianPhone != nil) {
		return fmt.Errorf("%w: guardian details are only allowed for students", ErrInvalidInput)
	}
	if req.Email != nil {
		if actor == id && *req.Email != strings.ToLower(strings.TrimSpace(user.Email)) {
			return fmt.Errorf("%w: use Account Security to change your email", ErrForbiddenAccount)
		}
		if *req.Email != strings.ToLower(strings.TrimSpace(user.Email)) {
			exists, e := s.userRepo.EmailExistsTx(ctx, *req.Email, tx)
			if e != nil {
				return e
			}
			if exists {
				return ErrEmailConflict
			}
		}
	}
	if user.Role == model.RoleAdmin && user.IsActive && req.IsActive != nil && !*req.IsActive {
		count, e := s.userRepo.CountActiveAdminsTx(ctx, tx)
		if e != nil {
			return e
		}
		if count <= 1 {
			return ErrFinalActiveAdmin
		}
	}
	if err = s.userRepo.UpdateUserTx(ctx, id, req, tx); err != nil {
		return err
	}
	if req.Email != nil {
		if err = s.userRepo.InvalidateResetCodesTx(ctx, tx, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}
