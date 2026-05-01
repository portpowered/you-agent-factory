package replay

import (
	"fmt"

	factoryapi "github.com/portpowered/agent-factory/pkg/api/generated"
	"github.com/portpowered/agent-factory/pkg/interfaces"
	"github.com/portpowered/agent-factory/pkg/workers"
)

type replayEventLog struct {
	Factory     factoryapi.Factory
	Submissions []replaySubmission
	Dispatches  []replayDispatch
	Completions []replayCompletion
	Diagnostics interfaces.ReplayDiagnostics
	WallClock   *interfaces.ReplayWallClockMetadata
}

type replaySubmission struct {
	eventID      string
	observedTick int
	request      interfaces.WorkRequest
	source       string
}

type replayDispatch struct {
	eventID     string
	dispatchID  string
	createdTick int
	dispatch    interfaces.WorkDispatch
}

type replayCompletion struct {
	eventID      string
	completionID string
	dispatchID   string
	observedTick int
	result       interfaces.WorkResult
	diagnostics  *interfaces.WorkDiagnostics
}

type replayInferenceAttempt struct {
	attempt         int
	providerSession *interfaces.ProviderSessionMetadata
	diagnostics     *interfaces.WorkDiagnostics
}

func reduceReplayEvents(artifact *interfaces.ReplayArtifact) (*replayEventLog, error) {
	if err := validateReplayEventEnvelope(artifact); err != nil {
		return nil, err
	}
	reduced := &replayEventLog{}
	inferenceAttemptsByDispatchID := make(map[string]replayInferenceAttempt)
	workByID := make(map[string]interfaces.Work)
	for _, event := range artifact.Events {
		switch event.Type {
		case factoryapi.FactoryEventTypeRunRequest:
			payload, err := runStartedPayloadFromEvent(event)
			if err != nil {
				return nil, err
			}
			reduced.Factory = payload.Factory
			reduced.WallClock = replayWallClockFromGenerated(payload.WallClock)
			reduced.Diagnostics = replayDiagnosticsFromGenerated(payload.Diagnostics)
		case factoryapi.FactoryEventTypeWorkRequest:
			submissions, err := replaySubmissionsFromEvent(event)
			if err != nil {
				return nil, err
			}
			reduced.Submissions = append(reduced.Submissions, submissions...)
			indexReplaySubmissionWork(workByID, submissions)
		case factoryapi.FactoryEventTypeDispatchRequest:
			dispatch, err := replayDispatchFromEvent(reduced.Factory, event, workByID)
			if err != nil {
				return nil, err
			}
			reduced.Dispatches = append(reduced.Dispatches, dispatch)
		case factoryapi.FactoryEventTypeInferenceResponse:
			dispatchID, attempt, err := replayInferenceAttemptFromEvent(event)
			if err != nil {
				return nil, err
			}
			if dispatchID != "" {
				current := inferenceAttemptsByDispatchID[dispatchID]
				if attempt.attempt >= current.attempt {
					inferenceAttemptsByDispatchID[dispatchID] = attempt
				}
			}
		case factoryapi.FactoryEventTypeDispatchResponse:
			completion, err := replayCompletionFromEvent(event, inferenceAttemptsByDispatchID[stringValue(event.Context.DispatchId)])
			if err != nil {
				return nil, err
			}
			reduced.Completions = append(reduced.Completions, completion)
		case factoryapi.FactoryEventTypeRunResponse:
			payload, err := event.Payload.AsRunResponseEventPayload()
			if err != nil {
				return nil, fmt.Errorf("decode run finished event %q: %w", event.Id, err)
			}
			if wallClock := replayWallClockFromGenerated(payload.WallClock); wallClock != nil {
				reduced.WallClock = wallClock
			}
			if diagnostics := replayDiagnosticsFromGenerated(payload.Diagnostics); len(diagnostics.Notes) > 0 || len(diagnostics.Workers) > 0 {
				reduced.Diagnostics = diagnostics
			}
		}
	}
	if !generatedFactoryHasConfig(reduced.Factory) {
		return nil, fmt.Errorf("replay event log RUN_REQUEST factory is required")
	}
	return reduced, nil
}

