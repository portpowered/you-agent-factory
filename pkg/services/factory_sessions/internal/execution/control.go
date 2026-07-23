package factorysessionexecution

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// NormalizeControlRequest validates optional metadata for pause/resume/cancel/terminate.
func NormalizeControlRequest(req ControlRequest) (ControlRequest, error) {
	return ControlRequest{
		RequestID: strings.TrimSpace(req.RequestID),
		Reason:    strings.TrimSpace(req.Reason),
	}, nil
}

// NormalizeApproveRequest validates one durable session approval request.
func NormalizeApproveRequest(req ApproveRequest) (ApproveRequest, error) {
	base, err := NormalizeControlRequest(req.ControlRequest)
	if err != nil {
		return ApproveRequest{}, err
	}
	return ApproveRequest{
		ControlRequest:    base,
		ApprovalPreviewID: strings.TrimSpace(req.ApprovalPreviewID),
		ApprovedPolicy:    cloneArgs(req.ApprovedPolicy),
	}, nil
}

// NormalizeRetryDispatchRequest validates one durable retry-dispatch request.
func NormalizeRetryDispatchRequest(req RetryDispatchRequest) (RetryDispatchRequest, error) {
	base, err := NormalizeControlRequest(req.ControlRequest)
	if err != nil {
		return RetryDispatchRequest{}, err
	}
	dispatchID := strings.TrimSpace(req.DispatchID)
	if dispatchID == "" {
		return RetryDispatchRequest{}, NewValidationError("dispatchId", "dispatchId is required")
	}
	return RetryDispatchRequest{
		ControlRequest:    base,
		DispatchID:        dispatchID,
		ForceNewAttempt:   req.ForceNewAttempt,
		ResetAttemptCount: req.ResetAttemptCount,
	}, nil
}

// NormalizeInterruptDispatchRequest validates one durable interrupt-dispatch request.
func NormalizeInterruptDispatchRequest(req InterruptDispatchRequest) (InterruptDispatchRequest, error) {
	base, err := NormalizeControlRequest(req.ControlRequest)
	if err != nil {
		return InterruptDispatchRequest{}, err
	}
	dispatchID := strings.TrimSpace(req.DispatchID)
	if dispatchID == "" {
		return InterruptDispatchRequest{}, NewValidationError("dispatchId", "dispatchId is required")
	}
	return InterruptDispatchRequest{
		ControlRequest: base,
		DispatchID:     dispatchID,
	}, nil
}

// NormalizeSessionID validates one durable session identifier for read/control calls.
func NormalizeSessionID(sessionID string) (string, error) {
	trimmed := strings.TrimSpace(sessionID)
	if trimmed == "" {
		return "", NewValidationError("sessionId", "sessionId is required")
	}
	return trimmed, nil
}

type controlIdempotencyDocument struct {
	Operation         string `json:"operation"`
	SessionID         string `json:"sessionId"`
	DispatchID        string `json:"dispatchId,omitempty"`
	ForceNewAttempt   bool   `json:"forceNewAttempt,omitempty"`
	ResetAttemptCount bool   `json:"resetAttemptCount,omitempty"`
	ApprovalPreviewID string `json:"approvalPreviewId,omitempty"`
	ApprovedPolicy    any    `json:"approvedPolicy,omitempty"`
}

type controlReplayRecord struct {
	tupleHash string
	result    LifecycleControlResult
	err       error
}

func lookupControlReplay(
	controlReplay map[string]controlReplayRecord,
	sessionID, requestID, tupleHash string,
	operation LifecycleControlKind,
	currentStatus LifecycleStatus,
) (LifecycleControlResult, error, bool) {
	if strings.TrimSpace(requestID) == "" {
		return LifecycleControlResult{}, nil, false
	}
	recorded, ok := controlReplay[requestID]
	if !ok {
		return LifecycleControlResult{}, nil, false
	}
	if err := CheckControlRequestIDReplay(requestID, recorded.tupleHash, tupleHash); err != nil {
		return LifecycleControlResult{}, &ControlError{
			Operation: operation,
			Outcome:   LifecycleControlOutcomeConflict,
			Status:    currentStatus,
			Message:   "control requestId was reused with a different operation or target",
			Links:     LifecycleControlLinksForSession(sessionID, true),
		}, true
	}
	if recorded.err != nil {
		return LifecycleControlResult{}, recorded.err, true
	}
	return recorded.result, nil, true
}

