package internal

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// NewRuntimeRoot constructs the one process-scoped Recordings root. Runtime
// ledgers and lifecycle bindings are acquired through OpenRuntime; no caller
// receives a constructor for those private resources.
func NewRuntimeRoot(
	targets recordings.LiveRecordingTargetPlanner,
	writeFile func(string, []byte) error,
	publication interface {
		Publish(context.Context, string, []byte) error
		Read(context.Context, string) ([]byte, error)
	},
	captureSnapshot factorydefinitions.LoadedFactorySnapshotCapturer,
	decodeSnapshot factorydefinitions.FactorySnapshotJSONDecoder,
	decodeRuntimeConfig factorydefinitions.ReplayRuntimeConfigDecoder,
	replayInputs recordings.ReplayInputLoader,
	clocks ...recordings.RecordingClock,
) recordings.Root {
	router := newRuntimeLedgerRouter()
	projection := NewProjectionService()
	var writer recordings.RecordingSnapshotWriter
	var tickers recordings.RecordingFlushTickerFactory
	if writeFile != nil {
		writer = NewReplayRecordingSnapshotWriter(writeFile)
		tickers = NewRecordingFlushTickerFactory()
	}
	service := NewServiceWithLifecycleEffects(
		router,
		projection,
		targets,
		writer,
		tickers,
		publication,
		clocks...,
	)
	root, ok := service.(*combinedService)
	if !ok || root == nil {
		return nil
	}
	root.runtimeRouter = router
	root.runtimeSnapshotCapture = captureSnapshot
	root.replaySnapshotDecoder = decodeSnapshot
	root.replayConfigDecoder = decodeRuntimeConfig
	root.replayInputs = replayInputs
	return root
}

var _ recordings.Root = (*combinedService)(nil)

func (service *combinedService) Projection() recordings.ProjectionService {
	if service == nil {
		return nil
	}
	return service.ProjectionService
}

func (service *combinedService) ReplayClock(
	artifact *recordings.ReplayArtifact,
) recordings.Clock {
	return NewReplayClock(artifact)
}

func (service *combinedService) ReplayExecution(
	artifact *recordings.ReplayArtifact,
) (
	workers.Provider,
	workers.CommandRunner,
	[]recordings.ReplayHook,
	recordings.CompletionDeliveryPlanner,
	error,
) {
	if service == nil {
		return nil, nil, nil, nil, fmt.Errorf("Recordings runtime opening is unavailable")
	}
	return NewReplayExecution(
		artifact,
		service.replaySnapshotDecoder,
		service.replayConfigDecoder,
	)
}

// LoadReplayInput keeps path-based replay classification on the Recordings
// root while retaining the narrow pre-ledger capability used by loading.
func (service *combinedService) LoadReplayInput(
	request recordings.LoadReplayInputRequest,
) (recordings.LoadReplayInputResult, error) {
	if service == nil || service.replayInputs == nil {
		return recordings.LoadReplayInputResult{}, recordings.ErrMissingReplayArtifact
	}
	return service.replayInputs.LoadReplayInput(request)
}

