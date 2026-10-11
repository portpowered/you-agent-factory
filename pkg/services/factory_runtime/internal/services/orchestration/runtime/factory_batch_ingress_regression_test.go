package runtime

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil/recordingfixtures"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryhost "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/host"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
	factorytoken "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/token"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// TestBlockedDispatchConcurrentBatchIngressRegression_ServiceModeWorkerPool is the
// deterministic regression for issue #1352. It blocks one unrelated dispatch,
// submits a concurrent batch, and asserts observable acceptance (WORK_REQUEST plus
// marking visibility) and same-request-ID replay idempotency before the blocker
// completes. It fails on the pre-fix starvation path where submit returned while
// WORK_REQUEST and marking visibility were still missing.
func TestBlockedDispatchConcurrentBatchIngressRegression_ServiceModeWorkerPool(t *testing.T) {
	t.Helper()

	executor, history, h := startBlockedDispatchIngressHarness(t)

	const (
		requestID = "request-batch-ingress-regression"
		workID    = "work-batch-ingress-regression"
		traceID   = "trace-batch-ingress-regression"
	)

	batchRequest := ingressBatchRequest(requestID, workID, traceID)

	first, err := h.Factory.SubmitWorkRequest(context.Background(), batchRequest)
	if err != nil {
		t.Fatalf("first SubmitWorkRequest: %v", err)
	}
	if !first.Accepted {
		t.Fatalf("first submit accepted = false, want true before blocked dispatch completes")
	}
	if first.RequestID != requestID {
		t.Fatalf("first submit request_id = %q, want %q", first.RequestID, requestID)
	}
	assertIngressObservable(t, history, h.Factory, requestID, workID)

	replayed, err := h.Factory.SubmitWorkRequest(context.Background(), batchRequest)
	if err != nil {
		t.Fatalf("replay SubmitWorkRequest: %v", err)
	}
	if replayed.Accepted {
		t.Fatalf("replay accepted = true, want idempotent no-op")
	}
	if replayed.RequestID != first.RequestID || replayed.WorkID != first.WorkID {
		t.Fatalf("replay identity = %#v, want preserved %#v", replayed, first)
	}
	if countWorkRequestRecords(history, requestID) != 1 {
		t.Fatalf(
			"WORK_REQUEST count for %q = %d, want 1 before blocked dispatch completes",
			requestID,
			countWorkRequestRecords(history, requestID),
		)
	}

	select {
	case <-executor.release:
		t.Fatal("blocked dispatch completed before ingress regression assertions finished")
	default:
	}

	releaseBlockedDispatchHarness(t, executor, h)
}

func TestRestoredHistoricalWorkIDsIncludesAdmittedEmittedConsumedAndFailedWork(t *testing.T) {
	t.Parallel()

	outputWork := []work.WorkRequestEventWork{{WorkID: "work-task-80", WorkTypeID: "task", Name: "emitted"}}
	triggerWorkID := "work-task-72"
	cfg := &runtimeConfig{
		restoredWorldState: &interfaces.FactoryWorldState{
			WorkItemsByID: map[string]work.FactoryWorkItem{
				"work-task-3": {ID: "work-task-3", WorkTypeID: "task"},
			},
			FailedWorkItemsByID: map[string]work.FactoryWorkItem{
				"work-plan-41": {ID: "work-plan-41", WorkTypeID: "plan"},
			},
			TerminalWorkByID: map[string]interfaces.FactoryTerminalWork{
				"work-review-55": {WorkItem: work.FactoryWorkItem{ID: "work-review-55", WorkTypeID: "review"}},
			},
			CompletedDispatches: []interfaces.FactoryWorldDispatchCompletion{{
				ConsumedInputs:  []interfaces.WorkstationInput{{WorkItem: &work.FactoryWorkItem{ID: "work-idea-60"}}},
				OutputWorkItems: []work.FactoryWorkItem{{ID: "work-plan-61"}},
			}},
		},
		restoredEventPrefix: []interfaces.FactoryEvent{
			{
				Type:    interfaces.FactoryEventTypeWorkRequest,
				Context: interfaces.FactoryEventContext{WorkIDs: stringSliceForRecordedTest([]string{"work-idea-65"})},
				Payload: mustMarshalRecordedTest(t, work.WorkRequestEventPayload{Works: []work.WorkRequestEventWork{{
					WorkID: "work-plan-70", WorkTypeID: "plan", Name: "admitted",
				}}}),
			},
			{
				Type:    interfaces.FactoryEventTypeDispatchResponse,
				Payload: mustMarshalRecordedTest(t, workers.DispatchResponseEventPayload{OutputWork: &outputWork}),
			},
			{
				Type: interfaces.FactoryEventTypeWorkStateChange,
				Payload: mustMarshalRecordedTest(t, interfaces.WorkStateChangeEventPayload{
					WorkID: "work-task-71", TriggerWorkID: &triggerWorkID,
				}),
			},
		},
	}

	got := restoredHistoricalWorkIDs(cfg)
	for _, workID := range []string{
		"work-task-3", "work-plan-41", "work-review-55", "work-idea-60",
		"work-plan-61", "work-idea-65", "work-plan-70", "work-task-71",
		"work-task-72", "work-task-80",
	} {
		if _, ok := got[workID]; !ok {
			t.Errorf("historical Work IDs = %#v, missing %q", got, workID)
		}
	}

	transformer, _ := buildRuntimeSubsystems(
		&runtimeConfig{net: buildSimpleNet(), clock: platformclock.Real{}},
		nil,
		logging.EnsureLogger(nil),
		func() string { return "runtime-id" },
		nil,
		got,
	)
	token, err := transformer.InitialTokenFromSubmit(work.SubmitRequest{WorkTypeID: "task"}, time.Now())
	if err != nil {
		t.Fatalf("InitialTokenFromSubmit: %v", err)
	}
	if token.Color.WorkID != "work-task-81" {
		t.Fatalf("restored next Work ID = %q, want work-task-81", token.Color.WorkID)
	}
}

