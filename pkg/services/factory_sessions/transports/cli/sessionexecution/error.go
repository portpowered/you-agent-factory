package sessionexecution

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	factorysessionexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

const (
	errorCodeValidation              = "SESSION_EXECUTION_VALIDATION_FAILED"
	errorCodeRequestIDConflict       = "EXECUTION_REQUEST_ID_CONFLICT"
	errorCodeSessionNotFound         = "SESSION_NOT_FOUND"
	errorCodeReconnectCursorNotFound = "RECONNECT_CURSOR_NOT_FOUND"
)

type executionError struct {
	Code    string
	Message string
	Field   string
}

func (e *executionError) Error() string {
	if e == nil {
		return ""
	}
	if strings.TrimSpace(e.Field) == "" {
		return fmt.Sprintf("%s: %s", e.Code, e.Message)
	}
	return fmt.Sprintf("%s: %s (%s)", e.Code, e.Message, e.Field)
}

type executionErrorPayload struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Field   string `json:"field,omitempty"`
}

func writeExecutionError(w io.Writer, err error, jsonOutput bool) bool {
	executionErr := asExecutionError(err)
	if executionErr == nil {
		return false
	}
	if w == nil {
		return true
	}
	if jsonOutput {
		payload := executionErrorPayload{
			Code:    executionErr.Code,
			Message: executionErr.Message,
			Field:   strings.TrimSpace(executionErr.Field),
		}
		if encoded, marshalErr := json.Marshal(payload); marshalErr == nil {
			_, _ = fmt.Fprintln(w, string(encoded))
			return true
		}
	}
	_, _ = fmt.Fprintln(w, executionErr.Error())
	return true
}

func asExecutionError(err error) *executionError {
	if err == nil {
		return nil
	}
	var executionErr *executionError
	if errors.As(err, &executionErr) {
		return executionErr
	}
	var inputErr *work.InputError
	if errors.As(err, &inputErr) {
		return &executionError{
			Code:    string(inputErr.Code),
			Message: inputErr.Message,
		}
	}
	var validationErr *factorysessionexecution.ExecutionValidationError
	if errors.As(err, &validationErr) {
		return &executionError{
			Code:    errorCodeValidation,
			Message: validationErr.Message,
			Field:   validationErr.Field,
		}
	}
	if errors.Is(err, factorysessionexecution.ErrExecutionRequestIDConflict) {
		return &executionError{
			Code:    errorCodeRequestIDConflict,
			Message: "execution request id was reused with a different normalized request",
			Field:   "requestId",
		}
	}
	if errors.Is(err, factorysessionexecution.ErrDurableSessionNotFound) {
		return &executionError{
			Code:    errorCodeSessionNotFound,
			Message: "factory session not found",
			Field:   "sessionId",
		}
	}
	if errors.Is(err, factorysessionexecution.ErrReconnectCursorNotFound) {
		return &executionError{
			Code:    errorCodeReconnectCursorNotFound,
			Message: "event reconnect cursor not found in session history",
			Field:   "afterEventId",
		}
	}
	return nil
}
