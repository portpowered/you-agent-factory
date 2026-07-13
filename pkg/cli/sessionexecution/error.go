package sessionexecution

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/portpowered/infinite-you/pkg/factorysessionexecution"
	invocations "github.com/portpowered/infinite-you/pkg/work/invocation"
)

const (
	ErrorCodeUnsupportedMode         = "SESSION_EXECUTION_UNSUPPORTED_MODE"
	ErrorCodeSourceConflict          = "SESSION_EXECUTION_SOURCE_CONFLICT"
	ErrorCodeMissingSource           = "SESSION_EXECUTION_MISSING_SOURCE"
	ErrorCodeInvalidArgs             = "SESSION_EXECUTION_INVALID_ARGS"
	ErrorCodeInvalidPolicy           = "SESSION_EXECUTION_INVALID_POLICY"
	ErrorCodeValidation              = "SESSION_EXECUTION_VALIDATION_FAILED"
	ErrorCodeRequestIDConflict       = "EXECUTION_REQUEST_ID_CONFLICT"
	ErrorCodeSessionNotFound         = "SESSION_NOT_FOUND"
	ErrorCodeReconnectCursorNotFound = "RECONNECT_CURSOR_NOT_FOUND"
)

// ExecutionError is the stable CLI durable session execution failure contract.
type ExecutionError struct {
	Code    string
	Message string
	Field   string
}

func (e *ExecutionError) Error() string {
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

// WriteExecutionError renders the stable durable session execution failure
// contract. It returns true when err matched a known execution contract error.
func WriteExecutionError(w io.Writer, err error, jsonOutput bool) bool {
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

func asExecutionError(err error) *ExecutionError {
	if err == nil {
		return nil
	}
	var executionErr *ExecutionError
	if errors.As(err, &executionErr) {
		return executionErr
	}
	var inputErr *invocations.InputError
	if errors.As(err, &inputErr) {
		return &ExecutionError{
			Code:    string(inputErr.Code),
			Message: inputErr.Message,
		}
	}
	var validationErr *factorysessionexecution.ValidationError
	if errors.As(err, &validationErr) {
		return &ExecutionError{
			Code:    ErrorCodeValidation,
			Message: validationErr.Message,
			Field:   validationErr.Field,
		}
	}
	if errors.Is(err, factorysessionexecution.ErrExecutionRequestIDConflict) {
		return &ExecutionError{
			Code:    ErrorCodeRequestIDConflict,
			Message: "execution request id was reused with a different normalized request",
			Field:   "requestId",
		}
	}
	if errors.Is(err, factorysessionexecution.ErrSessionNotFound) {
		return &ExecutionError{
			Code:    ErrorCodeSessionNotFound,
			Message: "factory session not found",
			Field:   "sessionId",
		}
	}
	if errors.Is(err, factorysessionexecution.ErrReconnectCursorNotFound) {
		return &ExecutionError{
			Code:    ErrorCodeReconnectCursorNotFound,
			Message: "event reconnect cursor not found in session history",
			Field:   "afterEventId",
		}
	}
	return nil
}

func newExecutionError(code, message, field string) *ExecutionError {
	return &ExecutionError{
		Code:    code,
		Message: message,
		Field:   field,
	}
}
