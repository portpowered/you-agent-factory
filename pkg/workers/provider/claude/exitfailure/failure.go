package exitfailure

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"
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

const ThrottleFailureMessage = "Claude is temporarily unavailable due to rate or capacity limits."
const TimeoutFailureMessage = "Claude request timed out."

var sensitiveCommandEnvNameFragments = []string{
	"TOKEN", "SECRET", "PASSWORD", "PASS", "KEY", "CREDENTIAL", "CREDENTIALS", "AUTH",
	"ANTHROPIC", "OPENAI", "GEMINI", "GOOGLE_APPLICATION_CREDENTIALS",
	"AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY",
}

func isRedactedCommandEnvKey(name string) bool {
	normalized := strings.ToUpper(strings.TrimSpace(name))
	if normalized == "" {
		return false
	}
	for _, fragment := range sensitiveCommandEnvNameFragments {
		if strings.Contains(normalized, fragment) {
			return true
		}
	}
	return false
}

func ParseProviderFailure(input FailureInput) FailureResult {
	return parseProviderFailure(input)
}

const (
	claudeFailureMessageBytes = 1024
	claudeFailureScanBytes    = 64 * 1024
)

const (
	claudeAuthFailureMessage       = "Claude authentication failed."
	claudeBadRequestFailureMessage = "Claude rejected the request as invalid."
	claudeConfigFailureMessage     = "Claude is not configured correctly."
	claudeServerFailureMessage     = "Claude encountered a temporary server error."
	claudeThrottleFailureMessage   = "Claude is temporarily unavailable due to rate or capacity limits."
	claudeTimeoutFailureMessage    = "Claude request timed out."
)

type claudeStructuredFailure struct {
	Type    string
	Status  int
	Message string
}

// ParseClaudeProviderFailure parses Claude CLI API Error records before any
// text fallback and returns the reason and customer-visible message together.
// Structured scanning covers the complete captured streams while bounding each
// candidate record; text fallback and actionable messages are separately
// bounded so a failed command cannot publish its transcript.
func parseProviderFailure(input FailureInput) FailureResult {
	result := input
	structuredStreams := [][]byte{result.Stderr, result.Stdout}
	if failure, ok := lastClaudeStructuredFailure(structuredStreams); ok {
		return failure
	}

	streams := []string{
		tailForClaudeFailureScan(result.Stderr),
		tailForClaudeFailureScan(result.Stdout),
	}
	if failure, ok := lastClaudeTextFailure(streams); ok {
		return failure
	}
	if result.ExitCode == 124 {
		return FailureResult{
			Reason:  workerexecution.WorkFailureTypeTimeout,
			Message: claudeTimeoutFailureMessage,
		}
	}
	return FailureResult{
		Reason:  workerexecution.WorkFailureTypeUnknown,
		Message: claudeUnknownFailureMessage(streams, result.ExitCode),
	}
}

// Streams are ordered stderr then stdout, and lines retain their stream order.
// The final recognized record wins; malformed and unrelated later lines do not
// displace it. This matches structured-record selection above.
func lastClaudeStructuredFailure(streams [][]byte) (FailureResult, bool) {
	var last FailureResult
	var found bool
	for _, stream := range streams {
		for len(stream) > 0 {
			line := stream
			if newline := bytes.IndexByte(stream, '\n'); newline >= 0 {
				line = stream[:newline]
				stream = stream[newline+1:]
			} else {
				stream = nil
			}
			if len(line) > claudeFailureScanBytes {
				continue
			}
			failure, ok := decodeClaudeAPIError(string(line))
			if !ok {
				continue
			}
			reason, recognized := classifyClaudeStructuredFailure(failure.Type, failure.Status)
			if !recognized {
				continue
			}
			last = FailureResult{
				Reason:  reason,
				Message: claudeStructuredFailureMessage(failure.Message, reason),
			}
			found = true
		}
	}
	return last, found
}

