// Factory Runtime state adapters remain session-owned.
package service

import (
	"context"
	"fmt"

	"github.com/portpowered/infinite-you/pkg/services/work"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimebinding"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"go.uber.org/zap"
)

func (a *Assembly) scopeSyncActiveSessionDir(fs *SessionRuntime, runtimeBundle factoryRuntimeBundle) {
	if fs == nil {
		return
	}
	runtimebinding.SyncActiveDirectory(&fs.runtimeMu, &fs.dir, fs.factoryRootDir, runtimeBundle)
}

// SubmitWorkRequest submits a canonical work request batch to the factory.
func (a *Assembly) scopedSubmitWorkRequest(fs *SessionRuntime, ctx context.Context, request work.WorkRequest) (work.WorkRequestSubmitResult, error) {
	if fs == nil || a.state == nil {
		return work.WorkRequestSubmitResult{}, fmt.Errorf("factory session service is required")
	}
	var result work.WorkRequestSubmitResult
	err := a.state.WithRuntimeRead(func(runtime *factorysessions.LiveRuntime) error {
		var submitErr error
		submitter, ok := runtimebinding.WorkAndEventIngressForLiveRuntime(runtime)
		if !ok {
			return fmt.Errorf("Factory Runtime work submission is required")
		}
		result, submitErr = submitter.SubmitWorkRequest(ctx, request)
		return submitErr
	})
	return result, err
}

// SubscribeFactoryEvents returns canonical factory event history followed by
// live events from the current service-owned runtime.
func (a *Assembly) scopedSubscribeFactoryEvents(fs *SessionRuntime, ctx context.Context, reconnect *interfaces.FactoryEventReconnectCursor, scope interfaces.FactoryEventReconnectScope) (*interfaces.FactoryEventStream, error) {
	if fs == nil {
		return nil, fmt.Errorf("factory session service is required")
	}
	runtime := fs.currentRuntimeService()
	if runtime == nil {
		return nil, fmt.Errorf("factory runtime is not available")
	}
	events, ok := runtimebinding.WorkAndEventIngressForService(runtime)
	if !ok {
		return nil, fmt.Errorf("Factory Runtime event subscription is required until Recordings migration")
	}
	return events.SubscribeFactoryEvents(ctx, reconnect, scope)
}

// GetEngineStateSnapshot returns the factory boundary's aggregate
// observability snapshot.
func (fs *SessionRuntime) GetEngineStateSnapshot(ctx context.Context) (*interfaces.EngineStateSnapshot[factory.PetriMarkingSnapshot, *factory.RuntimeNet], error) {
	if fs == nil {
		return nil, fmt.Errorf("factory session service is required")
	}
	runtime := fs.currentRuntimeService()
	if runtime == nil {
		return nil, fmt.Errorf("factory runtime is not available")
	}
	legacyObservation, err := runtimebinding.LegacyObservationForService(runtime)
	if err != nil {
		return nil, err
	}
	return legacyObservation.GetEngineStateSnapshot(ctx)
}

// CleanInvocationSnapshot forwards the Runtime-owned clean-invocation
// projection through the replaceable Factory Session runtime.
func (a *Assembly) scopedCleanInvocationSnapshot(fs *SessionRuntime, ctx context.Context) (factory.CleanInvocationSnapshot, error) {
	runtime := fs.currentRuntimeService()
	if runtime == nil {
		return factory.CleanInvocationSnapshot{}, factory.ErrNotRunning
	}
	return runtime.CleanInvocationSnapshot(ctx)
}

// ControlPause routes root control to the current replaceable runtime.
func (a *Assembly) scopedControlPause(fs *SessionRuntime, ctx context.Context, req factory.PauseRequest) (factory.PauseResult, error) {
	runtime := fs.currentRuntimeService()
	if runtime == nil {
		return factory.PauseResult{}, factory.ErrNotFound
	}
	return runtime.ControlPause(ctx, req)
}

// ControlResume routes root control to the current replaceable runtime.
func (a *Assembly) scopedControlResume(fs *SessionRuntime, ctx context.Context, req factory.ResumeRequest) (factory.ResumeResult, error) {
	runtime := fs.currentRuntimeService()
	if runtime == nil {
		return factory.ResumeResult{}, factory.ErrNotFound
	}
	return runtime.ControlResume(ctx, req)
}

