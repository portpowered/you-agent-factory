package kiro

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode"

	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"
)

const (
	errorLineScanBytes  = 64 * 1024
	failureMessageBytes = 1024
)

const TimeoutFailureMessage = "Kiro request timed out."

const (
	kiroAuthFailureMessage       = "Kiro authentication failed. Sign in again and retry."
	kiroBadRequestFailureMessage = "Kiro rejected the request as invalid."
	kiroThrottleFailureMessage   = "Kiro is temporarily unavailable due to usage or capacity limits."
	kiroTimeoutFailureMessage    = "Kiro request timed out."
	kiroServerFailureMessage     = "Kiro encountered a temporary service error."
)

type FailureInput struct {
	Stdout   []byte
	Stderr   []byte
	ExitCode int
}

type FailureResult struct {
	Reason  workerexecution.WorkFailureType
	Message string
}

func ParseProviderFailure(input FailureInput) FailureResult {
	return parseProviderFailure(input)
}

func tailForErrorScan(output []byte) string {
	if len(output) <= errorLineScanBytes {
		return string(output)
	}
	return string(output[len(output)-errorLineScanBytes:])
}

// ParseKiroProviderFailure is the pure Kiro-owned normalization boundary for
// non-zero CLI exits. It inspects bounded stderr/stdout tails, gives recognized
// structured records precedence over text, and returns only canonical reasons
// with product-owned messages for known failures.
func parseProviderFailure(input FailureInput) FailureResult {
	result := input
	if result.ExitCode == 124 {
		return knownKiroFailure(workerexecution.WorkFailureTypeTimeout)
	}
	streams := []string{
		tailForErrorScan(result.Stderr),
		tailForErrorScan(result.Stdout),
	}
	if failure, ok := firstKiroStructuredFailure(streams); ok {
		return failure
	}
	if failure, ok := firstKiroTextFailure(streams, result.ExitCode); ok {
		return failure
	}
	if message, ok := firstKiroUnknownFailureExcerpt(streams); ok {
		return FailureResult{
			Reason:  workerexecution.WorkFailureTypeUnknown,
			Message: message,
		}
	}
	return FailureResult{
		Reason:  workerexecution.WorkFailureTypeUnknown,
		Message: kiroExitFailureMessage(result.ExitCode),
	}
}

func firstKiroStructuredFailure(streams []string) (FailureResult, bool) {
	for _, stream := range streams {
		for _, line := range strings.Split(stream, "\n") {
			payload := kiroStructuredPayload(line)
			if payload == "" {
				continue
			}
			reason, ok := decodeKiroStructuredFailure(payload)
			if ok {
				return knownKiroFailure(reason), true
			}
		}
	}
	return FailureResult{}, false
}

func kiroStructuredPayload(line string) string {
	trimmed := strings.TrimSpace(line)
	for _, prefix := range []string{"ERROR:", "Error:", "KIRO_ERROR:"} {
		trimmed = strings.TrimSpace(strings.TrimPrefix(trimmed, prefix))
	}
	if !strings.HasPrefix(trimmed, "{") {
		return ""
	}
	return trimmed
}

func decodeKiroStructuredFailure(payload string) (workerexecution.WorkFailureType, bool) {
	var envelope map[string]any
	if err := json.Unmarshal([]byte(payload), &envelope); err != nil {
		return workerexecution.WorkFailureTypeUnknown, false
	}

	signals := kiroStructuredSignals(envelope)
	if nested, ok := envelope["error"].(map[string]any); ok {
		signals = append(kiroStructuredSignals(nested), signals...)
	}
	for _, signal := range signals {
		if reason := classifyKiroSignal(signal); reason != workerexecution.WorkFailureTypeUnknown {
			return reason, true
		}
	}
	for _, status := range kiroStructuredStatuses(envelope) {
		if reason := classifyKiroStatus(status); reason != workerexecution.WorkFailureTypeUnknown {
			return reason, true
		}
	}
	if nested, ok := envelope["error"].(map[string]any); ok {
		for _, status := range kiroStructuredStatuses(nested) {
			if reason := classifyKiroStatus(status); reason != workerexecution.WorkFailureTypeUnknown {
				return reason, true
			}
		}
	}
	return workerexecution.WorkFailureTypeUnknown, false
}