func TestNew_RestoredHistoricalWorkIDIsReservedFromExplicitReadmission(t *testing.T) {
	t.Parallel()

	ledger := &recordingfixtures.ScriptedRuntimeLedger{Events: []interfaces.FactoryEvent{{
		Type: interfaces.FactoryEventTypeWorkRequest,
		Payload: mustMarshalRecordedTest(t, work.WorkRequestEventPayload{Works: []work.WorkRequestEventWork{{
			WorkID: "work-task-3", WorkTypeID: "task", Name: "historical-consumed-task",
		}}}),
	}}}
	runtime, err := newTestFactory(
		withNet(buildSimpleNet()),
		withFactoryEventHistory(ledger),
		withRestoredWorldState(&interfaces.FactoryWorldState{}),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	result, err := runtime.SubmitWorkRequest(context.Background(), work.WorkRequest{
		RequestID: "request-new-task-reusing-history",
		Type:      work.WorkRequestTypeFactoryRequestBatch,
		Works: []work.Work{{
			Name: "new-task", WorkID: "work-task-3", WorkTypeID: "task",
		}},
	})
	if !errors.Is(err, work.ErrWorkRequestConflict) {
		t.Fatalf("SubmitWorkRequest error = %v, want Work Request conflict", err)
	}
	if !reflect.DeepEqual(result, work.WorkRequestSubmitResult{}) {
		t.Fatalf("SubmitWorkRequest result = %#v, want zero result", result)
	}
	snapshot, err := runtime.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	if got := len(snapshot.Marking.PlaceTokens["task:init"]); got != 0 {
		t.Fatalf("task:init token count = %d, want zero after atomic historical-ID rejection", got)
	}
}

func assertLegacyWorkInput(t *testing.T, input workers.WorkInput) {
	t.Helper()
	if input.WorkID != "work-legacy" {
		t.Fatalf("legacy input WorkID = %q, want work-legacy", input.WorkID)
	}
	if len(input.Content) != 1 {
		t.Fatalf("legacy input content = %#v, want one payload part", input.Content)
	}
	if input.Content[0].Text != "legacy payload" {
		t.Fatalf("legacy input content = %#v, want payload fallback", input.Content)
	}
	if input.AttemptFacts.AttemptNumber != 3 || input.AttemptFacts.LastFailure != "last failure" {
		t.Fatalf("legacy input attempt facts = %#v, want attempt 3 and last failure", input.AttemptFacts)
	}
	if input.Tags["source"] != "legacy" {
		t.Fatalf("legacy input tags = %#v, want source tag", input.Tags)
	}
	if input.Kind != string(workers.DataTypeWork) || input.State != "init" {
		t.Fatalf("legacy input kind/state = %q/%q, want work/init", input.Kind, input.State)
	}
	if len(input.InputNames) != 1 {
		t.Fatalf("legacy input names = %#v, want primary", input.InputNames)
	}
	if input.InputNames[0] != "primary" {
		t.Fatalf("legacy input name = %q, want primary", input.InputNames[0])
	}
}

func assertContentWorkInput(t *testing.T, input workers.WorkInput) {
	t.Helper()
	if input.WorkID != "work-content" {
		t.Fatalf("content input WorkID = %q, want work-content", input.WorkID)
	}
	if input.Name != "content display name" {
		t.Fatalf("content input name = %q, want content display name", input.Name)
	}
	if len(input.Content) != 1 {
		t.Fatalf("content input content = %#v, want one body part", input.Content)
	}
	if input.Content[0].Text != "content body" {
		t.Fatalf("content input content = %#v, want content body", input.Content)
	}
	if input.AttemptFacts.AttemptNumber != 5 || input.State != "review" {
		t.Fatalf("content input facts/state = %#v/%q, want attempt 5/review", input.AttemptFacts, input.State)
	}
	if len(input.InputNames) != 1 {
		t.Fatalf("content input names = %#v, want secondary", input.InputNames)
	}
	if input.InputNames[0] != "secondary" {
		t.Fatalf("content input name = %q, want secondary", input.InputNames[0])
	}
}

func workInputsFromDispatchFixture() work.WorkDispatch {
	invocation := &work.InvocationArguments{Arguments: map[string]work.InvocationArgument{
		"mode": {Values: []string{"fast"}},
	}}
	return work.WorkDispatch{
		TransitionID: "transition-inputs",
		InputBindings: map[string][]string{
			"primary":   {"legacy"},
			"secondary": {"content"},
		},
		InputTokens: workers.InputTokens(
			workers.Token{ID: "resource", State: "ready", Color: workers.Color{DataType: workers.DataTypeResource}},
			workers.Token{
				ID:    "legacy",
				State: "init",
				Color: workers.Color{
					DataType:            workers.DataTypeWork,
					WorkID:              "work-legacy",
					WorkTypeID:          "task",
					RequestID:           "request-legacy",
					Payload:             []byte("legacy payload"),
					Tags:                map[string]string{"source": "legacy"},
					InvocationArguments: invocation,
				},
				History: workers.History{
					TotalVisits: map[string]int{"transition-inputs": 2},
					FailureLog:  []workers.Failure{{Error: "last failure"}},
				},
			},
			workers.Token{
				ID:    "content",
				State: "review",
				Color: workers.Color{
					Name:       "content display name",
					DataType:   workers.DataTypeWork,
					WorkID:     "work-content",
					WorkTypeID: "review",
					Content: []work.WorkContentPart{{
						Type: work.WorkContentPartTypeText,
						Text: "content body",
					}},
				},
				History: workers.History{TotalVisits: map[string]int{"transition-inputs": 4}},
			},
		),
	}
}

func buildSimpleNet() *state.Net {
	wt := &state.WorkType{
		ID:   "task",
		Name: "Task",
		States: []state.StateDefinition{
			{Value: "init", Category: state.StateCategoryInitial},
			{Value: "done", Category: state.StateCategoryTerminal},
			{Value: "failed", Category: state.StateCategoryFailed},
		},
	}

	places := make(map[string]*petri.Place)
	for _, p := range wt.GeneratePlaces() {
		places[p.ID] = p
	}

	transition := &petri.Transition{
		ID:         "t-process",
		Name:       "Process",
		Type:       petri.TransitionNormal,
		WorkerType: "mock",
		InputArcs: []petri.Arc{{
			ID:          "a-in",
			Name:        "input",
			PlaceID:     "task:init",
			Direction:   petri.ArcInput,
			Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne},
		}},
		OutputArcs: []petri.Arc{{
			ID:          "a-out",
			Name:        "output",
			PlaceID:     "task:done",
			Direction:   petri.ArcOutput,
			Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne},
		}},
	}

	return &state.Net{
		ID:          "test-net",
		Places:      places,
		Transitions: map[string]*petri.Transition{"t-process": transition},
		WorkTypes:   map[string]*state.WorkType{"task": wt},
		Resources:   make(map[string]*state.ResourceDef),
	}
}