func validateReplayEventEnvelope(artifact *interfaces.ReplayArtifact) error {
	if artifact == nil {
		return fmt.Errorf("replay artifact is required")
	}
	if artifact.SchemaVersion == "" {
		return fmt.Errorf("replay artifact schemaVersion is required")
	}
	if artifact.SchemaVersion != CurrentSchemaVersion {
		return fmt.Errorf("unsupported replay artifact schemaVersion %q; supported schemaVersion is %q", artifact.SchemaVersion, CurrentSchemaVersion)
	}
	if artifact.RecordedAt.IsZero() {
		return fmt.Errorf("replay artifact recordedAt is required")
	}
	if len(artifact.Events) == 0 {
		return fmt.Errorf("replay artifact events is required")
	}
	for i, event := range artifact.Events {
		if event.SchemaVersion != factoryapi.AgentFactoryEventV1 {
			return fmt.Errorf("replay artifact events[%d].schemaVersion = %q, want %q", i, event.SchemaVersion, factoryapi.AgentFactoryEventV1)
		}
		if event.Context.Sequence != i {
			return fmt.Errorf("replay artifact events[%d].context.sequence = %d, want %d", i, event.Context.Sequence, i)
		}
		if event.Id == "" {
			return fmt.Errorf("replay artifact events[%d].id is required", i)
		}
		if event.Type == "" {
			return fmt.Errorf("replay artifact events[%d].type is required", i)
		}
		if event.Context.EventTime.IsZero() {
			return fmt.Errorf("replay artifact events[%d].context.eventTime is required", i)
		}
	}
	return nil
}

func indexReplaySubmissionWork(workByID map[string]interfaces.Work, submissions []replaySubmission) {
	for _, submission := range submissions {
		for _, work := range submission.request.Works {
			if work.WorkID == "" {
				continue
			}
			workByID[work.WorkID] = work
		}
	}
}

func replaySubmissionsFromEvent(event factoryapi.FactoryEvent) ([]replaySubmission, error) {
	payload, err := event.Payload.AsWorkRequestEventPayload()
	if err != nil {
		return nil, fmt.Errorf("decode work request event %q: %w", event.Id, err)
	}
	source := stringValue(payload.Source)
	if source == "" {
		source = stringValue(event.Context.Source)
	}
	if isWorkerOutputSource(source) {
		return nil, nil
	}
	requestID := stringValue(event.Context.RequestId)
	works := generatedWorksValue(payload.Works)
	if len(works) == 0 {
		return nil, nil
	}
	contextWorkIDs := stringSliceValue(event.Context.WorkIds)
	contextTraceIDs := stringSliceValue(event.Context.TraceIds)
	request := interfaces.WorkRequest{
		RequestID: requestID,
		Type:      interfaces.WorkRequestType(payload.Type),
		Works:     make([]interfaces.Work, 0, len(works)),
		Relations: workRelationsFromGenerated(works, payload.Relations),
	}
	for i, work := range works {
		item := workFromGeneratedWork(work, requestID)
		if item.WorkID == "" && i < len(contextWorkIDs) {
			item.WorkID = contextWorkIDs[i]
		}
		if item.TraceID == "" {
			if i < len(contextTraceIDs) {
				item.TraceID = contextTraceIDs[i]
			} else {
				item.TraceID = firstString(event.Context.TraceIds)
			}
		}
		request.Works = append(request.Works, item)
	}
	if request.Type == "" {
		request.Type = interfaces.WorkRequestTypeFactoryRequestBatch
	}
	return []replaySubmission{
		{
			eventID:      event.Id,
			observedTick: event.Context.Tick,
			request:      request,
			source:       source,
		},
	}, nil
}

