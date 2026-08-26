package replay

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestReplayCompletionFromEvent_DecodesWorkerExecutionPayload(t *testing.T) {
	dispatchID := "dispatch-domain"
	event := interfaces.FactoryEvent{
		Id:   "factory-event/dispatch-completed/dispatch-domain",
		Type: interfaces.FactoryEventTypeDispatchResponse,
		Context: interfaces.FactoryEventContext{
			DispatchID: &dispatchID,
			Tick:       3,
		},
		Payload: json.RawMessage(`{
			"completionId":"completion-domain","transitionId":"process","outcome":"FAILED",
			"output":"partial output","error":"provider timed out","feedback":"retry later",
			"selectedClassificationLabel":"needs-review",
			"providerFailure":{"family":"retryable","type":"timeout"},
			"metrics":{"durationMillis":1250,"cost":0.42,"retryCount":2},
			"outputWork":[{"name":"follow-up","workId":"work-output-1","workTypeName":"task","state":{"name":"ready"},"traceId":"trace-output","content":[{"type":"TEXT","text":"inspect me"}],"tags":{"kind":"follow-up"}}]
		}`),
	}

	completion, err := replayCompletionFromEvent(event, replayInferenceAttempt{})
	if err != nil {
		t.Fatalf("replayCompletionFromEvent: %v", err)
	}
	want := replayCompletion{
		eventID:      event.Id,
		completionID: "completion-domain",
		dispatchID:   dispatchID,
		observedTick: 3,
		result: workerexecution.WorkResult{
			DispatchID:                  dispatchID,
			TransitionID:                "process",
			Outcome:                     workerexecution.OutcomeFailed,
			Output:                      "partial output",
			OutputContent:               []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "inspect me"}},
			Error:                       "provider timed out",
			Feedback:                    "retry later",
			SelectedClassificationLabel: "needs-review",
			RecordedOutputWork: []work.FactoryWorkItem{{
				ID:                     "work-output-1",
				WorkTypeID:             "task",
				State:                  "ready",
				DisplayName:            "follow-up",
				CurrentChainingTraceID: "trace-output",
				TraceID:                "trace-output",
				Content:                []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "inspect me"}},
				Tags:                   map[string]string{"kind": "follow-up"},
			}},
			FailureMetadata: &workerexecution.WorkFailureMetadata{Family: workerexecution.WorkFailureFamilyRetryable, Type: workerexecution.WorkFailureTypeTimeout},
			Metrics:         workerexecution.WorkMetrics{Duration: 1250 * time.Millisecond, Cost: 0.42, RetryCount: 2},
		},
	}
	if !reflect.DeepEqual(completion, want) {
		t.Fatalf("completion = %#v, want %#v", completion, want)
	}
}

func TestReplayCompletionFromEvent_RejectsMalformedWorkerExecutionPayload(t *testing.T) {
	event := interfaces.FactoryEvent{Id: "event-bad", Type: interfaces.FactoryEventTypeDispatchResponse, Payload: json.RawMessage(`{"outcome":`)}
	if _, err := replayCompletionFromEvent(event, replayInferenceAttempt{}); err == nil {
		t.Fatal("replayCompletionFromEvent error = nil, want malformed payload error")
	}
}

