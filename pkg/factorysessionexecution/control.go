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
	requestID, tupleHash string,
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
) (string, error) {
	document, err := normalizeControlIdempotencyDocument(operation, sessionID, approve, retry)
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
	}
	return document, nil
}

func (s *JavaScriptRuntimeService) Pause(ctx context.Context, sessionID string, req ControlRequest) (LifecycleControlResult, error) {
	return s.applyRuntimeExtendedLifecycleControl(ctx, sessionID, LifecycleControlPause, req, ApproveRequest{}, RetryDispatchRequest{})
}

func (s *JavaScriptRuntimeService) Resume(ctx context.Context, sessionID string, req ControlRequest) (LifecycleControlResult, error) {
	return s.applyRuntimeExtendedLifecycleControl(ctx, sessionID, LifecycleControlResume, req, ApproveRequest{}, RetryDispatchRequest{})
}

func (s *JavaScriptRuntimeService) Cancel(ctx context.Context, sessionID string, req ControlRequest) (LifecycleControlResult, error) {
	return s.applyRuntimeExtendedLifecycleControl(ctx, sessionID, LifecycleControlCancel, req, ApproveRequest{}, RetryDispatchRequest{})
}

func (s *JavaScriptRuntimeService) Terminate(ctx context.Context, sessionID string, req ControlRequest) (LifecycleControlResult, error) {
	return s.applyRuntimeExtendedLifecycleControl(ctx, sessionID, LifecycleControlTerminate, req, ApproveRequest{}, RetryDispatchRequest{})
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
	currentStatus LifecycleStatus,
) runtimeExtendedControlReplayLookup {
	requestID := strings.TrimSpace(control.RequestID)
	if requestID == "" {
		return runtimeExtendedControlReplayLookup{}
	}
	tupleHash, err := ControlIdempotencyTupleHash(operation, sessionID, approve, retry)
	if err != nil {
		return runtimeExtendedControlReplayLookup{requestID: requestID, err: err, stop: true}
	}
	if replayResult, replayErr, replayed := lookupControlReplay(
		controlReplay,
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

func (s *JavaScriptRuntimeService) applyRuntimeExtendedLifecycleControl(
	ctx context.Context,
	sessionID string,
	operation LifecycleControlKind,
	control ControlRequest,
	approve ApproveRequest,
	retry RetryDispatchRequest,
) (LifecycleControlResult, error) {
	if err := ctx.Err(); err != nil {
		return LifecycleControlResult{}, err
	}
	id, err := NormalizeSessionID(sessionID)
	if err != nil {
		return LifecycleControlResult{}, err
	}
	if err := validateLifecycleControlRequest(operation, control, approve, retry); err != nil {
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
		state.session.Status,
	)
	if replay.stop {
		if replay.err != nil {
			return LifecycleControlResult{}, replay.err
		}
		return replay.result, nil
	}

	if operation == LifecycleControlRetryDispatch {
		if _, ok := findDispatchSummary(state.dispatches, retry.DispatchID); !ok {
			return LifecycleControlResult{}, ErrDispatchNotFound
		}
	}

	outcome := EvaluateLifecycleControl(operation, state.session.Status)
	if outcome == LifecycleControlOutcomeInvalidState || outcome == LifecycleControlOutcomeTerminalSession {
		controlErr := &ControlError{
			Operation: operation,
			Outcome:   outcome,
			Status:    state.session.Status,
			Message:   fmt.Sprintf("%s rejected for session %s in status %s", operation, id, state.session.Status),
		}
		storeControlReplay(s.controlReplay, replay.requestID, replay.tupleHash, LifecycleControlResult{}, controlErr)
		return LifecycleControlResult{}, controlErr
	}

	if outcome == LifecycleControlOutcomeAccepted {
		previousStatus := state.session.Status
		interruptRuntime := applyRuntimeAcceptedLifecycleControl(s, state, operation, retry)
		if interruptRuntime && state.runCancel != nil {
			state.runCancel()
		}
		switch operation {
		case LifecycleControlPause, LifecycleControlResume:
			occurredAt := time.Now().UTC()
			if operation == LifecycleControlPause && state.session.Lifecycle != nil && state.session.Lifecycle.PausedAt != nil {
				occurredAt = state.session.Lifecycle.PausedAt.UTC()
			}
			if operation == LifecycleControlResume && state.session.Lifecycle != nil && state.session.Lifecycle.ResumedAt != nil {
				occurredAt = state.session.Lifecycle.ResumedAt.UTC()
			}
			state.events = AppendSessionLifecycleControlEvent(
				state.events,
				state.session,
				previousStatus,
				operation,
				outcome,
				occurredAt,
				canonicalEventSourceRuntimeService,
				control.Reason,
			)
		default:
			state.events = BuildCanonicalRuntimeSessionEvents(state.session, state.result)
		}
	}

	result := runtimeExtendedLifecycleControlResultFromState(state, id, operation, outcome, retry)
	storeControlReplay(s.controlReplay, replay.requestID, replay.tupleHash, result, nil)
	return result, nil
}

// pkgmaintcheck:ignore-cyclomatic-complexity this runtime mutation helper keeps accepted lifecycle control transitions together on one seam.
func applyRuntimeAcceptedLifecycleControl(
	s *JavaScriptRuntimeService,
	state *runtimeSessionState,
	operation LifecycleControlKind,
	retry RetryDispatchRequest,
) bool {
	interruptRuntime := false
	switch operation {
	case LifecycleControlPause:
		if state.session.Status == LifecycleStatusRunning || state.session.Status == LifecycleStatusResuming {
			pausedAt := time.Now().UTC()
			state.session.Status = LifecycleStatusPaused
			state.result.SessionStatus = LifecycleStatusPaused
			if state.session.Lifecycle != nil {
				state.session.Lifecycle.PausedAt = &pausedAt
			}
		}
	case LifecycleControlResume:
		if state.session.Status == LifecycleStatusPaused {
			resumedAt := time.Now().UTC()
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
		finishedAt := time.Now().UTC()
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
			startedAt := time.Now().UTC()
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
	}
	return interruptRuntime
}

func runtimeExtendedLifecycleControlResultFromState(
	state *runtimeSessionState,
	id string,
	operation LifecycleControlKind,
	outcome LifecycleControlOutcome,
	retry RetryDispatchRequest,
) LifecycleControlResult {
	result := LifecycleControlResult{
		SessionID: id,
		Operation: operation,
		Outcome:   outcome,
		Status:    state.session.Status,
		Links:     LifecycleControlLinksForSession(id, true),
	}
	if operation == LifecycleControlRetryDispatch {
		result.DispatchID = retry.DispatchID
		if outcome == LifecycleControlOutcomeAccepted {
			result.RetryDispatchID = retry.DispatchID
		}
	}
	if outcome == LifecycleControlOutcomeAccepted || outcome == LifecycleControlOutcomeNoOp {
		session := cloneSessionRead(state.session)
		result.Session = &session
	}
	return result
}
