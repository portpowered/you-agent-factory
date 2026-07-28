package service

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	instancehost "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/instance_host"
	factoryhost "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/host"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"go.uber.org/zap"
)

func newTestHost(t *testing.T) *Host {
	t.Helper()
	host, err := New(instancehost.Dependencies{Clock: clockwork.NewFakeClock()})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	concrete, ok := host.(*Host)
	if !ok {
		t.Fatalf("New() type = %T, want *Host", host)
	}
	return concrete
}

type executeObserverFactory struct {
	mu          sync.RWMutex
	engineState *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]
}
func (f *executeObserverFactory) Run(ctx context.Context) error {
	<-ctx.Done()
	return ctx.Err()
}
func (f *executeObserverFactory) SubmitWorkRequest(context.Context, work.WorkRequest) (work.WorkRequestSubmitResult, error) {
	return work.WorkRequestSubmitResult{}, nil
}
func (f *executeObserverFactory) SubscribeFactoryEvents(context.Context, *interfaces.FactoryEventReconnectCursor, interfaces.FactoryEventReconnectScope) (*interfaces.FactoryEventStream, error) {
	return &interfaces.FactoryEventStream{Events: make(chan interfaces.FactoryEvent)}, nil
}
func (f *executeObserverFactory) Pause(context.Context) error  { return nil }
func (f *executeObserverFactory) Resume(context.Context) error { return nil }
func (f *executeObserverFactory) Terminate(context.Context, factory.TerminateRequest) (factory.TerminateResult, error) {
	return factory.TerminateResult{Outcome: factory.ControlOutcomeAccepted}, nil
}
func (f *executeObserverFactory) Observe(context.Context, factory.ObserveRequest) (factory.ObserveResult, error) {
	return factory.ObserveResult{}, nil
}
func (f *executeObserverFactory) PlanDispatch(_ context.Context, req factory.PlanDispatchRequest) (factory.PlanDispatchResult, error) {
	return factory.PlanDispatchResult{
		Outcome:       factory.DispatchPlanOutcomeAccepted,
		DispatchID:    req.DispatchID,
		CorrelationID: req.CorrelationID,
	}, nil
}
func (f *executeObserverFactory) AcceptDispatchResult(_ context.Context, req factory.AcceptDispatchResultRequest) (factory.AcceptDispatchResultResult, error) {
	return factory.AcceptDispatchResultResult{
		Outcome:       factory.DispatchPlanOutcomeRetired,
		DispatchID:    req.DispatchID,
		CorrelationID: req.CorrelationID,
	}, nil
}
func (f *executeObserverFactory) CaptureCheckpoint(_ context.Context, req factory.CaptureCheckpointRequest) (factory.CaptureCheckpointResult, error) {
	id := req.CheckpointID
	if id == "" {
		id = "checkpoint-stub"
	}
	return factory.CaptureCheckpointResult{
		Outcome: factory.CheckpointOutcomeCaptured,
		Checkpoint: factory.Checkpoint{
			CheckpointID:  id,
			SchemaVersion: 1,
			StrategyKind:  "runtime",
			Payload:       []byte(`{}`),
		},
	}, nil
}
func (f *executeObserverFactory) LoadCheckpoint(_ context.Context, req factory.LoadCheckpointRequest) (factory.LoadCheckpointResult, error) {
	if req.CheckpointID == "" {
		return factory.LoadCheckpointResult{}, factory.ErrCheckpointNotFound
	}
	return factory.LoadCheckpointResult{}, factory.ErrCheckpointNotFound
}
func (f *executeObserverFactory) RestoreCheckpoint(_ context.Context, req factory.RestoreCheckpointRequest) (factory.RestoreCheckpointResult, error) {
	return factory.RestoreCheckpointResult{
		Outcome:      factory.CheckpointOutcomeRestored,
		CheckpointID: req.Checkpoint.CheckpointID,
	}, nil
}
func (f *executeObserverFactory) MoveWork(context.Context, string, string, work.WorkStateChangeSource, string) (work.OperatorMoveResult, error) {
	return work.OperatorMoveResult{}, nil
}
func (f *executeObserverFactory) GetEngineStateSnapshot(context.Context) (*interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.engineState, nil
}
func (f *executeObserverFactory) GetFactoryEvents(context.Context) ([]interfaces.FactoryEvent, error) {
	return nil, nil
}
func (f *executeObserverFactory) WaitToComplete() <-chan struct{} {
	ch := make(chan struct{})
	close(ch)
	return ch
}
func (f *executeObserverFactory) setEngineState(state *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.engineState = state
}

