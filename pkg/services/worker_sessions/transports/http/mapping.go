package http

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
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

// WorkerSessionObservationCursorFromAPI decodes the canonical Worker Session
// reconnect fields. after_sequence remains accepted as a compatibility alias
// for after_position; conflicting aliases are rejected before the service is
// invoked.
func WorkerSessionObservationCursorFromAPI(
	afterPosition *factoryapi.WorkerSessionAfterPosition,
	afterSequence *factoryapi.AfterSequence,
	streamGenerationID *factoryapi.WorkerSessionStreamGenerationID,
) (*workersessions.ObservationCursor, error) {
	if afterPosition == nil && afterSequence == nil {
		if streamGenerationID != nil && strings.TrimSpace(string(*streamGenerationID)) != "" {
			return nil, workersessions.ErrInvalidObservationCursor
		}
		return nil, nil
	}
	position := int64(0)
	if afterPosition != nil {
		position = int64(*afterPosition)
	}
	if afterSequence != nil {
		sequence := int64(*afterSequence)
		if afterPosition != nil && sequence != position {
			return nil, workersessions.ErrInvalidObservationCursor
		}
		position = sequence
	}
	if position <= 0 {
		return nil, workersessions.ErrInvalidObservationCursor
	}
	generation := ""
	if streamGenerationID != nil {
		generation = strings.TrimSpace(string(*streamGenerationID))
		if generation == "" {
			return nil, workersessions.ErrInvalidObservationCursor
		}
	}
	cursor := &workersessions.ObservationCursor{Position: uint64(position), StreamGenerationID: generation}
	if err := cursor.Validate(); err != nil {
		return nil, err
	}
	return cursor, nil
}

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

