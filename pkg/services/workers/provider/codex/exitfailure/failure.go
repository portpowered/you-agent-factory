package exitfailure

import (
	"encoding/json"
	"fmt"
	"strings"

	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

const (
	errorLineScanBytes              = 64 * 1024
	failureMessageBytes             = 1024
	WindowsProcessFailureExitCode   = 4294967295
	HighDemandTemporaryErrorsNeedle = "we're currently experiencing high demand, which may cause temporary errors."
)

type ExitFailureInput struct {
	Stdout   []byte
	Stderr   []byte
	ExitCode int
}

type ExitFailureResult struct {
	Reason  workerexecution.WorkFailureType
	Message string
}

const AuthFailureMessage = "Codex authentication failed."

const UnknownFailureMessage = "Codex reported a terminal error."

const GPT56SolUpgradeMessage = "The 'gpt-5.6-sol' model requires a newer version of Codex. Please upgrade to the latest app or CLI and try again."

const (
	codexAuthFailureMessage       = "Codex authentication failed."
	codexBadRequestFailureMessage = "Codex rejected the request as invalid."
	codexGPT56SolUpgradeMessage   = "The 'gpt-5.6-sol' model requires a newer version of Codex. Please upgrade to the latest app or CLI and try again."
	codexServerFailureMessage     = "Codex encountered a temporary server error."
	codexThrottleFailureMessage   = "Codex is temporarily unavailable due to usage or capacity limits."
	codexTimeoutFailureMessage    = "Codex request timed out."
)

func ExtractErrorLine(input ExitFailureInput) (string, bool) {
	combined := strings.Join([]string{
		tailForErrorScan(input.Stderr),
		tailForErrorScan(input.Stdout),
	}, "\n")
	var match string
	for _, line := range strings.Split(combined, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "ERROR:") {
			match = trimmed
		}
	}
	if match == "" {
		return "", false
	}
	return match, true
}

func tailForErrorScan(output []byte) string {
	if len(output) <= errorLineScanBytes {
		return string(output)
	}
	return string(output[len(output)-errorLineScanBytes:])
}

type codexStructuredFailure struct {
	Type    string
	Status  int
	Message string
}

// ParseCodexProviderFailure deterministically parses bounded subprocess output
// into the canonical provider-failure contract. Each stream is limited by
// errorLineScanBytes before parsing, and returned messages are limited by
// codexFailureMessageBytes.
func ParseExitFailure(input ExitFailureInput) ExitFailureResult {
	streams := []string{
		tailForErrorScan(input.Stderr),
		tailForErrorScan(input.Stdout),
	}
	if failure, ok := lastCodexStructuredFailure(streams); ok {
		return failure
	}

	if failure, ok := lastCodexTextFailure(streams, input.ExitCode); ok {
		return ExitFailureResult{
			Reason:  failure.Reason,
			Message: failure.Message,
		}
	}

	return ExitFailureResult{
		Reason:  classifyCodexExitFailure(input.ExitCode),
		Message: codexExitFailureMessage(input.ExitCode),
	}
}

func lastCodexStructuredFailure(streams []string) (ExitFailureResult, bool) {
	var last ExitFailureResult
	var found bool
	for _, stream := range streams {
		for _, line := range strings.Split(stream, "\n") {
			payload, ok := codexErrorPayload(line)
			if !ok || !strings.HasPrefix(payload, "{") {
				continue
			}
			failure, ok := decodeCodexStructuredFailure(payload)
			if !ok {
				continue
			}
			reason, recognized := classifyCodexStructuredSignal(failure.Type, failure.Status)
			if recognized {
				last = ExitFailureResult{
					Reason:  reason,
					Message: codexStructuredFailureMessage(failure.Message, reason),
				}
				found = true
			}
		}
	}
	return last, found
}

// codexStructuredFailureMessage publishes only positively audited text. Other
// structured messages can contain prompts, transcripts, cleanup paths, or
// credentials, so recognized reasons use fixed customer-visible messages.
func codexStructuredFailureMessage(message string, reason workerexecution.WorkFailureType) string {
	message = strings.Join(strings.Fields(message), " ")
	if reason == workerexecution.WorkFailureTypePermanentBadRequest && message == codexGPT56SolUpgradeMessage {
		return message
	}
	return codexTextFailureMessage(reason)
}

