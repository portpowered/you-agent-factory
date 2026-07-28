package invocation_test

import (
	"context"
	"testing"
	"time"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	sessioninvocation "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/invocation"
	invocationruntime "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/invocation/runtimeadapter"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/responsestream"
	sessionruntime "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimebinding"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/sessionregistry"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"go.uber.org/zap"
)

func newInvocationTestSessionState() *sessionruntime.Service {
	clock := platformclock.Real{}
	newStream := func() *responsestream.SessionResponseStream {
		return responsestream.NewSessionResponseStream(clock)
	}
	return sessionruntime.New(
		sessionregistry.New(),
		responsestream.NewRegistry(newStream, clock),
		nil,
		clock,
		func() string { return "response-event-test-id" },
		func() string { return "session-test-id" },
	)
}

type peerShapedInvocationRuntime struct {
	factory.Service
	observation factory.Observation
}

func (runtime peerShapedInvocationRuntime) Observe(
	_ context.Context,
	req factory.ObserveRequest,
) (factory.ObserveResult, error) {
	if req.Scope == "" {
		return factory.ObserveResult{}, factory.ErrInvalidObservationScope
	}
	return factory.ObserveResult{Observation: runtime.observation}, nil
}

func (peerShapedInvocationRuntime) GetFactoryEvents(context.Context) ([]interfaces.FactoryEvent, error) {
	return nil, nil
}

type legacyInvocationRuntime struct {
	peerShapedInvocationRuntime
}

func (runtime legacyInvocationRuntime) GetEngineStateSnapshot(context.Context) (*interfaces.EngineStateSnapshot[factory.PetriMarkingSnapshot, *factory.Net], error) {
	return &interfaces.EngineStateSnapshot[factory.PetriMarkingSnapshot, *factory.Net]{
		FactoryState: string(interfaces.FactoryStateIdle),
	}, nil
}

type invocationHostedInstance struct {
	service factory.Service
}

func (instance invocationHostedInstance) RuntimeService() factory.Service { return instance.service }
func (invocationHostedInstance) Directory() string                        { return "" }
func (invocationHostedInstance) FolderDirectory() string                  { return "" }
func (invocationHostedInstance) BackendScope() string                     { return "" }
func (invocationHostedInstance) StartTime() time.Time                     { return time.Time{} }
func (invocationHostedInstance) LoadedRuntimeConfig() factory.LoadedConfig {
	return nil
}
func (invocationHostedInstance) CanonicalEvents() []interfaces.FactoryEvent { return nil }
func (invocationHostedInstance) AddEventTypeRecorder(func(interfaces.FactoryEventType)) {
}
func (invocationHostedInstance) StreamGeneration() string { return "" }
func (invocationHostedInstance) RuntimeLogger() *zap.Logger {
	return zap.NewNop()
}
func (invocationHostedInstance) RuntimeMetrics() factory.MetricsEmitter {
	return nil
}
func (invocationHostedInstance) RuntimeDiagnostics() factory.RuntimeLogDiagnostics {
	return factory.RuntimeLogDiagnostics{}
}
func (invocationHostedInstance) RecordingLedger() recordings.Ledger { return nil }
func (invocationHostedInstance) CloseArtifacts() error              { return nil }

var _ factory.HostedInstance = invocationHostedInstance{}

type invocationHostedHandle struct {
	instance factory.HostedInstance
	done     chan struct{}
}

func (handle invocationHostedHandle) RuntimeInstance() factory.HostedInstance {
	return handle.instance
}
func (invocationHostedHandle) Completed() bool                   { return false }
func (invocationHostedHandle) Result() error                     { return nil }
func (invocationHostedHandle) Wait() error                       { return nil }
func (invocationHostedHandle) CancelRun()                        {}
func (handle invocationHostedHandle) RunDoneCh() <-chan struct{} { return handle.done }

var _ factory.HostedHandle = invocationHostedHandle{}

func TestObserveRuntimeUsesServiceObservationWithoutLegacySnapshot(t *testing.T) {
	t.Parallel()

	sessions := newInvocationTestSessionState()
	activeFactory := peerShapedInvocationRuntime{
		observation: factory.Observation{
			Status: factory.ObservationStatusIdle,
			Health: factory.ObservationHealth{
				FactoryState: string(interfaces.FactoryStateIdle),
			},
		},
	}
	if _, ok := any(activeFactory).(factory.LegacySnapshotProvider); ok {
		t.Fatal("peer-shaped invocation runtime must not implement LegacySnapshotProvider")
	}
	instance := invocationHostedInstance{service: activeFactory}
	sessions.Register(sessionruntime.Registration{
		SessionID: "session-1",
		Handle: &runtimebinding.SessionState{
			Instance: instance,
			Handle:   invocationHostedHandle{instance: instance, done: make(chan struct{})},
		},
		Runtime: &factorysessions.LiveRuntime{Factory: activeFactory},
		Select:  true,
	})

	observation, err := invocationruntime.Observe(
		context.Background(), sessions, "session-1",
		sessioninvocation.SessionInvocationWaitInput{},
		func(_ []interfaces.FactoryEvent, tick int) (interfaces.FactoryWorldState, error) {
			return interfaces.FactoryWorldState{Tick: tick}, nil
		},
	)
	if err != nil {
		t.Fatalf("ObserveRuntime: %v", err)
	}
	if observation.FactoryState != string(interfaces.FactoryStateIdle) || observation.ActiveWork {
		t.Fatalf("observation = %#v, want idle factory state without active work", observation)
	}
}

func TestObserveRuntimeUsesLegacySnapshotOnlyForMissingPrimaryClassification(t *testing.T) {
	t.Parallel()

	sessions := newInvocationTestSessionState()
	activeFactory := legacyInvocationRuntime{
		peerShapedInvocationRuntime: peerShapedInvocationRuntime{
			observation: factory.Observation{
				Status: factory.ObservationStatusIdle,
				Health: factory.ObservationHealth{
					FactoryState: string(interfaces.FactoryStateIdle),
				},
			},
		},
	}
	instance := invocationHostedInstance{service: activeFactory}
	sessions.Register(sessionruntime.Registration{
		SessionID: "session-1",
		Handle: &runtimebinding.SessionState{
			Instance: instance,
			Handle:   invocationHostedHandle{instance: instance, done: make(chan struct{})},
		},
		Runtime: &factorysessions.LiveRuntime{Factory: activeFactory},
		Select:  true,
	})

	observation, err := invocationruntime.Observe(
		context.Background(), sessions, "session-1",
		sessioninvocation.SessionInvocationWaitInput{RequestID: "request-1"},
		func(_ []interfaces.FactoryEvent, tick int) (interfaces.FactoryWorldState, error) {
			return interfaces.FactoryWorldState{Tick: tick}, nil
		},
	)
	if err != nil {
		t.Fatalf("ObserveRuntime: %v", err)
	}
	if observation.FactoryState != string(interfaces.FactoryStateIdle) || observation.ActiveWork {
		t.Fatalf("observation = %#v, want idle factory state without active work", observation)
	}
}
