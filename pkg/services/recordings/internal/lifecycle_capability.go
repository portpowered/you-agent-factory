package internal

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/recordings/internal/canonical"
	recordingevents "github.com/portpowered/infinite-you/pkg/services/recordings/internal/events"
)

var _ recordings.RecordingLifecycle = (*combinedService)(nil)

func (service *combinedService) openRuntimeLedger(
	request recordings.RuntimeScopeRequest,
	streamGenerationID string,
) (recordings.RuntimeEventLedger, string, error) {
	ledger := NewRuntimeLedger(
		request.Topology,
		request.Now,
		streamGenerationID,
		request.Definitions,
	)
	if ledger == nil {
		return nil, "", fmt.Errorf("Recordings runtime ledger is unavailable")
	}
	if len(request.ReplayEvents) > 0 {
		seeder, ok := ledger.(interface {
			SeedCanonicalEvents([]factorydefinitions.FactoryEvent) error
		})
		if !ok {
			return nil, "", fmt.Errorf("Recordings runtime ledger does not support restored event history")
		}
		if err := seeder.SeedCanonicalEvents(request.ReplayEvents); err != nil {
			return nil, "", fmt.Errorf("seed restored Factory Event history: %w", err)
		}
	}
	if binder, ok := ledger.(interface {
		SetCompletedFlushWatermarkReader(recordings.CompletedFlushWatermarkReader)
	}); ok {
		binder.SetCompletedFlushWatermarkReader(service)
	}
	routeKey := strings.TrimSpace(request.FactorySessionID)
	if routeKey == "" {
		routeKey = ledger.StreamGenerationID()
	}
	if err := service.runtimeRouter.register(routeKey, ledger); err != nil {
		return nil, "", err
	}
	return ledger, routeKey, nil
}

// LoadResumeInput keeps explicit resume classification on the Recordings root
// while retaining the narrow pre-ledger compatibility seam used by the
// Factory Sessions opener. Resume is a separate operation even when the
// current path-backed adapter can reuse the same detached input parser.
func (service *combinedService) LoadResumeInput(
	request recordings.LoadResumeInputRequest,
) (recordings.LoadResumeInputResult, error) {
	input, err := service.LoadReplayInput(recordings.LoadReplayInputRequest{Path: request.Path})
	if err != nil {
		return recordings.LoadResumeInputResult{}, err
	}
	// V2 keeps its canonical source identity in the framing header. The full
	// replay input intentionally normalizes that header away, so ask the same
	// Recordings loader for detached metadata when the full result did not
	// retain it. Metadata failure is non-fatal here: legacy alias-only inputs
	// remain resumable, but they cannot contribute a source metrics identity.
	if input.Metadata == nil {
		if metadataResult, metadataErr := service.LoadReplayInput(recordings.LoadReplayInputRequest{
			Path: request.Path, MetadataOnly: true,
		}); metadataErr == nil && metadataResult.Metadata != nil {
			input.Metadata = metadataResult.Metadata
		}
	}
	sourceID, err := resumeSourceCanonicalSessionIDForPath(input, request.Path)
	if err != nil {
		return recordings.LoadResumeInputResult{}, fmt.Errorf("resolve resume source identity: %w", err)
	}
	return recordings.LoadResumeInputResult{
		Input:                    input,
		SourceCanonicalSessionID: sourceID,
	}, nil
}

// Begin implements recordings.RecordingLifecycle by adapting the existing
// StartRecording lifecycle operation to the root-owned lifecycle vocabulary.
func (service *combinedService) Begin(
	request recordings.BeginRecordingRequest,
) (recordings.RecordingLifecycleResult, error) {
	result, err := service.StartRecording(recordings.StartRecordingRequest{
		Enabled:     request.Enabled,
		RecordingID: recordings.RecordingID(request.RecordingID),
		Scope:       fromLifecycleScope(request.Scope),
		Target: recordings.RecordingTargetRequest{
			Artifact:           recordings.RecordingArtifactReference(request.Artifact),
			HomeDir:            request.HomeDir,
			CanonicalSessionID: request.CanonicalSessionID,
			ReportedSessionID:  request.ReportedSessionID,
		},
		FlushInterval: request.FlushInterval,
	})
	if err != nil {
		return recordings.RecordingLifecycleResult{}, translateLifecycleError(err)
	}
	if !result.Enabled {
		return recordings.RecordingLifecycleResult{}, nil
	}
	return recordings.RecordingLifecycleResult{Status: toLifecycleStatus(result.Status)}, nil
}

