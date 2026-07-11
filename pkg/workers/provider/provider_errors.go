package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode"

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

const opencodeFailureMessageBytes = 512

const (
	geminiFailureScanBytes    = 64 * 1024
	geminiFailureMessageRunes = 1024
)

const (
	codexAuthFailureMessage       = "Codex authentication failed."
	codexBadRequestFailureMessage = "Codex rejected the request as invalid."
	codexGPT56SolUpgradeMessage   = "The 'gpt-5.6-sol' model requires a newer version of Codex. Please upgrade to the latest app or CLI and try again."
	codexServerFailureMessage     = "Codex encountered a temporary server error."
	codexThrottleFailureMessage   = "Codex is temporarily unavailable due to usage or capacity limits."
	codexTimeoutFailureMessage    = "Codex request timed out."
)

const (
	geminiAuthFailureMessage         = "Gemini authentication failed."
	geminiBadRequestMessage          = "Gemini rejected the request."
	geminiThrottleFailureMessage     = "The provider is rate limited; retry after capacity becomes available."
	geminiTimeoutFailureMessage      = "Gemini request timed out."
	geminiServerFailureMessage       = "Gemini encountered a temporary server error."
	opencodeAuthFailureMessage       = "OpenCode authentication failed."
	opencodeBadRequestFailureMessage = "OpenCode rejected the request as invalid."
	opencodeServerFailureMessage     = "OpenCode encountered a temporary server error."
	opencodeThrottleFailureMessage   = "OpenCode is temporarily unavailable due to usage or capacity limits."
	opencodeTimeoutFailureMessage    = "OpenCode request timed out."
)

type codexStructuredFailure struct {
	Type    string
	Status  int
	Message string
}

type geminiStructuredFailure struct {
	Type    string
	Status  string
	Code    string
	Message string
}

// ParseGeminiProviderFailure converts Gemini-owned structured and text failure
// shapes into one canonical reason/message pair. The final valid structured
// record wins (stderr is the deterministic cross-stream tie-breaker), followed
// by recognized stderr text, recognized stdout text, then safe unknown excerpts
// in that same stderr/stdout order. Both inspected streams and published text
// are bounded by the Gemini-specific limits above.
func ParseGeminiProviderFailure(result CommandResult) ProviderFailureResult {
	streams := []string{
		tailForGeminiFailureScan(result.Stdout),
		tailForGeminiFailureScan(result.Stderr),
	}
	if failure, ok := lastGeminiStructuredFailure(streams); ok {
		if failure.Message == "" {
			failure.Message = fmt.Sprintf("gemini exited with code %d", result.ExitCode)
		}
		return failure
	}
	if result.ExitCode == 124 {
		return geminiFailureResult(interfaces.WorkFailureTypeTimeout, "")
	}
	stderr := tailForGeminiFailureScan(result.Stderr)
	if failure, ok := lastGeminiTextFailure(stderr); ok {
		return failure
	}
	stdout := tailForGeminiFailureScan(result.Stdout)
	if failure, ok := lastGeminiTextFailure(stdout); ok {
		return failure
	}
	if message := lastGeminiUnknownMessage(stderr); message != "" {
		return ProviderFailureResult{Reason: interfaces.WorkFailureTypeUnknown, Message: message}
	}
	if message := lastGeminiUnknownMessage(stdout); message != "" {
		return ProviderFailureResult{Reason: interfaces.WorkFailureTypeUnknown, Message: message}
	}
	return ProviderFailureResult{
		Reason:  interfaces.WorkFailureTypeUnknown,
		Message: fmt.Sprintf("gemini exited with code %d", result.ExitCode),
	}
}

