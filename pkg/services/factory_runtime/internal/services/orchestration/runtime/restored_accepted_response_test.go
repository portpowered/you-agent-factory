package runtime

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil/recordingfixtures"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestRecordedResponseRejectsIncompleteAcceptedOutputBeforeReconciliation(t *testing.T) {
	t.Parallel()
	for _, test := range incompleteAcceptedResponseCases() {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			events := acceptedResponseFixture(t)
			events = test.mutate(t, events)
			ledger := &recordingfixtures.ScriptedRuntimeLedger{Events: events}
			before := ledger.CanonicalEvents()
			for index := range before {
				before[index].Payload = append(json.RawMessage(nil), before[index].Payload...)
			}
			cfg := acceptedResponseConfig()
			// A second interrupted dispatch verifies batch validation happens
			// before any recovery event, regardless of map iteration order.
			cfg.restoredWorldState.ActiveDispatches["a-unaccepted"] = interfaces.FactoryWorldDispatch{DispatchID: "a-unaccepted"}
			err := reconcileRestoredDispatches(cfg, ledger)
			if err == nil || !strings.Contains(err.Error(), "cannot recover accepted dispatch") ||
				!strings.Contains(err.Error(), "will not be retried") || strings.Contains(err.Error(), "PRIVATE-PAYLOAD") {
				t.Fatalf("recovery diagnostic = %v, want actionable payload-safe failure", err)
			}
			if !reflect.DeepEqual(ledger.CanonicalEvents(), before) {
				t.Fatal("reconciliation changed source events")
			}
			if calls := ledger.CallsSnapshot(); len(calls) != 0 {
				t.Fatalf("reconciliation wrote events before validation: %v", calls)
			}
		})
	}
}

type incompleteAcceptedResponseCase struct {
	name   string
	mutate func(*testing.T, []interfaces.FactoryEvent) []interfaces.FactoryEvent
}