// Bind implements recordings.RecordingLifecycle by adapting the existing
// BindRecording lifecycle operation to the root-owned lifecycle vocabulary.
func (service *combinedService) Bind(
	request recordings.BindLifecycleRequest,
) (recordings.RecordingLifecycleResult, error) {
	result, err := service.BindRecording(recordings.BindRecordingRequest{
		RecordingID: recordings.RecordingID(request.RecordingID),
		Artifact:    recordings.RecordingArtifactReference(request.Artifact),
		Scope:       fromLifecycleScope(request.Scope),
	})
	if err != nil {
		return recordings.RecordingLifecycleResult{}, translateLifecycleError(err)
	}
	return recordings.RecordingLifecycleResult{Status: toLifecycleStatus(result.Status)}, nil
}

// AppendEvent implements recordings.RecordingLifecycle by adapting the
// existing RecordRecordingEvent operation to the root-owned lifecycle
// vocabulary.
func (service *combinedService) AppendEvent(
	request recordings.AppendLifecycleEventRequest,
) (recordings.RecordingLifecycleResult, error) {
	event := request.Event
	result, err := service.RecordRecordingEvent(recordings.RecordRecordingEventRequest{
		RecordingID:      recordings.RecordingID(request.RecordingID),
		SecretProvenance: append([]recordings.RecordingSecret(nil), request.SecretProvenance...),
		Event: recordings.CanonicalEvent{
			ID:          recordings.CanonicalEventID(event.ID),
			Sequence:    recordings.CanonicalEventSequence(event.Sequence),
			FactoryTick: event.FactoryTick,
			Scope:       fromLifecycleScope(event.Scope),
			Cursor: recordings.CanonicalEventCursor{
				StreamGenerationID: event.Cursor.StreamGenerationID,
				Sequence:           recordings.CanonicalEventSequence(event.Cursor.Sequence),
			},
			RecordedAt:    event.RecordedAt,
			Kind:          recordings.CanonicalEventKind(event.Kind),
			Payload:       event.Payload,
			SourceContext: event.SourceContext,
		},
	})
	if err != nil {
		return recordings.RecordingLifecycleResult{}, translateLifecycleError(err)
	}
	return recordings.RecordingLifecycleResult{Status: toLifecycleStatus(result.Status)}, nil
}

// RecordFailure implements recordings.RecordingLifecycle by adapting the
// existing RecordRecordingError operation to the root-owned lifecycle
// vocabulary.
func (service *combinedService) RecordFailure(
	request recordings.RecordLifecycleFailureRequest,
) (recordings.RecordingLifecycleResult, error) {
	result, err := service.RecordRecordingError(recordings.RecordRecordingErrorRequest{
		RecordingID: recordings.RecordingID(request.RecordingID),
		Failure: recordings.RecordingFailure{
			Code:       request.Failure.Code,
			Message:    request.Failure.Message,
			RecordedAt: request.Failure.RecordedAt,
		},
		Cause: request.Cause,
	})
	if err != nil {
		return recordings.RecordingLifecycleResult{}, translateLifecycleError(err)
	}
	return recordings.RecordingLifecycleResult{Status: toLifecycleStatus(result.Status)}, nil
}

// Flush implements recordings.RecordingLifecycle by adapting the existing
// FlushRecording operation to the root-owned lifecycle vocabulary.
func (service *combinedService) Flush(
	request recordings.FlushLifecycleRequest,
) (recordings.RecordingLifecycleResult, error) {
	result, err := service.FlushRecording(recordings.FlushRecordingRequest{
		RecordingID: recordings.RecordingID(request.RecordingID),
	})
	if err != nil {
		return recordings.RecordingLifecycleResult{}, translateLifecycleError(err)
	}
	return recordings.RecordingLifecycleResult{Status: toLifecycleStatus(result.Status)}, nil
}

// Stop implements recordings.RecordingLifecycle by adapting the existing
// StopRecording operation to the root-owned lifecycle vocabulary.
func (service *combinedService) Stop(request recordings.StopLifecycleRequest) error {
	_, err := service.StopRecording(recordings.StopRecordingRequest{
		RecordingID: recordings.RecordingID(request.RecordingID),
	})
	return translateLifecycleError(err)
}

// Finish implements recordings.RecordingLifecycle by adapting the existing
// FinishRecording operation to the root-owned lifecycle vocabulary. The
// detached terminal status is returned alongside a translated error so
// callers can observe finalized-with-failures status.
func (service *combinedService) Finish(
	request recordings.FinishLifecycleRequest,
) (recordings.RecordingLifecycleResult, error) {
	result, err := service.FinishRecording(recordings.FinishRecordingRequest{
		RecordingID: recordings.RecordingID(request.RecordingID),
		FinishedAt:  request.FinishedAt,
	})
	return recordings.RecordingLifecycleResult{Status: toLifecycleStatus(result.Status)}, translateLifecycleError(err)
}

