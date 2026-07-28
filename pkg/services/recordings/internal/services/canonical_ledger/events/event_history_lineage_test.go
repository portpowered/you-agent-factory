package events

import (
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workdomain "github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestFactoryEventHistory_RecordWorkRequest_PreservesGeneratedWorkChainingTraceLineage(t *testing.T) {
	eventTime := time.Date(2026, 4, 22, 18, 0, 0, 0, time.UTC)
	history := newTestFactoryEventHistory(eventHistoryProjectionNet(), func() time.Time { return time.Unix(0, 0).UTC() })

	history.RecordWorkRequest(7, work.WorkRequestRecord{
		RequestID: "request-generated-lineage",
		Type:      workdomain.WorkRequestTypeFactoryRequestBatch,
		TraceID:   "trace-generated-current",
		WorkItems: []workdomain.FactoryWorkItem{{
			ID:                       "work-generated-lineage",
			WorkTypeID:               "task",
			DisplayName:              "generated-lineage",
			CurrentChainingTraceID:   "trace-generated-current",
			PreviousChainingTraceIDs: []string{"trace-a", "trace-z"},
			TraceID:                  "trace-generated-current",
			Content: []workdomain.WorkContentPart{
				{Type: workdomain.WorkContentPartTypeText, Text: "review image"},
				{Type: workdomain.WorkContentPartTypeImage, URL: "file://fixtures/review.png"},
			},
		}},
	}, eventTime)

	events := generatedHistoryEvents(t, history)
	if len(events) != 1 {
		t.Fatalf("event count = %d, want 1", len(events))
	}
	payload, err := events[0].Payload.AsWorkRequestEventPayload()
	if err != nil {
		t.Fatalf("work request payload: %v", err)
	}
	if payload.Works == nil || len(*payload.Works) != 1 {
		t.Fatalf("payload works = %#v, want one generated work item", payload.Works)
	}
	work := (*payload.Works)[0]
	if stringValueForEventHistoryTest(work.CurrentChainingTraceId) != "trace-generated-current" {
		t.Fatalf("work current chaining trace ID = %q, want trace-generated-current", stringValueForEventHistoryTest(work.CurrentChainingTraceId))
	}
	if got := stringSliceValueForEventHistoryTest(work.PreviousChainingTraceIds); len(got) != 2 || got[0] != "trace-a" || got[1] != "trace-z" {
		t.Fatalf("work previous chaining trace IDs = %#v, want [trace-a trace-z]", got)
	}
	assertEventHistoryWorkContent(t, work.Content, []workdomain.WorkContentPart{
		{Type: workdomain.WorkContentPartTypeText, Text: "review image"},
		{Type: workdomain.WorkContentPartTypeImage, URL: "file://fixtures/review.png"},
	})
}

func TestFactoryEventHistory_RecordWorkRequest_AppendsWorkOwnedCanonicalPayloads(t *testing.T) {
	eventTime := time.Date(2026, 7, 15, 22, 45, 0, 0, time.UTC)
	history := newTestFactoryEventHistory(eventHistoryProjectionNet(), func() time.Time { return time.Unix(0, 0).UTC() })
	var recorded []interfaces.FactoryEvent
	history.AddEventRecorder(func(event interfaces.FactoryEvent) {
		recorded = append(recorded, event)
	})

	history.RecordWorkRequest(7, work.WorkRequestRecord{
		RequestID:     "request-domain-owned",
		Type:          work.WorkRequestTypeFactoryRequestBatch,
		TraceID:       "trace-domain-owned",
		Source:        "api",
		ParentLineage: []string{"trace-parent"},
		WorkItems: []work.FactoryWorkItem{{
			ID:          "work-domain-owned",
			WorkTypeID:  "task",
			DisplayName: "domain-owned",
			State:       "queued",
			TraceID:     "trace-domain-owned",
		}},
		Relations: []work.FactoryRelation{{
			Type:           string(work.WorkRelationDependsOn),
			SourceWorkName: "domain-owned",
			TargetWorkID:   "work-parent",
			RequiredState:  "done",
		}},
	}, eventTime)

	if len(recorded) != 2 {
		t.Fatalf("canonical event count = %d, want request plus relationship", len(recorded))
	}
	var requestPayload work.WorkRequestEventPayload
	if err := recorded[0].DecodePayload(&requestPayload); err != nil {
		t.Fatalf("decode canonical work request payload: %v", err)
	}
	if requestPayload.Source != "api" || len(requestPayload.Works) != 1 || requestPayload.Works[0].WorkID != "work-domain-owned" {
		t.Fatalf("canonical work request payload = %#v, want Work-owned request fields", requestPayload)
	}
	if len(requestPayload.Relations) != 1 || requestPayload.Relations[0].TargetWorkID != "work-parent" {
		t.Fatalf("canonical request relations = %#v, want target work-parent", requestPayload.Relations)
	}
	var relationshipPayload work.RelationshipChangeRequestEventPayload
	if err := recorded[1].DecodePayload(&relationshipPayload); err != nil {
		t.Fatalf("decode canonical relationship payload: %v", err)
	}
	if relationshipPayload.Relation.TargetWorkID != "work-parent" || relationshipPayload.Relation.RequiredState != "done" {
		t.Fatalf("canonical relationship payload = %#v, want Work-owned relationship", relationshipPayload)
	}

	generated := generatedHistoryEvents(t, history)
	if _, err := generated[0].Payload.AsWorkRequestEventPayload(); err != nil {
		t.Fatalf("decode generated work request boundary: %v", err)
	}
	if _, err := generated[1].Payload.AsRelationshipChangeRequestEventPayload(); err != nil {
		t.Fatalf("decode generated relationship boundary: %v", err)
	}
}

