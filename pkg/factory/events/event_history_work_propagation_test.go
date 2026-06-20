package events

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/workers"
)

func TestFactoryEventHistory_RecordWorkstationResponse_PreserveInputExposesConsumedContentOnOutputWork(t *testing.T) {
	eventTime := time.Date(2026, time.June, 20, 12, 0, 0, 0, time.UTC)
	history := NewFactoryEventHistory(eventHistoryProjectionNet(), func() time.Time { return time.Unix(0, 0).UTC() })

	consumed := []interfaces.Token{{
		ID:      "tok-input",
		PlaceID: "task:init",
		Color: interfaces.TokenColor{
			DataType:   interfaces.DataTypeWork,
			WorkID:     "work-input",
			WorkTypeID: "task",
			Name:       "Input",
			TraceID:    "trace-input",
			Payload:    []byte("input-payload"),
			Content: []interfaces.WorkContentPart{{
				Type: interfaces.WorkContentPartTypeText,
				Text: "input-content",
			}},
		},
	}}
	outputToken := interfaces.Token{
		ID:      "work-input",
		PlaceID: "task:done",
		Color: interfaces.TokenColor{
			DataType:   interfaces.DataTypeWork,
			WorkID:     "work-input",
			WorkTypeID: "task",
			Name:       "Input",
			TraceID:    "trace-input",
			Payload:    []byte("input-payload"),
			Content: []interfaces.WorkContentPart{{
				Type: interfaces.WorkContentPartTypeText,
				Text: "input-content",
			}},
		},
	}

	history.RecordWorkstationRequest(1, interfaces.FactoryDispatchRecord{
		DispatchID: "dispatch-preserve",
		Dispatch: interfaces.WorkDispatch{
			DispatchID:      "dispatch-preserve",
			TransitionID:    "execute",
			WorkstationName: "Execute",
			InputTokens:     workers.InputTokens(consumed...),
			Execution: interfaces.ExecutionMetadata{
				WorkIDs: []string{"work-input"},
			},
		},
	}, eventTime)
	history.RecordWorkstationResponse(2, interfaces.WorkResult{
		DispatchID:   "dispatch-preserve",
		TransitionID: "execute",
		Outcome:      interfaces.OutcomeAccepted,
		Output:       "worker-output",
	}, interfaces.CompletedDispatch{
		DispatchID:     "dispatch-preserve",
		TransitionID:   "execute",
		Outcome:        interfaces.OutcomeAccepted,
		EndTime:        eventTime,
		Duration:       time.Second,
		ConsumedTokens: consumed,
		OutputMutations: []interfaces.TokenMutationRecord{{
			Type:  interfaces.MutationCreate,
			Token: &outputToken,
		}},
	})

	events := history.Events()
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
