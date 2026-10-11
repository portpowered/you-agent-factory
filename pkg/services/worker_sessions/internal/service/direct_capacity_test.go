package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestDirectCapacityAtomicReservationReplayAndConflict(t *testing.T) {
	t.Parallel()
	r := newTestRegistry(t)
	r.directCapacity = 1
	request := func(id string) workersessions.StartRequest {
		return workersessions.StartRequest{RequestID: id + "-request", ID: id}
	}
	gate := make(chan struct{})
	results := make(chan string, 2)
	var group sync.WaitGroup
	for _, id := range []string{"first", "second"} {
		group.Add(1)
		go func() {
			defer group.Done()
			<-gate
			_, owner, err := r.reserveStart(request(id))
			if err == nil && owner {
				results <- id
			} else if errors.Is(err, workersessions.ErrDirectCapacityExceeded) {
				results <- "refused"
			} else {
				results <- "invalid"
			}
		}()
	}
	close(gate)
	group.Wait()
	close(results)
	winner := ""
	refusals := 0
	for result := range results {
		if result == "refused" {
			refusals++
		} else {
			winner = result
		}
	}
	if winner == "" || winner == "invalid" || refusals != 1 {
		t.Fatalf("admission winner=%s refusals=%d", winner, refusals)
	}
	loser := "first"
	if winner == loser {
		loser = "second"
	}
	if _, err := r.Get(context.Background(), workersessions.GetRequest{ID: loser}); !errors.Is(err, workersessions.ErrSessionNotFound) {
		t.Fatalf("loser reserved: %v", err)
	}

}

