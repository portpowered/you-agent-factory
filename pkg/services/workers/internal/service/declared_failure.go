package service

import (
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/diagnostics"
)

const declaredFailureMessageRuneLimit = 512

// The runner has already validated the decision envelope. Feedback explains
// that deliberate decision; it must not override a typed provider failure.
func declaredFailureMessage(feedback string) string {
	message := strings.TrimSpace(feedback)
	if message == "" {
		message = "worker explicitly returned a FAILED decision"
	}
	runes := []rune(message)
	if len(runes) > declaredFailureMessageRuneLimit {
		message = string(runes[:declaredFailureMessageRuneLimit])
	}
	return message
}

func declaredFailureFeedback(feedback string, request workers.ExecuteRequest) string {
	message := feedback
	for _, argument := range request.Input.Invocation.Arguments {
		sensitive := argument.Sensitive
		for _, source := range argument.Sources {
			sensitive = sensitive || source.Redact
		}
		if sensitive {
			for _, value := range argument.Values {
				message = redactDeclaredFailureValue(message, value)
			}
		}
	}
	for key, value := range request.Target.Environment.Vars {
		if diagnostics.ClassifyCommandEnvKey(key) == workers.CommandEnvClassificationRedacted {
			message = redactDeclaredFailureValue(message, value)
		}
	}
	for _, entry := range request.Target.Environment.ProcessEnvironment {
		key, value, ok := strings.Cut(entry, "=")
		if ok && diagnostics.ClassifyCommandEnvKey(key) == workers.CommandEnvClassificationRedacted {
			message = redactDeclaredFailureValue(message, value)
		}
	}
	return message
}

func redactDeclaredFailureValue(message, value string) string {
	if value == "" {
		return message
	}
	return strings.ReplaceAll(message, value, "<redacted>")
}
