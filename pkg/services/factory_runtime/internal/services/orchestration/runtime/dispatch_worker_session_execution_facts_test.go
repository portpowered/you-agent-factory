package runtime

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil/recordingfixtures"
	"github.com/portpowered/infinite-you/internal/testutil/runtimefixtures"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	eventswire "github.com/portpowered/infinite-you/pkg/services/events/wire"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	workersessionswire "github.com/portpowered/infinite-you/pkg/services/worker_sessions/wire"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

type resolvedModelAssociationLedger struct {
	*recordingfixtures.ScriptedRuntimeLedger
}

func (l *resolvedModelAssociationLedger) RecordDispatchWorkerSessionAssociationWithExecution(
	tick int,
	dispatchID string,
	workerSessionID string,
	requestID string,
	facts recordings.DispatchWorkerSessionExecutionFacts,
	eventTime time.Time,
) {
	l.ScriptedRuntimeLedger.RecordDispatchWorkerSessionAssociation(tick, dispatchID, workerSessionID, requestID, eventTime)
	payload, err := json.Marshal(struct {
		WorkerSessionID string `json:"workerSessionId"`
		Model           string `json:"model,omitempty"`
		ReasoningEffort string `json:"reasoningEffort,omitempty"`
	}{
		WorkerSessionID: workerSessionID,
		Model:           facts.Model,
		ReasoningEffort: facts.ReasoningEffort,
	})
	if err != nil {
		panic(err)
	}
	l.ScriptedRuntimeLedger.AppendRecordedEvent(interfaces.FactoryEvent{
		Context: interfaces.FactoryEventContext{
			Tick:       tick,
			EventTime:  eventTime,
			DispatchID: stringPointerForRecordedTest(dispatchID),
			RequestID:  stringPointerForRecordedTest(requestID),
		},
		Id:            "factory-event/dispatch-worker-session-association/" + dispatchID,
		Payload:       payload,
		SchemaVersion: interfaces.FactoryEventSchemaVersionV1,
		Type:          interfaces.FactoryEventTypeDispatchWorkerSessionAssoc,
	})
}

func TestFactoryImpl_PlanDispatchCapturesResolvedWorkerDefinitionFactsInObservation(t *testing.T) {
	impl, execution := newResolvedWorkerDefinitionRuntime(t)
	// This focused factory has no durable Worker recording reader; leave the
	// optional health sidecar disabled so the decorator exercises its canonical
	// association projection directly.
	impl.cfg.recordingID = ""
	impl.state = interfaces.FactoryStateRunning
	plan := factory.PlanDispatchRequest{
		DispatchID: "resolved-model-dispatch", CorrelationID: "resolved-model-correlation",
		WorkIDs: []string{"resolved-model-work"}, WorkstationName: "t-process",
		WorkerType: "definition-worker", ReplayKey: "t-process/resolved-model-trace/resolved-model-work",
	}
	planned, err := impl.PlanDispatch(t.Context(), plan)
	requireNoRootErr(t, err, "PlanDispatch")
	if planned.Outcome != factory.DispatchPlanOutcomeAccepted {
		t.Fatalf("PlanDispatch outcome = %q, want ACCEPTED", planned.Outcome)
	}
	request, ok := execution.lastRequest.Load().(workers.ExecuteRequest)
	if !ok {
		t.Fatal("Workers Execute request was not recorded")
	}
	assertResolvedWorkerRequest(t, plan, request)
	observationService := impl.WorkerSessionsObservation()
	if observationService == nil {
		t.Fatal("WorkerSessionsObservation() returned nil")
	}
	observation, err := observationService.GetObservationByWorkerSessionID(
		t.Context(),
		workersessions.GetObservationByWorkerSessionIDRequest{WorkerSessionID: plan.DispatchID},
	)
	requireNoRootErr(t, err, "GetObservationByWorkerSessionID")
	assertResolvedWorkerObservation(t, plan, request, observation)
}

