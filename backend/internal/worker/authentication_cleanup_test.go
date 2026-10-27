package worker

import (
	"bytes"
	"context"
	"errors"
	"log"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alcidheek/school-portal/internal/repository"
)

type cleanerFunc func(context.Context) (repository.AuthenticationCleanupCounts, error)

func (f cleanerFunc) Cleanup(ctx context.Context) (repository.AuthenticationCleanupCounts, error) {
	return f(ctx)
}

func TestCleanupDefaults(t *testing.T) {
	w := NewAuthenticationCleanup(nil, nil)
	if w.interval != 24*time.Hour || w.initialDelay != time.Minute || w.timeout != 30*time.Second {
		t.Fatal("unexpected production schedule")
	}
}

func TestCleanupFailureContinuesAndCancellationStops(t *testing.T) {
	var calls atomic.Int32
	var output bytes.Buffer
	reached := make(chan struct{})
	w := NewAuthenticationCleanup(cleanerFunc(func(context.Context) (repository.AuthenticationCleanupCounts, error) {
		if calls.Add(1) == 1 {
			return repository.AuthenticationCleanupCounts{}, errors.New("private SQL token secret")
		}
		if calls.Load() == 2 {
			close(reached)
		}
		return repository.AuthenticationCleanupCounts{ResetCodes: 2, RevokedTokens: 3, ResetStates: 1}, nil
	}), log.New(&output, "", 0))
	w.interval = 10 * time.Millisecond
	w.initialDelay = 0
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() { defer close(done); w.Run(ctx) }()
	select {
	case <-reached:
	case <-time.After(time.Second):
		t.Fatal("worker did not recover from failure")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("worker did not stop")
	}
	previous := calls.Load()
	time.Sleep(30 * time.Millisecond)
	if calls.Load() != previous {
		t.Fatal("cleanup executed after cancellation")
	}
	if !strings.Contains(output.String(), "cleanup failed") || strings.Contains(output.String(), "secret") || strings.Contains(output.String(), "private SQL") {
		t.Fatal("unsafe error logging")
	}
}

func TestCleanupTimeoutAndNoOverlap(t *testing.T) {
	var active, maxActive, calls atomic.Int32
	reached := make(chan struct{})
	w := NewAuthenticationCleanup(cleanerFunc(func(ctx context.Context) (repository.AuthenticationCleanupCounts, error) {
		count := active.Add(1)
		if count > maxActive.Load() {
			maxActive.Store(count)
		}
		defer active.Add(-1)
		if calls.Add(1) == 2 {
			close(reached)
		}
		if _, ok := ctx.Deadline(); !ok {
			t.Error("database operation lacks a timeout")
		}
		<-ctx.Done()
		return repository.AuthenticationCleanupCounts{}, ctx.Err()
	}), log.New(&bytes.Buffer{}, "", 0))
	w.interval = time.Millisecond
	w.initialDelay = 0
	w.timeout = 10 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); w.Run(ctx) }()
	select {
	case <-reached:
	case <-time.After(time.Second):
		cancel()
		t.Fatal("timeout prevented subsequent run")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("active operation not cancelled")
	}
	if maxActive.Load() != 1 {
		t.Fatal("overlapping executions")
	}
}

func TestCancelledWorkerDoesNotExecute(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	w := NewAuthenticationCleanup(cleanerFunc(func(context.Context) (repository.AuthenticationCleanupCounts, error) {
		t.Fatal("cleanup executed")
		return repository.AuthenticationCleanupCounts{}, nil
	}), log.Default())
	w.Run(ctx)
}
