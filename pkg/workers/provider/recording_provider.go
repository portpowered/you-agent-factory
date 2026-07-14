package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

const (
	inferenceRequestEventIDPrefix  = "factory-event/inference-request"
	inferenceResponseEventIDPrefix = "factory-event/inference-response"
)

// InferenceEventRecorder receives generated provider-boundary inference events.
type InferenceEventRecorder func(factoryapi.FactoryEvent)

// RecordingProvider wraps a Provider and emits inference request/response events
// around each delegated provider call.
type RecordingProvider struct {
	inner    Provider
	recorder InferenceEventRecorder
	now      func() time.Time

	mu       sync.Mutex
	attempts map[string]int
}

// RecordingProviderOption configures a RecordingProvider.
type RecordingProviderOption func(*RecordingProvider)

// WithRecordingProviderClock sets the clock used for event occurrence times and
// provider-call duration measurement.
func WithRecordingProviderClock(now func() time.Time) RecordingProviderOption {
	return func(p *RecordingProvider) {
		if now != nil {
			p.now = now
		}
	}
}

// NewRecordingProvider creates a Provider wrapper that records generated
// inference events before and after calls to inner.
func NewRecordingProvider(inner Provider, recorder InferenceEventRecorder, opts ...RecordingProviderOption) *RecordingProvider {
	provider := &RecordingProvider{
		inner:    inner,
		recorder: recorder,
		now:      time.Now,
		attempts: make(map[string]int),
	}
	for _, opt := range opts {
		opt(provider)
	}
	return provider
}

// Infer records a request event, delegates to the wrapped provider, then records
// the matching response event with success or failure details.
func (p *RecordingProvider) Infer(ctx context.Context, req interfaces.ProviderInferenceRequest) (interfaces.InferenceResponse, error) {
	attempt := p.nextAttempt(req.Dispatch.DispatchID)
	inferenceRequestID := inferenceRequestID(req.Dispatch.DispatchID, attempt)
	started := p.now()
	p.record(inferenceRequestEvent(req, attempt, inferenceRequestID, started))

	resp, err := p.inferInner(ctx, req)

	ended := p.now()
	p.record(inferenceResponseEvent(req, resp, err, attempt, inferenceRequestID, ended.Sub(started), ended))
	if err == nil || !isRetryableProviderFailure(err) {
		p.clearAttempts(req.Dispatch.DispatchID)
	}
	return resp, err
}

func (p *RecordingProvider) inferInner(ctx context.Context, req interfaces.ProviderInferenceRequest) (interfaces.InferenceResponse, error) {
	if p.inner == nil {
		return interfaces.InferenceResponse{}, NewProviderError(
			interfaces.WorkFailureTypeMisconfigured,
			"recording provider requires an inner provider",
			nil,
		)
	}
	return p.inner.Infer(ctx, req)
}

func (p *RecordingProvider) record(event factoryapi.FactoryEvent) {
	if p.recorder != nil {
		p.recorder(event)
	}
}

func (p *RecordingProvider) nextAttempt(dispatchID string) int {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.attempts[dispatchID]++
	return p.attempts[dispatchID]
}

func (p *RecordingProvider) clearAttempts(dispatchID string) {
	p.mu.Lock()
	delete(p.attempts, dispatchID)
	p.mu.Unlock()
}

func inferenceRequestID(dispatchID string, attempt int) string {
	if dispatchID == "" {
		return fmt.Sprintf("inference-request/%d", attempt)
	}
	return fmt.Sprintf("%s/inference-request/%d", dispatchID, attempt)
}

func inferenceRequestEvent(req interfaces.ProviderInferenceRequest, attempt int, inferenceRequestID string, eventTime time.Time) factoryapi.FactoryEvent {
	payload := factoryapi.InferenceRequestEventPayload{
		InferenceRequestId: inferenceRequestID,
		Attempt:            attempt,
		WorkingDirectory:   req.WorkingDirectory,
		Worktree:           req.Worktree,
		Prompt:             req.UserMessage,
	}
	return factoryapi.FactoryEvent{
		SchemaVersion: factoryapi.AgentFactoryEventV1,
		Type:          factoryapi.FactoryEventTypeInferenceRequest,
		Id:            fmt.Sprintf("%s/%s", inferenceRequestEventIDPrefix, inferenceRequestID),
		Context:       inferenceEventContext(req, eventTime),
		Payload:       inferenceRequestFactoryEventPayload(payload),
	}
}

