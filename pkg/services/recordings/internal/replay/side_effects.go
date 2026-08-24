package replay

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

const (
	DivergenceCategoryMissingDispatch    = "missing_dispatch"
	DivergenceCategoryDispatchMismatch   = "dispatch_mismatch"
	DivergenceCategoryUnknownCompletion  = "unknown_completion"
	DivergenceCategorySideEffectMismatch = "side_effect_mismatch"
	DivergenceCategoryConfigMismatch     = "config_mismatch"
)

// DivergenceReport describes a material difference between a replay artifact
// and the behavior observed while replaying it.
type DivergenceReport struct {
	Category        string `json:"category"`
	Tick            int    `json:"tick,omitempty"`
	DispatchID      string `json:"dispatch_id,omitempty"`
	ExpectedEventID string `json:"expected_event_id,omitempty"`
	ObservedEventID string `json:"observed_event_id,omitempty"`
	Expected        string `json:"expected,omitempty"`
	Observed        string `json:"observed,omitempty"`
}

// DivergenceError stops replay instead of allowing execution to continue after
// it no longer represents the recorded run.
type DivergenceError struct {
	Report DivergenceReport
}

func (e *DivergenceError) Error() string {
	if e == nil {
		return "replay divergence"
	}
	report := e.Report
	message := fmt.Sprintf("replay divergence: category=%s", report.Category)
	if report.Tick != 0 {
		message += fmt.Sprintf(" tick=%d", report.Tick)
	}
	if report.DispatchID != "" {
		message += fmt.Sprintf(" dispatch_id=%s", report.DispatchID)
	}
	if report.ExpectedEventID != "" {
		message += fmt.Sprintf(" expected_event_id=%s", report.ExpectedEventID)
	}
	if report.ObservedEventID != "" {
		message += fmt.Sprintf(" observed_event_id=%s", report.ObservedEventID)
	}
	if report.Expected != "" {
		message += fmt.Sprintf(" expected=%q", report.Expected)
	}
	if report.Observed != "" {
		message += fmt.Sprintf(" observed=%q", report.Observed)
	}
	return message
}

type divergenceOption func(*DivergenceReport)

func withExpectedEventID(eventID string) divergenceOption {
	return func(report *DivergenceReport) {
		report.ExpectedEventID = eventID
	}
}

func newDivergenceError(category string, tick int, dispatchID, expected, observed string, opts ...divergenceOption) error {
	report := DivergenceReport{
		Category:   category,
		Tick:       tick,
		DispatchID: dispatchID,
		Expected:   expected,
		Observed:   observed,
	}
	for _, opt := range opts {
		opt(&report)
	}
	return &DivergenceError{Report: report}
}

// SideEffects replays recorded provider and script-command behavior through
// the normal worker side-effect interfaces.
type SideEffects struct {
	mu      sync.Mutex
	records []sideEffectRecord
}

// ListProviders and GetProvider expose the small catalog projection replay
// needs at the Providers boundary. Replay does not discover live providers;
// it treats the requested identity as the recorded identity and leaves live
// catalog availability out of the replay decision.
func (s *SideEffects) ListProviders(context.Context, providers.ListProvidersRequest) (providers.ListProvidersResult, error) {
	return providers.ListProvidersResult{}, nil
}

func (s *SideEffects) GetProvider(_ context.Context, request providers.GetProviderRequest) (providers.GetProviderResult, error) {
	if err := request.Validate(); err != nil {
		return providers.GetProviderResult{}, err
	}
	return providers.GetProviderResult{Provider: providers.Descriptor{ID: request.ID}}, nil
}

func (s *SideEffects) ResolveIdentity(_ context.Context, request providers.ResolveIdentityRequest) (providers.ResolveIdentityResult, error) {
	if err := request.Validate(); err != nil {
		return providers.ResolveIdentityResult{}, err
	}
	return providers.ResolveIdentityResult{ID: providers.ID(request.Identity)}, nil
}

func (s *SideEffects) ResolveSelection(_ context.Context, request providers.ResolveSelectionRequest) (providers.ResolveSelectionResult, error) {
	identity := request.Workstation
	if identity == "" {
		identity = request.Factory
	}
	if identity == "" {
		identity = request.ModelProvider
	}
	resolved, err := s.ResolveIdentity(context.Background(), providers.ResolveIdentityRequest{Identity: identity})
	if err != nil {
		return providers.ResolveSelectionResult{}, err
	}
	return providers.ResolveSelectionResult{Provider: resolved.ID}, nil
}