func testBundle(factoryStub factory.Factory, instanceID string) *factoryhost.Bundle {
	return &factoryhost.Bundle{
		RuntimeInstanceID: instanceID,
		Factory:           factoryStub,
		Logger:            zap.NewNop(),
	}
}

func TestStartRejectsNilAndInvalidHostedInstance(t *testing.T) {
	t.Parallel()

	host := newTestHost(t)
	ctx := context.Background()

	handle, err := host.Start(ctx, nil)
	if handle != nil || err == nil || !strings.Contains(err.Error(), "requires a built runtime instance") {
		t.Fatalf("Start(nil) = (%v, %v), want built-instance validation error", handle, err)
	}

	handle, err = host.Start(ctx, &invalidHostedInstance{})
	if handle != nil || err == nil || !strings.Contains(err.Error(), "requires a built runtime instance") {
		t.Fatalf("Start(invalid) = (%v, %v), want built-instance validation error", handle, err)
	}
}

type invalidHostedInstance struct{}

func (*invalidHostedInstance) RuntimeService() factory.Service               { return nil }
func (*invalidHostedInstance) Directory() string                             { return "" }
func (*invalidHostedInstance) FolderDirectory() string                       { return "" }
func (*invalidHostedInstance) BackendScope() string                          { return "" }
func (*invalidHostedInstance) StartTime() time.Time                          { return time.Time{} }
func (*invalidHostedInstance) LoadedRuntimeConfig() factory.LoadedConfig     { return nil }
func (*invalidHostedInstance) CanonicalEvents() []interfaces.FactoryEvent    { return nil }
func (*invalidHostedInstance) AddEventTypeRecorder(func(interfaces.FactoryEventType)) {}
func (*invalidHostedInstance) StreamGeneration() string                      { return "" }
func (*invalidHostedInstance) RuntimeLogger() *zap.Logger                    { return zap.NewNop() }
func (*invalidHostedInstance) RuntimeMetrics() factory.MetricsEmitter        { return nil }
func (*invalidHostedInstance) RuntimeDiagnostics() factory.RuntimeLogDiagnostics {
	return factory.RuntimeLogDiagnostics{}
}
func (*invalidHostedInstance) RecordingLedger() recordings.Ledger            { return nil }
func (*invalidHostedInstance) CloseArtifacts() error                         { return nil }

func TestStartStartsOneRunLoopAndWaitForStartObservesReadiness(t *testing.T) {
	t.Parallel()

	host := newTestHost(t)
	factoryStub := &executeObserverFactory{}
	factoryStub.setEngineState(&interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		RuntimeStatus: interfaces.RuntimeStatusActive,
		FactoryState:  string(interfaces.FactoryStateRunning),
	})
	bundle := testBundle(factoryStub, "runtime-execute-ready")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	handle, err := host.Start(ctx, bundle)
	if err != nil || handle == nil {
		t.Fatalf("Start() = (%v, %v), want hosted handle", handle, err)
	}
	if len(host.handles) != 1 {
		t.Fatalf("active handles = %d, want one registered handle", len(host.handles))
	}

	if err := host.WaitForStart(ctx, handle); err != nil {
		t.Fatalf("WaitForStart() error = %v", err)
	}

	handle2, err := host.Start(ctx, bundle)
	if handle2 != nil || err == nil || !strings.Contains(err.Error(), "already has an active hosted handle") {
		t.Fatalf("Start() duplicate = (%v, %v), want active-handle rejection", handle2, err)
	}

	if err := host.Stop(handle); err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("Stop() error = %v", err)
	}
	if len(host.handles) != 0 {
		t.Fatalf("handles after stop = %d, want empty registry", len(host.handles))
	}
}

