package service_test

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"sync"
	"testing"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	stateaccess "github.com/portpowered/infinite-you/pkg/services/work/internal/services/state_access"
	internalservice "github.com/portpowered/infinite-you/pkg/services/work/internal/services/state_access/internal/service"
)

type recordingSessionAdapter struct {
	submitted   work.WorkRequest
	movedID     string
	source      work.WorkStateChangeSource
	requestID   string
	submitErr   error
	moveErr     error
	snapshot    work.ReadSnapshot
	snapshotErr error
}

func (a *recordingSessionAdapter) SubmitWorkRequest(
	_ context.Context,
	request work.WorkRequest,
) (work.WorkRequestSubmitResult, error) {
	a.submitted = request
	if a.submitErr != nil {
		return work.WorkRequestSubmitResult{}, a.submitErr
	}
	return work.WorkRequestSubmitResult{
		RequestID: request.RequestID,
		Accepted:  true,
		Works: []work.WorkRequestSubmittedWork{{
			Name:         "story-1",
			WorkTypeName: "story",
			WorkID:       "work-1",
		}},
	}, nil
}

func (a *recordingSessionAdapter) MoveWork(
	_ context.Context,
	workID string,
	_ string,
	source work.WorkStateChangeSource,
	requestID string,
) (work.OperatorMoveResult, error) {
	a.movedID, a.source, a.requestID = workID, source, requestID
	if a.moveErr != nil {
		return work.OperatorMoveResult{}, a.moveErr
	}
	return work.OperatorMoveResult{
		WorkID:     workID,
		WorkTypeID: "story",
		FromState:  "draft",
		ToState:    "review",
		TokenID:    "tok-1",
	}, nil
}

func (a *recordingSessionAdapter) ReadWorkSnapshot(context.Context) (work.ReadSnapshot, error) {
	if a.snapshotErr != nil {
		return work.ReadSnapshot{}, a.snapshotErr
	}
	return a.snapshot, nil
}

type stubSessionResolver struct {
	adapter stateaccess.SessionAdapter
	err     error
}

func (r stubSessionResolver) ResolveSessionAdapter(string) (stateaccess.SessionAdapter, error) {
	if r.err != nil {
		return nil, r.err
	}
	return r.adapter, nil
}

func TestSubmitWorkRequestForSessionReturnsDetachedResult(t *testing.T) {
	t.Parallel()

	adapter := &recordingSessionAdapter{}
	svc := internalservice.New(stubSessionResolver{adapter: adapter},
		nil,
		nil)
	ctx := context.Background()
	request := work.WorkRequest{RequestID: "request-1"}

	result, err := svc.SubmitWorkRequestForSession(ctx, "session-1", request)
	if err != nil {
		t.Fatalf("SubmitWorkRequestForSession: %v", err)
	}
	if !result.Accepted || result.RequestID != "request-1" || len(result.Works) != 1 {
		t.Fatalf("result = %#v, want detached accepted submit facts", result)
	}
	if adapter.submitted.RequestID != "request-1" {
		t.Fatalf("submitted request = %#v", adapter.submitted)
	}
}

func TestMoveWorkForSessionReturnsDetachedResult(t *testing.T) {
	t.Parallel()

	adapter := &recordingSessionAdapter{}
	svc := internalservice.New(stubSessionResolver{adapter: adapter},
		nil,
		nil)
	ctx := context.Background()

	result, err := svc.MoveWorkForSession(ctx, "session-1", "work-1", "review", "move-1")
	if err != nil {
		t.Fatalf("MoveWorkForSession: %v", err)
	}
	if result.WorkID != "work-1" || result.FromState != "draft" || result.ToState != "review" {
		t.Fatalf("result = %#v, want detached draft->review move facts", result)
	}
	if result.TokenID != "" {
		t.Fatalf("result leaked Petri fields: %#v", result)
	}
	if adapter.movedID != "work-1" || adapter.source != work.WorkStateChangeSourceAPI || adapter.requestID != "move-1" {
		t.Fatalf("adapter move = (%q, %q, %q)", adapter.movedID, adapter.source, adapter.requestID)
	}
}