func (s *SideEffects) ValidatePrerequisites(_ context.Context, request providers.ValidatePrerequisitesRequest) error {
	return request.Validate()
}

// Execute is the Providers-owned replay execution boundary. The conversion
// into the retained event-recording vocabulary is local to replay; callers
// never receive a Workers provider client.
func (s *SideEffects) Execute(ctx context.Context, request providers.ExecuteRequest) (providers.ExecuteResult, error) {
	if err := request.Validate(); err != nil {
		return providers.ExecuteResult{}, err
	}
	dispatchID := request.Correlation.DispatchID
	if dispatchID == "" {
		dispatchID = request.AttemptID
	}
	response, err := s.Infer(ctx, workerexecution.ProviderInferenceRequest{
		Dispatch: work.WorkDispatch{
			DispatchID:      dispatchID,
			TransitionID:    request.TransitionID,
			WorkerType:      request.WorkerType,
			WorkstationName: request.WorkstationName,
			ProjectID:       request.ProjectID,
			InputTokens:     append([]any(nil), request.InputTokens...),
			InputBindings:   cloneStringSliceMap(request.InputBindings),
			Execution: work.ExecutionMetadata{
				RequestID: request.Correlation.RequestID,
				ReplayKey: request.Correlation.ReplayKey,
				TraceID:   request.Correlation.TraceID,
				WorkIDs:   append([]string(nil), request.Correlation.WorkIDs...),
			},
		},
		RunnerID:  request.RunnerID,
		ProjectID: request.ProjectID,
		Correlation: workerexecution.ExecutionCorrelation{
			FactorySessionID: request.Correlation.FactorySessionID,
			RuntimeID:        request.Correlation.RuntimeID,
			GenerationID:     request.Correlation.GenerationID,
			DispatchID:       request.Correlation.DispatchID,
			AttemptID:        request.Correlation.AttemptID,
			RequestID:        request.Correlation.RequestID,
			TraceID:          request.Correlation.TraceID,
		},
		WorkerType:         request.WorkerType,
		WorkstationType:    request.WorkstationName,
		Model:              request.Model,
		ModelProvider:      request.Provider.String(),
		ReasoningEffort:    request.ReasoningEffort,
		SystemPrompt:       request.SystemPrompt,
		UserMessage:        request.UserMessage,
		InputTokens:        append([]any(nil), request.InputTokens...),
		SessionID:          request.SessionID,
		OutputSchema:       request.OutputSchema,
		WorkingDirectory:   request.WorkingDirectory,
		Worktree:           request.Worktree,
		EnvVars:            cloneStringMap(request.EnvVars),
		ProcessEnvironment: append([]string(nil), request.ProcessEnvironment...),
		SkipPermissions:    request.SkipPermissions,
		PrintTimeout:       request.PrintTimeout,
		ExecutionLogger:    request.ExecutionLogger,
	})
	if err != nil {
		return providers.ExecuteResult{}, replayExecuteFailure(err)
	}
	result := providers.ExecuteResult{Content: response.Content}
	if response.Continuation != nil {
		reference, err := response.Continuation.ToSessionRef()
		if err != nil {
			return providers.ExecuteResult{}, replayExecuteFailure(err)
		}
		result.SessionRef = &reference
	}
	if response.Diagnostics != nil {
		metadata := cloneStringMap(response.Diagnostics.Metadata)
		if response.Diagnostics.Provider != nil {
			for key, value := range response.Diagnostics.Provider.ResponseMetadata {
				if metadata == nil {
					metadata = make(map[string]string)
				}
				metadata[key] = value
			}
		}
		result.Diagnostics = &providers.ExecuteDiagnostics{Metadata: metadata}
		if response.Diagnostics.Command != nil {
			result.Diagnostics.Command = &providers.ExecuteCommandDiagnostics{
				Command:    response.Diagnostics.Command.Command,
				Args:       append([]string(nil), response.Diagnostics.Command.Args...),
				Env:        cloneStringMap(response.Diagnostics.Command.Env),
				Stdin:      response.Diagnostics.Command.Stdin,
				Stdout:     response.Diagnostics.Command.Stdout,
				Stderr:     response.Diagnostics.Command.Stderr,
				ExitCode:   response.Diagnostics.Command.ExitCode,
				TimedOut:   response.Diagnostics.Command.TimedOut,
				DurationMS: response.Diagnostics.Command.Duration.Milliseconds(),
				WorkingDir: response.Diagnostics.Command.WorkingDir,
			}
		}
		if response.Diagnostics.Panic != nil {
			result.Diagnostics.Panic = &providers.ExecutePanicDiagnostics{
				Message: response.Diagnostics.Panic.Message,
				Stack:   response.Diagnostics.Panic.Stack,
			}
		}
	}
	return result, nil
}

