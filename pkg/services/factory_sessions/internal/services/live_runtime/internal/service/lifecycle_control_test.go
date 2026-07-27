package service_test

import (
	"context"
	"errors"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livesession"
	liveruntimewire "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/live_runtime/wire"
)

func TestServiceResumeAfterPauseReturnsAcceptedOutcomeOnce(t *testing.T) {
	t.Parallel()

	runtime := &testFactoryRuntime{state: string(factorydefinitions.FactoryStateRunning)}
	dependencies := testDependencies()
	dependencies.SessionFactory = func(string) (factoryruntime.Service, error) { return runtime, nil }
	service, err := liveruntimewire.NewService(dependencies)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	paused, err := service.ApplyControl(
		context.Background(),
		"sess-control",
		factorysessions.LifecycleControlPause,
		factorysessions.ControlRequest{},
	)
	if err != nil || paused.Outcome != factorysessions.LifecycleControlOutcomeAccepted || paused.Status != factorysessions.LifecycleStatusPaused {
		t.Fatalf("pause = (%#v, %v), want accepted paused", paused, err)
	}
	if runtime.pauseCalls != 1 {
		t.Fatalf("pause calls = %d, want 1", runtime.pauseCalls)
	}

	runtime.state = string(factorydefinitions.FactoryStatePaused)
	resumed, err := service.ApplyControl(
		context.Background(),
		"sess-control",
		factorysessions.LifecycleControlResume,
		factorysessions.ControlRequest{},
	)
	if err != nil || resumed.Outcome != factorysessions.LifecycleControlOutcomeAccepted || resumed.Status != factorysessions.LifecycleStatusRunning {
		t.Fatalf("resume = (%#v, %v), want accepted running", resumed, err)
	}
	if runtime.resumeCalls != 1 {
		t.Fatalf("resume calls = %d, want 1", runtime.resumeCalls)
	}
}

func TestServiceControlRejectsInvalidStateWithoutRegistryMutation(t *testing.T) {
	t.Parallel()

	runtime := &testFactoryRuntime{state: "QUEUED"}
	sessions := map[string]*livesession.LiveSession{
		"sess-invalid": {ID: "sess-invalid"},
	}
	stopCalls := 0
	dependencies := testDependencies()
	dependencies.SessionFactory = func(string) (factoryruntime.Service, error) { return runtime, nil }
	dependencies.GetSession = func(id string) *livesession.LiveSession { return sessions[id] }
	dependencies.RequireSession = func(id string) (*livesession.LiveSession, error) {
		if session := sessions[id]; session != nil {
			return session, nil
		}
		return nil, factorysessions.ErrNotFound
	}
	dependencies.StopSession = func(string) error {
		stopCalls++
		return nil
	}
	service, err := liveruntimewire.NewService(dependencies)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	_, err = service.ApplyControl(
		context.Background(),
		"sess-invalid",
		factorysessions.LifecycleControlPause,
		factorysessions.ControlRequest{},
	)
	var controlErr *factorysessions.ControlError
	if !errors.As(err, &controlErr) || controlErr.Outcome != factorysessions.LifecycleControlOutcomeInvalidState {
		t.Fatalf("pause error = %v, want invalid-state control error", err)
	}
	if runtime.pauseCalls != 0 || stopCalls != 0 {
		t.Fatalf("invalid pause mutated registry: pause=%d stop=%d", runtime.pauseCalls, stopCalls)
	}
	if service.Resolve("sess-invalid") == nil {
		t.Fatal("registry entry removed after invalid control rejection")
	}
}