func TestReplaySubmissionsFromEventDecodesWorkOwnedPayload(t *testing.T) {
	payload, err := json.Marshal(work.WorkRequestEventPayload{
		Source: "api",
		Type:   work.WorkRequestTypeFactoryRequestBatch,
		Works: []work.WorkRequestEventWork{
			{
				Name:       "source",
				WorkID:     "work-1",
				WorkTypeID: "task",
				State:      &work.WorkEventState{Name: "queued"},
				Content:    []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "hello"}},
				Payload:    json.RawMessage(`{"priority":2}`),
				Tags:       map[string]string{"team": "runtime"},
			},
			{Name: "target", WorkID: "work-2", WorkTypeID: "task"},
		},
		Relations: []work.WorkRequestEventRelation{{
			Type:           work.WorkRelationDependsOn,
			SourceWorkName: "work-1",
			TargetWorkID:   "work-2",
			RequiredState:  "done",
		}},
	})
	if err != nil {
		t.Fatalf("marshal Work payload: %v", err)
	}
	requestID := "request-1"
	workIDs := []string{"context-work-1", "context-work-2"}
	traceIDs := []string{"trace-1", "trace-2"}
	event := interfaces.FactoryEvent{
		Id:            "event-1",
		SchemaVersion: interfaces.FactoryEventSchemaVersionV1,
		Type:          interfaces.FactoryEventTypeWorkRequest,
		Context: interfaces.FactoryEventContext{
			EventTime: time.Date(2026, time.July, 15, 18, 30, 0, 0, time.UTC),
			RequestID: &requestID,
			WorkIDs:   &workIDs,
			TraceIDs:  &traceIDs,
			Tick:      4,
		},
		Payload: payload,
	}

	submissions, err := replaySubmissionsFromEvent(event)
	if err != nil {
		t.Fatalf("replaySubmissionsFromEvent: %v", err)
	}
	if len(submissions) != 1 {
		t.Fatalf("submissions = %d, want 1", len(submissions))
	}
	got := submissions[0]
	if got.eventID != event.Id || got.observedTick != 4 || got.source != "api" {
		t.Fatalf("submission metadata = %#v", got)
	}
	if got.request.RequestID != requestID || got.request.Type != work.WorkRequestTypeFactoryRequestBatch {
		t.Fatalf("request identity = %#v", got.request)
	}
	if got.request.Works[0].TraceID != "trace-1" || got.request.Works[1].TraceID != "trace-2" {
		t.Fatalf("context trace fallbacks = %#v", got.request.Works)
	}
	if string(got.request.Works[0].Payload.([]byte)) != `{"priority":2}` {
		t.Fatalf("payload = %q", got.request.Works[0].Payload)
	}
	if !reflect.DeepEqual(got.request.Works[0].Content, []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "hello"}}) {
		t.Fatalf("content = %#v", got.request.Works[0].Content)
	}
	wantRelations := []work.WorkRelation{{Type: work.WorkRelationDependsOn, SourceWorkName: "source", TargetWorkID: "work-2", TargetWorkName: "target", RequiredState: "done"}}
	if !reflect.DeepEqual(got.request.Relations, wantRelations) {
		t.Fatalf("relations = %#v, want %#v", got.request.Relations, wantRelations)
	}
}

func TestReplaySubmissionsFromEventPreservesIDOnlyCrossBatchDependency(t *testing.T) {
	payload, err := json.Marshal(work.WorkRequestEventPayload{
		Source: "api",
		Type:   work.WorkRequestTypeFactoryRequestBatch,
		Works: []work.WorkRequestEventWork{{
			Name:       "dependent",
			WorkID:     "work-dependent",
			WorkTypeID: "task",
			State:      &work.WorkEventState{Name: "queued"},
		}},
		Relations: []work.WorkRequestEventRelation{{
			Type:           work.WorkRelationDependsOn,
			SourceWorkName: "dependent",
			TargetWorkID:   "work-prior",
			TargetWorkName: "work-prior",
			RequiredState:  "complete",
		}},
	})
	if err != nil {
		t.Fatalf("marshal cross-batch ID-only payload: %v", err)
	}
	event := interfaces.FactoryEvent{
		Id:      "event-cross-batch-id-only",
		Payload: payload,
	}

	submissions, err := replaySubmissionsFromEvent(event)
	if err != nil {
		t.Fatalf("replaySubmissionsFromEvent: %v", err)
	}
	if len(submissions) != 1 || len(submissions[0].request.Relations) != 1 {
		t.Fatalf("replayed submissions = %#v, want one request with one relation", submissions)
	}
	relation := submissions[0].request.Relations[0]
	if relation.TargetWorkID != "work-prior" || relation.TargetWorkName != "" {
		t.Fatalf("replayed ID-only relation = %#v, want target ID with no authored target name", relation)
	}
}

func TestReplaySubmissionsFromEventRejectsMalformedWorkOwnedPayload(t *testing.T) {
	event := interfaces.FactoryEvent{Id: "event-bad", Type: interfaces.FactoryEventTypeWorkRequest, Payload: json.RawMessage(`{"works":`)}
	if _, err := replaySubmissionsFromEvent(event); err == nil {
		t.Fatal("replaySubmissionsFromEvent error = nil, want malformed payload error")
	}
}