func inferenceResponseEvent(req interfaces.ProviderInferenceRequest, resp interfaces.InferenceResponse, err error, attempt int, inferenceRequestID string, duration time.Duration, eventTime time.Time) factoryapi.FactoryEvent {
	payload := factoryapi.InferenceResponseEventPayload{
		InferenceRequestId: inferenceRequestID,
		Attempt:            attempt,
		DurationMillis:     duration.Milliseconds(),
	}
	baseDiagnostics := workDiagnosticsForInferenceRequest(req)
	if err != nil {
		payload.Outcome = factoryapi.InferenceOutcomeFailed
		payload.FailureDetail = providerFailureDetail(err)
		payload.ExitCode = providerErrorExitCode(err)
		payload.ProviderSession = interfaces.GeneratedProviderSessionMetadata(providerSessionFromInferenceError(err))
		payload.Diagnostics = interfaces.GeneratedSafeWorkDiagnosticsFromWorkDiagnostics(
			mergeWorkDiagnostics(
				withInferenceErrorDiagnostics(baseDiagnostics, err, attempt-1),
				diagnosticsFromInferenceError(err),
			),
		)
	} else {
		payload.Outcome = factoryapi.InferenceOutcomeSucceeded
		payload.Response = stringPtr(resp.Content)
		payload.ProviderSession = interfaces.GeneratedProviderSessionMetadata(resp.ProviderSession)
		payload.Diagnostics = interfaces.GeneratedSafeWorkDiagnosticsFromWorkDiagnostics(
			withInferenceResponseDiagnostics(baseDiagnostics, resp, attempt-1),
		)
	}
	return factoryapi.FactoryEvent{
		SchemaVersion: factoryapi.AgentFactoryEventV1,
		Type:          factoryapi.FactoryEventTypeInferenceResponse,
		Id:            fmt.Sprintf("%s/%s", inferenceResponseEventIDPrefix, inferenceRequestID),
		Context:       inferenceEventContext(req, eventTime),
		Payload:       inferenceResponseFactoryEventPayload(payload),
	}
}

func providerFailureDetail(err error) *factoryapi.FailureDetail {
	var providerErr *ProviderError
	if errors.As(err, &providerErr) {
		message := strings.TrimSpace(providerErr.Message)
		if message == "" {
			message = "The provider request failed without an available explanation."
		}
		return &factoryapi.FailureDetail{Reason: factoryapi.WorkFailureType(providerErrorClass(err)), Message: message}
	}
	return &factoryapi.FailureDetail{
		Reason:  factoryapi.WorkFailureTypeUnknown,
		Message: "The provider request failed without an available explanation.",
	}
}

func providerSessionFromInferenceError(err error) *interfaces.ProviderSessionMetadata {
	var providerErr *ProviderError
	if !errors.As(err, &providerErr) {
		return nil
	}
	return providerErr.ProviderSession
}

func diagnosticsFromInferenceError(err error) *interfaces.WorkDiagnostics {
	var providerErr *ProviderError
	if !errors.As(err, &providerErr) {
		return nil
	}
	return providerErr.Diagnostics
}

func inferenceEventContext(req interfaces.ProviderInferenceRequest, eventTime time.Time) factoryapi.FactoryEventContext {
	return factoryapi.FactoryEventContext{
		Tick:       inferenceEventTick(req.Dispatch.Execution),
		EventTime:  interfaces.CanonicalEventTime(eventTime),
		DispatchId: stringPtrIfNotEmpty(req.Dispatch.DispatchID),
		RequestId:  stringPtrIfNotEmpty(req.Dispatch.Execution.RequestID),
		TraceIds:   stringSlicePtr(req.Dispatch.Execution.TraceID),
		WorkIds:    stringSlicePtr(req.Dispatch.Execution.WorkIDs...),
	}
}

func inferenceEventTick(metadata interfaces.ExecutionMetadata) int {
	if metadata.CurrentTick != 0 {
		return metadata.CurrentTick
	}
	return metadata.DispatchCreatedTick
}

func isRetryableProviderFailure(err error) bool {
	var providerErr *ProviderError
	if !errors.As(err, &providerErr) {
		return false
	}
	return WorkFailureDecisionFromProviderError(providerErr).Retryable
}

func providerErrorClass(err error) string {
	var providerErr *ProviderError
	if errors.As(err, &providerErr) && providerErr.Type != "" {
		return string(providerErr.Type)
	}
	return string(interfaces.WorkFailureTypeUnknown)
}

func providerErrorExitCode(err error) *int {
	var providerErr *ProviderError
	if !errors.As(err, &providerErr) || providerErr.Diagnostics == nil || providerErr.Diagnostics.Command == nil {
		return nil
	}
	return workerEventExitCode(
		providerErr.Diagnostics.Command.ExitCode,
		true,
		omitZeroWorkerEventExitCode,
	)
}