func TestServiceCloseTerminatesStopsAndRetiresRegistryEntry(t *testing.T) {
	t.Parallel()

	runtime := &testFactoryRuntime{state: string(factorydefinitions.FactoryStateRunning)}
	session := &livesession.LiveSession{
		ID: "sess-close",
		Runtime: &factorysessions.LiveRuntime{
			Factory: runtime,
		},
	}
	sessions := map[string]*livesession.LiveSession{session.ID: session}
	dependencies := testDependencies()
	dependencies.SessionFactory = func(string) (factoryruntime.Service, error) { return runtime, nil }
	dependencies.GetSession = func(id string) *livesession.LiveSession { return sessions[id] }
	dependencies.RequireSession = func(id string) (*livesession.LiveSession, error) {
		if session := sessions[id]; session != nil {
			return session, nil
		}
		return nil, factorysessions.ErrNotFound
	}
	dependencies.StopSession = func(id string) error {
		delete(sessions, id)
		return nil
	}
	service, err := liveruntimewire.NewService(dependencies)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	if err := service.Close(context.Background(), session.ID); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if runtime.terminateCalls != 1 {
		t.Fatalf("terminate calls = %d, want 1", runtime.terminateCalls)
	}
	if service.Resolve(session.ID) != nil {
		t.Fatal("closed session still resolves as active")
	}
	_, getErr := service.Get(context.Background(), session.ID)
	if getErr == nil || !errors.Is(getErr, factorysessions.ErrNotFound) {
		t.Fatalf("Get after close = %v, want ErrNotFound", getErr)
	}
}

func TestServiceCloseUnknownSessionReturnsNotFoundWithoutStoppingOthers(t *testing.T) {
	t.Parallel()

	remaining := &livesession.LiveSession{ID: "sess-remaining"}
	sessions := map[string]*livesession.LiveSession{remaining.ID: remaining}
	stopped := []string{}
	dependencies := testDependencies()
	dependencies.GetSession = func(id string) *livesession.LiveSession { return sessions[id] }
	dependencies.RequireSession = func(id string) (*livesession.LiveSession, error) {
		if session := sessions[id]; session != nil {
			return session, nil
		}
		return nil, factorysessions.ErrNotFound
	}
	dependencies.StopSession = func(id string) error {
		stopped = append(stopped, id)
		delete(sessions, id)
		return nil
	}
	service, err := liveruntimewire.NewService(dependencies)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	err = service.Close(context.Background(), "missing")
	if err == nil || !errors.Is(err, factorysessions.ErrNotFound) {
		t.Fatalf("Close error = %v, want ErrNotFound", err)
	}
	if len(stopped) != 0 {
		t.Fatalf("stop calls = %#v, want none", stopped)
	}
	if service.Resolve(remaining.ID) == nil {
		t.Fatal("unrelated session removed after close of unknown id")
	}
}

func TestServiceApplyControlHonorsContextCancellation(t *testing.T) {
	t.Parallel()

	runtime := &testFactoryRuntime{state: string(factorydefinitions.FactoryStateRunning)}
	dependencies := testDependencies()
	dependencies.SessionFactory = func(string) (factoryruntime.Service, error) { return runtime, nil }
	service, err := liveruntimewire.NewService(dependencies)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = service.ApplyControl(ctx, "sess-cancel", factorysessions.LifecycleControlPause, factorysessions.ControlRequest{})
	if err == nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("ApplyControl error = %v, want context canceled", err)
	}
	if runtime.pauseCalls != 0 {
		t.Fatalf("pause calls = %d after cancellation, want none", runtime.pauseCalls)
	}
}

func TestServiceCloseHonorsContextCancellation(t *testing.T) {
	t.Parallel()

	session := &livesession.LiveSession{ID: "sess-close-cancel"}
	sessions := map[string]*livesession.LiveSession{session.ID: session}
	stopCalls := 0
	dependencies := testDependencies()
	dependencies.GetSession = func(id string) *livesession.LiveSession { return sessions[id] }
	dependencies.RequireSession = func(id string) (*livesession.LiveSession, error) {
		if session := sessions[id]; session != nil {
			return session, nil
		}
		return nil, factorysessions.ErrNotFound
	}
	dependencies.StopSession = func(string) error {
		stopCalls++
		return nil
	}
	service, err := liveruntimewire.NewService(dependencies)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err = service.Close(ctx, session.ID)
	if err == nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("Close error = %v, want context canceled", err)
	}
	if stopCalls != 0 {
		t.Fatalf("stop calls = %d after cancellation, want none", stopCalls)
	}
	if service.Resolve(session.ID) == nil {
		t.Fatal("registry entry removed after cancelled close")
	}
}