// Status implements recordings.RecordingLifecycle by adapting the existing
// QueryRecordingStatus operation to the root-owned lifecycle vocabulary.
func (service *combinedService) Status(
	request recordings.LifecycleStatusRequest,
) (recordings.RecordingLifecycleResult, error) {
	result, err := service.QueryRecordingStatus(recordings.RecordingStatusRequest{
		RecordingID: recordings.RecordingID(request.RecordingID),
	})
	if err != nil {
		return recordings.RecordingLifecycleResult{}, translateLifecycleError(err)
	}
	return recordings.RecordingLifecycleResult{Status: toLifecycleStatus(result.Status)}, nil
}

func fromLifecycleScope(scope recordings.LifecycleScope) recordings.CanonicalEventScope {
	return recordings.CanonicalEventScope{FactorySessionID: scope.FactorySessionID}
}

func toLifecycleCursor(cursor *recordings.CanonicalEventCursor) *recordings.LifecycleEventCursor {
	if cursor == nil {
		return nil
	}
	detached := recordings.LifecycleEventCursor{
		StreamGenerationID: cursor.StreamGenerationID,
		Sequence:           int64(cursor.Sequence),
	}
	return &detached
}

func toLifecycleFailures(failures []recordings.RecordingFailure) []recordings.LifecycleFailure {
	if len(failures) == 0 {
		return nil
	}
	detached := make([]recordings.LifecycleFailure, len(failures))
	for index, failure := range failures {
		detached[index] = recordings.LifecycleFailure{
			Code:       failure.Code,
			Message:    failure.Message,
			RecordedAt: failure.RecordedAt,
		}
	}
	return detached
}

func toLifecycleStatus(status recordings.RecordingStatusFacts) recordings.LifecycleStatus {
	return recordings.LifecycleStatus{
		RecordingID:    recordings.LifecycleRecordingID(status.RecordingID),
		Artifact:       recordings.LifecycleArtifactReference(status.Artifact),
		Scope:          recordings.LifecycleScope{FactorySessionID: status.Scope.FactorySessionID},
		State:          recordings.LifecycleState(status.State),
		AcceptedEvents: status.AcceptedEvents,
		LastEvent:      toLifecycleCursor(status.LastEvent),
		FlushedThrough: toLifecycleCursor(status.FlushedThrough),
		Failures:       toLifecycleFailures(status.Failures),
		FinalizedAt:    status.FinalizedAt,
	}
}

func translateLifecycleError(err error) error {
	if err == nil {
		return nil
	}
	kind := recordings.LifecycleErrorWriteFailed
	switch {
	case errors.Is(err, recordings.ErrMissingRecordingTarget):
		kind = recordings.LifecycleErrorInvalidTarget
	case errors.Is(err, recordings.ErrInvalidRecordingScope):
		kind = recordings.LifecycleErrorInvalidScope
	case errors.Is(err, recordings.ErrRecordingBindingConflict):
		kind = recordings.LifecycleErrorBindingConflict
	case errors.Is(err, recordings.ErrInvalidRecordingEvent):
		kind = recordings.LifecycleErrorInvalidEvent
	case errors.Is(err, recordings.ErrInvalidRecordingFailure):
		kind = recordings.LifecycleErrorInvalidFailure
	case errors.Is(err, recordings.ErrRecordingWriteRejected):
		kind = recordings.LifecycleErrorTerminal
	case errors.Is(err, recordings.ErrInvalidRecordingTerminalMetadata):
		kind = recordings.LifecycleErrorInvalidTerminalMetadata
	}
	return &recordings.LifecycleError{Kind: kind, Message: err.Error(), Cause: err}
}

type recordingScopeBinding struct {
	recordingID recordings.RecordingID
	eventScope  recordings.CanonicalEventScope
	historical  bool
	replayPlans map[recordings.ReplayPlanHandle]struct{}

	mu          sync.Mutex
	finalized   bool
	closed      bool
	terminal    recordings.RecordingScopeStatus
	finalizeErr error
}

var _ recordings.RecordingScopeService = (*combinedService)(nil)

func recordingScopeIssuer(service *combinedService) string {
	digest := sha256.Sum256([]byte(fmt.Sprintf("recordings-root:%p", service)))
	return hex.EncodeToString(digest[:16])
}

