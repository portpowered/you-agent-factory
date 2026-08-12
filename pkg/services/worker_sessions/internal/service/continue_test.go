package service_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/providers"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestContinue_CreatesDistinctSuccessorWithExactReferenceAndLineage(t *testing.T) {
	boundary := newControlledBoundary()
	registry := newControlledRegistry(t, boundary)
	reference := providers.SessionRef{
		Provider: providers.IDCodex,
		Kind:     providers.SessionIDKind,
		ID:       "provider-session-exact",
	}

	sourceResult := startControlledSession(t, registry, boundary, "source-session", "dispatch-source")
	boundary.complete(completedDispatchWithProviderSession("dispatch-source", reference), nil)
	source := <-sourceResult
	if source.Session.State != workersessions.StateCompleted {
		t.Fatalf("source session = %#v, want COMPLETED", source.Session)
	}

	request := workersessions.ContinueRequest{
		RequestID:                "continue-request-1",
		SourceWorkerSessionID:    "source-session",
		SuccessorWorkerSessionID: "successor-session",
		FollowUpInput:            "the exact follow-up input",
	}
	continued, err := registry.Continue(context.Background(), request)
	if err != nil {
		t.Fatalf("Continue() error = %v, want nil", err)
	}
	assertContinuationAdmission(t, continued, request)

	handoff := boundary.currentRequest()
	assertContinuationHandoff(t, handoff, request, reference, source)

	sourceAfter, err := registry.Get(context.Background(), workersessions.GetRequest{ID: request.SourceWorkerSessionID})
	if err != nil {
		t.Fatalf("Get(source) error = %v", err)
	}
	assertSourceLineage(t, sourceAfter, request, reference)

	successorAfter, err := registry.Get(context.Background(), workersessions.GetRequest{ID: request.SuccessorWorkerSessionID})
	if err != nil {
		t.Fatalf("Get(successor) error = %v", err)
	}
	assertSuccessorLineage(t, successorAfter, request, reference)

	boundary.complete(completedDispatchWithProviderSession(handoff.Execution.Dispatch.DispatchID, reference), nil)
	finalSuccessor, err := registry.Get(context.Background(), workersessions.GetRequest{ID: request.SuccessorWorkerSessionID})
	if err != nil {
		t.Fatalf("Get(final successor) error = %v", err)
	}
	if finalSuccessor.State != workersessions.StateCompleted {
		t.Fatalf("final successor = %#v, want COMPLETED", finalSuccessor)
	}
}

func assertContinuationAdmission(
	t *testing.T,
	result workersessions.ContinueResult,
	request workersessions.ContinueRequest,
) {
	t.Helper()
	if result.SourceWorkerSessionID != request.SourceWorkerSessionID ||
		result.SuccessorWorkerSessionID != request.SuccessorWorkerSessionID {
		t.Fatalf("Continue() lineage result = %#v, want source/successor identities", result)
	}
	if result.Session.State != workersessions.StateRunning {
		t.Fatalf("Continue() successor = %#v, want RUNNING at admission", result.Session)
	}
}

func assertContinuationHandoff(
	t *testing.T,
	handoff workers.WorkstationDispatchRequest,
	request workersessions.ContinueRequest,
	reference providers.SessionRef,
	source workersessions.InvokeSessionResult,
) {
	t.Helper()
	if handoff.Execution.UserMessage != request.FollowUpInput {
		t.Fatalf("continuation input = %q, want %q", handoff.Execution.UserMessage, request.FollowUpInput)
	}
	if handoff.Execution.ResumeSession == nil || *handoff.Execution.ResumeSession != reference {
		t.Fatalf("continuation ResumeSession = %#v, want exact %#v", handoff.Execution.ResumeSession, reference)
	}
	if source.Session.ProviderSessionAssociation == nil {
		t.Fatal("source session has no provider association")
	}
	if handoff.Execution.ResumeSession == &source.Session.ProviderSessionAssociation.Reference {
		t.Fatal("continuation ResumeSession shares source association storage")
	}
	if handoff.Execution.Dispatch.DispatchID == "dispatch-source" || handoff.Execution.Dispatch.DispatchID == "" {
		t.Fatalf("continuation dispatch ID = %q, want a distinct non-empty identity", handoff.Execution.Dispatch.DispatchID)
	}
}

