package opencode

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	workerprocess "github.com/portpowered/infinite-you/pkg/workers/process"
	"github.com/portpowered/infinite-you/pkg/workers/provider/adapter"
)

var errMissingFinalSnapshot = errors.New("OpenCode structured output did not contain an authoritative response")

type structuredTerminalError struct {
	failureType interfaces.WorkFailureType
	message     string
	retryable   bool
}

func (e *structuredTerminalError) Error() string { return e.message }

func (a *NegotiatedAdapter) ParseFinal(ctx context.Context, input adapter.FinalParseContext) (adapter.FinalParseResult, error) {
	if a.decision.Mode == ModeFinalOnly {
		return parseFinalOnly(ctx, input)
	}
	parsed, err := parseStructuredFinal(input.CommandResult.Stdout)
	if err != nil {
		return adapter.FinalParseResult{}, err
	}
	if input.CommandError != nil || input.CommandResult.ExitCode != 0 {
		return adapter.FinalParseResult{}, processTerminalError(input.CommandResult, input.CommandError)
	}
	return adapter.FinalParseResult{Response: interfaces.InferenceResponse{
		Content: parsed.content, ProviderSession: providerSession(parsed.sessionID),
	}}, nil
}

type parsedStructuredFinal struct {
	content   string
	sessionID string
}

func parseStructuredFinal(stdout []byte) (parsedStructuredFinal, error) {
	var parsed parsedStructuredFinal
	var deltas strings.Builder
	hasSnapshot := false
	for _, raw := range splitStructuredLines(stdout) {
		record, err := decodeStructuredRecord(raw)
		if err != nil || record.Type == "" {
			continue
		}
		if sessionID := recordSessionID(record); sessionID != "" {
			parsed.sessionID = sessionID
		}
		switch record.Type {
		case "error":
			return parsedStructuredFinal{}, classifyStructuredError(record.Error.Name, record.Error.Data)
		case "text":
			if strings.TrimSpace(record.Part.Text) == "" {
				continue
			}
			if record.Part.Time.End != nil {
				parsed.content = boundedText(record.Part.Text)
				hasSnapshot = true
			} else if !hasSnapshot {
				deltas.WriteString(record.Part.Text)
			}
		}
	}
	if !hasSnapshot {
		parsed.content = boundedText(deltas.String())
	}
	if strings.TrimSpace(parsed.content) == "" {
		return parsedStructuredFinal{}, errMissingFinalSnapshot
	}
	return parsed, nil
}

func splitStructuredLines(stdout []byte) [][]byte {
	normalized := bytes.ReplaceAll(stdout, []byte("\r\n"), []byte("\n"))
	lines := bytes.Split(normalized, []byte("\n"))
	result := make([][]byte, 0, len(lines))
	for _, line := range lines {
		if trimmed := bytes.TrimSpace(line); len(trimmed) > 0 && len(trimmed) <= maxStructuredRecordBytes {
			result = append(result, trimmed)
		}
	}
	return result
}

func classifyStructuredError(name string, data json.RawMessage) *structuredTerminalError {
	failureType := failureTypeFromStatus(structuredErrorStatus(data))
	if failureType == interfaces.WorkFailureTypeUnknown {
		failureType = failureTypeFromSignal(name)
	}
	return terminalErrorForType(failureType)
}

func failureTypeFromStatus(status int) interfaces.WorkFailureType {
	switch status {
	case 401, 403:
		return interfaces.WorkFailureTypeAuthFailure
	case 400, 422:
		return interfaces.WorkFailureTypePermanentBadRequest
	case 429:
		return interfaces.WorkFailureTypeThrottled
	case 408, 504:
		return interfaces.WorkFailureTypeTimeout
	default:
		if status >= 500 {
			return interfaces.WorkFailureTypeInternalServerError
		}
		return interfaces.WorkFailureTypeUnknown
	}
}

func failureTypeFromSignal(name string) interfaces.WorkFailureType {
	signal := strings.ToLower(strings.TrimSpace(name))
	switch {
	case containsSignal(signal, "auth", "unauthorized"):
		return interfaces.WorkFailureTypeAuthFailure
	case containsSignal(signal, "invalid", "badrequest"):
		return interfaces.WorkFailureTypePermanentBadRequest
	case containsSignal(signal, "rate", "throttl", "capacity"):
		return interfaces.WorkFailureTypeThrottled
	case containsSignal(signal, "timeout", "deadline"):
		return interfaces.WorkFailureTypeTimeout
	case containsSignal(signal, "server", "apierror"):
		return interfaces.WorkFailureTypeInternalServerError
	default:
		return interfaces.WorkFailureTypeUnknown
	}
}

func containsSignal(signal string, candidates ...string) bool {
	for _, candidate := range candidates {
		if strings.Contains(signal, candidate) {
			return true
		}
	}
	return false
}

