package factory

import (
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/workers"
)

func TestFactoryEventHistory_RecordWorkRequest_PreservesGeneratedWorkChainingTraceLineage(t *testing.T) {
	eventTime := time.Date(2026, 4, 22, 18, 0, 0, 0, time.UTC)
	history := NewFactoryEventHistory(eventHistoryProjectionNet(), func() time.Time { return time.Unix(0, 0).UTC() })

	history.RecordWorkRequest(7, interfaces.WorkRequestRecord{
		RequestID: "request-generated-lineage",
		Type:      interfaces.WorkRequestTypeFactoryRequestBatch,
		TraceID:   "trace-generated-current",
		WorkItems: []interfaces.FactoryWorkItem{{
			ID:                       "work-generated-lineage",
			WorkTypeID:               "task",
			DisplayName:              "generated-lineage",
			CurrentChainingTraceID:   "trace-generated-current",
			PreviousChainingTraceIDs: []string{"trace-a", "trace-z"},
			TraceID:                  "trace-generated-current",
			Content: []interfaces.WorkContentPart{
				{Type: interfaces.WorkContentPartTypeText, Text: "review image"},
				{Type: interfaces.WorkContentPartTypeImage, File: "fixtures/review.png"},
			},
		}},
	}, eventTime)

	events := history.Events()
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
	assertEventHistoryWorkContent(t, work.Content, []interfaces.WorkContentPart{
		{Type: interfaces.WorkContentPartTypeText, Text: "review image"},
		{Type: interfaces.WorkContentPartTypeImage, File: "fixtures/review.png"},
	})
}

