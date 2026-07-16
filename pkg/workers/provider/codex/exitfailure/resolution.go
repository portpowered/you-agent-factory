package exitfailure

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"
)

const (
	FlushReasonCanceled   = "canceled"
	codexJSONLRecordBytes = 1024 * 1024
)

const (
	internalCauseExecutionCanceled      = "execution canceled"
	internalCauseDeadlineExceeded       = "context deadline exceeded"
	internalCauseUnrecognizedStructured = "unrecognized structured stream failure"
)

// ResolutionInput carries runtime cancellation and flush facts that outrank
// structured, stderr, and exit signals.
type ResolutionInput struct {
	CommandError error
	FlushReason  string
}

// FailureResolution is the winning Codex exit-failure outcome plus a bounded
// internal-cause excerpt from the selected signal.
type FailureResolution struct {
	Result        ExitFailureResult
	InternalCause string
}

type failureSignalTier int

const (
	signalTierCancelTimeout failureSignalTier = iota
	signalTierStructured
	signalTierStderr
	signalTierExit
)

type competingSignal struct {
	Tier          failureSignalTier
	Recognized    bool
	Result        ExitFailureResult
	InternalCause string
}

// ResolveFailure applies Codex structured/stderr/exit precedence to subprocess
// output and optional runtime facts.
func ResolveFailure(input ExitFailureInput, resolution ResolutionInput) (FailureResolution, bool) {
	signals := collectFailureSignals(input, resolution)
	if len(signals) == 0 {
		return FailureResolution{}, false
	}
	return selectFailureByPrecedence(signals)
}

func collectFailureSignals(input ExitFailureInput, resolution ResolutionInput) []competingSignal {
	var signals []competingSignal
	if errors.Is(resolution.CommandError, context.Canceled) || resolution.FlushReason == FlushReasonCanceled {
		signals = append(signals, competingSignal{
			Tier: signalTierCancelTimeout,
			Result: ExitFailureResult{
				Reason:  workerexecution.WorkFailureTypeUnknown,
				Message: "Codex execution was canceled.",
			},
			InternalCause: internalCauseExecutionCanceled,
		})
	}
	if errors.Is(resolution.CommandError, context.DeadlineExceeded) || input.ExitCode == 124 {
		internalCause := internalCauseDeadlineExceeded
		if input.ExitCode == 124 && !errors.Is(resolution.CommandError, context.DeadlineExceeded) {
			internalCause = exitInternalCause(input.ExitCode)
		}
		signals = append(signals, competingSignal{
			Tier:       signalTierCancelTimeout,
			Recognized: true,
			Result: ExitFailureResult{
				Reason:  workerexecution.WorkFailureTypeTimeout,
				Message: "Codex execution timed out.",
			},
			InternalCause: internalCause,
		})
	}
	if failure, internalCause, ok := parseStructuredStreamFailureWithCause(input.Stdout); ok {
		signals = append(signals, competingSignal{
			Tier:          signalTierStructured,
			Recognized:    isRecognizedFailureType(failure.Reason),
			Result:        failure,
			InternalCause: internalCause,
		})
	}
	structured, structuredCause, hasStructured, stderr, stderrCause, hasStderr, exit := processExitFailureLayersWithCause(input)
	if hasStructured {
		signals = append(signals, competingSignal{
			Tier:          signalTierStructured,
			Recognized:    isRecognizedFailureType(structured.Reason),
			Result:        structured,
			InternalCause: structuredCause,
		})
	}
	if hasStderr {
		signals = append(signals, competingSignal{
			Tier:          signalTierStderr,
			Recognized:    true,
			Result:        stderr,
			InternalCause: stderrCause,
		})
	}
	signals = append(signals, competingSignal{
		Tier:          signalTierExit,
		Recognized:    isRecognizedFailureType(exit.Reason),
		Result:        exit,
		InternalCause: exitInternalCause(input.ExitCode),
	})
	return signals
}

func selectFailureByPrecedence(signals []competingSignal) (FailureResolution, bool) {
	if len(signals) == 0 {
		return FailureResolution{}, false
	}
	if selected, ok := selectFailureForTier(signals, signalTierCancelTimeout, false); ok {
		return selected, true
	}
	if selected, ok := selectFailureForTier(signals, signalTierStructured, true); ok {
		return selected, true
	}
	if selected, ok := selectFailureForTier(signals, signalTierStderr, true); ok {
		return selected, true
	}
	if selected, ok := selectFailureForTier(signals, signalTierStructured, false); ok {
		return selected, true
	}
	if selected, ok := selectFailureForTier(signals, signalTierStderr, false); ok {
		return selected, true
	}
	return selectFailureForTier(signals, signalTierExit, false)
}

func selectFailureForTier(signals []competingSignal, tier failureSignalTier, recognizedOnly bool) (FailureResolution, bool) {
	var selected FailureResolution
	var found bool
	for _, signal := range signals {
		if signal.Tier != tier {
			continue
		}
		if recognizedOnly && !signal.Recognized {
			continue
		}
		selected = FailureResolution{
			Result:        signal.Result,
			InternalCause: signal.InternalCause,
		}
		found = true
	}
	return selected, found
}

// StructuredStreamReportingOutcome classifies terminal JSONL stdout without
// applying process-exit stderr or exit-status fallback.
func StructuredStreamReportingOutcome(stdout []byte) (ExitFailureResult, bool) {
	failure, _, ok := parseStructuredStreamFailureWithCause(stdout)
	return failure, ok
}

// ProcessExitReportingOutcome classifies stderr and exit status without
// structured-stream JSONL signals.
func ProcessExitReportingOutcome(input ExitFailureInput) ExitFailureResult {
	isolated := input
	isolated.Stdout = nil
	return ParseFailureLayers(isolated)
}

