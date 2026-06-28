package run

import (
	"fmt"
	"strings"
)

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
	if strings.TrimSpace(cfg.ReplayPath) != "" {
		return &InvocationError{
			Code:    "INVOCATION_OUTPUT_UNSUPPORTED",
			Message: "response-stream output requires a live runtime owned by this CLI invocation; replay mode has no internal response stream",
		}
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
