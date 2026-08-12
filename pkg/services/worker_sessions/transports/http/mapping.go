package http

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// MaxHTTPStartAttempts bounds the retry work one remote start may reserve.
// Worker Sessions still owns retry classification; this transport limit only
// prevents an unbounded caller-provided attempt budget.
const MaxHTTPStartAttempts = 16

var errInvalidStartRetry = errors.New("invalid Worker Session start retry policy")

// WorkerSessionStartRequestFromAPI maps one generated HTTP request into the
// detached Worker Sessions contract and validates the transport-owned retry
// bound before any service-side reservation can occur.
func WorkerSessionStartRequestFromAPI(request factoryapi.WorkerSessionStartRequest) (workersessions.StartRequest, error) {
	execution, err := workerSessionResolvedExecutionFromAPI(request.Execution)
	if err != nil {
		return workersessions.StartRequest{}, err
	}
	retry := workersessions.RetryPolicy{}
	if request.Retry != nil && request.Retry.MaxAttempts != nil {
		if *request.Retry.MaxAttempts > MaxHTTPStartAttempts {
			return workersessions.StartRequest{}, fmt.Errorf("%w: maxAttempts must not exceed %d", errInvalidStartRetry, MaxHTTPStartAttempts)
		}
		retry.MaxAttempts = *request.Retry.MaxAttempts
	}
	start := workersessions.StartRequest{
		RequestID: strings.TrimSpace(request.RequestId),
		ID:        strings.TrimSpace(request.WorkerSessionId),
		Execution: execution,
		Retry:     retry,
	}
	if err := start.Validate(); err != nil {
		return workersessions.StartRequest{}, err
	}
	return start, nil
}

func workerSessionResolvedExecutionFromAPI(
	value factoryapi.WorkerSessionResolvedExecution,
) (workers.WorkstationDispatchRequest, error) {
	dispatch, err := workerSessionResolvedDispatchFromAPI(value.Dispatch)
	if err != nil {
		return workers.WorkstationDispatchRequest{}, err
	}
	execution := workers.WorkstationExecutionRequest{
		Dispatch:                 dispatch,
		WorkerType:               optionalString(value.WorkerType),
		WorkstationType:          optionalString(value.WorkstationType),
		RunnerID:                 optionalString(value.RunnerId),
		RunnerSelectionSource:    workers.RunnerSelectionSource(optionalString(value.RunnerSelectionSource)),
		ExecutorProvider:         optionalString(value.ExecutorProvider),
		ProjectID:                optionalString(value.ProjectId),
		FactorySessionID:         optionalString(value.FactorySessionId),
		ModelOperation:           optionalString(value.ModelOperation),
		Model:                    optionalString(value.Model),
		ModelProvider:            optionalString(value.ModelProvider),
		ReasoningEffort:          optionalString(value.ReasoningEffort),
		SystemPrompt:             optionalString(value.SystemPrompt),
		UserMessage:              optionalString(value.UserMessage),
		OutputSchema:             optionalString(value.OutputSchema),
		OutputContract:           optionalString(value.OutputContract),
		Worktree:                 optionalString(value.Worktree),
		WorkingDirectory:         optionalString(value.WorkingDirectory),
		WorkingDirectoryAuthored: optionalBool(value.WorkingDirectoryAuthored),
		SkipPermissions:          optionalBool(value.SkipPermissions),
	}
	if value.InputTokens != nil {
		execution.InputTokens = append([]any(nil), (*value.InputTokens)...)
	}
	if value.EnvVars != nil {
		execution.EnvVars = cloneStringMap(*value.EnvVars)
	}
	if value.ResumeSession != nil {
		execution.ResumeSession = &providers.SessionRef{
			Provider: providers.ID(strings.TrimSpace(value.ResumeSession.Provider)),
			Kind:     strings.TrimSpace(value.ResumeSession.Kind),
			ID:       strings.TrimSpace(value.ResumeSession.Id),
		}
		if err := execution.ResumeSession.Validate(); err != nil {
			return workers.WorkstationDispatchRequest{}, fmt.Errorf("%w: resumeSession: %v", workersessions.ErrInvalidExecutionRequest, err)
		}
	}
	if value.ModelBindings != nil {
		bindings, err := modelBindingsFromAPI(*value.ModelBindings)
		if err != nil {
			return workers.WorkstationDispatchRequest{}, fmt.Errorf("%w: modelBindings: %v", workersessions.ErrInvalidExecutionRequest, err)
		}
		execution.ModelBindings = bindings
	}
	return workers.WorkstationDispatchRequest{
		WorkstationName: strings.TrimSpace(value.WorkstationName),
		Execution:       execution,
	}, nil
}