func lastGeminiStructuredFailure(streams []string) (ProviderFailureResult, bool) {
	var last geminiStructuredFailure
	var found bool
	for _, stream := range streams {
		for _, line := range strings.Split(stream, "\n") {
			failure, ok := decodeGeminiStructuredFailure(strings.TrimSpace(line))
			if !ok {
				continue
			}
			last = failure
			found = true
		}
	}
	if !found {
		return ProviderFailureResult{}, false
	}
	reason := classifyGeminiFailureSignal(last.Type, last.Status, last.Code, last.Message)
	if reason != interfaces.WorkFailureTypeUnknown {
		return geminiFailureResult(reason, last.Message), true
	}
	return ProviderFailureResult{
		Reason:  interfaces.WorkFailureTypeUnknown,
		Message: safeGeminiStructuredMessage(last.Message),
	}, true
}

func decodeGeminiStructuredFailure(payload string) (geminiStructuredFailure, bool) {
	if !strings.HasPrefix(payload, "{") {
		return geminiStructuredFailure{}, false
	}
	var envelope struct {
		Type    string          `json:"type"`
		Status  json.RawMessage `json:"status"`
		Code    json.RawMessage `json:"code"`
		Message string          `json:"message"`
		Error   *struct {
			Type    string          `json:"type"`
			Status  json.RawMessage `json:"status"`
			Code    json.RawMessage `json:"code"`
			Message string          `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(payload), &envelope); err != nil {
		return geminiStructuredFailure{}, false
	}
	if envelope.Error == nil && !isGeminiErrorRecordType(envelope.Type) {
		return geminiStructuredFailure{}, false
	}
	failure := geminiStructuredFailure{
		Type:    envelope.Type,
		Status:  geminiJSONScalar(envelope.Status),
		Code:    geminiJSONScalar(envelope.Code),
		Message: envelope.Message,
	}
	if envelope.Error != nil {
		if envelope.Error.Type != "" {
			failure.Type = envelope.Error.Type
		}
		if status := geminiJSONScalar(envelope.Error.Status); status != "" {
			failure.Status = status
		}
		if code := geminiJSONScalar(envelope.Error.Code); code != "" {
			failure.Code = code
		}
		if envelope.Error.Message != "" {
			failure.Message = envelope.Error.Message
		}
	}
	if failure.Type == "" && failure.Status == "" && failure.Code == "" && failure.Message == "" {
		return geminiStructuredFailure{}, false
	}
	return failure, true
}

func isGeminiErrorRecordType(recordType string) bool {
	switch strings.ToLower(strings.TrimSpace(recordType)) {
	case "error", "fatalauthenticationerror", "authenticationerror", "badrequesterror":
		return true
	default:
		return false
	}
}

func geminiJSONScalar(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return text
	}
	return strings.TrimSpace(string(raw))
}

func lastGeminiTextFailure(stream string) (ProviderFailureResult, bool) {
	var last ProviderFailureResult
	var found bool
	for _, line := range strings.Split(stream, "\n") {
		message := safeGeminiTextCandidate(line)
		if message == "" {
			continue
		}
		reason := classifyGeminiFailureSignal("", "", "", message)
		if reason == interfaces.WorkFailureTypeUnknown {
			continue
		}
		last = geminiFailureResult(reason, "")
		found = true
	}
	return last, found
}

func lastGeminiUnknownMessage(stream string) string {
	var last string
	for _, line := range strings.Split(stream, "\n") {
		if candidate := safeGeminiTextCandidate(line); candidate != "" {
			last = candidate
		}
	}
	return last
}

func safeGeminiTextCandidate(line string) string {
	message := sanitizeGeminiMessage(line)
	if message == "" || strings.HasPrefix(message, "{") {
		return ""
	}
	normalized := strings.ToLower(message)
	if isRejectedGeminiMessage(normalized) || !isGeminiErrorSignal(normalized) {
		return ""
	}
	return message
}

func isGeminiErrorSignal(normalized string) bool {
	if strings.HasPrefix(normalized, "error:") ||
		strings.HasPrefix(normalized, "gemini error:") ||
		strings.HasPrefix(normalized, "fatal") ||
		strings.HasPrefix(normalized, "failed") ||
		strings.HasPrefix(normalized, "failure:") ||
		strings.HasPrefix(normalized, "cannot ") ||
		strings.HasPrefix(normalized, "could not ") {
		return true
	}
	return containsAny(normalized,
		"http 4", "http 5", "status 4", "status 5",
		"unauthenticated", "permission_denied", "resource_exhausted", "resource exhausted",
		"deadline_exceeded", "timed out", "timeout", "permission denied",
		"rate limit exceeded", "quota exceeded", "too many requests",
		"invalid request", "bad request", "service unavailable", "upstream unavailable")
}

func classifyGeminiFailureSignal(errorType, status, code, message string) interfaces.WorkFailureType {
	structuredSignals := []string{
		strings.ToLower(strings.TrimSpace(errorType)),
		strings.ToLower(strings.TrimSpace(status)),
		strings.ToLower(strings.TrimSpace(code)),
	}
	for _, signal := range structuredSignals {
		switch signal {
		case "fatalauthenticationerror", "authenticationerror", "unauthenticated", "permission_denied", "401", "403":
			return interfaces.WorkFailureTypeAuthFailure
		case "badrequesterror", "invalid_argument", "400":
			return interfaces.WorkFailureTypePermanentBadRequest
		case "resource_exhausted", "ratelimitexceeded", "429":
			return interfaces.WorkFailureTypeThrottled
		case "deadline_exceeded", "124", "408":
			return interfaces.WorkFailureTypeTimeout
		case "internal", "unavailable", "500", "502", "503", "504":
			return interfaces.WorkFailureTypeInternalServerError
		}
	}

	normalized := strings.ToLower(strings.TrimSpace(message))
	switch {
	case containsAny(normalized,
		"fatalauthenticationerror", "unauthenticated", "permission_denied",
		"permission denied", "http 401", "status 401", "code 401",
		"http 403", "status 403", "code 403", "authentication failed",
		"unauthorized", "forbidden"):
		return interfaces.WorkFailureTypeAuthFailure
	case containsAny(normalized,
		"badrequesterror", "invalid_argument", "invalid argument", "invalid request",
		"bad request", "http 400", "status 400", "code 400"):
		return interfaces.WorkFailureTypePermanentBadRequest
	case containsAny(normalized,
		"resource_exhausted", "resource exhausted", "ratelimitexceeded", "rate limit",
		"quota exceeded", "too many requests", "http 429", "status 429", "code 429"):
		return interfaces.WorkFailureTypeThrottled
	case containsAny(normalized,
		"deadline_exceeded", "deadline exceeded", "request timed out", "request timeout",
		"command timed out", "provider timed out"):
		return interfaces.WorkFailureTypeTimeout
	case containsAny(normalized,
		"internal server error", "service unavailable", "upstream unavailable",
		"http 500", "status 500", "code 500", "http 502", "status 502", "code 502",
		"http 503", "status 503", "code 503", "http 504", "status 504", "code 504"):
		return interfaces.WorkFailureTypeInternalServerError
	default:
		return interfaces.WorkFailureTypeUnknown
	}
}

func geminiFailureResult(reason interfaces.WorkFailureType, upstreamMessage string) ProviderFailureResult {
	message := geminiFixedFailureMessage(reason)
	if reason == interfaces.WorkFailureTypeAuthFailure || reason == interfaces.WorkFailureTypePermanentBadRequest {
		if safe := safeGeminiStructuredMessage(upstreamMessage); safe != "" {
			message = safe
		}
	}
	return ProviderFailureResult{Reason: reason, Message: message}
}

func geminiFixedFailureMessage(reason interfaces.WorkFailureType) string {
	switch reason {
	case interfaces.WorkFailureTypeAuthFailure:
		return geminiAuthFailureMessage
	case interfaces.WorkFailureTypePermanentBadRequest:
		return geminiBadRequestMessage
	case interfaces.WorkFailureTypeThrottled:
		return geminiThrottleFailureMessage
	case interfaces.WorkFailureTypeTimeout:
		return geminiTimeoutFailureMessage
	case interfaces.WorkFailureTypeInternalServerError:
		return geminiServerFailureMessage
	default:
		return ""
	}
}

type opencodeStructuredFailure struct {
	Name       string
	Type       string
	Code       string
	StatusCode int
	Message    string
}

// ParseOpenCodeProviderFailure deterministically parses bounded OpenCode
// subprocess output into the canonical provider-failure contract. Recognized
// structured records take precedence over recognized text diagnostics.
func ParseOpenCodeProviderFailure(result CommandResult) ProviderFailureResult {
	streams := []string{
		tailForCodexErrorScan(result.Stderr),
		tailForCodexErrorScan(result.Stdout),
	}
	if failure, ok := lastOpenCodeStructuredFailure(streams); ok {
		return failure
	}
	if failure, ok := lastOpenCodeTextFailure(streams, result.ExitCode); ok {
		return failure
	}
	if excerpt, ok := lastSafeOpenCodeUnknownExcerpt(streams); ok {
		return ProviderFailureResult{
			Reason:  interfaces.WorkFailureTypeUnknown,
			Message: excerpt,
		}
	}
	return ProviderFailureResult{
		Reason:  interfaces.WorkFailureTypeUnknown,
		Message: fmt.Sprintf("opencode exited with code %d", result.ExitCode),
	}
}

func firstKiroStructuredFailure(streams []string) (ProviderFailureResult, bool) {
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
	return ProviderFailureResult{}, false
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

func decodeKiroStructuredFailure(payload string) (interfaces.WorkFailureType, bool) {
	var envelope map[string]any
	if err := json.Unmarshal([]byte(payload), &envelope); err != nil {
		return interfaces.WorkFailureTypeUnknown, false
	}

	signals := kiroStructuredSignals(envelope)
	if nested, ok := envelope["error"].(map[string]any); ok {
		signals = append(kiroStructuredSignals(nested), signals...)
	}
	for _, signal := range signals {
		if reason := classifyKiroSignal(signal); reason != interfaces.WorkFailureTypeUnknown {
			return reason, true
		}
	}
	for _, status := range kiroStructuredStatuses(envelope) {
		if reason := classifyKiroStatus(status); reason != interfaces.WorkFailureTypeUnknown {
			return reason, true
		}
	}
	if nested, ok := envelope["error"].(map[string]any); ok {
		for _, status := range kiroStructuredStatuses(nested) {
			if reason := classifyKiroStatus(status); reason != interfaces.WorkFailureTypeUnknown {
				return reason, true
			}
		}
	}
	return interfaces.WorkFailureTypeUnknown, false
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

func classifyKiroSignal(signal string) interfaces.WorkFailureType {
	normalized := strings.ToLower(strings.TrimSpace(signal))
	switch {
	case containsAny(normalized, "authentication", "authorization", "unauthorized", "forbidden", "access_denied", "accessdenied"):
		return interfaces.WorkFailureTypeAuthFailure
	case containsAny(normalized, "invalid_request", "invalidrequest", "validation", "bad_request", "badrequest", "invalid_argument"):
		return interfaces.WorkFailureTypePermanentBadRequest
	case containsAny(normalized, "rate_limit", "ratelimit", "throttl", "too_many_requests", "capacity", "overloaded"):
		return interfaces.WorkFailureTypeThrottled
	case containsAny(normalized, "timeout", "timed_out", "deadline_exceeded"):
		return interfaces.WorkFailureTypeTimeout
	case containsAny(normalized, "internal_server", "internalserver", "server_error", "service_unavailable", "serviceunavailable", "api_error"):
		return interfaces.WorkFailureTypeInternalServerError
	default:
		return interfaces.WorkFailureTypeUnknown
	}
}

func classifyKiroStatus(status int) interfaces.WorkFailureType {
	switch {
	case status == 401 || status == 403:
		return interfaces.WorkFailureTypeAuthFailure
	case status == 400 || status == 422:
		return interfaces.WorkFailureTypePermanentBadRequest
	case status == 429:
		return interfaces.WorkFailureTypeThrottled
	case status == 408 || status == 504:
		return interfaces.WorkFailureTypeTimeout
	case status >= 500 && status <= 599:
		return interfaces.WorkFailureTypeInternalServerError
	default:
		return interfaces.WorkFailureTypeUnknown
	}
}

func firstKiroTextFailure(streams []string, exitCode int) (ProviderFailureResult, bool) {
	for _, stream := range streams {
		lines := strings.Split(stream, "\n")
		for _, line := range lines {
			message, ok := kiroTextErrorCandidate(line, len(lines) == 1)
			if !ok {
				continue
			}
			if reason := classifyKiroTextFailure(message, exitCode); reason != interfaces.WorkFailureTypeUnknown {
				return knownKiroFailure(reason), true
			}
		}
	}
	return ProviderFailureResult{}, false
}

func openCodeErrorTextLine(line string) bool {
	normalized := strings.ToLower(line)
	return strings.HasPrefix(normalized, "error:") || strings.HasPrefix(normalized, "api error:")
}

func recognizedOpenCodeTextFailure(message string, exitCode int) (ProviderFailureResult, bool) {
	normalized := strings.ToLower(strings.TrimSpace(message))
	var reason interfaces.WorkFailureType
	switch {
	case exitCode == 124 || containsAny(normalized, "deadline exceeded", "request timed out", "timed out", "timeout"):
		reason = interfaces.WorkFailureTypeTimeout
	case containsAny(normalized, "authentication", "login required", "not authenticated", "unauthorized", "forbidden", "api key"):
		reason = interfaces.WorkFailureTypeAuthFailure
	case containsAny(normalized, "invalid request", "bad request", "invalid argument", "model not found"):
		reason = interfaces.WorkFailureTypePermanentBadRequest
	case containsAny(normalized, "rate limit", "too many requests", "usage limit", "at capacity", "status 429"):
		reason = interfaces.WorkFailureTypeThrottled
	case containsAny(normalized, "internal server error", "server error", "status 500", "status 502", "status 503", "status 504"):
		reason = interfaces.WorkFailureTypeInternalServerError
	default:
		return ProviderFailureResult{}, false
	}
	return ProviderFailureResult{Reason: reason, Message: openCodeFailureMessage(reason, message)}, true
}

func openCodeFailureMessage(reason interfaces.WorkFailureType, detail string) string {
	if reason == interfaces.WorkFailureTypeAuthFailure || reason == interfaces.WorkFailureTypePermanentBadRequest {
		if sanitized, ok := safeOpenCodeFailureDetail(detail); ok {
			return sanitized
		}
	}
	switch reason {
	case interfaces.WorkFailureTypeAuthFailure:
		return opencodeAuthFailureMessage
	case interfaces.WorkFailureTypePermanentBadRequest:
		return opencodeBadRequestFailureMessage
	case interfaces.WorkFailureTypeThrottled:
		return opencodeThrottleFailureMessage
	case interfaces.WorkFailureTypeTimeout:
		return opencodeTimeoutFailureMessage
	case interfaces.WorkFailureTypeInternalServerError:
		return opencodeServerFailureMessage
	default:
		return ""
	}
}

func safeGeminiStructuredMessage(message string) string {
	message = sanitizeGeminiMessage(message)
	if message == "" {
		return ""
	}
	normalized := strings.ToLower(message)
	if isRejectedGeminiMessage(normalized) {
		return ""
	}
	return message
}

func sanitizeGeminiMessage(message string) string {
	message = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, message)
	message = strings.Join(strings.Fields(message), " ")
	runes := []rune(message)
	if len(runes) > geminiFailureMessageRunes {
		message = string(runes[:geminiFailureMessageRunes])
	}
	return message
}

func isRejectedGeminiMessage(normalized string) bool {
	if strings.HasPrefix(normalized, "at ") || strings.HasPrefix(normalized, "goroutine ") {
		return true
	}
	return containsAny(normalized,
		"authorization:", "basic ", "bearer ", "api_key=", "api-key=", "password=", "token=", "secret=", "sk-",
		"api key:", "credential=", "aiza", "ya29.", "-----begin private key",
		"customer prompt", "user prompt", "prompt:", "model response", "transcript:",
		"[debug]", "debug:", "[progress]", "progress:", "traceback", "stack trace",
		"error report", "report written", "cleanup", "cleaning up", "/tmp/", "/var/tmp/", ".gemini/tmp/")
}

func tailForGeminiFailureScan(output []byte) string {
	if len(output) <= geminiFailureScanBytes {
		return string(output)
	}
	return string(output[len(output)-geminiFailureScanBytes:])
}

func safeOpenCodeFailureDetail(detail string) (string, bool) {
	detail = strings.ToValidUTF8(strings.Join(strings.Fields(detail), " "), "")
	normalized := strings.ToLower(detail)
	if detail == "" || containsAny(normalized,
		"authorization:", "bearer ", "api_key=", "api-key=", `"token":`, "secret=", "sk-", "prompt:", "transcript:",
	) {
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
	if len(message) <= kiroFailureMessageBytes {
		return message
	}
	end := 0
	for index := range message {
		if index > kiroFailureMessageBytes {
			break
		}
		end = index
	}
	return strings.TrimSpace(message[:end])
}

func classifyKiroTextFailure(message string, exitCode int) interfaces.WorkFailureType {
	normalized := strings.ToLower(message)
	switch {
	case exitCode == 124, containsAny(normalized, "request timed out", "operation timed out", "timeout waiting", "deadline exceeded"):
		return interfaces.WorkFailureTypeTimeout
	case containsAny(normalized, "authentication required", "authentication failed", "authorization failed", "not authorized", "unauthorized", "forbidden", "sign in required", "login required"):
		return interfaces.WorkFailureTypeAuthFailure
	case containsAny(normalized, "invalid request", "invalid input", "invalid argument", "bad request", "validation failed"):
		return interfaces.WorkFailureTypePermanentBadRequest
	case containsAny(normalized, "rate limit", "too many requests", "throttl", "capacity limit", "at capacity", "resource exhausted"):
		return interfaces.WorkFailureTypeThrottled
	case containsAny(normalized, "internal server error", "temporary service error", "service unavailable", "unexpected status 500", "unexpected status 502", "unexpected status 503"):
		return interfaces.WorkFailureTypeInternalServerError
	default:
		return interfaces.WorkFailureTypeUnknown
	}
}

func knownKiroFailure(reason interfaces.WorkFailureType) ProviderFailureResult {
	message := ""
	switch reason {
	case interfaces.WorkFailureTypeAuthFailure:
		message = kiroAuthFailureMessage
	case interfaces.WorkFailureTypePermanentBadRequest:
		message = kiroBadRequestFailureMessage
	case interfaces.WorkFailureTypeThrottled:
		message = kiroThrottleFailureMessage
	case interfaces.WorkFailureTypeTimeout:
		message = kiroTimeoutFailureMessage
	case interfaces.WorkFailureTypeInternalServerError:
		message = kiroServerFailureMessage
	}
	if len(message) > kiroFailureMessageBytes {
		message = message[:kiroFailureMessageBytes]
	}
	return ProviderFailureResult{Reason: reason, Message: message}
}

func tailForKiroErrorScan(output []byte) string {
	if len(output) <= kiroErrorLineScanBytes {
		return string(output)
	}
	return string(output[len(output)-kiroErrorLineScanBytes:])
}

func kiroExitFailureMessage(exitCode int) string {
	return fmt.Sprintf("kiro-cli exited with code %d", exitCode)
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