func decodeClaudeAPIError(line string) (claudeStructuredFailure, bool) {
	const prefix = "API Error:"
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, prefix) {
		return claudeStructuredFailure{}, false
	}
	remainder := strings.TrimSpace(strings.TrimPrefix(trimmed, prefix))
	statusText, payload, ok := strings.Cut(remainder, " ")
	if !ok {
		return claudeStructuredFailure{}, false
	}
	status, err := strconv.Atoi(statusText)
	if err != nil || !strings.HasPrefix(strings.TrimSpace(payload), "{") {
		return claudeStructuredFailure{}, false
	}

	var envelope struct {
		Type    string `json:"type"`
		Message string `json:"message"`
		Error   *struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(payload)), &envelope); err != nil {
		return claudeStructuredFailure{}, false
	}

	failure := claudeStructuredFailure{Type: envelope.Type, Status: status, Message: envelope.Message}
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

func classifyClaudeStructuredFailure(errorType string, status int) (workerexecution.WorkFailureType, bool) {
	switch strings.ToLower(strings.TrimSpace(errorType)) {
	case "authentication_error", "permission_error":
		return workerexecution.WorkFailureTypeAuthFailure, true
	case "invalid_request_error":
		return workerexecution.WorkFailureTypePermanentBadRequest, true
	case "rate_limit_error", "overloaded_error":
		return workerexecution.WorkFailureTypeThrottled, true
	case "api_error", "server_error":
		return workerexecution.WorkFailureTypeInternalServerError, true
	}

	switch {
	case status == 401 || status == 403:
		return workerexecution.WorkFailureTypeAuthFailure, true
	case status == 400 || status == 413 || status == 422:
		return workerexecution.WorkFailureTypePermanentBadRequest, true
	case status == 429 || status == 529:
		return workerexecution.WorkFailureTypeThrottled, true
	case status >= 500 && status <= 599:
		return workerexecution.WorkFailureTypeInternalServerError, true
	default:
		return workerexecution.WorkFailureTypeUnknown, false
	}
}

func claudeStructuredFailureMessage(message string, reason workerexecution.WorkFailureType) string {
	if reason == workerexecution.WorkFailureTypeAuthFailure || reason == workerexecution.WorkFailureTypePermanentBadRequest {
		if normalized, ok := safeClaudeActionableMessage(message); ok {
			return normalized
		}
	}
	return claudeFallbackFailureMessage(reason, 0)
}

func safeClaudeActionableMessage(message string) (string, bool) {
	normalized := normalizeClaudeMessage(message)
	lower := strings.ToLower(normalized)
	if normalized == "" ||
		strings.HasPrefix(lower, "api error:") ||
		containsClaudeCredentialSignal(lower) ||
		containsClaudeTranscriptMarker(lower) {
		return "", false
	}
	return boundUTF8Bytes(normalized, claudeFailureMessageBytes), true
}

func containsClaudeCredentialSignal(message string) bool {
	if containsAny(message,
		"authorization:",
		"bearer ",
		"api_key=",
		"api-key:",
		"api key:",
		"sk-ant-",
	) {
		return true
	}
	if containsClaudeCredentialWord(message) || containsClaudeCredentialFieldValue(message) || containsClaudeSensitiveIdentifier(message) {
		return true
	}

	// Keep assignment and header detection aligned with the environment
	// diagnostic policy so supported sensitive names cannot be published just
	// because their exact spelling was absent from the literals above.
	for remainder := message; ; {
		separator := strings.IndexAny(remainder, "=:")
		if separator < 0 {
			break
		}
		prefix := strings.TrimSpace(remainder[:separator])
		if boundary := strings.LastIndexAny(prefix, " \t\r\n"); boundary >= 0 {
			prefix = prefix[boundary+1:]
		}
		name := strings.Trim(prefix, "\"'{}[](),")
		if isRedactedCommandEnvKey(name) {
			return true
		}
		remainder = remainder[separator+1:]
	}
	return false
}

func containsClaudeCredentialFieldValue(message string) bool {
	fields := strings.Fields(message)
	for index, field := range fields {
		normalized := strings.Trim(field, "\"'{}[](),.;:!?=")
		if isClaudeCredentialField(normalized) && index+1 < len(fields) {
			return true
		}
		if normalized == "api" && index+2 < len(fields) &&
			strings.Trim(fields[index+1], "\"'{}[](),.;:!?=") == "key" {
			return true
		}
		if normalized == "auth" && index+2 < len(fields) &&
			strings.Trim(fields[index+1], "\"'{}[](),.;:!?=") == "token" {
			return true
		}
	}
	return false
}

