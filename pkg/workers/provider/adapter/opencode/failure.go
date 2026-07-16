package opencode

import (
	"encoding/json"
	"fmt"
	"strings"

	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"
)

const (
	errorLineScanBytes       = 64 * 1024
	failureMessageBytes      = 512
	ThrottleFailureMessage   = "OpenCode is temporarily unavailable due to usage or capacity limits."
	TimeoutFailureMessage    = "OpenCode request timed out."
	BadRequestFailureMessage = "OpenCode rejected the request as invalid."
)

const (
	authFailureMessage       = "OpenCode authentication failed."
	badRequestFailureMessage = "OpenCode rejected the request as invalid."
	serverFailureMessage     = "OpenCode encountered a temporary server error."
	throttleFailureMessage   = "OpenCode is temporarily unavailable due to usage or capacity limits."
	timeoutFailureMessage    = "OpenCode request timed out."
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

func firstNonZero(values ...int) int {
	for _, value := range values {
		if value != 0 {
			return value
		}
	}
	return 0
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

type structuredFailure struct {
	Name       string
	Type       string
	Code       string
	StatusCode int
	Message    string
}

// ParseOpenCodeProviderFailure deterministically parses bounded OpenCode
// subprocess output into the canonical provider-failure contract. Recognized
// structured records take precedence over recognized text diagnostics.
func parseProviderFailure(input FailureInput) FailureResult {
	streams := []string{
		tailForErrorScan(input.Stderr),
		tailForErrorScan(input.Stdout),
	}
	if failure, ok := lastStructuredFailure(streams); ok {
		return failure
	}
	if failure, ok := lastTextFailure(streams, input.ExitCode); ok {
		return failure
	}
	if excerpt, ok := lastSafeUnknownExcerpt(streams); ok {
		return FailureResult{
			Reason:  workerexecution.WorkFailureTypeUnknown,
			Message: excerpt,
		}
	}
	return FailureResult{
		Reason:  workerexecution.WorkFailureTypeUnknown,
		Message: fmt.Sprintf("opencode exited with code %d", input.ExitCode),
	}
}

func lastSafeUnknownExcerpt(streams []string) (string, bool) {
	var last string
	for _, stream := range streams {
		for _, line := range strings.Split(stream, "\n") {
			trimmed := strings.TrimSpace(line)
			if failure, ok := decodeStructuredFailure(trimmed); ok {
				if excerpt, safe := safeFailureDetail(failure.Message); safe {
					last = excerpt
				}
				continue
			}
			if !errorTextLine(trimmed) {
				continue
			}
			if excerpt, safe := safeFailureDetail(trimmed); safe {
				last = excerpt
			}
		}
	}
	return last, last != ""
}

func lastStructuredFailure(streams []string) (FailureResult, bool) {
	var last FailureResult
	var found bool
	for _, stream := range streams {
		for _, line := range strings.Split(stream, "\n") {
			failure, ok := decodeStructuredFailure(strings.TrimSpace(line))
			if !ok {
				continue
			}
			reason, recognized := classifyStructuredFailure(failure)
			if !recognized {
				continue
			}
			last = FailureResult{
				Reason:  reason,
				Message: failureMessage(reason, failure.Message),
			}
			found = true
		}
	}
	return last, found
}

func decodeStructuredFailure(line string) (structuredFailure, bool) {
	if !strings.HasPrefix(line, "{") {
		return structuredFailure{}, false
	}
	var envelope struct {
		Type       string `json:"type"`
		Name       string `json:"name"`
		Code       string `json:"code"`
		Status     int    `json:"status"`
		StatusCode int    `json:"statusCode"`
		Message    string `json:"message"`
		Error      *struct {
			Type       string `json:"type"`
			Name       string `json:"name"`
			Code       string `json:"code"`
			Status     int    `json:"status"`
			StatusCode int    `json:"statusCode"`
			Message    string `json:"message"`
			Data       *struct {
				Code       string `json:"code"`
				Status     int    `json:"status"`
				StatusCode int    `json:"statusCode"`
				Message    string `json:"message"`
			} `json:"data"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(line), &envelope); err != nil {
		return structuredFailure{}, false
	}

	failure := structuredFailure{
		Name:       envelope.Name,
		Type:       envelope.Type,
		Code:       envelope.Code,
		StatusCode: firstNonZero(envelope.StatusCode, envelope.Status),
		Message:    envelope.Message,
	}
	if envelope.Error != nil {
		failure.Name = firstNonEmpty(envelope.Error.Name, failure.Name)
		failure.Type = firstNonEmpty(envelope.Error.Type, failure.Type)
		failure.Code = firstNonEmpty(envelope.Error.Code, failure.Code)
		failure.StatusCode = firstNonZero(envelope.Error.StatusCode, envelope.Error.Status, failure.StatusCode)
		failure.Message = firstNonEmpty(envelope.Error.Message, failure.Message)
		if envelope.Error.Data != nil {
			failure.Code = firstNonEmpty(envelope.Error.Data.Code, failure.Code)
			failure.StatusCode = firstNonZero(envelope.Error.Data.StatusCode, envelope.Error.Data.Status, failure.StatusCode)
			failure.Message = firstNonEmpty(envelope.Error.Data.Message, failure.Message)
		}
	}
	if envelope.Error == nil && !strings.EqualFold(envelope.Type, "error") {
		return structuredFailure{}, false
	}
	return failure, true
}

func classifyStructuredFailure(failure structuredFailure) (workerexecution.WorkFailureType, bool) {
	signal := strings.ToLower(strings.Join([]string{failure.Name, failure.Type, failure.Code}, " "))
	switch {
	case containsAny(signal, "providerautherror", "authentication_error", "permission_error", "unauthorized", "forbidden"):
		return workerexecution.WorkFailureTypeAuthFailure, true
	case containsAny(signal, "invalid_request_error", "badrequesterror", "invalidrequesterror"):
		return workerexecution.WorkFailureTypePermanentBadRequest, true
	case containsAny(signal, "ratelimiterror", "rate_limit_error", "overloaded_error", "quotaexceeded"):
		return workerexecution.WorkFailureTypeThrottled, true
	case containsAny(signal, "timeouterror", "timeout_error", "etimedout"):
		return workerexecution.WorkFailureTypeTimeout, true
	case containsAny(signal, "server_error", "internalservererror"):
		return workerexecution.WorkFailureTypeInternalServerError, true
	}
	switch {
	case failure.StatusCode == 401 || failure.StatusCode == 403:
		return workerexecution.WorkFailureTypeAuthFailure, true
	case failure.StatusCode == 400 || failure.StatusCode == 422:
		return workerexecution.WorkFailureTypePermanentBadRequest, true
	case failure.StatusCode == 408:
		return workerexecution.WorkFailureTypeTimeout, true
	case failure.StatusCode == 429:
		return workerexecution.WorkFailureTypeThrottled, true
	case failure.StatusCode >= 500 && failure.StatusCode <= 599:
		return workerexecution.WorkFailureTypeInternalServerError, true
	default:
		return workerexecution.WorkFailureTypeUnknown, false
	}
}

func lastTextFailure(streams []string, exitCode int) (FailureResult, bool) {
	var last FailureResult
	var found bool
	for _, stream := range streams {
		for _, line := range strings.Split(stream, "\n") {
			trimmed := strings.TrimSpace(line)
			if !errorTextLine(trimmed) {
				continue
			}
			if failure, ok := recognizedTextFailure(trimmed, exitCode); ok {
				last, found = failure, true
			}
		}
	}
	if found {
		return last, true
	}
	for _, stream := range streams {
		trimmed := strings.TrimSpace(stream)
		if trimmed == "" || strings.Contains(trimmed, "\n") {
			continue
		}
		if failure, ok := recognizedTextFailure(trimmed, exitCode); ok {
			last, found = failure, true
		}
	}
	return last, found
}

func errorTextLine(line string) bool {
	normalized := strings.ToLower(line)
	return strings.HasPrefix(normalized, "error:") || strings.HasPrefix(normalized, "api error:")
}

func recognizedTextFailure(message string, exitCode int) (FailureResult, bool) {
	normalized := strings.ToLower(strings.TrimSpace(message))
	var reason workerexecution.WorkFailureType
	switch {
	case exitCode == 124 || containsAny(normalized, "deadline exceeded", "request timed out", "timed out", "timeout"):
		reason = workerexecution.WorkFailureTypeTimeout
	case containsAny(normalized, "authentication", "login required", "not authenticated", "unauthorized", "forbidden", "api key"):
		reason = workerexecution.WorkFailureTypeAuthFailure
	case containsAny(normalized, "invalid request", "bad request", "invalid argument", "model not found"):
		reason = workerexecution.WorkFailureTypePermanentBadRequest
	case containsAny(normalized, "rate limit", "too many requests", "usage limit", "at capacity", "status 429"):
		reason = workerexecution.WorkFailureTypeThrottled
	case containsAny(normalized, "internal server error", "server error", "status 500", "status 502", "status 503", "status 504"):
		reason = workerexecution.WorkFailureTypeInternalServerError
	default:
		return FailureResult{}, false
	}
	return FailureResult{Reason: reason, Message: failureMessage(reason, message)}, true
}

func failureMessage(reason workerexecution.WorkFailureType, detail string) string {
	if reason == workerexecution.WorkFailureTypeAuthFailure || reason == workerexecution.WorkFailureTypePermanentBadRequest {
		if sanitized, ok := safeFailureDetail(detail); ok {
			return sanitized
		}
	}
	switch reason {
	case workerexecution.WorkFailureTypeAuthFailure:
		return authFailureMessage
	case workerexecution.WorkFailureTypePermanentBadRequest:
		return badRequestFailureMessage
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

func safeFailureDetail(detail string) (string, bool) {
	detail = strings.ToValidUTF8(strings.Join(strings.Fields(detail), " "), "")
	normalized := strings.ToLower(detail)
	if detail == "" || containsAny(normalized,
		"authorization:", "bearer ", "api_key=", "api-key=", `"token":`, "secret=", "sk-", "prompt:", "transcript:",
	) {
		return "", false
	}
	if len(detail) <= failureMessageBytes {
		return detail, true
	}
	end := failureMessageBytes
	for end > 0 && detail[end]&0xc0 == 0x80 {
		end--
	}
	return detail[:end], true
}

func containsAny(haystack string, needles ...string) bool {
	for _, needle := range needles {
		if needle != "" && strings.Contains(haystack, needle) {
			return true
		}
	}
	return false
}
