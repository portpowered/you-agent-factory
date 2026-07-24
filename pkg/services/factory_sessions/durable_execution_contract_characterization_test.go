package factorysessions_test

import (
	"context"
	"errors"
	"testing"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
)

// peerDurableExecutionFake exercises the published durable-execution root slice
// through the singular Service. It compiles against only the Sessions root
// package and never imports factory_sessions/internal or nested execution
// implementation packages.
type peerDurableExecutionFake struct {
	*peerRootServiceFake
	starts    map[string]factorysessions.DurableAsyncStartResult
	sessions  map[string]factorysessions.DurableInspectResult
	lifecycle map[string]factorysessions.LifecycleStatus
	resumeErr map[string]error
	startErr  map[string]error
}

func newPeerDurableExecutionFake() *peerDurableExecutionFake {
	return &peerDurableExecutionFake{
		peerRootServiceFake: newPeerRootServiceFake(),
		starts:              make(map[string]factorysessions.DurableAsyncStartResult),
		sessions:            make(map[string]factorysessions.DurableInspectResult),
		lifecycle:           make(map[string]factorysessions.LifecycleStatus),
		resumeErr:           make(map[string]error),
		startErr:            make(map[string]error),
	}
}

var _ factorysessions.Service = (*peerDurableExecutionFake)(nil)

func (fake *peerDurableExecutionFake) StartAsync(
	_ context.Context,
	req factorysessions.DurableStartRequest,
) (factorysessions.DurableAsyncStartResult, error) {
	if err, ok := fake.startErr[req.RequestID]; ok {
		return factorysessions.DurableAsyncStartResult{}, err
	}
	if result, ok := fake.starts[req.RequestID]; ok {
		return result, nil
	}
	return factorysessions.DurableAsyncStartResult{}, &factorysessions.DurableValidationError{
		Field:   "source",
		Message: "source is required",
	}
}

func (fake *peerDurableExecutionFake) ResumeInterruptedSession(
	_ context.Context,
	sessionID string,
	_ factorysessions.DurableResumeRequest,
) (factorysessions.DurableAsyncStartResult, error) {
	if err, ok := fake.resumeErr[sessionID]; ok {
		return factorysessions.DurableAsyncStartResult{}, err
	}
	if result, ok := fake.starts[sessionID]; ok {
		return result, nil
	}
	return factorysessions.DurableAsyncStartResult{}, factorysessions.ErrDurableSessionNotFound
}

func (fake *peerDurableExecutionFake) GetSession(
	_ context.Context,
	sessionID string,
) (factorysessions.DurableInspectResult, error) {
	if result, ok := fake.sessions[sessionID]; ok {
		return result, nil
	}
	return factorysessions.DurableInspectResult{}, factorysessions.ErrDurableSessionNotFound
}

func (fake *peerDurableExecutionFake) Pause(
	_ context.Context,
	sessionID string,
	_ factorysessions.DurableControlRequest,
) (factorysessions.DurableControlResult, error) {
	return fake.applyDurableControl(sessionID, factorysessions.LifecycleControlPause)
}

func (fake *peerDurableExecutionFake) Resume(
	_ context.Context,
	sessionID string,
	_ factorysessions.DurableControlRequest,
) (factorysessions.DurableControlResult, error) {
	return fake.applyDurableControl(sessionID, factorysessions.LifecycleControlResume)
}

func (fake *peerDurableExecutionFake) applyDurableControl(
	sessionID string,
	operation factorysessions.LifecycleControlKind,
) (factorysessions.DurableControlResult, error) {
	status, ok := fake.lifecycle[sessionID]
	if !ok {
		return factorysessions.DurableControlResult{}, factorysessions.ErrDurableSessionNotFound
	}
	switch status {
	case factorysessions.LifecycleStatusSucceeded, factorysessions.LifecycleStatusFailed:
		return factorysessions.DurableControlResult{}, &factorysessions.DurableControlError{
			Operation: operation,
			Outcome:   factorysessions.LifecycleControlOutcomeTerminalSession,
			Status:    status,
			Message:   string(factorysessions.LifecycleControlOutcomeTerminalSession),
		}
	case factorysessions.LifecycleStatusRunning:
		if operation == factorysessions.LifecycleControlPause {
			fake.lifecycle[sessionID] = factorysessions.LifecycleStatusPaused
			return factorysessions.DurableControlResult{
				SessionID: sessionID,
				Operation: operation,
				Outcome:   factorysessions.LifecycleControlOutcomeAccepted,
				Status:    factorysessions.LifecycleStatusPaused,
			}, nil
		}
	}
	return factorysessions.DurableControlResult{}, &factorysessions.DurableControlError{
		Operation: operation,
		Outcome:   factorysessions.LifecycleControlOutcomeInvalidState,
		Status:    status,
		Message:   string(factorysessions.LifecycleControlOutcomeInvalidState),
	}
}