func TestReplayWorkStateChangeFromEvent_DecodesDomainPayloadAndContextFallbacks(t *testing.T) {
	requestID := "request-recover"
	workIDs := []string{"work-recover"}
	triggerWorkID := "work-trigger"
	reason := "operator retry"
	payload, err := json.Marshal(interfaces.WorkStateChangeEventPayload{
		WorkTypeName:  "task",
		FromState:     "failed",
		ToState:       "init",
		Source:        work.WorkStateChangeSourceCLI,
		TriggerWorkID: &triggerWorkID,
		Reason:        &reason,
	})
	if err != nil {
		t.Fatalf("marshal domain payload: %v", err)
	}

	change, err := replayWorkStateChangeFromEvent(interfaces.FactoryEvent{
		Id:      "factory-event/work-state-change/work-recover/4",
		Type:    interfaces.FactoryEventTypeWorkStateChange,
		Payload: payload,
		Context: interfaces.FactoryEventContext{
			Tick:      4,
			RequestID: &requestID,
			WorkIDs:   &workIDs,
		},
	})
	if err != nil {
		t.Fatalf("replayWorkStateChangeFromEvent: %v", err)
	}
	if change == nil {
		t.Fatal("change = nil, want operator move")
	}
	want := work.WorkStateChangeRecord{
		WorkID:        "work-recover",
		WorkTypeName:  "task",
		FromState:     "failed",
		ToState:       "init",
		Source:        work.WorkStateChangeSourceCLI,
		RequestID:     requestID,
		TriggerWorkID: triggerWorkID,
		Reason:        reason,
	}
	if change.observedTick != 4 || change.change != want {
		t.Fatalf("change = %#v, want tick 4 and %#v", change, want)
	}
}

func replayDispatchFromGeneratedEvent(t testing.TB, factory factoryapi.Factory, event factoryapi.FactoryEvent, workByID map[string]work.Work) (replayDispatch, error) {
	t.Helper()
	runtimeConfig, err := RuntimeConfigFromGeneratedFactory(factory)
	if err != nil && generatedFactoryHasConfig(factory) {
		t.Fatalf("convert generated replay factory: %v", err)
	}
	domainEvent, err := interfaces.NewFactoryEvent(event)
	if err != nil {
		t.Fatalf("convert generated dispatch event: %v", err)
	}
	return replayDispatchFromEvent(runtimeConfig, domainEvent, workByID)
}

func TestReplayDispatchFromEvent_PreservesConsumedInputChainingLineage(t *testing.T) {
	payload := factoryapi.DispatchRequestEventPayload{
		TransitionId: "merge",
		Inputs:       []factoryapi.DispatchConsumedWorkRef{{WorkId: "work-generated"}},
	}
	var union factoryapi.FactoryEvent_Payload
	if err := union.FromDispatchRequestEventPayload(payload); err != nil {
		t.Fatalf("encode dispatch payload: %v", err)
	}

	replayed, err := replayDispatchFromGeneratedEvent(t, factoryapi.Factory{}, factoryapi.FactoryEvent{
		Id:            "factory-event/dispatch-created/dispatch-1",
		SchemaVersion: factoryapi.AgentFactoryEventV1,
		Type:          factoryapi.FactoryEventTypeDispatchRequest,
		Context: factoryapi.FactoryEventContext{
			EventTime:  time.Date(2026, 4, 22, 19, 5, 0, 0, time.UTC),
			Tick:       5,
			DispatchId: stringPtrIfNotEmpty("dispatch-1"),
			TraceIds:   slicePtr([]string{"trace-generated"}),
			WorkIds:    slicePtr([]string{"work-generated"}),
		},
		Payload: union,
	}, map[string]work.Work{
		"work-generated": {
			WorkID:                   "work-generated",
			Name:                     "generated-merge-input",
			WorkTypeID:               "task",
			CurrentChainingTraceID:   "trace-generated",
			PreviousChainingTraceIDs: []string{"trace-parent-a", "trace-parent-z"},
			TraceID:                  "trace-generated",
		},
	})
	if err != nil {
		t.Fatalf("replayDispatchFromEvent: %v", err)
	}

	tokens := workers.WorkDispatchInputTokens(replayed.dispatch)
	if len(tokens) != 1 {
		t.Fatalf("replayed input tokens = %#v, want one token", tokens)
	}
	if tokens[0].Color.CurrentChainingTraceID != "trace-generated" {
		t.Fatalf("replayed token current chaining trace ID = %q, want trace-generated", tokens[0].Color.CurrentChainingTraceID)
	}
	if got := tokens[0].Color.PreviousChainingTraceIDs; len(got) != 2 || got[0] != "trace-parent-a" || got[1] != "trace-parent-z" {
		t.Fatalf("replayed token previous chaining trace IDs = %#v, want [trace-parent-a trace-parent-z]", got)
	}
	if got := replayed.dispatch.PreviousChainingTraceIDs; len(got) != 1 || got[0] != "trace-generated" {
		t.Fatalf("replayed dispatch previous chaining trace IDs = %#v, want [trace-generated]", got)
	}
}

