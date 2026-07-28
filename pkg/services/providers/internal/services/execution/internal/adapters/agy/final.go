package agy

import (
	"errors"
	"strings"
	"unicode/utf8"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/workers/agypty"
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

func partialTimeoutStdout(stdout []byte, effectErr error) (string, bool) {
	if !isTimeoutPTYError(effectErr) {
		return "", false
	}
	if !utf8.Valid(stdout) {
		return "", false
	}
	content := strings.TrimSpace(string(stdout))
	if content == "" {
		return "", false
	}
	if agypty.ContainsTerminalEscapeOrControl(content) {
		return "", false
	}
	return boundedText(content), true
}

func isTimeoutPTYError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, agypty.ErrSessionTimedOut) {
		return true
	}
	var declared providers.ExecuteFailure
	if errors.As(err, &declared) {
		return declared.Kind == providers.ExecuteFailureKindTimeout
	}
	var declaredPointer *providers.ExecuteFailure
	if errors.As(err, &declaredPointer) && declaredPointer != nil {
		return declaredPointer.Kind == providers.ExecuteFailureKindTimeout
	}
	return false
}