func TestFactoryEventHistory_RecordWorkRequest_UsesCanonicalGeneratedWorkContentTranslation(t *testing.T) {
	eventTime := time.Date(2026, 4, 22, 18, 1, 0, 0, time.UTC)
	history := newTestFactoryEventHistory(eventHistoryProjectionNet(), func() time.Time { return time.Unix(0, 0).UTC() })

	history.RecordWorkRequest(7, work.WorkRequestRecord{
		RequestID: "request-generated-content",
		Type:      workdomain.WorkRequestTypeFactoryRequestBatch,
		TraceID:   "trace-generated-content",
		WorkItems: []workdomain.FactoryWorkItem{
			{
				ID:          "work-generated-content",
				WorkTypeID:  "task",
				DisplayName: "generated-content",
				TraceID:     "trace-generated-content",
				Content: []workdomain.WorkContentPart{
					{Type: workdomain.WorkContentPartTypeText, Text: "alpha"},
					{Type: workdomain.WorkContentPartType("audio"), File: "fixtures/ignored.wav"},
					{Type: workdomain.WorkContentPartTypeImage, URL: "file://fixtures/diagram.png"},
				},
			},
			{
				ID:          "work-empty-content",
				WorkTypeID:  "task",
				DisplayName: "empty-content",
				TraceID:     "trace-empty-content",
			},
		},
	}, eventTime)

	events := generatedHistoryEvents(t, history)
	if len(events) != 1 {
		t.Fatalf("event count = %d, want 1", len(events))
	}
	payload, err := events[0].Payload.AsWorkRequestEventPayload()
	if err != nil {
		t.Fatalf("work request payload: %v", err)
	}
	if payload.Works == nil || len(*payload.Works) != 2 {
		t.Fatalf("payload works = %#v, want two generated work items", payload.Works)
	}
	assertEventHistoryWorkContent(t, (*payload.Works)[0].Content, []workdomain.WorkContentPart{
		{Type: workdomain.WorkContentPartTypeText, Text: "alpha"},
		{Type: workdomain.WorkContentPartTypeImage, URL: "file://fixtures/diagram.png"},
	})
	if (*payload.Works)[1].Content != nil {
		t.Fatalf("work content = %#v, want nil for empty content", (*payload.Works)[1].Content)
	}
}

func TestFactoryEventHistory_RecordWorkstationEvents_PreserveChainingTraceLineage(t *testing.T) {
	eventTime := time.Date(2026, 4, 22, 18, 5, 0, 0, time.UTC)
	history := newTestFactoryEventHistory(eventHistoryProjectionNet(), func() time.Time { return time.Unix(0, 0).UTC() })
	consumed := chainingTraceLineageConsumedTokens()
	history.RecordWorkstationRequest(8, chainingTraceLineageDispatchRecord(consumed), eventTime)
	history.RecordWorkstationResponse(9, chainingTraceLineageResult(), chainingTraceLineageCompletion(eventTime, consumed))

	events := generatedHistoryEvents(t, history)
	if len(events) != 2 {
		t.Fatalf("event count = %d, want 2", len(events))
	}

	assertEventHistoryRequestLineage(t, events[0])
	assertEventHistoryResponseLineage(t, events[1])
}

func TestFactoryEventHistory_RecordWorkstationEvents_CanonicalizesFanInLineageAndPreservesFanOutOutputs(t *testing.T) {
	eventTime := time.Date(2026, 4, 22, 18, 10, 0, 0, time.UTC)
	history := newTestFactoryEventHistory(eventHistoryProjectionNet(), func() time.Time { return time.Unix(0, 0).UTC() })
	record, completion := fanInLineageFixture(eventTime)

	history.RecordWorkstationRequest(12, record, eventTime)
	history.RecordWorkstationResponse(13, chainingTraceLineageResult(), completion)

	events := generatedHistoryEvents(t, history)
	if len(events) != 2 {
		t.Fatalf("event count = %d, want 2", len(events))
	}
	assertFanInLineageRequest(t, events[0])
	assertFanInLineageResponse(t, events[1])
}

