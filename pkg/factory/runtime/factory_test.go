package runtime

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	factoryboundary "github.com/portpowered/agent-factory/pkg/api"
	factoryapi "github.com/portpowered/agent-factory/pkg/api/generated"
	"github.com/portpowered/agent-factory/pkg/interfaces"

	"github.com/portpowered/agent-factory/pkg/factory"
	"github.com/portpowered/agent-factory/pkg/factory/projections"
	"github.com/portpowered/agent-factory/pkg/factory/scheduler"
	"github.com/portpowered/agent-factory/pkg/factory/state"
	"github.com/portpowered/agent-factory/pkg/internal/submission"
	"github.com/portpowered/agent-factory/pkg/logging"
	"github.com/portpowered/agent-factory/pkg/petri"
	"github.com/portpowered/agent-factory/pkg/replay"
	"github.com/portpowered/agent-factory/pkg/testutil/runtimefixtures"
	"github.com/portpowered/agent-factory/pkg/workers"
)

type passExecutor struct{}

func (e *passExecutor) Execute(_ context.Context, dispatch interfaces.WorkDispatch) (interfaces.WorkResult, error) {
	return interfaces.WorkResult{
		DispatchID:   dispatch.DispatchID,
		TransitionID: dispatch.TransitionID,
		Outcome:      interfaces.OutcomeAccepted,
		Output:       "done",
	}, nil
}

type blockingExecutor struct {
	started chan struct{}
	release chan struct{}
}

func (e *blockingExecutor) Execute(_ context.Context, dispatch interfaces.WorkDispatch) (interfaces.WorkResult, error) {
	close(e.started)
	<-e.release
	return interfaces.WorkResult{
		DispatchID:   dispatch.DispatchID,
		TransitionID: dispatch.TransitionID,
		Outcome:      interfaces.OutcomeAccepted,
		Output:       "done",
	}, nil
}

type safeDiagnosticsBoundaryExecutor struct{}

func (e *safeDiagnosticsBoundaryExecutor) Execute(_ context.Context, dispatch interfaces.WorkDispatch) (interfaces.WorkResult, error) {
	workID := safeBoundaryWorkID(dispatch)
	switch workID {
	case "work-safe-success":
		return safeBoundaryResult(dispatch, workID, interfaces.OutcomeAccepted, "", nil, &interfaces.ProviderSessionMetadata{
			Provider: "codex",
			Kind:     "response_id",
			ID:       "resp-safe-success",
		}, "1"), nil
	case "work-safe-failure":
		return safeBoundaryResult(dispatch, workID, interfaces.OutcomeFailed, "provider timed out", &interfaces.ProviderFailureMetadata{
			Family: interfaces.ProviderErrorFamilyRetryable,
			Type:   interfaces.ProviderErrorTypeTimeout,
		}, &interfaces.ProviderSessionMetadata{
			Provider: "codex",
			Kind:     "session_id",
			ID:       "sess-safe-failure",
		}, "2"), nil
	case "work-safe-windows-process-failure":
		return safeBoundaryResult(dispatch, workID, interfaces.OutcomeFailed, "provider error: internal_server_error: codex exited with code 4294967295: stderr: OpenAI Codex v0.118.0 (research preview)", &interfaces.ProviderFailureMetadata{
			Family: interfaces.ProviderErrorFamilyRetryable,
			Type:   interfaces.ProviderErrorTypeInternalServerError,
		}, &interfaces.ProviderSessionMetadata{
			Provider: "codex",
			Kind:     "session_id",
			ID:       "sess-safe-windows-4294967295",
		}, "2"), nil
	default:
		return interfaces.WorkResult{
			DispatchID:   dispatch.DispatchID,
			TransitionID: dispatch.TransitionID,
			Outcome:      interfaces.OutcomeAccepted,
			Output:       "done",
		}, nil
	}
}

type fixedCompletionDeliveryPlanner struct {
	tick          int
	plannedResult interfaces.WorkResult
}

func (p fixedCompletionDeliveryPlanner) DeliveryTickForDispatch(interfaces.WorkDispatch) (int, bool, error) {
	return p.tick, true, nil
}

func (p fixedCompletionDeliveryPlanner) PlannedResultForDispatch(dispatch interfaces.WorkDispatch) (interfaces.WorkResult, bool, error) {
	if p.plannedResult.DispatchID == "" && p.plannedResult.TransitionID == "" && p.plannedResult.Output == "" && p.plannedResult.Outcome == "" {
		return interfaces.WorkResult{}, false, nil
	}
	result := p.plannedResult
	result.DispatchID = dispatch.DispatchID
	result.TransitionID = dispatch.TransitionID
	return result, true, nil
}

func submitWorkRequests(ctx context.Context, f factory.Factory, reqs []interfaces.SubmitRequest) (interfaces.WorkRequestSubmitResult, error) {
	return f.SubmitWorkRequest(ctx, submission.WorkRequestFromSubmitRequests(reqs))
}

type runtimeProjectionConfig = runtimefixtures.RuntimeDefinitionLookupFixture
type runtimeSchedulerConfig = *runtimefixtures.RuntimeDefinitionLookupFixture

type runtimeAwareScheduler struct {
	configured interfaces.RuntimeWorkstationLookup
}

func (s *runtimeAwareScheduler) Select([]interfaces.EnabledTransition, *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) []interfaces.FiringDecision {
	return nil
}

func (s *runtimeAwareScheduler) SetRuntimeConfig(runtimeConfig interfaces.RuntimeWorkstationLookup) {
	s.configured = runtimeConfig
}

type generatedBatchHook struct {
	batch   interfaces.GeneratedSubmissionBatch
	emitted bool
}

func (h *generatedBatchHook) Name() string {
	return "generated-batch-test"
}

func (h *generatedBatchHook) Priority() int {
	return 1
}