func (service *combinedService) newRecordingScope() recordings.RecordingScopeRef {
	service.scopeMu.Lock()
	defer service.scopeMu.Unlock()
	service.nextScopeID++
	identity := fmt.Sprintf("%s:%d", service.scopeIssuer, service.nextScopeID)
	digest := sha256.Sum256([]byte("recording-scope:" + identity))
	ref, _ := (recordings.RecordingScopeRef{}).Parse(
		service.scopeIssuer + "." + hex.EncodeToString(digest[:16]),
	)
	return ref
}

func (service *combinedService) recordingScope(
	ref recordings.RecordingScopeRef,
) (*recordingScopeBinding, error) {
	if ref.IsZero() {
		return nil, recordings.ErrRecordingScopeInvalid
	}
	issuer, token, ok := strings.Cut(ref.String(), ".")
	if !ok || !validRecordingScopeToken(issuer) || !validRecordingScopeToken(token) {
		return nil, recordings.ErrRecordingScopeInvalid
	}
	if issuer != service.scopeIssuer {
		return nil, recordings.ErrRecordingScopeForeign
	}
	service.scopeMu.RLock()
	binding, ok := service.scopeByRef[ref]
	service.scopeMu.RUnlock()
	if !ok {
		return nil, recordings.ErrRecordingScopeStale
	}
	return binding, nil
}