func TestMoveWorkForSessionPropagatesAlreadyAppliedFailure(t *testing.T) {
	t.Parallel()

	adapter := &recordingSessionAdapter{moveErr: work.ErrMoveWorkRequestAlreadyApplied}
	svc := internalservice.New(stubSessionResolver{adapter: adapter},
		nil,
		nil)
	ctx := context.Background()

	_, err := svc.MoveWorkForSession(ctx, "session-1", "work-1", "done", "dup-move")
	if !errors.Is(err, work.ErrMoveWorkRequestAlreadyApplied) {
		t.Fatalf("error = %v, want ErrMoveWorkRequestAlreadyApplied", err)
	}
}

func TestResolveSessionResolverError(t *testing.T) {
	t.Parallel()

	resolverErr := errors.New("session missing")
	svc := internalservice.New(stubSessionResolver{err: resolverErr},
		nil,
		nil)
	ctx := context.Background()

	_, err := svc.SubmitWorkRequestForSession(ctx, "missing", work.WorkRequest{})
	if !errors.Is(err, resolverErr) {
		t.Fatalf("error = %v, want resolver error", err)
	}
}

type recordingFactory struct {
	submitted work.WorkRequest
	movedID   string
	source    work.WorkStateChangeSource
}

func (f *recordingFactory) SubmitWorkRequest(_ context.Context, request work.WorkRequest) (work.WorkRequestSubmitResult, error) {
	f.submitted = request
	return work.WorkRequestSubmitResult{}, nil
}

func (f *recordingFactory) MoveWork(_ context.Context, workID, _ string, source work.WorkStateChangeSource, _ string) (work.OperatorMoveResult, error) {
	f.movedID, f.source = workID, source
	return work.OperatorMoveResult{}, nil
}

func (f *recordingFactory) ReadWorkSnapshot(context.Context) (work.ReadSnapshot, error) {
	return work.ReadSnapshot{}, nil
}

func TestStateAccessRoutesThroughSessionContract(t *testing.T) {
	t.Parallel()
	runtime := &recordingFactory{}
	service := internalservice.New(stubSessionResolver{adapter: runtime}, nil, nil)

	request := work.WorkRequest{RequestID: "request-root-contract"}
	if _, err := service.SubmitWorkRequestForSession(
		context.Background(),
		"session-1",
		request,
	); err != nil {
		t.Fatalf("SubmitWorkRequestForSession: %v", err)
	}
	if _, err := service.MoveWorkForSession(
		context.Background(),
		"session-1",
		"work-1",
		"done",
		"move-1",
	); err != nil {
		t.Fatalf("MoveWorkForSession: %v", err)
	}
	if runtime.submitted.RequestID != request.RequestID ||
		runtime.movedID != "work-1" ||
		runtime.source != work.WorkStateChangeSourceAPI {
		t.Fatalf(
			"routed calls = (%q, %q, %q)",
			runtime.submitted.RequestID,
			runtime.movedID,
			runtime.source,
		)
	}
}

func TestStateAccessPropagatesSessionResolverError(t *testing.T) {
	t.Parallel()
	service := internalservice.New(stubSessionResolver{err: factorysessions.ErrSessionNotFound}, nil, nil)
	_, err := service.SubmitWorkRequestForSession(context.Background(), "missing", work.WorkRequest{})
	if !errors.Is(err, factorysessions.ErrSessionNotFound) {
		t.Fatalf("error = %v, want ErrSessionNotFound", err)
	}
}

func TestStateAccessConcurrentSessionOperationsRemainIsolated(t *testing.T) {
	t.Parallel()

	first := &isolatedWorkRuntime{sessionID: "session-first"}
	second := &isolatedWorkRuntime{sessionID: "session-second"}
	service := internalservice.New(isolatedWorkRuntimeResolver{
		runtimes: map[string]stateaccess.SessionAdapter{
			first.sessionID:  first,
			second.sessionID: second,
		},
	}, nil, nil)

	const operationsPerSession = 16
	var wait sync.WaitGroup
	errorsCh := make(chan error, operationsPerSession*2)
	for _, sessionID := range []string{first.sessionID, second.sessionID} {
		sessionID := sessionID
		for index := 0; index < operationsPerSession; index++ {
			index := index
			wait.Add(1)
			go runIsolatedSessionOperations(service, sessionID, index, &wait, errorsCh)
		}
	}
	wait.Wait()
	close(errorsCh)
	for err := range errorsCh {
		t.Fatalf("concurrent Work operation: %v", err)
	}

	for _, runtime := range []*isolatedWorkRuntime{first, second} {
		assertIsolatedRuntime(t, runtime, operationsPerSession)
	}
}