func incompleteAcceptedResponseCases() []incompleteAcceptedResponseCase {
	return []incompleteAcceptedResponseCase{
		{"missing output", func(_ *testing.T, events []interfaces.FactoryEvent) []interfaces.FactoryEvent {
			return append(events[:2], events[3:]...)
		}},
		{"conflicting later response", func(t *testing.T, events []interfaces.FactoryEvent) []interfaces.FactoryEvent {
			return append(events, acceptedFixtureEvent(t, interfaces.FactoryEventTypeAgentRunResponse, workers.AgentRunResponseEventPayload{
				AgentRunID: "dispatch-1/agent-run/1", Outcome: "REJECTED",
			}))
		}},
		{"preview only", func(t *testing.T, events []interfaces.FactoryEvent) []interfaces.FactoryEvent {
			preview := "PRIVATE-PAYLOAD truncated"
			events[2] = acceptedFixtureEvent(t, interfaces.FactoryEventTypeModelResponse, workers.ModelResponseEventPayload{
				ModelRequestID: "model-1", Attempt: 1, Outcome: workers.InferenceOutcomeSucceeded, OutputPreview: &preview,
			})
			return events
		}},
		{"malformed response", func(_ *testing.T, events []interfaces.FactoryEvent) []interfaces.FactoryEvent {
			events[2].Payload = json.RawMessage(`{"PRIVATE-PAYLOAD":`)
			return events
		}},
		{"missing typed content", func(t *testing.T, events []interfaces.FactoryEvent) []interfaces.FactoryEvent {
			content := []work.WorkContentPart{{Type: work.WorkContentPartTypeText}}
			events[2] = acceptedFixtureEvent(t, interfaces.FactoryEventTypeModelResponse, workers.ModelResponseEventPayload{
				ModelRequestID: "model-1", Attempt: 1, Outcome: workers.InferenceOutcomeSucceeded, OutputContent: &content,
			})
			return events
		}},
		{"truncated inference JSON", func(t *testing.T, events []interfaces.FactoryEvent) []interfaces.FactoryEvent {
			raw := `{"content":[{"type":"TEXT","text":"PRIVATE-PAYLOAD`
			events[1] = acceptedFixtureEvent(t, interfaces.FactoryEventTypeInferenceRequest, workers.InferenceRequestEventPayload{InferenceRequestID: "infer-1", Attempt: 1})
			events[2] = acceptedFixtureEvent(t, interfaces.FactoryEventTypeInferenceResponse, workers.InferenceResponseEventPayload{
				InferenceRequestID: "infer-1", Attempt: 1, Outcome: workers.InferenceOutcomeSucceeded, Response: &raw,
			})
			return events
		}},
		{"malformed acceptance", func(_ *testing.T, events []interfaces.FactoryEvent) []interfaces.FactoryEvent {
			events[3].Payload = json.RawMessage(`{"PRIVATE-PAYLOAD":`)
			return events
		}},
		{"mismatched model request", func(t *testing.T, events []interfaces.FactoryEvent) []interfaces.FactoryEvent {
			events[1] = acceptedFixtureEvent(t, interfaces.FactoryEventTypeModelRequest, workers.ModelRequestEventPayload{ModelRequestID: "other", Attempt: 1})
			return events
		}},
		{"mismatched attempt", func(t *testing.T, events []interfaces.FactoryEvent) []interfaces.FactoryEvent {
			events[1] = acceptedFixtureEvent(t, interfaces.FactoryEventTypeModelRequest, workers.ModelRequestEventPayload{ModelRequestID: "model-1", Attempt: 2})
			return events
		}},
		{"another logical session", func(_ *testing.T, events []interfaces.FactoryEvent) []interfaces.FactoryEvent {
			session := "other-session"
			events[2].Context.SessionID = &session
			return events
		}},
		{"another dispatch", func(_ *testing.T, events []interfaces.FactoryEvent) []interfaces.FactoryEvent {
			dispatch := "other-dispatch"
			events[2].Context.DispatchID = &dispatch
			return events
		}},
		{"output after acceptance", func(_ *testing.T, events []interfaces.FactoryEvent) []interfaces.FactoryEvent {
			events[2], events[3] = events[3], events[2]
			return events
		}},
		{"missing dispatch request", func(_ *testing.T, events []interfaces.FactoryEvent) []interfaces.FactoryEvent {
			return events[1:]
		}},
		{"transcript only", func(t *testing.T, events []interfaces.FactoryEvent) []interfaces.FactoryEvent {
			events[3] = acceptedFixtureEvent(t, interfaces.FactoryEventTypeAgentRunResponse, workers.AgentRunResponseEventPayload{
				AgentRunID: "dispatch-1/agent-run/1", Outcome: "ACCEPTED", Diagnostics: json.RawMessage(`{"agentRun":{"transcript":[{"summary":"PRIVATE-PAYLOAD"}]}}`),
			})
			return append(events[:2], events[3:]...)
		}},
	}
}

func TestRecordedResponseValidCompleteFactsPassValidation(t *testing.T) {
	t.Parallel()
	for _, shape := range []string{"model content", "inference response"} {
		t.Run(shape, func(t *testing.T) {
			t.Parallel()
			events := acceptedResponseFixture(t)
			if shape == "inference response" {
				raw := `{"content":[{"type":"TEXT","text":"full response"}]}`
				events[1] = acceptedFixtureEvent(t, interfaces.FactoryEventTypeInferenceRequest, workers.InferenceRequestEventPayload{InferenceRequestID: "inference-1", Attempt: 1})
				events[2] = acceptedFixtureEvent(t, interfaces.FactoryEventTypeInferenceResponse, workers.InferenceResponseEventPayload{
					InferenceRequestID: "inference-1", Attempt: 1, Outcome: workers.InferenceOutcomeSucceeded, Response: &raw,
				})
			}
			before := append([]interfaces.FactoryEvent(nil), events...)
			if err := validateRestoredAcceptedResponses(acceptedResponseConfig(), events); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(events, before) {
				t.Fatal("validation mutated the recorded facts")
			}
		})
	}
}