func TestReplayDispatchFromEvent_PreservesExpectedArtifactTemplateContext(t *testing.T) {
	payload := factoryapi.DispatchRequestEventPayload{
		TransitionId: "publish",
		Inputs:       []factoryapi.DispatchConsumedWorkRef{{WorkId: "work-generated"}},
		ExpectedArtifactContext: &factoryapi.ExpectedArtifactTemplateContext{
			Inputs: &[]factoryapi.ExpectedArtifactTemplateInput{{
				Name:    stringPtrIfNotEmpty("input-7"),
				Project: stringPtrIfNotEmpty("input-project-7"),
				Payload: stringPtrIfNotEmpty("payload-7"),
			}},
			Project:   stringPtrIfNotEmpty("project-7"),
			SessionId: stringPtrIfNotEmpty("session-9"),
		},
	}
	var union factoryapi.FactoryEvent_Payload
	if err := union.FromDispatchRequestEventPayload(payload); err != nil {
		t.Fatalf("encode dispatch payload: %v", err)
	}
	replayed, err := replayDispatchFromGeneratedEvent(t, factoryapi.Factory{}, factoryapi.FactoryEvent{
		Id:            "factory-event/dispatch-created/dispatch-context",
		SchemaVersion: factoryapi.AgentFactoryEventV1,
		Type:          factoryapi.FactoryEventTypeDispatchRequest,
		Context: factoryapi.FactoryEventContext{
			EventTime:  time.Date(2026, 4, 22, 19, 9, 0, 0, time.UTC),
			Tick:       8,
			DispatchId: stringPtrIfNotEmpty("dispatch-context"),
			WorkIds:    slicePtr([]string{"work-generated"}),
		},
		Payload: union,
	}, map[string]work.Work{
		"work-generated": {WorkID: "work-generated", Name: "generated", WorkTypeID: "task"},
	})
	if err != nil {
		t.Fatalf("replayDispatchFromEvent: %v", err)
	}
	if replayed.dispatch.ExpectedArtifactContext == nil ||
		replayed.dispatch.ExpectedArtifactContext.Project != "project-7" ||
		replayed.dispatch.ExpectedArtifactContext.SessionID != "session-9" {
		t.Fatalf("replayed expected artifact context = %#v", replayed.dispatch.ExpectedArtifactContext)
	}
	if replayed.dispatch.ExpectedArtifactContext == nil ||
		len(replayed.dispatch.ExpectedArtifactContext.Inputs) != 1 ||
		replayed.dispatch.ExpectedArtifactContext.Inputs[0].Name != "input-7" ||
		replayed.dispatch.ExpectedArtifactContext.Inputs[0].Project != "input-project-7" ||
		replayed.dispatch.ExpectedArtifactContext.Inputs[0].Payload != "payload-7" {
		t.Fatalf("replayed expected artifact inputs = %#v", replayed.dispatch.ExpectedArtifactContext)
	}
}