func assertEventHistoryWorkContent(t *testing.T, content *factoryapi.WorkContent, want []workdomain.WorkContentPart) {
	t.Helper()
	if content == nil {
		t.Fatalf("work content = nil, want %#v", want)
	}
	if len(*content) != len(want) {
		t.Fatalf("work content count = %d, want %d", len(*content), len(want))
	}
	textPart, err := (*content)[0].AsWorkTextContentPart()
	if err != nil {
		t.Fatalf("decode text content: %v", err)
	}
	if textPart.Text != want[0].Text {
		t.Fatalf("text content = %q, want %q", textPart.Text, want[0].Text)
	}
	imagePart, err := (*content)[1].AsWorkImageContentPart()
	if err != nil {
		t.Fatalf("decode image content: %v", err)
	}
	if string(imagePart.Url) != want[1].URL {
		t.Fatalf("image content url = %q, want %q", imagePart.Url, want[1].URL)
	}
}

func fanInLineageFixture(eventTime time.Time) (interfaces.FactoryDispatchRecord, interfaces.CompletedDispatch) {
	consumed := fanInLineageConsumedTokens()
	record := chainingTraceLineageDispatchRecord(consumed)
	record.Dispatch.CurrentChainingTraceID = "trace-z"
	record.Dispatch.PreviousChainingTraceIDs = []string{"trace-a", "trace-z"}
	record.Dispatch.Execution.WorkIDs = []string{"work-z", "work-a", "work-z-duplicate"}
	return record, fanInLineageCompletion(eventTime, consumed)
}

func fanInLineageConsumedTokens() []workerexecution.Token {
	return []workerexecution.Token{
		newFanInWorkToken("tok-z", "work-z", "source-z", "request-z", "trace-z", []string{"trace-root-z"}),
		{
			ID:      "tok-resource",
			PlaceID: "resource:available",
			Color: workerexecution.Color{
				DataType:                 workerexecution.DataTypeResource,
				WorkTypeID:               "gpu",
				Name:                     "gpu",
				CurrentChainingTraceID:   "trace-resource-ignored",
				PreviousChainingTraceIDs: []string{"trace-resource-parent"},
				TraceID:                  "trace-resource-ignored",
			},
		},
		newFanInWorkToken("tok-a", "work-a", "source-a", "request-a", "trace-a", []string{"trace-root-a"}),
		newFanInWorkToken("tok-z-duplicate", "work-z-duplicate", "source-z-duplicate", "request-z", "trace-z", []string{"trace-root-z"}),
	}
}

func newFanInWorkToken(id, workID, name, requestID, traceID string, previous []string) workerexecution.Token {
	return workerexecution.Token{
		ID:      id,
		PlaceID: "task:init",
		Color: workerexecution.Color{
			DataType:                 workerexecution.DataTypeWork,
			WorkID:                   workID,
			WorkTypeID:               "task",
			Name:                     name,
			RequestID:                requestID,
			CurrentChainingTraceID:   traceID,
			PreviousChainingTraceIDs: previous,
			TraceID:                  traceID,
		},
	}
}

func fanInLineageCompletion(eventTime time.Time, consumed []workerexecution.Token) interfaces.CompletedDispatch {
	return interfaces.CompletedDispatch{
		DispatchID:      "dispatch-lineage",
		TransitionID:    "build",
		WorkstationName: "Build",
		Outcome:         workerexecution.OutcomeAccepted,
		EndTime:         eventTime,
		Duration:        2 * time.Second,
		ConsumedTokens:  consumed,
		OutputMutations: []interfaces.TokenMutationRecord{
			newFanOutWorkMutation("tok-output-a", "task:review", "work-output-a", "fan-out-a", "trace-output-a", []string{"not-used"}),
			newFanOutWorkMutation("tok-output-b", "task:done", "work-output-b", "fan-out-b", "trace-output-b", nil),
			{
				Type: interfaces.MutationCreate,
				Token: &workerexecution.Token{
					ID:      "tok-resource-out",
					PlaceID: "gpu:available",
					Color: workerexecution.Color{
						DataType:   workerexecution.DataTypeResource,
						WorkTypeID: "gpu",
						Name:       "gpu",
					},
				},
			},
		},
	}
}

func newFanOutWorkMutation(id, placeID, workID, name, traceID string, previous []string) interfaces.TokenMutationRecord {
	return interfaces.TokenMutationRecord{
		Type: interfaces.MutationCreate,
		Token: &workerexecution.Token{
			ID:      id,
			PlaceID: placeID,
			Color: workerexecution.Color{
				DataType:                 workerexecution.DataTypeWork,
				WorkID:                   workID,
				WorkTypeID:               "task",
				Name:                     name,
				CurrentChainingTraceID:   traceID,
				PreviousChainingTraceIDs: previous,
				TraceID:                  traceID,
			},
		},
	}
}