func inferenceRequestFactoryEventPayload(payload factoryapi.InferenceRequestEventPayload) factoryapi.FactoryEvent_Payload {
	var out factoryapi.FactoryEvent_Payload
	if err := out.FromInferenceRequestEventPayload(payload); err != nil {
		panic(fmt.Sprintf("inference request event payload: %v", err))
	}
	return out
}

func inferenceResponseFactoryEventPayload(payload factoryapi.InferenceResponseEventPayload) factoryapi.FactoryEvent_Payload {
	var out factoryapi.FactoryEvent_Payload
	if err := out.FromInferenceResponseEventPayload(payload); err != nil {
		panic(fmt.Sprintf("inference response event payload: %v", err))
	}
	return out
}

func stringPtr(value string) *string {
	return &value
}

func stringPtrIfNotEmpty(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func stringSlicePtr(values ...string) *[]string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		out = append(out, value)
	}
	if len(out) == 0 {
		return nil
	}
	return &out
}

var _ Provider = (*RecordingProvider)(nil)

const codexJSONLRecordBytes = 1024 * 1024

const codexStructuredStreamUnknownMessage = codexUnknownFailureMessage

const (
	codexInternalCauseExecutionCanceled      = "execution canceled"
	codexInternalCauseDeadlineExceeded       = "context deadline exceeded"
	codexInternalCauseUnrecognizedStructured = "unrecognized structured stream failure"
)

// CodexFailureResolutionInput carries runtime cancellation and flush facts that
// outrank structured, stderr, and exit signals.
type CodexFailureResolutionInput struct {
	CommandError error
	FlushReason  string
}

const (
	CodexFlushReasonCanceled = "canceled"
)

// ResolveCodexProviderFailure applies the shared structured/stderr/exit
// precedence model to Codex subprocess output and optional runtime facts.
func ResolveCodexProviderFailure(result CommandResult, input CodexFailureResolutionInput) (ProviderFailureResolution, bool) {
	signals := collectCodexFailureSignals(result, input)
	if len(signals) == 0 {
		return ProviderFailureResolution{}, false
	}
	return SelectFailureByPrecedence(signals)
}

func collectCodexFailureSignals(result CommandResult, input CodexFailureResolutionInput) []CompetingFailureSignal {
	var signals []CompetingFailureSignal
	if errors.Is(input.CommandError, context.Canceled) || input.FlushReason == CodexFlushReasonCanceled {
		signals = append(signals, CompetingFailureSignal{
			Tier: FailureSignalTierCancelTimeout,
			Result: ProviderFailureResult{
				Reason:  interfaces.WorkFailureTypeUnknown,
				Message: "Codex execution was canceled.",
			},
			InternalCause: codexInternalCauseExecutionCanceled,
		})
	}
	if errors.Is(input.CommandError, context.DeadlineExceeded) || result.ExitCode == 124 {
		internalCause := codexInternalCauseDeadlineExceeded
		if result.ExitCode == 124 && !errors.Is(input.CommandError, context.DeadlineExceeded) {
			internalCause = codexExitInternalCause(result.ExitCode)
		}
		signals = append(signals, CompetingFailureSignal{
			Tier:       FailureSignalTierCancelTimeout,
			Recognized: true,
			Result: ProviderFailureResult{
				Reason:  interfaces.WorkFailureTypeTimeout,
				Message: "Codex execution timed out.",
			},
			InternalCause: internalCause,
		})
	}
	if failure, internalCause, ok := parseCodexStructuredStreamFailureWithCause(result.Stdout); ok {
		signals = append(signals, CompetingFailureSignal{
			Tier:          FailureSignalTierStructured,
			Recognized:    IsRecognizedFailureType(failure.Reason),
			Result:        failure,
			InternalCause: internalCause,
		})
	}
	structured, structuredCause, hasStructured, stderr, stderrCause, hasStderr, exit := CodexProcessExitFailureLayersWithCause(result)
	if hasStructured {
		signals = append(signals, CompetingFailureSignal{
			Tier:          FailureSignalTierStructured,
			Recognized:    IsRecognizedFailureType(structured.Reason),
			Result:        structured,
			InternalCause: structuredCause,
		})
	}
	if hasStderr {
		signals = append(signals, CompetingFailureSignal{
			Tier:          FailureSignalTierStderr,
			Recognized:    true,
			Result:        stderr,
			InternalCause: stderrCause,
		})
	}
	signals = append(signals, CompetingFailureSignal{
		Tier:          FailureSignalTierExit,
		Recognized:    IsRecognizedFailureType(exit.Reason),
		Result:        exit,
		InternalCause: codexExitInternalCause(result.ExitCode),
	})
	return signals
}

