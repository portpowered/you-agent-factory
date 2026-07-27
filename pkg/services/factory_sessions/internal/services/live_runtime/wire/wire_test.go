package wire_test

import (
	"context"
	"testing"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livesession"
	liveruntime "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/live_runtime"
	liveruntimewire "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/live_runtime/wire"
)

func TestNewServiceConstructsInertCapability(t *testing.T) {
	t.Parallel()

	openCalled := false
	listCalled := false
	dependencies := testDependencies()
	dependencies.OpenForTarget = func(context.Context, factorysessions.Target) (string, error) {
		openCalled = true
		return "session", nil
	}
	dependencies.ListSessionIDs = func() []string {
		listCalled = true
		return nil
	}

	service, err := liveruntimewire.NewService(dependencies)
	if err != nil || service == nil {
		t.Fatalf("NewService = (%v, %v)", service, err)
	}
	if openCalled || listCalled {
		t.Fatal("construction invoked live-runtime effects")
	}
}

func TestNewServiceRejectsMissingRuntimeEffects(t *testing.T) {
	t.Parallel()

	dependencies := testDependencies()
	dependencies.StopSession = nil

	service, err := liveruntimewire.NewService(dependencies)
	if err == nil || service != nil {
		t.Fatalf("NewService without stop effect = (%v, %v), want error", service, err)
	}
}

func testDependencies() liveruntime.Dependencies {
	return liveruntime.Dependencies{
		OpenForTarget:  func(context.Context, factorysessions.Target) (string, error) { return "session", nil },
		ListSessionIDs: func() []string { return nil },
		GetSession:     func(string) *livesession.LiveSession { return nil },
		RequireSession: func(string) (*livesession.LiveSession, error) { return nil, factorysessions.ErrSessionNotFound },
		BuildProjectionContext: func(context.Context, *livesession.LiveSession) (factorysessions.ProjectionContext, error) {
			return factorysessions.ProjectionContext{}, nil
		},
		SessionFactory: func(string) (factoryruntime.Service, error) { return nil, factorysessions.ErrSessionNotFound },
		StopSession:    func(string) error { return nil },
		ObserveControl: func(string, factorysessions.LifecycleControlKind, factorysessions.ControlRequest, factorysessions.LifecycleControlOutcome, factorysessions.LifecycleStatus, error) {
		},
	}
}
