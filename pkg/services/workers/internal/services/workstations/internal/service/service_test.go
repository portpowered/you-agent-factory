package service

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/workers"
	workstations "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations"
)

func TestPoolLifecycleMakesOnlyStartedRoutesAvailable(t *testing.T) {
	t.Parallel()

	pool := New()
	if err := pool.Route(context.Background(), "review"); !errors.Is(err, workers.ErrWorkstationPoolUnavailable) {
		t.Fatalf("constructed route error = %v, want ErrWorkstationPoolUnavailable", err)
	}

	outcome, err := pool.Start(context.Background(), []workstations.Route{
		{WorkstationName: "review"},
		{WorkstationName: "implement"},
	})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if outcome != workers.WorkstationPoolLifecycleOutcomeStarted {
		t.Fatalf("Start() outcome = %q, want STARTED", outcome)
	}
	if err := pool.Route(context.Background(), "review"); err != nil {
		t.Fatalf("started route error = %v", err)
	}
	if err := pool.Route(context.Background(), "missing"); !errors.Is(err, workers.ErrUnknownWorkstationRoute) {
		t.Fatalf("unknown route error = %v, want ErrUnknownWorkstationRoute", err)
	}

	outcome, err = pool.Start(context.Background(), []workstations.Route{{WorkstationName: "other"}})
	if err != nil {
		t.Fatalf("repeated Start() error = %v", err)
	}
	if outcome != workers.WorkstationPoolLifecycleOutcomeAlreadyRunning {
		t.Fatalf("repeated Start() outcome = %q, want ALREADY_RUNNING", outcome)
	}
	if err := pool.Route(context.Background(), "other"); !errors.Is(err, workers.ErrUnknownWorkstationRoute) {
		t.Fatalf("repeated start replaced routes: error = %v", err)
	}

	outcome, err = pool.Stop(context.Background())
	if err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if outcome != workers.WorkstationPoolLifecycleOutcomeStopped {
		t.Fatalf("Stop() outcome = %q, want STOPPED", outcome)
	}
	if err := pool.Route(context.Background(), "review"); !errors.Is(err, workers.ErrWorkstationPoolStopped) {
		t.Fatalf("stopped route error = %v, want ErrWorkstationPoolStopped", err)
	}
	if _, err := pool.Start(context.Background(), []workstations.Route{{WorkstationName: "review"}}); !errors.Is(err, workers.ErrWorkstationPoolStopped) {
		t.Fatalf("Start() after stop error = %v, want ErrWorkstationPoolStopped", err)
	}
}

func TestPoolStopIsRepeatSafeUnderConcurrentCalls(t *testing.T) {
	t.Parallel()

	pool := New()
	if _, err := pool.Start(context.Background(), []workstations.Route{{WorkstationName: "review"}}); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	const callers = 32
	outcomes := make(chan workers.WorkstationPoolLifecycleOutcome, callers)
	errorsCh := make(chan error, callers)
	var wait sync.WaitGroup
	wait.Add(callers)
	for range callers {
		go func() {
			defer wait.Done()
			outcome, err := pool.Stop(context.Background())
			outcomes <- outcome
			errorsCh <- err
		}()
	}
	wait.Wait()
	close(outcomes)
	close(errorsCh)

	stopped := 0
	alreadyStopped := 0
	for err := range errorsCh {
		if err != nil {
			t.Fatalf("concurrent Stop() error = %v", err)
		}
	}
	for outcome := range outcomes {
		switch outcome {
		case workers.WorkstationPoolLifecycleOutcomeStopped:
			stopped++
		case workers.WorkstationPoolLifecycleOutcomeAlreadyStopped:
			alreadyStopped++
		default:
			t.Fatalf("concurrent Stop() outcome = %q", outcome)
		}
	}
	if stopped != 1 || alreadyStopped != callers-1 {
		t.Fatalf("stop outcomes = stopped:%d already:%d", stopped, alreadyStopped)
	}
}

func TestPoolRejectsInvalidRoutesAndCancelledLifecycleCalls(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		routes []workstations.Route
	}{
		{name: "empty"},
		{name: "blank", routes: []workstations.Route{{WorkstationName: " "}}},
		{
			name: "duplicate",
			routes: []workstations.Route{
				{WorkstationName: "review"},
				{WorkstationName: " review "},
			},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			if _, err := New().Start(context.Background(), testCase.routes); !errors.Is(err, workers.ErrInvalidWorkstationPoolStart) {
				t.Fatalf("Start() error = %v, want ErrInvalidWorkstationPoolStart", err)
			}
		})
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	pool := New()
	if _, err := pool.Start(ctx, []workstations.Route{{WorkstationName: "review"}}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled Start() error = %v, want context.Canceled", err)
	}
	if _, err := pool.Stop(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled Stop() error = %v, want context.Canceled", err)
	}
}

func TestPoolCanStopBeforeStartWithoutActivating(t *testing.T) {
	t.Parallel()

	pool := New()
	outcome, err := pool.Stop(context.Background())
	if err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if outcome != workers.WorkstationPoolLifecycleOutcomeStopped {
		t.Fatalf("Stop() outcome = %q, want STOPPED", outcome)
	}
	outcome, err = pool.Stop(context.Background())
	if err != nil {
		t.Fatalf("repeated Stop() error = %v", err)
	}
	if outcome != workers.WorkstationPoolLifecycleOutcomeAlreadyStopped {
		t.Fatalf("repeated Stop() outcome = %q, want ALREADY_STOPPED", outcome)
	}
}
