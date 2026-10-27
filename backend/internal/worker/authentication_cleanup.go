package worker

import (
	"context"
	"errors"
	"time"

	"github.com/alcidheek/school-portal/internal/repository"
)

type authenticationCleaner interface {
	Cleanup(context.Context) (repository.AuthenticationCleanupCounts, error)
}
type cleanupLogger interface{ Printf(string, ...any) }

type AuthenticationCleanup struct {
	cleaner                         authenticationCleaner
	logger                          cleanupLogger
	interval, initialDelay, timeout time.Duration
}

func NewAuthenticationCleanup(cleaner authenticationCleaner, logger cleanupLogger) *AuthenticationCleanup {
	return &AuthenticationCleanup{cleaner: cleaner, logger: logger, interval: 24 * time.Hour, initialDelay: time.Minute, timeout: 30 * time.Second}
}

// Run is called once by startup and executes each cleanup synchronously. A slow
// execution cannot overlap the next tick; the database lock also guards instances.
func (w *AuthenticationCleanup) Run(ctx context.Context) {
	if ctx.Err() != nil {
		return
	}
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	initial := time.NewTimer(w.initialDelay)
	defer initial.Stop()
	initialTick := initial.C
	for {
		select {
		case <-ctx.Done():
			return
		case <-initialTick:
			initialTick = nil
		case <-ticker.C:
		}
		if ctx.Err() != nil {
			return
		}
		runCtx, cancel := context.WithTimeout(ctx, w.timeout)
		counts, err := w.cleaner.Cleanup(runCtx)
		cancel()
		if ctx.Err() != nil {
			return
		}
		switch {
		case errors.Is(err, context.DeadlineExceeded):
			w.logger.Printf("Authentication cleanup timed out")
		case err != nil:
			w.logger.Printf("Authentication cleanup failed; database operation or transaction error")
		case counts.Skipped:
			w.logger.Printf("Authentication cleanup skipped; account maintenance lock is busy")
		default:
			w.logger.Printf("Authentication cleanup completed: reset_codes=%d revoked_tokens=%d reset_states=%d", counts.ResetCodes, counts.RevokedTokens, counts.ResetStates)
		}
	}
}
