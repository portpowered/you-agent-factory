package cursor

import (
	"encoding/json"
	"fmt"
	"strings"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

type terminalResultEnvelope struct {
	Type      string `json:"type"`
	Subtype   string `json:"subtype"`
	IsError   bool   `json:"is_error"`
	Result    string `json:"result"`
	SessionID string `json:"session_id"`
}

func exitFailureFromCommandResult(result workers.CommandResult) error {
	if failure, ok := declaredFailureFromCommandOutput(result.Stdout, result.Stderr); ok {
		return failure
	}
	normalized := strings.ToLower(formatCombinedCommandOutput(result))
	switch {
	case containsAny(normalized, "api key", "authentication", "unauthorized", "forbidden", "login required", "not authenticated"):
		return providers.ExecuteFailure{Kind: providers.ExecuteFailureKindAuthentication, Message: cursorDeclaredFailureMessage(providers.ExecuteFailureKindAuthentication)}
	case containsAny(normalized, "invalid argument", "bad request", "invalid request"):
		return providers.ExecuteFailure{Kind: providers.ExecuteFailureKindInvalidRequest, Message: cursorDeclaredFailureMessage(providers.ExecuteFailureKindInvalidRequest)}
	case containsAny(normalized, "rate limit", "too many requests", "resource exhausted", "at capacity", "429", "overloaded"):
		return providers.ExecuteFailure{Kind: providers.ExecuteFailureKindThrottled, Message: cursorDeclaredFailureMessage(providers.ExecuteFailureKindThrottled)}
	case containsAny(normalized, "internal server error", "unexpected status 500", "unexpected status 502", "unexpected status 503", "unexpected status 504", "server error"):
		return providers.ExecuteFailure{Kind: providers.ExecuteFailureKindDependency, Message: cursorDeclaredFailureMessage(providers.ExecuteFailureKindDependency)}
	case result.ExitCode == 124 || containsAny(normalized, "deadline exceeded", "timed out", "timeout", "request timed out"):
		return providers.ExecuteFailure{Kind: providers.ExecuteFailureKindTimeout, Message: cursorDeclaredFailureMessage(providers.ExecuteFailureKindTimeout)}
	}
	return fmt.Errorf("cursor exited with code %d", result.ExitCode)
}

func declaredFailureFromCommandOutput(stdout, stderr []byte) (providers.ExecuteFailure, bool) {
	combined := formatCombinedCommandOutput(workers.CommandResult{Stdout: stdout, Stderr: stderr})
	for _, line := range strings.Split(combined, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if idx := strings.Index(line, "{"); idx >= 0 {
			line = line[idx:]
		}
		var envelope terminalResultEnvelope
		if json.Unmarshal([]byte(line), &envelope) != nil {
			continue
		}
		if envelope.Type != resultTypeResult || envelope.Subtype == resultSubtypeSuccess || !envelope.IsError {
			continue
		}
		message := boundedDetail(strings.TrimSpace(envelope.Result))
		if message == "" {
			message = cursorDeclaredFailureMessage(providers.ExecuteFailureKindUnknown)
		}
		return providers.ExecuteFailure{Kind: providers.ExecuteFailureKindUnknown, Message: message}, true
	}
	return providers.ExecuteFailure{}, false
}

func cursorDeclaredFailureMessage(kind providers.ExecuteFailureKind) string {
	switch kind {
	case providers.ExecuteFailureKindAuthentication:
		return "Cursor authentication failed. Sign in again or check the configured credentials."
	case providers.ExecuteFailureKindInvalidRequest:
		return "Cursor rejected the request as invalid. Check the model and Cursor configuration."
	case providers.ExecuteFailureKindThrottled:
		return "Cursor is temporarily unavailable due to usage or capacity limits."
	case providers.ExecuteFailureKindTimeout:
		return "Cursor request timed out."
	case providers.ExecuteFailureKindDependency:
		return "Cursor encountered a temporary server error."
	default:
		return "Cursor reported an unsuccessful result."
	}
}

func formatCombinedCommandOutput(result workers.CommandResult) string {
	stdout := strings.TrimSpace(string(result.Stdout))
	stderr := strings.TrimSpace(string(result.Stderr))
	switch {
	case stdout != "" && stderr != "":
		return stdout + "\n" + stderr
	case stderr != "":
		return stderr
	default:
		return stdout
	}
}

func containsAny(text string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(text, needle) {
			return true
		}
	}
	return false
}