func chainingTraceLineageConsumedTokens() []workerexecution.Token {
	return []workerexecution.Token{
		newFanInWorkToken("tok-z", "work-z", "source-z", "request-z", "trace-z", []string{"trace-origin-z"}),
		newFanInWorkToken("tok-a", "work-a", "source-a", "request-a", "trace-a", []string{"trace-origin-a-1", "trace-origin-a-2"}),
	}
}

func chainingTraceLineageDispatchRecord(consumed []workerexecution.Token) interfaces.FactoryDispatchRecord {
	return interfaces.FactoryDispatchRecord{
		DispatchID:  "dispatch-lineage",
		CreatedTick: 8,
		Dispatch: work.WorkDispatch{
			DispatchID:               "dispatch-lineage",
			TransitionID:             "build",
			WorkstationName:          "Build",
			CurrentChainingTraceID:   "trace-z",
			PreviousChainingTraceIDs: []string{"trace-a", "trace-z"},
			InputTokens:              workers.InputTokens(consumed...),
			Execution: work.ExecutionMetadata{
				RequestID: "request-z",
				TraceID:   "trace-z",
				WorkIDs:   []string{"work-z", "work-a"},
			},
		},
	}
}

func chainingTraceLineageResult() workerexecution.WorkResult {
	return workerexecution.WorkResult{
		DispatchID:   "dispatch-lineage",
		TransitionID: "build",
		Outcome:      workerexecution.OutcomeAccepted,
		Output:       "merged output",
	}
}

func chainingTraceLineageCompletion(eventTime time.Time, consumed []workerexecution.Token) interfaces.CompletedDispatch {
	return interfaces.CompletedDispatch{
		DispatchID:      "dispatch-lineage",
		TransitionID:    "build",
		WorkstationName: "Build",
		Outcome:         workerexecution.OutcomeAccepted,
		EndTime:         eventTime,
		Duration:        1500 * time.Millisecond,
		ConsumedTokens:  consumed,
		OutputMutations: []interfaces.TokenMutationRecord{{
			Type: interfaces.MutationCreate,
			Token: &workerexecution.Token{
				ID:      "tok-output",
				PlaceID: "task:done",
				Color: workerexecution.Color{
					DataType:   workerexecution.DataTypeWork,
					WorkID:     "work-output",
					WorkTypeID: "task",
					Name:       "merged-output",
					TraceID:    "trace-output",
				},
			},
		}},
	}
}

func assertEventHistoryRequestLineage(t *testing.T, event factoryapi.FactoryEvent) {
	t.Helper()

	requestPayload, err := event.Payload.AsDispatchRequestEventPayload()
	if err != nil {
		t.Fatalf("dispatch request payload: %v", err)
	}
	if stringValueForEventHistoryTest(requestPayload.CurrentChainingTraceId) != "trace-z" {
		t.Fatalf("dispatch request current chaining trace ID = %q, want trace-z", stringValueForEventHistoryTest(requestPayload.CurrentChainingTraceId))
	}
	if got := stringSliceValueForEventHistoryTest(requestPayload.PreviousChainingTraceIds); len(got) != 2 || got[0] != "trace-a" || got[1] != "trace-z" {
		t.Fatalf("dispatch request previous chaining trace IDs = %#v, want [trace-a trace-z]", got)
	}
	if stringValueForEventHistoryTest(event.Context.CurrentChainingTraceId) != "trace-z" {
		t.Fatalf("dispatch request context current chaining trace ID = %q, want trace-z", stringValueForEventHistoryTest(event.Context.CurrentChainingTraceId))
	}
	if got := stringSliceValueForEventHistoryTest(event.Context.PreviousChainingTraceIds); len(got) != 2 || got[0] != "trace-a" || got[1] != "trace-z" {
		t.Fatalf("dispatch request context previous chaining trace IDs = %#v, want [trace-a trace-z]", got)
	}
	if len(requestPayload.Inputs) != 2 {
		t.Fatalf("dispatch request inputs = %#v, want two consumed work refs", requestPayload.Inputs)
	}
	if requestPayload.Inputs[0].WorkId != "work-z" {
		t.Fatalf("first dispatch request input work ID = %q, want work-z", requestPayload.Inputs[0].WorkId)
	}
	if requestPayload.Inputs[1].WorkId != "work-a" {
		t.Fatalf("second dispatch request input work ID = %q, want work-a", requestPayload.Inputs[1].WorkId)
	}
}

