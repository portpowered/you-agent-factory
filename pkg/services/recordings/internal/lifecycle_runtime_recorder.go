package internal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	canonicalpkg "github.com/portpowered/infinite-you/pkg/services/recordings/internal/canonical"
	recordingevents "github.com/portpowered/infinite-you/pkg/services/recordings/internal/events"
	replayimpl "github.com/portpowered/infinite-you/pkg/services/recordings/internal/replay"
	historicalquery "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/historical_query"
	"sync"
	"time"
)

// lifecycleRuntimeRecorder adapts Factory Runtime's focused recording port to
// the session's Recordings-owned RecordingLifecycle capability without
// becoming a second lifecycle owner.
type lifecycleRuntimeRecorder struct {
	mu                 sync.Mutex
	lifecycle          recordings.RecordingLifecycle
	requestedID        recordings.LifecycleRecordingID
	recordingID        recordings.LifecycleRecordingID
	scope              recordings.CanonicalEventScope
	target             recordings.LifecycleArtifactReference
	canonicalSessionID string
	flushInterval      time.Duration
	now                func() time.Time
	startedAt          time.Time
	streamGenerationID string
	initialEvent       recordings.FactoryEvent
	initialProvenance  []recordings.RecordingSecret
	seen               map[string]struct{}
	nextSequence       recordings.CanonicalEventSequence
	producerErr        error
	finalizeErr        error
	stopErr            error
	pending            []pendingRuntimeRecording
}

type pendingRuntimeRecording struct {
	event      *recordings.FactoryEvent
	provenance []recordings.RecordingSecret
	err        error
}

var _ recordings.RuntimeRecorder = (*lifecycleRuntimeRecorder)(nil)
var _ recordings.RuntimeRecorderWithProvenance = (*lifecycleRuntimeRecorder)(nil)
var _ recordings.RuntimeRecordingBinder = (*lifecycleRuntimeRecorder)(nil)

// NewLifecycleRuntimeRecorder prepares a runtime recorder whose lifecycle is
// activated only when Factory Sessions binds a RecordingLifecycle capability.
func NewLifecycleRuntimeRecorder(
	flushInterval time.Duration,
	loaded factorydefinitions.LoadedFactorySource,
	now func() time.Time,
	recordingID string,
	recordPath string,
	captureLoadedFactorySnapshot factorydefinitions.LoadedFactorySnapshotCapturer,
	streamGenerationIDs ...string,
) (recordings.RuntimeRecorder, error) {
	if strings.TrimSpace(recordPath) == "" {
		return nil, nil
	}
	if now == nil {
		return nil, fmt.Errorf("recording clock is required")
	}
	if captureLoadedFactorySnapshot == nil {
		return nil, fmt.Errorf("loaded Factory snapshot capturer is required")
	}
	recordedAt := now().UTC()
	sourceDirectory := ""
	if loaded != nil {
		sourceDirectory = loaded.FactoryDir()
	}
	snapshot, err := captureLoadedFactorySnapshot(loaded, sourceDirectory, nil)
	if err != nil {
		return nil, fmt.Errorf("build replay artifact config: %w", err)
	}
	artifact, err := replayimpl.NewEventLogArtifact(
		recordedAt,
		snapshot,
		&recordings.ReplayWallClockMetadata{StartedAt: recordedAt},
		recordings.ReplayDiagnostics{},
	)
	if err != nil {
		return nil, err
	}
	streamGenerationID := strings.TrimSpace(recordingID)
	if len(streamGenerationIDs) > 0 && strings.TrimSpace(streamGenerationIDs[0]) != "" {
		streamGenerationID = strings.TrimSpace(streamGenerationIDs[0])
	}
	return &lifecycleRuntimeRecorder{
		requestedID:        recordings.LifecycleRecordingID(strings.TrimSpace(recordingID)),
		target:             recordings.LifecycleArtifactReference(recordPath),
		flushInterval:      flushInterval,
		now:                now,
		startedAt:          recordedAt,
		streamGenerationID: streamGenerationID,
		initialEvent:       artifact.Events[0],
		seen:               make(map[string]struct{}),
	}, nil
}

