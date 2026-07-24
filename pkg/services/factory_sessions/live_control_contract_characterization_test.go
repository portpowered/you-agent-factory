package factorysessions_test

import (
	"context"
	"errors"
	"testing"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
)

// peerLiveControlFake exercises the published live-control root slice through
// the singular Service. It compiles against only the Sessions root package and
// never imports factory_sessions/internal or live-runtime registry/host types.
type peerLiveControlFake struct {
	*peerRootServiceFake
	openResults map[string]*factorysessions.OpenResult
	listed      []factorysessions.ReadProjection
	lifecycle   map[string]factorysessions.LifecycleStatus
	closed      map[string]bool
}

func newPeerLiveControlFake() *peerLiveControlFake {
	return &peerLiveControlFake{
		peerRootServiceFake: newPeerRootServiceFake(),
		openResults:         make(map[string]*factorysessions.OpenResult),
		lifecycle:           make(map[string]factorysessions.LifecycleStatus),
		closed:              make(map[string]bool),
	}
}

var _ factorysessions.Service = (*peerLiveControlFake)(nil)

func (fake *peerLiveControlFake) OpenFactorySession(
	_ context.Context,
	request factorysessions.LiveControlOpenRequest,
) (*factorysessions.LiveControlOpenResult, error) {
	if result, ok := fake.openResults[request.FolderPath]; ok {
		return result, nil
	}
	return nil, factorysessions.ErrSessionNotFound
}

func (fake *peerLiveControlFake) ListFactorySessions(context.Context) ([]factorysessions.LiveControlListItem, error) {
	out := make([]factorysessions.LiveControlListItem, len(fake.listed))
	copy(out, fake.listed)
	return out, nil
}

func (fake *peerLiveControlFake) GetFactorySession(
	_ context.Context,
	sessionID string,
) (factorysessions.LiveControlSnapshot, error) {
	if fake.closed[sessionID] {
		return factorysessions.LiveControlSnapshot{}, factorysessions.ErrSessionNotFound
	}
	if projection, ok := fake.sessions[sessionID]; ok {
		return projection, nil
	}
	return factorysessions.LiveControlSnapshot{}, factorysessions.ErrSessionNotFound
}

func (fake *peerLiveControlFake) PauseLiveFactorySession(
	_ context.Context,
	sessionID string,
	_ factorysessions.LiveControlRequest,
) (factorysessions.LiveControlResult, error) {
	return fake.applyLiveControl(sessionID, factorysessions.LifecycleControlPause)
}

func (fake *peerLiveControlFake) ResumeLiveFactorySession(
	_ context.Context,
	sessionID string,
	_ factorysessions.LiveControlRequest,
) (factorysessions.LiveControlResult, error) {
	return fake.applyLiveControl(sessionID, factorysessions.LifecycleControlResume)
}

func (fake *peerLiveControlFake) CloseFactorySession(_ context.Context, sessionID string) error {
	if _, ok := fake.sessions[sessionID]; !ok && !fake.closed[sessionID] {
		if _, opened := fake.lifecycle[sessionID]; !opened {
			return factorysessions.ErrSessionNotFound
		}
	}
	fake.closed[sessionID] = true
	delete(fake.sessions, sessionID)
	delete(fake.lifecycle, sessionID)
	return nil
}

func (fake *peerLiveControlFake) applyLiveControl(
	sessionID string,
	operation factorysessions.LifecycleControlKind,
) (factorysessions.LiveControlResult, error) {
	if fake.closed[sessionID] {
		return factorysessions.LiveControlResult{}, factorysessions.ErrSessionNotFound
	}
	status, ok := fake.lifecycle[sessionID]
	if !ok {
		return factorysessions.LiveControlResult{}, factorysessions.ErrSessionNotFound
	}
	// Peer fake hardcodes representative outcomes; it does not re-test nested
	// lifecycle evaluation algorithms.
	switch status {
	case factorysessions.LifecycleStatusSucceeded, factorysessions.LifecycleStatusFailed:
		return factorysessions.LiveControlResult{}, &factorysessions.LiveControlError{
			Operation: operation,
			Outcome:   factorysessions.LifecycleControlOutcomeTerminalSession,
			Status:    status,
			Message:   string(factorysessions.LifecycleControlOutcomeTerminalSession),
		}
	case factorysessions.LifecycleStatusPaused:
		if operation == factorysessions.LifecycleControlPause {
			return factorysessions.LiveControlResult{
				SessionID: sessionID,
				Operation: operation,
				Outcome:   factorysessions.LifecycleControlOutcomeNoOp,
				Status:    status,
			}, nil
		}
		if operation == factorysessions.LifecycleControlResume {
			fake.lifecycle[sessionID] = factorysessions.LifecycleStatusRunning
			return factorysessions.LiveControlResult{
				SessionID: sessionID,
				Operation: operation,
				Outcome:   factorysessions.LifecycleControlOutcomeAccepted,
				Status:    factorysessions.LifecycleStatusRunning,
			}, nil
		}
	case factorysessions.LifecycleStatusRunning:
		if operation == factorysessions.LifecycleControlPause {
			fake.lifecycle[sessionID] = factorysessions.LifecycleStatusPaused
			return factorysessions.LiveControlResult{
				SessionID: sessionID,
				Operation: operation,
				Outcome:   factorysessions.LifecycleControlOutcomeAccepted,
				Status:    factorysessions.LifecycleStatusPaused,
			}, nil
		}
		if operation == factorysessions.LifecycleControlResume {
			return factorysessions.LiveControlResult{
				SessionID: sessionID,
				Operation: operation,
				Outcome:   factorysessions.LifecycleControlOutcomeNoOp,
				Status:    status,
			}, nil
		}
	}
	return factorysessions.LiveControlResult{}, &factorysessions.LiveControlError{
		Operation: operation,
		Outcome:   factorysessions.LifecycleControlOutcomeInvalidState,
		Status:    status,
		Message:   string(factorysessions.LifecycleControlOutcomeInvalidState),
	}
}

