package opencode

import (
	"bytes"
	"fmt"
	"strings"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func exitFailureFromCommandResult(result workers.CommandResult) error {
	if failure, ok := declaredFailureFromCommandOutput(result.Stdout, result.Stderr); ok {
		return failure
	}
	normalized := strings.ToLower(formatCombinedCommandOutput(result))
	switch {
	case containsAny(normalized, "authentication", "login required", "not authenticated", "unauthorized", "forbidden", "api key"):
		return providers.ExecuteFailure{Kind: providers.ExecuteFailureKindAuthentication, Message: openCodeDeclaredFailureMessage(providers.ExecuteFailureKindAuthentication)}
	case containsAny(normalized, "invalid request", "bad request", "invalid argument", "model not found"):
		return providers.ExecuteFailure{Kind: providers.ExecuteFailureKindInvalidRequest, Message: openCodeDeclaredFailureMessage(providers.ExecuteFailureKindInvalidRequest)}
	case containsAny(normalized, "rate limit", "too many requests", "usage limit", "at capacity", "status 429"):
		return providers.ExecuteFailure{Kind: providers.ExecuteFailureKindThrottled, Message: openCodeDeclaredFailureMessage(providers.ExecuteFailureKindThrottled)}
	case containsAny(normalized, "internal server error", "server error", "status 500", "status 502", "status 503", "status 504"):
		return providers.ExecuteFailure{Kind: providers.ExecuteFailureKindDependency, Message: openCodeDeclaredFailureMessage(providers.ExecuteFailureKindDependency)}
	case result.ExitCode == 124 || containsAny(normalized, "deadline exceeded", "request timed out", "timed out", "timeout"):
		return providers.ExecuteFailure{Kind: providers.ExecuteFailureKindTimeout, Message: openCodeDeclaredFailureMessage(providers.ExecuteFailureKindTimeout)}
	}
	return fmt.Errorf("opencode exited with code %d", result.ExitCode)
}

func declaredFailureFromCommandOutput(stdout, stderr []byte) (providers.ExecuteFailure, bool) {
	combined := append(append([]byte(nil), stdout...), stderr...)
	for _, line := range splitStructuredLines(combined) {
		record, err := decodeStructuredRecord(line)
		if err != nil || record.Type != "error" {
			continue
		}
		message := boundedDetail(strings.TrimSpace(record.Error.Name))
		if message == "" {
			message = openCodeDeclaredFailureMessage(providers.ExecuteFailureKindUnknown)
		}
		return providers.ExecuteFailure{Kind: providers.ExecuteFailureKindUnknown, Message: message}, true
	}
	return providers.ExecuteFailure{}, false
}

func openCodeDeclaredFailureMessage(kind providers.ExecuteFailureKind) string {
	switch kind {
	case providers.ExecuteFailureKindAuthentication:
		return "OpenCode authentication failed."
	case providers.ExecuteFailureKindInvalidRequest:
		return "OpenCode rejected the request as invalid."
	case providers.ExecuteFailureKindThrottled:
		return "OpenCode is temporarily unavailable due to usage or capacity limits."
	case providers.ExecuteFailureKindTimeout:
		return "OpenCode request timed out."
	case providers.ExecuteFailureKindDependency:
		return "OpenCode encountered a temporary server error."
	default:
		return "OpenCode reported a structured execution failure."
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

func splitStructuredLines(stdout []byte) [][]byte {
	normalized := bytes.ReplaceAll(stdout, []byte("\r\n"), []byte("\n"))
	lines := bytes.Split(normalized, []byte("\n"))
	result := make([][]byte, 0, len(lines))
	for _, line := range lines {
		if trimmed := bytes.TrimSpace(line); len(trimmed) > 0 && len(trimmed) <= maxRecordBytes {
			result = append(result, trimmed)
		}
	}
	return result
}

func containsAny(text string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(text, needle) {
			return true
		}
	}
	return false
}
