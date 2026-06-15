package factorysessionexecution

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
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

func (s *RuntimeService) Pause(ctx context.Context, sessionID string, req ControlRequest) (LifecycleControlResult, error) {
	return s.applyRuntimeExtendedLifecycleControl(ctx, sessionID, LifecycleControlPause, req, ApproveRequest{}, RetryDispatchRequest{})
}

func (s *RuntimeService) Resume(ctx context.Context, sessionID string, req ControlRequest) (LifecycleControlResult, error) {
	return s.applyRuntimeExtendedLifecycleControl(ctx, sessionID, LifecycleControlResume, req, ApproveRequest{}, RetryDispatchRequest{})
}

func (s *RuntimeService) Cancel(ctx context.Context, sessionID string, req ControlRequest) (LifecycleControlResult, error) {
	return s.applyRuntimeExtendedLifecycleControl(ctx, sessionID, LifecycleControlCancel, req, ApproveRequest{}, RetryDispatchRequest{})
}

func (s *RuntimeService) Terminate(ctx context.Context, sessionID string, req ControlRequest) (LifecycleControlResult, error) {
	return s.applyRuntimeExtendedLifecycleControl(ctx, sessionID, LifecycleControlTerminate, req, ApproveRequest{}, RetryDispatchRequest{})
}

func (s *RuntimeService) Approve(ctx context.Context, sessionID string, req ApproveRequest) (LifecycleControlResult, error) {
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

func (s *RuntimeService) RetryDispatch(ctx context.Context, sessionID string, req RetryDispatchRequest) (LifecycleControlResult, error) {
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

func (s *RuntimeService) applyRuntimeExtendedLifecycleControl(
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

	if operation == LifecycleControlRetryDispatch {
		if _, ok := findDispatchSummary(state.dispatches, retry.DispatchID); !ok {
			return LifecycleControlResult{}, ErrDispatchNotFound
		}
	}

	currentStatus := state.session.Status
	outcome := EvaluateLifecycleControl(operation, currentStatus)
	if outcome == LifecycleControlOutcomeInvalidState || outcome == LifecycleControlOutcomeTerminalSession {
		return LifecycleControlResult{}, &ControlError{
			Operation: operation,
			Outcome:   outcome,
			Status:    currentStatus,
			Message:   fmt.Sprintf("%s rejected for session %s in status %s", operation, id, currentStatus),
		}
	}

	if outcome == LifecycleControlOutcomeAccepted {
		interruptRuntime := applyRuntimeAcceptedLifecycleControl(s, state, operation, retry)
		if interruptRuntime && state.runCancel != nil {
			state.runCancel()
		}
		state.events = BuildCanonicalRuntimeSessionEvents(state.session, state.result)
	}

	return runtimeExtendedLifecycleControlResultFromState(state, id, operation, outcome, retry), nil
}

// pkgmaintcheck:ignore-cyclomatic-complexity this runtime mutation helper keeps accepted lifecycle control transitions together on one seam.
func applyRuntimeAcceptedLifecycleControl(
	s *RuntimeService,
	state *runtimeSessionState,
	operation LifecycleControlKind,
	retry RetryDispatchRequest,
) bool {
	interruptRuntime := false
	switch operation {
	case LifecycleControlPause:
		if state.session.Status == LifecycleStatusRunning || state.session.Status == LifecycleStatusResuming {
			pausedAt := s.now().UTC()
			state.session.Status = LifecycleStatusPaused
			state.result.SessionStatus = LifecycleStatusPaused
			if state.session.Lifecycle != nil {
				state.session.Lifecycle.PausedAt = &pausedAt
			}
		}
	case LifecycleControlResume:
		if state.session.Status == LifecycleStatusPaused {
			resumedAt := s.now().UTC()
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
		finishedAt := s.now().UTC()
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
			startedAt := s.now().UTC()
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