// ExitInternalCause returns a bounded internal-cause label for a process exit code.
func ExitInternalCause(exitCode int) string {
	return exitInternalCause(exitCode)
}

func exitInternalCause(exitCode int) string {
	return fmt.Sprintf("exit code %d", exitCode)
}

func processExitFailureLayersWithCause(input ExitFailureInput) (
	structured ExitFailureResult,
	structuredCause string,
	hasStructured bool,
	stderr ExitFailureResult,
	stderrCause string,
	hasStderr bool,
	exit ExitFailureResult,
) {
	structured, hasStructured, stderr, hasStderr, exit = ProcessExitFailureLayers(input)
	if hasStructured {
		structuredCause = processExitStructuredInternalCause(input)
	}
	if hasStderr {
		stderrCause = processExitStderrInternalCause(input)
	}
	return structured, structuredCause, hasStructured, stderr, stderrCause, hasStderr, exit
}

func parseStructuredStreamFailureWithCause(stdout []byte) (ExitFailureResult, string, bool) {
	var (
		selected           ExitFailureResult
		selectedCause      string
		found              bool
		selectedRecognized bool
	)
	forEachJSONLRecord(stdout, func(raw []byte) {
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
		failure, recognized := structuredStreamFailureFromMessage(nativeMessage)
		if shouldSelectStructuredStreamFailure(selectedRecognized, recognized) {
			selected = failure
			selectedCause = structuredStreamNativeInternalCause(nativeMessage, recognized)
			found = true
		}
		selectedRecognized = selectedRecognized || recognized
	})
	return selected, selectedCause, found
}

func shouldSelectStructuredStreamFailure(selectedRecognized, candidateRecognized bool) bool {
	return candidateRecognized || !selectedRecognized
}

func structuredStreamFailureFromMessage(message string) (ExitFailureResult, bool) {
	parsed := ParseFailureLayers(ExitFailureInput{
		ExitCode: 1,
		Stderr:   []byte("ERROR: " + strings.TrimSpace(message)),
	})
	if !isRecognizedFailureType(parsed.Reason) {
		return ExitFailureResult{
			Reason:  workerexecution.WorkFailureTypeUnknown,
			Message: UnknownFailureMessage,
		}, false
	}
	return parsed, true
}

func forEachJSONLRecord(output []byte, visit func([]byte)) {
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

func structuredStreamNativeInternalCause(nativeMessage string, recognized bool) string {
	if recognized {
		if cause := recognizedNativeInternalCause(nativeMessage); cause != "" {
			return cause
		}
	}
	return internalCauseUnrecognizedStructured
}

func recognizedNativeInternalCause(message string) string {
	message = strings.TrimSpace(message)
	if message == "" {
		return ""
	}
	reason := classifyRecognizedCodexTextFailure(strings.ToLower(message), 1)
	if reason == workerexecution.WorkFailureTypeUnknown {
		return ""
	}
	return boundedInternalCause(message)
}

func boundedInternalCause(value string) string {
	value = strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	if value == "" {
		return ""
	}
	if len(value) <= failureMessageBytes {
		return value
	}
	end := 0
	for index := range value {
		if index > failureMessageBytes {
			break
		}
		end = index
	}
	return strings.TrimSpace(value[:end])
}

func structuredJSONInternalCause(payload string) string {
	failure, ok := decodeCodexStructuredFailure(payload)
	if !ok {
		return ""
	}
	reason, recognized := classifyCodexStructuredSignal(failure.Type, failure.Status)
	if !recognized {
		return ""
	}
	if reason == workerexecution.WorkFailureTypePermanentBadRequest && failure.Message == GPT56SolUpgradeMessage {
		return boundedInternalCause(failure.Message)
	}
	parts := make([]string, 0, 3)
	if failure.Type != "" {
		parts = append(parts, failure.Type)
	}
	if failure.Status != 0 {
		parts = append(parts, fmt.Sprintf("status %d", failure.Status))
	}
	if native := recognizedNativeInternalCause(failure.Message); native != "" {
		parts = append(parts, native)
	}
	return boundedInternalCause(strings.Join(parts, ", "))
}

func processExitStructuredInternalCause(input ExitFailureInput) string {
	streams := []string{
		tailForErrorScan(input.Stderr),
		tailForErrorScan(input.Stdout),
	}
	var last string
	for _, stream := range streams {
		for _, line := range strings.Split(stream, "\n") {
			payload, ok := codexErrorPayload(line)
			if !ok || !strings.HasPrefix(payload, "{") {
				continue
			}
			if cause := structuredJSONInternalCause(payload); cause != "" {
				last = cause
			}
		}
	}
	return last
}

func processExitStderrInternalCause(input ExitFailureInput) string {
	streams := []string{
		tailForErrorScan(input.Stderr),
		tailForErrorScan(input.Stdout),
	}
	var last string
	for _, stream := range streams {
		for _, line := range strings.Split(stream, "\n") {
			payload, ok := codexErrorPayload(line)
			if !ok || strings.HasPrefix(payload, "{") {
				continue
			}
			if cause := recognizedNativeInternalCause(payload); cause != "" {
				last = cause
			}
		}
	}
	return last
}

// SanitizedFailureFixture is the safe public projection used by Codex alignment tests.
type SanitizedFailureFixture struct {
	Type          workerexecution.WorkFailureType   `json:"type"`
	Family        workerexecution.WorkFailureFamily `json:"family"`
	Message       string                            `json:"message"`
	Retryable     bool                              `json:"retryable"`
	Terminal      bool                              `json:"terminal"`
	InternalCause string                            `json:"internal_cause,omitempty"`
}