func kiroStructuredSignals(record map[string]any) []string {
	keys := []string{"type", "code", "error_type", "errorType", "name"}
	signals := make([]string, 0, len(keys))
	for _, key := range keys {
		if value, ok := record[key].(string); ok {
			signals = append(signals, value)
		}
	}
	return signals
}

func kiroStructuredStatuses(record map[string]any) []int {
	keys := []string{"status", "status_code", "statusCode"}
	statuses := make([]int, 0, len(keys))
	for _, key := range keys {
		switch value := record[key].(type) {
		case float64:
			statuses = append(statuses, int(value))
		case string:
			var status int
			if _, err := fmt.Sscanf(value, "%d", &status); err == nil {
				statuses = append(statuses, status)
			}
		}
	}
	return statuses
}

func classifyKiroSignal(signal string) workerexecution.WorkFailureType {
	normalized := strings.ToLower(strings.TrimSpace(signal))
	switch {
	case containsAny(normalized, "authentication", "authorization", "unauthorized", "forbidden", "access_denied", "accessdenied"):
		return workerexecution.WorkFailureTypeAuthFailure
	case containsAny(normalized, "invalid_request", "invalidrequest", "validation", "bad_request", "badrequest", "invalid_argument"):
		return workerexecution.WorkFailureTypePermanentBadRequest
	case containsAny(normalized, "rate_limit", "ratelimit", "throttl", "too_many_requests", "capacity", "overloaded"):
		return workerexecution.WorkFailureTypeThrottled
	case containsAny(normalized, "timeout", "timed_out", "deadline_exceeded"):
		return workerexecution.WorkFailureTypeTimeout
	case containsAny(normalized, "internal_server", "internalserver", "server_error", "service_unavailable", "serviceunavailable", "api_error"):
		return workerexecution.WorkFailureTypeInternalServerError
	default:
		return workerexecution.WorkFailureTypeUnknown
	}
}

func classifyKiroStatus(status int) workerexecution.WorkFailureType {
	switch {
	case status == 401 || status == 403:
		return workerexecution.WorkFailureTypeAuthFailure
	case status == 400 || status == 422:
		return workerexecution.WorkFailureTypePermanentBadRequest
	case status == 429:
		return workerexecution.WorkFailureTypeThrottled
	case status == 408 || status == 504:
		return workerexecution.WorkFailureTypeTimeout
	case status >= 500 && status <= 599:
		return workerexecution.WorkFailureTypeInternalServerError
	default:
		return workerexecution.WorkFailureTypeUnknown
	}
}

func firstKiroTextFailure(streams []string, exitCode int) (FailureResult, bool) {
	for _, stream := range streams {
		lines := strings.Split(stream, "\n")
		for _, line := range lines {
			message, ok := kiroTextErrorCandidate(line, len(lines) == 1)
			if !ok {
				continue
			}
			if reason := classifyKiroTextFailure(message, exitCode); reason != workerexecution.WorkFailureTypeUnknown {
				return knownKiroFailure(reason), true
			}
		}
	}
	return FailureResult{}, false
}

func kiroTextErrorCandidate(line string, singleLine bool) (string, bool) {
	trimmed := normalizeKiroText(line)
	if trimmed == "" {
		return "", false
	}
	lower := strings.ToLower(trimmed)
	if singleLine || strings.HasPrefix(lower, "error:") || strings.HasPrefix(lower, "kiro error:") || strings.HasPrefix(lower, "api error:") {
		return trimmed, true
	}
	return "", false
}

func normalizeKiroText(value string) string {
	value = strings.ToValidUTF8(value, " ")
	value = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, value)
	return strings.Join(strings.Fields(value), " ")
}