func TestWaitForStartFailureCleansUpHandleWithoutOrphan(t *testing.T) {
	t.Parallel()

	host := newTestHost(t)
	factoryStub := &executeObserverFactory{}
	factoryStub.setEngineState(&interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		RuntimeStatus: interfaces.RuntimeStatusActive,
		FactoryState:  string(interfaces.FactoryStateIdle),
	})
	bundle := testBundle(factoryStub, "runtime-execute-early-fail")

	runCtx, cancelRun := context.WithCancel(context.Background())
	defer cancelRun()

	handle, err := host.Start(runCtx, bundle)
	if err != nil || handle == nil {
		t.Fatalf("Start() = (%v, %v), want hosted handle", handle, err)
	}

	readinessCtx, cancelReadiness := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancelReadiness()
	waitErr := host.WaitForStart(readinessCtx, handle)
	if waitErr == nil {
		t.Fatal("WaitForStart() error = nil, want readiness failure")
	}
	if len(host.handles) != 0 {
		t.Fatalf("handles after failed readiness = %d, want no orphaned registry entry", len(host.handles))
	}
	concrete, ok := handle.(*factoryhost.Handle)
	if !ok || !concrete.Completed() {
		t.Fatal("failed readiness should leave a completed handle after cleanup stop")
	}
}

func TestWaitForStartRejectsInvalidHandle(t *testing.T) {
	t.Parallel()

	host := newTestHost(t)
	err := host.WaitForStart(context.Background(), nil)
	if err == nil || !strings.Contains(err.Error(), "requires a runtime handle") {
		t.Fatalf("WaitForStart(nil) error = %v, want runtime-handle validation error", err)
	}
}

func TestStartAfterStopAllowsNewHandleForSameInstance(t *testing.T) {
	t.Parallel()

	host := newTestHost(t)
	factoryStub := &executeObserverFactory{}
	factoryStub.setEngineState(&interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		RuntimeStatus: interfaces.RuntimeStatusActive,
		FactoryState:  string(interfaces.FactoryStateRunning),
	})
	bundle := testBundle(factoryStub, "runtime-execute-restart")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	first, err := host.Start(ctx, bundle)
	if err != nil {
		t.Fatalf("first Start() error = %v", err)
	}
	if err := host.WaitForStart(ctx, first); err != nil {
		t.Fatalf("first WaitForStart() error = %v", err)
	}
	if err := host.Stop(first); err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("Stop() error = %v", err)
	}

	second, err := host.Start(ctx, bundle)
	if err != nil || second == nil {
		t.Fatalf("second Start() = (%v, %v), want new hosted handle after stop", second, err)
	}
	if second == first {
		t.Fatal("second Start() returned same handle identity after prior stop")
	}
	if err := host.Stop(second); err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("Stop(second) error = %v", err)
	}
}

func TestWaitForStartObservesReadinessWithoutSecondHandle(t *testing.T) {
	t.Parallel()

	host := newTestHost(t)
	factoryStub := &executeObserverFactory{}
	factoryStub.setEngineState(&interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		RuntimeStatus: interfaces.RuntimeStatusActive,
		FactoryState:  string(interfaces.FactoryStateRunning),
	})
	bundle := testBundle(factoryStub, "runtime-wait-readiness")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	handle, err := host.Start(ctx, bundle)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	deadline := time.After(2 * time.Second)
	ready := make(chan error, 1)
	go func() {
		ready <- host.WaitForStart(ctx, handle)
	}()

	select {
	case err := <-ready:
		if err != nil {
			t.Fatalf("WaitForStart() error = %v", err)
		}
	case <-deadline:
		t.Fatal("WaitForStart() did not observe running readiness promptly")
	}
	if len(host.handles) != 1 {
		t.Fatalf("handles during readiness = %d, want single active handle", len(host.handles))
	}
	if err := host.Stop(handle); err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("Stop() error = %v", err)
	}
}