func terminalErrorForType(failureType interfaces.WorkFailureType) *structuredTerminalError {
	switch failureType {
	case interfaces.WorkFailureTypeAuthFailure:
		return &structuredTerminalError{failureType: interfaces.WorkFailureTypeAuthFailure, message: "OpenCode authentication failed."}
	case interfaces.WorkFailureTypePermanentBadRequest:
		return &structuredTerminalError{failureType: interfaces.WorkFailureTypePermanentBadRequest, message: "OpenCode rejected the request as invalid."}
	case interfaces.WorkFailureTypeThrottled:
		return &structuredTerminalError{failureType: interfaces.WorkFailureTypeThrottled, message: "OpenCode is temporarily unavailable due to usage or capacity limits.", retryable: true}
	case interfaces.WorkFailureTypeTimeout:
		return &structuredTerminalError{failureType: interfaces.WorkFailureTypeTimeout, message: "OpenCode request timed out.", retryable: true}
	case interfaces.WorkFailureTypeInternalServerError:
		return &structuredTerminalError{failureType: interfaces.WorkFailureTypeInternalServerError, message: "OpenCode encountered a temporary server error.", retryable: true}
	default:
		return &structuredTerminalError{failureType: interfaces.WorkFailureTypeUnknown, message: "OpenCode reported a structured execution failure."}
	}
}

func structuredErrorStatus(data json.RawMessage) int {
	if len(data) == 0 {
		return 0
	}
	var fields struct {
		Status     int `json:"status"`
		StatusCode int `json:"statusCode"`
	}
	if err := json.Unmarshal(data, &fields); err != nil {
		return 0
	}
	if fields.Status != 0 {
		return fields.Status
	}
	return fields.StatusCode
}

func processTerminalError(result workerprocess.CommandResult, commandErr error) *structuredTerminalError {
	if errors.Is(commandErr, context.Canceled) || errors.Is(commandErr, context.DeadlineExceeded) {
		return &structuredTerminalError{failureType: interfaces.WorkFailureTypeTimeout, message: "OpenCode request was canceled or timed out.", retryable: true}
	}
	return &structuredTerminalError{
		failureType: interfaces.WorkFailureTypeUnknown,
		message:     fmt.Sprintf("OpenCode execution exited with code %d.", result.ExitCode),
	}
}

func providerSession(sessionID string) *interfaces.ProviderSessionMetadata {
	if !validCorrelation(sessionID) {
		return nil
	}
	return &interfaces.ProviderSessionMetadata{Provider: "opencode", Kind: providerSessionKind, ID: sessionID}
}

func (a *NegotiatedAdapter) ClassifyFailure(_ context.Context, input adapter.FailureContext) adapter.FailureResult {
	if input.FlushReason == adapter.FlushReasonCanceled {
		return failureResult(&structuredTerminalError{
			failureType: interfaces.WorkFailureTypeTimeout, message: "OpenCode request was canceled or timed out.", retryable: true,
		}, nil)
	}
	for _, candidate := range []error{input.ParseError, input.DecodeError, input.FlushError, input.CommandError} {
		if candidate == nil {
			continue
		}
		var terminal *structuredTerminalError
		if errors.As(candidate, &terminal) {
			return failureResult(terminal, providerSessionFromOutput(input.CommandResult.Stdout))
		}
		return failureResult(&structuredTerminalError{
			failureType: interfaces.WorkFailureTypeUnknown, message: "OpenCode output could not be processed.",
		}, providerSessionFromOutput(input.CommandResult.Stdout))
	}
	if input.CommandResult.ExitCode != 0 {
		return failureResult(processTerminalError(input.CommandResult, nil), providerSessionFromOutput(input.CommandResult.Stdout))
	}
	return adapter.FailureResult{}
}

func failureResult(terminal *structuredTerminalError, session *interfaces.ProviderSessionMetadata) adapter.FailureResult {
	family := interfaces.WorkFailureFamilyTerminal
	if terminal.retryable {
		family = interfaces.WorkFailureFamilyRetryable
	}
	if terminal.failureType == interfaces.WorkFailureTypeThrottled {
		family = interfaces.WorkFailureFamilyThrottle
	}
	return adapter.FailureResult{Failure: &adapter.FailureFacts{
		Family: family, Type: terminal.failureType, Message: terminal.message,
		Retry: adapter.RetryGuidance{Retryable: terminal.retryable}, ProviderSession: session,
	}}
}

func providerSessionFromOutput(stdout []byte) *interfaces.ProviderSessionMetadata {
	var latest string
	for _, raw := range splitStructuredLines(stdout) {
		record, err := decodeStructuredRecord(raw)
		if err == nil {
			if sessionID := recordSessionID(record); sessionID != "" {
				latest = sessionID
			}
		}
	}
	return providerSession(latest)
}
