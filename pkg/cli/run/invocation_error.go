package run

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

const (
	InvocationErrorCodeFailed    = "RUN_INVOCATION_FAILED"
	InvocationErrorCodeCancelled = "RUN_INVOCATION_CANCELLED"
	InvocationErrorCodeTimeout   = "RUN_INVOCATION_TIMEOUT"
)

type InvocationError struct {
	Code    string
	Message string
	Cause   error
}

func (e *InvocationError) Error() string {
	if e == nil {
		return ""
	}
	if strings.TrimSpace(e.Message) == "" {
		return e.Code
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *InvocationError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

type invocationErrorPayload struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// WriteInvocationError renders the stable clean-invocation failure contract to
// stderr. It returns true when err matched an invocation contract error.
func WriteInvocationError(w io.Writer, err error, jsonOutput bool) bool {
	var invocationErr *InvocationError
	if !errors.As(err, &invocationErr) {
		return false
	}
	if w == nil {
		return true
	}
	if jsonOutput {
		data, marshalErr := json.Marshal(invocationErrorPayload{
			Code:    invocationErr.Code,
			Message: invocationErr.Message,
		})
		if marshalErr == nil {
			_, _ = fmt.Fprintln(w, string(data))
			return true
		}
	}
	_, _ = fmt.Fprintln(w, invocationErr.Error())
	return true
}
