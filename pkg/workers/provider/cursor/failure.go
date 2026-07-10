package cursor

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

const FailureMessageLimit = 1024

const (
	cursorAuthFailureMessage       = "Cursor authentication failed. Sign in again or check the configured credentials."
	cursorBadRequestFailureMessage = "Cursor rejected the request as invalid. Check the model and Cursor configuration."
	cursorThrottleFailureMessage   = "Cursor is temporarily unavailable due to usage or capacity limits."
	cursorTimeoutFailureMessage    = "Cursor request timed out."
	cursorServerFailureMessage     = "Cursor encountered a temporary server error."
	cursorUnknownFailureMessage    = "Cursor reported an unsuccessful result."
)

var unsafeCursorFailureTextPattern = regexp.MustCompile(`(?i)(authorization\s*:|bearer\s+\S+|api[_ -]?key\s*[:=]\s*\S+|password\s*[:=]|secret\s*[:=]|token\s*[:=]|private prompt|user prompt|complete transcript|full transcript|cleanup noise|could not be terminated|failed to terminate process)`)

// FailureInput contains the bounded subprocess surfaces used by Cursor's pure
// failure parser. Runtime policy is intentionally not part of this contract.
type FailureInput struct {
	Stdout   []byte
	Stderr   []byte
	ExitCode int
}

// FailureResult is Cursor's canonical failure reason and customer-visible
// message. Provider runtime policy is derived centrally from Reason.
type FailureResult struct {
	Reason          interfaces.WorkFailureType
	Message         string
	ProviderSession *interfaces.ProviderSessionMetadata
}

// ParseProviderFailure returns one canonical Cursor-owned failure result.
// Structured terminal result records take precedence over unstructured output.
func ParseProviderFailure(input FailureInput) FailureResult {
	if payload, ok := terminalFailurePayload(input.Stdout); ok {
		return failureResultFromPayload(payload)
	}
	return FailureResult{
		Reason:  interfaces.WorkFailureTypeUnknown,
		Message: fmt.Sprintf("cursor exited with code %d", input.ExitCode),
	}
}

func terminalFailurePayload(stdout []byte) (resultPayload, bool) {
	trimmed := strings.TrimSpace(string(stdout))
	if trimmed == "" {
		return resultPayload{}, false
	}

	var payload resultPayload
	if json.Unmarshal([]byte(trimmed), &payload) == nil && payload.Type == ResultTypeResult {
		return payload, payload.Subtype != ResultSubtypeSuccess || payload.IsError
	}

	var terminal resultPayload
	found := false
	for _, line := range splitNonEmptyLines(stdout) {
		var candidate resultPayload
		if json.Unmarshal([]byte(line), &candidate) != nil || candidate.Type != ResultTypeResult {
			continue
		}
		terminal = candidate
		found = true
	}
	if !found || (terminal.Subtype == ResultSubtypeSuccess && !terminal.IsError) {
		return resultPayload{}, false
	}
	return terminal, true
}

func failureResultFromPayload(payload resultPayload) FailureResult {
	reason := classifyTerminalFailure(payload.Subtype, payload.Result)
	message := normalizedSafeFailureText(payload.Result)
	if message == "" {
		message = cursorFailureGuidance(reason)
	}
	return FailureResult{
		Reason:          reason,
		Message:         message,
		ProviderSession: canonicalProviderSession(string(interfaces.ModelProviderCursor), payload.SessionID),
	}
}

func classifyTerminalFailure(subtype, result string) interfaces.WorkFailureType {
	signal := strings.ToLower(strings.Join([]string{subtype, result}, " "))
	switch {
	case containsCursorSignal(signal, "authentication_error", "authentication failed", "login required", "sign in", "unauthorized", "forbidden", "invalid api key", "401", "403"):
		return interfaces.WorkFailureTypeAuthFailure
	case containsCursorSignal(signal, "invalid_request", "bad request", "invalid argument", "invalid configuration", "configuration error", "config error", "model not found", "unsupported model"):
		return interfaces.WorkFailureTypePermanentBadRequest
	case containsCursorSignal(signal, "rate_limit", "rate limit", "throttl", "too many requests", "usage limit", "capacity", "resource exhausted", "quota", "429"):
		return interfaces.WorkFailureTypeThrottled
	case containsCursorSignal(signal, "timeout", "timed out", "deadline exceeded"):
		return interfaces.WorkFailureTypeTimeout
	case containsCursorSignal(signal, "api_error", "server_error", "internal server", "service unavailable", "provider unavailable", "upstream unavailable", "status 500", "status 502", "status 503", "status 504"):
		return interfaces.WorkFailureTypeInternalServerError
	default:
		return interfaces.WorkFailureTypeUnknown
	}
}

func containsCursorSignal(signal string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(signal, needle) {
			return true
		}
	}
	return false
}

func normalizedSafeFailureText(value string) string {
	normalized := strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, value)
	normalized = strings.Join(strings.Fields(normalized), " ")
	if normalized == "" || unsafeCursorFailureTextPattern.MatchString(normalized) {
		return ""
	}
	runes := []rune(normalized)
	if len(runes) <= FailureMessageLimit {
		return normalized
	}
	return string(runes[:FailureMessageLimit]) + "..."
}

func cursorFailureGuidance(reason interfaces.WorkFailureType) string {
	switch reason {
	case interfaces.WorkFailureTypeAuthFailure:
		return cursorAuthFailureMessage
	case interfaces.WorkFailureTypePermanentBadRequest, interfaces.WorkFailureTypeMisconfigured:
		return cursorBadRequestFailureMessage
	case interfaces.WorkFailureTypeThrottled:
		return cursorThrottleFailureMessage
	case interfaces.WorkFailureTypeTimeout:
		return cursorTimeoutFailureMessage
	case interfaces.WorkFailureTypeInternalServerError:
		return cursorServerFailureMessage
	default:
		return cursorUnknownFailureMessage
	}
}