func assertSourceLineage(
	t *testing.T,
	session workersessions.Session,
	request workersessions.ContinueRequest,
	reference providers.SessionRef,
) {
	t.Helper()
	if session.State != workersessions.StateCompleted || session.SuccessorWorkerSessionID != request.SuccessorWorkerSessionID {
		t.Fatalf("source after continuation = %#v, want terminal source with successor", session)
	}
	if session.ProviderSessionAssociation == nil || session.ProviderSessionAssociation.Reference != reference {
		t.Fatalf("source association after continuation = %#v, want unchanged reference", session.ProviderSessionAssociation)
	}
}

func assertSuccessorLineage(
	t *testing.T,
	session workersessions.Session,
	request workersessions.ContinueRequest,
	reference providers.SessionRef,
) {
	t.Helper()
	if session.PredecessorWorkerSessionID != request.SourceWorkerSessionID {
		t.Fatalf("successor predecessor = %q, want %q", session.PredecessorWorkerSessionID, request.SourceWorkerSessionID)
	}
	if session.ProviderSessionAssociation == nil || session.ProviderSessionAssociation.Reference != reference {
		t.Fatalf("successor association = %#v, want exact reference", session.ProviderSessionAssociation)
	}
}

func TestContinue_IdempotencyAndLineageConflictsAvoidDuplicateAdmission(t *testing.T) {
	boundary := newControlledBoundary()
	registry := newControlledRegistry(t, boundary)
	reference := providers.SessionRef{Provider: providers.IDCodex, Kind: providers.SessionIDKind, ID: "provider-session-1"}
	sourceResult := startControlledSession(t, registry, boundary, "source-session", "dispatch-source")
	boundary.complete(completedDispatchWithProviderSession("dispatch-source", reference), nil)
	<-sourceResult

	request := workersessions.ContinueRequest{
		RequestID:                "continue-request-1",
		SourceWorkerSessionID:    "source-session",
		SuccessorWorkerSessionID: "successor-session",
		FollowUpInput:            "first follow-up",
	}
	first, err := registry.Continue(context.Background(), request)
	if err != nil {
		t.Fatalf("first Continue() error = %v", err)
	}
	duplicate, err := registry.Continue(context.Background(), request)
	if err != nil {
		t.Fatalf("duplicate Continue() error = %v", err)
	}
	if duplicate.SuccessorWorkerSessionID != first.SuccessorWorkerSessionID || duplicate.Session.ID != first.Session.ID {
		t.Fatalf("duplicate result = %#v, want the original successor", duplicate)
	}

	conflict := request
	conflict.FollowUpInput = "different follow-up"
	if _, err := registry.Continue(context.Background(), conflict); !errors.Is(err, workersessions.ErrContinuationRequestIDConflict) {
		t.Fatalf("request-ID reuse error = %v, want ErrContinuationRequestIDConflict", err)
	}

	competing := request
	competing.RequestID = "continue-request-2"
	competing.SuccessorWorkerSessionID = "another-successor"
	if _, err := registry.Continue(context.Background(), competing); !errors.Is(err, workersessions.ErrContinuationSourceConflict) {
		t.Fatalf("competing continuation error = %v, want ErrContinuationSourceConflict", err)
	}

	if boundary.publishCount() != 2 {
		t.Fatalf("boundary publish count = %d, want source plus one successor", boundary.publishCount())
	}
	handoff := boundary.currentRequest()
	boundary.complete(completedDispatchWithProviderSession(handoff.Execution.Dispatch.DispatchID, reference), nil)
}

func TestContinue_ConcurrentIdenticalRequestsShareOneAdmission(t *testing.T) {
	base := newControlledBoundary()
	continuationGate := make(chan struct{})
	boundary := &continuationAdmissionBoundary{
		controlledBoundary: base,
		continuationGate:   continuationGate,
		continuationReady:  make(chan struct{}),
	}
	registry := newControlledRegistry(t, boundary)
	reference := providers.SessionRef{Provider: providers.IDCodex, Kind: providers.SessionIDKind, ID: "provider-session-concurrent"}
	sourceResult := startControlledSession(t, registry, base, "source-session", "dispatch-source")
	base.complete(completedDispatchWithProviderSession("dispatch-source", reference), nil)
	if result := <-sourceResult; result.Session.State != workersessions.StateCompleted {
		t.Fatalf("source result = %#v, want COMPLETED", result.Session)
	}

	request := workersessions.ContinueRequest{
		RequestID:                "continue-request-concurrent",
		SourceWorkerSessionID:    "source-session",
		SuccessorWorkerSessionID: "successor-session",
		FollowUpInput:            "concurrent follow-up",
	}
	const callers = 8
	start := make(chan struct{})
	results := make(chan struct {
		result workersessions.ContinueResult
		err    error
	}, callers)
	var group sync.WaitGroup
	group.Add(callers)
	for range callers {
		go func() {
			defer group.Done()
			<-start
			result, err := registry.Continue(context.Background(), request)
			results <- struct {
				result workersessions.ContinueResult
				err    error
			}{result: result, err: err}
		}()
	}
	close(start)

	select {
	case <-boundary.continuationReady:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for one continuation admission")
	}
	if got := base.publishCount(); got != 2 {
		t.Fatalf("Worker Sessions publish count before release = %d, want source plus one continuation", got)
	}
	close(continuationGate)
	group.Wait()
	close(results)

	for outcome := range results {
		if outcome.err != nil {
			t.Fatalf("concurrent Continue() error = %v, want nil", outcome.err)
		}
		if outcome.result.SuccessorWorkerSessionID != request.SuccessorWorkerSessionID ||
			outcome.result.Session.ID != request.SuccessorWorkerSessionID {
			t.Fatalf("concurrent Continue() result = %#v, want shared successor", outcome.result)
		}
	}
	if got := base.publishCount(); got != 2 {
		t.Fatalf("Worker Sessions publish count = %d, want source plus one continuation", got)
	}
	handoff := base.currentRequest()
	base.complete(completedDispatchWithProviderSession(handoff.Execution.Dispatch.DispatchID, reference), nil)
}