func workerSessionResolvedDispatchFromAPI(
	value factoryapi.WorkerSessionResolvedDispatch,
) (work.WorkDispatch, error) {
	dispatch := work.WorkDispatch{
		DispatchID:               strings.TrimSpace(value.DispatchId),
		TransitionID:             optionalString(value.TransitionId),
		WorkerType:               optionalString(value.WorkerType),
		WorkstationName:          strings.TrimSpace(value.WorkstationName),
		ProjectID:                optionalString(value.ProjectId),
		CurrentChainingTraceID:   optionalString(value.CurrentChainingTraceId),
		PreviousChainingTraceIDs: cloneStringSlice(value.PreviousChainingTraceIds),
	}
	if value.ExpectedArtifactContext != nil {
		data, err := json.Marshal(*value.ExpectedArtifactContext)
		if err != nil {
			return work.WorkDispatch{}, fmt.Errorf("%w: encode expected artifact context: %v", workersessions.ErrInvalidExecutionRequest, err)
		}
		var artifactContext work.ExpectedArtifactTemplateContext
		if err := json.Unmarshal(data, &artifactContext); err != nil {
			return work.WorkDispatch{}, fmt.Errorf("%w: decode expected artifact context: %v", workersessions.ErrInvalidExecutionRequest, err)
		}
		dispatch.ExpectedArtifactContext = &artifactContext
	}
	if value.Execution != nil {
		dispatch.Execution = work.ExecutionMetadata{
			DispatchCreatedTick: optionalInt(value.Execution.DispatchCreatedTick),
			CurrentTick:         optionalInt(value.Execution.CurrentTick),
			RequestID:           optionalString(value.Execution.RequestId),
			TraceID:             optionalString(value.Execution.TraceId),
			WorkIDs:             cloneStringSlice(value.Execution.WorkIds),
			ReplayKey:           optionalString(value.Execution.ReplayKey),
		}
	}
	if value.InputTokens != nil {
		dispatch.InputTokens = append([]any(nil), (*value.InputTokens)...)
	}
	if value.InputBindings != nil {
		dispatch.InputBindings = cloneStringSliceMap(*value.InputBindings)
	}
	return dispatch, nil
}

type modelBindingPayload struct {
	Slot    string                 `json:"slot"`
	Source  string                 `json:"source"`
	Content []work.WorkContentPart `json:"content,omitempty"`
}

func modelBindingsFromAPI(values []map[string]interface{}) ([]workers.ResolvedModelOperationBinding, error) {
	bindings := make([]workers.ResolvedModelOperationBinding, len(values))
	for index, value := range values {
		data, err := json.Marshal(value)
		if err != nil {
			return nil, fmt.Errorf("encode model binding %d: %w", index, err)
		}
		var payload modelBindingPayload
		if err := json.Unmarshal(data, &payload); err != nil {
			return nil, fmt.Errorf("decode model binding %d: %w", index, err)
		}
		bindings[index] = workers.ResolvedModelOperationBinding{
			Slot: payload.Slot, Source: workers.ModelOperationBindingSource(payload.Source), Content: payload.Content,
		}
	}
	return bindings, nil
}