func isClaudeCredentialField(field string) bool {
	if field == "authorization" {
		return true
	}
	identifier := strings.ReplaceAll(field, "-", "_")
	return containsClaudeSensitiveIdentifierPart(identifier) &&
		isRedactedCommandEnvKey(identifier)
}

func containsClaudeCredentialWord(message string) bool {
	for _, field := range strings.Fields(message) {
		switch strings.Trim(field, "\"'{}[](),.;:!?=") {
		case "credential", "credentials", "password", "secret", "token":
			return true
		}
	}
	return false
}

func containsClaudeSensitiveIdentifier(message string) bool {
	for _, field := range strings.Fields(message) {
		identifier := strings.Trim(field, "\"'{}[](),.;:!?=")
		if strings.Contains(identifier, "_") &&
			containsClaudeSensitiveIdentifierPart(identifier) &&
			isRedactedCommandEnvKey(identifier) {
			return true
		}
	}
	return false
}

func containsClaudeSensitiveIdentifierPart(identifier string) bool {
	for _, part := range strings.Split(identifier, "_") {
		switch part {
		case "auth", "credential", "credentials", "key", "pass", "password", "secret", "token":
			return true
		}
	}
	return false
}

func lastClaudeTextFailure(streams []string) (FailureResult, bool) {
	var last FailureResult
	var found bool
	for _, stream := range streams {
		for _, line := range strings.Split(stream, "\n") {
			normalized := normalizeClaudeMessage(line)
			if !isClaudeTextDiagnosticRecord(normalized) {
				continue
			}
			reason, ok := claudeTextFailureReason(strings.ToLower(normalized))
			if !ok {
				continue
			}
			last = FailureResult{
				Reason:  reason,
				Message: claudeTextFailureMessage(normalized, reason),
			}
			found = true
		}
	}
	return last, found
}

// Text fallback accepts only known Claude diagnostic shapes. Transcript and
// prompt markers are rejected wherever they appear so their content cannot
// influence failure policy or become a customer-visible message.
func isClaudeTextDiagnosticRecord(message string) bool {
	lower := strings.ToLower(message)
	if lower == "" || containsClaudeTranscriptMarker(lower) {
		return false
	}
	return hasAnyPrefix(lower,
		"api error:",
		"error:",
		"claude error:",
		"request failed:",
		"configuration error",
		"configuration is invalid",
		"invalid configuration",
		"config file not found",
		"anthropic_api_key is not set",
		"model is not configured",
		"api key",
		"authentication error",
		"permission error",
		"not logged in",
		"login required",
		"unauthorized",
		"forbidden",
		"invalid_request_error",
		"bad request",
		"invalid request",
		"request_too_large",
		"rate limit",
		"too many requests",
		"overloaded",
		"529",
		"internal server error",
		"unexpected status 500",
		"unexpected status 502",
		"unexpected status 503",
		"unexpected status 504",
		"request deadline exceeded",
		"request timed out",
		"request timeout",
		"deadline exceeded",
		"timed out",
		"timeout",
	)
}

func containsClaudeTranscriptMarker(message string) bool {
	return containsAny(message, "user:", "human:", "assistant:", "system:", "prompt:")
}

