package factorysessionexecution

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

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
