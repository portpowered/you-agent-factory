package runtime

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil/recordingfixtures"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
	factorytoken "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/token"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
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

func TestRecordedResponseExtractsCompleteContent(t *testing.T) {
	t.Parallel()
	for _, shape := range []string{"model content", "inference response"} {
		t.Run(shape, func(t *testing.T) {
			t.Parallel()
			events := acceptedResponseFixture(t)
			content := []work.WorkContentPart{
				{Type: work.WorkContentPartTypeText, Text: strings.Repeat("full output ", 400)},
				{Type: work.WorkContentPartTypeJSON, JSON: json.RawMessage(`{"nested":{"result":"accepted"}}`)},
			}
			if shape == "model content" {
				preview := "short preview"
				events[2] = acceptedFixtureEvent(t, interfaces.FactoryEventTypeModelResponse, workers.ModelResponseEventPayload{
					ModelRequestID: "model-1", Attempt: 1, Outcome: workers.InferenceOutcomeSucceeded,
					OutputContent: &content, OutputPreview: &preview,
				})
			} else {
				raw, err := json.Marshal(map[string]any{"content": content})
				if err != nil {
					t.Fatal(err)
				}
				response := string(raw)
				events[1] = acceptedFixtureEvent(t, interfaces.FactoryEventTypeInferenceRequest, workers.InferenceRequestEventPayload{InferenceRequestID: "inference-1", Attempt: 1})
				events[2] = acceptedFixtureEvent(t, interfaces.FactoryEventTypeInferenceResponse, workers.InferenceResponseEventPayload{
					InferenceRequestID: "inference-1", Attempt: 1, Outcome: workers.InferenceOutcomeSucceeded, Response: &response,
				})
			}
			dispatch := acceptedResponseConfig().restoredWorldState.ActiveDispatches["dispatch-1"]
			got, ok := restoredAcceptedOutput(events[:3], events[3].Context, dispatch)
			if !ok || !reflect.DeepEqual(got, content) {
				t.Fatalf("extracted content differs from the complete recorded output: available=%v", ok)
			}
			got[0].Text = "changed"
			got[1].JSON[0] = '['
			again, ok := restoredAcceptedOutput(events[:3], events[3].Context, dispatch)
			if !ok || !reflect.DeepEqual(again, content) {
				t.Fatal("mutating extracted output changed the source recording")
			}
		})
	}
}