func TestReplayDispatchFromEvent_PrefersContextChainingLineageOverPayloadCompatibilityCopy(t *testing.T) {
	payload := factoryapi.DispatchRequestEventPayload{
		TransitionId:             "merge",
		CurrentChainingTraceId:   stringPtrIfNotEmpty("payload-current"),
		PreviousChainingTraceIds: slicePtr([]string{"payload-a", "payload-z"}),
		Inputs:                   []factoryapi.DispatchConsumedWorkRef{{WorkId: "work-generated"}},
	}
	var union factoryapi.FactoryEvent_Payload
	if err := union.FromDispatchRequestEventPayload(payload); err != nil {
		t.Fatalf("encode dispatch payload: %v", err)
	}

	replayed, err := replayDispatchFromGeneratedEvent(t, factoryapi.Factory{}, factoryapi.FactoryEvent{
		Id:            "factory-event/dispatch-created/dispatch-context-first",
		SchemaVersion: factoryapi.AgentFactoryEventV1,
		Type:          factoryapi.FactoryEventTypeDispatchRequest,
		Context: factoryapi.FactoryEventContext{
			EventTime:                time.Date(2026, 4, 22, 19, 7, 0, 0, time.UTC),
			Tick:                     6,
			DispatchId:               stringPtrIfNotEmpty("dispatch-context-first"),
			CurrentChainingTraceId:   stringPtrIfNotEmpty("context-current"),
			PreviousChainingTraceIds: slicePtr([]string{"context-a", "context-z"}),
			TraceIds:                 slicePtr([]string{"trace-generated"}),
			WorkIds:                  slicePtr([]string{"work-generated"}),
		},
		Payload: union,
	}, map[string]work.Work{
		"work-generated": {
			WorkID:                   "work-generated",
			Name:                     "generated-merge-input",
			WorkTypeID:               "task",
			CurrentChainingTraceID:   "trace-generated",
			PreviousChainingTraceIDs: []string{"trace-parent-a", "trace-parent-z"},
			TraceID:                  "trace-generated",
		},
	})
	if err != nil {
		t.Fatalf("replayDispatchFromEvent: %v", err)
	}

	if replayed.dispatch.CurrentChainingTraceID != "context-current" {
		t.Fatalf("replayed dispatch current chaining trace ID = %q, want context-current", replayed.dispatch.CurrentChainingTraceID)
	}
	if got := replayed.dispatch.PreviousChainingTraceIDs; len(got) != 2 || got[0] != "context-a" || got[1] != "context-z" {
		t.Fatalf("replayed dispatch previous chaining trace IDs = %#v, want [context-a context-z]", got)
	}
}

func TestReplayDispatchFromEvent_FallsBackToContextWorkIDsWhenConsumedRefsOmitWorkID(t *testing.T) {
	payload := factoryapi.DispatchRequestEventPayload{
		TransitionId: "process",
		Inputs:       []factoryapi.DispatchConsumedWorkRef{{}},
	}
	var union factoryapi.FactoryEvent_Payload
	if err := union.FromDispatchRequestEventPayload(payload); err != nil {
		t.Fatalf("encode dispatch payload: %v", err)
	}

	replayed, err := replayDispatchFromGeneratedEvent(t, factoryapi.Factory{}, factoryapi.FactoryEvent{
		Id:            "factory-event/dispatch-created/dispatch-legacy",
		SchemaVersion: factoryapi.AgentFactoryEventV1,
		Type:          factoryapi.FactoryEventTypeDispatchRequest,
		Context: factoryapi.FactoryEventContext{
			EventTime:  time.Date(2026, 4, 22, 19, 6, 0, 0, time.UTC),
			Tick:       1,
			DispatchId: stringPtrIfNotEmpty("dispatch-legacy"),
			TraceIds:   slicePtr([]string{"trace-task-1"}),
			WorkIds:    slicePtr([]string{"work-task-1"}),
		},
		Payload: union,
	}, map[string]work.Work{
		"work-task-1": {
			WorkID:                 "work-task-1",
			Name:                   "task-1",
			WorkTypeID:             "task",
			CurrentChainingTraceID: "trace-task-1",
			TraceID:                "trace-task-1",
		},
	})
	if err != nil {
		t.Fatalf("replayDispatchFromEvent: %v", err)
	}

	if got := replayed.dispatch.Execution.WorkIDs; len(got) != 1 || got[0] != "work-task-1" {
		t.Fatalf("replayed dispatch work IDs = %#v, want [work-task-1]", got)
	}
	tokens := workers.WorkDispatchInputTokens(replayed.dispatch)
	if len(tokens) != 1 {
		t.Fatalf("replayed input tokens = %#v, want one token", tokens)
	}
	if tokens[0].Color.WorkID != "work-task-1" {
		t.Fatalf("replayed token work ID = %q, want work-task-1", tokens[0].Color.WorkID)
	}
	if tokens[0].ID != "work-task-1" {
		t.Fatalf("replayed token ID = %q, want work-task-1", tokens[0].ID)
	}
}

