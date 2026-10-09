package codex

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	providerservice "github.com/portpowered/infinite-you/pkg/services/providers/internal/service"
)

const (
	codexUntrustedWorkingDirectoryNeedle = "not inside a trusted directory"
	codexSkipGitRepoCheckNeedle          = "--skip-git-repo-check was not specified"
	codexUntrustedWorkingDirectoryExit   = 1
	maxWorkingDirectoryDiagnosticRunes   = 256
)

func exitFailureFromCommandResult(result providerservice.CommandResult, workingDirectory string) error {
	if kind := explicitHTTPFailureKind(formatCombinedCommandOutput(result)); kind != providers.ExecuteFailureKindUnknown {
		return providers.ExecuteFailure{Kind: kind, Message: declaredFailureMessage(kind)}
	}
	if failure, ok := declaredFailureFromCommandOutput(result.Stdout, result.Stderr); ok {
		return failure
	}
	normalized := strings.ToLower(formatCombinedCommandOutput(result))
	switch {
	case containsAny(normalized, "no rollout found", "no conversation found", "no thread found", "thread not found"):
		return providers.ExecuteFailure{Kind: providers.ExecuteFailureKindSessionNotFound, Message: declaredFailureMessage(providers.ExecuteFailureKindSessionNotFound)}
	case result.ExitCode == codexUntrustedWorkingDirectoryExit &&
		containsAny(normalized, codexUntrustedWorkingDirectoryNeedle) &&
		strings.Contains(normalized, codexSkipGitRepoCheckNeedle):
		return untrustedWorkingDirectoryFailure(workingDirectory)
	case containsAny(normalized, "api key", "authentication", "unauthorized", "forbidden", "login required", "not authenticated"):
		return providers.ExecuteFailure{Kind: providers.ExecuteFailureKindAuthentication, Message: declaredFailureMessage(providers.ExecuteFailureKindAuthentication)}
	case containsAny(normalized, "invalid argument", "bad request", "invalid request"):
		return providers.ExecuteFailure{Kind: providers.ExecuteFailureKindInvalidRequest, Message: declaredFailureMessage(providers.ExecuteFailureKindInvalidRequest)}
	case containsAny(normalized, "rate limit", "too many requests", "resource exhausted", "at capacity", "server_overloaded", "429"):
		return providers.ExecuteFailure{Kind: providers.ExecuteFailureKindThrottled, Message: declaredFailureMessage(providers.ExecuteFailureKindThrottled)}
	case containsAny(normalized, "internal server error"):
		return providers.ExecuteFailure{Kind: providers.ExecuteFailureKindDependency, Message: declaredFailureMessage(providers.ExecuteFailureKindDependency)}
	case result.ExitCode == 124 || containsAny(normalized, "deadline exceeded", "timed out", "timeout", "request timed out"):
		return providers.ExecuteFailure{Kind: providers.ExecuteFailureKindTimeout, Message: declaredFailureMessage(providers.ExecuteFailureKindTimeout)}
	case containsAny(normalized, "service unavailable", "please try again"):
		return providers.ExecuteFailure{Kind: providers.ExecuteFailureKindDependency, Message: declaredFailureMessage(providers.ExecuteFailureKindDependency)}
	}
	return fmt.Errorf("codex exited with code %d", result.ExitCode)
}

func untrustedWorkingDirectoryFailure(workingDirectory string) providers.ExecuteFailure {
	directory := safeWorkingDirectoryDiagnostic(workingDirectory)
	if directory == "" {
		directory = "the current working directory"
	} else {
		directory = "[" + directory + "]"
	}
	return providers.ExecuteFailure{
		Kind: providers.ExecuteFailureKindInvalidRequest,
		Message: fmt.Sprintf(
			"Codex requires a trusted working directory: %s is not trusted. Run this invocation from a suitable trusted Git repository, or establish trust using Codex's supported workflow.",
			directory,
		),
		Diagnostics: &providers.ExecuteDiagnostics{Metadata: map[string]string{
			providers.ExecuteDiagnosticMetadataSafeFailureMessage: "true",
		}},
	}
}

func safeWorkingDirectoryDiagnostic(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	var builder strings.Builder
	for _, character := range value {
		switch character {
		case '\r':
			builder.WriteString(`\r`)
		case '\n':
			builder.WriteString(`\n`)
		case '\t':
			builder.WriteString(`\t`)
		default:
			if character < 0x20 || character == 0x7f || !utf8.ValidRune(character) {
				builder.WriteRune('?')
				continue
			}
			builder.WriteRune(character)
		}
	}
	rendered := builder.String()
	runes := []rune(rendered)
	if len(runes) > maxWorkingDirectoryDiagnosticRunes {
		return string(runes[:maxWorkingDirectoryDiagnosticRunes])
	}
	return rendered
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
		var envelope recordEnvelope
		if json.Unmarshal([]byte(line), &envelope) != nil {
			continue
		}
		if envelope.Error != nil {
			failure := classifyDeclaredFailure(*envelope.Error)
			markUnrecognizedProviderRefusal(&failure)
			return failure, true
		}
		if envelope.Type == "error" && strings.TrimSpace(envelope.Message) != "" {
			failure := classifyDeclaredFailure(errorRecord{Message: envelope.Message})
			markUnrecognizedProviderRefusal(&failure)
			return failure, true
		}
		if envelope.Type == "task_complete" && envelope.Error == nil {
			continue
		}
		var direct errorRecord
		if json.Unmarshal([]byte(line), &direct) == nil && strings.TrimSpace(direct.Message) != "" {
			failure := classifyDeclaredFailure(direct)
			markUnrecognizedProviderRefusal(&failure)
			return failure, true
		}
	}
	return providers.ExecuteFailure{}, false
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
