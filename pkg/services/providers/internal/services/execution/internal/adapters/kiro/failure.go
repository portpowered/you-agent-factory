package kiro

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

const (
	errorLineScanBytes  = 64 * 1024
	failureMessageBytes = 1024
)

const (
	// TimeoutFailureMessage is the canonical Kiro timeout outcome.
	TimeoutFailureMessage = "Kiro request timed out."
)

const (
	kiroAuthFailureMessage       = "Kiro authentication failed. Sign in again and retry."
	kiroBadRequestFailureMessage = "Kiro rejected the request as invalid."
	kiroThrottleFailureMessage   = "Kiro is temporarily unavailable due to usage or capacity limits."
	kiroTimeoutFailureMessage    = TimeoutFailureMessage
	kiroServerFailureMessage     = "Kiro encountered a temporary service error."
)

type commandFailureInput struct {
	Stdout   []byte
	Stderr   []byte
	ExitCode int
}

type parsedFailure struct {
	Reason  workers.WorkFailureType
	Message string
}

func parseCommandFailure(input commandFailureInput) parsedFailure {
	if input.ExitCode == 124 {
		return knownKiroFailure(workers.WorkFailureTypeTimeout)
	}
	streams := []string{
		tailForErrorScan(input.Stderr),
		tailForErrorScan(input.Stdout),
	}
	if failure, ok := firstKiroStructuredFailure(streams); ok {
		return failure
	}
	if failure, ok := firstKiroTextFailure(streams, input.ExitCode); ok {
		return failure
	}
	if message, ok := firstKiroUnknownFailureExcerpt(streams); ok {
		return parsedFailure{
			Reason:  workers.WorkFailureTypeUnknown,
			Message: message,
		}
	}
	return parsedFailure{
		Reason:  workers.WorkFailureTypeUnknown,
		Message: kiroExitFailureMessage(input.ExitCode),
	}
}

