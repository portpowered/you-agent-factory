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

const (
	// InvocationOutputPrimaryResult is the default one-shot invocation stdout
	// contract: primary-result-only output with no live progress rendering.
	InvocationOutputPrimaryResult = ""
	// invocationOutputPrimaryLiteral is the documented spelling for the default
	// primary-result-only output mode.
	invocationOutputPrimaryLiteral = "primary"
	// InvocationOutputResponseStream enables live internal SessionResponseStream
	// progress rendering for supported one-shot factory invocations.
	InvocationOutputResponseStream = "response-stream"
)

func NormalizeInvocationOutputMode(raw string) (string, error) {
	return normalizeInvocationOutputMode(raw)
}

func normalizeInvocationOutputMode(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	switch trimmed {
	case InvocationOutputPrimaryResult, invocationOutputPrimaryLiteral:
		return InvocationOutputPrimaryResult, nil
	case InvocationOutputResponseStream:
		return InvocationOutputResponseStream, nil
	default:
		return "", fmt.Errorf(
			"unsupported --output value %q; supported values are primary (default) and response-stream",
			trimmed,
		)
	}
}

func isResponseStreamOutputMode(mode string) bool {
	return strings.TrimSpace(mode) == InvocationOutputResponseStream
}

func validateInvocationOutputMode(cfg RunConfig, invocationMode bool) error {
	if !isResponseStreamOutputMode(cfg.InvocationOutputMode) {
		return nil
	}
	if cfg.Continuously {
		return &InvocationError{
			Code:    "INVOCATION_OUTPUT_UNSUPPORTED",
			Message: "response-stream output is not supported with --continuously",
		}
	}
	if !invocationMode {
		return &InvocationError{
			Code:    "INVOCATION_OUTPUT_UNSUPPORTED",
			Message: "response-stream output requires a one-shot factory invocation such as you run --named or you run --factory with positional text or piped stdin",
		}
	}
	return nil
}