func replayDispatchFromEvent(factory factoryapi.Factory, event factoryapi.FactoryEvent, workByID map[string]interfaces.Work) (replayDispatch, error) {
	payload, err := event.Payload.AsDispatchRequestEventPayload()
	if err != nil {
		return replayDispatch{}, fmt.Errorf("decode dispatch created event %q: %w", event.Id, err)
	}
	dispatchID := stringValue(event.Context.DispatchId)
	if dispatchID == "" {
		return replayDispatch{}, fmt.Errorf("dispatch created event %q context.dispatchId is required", event.Id)
	}
	workstation := generatedReplayWorkstation(factory, payload.TransitionId)
	dispatch := interfaces.WorkDispatch{
		DispatchID:      dispatchID,
		TransitionID:    payload.TransitionId,
		WorkerType:      generatedReplayWorkerName(workstation),
		WorkstationName: generatedReplayWorkstationName(workstation, payload.TransitionId),
		InputTokens:     replayInputTokensFromDispatchPayload(event.Context, payload, workByID),
		Execution: interfaces.ExecutionMetadata{
			RequestID:           stringValue(event.Context.RequestId),
			TraceID:             firstString(event.Context.TraceIds),
			WorkIDs:             stringSliceValue(event.Context.WorkIds),
			DispatchCreatedTick: event.Context.Tick,
		},
	}
	dispatch.Execution.ReplayKey = replayMetadataValue(payload.Metadata)
	if len(dispatch.Execution.WorkIDs) == 0 {
		dispatch.Execution.WorkIDs = workIDsFromDispatchRefs(payload.Inputs, stringSliceValue(event.Context.WorkIds))
	}
	if dispatch.Execution.TraceID == "" {
		dispatch.Execution.TraceID = firstTraceIDFromDispatchRefs(payload.Inputs, stringSliceValue(event.Context.WorkIds), workByID)
	}
	dispatch.CurrentChainingTraceID = replayDispatchCurrentChainingTraceID(
		event.Context.CurrentChainingTraceId,
		payload.CurrentChainingTraceId,
		dispatch.Execution.TraceID,
	)
	dispatch.PreviousChainingTraceIDs = replayDispatchPreviousChainingTraceIDs(
		event.Context.PreviousChainingTraceIds,
		payload.PreviousChainingTraceIds,
		workDispatchInputTokensForReplay(event.Context, payload, workByID),
	)
	return replayDispatch{
		eventID:     event.Id,
		dispatchID:  dispatchID,
		createdTick: event.Context.Tick,
		dispatch:    dispatch,
	}, nil
}

func replayInferenceAttemptFromEvent(event factoryapi.FactoryEvent) (string, replayInferenceAttempt, error) {
	payload, err := event.Payload.AsInferenceResponseEventPayload()
	if err != nil {
		return "", replayInferenceAttempt{}, fmt.Errorf("decode inference response event %q: %w", event.Id, err)
	}
	return stringValue(event.Context.DispatchId), replayInferenceAttempt{
		attempt:         payload.Attempt,
		providerSession: interfaces.ProviderSessionMetadataFromGenerated(payload.ProviderSession),
		diagnostics:     workDiagnosticsFromSafe(interfaces.SafeWorkDiagnosticsFromGenerated(payload.Diagnostics)),
	}, nil
}

