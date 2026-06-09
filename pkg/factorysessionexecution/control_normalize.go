package factorysessionexecution

import "strings"

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
