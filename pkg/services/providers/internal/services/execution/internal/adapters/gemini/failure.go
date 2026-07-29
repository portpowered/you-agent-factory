package gemini

import (
	"encoding/json"
	"strings"
	"unicode"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

const (
	failureScanBytes = 64 * 1024
)

const (
	authFailureMessage     = "Gemini authentication failed."
	badRequestMessage      = "Gemini rejected the request."
	throttleFailureMessage = "The provider is rate limited; retry after capacity becomes available."
	// TimeoutFailureMessage is the canonical Gemini timeout outcome.
	TimeoutFailureMessage = "Gemini request timed out."
	timeoutFailureMessage = TimeoutFailureMessage
	serverFailureMessage  = "Gemini encountered a temporary server error."
	unknownFailureMessage = "Gemini invocation failed."
)

type failureReason string

const (
	failureReasonAuth       failureReason = "authentication"
	failureReasonBadRequest failureReason = "invalid_request"
	failureReasonThrottled  failureReason = "throttled"
	failureReasonTimeout    failureReason = "timeout"
	failureReasonDependency failureReason = "dependency"
	failureReasonUnknown    failureReason = "unknown"
)

type commandFailureInput struct {
	Stdout   []byte
	Stderr   []byte
	ExitCode int
}

type parsedFailure struct {
	Reason  failureReason
	Message string
}

func tailForFailureScan(output []byte) string {
	if len(output) <= failureScanBytes {
		return string(output)
	}
	return string(output[len(output)-failureScanBytes:])
}

type geminiStructuredFailure struct {
	Type    string
	Status  string
	Code    string
	Message string
}

func parseCommandFailure(input commandFailureInput) parsedFailure {
	streams := []string{
		tailForFailureScan(input.Stdout),
		tailForFailureScan(input.Stderr),
	}
	if failure, ok := lastGeminiStructuredFailure(streams); ok {
		return failure
	}
	if input.ExitCode == 124 {
		return timeoutFailureResult()
	}
	stderr := tailForFailureScan(input.Stderr)
	if failure, ok := lastGeminiTextFailure(stderr); ok {
		return failure
	}
	stdout := tailForFailureScan(input.Stdout)
	if failure, ok := lastGeminiTextFailure(stdout); ok {
		return failure
	}
	return parsedFailure{
		Reason:  failureReasonUnknown,
		Message: unknownFailureMessage,
	}
}

func lastGeminiStructuredFailure(streams []string) (parsedFailure, bool) {
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
		return parsedFailure{}, false
	}
	reason := classifyGeminiFailureSignal(last.Type, last.Status, last.Code, last.Message)
	if reason != failureReasonUnknown {
		return failureResult(reason), true
	}
	return parsedFailure{
		Reason:  failureReasonUnknown,
		Message: unknownFailureMessage,
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

func lastGeminiTextFailure(stream string) (parsedFailure, bool) {
	var last parsedFailure
	var found bool
	for _, line := range strings.Split(stream, "\n") {
		message := safeGeminiTextCandidate(line)
		if message == "" {
			continue
		}
		reason := classifyGeminiFailureSignal("", "", "", message)
		if reason == failureReasonUnknown {
			continue
		}
		last = failureResult(reason)
		found = true
	}
	return last, found
}

func classifyGeminiFailureSignal(errorType, status, code, message string) failureReason {
	structuredSignals := []string{
		strings.ToLower(strings.TrimSpace(errorType)),
		strings.ToLower(strings.TrimSpace(status)),
		strings.ToLower(strings.TrimSpace(code)),
	}
	for _, signal := range structuredSignals {
		switch signal {
		case "fatalauthenticationerror", "authenticationerror", "unauthenticated", "permission_denied", "401", "403":
			return failureReasonAuth
		case "badrequesterror", "invalid_argument", "400":
			return failureReasonBadRequest
		case "resource_exhausted", "ratelimitexceeded", "429":
			return failureReasonThrottled
		case "deadline_exceeded", "124", "408":
			return failureReasonTimeout
		case "internal", "unavailable", "500", "502", "503", "504":
			return failureReasonDependency
		}
	}

	normalized := strings.ToLower(strings.TrimSpace(message))
	switch {
	case containsAny(normalized,
		"fatalauthenticationerror", "unauthenticated", "permission_denied",
		"permission denied", "http 401", "status 401", "code 401",
		"http 403", "status 403", "code 403", "authentication failed",
		"unauthorized", "forbidden"):
		return failureReasonAuth
	case containsAny(normalized,
		"badrequesterror", "invalid_argument", "invalid argument", "invalid request",
		"bad request", "http 400", "status 400", "code 400"):
		return failureReasonBadRequest
	case containsAny(normalized,
		"resource_exhausted", "resource exhausted", "ratelimitexceeded", "rate limit",
		"quota exceeded", "too many requests", "http 429", "status 429", "code 429"):
		return failureReasonThrottled
	case containsAny(normalized,
		"deadline_exceeded", "deadline exceeded", "request timed out", "request timeout",
		"command timed out", "provider timed out"):
		return failureReasonTimeout
	case containsAny(normalized,
		"internal server error", "service unavailable", "upstream unavailable",
		"http 500", "status 500", "code 500", "http 502", "status 502", "code 502",
		"http 503", "status 503", "code 503", "http 504", "status 504", "code 504"):
		return failureReasonDependency
	default:
		return failureReasonUnknown
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

func failureResult(reason failureReason) parsedFailure {
	return parsedFailure{Reason: reason, Message: fixedFailureMessage(reason)}
}

func timeoutFailureResult() parsedFailure {
	return failureResult(failureReasonTimeout)
}

func fixedFailureMessage(reason failureReason) string {
	switch reason {
	case failureReasonAuth:
		return authFailureMessage
	case failureReasonBadRequest:
		return badRequestMessage
	case failureReasonThrottled:
		return throttleFailureMessage
	case failureReasonTimeout:
		return timeoutFailureMessage
	case failureReasonDependency:
		return serverFailureMessage
	default:
		return unknownFailureMessage
	}
}

func sanitizeGeminiMessage(message string) string {
	message = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, message)
	message = strings.Join(strings.Fields(message), " ")
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

func exitFailureFromCommandResult(result workers.CommandResult) error {
	parsed := parseCommandFailure(commandFailureInput{
		Stdout:   result.Stdout,
		Stderr:   result.Stderr,
		ExitCode: result.ExitCode,
	})
	return declaredFailureFromParsed(parsed)
}

func declaredFailureFromParsed(parsed parsedFailure) providers.ExecuteFailure {
	return providers.ExecuteFailure{
		Kind:    failureKindFromReason(parsed.Reason),
		Message: parsed.Message,
	}
}

func failureKindFromReason(reason failureReason) providers.ExecuteFailureKind {
	switch reason {
	case failureReasonAuth:
		return providers.ExecuteFailureKindAuthentication
	case failureReasonBadRequest:
		return providers.ExecuteFailureKindInvalidRequest
	case failureReasonThrottled:
		return providers.ExecuteFailureKindThrottled
	case failureReasonTimeout:
		return providers.ExecuteFailureKindTimeout
	case failureReasonDependency:
		return providers.ExecuteFailureKindDependency
	default:
		return providers.ExecuteFailureKindUnknown
	}
}