func replayCompletionFromEvent(event factoryapi.FactoryEvent, inference replayInferenceAttempt) (replayCompletion, error) {
	payload, err := event.Payload.AsDispatchResponseEventPayload()
	if err != nil {
		return replayCompletion{}, fmt.Errorf("decode dispatch completed event %q: %w", event.Id, err)
	}
	diagnostics := cloneWorkDiagnostics(inference.diagnostics)
	completionID := stringValue(payload.CompletionId)
	if completionID == "" {
		completionID = event.Id
	}
	recordedOutputWork := make([]interfaces.FactoryWorkItem, 0, len(generatedWorksValue(payload.OutputWork)))
	for _, work := range generatedWorksValue(payload.OutputWork) {
		recordedOutputWork = append(recordedOutputWork, factoryWorkItemFromGeneratedWork(work))
	}
	return replayCompletion{
		eventID:      event.Id,
		completionID: completionID,
		dispatchID:   stringValue(event.Context.DispatchId),
		observedTick: event.Context.Tick,
		result: interfaces.WorkResult{
			DispatchID:         stringValue(event.Context.DispatchId),
			TransitionID:       payload.TransitionId,
			Outcome:            interfaces.WorkOutcome(payload.Outcome),
			Output:             stringValue(payload.Output),
			Error:              stringValue(payload.Error),
			Feedback:           stringValue(payload.Feedback),
			RecordedOutputWork: recordedOutputWork,
			ProviderFailure:    interfaces.ProviderFailureMetadataFromGenerated(payload.ProviderFailure),
			ProviderSession:    cloneProviderSession(inference.providerSession),
			Metrics:            replayWorkMetricsFromGenerated(payload.Metrics),
			Diagnostics:        diagnostics,
		},
		diagnostics: diagnostics,
	}, nil
}

func workDiagnosticsFromSafe(diagnostics *interfaces.SafeWorkDiagnostics) *interfaces.WorkDiagnostics {
	if diagnostics == nil {
		return nil
	}
	return &interfaces.WorkDiagnostics{
		RenderedPrompt: renderedPromptDiagnosticFromSafe(diagnostics.RenderedPrompt),
		Provider:       providerDiagnosticFromSafe(diagnostics.Provider),
	}
}

func renderedPromptDiagnosticFromSafe(diagnostic *interfaces.SafeRenderedPromptDiagnostic) *interfaces.RenderedPromptDiagnostic {
	if diagnostic == nil {
		return nil
	}
	return &interfaces.RenderedPromptDiagnostic{
		SystemPromptHash: diagnostic.SystemPromptHash,
		UserMessageHash:  diagnostic.UserMessageHash,
		Variables:        cloneStringMap(diagnostic.Variables),
	}
}

func providerDiagnosticFromSafe(diagnostic *interfaces.SafeProviderDiagnostic) *interfaces.ProviderDiagnostic {
	if diagnostic == nil {
		return nil
	}
	return &interfaces.ProviderDiagnostic{
		Provider:         diagnostic.Provider,
		Model:            diagnostic.Model,
		RequestMetadata:  cloneStringMap(diagnostic.RequestMetadata),
		ResponseMetadata: cloneStringMap(diagnostic.ResponseMetadata),
	}
}

func replayInputTokensFromDispatchPayload(
	context factoryapi.FactoryEventContext,
	payload factoryapi.DispatchRequestEventPayload,
	workByID map[string]interfaces.Work,
) []any {
	tokens := workDispatchInputTokensForReplay(context, payload, workByID)
	return workers.InputTokens(tokens...)
}

func workDispatchInputTokensForReplay(
	context factoryapi.FactoryEventContext,
	payload factoryapi.DispatchRequestEventPayload,
	workByID map[string]interfaces.Work,
) []interfaces.Token {
	tokens := make([]interfaces.Token, 0, len(payload.Inputs)+len(resourceValues(payload.Resources)))
	contextWorkIDs := stringSliceValue(context.WorkIds)
	for i, ref := range payload.Inputs {
		workID := dispatchConsumedWorkID(ref, i, contextWorkIDs)
		work := workByID[workID]
		traceID := work.TraceID
		if traceID == "" {
			traceID = firstString(context.TraceIds)
		}
		currentChainingTraceID := work.CurrentChainingTraceID
		if currentChainingTraceID == "" {
			currentChainingTraceID = traceID
		}
		tokens = append(tokens, interfaces.Token{
			ID: workID,
			Color: interfaces.TokenColor{
				WorkID:                   workID,
				WorkTypeID:               work.WorkTypeID,
				DataType:                 interfaces.DataTypeWork,
				CurrentChainingTraceID:   currentChainingTraceID,
				PreviousChainingTraceIDs: append([]string(nil), work.PreviousChainingTraceIDs...),
				TraceID:                  traceID,
				Name:                     work.Name,
				Tags:                     cloneStringMap(work.Tags),
			},
		})
	}
	for _, resource := range resourceValues(payload.Resources) {
		tokens = append(tokens, interfaces.Token{
			ID:      "resource/" + resource.Name,
			PlaceID: resource.Name + ":available",
			Color: interfaces.TokenColor{
				WorkTypeID: resource.Name,
				DataType:   interfaces.DataTypeResource,
				Name:       resource.Name,
			},
		})
	}
	return tokens
}

