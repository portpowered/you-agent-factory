package codex

import (
	"encoding/json"
	"fmt"
	"strings"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	effects "github.com/portpowered/infinite-you/pkg/services/providers/internal/service"
)

func exitFailureFromCommandResult(result effects.CommandResult) error {
	if failure, ok := declaredFailureFromCommandOutput(result.Stdout, result.Stderr); ok {
		return failure
	}
	normalized := strings.ToLower(formatCombinedCommandOutput(result))
	switch {
	case containsAny(normalized, "api key", "authentication", "unauthorized", "forbidden", "login required", "not authenticated"):
		return providers.ExecuteFailure{Kind: providers.ExecuteFailureKindAuthentication, Message: declaredFailureMessage(providers.ExecuteFailureKindAuthentication)}
	case containsAny(normalized, "invalid argument", "bad request", "invalid request"):
		return providers.ExecuteFailure{Kind: providers.ExecuteFailureKindInvalidRequest, Message: declaredFailureMessage(providers.ExecuteFailureKindInvalidRequest)}
	case containsAny(normalized, "rate limit", "too many requests", "resource exhausted", "at capacity", "429"):
		return providers.ExecuteFailure{Kind: providers.ExecuteFailureKindThrottled, Message: declaredFailureMessage(providers.ExecuteFailureKindThrottled)}
	case containsAny(normalized, "internal server error", "unexpected status 500", "unexpected status 502", "unexpected status 503", "unexpected status 504"):
		return providers.ExecuteFailure{Kind: providers.ExecuteFailureKindDependency, Message: declaredFailureMessage(providers.ExecuteFailureKindDependency)}
	case result.ExitCode == 124 || containsAny(normalized, "deadline exceeded", "timed out", "timeout", "request timed out"):
		return providers.ExecuteFailure{Kind: providers.ExecuteFailureKindTimeout, Message: declaredFailureMessage(providers.ExecuteFailureKindTimeout)}
	}
	return fmt.Errorf("codex exited with code %d", result.ExitCode)
}

func declaredFailureFromCommandOutput(stdout, stderr []byte) (providers.ExecuteFailure, bool) {
	combined := formatCombinedCommandOutput(effects.CommandResult{Stdout: stdout, Stderr: stderr})
	for _, line := range strings.Split(combined, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if idx := strings.Index(line, "{"); idx >= 0 {
			line = line[idx:]
		}
		var envelope recordEnvelope
		if json.Unmarshal([]byte(line), &envelope) != nil {
			continue
		}
		if envelope.Error != nil {
			failure := classifyDeclaredFailure(*envelope.Error)
			if failure.Kind != providers.ExecuteFailureKindUnknown {
				return failure, true
			}
		}
		if envelope.Type == "error" && strings.TrimSpace(envelope.Message) != "" {
			failure := classifyDeclaredFailure(errorRecord{Message: envelope.Message})
			if failure.Kind != providers.ExecuteFailureKindUnknown {
				return failure, true
			}
		}
		var direct errorRecord
		if json.Unmarshal([]byte(line), &direct) == nil && strings.TrimSpace(direct.Message) != "" {
			failure := classifyDeclaredFailure(direct)
			if failure.Kind != providers.ExecuteFailureKindUnknown {
				return failure, true
			}
		}
	}
	return providers.ExecuteFailure{}, false
}

func formatCombinedCommandOutput(result effects.CommandResult) string {
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
