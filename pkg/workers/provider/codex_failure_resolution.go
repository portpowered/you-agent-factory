package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

const codexJSONLRecordBytes = 1024 * 1024

const codexStructuredStreamUnknownMessage = codexUnknownFailureMessage

const (
	codexInternalCauseExecutionCanceled      = "execution canceled"
	codexInternalCauseDeadlineExceeded       = "context deadline exceeded"
	codexInternalCauseUnrecognizedStructured = "unrecognized structured stream failure"
)

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
func ResolveCodexProviderFailure(result CommandResult, input CodexFailureResolutionInput) (ProviderFailureResolution, bool) {
	signals := collectCodexFailureSignals(result, input)
	if len(signals) == 0 {
		return ProviderFailureResolution{}, false
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
			InternalCause: codexInternalCauseExecutionCanceled,
		})
	}
	if errors.Is(input.CommandError, context.DeadlineExceeded) || result.ExitCode == 124 {
		internalCause := codexInternalCauseDeadlineExceeded
		if result.ExitCode == 124 && !errors.Is(input.CommandError, context.DeadlineExceeded) {
			internalCause = codexExitInternalCause(result.ExitCode)
		}
		signals = append(signals, CompetingFailureSignal{
			Tier:       FailureSignalTierCancelTimeout,
			Recognized: true,
			Result: ProviderFailureResult{
				Reason:  interfaces.WorkFailureTypeTimeout,
				Message: "Codex execution timed out.",
			},
			InternalCause: internalCause,
		})
	}
	if failure, internalCause, ok := parseCodexStructuredStreamFailureWithCause(result.Stdout); ok {
		signals = append(signals, CompetingFailureSignal{
			Tier:          FailureSignalTierStructured,
			Recognized:    IsRecognizedFailureType(failure.Reason),
			Result:        failure,
			InternalCause: internalCause,
		})
	}
	structured, structuredCause, hasStructured, stderr, stderrCause, hasStderr, exit := CodexProcessExitFailureLayersWithCause(result)
	if hasStructured {
		signals = append(signals, CompetingFailureSignal{
			Tier:          FailureSignalTierStructured,
			Recognized:    IsRecognizedFailureType(structured.Reason),
			Result:        structured,
			InternalCause: structuredCause,
		})
	}
	if hasStderr {
		signals = append(signals, CompetingFailureSignal{
			Tier:          FailureSignalTierStderr,
			Recognized:    true,
			Result:        stderr,
			InternalCause: stderrCause,
		})
	}
	signals = append(signals, CompetingFailureSignal{
		Tier:          FailureSignalTierExit,
		Recognized:    IsRecognizedFailureType(exit.Reason),
		Result:        exit,
		InternalCause: codexExitInternalCause(result.ExitCode),
	})
	return signals
}

func parseCodexStructuredStreamFailure(stdout []byte) (ProviderFailureResult, bool) {
	failure, _, ok := parseCodexStructuredStreamFailureWithCause(stdout)
	return failure, ok
}

func parseCodexStructuredStreamFailureWithCause(stdout []byte) (ProviderFailureResult, string, bool) {
	var (
		selected             ProviderFailureResult
		selectedCause        string
		found                bool
		selectedRecognized   bool
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
		var nativeMessage string
		switch record.Type {
		case "turn.failed":
			if record.Error == nil || strings.TrimSpace(record.Error.Message) == "" {
				return
			}
			nativeMessage = record.Error.Message
		case "error":
			if strings.TrimSpace(record.Message) == "" {
				return
			}
			nativeMessage = record.Message
		default:
			return
		}
		failure, recognized := codexStructuredStreamFailureFromMessage(nativeMessage)
		if shouldSelectCodexStructuredStreamFailure(selectedRecognized, recognized) {
			selected = failure
			selectedCause = codexStructuredStreamNativeInternalCause(nativeMessage, recognized)
			found = true
		}
		selectedRecognized = selectedRecognized || recognized
	})
	return selected, selectedCause, found
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
	return codexProcessExitFallback(result.ExitCode)
}

