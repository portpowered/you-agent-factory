package opencode

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"

	workerprocess "github.com/portpowered/infinite-you/pkg/workers/process"
	"github.com/portpowered/infinite-you/pkg/workers/provider/adapter"
)

var errMissingFinalSnapshot = errors.New("OpenCode structured output did not contain an authoritative response")

type structuredTerminalError struct {
	failureType workerexecution.WorkFailureType
	message     string
	retryable   bool
}

func (e *structuredTerminalError) Error() string { return e.message }

func (a *NegotiatedAdapter) ParseFinal(ctx context.Context, input adapter.FinalParseContext) (adapter.FinalParseResult, error) {
	if a.decision.Mode == ModeFinalOnly {
		return parseFinalOnly(ctx, input)
	}
	if a.requireStructured && unsupportedStructuredProcessRejection(input.CommandResult, input.CommandError) {
		return adapter.FinalParseResult{}, terminalErrorForType(workerexecution.WorkFailureTypePermanentBadRequest)
	}
	parsed, err := parseStructuredFinal(input.CommandResult.Stdout)
	if err != nil {
		return adapter.FinalParseResult{}, err
	}
	if input.CommandError != nil || input.CommandResult.ExitCode != 0 {
		return adapter.FinalParseResult{}, processTerminalError(input.CommandResult, input.CommandError)
	}
	return adapter.FinalParseResult{Response: workerexecution.InferenceResponse{
		Content: parsed.content, ProviderSession: providerSession(parsed.sessionID),
	}}, nil
}

func unsupportedStructuredProcessRejection(result workerprocess.CommandResult, commandErr error) bool {
	return commandErr == nil && result.ExitCode != 0 && len(bytes.TrimSpace(result.Stdout)) == 0 &&
		len(result.Stderr) > 0 && len(result.Stderr) <= maxUnsupportedRejectionBytes &&
		positiveUnsupportedFormatSignal(strings.ToLower(strings.Join(strings.Fields(string(result.Stderr)), " ")))
}

type parsedStructuredFinal struct {
	content   string
	sessionID string
}

func parseStructuredFinal(stdout []byte) (parsedStructuredFinal, error) {
	var parsed parsedStructuredFinal
	var deltas strings.Builder
	var snapshots strings.Builder
	seenSnapshotParts := make(map[string]struct{})
	hasSnapshot := false
	for _, raw := range splitAuthoritativeStructuredLines(stdout) {
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
				hasSnapshot = true
				partID := strings.TrimSpace(record.Part.ID)
				if partID != "" {
					if _, seen := seenSnapshotParts[partID]; seen {
						continue
					}
					seenSnapshotParts[partID] = struct{}{}
				}
				snapshots.WriteString(record.Part.Text)
			} else if !hasSnapshot {
				deltas.WriteString(record.Part.Text)
			}
		}
	}
	if hasSnapshot {
		parsed.content = snapshots.String()
	} else {
		parsed.content = deltas.String()
	}
	if strings.TrimSpace(parsed.content) == "" {
		return parsedStructuredFinal{}, errMissingFinalSnapshot
	}
	return parsed, nil
}

func splitStructuredLines(stdout []byte) [][]byte {
	return splitNonEmptyStructuredLines(stdout, maxStructuredRecordBytes)
}

// splitAuthoritativeStructuredLines deliberately has no publication-oriented
// record limit. OpenCode emits each completed text part as one JSON line, so
// the final response parser must preserve valid results larger than the
// incremental decoder's bounded observation record.
func splitAuthoritativeStructuredLines(stdout []byte) [][]byte {
	return splitNonEmptyStructuredLines(stdout, 0)
}

func splitNonEmptyStructuredLines(stdout []byte, maximumBytes int) [][]byte {
	normalized := bytes.ReplaceAll(stdout, []byte("\r\n"), []byte("\n"))
	lines := bytes.Split(normalized, []byte("\n"))
	result := make([][]byte, 0, len(lines))
	for _, line := range lines {
		if trimmed := bytes.TrimSpace(line); len(trimmed) > 0 && (maximumBytes == 0 || len(trimmed) <= maximumBytes) {
			result = append(result, trimmed)
		}
	}
	return result
}