func runIsolatedSessionOperations(
	service stateaccess.Service,
	sessionID string,
	index int,
	wait *sync.WaitGroup,
	errorsCh chan<- error,
) {
	defer wait.Done()
	requestID := sessionID + "-request-" + strconv.Itoa(index)
	if _, err := service.SubmitWorkRequestForSession(context.Background(), sessionID, work.WorkRequest{
		RequestID: requestID,
	}); err != nil {
		errorsCh <- err
		return
	}
	listed, err := service.ListWork(context.Background(), sessionID, work.ListOptions{})
	if err != nil {
		errorsCh <- err
		return
	}
	if len(listed.Results) != 1 || listed.Results[0].WorkID != sessionID+"-work" {
		errorsCh <- errors.New("list crossed session boundary")
		return
	}
	got, err := service.GetWork(context.Background(), sessionID, sessionID+"-work")
	if err != nil {
		errorsCh <- err
		return
	}
	if got.WorkID != sessionID+"-work" {
		errorsCh <- errors.New("get crossed session boundary")
		return
	}
	moved, err := service.MoveWorkForSession(
		context.Background(), sessionID, sessionID+"-work", "done", requestID+"-move",
	)
	if err != nil {
		errorsCh <- err
		return
	}
	if moved.WorkID != sessionID+"-work" {
		errorsCh <- errors.New("move crossed session boundary")
	}
}

func assertIsolatedRuntime(t *testing.T, runtime *isolatedWorkRuntime, wantOperations int) {
	t.Helper()
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if len(runtime.requestIDs) != wantOperations {
		t.Fatalf("%s request count = %d, want %d", runtime.sessionID, len(runtime.requestIDs), wantOperations)
	}
	for _, requestID := range runtime.requestIDs {
		if !strings.HasPrefix(requestID, runtime.sessionID+"-") {
			t.Fatalf("%s received request %q from another session", runtime.sessionID, requestID)
		}
	}
	if len(runtime.moveIDs) != wantOperations {
		t.Fatalf("%s move count = %d, want %d", runtime.sessionID, len(runtime.moveIDs), wantOperations)
	}
}

type isolatedWorkRuntimeResolver struct {
	runtimes map[string]stateaccess.SessionAdapter
}

func (r isolatedWorkRuntimeResolver) ResolveSessionAdapter(sessionID string) (stateaccess.SessionAdapter, error) {
	runtime := r.runtimes[sessionID]
	if runtime == nil {
		return nil, factorysessions.ErrSessionNotFound
	}
	return runtime, nil
}

type isolatedWorkRuntime struct {
	sessionID  string
	mu         sync.Mutex
	requestIDs []string
	moveIDs    []string
}

func (r *isolatedWorkRuntime) SubmitWorkRequest(
	_ context.Context,
	request work.WorkRequest,
) (work.WorkRequestSubmitResult, error) {
	r.mu.Lock()
	r.requestIDs = append(r.requestIDs, request.RequestID)
	r.mu.Unlock()
	return work.WorkRequestSubmitResult{
		RequestID: request.RequestID,
		Accepted:  true,
		Works: []work.WorkRequestSubmittedWork{{
			Name: "work", WorkTypeName: "task", WorkID: r.sessionID + "-work",
		}},
	}, nil
}

func (r *isolatedWorkRuntime) ReadWorkSnapshot(context.Context) (work.ReadSnapshot, error) {
	return work.ReadSnapshot{Items: []work.ReadModel{{
		WorkID: r.sessionID + "-work",
		Name:   "work",
		State:  &work.State{Name: "draft", Type: work.StateTypeInitial},
	}}}, nil
}

func (r *isolatedWorkRuntime) MoveWork(
	_ context.Context,
	workID string,
	stateName string,
	_ work.WorkStateChangeSource,
	requestID string,
) (work.OperatorMoveResult, error) {
	r.mu.Lock()
	r.moveIDs = append(r.moveIDs, requestID)
	r.mu.Unlock()
	return work.OperatorMoveResult{WorkID: workID, ToState: stateName}, nil
}