// BindRecordingLifecycle implements recordings.RuntimeRecordingBinder by
// binding this runtime recorder to an already-constructed RecordingLifecycle
// capability rather than the broad Recordings Service.
func (recorder *lifecycleRuntimeRecorder) BindRecordingLifecycle(
	lifecycle recordings.RecordingLifecycle,
	scope recordings.CanonicalEventScope,
) error {
	if recorder == nil {
		return nil
	}
	if lifecycle == nil {
		return fmt.Errorf("Recordings lifecycle capability is required")
	}
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	if recorder.lifecycle != nil {
		if recorder.lifecycle == lifecycle && recorder.scope == scope {
			return nil
		}
		return recordings.ErrRecordingBindingConflict
	}
	result, err := lifecycle.Begin(recordings.BeginRecordingRequest{
		Enabled:            true,
		RecordingID:        recorder.requestedID,
		Scope:              recordings.LifecycleScope{FactorySessionID: scope.FactorySessionID},
		Artifact:           recorder.target,
		CanonicalSessionID: recorder.canonicalSessionID,
		ReportedSessionID:  scope.FactorySessionID,
		FlushInterval:      recorder.flushInterval,
	})
	if err != nil {
		return err
	}
	recorder.lifecycle = lifecycle
	recorder.recordingID = result.Status.RecordingID
	recorder.scope = scope
	if err := recorder.recordEventLockedWithProvenance(
		recorder.initialEvent,
		recorder.initialProvenance,
	); err != nil {
		appendErr := fmt.Errorf("record initial Factory snapshot: %w", err)
		recorder.stopErr = recorder.lifecycle.Stop(recordings.StopLifecycleRequest{
			RecordingID: recorder.recordingID,
		})
		return errors.Join(appendErr, recorder.stopErr)
	}
	for _, pending := range recorder.pending {
		if pending.event != nil {
			if err := recorder.recordEventLockedWithProvenance(*pending.event, pending.provenance); err != nil {
				recorder.recordErrorLocked("producer_boundary_failed", "accept Factory event", err)
				recorder.producerErr = errors.Join(recorder.producerErr, err)
			}
		} else {
			recorder.recordErrorLocked(
				"producer_boundary_failed",
				"Factory event producer failed",
				pending.err,
			)
		}
	}
	recorder.pending = nil
	return nil
}

func (recorder *lifecycleRuntimeRecorder) Start(context.Context) {}

func (recorder *lifecycleRuntimeRecorder) Stop() {
	if recorder == nil {
		return
	}
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	if recorder.lifecycle == nil {
		return
	}
	recorder.stopErr = recorder.lifecycle.Stop(recordings.StopLifecycleRequest{
		RecordingID: recorder.recordingID,
	})
}

func (recorder *lifecycleRuntimeRecorder) RecordEvent(event recordings.FactoryEvent) {
	recorder.RecordEventWithProvenance(event, nil)
}

func (recorder *lifecycleRuntimeRecorder) RecordEventWithProvenance(
	event recordings.FactoryEvent,
	provenance []recordings.RecordingSecret,
) {
	if recorder == nil {
		return
	}
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	if recorder.lifecycle == nil {
		event.Payload = append([]byte(nil), event.Payload...)
		recorder.pending = append(recorder.pending, pendingRuntimeRecording{
			event:      &event,
			provenance: append([]recordings.RecordingSecret(nil), provenance...),
		})
		return
	}
	if err := recorder.recordEventLockedWithProvenance(event, provenance); err != nil {
		recorder.recordErrorLocked("producer_boundary_failed", "accept Factory event", err)
		recorder.producerErr = errors.Join(recorder.producerErr, err)
	}
}

func (recorder *lifecycleRuntimeRecorder) recordEventLocked(
	event recordings.FactoryEvent,
) error {
	return recorder.recordEventLockedWithProvenance(event, nil)
}