func parseCodexStructuredStreamFailure(stdout []byte) (ProviderFailureResult, bool) {
	failure, _, ok := parseCodexStructuredStreamFailureWithCause(stdout)
	return failure, ok
}

func parseCodexStructuredStreamFailureWithCause(stdout []byte) (ProviderFailureResult, string, bool) {
	var (
		selected             ProviderFailureResult
		selectedCause        string
		found                bool
		selectedRecognized   bool
	)
	forEachCodexJSONLRecord(stdout, func(raw []byte) {
		var record struct {
			Type     string `json:"type"`
			Message  string `json:"message"`
			ThreadID string `json:"thread_id"`
			Error    *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal(raw, &record) != nil {
			return
		}
		var nativeMessage string
		switch record.Type {
		case "turn.failed":
			if record.Error == nil || strings.TrimSpace(record.Error.Message) == "" {
				return
			}
			nativeMessage = record.Error.Message
		case "error":
			if strings.TrimSpace(record.Message) == "" {
				return
			}
			nativeMessage = record.Message
		default:
			return
		}
		failure, recognized := codexStructuredStreamFailureFromMessage(nativeMessage)
		if shouldSelectCodexStructuredStreamFailure(selectedRecognized, recognized) {
			selected = failure
			selectedCause = codexStructuredStreamNativeInternalCause(nativeMessage, recognized)
			found = true
		}
		selectedRecognized = selectedRecognized || recognized
	})
	return selected, selectedCause, found
}

func shouldSelectCodexStructuredStreamFailure(selectedRecognized, candidateRecognized bool) bool {
	return candidateRecognized || !selectedRecognized
}

func codexStructuredStreamFailureFromMessage(message string) (ProviderFailureResult, bool) {
	parsed := ParseCodexProviderFailureLayers(CommandResult{
		ExitCode: 1,
		Stderr:   []byte("ERROR: " + strings.TrimSpace(message)),
	})
	if !IsRecognizedFailureType(parsed.Reason) {
		return ProviderFailureResult{
			Reason:  interfaces.WorkFailureTypeUnknown,
			Message: codexStructuredStreamUnknownMessage,
		}, false
	}
	return parsed, true
}

// ParseCodexProviderFailureLayers returns the recognized structured ERROR-record
// or stderr-classified outcome without applying cross-signal precedence.
func ParseCodexProviderFailureLayers(result CommandResult) ProviderFailureResult {
	structured, hasStructured, stderr, hasStderr, _ := CodexProcessExitFailureLayers(result)
	if hasStructured {
		return structured
	}
	if hasStderr {
		return stderr
	}
	return codexProcessExitFallback(result.ExitCode)
}

// CodexStructuredStreamReportingOutcome classifies terminal JSONL stdout without
// applying process-exit stderr or exit-status fallback.
func CodexStructuredStreamReportingOutcome(stdout []byte) (ProviderFailureResult, bool) {
	return parseCodexStructuredStreamFailure(stdout)
}

// CodexProcessExitReportingOutcome classifies stderr and exit status without
// structured-stream JSONL signals.
func CodexProcessExitReportingOutcome(result CommandResult) ProviderFailureResult {
	isolated := result
	isolated.Stdout = nil
	return ParseCodexProviderFailureLayers(isolated)
}

func forEachCodexJSONLRecord(output []byte, visit func([]byte)) {
	for len(output) > 0 {
		newline := bytes.IndexByte(output, '\n')
		if newline < 0 {
			newline = len(output)
		}
		if newline <= codexJSONLRecordBytes {
			visit(output[:newline])
		}
		if newline == len(output) {
			return
		}
		output = output[newline+1:]
	}
}

func codexStructuredStreamNativeInternalCause(nativeMessage string, recognized bool) string {
	if recognized {
		if cause := codexRecognizedNativeInternalCause(nativeMessage); cause != "" {
			return cause
		}
	}
	return codexInternalCauseUnrecognizedStructured
}

func codexRecognizedNativeInternalCause(message string) string {
	message = strings.TrimSpace(message)
	if message == "" {
		return ""
	}
	reason := classifyRecognizedCodexTextFailure(strings.ToLower(message), 1)
	if reason == interfaces.WorkFailureTypeUnknown {
		return ""
	}
	return codexBoundedInternalCause(message)
}

func codexBoundedInternalCause(value string) string {
	value = strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	if value == "" {
		return ""
	}
	if len(value) <= codexFailureMessageBytes {
		return value
	}
	end := 0
	for index := range value {
		if index > codexFailureMessageBytes {
			break
		}
		end = index
	}
	return strings.TrimSpace(value[:end])
}

func codexExitInternalCause(exitCode int) string {
	return fmt.Sprintf("exit code %d", exitCode)
}

func codexStructuredJSONInternalCause(payload string) string {
	failure, ok := decodeCodexStructuredFailure(payload)
	if !ok {
		return ""
	}
	reason, recognized := classifyCodexStructuredSignal(failure.Type, failure.Status)
	if !recognized {
		return ""
	}
	if reason == interfaces.WorkFailureTypePermanentBadRequest && failure.Message == codexGPT56SolUpgradeMessage {
		return codexBoundedInternalCause(failure.Message)
	}
	parts := make([]string, 0, 3)
	if failure.Type != "" {
		parts = append(parts, failure.Type)
	}
	if failure.Status != 0 {
		parts = append(parts, fmt.Sprintf("status %d", failure.Status))
	}
	if native := codexRecognizedNativeInternalCause(failure.Message); native != "" {
		parts = append(parts, native)
	}
	return codexBoundedInternalCause(strings.Join(parts, ", "))
}

func codexProcessExitStructuredInternalCause(result CommandResult) string {
	streams := []string{
		tailForCodexErrorScan(result.Stderr),
		tailForCodexErrorScan(result.Stdout),
	}
	var last string
	for _, stream := range streams {
		for _, line := range strings.Split(stream, "\n") {
			payload, ok := codexErrorPayload(line)
			if !ok || !strings.HasPrefix(payload, "{") {
				continue
			}
			if cause := codexStructuredJSONInternalCause(payload); cause != "" {
				last = cause
			}
		}
	}
	return last
}

func codexProcessExitStderrInternalCause(result CommandResult) string {
	streams := []string{
		tailForCodexErrorScan(result.Stderr),
		tailForCodexErrorScan(result.Stdout),
	}
	var last string
	for _, stream := range streams {
		for _, line := range strings.Split(stream, "\n") {
			payload, ok := codexErrorPayload(line)
			if !ok || strings.HasPrefix(payload, "{") {
				continue
			}
			if cause := codexRecognizedNativeInternalCause(payload); cause != "" {
				last = cause
			}
		}
	}
	return last
}

// CodexSanitizedFailureFixture is the safe public projection used by Codex
// alignment tests. It carries only customer-safe fields plus a bounded
// internal-cause excerpt suitable for maintainer diagnostics.
type CodexSanitizedFailureFixture struct {
	Type          interfaces.WorkFailureType   `json:"type"`
	Family        interfaces.WorkFailureFamily `json:"family"`
	Message       string                       `json:"message"`
	Retryable     bool                         `json:"retryable"`
	Terminal      bool                         `json:"terminal"`
	InternalCause string                       `json:"internal_cause,omitempty"`
}

// CodexSanitizedFailureFixtureFromResolution projects one winning Codex failure
// resolution onto the sanitized alignment fixture shape.
func CodexSanitizedFailureFixtureFromResolution(resolution ProviderFailureResolution) CodexSanitizedFailureFixture {
	providerErr := NewProviderErrorFromResult(resolution.Result, ProviderFailureInternalCauseError(resolution.InternalCause))
	decision := WorkFailureDecisionFromProviderError(providerErr)
	return CodexSanitizedFailureFixture{
		Type:          providerErr.Type,
		Family:        providerErr.Family,
		Message:       providerErr.Message,
		Retryable:     decision.Retryable,
		Terminal:      decision.Terminal,
		InternalCause: resolution.InternalCause,
	}
}

// CodexSanitizedFailureFixtureFromProviderError projects a normalized provider
// error onto the sanitized alignment fixture shape.
func CodexSanitizedFailureFixtureFromProviderError(providerErr *ProviderError) CodexSanitizedFailureFixture {
	if providerErr == nil {
		return CodexSanitizedFailureFixture{}
	}
	decision := WorkFailureDecisionFromProviderError(providerErr)
	internalCause := ""
	if providerErr.Cause != nil {
		internalCause = providerErr.Cause.Error()
	}
	return CodexSanitizedFailureFixture{
		Type:          providerErr.Type,
		Family:        providerErr.Family,
		Message:       providerErr.Message,
		Retryable:     decision.Retryable,
		Terminal:      decision.Terminal,
		InternalCause: internalCause,
	}
}
