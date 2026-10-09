package service

import (
	"sort"
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
	var privateValues []string
	for _, argument := range request.Input.Invocation.Arguments {
		sensitive := argument.Sensitive
		for _, source := range argument.Sources {
			sensitive = sensitive || source.Redact
		}
		if sensitive {
			privateValues = append(privateValues, argument.Values...)
		}
	}
	for key, value := range request.Target.Environment.Vars {
		if diagnostics.ClassifyCommandEnvKey(key) == workers.CommandEnvClassificationRedacted {
			privateValues = append(privateValues, value)
		}
	}
	for _, entry := range request.Target.Environment.ProcessEnvironment {
		key, value, ok := strings.Cut(entry, "=")
		if ok && diagnostics.ClassifyCommandEnvKey(key) == workers.CommandEnvClassificationRedacted {
			privateValues = append(privateValues, value)
		}
	}
	// Match the original feedback once. A shorter value from any source must
	// not hide a longer private value or rescan an inserted redaction marker.
	sort.Slice(privateValues, func(i, j int) bool {
		return len(privateValues[i]) > len(privateValues[j])
	})
	replacements := make([]string, 0, 2*len(privateValues))
	for _, value := range privateValues {
		if value != "" {
			replacements = append(replacements, value, "<redacted>")
		}
	}
	return strings.NewReplacer(replacements...).Replace(feedback)
}