// ControlTerminate routes root control to the current replaceable runtime.
func (a *Assembly) scopedControlTerminate(fs *SessionRuntime, ctx context.Context, req factory.TerminateRequest) (factory.TerminateResult, error) {
	runtime := fs.currentRuntimeService()
	if runtime == nil {
		return factory.TerminateResult{}, factory.ErrNotFound
	}
	return runtime.ControlTerminate(ctx, req)
}

// ControlWaitToComplete returns the current runtime's completion signal.
func (a *Assembly) scopedControlWaitToComplete(fs *SessionRuntime, req factory.WaitToCompleteRequest) factory.WaitToCompleteResult {
	runtime := fs.currentRuntimeService()
	if runtime != nil {
		return runtime.ControlWaitToComplete(req)
	}
	done := make(chan struct{})
	close(done)
	return factory.WaitToCompleteResult{Done: done}
}

// ControlMoveWork routes root work relocation to the current replaceable runtime.
func (a *Assembly) scopedControlMoveWork(fs *SessionRuntime, ctx context.Context, req factory.MoveWorkRequest) (factory.MoveWorkResult, error) {
	runtime := fs.currentRuntimeService()
	if runtime == nil {
		return factory.MoveWorkResult{}, factory.ErrNotFound
	}
	return runtime.ControlMoveWork(ctx, req)
}

// Observe routes root observation to the current replaceable runtime.
func (a *Assembly) scopedObserve(fs *SessionRuntime, ctx context.Context, req factory.ObserveRequest) (factory.ObserveResult, error) {
	runtime := fs.currentRuntimeService()
	if runtime == nil {
		return factory.ObserveResult{}, factory.ErrNotFound
	}
	return runtime.Observe(ctx, req)
}

// PlanDispatch routes root dispatch planning to the current replaceable runtime.
func (a *Assembly) scopedPlanDispatch(fs *SessionRuntime, ctx context.Context, req factory.PlanDispatchRequest) (factory.PlanDispatchResult, error) {
	runtime := fs.currentRuntimeService()
	if runtime == nil {
		return factory.PlanDispatchResult{}, factory.ErrNotFound
	}
	return runtime.PlanDispatch(ctx, req)
}

// AcceptDispatchResult routes correlated worker results to the current runtime.
func (a *Assembly) scopedAcceptDispatchResult(fs *SessionRuntime, ctx context.Context, req factory.AcceptDispatchResultRequest) (factory.AcceptDispatchResultResult, error) {
	runtime := fs.currentRuntimeService()
	if runtime == nil {
		return factory.AcceptDispatchResultResult{}, factory.ErrNotFound
	}
	return runtime.AcceptDispatchResult(ctx, req)
}

// InvokeWorker routes one orchestrator-resolved Worker invocation to the
// current runtime, which owns the Worker Sessions service and the canonical
// ledger the invocation's dispatch/Worker Session association must land on.
func (a *Assembly) scopedInvokeWorker(fs *SessionRuntime, ctx context.Context, req factory.InvokeWorkerRequest) (factory.InvokeWorkerResult, error) {
	runtime := fs.currentRuntimeService()
	if runtime == nil {
		return factory.InvokeWorkerResult{}, factory.ErrNotFound
	}
	return runtime.InvokeWorker(ctx, req)
}

type workerSessionsObservationProvider interface {
	WorkerSessionsObservation() workersessions.ObservationService
}

// WorkerSessionsObservation forwards the optional runtime projection through
// the replaceable Factory Session runtime. Without this capability adapter,
// service-mode HTTP binding sees only the broad Factory Runtime contract and
// leaves the public Worker Sessions routes unavailable.
func (a *Assembly) scopedWorkerSessionsObservation(fs *SessionRuntime) workersessions.ObservationService {
	runtime := fs.currentRuntimeService()
	provider, _ := runtime.(workerSessionsObservationProvider)
	if provider == nil {
		return nil
	}
	return provider.WorkerSessionsObservation()
}

// WorkerSessionsObservationForSession forwards the effective public Factory
// Session identity through the replaceable runtime read projection.
func (a *Assembly) scopedWorkerSessionsObservationForSession(fs *SessionRuntime, factorySessionID string) workersessions.ObservationService {
	reader := sessionLifecycleReader{}
	if fs != nil {
		reader.state, reader.active = a.state, &fs.runtimeState
	}
	return reader.WorkerSessionsObservationForSession(factorySessionID)
}