func TestDirectCapacityFrozenResolution(t *testing.T) {
	t.Parallel()
	r := newTestRegistry(t)
	calls := 0
	r.capacityResolver = func(context.Context) (int64, error) { calls++; return 2, nil }
	if err := r.resolveDirectCapacity(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := r.resolveDirectCapacity(context.Background()); err != nil || calls != 1 || r.directCapacity != 2 {
		t.Fatalf("frozen capacity=%d calls=%d error=%v", r.directCapacity, calls, err)
	}
}

func TestDirectCapacityPreDriverFailureReleases(t *testing.T) {
	t.Parallel()
	r := newTestRegistry(t)
	r.directCapacity = 1
	request := workersessions.StartRequest{RequestID: "failed", ID: "failed", Execution: validStartRequest("failed", "failed-dispatch").Execution}
	r.events = &runtimeAttemptBrokenAppender{err: errors.New("controlled opening failure")}
	if _, _, err := r.reserveStart(request); err != nil {
		t.Fatal(err)
	}
	// Opening publication fails before the driver takes ownership.
	if _, err := r.startReservedWithEffects(request, r.execution, r.clock, r.scheduler); err == nil {
		t.Fatal("invalid execution was admitted")
	}
	if _, owner, err := r.reserveStart(workersessions.StartRequest{RequestID: "new", ID: "new"}); err != nil || !owner {
		t.Fatalf("failed opening leaked charge: %t %v", owner, err)
	}
}

func TestDirectCapacityJoinedLifetime(t *testing.T) {
	for _, outcome := range []string{"success", "failure", "cancel", "terminate", "early-terminal"} {
		t.Run(outcome, func(t *testing.T) { t.Parallel(); directCapacityLifetime(t, outcome) })
	}
}

func directCapacityLifetime(t *testing.T, outcome string) {
	t.Helper()
	r := newTestRegistry(t)
	r.capacityResolver = func(context.Context) (int64, error) { return 1, nil }
	started, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	defer unblock()
	r.execution = capacityLifetimeExecution(outcome, started, release)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	caller, detach := context.WithCancel(ctx)
	request := workersessions.StartRequest{ID: "held", RequestID: "held", Execution: dispatchHandoff("held-dispatch")}
	admitted, err := r.Start(caller, request)
	if err != nil || admitted.Session.ID != "held" {
		t.Fatalf("START=%+v %v", admitted, err)
	}
	<-started
	detach()
	overflow := workersessions.StartRequest{ID: "excess", RequestID: "excess", Execution: dispatchHandoff("excess-dispatch")}
	assertFull := func() {
		t.Helper()
		if _, err := r.Start(ctx, overflow); !errors.Is(err, workersessions.ErrDirectCapacityExceeded) {
			t.Fatalf("live charge=%v", err)
		}
	}
	assertFull()
	if replay, err := r.Start(ctx, request); err != nil || replay.Session.ID != "held" {
		t.Fatalf("saturated replay=%+v %v", replay, err)
	}
	if outcome == "early-terminal" {
		r.commitTerminal("held", workersessions.StateFailed, workersessions.TerminalResult{})
		assertFull()
	}
	var controlDone chan error
	if outcome == "cancel" || outcome == "terminate" {
		controlDone = make(chan error, 1)
		go func() {
			var err error
			if outcome == "cancel" {
				_, err = r.Cancel(ctx, workersessions.ControlRequest{ID: "held"})
			} else {
				_, err = r.Terminate(ctx, workersessions.ControlRequest{ID: "held"})
			}
			controlDone <- err
		}()
	}
	unblock()
	if err := r.waitForSupervisionDriver(ctx, "held"); err != nil {
		t.Fatal(err)
	}
	if controlDone != nil {
		if err := <-controlDone; err != nil {
			t.Fatal(err)
		}
	}
	replacement := workersessions.StartRequest{ID: "replacement", RequestID: "replacement"}
	if _, owner, err := r.reserveStart(replacement); err != nil || !owner {
		t.Fatalf("joined charge leaked: %t %v", owner, err)
	}
	r.releaseDirect("held")
	assertFull()
	r.releaseDirect("replacement")
}

func capacityLifetimeExecution(outcome string, started chan struct{}, release <-chan struct{}) coverageExecution {
	return coverageExecution{execute: func(ctx context.Context, request workers.ExecuteRequest) (workers.ExecuteResult, error) {
		close(started)
		if outcome == "cancel" || outcome == "terminate" {
			<-ctx.Done()
		}
		<-release
		if outcome == "failure" {
			return coverageExecutionResult(request, workers.ExecutionOutcomeFailed), errors.New("controlled failure")
		}
		if ctx.Err() != nil {
			return coverageExecutionResult(request, workers.ExecutionOutcomeCanceled), ctx.Err()
		}
		return coverageExecutionResult(request, workers.ExecutionOutcomeAccepted), nil
	}}
}

func TestDirectCapacityReplayConflictAndReleaseOnce(t *testing.T) {
	t.Parallel()
	r := newTestRegistry(t)
	r.directCapacity = 1
	request := func(id string) workersessions.StartRequest {
		return workersessions.StartRequest{RequestID: id + "-request", ID: id}
	}
	winner := "first"
	if _, owner, err := r.reserveStart(request(winner)); err != nil || !owner {
		t.Fatalf("reservation=%t %v", owner, err)
	}
	replay, owner, err := r.reserveStart(request(winner))
	if err != nil || owner || replay == nil {
		t.Fatalf("saturated replay owner=%t err=%v", owner, err)
	}
	conflict := request(winner)
	conflict.ID = "conflict"
	if _, _, err := r.reserveStart(conflict); !errors.Is(err, workersessions.ErrStartRequestIDConflict) {
		t.Fatalf("conflict=%v", err)
	}
	r.releaseDirect(winner)
	r.releaseDirect(winner)
	if _, owner, err := r.reserveStart(request("replacement")); !owner || err != nil {
		t.Fatalf("replacement=%t %v", owner, err)
	}
	r.releaseDirect(winner)
	if _, _, err := r.reserveStart(request("overflow")); !errors.Is(err, workersessions.ErrDirectCapacityExceeded) {
		t.Fatalf("duplicate release freed replacement: %v", err)
	}
}