func replayMetadataValue(metadata *factoryapi.DispatchRequestEventMetadata) string {
	if metadata == nil {
		return ""
	}
	return stringValue(metadata.ReplayKey)
}

func replayDispatchCurrentChainingTraceID(
	contextCurrent *string,
	payloadCurrent *string,
	fallbackTraceID string,
) string {
	if current := stringValue(contextCurrent); current != "" {
		return current
	}
	if current := stringValue(payloadCurrent); current != "" {
		return current
	}
	return fallbackTraceID
}

func replayDispatchPreviousChainingTraceIDs(
	contextPrevious *[]string,
	payloadPrevious *[]string,
	inputTokens []interfaces.Token,
) []string {
	if previous := stringSliceValue(contextPrevious); len(previous) > 0 {
		return interfaces.CanonicalChainingTraceIDs(previous)
	}
	if previous := stringSliceValue(payloadPrevious); len(previous) > 0 {
		return interfaces.CanonicalChainingTraceIDs(previous)
	}
	return interfaces.PreviousChainingTraceIDsFromTokens(inputTokens)
}

func generatedReplayWorkstation(factory factoryapi.Factory, transitionID string) *factoryapi.Workstation {
	for _, workstation := range generatedWorkstationSlice(factory.Workstations) {
		if stringValue(workstation.Id) == transitionID || workstation.Name == transitionID {
			return &workstation
		}
	}
	return nil
}

func generatedReplayWorkstationName(workstation *factoryapi.Workstation, transitionID string) string {
	if workstation == nil {
		return transitionID
	}
	if workstation.Name != "" {
		return workstation.Name
	}
	return transitionID
}

func generatedReplayWorkerName(workstation *factoryapi.Workstation) string {
	if workstation == nil {
		return ""
	}
	return workstation.Worker
}

func workIDsFromDispatchRefs(refs []factoryapi.DispatchConsumedWorkRef, contextWorkIDs []string) []string {
	out := make([]string, 0, len(refs))
	for i, ref := range refs {
		if id := dispatchConsumedWorkID(ref, i, contextWorkIDs); id != "" {
			out = append(out, id)
		}
	}
	return out
}

func firstTraceIDFromDispatchRefs(refs []factoryapi.DispatchConsumedWorkRef, contextWorkIDs []string, workByID map[string]interfaces.Work) string {
	for i, ref := range refs {
		if traceID := workByID[dispatchConsumedWorkID(ref, i, contextWorkIDs)].TraceID; traceID != "" {
			return traceID
		}
	}
	return ""
}

func dispatchConsumedWorkID(ref factoryapi.DispatchConsumedWorkRef, index int, contextWorkIDs []string) string {
	if ref.WorkId != "" {
		return ref.WorkId
	}
	if index >= 0 && index < len(contextWorkIDs) {
		return contextWorkIDs[index]
	}
	return ""
}

func resourceValues(resources *[]factoryapi.Resource) []factoryapi.Resource {
	if resources == nil {
		return nil
	}
	return *resources
}

func isWorkerOutputSource(source string) bool {
	return len(source) >= len("worker-output:") && source[:len("worker-output:")] == "worker-output:"
}
