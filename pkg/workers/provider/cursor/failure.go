package cursor

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"unicode"

	modelprovider "github.com/portpowered/infinite-you/pkg/models/provider"

	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"
)

const FailureMessageLimit = 1024

const (
	cursorAuthFailureMessage        = "Cursor authentication failed. Sign in again or check the configured credentials."
	cursorBadRequestFailureMessage  = "Cursor rejected the request as invalid. Check the model and Cursor configuration."
	cursorThrottleFailureMessage    = "Cursor is temporarily unavailable due to usage or capacity limits."
	cursorTimeoutFailureMessage     = "Cursor request timed out."
	cursorServerFailureMessage      = "Cursor encountered a temporary server error."
	cursorCommandLineTooLongMessage = "Cursor could not start because the rendered command exceeded the operating system command-line limit."
	cursorUnknownFailureMessage     = "Cursor reported an unsuccessful result."
)

var unsafeCursorFailureTextPattern = regexp.MustCompile(`(?i)(authorization\s*:|bearer\s+\S+|api[_ -]?key\s*[:=]\s*\S+|password\s*[:=]|secret\s*[:=]|token\s*[:=]|private prompt|user prompt|complete transcript|full transcript|cleanup noise|could not be terminated|failed to terminate process)`)

// FailureInput contains the bounded subprocess surfaces used by Cursor's pure
// failure parser. Runtime policy is intentionally not part of this contract.
type FailureInput struct {
	Stdout          []byte
	Stderr          []byte
	ExitCode        int
	FallbackReason  workerexecution.WorkFailureType
	FallbackMessage string
}

// FailureResult is Cursor's canonical failure reason and customer-visible
// message. Provider runtime policy is derived centrally from Reason.
type FailureResult struct {
	Reason          workerexecution.WorkFailureType
	Message         string
	ProviderSession *workerexecution.ProviderSessionMetadata
}

// ParseProviderFailure returns one canonical Cursor-owned failure result.
// Structured terminal result records take precedence over unstructured output.
func ParseProviderFailure(input FailureInput) FailureResult {
	if payload, ok := terminalFailurePayload(input.Stdout); ok {
		return failureResultFromPayload(payload)
	}
	if result, ok := failureResultFromText(input.Stderr, input.FallbackReason); ok {
		return result
	}
	if result, ok := failureResultFromText(input.Stdout, input.FallbackReason); ok {
		return result
	}
	if message := normalizedSafeFailureText(input.FallbackMessage); message != "" {
		return FailureResult{Reason: normalizedFallbackReason(input.FallbackReason), Message: message}
	}
	if reason := normalizedFallbackReason(input.FallbackReason); reason != workerexecution.WorkFailureTypeUnknown {
		return FailureResult{Reason: reason, Message: cursorFailureGuidance(reason)}
	}
	return FailureResult{
		Reason:  normalizedFallbackReason(input.FallbackReason),
		Message: fmt.Sprintf("cursor exited with code %d", input.ExitCode),
	}
}

func failureResultFromText(output []byte, fallbackReason workerexecution.WorkFailureType) (FailureResult, bool) {
	candidates := cursorFailureTextCandidates(output)
	if len(candidates) == 0 {
		return FailureResult{}, false
	}

	for _, reason := range []workerexecution.WorkFailureType{
		workerexecution.WorkFailureTypeCommandLineTooLong,
		workerexecution.WorkFailureTypeAuthFailure,
		workerexecution.WorkFailureTypePermanentBadRequest,
		workerexecution.WorkFailureTypeThrottled,
		workerexecution.WorkFailureTypeTimeout,
		workerexecution.WorkFailureTypeInternalServerError,
	} {
		for _, candidate := range candidates {
			if classifyCursorFailureSignal(candidate.normalized) == reason {
				message := candidate.safe
				if message == "" {
					message = cursorFailureGuidance(reason)
				}
				return FailureResult{Reason: reason, Message: message}, true
			}
		}
	}

	for _, candidate := range candidates {
		if candidate.safe != "" {
			return FailureResult{Reason: normalizedFallbackReason(fallbackReason), Message: candidate.safe}, true
		}
	}
	return FailureResult{}, false
}

type cursorFailureTextCandidate struct {
	normalized string
	safe       string
}