type continuationAdmissionBoundary struct {
	*controlledBoundary
	continuationGate  <-chan struct{}
	continuationReady chan struct{}
	readyOnce         sync.Once
}

type continuationFailureBoundary struct {
	*controlledBoundary
	err error
}

func (b *continuationFailureBoundary) Publish(
	ctx context.Context,
	request workers.WorkstationDispatchRequest,
	accept workers.WorkstationDispatchAcceptFunc,
) error {
	return b.PublishWithAdmission(ctx, request, nil, accept)
}

func (b *continuationFailureBoundary) PublishWithAdmission(
	ctx context.Context,
	request workers.WorkstationDispatchRequest,
	admitted workers.WorkstationDispatchAdmissionFunc,
	accept workers.WorkstationDispatchAcceptFunc,
) error {
	if request.Execution.ResumeSession == nil {
		return b.controlledBoundary.PublishWithAdmission(ctx, request, admitted, accept)
	}
	accept(context.Background(), request, workers.WorkstationDispatchResult{
		DispatchID:      request.Execution.Dispatch.DispatchID,
		TerminalOutcome: workers.WorkstationDispatchTerminalOutcomeFailed,
		Result: workers.WorkResult{
			DispatchID: request.Execution.Dispatch.DispatchID,
			Outcome:    workers.OutcomeFailed,
		},
	}, b.err)
	return nil
}

func TestContinue_PreAdmissionFailureReturnsTerminalSuccessorAndTypedError(t *testing.T) {
	base := newControlledBoundary()
	boundary := &continuationFailureBoundary{controlledBoundary: base, err: errors.New("continuation admission rejected")}
	registry := newControlledRegistry(t, boundary)
	reference := providers.SessionRef{Provider: providers.IDCodex, Kind: providers.SessionIDKind, ID: "provider-session-failure"}
	sourceResult := startControlledSession(t, registry, base, "source-session", "dispatch-source")
	base.complete(completedDispatchWithProviderSession("dispatch-source", reference), nil)
	if source := <-sourceResult; source.Session.State != workersessions.StateCompleted {
		t.Fatalf("source result = %#v, want COMPLETED", source.Session)
	}

	request := workersessions.ContinueRequest{
		RequestID:                "continue-request-failure",
		SourceWorkerSessionID:    "source-session",
		SuccessorWorkerSessionID: "successor-session",
		FollowUpInput:            "follow-up",
	}
	result, err := registry.Continue(context.Background(), request)
	if !errors.Is(err, workersessions.ErrContinuationNotAccepted) {
		t.Fatalf("Continue() error = %v, want ErrContinuationNotAccepted", err)
	}
	if result.Session.ID != request.SuccessorWorkerSessionID || result.Session.State != workersessions.StateFailed {
		t.Fatalf("Continue() result = %#v, want failed successor snapshot", result)
	}
	if result.Session.Result == nil || result.Session.Result.Cause == nil {
		t.Fatalf("Continue() result = %#v, want terminal failure cause", result.Session)
	}
}