func (a *Assembly) scopeSubmitWorkFile(fs *SessionRuntime, ctx context.Context) error {
	workFile := fs.workFile
	if a.initialWorkFiles == nil {
		return fmt.Errorf("Factory Session initial Work file reader is required")
	}
	data, err := a.initialWorkFiles.ReadFile(workFile)
	if err != nil {
		return fmt.Errorf("read work file %s: %w", workFile, err)
	}
	request, err := work.ParseCanonicalWorkRequestJSON(data)
	if err != nil {
		return fmt.Errorf("parse work file %s: %w", workFile, err)
	}
	target := fs.currentRuntimeService()
	if target == nil {
		return fmt.Errorf("factory runtime is not available")
	}
	submitter, ok := runtimebinding.WorkAndEventIngressForService(target)
	if !ok {
		return fmt.Errorf("Factory Runtime work submission is required")
	}
	if _, err := submitter.SubmitWorkRequest(ctx, request); err != nil {
		return fmt.Errorf("submit initial work: %w", err)
	}
	fs.logger.Info("submitted initial work", zap.String("file", workFile))
	return nil
}

func (a *Assembly) scopeCurrentRuntimeConfig(fs *SessionRuntime) interfaces.LoadedFactorySource {
	if bundle := fs.currentRuntimeBundle(); bundle != nil {
		loaded, _ := bundle.LoadedRuntimeConfig().(interfaces.LoadedFactorySource)
		if loaded != nil {
			return loaded
		}
	}
	if fs != nil && a.state != nil {
		if runtime := a.state.CurrentRuntime(); runtime != nil && runtime.RuntimeConfig != nil {
			return runtime.RuntimeConfig
		}
	}
	if fs == nil || a.state == nil {
		return nil
	}
	record := runtimebinding.BundleFromSession(a.state.Default())
	if record == nil {
		return nil
	}
	loaded, _ := record.LoadedRuntimeConfig().(interfaces.LoadedFactorySource)
	return loaded
}

// CurrentRuntimeConfig returns the loaded definition/configuration for the
// currently selected runtime without exposing its host bundle.
func (a *Assembly) scopedCurrentRuntimeConfig(fs *SessionRuntime) interfaces.LoadedFactorySource {
	return fs.currentRuntimeConfig()
}

func (a *Assembly) scopeCurrentRuntimeService(fs *SessionRuntime) factory.Service {
	// The SessionRuntime is invocation-owned. Its active/startup bundle must win
	// over the process-wide selected session, which can belong to a concurrent
	// command using the same root-built process.
	if instance := fs.currentRuntimeBundle(); instance != nil {
		if service := instance.RuntimeService(); service != nil {
			return service
		}
	}
	if fs != nil && a.state != nil {
		if runtime := a.state.CurrentRuntime(); runtime != nil {
			if service := runtimebinding.ServiceForLiveRuntime(runtime); service != nil {
				return service
			}
		}
	}
	return nil
}

// StartupWorkerConfig returns the named worker from the built startup runtime config.
func (a *Assembly) scopedStartupWorkerConfig(fs *SessionRuntime, name string) (*interfaces.FactoryWorkerConfig, bool) {
	runtimeCfg := fs.currentRuntimeConfig()
	if runtimeCfg == nil {
		return nil, false
	}
	return runtimeCfg.Worker(name)
}

func (fs *SessionRuntime) syncActiveSessionDir(runtimeBundle factoryRuntimeBundle) {
	var a *Assembly
	if fs != nil {
		a = fs.owner
	}
	a.scopeSyncActiveSessionDir(fs, runtimeBundle)
}

func (fs *SessionRuntime) SubmitWorkRequest(ctx context.Context, request work.WorkRequest) (work.WorkRequestSubmitResult, error) {
	var a *Assembly
	if fs != nil {
		a = fs.owner
	}
	return a.scopedSubmitWorkRequest(fs, ctx, request)
}

func (fs *SessionRuntime) SubscribeFactoryEvents(ctx context.Context, reconnect *interfaces.FactoryEventReconnectCursor, scope interfaces.FactoryEventReconnectScope) (*interfaces.FactoryEventStream, error) {
	var a *Assembly
	if fs != nil {
		a = fs.owner
	}
	return a.scopedSubscribeFactoryEvents(fs, ctx, reconnect, scope)
}

func (fs *SessionRuntime) CleanInvocationSnapshot(ctx context.Context) (factory.CleanInvocationSnapshot, error) {
	var a *Assembly
	if fs != nil {
		a = fs.owner
	}
	return a.scopedCleanInvocationSnapshot(fs, ctx)
}

func (fs *SessionRuntime) ControlPause(ctx context.Context, req factory.PauseRequest) (factory.PauseResult, error) {
	var a *Assembly
	if fs != nil {
		a = fs.owner
	}
	return a.scopedControlPause(fs, ctx, req)
}