func TestLiveControlRootContract_OpenListGetStableIdentity(t *testing.T) {
	t.Parallel()

	fake := newPeerLiveControlFake()
	sessionID := "live-session-alpha"
	folder := "/workspace/factories/demo"
	fake.openResults[folder] = &factorysessions.LiveControlOpenResult{
		SessionID:  sessionID,
		FolderPath: folder,
		Session: &factorysessions.ScopedLiveSessionSummary{
			ID:         sessionID,
			FolderPath: folder,
			IsDefault:  true,
		},
	}
	snapshot := factorysessions.LiveControlSnapshot{
		Context: factorysessions.ProjectionContext{
			FactorySessionID: sessionID,
			Session: &factorysessions.ScopedLiveSessionSummary{
				ID:         sessionID,
				FolderPath: folder,
				IsDefault:  true,
			},
		},
		Runtime: factorysessions.RuntimeProjection{Status: "RUNNING"},
	}
	fake.sessions[sessionID] = snapshot
	fake.lifecycle[sessionID] = factorysessions.LifecycleStatusRunning
	fake.listed = []factorysessions.LiveControlListItem{{
		Context:          snapshot.Context,
		Runtime:          snapshot.Runtime,
		RuntimeAvailable: true,
	}}

	var service factorysessions.Service = fake
	ctx := context.Background()

	opened, err := service.OpenFactorySession(ctx, factorysessions.LiveControlOpenRequest{FolderPath: folder})
	if err != nil {
		t.Fatalf("OpenFactorySession: %v", err)
	}
	if opened == nil || opened.SessionID != sessionID || opened.Session == nil || opened.Session.ID != sessionID {
		t.Fatalf("OpenFactorySession result = %#v, want stable session identity %q", opened, sessionID)
	}

	listed, err := service.ListFactorySessions(ctx)
	if err != nil {
		t.Fatalf("ListFactorySessions: %v", err)
	}
	if len(listed) != 1 || listed[0].Context.FactorySessionID != sessionID {
		t.Fatalf("ListFactorySessions = %#v, want one row for %q", listed, sessionID)
	}

	got, err := service.GetFactorySession(ctx, sessionID)
	if err != nil {
		t.Fatalf("GetFactorySession: %v", err)
	}
	if got.Context.FactorySessionID != sessionID || got.Runtime.Status != "RUNNING" {
		t.Fatalf("GetFactorySession = %#v, want live projection for %q", got, sessionID)
	}

	paused, err := service.PauseLiveFactorySession(ctx, sessionID, factorysessions.LiveControlRequest{Reason: "operator-pause"})
	if err != nil {
		t.Fatalf("PauseLiveFactorySession: %v", err)
	}
	if paused.SessionID != sessionID ||
		paused.Outcome != factorysessions.LifecycleControlOutcomeAccepted ||
		paused.Status != factorysessions.LifecycleStatusPaused {
		t.Fatalf("PauseLiveFactorySession = %#v, want accepted pause", paused)
	}
}

func TestLiveControlRootContract_TypedMissingAndLifecycleFailures(t *testing.T) {
	t.Parallel()

	fake := newPeerLiveControlFake()
	terminalID := "live-session-terminal"
	fake.lifecycle[terminalID] = factorysessions.LifecycleStatusSucceeded
	fake.sessions[terminalID] = factorysessions.LiveControlSnapshot{
		Context: factorysessions.ProjectionContext{FactorySessionID: terminalID},
		Runtime: factorysessions.RuntimeProjection{Status: "SUCCEEDED"},
	}

	var service factorysessions.Service = fake
	ctx := context.Background()

	_, err := service.GetFactorySession(ctx, "missing-live-session")
	if !errors.Is(err, factorysessions.ErrSessionNotFound) {
		t.Fatalf("GetFactorySession missing = %v, want ErrSessionNotFound", err)
	}

	_, err = service.PauseLiveFactorySession(ctx, "missing-live-session", factorysessions.LiveControlRequest{})
	if !errors.Is(err, factorysessions.ErrSessionNotFound) {
		t.Fatalf("PauseLiveFactorySession missing = %v, want ErrSessionNotFound", err)
	}

	_, err = service.PauseLiveFactorySession(ctx, terminalID, factorysessions.LiveControlRequest{})
	var rejected *factorysessions.LiveControlError
	if !errors.As(err, &rejected) {
		t.Fatalf("PauseLiveFactorySession terminal = %v, want *LiveControlError", err)
	}
	if rejected.Outcome != factorysessions.LifecycleControlOutcomeTerminalSession {
		t.Fatalf("PauseLiveFactorySession outcome = %q, want TERMINAL_SESSION", rejected.Outcome)
	}
	if errors.Is(err, factorysessions.ErrSessionNotFound) {
		t.Fatal("invalid lifecycle transition must stay distinct from ErrSessionNotFound")
	}

	if err := service.CloseFactorySession(ctx, "missing-live-session"); !errors.Is(err, factorysessions.ErrSessionNotFound) {
		t.Fatalf("CloseFactorySession missing = %v, want ErrSessionNotFound", err)
	}
}
