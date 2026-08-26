package replay

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/portpowered/infinite-you/pkg/platform/jsonvalue"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workdomain "github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerdiagnostics "github.com/portpowered/infinite-you/pkg/services/workers"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

type replayEventLog struct {
	RuntimeConfig    interfaces.ReplayRuntimeConfig
	Submissions      []replaySubmission
	Dispatches       []replayDispatch
	WorkerSessionIDs map[string]string
	Completions      []replayCompletion
	WorkStateChanges []replayWorkStateChange
	Diagnostics      interfaces.ReplayDiagnostics
	WallClock        *interfaces.ReplayWallClockMetadata
}

type replayWorkStateChange struct {
	eventID      string
	observedTick int
	change       work.WorkStateChangeRecord
	fromPlaceID  string
	toPlaceID    string
}

type replaySubmission struct {
	eventID      string
	observedTick int
	request      workdomain.WorkRequest
	source       string
}

type replayDispatch struct {
	eventID     string
	dispatchID  string
	createdTick int
	dispatch    work.WorkDispatch
}

type replayCompletion struct {
	eventID      string
	completionID string
	dispatchID   string
	observedTick int
	result       workerexecution.WorkResult
	diagnostics  *workerexecution.WorkDiagnostics
}

type replayInferenceAttempt struct {
	attempt         int
	providerSession *providers.SessionMetadata
	diagnostics     *workerexecution.WorkDiagnostics
}

func reduceReplayEvents(
	artifact *interfaces.ReplayArtifact,
	decodeFactorySnapshot interfaces.FactorySnapshotJSONDecoder,
	decodeRuntimeConfig interfaces.ReplayRuntimeConfigDecoder,
) (*replayEventLog, error) {
	if err := validateReplayEventEnvelope(artifact); err != nil {
		return nil, err
	}
	if decodeRuntimeConfig == nil {
		return nil, fmt.Errorf("Factory Definition replay runtime decoder is required")
	}
	if decodeFactorySnapshot == nil {
		return nil, fmt.Errorf("Factory snapshot decoder is required")
	}
	reduced := &replayEventLog{WorkerSessionIDs: make(map[string]string)}
	inferenceAttemptsByDispatchID := make(map[string]replayInferenceAttempt)
	workByID := make(map[string]work.Work)
	for _, event := range artifact.Events {
		if err := reduceReplayEvent(
			reduced,
			event,
			workByID,
			inferenceAttemptsByDispatchID,
			decodeFactorySnapshot,
			decodeRuntimeConfig,
		); err != nil {
			return nil, err
		}
	}
	if reduced.RuntimeConfig == nil || reduced.RuntimeConfig.FactoryConfig() == nil {
		return nil, fmt.Errorf("replay event log RUN_REQUEST factory is required")
	}
	return reduced, nil
}

func reduceReplayEvent(
	reduced *replayEventLog,
	event interfaces.FactoryEvent,
	workByID map[string]work.Work,
	inferenceAttemptsByDispatchID map[string]replayInferenceAttempt,
	decodeFactorySnapshot interfaces.FactorySnapshotJSONDecoder,
	decodeRuntimeConfig interfaces.ReplayRuntimeConfigDecoder,
) error {
	switch event.Type {
	case interfaces.FactoryEventTypeRunRequest:
		return applyReplayRunRequest(
			reduced,
			event,
			decodeFactorySnapshot,
			decodeRuntimeConfig,
		)
	case interfaces.FactoryEventTypeDispatchRequest:
		return applyReplayDispatchRequest(reduced, event, workByID)
	case interfaces.FactoryEventTypeDispatchWorkerSessionAssoc:
		return applyReplayWorkerSessionAssociation(reduced, event)
	case interfaces.FactoryEventTypeWorkStateChange:
		return applyReplayWorkStateChange(reduced, event)
	case interfaces.FactoryEventTypeWorkRequest:
		return applyReplayWorkRequest(reduced, event, workByID)
	case interfaces.FactoryEventTypeRunResponse:
		return applyReplayRunResponse(reduced, event)
	case interfaces.FactoryEventTypeInferenceResponse,
		interfaces.FactoryEventTypeModelResponse:
		return applyReplayInferenceResponse(event, inferenceAttemptsByDispatchID)
	}
	switch event.Type {
	case interfaces.FactoryEventTypeDispatchResponse:
		return applyReplayDispatchResponse(reduced, event, inferenceAttemptsByDispatchID)
	default:
		return nil
	}
}