func newResolvedWorkerDefinitionRuntime(t *testing.T) (*factoryImpl, *recordingRootExecutionService) {
	t.Helper()
	execution := &recordingRootExecutionService{testWorkstationBoundary: &testWorkstationBoundary{}}
	events, err := eventswire.NewService(logging.NoopLogger{})
	requireNoRootErr(t, err, "New events service")
	workerSessions, err := workersessionswire.NewService(
		execution,
		events,
		logging.NoopLogger{},
		platformclock.Real{},
		unavailableProviderSessions{},
		nil,
	)
	requireNoRootErr(t, err, "New Worker Sessions service")
	ledger := &resolvedModelAssociationLedger{ScriptedRuntimeLedger: &recordingfixtures.ScriptedRuntimeLedger{}}
	runtime, err := newTestFactory(
		withNet(buildSimpleNet()),
		withInlineDispatch(),
		withWorkerService(execution),
		withWorkerSessions(workerSessions),
		withFactoryEventHistory(ledger),
		withRuntimeConfig(runtimefixtures.RuntimeDefinitionLookupFixture{
			Workstations: map[string]*interfaces.FactoryWorkstationConfig{
				"t-process": {
					Name:           "t-process",
					Type:           interfaces.WorkstationTypeModel,
					WorkerTypeName: "definition-worker",
				},
			},
			Workers: map[string]*interfaces.FactoryWorkerConfig{
				"definition-worker": {
					Name:             "definition-worker",
					Type:             interfaces.WorkerTypeModel,
					ExecutorProvider: "codex",
					ModelProvider:    "resolved-provider",
					Model:            "gpt-5.6-luna",
					ReasoningEffort:  "high",
				},
			},
		}),
		withLogger(logging.NoopLogger{}),
	)
	requireNoRootErr(t, err, "New")
	impl, ok := runtime.(*factoryImpl)
	if !ok {
		t.Fatalf("factory type = %T, want *factoryImpl", runtime)
	}
	impl.cfg.worldStateProjector = func(_ []interfaces.FactoryEvent, _ int) (interfaces.FactoryWorldState, error) {
		// The read assertion only needs the association fact; supplying a
		// projector ensures WorkerSessionsObservation exercises its recorded
		// decorator instead of falling back to the raw live service.
		return interfaces.FactoryWorldState{}, nil
	}
	return impl, execution
}

func assertResolvedWorkerRequest(t *testing.T, plan factory.PlanDispatchRequest, request workers.ExecuteRequest) {
	t.Helper()
	if request.Correlation.DispatchID != plan.DispatchID {
		t.Fatalf("executed dispatch correlation = %q, want %q", request.Correlation.DispatchID, plan.DispatchID)
	}
	if request.Target.Model.Name != "gpt-5.6-luna" {
		t.Fatalf("downstream model = %q, want resolved model", request.Target.Model.Name)
	}
	if request.Target.Model.Provider != "resolved-provider" {
		t.Fatalf("downstream provider = %q, want resolved provider", request.Target.Model.Provider)
	}
	if request.Target.Model.ReasoningEffort != "high" {
		t.Fatalf("downstream reasoning effort = %q, want high", request.Target.Model.ReasoningEffort)
	}
}

func assertResolvedWorkerObservation(
	t *testing.T,
	plan factory.PlanDispatchRequest,
	request workers.ExecuteRequest,
	observation workersessions.Observation,
) {
	t.Helper()
	if observation.WorkerSessionID != plan.DispatchID {
		t.Fatalf("observation Worker Session ID = %q, want %q", observation.WorkerSessionID, plan.DispatchID)
	}
	if observation.AttemptID == "" {
		t.Fatal("observation attempt ID is empty")
	}
	assertOptionalExecutionFact(t, "model", observation.Model, request.Target.Model.Name)
	assertOptionalExecutionFact(t, "reasoning effort", observation.ReasoningEffort, request.Target.Model.ReasoningEffort)
}