// WorkerSessionStartRequestToAPI maps the normalized service request back to
// the public top-level start contract. Keeping this inverse next to the
// boundary mapper lets a CLI normalize once, then send the exact detached
// request it validated without re-reading or resolving any input.
func WorkerSessionStartRequestToAPI(request workersessions.StartRequest) (factoryapi.WorkerSessionStartRequest, error) {
	execution, err := workerSessionResolvedExecutionToAPI(request.Execution)
	if err != nil {
		return factoryapi.WorkerSessionStartRequest{}, err
	}
	result := factoryapi.WorkerSessionStartRequest{
		RequestId:       strings.TrimSpace(request.RequestID),
		WorkerSessionId: strings.TrimSpace(request.ID),
		Execution:       execution,
	}
	if attempts := request.Retry.MaxAttempts; attempts > 0 {
		result.Retry = &factoryapi.WorkerSessionStartRetryPolicy{MaxAttempts: &attempts}
	}
	return result, nil
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
		reference := providers.SessionRef{
			Provider: providers.ID(strings.TrimSpace(value.ResumeSession.Provider)),
			Kind:     strings.TrimSpace(value.ResumeSession.Kind),
			ID:       strings.TrimSpace(value.ResumeSession.Id),
		}
		if err := reference.Validate(); err != nil {
			return workers.WorkstationDispatchRequest{}, fmt.Errorf("%w: resumeSession: %v", workersessions.ErrInvalidExecutionRequest, err)
		}
		continuation := reference.ContinuationRef()
		execution.Continuation = &continuation
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

func workerSessionResolvedExecutionToAPI(
	value workers.WorkstationDispatchRequest,
) (factoryapi.WorkerSessionResolvedExecution, error) {
	dispatch, err := workerSessionResolvedDispatchToAPI(value.Execution.Dispatch)
	if err != nil {
		return factoryapi.WorkerSessionResolvedExecution{}, err
	}
	result := factoryapi.WorkerSessionResolvedExecution{
		Dispatch:              dispatch,
		WorkstationName:       strings.TrimSpace(value.WorkstationName),
		WorkerType:            stringPointer(value.Execution.WorkerType),
		WorkstationType:       stringPointer(value.Execution.WorkstationType),
		RunnerId:              stringPointer(value.Execution.RunnerID),
		RunnerSelectionSource: stringPointer(string(value.Execution.RunnerSelectionSource)),
		ExecutorProvider:      stringPointer(value.Execution.ExecutorProvider),
		ProjectId:             stringPointer(value.Execution.ProjectID),
		FactorySessionId:      stringPointer(value.Execution.FactorySessionID),
		ModelOperation:        stringPointer(value.Execution.ModelOperation),
		Model:                 stringPointer(value.Execution.Model),
		ModelProvider:         stringPointer(value.Execution.ModelProvider),
		ReasoningEffort:       stringPointer(value.Execution.ReasoningEffort),
		SystemPrompt:          stringPointer(value.Execution.SystemPrompt),
		UserMessage:           stringPointer(value.Execution.UserMessage),
		OutputSchema:          stringPointer(value.Execution.OutputSchema),
		OutputContract:        stringPointer(value.Execution.OutputContract),
		Worktree:              stringPointer(value.Execution.Worktree),
		WorkingDirectory:      stringPointer(value.Execution.WorkingDirectory),
	}
	if value.Execution.WorkingDirectoryAuthored {
		workingDirectoryAuthored := true
		result.WorkingDirectoryAuthored = &workingDirectoryAuthored
	}
	if value.Execution.SkipPermissions {
		skipPermissions := true
		result.SkipPermissions = &skipPermissions
	}
	if len(value.Execution.InputTokens) > 0 {
		inputTokens := append([]any(nil), value.Execution.InputTokens...)
		result.InputTokens = &inputTokens
	}
	if len(value.Execution.EnvVars) > 0 {
		envVars := cloneStringMap(value.Execution.EnvVars)
		result.EnvVars = &envVars
	}
	if len(value.Execution.ModelBindings) > 0 {
		bindings := make([]map[string]interface{}, len(value.Execution.ModelBindings))
		for index, binding := range value.Execution.ModelBindings {
			encoded, err := json.Marshal(binding)
			if err != nil {
				return factoryapi.WorkerSessionResolvedExecution{}, fmt.Errorf("encode model binding %d: %w", index, err)
			}
			if err := json.Unmarshal(encoded, &bindings[index]); err != nil {
				return factoryapi.WorkerSessionResolvedExecution{}, fmt.Errorf("decode model binding %d: %w", index, err)
			}
		}
		result.ModelBindings = &bindings
	}
	if value.Execution.Continuation != nil {
		reference, err := value.Execution.Continuation.ToSessionRef()
		if err != nil {
			return factoryapi.WorkerSessionResolvedExecution{}, fmt.Errorf("invalid continuation: %w", err)
		}
		result.ResumeSession = &factoryapi.WorkerSessionProviderSessionRef{
			Provider: string(reference.Provider),
			Kind:     reference.Kind,
			Id:       reference.ID,
		}
	}
	return result, nil
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

func workerSessionResolvedDispatchToAPI(
	value work.WorkDispatch,
) (factoryapi.WorkerSessionResolvedDispatch, error) {
	result := factoryapi.WorkerSessionResolvedDispatch{
		DispatchId:               strings.TrimSpace(value.DispatchID),
		TransitionId:             stringPointer(value.TransitionID),
		WorkerType:               stringPointer(value.WorkerType),
		WorkstationName:          strings.TrimSpace(value.WorkstationName),
		ProjectId:                stringPointer(value.ProjectID),
		CurrentChainingTraceId:   stringPointer(value.CurrentChainingTraceID),
		PreviousChainingTraceIds: stringSlicePointer(value.PreviousChainingTraceIDs),
	}
	if value.ExpectedArtifactContext != nil {
		encoded, err := json.Marshal(value.ExpectedArtifactContext)
		if err != nil {
			return factoryapi.WorkerSessionResolvedDispatch{}, fmt.Errorf("encode expected artifact context: %w", err)
		}
		var context map[string]interface{}
		if err := json.Unmarshal(encoded, &context); err != nil {
			return factoryapi.WorkerSessionResolvedDispatch{}, fmt.Errorf("decode expected artifact context: %w", err)
		}
		result.ExpectedArtifactContext = &context
	}
	if len(value.InputTokens) > 0 {
		inputTokens := append([]any(nil), value.InputTokens...)
		result.InputTokens = &inputTokens
	}
	if len(value.InputBindings) > 0 {
		inputBindings := cloneStringSliceMap(value.InputBindings)
		result.InputBindings = &inputBindings
	}
	if value.Execution.DispatchCreatedTick != 0 || value.Execution.CurrentTick != 0 ||
		value.Execution.RequestID != "" || value.Execution.TraceID != "" ||
		value.Execution.ReplayKey != "" || len(value.Execution.WorkIDs) > 0 {
		execution := factoryapi.WorkerSessionExecutionMetadata{
			DispatchCreatedTick: intPointer(value.Execution.DispatchCreatedTick),
			CurrentTick:         intPointer(value.Execution.CurrentTick),
			ReplayKey:           stringPointer(value.Execution.ReplayKey),
			RequestId:           stringPointer(value.Execution.RequestID),
			TraceId:             stringPointer(value.Execution.TraceID),
			WorkIds:             stringSlicePointer(value.Execution.WorkIDs),
		}
		result.Execution = &execution
	}
	return result, nil
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

// WorkerSessionContinueRequestFromAPI maps the source path identity and the
// caller-owned continuation tuple into the Worker Sessions contract.
func WorkerSessionContinueRequestFromAPI(
	sourceWorkerSessionID string,
	request factoryapi.WorkerSessionContinueRequest,
) (workersessions.ContinueRequest, error) {
	continuation := workersessions.ContinueRequest{
		RequestID:                strings.TrimSpace(request.RequestId),
		SourceWorkerSessionID:    strings.TrimSpace(sourceWorkerSessionID),
		SuccessorWorkerSessionID: strings.TrimSpace(request.SuccessorWorkerSessionId),
		FollowUpInput:            request.FollowUpInput,
	}
	continuation = continuation.Normalize()
	if err := continuation.Validate(); err != nil {
		return workersessions.ContinueRequest{}, err
	}
	return continuation, nil
}

// WorkerSessionContinueResponseToAPI maps the admitted successor snapshot
// and its explicit source lineage without exposing provider selection data.
func WorkerSessionContinueResponseToAPI(
	result workersessions.ContinueResult,
) factoryapi.WorkerSessionContinueResponse {
	return factoryapi.WorkerSessionContinueResponse{
		RequestId:                  result.RequestID,
		SourceWorkerSessionId:      result.SourceWorkerSessionID,
		SuccessorWorkerSessionId:   result.SuccessorWorkerSessionID,
		PredecessorWorkerSessionId: result.SourceWorkerSessionID,
		Accepted:                   true,
		State:                      factoryapi.WorkerSessionContinueResponseState(result.Session.State),
		EventTopic:                 string(workersessions.Topic(result.Session.ID)),
	}
}

// WorkerSessionInterruptRequestFromAPI maps the source path identity and the
// caller-owned replacement tuple into the Worker Sessions contract.
func WorkerSessionInterruptRequestFromAPI(
	sourceWorkerSessionID string,
	request factoryapi.WorkerSessionInterruptRequest,
) (workersessions.InterruptRequest, error) {
	interrupt := workersessions.InterruptRequest{
		RequestID:                strings.TrimSpace(request.RequestId),
		SourceWorkerSessionID:    strings.TrimSpace(sourceWorkerSessionID),
		SuccessorWorkerSessionID: strings.TrimSpace(request.SuccessorWorkerSessionId),
		ReplacementMessage:       request.ReplacementMessage,
	}
	interrupt = interrupt.Normalize()
	if err := interrupt.Validate(); err != nil {
		return workersessions.InterruptRequest{}, err
	}
	return interrupt, nil
}

// WorkerSessionInterruptResponseToAPI maps the authoritative source and
// successor snapshots without exposing provider selection inputs.
func WorkerSessionInterruptResponseToAPI(
	result workersessions.InterruptResult,
) factoryapi.WorkerSessionInterruptResponse {
	return factoryapi.WorkerSessionInterruptResponse{
		RequestId:                result.RequestID,
		SourceWorkerSessionId:    result.SourceWorkerSessionID,
		SuccessorWorkerSessionId: result.SuccessorWorkerSessionID,
		Phase:                    factoryapi.WorkerSessionInterruptResponsePhase(result.Phase),
		Accepted:                 result.Accepted,
		Source:                   workerSessionInterruptSnapshotToAPI(result.Source),
		Successor:                workerSessionInterruptSnapshotToAPI(result.Successor),
	}
}

// WorkerSessionControlResponseToAPI maps the detached lifecycle result while
// preserving the exact action, outcome, state, and admitted dispatch identity
// selected by the Worker Sessions root.
func WorkerSessionControlResponseToAPI(
	result workersessions.ControlResult,
) factoryapi.WorkerSessionControlResponse {
	return factoryapi.WorkerSessionControlResponse{
		WorkerSessionId: result.Session.ID,
		Action:          factoryapi.WorkerSessionControlResponseAction(result.Action),
		Outcome:         factoryapi.WorkerSessionControlResponseOutcome(result.Outcome),
		State:           factoryapi.WorkerSessionControlResponseState(result.Session.State),
		DispatchId:      result.DispatchID,
	}
}

func workerSessionInterruptSnapshotToAPI(
	session workersessions.Session,
) factoryapi.WorkerSessionInterruptSnapshot {
	return factoryapi.WorkerSessionInterruptSnapshot{
		WorkerSessionId: session.ID,
		State:           factoryapi.WorkerSessionInterruptSnapshotState(session.State),
		EventTopic:      string(workersessions.Topic(session.ID)),
	}
}

func optionalString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func stringPointer(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func intPointer(value int) *int {
	if value == 0 {
		return nil
	}
	return &value
}

func stringSlicePointer(value []string) *[]string {
	if len(value) == 0 {
		return nil
	}
	clone := append([]string(nil), value...)
	return &clone
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

type workerSessionWorkAttribution struct {
	WorkID   string
	WorkName string
}

func listWorkerSessionObservationsResponseToAPI(
	result workersessions.ListWorkerSessionObservationsResult,
	attribution map[string]workerSessionWorkAttribution,
) factoryapi.ListWorkerSessionsResponse {
	sessions := make([]factoryapi.WorkerSessionObservation, 0, len(result.Observations))
	for _, observation := range result.Observations {
		mapped := WorkerSessionObservationToAPI(observation)
		if work, ok := attribution[observation.WorkerSessionID]; ok {
			if work.WorkID != "" {
				mapped.WorkId = stringPtr(work.WorkID)
			}
			if work.WorkName != "" {
				mapped.WorkName = stringPtr(work.WorkName)
			}
		}
		sessions = append(sessions, mapped)
	}
	response := factoryapi.ListWorkerSessionsResponse{Sessions: sessions}
	if result.MaxResults > 0 {
		response.PaginationContext = &factoryapi.PaginationContext{MaxResults: result.MaxResults}
		if result.NextToken != "" {
			response.PaginationContext.NextToken = stringPtr(result.NextToken)
		}
	}
	return response
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
	health := result.RecordingHealth
	if health == "" && (result.ProviderSession.Provider != "" || result.ProviderSession.Kind != "" || result.ProviderSession.ID != "") {
		health = recordings.WorkerRecordingStatusComplete
	}
	response := factoryapi.WorkerSessionTranscriptResponse{
		WorkerSessionId: result.WorkerSessionID,
		WorkIds:         append([]string(nil), result.WorkIDs...),
		AttemptId:       result.AttemptID,
		State:           string(result.State),
		Entries:         entries,
		RecordingHealth: factoryapi.WorkerSessionTranscriptResponseRecordingHealth(health),
	}
	if result.ProviderSession.Provider != "" || result.ProviderSession.Kind != "" || result.ProviderSession.ID != "" {
		response.ProviderSession = &factoryapi.WorkerSessionProviderSessionRef{
			Provider: string(result.ProviderSession.Provider), Kind: result.ProviderSession.Kind, Id: result.ProviderSession.ID,
		}
	}
	if result.RecordingHealthReason != "" {
		response.RecordingHealthReason = stringPtr(result.RecordingHealthReason)
	}
	if result.TurnID != "" {
		response.TurnId = stringPtr(result.TurnID)
	}
	return response
}

// WorkerSessionObservationToAPI maps one detached observation.
func WorkerSessionObservationToAPI(observation workersessions.Observation) factoryapi.WorkerSessionObservation {
	confirmationState := observation.ConfirmationState
	if confirmationState == "" {
		confirmationState = workersessions.ConfirmationStateUnconfirmed
	}
	result := factoryapi.WorkerSessionObservation{
		WorkerSessionId:          observation.WorkerSessionID,
		Direct:                   observation.Direct,
		ProviderSessionAvailable: observation.ProviderSessionAvailable,
		WorkIds:                  append([]string(nil), observation.WorkIDs...),
		AttemptId:                observation.AttemptID,
		State:                    factoryapi.WorkerSessionObservationState(observation.State),
		ConfirmationState:        factoryapi.ConfirmationState(confirmationState),
		DurationBasis:            factoryapi.WorkerSessionObservationDurationBasis(observation.DurationBasis),
		Transcript:               factoryapi.WorkerSessionObservationTranscript(observation.Transcript),
		Parse:                    workerSessionParseDiagnosticsToAPI(observation.Parse),
	}
	mapWorkerSessionIdentity(&result, observation)
	mapWorkerSessionResolvedFacts(&result, observation)
	mapWorkerSessionProvider(&result, observation)
	mapWorkerSessionTiming(&result, observation)
	mapWorkerSessionUsage(&result, observation)
	mapWorkerSessionFailure(&result, observation)
	return result
}

func mapWorkerSessionIdentity(result *factoryapi.WorkerSessionObservation, observation workersessions.Observation) {
	if observation.FactorySessionID != "" {
		result.FactorySessionId = stringPtr(observation.FactorySessionID)
	}
	if len(observation.WorkIDs) > 0 && strings.TrimSpace(observation.WorkIDs[0]) != "" {
		result.WorkId = stringPtr(observation.WorkIDs[0])
	}
}

func mapWorkerSessionResolvedFacts(result *factoryapi.WorkerSessionObservation, observation workersessions.Observation) {
	if observation.Model != nil {
		result.Model = cloneString(observation.Model)
	}
	if observation.ReasoningEffort != nil {
		result.ReasoningEffort = cloneString(observation.ReasoningEffort)
	}
	if observation.RecordingHealth != "" {
		health := factoryapi.WorkerSessionObservationRecordingHealth(observation.RecordingHealth)
		result.RecordingHealth = &health
	}
	if observation.RecordingHealthReason != "" {
		result.RecordingHealthReason = stringPtr(observation.RecordingHealthReason)
	}
	if observation.TurnID != "" {
		result.TurnId = stringPtr(observation.TurnID)
	}
}

func mapWorkerSessionProvider(result *factoryapi.WorkerSessionObservation, observation workersessions.Observation) {
	if observation.ProviderSessionAvailable {
		result.ProviderSession = &factoryapi.WorkerSessionProviderSessionRef{
			Provider: string(observation.ProviderSession.Provider),
			Kind:     observation.ProviderSession.Kind,
			Id:       observation.ProviderSession.ID,
		}
	}
}

func mapWorkerSessionTiming(result *factoryapi.WorkerSessionObservation, observation workersessions.Observation) {
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
}

func mapWorkerSessionUsage(result *factoryapi.WorkerSessionObservation, observation workersessions.Observation) {
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
	if observation.TurnUsage != nil {
		result.TurnUsage = &factoryapi.WorkerSessionTurnUsage{
			TurnCount:          observation.TurnUsage.TurnCount,
			FinalContextTokens: observation.TurnUsage.FinalContextTokens,
			PeakContextTokens:  observation.TurnUsage.PeakContextTokens,
		}
	}
}

func mapWorkerSessionFailure(result *factoryapi.WorkerSessionObservation, observation workersessions.Observation) {
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
