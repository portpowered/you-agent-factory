package codex

import (
	"encoding/json"
	"strings"

	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"

	provider "github.com/portpowered/infinite-you/pkg/services/workers/provider"
	codexexitfailure "github.com/portpowered/infinite-you/pkg/services/workers/provider/codex/exitfailure"
)

const unknownTerminalFailureMessage = "Codex reported a terminal error."

// TerminalFailure is the bounded Codex-owned terminal fact used by provider
// orchestration. Retry execution remains caller-owned policy.
type TerminalFailure struct {
	Type            workerexecution.WorkFailureType
	Message         string
	Retryable       bool
	ProviderSession *workerexecution.ProviderSessionMetadata
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
	parsed := codexexitfailure.ParseFailureLayers(codexexitfailure.ExitFailureInput{
		ExitCode: 1,
		Stderr:   []byte("ERROR: " + strings.TrimSpace(message)),
	})
	recognized := parsed.Reason != workerexecution.WorkFailureTypeUnknown
	if !recognized {
		parsed.Message = unknownTerminalFailureMessage
	}
	providerErr := provider.NewProviderErrorFromResult(provider.ProviderFailureResult{
		Reason: parsed.Reason, Message: parsed.Message,
	}, nil)
	decision := provider.WorkFailureDecisionFromProviderError(providerErr)
	return TerminalFailure{
		Type: parsed.Reason, Message: parsed.Message, Retryable: decision.Retryable,
		ProviderSession: providerSession(threadID), NativeEventType: nativeType,
	}, recognized
}

func providerSession(id string) *workerexecution.ProviderSessionMetadata {
	if strings.TrimSpace(id) == "" {
		return nil
	}
	return &workerexecution.ProviderSessionMetadata{Provider: "codex", Kind: ProviderSessionKindSessionID, ID: id}
}
