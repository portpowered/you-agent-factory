package agentmessage

import (
	"errors"
	"net/http"

	agentmessages "github.com/portpowered/infinite-you/pkg/services/agent_messages"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// Error translates only stable codes and detached public facts. Collaborator
// diagnostics, requests and caller credentials never enter error responses.
func (Mapper) Error(err error) (int, factoryapi.ErrorResponse) {
	status, code := messageErrorCode(err)
	response := factoryapi.ErrorResponse{Code: factoryapi.ErrorResponseCode(code), Message: code, Family: messageErrorFamily(status)}
	var ambiguous *workersessions.AmbiguousAddressError
	if errors.As(err, &ambiguous) && ambiguous != nil {
		details := factoryapi.WorkerSessionAddressDetails{Candidates: []factoryapi.WorkerSessionAddressCandidate{}}
		for _, candidate := range ambiguous.Clone().Candidates {
			details.Candidates = append(details.Candidates, factoryapi.WorkerSessionAddressCandidate{
				FactorySessionId: candidate.FactorySessionID, WorkerSessionId: candidate.WorkerSessionID,
				WorkId: candidate.WorkID, State: factoryapi.WorkerSessionAddressCandidateState(candidate.State),
			})
		}
		response.Details = details
	}
	var limit *agentmessages.LimitError
	if errors.As(err, &limit) && limit != nil {
		response.Details = struct {
			Dimension         string `json:"dimension"`
			RetryAfterSeconds int    `json:"retryAfterSeconds,omitempty"`
		}{Dimension: limit.Dimension, RetryAfterSeconds: limit.RetryAfterSeconds}
	}
	return status, response
}

func messageErrorCode(err error) (int, string) {
	for _, mapping := range []struct {
		err    error
		status int
		code   string
	}{
		{agentmessages.ErrBadRequest, http.StatusBadRequest, "MESSAGE_INVALID_REQUEST"},
		{agentmessages.ErrCursorInvalid, http.StatusBadRequest, "MESSAGE_CURSOR_INVALID"},
		{agentmessages.ErrInterruptUnsupported, http.StatusBadRequest, "MESSAGE_INTERRUPT_UNSUPPORTED"},
		{workersessions.ErrCallerInvalid, http.StatusForbidden, "WORKER_SESSION_CALLER_INVALID"},
		{agentmessages.ErrNotPermitted, http.StatusForbidden, "MESSAGE_NOT_PERMITTED"},
		{agentmessages.ErrRecipientNotFound, http.StatusNotFound, "MESSAGE_RECIPIENT_NOT_FOUND"},
		{agentmessages.ErrMessageNotFound, http.StatusNotFound, "MESSAGE_NOT_FOUND"},
		{workersessions.ErrWorkerSessionAmbiguous, http.StatusConflict, "WORKER_SESSION_AMBIGUOUS"},
		{agentmessages.ErrRequestConflict, http.StatusConflict, "MESSAGE_REQUEST_CONFLICT"},
		{agentmessages.ErrLimitExceeded, http.StatusTooManyRequests, "MESSAGE_LIMIT_EXCEEDED"},
		{agentmessages.ErrDisabled, http.StatusServiceUnavailable, "MESSAGING_DISABLED"},
		{agentmessages.ErrStoreUnavailable, http.StatusServiceUnavailable, "MESSAGE_STORE_UNAVAILABLE"},
		{agentmessages.ErrStoreCorrupt, http.StatusServiceUnavailable, "MESSAGE_STORE_CORRUPT"},
		{agentmessages.ErrStreamUnavailable, http.StatusServiceUnavailable, "MESSAGE_STREAM_UNAVAILABLE"},
		{agentmessages.ErrStreamGap, http.StatusGone, "MESSAGE_STREAM_GAP"},
		{agentmessages.ErrStreamBackpressure, http.StatusServiceUnavailable, "MESSAGE_STREAM_BACKPRESSURE"},
	} {
		if errors.Is(err, mapping.err) {
			return mapping.status, mapping.code
		}
	}
	return http.StatusInternalServerError, "INTERNAL_ERROR"
}

func messageErrorFamily(status int) factoryapi.ErrorFamily {
	switch status {
	case http.StatusBadRequest:
		return factoryapi.ErrorFamilyBadRequest
	case http.StatusNotFound:
		return factoryapi.ErrorFamilyNotFound
	case http.StatusConflict:
		return factoryapi.ErrorFamilyConflict
	case http.StatusGone:
		return factoryapi.ErrorFamilyGone
	default:
		return factoryapi.ErrorFamilyInternalServerError
	}
}