func TestContinue_CanceledCallerDoesNotCancelReservedContinuation(t *testing.T) {
	base := newControlledBoundary()
	continuationGate := make(chan struct{})
	boundary := &continuationAdmissionBoundary{
		controlledBoundary: base,
		continuationGate:   continuationGate,
		continuationReady:  make(chan struct{}),
	}
	registry := newControlledRegistry(t, boundary)
	reference := providers.SessionRef{Provider: providers.IDCodex, Kind: providers.SessionIDKind, ID: "provider-session-cancel"}
	sourceResult := startControlledSession(t, registry, base, "source-session", "dispatch-source")
	base.complete(completedDispatchWithProviderSession("dispatch-source", reference), nil)
	if source := <-sourceResult; source.Session.State != workersessions.StateCompleted {
		t.Fatalf("source result = %#v, want COMPLETED", source.Session)
	}

	request := workersessions.ContinueRequest{
		RequestID:                "continue-request-cancel",
		SourceWorkerSessionID:    "source-session",
		SuccessorWorkerSessionID: "successor-session",
		FollowUpInput:            "follow up",
	}
	ctx, cancel := context.WithCancel(context.Background())
	outcomes := make(chan struct {
		result workersessions.ContinueResult
		err    error
	}, 1)
	go func() {
		result, err := registry.Continue(ctx, request)
		outcomes <- struct {
			result workersessions.ContinueResult
			err    error
		}{result: result, err: err}
	}()
	select {
	case <-boundary.continuationReady:
	case <-time.After(time.Second):
		t.Fatal("continuation did not reach admission wait")
	}
	cancel()
	select {
	case outcome := <-outcomes:
		if !errors.Is(outcome.err, context.Canceled) {
			t.Fatalf("Continue(canceled caller) error = %v, want context.Canceled", outcome.err)
		}
	case <-time.After(time.Second):
		t.Fatal("Continue(canceled caller) did not return")
	}
	close(continuationGate)
	base.complete(completedDispatchWithProviderSession(base.currentRequest().Execution.Dispatch.DispatchID, reference), nil)
	final, err := registry.Get(context.Background(), workersessions.GetRequest{ID: request.SuccessorWorkerSessionID})
	if err != nil || final.State != workersessions.StateCompleted {
		t.Fatalf("successor after canceled caller = %#v, %v, want server-owned completion", final, err)
	}
}

