package service

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

type preflightExecution struct {
	workers.Service
	failure                 error
	validations, executions atomic.Int32
	started                 chan struct{}
	release                 <-chan struct{}
}

func (e *preflightExecution) ValidateExecution(ctx context.Context, _ workers.ExecuteRequest) error {
	e.validations.Add(1)
	if e.started != nil {
		e.started <- struct{}{}
		select {
		case <-e.release:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return e.failure
}

func (e *preflightExecution) Execute(_ context.Context, request workers.ExecuteRequest) (workers.ExecuteResult, error) {
	e.executions.Add(1)
	return workers.ExecuteResult{Correlation: request.Correlation, Outcome: workers.ExecutionOutcomeAccepted}, nil
}

func newPreflightRegistry(t *testing.T, executor workers.Service) (*registry, workersessions.StartRequest) {
	t.Helper()
	service, err := New(executor, newEventsAppender(), logging.NoopLogger{}, platformclock.Real{}, platformclock.Real{}, nil, unavailableWorkerControlStore{}, unavailableWorkerControlStore{}, continuationInspectionFake{})
	if err != nil {
		t.Fatal(err)
	}
	r := service.(*registry)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := r.Stop(ctx); err != nil {
			t.Error(err)
		}
	})
	return r, workersessions.StartRequest{RequestID: "request", ID: "worker", Execution: dispatchHandoff("attempt")}
}

func TestT7StartPreflightRejectsBeforeIdentityOpeningAndExecution(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name        string
		cause, want error
	}{
		{"unsupported", providers.ExecuteFailure{Kind: providers.ExecuteFailureKindInvalidRequest}, workersessions.ErrInvalidExecutionRequest},
		{"unavailable", providers.ErrProviderUnavailable, workersessions.ErrStartAdmissionFailed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			executor := &preflightExecution{failure: tc.cause}
			r, req := newPreflightRegistry(t, executor)
			if _, err := r.Start(t.Context(), req); !errors.Is(err, tc.want) {
				t.Fatalf("Start = %v, want %v", err, tc.want)
			}
			if _, err := r.Get(t.Context(), workersessions.GetRequest{ID: req.ID}); !errors.Is(err, workersessions.ErrSessionNotFound) {
				t.Fatalf("Get after rejected preflight = %v", err)
			}
			if executor.executions.Load() != 0 {
				t.Fatal("rejected preflight executed worker")
			}
			executor.failure = nil
			if result, err := r.Start(t.Context(), req); err != nil || result.Session.ID != req.ID {
				t.Fatalf("retry = %#v, %v", result, err)
			}
			if err := r.waitForSupervisionDriver(t.Context(), req.ID); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestT7StartAcceptedReplaySkipsChangedReadinessAndConflicts(t *testing.T) {
	t.Parallel()
	executor := &preflightExecution{}
	r, req := newPreflightRegistry(t, executor)
	first, err := r.Start(t.Context(), req)
	if err != nil {
		t.Fatal(err)
	}
	executor.failure = providers.ErrProviderUnavailable
	replayed, err := r.Start(t.Context(), req)
	if err != nil || replayed.Session.ID != first.Session.ID {
		t.Fatalf("replay = %#v, %v", replayed, err)
	}
	req.Execution.Execution.Model = "changed-model"
	if _, err := r.Start(t.Context(), req); !errors.Is(err, workersessions.ErrStartRequestIDConflict) {
		t.Fatalf("changed tuple = %v", err)
	}
	if executor.validations.Load() != 1 {
		t.Fatalf("preflight calls = %d, want 1", executor.validations.Load())
	}
	if err := r.waitForSupervisionDriver(t.Context(), req.ID); err != nil {
		t.Fatal(err)
	}
}

func TestT7ConcurrentPreflightReservesAndExecutesOnce(t *testing.T) {
	t.Parallel()
	release := make(chan struct{})
	executor := &preflightExecution{started: make(chan struct{}, 2), release: release}
	r, req := newPreflightRegistry(t, executor)
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			result, err := r.Start(ctx, req)
			if err == nil && result.Session.ID != req.ID {
				err = errors.New("replay identity changed")
			}
			results <- err
		}()
	}
	for i := 0; i < 2; i++ {
		select {
		case <-executor.started:
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	close(release)
	for i := 0; i < 2; i++ {
		select {
		case err := <-results:
			if err != nil {
				t.Fatal(err)
			}
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	if err := r.waitForSupervisionDriver(ctx, req.ID); err != nil {
		t.Fatal(err)
	}
	if executor.executions.Load() != 1 {
		t.Fatalf("concurrent attempts = %d", executor.executions.Load())
	}
}