func cursorFailureTextCandidates(output []byte) []cursorFailureTextCandidate {
	seen := make(map[string]struct{})
	candidates := make([]cursorFailureTextCandidate, 0)
	for _, rawLine := range splitNonEmptyLines(output) {
		if isCursorStructuredRecord(rawLine) {
			continue
		}
		normalized := normalizedCursorFailureText(rawLine)
		if normalized == "" || isCursorCleanupNoise(normalized) {
			continue
		}
		key := strings.ToLower(normalized)
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}
		candidates = append(candidates, cursorFailureTextCandidate{
			normalized: normalized,
			safe:       normalizedSafeFailureText(normalized),
		})
	}
	return candidates
}

func isCursorStructuredRecord(line string) bool {
	trimmed := strings.TrimSpace(line)
	if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
		return true
	}
	var record any
	return json.Unmarshal([]byte(trimmed), &record) == nil
}

func isCursorCleanupNoise(line string) bool {
	normalized := strings.ToLower(line)
	return containsCursorSignal(normalized,
		"cleanup noise",
		"could not be terminated",
		"failed to terminate process",
		"process already exited",
	)
}

func normalizedFallbackReason(reason workerexecution.WorkFailureType) workerexecution.WorkFailureType {
	if reason == "" {
		return workerexecution.WorkFailureTypeUnknown
	}
	return reason
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
		ProviderSession: canonicalProviderSession(string(modelprovider.Cursor), payload.SessionID),
	}
}

func classifyTerminalFailure(subtype, result string) workerexecution.WorkFailureType {
	signal := strings.ToLower(strings.Join([]string{subtype, result}, " "))
	return classifyCursorFailureSignal(signal)
}

func classifyCursorFailureSignal(signal string) workerexecution.WorkFailureType {
	signal = strings.ToLower(signal)
	switch {
	case containsCursorSignal(signal, "the command line is too long", "command line too long", "command-line limit"):
		return workerexecution.WorkFailureTypeCommandLineTooLong
	case containsCursorSignal(signal, "authentication_error", "authentication failed", "authorization", "login required", "sign in", "unauthorized", "forbidden", "invalid api key", "401", "403"):
		return workerexecution.WorkFailureTypeAuthFailure
	case containsCursorSignal(signal, "invalid_request", "bad request", "invalid argument", "invalid configuration", "configuration error", "config error", "model not found", "unsupported model"):
		return workerexecution.WorkFailureTypePermanentBadRequest
	case containsCursorSignal(signal, "rate_limit", "rate limit", "throttl", "too many requests", "usage limit", "capacity", "resource exhausted", "quota", "429"):
		return workerexecution.WorkFailureTypeThrottled
	case containsCursorSignal(signal, "timeout", "timed out", "deadline exceeded"):
		return workerexecution.WorkFailureTypeTimeout
	case containsCursorSignal(signal, "api_error", "server_error", "internal server", "service unavailable", "provider unavailable", "upstream unavailable", "status 500", "status 502", "status 503", "status 504"):
		return workerexecution.WorkFailureTypeInternalServerError
	default:
		return workerexecution.WorkFailureTypeUnknown
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
	normalized := normalizedCursorFailureText(value)
	if normalized == "" || unsafeCursorFailureTextPattern.MatchString(normalized) {
		return ""
	}
	runes := []rune(normalized)
	if len(runes) <= FailureMessageLimit {
		return normalized
	}
	return string(runes[:FailureMessageLimit]) + "..."
}

func normalizedCursorFailureText(value string) string {
	normalized := strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, value)
	normalized = strings.Join(strings.Fields(normalized), " ")
	return normalized
}

func cursorFailureGuidance(reason workerexecution.WorkFailureType) string {
	switch reason {
	case workerexecution.WorkFailureTypeAuthFailure:
		return cursorAuthFailureMessage
	case workerexecution.WorkFailureTypePermanentBadRequest, workerexecution.WorkFailureTypeMisconfigured:
		return cursorBadRequestFailureMessage
	case workerexecution.WorkFailureTypeThrottled:
		return cursorThrottleFailureMessage
	case workerexecution.WorkFailureTypeTimeout:
		return cursorTimeoutFailureMessage
	case workerexecution.WorkFailureTypeInternalServerError:
		return cursorServerFailureMessage
	case workerexecution.WorkFailureTypeCommandLineTooLong:
		return cursorCommandLineTooLongMessage
	default:
		return cursorUnknownFailureMessage
	}
}