func TestRecordedObservationMergeBranches(t *testing.T) {
	liveObservation := workersessions.Observation{
		WorkerSessionID: "worker-1", ProviderSession: providers.SessionRef{Provider: providers.IDCodex, Kind: providers.SessionIDKind, ID: "live-session"}, ProviderSessionAvailable: true,
		TokenUsage: &workersessions.TokenUsage{InputTokens: intPointerForRecordedTest(7)}, Transcript: workersessions.TranscriptAvailabilityAvailable, Parse: workersessions.ParseDiagnostics{EventCount: 2},
		Failure: &workersessions.FailureCause{Kind: workersessions.FailureCauseWorkersExecutionFailure, Detail: "live failure"},
	}
	recorded := []workersessions.Observation{{WorkerSessionID: "worker-1"}}
	merged := mergeRecordedObservations(recorded, []workersessions.Observation{liveObservation})
	assertMergedLiveObservation(t, merged)
	assertRecordedObservationUnchanged(t, recorded)

	liveOnly := workersessions.Observation{WorkerSessionID: "worker-live-only", WorkIDs: []string{"work-live-only"}}
	liveOnlyResult := mergeRecordedObservations([]workersessions.Observation{{WorkerSessionID: "worker-recorded-only"}}, []workersessions.Observation{liveOnly})
	assertLiveOnlyMerge(t, liveOnlyResult, liveOnly)
	liveOnlyResult[0].WorkIDs[0] = "mutated"
	if liveOnly.WorkIDs[0] != "work-live-only" {
		t.Fatal("mergeRecordedObservations(live-only) returned a source-owned WorkIDs slice")
	}
	got := mergeRecordedObservations(nil, []workersessions.Observation{liveObservation})
	if len(got) != 1 {
		t.Fatalf("mergeRecordedObservations(empty recorded) returned %d observations, want 1", len(got))
	}
	if got[0].WorkerSessionID != liveObservation.WorkerSessionID {
		t.Fatalf("empty-recorded Worker Session ID = %q, want %q", got[0].WorkerSessionID, liveObservation.WorkerSessionID)
	}
}

func assertMergedLiveObservation(t *testing.T, merged []workersessions.Observation) {
	t.Helper()
	if len(merged) != 1 {
		t.Fatalf("mergeRecordedObservations() returned %d observations, want 1", len(merged))
	}
	observation := merged[0]
	if !observation.ProviderSessionAvailable {
		t.Fatal("merged observation did not retain provider session availability")
	}
	if observation.ProviderSession.ID != "live-session" {
		t.Fatalf("merged provider session ID = %q, want live-session", observation.ProviderSession.ID)
	}
	if observation.TokenUsage == nil {
		t.Fatal("merged observation did not retain token usage")
	}
	if observation.TokenUsage.InputTokens == nil {
		t.Fatal("merged observation did not retain input token usage")
	}
	if *observation.TokenUsage.InputTokens != 7 {
		t.Fatalf("merged input tokens = %d, want 7", *observation.TokenUsage.InputTokens)
	}
	if observation.Transcript != workersessions.TranscriptAvailabilityAvailable {
		t.Fatalf("merged transcript availability = %q, want available", observation.Transcript)
	}
	if observation.Parse.EventCount != 2 {
		t.Fatalf("merged parse event count = %d, want 2", observation.Parse.EventCount)
	}
	if observation.Failure == nil {
		t.Fatal("merged observation did not retain failure")
	}
}

func assertRecordedObservationUnchanged(t *testing.T, recorded []workersessions.Observation) {
	t.Helper()
	if recorded[0].ProviderSessionAvailable {
		t.Fatal("mergeRecordedObservations mutated recorded provider session")
	}
	if recorded[0].TokenUsage != nil {
		t.Fatal("mergeRecordedObservations mutated recorded token usage")
	}
	if recorded[0].Failure != nil {
		t.Fatal("mergeRecordedObservations mutated recorded failure")
	}
}