func TestFactoryEventHistory_RecordWorkRequest_UsesCanonicalGeneratedWorkContentTranslation(t *testing.T) {
	eventTime := time.Date(2026, 4, 22, 18, 1, 0, 0, time.UTC)
	history := NewFactoryEventHistory(eventHistoryProjectionNet(), func() time.Time { return time.Unix(0, 0).UTC() })

	history.RecordWorkRequest(7, interfaces.WorkRequestRecord{
		RequestID: "request-generated-content",
		Type:      interfaces.WorkRequestTypeFactoryRequestBatch,
		TraceID:   "trace-generated-content",
		WorkItems: []interfaces.FactoryWorkItem{
			{
				ID:          "work-generated-content",
				WorkTypeID:  "task",
				DisplayName: "generated-content",
				TraceID:     "trace-generated-content",
				Content: []interfaces.WorkContentPart{
					{Type: interfaces.WorkContentPartTypeText, Text: "alpha"},
					{Type: interfaces.WorkContentPartType("audio"), File: "fixtures/ignored.wav"},
					{Type: interfaces.WorkContentPartTypeImage, File: "fixtures/diagram.png"},
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

	events := history.Events()
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
	assertEventHistoryWorkContent(t, (*payload.Works)[0].Content, []interfaces.WorkContentPart{
		{Type: interfaces.WorkContentPartTypeText, Text: "alpha"},
		{Type: interfaces.WorkContentPartTypeImage, File: "fixtures/diagram.png"},
	})
	if (*payload.Works)[1].Content != nil {
		t.Fatalf("work content = %#v, want nil for empty content", (*payload.Works)[1].Content)
	}
}

func TestFactoryEventHistory_RecordWorkstationEvents_PreserveChainingTraceLineage(t *testing.T) {
	eventTime := time.Date(2026, 4, 22, 18, 5, 0, 0, time.UTC)
	history := NewFactoryEventHistory(eventHistoryProjectionNet(), func() time.Time { return time.Unix(0, 0).UTC() })
	consumed := chainingTraceLineageConsumedTokens()
	history.RecordWorkstationRequest(8, chainingTraceLineageDispatchRecord(consumed), eventTime)
	history.RecordWorkstationResponse(9, chainingTraceLineageResult(), chainingTraceLineageCompletion(eventTime, consumed))

	events := history.Events()
	if len(events) != 2 {
		t.Fatalf("event count = %d, want 2", len(events))
	}

	assertEventHistoryRequestLineage(t, events[0])
	assertEventHistoryResponseLineage(t, events[1])
}

func TestFactoryEventHistory_RecordWorkstationEvents_CanonicalizesFanInLineageAndPreservesFanOutOutputs(t *testing.T) {
	eventTime := time.Date(2026, 4, 22, 18, 10, 0, 0, time.UTC)
	history := NewFactoryEventHistory(eventHistoryProjectionNet(), func() time.Time { return time.Unix(0, 0).UTC() })
	record, completion := fanInLineageFixture(eventTime)

	history.RecordWorkstationRequest(12, record, eventTime)
	history.RecordWorkstationResponse(13, chainingTraceLineageResult(), completion)

	events := history.Events()
	if len(events) != 2 {
		t.Fatalf("event count = %d, want 2", len(events))
	}
	assertFanInLineageRequest(t, events[0])
	assertFanInLineageResponse(t, events[1])
}

func assertEventHistoryWorkContent(t *testing.T, content *factoryapi.WorkContent, want []interfaces.WorkContentPart) {
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
	if imagePart.File != want[1].File {
		t.Fatalf("image content = %q, want %q", imagePart.File, want[1].File)
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

func fanInLineageConsumedTokens() []interfaces.Token {
	return []interfaces.Token{
		newFanInWorkToken("tok-z", "work-z", "source-z", "request-z", "trace-z", []string{"trace-root-z"}),
		{
			ID:      "tok-resource",
			PlaceID: "resource:available",
			Color: interfaces.TokenColor{
				DataType:                 interfaces.DataTypeResource,
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

func newFanInWorkToken(id, workID, name, requestID, traceID string, previous []string) interfaces.Token {
	return interfaces.Token{
		ID:      id,
		PlaceID: "task:init",
		Color: interfaces.TokenColor{
			DataType:                 interfaces.DataTypeWork,
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

func fanInLineageCompletion(eventTime time.Time, consumed []interfaces.Token) interfaces.CompletedDispatch {
	return interfaces.CompletedDispatch{
		DispatchID:      "dispatch-lineage",
		TransitionID:    "build",
		WorkstationName: "Build",
		Outcome:         interfaces.OutcomeAccepted,
		EndTime:         eventTime,
		Duration:        2 * time.Second,
		ConsumedTokens:  consumed,
		OutputMutations: []interfaces.TokenMutationRecord{
			newFanOutWorkMutation("tok-output-a", "task:review", "work-output-a", "fan-out-a", "trace-output-a", []string{"not-used"}),
			newFanOutWorkMutation("tok-output-b", "task:done", "work-output-b", "fan-out-b", "trace-output-b", nil),
			{
				Type: interfaces.MutationCreate,
				Token: &interfaces.Token{
					ID:      "tok-resource-out",
					PlaceID: "gpu:available",
					Color: interfaces.TokenColor{
						DataType:   interfaces.DataTypeResource,
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
		Token: &interfaces.Token{
			ID:      id,
			PlaceID: placeID,
			Color: interfaces.TokenColor{
				DataType:                 interfaces.DataTypeWork,
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

func chainingTraceLineageConsumedTokens() []interfaces.Token {
	return []interfaces.Token{
		newFanInWorkToken("tok-z", "work-z", "source-z", "request-z", "trace-z", []string{"trace-origin-z"}),
		newFanInWorkToken("tok-a", "work-a", "source-a", "request-a", "trace-a", []string{"trace-origin-a-1", "trace-origin-a-2"}),
	}
}

func chainingTraceLineageDispatchRecord(consumed []interfaces.Token) interfaces.FactoryDispatchRecord {
	return interfaces.FactoryDispatchRecord{
		DispatchID:  "dispatch-lineage",
		CreatedTick: 8,
		Dispatch: interfaces.WorkDispatch{
			DispatchID:               "dispatch-lineage",
			TransitionID:             "build",
			WorkstationName:          "Build",
			CurrentChainingTraceID:   "trace-z",
			PreviousChainingTraceIDs: []string{"trace-a", "trace-z"},
			InputTokens:              workers.InputTokens(consumed...),
			Execution: interfaces.ExecutionMetadata{
				RequestID: "request-z",
				TraceID:   "trace-z",
				WorkIDs:   []string{"work-z", "work-a"},
			},
		},
	}
}

func chainingTraceLineageResult() interfaces.WorkResult {
	return interfaces.WorkResult{
		DispatchID:   "dispatch-lineage",
		TransitionID: "build",
		Outcome:      interfaces.OutcomeAccepted,
		Output:       "merged output",
	}
}

func chainingTraceLineageCompletion(eventTime time.Time, consumed []interfaces.Token) interfaces.CompletedDispatch {
	return interfaces.CompletedDispatch{
		DispatchID:      "dispatch-lineage",
		TransitionID:    "build",
		WorkstationName: "Build",
		Outcome:         interfaces.OutcomeAccepted,
		EndTime:         eventTime,
		Duration:        1500 * time.Millisecond,
		ConsumedTokens:  consumed,
		OutputMutations: []interfaces.TokenMutationRecord{{
			Type: interfaces.MutationCreate,
			Token: &interfaces.Token{
				ID:      "tok-output",
				PlaceID: "task:done",
				Color: interfaces.TokenColor{
					DataType:   interfaces.DataTypeWork,
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