// WorkerSessionStartResponseToAPI maps the admitted snapshot without exposing
// the Workers result or waiting for terminal output.
func WorkerSessionStartResponseToAPI(
	requestID string,
	result workersessions.StartResult,
) factoryapi.WorkerSessionStartResponse {
	return factoryapi.WorkerSessionStartResponse{
		RequestId:       requestID,
		WorkerSessionId: result.Session.ID,
		Accepted:        true,
		State:           factoryapi.WorkerSessionStartResponseState(result.Session.State),
		EventTopic:      string(workersessions.Topic(result.Session.ID)),
	}
}

func optionalString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func optionalBool(value *bool) bool {
	if value == nil {
		return false
	}
	return *value
}

func optionalInt(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}

func cloneStringSlice(value *[]string) []string {
	if value == nil {
		return nil
	}
	return append([]string(nil), (*value)...)
}

func cloneStringMap(value map[string]string) map[string]string {
	if len(value) == 0 {
		return nil
	}
	clone := make(map[string]string, len(value))
	for key, item := range value {
		clone[key] = item
	}
	return clone
}

func cloneStringSliceMap(value map[string][]string) map[string][]string {
	if len(value) == 0 {
		return nil
	}
	clone := make(map[string][]string, len(value))
	for key, items := range value {
		clone[key] = append([]string(nil), items...)
	}
	return clone
}

// ListWorkerSessionsResponseToAPI maps a detached Worker Sessions result to
// the generated API representation while preserving absent optional values.
func ListWorkerSessionsResponseToAPI(result workersessions.ListObservationsResult) factoryapi.ListWorkerSessionsResponse {
	sessions := make([]factoryapi.WorkerSessionObservation, 0, len(result.Observations))
	for _, observation := range result.Observations {
		sessions = append(sessions, WorkerSessionObservationToAPI(observation))
	}
	return factoryapi.ListWorkerSessionsResponse{Sessions: sessions}
}

// WorkerSessionTranscriptToAPI maps a detached normalized transcript result to
// the public session-scoped response without exposing Provider Sessions
// storage or reader details.
func WorkerSessionTranscriptToAPI(result workersessions.ReadTranscriptResult) factoryapi.WorkerSessionTranscriptResponse {
	entries := make([]factoryapi.ProviderSessionTranscriptEntry, len(result.Entries))
	for index, entry := range result.Entries {
		entries[index] = factoryapi.ProviderSessionTranscriptEntry{
			Arguments:        cloneString(entry.Arguments),
			CallId:           cloneString(entry.CallID),
			Encrypted:        cloneBool(entry.Encrypted),
			EncryptedContent: cloneString(entry.EncryptedContent),
			LineNumber:       cloneInt(entry.LineNumber),
			Name:             cloneString(entry.Name),
			Order:            entry.Order,
			Output:           cloneString(entry.Output),
			SourceType:       cloneString(entry.SourceType),
			Status:           cloneString(entry.Status),
			Summary:          cloneString(entry.Summary),
			Text:             cloneString(entry.Text),
			Timestamp:        cloneTime(entry.Timestamp),
			TurnIndex:        cloneInt(entry.TurnIndex),
			Type:             factoryapi.ProviderSessionTranscriptEntryType(entry.Type),
		}
	}
	response := factoryapi.WorkerSessionTranscriptResponse{
		WorkerSessionId: result.WorkerSessionID,
		ProviderSession: factoryapi.WorkerSessionProviderSessionRef{
			Provider: string(result.ProviderSession.Provider), Kind: result.ProviderSession.Kind, Id: result.ProviderSession.ID,
		},
		WorkIds:   append([]string(nil), result.WorkIDs...),
		AttemptId: result.AttemptID,
		State:     string(result.State),
		Entries:   entries,
	}
	if result.TurnID != "" {
		response.TurnId = stringPtr(result.TurnID)
	}
	return response
}