func applyReplayWorkerSessionAssociation(reduced *replayEventLog, event interfaces.FactoryEvent) error {
	if reduced == nil {
		return fmt.Errorf("replay event log is required")
	}
	dispatchID := stringValue(event.Context.DispatchID)
	if dispatchID == "" {
		return nil
	}
	var payload interfaces.DispatchWorkerSessionAssociationEventPayload
	if err := event.DecodePayload(&payload); err != nil {
		return fmt.Errorf("decode Worker Session association event %q: %w", event.Id, err)
	}
	workerSessionID := strings.TrimSpace(payload.WorkerSessionID)
	if workerSessionID == "" {
		return nil
	}
	if previous := strings.TrimSpace(reduced.WorkerSessionIDs[dispatchID]); previous != "" && previous != workerSessionID {
		return fmt.Errorf("replay Worker Session association for dispatch %q changes from %q to %q", dispatchID, previous, workerSessionID)
	}
	reduced.WorkerSessionIDs[dispatchID] = workerSessionID
	return nil
}

func applyReplayWorkStateChange(reduced *replayEventLog, event interfaces.FactoryEvent) error {
	change, err := replayWorkStateChangeFromEvent(event)
	if err != nil {
		return err
	}
	if change == nil {
		return nil
	}
	reduced.WorkStateChanges = append(reduced.WorkStateChanges, *change)
	return nil
}

func replayWorkStateChangeFromEvent(event interfaces.FactoryEvent) (*replayWorkStateChange, error) {
	var payload interfaces.WorkStateChangeEventPayload
	if err := event.DecodePayload(&payload); err != nil {
		return nil, fmt.Errorf("decode work state change event %q: %w", event.Id, err)
	}
	source := payload.Source
	if source != work.WorkStateChangeSourceAPI && source != work.WorkStateChangeSourceCLI {
		return nil, nil
	}
	workID := payload.WorkID
	if workID == "" {
		workID = firstString(event.Context.WorkIDs)
	}
	if workID == "" {
		return nil, fmt.Errorf("work state change event %q payload.workId is required", event.Id)
	}
	return &replayWorkStateChange{
		eventID:      event.Id,
		observedTick: event.Context.Tick,
		change: work.WorkStateChangeRecord{
			WorkID:        workID,
			WorkTypeName:  payload.WorkTypeName,
			FromState:     payload.FromState,
			ToState:       payload.ToState,
			Source:        source,
			RequestID:     stringValue(event.Context.RequestID),
			TriggerWorkID: stringValue(payload.TriggerWorkID),
			Reason:        stringValue(payload.Reason),
		},
		fromPlaceID: payload.FromPlaceID,
		toPlaceID:   payload.ToPlaceID,
	}, nil
}

func applyReplayRunRequest(
	reduced *replayEventLog,
	event interfaces.FactoryEvent,
	decodeFactorySnapshot interfaces.FactorySnapshotJSONDecoder,
	decodeRuntimeConfig interfaces.ReplayRuntimeConfigDecoder,
) error {
	payload, err := runStartedPayloadFromEventAtBoundary(
		event,
		decodeFactorySnapshot,
	)
	if err != nil {
		return err
	}
	runtimeConfig, err := decodeRuntimeConfig(payload.Factory)
	if err != nil {
		return fmt.Errorf("decode run started event %q factory: %w", event.Id, err)
	}
	reduced.RuntimeConfig = runtimeConfig
	reduced.WallClock = replayWallClockFromRunEvent(payload.WallClock)
	reduced.Diagnostics = replayDiagnosticsFromRunEvent(payload.Diagnostics)
	return nil
}

func applyReplayWorkRequest(reduced *replayEventLog, event interfaces.FactoryEvent, workByID map[string]work.Work) error {
	submissions, err := replaySubmissionsFromEvent(event)
	if err != nil {
		return err
	}
	reduced.Submissions = append(reduced.Submissions, submissions...)
	indexReplaySubmissionWork(workByID, submissions)
	return nil
}

