package codex

import (
	"encoding/json"
	"strings"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	provider "github.com/portpowered/infinite-you/pkg/workers/provider"
)

const unknownTerminalFailureMessage = "Codex reported a terminal error."

// TerminalFailure is the bounded Codex-owned terminal fact used by provider
// orchestration. Retry execution remains caller-owned policy.
type TerminalFailure struct {
	Type            interfaces.WorkFailureType
	Message         string
	Retryable       bool
	ProviderSession *interfaces.ProviderSessionMetadata
	NativeEventType string
}

// ParseTerminalFailure selects the final exact typed terminal failure from a
// completed Codex JSONL stream. Unknown and malformed records are not failures.
func ParseTerminalFailure(stdout []byte) (TerminalFailure, bool) {
	var result TerminalFailure
	var recognizedFailure bool
	var threadID string
	forEachBoundedRecord(stdout, func(raw []byte) {
		var record recordEnvelope
		if json.Unmarshal(raw, &record) != nil {
			return
		}
		switch record.Type {
		case "thread.started":
			threadID = strings.TrimSpace(record.ThreadID)
		case "turn.failed":
			if record.Error != nil && strings.TrimSpace(record.Error.Message) != "" {
				failure, recognized := classifyTerminalMessage(record.Type, record.Error.Message, threadID)
				if shouldSelectTerminalFailure(recognizedFailure, recognized) {
					result = failure
				}
				recognizedFailure = recognizedFailure || recognized
			}
		case "error":
			if strings.TrimSpace(record.Message) != "" {
				failure, recognized := classifyTerminalMessage(record.Type, record.Message, threadID)
				if shouldSelectTerminalFailure(recognizedFailure, recognized) {
					result = failure
				}
				recognizedFailure = recognizedFailure || recognized
			}
		}
	})
	return result, result.NativeEventType != ""
}

func shouldSelectTerminalFailure(selectedRecognized, candidateRecognized bool) bool {
	return candidateRecognized || !selectedRecognized
}

func classifyTerminalMessage(nativeType, message, threadID string) (TerminalFailure, bool) {
	parsed := provider.ParseCodexProviderFailure(provider.CommandResult{
		ExitCode: 1,
		Stderr:   []byte("ERROR: " + strings.TrimSpace(message)),
	})
	recognized := parsed.Reason != interfaces.WorkFailureTypeUnknown
	if !recognized {
		parsed.Message = unknownTerminalFailureMessage
	}
	providerErr := provider.NewProviderErrorFromResult(parsed, nil)
	decision := provider.WorkFailureDecisionFromProviderError(providerErr)
	return TerminalFailure{
		Type: parsed.Reason, Message: parsed.Message, Retryable: decision.Retryable,
		ProviderSession: providerSession(threadID), NativeEventType: nativeType,
	}, recognized
}