// CodexStructuredStreamReportingOutcome classifies terminal JSONL stdout without
// applying process-exit stderr or exit-status fallback.
func CodexStructuredStreamReportingOutcome(stdout []byte) (ProviderFailureResult, bool) {
	return parseCodexStructuredStreamFailure(stdout)
}

// CodexProcessExitReportingOutcome classifies stderr and exit status without
// structured-stream JSONL signals.
func CodexProcessExitReportingOutcome(result CommandResult) ProviderFailureResult {
	isolated := result
	isolated.Stdout = nil
	return ParseCodexProviderFailureLayers(isolated)
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

func codexStructuredStreamNativeInternalCause(nativeMessage string, recognized bool) string {
	if recognized {
		if cause := codexRecognizedNativeInternalCause(nativeMessage); cause != "" {
			return cause
		}
	}
	return codexInternalCauseUnrecognizedStructured
}

func codexRecognizedNativeInternalCause(message string) string {
	message = strings.TrimSpace(message)
	if message == "" {
		return ""
	}
	reason := classifyRecognizedCodexTextFailure(strings.ToLower(message), 1)
	if reason == interfaces.WorkFailureTypeUnknown {
		return ""
	}
	return codexBoundedInternalCause(message)
}

func codexBoundedInternalCause(value string) string {
	value = strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	if value == "" {
		return ""
	}
	if len(value) <= codexFailureMessageBytes {
		return value
	}
	end := 0
	for index := range value {
		if index > codexFailureMessageBytes {
			break
		}
		end = index
	}
	return strings.TrimSpace(value[:end])
}

func codexExitInternalCause(exitCode int) string {
	return fmt.Sprintf("exit code %d", exitCode)
}

func codexStructuredJSONInternalCause(payload string) string {
	failure, ok := decodeCodexStructuredFailure(payload)
	if !ok {
		return ""
	}
	reason, recognized := classifyCodexStructuredSignal(failure.Type, failure.Status)
	if !recognized {
		return ""
	}
	if reason == interfaces.WorkFailureTypePermanentBadRequest && failure.Message == codexGPT56SolUpgradeMessage {
		return codexBoundedInternalCause(failure.Message)
	}
	parts := make([]string, 0, 3)
	if failure.Type != "" {
		parts = append(parts, failure.Type)
	}
	if failure.Status != 0 {
		parts = append(parts, fmt.Sprintf("status %d", failure.Status))
	}
	if native := codexRecognizedNativeInternalCause(failure.Message); native != "" {
		parts = append(parts, native)
	}
	return codexBoundedInternalCause(strings.Join(parts, ", "))
}

func codexProcessExitStructuredInternalCause(result CommandResult) string {
	streams := []string{
		tailForCodexErrorScan(result.Stderr),
		tailForCodexErrorScan(result.Stdout),
	}
	var last string
	for _, stream := range streams {
		for _, line := range strings.Split(stream, "\n") {
			payload, ok := codexErrorPayload(line)
			if !ok || !strings.HasPrefix(payload, "{") {
				continue
			}
			if cause := codexStructuredJSONInternalCause(payload); cause != "" {
				last = cause
			}
		}
	}
	return last
}

func codexProcessExitStderrInternalCause(result CommandResult) string {
	streams := []string{
		tailForCodexErrorScan(result.Stderr),
		tailForCodexErrorScan(result.Stdout),
	}
	var last string
	for _, stream := range streams {
		for _, line := range strings.Split(stream, "\n") {
			payload, ok := codexErrorPayload(line)
			if !ok || strings.HasPrefix(payload, "{") {
				continue
			}
			if cause := codexRecognizedNativeInternalCause(payload); cause != "" {
				last = cause
			}
		}
	}
	return last
}