// firstKiroUnknownFailureExcerpt considers stderr before stdout and selects the
// first explicit error record in that stream. This precedence is intentional:
// Kiro writes invocation failures to stderr, while stdout can contain model
// output or an echoed prompt. Conservative rejection keeps ambiguous records
// on the fixed exit-code fallback instead of risking customer-data exposure.
func firstKiroUnknownFailureExcerpt(streams []string) (string, bool) {
	for _, stream := range streams {
		for _, line := range strings.Split(stream, "\n") {
			detail, ok := kiroUnknownErrorDetail(line)
			if !ok {
				continue
			}
			message := truncateKiroFailureMessage("Kiro error: " + detail)
			if message != "Kiro error:" {
				return message, true
			}
		}
	}
	return "", false
}

func kiroUnknownErrorDetail(line string) (string, bool) {
	normalized := normalizeKiroText(line)
	lower := strings.ToLower(normalized)
	prefixes := []string{"kiro_error:", "kiro error:", "api error:", "error:"}
	for _, prefix := range prefixes {
		if !strings.HasPrefix(lower, prefix) {
			continue
		}
		detail := strings.TrimSpace(normalized[len(prefix):])
		if detail == "" || unsafeKiroUnknownDetail(detail) {
			return "", false
		}
		return detail, true
	}
	return "", false
}

func unsafeKiroUnknownDetail(detail string) bool {
	lower := strings.ToLower(detail)
	if strings.HasPrefix(detail, "{") || strings.HasPrefix(detail, "[") || strings.Contains(detail, "=") {
		return true
	}
	return containsAny(lower,
		"prompt", "transcript", "response draft", "model output", "user message",
		"request body", "request payload", "input content", "output content",
		"progress", "cleanup", "credential", "secret", "password",
		"api key", "api_key", "access key", "authorization", "bearer ",
		"access token", "refresh token", "session token", "environment", "env var",
	)
}

func truncateKiroFailureMessage(message string) string {
	if len(message) <= failureMessageBytes {
		return message
	}
	end := 0
	for index := range message {
		if index > failureMessageBytes {
			break
		}
		end = index
	}
	return strings.TrimSpace(message[:end])
}

func classifyKiroTextFailure(message string, exitCode int) workerexecution.WorkFailureType {
	normalized := strings.ToLower(message)
	switch {
	case exitCode == 124, containsAny(normalized, "request timed out", "operation timed out", "timeout waiting", "deadline exceeded"):
		return workerexecution.WorkFailureTypeTimeout
	case containsAny(normalized, "authentication required", "authentication failed", "authorization failed", "not authorized", "unauthorized", "forbidden", "sign in required", "login required"):
		return workerexecution.WorkFailureTypeAuthFailure
	case containsAny(normalized, "invalid request", "invalid input", "invalid argument", "bad request", "validation failed"):
		return workerexecution.WorkFailureTypePermanentBadRequest
	case containsAny(normalized, "rate limit", "too many requests", "throttl", "capacity limit", "at capacity", "resource exhausted"):
		return workerexecution.WorkFailureTypeThrottled
	case containsAny(normalized, "internal server error", "temporary service error", "service unavailable", "unexpected status 500", "unexpected status 502", "unexpected status 503"):
		return workerexecution.WorkFailureTypeInternalServerError
	default:
		return workerexecution.WorkFailureTypeUnknown
	}
}

func knownKiroFailure(reason workerexecution.WorkFailureType) FailureResult {
	message := ""
	switch reason {
	case workerexecution.WorkFailureTypeAuthFailure:
		message = kiroAuthFailureMessage
	case workerexecution.WorkFailureTypePermanentBadRequest:
		message = kiroBadRequestFailureMessage
	case workerexecution.WorkFailureTypeThrottled:
		message = kiroThrottleFailureMessage
	case workerexecution.WorkFailureTypeTimeout:
		message = kiroTimeoutFailureMessage
	case workerexecution.WorkFailureTypeInternalServerError:
		message = kiroServerFailureMessage
	}
	if len(message) > failureMessageBytes {
		message = message[:failureMessageBytes]
	}
	return FailureResult{Reason: reason, Message: message}
}

func kiroExitFailureMessage(exitCode int) string {
	return fmt.Sprintf("kiro-cli exited with code %d", exitCode)
}

func containsAny(haystack string, needles ...string) bool {
	for _, needle := range needles {
		if needle != "" && strings.Contains(haystack, needle) {
			return true
		}
	}
	return false
}
