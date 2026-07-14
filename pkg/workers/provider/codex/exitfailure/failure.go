package exitfailure

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

const (
	errorLineScanBytes            = 64 * 1024
	failureMessageBytes            = 1024
	WindowsProcessFailureExitCode  = 4294967295
	HighDemandTemporaryErrorsNeedle = "we're currently experiencing high demand, which may cause temporary errors."
)

type ExitFailureInput struct {
	Stdout   []byte
	Stderr   []byte
	ExitCode int
}

type ExitFailureResult struct {
	Reason  interfaces.WorkFailureType
	Message string
}

const AuthFailureMessage = "Codex authentication failed."

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
func codexStructuredFailureMessage(message string, reason interfaces.WorkFailureType) string {
	message = strings.Join(strings.Fields(message), " ")
	if reason == interfaces.WorkFailureTypePermanentBadRequest && message == codexGPT56SolUpgradeMessage {
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
	if reason == interfaces.WorkFailureTypeUnknown {
		return ExitFailureResult{}, false
	}
	return ExitFailureResult{
		Reason:  reason,
		Message: codexTextFailureMessage(reason),
	}, true
}

func classifyRecognizedCodexTextFailure(message string, exitCode int) interfaces.WorkFailureType {
	switch {
	case exitCode == 124,
		strings.HasPrefix(message, "context deadline exceeded"),
		strings.HasPrefix(message, "command timed out"),
		strings.HasPrefix(message, "request timed out"),
		strings.HasPrefix(message, "provider timeout"),
		strings.HasPrefix(message, "context canceled after command timed out"):
		return interfaces.WorkFailureTypeTimeout
	case strings.HasPrefix(message, "unexpected status 401"),
		strings.HasPrefix(message, "unexpected status 403"):
		return interfaces.WorkFailureTypeAuthFailure
	case strings.HasPrefix(message, "unexpected status 400"):
		return interfaces.WorkFailureTypePermanentBadRequest
	case strings.HasPrefix(message, "unexpected status 429"),
		strings.HasPrefix(message, "you've hit your usage limit"),
		strings.HasPrefix(message, "selected model is at capacity"):
		return interfaces.WorkFailureTypeThrottled
	case strings.HasPrefix(message, "unexpected status 500"),
		strings.HasPrefix(message, "unexpected status 502"),
		strings.HasPrefix(message, "unexpected status 503"),
		strings.HasPrefix(message, "unexpected status 504"),
		message == HighDemandTemporaryErrorsNeedle:
		return interfaces.WorkFailureTypeInternalServerError
	default:
		return interfaces.WorkFailureTypeUnknown
	}
}

func codexTextFailureMessage(reason interfaces.WorkFailureType) string {
	switch reason {
	case interfaces.WorkFailureTypeAuthFailure:
		return codexAuthFailureMessage
	case interfaces.WorkFailureTypePermanentBadRequest:
		return codexBadRequestFailureMessage
	case interfaces.WorkFailureTypeThrottled:
		return codexThrottleFailureMessage
	case interfaces.WorkFailureTypeInternalServerError:
		return codexServerFailureMessage
	case interfaces.WorkFailureTypeTimeout:
		return codexTimeoutFailureMessage
	default:
		return ""
	}
}

func classifyCodexExitFailure(exitCode int) interfaces.WorkFailureType {
	if exitCode == 124 {
		return interfaces.WorkFailureTypeTimeout
	}
	if exitCode == WindowsProcessFailureExitCode {
		return interfaces.WorkFailureTypeInternalServerError
	}
	return interfaces.WorkFailureTypeUnknown
}

func classifyCodexStructuredSignal(errorType string, status int) (interfaces.WorkFailureType, bool) {
	normalizedType := strings.ToLower(strings.TrimSpace(errorType))
	switch normalizedType {
	case "authentication_error", "permission_error":
		return interfaces.WorkFailureTypeAuthFailure, true
	case "invalid_request_error":
		return interfaces.WorkFailureTypePermanentBadRequest, true
	case "rate_limit_error", "overloaded_error":
		return interfaces.WorkFailureTypeThrottled, true
	case "api_error", "server_error":
		return interfaces.WorkFailureTypeInternalServerError, true
	case "", "error":
		// Generic Codex envelopes carry the provider classification in status.
	default:
		// An explicit unknown type can identify cleanup or diagnostic records;
		// its status must not let it override a recognized provider failure.
		return interfaces.WorkFailureTypeUnknown, false
	}

	switch {
	case status == 401 || status == 403:
		return interfaces.WorkFailureTypeAuthFailure, true
	case status == 400:
		return interfaces.WorkFailureTypePermanentBadRequest, true
	case status == 429:
		return interfaces.WorkFailureTypeThrottled, true
	case status >= 500 && status <= 599:
		return interfaces.WorkFailureTypeInternalServerError, true
	case status == 408:
		return interfaces.WorkFailureTypeTimeout, true
	default:
		return interfaces.WorkFailureTypeUnknown, false
	}
}
func codexExitFailureMessage(exitCode int) string {
	return fmt.Sprintf("codex exited with code %d", exitCode)
}