func TestReplayDispatchFromEvent_FallsBackToPayloadChainingLineageForLegacyEvents(t *testing.T) {
	payload := factoryapi.DispatchRequestEventPayload{
		TransitionId:             "legacy",
		CurrentChainingTraceId:   stringPtrIfNotEmpty("payload-current"),
		PreviousChainingTraceIds: slicePtr([]string{"payload-a", "payload-z"}),
	}
	var union factoryapi.FactoryEvent_Payload
	if err := union.FromDispatchRequestEventPayload(payload); err != nil {
		t.Fatalf("encode dispatch payload: %v", err)
	}

	replayed, err := replayDispatchFromGeneratedEvent(t, factoryapi.Factory{}, factoryapi.FactoryEvent{
		Id:            "factory-event/dispatch-created/dispatch-payload-fallback",
		SchemaVersion: factoryapi.AgentFactoryEventV1,
		Type:          factoryapi.FactoryEventTypeDispatchRequest,
		Context: factoryapi.FactoryEventContext{
			EventTime:  time.Date(2026, 4, 22, 19, 8, 0, 0, time.UTC),
			Tick:       7,
			DispatchId: stringPtrIfNotEmpty("dispatch-payload-fallback"),
			TraceIds:   slicePtr([]string{"trace-generated"}),
		},
		Payload: union,
	}, nil)
	if err != nil {
		t.Fatalf("replayDispatchFromEvent: %v", err)
	}

	if replayed.dispatch.CurrentChainingTraceID != "payload-current" {
		t.Fatalf("replayed dispatch current chaining trace ID = %q, want payload-current", replayed.dispatch.CurrentChainingTraceID)
	}
	if got := replayed.dispatch.PreviousChainingTraceIDs; len(got) != 2 || got[0] != "payload-a" || got[1] != "payload-z" {
		t.Fatalf("replayed dispatch previous chaining trace IDs = %#v, want [payload-a payload-z]", got)
	}
}

func TestReplayDispatchFromEvent_DerivesCanonicalPreviousLineageFromMixedInputs(t *testing.T) {
	payload := factoryapi.DispatchRequestEventPayload{
		TransitionId: "merge",
		Inputs: []factoryapi.DispatchConsumedWorkRef{
			{WorkId: "work-z"},
			{WorkId: "work-a"},
			{WorkId: "work-z-duplicate"},
		},
		Resources: &[]factoryapi.Resource{{Name: "gpu"}},
	}
	var union factoryapi.FactoryEvent_Payload
	if err := union.FromDispatchRequestEventPayload(payload); err != nil {
		t.Fatalf("encode dispatch payload: %v", err)
	}

	replayed, err := replayDispatchFromGeneratedEvent(t, factoryapi.Factory{}, factoryapi.FactoryEvent{
		Id:            "factory-event/dispatch-created/dispatch-fan-in",
		SchemaVersion: factoryapi.AgentFactoryEventV1,
		Type:          factoryapi.FactoryEventTypeDispatchRequest,
		Context: factoryapi.FactoryEventContext{
			EventTime:  time.Date(2026, 4, 22, 19, 9, 0, 0, time.UTC),
			Tick:       8,
			DispatchId: stringPtrIfNotEmpty("dispatch-fan-in"),
			TraceIds:   slicePtr([]string{"trace-z", "trace-a", "trace-z"}),
			WorkIds:    slicePtr([]string{"work-z", "work-a", "work-z-duplicate"}),
		},
		Payload: union,
	}, map[string]work.Work{
		"work-z": {
			WorkID:                   "work-z",
			Name:                     "work-z",
			WorkTypeID:               "task",
			CurrentChainingTraceID:   "trace-z",
			PreviousChainingTraceIDs: []string{"trace-root-z"},
			TraceID:                  "trace-z",
		},
		"work-a": {
			WorkID:                   "work-a",
			Name:                     "work-a",
			WorkTypeID:               "task",
			CurrentChainingTraceID:   "trace-a",
			PreviousChainingTraceIDs: []string{"trace-root-a"},
			TraceID:                  "trace-a",
		},
		"work-z-duplicate": {
			WorkID:                   "work-z-duplicate",
			Name:                     "work-z-duplicate",
			WorkTypeID:               "task",
			CurrentChainingTraceID:   "trace-z",
			PreviousChainingTraceIDs: []string{"trace-root-z"},
			TraceID:                  "trace-z",
		},
	})
	if err != nil {
		t.Fatalf("replayDispatchFromEvent: %v", err)
	}

	if got := replayed.dispatch.PreviousChainingTraceIDs; len(got) != 2 || got[0] != "trace-a" || got[1] != "trace-z" {
		t.Fatalf("replayed dispatch previous chaining trace IDs = %#v, want [trace-a trace-z]", got)
	}
	tokens := workers.WorkDispatchInputTokens(replayed.dispatch)
	if len(tokens) != 4 {
		t.Fatalf("replayed input tokens = %#v, want three work tokens plus one resource token", tokens)
	}
	if tokens[3].Color.DataType != workers.DataTypeResource {
		t.Fatalf("replayed resource token data type = %q, want resource", tokens[3].Color.DataType)
	}
}