func (b *continuationAdmissionBoundary) PublishWithAdmission(
	ctx context.Context,
	request workers.WorkstationDispatchRequest,
	admitted workers.WorkstationDispatchAdmissionFunc,
	accept workers.WorkstationDispatchAcceptFunc,
) error {
	embeddedAdmission := admitted
	if request.Execution.ResumeSession != nil {
		embeddedAdmission = nil
	}
	if err := b.controlledBoundary.PublishWithAdmission(ctx, request, embeddedAdmission, accept); err != nil {
		return err
	}
	if request.Execution.ResumeSession == nil || admitted == nil {
		if admitted != nil {
			admitted()
		}
		return nil
	}
	b.readyOnce.Do(func() { close(b.continuationReady) })
	select {
	case <-b.continuationGate:
		admitted()
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func TestContinue_ProviderFailureLeavesSuccessorFailedWithExactAssociation(t *testing.T) {
	boundary := newControlledBoundary()
	registry := newControlledRegistry(t, boundary)
	reference := providers.SessionRef{Provider: providers.IDCodex, Kind: providers.SessionIDKind, ID: "provider-session-1"}
	sourceResult := startControlledSession(t, registry, boundary, "source-session", "dispatch-source")
	boundary.complete(completedDispatchWithProviderSession("dispatch-source", reference), nil)
	<-sourceResult

	request := workersessions.ContinueRequest{
		RequestID:                "continue-request-1",
		SourceWorkerSessionID:    "source-session",
		SuccessorWorkerSessionID: "successor-session",
		FollowUpInput:            "follow-up",
	}
	continued, err := registry.Continue(context.Background(), request)
	if err != nil {
		t.Fatalf("Continue() error = %v, want nil at admission", err)
	}
	handoff := boundary.currentRequest()
	boundary.complete(failedContinuationDispatch(handoff.Execution.Dispatch.DispatchID, reference), nil)

	successor, err := registry.Get(context.Background(), workersessions.GetRequest{ID: request.SuccessorWorkerSessionID})
	if err != nil {
		t.Fatalf("Get(successor) error = %v", err)
	}
	if successor.State != workersessions.StateFailed || successor.Result == nil || successor.Result.Cause == nil {
		t.Fatalf("successor after provider failure = %#v, want FAILED with cause", successor)
	}
	if successor.Result.Cause.ProviderContinuationFailureKind != providers.ContinuationFailureKindStale {
		t.Fatalf("successor failure cause = %#v, want stale continuation classification", successor.Result.Cause)
	}
	if successor.ProviderSessionAssociation == nil || successor.ProviderSessionAssociation.Reference != reference {
		t.Fatalf("successor association = %#v, want exact retained reference", successor.ProviderSessionAssociation)
	}
	source, err := registry.Get(context.Background(), workersessions.GetRequest{ID: request.SourceWorkerSessionID})
	if err != nil {
		t.Fatalf("Get(source) error = %v", err)
	}
	if source.State != workersessions.StateCompleted || source.ProviderSessionAssociation == nil || source.ProviderSessionAssociation.Reference != reference {
		t.Fatalf("source after provider failure = %#v, want unchanged terminal source/reference", source)
	}
	if continued.Session.ID != request.SuccessorWorkerSessionID {
		t.Fatalf("admitted result = %#v, want successor identity", continued)
	}
}

func TestContinue_RejectsUnknownActiveAndUnassociatedSourcesWithoutSuccessor(t *testing.T) {
	tests := []struct {
		name    string
		prepare func(t *testing.T, registry workersessions.Service, boundary *controlledBoundary)
		wantErr error
	}{
		{
			name:    "unknown",
			prepare: func(*testing.T, workersessions.Service, *controlledBoundary) {},
			wantErr: workersessions.ErrContinuationSourceNotFound,
		},
		{
			name: "active",
			prepare: func(t *testing.T, registry workersessions.Service, _ *controlledBoundary) {
				t.Helper()
				if _, err := registry.Reserve(context.Background(), workersessions.ReserveRequest{ID: "source-session"}); err != nil {
					t.Fatalf("Reserve(source) error = %v", err)
				}
			},
			wantErr: workersessions.ErrContinuationSourceActive,
		},
		{
			name: "unassociated terminal",
			prepare: func(t *testing.T, registry workersessions.Service, boundary *controlledBoundary) {
				t.Helper()
				sourceResult := startControlledSession(t, registry, boundary, "source-session", "dispatch-source")
				boundary.complete(completedDispatch("dispatch-source"), nil)
				if result := <-sourceResult; result.Session.State != workersessions.StateCompleted {
					t.Fatalf("source result = %#v, want COMPLETED", result.Session)
				}
			},
			wantErr: workersessions.ErrContinuationProviderSessionMissing,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			boundary := newControlledBoundary()
			registry := newControlledRegistry(t, boundary)
			test.prepare(t, registry, boundary)
			publishCountBefore := boundary.publishCount()
			_, err := registry.Continue(context.Background(), workersessions.ContinueRequest{
				RequestID:                "continue-request-" + test.name,
				SourceWorkerSessionID:    "source-session",
				SuccessorWorkerSessionID: "successor-session",
				FollowUpInput:            "follow-up",
			})
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("Continue() error = %v, want %v", err, test.wantErr)
			}
			if _, getErr := registry.Get(context.Background(), workersessions.GetRequest{ID: "successor-session"}); !errors.Is(getErr, workersessions.ErrSessionNotFound) {
				t.Fatalf("Get(successor) error = %v, want ErrSessionNotFound", getErr)
			}
			if boundary.publishCount() != publishCountBefore {
				t.Fatalf("boundary publish count = %d, want unchanged %d and zero continuation effects", boundary.publishCount(), publishCountBefore)
			}
		})
	}
}

func completedDispatchWithProviderSession(dispatchID string, reference providers.SessionRef) workers.WorkstationDispatchResult {
	result := completedDispatch(dispatchID)
	result.Result.ProviderSession = &workers.ProviderSessionMetadata{
		Provider: reference.Provider.String(),
		Kind:     reference.Kind,
		ID:       reference.ID,
	}
	return result
}

func completedDispatch(dispatchID string) workers.WorkstationDispatchResult {
	return workers.WorkstationDispatchResult{
		DispatchID:      dispatchID,
		TerminalOutcome: workers.WorkstationDispatchTerminalOutcomeCompleted,
		Result: workers.WorkResult{
			DispatchID: dispatchID,
			Outcome:    workers.OutcomeAccepted,
		},
	}
}

func failedContinuationDispatch(dispatchID string, reference providers.SessionRef) workers.WorkstationDispatchResult {
	result := completedDispatchWithProviderSession(dispatchID, reference)
	result.TerminalOutcome = workers.WorkstationDispatchTerminalOutcomeFailed
	result.Result.Outcome = workers.OutcomeFailed
	result.Result.FailureMetadata = &workers.WorkFailureMetadata{
		Family: workers.WorkFailureFamilyTerminal,
		Type:   workers.WorkFailureTypePermanentBadRequest,
	}
	result.Result.ProviderContinuationFailureKind = providers.ContinuationFailureKindStale
	return result
}
