package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

// ProviderError is the shared normalized provider failure contract. Provider
// implementations should return this typed error so executor, pause, and
// customer-messaging logic can make deterministic decisions without parsing raw
// provider output at every call site.
type ProviderError struct {
	Family          interfaces.WorkFailureFamily
	Type            interfaces.WorkFailureType
	Message         string
	ProviderSession *interfaces.ProviderSessionMetadata
	Diagnostics     *interfaces.WorkDiagnostics
	Cause           error
}

// ProviderFailureResult is the pure output of provider failure parsing. It
// deliberately carries only the canonical reason and customer-visible message;
// runtime policy is derived from Reason when the result crosses into execution.
type ProviderFailureResult struct {
	Reason  interfaces.WorkFailureType
	Message string
}

const codexFailureMessageBytes = 1024

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

const (
	codexAuthFailureMessage       = "Codex authentication failed."
	codexBadRequestFailureMessage = "Codex rejected the request as invalid."
	codexGPT56SolUpgradeMessage   = "The 'gpt-5.6-sol' model requires a newer version of Codex. Please upgrade to the latest app or CLI and try again."
	codexServerFailureMessage     = "Codex encountered a temporary server error."
	codexThrottleFailureMessage   = "Codex is temporarily unavailable due to usage or capacity limits."
	codexTimeoutFailureMessage    = "Codex request timed out."
)

type codexStructuredFailure struct {
	Type    string
	Status  int
	Message string
}

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
func ParseClaudeProviderFailure(result CommandResult) ProviderFailureResult {
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
		return ProviderFailureResult{
			Reason:  interfaces.WorkFailureTypeTimeout,
			Message: claudeTimeoutFailureMessage,
		}
	}
	return ProviderFailureResult{
		Reason:  interfaces.WorkFailureTypeUnknown,
		Message: claudeUnknownFailureMessage(streams, result.ExitCode),
	}
}