func (recorder *lifecycleRuntimeRecorder) recordEventLockedWithProvenance(
	event recordings.FactoryEvent,
	provenance []recordings.RecordingSecret,
) error {
	if recorder.lifecycle == nil {
		return fmt.Errorf("Recordings lifecycle capability is not bound")
	}
	if _, exists := recorder.seen[event.Id]; exists {
		return nil
	}
	nextSequence := recorder.nextSequence
	if status, err := recorder.lifecycle.Status(recordings.LifecycleStatusRequest{
		RecordingID: recorder.recordingID,
	}); err == nil && status.Status.LastEvent != nil {
		currentNext := recordings.CanonicalEventSequence(status.Status.LastEvent.Sequence + 1)
		if currentNext > nextSequence {
			nextSequence = currentNext
		}
	}
	event.Context.Sequence = int(nextSequence)
	streamGenerationID := recorder.streamGenerationID
	if streamGenerationID == "" {
		streamGenerationID = string(recorder.recordingID)
	}
	eventForRecording := event
	if event.Type == recordings.FactoryEventTypeSessionCompleted {
		// Keep the live Factory Event stream on its public selector, but retain
		// the durable recording identity in SourceContext. The recording
		// lifecycle scope must remain public so its route and binding continue
		// to validate against the active Factory Session.
		if canonicalSessionID := strings.TrimSpace(recorder.canonicalSessionID); canonicalSessionID != "" {
			eventForRecording.Context.SessionID = &canonicalSessionID
		}
	}
	canonical := canonicalpkg.CanonicalEventFromFactory(eventForRecording, streamGenerationID)
	canonical.Scope = recorder.scope
	canonical.Sequence = nextSequence
	canonical.Cursor.Sequence = nextSequence
	result, err := recorder.lifecycle.AppendEvent(recordings.AppendLifecycleEventRequest{
		RecordingID:      recorder.recordingID,
		Event:            lifecycleEventFromCanonical(canonical),
		SecretProvenance: append([]recordings.RecordingSecret(nil), provenance...),
	})
	if err != nil {
		return err
	}
	recorder.seen[event.Id] = struct{}{}
	recorder.nextSequence = recordings.CanonicalEventSequence(result.Status.AcceptedEvents)
	return nil
}

func (recorder *lifecycleRuntimeRecorder) RecordError(err error) {
	if recorder == nil || err == nil {
		return
	}
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	if recorder.lifecycle == nil {
		recorder.pending = append(recorder.pending, pendingRuntimeRecording{err: err})
		return
	}
	recorder.recordErrorLocked("producer_boundary_failed", "Factory event producer failed", err)
	recorder.producerErr = errors.Join(recorder.producerErr, err)
}

func (recorder *lifecycleRuntimeRecorder) recordErrorLocked(code, operation string, cause error) {
	if recorder.lifecycle == nil || cause == nil {
		return
	}
	_, _ = recorder.lifecycle.RecordFailure(recordings.RecordLifecycleFailureRequest{
		RecordingID: recorder.recordingID,
		Failure: recordings.LifecycleFailure{
			Code:       code,
			Message:    operation + ": " + cause.Error(),
			RecordedAt: recorder.now().UTC(),
		},
		Cause: cause,
	})
}

func (recorder *lifecycleRuntimeRecorder) Finish(finishedAt time.Time) {
	_ = recorder.Finalize(finishedAt)
}

func (recorder *lifecycleRuntimeRecorder) Flush() error {
	if recorder == nil {
		return nil
	}
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	if recorder.lifecycle == nil {
		return fmt.Errorf("Recordings lifecycle capability is not bound")
	}
	_, err := recorder.lifecycle.Flush(recordings.FlushLifecycleRequest{
		RecordingID: recorder.recordingID,
	})
	return errors.Join(err, recorder.producerErr)
}

// Err reports the last preserved cleanup or finalize failure. Stop failures
// surface here even before Finalize runs, so startup callers that must stop
// periodic work on an early failure (before finish) can still observe and
// join the cleanup cause rather than discarding it.
func (recorder *lifecycleRuntimeRecorder) Err() error {
	if recorder == nil {
		return nil
	}
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	return errors.Join(recorder.stopErr, recorder.producerErr, recorder.finalizeErr)
}

func (recorder *lifecycleRuntimeRecorder) Finalize(finishedAt time.Time) error {
	if recorder == nil {
		return nil
	}
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	if recorder.lifecycle == nil {
		return fmt.Errorf("Recordings lifecycle capability is not bound")
	}
	if recorder.finalizeErr != nil {
		return recorder.finalizeErr
	}
	if err := recorder.recordEventLocked(recordingevents.RunFinishedFactoryEvent(recorder.startedAt, finishedAt)); err != nil {
		recorder.recordErrorLocked("terminal_metadata_failed", "record terminal Factory event", err)
		recorder.producerErr = errors.Join(recorder.producerErr, err)
	}
	_, recorder.finalizeErr = recorder.lifecycle.Finish(recordings.FinishLifecycleRequest{
		RecordingID: recorder.recordingID,
		FinishedAt:  finishedAt,
	})
	return recorder.finalizeErr
}