func TestRecordedResponseDoesNotRecoverEarlierInferenceOverIncompleteModel(t *testing.T) {
	t.Parallel()
	for _, boundary := range []string{"preview", "failed", "new request"} {
		t.Run(boundary, func(t *testing.T) {
			t.Parallel()
			events := acceptedResponseFixture(t)
			raw := "earlier successful inference"
			inference := []interfaces.FactoryEvent{
				acceptedFixtureEvent(t, interfaces.FactoryEventTypeInferenceRequest, workers.InferenceRequestEventPayload{InferenceRequestID: "infer-1", Attempt: 1}),
				acceptedFixtureEvent(t, interfaces.FactoryEventTypeInferenceResponse, workers.InferenceResponseEventPayload{
					InferenceRequestID: "infer-1", Attempt: 1, Outcome: workers.InferenceOutcomeSucceeded, Response: &raw,
				}),
			}
			if boundary == "new request" {
				events = append(events[:3], events[1], events[3])
			} else {
				response := workers.ModelResponseEventPayload{ModelRequestID: "model-1", Attempt: 1, Outcome: workers.InferenceOutcomeSucceeded, OutputPreview: &raw}
				if boundary == "failed" {
					response.Outcome = workers.InferenceOutcomeFailed
				}
				events[2] = acceptedFixtureEvent(t, interfaces.FactoryEventTypeModelResponse, response)
			}
			events = append(append(append([]interfaces.FactoryEvent(nil), events[:1]...), inference...), events[1:]...)
			ledger := &recordingfixtures.ScriptedRuntimeLedger{Events: events}
			if err := reconcileRestoredDispatches(acceptedResponseConfig(), ledger); err == nil {
				t.Fatal("earlier inference authorized an incomplete later model output")
			}
			if len(ledger.CallsSnapshot()) != 0 {
				t.Fatal("incomplete accepted output was retried or reconciled")
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

func TestRecordedResponseAcceptedDispatchIsNotInterrupted(t *testing.T) {
	t.Parallel()
	ledger := &recordingfixtures.ScriptedRuntimeLedger{Events: acceptedResponseFixture(t)}
	before := ledger.CanonicalEvents()
	if err := reconcileRestoredDispatches(acceptedResponseConfig(), ledger); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(ledger.CanonicalEvents(), before) || len(ledger.CallsSnapshot()) != 0 {
		t.Fatal("valid accepted dispatch was reconciled as an interruption")
	}
}

func TestRecordedResponseRestoresOriginalClaimsAndLineage(t *testing.T) {
	t.Parallel()
	cfg := acceptedResponseConfig()
	cfg.restoredEventPrefix = acceptedResponseFixture(t)
	cfg.net = &state.Net{Transitions: map[string]*petri.Transition{
		"process": {ID: "process", Name: "process", WorkerType: "agent"},
	}}
	dispatch := cfg.restoredWorldState.ActiveDispatches["dispatch-1"]
	dispatch.Resources = []interfaces.FactoryResourceUnit{{ResourceID: "agent-slot", TokenID: "old-process-resource"}}
	dispatch.CurrentChainingTraceID = "original-trace"
	dispatch.PreviousChainingTraceIDs = []string{"previous-trace"}
	cfg.restoredWorldState.ActiveDispatches["dispatch-1"] = dispatch
	marking := petri.NewMarking("restored")
	marking.AddToken(&factorytoken.Token{ID: "restored-input", PlaceID: "task:init", Color: factorytoken.Color{
		WorkID: "work-1", WorkTypeID: "task", DataType: factorytoken.DataTypeWork,
	}})
	marking.AddToken(&factorytoken.Token{ID: "fresh-resource", PlaceID: "agent-slot:available", Color: factorytoken.Color{
		WorkTypeID: "agent-slot", DataType: factorytoken.DataTypeResource,
	}})
	before, _ := json.Marshal(cfg.restoredEventPrefix)
	if err := prepareRestoredAcceptedDispatches(cfg, marking); err != nil {
		t.Fatal(err)
	}
	restored := cfg.restoredAcceptedDispatches["dispatch-1"]
	if len(marking.Tokens) != 0 || len(restored.entry.ConsumedTokens) != 2 {
		t.Fatal("accepted input and resource claims remain available for scheduling")
	}
	if restored.dispatch.CurrentChainingTraceID != "original-trace" || restored.dispatch.PreviousChainingTraceIDs[0] != "previous-trace" ||
		restored.dispatch.Execution.ReplayKey != "process/original-trace/work-1" || restored.dispatch.Execution.WorkIDs[0] != "work-1" ||
		restored.result.Output != "PRIVATE-PAYLOAD complete" {
		t.Fatal("recorded result or original Work/trace lineage was lost")
	}
	after, _ := json.Marshal(cfg.restoredEventPrefix)
	if string(before) != string(after) {
		t.Fatal("preparation changed source facts")
	}
}

func TestRecordedAcceptanceIsNotRecoveredDuringExplicitReplay(t *testing.T) {
	t.Parallel()
	cfg := acceptedResponseConfig()
	cfg.skipRestoredDispatchReconciliation = true
	cfg.restoredEventPrefix = acceptedResponseFixture(t)
	marking := petri.NewMarking("replay")
	marking.AddToken(&factorytoken.Token{ID: "recorded-input", PlaceID: "task:init", Color: factorytoken.Color{WorkID: "work-1"}})
	if err := prepareRestoredAcceptedDispatches(cfg, marking); err != nil {
		t.Fatal(err)
	}
	if len(cfg.restoredAcceptedDispatches) != 0 || len(marking.Tokens) != 1 {
		t.Fatal("explicit replay performed live restart completion or consumed recorded input")
	}
}

func TestRecordedResponseRejectsTruncatedTypedText(t *testing.T) {
	t.Parallel()
	events := acceptedResponseFixture(t)
	content := []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: `{"content":"PRIVATE-PAYLOAD`}}
	events[2] = acceptedFixtureEvent(t, interfaces.FactoryEventTypeModelResponse, workers.ModelResponseEventPayload{
		ModelRequestID: "model-1", Attempt: 1, Outcome: workers.InferenceOutcomeSucceeded, OutputContent: &content,
	})
	ledger := &recordingfixtures.ScriptedRuntimeLedger{Events: events}
	if err := reconcileRestoredDispatches(acceptedResponseConfig(), ledger); err == nil || strings.Contains(err.Error(), "PRIVATE-PAYLOAD") {
		t.Fatalf("truncated typed text diagnostic = %v", err)
	}
	if len(ledger.CallsSnapshot()) != 0 {
		t.Fatal("truncated typed text was interrupted or applied")
	}
}

func TestRecordedResponsePreservesStructuredNullAndRequiresCompleteEnvelope(t *testing.T) {
	t.Parallel()
	cfg := acceptedResponseConfig()
	station := &interfaces.FactoryWorkstationConfig{OutputSchema: `{"type":"null"}`}
	cfg.runtimeConfig = runtimeProjectionConfig{Workstations: map[string]*interfaces.FactoryWorkstationConfig{"process": station}}
	dispatch := work.WorkDispatch{DispatchID: "dispatch-1", TransitionID: "process", WorkstationName: "process"}
	result, err := restoredAcceptedWorkResult(cfg, dispatch, interfaces.FactoryEventContext{}, []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "null"}})
	if err != nil || result.StructuredResult != nil || !result.StructuredResultPresent {
		t.Fatalf("structured null recovery = %#v, %v", result, err)
	}
	station.OutputSchema = ""
	station.OutcomeFormat = interfaces.WorkstationOutcomeFormatDecisionEnvelope
	cfg.restoredEventPrefix = acceptedResponseFixture(t)
	_, err = restoredAcceptedWorkResult(cfg, dispatch, cfg.restoredEventPrefix[3].Context,
		[]work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "PRIVATE-PAYLOAD complete"}})
	if err == nil || strings.Contains(err.Error(), "PRIVATE-PAYLOAD") {
		t.Fatalf("missing full decision envelope diagnostic = %v", err)
	}
}