func firstKiroStructuredFailure(streams []string) (parsedFailure, bool) {
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
	return parsedFailure{}, false
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

func decodeKiroStructuredFailure(payload string) (workers.WorkFailureType, bool) {
	var envelope map[string]any
	if err := json.Unmarshal([]byte(payload), &envelope); err != nil {
		return workers.WorkFailureTypeUnknown, false
	}

	signals := kiroStructuredSignals(envelope)
	if nested, ok := envelope["error"].(map[string]any); ok {
		signals = append(kiroStructuredSignals(nested), signals...)
	}
	for _, signal := range signals {
		if reason := classifyKiroSignal(signal); reason != workers.WorkFailureTypeUnknown {
			return reason, true
		}
	}
	for _, status := range kiroStructuredStatuses(envelope) {
		if reason := classifyKiroStatus(status); reason != workers.WorkFailureTypeUnknown {
			return reason, true
		}
	}
	if nested, ok := envelope["error"].(map[string]any); ok {
		for _, status := range kiroStructuredStatuses(nested) {
			if reason := classifyKiroStatus(status); reason != workers.WorkFailureTypeUnknown {
				return reason, true
			}
		}
	}
	return workers.WorkFailureTypeUnknown, false
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

func classifyKiroSignal(signal string) workers.WorkFailureType {
	normalized := strings.ToLower(strings.TrimSpace(signal))
	switch {
	case containsAny(normalized, "authentication", "authorization", "unauthorized", "forbidden", "access_denied", "accessdenied"):
		return workers.WorkFailureTypeAuthFailure
	case containsAny(normalized, "invalid_request", "invalidrequest", "validation", "bad_request", "badrequest", "invalid_argument"):
		return workers.WorkFailureTypePermanentBadRequest
	case containsAny(normalized, "rate_limit", "ratelimit", "throttl", "too_many_requests", "capacity", "overloaded"):
		return workers.WorkFailureTypeThrottled
	case containsAny(normalized, "timeout", "timed_out", "deadline_exceeded"):
		return workers.WorkFailureTypeTimeout
	case containsAny(normalized, "internal_server", "internalserver", "server_error", "service_unavailable", "serviceunavailable", "api_error"):
		return workers.WorkFailureTypeInternalServerError
	default:
		return workers.WorkFailureTypeUnknown
	}
}

func classifyKiroStatus(status int) workers.WorkFailureType {
	switch {
	case status == 401 || status == 403:
		return workers.WorkFailureTypeAuthFailure
	case status == 400 || status == 422:
		return workers.WorkFailureTypePermanentBadRequest
	case status == 429:
		return workers.WorkFailureTypeThrottled
	case status == 408 || status == 504:
		return workers.WorkFailureTypeTimeout
	case status >= 500 && status <= 599:
		return workers.WorkFailureTypeInternalServerError
	default:
		return workers.WorkFailureTypeUnknown
	}
}

func firstKiroTextFailure(streams []string, exitCode int) (parsedFailure, bool) {
	for _, stream := range streams {
		lines := strings.Split(stream, "\n")
		for _, line := range lines {
			message, ok := kiroTextErrorCandidate(line, len(lines) == 1)
			if !ok {
				continue
			}
			if reason := classifyKiroTextFailure(message, exitCode); reason != workers.WorkFailureTypeUnknown {
				return knownKiroFailure(reason), true
			}
		}
	}
	return parsedFailure{}, false
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
	if containsPrivatePath(lower) {
		return true
	}
	return containsAny(lower,
		"prompt", "transcript", "response draft", "model output", "user message",
		"request body", "request payload", "input content", "output content",
		"progress", "cleanup", "credential", "secret", "password",
		"api key", "api_key", "access key", "authorization", "bearer ",
		"access token", "refresh token", "session token", "environment", "env var",
		"token:", "sk-", "ghp_", "github_pat_", "akia",
	)
}

func containsPrivatePath(detail string) bool {
	if containsAny(detail,
		"/home/", "/users/", "/tmp/", "/var/tmp/", "/private/",
		`:\users\`, `:\documents and settings\`, `\appdata\`,
	) {
		return true
	}
	for index := 0; index+2 < len(detail); index++ {
		if detail[index] >= 'a' && detail[index] <= 'z' &&
			detail[index+1] == ':' &&
			(detail[index+2] == '\\' || detail[index+2] == '/') {
			return true
		}
	}
	return false
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

func classifyKiroTextFailure(message string, exitCode int) workers.WorkFailureType {
	normalized := strings.ToLower(message)
	switch {
	case exitCode == 124, containsAny(normalized, "request timed out", "operation timed out", "timeout waiting", "deadline exceeded"):
		return workers.WorkFailureTypeTimeout
	case containsAny(normalized, "authentication required", "authentication failed", "authorization failed", "not authorized", "unauthorized", "forbidden", "sign in required", "login required"):
		return workers.WorkFailureTypeAuthFailure
	case containsAny(normalized, "invalid request", "invalid input", "invalid argument", "bad request", "validation failed"):
		return workers.WorkFailureTypePermanentBadRequest
	case containsAny(normalized, "rate limit", "too many requests", "throttl", "capacity limit", "at capacity", "resource exhausted"):
		return workers.WorkFailureTypeThrottled
	case containsAny(normalized, "internal server error", "temporary service error", "service unavailable", "unexpected status 500", "unexpected status 502", "unexpected status 503"):
		return workers.WorkFailureTypeInternalServerError
	default:
		return workers.WorkFailureTypeUnknown
	}
}

func knownKiroFailure(reason workers.WorkFailureType) parsedFailure {
	message := ""
	switch reason {
	case workers.WorkFailureTypeAuthFailure:
		message = kiroAuthFailureMessage
	case workers.WorkFailureTypePermanentBadRequest:
		message = kiroBadRequestFailureMessage
	case workers.WorkFailureTypeThrottled:
		message = kiroThrottleFailureMessage
	case workers.WorkFailureTypeTimeout:
		message = kiroTimeoutFailureMessage
	case workers.WorkFailureTypeInternalServerError:
		message = kiroServerFailureMessage
	}
	if len(message) > failureMessageBytes {
		message = message[:failureMessageBytes]
	}
	return parsedFailure{Reason: reason, Message: message}
}

func kiroExitFailureMessage(exitCode int) string {
	return fmt.Sprintf("kiro-cli exited with code %d", exitCode)
}

func tailForErrorScan(output []byte) string {
	if len(output) <= errorLineScanBytes {
		return string(output)
	}
	return string(output[len(output)-errorLineScanBytes:])
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

func failureKindFromReason(reason workers.WorkFailureType) providers.ExecuteFailureKind {
	switch reason {
	case workers.WorkFailureTypeAuthFailure:
		return providers.ExecuteFailureKindAuthentication
	case workers.WorkFailureTypePermanentBadRequest:
		return providers.ExecuteFailureKindInvalidRequest
	case workers.WorkFailureTypeThrottled:
		return providers.ExecuteFailureKindThrottled
	case workers.WorkFailureTypeTimeout:
		return providers.ExecuteFailureKindTimeout
	case workers.WorkFailureTypeInternalServerError:
		return providers.ExecuteFailureKindDependency
	default:
		return providers.ExecuteFailureKindUnknown
	}
}