func codexErrorPayload(line string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "ERROR:") {
		return "", false
	}
	return strings.TrimSpace(strings.TrimPrefix(trimmed, "ERROR:")), true
}

func decodeCodexStructuredFailure(payload string) (codexStructuredFailure, bool) {
	var envelope struct {
		Type    string `json:"type"`
		Status  int    `json:"status"`
		Message string `json:"message"`
		Error   *struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(payload), &envelope); err != nil {
		return codexStructuredFailure{}, false
	}
	if envelope.Error == nil && envelope.Type == "" && envelope.Status == 0 {
		return codexStructuredFailure{}, false
	}

	failure := codexStructuredFailure{
		Type:    envelope.Type,
		Status:  envelope.Status,
		Message: envelope.Message,
	}
	if envelope.Error != nil {
		if envelope.Error.Type != "" {
			failure.Type = envelope.Error.Type
		}
		if envelope.Error.Message != "" {
			failure.Message = envelope.Error.Message
		}
	}
	return failure, true
}

func lastCodexTextFailure(streams []string, exitCode int) (ExitFailureResult, bool) {
	var last ExitFailureResult
	var found bool
	for _, stream := range streams {
		for _, line := range strings.Split(stream, "\n") {
			payload, isError := codexErrorPayload(line)
			if !isError || strings.HasPrefix(payload, "{") {
				continue
			}
			if failure, ok := recognizedCodexTextFailure(payload, exitCode); ok {
				last, found = failure, true
			}
		}
	}
	if found {
		return last, true
	}

	// A single unprefixed line is an existing Codex diagnostic shape (for
	// example transport timeouts). Multi-line output is treated as transcript
	// or header noise unless it contains an explicit ERROR record.
	for _, stream := range streams {
		trimmed := strings.TrimSpace(stream)
		if trimmed == "" || strings.Contains(trimmed, "\n") {
			continue
		}
		if failure, ok := recognizedCodexTextFailure(trimmed, exitCode); ok {
			last, found = failure, true
		}
	}
	return last, found
}

// recognizedCodexTextFailure accepts only audited Codex diagnostic shapes and
// returns fixed customer-visible text. Unknown ERROR lines may contain echoed
// prompts, transcripts, cleanup paths, or credentials, so they are never used
// as message excerpts.
func recognizedCodexTextFailure(message string, exitCode int) (ExitFailureResult, bool) {
	normalized := strings.ToLower(strings.TrimSpace(message))
	reason := classifyRecognizedCodexTextFailure(normalized, exitCode)
	if reason == workerexecution.WorkFailureTypeUnknown {
		return ExitFailureResult{}, false
	}
	return ExitFailureResult{
		Reason:  reason,
		Message: codexTextFailureMessage(reason),
	}, true
}

func classifyRecognizedCodexTextFailure(message string, exitCode int) workerexecution.WorkFailureType {
	switch {
	case exitCode == 124,
		strings.HasPrefix(message, "context deadline exceeded"),
		strings.HasPrefix(message, "command timed out"),
		strings.HasPrefix(message, "request timed out"),
		strings.HasPrefix(message, "provider timeout"),
		strings.HasPrefix(message, "context canceled after command timed out"):
		return workerexecution.WorkFailureTypeTimeout
	case strings.HasPrefix(message, "unexpected status 401"),
		strings.HasPrefix(message, "unexpected status 403"):
		return workerexecution.WorkFailureTypeAuthFailure
	case strings.HasPrefix(message, "unexpected status 400"):
		return workerexecution.WorkFailureTypePermanentBadRequest
	case strings.HasPrefix(message, "unexpected status 429"),
		strings.HasPrefix(message, "you've hit your usage limit"),
		strings.HasPrefix(message, "selected model is at capacity"):
		return workerexecution.WorkFailureTypeThrottled
	case strings.HasPrefix(message, "unexpected status 500"),
		strings.HasPrefix(message, "unexpected status 502"),
		strings.HasPrefix(message, "unexpected status 503"),
		strings.HasPrefix(message, "unexpected status 504"),
		message == HighDemandTemporaryErrorsNeedle:
		return workerexecution.WorkFailureTypeInternalServerError
	default:
		return workerexecution.WorkFailureTypeUnknown
	}
}

