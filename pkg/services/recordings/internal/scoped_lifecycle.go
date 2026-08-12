package internal

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/recordings/internal/canonical"
)

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
) (recordings.BeginRecordingScopeResult, error) {
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
		_, finishErr := service.FinishRecording(recordings.FinishRecordingRequest{
			RecordingID: binding.recordingID,
			FinishedAt:  time.Now().UTC(),
		})
		return recordings.BeginRecordingScopeResult{}, errors.Join(err, finishErr)
	}
	status, err := service.scopeStatus(ref, binding)
	if err != nil {
		return recordings.BeginRecordingScopeResult{}, err
	}
	return recordings.BeginRecordingScopeResult{Scope: ref, Status: status}, nil
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
) (recordings.AppendRecordingScopeEventResult, error) {
	if err := recordingScopeContext(ctx).Err(); err != nil {
		return recordings.AppendRecordingScopeEventResult{}, err
	}
	binding, err := service.recordingScope(request.Scope)
	if err != nil {
		return recordings.AppendRecordingScopeEventResult{}, err
	}
	binding.mu.Lock()
	defer binding.mu.Unlock()
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
	current, err := service.QueryRecordingStatus(recordings.RecordingStatusRequest{
		RecordingID: binding.recordingID,
	})
	if err != nil {
		return recordings.AppendRecordingScopeEventResult{}, err
	}
	if !validRecordingScopeEvent(current.Status, binding.eventScope, request.Event) {
		return recordings.AppendRecordingScopeEventResult{}, recordings.ErrInvalidRecordingEvent
	}
	accepted, err := service.canonicalLedger.Append(recordings.AppendRecordedEventRequest{
		Event: request.Event,
	})
	if err != nil {
		return recordings.AppendRecordingScopeEventResult{}, err
	}
	event := accepted.Event
	if event.ID == "" {
		return recordings.AppendRecordingScopeEventResult{}, recordings.ErrInvalidAppendEvent
	}
	if !validRecordingScopeEvent(current.Status, binding.eventScope, event) {
		return recordings.AppendRecordingScopeEventResult{}, recordings.ErrInvalidRecordingEvent
	}
	result, err := service.RecordRecordingEvent(recordings.RecordRecordingEventRequest{
		RecordingID: binding.recordingID,
		Event:       event,
	})
	if err != nil {
		return recordings.AppendRecordingScopeEventResult{}, err
	}
	status := scopeStatusFrom(request.Scope, result.Status)
	return recordings.AppendRecordingScopeEventResult{Event: event, Status: status}, nil
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
) (recordings.FlushRecordingScopeResult, error) {
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
	result, err := service.FlushRecording(recordings.FlushRecordingRequest{
		RecordingID: binding.recordingID,
	})
	if err != nil {
		return recordings.FlushRecordingScopeResult{}, err
	}
	return recordings.FlushRecordingScopeResult{
		Status: scopeStatusFrom(request.Scope, result.Status),
	}, nil
}

func (service *combinedService) FinalizeRecordingScope(
	ctx context.Context,
	request recordings.FinalizeRecordingScopeRequest,
) (recordings.FinalizeRecordingScopeResult, error) {
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
) (recordings.CloseRecordingScopeResult, error) {
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
) (recordings.QueryRecordingScopeResult, error) {
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
