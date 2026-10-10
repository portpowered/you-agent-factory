package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func callerSourceMetadata() *workersessions.SessionMetadata {
	return &workersessions.SessionMetadata{
		Requester:   &workersessions.Requester{Kind: "WORKER_SESSION", WorkerSessionID: "lead", WorkID: "project"},
		Correlation: &workersessions.Correlation{WorkID: "lane", FactorySessionID: "factory"},
		Labels:      []string{"project:example"},
	}
}

func runningCaller(t *testing.T, r *registry, id string) *workersessions.CallerIdentity {
	t.Helper()
	token := bindTestSessionToken(t, r, id)
	r.mu.Lock()
	session := r.sessions[id]
	session.State = workersessions.StateRunning
	session.Metadata = callerSourceMetadata()
	r.sessions[id] = session
	r.mu.Unlock()
	return &workersessions.CallerIdentity{WorkerSessionID: id, Token: token}
}

func TestCallerInvalidRefusesBeforeAdmissionEffects(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"partial", "malformed", "foreign", "starting", "ended", "stopping", "lost-owner"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			executor := &preflightExecution{}
			r, req := newPreflightRegistry(t, executor)
			caller := runningCaller(t, r, "source")
			switch name {
			case "partial":
				caller.WorkerSessionID = ""
			case "malformed":
				caller.Token = "invalid"
			case "foreign":
				caller.Token = base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{255}, 32))
			case "starting":
				s := r.sessions["source"]
				s.State = workersessions.StateStarting
				r.sessions["source"] = s
			case "ended":
				r.commitControlTerminal("source", workersessions.StateCanceled)
			case "stopping":
				r.stopping = true
			case "lost-owner":
				delete(r.sessions, "source")
			}
			req.Caller = caller
			_, err := r.Start(t.Context(), req)
			if !errors.Is(err, workersessions.ErrCallerInvalid) {
				t.Fatalf("Start refusal = %v", err)
			}
			invoke := validStartRequest("child", "dispatch")
			invoke.Caller = caller
			_, err = r.InvokeSession(t.Context(), invoke)
			if !errors.Is(err, workersessions.ErrCallerInvalid) || executor.validations.Load() != 0 || executor.executions.Load() != 0 {
				t.Fatal("invalid caller reached admission or execution")
			}
			if _, exists := r.sessions[req.ID]; exists {
				t.Fatal("invalid caller reserved a child")
			}
		})
	}
}

func TestCallerReservationUsesDetachedVerifiedFacts(t *testing.T) {
	t.Parallel()
	r := newTestRegistry(t)
	caller := runningCaller(t, r, "source")
	req := validStartRequest("child", "dispatch")
	req.Caller = caller
	req.Metadata = &workersessions.SessionMetadata{Requester: &workersessions.Requester{Kind: "WORKER_SESSION", WorkerSessionID: "forged"}}
	if err := r.reserveWithCaller(req.ID, req.Metadata, req.Caller); err != nil {
		t.Fatal(err)
	}
	want := callerSourceMetadata()
	want.Requester = &workersessions.Requester{Kind: "WORKER_SESSION", WorkerSessionID: "source", WorkID: want.Correlation.WorkID}
	child := r.sessions["child"]
	if !reflect.DeepEqual(child.Metadata, want) {
		t.Fatalf("reserved facts = %+v, want %+v", child.Metadata, want)
	}
	child.Metadata.Labels[0] = "mutated"
	if !reflect.DeepEqual(r.sessions["source"].Metadata, callerSourceMetadata()) {
		t.Fatal("child facts alias their source")
	}
	for _, value := range []any{req, workersessions.StartRequest{Caller: caller}, caller} {
		encoded, err := json.Marshal(value)
		if err != nil || bytes.Contains(encoded, []byte(caller.Token)) {
			t.Fatal("caller credentials escaped request serialization")
		}
	}
}

func TestCallerTokenSelectsExactScopedOwner(t *testing.T) {
	t.Parallel()
	r := newTestRegistry(t)
	first := runningCaller(t, r, "first")
	second := runningCaller(t, r, "second")
	for _, source := range []string{"first", "second"} {
		address := scopedWorkerAddress("shared", source)
		session := r.sessions[source]
		session.ID = "shared"
		session.Metadata.Correlation.FactorySessionID = source
		r.sessions[address] = session
		r.executionTokens[address] = r.executionTokens[source]
		delete(r.sessions, source)
		delete(r.executionTokens, source)
	}
	for _, caller := range []*workersessions.CallerIdentity{first, second} {
		wantScope := caller.WorkerSessionID
		caller.WorkerSessionID = "shared"
		metadata, err := r.resolveCallerMetadata(caller, nil)
		if err != nil || metadata.Correlation.FactorySessionID != wantScope || metadata.Requester.WorkerSessionID != "shared" {
			t.Fatal("equal public IDs selected the wrong owner")
		}
	}
	r.executionTokens[scopedWorkerAddress("shared", "second")] = first.Token
	if _, err := r.resolveCallerMetadata(first, nil); !errors.Is(err, workersessions.ErrCallerInvalid) {
		t.Fatal("ambiguous credential acquired authority")
	}
}

func TestCallerAbsentAndReservedMetadataRemainHonest(t *testing.T) {
	t.Parallel()
	r := newTestRegistry(t)
	if err := r.reserveWithCaller("root", nil, nil); err != nil || r.sessions["root"].Metadata != nil {
		t.Fatal("absent caller invented requester facts")
	}
	r.reserveIfAbsent("retained", callerSourceMetadata())
	caller := runningCaller(t, r, "source")
	if err := r.reserveWithCaller("retained", nil, caller); err != nil || !reflect.DeepEqual(r.sessions["retained"].Metadata, callerSourceMetadata()) {
		t.Fatal("verified caller rewrote reserved facts")
	}
}

func TestCallerLossDuringPreflightRefusesReservation(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	started, release := make(chan struct{}, 1), make(chan struct{})
	executor := &preflightExecution{started: started, release: release}
	r, req := newPreflightRegistry(t, executor)
	req.Caller = runningCaller(t, r, "source")
	result := make(chan error, 1)
	go func() { _, err := r.Start(ctx, req); result <- err }()
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	r.commitControlTerminal("source", workersessions.StateCanceled)
	close(release)
	select {
	case err := <-result:
		if !errors.Is(err, workersessions.ErrCallerInvalid) {
			t.Fatalf("owner loss refusal = %v", err)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if executor.executions.Load() != 0 || r.startReplays[req.RequestID] != nil {
		t.Fatal("owner loss acquired replay or execution")
	}
}

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
	service, err := New(executor, newEventsAppender(), logging.NoopLogger{}, platformclock.Real{}, platformclock.Real{}, nil, unavailableWorkerControlStore{}, unavailableWorkerControlStore{}, continuationInspectionFake{}, newTestHistoryBudget().Entropy)
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