// pkgmaintcheck:ignore-cyclomatic-complexity this helper keeps the full response-lineage contract together across generated work, fan-in, and fan-out assertions.
func assertEventHistoryResponseLineage(t *testing.T, event factoryapi.FactoryEvent) {
	t.Helper()

	responsePayload, err := event.Payload.AsDispatchResponseEventPayload()
	if err != nil {
		t.Fatalf("dispatch response payload: %v", err)
	}
	if stringValueForEventHistoryTest(responsePayload.CurrentChainingTraceId) != "trace-z" {
		t.Fatalf("dispatch response current chaining trace ID = %q, want trace-z", stringValueForEventHistoryTest(responsePayload.CurrentChainingTraceId))
	}
	if got := stringSliceValueForEventHistoryTest(responsePayload.PreviousChainingTraceIds); len(got) != 2 || got[0] != "trace-a" || got[1] != "trace-z" {
		t.Fatalf("dispatch response previous chaining trace IDs = %#v, want [trace-a trace-z]", got)
	}
	if stringValueForEventHistoryTest(event.Context.CurrentChainingTraceId) != "trace-z" {
		t.Fatalf("dispatch response context current chaining trace ID = %q, want trace-z", stringValueForEventHistoryTest(event.Context.CurrentChainingTraceId))
	}
	if got := stringSliceValueForEventHistoryTest(event.Context.PreviousChainingTraceIds); len(got) != 2 || got[0] != "trace-a" || got[1] != "trace-z" {
		t.Fatalf("dispatch response context previous chaining trace IDs = %#v, want [trace-a trace-z]", got)
	}
	if responsePayload.OutputWork == nil || len(*responsePayload.OutputWork) != 1 {
		t.Fatalf("output work = %#v, want one generated output work item", responsePayload.OutputWork)
	}
	outputWork := (*responsePayload.OutputWork)[0]
	if stringValueForEventHistoryTest(outputWork.CurrentChainingTraceId) != "trace-output" {
		t.Fatalf("output work current chaining trace ID = %q, want trace-output", stringValueForEventHistoryTest(outputWork.CurrentChainingTraceId))
	}
	if got := stringSliceValueForEventHistoryTest(outputWork.PreviousChainingTraceIds); len(got) != 2 || got[0] != "trace-a" || got[1] != "trace-z" {
		t.Fatalf("output work previous chaining trace IDs = %#v, want [trace-a trace-z]", got)
	}
}

func assertFanInLineageRequest(t *testing.T, event factoryapi.FactoryEvent) {
	t.Helper()

	requestPayload, err := event.Payload.AsDispatchRequestEventPayload()
	if err != nil {
		t.Fatalf("dispatch request payload: %v", err)
	}
	if got := stringSliceValueForEventHistoryTest(requestPayload.PreviousChainingTraceIds); len(got) != 2 || got[0] != "trace-a" || got[1] != "trace-z" {
		t.Fatalf("dispatch request previous chaining trace IDs = %#v, want [trace-a trace-z]", got)
	}
	if len(requestPayload.Inputs) != 3 {
		t.Fatalf("dispatch request inputs = %#v, want three non-resource work refs", requestPayload.Inputs)
	}
	for _, ref := range requestPayload.Inputs {
		if ref.WorkId == "" {
			t.Fatalf("dispatch request input ref = %#v, want non-empty work ID", ref)
		}
	}
}

func assertFanInLineageResponse(t *testing.T, event factoryapi.FactoryEvent) {
	t.Helper()

	responsePayload, err := event.Payload.AsDispatchResponseEventPayload()
	if err != nil {
		t.Fatalf("dispatch response payload: %v", err)
	}
	if got := stringSliceValueForEventHistoryTest(responsePayload.PreviousChainingTraceIds); len(got) != 2 || got[0] != "trace-a" || got[1] != "trace-z" {
		t.Fatalf("dispatch response previous chaining trace IDs = %#v, want [trace-a trace-z]", got)
	}
	if responsePayload.OutputWork == nil || len(*responsePayload.OutputWork) != 2 {
		t.Fatalf("dispatch response output work = %#v, want two work outputs", responsePayload.OutputWork)
	}
	for _, work := range *responsePayload.OutputWork {
		if got := stringSliceValueForEventHistoryTest(work.PreviousChainingTraceIds); len(got) != 2 || got[0] != "trace-a" || got[1] != "trace-z" {
			t.Fatalf("output work previous chaining trace IDs = %#v, want [trace-a trace-z]", got)
		}
	}
}