func assertLiveOnlyMerge(t *testing.T, merged []workersessions.Observation, live workersessions.Observation) {
	t.Helper()
	if len(merged) != 2 {
		t.Fatalf("live-only merge returned %d observations, want 2", len(merged))
	}
	if merged[0].WorkerSessionID != live.WorkerSessionID {
		t.Fatalf("live-only first Worker Session ID = %q, want %q", merged[0].WorkerSessionID, live.WorkerSessionID)
	}
	if merged[1].WorkerSessionID != "worker-recorded-only" {
		t.Fatalf("live-only second Worker Session ID = %q, want worker-recorded-only", merged[1].WorkerSessionID)
	}
}

func TestRecordedObservationMergePreservesExecutionFacts(t *testing.T) {
	recordedModel := "recorded-model"
	recordedEffort := "medium"
	liveModel := "live-model"
	liveEffort := "high"
	merged := mergeRecordedObservations(
		[]workersessions.Observation{
			{WorkerSessionID: "recorded-only", Model: &recordedModel, ReasoningEffort: &recordedEffort},
			{WorkerSessionID: "overlapping"},
		},
		[]workersessions.Observation{
			{WorkerSessionID: "live-only", Model: &liveModel, ReasoningEffort: &liveEffort},
			{WorkerSessionID: "overlapping", Model: &liveModel, ReasoningEffort: &liveEffort},
		},
	)
	if len(merged) != 3 {
		t.Fatalf("execution-fact merge returned %d observations, want 3", len(merged))
	}
	byID := make(map[string]workersessions.Observation, len(merged))
	for _, observation := range merged {
		byID[observation.WorkerSessionID] = observation
	}
	assertExecutionFacts(t, "recorded-only", byID["recorded-only"], recordedModel, recordedEffort)
	assertExecutionFacts(t, "live-only", byID["live-only"], liveModel, liveEffort)
	assertExecutionFacts(t, "overlapping", byID["overlapping"], liveModel, liveEffort)

	legacy := mergeRecordedObservations(
		[]workersessions.Observation{{WorkerSessionID: "legacy"}},
		[]workersessions.Observation{{WorkerSessionID: "legacy"}},
	)
	assertExecutionFactsAbsent(t, "legacy", legacy[0])

	emptyModel := ""
	emptyEffort := ""
	retained := mergeRecordedObservations(
		[]workersessions.Observation{{WorkerSessionID: "retained", Model: &recordedModel, ReasoningEffort: &recordedEffort}},
		[]workersessions.Observation{{WorkerSessionID: "retained", Model: &emptyModel, ReasoningEffort: &emptyEffort}},
	)
	assertExecutionFacts(t, "empty live", retained[0], recordedModel, recordedEffort)
}

func assertExecutionFacts(t *testing.T, label string, observation workersessions.Observation, model, effort string) {
	t.Helper()
	if observation.WorkerSessionID == "" {
		t.Fatalf("%s observation has no Worker Session ID", label)
	}
	assertOptionalExecutionFact(t, label+" model", observation.Model, model)
	assertOptionalExecutionFact(t, label+" reasoning effort", observation.ReasoningEffort, effort)
}

func assertExecutionFactsAbsent(t *testing.T, label string, observation workersessions.Observation) {
	t.Helper()
	if observation.Model != nil {
		t.Fatalf("%s model = %q, want absent", label, *observation.Model)
	}
	if observation.ReasoningEffort != nil {
		t.Fatalf("%s reasoning effort = %q, want absent", label, *observation.ReasoningEffort)
	}
}

func assertOptionalExecutionFact(t *testing.T, label string, value *string, want string) {
	t.Helper()
	if value == nil {
		t.Fatalf("%s is absent, want %q", label, want)
	}
	if *value != want {
		t.Fatalf("%s = %q, want %q", label, *value, want)
	}
}