func (h *generatedBatchHook) OnTick(context.Context, interfaces.SubmissionHookContext[interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]]) (interfaces.SubmissionHookResult, error) {
	if h.emitted {
		return interfaces.SubmissionHookResult{}, nil
	}
	h.emitted = true
	return interfaces.SubmissionHookResult{
		GeneratedBatches: []interfaces.GeneratedSubmissionBatch{h.batch},
	}, nil
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

func TestNew_RequiresNet(t *testing.T) {
	_, err := New()
	if err == nil {
		t.Fatal("expected error when Net is not provided")
	}
}

func TestNew_ConfiguresProvidedRuntimeAwareScheduler(t *testing.T) {
	net := buildSimpleNet()
	customScheduler := &runtimeAwareScheduler{}
	runtimeCfg := runtimeSchedulerConfig(&runtimefixtures.RuntimeDefinitionLookupFixture{})

	_, err := New(
		factory.WithNet(net),
		factory.WithScheduler(customScheduler),
		factory.WithRuntimeConfig(runtimeCfg),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if customScheduler.configured != runtimeCfg {
		t.Fatal("expected New to inject runtime config into provided scheduler")
	}

	var _ scheduler.Scheduler = customScheduler
}

func TestNew_InlineDispatchWithNoopExecutorCompletesWorkflow(t *testing.T) {
	n := buildSimpleNet()
	f, err := New(
		factory.WithNet(n),
		factory.WithInlineDispatch(),
		factory.WithWorkerExecutor("mock", &workers.NoopExecutor{}),
		factory.WithLogger(logging.NoopLogger{}),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go func() {
		time.Sleep(50 * time.Millisecond)
		_, _ = submitWorkRequests(ctx, f, []interfaces.SubmitRequest{{WorkTypeID: "task", TraceID: "trace-1"}})
	}()

	if err := f.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}

	snapshot, err := f.GetEngineStateSnapshot(ctx)
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	if snapshot.FactoryState != string(interfaces.FactoryStateCompleted) {
		t.Fatalf("factory state = %q, want %q", snapshot.FactoryState, interfaces.FactoryStateCompleted)
	}
}

func TestNew_InlineDispatchWithoutRegisteredExecutorRecordsMissingExecutorFailure(t *testing.T) {
	f, err := New(
		factory.WithNet(buildSimpleNetWithFailureArc()),
		factory.WithInlineDispatch(),
		factory.WithLogger(logging.NoopLogger{}),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	tickable, ok := f.(TickableFactory)
	if !ok {
		t.Fatal("factory is not tickable")
	}

	if _, err := submitWorkRequests(context.Background(), f, []interfaces.SubmitRequest{{
		WorkID:     "work-missing-executor",
		WorkTypeID: "task",
		TraceID:    "trace-missing-executor",
	}}); err != nil {
		t.Fatalf("SubmitWorkRequest: %v", err)
	}
	if err := tickable.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}

	snap, err := f.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	if len(snap.DispatchHistory) != 1 {
		t.Fatalf("dispatch history count = %d, want 1", len(snap.DispatchHistory))
	}
	completed := snap.DispatchHistory[0]
	if completed.Outcome != interfaces.OutcomeFailed {
		t.Fatalf("dispatch outcome = %q, want %q", completed.Outcome, interfaces.OutcomeFailed)
	}
	if !strings.Contains(completed.Reason, `no executor registered for worker type "mock"`) {
		t.Fatalf("dispatch reason = %q, want missing executor error", completed.Reason)
	}
}

// portos:func-length-exception owner=agent-factory reason=safe-diagnostics-boundary-e2e review=2026-07-21 removal=extract-runtime-safe-boundary-fixture-builders-before-next-boundary-regression-change
func TestNew_SafeDiagnosticsBoundarySurvivesReplayAndSelectedTickProjection(t *testing.T) {
	f, err := New(
		factory.WithNet(buildSimpleNetWithFailureArc()),
		factory.WithInlineDispatch(),
		factory.WithWorkerExecutor("mock", &safeDiagnosticsBoundaryExecutor{}),
		factory.WithLogger(logging.NoopLogger{}),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	tickable, ok := f.(TickableFactory)
	if !ok {
		t.Fatal("factory is not tickable")
	}

	if _, err := submitWorkRequests(context.Background(), f, []interfaces.SubmitRequest{
		{WorkID: "work-safe-success", WorkTypeID: "task", TraceID: "trace-safe-success", Payload: json.RawMessage(`{"story":"safe success"}`)},
		{WorkID: "work-safe-failure", WorkTypeID: "task", TraceID: "trace-safe-failure", Payload: json.RawMessage(`{"story":"safe failure"}`)},
		{WorkID: "work-safe-windows-process-failure", WorkTypeID: "task", TraceID: "trace-safe-windows-process-failure", Payload: json.RawMessage(`{"story":"safe windows process failure"}`)},
	}); err != nil {
		t.Fatalf("SubmitWorkRequest: %v", err)
	}

	for attempt := 0; attempt < 3; attempt++ {
		if err := tickable.Tick(context.Background()); err != nil {
			t.Fatalf("Tick attempt %d: %v", attempt+1, err)
		}
		if countFactoryEventType(runtimeGeneratedEvents(t, f), factoryapi.FactoryEventTypeDispatchResponse) == 3 {
			break
		}
	}

	events := runtimeGeneratedEvents(t, f)
	if got := countFactoryEventType(events, factoryapi.FactoryEventTypeDispatchResponse); got != 3 {
		t.Fatalf("dispatch completed event count = %d, want 3; events = %#v", got, events)
	}

	recordedAt := time.Date(2026, time.April, 21, 20, 0, 0, 0, time.UTC)
	artifact, err := replay.NewEventLogArtifactFromFactory(recordedAt, safeBoundaryGeneratedFactory(), nil, interfaces.ReplayDiagnostics{})
	if err != nil {
		t.Fatalf("NewEventLogArtifactFromFactory: %v", err)
	}
	artifact.Events = append(artifact.Events, events...)

	artifactPath := filepath.Join(t.TempDir(), "safe-boundary.replay.json")
	if err := replay.Save(artifactPath, artifact); err != nil {
		t.Fatalf("Save replay artifact: %v", err)
	}

	artifactJSON, err := os.ReadFile(artifactPath)
	if err != nil {
		t.Fatalf("ReadFile replay artifact: %v", err)
	}
	assertSafeBoundaryDoesNotLeak(t, string(artifactJSON))

	loaded, err := replay.Load(artifactPath)
	if err != nil {
		t.Fatalf("Load replay artifact: %v", err)
	}
	if got := countFactoryEventType(loaded.Events, factoryapi.FactoryEventTypeDispatchResponse); got != 3 {
		t.Fatalf("loaded dispatch completed event count = %d, want 3", got)
	}

	assertThinDispatchResponsesOmitRetiredProviderAttemptFields(t, loaded.Events)

	finalTick := maxEventTick(loaded.Events)
	worldState, err := projections.ReconstructFactoryWorldState(loaded.Events, finalTick)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState: %v", err)
	}
	if got := len(worldState.CompletedDispatches); got != 3 {
		t.Fatalf("completed dispatch count = %d, want 3", got)
	}
	if got := worldState.FailureDetailsByWorkID["work-safe-failure"].FailureReason; got != string(interfaces.ProviderErrorTypeTimeout) {
		t.Fatalf("failed work detail reason = %q, want timeout", got)
	}
	windowsDetail := worldState.FailureDetailsByWorkID["work-safe-windows-process-failure"]
	if windowsDetail.FailureReason != string(interfaces.ProviderErrorTypeInternalServerError) {
		t.Fatalf("windows failed work detail reason = %q, want %q", windowsDetail.FailureReason, interfaces.ProviderErrorTypeInternalServerError)
	}
	if windowsDetail.FailureMessage != "provider error: internal_server_error: codex exited with code 4294967295: stderr: OpenAI Codex v0.118.0 (research preview)" {
		t.Fatalf("windows failed work detail message = %q", windowsDetail.FailureMessage)
	}
	assertNoAuthRemediationText(t, windowsDetail.FailureMessage)
	assertSafeBoundaryDoesNotLeakJSON(t, worldState)

	view := projections.BuildFactoryWorldView(worldState)
	if view.Runtime.Session.DispatchedCount != 3 || view.Runtime.Session.FailedCount != 2 || len(view.Runtime.Session.DispatchHistory) != 3 {
		t.Fatalf("session counts = %#v, want dispatched=3 failed=2 with three request history rows", view.Runtime.Session)
	}

	successRequest := requestViewForWork(t, worldState, "work-safe-success")
	assertSafeBoundaryRequestView(t, successRequest, "", "", "", "")
	failureRequest := requestViewForWork(t, worldState, "work-safe-failure")
	assertSafeBoundaryRequestView(t, failureRequest, "", string(interfaces.ProviderErrorFamilyRetryable), string(interfaces.ProviderErrorTypeTimeout), "provider timed out")
	windowsRequest := requestViewForWork(t, worldState, "work-safe-windows-process-failure")
	assertSafeBoundaryRequestView(
		t,
		windowsRequest,
		"",
		string(interfaces.ProviderErrorFamilyRetryable),
		string(interfaces.ProviderErrorTypeInternalServerError),
		"provider error: internal_server_error: codex exited with code 4294967295: stderr: OpenAI Codex v0.118.0 (research preview)",
	)
	assertNoAuthRemediationText(t, stringValueForRuntimeTest(windowsRequest.Response.FailureMessage))
	assertSafeBoundaryDoesNotLeakJSON(t, view)
}

func TestNew_CompletesWorkflowThroughActiveSubsystems(t *testing.T) {
	f, err := New(
		factory.WithNet(buildSimpleNet()),
		factory.WithInlineDispatch(),
		factory.WithWorkerExecutor("mock", &passExecutor{}),
		factory.WithLogger(logging.NoopLogger{}),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := submitWorkRequests(context.Background(), f, []interfaces.SubmitRequest{{
		WorkID:     "work-active-path",
		WorkTypeID: "task",
		TraceID:    "trace-active-path",
	}}); err != nil {
		t.Fatalf("Submit: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := f.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}

	snapshot, err := f.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	if snapshot.FactoryState != string(interfaces.FactoryStateCompleted) {
		t.Fatalf("factory state = %q, want %q", snapshot.FactoryState, interfaces.FactoryStateCompleted)
	}
	if !markingContainsWorkAtPlace(&snapshot.Marking, "work-active-path", "task:done") {
		t.Fatalf("expected work-active-path to reach task:done, marking=%#v", snapshot.Marking.PlaceTokens)
	}

	events := runtimeGeneratedEvents(t, f)
	eventTypes := factoryEventTypes(events)
	if !hasFactoryEventType(events, factoryapi.FactoryEventTypeDispatchRequest) {
		t.Fatalf("expected generated dispatch-created event, got %v", eventTypes)
	}
	if !hasFactoryEventType(events, factoryapi.FactoryEventTypeDispatchResponse) {
		t.Fatalf("expected generated dispatch-completed event, got %v", eventTypes)
	}
}

// portos:func-length-exception owner=agent-factory reason=runtime-event-history-fixture review=2026-07-18 removal=split-setup-recording-and-event-assertions-before-next-history-expansion
func TestFactoryEventHistory_RecordsOrderedEventsWithStableIDs(t *testing.T) {
	f, err := New(
		factory.WithNet(buildSimpleNet()),
		factory.WithInlineDispatch(),
		factory.WithWorkerExecutor("mock", &passExecutor{}),
		factory.WithLogger(logging.NoopLogger{}),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	tickable, ok := f.(TickableFactory)
	if !ok {
		t.Fatal("factory is not tickable")
	}

	if _, err := submitWorkRequests(context.Background(), f, []interfaces.SubmitRequest{{
		WorkID:     "work-1",
		Name:       "Write PRD",
		WorkTypeID: "task",
		TraceID:    "trace-1",
		Relations: []interfaces.Relation{{
			Type:          interfaces.RelationDependsOn,
			TargetWorkID:  "upstream-1",
			RequiredState: "done",
		}},
	}}); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if err := tickable.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	if err := f.Pause(context.Background()); err != nil {
		t.Fatalf("Pause: %v", err)
	}

	events := runtimeGeneratedEvents(t, f)
	wantTypes := []factoryapi.FactoryEventType{
		factoryapi.FactoryEventTypeRunRequest,
		factoryapi.FactoryEventTypeInitialStructureRequest,
		factoryapi.FactoryEventTypeWorkRequest,
		factoryapi.FactoryEventTypeRelationshipChangeRequest,
		factoryapi.FactoryEventTypeDispatchRequest,
		factoryapi.FactoryEventTypeDispatchResponse,
		factoryapi.FactoryEventTypeFactoryStateResponse,
	}
	if len(events) != len(wantTypes) {
		t.Fatalf("event count = %d, want %d: %#v", len(events), len(wantTypes), events)
	}
	for i, wantType := range wantTypes {
		if events[i].Type != wantType {
			t.Fatalf("event[%d] type = %q, want %q", i, events[i].Type, wantType)
		}
		if events[i].Id == "" {
			t.Fatalf("event[%d] has empty id", i)
		}
		if i > 0 && events[i].Context.Tick < events[i-1].Context.Tick {
			t.Fatalf("event[%d] tick = %d before event[%d] tick = %d", i, events[i].Context.Tick, i-1, events[i-1].Context.Tick)
		}
	}

	batch, err := events[2].Payload.AsWorkRequestEventPayload()
	if err != nil {
		t.Fatalf("work request payload: %v", err)
	}
	if events[2].Context.RequestId == nil || batch.Type != factoryapi.WorkRequestTypeFactoryRequestBatch || firstRuntimeTestString(events[2].Context.TraceIds) != "trace-1" {
		t.Fatalf("work request payload = %#v, want canonical batch identity", batch)
	}
	if batch.Works == nil || len(*batch.Works) != 1 || stringValueForRuntimeTest((*batch.Works)[0].WorkId) != "work-1" {
		t.Fatalf("work request items = %#v, want work-1", batch.Works)
	}

	relation, err := events[3].Payload.AsRelationshipChangeRequestEventPayload()
	if err != nil {
		t.Fatalf("relationship payload: %v", err)
	}
	if relation.Relation.Type != factoryapi.RelationTypeDependsOn ||
		events[3].Context.WorkIds == nil ||
		stringValueForRuntimeTest(relation.Relation.TargetWorkId) != "upstream-1" {
		t.Fatalf("relationship payload = %#v, want submitted dependency", relation)
	}

	request, err := events[4].Payload.AsDispatchRequestEventPayload()
	if err != nil {
		t.Fatalf("dispatch created payload: %v", err)
	}
	if stringValueForRuntimeTest(events[4].Context.DispatchId) == "" || request.TransitionId != "t-process" {
		t.Fatalf("workstation request payload = %#v, want dispatch identity", request)
	}
	if len(request.Inputs) != 1 || request.Inputs[0].WorkId != "work-1" {
		t.Fatalf("workstation request inputs = %#v, want consumed work item", request.Inputs)
	}

	response, err := events[5].Payload.AsDispatchResponseEventPayload()
	if err != nil {
		t.Fatalf("dispatch completed payload: %v", err)
	}
	if stringValueForRuntimeTest(events[5].Context.DispatchId) != stringValueForRuntimeTest(events[4].Context.DispatchId) || response.Outcome != factoryapi.WorkOutcomeAccepted {
		t.Fatalf("workstation response payload = %#v, want accepted dispatch response", response)
	}
	if response.OutputWork == nil || len(*response.OutputWork) == 0 || stringValueForRuntimeTest((*response.OutputWork)[0].WorkId) != "work-1" {
		t.Fatalf("output work = %#v, want completed work item", response.OutputWork)
	}

	world, err := projections.ReconstructFactoryWorldState(events, events[5].Context.Tick)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState: %v", err)
	}
	if len(world.CompletedDispatches) != 1 || world.CompletedDispatches[0].DispatchID != stringValueForRuntimeTest(events[4].Context.DispatchId) {
		t.Fatalf("CompletedDispatches = %#v, want completed dispatch reconstructed from canonical events", world.CompletedDispatches)
	}
	if got := world.PlaceOccupancyByID["task:done"].WorkItemIDs; len(got) != 1 || got[0] != "work-1" {
		t.Fatalf("task:done occupancy = %#v, want work-1", world.PlaceOccupancyByID["task:done"])
	}
	view := projections.BuildFactoryWorldView(world)
	if view.Runtime.Session.CompletedCount != 1 {
		t.Fatalf("CompletedCount = %d, want 1", view.Runtime.Session.CompletedCount)
	}
	if got := view.Runtime.PlaceTokenCounts["task:done"]; got != 1 {
		t.Fatalf("task:done count = %d, want 1", got)
	}

	again := runtimeGeneratedEvents(t, f)
	for i := range events {
		if again[i].Id != events[i].Id {
			t.Fatalf("event[%d] id changed from %q to %q", i, events[i].Id, again[i].Id)
		}
	}
}

func TestNew_SubmitWorkRequestRecordsCanonicalWorkRequestEvent(t *testing.T) {
	f, err := New(
		factory.WithNet(buildSimpleNet()),
		factory.WithInlineDispatch(),
		factory.WithWorkerExecutor("mock", &passExecutor{}),
		factory.WithLogger(logging.NoopLogger{}),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	tickable, ok := f.(TickableFactory)
	if !ok {
		t.Fatal("factory is not tickable")
	}

	request := interfaces.WorkRequest{
		RequestID: "request-canonical-work-event",
		Type:      interfaces.WorkRequestTypeFactoryRequestBatch,
		Works: []interfaces.Work{{
			Name:       "canonical",
			WorkID:     "work-canonical",
			WorkTypeID: "task",
			TraceID:    "trace-canonical",
		}},
	}
	if _, err := f.SubmitWorkRequest(context.Background(), request); err != nil {
		t.Fatalf("SubmitWorkRequest: %v", err)
	}
	if err := tickable.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}

	events := runtimeGeneratedEvents(t, f)
	if len(events) < 3 {
		t.Fatalf("events = %#v, want run-started, initial structure, and canonical work request", events)
	}
	event := events[2]
	if event.Type != factoryapi.FactoryEventTypeWorkRequest {
		t.Fatalf("event[2] type = %q, want %q", event.Type, factoryapi.FactoryEventTypeWorkRequest)
	}
	if stringValueForRuntimeTest(event.Context.RequestId) != "request-canonical-work-event" ||
		firstRuntimeTestString(event.Context.TraceIds) != "trace-canonical" {
		t.Fatalf("event context = %#v, want submitted request and trace", event.Context)
	}
	payload, err := event.Payload.AsWorkRequestEventPayload()
	if err != nil {
		t.Fatalf("work request payload: %v", err)
	}
	if payload.Type != factoryapi.WorkRequestTypeFactoryRequestBatch ||
		payload.Works == nil ||
		len(*payload.Works) != 1 ||
		stringValueForRuntimeTest((*payload.Works)[0].WorkId) != "work-canonical" {
		t.Fatalf("work request payload = %#v, want canonical generated batch", payload)
	}
}

// portos:func-length-exception owner=agent-factory reason=batch-request-event-fixture review=2026-07-18 removal=split-batch-fixture-and-relationship-assertions-before-next-request-history-change
func TestFactoryEventHistory_BatchRequestAndRelationshipReplay(t *testing.T) {
	f, err := New(
		factory.WithNet(buildSimpleNet()),
		factory.WithInlineDispatch(),
		factory.WithWorkerExecutor("mock", &passExecutor{}),
		factory.WithLogger(logging.NoopLogger{}),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	tickable, ok := f.(TickableFactory)
	if !ok {
		t.Fatal("factory is not tickable")
	}

	var request interfaces.WorkRequest
	if err := json.Unmarshal([]byte(`{
		"request_id": "request-batch-events",
		"type": "FACTORY_REQUEST_BATCH",
		"works": [
			{"name": "first", "work_id": "work-first", "work_type_name": "task", "trace_id": "trace-batch"},
			{"name": "second", "work_id": "work-second", "work_type_name": "task"}
		],
		"relations": [
			{"type": "DEPENDS_ON", "source_work_name": "second", "target_work_name": "first", "required_state": "done"}
		]
	}`), &request); err != nil {
		t.Fatalf("Unmarshal WorkRequest: %v", err)
	}
	result, err := f.SubmitWorkRequest(context.Background(), request)
	if err != nil {
		t.Fatalf("SubmitWorkRequest: %v", err)
	}
	if result.RequestID != "request-batch-events" || result.TraceID != "trace-batch" || !result.Accepted {
		t.Fatalf("submit result = %#v, want accepted stable request metadata", result)
	}
	repeated, err := f.SubmitWorkRequest(context.Background(), request)
	if err != nil {
		t.Fatalf("duplicate SubmitWorkRequest: %v", err)
	}
	if repeated.RequestID != result.RequestID || repeated.TraceID != result.TraceID || repeated.Accepted {
		t.Fatalf("duplicate submit result = %#v, want original metadata with Accepted=false", repeated)
	}
	if err := tickable.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}

	events := runtimeGeneratedEvents(t, f)
	types := factoryEventTypes(events)
	wantPrefix := []factoryapi.FactoryEventType{
		factoryapi.FactoryEventTypeRunRequest,
		factoryapi.FactoryEventTypeInitialStructureRequest,
		factoryapi.FactoryEventTypeWorkRequest,
		factoryapi.FactoryEventTypeRelationshipChangeRequest,
	}
	for i, want := range wantPrefix {
		if types[i] != want {
			t.Fatalf("event[%d] type = %q, want %q (all types %v)", i, types[i], want, types)
		}
	}

	batch, err := events[2].Payload.AsWorkRequestEventPayload()
	if err != nil {
		t.Fatalf("batch payload: %v", err)
	}
	if stringValueForRuntimeTest(events[2].Context.RequestId) != "request-batch-events" ||
		stringValueForRuntimeTest(batch.Source) != "external-submit" ||
		firstRuntimeTestString(events[2].Context.TraceIds) != "trace-batch" {
		t.Fatalf("batch payload = %#v, want request/source/trace metadata", batch)
	}
	if batch.Works == nil || len(*batch.Works) != 2 ||
		stringValueForRuntimeTest((*batch.Works)[0].WorkId) != "work-first" ||
		stringValueForRuntimeTest((*batch.Works)[1].WorkId) != "work-second" ||
		stringValueForRuntimeTest((*batch.Works)[0].WorkTypeName) != "task" ||
		stringValueForRuntimeTest((*batch.Works)[1].WorkTypeName) != "task" {
		t.Fatalf("batch work items = %#v, want first and second", batch.Works)
	}
	if workRequestEvents := countFactoryEventsByType(events, factoryapi.FactoryEventTypeWorkRequest); workRequestEvents != 1 {
		t.Fatalf("work request events = %d, want 1 after idempotent retry", workRequestEvents)
	}

	relation, err := events[3].Payload.AsRelationshipChangeRequestEventPayload()
	if err != nil {
		t.Fatalf("relationship payload: %v", err)
	}
	if relation.Relation.SourceWorkName != "second" ||
		stringValueForRuntimeTest(relation.Relation.TargetWorkId) != "work-first" ||
		relation.Relation.TargetWorkName != "first" ||
		stringValueForRuntimeTest(relation.Relation.RequiredState) != "done" ||
		stringValueForRuntimeTest(events[3].Context.RequestId) != "request-batch-events" ||
		firstRuntimeTestString(events[3].Context.TraceIds) != "trace-batch" {
		t.Fatalf("relationship payload = %#v, want named batch dependency", relation)
	}

	world, err := projections.ReconstructFactoryWorldState(events, events[3].Context.Tick)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState: %v", err)
	}
	if got := world.WorkRequestsByID["request-batch-events"].WorkItems; len(got) != 2 {
		t.Fatalf("replayed batch work items = %#v, want 2 items", got)
	}
	relations := world.RelationsByWorkID["work-second"]
	if len(relations) != 1 || relations[0].TargetWorkID != "work-first" || relations[0].RequiredState != "done" {
		t.Fatalf("replayed relations = %#v, want second depends on first", relations)
	}
}

// portos:func-length-exception owner=agent-factory reason=generated-batch-event-fixture review=2026-07-18 removal=split-generated-batch-fixture-and-ordering-assertions-before-next-generated-batch-change
func TestFactoryEventHistory_GeneratedBatchPreservesMetadataAndOrdering(t *testing.T) {
	batch := interfaces.GeneratedSubmissionBatch{
		Request: interfaces.WorkRequest{
			RequestID: "generated-request-events",
			Type:      interfaces.WorkRequestTypeFactoryRequestBatch,
			Works: []interfaces.Work{
				{Name: "draft", WorkID: "work-draft", WorkTypeID: "task", TraceID: "trace-generated"},
				{Name: "review", WorkID: "work-review", WorkTypeID: "task"},
			},
			Relations: []interfaces.WorkRelation{{
				Type:           interfaces.WorkRelationDependsOn,
				SourceWorkName: "review",
				TargetWorkName: "draft",
				RequiredState:  "done",
			}},
		},
		Metadata: interfaces.GeneratedSubmissionBatchMetadata{
			Source: "worker-output:dispatch-parent",
			RelationContext: []interfaces.WorkRelation{{
				Type:           interfaces.WorkRelationDependsOn,
				SourceWorkName: "review",
				TargetWorkName: "draft",
				RequiredState:  "done",
			}},
			ParentLineage: []string{"request-parent", "work-parent"},
		},
		Submissions: []interfaces.SubmitRequest{{
			Name:        "review",
			WorkID:      "work-review",
			TargetState: "done",
			Tags:        map[string]string{"runtime": "true"},
		}},
	}
	f, err := New(
		factory.WithNet(buildSimpleNet()),
		factory.WithInlineDispatch(),
		factory.WithWorkerExecutor("mock", &passExecutor{}),
		factory.WithSubmissionHook(&generatedBatchHook{batch: batch}),
		factory.WithLogger(logging.NoopLogger{}),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	tickable, ok := f.(TickableFactory)
	if !ok {
		t.Fatal("factory is not tickable")
	}

	if err := tickable.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}

	events := runtimeGeneratedEvents(t, f)
	types := factoryEventTypes(events)
	wantPrefix := []factoryapi.FactoryEventType{
		factoryapi.FactoryEventTypeRunRequest,
		factoryapi.FactoryEventTypeInitialStructureRequest,
		factoryapi.FactoryEventTypeWorkRequest,
		factoryapi.FactoryEventTypeRelationshipChangeRequest,
	}
	if len(types) < len(wantPrefix) {
		t.Fatalf("event types = %v, want at least %v", types, wantPrefix)
	}
	for i, want := range wantPrefix {
		if types[i] != want {
			t.Fatalf("event[%d] type = %q, want %q (all types %v)", i, types[i], want, types)
		}
	}

	requestPayload, err := events[2].Payload.AsWorkRequestEventPayload()
	if err != nil {
		t.Fatalf("request payload: %v", err)
	}
	if stringValueForRuntimeTest(events[2].Context.RequestId) != "generated-request-events" ||
		stringValueForRuntimeTest(requestPayload.Source) != "worker-output:dispatch-parent" ||
		firstRuntimeTestString(events[2].Context.TraceIds) != "trace-generated" {
		t.Fatalf("request payload = %#v, want generated request metadata", requestPayload)
	}
	if got := strings.Join(sliceValueForRuntimeTest(requestPayload.ParentLineage), ","); got != "request-parent,work-parent" {
		t.Fatalf("parent lineage = %#v, want generated lineage metadata", requestPayload.ParentLineage)
	}
	if requestPayload.Works == nil || len(*requestPayload.Works) != 2 {
		t.Fatalf("request works = %#v, want generated work metadata", requestPayload.Works)
	}
	for _, work := range *requestPayload.Works {
		if stringValueForRuntimeTest(work.CurrentChainingTraceId) != "trace-generated" {
			t.Fatalf("generated work current chaining trace ID = %q, want trace-generated", stringValueForRuntimeTest(work.CurrentChainingTraceId))
		}
		if got := sliceValueForRuntimeTest(work.PreviousChainingTraceIds); len(got) != 0 {
			t.Fatalf("generated hook work previous chaining trace IDs = %#v, want none without consumed input lineage", got)
		}
	}

	relationPayload, err := events[3].Payload.AsRelationshipChangeRequestEventPayload()
	if err != nil {
		t.Fatalf("relationship payload: %v", err)
	}
	if stringValueForRuntimeTest(relationPayload.Relation.TargetWorkId) != "work-draft" ||
		stringValueForRuntimeTest(events[3].Context.RequestId) != "generated-request-events" ||
		firstRuntimeTestString(events[3].Context.TraceIds) != "trace-generated" {
		t.Fatalf("relationship payload = %#v, want generated request dependency", relationPayload)
	}

	world, err := projections.ReconstructFactoryWorldState(events, events[3].Context.Tick)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState: %v", err)
	}
	replayed := world.WorkRequestsByID["generated-request-events"]
	if got := strings.Join(replayed.ParentLineage, ","); got != "request-parent,work-parent" {
		t.Fatalf("replayed parent lineage = %#v, want generated lineage metadata", replayed.ParentLineage)
	}
	if len(replayed.WorkItems) != 2 {
		t.Fatalf("replayed work items = %#v, want generated request work", replayed.WorkItems)
	}
}

func TestNew_InitialStructureIncludesRuntimeConfigWorkerMetadata(t *testing.T) {
	f, err := New(
		factory.WithNet(buildSimpleNet()),
		factory.WithInlineDispatch(),
		factory.WithWorkerExecutor("mock", &passExecutor{}),
		factory.WithRuntimeConfig(runtimeProjectionConfig{
			Workers: map[string]*interfaces.WorkerConfig{
				"mock": {
					Type:             interfaces.WorkerTypeModel,
					ExecutorProvider: "codex-cli",
					ModelProvider:    "openai",
					Model:            "gpt-5.4",
				},
			},
		}),
		factory.WithLogger(logging.NoopLogger{}),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	events, err := f.GetFactoryEvents(context.Background())
	if err != nil {
		t.Fatalf("GetFactoryEvents: %v", err)
	}
	if len(events) != 2 || events[0].Type != factoryapi.FactoryEventTypeRunRequest || events[1].Type != factoryapi.FactoryEventTypeInitialStructureRequest {
		t.Fatalf("events = %#v, want run-started then initial structure only", events)
	}
	payload, err := events[1].Payload.AsInitialStructureRequestEventPayload()
	if err != nil {
		t.Fatalf("initial structure payload: %v", err)
	}
	if payload.Factory.Workers == nil || len(*payload.Factory.Workers) != 1 {
		t.Fatalf("Workers = %#v, want one runtime worker", payload.Factory.Workers)
	}
	worker := (*payload.Factory.Workers)[0]
	if worker.Name != "mock" || stringValueForRuntimeTest(worker.ExecutorProvider) != "script_wrap" ||
		stringValueForRuntimeTest(worker.ModelProvider) != "codex" ||
		stringValueForRuntimeTest(worker.Model) != "gpt-5.4" {
		t.Fatalf("worker metadata = %#v, want runtime config provider/model metadata", worker)
	}
}

func TestFactoryEventHistory_SubscribeReplaysHistoryThenStreamsLiveEvents(t *testing.T) {
	f, err := New(
		factory.WithNet(buildSimpleNet()),
		factory.WithInlineDispatch(),
		factory.WithWorkerExecutor("mock", &passExecutor{}),
		factory.WithLogger(logging.NoopLogger{}),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stream, err := f.SubscribeFactoryEvents(ctx)
	if err != nil {
		t.Fatalf("SubscribeFactoryEvents: %v", err)
	}
	if len(stream.History) != 2 ||
		stream.History[0].Type != factoryapi.FactoryEventTypeRunRequest ||
		stream.History[1].Type != factoryapi.FactoryEventTypeInitialStructureRequest {
		t.Fatalf("replayed history = %#v, want run-started and initial structure events", stream.History)
	}

	if _, err := submitWorkRequests(context.Background(), f, []interfaces.SubmitRequest{{WorkTypeID: "task", TraceID: "trace-live"}}); err != nil {
		t.Fatalf("SubmitWorkRequest: %v", err)
	}
	tickable, ok := f.(TickableFactory)
	if !ok {
		t.Fatal("factory is not tickable")
	}
	if err := tickable.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}

	select {
	case event := <-stream.Events:
		if event.Type != factoryapi.FactoryEventTypeWorkRequest {
			t.Fatalf("live event = %#v, want work request event", event)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for live canonical factory event")
	}

	cancel()
	deadline := time.After(time.Second)
	select {
	case <-deadline:
		t.Fatal("timed out waiting for live event stream closure")
	default:
	}
	for {
		select {
		case _, ok := <-stream.Events:
			if !ok {
				return
			}
		case <-deadline:
			t.Fatal("timed out waiting for live event stream closure")
		}
	}
}

func TestNew_BatchModeWithoutInitialWork_TerminatesWithoutCancellation(t *testing.T) {
	f, err := New(
		factory.WithNet(buildSimpleNet()),
		factory.WithInlineDispatch(),
		factory.WithWorkerExecutor("mock", &workers.NoopExecutor{}),
		factory.WithLogger(logging.NoopLogger{}),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if err := f.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}

	snapshot, err := f.GetEngineStateSnapshot(ctx)
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	if snapshot.FactoryState != string(interfaces.FactoryStateCompleted) {
		t.Fatalf("factory state = %q, want %q", snapshot.FactoryState, interfaces.FactoryStateCompleted)
	}
}

func TestNew_ServiceModeWithoutInitialWork_WaitsForCancellation(t *testing.T) {
	f, err := New(
		factory.WithNet(buildSimpleNet()),
		factory.WithServiceMode(),
		factory.WithInlineDispatch(),
		factory.WithWorkerExecutor("mock", &workers.NoopExecutor{}),
		factory.WithLogger(logging.NoopLogger{}),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- f.Run(ctx)
	}()

	select {
	case err := <-errCh:
		t.Fatalf("Run returned before cancellation: %v", err)
	case <-time.After(150 * time.Millisecond):
	}

	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Run after cancellation: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for service-mode runtime to stop after cancellation")
	}
}

func factoryEventTypes(events []factoryapi.FactoryEvent) []factoryapi.FactoryEventType {
	types := make([]factoryapi.FactoryEventType, len(events))
	for i, event := range events {
		types[i] = event.Type
	}
	return types
}

func countFactoryEventsByType(events []factoryapi.FactoryEvent, eventType factoryapi.FactoryEventType) int {
	count := 0
	for _, event := range events {
		if event.Type == eventType {
			count++
		}
	}
	return count
}

func TestNew_ServiceModeWithoutInitialWork_AcceptsLateSubmission(t *testing.T) {
	f, err := New(
		factory.WithNet(buildSimpleNet()),
		factory.WithServiceMode(),
		factory.WithInlineDispatch(),
		factory.WithWorkerExecutor("mock", &workers.NoopExecutor{}),
		factory.WithLogger(logging.NoopLogger{}),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- f.Run(ctx)
	}()

	select {
	case err := <-errCh:
		t.Fatalf("Run returned before late submission: %v", err)
	case <-time.After(150 * time.Millisecond):
	}

	runtimeBeforeSubmit, err := f.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot before submit: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	runtimeAfterIdleWait, err := f.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot after idle wait: %v", err)
	}

	if runtimeAfterIdleWait.TickCount != runtimeBeforeSubmit.TickCount {
		t.Fatalf("idle service mode should not busy-spin: tick count advanced from %d to %d without new events",
			runtimeBeforeSubmit.TickCount,
			runtimeAfterIdleWait.TickCount,
		)
	}

	if _, err := submitWorkRequests(context.Background(), f, []interfaces.SubmitRequest{{WorkTypeID: "task", TraceID: "trace-late-submit"}}); err != nil {
		t.Fatalf("SubmitWorkRequest late work: %v", err)
	}

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		snap, err := f.GetEngineStateSnapshot(context.Background())
		if err != nil {
			t.Fatalf("GetEngineStateSnapshot: %v", err)
		}
		for _, token := range snap.Marking.Tokens {
			if token.PlaceID == "task:done" {
				cancel()
				select {
				case err := <-errCh:
					if err != nil {
						t.Fatalf("Run after cancellation: %v", err)
					}
				case <-time.After(time.Second):
					t.Fatal("timed out waiting for service-mode runtime to stop after cancellation")
				}
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}

	cancel()
	<-errCh
	t.Fatal("late-submitted work did not reach task:done before timeout")
}

func TestNew_BatchModeWithoutInitialWork_RejectsLateSubmissionAfterTermination(t *testing.T) {
	f, err := New(
		factory.WithNet(buildSimpleNet()),
		factory.WithInlineDispatch(),
		factory.WithWorkerExecutor("mock", &workers.NoopExecutor{}),
		factory.WithLogger(logging.NoopLogger{}),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if err := f.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}

	_, err = submitWorkRequests(context.Background(), f, []interfaces.SubmitRequest{{WorkTypeID: "task", TraceID: "trace-after-stop"}})
	if err == nil {
		t.Fatal("expected late batch submission to fail after runtime termination")
	}
	if !strings.Contains(err.Error(), "terminated") {
		t.Fatalf("expected terminated error, got %v", err)
	}
}

func TestNew_WithMockExecutor(t *testing.T) {
	if _, err := New(factory.WithNet(buildSimpleNet()), factory.WithWorkerExecutor("mock", &passExecutor{})); err != nil {
		t.Fatalf("New: %v", err)
	}
}

func TestNew_WorkerPoolDispatchResultHookRecordsCompletionAtObservedTick(t *testing.T) {
	var dispatches []interfaces.FactoryDispatchRecord
	var completions []interfaces.FactoryCompletionRecord
	f, err := New(
		factory.WithNet(buildSimpleNet()),
		factory.WithWorkerExecutor("mock", &passExecutor{}),
		factory.WithDispatchRecorder(func(record interfaces.FactoryDispatchRecord) {
			dispatches = append(dispatches, record)
		}),
		factory.WithCompletionRecorder(func(record interfaces.FactoryCompletionRecord) {
			completions = append(completions, record)
		}),
		factory.WithLogger(logging.NoopLogger{}),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := submitWorkRequests(context.Background(), f, []interfaces.SubmitRequest{{WorkTypeID: "task", TraceID: "trace-hook"}}); err != nil {
		t.Fatalf("SubmitWorkRequest: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := f.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(dispatches) != 1 {
		t.Fatalf("expected 1 recorded dispatch, got %d", len(dispatches))
	}
	if len(completions) != 1 {
		t.Fatalf("expected 1 recorded completion, got %d", len(completions))
	}
	dispatch := dispatches[0].Dispatch
	if completions[0].DispatchID != dispatch.DispatchID {
		t.Fatalf("completion dispatch ID = %q, want %q", completions[0].DispatchID, dispatch.DispatchID)
	}
	if completions[0].ObservedTick <= dispatch.Execution.DispatchCreatedTick {
		t.Fatalf("completion observed tick = %d, want after dispatch tick %d", completions[0].ObservedTick, dispatch.Execution.DispatchCreatedTick)
	}
}

func TestNew_ReplayDelayedWorkerPoolCompletionWakesAtPlannedTick(t *testing.T) {
	var completions []interfaces.FactoryCompletionRecord
	f, err := New(
		factory.WithNet(buildSimpleNet()),
		factory.WithWorkerExecutor("mock", &passExecutor{}),
		factory.WithCompletionDeliveryPlanner(fixedCompletionDeliveryPlanner{tick: 4}),
		factory.WithCompletionRecorder(func(record interfaces.FactoryCompletionRecord) {
			completions = append(completions, record)
		}),
		factory.WithLogger(logging.NoopLogger{}),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := submitWorkRequests(context.Background(), f, []interfaces.SubmitRequest{{WorkTypeID: "task", TraceID: "trace-delayed"}}); err != nil {
		t.Fatalf("SubmitWorkRequest: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := f.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(completions) != 1 {
		t.Fatalf("expected 1 completion, got %d", len(completions))
	}
	if completions[0].ObservedTick != 4 {
		t.Fatalf("completion observed tick = %d, want 4", completions[0].ObservedTick)
	}

	snap, err := f.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	if len(snap.Marking.TokensInPlace("task:done")) != 1 {
		t.Fatalf("expected token to reach task:done at planned completion tick, marking = %#v", snap.Marking.Tokens)
	}
}

func TestNew_ReplayPlannerCanReplaceWorkerCompletionResult(t *testing.T) {
	var completions []interfaces.FactoryCompletionRecord
	f, err := New(
		factory.WithNet(buildSimpleNet()),
		factory.WithWorkerExecutor("mock", &passExecutor{}),
		factory.WithCompletionDeliveryPlanner(fixedCompletionDeliveryPlanner{
			tick: 4,
			plannedResult: interfaces.WorkResult{
				Outcome: interfaces.OutcomeAccepted,
				Output:  "replayed-output",
			},
		}),
		factory.WithCompletionRecorder(func(record interfaces.FactoryCompletionRecord) {
			completions = append(completions, record)
		}),
		factory.WithLogger(logging.NoopLogger{}),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := submitWorkRequests(context.Background(), f, []interfaces.SubmitRequest{{WorkTypeID: "task", TraceID: "trace-replayed-result"}}); err != nil {
		t.Fatalf("SubmitWorkRequest: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := f.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(completions) != 1 {
		t.Fatalf("expected 1 completion, got %d", len(completions))
	}
	if completions[0].Result.Output != "replayed-output" {
		t.Fatalf("completion output = %q, want replayed-output", completions[0].Result.Output)
	}
}

func TestNew_ServiceModeWorkerPoolResultSignalCompletesLateSubmission(t *testing.T) {
	executor := &blockingExecutor{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	f, err := New(
		factory.WithNet(buildSimpleNet()),
		factory.WithServiceMode(),
		factory.WithWorkerExecutor("mock", executor),
		factory.WithLogger(logging.NoopLogger{}),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	runCtx, cancelRun := context.WithCancel(context.Background())
	defer cancelRun()
	errCh := make(chan error, 1)
	go func() {
		errCh <- f.Run(runCtx)
	}()

	select {
	case err := <-errCh:
		t.Fatalf("Run returned before late worker-pool submission: %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	if _, err := submitWorkRequests(context.Background(), f, []interfaces.SubmitRequest{{
		WorkID:     "work-late-pool",
		WorkTypeID: "task",
		TraceID:    "trace-late-pool",
	}}); err != nil {
		t.Fatalf("SubmitWorkRequest late work: %v", err)
	}

	select {
	case <-executor.started:
	case err := <-errCh:
		t.Fatalf("Run returned before worker result: %v", err)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for worker-pool executor to start")
	}
	waitForAggregateSnapshot(t, f, func(snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) bool {
		return snapshot.InFlightCount > 0
	})

	close(executor.release)
	snap := waitForAggregateSnapshot(t, f, func(snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) bool {
		return snapshot.InFlightCount == 0 &&
			len(snapshot.DispatchHistory) == 1 &&
			markingContainsWorkAtPlace(&snapshot.Marking, "work-late-pool", "task:done")
	})
	if !hasFactoryEventType(runtimeGeneratedEvents(t, f), factoryapi.FactoryEventTypeDispatchResponse) {
		t.Fatalf("expected generated dispatch-completed event after result wake-up, snapshot=%#v", snap)
	}

	cancelRun()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Run after cancellation: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for service-mode runtime to stop")
	}
}

func TestSubmit_AssignsTraceIDWhenMissing(t *testing.T) {
	f, err := New(
		factory.WithNet(buildSimpleNet()),
		factory.WithInlineDispatch(),
		factory.WithWorkerExecutor("mock", &workers.NoopExecutor{}),
		factory.WithLogger(logging.NoopLogger{}),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	tickable, ok := f.(TickableFactory)
	if !ok {
		t.Fatal("expected TickableFactory")
	}

	if _, err := submitWorkRequests(context.Background(), f, []interfaces.SubmitRequest{{WorkTypeID: "task"}}); err != nil {
		t.Fatalf("SubmitWorkRequest: %v", err)
	}
	if err := tickable.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}

	snap, err := f.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}

	if len(snap.Marking.Tokens) != 1 {
		t.Fatalf("expected 1 token, got %d", len(snap.Marking.Tokens))
	}
	for _, tok := range snap.Marking.Tokens {
		if tok.Color.TraceID == "" {
			t.Fatal("expected submitted token to have an assigned trace ID")
		}
	}
}

func TestNew_WithClockStampsDispatchesDeterministically(t *testing.T) {
	base := time.Date(2026, time.April, 10, 12, 0, 0, 0, time.UTC)
	clock := replay.NewDeterministicClock(base, time.Second)
	f, err := New(
		factory.WithNet(buildSimpleNet()),
		factory.WithInlineDispatch(),
		factory.WithWorkerExecutor("mock", &workers.NoopExecutor{}),
		factory.WithLogger(logging.NoopLogger{}),
		factory.WithClock(clock),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	tickable, ok := f.(TickableFactory)
	if !ok {
		t.Fatal("expected TickableFactory")
	}
	if _, err := submitWorkRequests(context.Background(), f, []interfaces.SubmitRequest{{WorkTypeID: "task", TraceID: "trace-clock"}}); err != nil {
		t.Fatalf("SubmitWorkRequest: %v", err)
	}
	if err := tickable.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}

	snap, err := f.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	if len(snap.DispatchHistory) != 1 {
		t.Fatalf("expected 1 completed dispatch, got %d", len(snap.DispatchHistory))
	}
	want := base.Add(time.Second)
	completed := snap.DispatchHistory[0]
	if !completed.StartTime.Equal(want) {
		t.Fatalf("dispatch start = %s, want %s", completed.StartTime, want)
	}
	if !completed.EndTime.Equal(want) {
		t.Fatalf("dispatch end = %s, want %s", completed.EndTime, want)
	}
}

func TestGetEngineStateSnapshot_AggregatesRuntimeLifecycleUptimeAndTopology(t *testing.T) {
	base := time.Date(2026, time.April, 10, 12, 0, 0, 0, time.UTC)
	executor := &blockingExecutor{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	net := buildSimpleNet()
	f, err := New(
		factory.WithNet(net),
		factory.WithServiceMode(),
		factory.WithLogger(logging.NoopLogger{}),
		factory.WithClock(replay.NewDeterministicClock(base, time.Second)),
		factory.WithWorkerExecutor("mock", executor),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	runCtx, cancelRun := context.WithCancel(context.Background())
	defer cancelRun()
	errCh := make(chan error, 1)
	go func() {
		errCh <- f.Run(runCtx)
	}()

	if _, err := submitWorkRequests(context.Background(), f, []interfaces.SubmitRequest{{
		WorkTypeID: "task",
		TraceID:    "trace-aggregate-snapshot",
	}}); err != nil {
		t.Fatalf("SubmitWorkRequest: %v", err)
	}

	select {
	case <-executor.started:
	case err := <-errCh:
		t.Fatalf("Run returned before in-flight snapshot: %v", err)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for blocking executor to start")
	}

	snap := waitForAggregateSnapshot(t, f, func(snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) bool {
		return snapshot.RuntimeStatus == interfaces.RuntimeStatusActive && snapshot.InFlightCount > 0
	})

	if snap.FactoryState != string(interfaces.FactoryStateRunning) {
		t.Fatalf("factory state = %q, want %q", snap.FactoryState, interfaces.FactoryStateRunning)
	}
	if snap.Uptime <= 0 {
		t.Fatalf("uptime = %v, want positive duration", snap.Uptime)
	}
	if snap.Topology != net {
		t.Fatal("aggregate snapshot did not include factory topology")
	}
	if snap.TickCount == 0 {
		t.Fatal("expected non-zero tick count in aggregate snapshot")
	}
	if len(snap.Dispatches) == 0 {
		t.Fatal("expected in-flight dispatch details in aggregate snapshot")
	}
	var consumed int
	for _, dispatch := range snap.Dispatches {
		consumed += len(dispatch.ConsumedTokens)
	}
	if consumed == 0 {
		t.Fatal("expected aggregate snapshot dispatches to include consumed tokens")
	}

	close(executor.release)
	cancelRun()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Run after cancellation: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for factory run to stop")
	}
}

func waitForAggregateSnapshot(
	t *testing.T,
	f factory.Factory,
	match func(*interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) bool,
) *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net] {
	t.Helper()

	deadline := time.Now().Add(time.Second)
	var last *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]
	for time.Now().Before(deadline) {
		snap, err := f.GetEngineStateSnapshot(context.Background())
		if err != nil {
			t.Fatalf("GetEngineStateSnapshot: %v", err)
		}
		last = snap
		if match(snap) {
			return snap
		}
		time.Sleep(10 * time.Millisecond)
	}
	if last == nil {
		t.Fatal("timed out waiting for aggregate snapshot; no snapshot captured")
	}
	t.Fatalf("timed out waiting for aggregate snapshot; last status=%q in_flight=%d tick=%d",
		last.RuntimeStatus,
		last.InFlightCount,
		last.TickCount,
	)
	return nil
}

func runtimeGeneratedEvents(t *testing.T, f factory.Factory) []factoryapi.FactoryEvent {
	t.Helper()
	events, err := f.GetFactoryEvents(context.Background())
	if err != nil {
		t.Fatalf("GetFactoryEvents: %v", err)
	}
	return events
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

func hasFactoryEventType(events []factoryapi.FactoryEvent, eventType factoryapi.FactoryEventType) bool {
	for _, event := range events {
		if event.Type == eventType {
			return true
		}
	}
	return false
}

func countFactoryEventType(events []factoryapi.FactoryEvent, eventType factoryapi.FactoryEventType) int {
	count := 0
	for _, event := range events {
		if event.Type == eventType {
			count++
		}
	}
	return count
}

func safeBoundaryWorkID(dispatch interfaces.WorkDispatch) string {
	for _, token := range workers.WorkDispatchInputTokens(dispatch) {
		if token.Color.DataType == interfaces.DataTypeResource {
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
	dispatch interfaces.WorkDispatch,
	workID string,
	outcome interfaces.WorkOutcome,
	errText string,
	providerFailure *interfaces.ProviderFailureMetadata,
	providerSession *interfaces.ProviderSessionMetadata,
	retryCount string,
) interfaces.WorkResult {
	return interfaces.WorkResult{
		DispatchID:      dispatch.DispatchID,
		TransitionID:    dispatch.TransitionID,
		Outcome:         outcome,
		Output:          "safe boundary output for " + workID,
		Error:           errText,
		ProviderFailure: providerFailure,
		ProviderSession: providerSession,
		Diagnostics: &interfaces.WorkDiagnostics{
			RenderedPrompt: &interfaces.RenderedPromptDiagnostic{
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
			Provider: &interfaces.ProviderDiagnostic{
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
			Command: &interfaces.CommandDiagnostic{
				Command: "echo",
				Stdin:   "raw command stdin must stay private",
				Env: map[string]string{
					"AGENT_FACTORY_AUTH_TOKEN": "raw environment value must stay private",
				},
			},
			Panic: &interfaces.PanicDiagnostic{Stack: "panic stack should not be stored"},
		},
	}
}

func safeBoundaryGeneratedFactory() factoryapi.Factory {
	workstationID := "t-process"
	return factoryapi.Factory{
		WorkTypes: &[]factoryapi.WorkType{{
			Name: "task",
			States: []factoryapi.WorkState{
				{Name: "init", Type: factoryapi.WorkStateTypeINITIAL},
				{Name: "done", Type: factoryapi.WorkStateTypeTERMINAL},
				{Name: "failed", Type: factoryapi.WorkStateTypeFAILED},
			},
		}},
		Workers: &[]factoryapi.Worker{{Name: "mock"}},
		Workstations: &[]factoryapi.Workstation{{
			Id:        &workstationID,
			Name:      "Process",
			Worker:    "mock",
			Inputs:    []factoryapi.WorkstationIO{{WorkType: "task", State: "init"}},
			Outputs:   []factoryapi.WorkstationIO{{WorkType: "task", State: "done"}},
			OnFailure: &factoryapi.WorkstationIO{WorkType: "task", State: "failed"},
		}},
	}
}

func assertThinDispatchResponsesOmitRetiredProviderAttemptFields(t *testing.T, events []factoryapi.FactoryEvent) {
	t.Helper()
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeDispatchResponse {
			continue
		}
		encoded, err := json.Marshal(event)
		if err != nil {
			t.Fatalf("marshal dispatch response %s: %v", event.Id, err)
		}
		var raw map[string]any
		if err := json.Unmarshal(encoded, &raw); err != nil {
			t.Fatalf("unmarshal dispatch response %s: %v", event.Id, err)
		}
		payload, ok := raw["payload"].(map[string]any)
		if !ok {
			t.Fatalf("dispatch response payload = %#v, want object", raw["payload"])
		}
		for _, retired := range []string{"inputs", "providerSession", "diagnostics"} {
			if _, ok := payload[retired]; ok {
				t.Fatalf("dispatch response payload unexpectedly carried %q: %#v", retired, payload)
			}
		}
	}
}

func requestViewForWork(t *testing.T, state interfaces.FactoryWorldState, workID string) factoryapi.FactoryWorldWorkstationRequestView {
	t.Helper()
	slice := factoryboundary.BuildFactoryWorldWorkstationRequestProjectionSlice(state)
	if slice.WorkstationRequestsByDispatchId == nil {
		t.Fatalf("workstation request slice = %#v, want work %q", slice, workID)
	}
	for _, request := range *slice.WorkstationRequestsByDispatchId {
		if request.Request.InputWorkItems == nil {
			continue
		}
		for _, item := range *request.Request.InputWorkItems {
			if item.WorkId == workID {
				return request
			}
		}
	}
	t.Fatalf("missing workstation request for work %q: %#v", workID, slice.WorkstationRequestsByDispatchId)
	return factoryapi.FactoryWorldWorkstationRequestView{}
}

func assertSafeBoundaryRequestView(
	t *testing.T,
	request factoryapi.FactoryWorldWorkstationRequestView,
	sessionID string,
	family string,
	providerFailureType string,
	failureMessage string,
) {
	t.Helper()
	if request.Response == nil {
		t.Fatalf("request response = nil, want response for %#v", request)
	}
	if sessionID == "" {
		if request.Response.ProviderSession != nil {
			t.Fatalf("response provider session = %#v, want nil without inference response", request.Response.ProviderSession)
		}
		if request.Response.Diagnostics != nil {
			t.Fatalf("response diagnostics = %#v, want nil without inference response", request.Response.Diagnostics)
		}
		if request.Response.ResponseMetadata != nil {
			t.Fatalf("response metadata = %#v, want nil without inference response", request.Response.ResponseMetadata)
		}
	} else {
		if request.Request.Provider == nil || *request.Request.Provider != "codex" ||
			request.Request.Model == nil || *request.Request.Model != "gpt-5.4" {
			t.Fatalf("request provider/model = %#v/%#v, want codex/gpt-5.4", request.Request.Provider, request.Request.Model)
		}
		if request.Request.RequestMetadata == nil || (*request.Request.RequestMetadata)["worker_type"] != "mock" {
			t.Fatalf("request metadata = %#v, want worker_type=mock", request.Request.RequestMetadata)
		}
		if request.Request.WorkingDirectory == nil || request.Request.Worktree == nil {
			t.Fatalf("request working directory/worktree = %#v, want allowlisted metadata projected", request.Request)
		}
		if request.Response.ProviderSession == nil || stringValueForRuntimeTest(request.Response.ProviderSession.Id) != sessionID {
			t.Fatalf("response provider session = %#v, want %q", request.Response.ProviderSession, sessionID)
		}
		if request.Response.Diagnostics == nil || request.Response.Diagnostics.Provider == nil || request.Response.Diagnostics.RenderedPrompt == nil {
			t.Fatalf("response diagnostics = %#v, want safe diagnostics", request.Response.Diagnostics)
		}
		if request.Response.ResponseMetadata == nil || (*request.Response.ResponseMetadata)["provider_session_id"] != sessionID {
			t.Fatalf("response metadata = %#v, want provider_session_id=%q", request.Response.ResponseMetadata, sessionID)
		}
	}
	if family == "" && stringValueForRuntimeTest(request.Response.FailureReason) != "" {
		t.Fatalf("failure reason = %q, want empty for successful request", stringValueForRuntimeTest(request.Response.FailureReason))
	}
	if family != "" {
		if stringValueForRuntimeTest(request.Response.FailureReason) != providerFailureType {
			t.Fatalf("failure reason = %q, want %q", stringValueForRuntimeTest(request.Response.FailureReason), providerFailureType)
		}
		if stringValueForRuntimeTest(request.Response.FailureMessage) != failureMessage {
			t.Fatalf("failure message = %q, want %q", stringValueForRuntimeTest(request.Response.FailureMessage), failureMessage)
		}
		if request.Response.Diagnostics != nil {
			if request.Response.Diagnostics.Provider == nil || request.Response.Diagnostics.Provider.ResponseMetadata == nil || (*request.Response.Diagnostics.Provider.ResponseMetadata)["retry_count"] != "2" {
				t.Fatalf("response metadata = %#v, want retry_count=2 for failed request", request.Response.Diagnostics.Provider.ResponseMetadata)
			}
		}
	}
}

func assertNoAuthRemediationText(t *testing.T, body string) {
	t.Helper()
	lowered := strings.ToLower(body)
	for _, forbidden := range []string{"auth_failure", "authentication", "api key", "unauthorized", "forbidden"} {
		if strings.Contains(lowered, forbidden) {
			t.Fatalf("expected operator-facing text to avoid %q, got %q", forbidden, body)
		}
	}
}

func assertSafeBoundaryDoesNotLeakJSON(t *testing.T, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal JSON boundary: %v", err)
	}
	assertSafeBoundaryDoesNotLeak(t, string(data))
}

func assertSafeBoundaryDoesNotLeak(t *testing.T, body string) {
	t.Helper()
	for _, unsafe := range safeBoundaryUnsafeValues() {
		if strings.Contains(body, unsafe) {
			t.Fatalf("safe boundary leaked unsafe value %q: %s", unsafe, body)
		}
	}
}

func safeBoundaryUnsafeValues() []string {
	return []string{
		"raw prompt body must stay private",
		"raw response prompt body must stay private",
		"raw stdin payload must stay private",
		"raw response stdin payload must stay private",
		"raw env secret must stay private",
		"raw rendered system prompt must stay private",
		"raw rendered user message must stay private",
		"raw rendered stdin must stay private",
		"raw rendered environment must stay private",
		"raw command stdin must stay private",
		"raw environment value must stay private",
		"AGENT_FACTORY_AUTH_TOKEN",
		"panic stack should not be stored",
	}
}

func maxEventTick(events []factoryapi.FactoryEvent) int {
	maxTick := 0
	for _, event := range events {
		if event.Context.Tick > maxTick {
			maxTick = event.Context.Tick
		}
	}
	return maxTick
}

func stringValueForRuntimeTest[T ~string](value *T) string {
	if value == nil {
		return ""
	}
	return string(*value)
}

func firstRuntimeTestString(values *[]string) string {
	for _, value := range sliceValueForRuntimeTest(values) {
		if value != "" {
			return value
		}
	}
	return ""
}

func sliceValueForRuntimeTest[T any](values *[]T) []T {
	if values == nil {
		return nil
	}
	return *values
}
