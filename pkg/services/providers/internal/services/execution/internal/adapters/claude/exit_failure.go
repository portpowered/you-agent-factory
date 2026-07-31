package claude

import (
	"encoding/json"
	"fmt"
	"strings"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

type apiErrorEnvelope struct {
	Type  string          `json:"type"`
	Error *apiErrorRecord `json:"error"`
}

type apiErrorRecord struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

func exitFailureFromCommandResult(result workers.CommandResult) error {
	if failure, ok := declaredFailureFromCommandOutput(result.Stdout, result.Stderr); ok {
		return failure
	}
	normalized := strings.ToLower(formatCombinedCommandOutput(result))
	switch {
	case containsAny(normalized, "api key", "authentication", "unauthorized", "forbidden", "login required", "not authenticated"):
		return providers.ExecuteFailure{Kind: providers.ExecuteFailureKindAuthentication, Message: claudeDeclaredFailureMessage(providers.ExecuteFailureKindAuthentication)}
	case containsAny(normalized, "invalid argument", "bad request", "invalid request"):
		return providers.ExecuteFailure{Kind: providers.ExecuteFailureKindInvalidRequest, Message: claudeDeclaredFailureMessage(providers.ExecuteFailureKindInvalidRequest)}
	case containsAny(normalized, "rate limit", "too many requests", "resource exhausted", "at capacity", "429", "overloaded"):
		return providers.ExecuteFailure{Kind: providers.ExecuteFailureKindThrottled, Message: claudeDeclaredFailureMessage(providers.ExecuteFailureKindThrottled)}
	case containsAny(normalized, "internal server error", "unexpected status 500", "unexpected status 502", "unexpected status 503", "unexpected status 504", "api_error", "server_error"):
		return providers.ExecuteFailure{Kind: providers.ExecuteFailureKindDependency, Message: claudeDeclaredFailureMessage(providers.ExecuteFailureKindDependency)}
	case result.ExitCode == 124 || containsAny(normalized, "deadline exceeded", "timed out", "timeout", "request timed out"):
		return providers.ExecuteFailure{Kind: providers.ExecuteFailureKindTimeout, Message: claudeDeclaredFailureMessage(providers.ExecuteFailureKindTimeout)}
	}
	return fmt.Errorf("claude exited with code %d", result.ExitCode)
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
		var envelope apiErrorEnvelope
		if json.Unmarshal([]byte(line), &envelope) == nil && envelope.Error != nil {
			failure := classifyAPIErrorRecord(*envelope.Error)
			if failure.Kind != providers.ExecuteFailureKindUnknown {
				return failure, true
			}
		}
		var direct apiErrorRecord
		if json.Unmarshal([]byte(line), &direct) == nil && strings.TrimSpace(direct.Message) != "" {
			failure := classifyAPIErrorRecord(direct)
			if failure.Kind != providers.ExecuteFailureKindUnknown {
				return failure, true
			}
		}
	}
	return providers.ExecuteFailure{}, false
}

func classifyAPIErrorRecord(record apiErrorRecord) providers.ExecuteFailure {
	subtype := strings.ToLower(strings.TrimSpace(record.Type))
	message := strings.TrimSpace(record.Message)
	kind := providers.ExecuteFailureKindUnknown
	switch subtype {
	case "authentication_error", "permission_error":
		kind = providers.ExecuteFailureKindAuthentication
	case "invalid_request_error":
		kind = providers.ExecuteFailureKindInvalidRequest
	case "rate_limit_error", "overloaded_error":
		kind = providers.ExecuteFailureKindThrottled
	case "api_error", "server_error":
		kind = providers.ExecuteFailureKindDependency
	}
	if kind == providers.ExecuteFailureKindUnknown {
		return providers.ExecuteFailure{Kind: kind}
	}
	if message == "" {
		message = claudeDeclaredFailureMessage(kind)
	}
	return providers.ExecuteFailure{Kind: kind, Message: message}
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