func (s *SideEffects) ControlAttempt(_ context.Context, request providers.ControlAttemptRequest) (providers.ControlAttemptResult, error) {
	if err := request.Validate(); err != nil {
		return providers.ControlAttemptResult{}, err
	}
	return providers.ControlAttemptResult{
		Provider:  request.Provider,
		AttemptID: request.AttemptID,
		Action:    request.Action,
		Outcome:   providers.ControlOutcomeUnsupported,
	}, nil
}

func (s *SideEffects) Continue(ctx context.Context, request providers.ContinueRequest) (providers.ContinueResult, error) {
	if err := request.Validate(); err != nil {
		return providers.ContinueResult{}, err
	}
	result, err := s.Execute(ctx, request.Attempt)
	if err != nil {
		return providers.ContinueResult{}, err
	}
	return providers.ContinueResult{
		Reference: request.Reference.Clone(),
		Outcome:   providers.ContinuationOutcomeResumed,
		Result:    result,
	}, nil
}

func (s *SideEffects) ContinueReference(ctx context.Context, request providers.ContinueReferenceRequest) (providers.ContinueReferenceResult, error) {
	reference, err := request.Reference.ToSessionRef()
	if err != nil {
		return providers.ContinueReferenceResult{}, replayContinuationFailure(providers.ContinuationFailureKindInvalid, err.Error(), request.Reference)
	}
	if s == nil {
		return providers.ContinueReferenceResult{}, replayContinuationFailure(providers.ContinuationFailureKindInvalid, "Providers service is required", request.Reference)
	}
	canonical, err := s.ResolveIdentity(ctx, providers.ResolveIdentityRequest{Identity: reference.Provider.String()})
	if err != nil {
		return providers.ContinueReferenceResult{}, replayContinuationFailure(providers.ContinuationFailureKindForeign, err.Error(), request.Reference)
	}
	if err := canonical.ID.Validate(); err != nil {
		return providers.ContinueReferenceResult{}, replayContinuationFailure(providers.ContinuationFailureKindForeign, err.Error(), request.Reference)
	}
	reference.Provider = canonical.ID
	attempt := request.Attempt.Clone()
	if strings.TrimSpace(attempt.Provider.String()) == "" {
		attempt.Provider = canonical.ID
	} else {
		attemptIdentity, resolveErr := s.ResolveIdentity(ctx, providers.ResolveIdentityRequest{Identity: attempt.Provider.String()})
		if resolveErr != nil || attemptIdentity.ID != canonical.ID {
			message := "attempt provider does not match continuation provider"
			if resolveErr != nil {
				message = resolveErr.Error()
			}
			return providers.ContinueReferenceResult{}, replayContinuationFailure(providers.ContinuationFailureKindForeign, message, request.Reference)
		}
		attempt.Provider = canonical.ID
	}
	continued, err := s.Continue(ctx, providers.ContinueRequest{Reference: reference, Attempt: attempt})
	if err != nil {
		return providers.ContinueReferenceResult{}, err
	}
	continuedReference := continued.Reference
	if strings.TrimSpace(continuedReference.Provider.String()) == "" {
		continuedReference = reference
	}
	resultReference := continuedReference.ContinuationRef()
	resultReference.ExternalRef = request.Reference.Normalize().ExternalRef
	return providers.ContinueReferenceResult{Reference: resultReference, Outcome: continued.Outcome, Result: continued.Result}, nil
}

func replayContinuationFailure(kind providers.ContinuationFailureKind, message string, ref providers.ContinuationRef) providers.ContinuationFailure {
	normalized := ref.Normalize()
	identity := strings.TrimSpace(normalized.ProviderSessionID)
	if identity == "" {
		identity = strings.TrimSpace(normalized.ExternalRef)
	}
	return providers.ContinuationFailure{
		Kind:    kind,
		Message: message,
		Reference: providers.SessionRef{
			Provider: providers.ID(normalized.Provider),
			Kind:     normalized.Kind,
			ID:       identity,
		},
	}
}

