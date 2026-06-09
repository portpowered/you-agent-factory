package factorysessionexecution

import "strings"

// NormalizeResultRequest validates and normalizes one durable result read request.
func NormalizeResultRequest(req ResultRequest) (ResultRequest, error) {
	mode := req.Mode
	if mode == "" {
		mode = ResultModeFinal
	}
	switch mode {
	case ResultModeFinal, ResultModePartial:
	default:
		return ResultRequest{}, NewValidationError("mode", "mode must be final or partial")
	}
	return ResultRequest{
		Mode:             mode,
		IncludeArtifacts: req.IncludeArtifacts,
	}, nil
}

// NormalizeEventReconnectRequest validates one durable session event reconnect request.
func NormalizeEventReconnectRequest(req EventReconnectRequest) (EventReconnectRequest, error) {
	normalized := EventReconnectRequest{
		AfterEventID: strings.TrimSpace(req.AfterEventID),
	}
	if req.AfterSequence != nil {
		sequence := *req.AfterSequence
		if sequence < 0 {
			return EventReconnectRequest{}, NewValidationError("afterSequence", "afterSequence must be non-negative")
		}
		normalized.AfterSequence = &sequence
	}
	return normalized, nil
}
