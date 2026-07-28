package agy

import (
	"strings"
	"unicode/utf8"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

const maxPublishedTextBytes = 256 * 1024

func parseFinalOutput(stdout []byte) (string, *providers.ExecuteFailure) {
	if !utf8.Valid(stdout) {
		return "", unusableFinalOnlyFailure()
	}
	content := strings.TrimSpace(string(stdout))
	if content == "" {
		return "", unusableFinalOnlyFailure()
	}
	return boundedText(content), nil
}

func unusableFinalOnlyFailure() *providers.ExecuteFailure {
	return &providers.ExecuteFailure{
		Kind:    providers.ExecuteFailureKindUnknown,
		Message: "Agy final-only output did not contain an authoritative response.",
	}
}

func boundedText(value string) string {
	if len(value) <= maxPublishedTextBytes {
		return value
	}
	end := maxPublishedTextBytes
	for end > 0 && !utf8.ValidString(value[:end]) {
		end--
	}
	return value[:end]
}

// ParseFinalOutputForTest exposes final-only selection for adapter tests.
func ParseFinalOutputForTest(stdout []byte) (string, *providers.ExecuteFailure) {
	return parseFinalOutput(stdout)
}