func replayExecuteFailure(err error) error {
	if err == nil {
		return nil
	}
	kind := providers.ExecuteFailureKindUnknown
	switch {
	case errors.Is(err, context.Canceled):
		kind = providers.ExecuteFailureKindCanceled
	case errors.Is(err, context.DeadlineExceeded):
		kind = providers.ExecuteFailureKindTimeout
	}
	var providerErr *workers.ProviderError
	if errors.As(err, &providerErr) && providerErr != nil {
		switch providerErr.Type {
		case workers.WorkFailureTypeAuthFailure:
			kind = providers.ExecuteFailureKindAuthentication
		case workers.WorkFailureTypePermanentBadRequest:
			kind = providers.ExecuteFailureKindInvalidRequest
		case workers.WorkFailureTypeThrottled:
			kind = providers.ExecuteFailureKindThrottled
		case workers.WorkFailureTypeTimeout:
			kind = providers.ExecuteFailureKindTimeout
		case workers.WorkFailureTypeMisconfigured:
			kind = providers.ExecuteFailureKindMisconfigured
		}
	}
	return providers.ExecuteFailure{Kind: kind, Message: err.Error()}
}

type sideEffectRecord struct {
	dispatch      replayDispatch
	completion    *replayCompletion
	hasCompletion bool
	usedBy        string
}

// NewSideEffects builds replay-aware side-effect substitutes from an artifact.
func NewSideEffects(
	decodeFactorySnapshot interfaces.FactorySnapshotJSONDecoder,
	decodeRuntimeConfig interfaces.ReplayRuntimeConfigDecoder,
	artifact *interfaces.ReplayArtifact,
) (*SideEffects, error) {
	eventLog, err := reduceReplayEvents(
		artifact,
		decodeFactorySnapshot,
		decodeRuntimeConfig,
	)
	if err != nil {
		return nil, err
	}

	dispatches := make(map[string]replayDispatch, len(eventLog.Dispatches))
	for _, dispatch := range eventLog.Dispatches {
		dispatches[dispatch.dispatchID] = dispatch
	}

	completions := make(map[string]replayCompletion, len(eventLog.Completions))
	for _, completion := range eventLog.Completions {
		if _, ok := dispatches[completion.dispatchID]; !ok {
			return nil, newDivergenceError(
				DivergenceCategoryUnknownCompletion,
				completion.observedTick,
				completion.dispatchID,
				"recorded dispatch for completion "+completion.completionID,
				"completion references unknown dispatch "+completion.dispatchID,
				withExpectedEventID(completion.eventID),
			)
		}
		completions[completion.dispatchID] = completion
	}

	records := make([]sideEffectRecord, 0, len(eventLog.Dispatches))
	for _, dispatch := range eventLog.Dispatches {
		record := sideEffectRecord{dispatch: dispatch}
		if completion, ok := completions[dispatch.dispatchID]; ok {
			completionCopy := completion
			record.completion = &completionCopy
			record.hasCompletion = true
		}
		records = append(records, sideEffectRecord{
			dispatch:      record.dispatch,
			completion:    record.completion,
			hasCompletion: record.hasCompletion,
		})
	}

	return &SideEffects{records: records}, nil
}

// Infer implements provider.Provider by returning the recorded provider response
// for the matching dispatch.
func (s *SideEffects) Infer(ctx context.Context, req workerexecution.ProviderInferenceRequest) (workerexecution.InferenceResponse, error) {
	record, err := s.claim(ctx, "provider", func(candidate sideEffectRecord) bool {
		return providerRequestMatches(candidate, req)
	})
	if err != nil {
		return workerexecution.InferenceResponse{}, err
	}
	if !record.hasCompletion {
		return workerexecution.InferenceResponse{}, missingCompletionError(record.dispatch)
	}

	result := record.completion.result
	failureMetadata := result.FailureMetadata
	if result.Outcome == workerexecution.OutcomeFailed && failureMetadata != nil {
		return workerexecution.InferenceResponse{}, workers.NewProviderError(
			failureMetadata.Type,
			result.Error,
			errors.New(result.Error),
		)
	}

	diagnostics := workerexecution.CloneWorkDiagnostics(record.completion.diagnostics)
	if result.Output != "" {
		if diagnostics == nil {
			diagnostics = &workerexecution.WorkDiagnostics{}
		}
		if diagnostics.Metadata == nil {
			diagnostics.Metadata = make(map[string]string, 1)
		}
		if diagnostics.Metadata[workerexecution.ProviderResponseMetadataCompletionEvidence] == "" &&
			(diagnostics.Provider == nil || diagnostics.Provider.ResponseMetadata[workerexecution.ProviderResponseMetadataCompletionEvidence] == "") {
			diagnostics.Metadata[workerexecution.ProviderResponseMetadataCompletionEvidence] = "provider_response"
		}
	}

	return workerexecution.InferenceResponse{
		Content:      result.Output,
		Continuation: cloneReplayContinuation(result.Continuation),
		Diagnostics:  diagnostics,
	}, nil
}