func applyReplayDispatchRequest(reduced *replayEventLog, event interfaces.FactoryEvent, workByID map[string]work.Work) error {
	dispatch, err := replayDispatchFromEvent(reduced.RuntimeConfig, event, workByID)
	if err != nil {
		return err
	}
	reduced.Dispatches = append(reduced.Dispatches, dispatch)
	return nil
}

func applyReplayInferenceResponse(event interfaces.FactoryEvent, inferenceAttemptsByDispatchID map[string]replayInferenceAttempt) error {
	dispatchID, attempt, err := replayInferenceAttemptFromEvent(event)
	if err != nil {
		return err
	}
	if dispatchID == "" {
		return nil
	}
	current := inferenceAttemptsByDispatchID[dispatchID]
	if attempt.attempt >= current.attempt {
		inferenceAttemptsByDispatchID[dispatchID] = attempt
	}
	return nil
}

func applyReplayDispatchResponse(
	reduced *replayEventLog,
	event interfaces.FactoryEvent,
	inferenceAttemptsByDispatchID map[string]replayInferenceAttempt,
) error {
	completion, err := replayCompletionFromEvent(event, inferenceAttemptsByDispatchID[stringValue(event.Context.DispatchID)])
	if err != nil {
		return err
	}
	reduced.Completions = append(reduced.Completions, completion)
	return nil
}