func buildSimpleNetWithFailureArc() *state.Net {
	n := buildSimpleNet()
	n.Transitions["t-process"].FailureArcs = []petri.Arc{{
		ID:        "a-failed",
		Name:      "failed",
		PlaceID:   "task:failed",
		Direction: petri.ArcOutput,
	}}
	return n
}

func newPassingInlineRuntime(t *testing.T) factoryhost.Engine {
	t.Helper()
	f, err := newTestFactory(
		withNet(buildSimpleNet()),
		withInlineDispatch(),
		withWorkerExecutor("mock", &passExecutor{}),
		withLogger(logging.NoopLogger{}),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return f
}

func newPassingInlineRuntimeWithLedger(
	t *testing.T,
) (factoryhost.Engine, *recordingfixtures.ScriptedRuntimeLedger) {
	t.Helper()
	f, ledger, err := newTestFactoryWithScriptedLedger(
		withNet(buildSimpleNet()),
		withInlineDispatch(),
		withWorkerExecutor("mock", &passExecutor{}),
		withLogger(logging.NoopLogger{}),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return f, ledger
}

func tickableFactory(t *testing.T, f factoryhost.Engine) TickableFactory {
	t.Helper()
	tickable, ok := f.(TickableFactory)
	if !ok {
		t.Fatal("factory is not tickable")
	}
	return tickable
}

func runtimeGeneratedEvents(t *testing.T, f factoryhost.Engine) []factoryapi.FactoryEvent {
	t.Helper()
	events, err := f.GetFactoryEvents(context.Background())
	if err != nil {
		t.Fatalf("GetFactoryEvents: %v", err)
	}
	mapped := make([]factoryapi.FactoryEvent, len(events))
	for index, event := range events {
		mapped[index] = runtimeGeneratedFactoryEvent(t, event)
	}
	return mapped
}

func runtimeGeneratedFactoryEvent(t testing.TB, event interfaces.FactoryEvent) factoryapi.FactoryEvent {
	t.Helper()
	var mapped factoryapi.FactoryEvent
	if err := event.Decode(&mapped); err != nil {
		t.Fatalf("map Factory event %q: %v", event.Id, err)
	}
	return mapped
}

func markingContainsWorkAtPlace(marking *petri.MarkingSnapshot, workID string, placeID string) bool {
	if marking == nil {
		return false
	}
	for _, tokenID := range marking.PlaceTokens[placeID] {
		token := marking.Tokens[tokenID]
		if token != nil && token.Color.WorkID == workID {
			return true
		}
	}
	return false
}

func safeBoundaryWorkID(dispatch work.WorkDispatch) string {
	for _, token := range workers.WorkDispatchInputTokens(dispatch) {
		if token.Color.DataType == factorytoken.DataTypeResource {
			continue
		}
		if token.Color.WorkID != "" {
			return token.Color.WorkID
		}
		if token.ID != "" {
			return token.ID
		}
	}
	for _, workID := range dispatch.Execution.WorkIDs {
		if workID != "" {
			return workID
		}
	}
	return ""
}

func safeBoundaryResult(
	dispatch work.WorkDispatch,
	workID string,
	outcome workers.WorkOutcome,
	errText string,
	providerFailure *workers.WorkFailureMetadata,
	providerSession *providers.SessionMetadata,
	retryCount string,
) workers.WorkResult {
	return workers.WorkResult{
		DispatchID:      dispatch.DispatchID,
		TransitionID:    dispatch.TransitionID,
		Outcome:         outcome,
		Output:          "safe boundary output for " + workID,
		Error:           errText,
		FailureMetadata: providerFailure,
		Continuation:    (providerSession).ContinuationRef(),
		Diagnostics: &workers.WorkDiagnostics{
			RenderedPrompt: &workers.RenderedPromptDiagnostic{
				SystemPromptHash: "system-hash-" + workID,
				UserMessageHash:  "user-hash-" + workID,
				Variables: map[string]string{
					"prompt_source":  "factory-renderer",
					"work_type_name": "task",
					"system_prompt":  "raw rendered system prompt must stay private",
					"user_message":   "raw rendered user message must stay private",
					"stdin":          "raw rendered stdin must stay private",
					"env":            "raw rendered environment must stay private",
				},
			},
			Provider: &workers.ProviderDiagnostic{
				Provider: "codex",
				Model:    "gpt-5.4",
				RequestMetadata: map[string]string{
					"prompt_source":      "provider-renderer",
					"worker_type":        "mock",
					"working_directory":  "/workspace/" + workID,
					"worktree":           "/workspace/" + workID + "/.worktree",
					"system_prompt_body": "raw prompt body must stay private",
					"stdin_payload":      "raw stdin payload must stay private",
					"env_secret":         "raw env secret must stay private",
				},
				ResponseMetadata: map[string]string{
					"provider_session_id": providerSession.ID,
					"retry_count":         retryCount,
					"system_prompt_body":  "raw response prompt body must stay private",
					"stdin_payload":       "raw response stdin payload must stay private",
					"env_secret":          "raw response env secret must stay private",
				},
			},
			Command: &workers.CommandDiagnostic{
				Command: "echo",
				Stdin:   "raw command stdin must stay private",
				Env: map[string]string{
					"AGENT_FACTORY_AUTH_TOKEN": "raw environment value must stay private",
				},
			},
			Panic: &workers.PanicDiagnostic{Stack: "panic stack should not be stored"},
		},
	}
}