// InvokeModel implements the request-scoped managed-model replay effect. The
// normal Models service remains responsible for live preparation and backend
// execution; replay returns the detached output captured on the canonical
// dispatch completion so no model host or invocation backend is consulted.
func (s *SideEffects) InvokeModel(
	ctx context.Context,
	request models.InvokeModelRequest,
) (models.InvokeModelResult, error) {
	record, err := s.claim(ctx, "model", func(candidate sideEffectRecord) bool {
		return candidate.hasCompletion && candidate.completion != nil
	})
	if err != nil {
		return models.InvokeModelResult{}, err
	}
	if record.completion.result.Outcome == workerexecution.OutcomeFailed {
		message := strings.TrimSpace(record.completion.result.Error)
		if message == "" {
			message = "recorded model invocation failed"
		}
		return models.InvokeModelResult{
			ModelName: request.Model.NameOrURI,
			Operation: request.Operation,
			Status:    models.ModelInvocationStatusFailed,
		}, errors.New(replayModelInvocationFailureDetail(message))
	}

	outputs := replayModelOutputs(record.completion.result)
	if len(outputs) == 0 {
		return models.InvokeModelResult{}, fmt.Errorf("recorded model invocation has no output")
	}
	content := make([]models.InferenceContent, 0, len(outputs))
	for _, output := range outputs {
		content = append(content, models.InferenceContent{
			Name:        output.Name,
			Modality:    output.Modality,
			ContentType: output.ContentType,
			MediaType:   output.MediaType,
			Content:     output.Content,
		})
	}
	return models.InvokeModelResult{
		ModelName: request.Model.NameOrURI,
		Operation: request.Operation,
		Status:    models.ModelInvocationStatusCompleted,
		Content:   content,
		Outputs:   outputs,
	}, nil
}

// replayModelInvocationFailureDetail reverses the Worker-owned classification
// around a recorded Models error. The live model boundary returns the detail
// (for example, "model inference failed: backend failure") to the inference
// runner, which adds the stable worker/model/operation context. A completion
// records that outer classification, so replay must remove it before the
// normal runner classifies the failure again or the public message is nested.
func replayModelInvocationFailureDetail(message string) string {
	message = strings.TrimSpace(message)
	if !strings.HasPrefix(message, "inference failed for ") {
		return message
	}
	if separator := strings.Index(message, ": "); separator >= 0 {
		if detail := strings.TrimSpace(message[separator+2:]); detail != "" {
			return detail
		}
	}
	return message
}

func replayModelOutputs(result workerexecution.WorkResult) []models.InferenceOutput {
	content := result.OutputContent
	if len(content) == 0 && len(result.RecordedOutputWork) > 0 {
		content = result.RecordedOutputWork[0].Content
	}
	if len(content) == 0 {
		return nil
	}
	outputs := make([]models.InferenceOutput, 0, len(content))
	for index, part := range content {
		modality, value := replayModelOutputValue(part)
		if modality == "" || strings.TrimSpace(value) == "" {
			continue
		}
		name := strings.TrimSpace(part.Slot)
		if name == "" {
			name = fmt.Sprintf("output-%d", index)
		}
		mediaType := strings.TrimSpace(part.ContentType)
		outputs = append(outputs, models.InferenceOutput{
			Name:        name,
			Modality:    modality,
			ContentType: mediaType,
			MediaType:   mediaType,
			Content:     value,
		})
	}
	return outputs
}

func replayModelOutputValue(part work.WorkContentPart) (models.Modality, string) {
	switch part.Type.Normalized() {
	case work.WorkContentPartTypeText:
		return models.ModalityText, part.Text
	case work.WorkContentPartTypeJSON:
		return models.ModalityJSON, string(part.JSON)
	case work.WorkContentPartTypeAudio:
		return models.ModalityAudio, firstReplayContentReference(part)
	case work.WorkContentPartTypeImage:
		return models.ModalityImage, firstReplayContentReference(part)
	case work.WorkContentPartTypeBinary:
		return models.ModalityBinary, firstReplayContentReference(part)
	default:
		return "", ""
	}
}