func TestFactoryEventHistory_RecordWorkstationResponse_PreserveInputExposesConsumedContentOnOutputWork(t *testing.T) {
	eventTime := time.Date(2026, time.June, 20, 12, 0, 0, 0, time.UTC)
	history := newTestFactoryEventHistory(eventHistoryProjectionNet(), func() time.Time { return time.Unix(0, 0).UTC() })

	consumed := []workerexecution.Token{{
		ID:      "tok-input",
		PlaceID: "task:init",
		Color: workerexecution.Color{
			DataType:   workerexecution.DataTypeWork,
			WorkID:     "work-input",
			WorkTypeID: "task",
			Name:       "Input",
			TraceID:    "trace-input",
			Payload:    []byte("input-payload"),
			Content: []workdomain.WorkContentPart{{
				Type: workdomain.WorkContentPartTypeText,
				Text: "input-content",
			}},
		},
	}}
	outputToken := workerexecution.Token{
		ID:      "work-input",
		PlaceID: "task:done",
		Color: workerexecution.Color{
			DataType:   workerexecution.DataTypeWork,
			WorkID:     "work-input",
			WorkTypeID: "task",
			Name:       "Input",
			TraceID:    "trace-input",
			Payload:    []byte("input-payload"),
			Content: []workdomain.WorkContentPart{{
				Type: workdomain.WorkContentPartTypeText,
				Text: "input-content",
			}},
		},
	}

	history.RecordWorkstationRequest(1, interfaces.FactoryDispatchRecord{
		DispatchID: "dispatch-preserve",
		Dispatch: work.WorkDispatch{
			DispatchID:      "dispatch-preserve",
			TransitionID:    "execute",
			WorkstationName: "Execute",
			InputTokens:     workers.InputTokens(consumed...),
			Execution: work.ExecutionMetadata{
				WorkIDs: []string{"work-input"},
			},
		},
	}, eventTime)
	history.RecordWorkstationResponse(2, workerexecution.WorkResult{
		DispatchID:   "dispatch-preserve",
		TransitionID: "execute",
		Outcome:      workerexecution.OutcomeAccepted,
		Output:       "worker-output",
	}, interfaces.CompletedDispatch{
		DispatchID:     "dispatch-preserve",
		TransitionID:   "execute",
		Outcome:        workerexecution.OutcomeAccepted,
		EndTime:        eventTime,
		Duration:       time.Second,
		ConsumedTokens: consumed,
		OutputMutations: []interfaces.TokenMutationRecord{{
			Type:  interfaces.MutationCreate,
			Token: &outputToken,
		}},
	})

	events := generatedHistoryEvents(t, history)
	if len(events) != 2 {
		t.Fatalf("event count = %d, want 2", len(events))
	}
	response, err := events[1].Payload.AsDispatchResponseEventPayload()
	if err != nil {
		t.Fatalf("dispatch response payload: %v", err)
	}
	if response.OutputWork == nil || len(*response.OutputWork) != 1 {
		t.Fatalf("output work = %#v, want one preserved output work item", response.OutputWork)
	}
	work := (*response.OutputWork)[0]
	if work.Content == nil || len(*work.Content) != 1 {
		t.Fatalf("output work content = %#v, want one preserved content part", work.Content)
	}
	textPart, err := (*work.Content)[0].AsWorkTextContentPart()
	if err != nil {
		t.Fatalf("decode text content: %v", err)
	}
	if textPart.Text != "input-content" {
		t.Fatalf("output work text = %q, want input-content", textPart.Text)
	}
	if stringValueForEventHistoryTest(response.Output) != "worker-output" {
		t.Fatalf("response output = %#v, want worker output retained separately from downstream payload", response.Output)
	}
}

func TestFactoryEventHistory_RecordWorkstationResponse_OutputAsPayloadExposesResponseContentOnOutputWork(t *testing.T) {
	eventTime := time.Date(2026, time.July, 12, 18, 0, 0, 0, time.UTC)
	history := newTestFactoryEventHistory(eventHistoryProjectionNet(), func() time.Time { return time.Unix(0, 0).UTC() })

	consumed := []workerexecution.Token{{
		ID:      "tok-input",
		PlaceID: "task:init",
		Color: workerexecution.Color{
			DataType:   workerexecution.DataTypeWork,
			WorkID:     "work-input",
			WorkTypeID: "task",
			Name:       "Input",
			TraceID:    "trace-input",
			Payload:    []byte("input-payload"),
			Content: []workdomain.WorkContentPart{{
				Type: workdomain.WorkContentPartTypeText,
				Text: "input-content",
			}},
		},
	}}
	outputToken := workerexecution.Token{
		ID:      "work-input",
		PlaceID: "task:done",
		Color: workerexecution.Color{
			DataType:   workerexecution.DataTypeWork,
			WorkID:     "work-input",
			WorkTypeID: "task",
			Name:       "Input",
			TraceID:    "trace-input",
			Payload:    []byte("worker-response"),
			Content: []workdomain.WorkContentPart{{
				Type: workdomain.WorkContentPartTypeText,
				Text: "worker-response",
			}},
		},
	}

	history.RecordWorkstationRequest(1, interfaces.FactoryDispatchRecord{
		DispatchID: "dispatch-response",
		Dispatch: work.WorkDispatch{
			DispatchID:      "dispatch-response",
			TransitionID:    "execute",
			WorkstationName: "Execute",
			InputTokens:     workers.InputTokens(consumed...),
			Execution: work.ExecutionMetadata{
				WorkIDs: []string{"work-input"},
			},
		},
	}, eventTime)
	history.RecordWorkstationResponse(2, workerexecution.WorkResult{
		DispatchID:   "dispatch-response",
		TransitionID: "execute",
		Outcome:      workerexecution.OutcomeAccepted,
		Output:       "worker-response",
	}, interfaces.CompletedDispatch{
		DispatchID:     "dispatch-response",
		TransitionID:   "execute",
		Outcome:        workerexecution.OutcomeAccepted,
		EndTime:        eventTime,
		Duration:       time.Second,
		ConsumedTokens: consumed,
		OutputMutations: []interfaces.TokenMutationRecord{{
			Type:  interfaces.MutationCreate,
			Token: &outputToken,
		}},
	})

	events := generatedHistoryEvents(t, history)
	if len(events) != 2 {
		t.Fatalf("event count = %d, want 2", len(events))
	}
	response, err := events[1].Payload.AsDispatchResponseEventPayload()
	if err != nil {
		t.Fatalf("dispatch response payload: %v", err)
	}
	if response.OutputWork == nil || len(*response.OutputWork) != 1 {
		t.Fatalf("output work = %#v, want one response output work item", response.OutputWork)
	}
	work := (*response.OutputWork)[0]
	if work.Content == nil || len(*work.Content) != 1 {
		t.Fatalf("output work content = %#v, want one response content part", work.Content)
	}
	textPart, err := (*work.Content)[0].AsWorkTextContentPart()
	if err != nil {
		t.Fatalf("decode text content: %v", err)
	}
	if textPart.Text != "worker-response" {
		t.Fatalf("output work text = %q, want worker-response", textPart.Text)
	}
	if textPart.Text == "input-content" {
		t.Fatalf("output work echoed submitted request content")
	}
}