func TestRecordedResponseProviderSuccessWithoutAcceptanceRetainsRestartRetry(t *testing.T) {
	t.Parallel()
	events := acceptedResponseFixture(t)[:3]
	ledger := &recordingfixtures.ScriptedRuntimeLedger{Events: events}
	if err := reconcileRestoredDispatches(acceptedResponseConfig(), ledger); err != nil {
		t.Fatal(err)
	}
	interruptions := 0
	for _, event := range ledger.CanonicalEvents() {
		if event.Type != interfaces.FactoryEventTypeDispatchInterrupted {
			continue
		}
		interruptions++
		var payload interfaces.DispatchInterruptedEventPayload
		if err := event.DecodePayload(&payload); err != nil {
			t.Fatal(err)
		}
		if !payload.RetryPlanned || payload.Reason != daemonRestartDispatchInterruptionReason {
			t.Fatalf("interruption = %#v, want existing restart retry", payload)
		}
	}
	if interruptions != 1 {
		t.Fatalf("interruptions = %d, want one", interruptions)
	}
	if err := reconcileRestoredDispatches(acceptedResponseConfig(), ledger); err != nil {
		t.Fatal(err)
	}
	if len(ledger.CanonicalEvents()) != len(events)+2 {
		t.Fatal("repeat reconciliation added a second interruption")
	}
}

func TestRecordedResponseTerminalCompletionPrecedesIncompleteAcceptance(t *testing.T) {
	t.Parallel()
	events := acceptedResponseFixture(t)
	events[2].Payload = json.RawMessage(`{}`)
	events = append(events, acceptedFixtureEvent(t, interfaces.FactoryEventTypeDispatchResponse, workers.DispatchResponseEventPayload{}))
	ledger := &recordingfixtures.ScriptedRuntimeLedger{Events: events}
	if err := reconcileRestoredDispatches(acceptedResponseConfig(), ledger); err != nil {
		t.Fatal(err)
	}
	if len(ledger.CallsSnapshot()) != 0 {
		t.Fatal("terminal dispatch was reconciled again")
	}
}

func acceptedResponseConfig() *runtimeConfig {
	return &runtimeConfig{
		clock: platformclock.NewDeterministic(time.Unix(0, 0).UTC(), time.Second),
		restoredWorldState: &interfaces.FactoryWorldState{ActiveDispatches: map[string]interfaces.FactoryWorldDispatch{
			"dispatch-1": {DispatchID: "dispatch-1", TransitionID: "process", WorkItemIDs: []string{"work-1"}},
		}},
	}
}

func acceptedResponseFixture(t *testing.T) []interfaces.FactoryEvent {
	t.Helper()
	content := []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "PRIVATE-PAYLOAD complete"}}
	return []interfaces.FactoryEvent{
		acceptedFixtureEvent(t, interfaces.FactoryEventTypeDispatchRequest, interfaces.DispatchRequestEventPayload{TransitionID: "process"}),
		acceptedFixtureEvent(t, interfaces.FactoryEventTypeModelRequest, workers.ModelRequestEventPayload{ModelRequestID: "model-1", Attempt: 1}),
		acceptedFixtureEvent(t, interfaces.FactoryEventTypeModelResponse, workers.ModelResponseEventPayload{
			ModelRequestID: "model-1", Attempt: 1, Outcome: workers.InferenceOutcomeSucceeded, OutputContent: &content,
		}),
		acceptedFixtureEvent(t, interfaces.FactoryEventTypeAgentRunResponse, workers.AgentRunResponseEventPayload{AgentRunID: "dispatch-1/agent-run/1", Outcome: "ACCEPTED"}),
	}
}

func acceptedFixtureEvent(t *testing.T, kind interfaces.FactoryEventType, payload any) interfaces.FactoryEvent {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	dispatch, session := "dispatch-1", "source-session"
	return interfaces.FactoryEvent{Type: kind, Payload: raw, Context: interfaces.FactoryEventContext{DispatchID: &dispatch, SessionID: &session}}
}
