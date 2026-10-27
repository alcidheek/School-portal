package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/alcidheek/school-portal/internal/model"
	"github.com/alcidheek/school-portal/internal/repository"
	"github.com/google/uuid"
	"github.com/lib/pq"
)

var (
	ErrInvalidAcademicInput    = errors.New("invalid academic input")
	ErrAcademicSessionNotFound = errors.New("academic session not found")
	ErrActiveTermNotFound      = errors.New("active term not found")
	ErrAcademicSessionInactive = errors.New("academic session is not active")
	ErrNoAvailableTerm         = errors.New("academic session has no available term")
	ErrTermDatesConflict       = errors.New("requested dates conflict with existing term dates")
	ErrTermSessionMismatch     = errors.New("term does not belong to academic session")
	ErrAcademicConflict        = errors.New("academic lifecycle conflict")
)

// One stable school-wide key serializes every lifecycle transaction across
// application instances. Acquire it before any lifecycle DB reads or writes.
const academicLifecycleLockKey int64 = 0x5343484f4f4c01

type AcademicSessionService struct {
	sessionRepo *repository.AcademicSessionRepository
	termRepo    *repository.TermRepository
}

func NewAcademicSessionService(sessionRepo *repository.AcademicSessionRepository, termRepo *repository.TermRepository) *AcademicSessionService {
	return &AcademicSessionService{sessionRepo: sessionRepo, termRepo: termRepo}
}

func (s *AcademicSessionService) beginLifecycle(ctx context.Context) (*sql.Tx, error) {
	tx, err := s.sessionRepo.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin academic transaction: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, academicLifecycleLockKey); err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("lock academic lifecycle: %w", err)
	}
	return tx, nil
}

func academicStorageError(err error) error {
	var pgErr *pq.Error
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return fmt.Errorf("%w: %w", ErrAcademicConflict, err)
	}
	return err
}

func parseAcademicDates(start, end, label string) (time.Time, time.Time, error) {
	a, err := time.Parse("2006-01-02", start)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("%w: invalid %s start date", ErrInvalidAcademicInput, label)
	}
	b, err := time.Parse("2006-01-02", end)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("%w: invalid %s end date", ErrInvalidAcademicInput, label)
	}
	if !b.After(a) {
		return time.Time{}, time.Time{}, fmt.Errorf("%w: %s end date must be after start date", ErrInvalidAcademicInput, label)
	}
	return a, b, nil
}

func validateTermName(name string) error {
	switch model.TermName(name) {
	case model.FirstTerm, model.SecondTerm, model.ThirdTerm:
		return nil
	}
	return fmt.Errorf("%w: invalid term name", ErrInvalidAcademicInput)
}

func validateTermDates(session *model.AcademicSession, start, end time.Time) error {
	if !end.After(start) || start.Before(session.StartDate) || end.After(session.EndDate) {
		return fmt.Errorf("%w: term dates must fall within academic session dates and end after start", ErrInvalidAcademicInput)
	}
	return nil
}

func (s *AcademicSessionService) sessionTx(ctx context.Context, tx *sql.Tx, id uuid.UUID) (*model.AcademicSession, error) {
	session, err := s.sessionRepo.GetByIDTx(ctx, tx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrAcademicSessionNotFound
	}
	return session, err
}