func TestDurableExecutionRootContract_StartAndInspectSuccess(t *testing.T) {
	t.Parallel()

	fake := newPeerDurableExecutionFake()
	sessionID := "dur-sess-alpha"
	requestID := "req-durable-start-1"
	fake.starts[requestID] = factorysessions.DurableAsyncStartResult{
		SessionID:        sessionID,
		Status:           string(factorysessions.LifecycleStatusRunning),
		OrchestratorKind: "JAVASCRIPT",
		SourceHash:       "source-hash-alpha",
	}
	fake.sessions[sessionID] = factorysessions.DurableInspectResult{
		SessionID:        sessionID,
		Status:           factorysessions.LifecycleStatusRunning,
		OrchestratorKind: "JAVASCRIPT",
		SourceHash:       "source-hash-alpha",
	}
	fake.lifecycle[sessionID] = factorysessions.LifecycleStatusRunning

	var service factorysessions.Service = fake
	ctx := context.Background()

	started, err := service.StartAsync(ctx, factorysessions.DurableStartRequest{
		RequestID: requestID,
		Source:    factorysessions.Source{Kind: "WORKFLOW_NAME", WorkflowName: "demo"},
	})
	if err != nil {
		t.Fatalf("StartAsync: %v", err)
	}
	if started.SessionID != sessionID || started.Status != string(factorysessions.LifecycleStatusRunning) {
		t.Fatalf("StartAsync = %#v, want published durable start success for %q", started, sessionID)
	}

	inspected, err := service.GetSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if inspected.SessionID != sessionID || inspected.Status != factorysessions.LifecycleStatusRunning {
		t.Fatalf("GetSession = %#v, want durable inspect success shape", inspected)
	}

	paused, err := service.Pause(ctx, sessionID, factorysessions.DurableControlRequest{Reason: "operator-pause"})
	if err != nil {
		t.Fatalf("Pause: %v", err)
	}
	if paused.SessionID != sessionID ||
		paused.Outcome != factorysessions.LifecycleControlOutcomeAccepted ||
		paused.Status != factorysessions.LifecycleStatusPaused {
		t.Fatalf("Pause = %#v, want accepted durable control result", paused)
	}
}

func TestDurableExecutionRootContract_TypedFailures(t *testing.T) {
	t.Parallel()

	fake := newPeerDurableExecutionFake()
	terminalID := "dur-sess-terminal"
	checkpointID := "dur-sess-missing-checkpoint"
	fake.lifecycle[terminalID] = factorysessions.LifecycleStatusSucceeded
	fake.sessions[terminalID] = factorysessions.DurableInspectResult{
		SessionID: terminalID,
		Status:    factorysessions.LifecycleStatusSucceeded,
	}
	fake.startErr["req-invalid-source"] = &factorysessions.DurableValidationError{
		Field:   "source",
		Message: "source.kind is invalid",
	}
	fake.startErr["req-invalid-policy"] = &factorysessions.DurableValidationError{
		Field:   "requestedPolicy",
		Message: "requestedPolicy is invalid",
	}
	fake.resumeErr[checkpointID] = &factorysessions.DurableResumeError{
		Outcome:   factorysessions.DurableResumeOutcomeMissingCheckpoint,
		Status:    factorysessions.LifecycleStatusPaused,
		Field:     "checkpointSummary",
		Message:   string(factorysessions.DurableResumeOutcomeMissingCheckpoint),
		SessionID: checkpointID,
	}

	var service factorysessions.Service = fake
	ctx := context.Background()

	_, err := service.StartAsync(ctx, factorysessions.DurableStartRequest{RequestID: "req-invalid-source"})
	var invalidSource *factorysessions.DurableValidationError
	if !errors.As(err, &invalidSource) || invalidSource.Field != "source" {
		t.Fatalf("StartAsync invalid source = %v, want *DurableValidationError field=source", err)
	}

	_, err = service.StartAsync(ctx, factorysessions.DurableStartRequest{RequestID: "req-invalid-policy"})
	var invalidPolicy *factorysessions.DurableValidationError
	if !errors.As(err, &invalidPolicy) || invalidPolicy.Field != "requestedPolicy" {
		t.Fatalf("StartAsync invalid policy = %v, want *DurableValidationError field=requestedPolicy", err)
	}

	_, err = service.GetSession(ctx, "dur-sess-missing")
	if !errors.Is(err, factorysessions.ErrDurableSessionNotFound) {
		t.Fatalf("GetSession missing = %v, want ErrDurableSessionNotFound", err)
	}

	_, err = service.ResumeInterruptedSession(ctx, checkpointID, factorysessions.DurableResumeRequest{RequestID: "resume-1"})
	var missingCheckpoint *factorysessions.DurableResumeError
	if !errors.As(err, &missingCheckpoint) {
		t.Fatalf("ResumeInterruptedSession = %v, want *DurableResumeError", err)
	}
	if missingCheckpoint.Outcome != factorysessions.DurableResumeOutcomeMissingCheckpoint {
		t.Fatalf("ResumeInterruptedSession outcome = %q, want MISSING_CHECKPOINT", missingCheckpoint.Outcome)
	}
	if errors.Is(err, factorysessions.ErrDurableSessionNotFound) {
		t.Fatal("missing checkpoint must stay distinct from ErrDurableSessionNotFound")
	}

	_, err = service.Pause(ctx, terminalID, factorysessions.DurableControlRequest{})
	var rejected *factorysessions.DurableControlError
	if !errors.As(err, &rejected) {
		t.Fatalf("Pause terminal = %v, want *DurableControlError", err)
	}
	if rejected.Outcome != factorysessions.LifecycleControlOutcomeTerminalSession {
		t.Fatalf("Pause outcome = %q, want TERMINAL_SESSION", rejected.Outcome)
	}
	if errors.Is(err, factorysessions.ErrDurableSessionNotFound) {
		t.Fatal("rejected lifecycle control must stay distinct from ErrDurableSessionNotFound")
	}
}