func classifyStructuredError(name string, data json.RawMessage) *structuredTerminalError {
	failureType := failureTypeFromStatus(structuredErrorStatus(data))
	if failureType == workerexecution.WorkFailureTypeUnknown {
		failureType = failureTypeFromSignal(name)
	}
	return terminalErrorForType(failureType)
}

func failureTypeFromStatus(status int) workerexecution.WorkFailureType {
	switch status {
	case 401, 403:
		return workerexecution.WorkFailureTypeAuthFailure
	case 400, 422:
		return workerexecution.WorkFailureTypePermanentBadRequest
	case 429:
		return workerexecution.WorkFailureTypeThrottled
	case 408, 504:
		return workerexecution.WorkFailureTypeTimeout
	default:
		if status >= 500 {
			return workerexecution.WorkFailureTypeInternalServerError
		}
		return workerexecution.WorkFailureTypeUnknown
	}
}

func failureTypeFromSignal(name string) workerexecution.WorkFailureType {
	signal := strings.ToLower(strings.TrimSpace(name))
	switch {
	case containsSignal(signal, "auth", "unauthorized"):
		return workerexecution.WorkFailureTypeAuthFailure
	case containsSignal(signal, "invalid", "badrequest"):
		return workerexecution.WorkFailureTypePermanentBadRequest
	case containsSignal(signal, "rate", "throttl", "capacity"):
		return workerexecution.WorkFailureTypeThrottled
	case containsSignal(signal, "timeout", "deadline"):
		return workerexecution.WorkFailureTypeTimeout
	case containsSignal(signal, "server", "apierror"):
		return workerexecution.WorkFailureTypeInternalServerError
	default:
		return workerexecution.WorkFailureTypeUnknown
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

func terminalErrorForType(failureType workerexecution.WorkFailureType) *structuredTerminalError {
	switch failureType {
	case workerexecution.WorkFailureTypeAuthFailure:
		return &structuredTerminalError{failureType: workerexecution.WorkFailureTypeAuthFailure, message: "OpenCode authentication failed."}
	case workerexecution.WorkFailureTypePermanentBadRequest:
		return &structuredTerminalError{failureType: workerexecution.WorkFailureTypePermanentBadRequest, message: "OpenCode rejected the request as invalid."}
	case workerexecution.WorkFailureTypeThrottled:
		return &structuredTerminalError{failureType: workerexecution.WorkFailureTypeThrottled, message: "OpenCode is temporarily unavailable due to usage or capacity limits.", retryable: true}
	case workerexecution.WorkFailureTypeTimeout:
		return &structuredTerminalError{failureType: workerexecution.WorkFailureTypeTimeout, message: "OpenCode request timed out.", retryable: true}
	case workerexecution.WorkFailureTypeInternalServerError:
		return &structuredTerminalError{failureType: workerexecution.WorkFailureTypeInternalServerError, message: "OpenCode encountered a temporary server error.", retryable: true}
	default:
		return &structuredTerminalError{failureType: workerexecution.WorkFailureTypeUnknown, message: "OpenCode reported a structured execution failure."}
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
		return &structuredTerminalError{failureType: workerexecution.WorkFailureTypeTimeout, message: "OpenCode request was canceled or timed out.", retryable: true}
	}
	if failureType := classifyProcessFailure(result); failureType != workerexecution.WorkFailureTypeUnknown {
		return terminalErrorForType(failureType)
	}
	return &structuredTerminalError{
		failureType: workerexecution.WorkFailureTypeUnknown,
		message:     fmt.Sprintf("OpenCode execution exited with code %d.", result.ExitCode),
	}
}

func classifyProcessFailure(result workerprocess.CommandResult) workerexecution.WorkFailureType {
	if result.ExitCode == 124 {
		return workerexecution.WorkFailureTypeTimeout
	}
	streams := [][]byte{boundedFailureTail(result.Stderr), boundedFailureTail(result.Stdout)}
	for _, stream := range streams {
		for _, line := range splitStructuredLines(stream) {
			record, err := decodeStructuredRecord(line)
			if err != nil || record.Type != "error" {
				continue
			}
			if failureType := classifyStructuredError(record.Error.Name, record.Error.Data).failureType; failureType != workerexecution.WorkFailureTypeUnknown {
				return failureType
			}
		}
	}
	for _, stream := range streams {
		if failureType := classifyProcessTextFailure(string(stream)); failureType != workerexecution.WorkFailureTypeUnknown {
			return failureType
		}
	}
	return workerexecution.WorkFailureTypeUnknown
}

func boundedFailureTail(output []byte) []byte {
	const maximumFailureScanBytes = 64 * 1024
	if len(output) <= maximumFailureScanBytes {
		return output
	}
	return output[len(output)-maximumFailureScanBytes:]
}

func classifyProcessTextFailure(output string) workerexecution.WorkFailureType {
	for _, line := range strings.Split(output, "\n") {
		trimmed := strings.TrimSpace(line)
		normalized := strings.ToLower(trimmed)
		if !strings.HasPrefix(normalized, "error:") && !strings.HasPrefix(normalized, "api error:") {
			continue
		}
		if failureType := classifyRecognizedProcessText(normalized); failureType != workerexecution.WorkFailureTypeUnknown {
			return failureType
		}
	}
	trimmed := strings.TrimSpace(output)
	if trimmed == "" || strings.Contains(trimmed, "\n") {
		return workerexecution.WorkFailureTypeUnknown
	}
	return classifyRecognizedProcessText(strings.ToLower(strings.Join(strings.Fields(trimmed), " ")))
}

func classifyRecognizedProcessText(normalized string) workerexecution.WorkFailureType {
	switch {
	case containsSignal(normalized, "deadline exceeded", "request timed out", "timed out", "timeout"):
		return workerexecution.WorkFailureTypeTimeout
	case containsSignal(normalized, "authentication", "login required", "not authenticated", "unauthorized", "forbidden", "api key"):
		return workerexecution.WorkFailureTypeAuthFailure
	case containsSignal(normalized, "invalid request", "bad request", "invalid argument", "model not found"):
		return workerexecution.WorkFailureTypePermanentBadRequest
	case containsSignal(normalized, "rate limit", "too many requests", "usage limit", "at capacity", "status 429"):
		return workerexecution.WorkFailureTypeThrottled
	case containsSignal(normalized, "internal server error", "server error", "status 500", "status 502", "status 503", "status 504"):
		return workerexecution.WorkFailureTypeInternalServerError
	default:
		return workerexecution.WorkFailureTypeUnknown
	}
}

func providerSession(sessionID string) *workerexecution.ProviderSessionMetadata {
	if !validCorrelation(sessionID) {
		return nil
	}
	return &workerexecution.ProviderSessionMetadata{Provider: "opencode", Kind: providerSessionKind, ID: sessionID}
}

func (a *NegotiatedAdapter) ClassifyFailure(_ context.Context, input adapter.FailureContext) adapter.FailureResult {
	if input.FlushReason == adapter.FlushReasonCanceled {
		return failureResult(&structuredTerminalError{
			failureType: workerexecution.WorkFailureTypeTimeout, message: "OpenCode request was canceled or timed out.", retryable: true,
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
			failureType: workerexecution.WorkFailureTypeUnknown, message: "OpenCode output could not be processed.",
		}, providerSessionFromOutput(input.CommandResult.Stdout))
	}
	if input.CommandResult.ExitCode != 0 {
		return failureResult(processTerminalError(input.CommandResult, nil), providerSessionFromOutput(input.CommandResult.Stdout))
	}
	return adapter.FailureResult{}
}

func failureResult(terminal *structuredTerminalError, session *workerexecution.ProviderSessionMetadata) adapter.FailureResult {
	family := workerexecution.WorkFailureFamilyTerminal
	if terminal.retryable {
		family = workerexecution.WorkFailureFamilyRetryable
	}
	if terminal.failureType == workerexecution.WorkFailureTypeThrottled {
		family = workerexecution.WorkFailureFamilyThrottle
	}
	return adapter.FailureResult{Failure: &adapter.FailureFacts{
		Family: family, Type: terminal.failureType, Message: terminal.message,
		Retry: adapter.RetryGuidance{Retryable: terminal.retryable}, ProviderSession: session,
	}}
}

func providerSessionFromOutput(stdout []byte) *workerexecution.ProviderSessionMetadata {
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
