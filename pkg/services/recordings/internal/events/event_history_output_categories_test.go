package events

import (
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	work "github.com/portpowered/infinite-you/pkg/services/work"
	workers "github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestRecordWorkstationResponseStartFailedPreservesAuthoredStateCategories(t *testing.T) {
	t.Parallel()
	for name, category := range map[string]string{"escalated": "FAILED", "fin": "FAILED", "retry": "PROCESSING"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			now := time.Date(2026, 10, 6, 18, 0, 0, 0, time.UTC)
			source := eventHistoryProjectionSource{payload: interfaces.InitialStructurePayload{
				Places: []interfaces.FactoryPlace{{ID: "task:" + name, TypeID: "task", State: name, Category: category}},
			}}
			history := newTestFactoryEventHistory(source, func() time.Time { return now })
			token := workers.Token{ID: "token-1", State: name, Color: workers.Color{WorkID: "work-1", WorkTypeID: "task", DataType: workers.DataTypeWork}}
			history.RecordWorkstationResponse(3, workers.WorkResult{
				DispatchID: "start-failed", TransitionID: "review", Outcome: workers.OutcomeFailed,
				Error: "command line too long", FailureDetail: &workers.FailureDetail{Reason: workers.WorkFailureTypeCommandLineTooLong, Message: "command line too long"},
			}, interfaces.CompletedDispatch{
				EndTime: now, ConsumedTokens: []workers.Token{token},
				OutputMutations: []interfaces.TokenMutationRecord{{Type: interfaces.MutationCreate, TokenID: token.ID, ToPlace: "task:" + name, Token: &token}},
			})
			stream, err := history.Subscribe(t.Context(), nil, interfaces.FactoryEventReconnectScope{})
			if err != nil {
				t.Fatal(err)
			}
			if len(stream.History) != 1 {
				t.Fatalf("events=%d, want one terminal response", len(stream.History))
			}
			var payload workers.DispatchResponseEventPayload
			if err := stream.History[0].DecodePayload(&payload); err != nil {
				t.Fatal(err)
			}
			if payload.Outcome != workers.OutcomeFailed || payload.FailureDetail == nil || payload.FailureDetail.Reason != workers.WorkFailureTypeCommandLineTooLong || payload.OutputWork == nil || len(*payload.OutputWork) != 1 {
				t.Fatalf("terminal start failure payload=%#v", payload)
			}
			output := (*payload.OutputWork)[0]
			if output.WorkID != "work-1" || output.State == nil || *output.State != (work.WorkEventState{Name: name, Type: category}) {
				t.Fatalf("output state=%#v, want %s/%s", output.State, name, category)
			}
		})
	}
}