// WorkerSessionObservationToAPI maps one detached observation.
func WorkerSessionObservationToAPI(observation workersessions.Observation) factoryapi.WorkerSessionObservation {
	result := factoryapi.WorkerSessionObservation{
		WorkerSessionId:          observation.WorkerSessionID,
		ProviderSessionAvailable: observation.ProviderSessionAvailable,
		WorkIds:                  append([]string(nil), observation.WorkIDs...),
		AttemptId:                observation.AttemptID,
		State:                    factoryapi.WorkerSessionObservationState(observation.State),
		DurationBasis:            factoryapi.WorkerSessionObservationDurationBasis(observation.DurationBasis),
		Transcript:               factoryapi.WorkerSessionObservationTranscript(observation.Transcript),
		Parse:                    workerSessionParseDiagnosticsToAPI(observation.Parse),
	}
	if observation.TurnID != "" {
		result.TurnId = stringPtr(observation.TurnID)
	}
	if observation.ProviderSessionAvailable {
		result.ProviderSession = &factoryapi.WorkerSessionProviderSessionRef{
			Provider: string(observation.ProviderSession.Provider),
			Kind:     observation.ProviderSession.Kind,
			Id:       observation.ProviderSession.ID,
		}
	}
	if observation.StartedAt != nil {
		started := observation.StartedAt.UTC()
		result.StartedAt = &started
	}
	if observation.EndedAt != nil {
		ended := observation.EndedAt.UTC()
		result.EndedAt = &ended
	}
	if observation.Duration != nil {
		millis := observation.Duration.Milliseconds()
		result.DurationMillis = &millis
	}
	if observation.TokenUsage != nil {
		result.TokenUsage = &factoryapi.ProviderSessionTokenUsage{
			CacheWriteTokens:      cloneInt(observation.TokenUsage.CacheWriteTokens),
			CachedInputTokens:     cloneInt(observation.TokenUsage.CachedInputTokens),
			InputTokens:           cloneInt(observation.TokenUsage.InputTokens),
			OutputTokens:          cloneInt(observation.TokenUsage.OutputTokens),
			ReasoningOutputTokens: cloneInt(observation.TokenUsage.ReasoningOutputTokens),
			TotalTokens:           cloneInt(observation.TokenUsage.TotalTokens),
		}
	}
	if observation.Failure != nil {
		result.Failure = &factoryapi.WorkerSessionFailure{
			Kind:                            string(observation.Failure.Kind),
			Detail:                          observation.Failure.Detail,
			AgentRunFailureClass:            stringPtrIfNonEmpty(safeAgentRunFailureClass(observation.Failure.AgentRunFailureClass)),
			ProviderFailureKind:             stringPtrIfNonEmpty(string(observation.Failure.ProviderFailureKind)),
			ProviderContinuationFailureKind: stringPtrIfNonEmpty(string(observation.Failure.ProviderContinuationFailureKind)),
			ProviderContinuationOutcome:     stringPtrIfNonEmpty(string(observation.Failure.ProviderContinuationOutcome)),
		}
	}
	return result
}

func safeAgentRunFailureClass(class string) string {
	switch class {
	case workers.AgentRunFailureClassProvider, workers.AgentRunFailureClassHarness:
		return class
	default:
		return ""
	}
}

func workerSessionParseDiagnosticsToAPI(value workersessions.ParseDiagnostics) factoryapi.WorkerSessionParseDiagnostics {
	errors := make([]factoryapi.WorkerSessionParseDiagnostic, 0, len(value.Errors))
	for _, item := range value.Errors {
		errors = append(errors, factoryapi.WorkerSessionParseDiagnostic{
			Code: item.Code, LineNumber: item.LineNumber, Message: item.Message,
		})
	}
	return factoryapi.WorkerSessionParseDiagnostics{
		EventCount:         value.EventCount,
		MalformedLineCount: value.MalformedLineCount,
		UnknownEventCount:  value.UnknownEventCount,
		Errors:             errors,
	}
}

func stringPtr(value string) *string { return &value }

func stringPtrIfNonEmpty(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func cloneInt(value *int) *int {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}

func cloneBool(value *bool) *bool {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}

func cloneString(value *string) *string {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}