// OpenRuntime acquires the runtime-owned ledger and, when recording is
// enabled, binds its recorder to this root's shared lifecycle owner.
func (service *combinedService) OpenRuntime(
	ctx context.Context,
	request recordings.RuntimeScopeRequest,
) (recordings.RuntimeScopeResult, error) {
	if service == nil || service.runtimeRouter == nil {
		return recordings.RuntimeScopeResult{}, fmt.Errorf("Recordings runtime opening is unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return recordings.RuntimeScopeResult{}, err
	}
	if request.Topology == nil {
		return recordings.RuntimeScopeResult{}, fmt.Errorf("Recordings runtime topology is required")
	}
	if request.Now == nil {
		return recordings.RuntimeScopeResult{}, fmt.Errorf("Recordings runtime clock is required")
	}
	streamGenerationID := strings.TrimSpace(request.RecordingID)
	if streamGenerationID == "" {
		streamGenerationID = "recordings-runtime"
	}
	streamGenerationID += "-" + strings.TrimSpace(request.FactorySessionID)
	ledger := NewRuntimeLedger(
		request.Topology,
		request.Now,
		streamGenerationID,
		request.Definitions,
	)
	if ledger == nil {
		return recordings.RuntimeScopeResult{}, fmt.Errorf("Recordings runtime ledger is unavailable")
	}
	routeKey := strings.TrimSpace(request.FactorySessionID)
	if routeKey == "" {
		routeKey = ledger.StreamGenerationID()
	}
	if err := service.runtimeRouter.register(routeKey, ledger); err != nil {
		return recordings.RuntimeScopeResult{}, err
	}
	cleanupRoute := true
	defer func() {
		if cleanupRoute {
			service.runtimeRouter.unregister(routeKey, ledger)
		}
	}()

	if strings.TrimSpace(request.RecordPath) == "" {
		cleanupRoute = false
		return recordings.RuntimeScopeResult{
			Ledger:   ledger,
			Recorder: &runtimeScopeRecorder{owner: service, routeKey: routeKey, ledger: ledger},
		}, nil
	}
	if service.runtimeSnapshotCapture == nil {
		return recordings.RuntimeScopeResult{}, fmt.Errorf("Recordings runtime snapshot capture is required")
	}
	recorder, err := NewLifecycleRuntimeRecorder(
		request.FlushInterval,
		request.LoadedFactory,
		request.Now,
		request.RecordingID,
		request.RecordPath,
		service.runtimeSnapshotCapture,
	)
	if err != nil {
		return recordings.RuntimeScopeResult{}, err
	}
	if recorder == nil {
		return recordings.RuntimeScopeResult{}, fmt.Errorf("Recordings runtime recorder is unavailable")
	}
	lifecycleRecorder, ok := recorder.(*lifecycleRuntimeRecorder)
	if !ok || lifecycleRecorder == nil {
		return recordings.RuntimeScopeResult{}, fmt.Errorf("Recordings runtime recorder has unsupported implementation")
	}
	binder, ok := recorder.(recordings.RuntimeRecordingBinder)
	if !ok {
		return recordings.RuntimeScopeResult{}, fmt.Errorf("Recordings runtime recorder cannot bind")
	}
	if err := binder.BindRecordingLifecycle(
		recordings.RecordingLifecycle(service),
		requestScope(request),
	); err != nil {
		if lifecycleRecorder.recordingID != "" {
			_, _ = service.FinishRecording(recordings.FinishRecordingRequest{
				RecordingID: recordings.RecordingID(lifecycleRecorder.recordingID),
				FinishedAt:  request.Now().UTC(),
			})
		}
		return recordings.RuntimeScopeResult{}, fmt.Errorf("bind Recordings runtime scope: %w", err)
	}
	ref := service.newRecordingScope()
	binding := &recordingScopeBinding{
		recordingID: recordings.RecordingID(lifecycleRecorder.recordingID),
		eventScope:  lifecycleRecorder.scope,
		replayPlans: make(map[recordings.ReplayPlanHandle]struct{}),
	}
	service.scopeMu.Lock()
	service.scopeByRef[ref] = binding
	service.scopeMu.Unlock()
	cleanupRoute = false
	return recordings.RuntimeScopeResult{
		Ledger: ledger,
		Recorder: &runtimeScopeRecorder{
			inner: recorder, owner: service, scope: ref, routeKey: routeKey, ledger: ledger,
		},
		Scope: ref,
	}, nil
}

func requestScope(request recordings.RuntimeScopeRequest) recordings.CanonicalEventScope {
	return recordings.CanonicalEventScope{FactorySessionID: strings.TrimSpace(request.FactorySessionID)}
}

// closeRuntimeRecordingScope records the terminal state already produced by
// the runtime recorder and closes its opaque scope without calling Finish a
// second time. The runtime recorder owns terminal-event emission and lifecycle
// finalization; this method only closes the root-owned scope bookkeeping.
func (service *combinedService) closeRuntimeRecordingScope(
	ctx context.Context,
	ref recordings.RecordingScopeRef,
	finalizeErr error,
) error {
	if service == nil || ref.IsZero() {
		return finalizeErr
	}
	binding, err := service.recordingScope(ref)
	if err != nil {
		return errors.Join(finalizeErr, err)
	}
	binding.mu.Lock()
	defer binding.mu.Unlock()
	if binding.closed {
		return errors.Join(finalizeErr, binding.finalizeErr)
	}
	if err := recordingScopeContext(ctx).Err(); err != nil {
		return errors.Join(finalizeErr, err)
	}
	if !binding.finalized {
		binding.finalized = true
		binding.finalizeErr = finalizeErr
		status, statusErr := service.scopeStatus(ref, binding)
		binding.finalizeErr = errors.Join(binding.finalizeErr, statusErr)
		if statusErr == nil {
			binding.terminal = status
		}
	}
	binding.closed = true
	return errors.Join(finalizeErr, binding.finalizeErr)
}

type runtimeScopeRecorder struct {
	mu          sync.Mutex
	inner       recordings.RuntimeRecorder
	owner       *combinedService
	scope       recordings.RecordingScopeRef
	routeKey    string
	ledger      recordings.RuntimeEventLedger
	finalized   bool
	finalizeErr error
}

func (recorder *runtimeScopeRecorder) Start(ctx context.Context) {
	if recorder != nil && recorder.inner != nil {
		recorder.inner.Start(ctx)
	}
}

func (recorder *runtimeScopeRecorder) Stop() {
	if recorder != nil && recorder.inner != nil {
		recorder.inner.Stop()
	}
}

func (recorder *runtimeScopeRecorder) RecordEvent(event recordings.FactoryEvent) {
	if recorder != nil && recorder.inner != nil {
		recorder.inner.RecordEvent(event)
	}
}

func (recorder *runtimeScopeRecorder) RecordError(err error) {
	if recorder != nil && recorder.inner != nil {
		recorder.inner.RecordError(err)
	}
}

func (recorder *runtimeScopeRecorder) Flush() error {
	if recorder == nil || recorder.inner == nil {
		return nil
	}
	return recorder.inner.Flush()
}

func (recorder *runtimeScopeRecorder) Err() error {
	if recorder == nil {
		return nil
	}
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	var recorderErr error
	if recorder.inner != nil {
		recorderErr = recorder.inner.Err()
	}
	return errors.Join(recorder.finalizeErr, recorderErr)
}

func (recorder *runtimeScopeRecorder) Finish(finishedAt time.Time) {
	_ = recorder.Finalize(finishedAt)
}

func (recorder *runtimeScopeRecorder) Finalize(finishedAt time.Time) error {
	if recorder == nil {
		return nil
	}
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	if recorder.finalized {
		err := recorder.finalizeErr
		return err
	}
	recorder.finalized = true

	var finalizeErr error
	if recorder.inner != nil {
		finalizeErr = recorder.inner.Finalize(finishedAt)
	}
	var closeErr error
	if !recorder.scope.IsZero() && recorder.owner != nil {
		closeErr = recorder.owner.closeRuntimeRecordingScope(
			context.Background(), recorder.scope, finalizeErr,
		)
	}
	if recorder.owner != nil && recorder.owner.runtimeRouter != nil {
		recorder.owner.runtimeRouter.unregister(recorder.routeKey, recorder.ledger)
	}
	recorder.finalizeErr = errors.Join(finalizeErr, closeErr)
	return errors.Join(finalizeErr, closeErr)
}

var _ recordings.RuntimeRecorder = (*runtimeScopeRecorder)(nil)
