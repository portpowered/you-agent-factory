package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

const codexJSONLRecordBytes = 1024 * 1024

const codexStructuredStreamUnknownMessage = "Codex reported a terminal error."

// CodexFailureResolutionInput carries runtime cancellation and flush facts that
// outrank structured, stderr, and exit signals.
type CodexFailureResolutionInput struct {
	CommandError error
	FlushReason  string
}

const (
	CodexFlushReasonCanceled = "canceled"
)

// ResolveCodexProviderFailure applies the shared structured/stderr/exit
// precedence model to Codex subprocess output and optional runtime facts.
func ResolveCodexProviderFailure(result CommandResult, input CodexFailureResolutionInput) (ProviderFailureResult, bool) {
	signals := collectCodexFailureSignals(result, input)
	if len(signals) == 0 {
		return ProviderFailureResult{}, false
	}
	return SelectFailureByPrecedence(signals)
}

func collectCodexFailureSignals(result CommandResult, input CodexFailureResolutionInput) []CompetingFailureSignal {
	var signals []CompetingFailureSignal
	if errors.Is(input.CommandError, context.Canceled) || input.FlushReason == CodexFlushReasonCanceled {
		signals = append(signals, CompetingFailureSignal{
			Tier: FailureSignalTierCancelTimeout,
			Result: ProviderFailureResult{
				Reason:  interfaces.WorkFailureTypeUnknown,
				Message: "Codex execution was canceled.",
			},
		})
	}
	if errors.Is(input.CommandError, context.DeadlineExceeded) || result.ExitCode == 124 {
		signals = append(signals, CompetingFailureSignal{
			Tier:       FailureSignalTierCancelTimeout,
			Recognized: true,
			Result: ProviderFailureResult{
				Reason:  interfaces.WorkFailureTypeTimeout,
				Message: "Codex execution timed out.",
			},
		})
	}
	if failure, ok := parseCodexStructuredStreamFailure(result.Stdout); ok {
		signals = append(signals, CompetingFailureSignal{
			Tier:       FailureSignalTierStructured,
			Recognized: IsRecognizedFailureType(failure.Reason),
			Result:     failure,
		})
	}
	structured, hasStructured, stderr, hasStderr, exit := CodexProcessExitFailureLayers(result)
	if hasStructured {
		signals = append(signals, CompetingFailureSignal{
			Tier:       FailureSignalTierStructured,
			Recognized: IsRecognizedFailureType(structured.Reason),
			Result:     structured,
		})
	}
	if hasStderr {
		signals = append(signals, CompetingFailureSignal{
			Tier:       FailureSignalTierStderr,
			Recognized: true,
			Result:     stderr,
		})
	}
	signals = append(signals, CompetingFailureSignal{
		Tier:       FailureSignalTierExit,
		Recognized: IsRecognizedFailureType(exit.Reason),
		Result:     exit,
	})
	return signals
}

func parseCodexStructuredStreamFailure(stdout []byte) (ProviderFailureResult, bool) {
	var (
		selected          ProviderFailureResult
		found             bool
		selectedRecognized bool
	)
	forEachCodexJSONLRecord(stdout, func(raw []byte) {
		var record struct {
			Type     string `json:"type"`
			Message  string `json:"message"`
			ThreadID string `json:"thread_id"`
			Error    *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal(raw, &record) != nil {
			return
		}
		switch record.Type {
		case "turn.failed":
			if record.Error == nil || strings.TrimSpace(record.Error.Message) == "" {
				return
			}
			failure, recognized := codexStructuredStreamFailureFromMessage(record.Error.Message)
			if shouldSelectCodexStructuredStreamFailure(selectedRecognized, recognized) {
				selected = failure
				found = true
			}
			selectedRecognized = selectedRecognized || recognized
		case "error":
			if strings.TrimSpace(record.Message) == "" {
				return
			}
			failure, recognized := codexStructuredStreamFailureFromMessage(record.Message)
			if shouldSelectCodexStructuredStreamFailure(selectedRecognized, recognized) {
				selected = failure
				found = true
			}
			selectedRecognized = selectedRecognized || recognized
		}
	})
	return selected, found
}

func shouldSelectCodexStructuredStreamFailure(selectedRecognized, candidateRecognized bool) bool {
	return candidateRecognized || !selectedRecognized
}

func codexStructuredStreamFailureFromMessage(message string) (ProviderFailureResult, bool) {
	parsed := ParseCodexProviderFailureLayers(CommandResult{
		ExitCode: 1,
		Stderr:   []byte("ERROR: " + strings.TrimSpace(message)),
	})
	if !IsRecognizedFailureType(parsed.Reason) {
		return ProviderFailureResult{
			Reason:  interfaces.WorkFailureTypeUnknown,
			Message: codexStructuredStreamUnknownMessage,
		}, false
	}
	return parsed, true
}

// ParseCodexProviderFailureLayers returns the recognized structured ERROR-record
// or stderr-classified outcome without applying cross-signal precedence.
func ParseCodexProviderFailureLayers(result CommandResult) ProviderFailureResult {
	structured, hasStructured, stderr, hasStderr, _ := CodexProcessExitFailureLayers(result)
	if hasStructured {
		return structured
	}
	if hasStderr {
		return stderr
	}
	return ProviderFailureResult{
		Reason:  classifyCodexExitFailure(result.ExitCode),
		Message: codexExitFailureMessage(result.ExitCode),
	}
}

func forEachCodexJSONLRecord(output []byte, visit func([]byte)) {
	for len(output) > 0 {
		newline := bytes.IndexByte(output, '\n')
		if newline < 0 {
			newline = len(output)
		}
		if newline <= codexJSONLRecordBytes {
			visit(output[:newline])
		}
		if newline == len(output) {
			return
		}
		output = output[newline+1:]
	}
}
