package claude

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	providerservice "github.com/portpowered/infinite-you/pkg/services/providers/internal/service"
)

type apiErrorEnvelope struct {
	Type  string          `json:"type"`
	Error *apiErrorRecord `json:"error"`
}

type apiErrorRecord struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

func exitFailureFromCommandResult(result providerservice.CommandResult) error {
	if kind := explicitHTTPFailureKind(formatCombinedCommandOutput(result)); kind != providers.ExecuteFailureKindUnknown {
		return declaredProviderFailure(kind, claudeDeclaredFailureMessage(kind))
	}
	if failure, ok := declaredFailureFromCommandOutput(result.Stdout, result.Stderr); ok {
		return failure
	}
	normalized := strings.ToLower(formatCombinedCommandOutput(result))
	switch {
	case containsAny(normalized, "no conversation found", "no session found"):
		return declaredProviderFailure(providers.ExecuteFailureKindSessionNotFound, claudeDeclaredFailureMessage(providers.ExecuteFailureKindSessionNotFound))
	case containsAny(normalized, "api key", "authentication", "unauthorized", "forbidden", "login required", "not authenticated"):
		return declaredProviderFailure(providers.ExecuteFailureKindAuthentication, claudeDeclaredFailureMessage(providers.ExecuteFailureKindAuthentication))
	case containsAny(normalized, "invalid argument", "bad request", "invalid request"):
		return declaredProviderFailure(providers.ExecuteFailureKindInvalidRequest, claudeDeclaredFailureMessage(providers.ExecuteFailureKindInvalidRequest))
	case containsAny(normalized, "rate limit", "too many requests", "resource exhausted", "at capacity", "429", "overloaded"):
		return declaredProviderFailure(providers.ExecuteFailureKindThrottled, claudeDeclaredFailureMessage(providers.ExecuteFailureKindThrottled))
	case containsAny(normalized, "internal server error", "api_error", "server_error"):
		return declaredProviderFailure(providers.ExecuteFailureKindDependency, claudeDeclaredFailureMessage(providers.ExecuteFailureKindDependency))
	case result.ExitCode == 124 || containsAny(normalized, "deadline exceeded", "timed out", "timeout", "request timed out"):
		return declaredProviderFailure(providers.ExecuteFailureKindTimeout, claudeDeclaredFailureMessage(providers.ExecuteFailureKindTimeout))
	case containsAny(normalized, "service unavailable", "please try again"):
		return declaredProviderFailure(providers.ExecuteFailureKindDependency, claudeDeclaredFailureMessage(providers.ExecuteFailureKindDependency))
	}
	return fmt.Errorf("claude exited with code %d", result.ExitCode)
}

func declaredFailureFromCommandOutput(stdout, stderr []byte) (providers.ExecuteFailure, bool) {
	combined := formatCombinedCommandOutput(providerservice.CommandResult{Stdout: stdout, Stderr: stderr})
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
			markUnrecognizedProviderRefusal(&failure)
			return failure, true
		}
		var direct apiErrorRecord
		if json.Unmarshal([]byte(line), &direct) == nil && strings.TrimSpace(direct.Message) != "" {
			failure := classifyAPIErrorRecord(direct)
			markUnrecognizedProviderRefusal(&failure)
			return failure, true
		}
	}
	return providers.ExecuteFailure{}, false
}

func classifyAPIErrorRecord(record apiErrorRecord) providers.ExecuteFailure {
	subtype := strings.ToLower(strings.TrimSpace(record.Type))
	message := strings.TrimSpace(record.Message)
	kind := explicitHTTPFailureKind(message)
	if kind != providers.ExecuteFailureKindUnknown {
		return declaredProviderFailure(kind, claudeDeclaredFailureMessage(kind))
	}
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
	if kind == providers.ExecuteFailureKindUnknown && containsAny(strings.ToLower(message), "service unavailable", "please try again") {
		kind = providers.ExecuteFailureKindDependency
	}
	if kind == providers.ExecuteFailureKindUnknown {
		return declaredProviderFailure(kind, message)
	}
	if message == "" {
		message = claudeDeclaredFailureMessage(kind)
	}
	return declaredProviderFailure(kind, message)
}

func markUnrecognizedProviderRefusal(failure *providers.ExecuteFailure) {
	if failure == nil || failure.Kind != providers.ExecuteFailureKindUnknown {
		return
	}
	failure.Diagnostics = &providers.ExecuteDiagnostics{Metadata: map[string]string{
		providers.ExecuteDiagnosticMetadataUnrecognizedProviderRefusal: "true",
	}}
}

func formatCombinedCommandOutput(result providerservice.CommandResult) string {
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

// explicitHTTPFailureKind recognizes provider status diagnostics, not bare
// numbers in prompts or request identifiers. Server evidence wins over prose.
func explicitHTTPFailureKind(message string) providers.ExecuteFailureKind {
	matches := providerHTTPStatus.FindAllStringSubmatch(strings.ToLower(message), -1)
	kind := providers.ExecuteFailureKindUnknown
	for _, match := range matches {
		switch match[1][0] {
		case '5':
			return providers.ExecuteFailureKindDependency
		case '4':
			switch match[1] {
			case "400":
				kind = providers.ExecuteFailureKindInvalidRequest
			case "401", "403":
				kind = providers.ExecuteFailureKindAuthentication
			case "429":
				kind = providers.ExecuteFailureKindThrottled
			}
		}
	}
	return kind
}

var providerHTTPStatus = regexp.MustCompile(`\b(?:unexpected status|http(?: status)?)\s+([45][0-9]{2})\b`)

func declaredProviderFailure(kind providers.ExecuteFailureKind, message string) providers.ExecuteFailure {
	failure := providers.ExecuteFailure{Kind: kind, Message: message}
	if kind == providers.ExecuteFailureKindDependency {
		failure.Diagnostics = &providers.ExecuteDiagnostics{Metadata: map[string]string{
			providers.ExecuteDiagnosticMetadataUpstreamOutage: "true",
		}}
	}
	return failure
}