func storeControlReplay(
	controlReplay map[string]controlReplayRecord,
	requestID, tupleHash string,
	result LifecycleControlResult,
	err error,
) {
	if strings.TrimSpace(requestID) == "" {
		return
	}
	controlReplay[requestID] = controlReplayRecord{
		tupleHash: tupleHash,
		result:    result,
		err:       err,
	}
}

// ControlIdempotencyTupleHash returns a stable digest for one normalized lifecycle
// control tuple used to compare replay safety for one requestId.
func ControlIdempotencyTupleHash(
	operation LifecycleControlKind,
	sessionID string,
	approve ApproveRequest,
	retry RetryDispatchRequest,
	interrupt InterruptDispatchRequest,
) (string, error) {
	document, err := normalizeControlIdempotencyDocument(operation, sessionID, approve, retry, interrupt)
	if err != nil {
		return "", err
	}
	encoded, err := json.Marshal(document)
	if err != nil {
		return "", fmt.Errorf("marshal control idempotency tuple: %w", err)
	}
	sum := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

// CheckControlRequestIDReplay reports ErrControlRequestIDConflict when requestId
// was previously recorded with a different normalized control tuple.
func CheckControlRequestIDReplay(requestID, recordedHash, incomingHash string) error {
	if strings.TrimSpace(requestID) == "" {
		return nil
	}
	if recordedHash == "" || recordedHash == incomingHash {
		return nil
	}
	return ErrControlRequestIDConflict
}

func normalizeControlIdempotencyDocument(
	operation LifecycleControlKind,
	sessionID string,
	approve ApproveRequest,
	retry RetryDispatchRequest,
	interrupt InterruptDispatchRequest,
) (controlIdempotencyDocument, error) {
	trimmedSessionID := strings.TrimSpace(sessionID)
	if trimmedSessionID == "" {
		return controlIdempotencyDocument{}, NewValidationError("sessionId", "sessionId is required")
	}
	document := controlIdempotencyDocument{
		Operation: string(operation),
		SessionID: trimmedSessionID,
	}
	switch operation {
	case LifecycleControlApprove:
		normalized, err := NormalizeApproveRequest(approve)
		if err != nil {
			return controlIdempotencyDocument{}, err
		}
		document.ApprovalPreviewID = normalized.ApprovalPreviewID
		document.ApprovedPolicy = normalizeRequestedPolicyForIdempotency(normalized.ApprovedPolicy)
	case LifecycleControlRetryDispatch:
		normalized, err := NormalizeRetryDispatchRequest(retry)
		if err != nil {
			return controlIdempotencyDocument{}, err
		}
		document.DispatchID = normalized.DispatchID
		document.ForceNewAttempt = normalized.ForceNewAttempt
		document.ResetAttemptCount = normalized.ResetAttemptCount
	case LifecycleControlInterruptDispatch:
		normalized, err := NormalizeInterruptDispatchRequest(interrupt)
		if err != nil {
			return controlIdempotencyDocument{}, err
		}
		document.DispatchID = normalized.DispatchID
	}
	return document, nil
}

func (s *JavaScriptRuntimeService) Pause(ctx context.Context, sessionID string, req ControlRequest) (LifecycleControlResult, error) {
	return s.applyRuntimeExtendedLifecycleControl(ctx, sessionID, LifecycleControlPause, req, ApproveRequest{}, RetryDispatchRequest{}, InterruptDispatchRequest{})
}

func (s *JavaScriptRuntimeService) Resume(ctx context.Context, sessionID string, req ControlRequest) (LifecycleControlResult, error) {
	if result, handled, err := s.resumeInterruptedSessionViaLifecycleControl(ctx, sessionID, req); handled {
		return result, err
	}
	return s.applyRuntimeExtendedLifecycleControl(ctx, sessionID, LifecycleControlResume, req, ApproveRequest{}, RetryDispatchRequest{}, InterruptDispatchRequest{})
}

func (s *JavaScriptRuntimeService) Cancel(ctx context.Context, sessionID string, req ControlRequest) (LifecycleControlResult, error) {
	return s.applyRuntimeExtendedLifecycleControl(ctx, sessionID, LifecycleControlCancel, req, ApproveRequest{}, RetryDispatchRequest{}, InterruptDispatchRequest{})
}

func (s *JavaScriptRuntimeService) Terminate(ctx context.Context, sessionID string, req ControlRequest) (LifecycleControlResult, error) {
	return s.applyRuntimeExtendedLifecycleControl(ctx, sessionID, LifecycleControlTerminate, req, ApproveRequest{}, RetryDispatchRequest{}, InterruptDispatchRequest{})
}

func (s *JavaScriptRuntimeService) Approve(ctx context.Context, sessionID string, req ApproveRequest) (LifecycleControlResult, error) {
	normalized, err := NormalizeApproveRequest(req)
	if err != nil {
		return LifecycleControlResult{}, err
	}
	return s.applyRuntimeExtendedLifecycleControl(
		ctx,
		sessionID,
		LifecycleControlApprove,
		normalized.ControlRequest,
		normalized,
		RetryDispatchRequest{},
		InterruptDispatchRequest{},
	)
}

func (s *JavaScriptRuntimeService) RetryDispatch(ctx context.Context, sessionID string, req RetryDispatchRequest) (LifecycleControlResult, error) {
	normalized, err := NormalizeRetryDispatchRequest(req)
	if err != nil {
		return LifecycleControlResult{}, err
	}
	return s.applyRuntimeExtendedLifecycleControl(
		ctx,
		sessionID,
		LifecycleControlRetryDispatch,
		normalized.ControlRequest,
		ApproveRequest{},
		normalized,
		InterruptDispatchRequest{},
	)
}

func (s *JavaScriptRuntimeService) InterruptDispatch(ctx context.Context, sessionID string, req InterruptDispatchRequest) (LifecycleControlResult, error) {
	normalized, err := NormalizeInterruptDispatchRequest(req)
	if err != nil {
		return LifecycleControlResult{}, err
	}
	return s.applyRuntimeExtendedLifecycleControl(
		ctx,
		sessionID,
		LifecycleControlInterruptDispatch,
		normalized.ControlRequest,
		ApproveRequest{},
		RetryDispatchRequest{},
		normalized,
	)
}

type runtimeExtendedControlReplayLookup struct {
	requestID string
	tupleHash string
	result    LifecycleControlResult
	err       error
	stop      bool
}

func lookupRuntimeExtendedControlReplay(
	controlReplay map[string]controlReplayRecord,
	operation LifecycleControlKind,
	sessionID string,
	control ControlRequest,
	approve ApproveRequest,
	retry RetryDispatchRequest,
	interrupt InterruptDispatchRequest,
	currentStatus LifecycleStatus,
) runtimeExtendedControlReplayLookup {
	requestID := strings.TrimSpace(control.RequestID)
	if requestID == "" {
		return runtimeExtendedControlReplayLookup{}
	}
	tupleHash, err := ControlIdempotencyTupleHash(operation, sessionID, approve, retry, interrupt)
	if err != nil {
		return runtimeExtendedControlReplayLookup{requestID: requestID, err: err, stop: true}
	}
	if replayResult, replayErr, replayed := lookupControlReplay(
		controlReplay,
		sessionID,
		requestID,
		tupleHash,
		operation,
		currentStatus,
	); replayed {
		return runtimeExtendedControlReplayLookup{
			requestID: requestID,
			tupleHash: tupleHash,
			result:    replayResult,
			err:       replayErr,
			stop:      true,
		}
	}
	return runtimeExtendedControlReplayLookup{requestID: requestID, tupleHash: tupleHash}
}

func lookupExtendedControlDispatch(
	operation LifecycleControlKind,
	state *runtimeSessionState,
	retry RetryDispatchRequest,
	interrupt InterruptDispatchRequest,
) (DispatchSummary, error) {
	switch operation {
	case LifecycleControlRetryDispatch:
		dispatchSummary, ok := findDispatchSummary(state.dispatches, retry.DispatchID)
		if !ok {
			return DispatchSummary{}, ErrDispatchNotFound
		}
		return dispatchSummary, nil
	case LifecycleControlInterruptDispatch:
		dispatchSummary, ok := findDispatchSummary(state.dispatches, interrupt.DispatchID)
		if !ok {
			return DispatchSummary{}, ErrDispatchNotFound
		}
		return dispatchSummary, nil
	default:
		return DispatchSummary{}, nil
	}
}

func evaluateExtendedLifecycleControlOutcome(
	operation LifecycleControlKind,
	sessionStatus LifecycleStatus,
	dispatchStatus DispatchStatus,
) LifecycleControlOutcome {
	if operation == LifecycleControlInterruptDispatch {
		return EvaluateInterruptDispatchControl(sessionStatus, dispatchStatus)
	}
	return EvaluateLifecycleControl(operation, sessionStatus)
}

func (s *JavaScriptRuntimeService) recordAcceptedRuntimeInterrupt(
	state *runtimeSessionState,
	dispatchSummary DispatchSummary,
	interrupt InterruptDispatchRequest,
) {
	priorDispatchStatus := dispatchSummary.Status
	if applyRuntimeAcceptedLifecycleControl(s, state, LifecycleControlInterruptDispatch, RetryDispatchRequest{}, interrupt) && state.runCancel != nil {
		state.runCancel()
	}
	state.events = rebuildRuntimeSessionCanonicalEvents(state)
	state.events = AppendDispatchInterruptedEvent(
		state.events,
		state.session,
		dispatchSummary,
		interrupt,
		priorDispatchStatus,
		canonicalEventSourceRuntimeService,
		s.now(),
	)
}

func lifecycleControlEventOccurredAt(
	session SessionReadResult,
	operation LifecycleControlKind,
	occurredAt time.Time,
) time.Time {
	if operation == LifecycleControlPause && session.Lifecycle != nil && session.Lifecycle.PausedAt != nil {
		return session.Lifecycle.PausedAt.UTC()
	}
	if operation == LifecycleControlResume && session.Lifecycle != nil && session.Lifecycle.ResumedAt != nil {
		return session.Lifecycle.ResumedAt.UTC()
	}
	return occurredAt
}

func appendAcceptedSessionLifecycleEventIfNeeded(
	events []json.RawMessage,
	session SessionReadResult,
	previousStatus LifecycleStatus,
	operation LifecycleControlKind,
	outcome LifecycleControlOutcome,
	source string,
	reason string,
	occurredAt time.Time,
) []json.RawMessage {
	if operation != LifecycleControlPause && operation != LifecycleControlResume {
		return events
	}
	return AppendSessionLifecycleControlEvent(
		events,
		session,
		previousStatus,
		operation,
		outcome,
		lifecycleControlEventOccurredAt(session, operation, occurredAt),
		source,
		reason,
	)
}

// pkgmaintcheck:ignore-cyclomatic-complexity this runtime control helper keeps dispatch-targeted lifecycle validation and replay together on one seam.
// backendsizecheck:ignore-function accepted control mutation, persistence rollback, and idempotent replay remain atomic on this runtime seam.
// pkgmaintcheck:ignore-function-lines accepted control mutation, persistence rollback, and idempotent replay remain atomic on this runtime seam.
func (s *JavaScriptRuntimeService) applyRuntimeExtendedLifecycleControl(
	ctx context.Context,
	sessionID string,
	operation LifecycleControlKind,
	control ControlRequest,
	approve ApproveRequest,
	retry RetryDispatchRequest,
	interrupt InterruptDispatchRequest,
) (LifecycleControlResult, error) {
	if err := ctx.Err(); err != nil {
		return LifecycleControlResult{}, err
	}
	id, err := NormalizeSessionID(sessionID)
	if err != nil {
		return LifecycleControlResult{}, err
	}
	if err := validateLifecycleControlRequest(operation, control, approve, retry, interrupt); err != nil {
		return LifecycleControlResult{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	state, ok := s.sessions[id]
	if !ok {
		return LifecycleControlResult{}, ErrSessionNotFound
	}

	replay := lookupRuntimeExtendedControlReplay(
		s.controlReplay,
		operation,
		id,
		control,
		approve,
		retry,
		interrupt,
		state.session.Status,
	)
	if replay.stop {
		if replay.err != nil {
			return LifecycleControlResult{}, replay.err
		}
		return replay.result, nil
	}

	var dispatchSummary DispatchSummary
	if operation == LifecycleControlRetryDispatch || operation == LifecycleControlInterruptDispatch {
		var lookupErr error
		dispatchSummary, lookupErr = lookupExtendedControlDispatch(operation, state, retry, interrupt)
		if lookupErr != nil {
			return LifecycleControlResult{}, lookupErr
		}
	}

	outcome := evaluateExtendedLifecycleControlOutcome(operation, state.session.Status, dispatchSummary.Status)
	if outcome == LifecycleControlOutcomeInvalidState || outcome == LifecycleControlOutcomeTerminalSession {
		controlErr := &ControlError{
			Operation: operation,
			Outcome:   outcome,
			Status:    state.session.Status,
			Message:   fmt.Sprintf("%s rejected for session %s in status %s", operation, id, state.session.Status),
			Links:     LifecycleControlLinksForSession(id, true),
		}
		storeControlReplay(s.controlReplay, replay.requestID, replay.tupleHash, LifecycleControlResult{}, controlErr)
		return LifecycleControlResult{}, controlErr
	}

	if outcome == LifecycleControlOutcomeAccepted {
		priorState := cloneRuntimeSessionState(state)
		priorRunCancel := state.runCancel
		previousStatus := state.session.Status
		if operation == LifecycleControlInterruptDispatch {
			s.recordAcceptedRuntimeInterrupt(state, dispatchSummary, interrupt)
		} else {
			interruptRuntime := applyRuntimeAcceptedLifecycleControl(s, state, operation, retry, interrupt)
			if interruptRuntime && state.runCancel != nil {
				state.runCancel()
			}
			if operation == LifecycleControlPause || operation == LifecycleControlResume {
				state.events = appendAcceptedSessionLifecycleEventIfNeeded(
					state.events,
					state.session,
					previousStatus,
					operation,
					outcome,
					canonicalEventSourceRuntimeService,
					control.Reason,
					s.now(),
				)
			} else {
				state.events = rebuildRuntimeSessionCanonicalEvents(state)
			}
		}
		if operation == LifecycleControlPause {
			if err := s.persistSessionSnapshot(cloneRuntimeSessionState(state)); err != nil {
				*state = cloneRuntimeSessionState(&priorState)
				state.runCancel = priorRunCancel
				return LifecycleControlResult{}, err
			}
		}
	}

	result := runtimeExtendedLifecycleControlResultFromState(state, id, operation, outcome, retry, interrupt)
	storeControlReplay(s.controlReplay, replay.requestID, replay.tupleHash, result, nil)
	return result, nil
}

// pkgmaintcheck:ignore-cyclomatic-complexity this runtime mutation helper keeps accepted lifecycle control transitions together on one seam.
func applyRuntimeAcceptedLifecycleControl(
	s *JavaScriptRuntimeService,
	state *runtimeSessionState,
	operation LifecycleControlKind,
	retry RetryDispatchRequest,
	interrupt InterruptDispatchRequest,
) bool {
	interruptRuntime := false
	switch operation {
	case LifecycleControlPause:
		if state.session.Status == LifecycleStatusRunning || state.session.Status == LifecycleStatusResuming {
			pausedAt := s.now()
			state.session.Status = LifecycleStatusPaused
			state.result.SessionStatus = LifecycleStatusPaused
			if state.session.Lifecycle != nil {
				state.session.Lifecycle.PausedAt = &pausedAt
			}
		}
	case LifecycleControlResume:
		if state.session.Status == LifecycleStatusPaused {
			resumedAt := s.now()
			state.session.Status = LifecycleStatusRunning
			state.result.SessionStatus = LifecycleStatusRunning
			if state.session.Lifecycle != nil {
				state.session.Lifecycle.ResumedAt = &resumedAt
			}
		}
	case LifecycleControlCancel:
		state.session.Status = LifecycleStatusCanceling
		state.result.SessionStatus = LifecycleStatusCanceling
		interruptRuntime = true
	case LifecycleControlTerminate:
		finishedAt := s.now()
		state.session.Status = LifecycleStatusTerminated
		state.result.SessionStatus = LifecycleStatusTerminated
		state.result.ResultStatus = ResultStatusUnavailable
		state.result.Availability = defaultUnavailableAvailability()
		if state.session.Lifecycle != nil {
			state.session.Lifecycle.FinishedAt = &finishedAt
			state.session.Lifecycle.TerminatedAt = &finishedAt
		}
		state.session.ResultSummary = &ResultSummary{
			ResultStatus: string(ResultStatusUnavailable),
		}
		interruptRuntime = true
	case LifecycleControlApprove:
		if state.session.Status == LifecycleStatusAwaitingApproval {
			startedAt := s.now()
			state.session.Status = LifecycleStatusRunning
			state.result.SessionStatus = LifecycleStatusRunning
			if state.session.Lifecycle != nil {
				state.session.Lifecycle.StartedAt = &startedAt
			}
		}
	case LifecycleControlRetryDispatch:
		if state.session.Status == LifecycleStatusFailed {
			for index, dispatch := range state.dispatches {
				if dispatch.ID != retry.DispatchID {
					continue
				}
				dispatch.Status = DispatchStatusQueued
				dispatch.Attempt++
				state.dispatches[index] = dispatch
			}
			state.session.Status = LifecycleStatusRunning
			state.result.SessionStatus = LifecycleStatusRunning
		}
	case LifecycleControlInterruptDispatch:
		state.dispatches, state.dispatchStatusTransitions = MarkDispatchInterrupted(
			state.dispatches,
			state.dispatchStatusTransitions,
			interrupt.DispatchID,
			interrupt,
		)
		interruptRuntime = true
	}
	return interruptRuntime
}

func runtimeExtendedLifecycleControlResultFromState(
	state *runtimeSessionState,
	id string,
	operation LifecycleControlKind,
	outcome LifecycleControlOutcome,
	retry RetryDispatchRequest,
	interrupt InterruptDispatchRequest,
) LifecycleControlResult {
	result := LifecycleControlResult{
		SessionID: id,
		Operation: operation,
		Outcome:   outcome,
		Status:    state.session.Status,
		Links:     LifecycleControlLinksForSession(id, true),
	}
	switch operation {
	case LifecycleControlRetryDispatch:
		result.DispatchID = retry.DispatchID
		if outcome == LifecycleControlOutcomeAccepted {
			result.RetryDispatchID = retry.DispatchID
		}
	case LifecycleControlInterruptDispatch:
		result.DispatchID = interrupt.DispatchID
	}
	if outcome == LifecycleControlOutcomeAccepted || outcome == LifecycleControlOutcomeNoOp {
		session := cloneSessionRead(state.session)
		result.Session = &session
	}
	return result
}

// AppendSessionLifecycleControlEvent records one accepted pause or resume control on
// the canonical session event stream without rebuilding earlier lifecycle events.
func AppendSessionLifecycleControlEvent(
	events []json.RawMessage,
	session SessionReadResult,
	previousStatus LifecycleStatus,
	operation LifecycleControlKind,
	outcome LifecycleControlOutcome,
	occurredAt time.Time,
	source string,
	reason string,
) []json.RawMessage {
	if outcome != LifecycleControlOutcomeAccepted {
		return events
	}
	if operation != LifecycleControlPause && operation != LifecycleControlResume {
		return events
	}
	sessionID := strings.TrimSpace(session.SessionID)
	if sessionID == "" {
		return events
	}
	sessionSequence := nextCanonicalSessionEventSequence(events)
	eventID := fmt.Sprintf("session-lifecycle-control/%s/%d", sessionID, sessionSequence)
	event := buildSessionLifecycleControlEvent(
		session,
		previousStatus,
		session.Status,
		operation,
		outcome,
		occurredAt.UTC(),
		source,
		reason,
		eventID,
		sessionSequence,
	)
	return append(append([]json.RawMessage(nil), events...), event)
}

func synthesizeLifecycleControlEventsFromState(
	session SessionReadResult,
	events []json.RawMessage,
	source string,
) []json.RawMessage {
	if session.Lifecycle == nil {
		return synthesizeLifecycleControlEventsFromStatus(session, events, source)
	}
	out := append([]json.RawMessage(nil), events...)
	if session.Lifecycle.PausedAt != nil {
		previousStatus := LifecycleStatusRunning
		if session.Lifecycle.ResumedAt != nil && session.Lifecycle.ResumedAt.After(*session.Lifecycle.PausedAt) {
			// Session was resumed; still synthesize the pause that preceded resume.
		}
		out = appendLifecycleControlEventIfAbsent(
			out,
			session,
			previousStatus,
			LifecycleStatusPaused,
			LifecycleControlPause,
			*session.Lifecycle.PausedAt,
			source,
			"",
		)
	}
	if session.Lifecycle.ResumedAt != nil && session.Lifecycle.PausedAt != nil &&
		!session.Lifecycle.ResumedAt.Before(*session.Lifecycle.PausedAt) {
		out = appendLifecycleControlEventIfAbsent(
			out,
			session,
			LifecycleStatusPaused,
			LifecycleStatusRunning,
			LifecycleControlResume,
			*session.Lifecycle.ResumedAt,
			source,
			"",
		)
	}
	if len(out) > len(events) {
		return out
	}
	return synthesizeLifecycleControlEventsFromStatus(session, events, source)
}

func synthesizeLifecycleControlEventsFromStatus(
	session SessionReadResult,
	events []json.RawMessage,
	source string,
) []json.RawMessage {
	if session.Status != LifecycleStatusPaused {
		return events
	}
	baseTime := canonicalSessionEventTime(session).UTC()
	pausedAt := baseTime.Add(2 * time.Second)
	return appendLifecycleControlEventIfAbsent(
		events,
		session,
		LifecycleStatusRunning,
		LifecycleStatusPaused,
		LifecycleControlPause,
		pausedAt,
		source,
		"",
	)
}

func appendLifecycleControlEventIfAbsent(
	events []json.RawMessage,
	session SessionReadResult,
	previousStatus LifecycleStatus,
	newStatus LifecycleStatus,
	operation LifecycleControlKind,
	occurredAt time.Time,
	source string,
	reason string,
) []json.RawMessage {
	for _, raw := range events {
		var envelope canonicalFactoryEvent
		if err := json.Unmarshal(raw, &envelope); err != nil {
			continue
		}
		if envelope.Type != "SESSION_LIFECYCLE_CONTROL" {
			continue
		}
		var payload sessionLifecycleControlEventPayload
		if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
			continue
		}
		if payload.Operation == string(operation) && payload.NewStatus == string(newStatus) {
			return events
		}
	}
	sessionSequence := nextCanonicalSessionEventSequence(events)
	eventID := fmt.Sprintf("session-lifecycle-control/%s/%d", session.SessionID, sessionSequence)
	event := buildSessionLifecycleControlEvent(
		session,
		previousStatus,
		newStatus,
		operation,
		LifecycleControlOutcomeAccepted,
		occurredAt.UTC(),
		source,
		reason,
		eventID,
		sessionSequence,
	)
	return append(append([]json.RawMessage(nil), events...), event)
}

type sessionLifecycleControlEventPayload struct {
	Operation      string `json:"operation"`
	Outcome        string `json:"outcome"`
	PreviousStatus string `json:"previousStatus"`
	NewStatus      string `json:"newStatus"`
	OccurredAt     string `json:"occurredAt"`
	Reason         string `json:"reason,omitempty"`
}

func buildSessionLifecycleControlEvent(
	session SessionReadResult,
	previousStatus LifecycleStatus,
	newStatus LifecycleStatus,
	operation LifecycleControlKind,
	outcome LifecycleControlOutcome,
	occurredAt time.Time,
	source string,
	reason string,
	eventID string,
	sessionSequence int,
) json.RawMessage {
	sessionID := strings.TrimSpace(session.SessionID)
	orchestratorKind := string(session.OrchestratorKind)
	var orchestratorDialect *string
	if dialect := strings.TrimSpace(session.Dialect); dialect != "" {
		orchestratorDialect = &dialect
	}
	var phaseID *string
	var phaseName *string
	if phase := strings.TrimSpace(session.Phase); phase != "" {
		phaseID = &phase
		phaseName = &phase
	}
	payload := sessionLifecycleControlEventPayload{
		Operation:      string(operation),
		Outcome:        string(outcome),
		PreviousStatus: string(previousStatus),
		NewStatus:      string(newStatus),
		OccurredAt:     occurredAt.Format(time.RFC3339),
	}
	if trimmedReason := strings.TrimSpace(reason); trimmedReason != "" {
		payload.Reason = trimmedReason
	}
	encodedPayload, err := json.Marshal(payload)
	if err != nil {
		return json.RawMessage("{}")
	}

	sequence := sessionSequence + 1
	context := canonicalFactoryEventContext{
		Sequence:        sequence,
		Tick:            sequence,
		EventTime:       occurredAt,
		SessionID:       &sessionID,
		SessionSequence: intPtr(sessionSequence),
		Source:          &source,
	}
	if orchestratorKind != "" {
		context.OrchestratorKind = &orchestratorKind
	}
	if orchestratorDialect != nil {
		context.OrchestratorDialect = orchestratorDialect
	}
	if phaseID != nil {
		context.PhaseID = phaseID
	}
	if phaseName != nil {
		context.PhaseName = phaseName
	}
	encoded, err := json.Marshal(canonicalFactoryEvent{
		SchemaVersion: canonicalFactoryEventSchemaVersion,
		ID:            eventID,
		Type:          "SESSION_LIFECYCLE_CONTROL",
		Context:       context,
		Payload:       encodedPayload,
	})
	if err != nil {
		return json.RawMessage("{}")
	}
	return encoded
}

func nextCanonicalSessionEventSequence(events []json.RawMessage) int {
	next := 0
	for _, raw := range events {
		var envelope canonicalFactoryEvent
		if err := json.Unmarshal(raw, &envelope); err != nil {
			continue
		}
		if envelope.Context.SessionSequence == nil {
			continue
		}
		if *envelope.Context.SessionSequence >= next {
			next = *envelope.Context.SessionSequence + 1
		}
	}
	return next
}