func applyReplayRunResponse(reduced *replayEventLog, event interfaces.FactoryEvent) error {
	var payload interfaces.RunResponseEventPayload
	if err := event.DecodePayload(&payload); err != nil {
		return fmt.Errorf("decode run finished event %q: %w", event.Id, err)
	}
	if wallClock := replayWallClockFromRunEvent(payload.WallClock); wallClock != nil {
		reduced.WallClock = wallClock
	}
	if diagnostics := replayDiagnosticsFromRunEvent(payload.Diagnostics); len(diagnostics.Notes) > 0 || len(diagnostics.Workers) > 0 {
		reduced.Diagnostics = diagnostics
	}
	return nil
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
		if event.SchemaVersion != interfaces.FactoryEventSchemaVersionV1 {
			return fmt.Errorf("replay artifact events[%d].schemaVersion = %q, want %q", i, event.SchemaVersion, interfaces.FactoryEventSchemaVersionV1)
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

func indexReplaySubmissionWork(workByID map[string]work.Work, submissions []replaySubmission) {
	for _, submission := range submissions {
		for _, work := range submission.request.Works {
			if work.WorkID == "" {
				continue
			}
			workByID[work.WorkID] = work
		}
	}
}

func replaySubmissionsFromEvent(event interfaces.FactoryEvent) ([]replaySubmission, error) {
	var payload work.WorkRequestEventPayload
	if err := event.DecodePayload(&payload); err != nil {
		return nil, fmt.Errorf("decode work request event %q: %w", event.Id, err)
	}
	source := payload.Source
	if source == "" {
		source = stringValue(event.Context.Source)
	}
	if isWorkerOutputSource(source) {
		return nil, nil
	}
	requestID := stringValue(event.Context.RequestID)
	works := payload.Works
	if len(works) == 0 {
		return nil, nil
	}
	contextWorkIDs := stringSliceValue(event.Context.WorkIDs)
	contextTraceIDs := stringSliceValue(event.Context.TraceIDs)
	request := workdomain.WorkRequest{
		RequestID: requestID,
		Type:      work.WorkRequestType(payload.Type),
		Works:     make([]work.Work, 0, len(works)),
		Relations: replayWorkRequestRelations(works, payload.Relations),
	}
	for i, eventWork := range works {
		item := replayWorkRequestWork(eventWork, requestID)
		if item.WorkID == "" && i < len(contextWorkIDs) {
			item.WorkID = contextWorkIDs[i]
		}
		if item.TraceID == "" {
			if i < len(contextTraceIDs) {
				item.TraceID = contextTraceIDs[i]
			} else {
				item.TraceID = firstString(event.Context.TraceIDs)
			}
		}
		request.Works = append(request.Works, item)
	}
	if request.Type == "" {
		request.Type = workdomain.WorkRequestTypeFactoryRequestBatch
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

func replayWorkRequestWork(eventWork work.WorkRequestEventWork, requestID string) work.Work {
	currentChainingTraceID := eventWork.CurrentChainingTraceID
	if currentChainingTraceID == "" {
		currentChainingTraceID = eventWork.TraceID
	}
	state := ""
	if eventWork.State != nil {
		state = eventWork.State.Name
	}
	if state == "" && eventWork.WorkTypeID == interfaces.SystemTimeWorkTypeID {
		state = interfaces.SystemTimePendingState
	}
	return work.Work{
		RequestID:                requestID,
		WorkID:                   eventWork.WorkID,
		Name:                     eventWork.Name,
		WorkTypeID:               eventWork.WorkTypeID,
		State:                    state,
		ChainingTraceDepth:       eventWork.ChainingTraceDepth,
		CurrentChainingTraceID:   currentChainingTraceID,
		PreviousChainingTraceIDs: append([]string(nil), eventWork.PreviousChainingTraceIDs...),
		TraceID:                  eventWork.TraceID,
		Content:                  work.CloneWorkContentParts(eventWork.Content),
		Payload:                  replayWorkRequestPayload(eventWork.Payload),
		Tags:                     cloneStringMap(eventWork.Tags),
	}
}

func replayWorkRequestPayload(payload []byte) any {
	if len(payload) == 0 || string(payload) == "null" {
		return nil
	}
	var text string
	if json.Unmarshal(payload, &text) == nil {
		return []byte(text)
	}
	return append([]byte(nil), payload...)
}

func replayWorkRequestRelations(works []work.WorkRequestEventWork, relations []work.WorkRequestEventRelation) []work.WorkRelation {
	namesByID := make(map[string]string, len(works))
	for _, eventWork := range works {
		if eventWork.WorkID != "" && eventWork.Name != "" {
			namesByID[eventWork.WorkID] = eventWork.Name
		}
	}
	out := make([]work.WorkRelation, 0, len(relations))
	for _, relation := range relations {
		sourceName := relation.SourceWorkName
		if mapped := namesByID[sourceName]; mapped != "" {
			sourceName = mapped
		}
		targetName := relation.TargetWorkName
		if targetName == "" {
			targetName = namesByID[relation.TargetWorkID]
		} else if relation.TargetWorkID != "" && targetName == relation.TargetWorkID {
			// Event history uses the target ID as the required public name
			// fallback when an ID-only relation is recorded. Do not replay that
			// fallback as an authored name: a later admission must resolve the
			// canonical ID against the selected session board.
			targetName = namesByID[relation.TargetWorkID]
		}
		if sourceName == "" || (targetName == "" && relation.TargetWorkID == "") {
			continue
		}
		out = append(out, work.WorkRelation{
			Type:           relation.Type,
			SourceWorkName: sourceName,
			TargetWorkID:   relation.TargetWorkID,
			TargetWorkName: targetName,
			RequiredState:  relation.RequiredState,
		})
	}
	return out
}

func replayDispatchFromEvent(runtimeConfig interfaces.ReplayRuntimeConfig, event interfaces.FactoryEvent, workByID map[string]work.Work) (replayDispatch, error) {
	var payload interfaces.DispatchRequestEventPayload
	if err := event.DecodePayload(&payload); err != nil {
		return replayDispatch{}, fmt.Errorf("decode dispatch created event %q: %w", event.Id, err)
	}
	dispatchID := stringValue(event.Context.DispatchID)
	if dispatchID == "" {
		return replayDispatch{}, fmt.Errorf("dispatch created event %q context.dispatchId is required", event.Id)
	}
	workstation := replayWorkstation(runtimeConfig, payload.TransitionID)
	dispatch := work.WorkDispatch{
		DispatchID:              dispatchID,
		TransitionID:            payload.TransitionID,
		WorkerType:              replayWorkerName(workstation),
		WorkstationName:         replayWorkstationName(workstation, payload.TransitionID),
		ExpectedArtifactContext: cloneExpectedArtifactTemplateContext(payload.ExpectedArtifactContext),
		InputTokens:             replayInputTokensFromDispatchPayload(event.Context, payload, workByID),
		Execution: work.ExecutionMetadata{
			RequestID:           stringValue(event.Context.RequestID),
			TraceID:             firstString(event.Context.TraceIDs),
			WorkIDs:             stringSliceValue(event.Context.WorkIDs),
			DispatchCreatedTick: event.Context.Tick,
		},
	}
	dispatch.Execution.ReplayKey = replayMetadataValue(payload.Metadata)
	if len(dispatch.Execution.WorkIDs) == 0 {
		dispatch.Execution.WorkIDs = workIDsFromDispatchRefs(payload.Inputs, stringSliceValue(event.Context.WorkIDs))
	}
	if dispatch.Execution.TraceID == "" {
		dispatch.Execution.TraceID = firstTraceIDFromDispatchRefs(payload.Inputs, stringSliceValue(event.Context.WorkIDs), workByID)
	}
	dispatch.CurrentChainingTraceID = replayDispatchCurrentChainingTraceID(
		event.Context.CurrentChainingTraceID,
		payload.CurrentChainingTraceID,
		dispatch.Execution.TraceID,
	)
	dispatch.PreviousChainingTraceIDs = replayDispatchPreviousChainingTraceIDs(
		event.Context.PreviousChainingTraceIDs,
		payload.PreviousChainingTraceIDs,
		workDispatchInputTokensForReplay(event.Context, payload, workByID),
	)
	return replayDispatch{
		eventID:     event.Id,
		dispatchID:  dispatchID,
		createdTick: event.Context.Tick,
		dispatch:    dispatch,
	}, nil
}

func replayInferenceAttemptFromEvent(event interfaces.FactoryEvent) (string, replayInferenceAttempt, error) {
	var payload workerexecution.InferenceResponseEventPayload
	if err := event.DecodePayload(&payload); err != nil {
		return "", replayInferenceAttempt{}, fmt.Errorf("decode inference response event %q: %w", event.Id, err)
	}
	diagnostics, err := workerdiagnostics.WorkDiagnosticsFromSafeEventPayload(payload.Diagnostics)
	if err != nil {
		return "", replayInferenceAttempt{}, fmt.Errorf("decode inference response event %q: %w", event.Id, err)
	}
	providerSession := (payload.Continuation).SessionMetadata()
	if providerSession == nil {
		// Replay accepts older public event artifacts whose response payload
		// carried providerSession before canonical events switched to the opaque
		// continuation. This compatibility decode stays at the recording
		// boundary; Workers still receives only Continuation.
		var legacy struct {
			ProviderSession *providers.SessionMetadata `json:"providerSession"`
		}
		if decodeErr := json.Unmarshal(event.Payload, &legacy); decodeErr != nil {
			return "", replayInferenceAttempt{}, fmt.Errorf("decode legacy inference response event %q: %w", event.Id, decodeErr)
		}
		providerSession = (legacy.ProviderSession).Clone()
	}
	return stringValue(event.Context.DispatchID), replayInferenceAttempt{
		attempt:         payload.Attempt,
		providerSession: providerSession,
		diagnostics:     diagnostics,
	}, nil
}

func replayCompletionFromEvent(event interfaces.FactoryEvent, inference replayInferenceAttempt) (replayCompletion, error) {
	var payload workerexecution.DispatchResponseEventPayload
	if err := event.DecodePayload(&payload); err != nil {
		return replayCompletion{}, fmt.Errorf("decode dispatch completed event %q: %w", event.Id, err)
	}
	diagnostics := workerexecution.CloneWorkDiagnostics(inference.diagnostics)
	completionID := stringValue(payload.CompletionID)
	if completionID == "" {
		completionID = event.Id
	}
	recordedOutputWork := make([]workdomain.FactoryWorkItem, 0, len(workRequestEventWorks(payload.OutputWork)))
	for _, eventWork := range workRequestEventWorks(payload.OutputWork) {
		recordedOutputWork = append(recordedOutputWork, factoryWorkItemFromEventWork(eventWork))
	}
	dispatchID := stringValue(event.Context.DispatchID)
	return replayCompletion{
		eventID:      event.Id,
		completionID: completionID,
		dispatchID:   dispatchID,
		observedTick: event.Context.Tick,
		result: workerexecution.WorkResult{
			DispatchID:                  dispatchID,
			TransitionID:                payload.TransitionID,
			Outcome:                     payload.Outcome,
			Cancellation:                payload.Cancellation.Clone(),
			Output:                      stringValue(payload.Output),
			OutputContent:               replayOutputContentFromRecordedWork(recordedOutputWork),
			StructuredResult:            jsonvalue.Clone(payload.StructuredResult),
			StructuredResultPresent:     jsonvalue.Present(payload.StructuredResult, payload.StructuredResultPresent),
			Error:                       stringValue(payload.Error),
			Feedback:                    stringValue(payload.Feedback),
			SelectedClassificationLabel: stringValue(payload.SelectedClassificationLabel),
			RecordedOutputWork:          recordedOutputWork,
			FailureMetadata:             workerexecution.CloneWorkFailureMetadata(payload.ProviderFailure),
			Continuation:                (inference.providerSession).ContinuationRef(),
			Metrics:                     replayWorkMetricsFromEvent(payload.Metrics),
			Diagnostics:                 diagnostics,
		},
		diagnostics: diagnostics,
	}, nil
}

func replayOutputContentFromRecordedWork(items []workdomain.FactoryWorkItem) []workdomain.WorkContentPart {
	if len(items) == 0 || len(items[0].Content) == 0 {
		return nil
	}
	return workdomain.CloneWorkContentParts(items[0].Content)
}

func workRequestEventWorks(items *[]work.WorkRequestEventWork) []work.WorkRequestEventWork {
	if items == nil {
		return nil
	}
	return *items
}

func factoryWorkItemFromEventWork(eventWork work.WorkRequestEventWork) workdomain.FactoryWorkItem {
	state := ""
	if eventWork.State != nil {
		state = eventWork.State.Name
	}
	if state == "" && eventWork.WorkTypeID == interfaces.SystemTimeWorkTypeID {
		state = interfaces.SystemTimePendingState
	}
	currentChainingTraceID := eventWork.CurrentChainingTraceID
	if currentChainingTraceID == "" {
		currentChainingTraceID = eventWork.TraceID
	}
	content := work.CloneWorkContentParts(eventWork.Content)
	for i := range content {
		content[i].Type = content[i].Type.Normalized()
	}
	return workdomain.FactoryWorkItem{
		ID:                       eventWork.WorkID,
		WorkTypeID:               eventWork.WorkTypeID,
		State:                    state,
		DisplayName:              eventWork.Name,
		CurrentChainingTraceID:   currentChainingTraceID,
		PreviousChainingTraceIDs: append([]string(nil), eventWork.PreviousChainingTraceIDs...),
		TraceID:                  eventWork.TraceID,
		Content:                  content,
		StructuredResult:         jsonvalue.Clone(eventWork.StructuredResult),
		Tags:                     cloneStringMap(eventWork.Tags),
		StructuredResultPresent:  jsonvalue.Present(eventWork.StructuredResult, eventWork.StructuredResultPresent),
	}
}

func replayWorkMetricsFromEvent(metrics *workerexecution.WorkMetricsEventPayload) workerexecution.WorkMetrics {
	if metrics == nil {
		return workerexecution.WorkMetrics{}
	}
	return workerexecution.WorkMetrics{
		Duration:   time.Duration(int64Value(metrics.DurationMillis)) * time.Millisecond,
		Cost:       float64Value(metrics.Cost),
		RetryCount: intValue(metrics.RetryCount),
	}
}

func replayInputTokensFromDispatchPayload(
	context interfaces.FactoryEventContext,
	payload interfaces.DispatchRequestEventPayload,
	workByID map[string]work.Work,
) []any {
	tokens := workDispatchInputTokensForReplay(context, payload, workByID)
	return workers.InputTokens(tokens...)
}

func workDispatchInputTokensForReplay(
	context interfaces.FactoryEventContext,
	payload interfaces.DispatchRequestEventPayload,
	workByID map[string]work.Work,
) []workerexecution.Token {
	tokens := make([]workerexecution.Token, 0, len(payload.Inputs)+len(resourceValues(payload.Resources)))
	contextWorkIDs := stringSliceValue(context.WorkIDs)
	for i, ref := range payload.Inputs {
		workID := dispatchConsumedWorkID(ref, i, contextWorkIDs)
		work := workByID[workID]
		traceID := work.TraceID
		if traceID == "" {
			traceID = firstString(context.TraceIDs)
		}
		currentChainingTraceID := work.CurrentChainingTraceID
		if currentChainingTraceID == "" {
			currentChainingTraceID = traceID
		}
		tokens = append(tokens, workerexecution.Token{
			ID: workID,
			Color: workerexecution.Color{
				WorkID:                   workID,
				WorkTypeID:               work.WorkTypeID,
				DataType:                 workerexecution.DataTypeWork,
				CurrentChainingTraceID:   currentChainingTraceID,
				PreviousChainingTraceIDs: append([]string(nil), work.PreviousChainingTraceIDs...),
				TraceID:                  traceID,
				Name:                     work.Name,
				Content:                  append([]workdomain.WorkContentPart(nil), work.Content...),
				Tags:                     cloneStringMap(work.Tags),
			},
		})
	}
	for _, resource := range resourceValues(payload.Resources) {
		tokens = append(tokens, workerexecution.Token{
			ID:    "resource/" + resource.Name,
			State: "available",
			Color: workerexecution.Color{
				WorkTypeID: resource.Name,
				DataType:   workerexecution.DataTypeResource,
				Name:       resource.Name,
			},
		})
	}
	return tokens
}

func replayMetadataValue(metadata *interfaces.DispatchRequestEventMetadata) string {
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
	inputTokens []workerexecution.Token,
) []string {
	if previous := stringSliceValue(contextPrevious); len(previous) > 0 {
		return work.CanonicalChainingTraceIDs(previous)
	}
	if previous := stringSliceValue(payloadPrevious); len(previous) > 0 {
		return work.CanonicalChainingTraceIDs(previous)
	}
	return workerexecution.PreviousChainingTraceIDs(inputTokens)
}

func replayWorkstation(runtimeConfig interfaces.ReplayRuntimeConfig, transitionID string) *interfaces.FactoryWorkstationConfig {
	if runtimeConfig == nil {
		return nil
	}
	if workstation, ok := runtimeConfig.WorkstationByID(transitionID); ok {
		return workstation
	}
	workstation, _ := runtimeConfig.Workstation(transitionID)
	return workstation
}

func replayWorkstationName(workstation *interfaces.FactoryWorkstationConfig, transitionID string) string {
	if workstation == nil {
		return transitionID
	}
	if workstation.Name != "" {
		return workstation.Name
	}
	return transitionID
}

func replayWorkerName(workstation *interfaces.FactoryWorkstationConfig) string {
	if workstation == nil {
		return ""
	}
	return workstation.WorkerTypeName
}

func workIDsFromDispatchRefs(refs []interfaces.DispatchConsumedWorkRef, contextWorkIDs []string) []string {
	out := make([]string, 0, len(refs))
	for i, ref := range refs {
		if id := dispatchConsumedWorkID(ref, i, contextWorkIDs); id != "" {
			out = append(out, id)
		}
	}
	return out
}

func firstTraceIDFromDispatchRefs(refs []interfaces.DispatchConsumedWorkRef, contextWorkIDs []string, workByID map[string]work.Work) string {
	for i, ref := range refs {
		if traceID := workByID[dispatchConsumedWorkID(ref, i, contextWorkIDs)].TraceID; traceID != "" {
			return traceID
		}
	}
	return ""
}

func dispatchConsumedWorkID(ref interfaces.DispatchConsumedWorkRef, index int, contextWorkIDs []string) string {
	if ref.WorkID != "" {
		return ref.WorkID
	}
	if index >= 0 && index < len(contextWorkIDs) {
		return contextWorkIDs[index]
	}
	return ""
}

func resourceValues(resources *[]interfaces.DispatchResourceRef) []interfaces.DispatchResourceRef {
	if resources == nil {
		return nil
	}
	return *resources
}

func isWorkerOutputSource(source string) bool {
	return len(source) >= len("worker-output:") && source[:len("worker-output:")] == "worker-output:"
}

func cloneExpectedArtifactTemplateContext(
	context *work.ExpectedArtifactTemplateContext,
) *work.ExpectedArtifactTemplateContext {
	return context.Clone()
}