func firstReplayContentReference(part work.WorkContentPart) string {
	for _, value := range []string{part.URL, part.File, part.Text} {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func cloneReplayContinuation(reference *providers.ContinuationRef) *providers.ContinuationRef {
	if reference == nil {
		return nil
	}
	clone := reference.Clone()
	return &clone
}

// Run implements platformprocess.CommandRunner by returning the recorded
// script command outcome for the matching platform request.
func (s *SideEffects) Run(ctx context.Context, req platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	record, err := s.claim(ctx, "command", func(candidate sideEffectRecord) bool {
		return commandRequestMatches(candidate, req)
	})
	if err != nil {
		return platformprocess.CommandResult{}, err
	}
	if !record.hasCompletion {
		return platformprocess.CommandResult{}, missingCompletionError(record.dispatch)
	}
	if record.completion.diagnostics == nil || record.completion.diagnostics.Command == nil {
		result := platformprocess.CommandResult{
			Stdout: []byte(record.completion.result.Output),
			Stderr: []byte(record.completion.result.Error),
		}
		if record.completion.result.Outcome == workerexecution.OutcomeFailed {
			result.ExitCode = 1
		}
		return result, nil
	}

	command := record.completion.diagnostics.Command
	result := platformprocess.CommandResult{
		Stdout:   []byte(command.Stdout),
		Stderr:   []byte(command.Stderr),
		ExitCode: command.ExitCode,
	}
	if command.TimedOut {
		return result, context.DeadlineExceeded
	}
	return result, nil
}

func missingCompletionError(dispatch replayDispatch) error {
	return fmt.Errorf("recorded dispatch %q for transition %q has no completion", dispatch.dispatchID, dispatch.dispatch.TransitionID)
}

func (s *SideEffects) claim(ctx context.Context, kind string, matches func(sideEffectRecord) bool) (sideEffectRecord, error) {
	if s == nil {
		return sideEffectRecord{}, fmt.Errorf("replay side effects are required")
	}
	if err := ctx.Err(); err != nil {
		return sideEffectRecord{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return sideEffectRecord{}, err
	}

	for i := range s.records {
		if s.records[i].usedBy != "" {
			continue
		}
		if !matches(s.records[i]) {
			continue
		}
		s.records[i].usedBy = kind
		return s.records[i], nil
	}
	return sideEffectRecord{}, newDivergenceError(
		DivergenceCategorySideEffectMismatch,
		0,
		"",
		"recorded "+kind+" request",
		"replay "+kind+" request did not match a recorded completion",
	)
}

func providerRequestMatches(record sideEffectRecord, req workerexecution.ProviderInferenceRequest) bool {
	dispatch := record.dispatch.dispatch
	if !executionMetadataMatches(dispatch.Execution, req.Dispatch.Execution) {
		return false
	}
	if req.WorkerType != "" && req.WorkerType != dispatch.WorkerType {
		return false
	}
	if req.WorkstationType != "" && req.WorkstationType != dispatch.WorkstationName {
		return false
	}
	if record.completion != nil && record.completion.diagnostics != nil && record.completion.diagnostics.Provider != nil {
		provider := record.completion.diagnostics.Provider
		if provider.Provider != "" && req.ModelProvider != "" && provider.Provider != req.ModelProvider {
			return false
		}
		if provider.Model != "" && req.Model != "" && provider.Model != req.Model {
			return false
		}
	}
	return true
}

func commandRequestMatches(record sideEffectRecord, req platformprocess.CommandRequest) bool {
	if !record.hasCompletion {
		return true
	}
	if record.completion.diagnostics == nil {
		return true
	}
	if record.completion.diagnostics.Command == nil {
		return false
	}
	command := record.completion.diagnostics.Command
	if command.Command != "" && command.Command != req.Command {
		return false
	}
	if len(command.Args) > 0 && !reflect.DeepEqual(command.Args, req.Args) {
		return false
	}
	if command.WorkingDir != "" && command.WorkingDir != req.WorkDir {
		return false
	}
	return true
}

func executionMetadataMatches(recorded, observed work.ExecutionMetadata) bool {
	if recorded.ReplayKey != "" && observed.ReplayKey != recorded.ReplayKey {
		return false
	}
	if recorded.TraceID != "" && observed.TraceID != recorded.TraceID {
		return false
	}
	if len(recorded.WorkIDs) > 0 && !reflect.DeepEqual(recorded.WorkIDs, observed.WorkIDs) {
		return false
	}
	return true
}

var _ providers.Service = (*SideEffects)(nil)
var _ platformprocess.CommandRunner = (*SideEffects)(nil)
