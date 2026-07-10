package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

type codexStructuredFailure struct {
	Type    string
	Status  int
	Message string
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
		reason := classifyCodexFailure(failure.Type, failure.Status, failure.Message, result.ExitCode)
		return ProviderFailureResult{
			Reason:  reason,
			Message: safeCodexFailureMessage(failure.Message, codexExitFailureMessage(result.ExitCode)),
		}
	}

	if message, ok := lastCodexTextFailure(streams); ok {
		return ProviderFailureResult{
			Reason:  classifyCodexFailure("", 0, message, result.ExitCode),
			Message: safeCodexFailureMessage(message, codexExitFailureMessage(result.ExitCode)),
		}
	}

	return ProviderFailureResult{
		Reason:  classifyCodexFailure("", 0, "", result.ExitCode),
		Message: codexExitFailureMessage(result.ExitCode),
	}
}

func lastCodexStructuredFailure(streams []string) (codexStructuredFailure, bool) {
	var last codexStructuredFailure
	var found bool
	for _, stream := range streams {
		for _, line := range strings.Split(stream, "\n") {
			payload, ok := codexErrorPayload(line)
			if !ok || !strings.HasPrefix(payload, "{") {
				continue
			}
			failure, ok := decodeCodexStructuredFailure(payload)
			if ok {
				last, found = failure, true
			}
		}
	}
	return last, found
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

func lastCodexTextFailure(streams []string) (string, bool) {
	var last string
	for _, stream := range streams {
		for _, line := range strings.Split(stream, "\n") {
			trimmed := strings.TrimSpace(line)
			payload, isError := codexErrorPayload(trimmed)
			if isError && !strings.HasPrefix(payload, "{") {
				last = trimmed
			}
		}
	}
	if last != "" {
		return last, true
	}

	// A single unprefixed line is an existing Codex diagnostic shape (for
	// example transport timeouts). Multi-line output is treated as transcript
	// or header noise unless it contains an explicit ERROR record.
	for _, stream := range streams {
		trimmed := strings.TrimSpace(stream)
		if trimmed != "" && !strings.Contains(trimmed, "\n") && classifyCodexFailure("", 0, trimmed, 0) != interfaces.WorkFailureTypeUnknown {
			last = trimmed
		}
	}
	return last, last != ""
}

func classifyCodexFailure(errorType string, status int, message string, exitCode int) interfaces.WorkFailureType {
	if reason, ok := classifyCodexStructuredSignal(errorType, status); ok {
		return reason
	}
	if reason := classifyCodexMessageSignal(message, exitCode); reason != interfaces.WorkFailureTypeUnknown {
		return reason
	}
	if exitCode == codexWindowsProcessFailureExitCode {
		return interfaces.WorkFailureTypeInternalServerError
	}
	return interfaces.WorkFailureTypeUnknown
}

func classifyCodexStructuredSignal(errorType string, status int) (interfaces.WorkFailureType, bool) {
	normalizedType := strings.ToLower(strings.TrimSpace(errorType))
	switch {
	case containsAny(normalizedType, "authentication_error", "permission_error") || status == 401 || status == 403:
		return interfaces.WorkFailureTypeAuthFailure, true
	case containsAny(normalizedType, "invalid_request_error") || status == 400:
		return interfaces.WorkFailureTypePermanentBadRequest, true
	case containsAny(normalizedType, "rate_limit_error", "overloaded_error") || status == 429:
		return interfaces.WorkFailureTypeThrottled, true
	case containsAny(normalizedType, "api_error", "server_error") || status >= 500 && status <= 599:
		return interfaces.WorkFailureTypeInternalServerError, true
	case status == 408:
		return interfaces.WorkFailureTypeTimeout, true
	default:
		return interfaces.WorkFailureTypeUnknown, false
	}
}

func classifyCodexMessageSignal(message string, exitCode int) interfaces.WorkFailureType {
	normalizedMessage := strings.ToLower(message)
	switch {
	case exitCode == 124:
		return interfaces.WorkFailureTypeTimeout
	case containsAny(normalizedMessage, "authentication_error", "api key", "unauthorized", "forbidden", "401 unauthorized", "403 forbidden"):
		return interfaces.WorkFailureTypeAuthFailure
	case containsAny(normalizedMessage, "deadline exceeded", "timed out", "timeout"):
		return interfaces.WorkFailureTypeTimeout
	case containsAny(normalizedMessage, "invalid_request_error", "bad request", "400 item", "400 previous response", "400 "):
		return interfaces.WorkFailureTypePermanentBadRequest
	case containsAny(normalizedMessage, codexThrottledFailureNeedles...):
		return interfaces.WorkFailureTypeThrottled
	case containsAny(normalizedMessage, codexTemporaryServerFailureNeedles...):
		return interfaces.WorkFailureTypeInternalServerError
	default:
		return interfaces.WorkFailureTypeUnknown
	}
}

func safeCodexFailureMessage(message string, fallback string) string {
	message = strings.Join(strings.Fields(message), " ")
	if message == "" || containsSensitiveCodexFailureText(message) {
		return fallback
	}
	if len(message) <= codexFailureMessageBytes {
		return message
	}
	message = message[:codexFailureMessageBytes]
	for message != "" && !utf8.ValidString(message) {
		message = message[:len(message)-1]
	}
	return strings.TrimSpace(message)
}

func containsSensitiveCodexFailureText(message string) bool {
	normalized := strings.ToLower(message)
	return containsAny(normalized, "authorization:", "bearer ", "x-api-key", "api_key", "api-key", "sk-")
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