func codexTextFailureMessage(reason workerexecution.WorkFailureType) string {
	switch reason {
	case workerexecution.WorkFailureTypeAuthFailure:
		return codexAuthFailureMessage
	case workerexecution.WorkFailureTypePermanentBadRequest:
		return codexBadRequestFailureMessage
	case workerexecution.WorkFailureTypeThrottled:
		return codexThrottleFailureMessage
	case workerexecution.WorkFailureTypeInternalServerError:
		return codexServerFailureMessage
	case workerexecution.WorkFailureTypeTimeout:
		return codexTimeoutFailureMessage
	default:
		return ""
	}
}

func classifyCodexExitFailure(exitCode int) workerexecution.WorkFailureType {
	if exitCode == 124 {
		return workerexecution.WorkFailureTypeTimeout
	}
	if exitCode == WindowsProcessFailureExitCode {
		return workerexecution.WorkFailureTypeInternalServerError
	}
	return workerexecution.WorkFailureTypeUnknown
}

func classifyCodexStructuredSignal(errorType string, status int) (workerexecution.WorkFailureType, bool) {
	normalizedType := strings.ToLower(strings.TrimSpace(errorType))
	switch normalizedType {
	case "authentication_error", "permission_error":
		return workerexecution.WorkFailureTypeAuthFailure, true
	case "invalid_request_error":
		return workerexecution.WorkFailureTypePermanentBadRequest, true
	case "rate_limit_error", "overloaded_error":
		return workerexecution.WorkFailureTypeThrottled, true
	case "api_error", "server_error":
		return workerexecution.WorkFailureTypeInternalServerError, true
	case "", "error":
		// Generic Codex envelopes carry the provider classification in status.
	default:
		// An explicit unknown type can identify cleanup or diagnostic records;
		// its status must not let it override a recognized provider failure.
		return workerexecution.WorkFailureTypeUnknown, false
	}

	switch {
	case status == 401 || status == 403:
		return workerexecution.WorkFailureTypeAuthFailure, true
	case status == 400:
		return workerexecution.WorkFailureTypePermanentBadRequest, true
	case status == 429:
		return workerexecution.WorkFailureTypeThrottled, true
	case status >= 500 && status <= 599:
		return workerexecution.WorkFailureTypeInternalServerError, true
	case status == 408:
		return workerexecution.WorkFailureTypeTimeout, true
	default:
		return workerexecution.WorkFailureTypeUnknown, false
	}
}
func codexExitFailureMessage(exitCode int) string {
	return fmt.Sprintf("codex exited with code %d", exitCode)
}

func processExitFallback(exitCode int) ExitFailureResult {
	reason := classifyCodexExitFailure(exitCode)
	if message := codexTextFailureMessage(reason); message != "" {
		return ExitFailureResult{Reason: reason, Message: message}
	}
	return ExitFailureResult{Reason: reason, Message: UnknownFailureMessage}
}

func isRecognizedFailureType(failureType workerexecution.WorkFailureType) bool {
	return failureType != "" && failureType != workerexecution.WorkFailureTypeUnknown
}

// ProcessExitFailureLayers exposes structured ERROR-record, stderr-classified,
// and generic exit outcomes without applying structured-stream precedence.
func ProcessExitFailureLayers(input ExitFailureInput) (
	structured ExitFailureResult,
	hasStructured bool,
	stderr ExitFailureResult,
	hasStderr bool,
	exit ExitFailureResult,
) {
	streams := []string{
		tailForErrorScan(input.Stderr),
		tailForErrorScan(input.Stdout),
	}
	if failure, ok := lastCodexStructuredFailure(streams); ok {
		structured = failure
		hasStructured = true
	}
	if failure, ok := lastCodexTextFailure(streams, input.ExitCode); ok {
		stderr = failure
		hasStderr = true
	}
	exit = processExitFallback(input.ExitCode)
	return structured, hasStructured, stderr, hasStderr, exit
}

// ParseFailureLayers returns structured or stderr-classified outcomes without cross-signal precedence.
func ParseFailureLayers(input ExitFailureInput) ExitFailureResult {
	structured, hasStructured, stderr, hasStderr, exit := ProcessExitFailureLayers(input)
	if hasStructured {
		return structured
	}
	if hasStderr {
		return stderr
	}
	return exit
}
