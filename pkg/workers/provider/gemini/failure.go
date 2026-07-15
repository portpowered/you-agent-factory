package gemini

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode"

	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"
)

const (
	failureScanBytes    = 64 * 1024
	failureMessageRunes = 1024
)

const (
	authFailureMessage     = "Gemini authentication failed."
	badRequestMessage      = "Gemini rejected the request."
	throttleFailureMessage = "The provider is rate limited; retry after capacity becomes available."
	TimeoutFailureMessage  = "Gemini request timed out."
	timeoutFailureMessage  = TimeoutFailureMessage
	serverFailureMessage   = "Gemini encountered a temporary server error."
)

func tailForFailureScan(output []byte) string {
	if len(output) <= failureScanBytes {
		return string(output)
	}
	return string(output[len(output)-failureScanBytes:])
}

type FailureInput struct {
	Stdout   []byte
	Stderr   []byte
	ExitCode int
}

type FailureResult struct {
	Reason  workerexecution.WorkFailureType
	Message string
}

type geminiStructuredFailure struct {
	Type    string
	Status  string
	Code    string
	Message string
}

func ParseProviderFailure(input FailureInput) FailureResult {
	return parseProviderFailure(input)
}

// ParseGeminiProviderFailure converts Gemini-owned structured and text failure
// shapes into one canonical reason/message pair. The final valid structured
// record wins (stderr is the deterministic cross-stream tie-breaker), followed
// by recognized stderr text, recognized stdout text, then safe unknown excerpts
// in that same stderr/stdout order. Both inspected streams and published text
// are bounded by the Gemini-specific limits above.
func parseProviderFailure(input FailureInput) FailureResult {
	result := input
	streams := []string{
		tailForFailureScan(result.Stdout),
		tailForFailureScan(result.Stderr),
	}
	if failure, ok := lastGeminiStructuredFailure(streams); ok {
		if failure.Message == "" {
			failure.Message = fmt.Sprintf("gemini exited with code %d", result.ExitCode)
		}
		return failure
	}
	if result.ExitCode == 124 {
		return geminiFailureResult(workerexecution.WorkFailureTypeTimeout, "")
	}
	stderr := tailForFailureScan(result.Stderr)
	if failure, ok := lastGeminiTextFailure(stderr); ok {
		return failure
	}
	stdout := tailForFailureScan(result.Stdout)
	if failure, ok := lastGeminiTextFailure(stdout); ok {
		return failure
	}
	if message := lastGeminiUnknownMessage(stderr); message != "" {
		return FailureResult{Reason: workerexecution.WorkFailureTypeUnknown, Message: message}
	}
	if message := lastGeminiUnknownMessage(stdout); message != "" {
		return FailureResult{Reason: workerexecution.WorkFailureTypeUnknown, Message: message}
	}
	return FailureResult{
		Reason:  workerexecution.WorkFailureTypeUnknown,
		Message: fmt.Sprintf("gemini exited with code %d", result.ExitCode),
	}
}

func lastGeminiStructuredFailure(streams []string) (FailureResult, bool) {
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
		return FailureResult{}, false
	}
	reason := classifyGeminiFailureSignal(last.Type, last.Status, last.Code, last.Message)
	if reason != workerexecution.WorkFailureTypeUnknown {
		return geminiFailureResult(reason, last.Message), true
	}
	return FailureResult{
		Reason:  workerexecution.WorkFailureTypeUnknown,
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

func lastGeminiTextFailure(stream string) (FailureResult, bool) {
	var last FailureResult
	var found bool
	for _, line := range strings.Split(stream, "\n") {
		message := safeGeminiTextCandidate(line)
		if message == "" {
			continue
		}
		reason := classifyGeminiFailureSignal("", "", "", message)
		if reason == workerexecution.WorkFailureTypeUnknown {
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

func classifyGeminiFailureSignal(errorType, status, code, message string) workerexecution.WorkFailureType {
	structuredSignals := []string{
		strings.ToLower(strings.TrimSpace(errorType)),
		strings.ToLower(strings.TrimSpace(status)),
		strings.ToLower(strings.TrimSpace(code)),
	}
	for _, signal := range structuredSignals {
		switch signal {
		case "fatalauthenticationerror", "authenticationerror", "unauthenticated", "permission_denied", "401", "403":
			return workerexecution.WorkFailureTypeAuthFailure
		case "badrequesterror", "invalid_argument", "400":
			return workerexecution.WorkFailureTypePermanentBadRequest
		case "resource_exhausted", "ratelimitexceeded", "429":
			return workerexecution.WorkFailureTypeThrottled
		case "deadline_exceeded", "124", "408":
			return workerexecution.WorkFailureTypeTimeout
		case "internal", "unavailable", "500", "502", "503", "504":
			return workerexecution.WorkFailureTypeInternalServerError
		}
	}

	normalized := strings.ToLower(strings.TrimSpace(message))
	switch {
	case containsAny(normalized,
		"fatalauthenticationerror", "unauthenticated", "permission_denied",
		"permission denied", "http 401", "status 401", "code 401",
		"http 403", "status 403", "code 403", "authentication failed",
		"unauthorized", "forbidden"):
		return workerexecution.WorkFailureTypeAuthFailure
	case containsAny(normalized,
		"badrequesterror", "invalid_argument", "invalid argument", "invalid request",
		"bad request", "http 400", "status 400", "code 400"):
		return workerexecution.WorkFailureTypePermanentBadRequest
	case containsAny(normalized,
		"resource_exhausted", "resource exhausted", "ratelimitexceeded", "rate limit",
		"quota exceeded", "too many requests", "http 429", "status 429", "code 429"):
		return workerexecution.WorkFailureTypeThrottled
	case containsAny(normalized,
		"deadline_exceeded", "deadline exceeded", "request timed out", "request timeout",
		"command timed out", "provider timed out"):
		return workerexecution.WorkFailureTypeTimeout
	case containsAny(normalized,
		"internal server error", "service unavailable", "upstream unavailable",
		"http 500", "status 500", "code 500", "http 502", "status 502", "code 502",
		"http 503", "status 503", "code 503", "http 504", "status 504", "code 504"):
		return workerexecution.WorkFailureTypeInternalServerError
	default:
		return workerexecution.WorkFailureTypeUnknown
	}
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

func geminiFailureResult(reason workerexecution.WorkFailureType, upstreamMessage string) FailureResult {
	message := geminiFixedFailureMessage(reason)
	if reason == workerexecution.WorkFailureTypeAuthFailure || reason == workerexecution.WorkFailureTypePermanentBadRequest {
		if safe := safeGeminiStructuredMessage(upstreamMessage); safe != "" {
			message = safe
		}
	}
	return FailureResult{Reason: reason, Message: message}
}

func geminiFixedFailureMessage(reason workerexecution.WorkFailureType) string {
	switch reason {
	case workerexecution.WorkFailureTypeAuthFailure:
		return authFailureMessage
	case workerexecution.WorkFailureTypePermanentBadRequest:
		return badRequestMessage
	case workerexecution.WorkFailureTypeThrottled:
		return throttleFailureMessage
	case workerexecution.WorkFailureTypeTimeout:
		return timeoutFailureMessage
	case workerexecution.WorkFailureTypeInternalServerError:
		return serverFailureMessage
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
	if len(runes) > failureMessageRunes {
		message = string(runes[:failureMessageRunes])
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

func containsAny(haystack string, needles ...string) bool {
	for _, needle := range needles {
		if needle != "" && strings.Contains(haystack, needle) {
			return true
		}
	}
	return false
}