func (fs *SessionRuntime) ControlResume(ctx context.Context, req factory.ResumeRequest) (factory.ResumeResult, error) {
	var a *Assembly
	if fs != nil {
		a = fs.owner
	}
	return a.scopedControlResume(fs, ctx, req)
}

func (fs *SessionRuntime) ControlTerminate(ctx context.Context, req factory.TerminateRequest) (factory.TerminateResult, error) {
	var a *Assembly
	if fs != nil {
		a = fs.owner
	}
	return a.scopedControlTerminate(fs, ctx, req)
}

func (fs *SessionRuntime) ControlWaitToComplete(req factory.WaitToCompleteRequest) factory.WaitToCompleteResult {
	var a *Assembly
	if fs != nil {
		a = fs.owner
	}
	return a.scopedControlWaitToComplete(fs, req)
}

func (fs *SessionRuntime) ControlMoveWork(ctx context.Context, req factory.MoveWorkRequest) (factory.MoveWorkResult, error) {
	var a *Assembly
	if fs != nil {
		a = fs.owner
	}
	return a.scopedControlMoveWork(fs, ctx, req)
}

func (fs *SessionRuntime) Observe(ctx context.Context, req factory.ObserveRequest) (factory.ObserveResult, error) {
	var a *Assembly
	if fs != nil {
		a = fs.owner
	}
	return a.scopedObserve(fs, ctx, req)
}

func (fs *SessionRuntime) PlanDispatch(ctx context.Context, req factory.PlanDispatchRequest) (factory.PlanDispatchResult, error) {
	var a *Assembly
	if fs != nil {
		a = fs.owner
	}
	return a.scopedPlanDispatch(fs, ctx, req)
}

func (fs *SessionRuntime) AcceptDispatchResult(ctx context.Context, req factory.AcceptDispatchResultRequest) (factory.AcceptDispatchResultResult, error) {
	var a *Assembly
	if fs != nil {
		a = fs.owner
	}
	return a.scopedAcceptDispatchResult(fs, ctx, req)
}

func (fs *SessionRuntime) InvokeWorker(ctx context.Context, req factory.InvokeWorkerRequest) (factory.InvokeWorkerResult, error) {
	var a *Assembly
	if fs != nil {
		a = fs.owner
	}
	return a.scopedInvokeWorker(fs, ctx, req)
}

func (fs *SessionRuntime) PrepareInvocation(ctx context.Context, request factory.InvocationWork, caller *factory.InvocationCaller) (factory.InvocationWork, func(), error) {
	runtime := fs.currentRuntimeService()
	if runtime == nil {
		return factory.InvocationWork{}, nil, factory.ErrNotFound
	}
	return runtime.PrepareInvocation(ctx, request, caller.Clone())
}

var _ factory.Service = (*SessionRuntime)(nil)

func (fs *SessionRuntime) WorkerSessionsObservation() workersessions.ObservationService {
	var a *Assembly
	if fs != nil {
		a = fs.owner
	}
	return a.scopedWorkerSessionsObservation(fs)
}

func (fs *SessionRuntime) WorkerSessionsObservationForSession(factorySessionID string) workersessions.ObservationService {
	var a *Assembly
	if fs != nil {
		a = fs.owner
	}
	return a.scopedWorkerSessionsObservationForSession(fs, factorySessionID)
}

func (fs *SessionRuntime) submitWorkFile(ctx context.Context) error {
	var a *Assembly
	if fs != nil {
		a = fs.owner
	}
	return a.scopeSubmitWorkFile(fs, ctx)
}

func (fs *SessionRuntime) currentRuntimeConfig() interfaces.LoadedFactorySource {
	var a *Assembly
	if fs != nil {
		a = fs.owner
	}
	return a.scopeCurrentRuntimeConfig(fs)
}

func (fs *SessionRuntime) CurrentRuntimeConfig() interfaces.LoadedFactorySource {
	var a *Assembly
	if fs != nil {
		a = fs.owner
	}
	return a.scopedCurrentRuntimeConfig(fs)
}

func (fs *SessionRuntime) currentRuntimeService() factory.Service {
	var a *Assembly
	if fs != nil {
		a = fs.owner
	}
	return a.scopeCurrentRuntimeService(fs)
}

func (fs *SessionRuntime) StartupWorkerConfig(name string) (*interfaces.FactoryWorkerConfig, bool) {
	var a *Assembly
	if fs != nil {
		a = fs.owner
	}
	return a.scopedStartupWorkerConfig(fs, name)
}