func TestFactoryEventHistory_RecordWorkstationResponse_OutputAsPayloadExposesNextTurnContentOnContinue(t *testing.T) {
	eventTime := time.Date(2026, time.July, 12, 19, 0, 0, 0, time.UTC)
	history := newTestFactoryEventHistory(eventHistoryProjectionNet(), func() time.Time { return time.Unix(0, 0).UTC() })

	consumed := []workerexecution.Token{{
		ID:      "tok-input",
		PlaceID: "task:init",
		Color: workerexecution.Color{
			DataType:   workerexecution.DataTypeWork,
			WorkID:     "work-input",
			WorkTypeID: "task",
			Name:       "Input",
			TraceID:    "trace-input",
			Payload:    []byte("input-payload"),
			Content: []workdomain.WorkContentPart{{
				Type: workdomain.WorkContentPartTypeText,
				Text: "input-content",
			}},
		},
	}}
	outputToken := workerexecution.Token{
		ID:      "work-input",
		PlaceID: "task:init",
		Color: workerexecution.Color{
			DataType:   workerexecution.DataTypeWork,
			WorkID:     "work-input",
			WorkTypeID: "task",
			Name:       "Input",
			TraceID:    "trace-input",
			Payload:    []byte("next-turn-output"),
			Content: []workdomain.WorkContentPart{{
				Type: workdomain.WorkContentPartTypeText,
				Text: "next-turn-output",
			}},
			Tags: map[string]string{"continue_feedback": "needs revision"},
		},
	}

	history.RecordWorkstationRequest(1, interfaces.FactoryDispatchRecord{
		DispatchID: "dispatch-continue",
		Dispatch: work.WorkDispatch{
			DispatchID:      "dispatch-continue",
			TransitionID:    "review",
			WorkstationName: "Review",
			InputTokens:     workers.InputTokens(consumed...),
			Execution: work.ExecutionMetadata{
				WorkIDs: []string{"work-input"},
			},
		},
	}, eventTime)
	history.RecordWorkstationResponse(2, workerexecution.WorkResult{
		DispatchID:   "dispatch-continue",
		TransitionID: "review",
		Outcome:      workerexecution.OutcomeContinue,
		Output:       "next-turn-output",
		Feedback:     "needs revision",
	}, interfaces.CompletedDispatch{
		DispatchID:     "dispatch-continue",
		TransitionID:   "review",
		Outcome:        workerexecution.OutcomeContinue,
		EndTime:        eventTime,
		Duration:       time.Second,
		ConsumedTokens: consumed,
		OutputMutations: []interfaces.TokenMutationRecord{{
			Type:  interfaces.MutationCreate,
			Token: &outputToken,
		}},
	})

	events := generatedHistoryEvents(t, history)
	if len(events) != 2 {
		t.Fatalf("event count = %d, want 2", len(events))
	}
	response, err := events[1].Payload.AsDispatchResponseEventPayload()
	if err != nil {
		t.Fatalf("dispatch response payload: %v", err)
	}
	if response.OutputWork == nil || len(*response.OutputWork) != 1 {
		t.Fatalf("output work = %#v, want one continued output work item", response.OutputWork)
	}
	work := (*response.OutputWork)[0]
	if work.Content == nil || len(*work.Content) != 1 {
		t.Fatalf("output work content = %#v, want one next-turn content part", work.Content)
	}
	textPart, err := (*work.Content)[0].AsWorkTextContentPart()
	if err != nil {
		t.Fatalf("decode text content: %v", err)
	}
	if textPart.Text != "next-turn-output" {
		t.Fatalf("output work text = %q, want next-turn-output", textPart.Text)
	}
	if textPart.Text == "input-content" {
		t.Fatalf("output work echoed submitted request content on continue")
	}
}