// Remember before clearing activity. A missing active term never erases history.
func (s *AcademicSessionService) preserveOutgoing(ctx context.Context, tx *sql.Tx) error {
	session, err := s.sessionRepo.GetActiveTx(ctx, tx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	term, err := s.termRepo.GetActiveBySessionIDTx(ctx, tx, session.ID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	return s.sessionRepo.SetLastActiveTermTx(ctx, tx, session.ID, uuid.NullUUID{UUID: term.ID, Valid: true})
}

func (s *AcademicSessionService) deactivateCurrent(ctx context.Context, tx *sql.Tx) error {
	if err := s.preserveOutgoing(ctx, tx); err != nil {
		return err
	}
	if err := s.termRepo.DeactivateAllActiveTx(ctx, tx); err != nil {
		return err
	}
	return s.sessionRepo.DeactivateActiveTx(ctx, tx)
}

func (s *AcademicSessionService) Create(ctx context.Context, name, startDateStr, endDateStr string, activate bool, termName, termStartDateStr, termEndDateStr string) (*model.AcademicSession, error) {
	if strings.TrimSpace(name) == "" {
		return nil, fmt.Errorf("%w: session name is required", ErrInvalidAcademicInput)
	}
	start, end, err := parseAcademicDates(startDateStr, endDateStr, "session")
	if err != nil {
		return nil, err
	}
	if err := validateTermName(termName); err != nil {
		return nil, err
	}
	termStart, termEnd, err := parseAcademicDates(termStartDateStr, termEndDateStr, "term")
	if err != nil {
		return nil, err
	}
	session := &model.AcademicSession{ID: uuid.New(), Name: name, StartDate: start, EndDate: end, IsActive: activate}
	if err := validateTermDates(session, termStart, termEnd); err != nil {
		return nil, err
	}
	term := &model.Term{ID: uuid.New(), AcademicSessionID: session.ID, Name: model.TermName(termName), StartDate: termStart, EndDate: termEnd, IsActive: activate}
	tx, err := s.beginLifecycle(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if activate {
		if err := s.deactivateCurrent(ctx, tx); err != nil {
			return nil, err
		}
	}
	if err := s.sessionRepo.CreateTx(ctx, tx, session); err != nil {
		return nil, academicStorageError(err)
	}
	if err := s.termRepo.CreateTx(ctx, tx, term); err != nil {
		return nil, academicStorageError(err)
	}
	if activate {
		if err := s.sessionRepo.SetLastActiveTermTx(ctx, tx, session.ID, uuid.NullUUID{UUID: term.ID, Valid: true}); err != nil {
			return nil, err
		}
	}
	session, err = s.sessionTx(ctx, tx, session.ID)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, academicStorageError(err)
	}
	return session, nil
}

func (s *AcademicSessionService) List(ctx context.Context) ([]model.AcademicSession, error) {
	return s.sessionRepo.List(ctx)
}

// Delete removes only unused, inactive sessions. Both advisory locks are taken
// before row locks so lifecycle changes and academic writes cannot race checks.
func (service *AcademicSessionService) Delete(requestContext context.Context, sessionID uuid.UUID) error {
	if sessionID == uuid.Nil {
		return fmt.Errorf("%w: academic session id is required", ErrInvalidAcademicInput)
	}

	transaction, err := service.beginLifecycle(requestContext)
	if err != nil {
		return err
	}
	defer transaction.Rollback()

	// Keep the existing lifecycle lock first, then the shared academic write lock.
	if _, err := transaction.ExecContext(requestContext, `SELECT pg_advisory_xact_lock(6006001)`); err != nil {
		return fmt.Errorf("lock academic records for session deletion: %w", err)
	}

	session, err := service.sessionRepo.GetByIDForUpdateTx(requestContext, transaction, sessionID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrAcademicSessionNotFound
	}
	if err != nil {
		return err
	}
	if session.IsActive {
		return fmt.Errorf("%w: active academic session cannot be deleted", ErrConflict)
	}

	hasActiveTerm, err := service.sessionRepo.LockTermsForDeletionTx(requestContext, transaction, sessionID)
	if err != nil {
		return err
	}
	if hasActiveTerm {
		return fmt.Errorf("%w: academic session with an active term cannot be deleted", ErrConflict)
	}

	hasProtectedRecords, err := service.sessionRepo.HasProtectedAcademicRecordsTx(requestContext, transaction, sessionID)
	if err != nil {
		return err
	}
	if hasProtectedRecords {
		return fmt.Errorf("%w: academic session has protected academic records or history", ErrConflict)
	}

	if err := service.sessionRepo.DeleteUnusedSessionTx(requestContext, transaction, sessionID); err != nil {
		return academicSessionDeletionError(err)
	}
	return academicSessionDeletionError(transaction.Commit())
}

// Deletion conflicts use a safe domain message rather than the shared foreign
// key error mapping, which describes missing references as not found.
func academicSessionDeletionError(err error) error {
	var databaseError *pq.Error
	if errors.As(err, &databaseError) && (databaseError.Code == "23503" || databaseError.Code == "P6001") {
		return fmt.Errorf("%w: academic session is referenced by protected records or locked configuration", ErrConflict)
	}
	return err
}

func (service *AcademicSessionService) ListTerms(ctx context.Context, sessionID uuid.UUID) ([]model.Term, error) {
	if _, err := service.sessionRepo.GetByID(ctx, sessionID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrAcademicSessionNotFound
		}
		return nil, err
	}
	return service.termRepo.ListBySessionID(ctx, sessionID)
}

func (s *AcademicSessionService) SwitchTerm(ctx context.Context, sessionID uuid.UUID, termName, startDateStr, endDateStr string) (*model.Term, error) {
	if err := validateTermName(termName); err != nil {
		return nil, err
	}
	start, end, err := parseAcademicDates(startDateStr, endDateStr, "term")
	if err != nil {
		return nil, err
	}
	tx, err := s.beginLifecycle(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	session, err := s.sessionTx(ctx, tx, sessionID)
	if err != nil {
		return nil, err
	}
	if !session.IsActive {
		return nil, ErrAcademicSessionInactive
	}
	if err := validateTermDates(session, start, end); err != nil {
		return nil, err
	}
	term, err := s.termRepo.GetBySessionAndNameTx(ctx, tx, sessionID, model.TermName(termName))
	create := errors.Is(err, sql.ErrNoRows)
	if err != nil && !create {
		return nil, err
	}
	if create {
		term = &model.Term{ID: uuid.New(), AcademicSessionID: sessionID, Name: model.TermName(termName), StartDate: start, EndDate: end, IsActive: true}
	} else {
		if term.AcademicSessionID != sessionID {
			return nil, ErrTermSessionMismatch
		}
		if err := validateTermDates(session, term.StartDate, term.EndDate); err != nil {
			return nil, err
		}
		if !term.StartDate.Equal(start) || !term.EndDate.Equal(end) {
			return nil, ErrTermDatesConflict
		}
	}
	if err := s.termRepo.DeactivateAllActiveTx(ctx, tx); err != nil {
		return nil, err
	}
	if create {
		if err := s.termRepo.CreateTx(ctx, tx, term); err != nil {
			return nil, academicStorageError(err)
		}
	} else if err := s.termRepo.ActivateTx(ctx, tx, sessionID, term.ID); err != nil {
		return nil, academicStorageError(err)
	}
	if err := s.sessionRepo.SetLastActiveTermTx(ctx, tx, sessionID, uuid.NullUUID{UUID: term.ID, Valid: true}); err != nil {
		return nil, err
	}
	term, err = s.termRepo.GetByIDTx(ctx, tx, term.ID)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, academicStorageError(err)
	}
	return term, nil
}

func (s *AcademicSessionService) restorationTerm(ctx context.Context, tx *sql.Tx, session *model.AcademicSession) (*model.Term, error) {
	// Repeated activation retains the current term even when legacy memory is null.
	if session.IsActive {
		term, err := s.termRepo.GetActiveBySessionIDTx(ctx, tx, session.ID)
		if err == nil {
			return term, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
	}
	if session.LastActiveTermID.Valid {
		term, err := s.termRepo.GetByIDTx(ctx, tx, session.LastActiveTermID.UUID)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNoAvailableTerm
		}
		return term, err
	}
	term, err := s.termRepo.GetFirstBySessionIDTx(ctx, tx, session.ID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNoAvailableTerm
	}
	return term, err
}

func (s *AcademicSessionService) Activate(ctx context.Context, sessionID uuid.UUID) error {
	tx, err := s.beginLifecycle(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	session, err := s.sessionTx(ctx, tx, sessionID)
	if err != nil {
		return err
	}
	term, err := s.restorationTerm(ctx, tx, session)
	if err != nil {
		return err
	}
	if term.AcademicSessionID != sessionID {
		return ErrTermSessionMismatch
	}
	if err := validateTermDates(session, term.StartDate, term.EndDate); err != nil {
		return err
	}
	if err := s.deactivateCurrent(ctx, tx); err != nil {
		return err
	}
	if err := s.sessionRepo.ActivateTx(ctx, tx, sessionID); err != nil {
		return academicStorageError(err)
	}
	if err := s.termRepo.ActivateTx(ctx, tx, sessionID, term.ID); err != nil {
		return academicStorageError(err)
	}
	if err := s.sessionRepo.SetLastActiveTermTx(ctx, tx, sessionID, uuid.NullUUID{UUID: term.ID, Valid: true}); err != nil {
		return err
	}
	return academicStorageError(tx.Commit())
}

// Read both records in the same locked transaction so a concurrent switch cannot
// cause the API to combine two different lifecycle states.
func (s *AcademicSessionService) GetActiveTerm(ctx context.Context, sessionID uuid.UUID) (*model.Term, error) {
	tx, err := s.beginLifecycle(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	session, err := s.sessionTx(ctx, tx, sessionID)
	if err != nil {
		return nil, err
	}
	if !session.IsActive {
		return nil, ErrActiveTermNotFound
	}
	term, err := s.termRepo.GetActiveBySessionIDTx(ctx, tx, sessionID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrActiveTermNotFound
	}
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return term, nil
}