// Streams are ordered stderr then stdout, and lines retain their stream order.
// The final recognized record wins; malformed and unrelated later lines do not
// displace it. This matches structured-record selection above.
func lastClaudeStructuredFailure(streams [][]byte) (ProviderFailureResult, bool) {
	var last ProviderFailureResult
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
			last = ProviderFailureResult{
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

func classifyClaudeStructuredFailure(errorType string, status int) (interfaces.WorkFailureType, bool) {
	switch strings.ToLower(strings.TrimSpace(errorType)) {
	case "authentication_error", "permission_error":
		return interfaces.WorkFailureTypeAuthFailure, true
	case "invalid_request_error":
		return interfaces.WorkFailureTypePermanentBadRequest, true
	case "rate_limit_error", "overloaded_error":
		return interfaces.WorkFailureTypeThrottled, true
	case "api_error", "server_error":
		return interfaces.WorkFailureTypeInternalServerError, true
	}

	switch {
	case status == 401 || status == 403:
		return interfaces.WorkFailureTypeAuthFailure, true
	case status == 400 || status == 413 || status == 422:
		return interfaces.WorkFailureTypePermanentBadRequest, true
	case status == 429 || status == 529:
		return interfaces.WorkFailureTypeThrottled, true
	case status >= 500 && status <= 599:
		return interfaces.WorkFailureTypeInternalServerError, true
	default:
		return interfaces.WorkFailureTypeUnknown, false
	}
}

func claudeStructuredFailureMessage(message string, reason interfaces.WorkFailureType) string {
	if reason == interfaces.WorkFailureTypeAuthFailure || reason == interfaces.WorkFailureTypePermanentBadRequest {
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
	if containsClaudeCredentialWord(message) || containsClaudeSensitiveIdentifier(message) {
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
		if ClassifyCommandEnvKey(name) == CommandEnvClassificationRedacted {
			return true
		}
		remainder = remainder[separator+1:]
	}
	return false
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
			ClassifyCommandEnvKey(identifier) == CommandEnvClassificationRedacted {
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

func lastClaudeTextFailure(streams []string) (ProviderFailureResult, bool) {
	var last ProviderFailureResult
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
			last = ProviderFailureResult{
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

func claudeTextFailureReason(message string) (interfaces.WorkFailureType, bool) {
	if strings.HasPrefix(message, "cleanup ") ||
		strings.HasPrefix(message, "cleaning up ") ||
		strings.HasPrefix(message, "teardown ") {
		return interfaces.WorkFailureTypeUnknown, false
	}
	switch {
	case containsAny(message, "configuration error", "configuration is invalid", "invalid configuration", "config file not found", "anthropic_api_key is not set", "model is not configured"):
		return interfaces.WorkFailureTypeMisconfigured, true
	case containsAny(message, `"type":"authentication_error"`, `"type":"permission_error"`, "api key", "authentication error", "permission error", "not logged in", "login required", "unauthorized", "forbidden"):
		return interfaces.WorkFailureTypeAuthFailure, true
	case containsAny(message, `"type":"invalid_request_error"`, "invalid_request_error", "bad request", "invalid request", "request_too_large"):
		return interfaces.WorkFailureTypePermanentBadRequest, true
	case containsAny(message, `"type":"rate_limit_error"`, `"type":"overloaded_error"`, "rate limit", "too many requests", "overloaded", "529"):
		return interfaces.WorkFailureTypeThrottled, true
	case containsAny(message, `"type":"api_error"`, "internal server error", "unexpected status 500", "unexpected status 502", "unexpected status 503", "unexpected status 504"):
		return interfaces.WorkFailureTypeInternalServerError, true
	case containsAny(message, "deadline exceeded", "timed out", "timeout"):
		return interfaces.WorkFailureTypeTimeout, true
	default:
		return interfaces.WorkFailureTypeUnknown, false
	}
}

func claudeTextFailureMessage(message string, reason interfaces.WorkFailureType) string {
	if reason == interfaces.WorkFailureTypeAuthFailure ||
		reason == interfaces.WorkFailureTypePermanentBadRequest ||
		reason == interfaces.WorkFailureTypeMisconfigured {
		if actionable, ok := safeClaudeActionableMessage(message); ok {
			return actionable
		}
	}
	return claudeFallbackFailureMessage(reason, 0)
}

func claudeFallbackFailureMessage(reason interfaces.WorkFailureType, exitCode int) string {
	switch reason {
	case interfaces.WorkFailureTypeAuthFailure:
		return claudeAuthFailureMessage
	case interfaces.WorkFailureTypePermanentBadRequest:
		return claudeBadRequestFailureMessage
	case interfaces.WorkFailureTypeMisconfigured:
		return claudeConfigFailureMessage
	case interfaces.WorkFailureTypeThrottled:
		return claudeThrottleFailureMessage
	case interfaces.WorkFailureTypeInternalServerError:
		return claudeServerFailureMessage
	case interfaces.WorkFailureTypeTimeout:
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

// ParseCodexProviderFailure deterministically parses bounded subprocess output
// into the canonical provider-failure contract. Each stream is limited by
// codexErrorLineScanBytes before parsing, and returned messages are limited by
// codexFailureMessageBytes.
func ParseCodexProviderFailure(result CommandResult) ProviderFailureResult {
	streams := []string{
		tailForCodexErrorScan(result.Stderr),
		tailForCodexErrorScan(result.Stdout),
	}
	if failure, ok := lastCodexStructuredFailure(streams); ok {
		return failure
	}

	if failure, ok := lastCodexTextFailure(streams, result.ExitCode); ok {
		return ProviderFailureResult{
			Reason:  failure.Reason,
			Message: failure.Message,
		}
	}

	return ProviderFailureResult{
		Reason:  classifyCodexExitFailure(result.ExitCode),
		Message: codexExitFailureMessage(result.ExitCode),
	}
}

func lastCodexStructuredFailure(streams []string) (ProviderFailureResult, bool) {
	var last ProviderFailureResult
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
				last = ProviderFailureResult{
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

func lastCodexTextFailure(streams []string, exitCode int) (ProviderFailureResult, bool) {
	var last ProviderFailureResult
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
func recognizedCodexTextFailure(message string, exitCode int) (ProviderFailureResult, bool) {
	normalized := strings.ToLower(strings.TrimSpace(message))
	reason := classifyRecognizedCodexTextFailure(normalized, exitCode)
	if reason == interfaces.WorkFailureTypeUnknown {
		return ProviderFailureResult{}, false
	}
	return ProviderFailureResult{
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
		message == codexHighDemandTemporaryErrorsNeedle:
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
	if exitCode == codexWindowsProcessFailureExitCode {
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

func NewProviderError(errorType interfaces.WorkFailureType, message string, cause error) *ProviderError {
	return NewProviderErrorFromResult(ProviderFailureResult{
		Reason:  errorType,
		Message: message,
	}, cause)
}

// NewProviderErrorFromResult turns a pure parse result into the normalized
// execution error while deriving all runtime policy from its canonical reason.
func NewProviderErrorFromResult(result ProviderFailureResult, cause error) *ProviderError {
	return &ProviderError{
		Family:  providerFailurePolicyForReason(result.Reason).Family,
		Type:    result.Reason,
		Message: result.Message,
		Cause:   cause,
	}
}

func newProviderErrorFromResultWithDiagnostics(result ProviderFailureResult, cause error, session *interfaces.ProviderSessionMetadata, diagnostics *interfaces.WorkDiagnostics) *ProviderError {
	err := NewProviderErrorFromResult(result, cause)
	err.ProviderSession = interfaces.CloneProviderSessionMetadata(session)
	err.Diagnostics = interfaces.CloneWorkDiagnostics(diagnostics)
	return err
}

func NewProviderErrorWithSession(errorType interfaces.WorkFailureType, message string, cause error, session *interfaces.ProviderSessionMetadata) *ProviderError {
	err := NewProviderError(errorType, message, cause)
	err.ProviderSession = interfaces.CloneProviderSessionMetadata(session)
	return err
}

func newProviderErrorWithDiagnostics(errorType interfaces.WorkFailureType, message string, cause error, session *interfaces.ProviderSessionMetadata, diagnostics *interfaces.WorkDiagnostics) *ProviderError {
	return newProviderErrorFromResultWithDiagnostics(ProviderFailureResult{
		Reason:  errorType,
		Message: message,
	}, cause, session, diagnostics)
}

func (e *ProviderError) Error() string {
	return fmt.Sprintf("provider error: %s", e.Type)
}

func (e *ProviderError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func ClassifyProviderFailure(err *ProviderError) interfaces.WorkFailureDecision {
	if err == nil {
		return interfaces.WorkFailureDecision{}
	}
	return providerFailurePolicyForReason(err.Type).Decision
}

// WorkFailureDecisionFromProviderError resolves retry behavior from a normalized
// provider error using the same FailureMetadata projection as WorkResult.
func WorkFailureDecisionFromProviderError(err *ProviderError) interfaces.WorkFailureDecision {
	return WorkFailureDecisionFromMetadata(WorkFailureMetadataFromError(err))
}

// WorkFailureDecisionFromMetadata resolves retry behavior from durable
// generalized failure metadata carried across runtime boundaries.
// The normalized type is canonical when present; family remains a fallback for
// older or partial metadata that omitted type.
func WorkFailureDecisionFromMetadata(metadata *interfaces.WorkFailureMetadata) interfaces.WorkFailureDecision {
	if metadata == nil {
		return interfaces.WorkFailureDecision{}
	}
	if metadata.Type != "" {
		return providerFailurePolicyForReason(metadata.Type).Decision
	}
	return providerFailureDecisionForFamily(metadata.Family)
}

type providerFailurePolicy struct {
	Family   interfaces.WorkFailureFamily
	Decision interfaces.WorkFailureDecision
}

func providerFailurePolicyForReason(reason interfaces.WorkFailureType) providerFailurePolicy {
	switch reason {
	case interfaces.WorkFailureTypeThrottled:
		return providerFailurePolicy{
			Family: interfaces.WorkFailureFamilyThrottle,
			Decision: interfaces.WorkFailureDecision{
				Retryable:             true,
				TriggersThrottlePause: true,
			},
		}
	case interfaces.WorkFailureTypeInternalServerError, interfaces.WorkFailureTypeTimeout:
		return providerFailurePolicy{
			Family:   interfaces.WorkFailureFamilyRetryable,
			Decision: interfaces.WorkFailureDecision{Retryable: true},
		}
	case interfaces.WorkFailureTypeAuthFailure,
		interfaces.WorkFailureTypePermanentBadRequest,
		interfaces.WorkFailureTypeUnknown,
		interfaces.WorkFailureTypeMisconfigured:
		return providerFailurePolicy{
			Family:   interfaces.WorkFailureFamilyTerminal,
			Decision: interfaces.WorkFailureDecision{Terminal: true},
		}
	default:
		return providerFailurePolicy{
			Family:   interfaces.WorkFailureFamilyTerminal,
			Decision: interfaces.WorkFailureDecision{Terminal: true},
		}
	}
}

func providerFailureDecisionForFamily(family interfaces.WorkFailureFamily) interfaces.WorkFailureDecision {
	switch family {
	case interfaces.WorkFailureFamilyRetryable:
		return interfaces.WorkFailureDecision{Retryable: true}
	case interfaces.WorkFailureFamilyThrottle:
		return interfaces.WorkFailureDecision{Retryable: true, TriggersThrottlePause: true}
	case interfaces.WorkFailureFamilyTerminal:
		return interfaces.WorkFailureDecision{Terminal: true}
	default:
		return interfaces.WorkFailureDecision{Terminal: true}
	}
}

func providerErrorFamilyForType(errorType interfaces.WorkFailureType) interfaces.WorkFailureFamily {
	return providerFailurePolicyForReason(errorType).Family
}

// WorkFailureMetadataFromError projects a provider-shaped execution error onto
// the in-process failure contract carried on WorkResult.FailureMetadata.
func WorkFailureMetadataFromError(err *ProviderError) *interfaces.WorkFailureMetadata {
	if err == nil {
		return nil
	}
	return &interfaces.WorkFailureMetadata{
		Family: providerFailurePolicyForReason(err.Type).Family,
		Type:   err.Type,
	}
}

// NormalizeProviderExecutionError projects raw execution failures that affect
// retry policy onto the shared provider failure contract before retry decisions
// are made.
func NormalizeProviderExecutionError(err error) *ProviderError {
	if err == nil {
		return nil
	}
	var providerErr *ProviderError
	if errors.As(err, &providerErr) {
		return providerErr
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return NewProviderError(interfaces.WorkFailureTypeTimeout, "execution timeout", err)
	}
	return nil
}