func lifecycleEventFromCanonical(event recordings.CanonicalEvent) recordings.LifecycleEvent {
	return recordings.LifecycleEvent{
		ID:          string(event.ID),
		Sequence:    int64(event.Sequence),
		FactoryTick: event.FactoryTick,
		Scope:       recordings.LifecycleScope{FactorySessionID: event.Scope.FactorySessionID},
		Cursor: recordings.LifecycleEventCursor{
			StreamGenerationID: event.Cursor.StreamGenerationID,
			Sequence:           int64(event.Cursor.Sequence),
		},
		RecordedAt:    event.RecordedAt,
		Kind:          string(event.Kind),
		Payload:       event.Payload,
		SourceContext: event.SourceContext,
	}
}

// NewRuntimeRoot constructs the one process-scoped Recordings root. Runtime
// ledgers and lifecycle bindings are acquired through OpenRuntime; no caller
// receives a constructor for those private resources.
func NewRuntimeRoot(
	targets recordings.LiveRecordingTargetPlanner,
	writeFile func(string, []byte) error,
	readFile recordings.RecordingReadFile,
	publication interface {
		Publish(context.Context, string, []byte) error
		Read(context.Context, string) ([]byte, error)
	},
	captureSnapshot factorydefinitions.LoadedFactorySnapshotCapturer,
	decodeSnapshot factorydefinitions.FactorySnapshotJSONDecoder,
	decodeRuntimeConfig factorydefinitions.ReplayRuntimeConfigDecoder,
	replayInputs recordings.ReplayInputLoader,
	logger logging.Logger,
	clocks ...recordings.RecordingClock,
) recordings.Service {
	return NewRuntimeRootWithHistoricalQuery(
		targets,
		writeFile,
		readFile,
		publication,
		captureSnapshot,
		decodeSnapshot,
		decodeRuntimeConfig,
		replayInputs,
		logger,
		nil,
		clocks...,
	)
}

// NewRuntimeRootWithHistoricalQuery constructs the process-scoped Recordings
// root with the Wire-selected durable historical reader.
func NewRuntimeRootWithHistoricalQuery(
	targets recordings.LiveRecordingTargetPlanner,
	writeFile func(string, []byte) error,
	readFile recordings.RecordingReadFile,
	publication interface {
		Publish(context.Context, string, []byte) error
		Read(context.Context, string) ([]byte, error)
	},
	captureSnapshot factorydefinitions.LoadedFactorySnapshotCapturer,
	decodeSnapshot factorydefinitions.FactorySnapshotJSONDecoder,
	decodeRuntimeConfig factorydefinitions.ReplayRuntimeConfigDecoder,
	replayInputs recordings.ReplayInputLoader,
	logger logging.Logger,
	historicalQuery historicalquery.Service,
	clocks ...recordings.RecordingClock,
) recordings.Service {
	return NewRuntimeRootWithHistoricalQueryAndAppender(
		targets,
		writeFile,
		nil,
		readFile,
		publication,
		captureSnapshot,
		decodeSnapshot,
		decodeRuntimeConfig,
		replayInputs,
		logger,
		historicalQuery,
		clocks...,
	)
}

var _ recordings.Service = (*combinedService)(nil)
var _ recordings.RuntimeOpening = (*combinedService)(nil)

func (service *combinedService) Projection() recordings.ProjectionService {
	if service == nil {
		return nil
	}
	return service.ProjectionService
}

// ReconstructCanonicalFactoryWorldState publishes the typed canonical
// reduction through the Recordings root without exposing its implementation
// package to runtime callers.
func (service *combinedService) ReconstructCanonicalFactoryWorldState(
	events []recordings.FactoryEvent,
	selectedTick int,
) (recordings.FactoryWorldState, error) {
	if service == nil || service.ProjectionService == nil {
		return recordings.FactoryWorldState{}, fmt.Errorf("Recordings projection is unavailable")
	}
	return service.ProjectionService.ReconstructFactoryWorldState(events, selectedTick)
}