func failedPreservesRequestContentFixture(eventTime time.Time) (consumed []workerexecution.Token, outputToken workerexecution.Token) {
	consumed = []workerexecution.Token{{
		ID:      "tok-input",
		PlaceID: "task:init",
		Color: workerexecution.Color{
			DataType:   workerexecution.DataTypeWork,
			WorkID:     "work-input",
			WorkTypeID: "task",
			Name:       "Input",
			TraceID:    "trace-input",
			Payload:    []byte("input-payload"),
			Content: []workdomain.WorkContentPart{{
				Type: workdomain.WorkContentPartTypeText,
				Text: "input-content",
			}},
		},
	}}
	outputToken = workerexecution.Token{
		ID:      "work-input",
		PlaceID: "task:failed",
		Color: workerexecution.Color{
			DataType:   workerexecution.DataTypeWork,
			WorkID:     "work-input",
			WorkTypeID: "task",
			Name:       "Input",
			TraceID:    "trace-input",
			Payload:    []byte("input-payload"),
			Content: []workdomain.WorkContentPart{{
				Type: workdomain.WorkContentPartTypeText,
				Text: "input-content",
			}},
		},
		History: workerexecution.History{
			LastError: "agent crashed",
			FailureLog: []workerexecution.Failure{{
				TransitionID: "execute",
				Error:        "agent crashed",
				Timestamp:    eventTime,
			}},
		},
	}
	return consumed, outputToken
}

func assertFailedPreservesRequestContentResponse(t *testing.T, events []factoryapi.FactoryEvent) {
	t.Helper()
	if len(events) != 2 {
		t.Fatalf("event count = %d, want 2", len(events))
	}
	response, err := events[1].Payload.AsDispatchResponseEventPayload()
	if err != nil {
		t.Fatalf("dispatch response payload: %v", err)
	}
	if response.Outcome != factoryapi.WorkOutcomeFailed {
		t.Fatalf("response outcome = %s, want FAILED", response.Outcome)
	}
	if response.OutputWork == nil || len(*response.OutputWork) != 1 {
		t.Fatalf("output work = %#v, want one failed output work item", response.OutputWork)
	}
	work := (*response.OutputWork)[0]
	if work.Content == nil || len(*work.Content) != 1 {
		t.Fatalf("output work content = %#v, want preserved request content", work.Content)
	}
	textPart, err := (*work.Content)[0].AsWorkTextContentPart()
	if err != nil {
		t.Fatalf("decode text content: %v", err)
	}
	if textPart.Text != "input-content" {
		t.Fatalf("output work text = %q, want input-content", textPart.Text)
	}
	if textPart.Text == "worker-output" {
		t.Fatalf("output work projected worker response as success-shaped content")
	}
	if stringValueForEventHistoryTest(response.Output) != "worker-output" {
		t.Fatalf("response output = %#v, want worker output retained separately from downstream payload", response.Output)
	}
}

func TestFactoryEventHistory_RecordWorkstationResponse_FailedPreservesRequestContentOnOutputWork(t *testing.T) {
	eventTime := time.Date(2026, time.July, 12, 20, 0, 0, 0, time.UTC)
	history := newTestFactoryEventHistory(eventHistoryProjectionNet(), func() time.Time { return time.Unix(0, 0).UTC() })
	consumed, outputToken := failedPreservesRequestContentFixture(eventTime)

	history.RecordWorkstationRequest(1, interfaces.FactoryDispatchRecord{
		DispatchID: "dispatch-failed",
		Dispatch: work.WorkDispatch{
			DispatchID:      "dispatch-failed",
			TransitionID:    "execute",
			WorkstationName: "Execute",
			InputTokens:     workers.InputTokens(consumed...),
			Execution: work.ExecutionMetadata{
				WorkIDs: []string{"work-input"},
			},
		},
	}, eventTime)
	history.RecordWorkstationResponse(2, workerexecution.WorkResult{
		DispatchID:   "dispatch-failed",
		TransitionID: "execute",
		Outcome:      workerexecution.OutcomeFailed,
		Output:       "worker-output",
		Error:        "agent crashed",
	}, interfaces.CompletedDispatch{
		DispatchID:     "dispatch-failed",
		TransitionID:   "execute",
		Outcome:        workerexecution.OutcomeFailed,
		EndTime:        eventTime,
		Duration:       time.Second,
		ConsumedTokens: consumed,
		OutputMutations: []interfaces.TokenMutationRecord{{
			Type:  interfaces.MutationCreate,
			Token: &outputToken,
		}},
	})

	assertFailedPreservesRequestContentResponse(t, generatedHistoryEvents(t, history))
}
