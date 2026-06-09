package factorysessionexecution

import (
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