func claudeTextFailureReason(message string) (workerexecution.WorkFailureType, bool) {
	if strings.HasPrefix(message, "cleanup ") ||
		strings.HasPrefix(message, "cleaning up ") ||
		strings.HasPrefix(message, "teardown ") {
		return workerexecution.WorkFailureTypeUnknown, false
	}
	switch {
	case containsAny(message, "configuration error", "configuration is invalid", "invalid configuration", "config file not found", "anthropic_api_key is not set", "model is not configured"):
		return workerexecution.WorkFailureTypeMisconfigured, true
	case containsAny(message, `"type":"authentication_error"`, `"type":"permission_error"`, "api key", "authentication error", "permission error", "not logged in", "login required", "unauthorized", "forbidden"):
		return workerexecution.WorkFailureTypeAuthFailure, true
	case containsAny(message, `"type":"invalid_request_error"`, "invalid_request_error", "bad request", "invalid request", "request_too_large"):
		return workerexecution.WorkFailureTypePermanentBadRequest, true
	case containsAny(message, `"type":"rate_limit_error"`, `"type":"overloaded_error"`, "rate limit", "too many requests", "overloaded", "529"):
		return workerexecution.WorkFailureTypeThrottled, true
	case containsAny(message, `"type":"api_error"`, "internal server error", "unexpected status 500", "unexpected status 502", "unexpected status 503", "unexpected status 504"):
		return workerexecution.WorkFailureTypeInternalServerError, true
	case containsAny(message, "deadline exceeded", "timed out", "timeout"):
		return workerexecution.WorkFailureTypeTimeout, true
	default:
		return workerexecution.WorkFailureTypeUnknown, false
	}
}

func claudeTextFailureMessage(message string, reason workerexecution.WorkFailureType) string {
	if reason == workerexecution.WorkFailureTypeAuthFailure ||
		reason == workerexecution.WorkFailureTypePermanentBadRequest ||
		reason == workerexecution.WorkFailureTypeMisconfigured {
		if actionable, ok := safeClaudeActionableMessage(message); ok {
			return actionable
		}
	}
	return claudeFallbackFailureMessage(reason, 0)
}

func claudeFallbackFailureMessage(reason workerexecution.WorkFailureType, exitCode int) string {
	switch reason {
	case workerexecution.WorkFailureTypeAuthFailure:
		return claudeAuthFailureMessage
	case workerexecution.WorkFailureTypePermanentBadRequest:
		return claudeBadRequestFailureMessage
	case workerexecution.WorkFailureTypeMisconfigured:
		return claudeConfigFailureMessage
	case workerexecution.WorkFailureTypeThrottled:
		return claudeThrottleFailureMessage
	case workerexecution.WorkFailureTypeInternalServerError:
		return claudeServerFailureMessage
	case workerexecution.WorkFailureTypeTimeout:
		return claudeTimeoutFailureMessage
	default:
		return fmt.Sprintf("claude exited with code %d", exitCode)
	}
}

func claudeUnknownFailureMessage(streams []string, exitCode int) string {
	var candidate string
	lineCount := 0
	for _, stream := range streams {
		for _, line := range strings.Split(stream, "\n") {
			normalized := normalizeClaudeMessage(line)
			if normalized == "" {
				continue
			}
			lineCount++
			candidate = normalized
		}
	}
	if lineCount == 1 && safeClaudeUnknownExcerpt(candidate) {
		return boundUTF8Bytes("Claude failed: "+candidate, claudeFailureMessageBytes)
	}
	return fmt.Sprintf("claude exited with code %d", exitCode)
}

func safeClaudeUnknownExcerpt(message string) bool {
	lower := strings.ToLower(message)
	return containsAny(lower, "error", "fail") &&
		!containsClaudeCredentialSignal(lower) &&
		!containsClaudeTranscriptMarker(lower) &&
		!strings.HasPrefix(lower, "api error:") &&
		!strings.ContainsAny(message, "{}")
}

func hasAnyPrefix(value string, prefixes ...string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(value, prefix) {
			return true
		}
	}
	return false
}

func normalizeClaudeMessage(message string) string {
	return strings.Join(strings.Fields(strings.ToValidUTF8(message, "")), " ")
}

func tailForClaudeFailureScan(output []byte) string {
	if len(output) <= claudeFailureScanBytes {
		return string(output)
	}
	return string(output[len(output)-claudeFailureScanBytes:])
}

func boundUTF8Bytes(message string, limit int) string {
	if limit <= 0 || len(message) <= limit {
		return message
	}
	bounded := []byte(message)[:limit]
	for !utf8.Valid(bounded) {
		bounded = bounded[:len(bounded)-1]
	}
	return strings.TrimSpace(string(bounded))

}

func containsAny(haystack string, needles ...string) bool {
	for _, needle := range needles {
		if needle != "" && strings.Contains(haystack, needle) {
			return true
		}
	}
	return false
}