func validRecordingScopeToken(value string) bool {
	if len(value) != 32 || value == strings.Repeat("0", 32) {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func (service *combinedService) BeginRecordingScope(
	ctx context.Context,
	request recordings.BeginRecordingScopeRequest,
) (result recordings.BeginRecordingScopeResult, err error) {
	operation := service.startOperationLog("recording_scope.begin", recordings.RecordingScopeRef{}, request.Scope)
	defer func() { operation.finish(err) }()
	if err := recordingScopeContext(ctx).Err(); err != nil {
		return recordings.BeginRecordingScopeResult{}, err
	}
	if !request.Enabled {
		return recordings.BeginRecordingScopeResult{}, nil
	}
	ref := service.newRecordingScope()
	started, err := service.StartRecording(recordings.StartRecordingRequest{
		Enabled:       true,
		RecordingID:   scopeRecordingID(ref, request.RecordingID),
		Scope:         request.Scope,
		Target:        request.Target,
		FlushInterval: request.FlushInterval,
	})
	if err != nil {
		return recordings.BeginRecordingScopeResult{}, err
	}
	binding := &recordingScopeBinding{
		recordingID: started.Status.RecordingID,
		eventScope:  started.Status.Scope,
		replayPlans: make(map[recordings.ReplayPlanHandle]struct{}),
	}
	service.scopeMu.Lock()
	service.scopeByRef[ref] = binding
	service.scopeMu.Unlock()
	if err := recordingScopeContext(ctx).Err(); err != nil {
		return recordings.BeginRecordingScopeResult{}, service.abandonRecordingScope(ref, binding, err)
	}
	status, err := service.scopeStatus(ref, binding)
	if err != nil {
		return recordings.BeginRecordingScopeResult{}, service.abandonRecordingScope(ref, binding, err)
	}
	return recordings.BeginRecordingScopeResult{Scope: ref, Status: status}, nil
}

func (service *combinedService) abandonRecordingScope(
	ref recordings.RecordingScopeRef,
	binding *recordingScopeBinding,
	cause error,
) error {
	service.scopeMu.Lock()
	if current, ok := service.scopeByRef[ref]; ok && current == binding {
		delete(service.scopeByRef, ref)
	}
	service.scopeMu.Unlock()
	_, finishErr := service.FinishRecording(recordings.FinishRecordingRequest{
		RecordingID: binding.recordingID,
		FinishedAt:  service.recordingFinishedAt(),
	})
	return errors.Join(cause, finishErr)
}

func scopeRecordingID(
	ref recordings.RecordingScopeRef,
	requested recordings.RecordingID,
) recordings.RecordingID {
	digest := sha256.Sum256([]byte("recording-id:" + string(requested) + ":" + ref.String()))
	return recordings.RecordingID("scope-" + hex.EncodeToString(digest[:16]))
}

func (service *combinedService) AppendRecordingScopeEvent(
	ctx context.Context,
	request recordings.AppendRecordingScopeEventRequest,
) (result recordings.AppendRecordingScopeEventResult, err error) {
	// Event append is a high-volume path; lifecycle and query operations emit
	// per-operation logs while append remains observable through its canonical
	// event and terminal status projections.
	if err := recordingScopeContext(ctx).Err(); err != nil {
		return recordings.AppendRecordingScopeEventResult{}, err
	}
	binding, err := service.recordingScope(request.Scope)
	if err != nil {
		return recordings.AppendRecordingScopeEventResult{}, err
	}
	binding.mu.Lock()
	defer binding.mu.Unlock()
	service.recordingMu.Lock()
	defer service.recordingMu.Unlock()
	if err := recordingScopeContext(ctx).Err(); err != nil {
		return recordings.AppendRecordingScopeEventResult{}, err
	}
	if binding.closed {
		return recordings.AppendRecordingScopeEventResult{}, recordings.ErrRecordingScopeClosed
	}
	if binding.finalized {
		return recordings.AppendRecordingScopeEventResult{}, errors.Join(
			recordings.ErrRecordingScopeFinalized,
			recordings.ErrRecordingWriteRejected,
		)
	}
	current, err := service.Service.QueryRecordingStatus(recordings.RecordingStatusRequest{
		RecordingID: binding.recordingID,
	})
	if err != nil {
		return recordings.AppendRecordingScopeEventResult{}, err
	}
	if !validRecordingScopeEvent(current.Status, binding.eventScope, request.Event) {
		return recordings.AppendRecordingScopeEventResult{}, recordings.ErrInvalidRecordingEvent
	}
	var lifecycleResult recordings.RecordRecordingEventResult
	accepted, err := service.canonicalLedger.AppendWithValidation(
		recordings.AppendRecordedEventRequest{Event: request.Event},
		func(event recordings.CanonicalEvent) error {
			if event.ID == "" {
				return recordings.ErrInvalidAppendEvent
			}
			if !validRecordingScopeEvent(current.Status, binding.eventScope, event) {
				return recordings.ErrInvalidRecordingEvent
			}
			var recordErr error
			lifecycleResult, recordErr = service.Service.RecordRecordingEvent(recordings.RecordRecordingEventRequest{
				RecordingID: binding.recordingID,
				Event:       event,
			})
			return recordErr
		},
	)
	if err != nil {
		return recordings.AppendRecordingScopeEventResult{}, err
	}
	event := accepted.Event
	status := scopeStatusFrom(request.Scope, lifecycleResult.Status)
	return recordings.AppendRecordingScopeEventResult{Event: event, Status: status}, nil
}

func (service *combinedService) StartRecording(
	request recordings.StartRecordingRequest,
) (recordings.StartRecordingResult, error) {
	service.recordingMu.Lock()
	defer service.recordingMu.Unlock()
	return service.Service.StartRecording(request)
}

func (service *combinedService) RecordRecordingEvent(
	request recordings.RecordRecordingEventRequest,
) (recordings.RecordRecordingEventResult, error) {
	service.recordingMu.Lock()
	defer service.recordingMu.Unlock()
	return service.Service.RecordRecordingEvent(request)
}

func (service *combinedService) FinishRecording(
	request recordings.FinishRecordingRequest,
) (recordings.FinishRecordingResult, error) {
	service.recordingMu.Lock()
	defer service.recordingMu.Unlock()
	return service.Service.FinishRecording(request)
}

func validRecordingScopeEvent(
	status recordings.RecordingStatusFacts,
	expectedScope recordings.CanonicalEventScope,
	event recordings.CanonicalEvent,
) bool {
	if !canonical.ValidAppendEvent(event) || event.Scope != expectedScope ||
		event.Sequence < 0 ||
		event.Cursor.StreamGenerationID == "" || event.Cursor.Sequence != event.Sequence {
		return false
	}
	if status.LastEvent == nil {
		return true
	}
	return event.Cursor.StreamGenerationID == status.LastEvent.StreamGenerationID &&
		event.Sequence > status.LastEvent.Sequence
}

func (service *combinedService) FlushRecordingScope(
	ctx context.Context,
	request recordings.FlushRecordingScopeRequest,
) (result recordings.FlushRecordingScopeResult, err error) {
	operation := service.startOperationLog("recording_scope.flush", request.Scope, recordings.CanonicalEventScope{})
	defer func() { operation.finish(err) }()
	if err := recordingScopeContext(ctx).Err(); err != nil {
		return recordings.FlushRecordingScopeResult{}, err
	}
	binding, err := service.recordingScope(request.Scope)
	if err != nil {
		return recordings.FlushRecordingScopeResult{}, err
	}
	binding.mu.Lock()
	defer binding.mu.Unlock()
	if binding.closed {
		return recordings.FlushRecordingScopeResult{}, recordings.ErrRecordingScopeClosed
	}
	flushed, err := service.FlushRecording(recordings.FlushRecordingRequest{
		RecordingID: binding.recordingID,
	})
	if err != nil {
		return recordings.FlushRecordingScopeResult{}, err
	}
	return recordings.FlushRecordingScopeResult{
		Status: scopeStatusFrom(request.Scope, flushed.Status),
	}, nil
}

func (service *combinedService) FinalizeRecordingScope(
	ctx context.Context,
	request recordings.FinalizeRecordingScopeRequest,
) (result recordings.FinalizeRecordingScopeResult, err error) {
	operation := service.startOperationLog("recording_scope.finalize", request.Scope, recordings.CanonicalEventScope{})
	defer func() { operation.finish(err) }()
	binding, err := service.recordingScope(request.Scope)
	if err != nil {
		return recordings.FinalizeRecordingScopeResult{}, err
	}
	binding.mu.Lock()
	defer binding.mu.Unlock()
	if binding.closed {
		return recordings.FinalizeRecordingScopeResult{}, recordings.ErrRecordingScopeClosed
	}
	status, finalizeErr := service.finalizeRecordingScopeLocked(
		ctx,
		request.Scope,
		binding,
		request.FinishedAt,
	)
	return recordings.FinalizeRecordingScopeResult{Status: status}, finalizeErr
}

func (service *combinedService) finalizeRecordingScopeLocked(
	ctx context.Context,
	ref recordings.RecordingScopeRef,
	binding *recordingScopeBinding,
	finishedAt time.Time,
) (recordings.RecordingScopeStatus, error) {
	if binding.finalized {
		return binding.terminal, binding.finalizeErr
	}
	if err := recordingScopeContext(ctx).Err(); err != nil {
		return recordings.RecordingScopeStatus{}, err
	}
	_, err := service.FinishRecording(recordings.FinishRecordingRequest{
		RecordingID: binding.recordingID,
		FinishedAt:  finishedAt,
	})
	binding.finalized = true
	binding.finalizeErr = err
	status, statusErr := service.scopeStatus(ref, binding)
	if statusErr != nil {
		return recordings.RecordingScopeStatus{}, errors.Join(err, statusErr)
	}
	binding.terminal = status
	return status, errors.Join(err, statusErr)
}

func (service *combinedService) CloseRecordingScope(
	ctx context.Context,
	request recordings.CloseRecordingScopeRequest,
) (result recordings.CloseRecordingScopeResult, err error) {
	operation := service.startOperationLog("recording_scope.close", request.Scope, recordings.CanonicalEventScope{})
	defer func() { operation.finish(err) }()
	binding, err := service.recordingScope(request.Scope)
	if err != nil {
		return recordings.CloseRecordingScopeResult{}, err
	}
	binding.mu.Lock()
	defer binding.mu.Unlock()
	if binding.closed {
		return recordings.CloseRecordingScopeResult{
			Scope: request.Scope, Closed: true, Status: binding.terminal,
		}, nil
	}
	status, finalizeErr := service.finalizeRecordingScopeLocked(
		ctx,
		request.Scope,
		binding,
		request.FinishedAt,
	)
	if !binding.finalized {
		return recordings.CloseRecordingScopeResult{Status: status}, finalizeErr
	}
	binding.closed = true
	return recordings.CloseRecordingScopeResult{
		Scope: request.Scope, Closed: true, Status: status,
	}, finalizeErr
}

func (service *combinedService) QueryRecordingScope(
	ctx context.Context,
	request recordings.QueryRecordingScopeRequest,
) (result recordings.QueryRecordingScopeResult, err error) {
	operation := service.startOperationLog("recording_scope.status", request.Scope, recordings.CanonicalEventScope{})
	defer func() { operation.finish(err) }()
	if err := recordingScopeContext(ctx).Err(); err != nil {
		return recordings.QueryRecordingScopeResult{}, err
	}
	binding, err := service.recordingScope(request.Scope)
	if err != nil {
		return recordings.QueryRecordingScopeResult{}, err
	}
	binding.mu.Lock()
	defer binding.mu.Unlock()
	if binding.closed {
		return recordings.QueryRecordingScopeResult{}, recordings.ErrRecordingScopeClosed
	}
	if binding.finalized {
		return recordings.QueryRecordingScopeResult{Status: binding.terminal}, nil
	}
	status, err := service.scopeStatus(request.Scope, binding)
	if err != nil {
		return recordings.QueryRecordingScopeResult{}, err
	}
	return recordings.QueryRecordingScopeResult{Status: status}, nil
}

func recordingScopeContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

func (service *combinedService) scopeStatus(
	ref recordings.RecordingScopeRef,
	binding *recordingScopeBinding,
) (recordings.RecordingScopeStatus, error) {
	result, err := service.QueryRecordingStatus(recordings.RecordingStatusRequest{
		RecordingID: binding.recordingID,
	})
	if err != nil {
		return recordings.RecordingScopeStatus{}, err
	}
	return scopeStatusFrom(ref, result.Status), nil
}

func scopeStatusFrom(
	ref recordings.RecordingScopeRef,
	status recordings.RecordingStatusFacts,
) recordings.RecordingScopeStatus {
	result := recordings.RecordingScopeStatus{
		Scope:          ref,
		EventScope:     status.Scope,
		Artifact:       status.Artifact,
		State:          status.State,
		AcceptedEvents: status.AcceptedEvents,
		Failures:       append([]recordings.RecordingFailure(nil), status.Failures...),
	}
	if status.LastEvent != nil {
		cursor := *status.LastEvent
		result.LastEvent = &cursor
	}
	if status.FlushedThrough != nil {
		cursor := *status.FlushedThrough
		result.FlushedThrough = &cursor
	}
	if status.FinalizedAt != nil {
		finishedAt := *status.FinalizedAt
		result.FinalizedAt = &finishedAt
	}
	return result
}

// runtimeLedgerRouter keeps the public Recordings root stable while routing
// session-scoped event operations to the private runtime ledgers it owns.
// The fallback ledger preserves the root contract for callers that append
// factory-wide events before a runtime scope has been opened.
type runtimeLedgerRouter struct {
	mu            sync.RWMutex
	fallback      recordings.Ledger
	routes        map[string]recordings.RuntimeEventLedger
	recorders     []func(factorydefinitions.FactoryEvent)
	typeRecorders []func(factorydefinitions.FactoryEventType)
}

func newRuntimeLedgerRouter(now func() time.Time) *runtimeLedgerRouter {
	fallback := recordingevents.NewRuntimeLedger(nil, now, "recordings-root", nil)
	return &runtimeLedgerRouter{
		fallback: fallback,
		routes:   make(map[string]recordings.RuntimeEventLedger),
	}
}

func (router *runtimeLedgerRouter) register(
	scope string,
	ledger recordings.RuntimeEventLedger,
) error {
	if router == nil || ledger == nil {
		return recordings.ErrInvalidRecordingScope
	}
	scope = strings.TrimSpace(scope)
	if scope == "" {
		scope = ledger.StreamGenerationID()
	}
	router.mu.Lock()
	router.routes[scope] = ledger
	recorders := append([]func(factorydefinitions.FactoryEvent){}, router.recorders...)
	typeRecorders := append([]func(factorydefinitions.FactoryEventType){}, router.typeRecorders...)
	router.mu.Unlock()
	for _, recorder := range recorders {
		ledger.AddEventRecorder(recorder)
	}
	for _, recorder := range typeRecorders {
		ledger.AddEventTypeRecorder(recorder)
	}
	return nil
}

func (router *runtimeLedgerRouter) unregister(
	scope string,
	ledger recordings.RuntimeEventLedger,
) {
	if router == nil || ledger == nil {
		return
	}
	scope = strings.TrimSpace(scope)
	router.mu.Lock()
	current, ok := router.routes[scope]
	if ok && current != nil && current.StreamGenerationID() == ledger.StreamGenerationID() {
		delete(router.routes, scope)
	}
	router.mu.Unlock()
}

func (router *runtimeLedgerRouter) route(scope string) recordings.Ledger {
	if router == nil {
		return nil
	}
	scope = strings.TrimSpace(scope)
	router.mu.RLock()
	defer router.mu.RUnlock()
	if scope != "" {
		return router.routes[scope]
	}
	if len(router.routes) == 1 {
		for _, ledger := range router.routes {
			return ledger
		}
	}
	return router.fallback
}

func (router *runtimeLedgerRouter) CanonicalEvents() []factorydefinitions.FactoryEvent {
	if router == nil {
		return nil
	}
	router.mu.RLock()
	ledgers := make([]recordings.RuntimeEventLedger, 0, len(router.routes))
	for _, ledger := range router.routes {
		ledgers = append(ledgers, ledger)
	}
	fallback := router.fallback
	router.mu.RUnlock()
	if len(ledgers) == 0 {
		if fallback == nil {
			return nil
		}
		return fallback.CanonicalEvents()
	}
	events := make([]factorydefinitions.FactoryEvent, 0)
	for _, ledger := range ledgers {
		events = append(events, ledger.CanonicalEvents()...)
	}
	sort.SliceStable(events, func(left, right int) bool {
		return events[left].Context.EventTime.Before(events[right].Context.EventTime)
	})
	return events
}

func (router *runtimeLedgerRouter) Subscribe(
	ctx context.Context,
	reconnect *factorydefinitions.FactoryEventReconnectCursor,
	scope factorydefinitions.FactoryEventReconnectScope,
) (factorydefinitions.FactoryEventStream, error) {
	ledger := router.route(scope.SessionID)
	if ledger == nil {
		return factorydefinitions.FactoryEventStream{}, recordings.ErrReconnectCursorUnavailable
	}
	return ledger.Subscribe(ctx, reconnect, scope)
}

func (router *runtimeLedgerRouter) StreamGenerationID() string {
	if router == nil {
		return ""
	}
	router.mu.RLock()
	defer router.mu.RUnlock()
	if len(router.routes) == 1 {
		for _, ledger := range router.routes {
			return ledger.StreamGenerationID()
		}
	}
	if router.fallback == nil {
		return ""
	}
	return router.fallback.StreamGenerationID()
}

func (router *runtimeLedgerRouter) AddEventRecorder(
	recorder func(factorydefinitions.FactoryEvent),
) {
	if router == nil || recorder == nil {
		return
	}
	router.mu.Lock()
	router.recorders = append(router.recorders, recorder)
	ledgers := make([]recordings.RuntimeEventLedger, 0, len(router.routes))
	for _, ledger := range router.routes {
		ledgers = append(ledgers, ledger)
	}
	fallback := router.fallback
	router.mu.Unlock()
	if fallback != nil {
		fallback.AddEventRecorder(recorder)
	}
	for _, ledger := range ledgers {
		ledger.AddEventRecorder(recorder)
	}
}

func (router *runtimeLedgerRouter) AddEventTypeRecorder(
	recorder func(factorydefinitions.FactoryEventType),
) {
	if router == nil || recorder == nil {
		return
	}
	router.mu.Lock()
	router.typeRecorders = append(router.typeRecorders, recorder)
	ledgers := make([]recordings.RuntimeEventLedger, 0, len(router.routes))
	for _, ledger := range router.routes {
		ledgers = append(ledgers, ledger)
	}
	fallback := router.fallback
	router.mu.Unlock()
	if fallback != nil {
		fallback.AddEventTypeRecorder(recorder)
	}
	for _, ledger := range ledgers {
		ledger.AddEventTypeRecorder(recorder)
	}
}

func (router *runtimeLedgerRouter) AppendRecordedEvent(
	event factorydefinitions.FactoryEvent,
) {
	if router == nil {
		return
	}
	scope := ""
	if event.Context.SessionID != nil {
		scope = strings.TrimSpace(*event.Context.SessionID)
	}
	ledger := router.route(scope)
	if ledger != nil {
		ledger.AppendRecordedEvent(event)
	}
}

func (router *runtimeLedgerRouter) SecretProvenanceForEvent(
	event factorydefinitions.FactoryEvent,
) []recordings.RecordingSecret {
	if router == nil {
		return nil
	}
	scope := ""
	if event.Context.SessionID != nil {
		scope = strings.TrimSpace(*event.Context.SessionID)
	}
	ledger := router.route(scope)
	lookup, ok := ledger.(interface {
		SecretProvenanceForEvent(factorydefinitions.FactoryEvent) []recordings.RecordingSecret
	})
	if !ok {
		return nil
	}
	return lookup.SecretProvenanceForEvent(event)
}

func (router *runtimeLedgerRouter) SecretProvenanceDuringAppend(
	event factorydefinitions.FactoryEvent,
) []recordings.RecordingSecret {
	if router == nil {
		return nil
	}
	scope := ""
	if event.Context.SessionID != nil {
		scope = strings.TrimSpace(*event.Context.SessionID)
	}
	ledger := router.route(scope)
	lookup, ok := ledger.(interface {
		SecretProvenanceDuringAppend(factorydefinitions.FactoryEvent) []recordings.RecordingSecret
	})
	if !ok {
		return nil
	}
	return lookup.SecretProvenanceDuringAppend(event)
}

func (router *runtimeLedgerRouter) AppendRecordedEventWithValidation(
	event factorydefinitions.FactoryEvent,
	validate func(factorydefinitions.FactoryEvent) error,
) (factorydefinitions.FactoryEvent, error) {
	if router == nil {
		return factorydefinitions.FactoryEvent{}, fmt.Errorf("recordings ledger router is unavailable")
	}
	scope := ""
	if event.Context.SessionID != nil {
		scope = strings.TrimSpace(*event.Context.SessionID)
	}
	ledger := router.route(scope)
	appender, ok := ledger.(interface {
		AppendRecordedEventWithValidation(
			factorydefinitions.FactoryEvent,
			func(factorydefinitions.FactoryEvent) error,
		) (factorydefinitions.FactoryEvent, error)
	})
	if !ok {
		return factorydefinitions.FactoryEvent{}, fmt.Errorf("recordings ledger does not support atomic append")
	}
	return appender.AppendRecordedEventWithValidation(event, validate)
}

var _ recordings.Ledger = (*runtimeLedgerRouter)(nil)