func TestRecordedResponseFullInferenceMatchesAcceptedSessionAndAttempt(t *testing.T) {
	t.Parallel()
	for _, shape := range []string{"complete", "wrong session", "wrong attempt", "truncated"} {
		t.Run(shape, func(t *testing.T) {
			t.Parallel()
			events := acceptedResponseFixture(t)
			raw := `{"decision":"ACCEPTED","output":"full output","feedback":"preserved"}`
			request := acceptedFixtureEvent(t, interfaces.FactoryEventTypeInferenceRequest, workers.InferenceRequestEventPayload{InferenceRequestID: "raw-1", Attempt: 1})
			response := acceptedFixtureEvent(t, interfaces.FactoryEventTypeInferenceResponse, workers.InferenceResponseEventPayload{InferenceRequestID: "raw-1", Attempt: 1, Outcome: workers.InferenceOutcomeSucceeded, Response: &raw})
			switch shape {
			case "wrong session":
				other := "other-session"
				response.Context.SessionID = &other
			case "wrong attempt":
				response = acceptedFixtureEvent(t, interfaces.FactoryEventTypeInferenceResponse, workers.InferenceResponseEventPayload{InferenceRequestID: "raw-1", Attempt: 2, Outcome: workers.InferenceOutcomeSucceeded, Response: &raw})
			case "truncated":
				raw = raw[:len(raw)-1]
				response = acceptedFixtureEvent(t, interfaces.FactoryEventTypeInferenceResponse, workers.InferenceResponseEventPayload{InferenceRequestID: "raw-1", Attempt: 1, Outcome: workers.InferenceOutcomeSucceeded, Response: &raw})
			}
			events = append(append(events[:3:3], request, response), events[3])
			got, ok := restoredAcceptedInferenceResponse(events, events[5].Context)
			if ok != (shape == "complete") || (ok && got != raw) {
				t.Fatalf("full inference recovery available=%v, want %v", ok, shape == "complete")
			}
		})
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

func TestRecordedWorkerSessionObservation_ProjectsRestartInterruptionAsTerminalFailure(t *testing.T) {
	base := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	workID := "work-restarted"
	dispatchID := "dispatch-restarted"
	reason := "daemon restart interrupted process-bound attempt"
	events := []interfaces.FactoryEvent{
		{
			Context: interfaces.FactoryEventContext{
				Tick: 1, Sequence: 1, EventTime: base,
				DispatchID: stringPointerForRecordedTest(dispatchID),
				WorkIDs:    stringSliceForRecordedTest([]string{workID}),
			},
			Type:    interfaces.FactoryEventTypeDispatchRequest,
			Payload: mustMarshalRecordedTest(t, interfaces.DispatchRequestEventPayload{}),
		},
		{
			Context: interfaces.FactoryEventContext{
				Tick: 1, Sequence: 2, EventTime: base.Add(time.Second),
				DispatchID: stringPointerForRecordedTest(dispatchID),
			},
			Type: interfaces.FactoryEventTypeDispatchWorkerSessionAssoc,
			Payload: mustMarshalRecordedTest(t, interfaces.DispatchWorkerSessionAssociationEventPayload{
				WorkerSessionID: "worker-restarted",
			}),
		},
		{
			Context: interfaces.FactoryEventContext{
				Tick: 2, Sequence: 3, EventTime: base.Add(2 * time.Second),
				DispatchID: stringPointerForRecordedTest(dispatchID),
				WorkIDs:    stringSliceForRecordedTest([]string{workID}),
			},
			Type: interfaces.FactoryEventTypeDispatchInterrupted,
			Payload: mustMarshalRecordedTest(t, interfaces.DispatchInterruptedEventPayload{
				InterruptedAt: base.Add(2 * time.Second), ObservedStatus: interfaces.FactoryDispatchStatusRunning,
				Reason: reason, RetryPlanned: true,
			}),
		},
	}
	service := newRecordedWorkerSessionObservation(
		nil,
		&recordingfixtures.ScriptedRuntimeLedger{Events: events},
		func(_ []interfaces.FactoryEvent, _ int) (interfaces.FactoryWorldState, error) {
			return interfaces.FactoryWorldState{
				WorkItemsByID: map[string]work.FactoryWorkItem{workID: {ID: workID}},
				ActiveDispatches: map[string]interfaces.FactoryWorldDispatch{dispatchID: {
					DispatchID: dispatchID, StartedAt: base, WorkItemIDs: []string{workID},
				}},
			}, nil
		},
		platformclock.Real{},
		nil,
	)
	prepareScopedTestFacts(service)

	result, err := service.ListObservations(context.Background(), workersessions.ListObservationsRequest{WorkID: workID})
	if err != nil || len(result.Observations) != 1 {
		t.Fatalf("ListObservations() = %#v, %v; want one interrupted observation", result, err)
	}
	observation := result.Observations[0]
	if observation.State != workersessions.StateFailed {
		t.Fatalf("interrupted observation state = %q, want FAILED", observation.State)
	}
	if observation.Failure == nil || observation.Failure.Kind != workersessions.FailureCauseProcessGone || observation.Failure.Detail != reason {
		t.Fatalf("interrupted observation failure = %#v, want process-gone restart reason", observation.Failure)
	}
	if observation.EndedAt == nil || !observation.EndedAt.Equal(base.Add(2*time.Second)) {
		t.Fatalf("interrupted observation endedAt = %#v, want interruption time", observation.EndedAt)
	}
	if err := observation.Validate(); err != nil {
		t.Fatalf("interrupted observation validation = %v", err)
	}
}

func TestNew_WithRestoredWorldStateSeedsEachOccupiedWorkExactlyOnce(t *testing.T) {
	base := time.Date(2026, time.April, 10, 12, 0, 0, 0, time.UTC)
	net := buildSimpleNet()
	net.WorkTypes["task"].States = []state.StateDefinition{
		{Value: "init", Category: state.StateCategoryInitial},
		{Value: "processing", Category: state.StateCategoryProcessing},
		{Value: "done", Category: state.StateCategoryTerminal},
		{Value: "failed", Category: state.StateCategoryFailed},
	}
	net.Places = make(map[string]*petri.Place)
	for _, place := range net.WorkTypes["task"].GeneratePlaces() {
		net.Places[place.ID] = place
	}
	restored := &interfaces.FactoryWorldState{
		WorkItemsByID: map[string]work.FactoryWorkItem{
			"work-init": {
				ID: "work-init", WorkTypeID: "task", State: "init",
				DisplayName: "Initialize", TraceID: "trace-batch",
			},
			"work-processing": {
				ID: "work-processing", WorkTypeID: "task", State: "processing",
				DisplayName: "Process", TraceID: "trace-batch", ParentID: "work-init",
			},
			"work-done": {
				ID: "work-done", WorkTypeID: "task", State: "done",
				DisplayName: "Done", TraceID: "trace-batch",
			},
		},
		ActiveWorkItemsByID: map[string]work.FactoryWorkItem{
			"work-init":       {ID: "work-init", WorkTypeID: "task", State: "init"},
			"work-processing": {ID: "work-processing", WorkTypeID: "task", State: "processing"},
		},
		TerminalWorkByID: map[string]interfaces.FactoryTerminalWork{
			"work-done": {WorkItem: work.FactoryWorkItem{ID: "work-done", WorkTypeID: "task", State: "done"}, Status: "TERMINAL"},
		},
		WorkRequestsByID: map[string]interfaces.WorkRequestPayload{
			"request-batch": {RequestID: "request-batch", WorkItems: []work.FactoryWorkItem{
				{ID: "work-init"}, {ID: "work-processing"}, {ID: "work-done"},
			}},
		},
		PlaceOccupancyByID: map[string]interfaces.FactoryPlaceOccupancy{
			"task:init": {PlaceID: "task:init", WorkItemIDs: []string{"work-init"}},
			"task:done": {PlaceID: "task:done", WorkItemIDs: []string{"work-done"}},
		},
		ActiveDispatches: map[string]interfaces.FactoryWorldDispatch{
			"dispatch-processing": {
				DispatchID:  "dispatch-processing",
				WorkItemIDs: []string{"work-processing"},
				Inputs: []interfaces.WorkstationInput{{
					TokenID: "work-processing", PlaceID: "task:processing",
					WorkItem: &work.FactoryWorkItem{ID: "work-processing"},
				}},
			},
		},
	}

	f, err := newTestFactory(
		withNet(net),
		withClock(platformclock.NewDeterministic(base, time.Second)),
		withRestoredWorldState(restored),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	snapshot, err := f.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	wantByPlace := map[string]string{
		"task:init":       "work-init",
		"task:processing": "work-processing",
		"task:done":       "work-done",
	}
	seen := make(map[string]int, len(wantByPlace))
	for placeID, workID := range wantByPlace {
		tokenIDs := snapshot.Marking.PlaceTokens[placeID]
		if len(tokenIDs) != 1 {
			t.Fatalf("restored token IDs at %q = %#v, want one token", placeID, tokenIDs)
		}
		token := snapshot.Marking.Tokens[tokenIDs[0]]
		if token == nil || token.Color.WorkID != workID {
			t.Fatalf("restored token at %q = %#v, want Work %q", placeID, token, workID)
		}
		seen[token.Color.WorkID]++
	}
	if len(seen) != len(wantByPlace) {
		t.Fatalf("restored Work identities = %#v, want one identity per occupied place", seen)
	}
	for workID, count := range seen {
		if count != 1 {
			t.Fatalf("restored Work %q appears %d times, want once", workID, count)
		}
	}
}

type restoredRequestLedger struct {
	*recordingfixtures.ScriptedRuntimeLedger
}

func (ledger *restoredRequestLedger) RecordWorkRequest(
	tick int,
	record work.WorkRequestRecord,
	eventTime time.Time,
) {
	ledger.ScriptedRuntimeLedger.RecordWorkRequest(tick, record, eventTime)
	workIDs := make([]string, 0, len(record.WorkItems))
	works := make([]work.WorkRequestEventWork, 0, len(record.WorkItems))
	for _, item := range record.WorkItems {
		workIDs = append(workIDs, item.ID)
		works = append(works, restoredRequestEventWork(record.RequestID, item))
	}
	payload, err := json.Marshal(work.WorkRequestEventPayload{
		ParentLineage: append([]string(nil), record.ParentLineage...),
		Source:        record.Source,
		Type:          record.Type,
		Works:         works,
	})
	if err != nil {
		panic(err)
	}
	requestID := record.RequestID
	source := record.Source
	ledger.AppendRecordedEvent(interfaces.FactoryEvent{
		Context: interfaces.FactoryEventContext{
			EventTime: eventTime,
			RequestID: &requestID,
			Source:    &source,
			Tick:      tick,
			WorkIDs:   stringSlicePointer(workIDs),
		},
		Id:            "factory-event/work-request/test/" + requestID,
		Payload:       payload,
		SchemaVersion: interfaces.FactoryEventSchemaVersionV1,
		Type:          interfaces.FactoryEventTypeWorkRequest,
	})
}

func restoredRequestEventWork(requestID string, item work.FactoryWorkItem) work.WorkRequestEventWork {
	name := item.DisplayName
	if name == "" {
		name = item.ID
	}
	var state *work.WorkEventState
	if item.State != "" {
		state = &work.WorkEventState{Name: item.State}
	}
	return work.WorkRequestEventWork{
		Name:                     name,
		WorkID:                   item.ID,
		RequestID:                requestID,
		WorkTypeID:               item.WorkTypeID,
		State:                    state,
		ChainingTraceDepth:       item.ChainingTraceDepth,
		CurrentChainingTraceID:   item.CurrentChainingTraceID,
		PreviousChainingTraceIDs: append([]string(nil), item.PreviousChainingTraceIDs...),
		TraceID:                  item.TraceID,
		Content:                  append([]work.WorkContentPart(nil), item.Content...),
		StructuredResult:         item.StructuredResult,
		StructuredResultPresent:  item.StructuredResultPresent,
		Tags:                     item.Tags,
	}
}

func TestRestoreCarriesWorkRequestsBeforeDispatchRecoveryFacts(t *testing.T) {
	ledger := &restoredRequestLedger{ScriptedRuntimeLedger: &recordingfixtures.ScriptedRuntimeLedger{}}
	item := work.FactoryWorkItem{
		ID: "work-restored", WorkTypeID: "task", State: "init", DisplayName: "Restored",
		TraceID: "trace-restored", Content: []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "durable content"}},
	}
	restored := &interfaces.FactoryWorldState{
		Tick:                12,
		WorkItemsByID:       map[string]work.FactoryWorkItem{item.ID: item},
		ActiveWorkItemsByID: map[string]work.FactoryWorkItem{item.ID: item},
		WorkRequestsByID: map[string]interfaces.WorkRequestPayload{
			"request-restored": {RequestID: "request-restored", Type: work.WorkRequestTypeFactoryRequestBatch, TraceID: item.TraceID, WorkItems: []work.FactoryWorkItem{item}},
		},
		ActiveDispatches: map[string]interfaces.FactoryWorldDispatch{
			"dispatch-restored": {
				DispatchID: "dispatch-restored", TransitionID: "t-process", StartedTick: 11,
				WorkItemIDs: []string{item.ID}, Inputs: []interfaces.WorkstationInput{{TokenID: item.ID, PlaceID: "task:init", WorkItem: &item}},
			},
		},
	}
	cfg := &runtimeConfig{clock: testRuntimeClock{}, restoredWorldState: restored}
	recordRestoredWorkRequests(cfg, ledger)
	if err := reconcileRestoredDispatches(cfg, ledger); err != nil {
		t.Fatalf("reconcileRestoredDispatches: %v", err)
	}

	events := ledger.CanonicalEvents()
	assertRestoredRecoveryEvents(t, events, item)
	if err := reconcileRestoredDispatches(cfg, ledger); err != nil {
		t.Fatalf("repeat reconcileRestoredDispatches: %v", err)
	}
	if got := len(ledger.CanonicalEvents()); got != len(events) {
		t.Fatalf("recovery events after repeat = %d, want %d", got, len(events))
	}
}

func assertRestoredRecoveryEvents(t *testing.T, events []interfaces.FactoryEvent, item work.FactoryWorkItem) {
	t.Helper()
	if len(events) != 4 {
		t.Fatalf("recovery events = %d, want Work request plus three dispatch facts", len(events))
	}
	wantTypes := []interfaces.FactoryEventType{
		interfaces.FactoryEventTypeWorkRequest,
		interfaces.FactoryEventTypeDispatchRequest,
		interfaces.FactoryEventTypeDispatchWorkerSessionAssoc,
		interfaces.FactoryEventTypeDispatchInterrupted,
	}
	for index, wantType := range wantTypes {
		if events[index].Type != wantType {
			t.Fatalf("recovery event %d type = %s, want %s", index, events[index].Type, wantType)
		}
	}
	assertRestoredWorkRequestPayload(t, events[0], item)
	assertRestoredDispatchRequestPayload(t, events[1], item)
}

func assertRestoredWorkRequestPayload(t *testing.T, event interfaces.FactoryEvent, item work.FactoryWorkItem) {
	t.Helper()
	var payload work.WorkRequestEventPayload
	if err := event.DecodePayload(&payload); err != nil {
		t.Fatalf("decode restored Work request: %v", err)
	}
	if len(payload.Works) != 1 {
		t.Fatalf("restored Work request payload Works = %#v, want one item", payload.Works)
	}
	restored := payload.Works[0]
	if restored.WorkID != item.ID || restored.State == nil || restored.State.Name != item.State || len(restored.Content) != 1 || restored.Content[0].Text != "durable content" {
		t.Fatalf("restored Work request payload = %#v, want durable identity/state/content", payload)
	}
}

func assertRestoredDispatchRequestPayload(t *testing.T, event interfaces.FactoryEvent, item work.FactoryWorkItem) {
	t.Helper()
	var payload interfaces.DispatchRequestEventPayload
	if err := event.DecodePayload(&payload); err != nil {
		t.Fatalf("decode restored dispatch request: %v", err)
	}
	if payload.TransitionID != "t-process" || len(payload.Inputs) != 1 || payload.Inputs[0].WorkID != item.ID {
		t.Fatalf("restored dispatch request payload = %#v, want transition and Work input", payload)
	}
}

func testRestoredNoAttempt(t *testing.T, base time.Time) {
	t.Helper()
	ledger := &recordingfixtures.ScriptedRuntimeLedger{}
	item := work.FactoryWorkItem{ID: "work-no-attempt", WorkTypeID: "task", State: "init"}
	f, err := newTestFactory(
		withNet(buildSimpleNet()),
		withClock(platformclock.NewDeterministic(base, time.Second)),
		withFactoryEventHistory(ledger),
		withRestoredWorldState(&interfaces.FactoryWorldState{
			WorkItemsByID:       map[string]work.FactoryWorkItem{item.ID: item},
			ActiveWorkItemsByID: map[string]work.FactoryWorkItem{item.ID: item},
			PlaceOccupancyByID:  map[string]interfaces.FactoryPlaceOccupancy{"task:init": {PlaceID: "task:init", WorkItemIDs: []string{item.ID}}},
		}),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	snapshot, err := f.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	if !markingContainsWorkAtPlace(&snapshot.Marking, item.ID, "task:init") {
		t.Fatalf("restored no-attempt Work marking = %#v, want task:init", snapshot.Marking.PlaceTokens)
	}
	if got := countRestoredInterruptions(ledger); got != 0 {
		t.Fatalf("no-attempt interruption count = %d, want zero", got)
	}
}

func testRestoredTerminalWork(t *testing.T, base time.Time) {
	t.Helper()
	ledger := &recordingfixtures.ScriptedRuntimeLedger{}
	item := work.FactoryWorkItem{ID: "work-terminal", WorkTypeID: "task", State: "done"}
	f, err := newTestFactory(
		withNet(buildSimpleNet()),
		withClock(platformclock.NewDeterministic(base, time.Second)),
		withFactoryEventHistory(ledger),
		withRestoredWorldState(&interfaces.FactoryWorldState{
			WorkItemsByID: map[string]work.FactoryWorkItem{item.ID: item},
			TerminalWorkByID: map[string]interfaces.FactoryTerminalWork{
				item.ID: {WorkItem: item, Status: "TERMINAL"},
			},
			PlaceOccupancyByID: map[string]interfaces.FactoryPlaceOccupancy{"task:done": {PlaceID: "task:done", WorkItemIDs: []string{item.ID}}},
		}),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	snapshot, err := f.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	if !markingContainsWorkAtPlace(&snapshot.Marking, item.ID, "task:done") {
		t.Fatalf("restored terminal Work marking = %#v, want task:done", snapshot.Marking.PlaceTokens)
	}
	if got := countRestoredInterruptions(ledger); got != 0 {
		t.Fatalf("terminal interruption count = %d, want zero", got)
	}
}

func testRestoredDependencyBlocked(t *testing.T, base time.Time) {
	t.Helper()
	ledger := &recordingfixtures.ScriptedRuntimeLedger{}
	net := buildSimpleNet()
	net.WorkTypes["task"].States = []state.StateDefinition{
		{Value: "init", Category: state.StateCategoryInitial},
		{Value: "processing", Category: state.StateCategoryProcessing},
		{Value: "done", Category: state.StateCategoryTerminal},
		{Value: "failed", Category: state.StateCategoryFailed},
	}
	net.Places = make(map[string]*petri.Place)
	for _, place := range net.WorkTypes["task"].GeneratePlaces() {
		net.Places[place.ID] = place
	}
	net.Transitions["t-process"].InputArcs[0].Guard = &petri.DependencyGuard{}
	dependent := work.FactoryWorkItem{ID: "work-dependent", WorkTypeID: "task", State: "init"}
	dependency := work.FactoryWorkItem{ID: "work-dependency", WorkTypeID: "task", State: "processing"}
	f, err := newTestFactory(
		withNet(net),
		withClock(platformclock.NewDeterministic(base, time.Second)),
		withFactoryEventHistory(ledger),
		withRestoredWorldState(&interfaces.FactoryWorldState{
			WorkItemsByID: map[string]work.FactoryWorkItem{
				dependent.ID: dependent, dependency.ID: dependency,
			},
			ActiveWorkItemsByID: map[string]work.FactoryWorkItem{
				dependent.ID: dependent, dependency.ID: dependency,
			},
			RelationsByWorkID: map[string][]work.FactoryRelation{
				dependent.ID: {{Type: string(work.WorkRelationDependsOn), TargetWorkID: dependency.ID, RequiredState: "done"}},
			},
			PlaceOccupancyByID: map[string]interfaces.FactoryPlaceOccupancy{
				"task:processing": {PlaceID: "task:processing", WorkItemIDs: []string{dependency.ID}},
			},
			ActiveDispatches: map[string]interfaces.FactoryWorldDispatch{
				"dispatch-dependent": {
					DispatchID:  "dispatch-dependent",
					StartedTick: 1,
					WorkItemIDs: []string{dependent.ID},
					Inputs: []interfaces.WorkstationInput{{
						TokenID: dependent.ID, PlaceID: "task:init",
						WorkItem: &dependent,
					}},
				},
			},
		}),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	snapshot, err := f.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	if !markingContainsWorkAtPlace(&snapshot.Marking, dependent.ID, "task:init") {
		t.Fatalf("re-armed dependency-blocked Work marking = %#v, want task:init", snapshot.Marking.PlaceTokens)
	}
	if len(snapshot.EnabledTransitions) != 0 {
		t.Fatalf("dependency-blocked enabled transitions = %#v, want none", snapshot.EnabledTransitions)
	}
	if got := countRestoredInterruptions(ledger); got != 1 {
		t.Fatalf("dependency-blocked interruption count = %d, want one", got)
	}
}

func countRestoredInterruptions(ledger *recordingfixtures.ScriptedRuntimeLedger) int {
	count := 0
	for _, event := range ledger.CanonicalEvents() {
		if event.Type == interfaces.FactoryEventTypeDispatchInterrupted {
			count++
		}
	}
	return count
}