func (service *combinedService) ReplayClock(
	artifact *recordings.ReplayArtifact,
) recordings.Clock {
	return NewReplayClock(artifact)
}

func (service *combinedService) ReplayExecution(
	artifact *recordings.ReplayArtifact,
) (
	providers.Service,
	platformprocess.CommandRunner,
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
	if err := validateRuntimeOpening(service, ctx, request); err != nil {
		return recordings.RuntimeScopeResult{}, err
	}
	streamGenerationID := runtimeStreamGenerationID(request)
	ledger, routeKey, err := service.openRuntimeLedger(request, streamGenerationID)
	if err != nil {
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
	recorder, ref, err := service.openRuntimeRecordingScope(request, streamGenerationID, routeKey, ledger)
	if err != nil {
		return recordings.RuntimeScopeResult{}, err
	}
	cleanupRoute = false
	return recordings.RuntimeScopeResult{
		Ledger:   ledger,
		Recorder: recorder,
		Scope:    ref,
	}, nil
}

func validateRuntimeOpening(
	service *combinedService,
	ctx context.Context,
	request recordings.RuntimeScopeRequest,
) error {
	if service == nil || service.runtimeRouter == nil {
		return fmt.Errorf("Recordings runtime opening is unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if request.Topology == nil {
		return fmt.Errorf("Recordings runtime topology is required")
	}
	if request.Now == nil {
		return fmt.Errorf("Recordings runtime clock is required")
	}
	return nil
}

func (service *combinedService) openRuntimeRecordingScope(
	request recordings.RuntimeScopeRequest,
	streamGenerationID string,
	routeKey string,
	ledger recordings.RuntimeEventLedger,
) (recordings.RuntimeRecorder, recordings.RecordingScopeRef, error) {
	if service.runtimeSnapshotCapture == nil {
		return nil, recordings.RecordingScopeRef{}, fmt.Errorf("Recordings runtime snapshot capture is required")
	}
	recorder, err := service.newLifecycleRuntimeRecorder(request, streamGenerationID)
	if err != nil {
		return nil, recordings.RecordingScopeRef{}, err
	}
	if err := service.bindRuntimeRecorder(recorder, request); err != nil {
		return nil, recordings.RecordingScopeRef{}, err
	}
	ref := service.newRecordingScope()
	service.scopeMu.Lock()
	service.scopeByRef[ref] = &recordingScopeBinding{
		recordingID: recordings.RecordingID(recorder.recordingID),
		eventScope:  recorder.scope,
		replayPlans: make(map[recordings.ReplayPlanHandle]struct{}),
	}
	service.scopeMu.Unlock()
	return &runtimeScopeRecorder{
		inner: recorder, owner: service, scope: ref, routeKey: routeKey, ledger: ledger,
	}, ref, nil
}

func (service *combinedService) newLifecycleRuntimeRecorder(
	request recordings.RuntimeScopeRequest,
	streamGenerationID string,
) (*lifecycleRuntimeRecorder, error) {
	recorder, err := NewLifecycleRuntimeRecorder(
		request.FlushInterval,
		request.LoadedFactory,
		request.Now,
		request.RecordingID,
		request.RecordPath,
		service.runtimeSnapshotCapture,
		streamGenerationID,
	)
	if err != nil {
		return nil, err
	}
	if recorder == nil {
		return nil, fmt.Errorf("Recordings runtime recorder is unavailable")
	}
	lifecycleRecorder, ok := recorder.(*lifecycleRuntimeRecorder)
	if !ok || lifecycleRecorder == nil {
		return nil, fmt.Errorf("Recordings runtime recorder has unsupported implementation")
	}
	lifecycleRecorder.canonicalSessionID = strings.TrimSpace(request.CanonicalSessionID)
	if initialEvent, ok := resumeRunRequestEvent(request.ReplayEvents); ok {
		// A successor recording must retain the source RUN_REQUEST payload.
		// Re-capturing the rehydrated RuntimeSnapshot is not hash-idempotent
		// and would disclose drift for an unchanged checkout.
		lifecycleRecorder.initialEvent = initialEvent
	}
	if source, ok := request.LoadedFactory.(interface {
		InvocationSensitiveJSONPointers() []string
	}); ok {
		lifecycleRecorder.initialProvenance = recordingSecretsForEventPayload(
			lifecycleRecorder.initialEvent.Payload,
			source.InvocationSensitiveJSONPointers(),
		)
	}
	return lifecycleRecorder, nil
}

func recordingSecretsForEventPayload(
	payload json.RawMessage,
	pointers []string,
) []recordings.RecordingSecret {
	if len(payload) == 0 || len(pointers) == 0 {
		return nil
	}
	var document any
	if err := json.Unmarshal(payload, &document); err != nil {
		return nil
	}
	secrets := make([]recordings.RecordingSecret, 0, len(pointers))
	for _, pointer := range pointers {
		jsonPointer := "/factory"
		if pointer != "" {
			jsonPointer += pointer
		}
		if !recordingJSONPointerExists(document, jsonPointer) {
			continue
		}
		secrets = append(secrets, recordings.RecordingSecret{
			JSONPointer: jsonPointer,
			Provenance:  recordings.RecordingSecretProvenanceDeclared,
		})
	}
	return secrets
}

func recordingJSONPointerExists(document any, pointer string) bool {
	if pointer == "" {
		return true
	}
	if !strings.HasPrefix(pointer, "/") {
		return false
	}
	current := document
	for _, encoded := range strings.Split(pointer[1:], "/") {
		segment, ok := decodeRecordingJSONPointerToken(encoded)
		if !ok {
			return false
		}
		switch value := current.(type) {
		case map[string]any:
			var exists bool
			current, exists = value[segment]
			if !exists {
				return false
			}
		case []any:
			index, err := strconv.Atoi(segment)
			if err != nil || index < 0 || index >= len(value) {
				return false
			}
			current = value[index]
		default:
			return false
		}
	}
	return true
}

func decodeRecordingJSONPointerToken(token string) (string, bool) {
	if !strings.Contains(token, "~") {
		return token, true
	}
	var builder strings.Builder
	for index := 0; index < len(token); index++ {
		if token[index] != '~' {
			builder.WriteByte(token[index])
			continue
		}
		if index+1 >= len(token) {
			return "", false
		}
		index++
		switch token[index] {
		case '0':
			builder.WriteByte('~')
		case '1':
			builder.WriteByte('/')
		default:
			return "", false
		}
	}
	return builder.String(), true
}

func resumeRunRequestEvent(events []factorydefinitions.FactoryEvent) (recordings.FactoryEvent, bool) {
	for _, event := range events {
		if event.Type == factorydefinitions.FactoryEventTypeRunRequest {
			return event.Clone(), true
		}
	}
	return recordings.FactoryEvent{}, false
}

func (service *combinedService) bindRuntimeRecorder(
	recorder *lifecycleRuntimeRecorder,
	request recordings.RuntimeScopeRequest,
) error {
	binder, ok := any(recorder).(recordings.RuntimeRecordingBinder)
	if !ok {
		return fmt.Errorf("Recordings runtime recorder cannot bind")
	}
	if err := binder.BindRecordingLifecycle(
		recordings.RecordingLifecycle(service),
		requestScope(request),
	); err != nil {
		if recorder.recordingID != "" {
			_, _ = service.FinishRecording(recordings.FinishRecordingRequest{
				RecordingID: recordings.RecordingID(recorder.recordingID),
				FinishedAt:  request.Now().UTC(),
			})
		}
		return fmt.Errorf("bind Recordings runtime scope: %w", err)
	}
	return nil
}

func requestScope(request recordings.RuntimeScopeRequest) recordings.CanonicalEventScope {
	return recordings.CanonicalEventScope{FactorySessionID: strings.TrimSpace(request.FactorySessionID)}
}

func runtimeStreamGenerationID(request recordings.RuntimeScopeRequest) string {
	recordingID := strings.TrimSpace(request.RecordingID)
	if recordingID == "" {
		recordingID = "recordings-runtime"
	}
	return recordingID + "-" + strings.TrimSpace(request.FactorySessionID)
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

func (recorder *runtimeScopeRecorder) RecordEventWithProvenance(
	event recordings.FactoryEvent,
	provenance []recordings.RecordingSecret,
) {
	if recorder == nil || recorder.inner == nil {
		return
	}
	if withProvenance, ok := recorder.inner.(recordings.RuntimeRecorderWithProvenance); ok {
		withProvenance.RecordEventWithProvenance(event, provenance)
		return
	}
	recorder.inner.RecordEvent(event)
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
