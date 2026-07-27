// backendsizecheck:ignore-file service-ownership migration preserves this consolidated surface until a dedicated responsibility split removes the exemption.
// pkgmaintcheck:ignore-file-lines service-ownership migration preserves this consolidated file; split responsibilities and remove this exemption.
package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil/recordingfixtures"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"

	"github.com/portpowered/infinite-you/internal/testutil/runtimefixtures"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factory_context "github.com/portpowered/infinite-you/pkg/services/factory_runtime/context"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/scheduler"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/state"
	factorytoken "github.com/portpowered/infinite-you/pkg/services/factory_runtime/token"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

type testFactoryOption func(*testFactoryConfig)

type testFactoryConfig struct {
	net                       *state.Net
	scheduler                 scheduler.Scheduler
	workerExecutors           map[string]workers.WorkerExecutor
	workerService             workstationExecutionBoundary
	runtimeConfig             interfaces.RuntimeDefinitionLookup
	workflowContext           *factory_context.FactoryContext
	runtimeMode               interfaces.RuntimeMode
	logger                    logging.Logger
	clock                     factory.Clock
	inlineDispatch            bool
	eventHistory              recordings.RuntimeEventLedger
	submissionHooks           []factory.SubmissionHook
	dispatchRecorder          recordings.DispatchRecorder
	completionRecorder        factory.CompletionRecorder
	petriMutationRecorder     factory.PetriMutationRecorder
	completionDeliveryPlanner factory.CompletionDeliveryPlanner
}

func newTestFactory(opts ...testFactoryOption) (factory.Factory, error) {
	cfg := &testFactoryConfig{runtimeMode: interfaces.RuntimeModeBatch, clock: platformclock.Real{}}
	for _, opt := range opts {
		opt(cfg)
	}
	if cfg.eventHistory == nil && cfg.net != nil {
		cfg.eventHistory = &recordingfixtures.ScriptedRuntimeLedger{}
	}
	var identity atomic.Int64
	workerService := cfg.workerService
	if workerService == nil {
		workerService = &testWorkstationBoundary{}
	}
	return New(
		cfg.net, cfg.scheduler, cfg.workerExecutors, workerService, cfg.runtimeConfig,
		cfg.workflowContext, cfg.runtimeMode, cfg.logger, cfg.clock,
		cfg.inlineDispatch, cfg.eventHistory, nil,
		nil, nil, cfg.submissionHooks,
		cfg.dispatchRecorder, cfg.completionRecorder, cfg.petriMutationRecorder,
		cfg.completionDeliveryPlanner,
		nil,
		nil,
		interfaces.WorkPropagationPolicyFunc(func(
			*interfaces.FactoryWorkstationConfig,
		) interfaces.WorkPropagationMode {
			return interfaces.WorkPropagationModeOutputAsPayload
		}),
		func() string { return fmt.Sprintf("work-request-test-id-%d", identity.Add(1)) },
		func() string { return fmt.Sprintf("runtime-test-id-%d", identity.Add(1)) },
	)
}

func newTestFactoryWithScriptedLedger(
	opts ...testFactoryOption,
) (factory.Factory, *recordingfixtures.ScriptedRuntimeLedger, error) {
	ledger := &recordingfixtures.ScriptedRuntimeLedger{}
	opts = append(opts, withFactoryEventHistory(ledger))
	runtime, err := newTestFactory(opts...)
	return runtime, ledger, err
}

func withNet(net *state.Net) testFactoryOption {
	return func(cfg *testFactoryConfig) { cfg.net = net }
}

func withScheduler(value scheduler.Scheduler) testFactoryOption {
	return func(cfg *testFactoryConfig) { cfg.scheduler = value }
}

func withWorkerExecutor(workerType string, executor workers.WorkerExecutor) testFactoryOption {
	return func(cfg *testFactoryConfig) {
		if cfg.workerExecutors == nil {
			cfg.workerExecutors = make(map[string]workers.WorkerExecutor)
		}
		cfg.workerExecutors[workerType] = executor
	}
}

func withWorkerService(service workstationExecutionBoundary) testFactoryOption {
	return func(cfg *testFactoryConfig) { cfg.workerService = service }
}

type testWorkstationBoundary struct {
	routes map[string]workers.WorkstationRequestExecutor
}

func (b *testWorkstationBoundary) StartWorkstationPool(
	_ context.Context,
	request workers.WorkstationPoolStartRequest,
) (workers.WorkstationPoolStartResult, error) {
	b.routes = make(map[string]workers.WorkstationRequestExecutor, len(request.Bindings))
	for _, binding := range request.Bindings {
		b.routes[binding.RoleName] = binding.Executor
	}
	return workers.WorkstationPoolStartResult{
		Outcome: workers.WorkstationPoolLifecycleOutcomeStarted,
	}, nil
}

func (*testWorkstationBoundary) StopWorkstationPool(
	context.Context,
) (workers.WorkstationPoolStopResult, error) {
	return workers.WorkstationPoolStopResult{
		Outcome: workers.WorkstationPoolLifecycleOutcomeStopped,
	}, nil
}

func (b *testWorkstationBoundary) DispatchWorkstation(
	ctx context.Context,
	request workers.WorkstationDispatchRequest,
) (workers.WorkstationDispatchResult, error) {
	executor := b.routes[request.WorkstationName]
	if executor == nil {
		result := workerexecution.WorkResult{
			DispatchID:   request.Execution.Dispatch.DispatchID,
			TransitionID: request.Execution.Dispatch.TransitionID,
			Outcome:      workerexecution.OutcomeFailed,
			Error:        fmt.Sprintf("no executor registered for worker type %q", request.Execution.WorkerType),
		}
		return workers.WorkstationDispatchResult{
			DispatchID:      result.DispatchID,
			WorkstationName: request.WorkstationName,
			TerminalOutcome: workers.WorkstationDispatchTerminalOutcomeFailed,
			Result:          result,
		}, nil
	}
	result, err := executor.Execute(ctx, request.Execution)
	terminal := workers.WorkstationDispatchTerminalOutcomeCompleted
	if err != nil || result.Outcome == workerexecution.OutcomeFailed {
		terminal = workers.WorkstationDispatchTerminalOutcomeFailed
	}
	return workers.WorkstationDispatchResult{
		DispatchID:      request.Execution.Dispatch.DispatchID,
		WorkstationName: request.WorkstationName,
		TerminalOutcome: terminal,
		Result:          result,
	}, err
}

func (*testWorkstationBoundary) CancelWorkstationDispatch(
	_ context.Context,
	request workers.WorkstationDispatchCancelRequest,
) (workers.WorkstationDispatchCancelResult, error) {
	return workers.WorkstationDispatchCancelResult{
		DispatchID: request.DispatchID,
		Outcome:    workers.WorkstationDispatchCancelOutcomeCanceled,
	}, nil
}

func withRuntimeConfig(value interfaces.RuntimeDefinitionLookup) testFactoryOption {
	return func(cfg *testFactoryConfig) { cfg.runtimeConfig = value }
}

func withWorkflowContext(value *factory_context.FactoryContext) testFactoryOption {
	return func(cfg *testFactoryConfig) { cfg.workflowContext = value }
}

func withServiceMode() testFactoryOption {
	return func(cfg *testFactoryConfig) { cfg.runtimeMode = interfaces.RuntimeModeService }
}

func withLogger(value logging.Logger) testFactoryOption {
	return func(cfg *testFactoryConfig) { cfg.logger = value }
}

func withClock(value factory.Clock) testFactoryOption {
	return func(cfg *testFactoryConfig) { cfg.clock = value }
}

func withInlineDispatch() testFactoryOption {
	return func(cfg *testFactoryConfig) { cfg.inlineDispatch = true }
}

func withFactoryEventHistory(value recordings.RuntimeEventLedger) testFactoryOption {
	return func(cfg *testFactoryConfig) { cfg.eventHistory = value }
}

func withSubmissionHook(value factory.SubmissionHook) testFactoryOption {
	return func(cfg *testFactoryConfig) {
		if value != nil {
			cfg.submissionHooks = append(cfg.submissionHooks, value)
		}
	}
}

func withDispatchRecorder(value recordings.DispatchRecorder) testFactoryOption {
	return func(cfg *testFactoryConfig) { cfg.dispatchRecorder = value }
}

func withCompletionRecorder(value factory.CompletionRecorder) testFactoryOption {
	return func(cfg *testFactoryConfig) { cfg.completionRecorder = value }
}

func withPetriMutationRecorder(value factory.PetriMutationRecorder) testFactoryOption {
	return func(cfg *testFactoryConfig) { cfg.petriMutationRecorder = value }
}

func withCompletionDeliveryPlanner(value factory.CompletionDeliveryPlanner) testFactoryOption {
	return func(cfg *testFactoryConfig) { cfg.completionDeliveryPlanner = value }
}

type passExecutor struct{}

func (e *passExecutor) Execute(_ context.Context, dispatch work.WorkDispatch) (workerexecution.WorkResult, error) {
	return workerexecution.WorkResult{
		DispatchID:   dispatch.DispatchID,
		TransitionID: dispatch.TransitionID,
		Outcome:      workerexecution.OutcomeAccepted,
		Output:       "done",
	}, nil
}

type acceptedNoOutputExecutor struct{}

func (*acceptedNoOutputExecutor) Execute(
	_ context.Context,
	dispatch work.WorkDispatch,
) (workerexecution.WorkResult, error) {
	return workerexecution.WorkResult{
		DispatchID:   dispatch.DispatchID,
		TransitionID: dispatch.TransitionID,
		Outcome:      workerexecution.OutcomeAccepted,
	}, nil
}

type blockingExecutor struct {
	started chan struct{}
	release chan struct{}
}

func (e *blockingExecutor) Execute(_ context.Context, dispatch work.WorkDispatch) (workerexecution.WorkResult, error) {
	close(e.started)
	<-e.release
	return workerexecution.WorkResult{
		DispatchID:   dispatch.DispatchID,
		TransitionID: dispatch.TransitionID,
		Outcome:      workerexecution.OutcomeAccepted,
		Output:       "done",
	}, nil
}

type safeDiagnosticsBoundaryExecutor struct{}

func (e *safeDiagnosticsBoundaryExecutor) Execute(_ context.Context, dispatch work.WorkDispatch) (workerexecution.WorkResult, error) {
	workID := safeBoundaryWorkID(dispatch)
	switch workID {
	case "work-safe-success":
		return safeBoundaryResult(dispatch, workID, workerexecution.OutcomeAccepted, "", nil, &workerexecution.ProviderSessionMetadata{
			Provider: "codex",
			Kind:     "response_id",
			ID:       "resp-safe-success",
		}, "1"), nil
	case "work-safe-failure":
		return safeBoundaryResult(dispatch, workID, workerexecution.OutcomeFailed, "provider timed out", &workerexecution.WorkFailureMetadata{
			Family: workerexecution.WorkFailureFamilyRetryable,
			Type:   workerexecution.WorkFailureTypeTimeout,
		}, &workerexecution.ProviderSessionMetadata{
			Provider: "codex",
			Kind:     "session_id",
			ID:       "sess-safe-failure",
		}, "2"), nil
	case "work-safe-windows-process-failure":
		return safeBoundaryResult(dispatch, workID, workerexecution.OutcomeFailed, "provider error: internal_server_error: codex exited with code 4294967295: stderr: OpenAI Codex v0.118.0 (research preview)", &workerexecution.WorkFailureMetadata{
			Family: workerexecution.WorkFailureFamilyRetryable,
			Type:   workerexecution.WorkFailureTypeInternalServerError,
		}, &workerexecution.ProviderSessionMetadata{
			Provider: "codex",
			Kind:     "session_id",
			ID:       "sess-safe-windows-4294967295",
		}, "2"), nil
	default:
		return workerexecution.WorkResult{
			DispatchID:   dispatch.DispatchID,
			TransitionID: dispatch.TransitionID,
			Outcome:      workerexecution.OutcomeAccepted,
			Output:       "done",
		}, nil
	}
}

type fixedCompletionDeliveryPlanner struct {
	tick          int
	plannedResult workerexecution.WorkResult
}

func (p fixedCompletionDeliveryPlanner) DeliveryTickForDispatch(work.WorkDispatch) (int, bool, error) {
	return p.tick, true, nil
}

func (p fixedCompletionDeliveryPlanner) PlannedResultForDispatch(dispatch work.WorkDispatch) (workerexecution.WorkResult, bool, error) {
	if p.plannedResult.DispatchID == "" && p.plannedResult.TransitionID == "" && p.plannedResult.Output == "" && p.plannedResult.Outcome == "" {
		return workerexecution.WorkResult{}, false, nil
	}
	result := p.plannedResult
	result.DispatchID = dispatch.DispatchID
	result.TransitionID = dispatch.TransitionID
	return result, true, nil
}

func submitWorkRequests(ctx context.Context, f factory.Factory, reqs []work.SubmitRequest) (work.WorkRequestSubmitResult, error) {
	return f.SubmitWorkRequest(ctx, work.WorkRequestFromSubmitRequests(reqs))
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
	batch   work.GeneratedSubmissionBatch
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
		GeneratedBatches: []work.GeneratedSubmissionBatch{h.batch},
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

func newPassingInlineRuntime(t *testing.T) factory.Factory {
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
) (factory.Factory, *recordingfixtures.ScriptedRuntimeLedger) {
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

func tickableFactory(t *testing.T, f factory.Factory) TickableFactory {
	t.Helper()
	tickable, ok := f.(TickableFactory)
	if !ok {
		t.Fatal("factory is not tickable")
	}
	return tickable
}

func runtimeGeneratedEvents(t *testing.T, f factory.Factory) []factoryapi.FactoryEvent {
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

func hasFactoryEventType(events []factoryapi.FactoryEvent, eventType factoryapi.FactoryEventType) bool {
	for _, event := range events {
		if event.Type == eventType {
			return true
		}
	}
	return false
}

// runtimePreWorkEventCount is the number of canonical startup events emitted
// before the first work lifecycle event (RUN_REQUEST, INITIAL_STRUCTURE_REQUEST,
// SESSION_STARTED).
const runtimePreWorkEventCount = 3

func runtimeStartupEventTypes() []factoryapi.FactoryEventType {
	return []factoryapi.FactoryEventType{
		factoryapi.FactoryEventTypeRunRequest,
		factoryapi.FactoryEventTypeInitialStructureRequest,
		factoryapi.FactoryEventTypeSessionStarted,
	}
}

func runtimeEventIndex(afterStartup int) int {
	return runtimePreWorkEventCount + afterStartup
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

func requestViewForWork(t *testing.T, state interfaces.FactoryWorldState, workID string) recordings.WorkstationFactoryWorldWorkstationRequestView {
	t.Helper()
	slice := recordings.BuildFactoryWorldWorkstationRequestProjectionSlice(state)
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
	return recordings.WorkstationFactoryWorldWorkstationRequestView{}
}

func inferenceAttemptForWork(
	t *testing.T,
	state interfaces.FactoryWorldState,
	workID string,
) (interfaces.FactoryWorldInferenceAttempt, bool) {
	t.Helper()
	request := requestViewForWork(t, state, workID)
	attempts := state.InferenceAttemptsByDispatchID[request.DispatchId]
	if len(attempts) == 0 {
		return interfaces.FactoryWorldInferenceAttempt{}, false
	}
	if len(attempts) != 1 {
		t.Fatalf("inference attempts for dispatch %q = %#v, want one attempt for work %q", request.DispatchId, attempts, workID)
	}
	for _, attempt := range attempts {
		return attempt, true
	}
	t.Fatalf("missing inference attempt for dispatch %q", request.DispatchId)
	return interfaces.FactoryWorldInferenceAttempt{}, false
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

func assertRuntimeSafeBoundaryOmittedInferenceFields(t *testing.T, payload any, keys []string) {
	t.Helper()
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Marshal(%T): %v", payload, err)
	}
	var raw map[string]any
	if err := json.Unmarshal(encoded, &raw); err != nil {
		t.Fatalf("Unmarshal(%T): %v", payload, err)
	}
	for _, key := range keys {
		if _, ok := raw[key]; ok {
			t.Fatalf("%T unexpectedly carried retired inference-owned field %q: %#v", payload, key, raw[key])
		}
	}
}

// pkgmaintcheck:ignore-cyclomatic-complexity this helper intentionally validates the full safe boundary request view contract in one place.
func assertSafeBoundaryRequestView(
	t *testing.T,
	state interfaces.FactoryWorldState,
	request recordings.WorkstationFactoryWorldWorkstationRequestView,
	workID string,
	sessionID string,
	family string,
	providerFailureType string,
	failureMessage string,
) {
	t.Helper()
	if request.Response == nil {
		t.Fatalf("request response = nil, want response for %#v", request)
	}
	assertRuntimeSafeBoundaryOmittedInferenceFields(t, request.Request, []string{"provider", "model", "requestMetadata", "workingDirectory", "worktree"})
	assertRuntimeSafeBoundaryOmittedInferenceFields(t, request.Response, []string{"providerSession", "diagnostics", "responseMetadata"})

	attempt, ok := inferenceAttemptForWork(t, state, workID)
	if ok {
		if attempt.ProviderSession == nil || attempt.ProviderSession.ID != sessionID {
			t.Fatalf("inference attempt provider session = %#v, want %q", attempt.ProviderSession, sessionID)
		}
		if attempt.Diagnostics == nil || attempt.Diagnostics.Provider == nil || attempt.Diagnostics.RenderedPrompt == nil {
			t.Fatalf("inference attempt diagnostics = %#v, want safe diagnostics", attempt.Diagnostics)
		}
		if attempt.Diagnostics.Provider.Provider != "codex" || attempt.Diagnostics.Provider.Model != "gpt-5.4" {
			t.Fatalf("inference attempt provider/model = %#v, want codex/gpt-5.4", attempt.Diagnostics.Provider)
		}
		if attempt.Diagnostics.Provider.RequestMetadata == nil || attempt.Diagnostics.Provider.RequestMetadata["worker_type"] != "mock" {
			t.Fatalf("inference attempt request metadata = %#v, want worker_type=mock", attempt.Diagnostics.Provider.RequestMetadata)
		}
		if attempt.Diagnostics.Provider.ResponseMetadata == nil || attempt.Diagnostics.Provider.ResponseMetadata["provider_session_id"] != sessionID {
			t.Fatalf("inference attempt response metadata = %#v, want provider_session_id=%q", attempt.Diagnostics.Provider.ResponseMetadata, sessionID)
		}
	}
	if family == "" && request.Response.FailureDetail != nil {
		t.Fatalf("failure detail = %#v, want empty for successful request", request.Response.FailureDetail)
	}
	if family == "" {
		return
	}
	if request.Response.FailureDetail == nil || string(request.Response.FailureDetail.Reason) != providerFailureType {
		t.Fatalf("failure detail = %#v, want reason %q", request.Response.FailureDetail, providerFailureType)
	}
	if request.Response.FailureDetail.Message != failureMessage {
		t.Fatalf("failure message = %q, want %q", request.Response.FailureDetail.Message, failureMessage)
	}
	if ok {
		metadata := attempt.Diagnostics.Provider.ResponseMetadata
		if metadata == nil || metadata["retry_count"] != "2" {
			t.Fatalf("response metadata = %#v, want retry_count=2 for failed request", metadata)
		}
	}
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
	outcome workerexecution.WorkOutcome,
	errText string,
	providerFailure *workerexecution.WorkFailureMetadata,
	providerSession *workerexecution.ProviderSessionMetadata,
	retryCount string,
) workerexecution.WorkResult {
	return workerexecution.WorkResult{
		DispatchID:      dispatch.DispatchID,
		TransitionID:    dispatch.TransitionID,
		Outcome:         outcome,
		Output:          "safe boundary output for " + workID,
		Error:           errText,
		FailureMetadata: providerFailure,
		ProviderSession: providerSession,
		Diagnostics: &workerexecution.WorkDiagnostics{
			RenderedPrompt: &workerexecution.RenderedPromptDiagnostic{
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
			Provider: &workerexecution.ProviderDiagnostic{
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
			Command: &workerexecution.CommandDiagnostic{
				Command: "echo",
				Stdin:   "raw command stdin must stay private",
				Env: map[string]string{
					"AGENT_FACTORY_AUTH_TOKEN": "raw environment value must stay private",
				},
			},
			Panic: &workerexecution.PanicDiagnostic{Stack: "panic stack should not be stored"},
		},
	}
}

func safeBoundaryGeneratedFactory() factoryapi.Factory {
	workstationID := "t-process"
	return factoryapi.Factory{
		Name: "safe-boundary-factory",
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
			Outputs:   &[]factoryapi.WorkstationIO{{WorkType: "task", State: "done"}},
			OnFailure: &[]factoryapi.WorkstationIO{{WorkType: "task", State: "failed"}},
		}},
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

type serviceModeRunHarness struct {
	t       *testing.T
	Factory factory.Factory
	cancel  context.CancelFunc
	errCh   chan error
}

func startServiceModeRunHarness(t *testing.T, opts ...testFactoryOption) *serviceModeRunHarness {
	t.Helper()

	f, err := newTestFactory(opts...)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- f.Run(ctx)
	}()

	waitForFactoryState(t, f, interfaces.FactoryStateRunning, time.Second)
	return &serviceModeRunHarness{t: t, Factory: f, cancel: cancel, errCh: errCh}
}

func (h *serviceModeRunHarness) pauseAndWait() {
	h.t.Helper()
	if err := h.Factory.Pause(context.Background()); err != nil {
		h.t.Fatalf("Pause: %v", err)
	}
	waitForFactoryState(h.t, h.Factory, interfaces.FactoryStatePaused, time.Second)
}

func (h *serviceModeRunHarness) resumeAndWait() {
	h.t.Helper()
	if err := h.Factory.Resume(context.Background()); err != nil {
		h.t.Fatalf("Resume: %v", err)
	}
	waitForFactoryState(h.t, h.Factory, interfaces.FactoryStateRunning, time.Second)
}

func (h *serviceModeRunHarness) stop() {
	h.t.Helper()
	h.cancel()
	select {
	case err := <-h.errCh:
		if err != nil {
			h.t.Fatalf("Run after cancellation: %v", err)
		}
	case <-time.After(time.Second):
		h.t.Fatal("timed out waiting for service-mode runtime to stop after cancellation")
	}
}

func submitPausedBufferTask(t *testing.T, f factory.Factory, requestID, traceID string) {
	t.Helper()
	result, err := submitWorkRequests(context.Background(), f, []work.SubmitRequest{{
		RequestID:  requestID,
		WorkTypeID: "task",
		TraceID:    traceID,
	}})
	if err != nil {
		t.Fatalf("SubmitWorkRequest while paused: %v", err)
	}
	if !result.Accepted {
		t.Fatalf("submit accepted = false, want true")
	}
}

func waitForBlockingWorkerStart(t *testing.T, executor *blockingExecutor, errCh <-chan error) {
	t.Helper()
	select {
	case <-executor.started:
	case err := <-errCh:
		t.Fatalf("Run returned before worker started: %v", err)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for worker to start")
	}
}

func pollPausedSnapshot(
	t *testing.T,
	f factory.Factory,
	duration time.Duration,
	assertFn func(*interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]),
) {
	t.Helper()
	deadline := time.Now().Add(duration)
	for time.Now().Before(deadline) {
		snap, err := f.GetEngineStateSnapshot(context.Background())
		if err != nil {
			t.Fatalf("GetEngineStateSnapshot while paused: %v", err)
		}
		assertFn(snap)
		time.Sleep(20 * time.Millisecond)
	}
}

func assertPausedSubmissionNotApplied(t *testing.T, f factory.Factory) {
	t.Helper()
	pollPausedSnapshot(t, f, 300*time.Millisecond, func(snap *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) {
		if snap.FactoryState != string(interfaces.FactoryStatePaused) {
			t.Fatalf("factory state = %q, want PAUSED", snap.FactoryState)
		}
		if hasWorkAtPlace(snap, "task:init") || hasWorkAtPlace(snap, "task:done") {
			t.Fatalf("paused submission applied to marking = %#v", snap.Marking.Tokens)
		}
		if snap.InFlightCount > 0 || len(snap.Dispatches) > 0 {
			t.Fatalf("running dispatches while paused inFlight=%d dispatches=%d", snap.InFlightCount, len(snap.Dispatches))
		}
	})
}

func observeNextBufferedResult(t *testing.T, f factory.Factory) <-chan struct{} {
	t.Helper()
	impl, ok := f.(*factoryImpl)
	if !ok || impl.dispatchFlow == nil {
		t.Fatal("test factory does not expose a canonical dispatch result hook")
	}
	written := make(chan struct{})
	var once sync.Once
	notify := func() { once.Do(func() { close(written) }) }
	impl.dispatchFlow.SetOnBufferedResult(notify)
	return written
}

func waitForBufferedResult(t *testing.T, written <-chan struct{}) {
	t.Helper()
	select {
	case <-written:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for worker result to enter runtime buffer")
	}
}

func assertPausedWorkerResultBuffered(t *testing.T, f factory.Factory) {
	t.Helper()
	snap, err := f.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot while paused: %v", err)
	}
	if snap.FactoryState != string(interfaces.FactoryStatePaused) {
		t.Fatalf("factory state = %q, want PAUSED", snap.FactoryState)
	}
	if hasWorkAtPlace(snap, "task:done") {
		t.Fatal("worker result applied while paused")
	}
	if snap.InFlightCount == 0 {
		t.Fatalf("dispatch completed while paused inFlight=%d", snap.InFlightCount)
	}
}

func assertPausedSubmissionNotDone(t *testing.T, f factory.Factory) {
	t.Helper()
	snap, err := f.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot while paused: %v", err)
	}
	if hasWorkAtPlace(snap, "task:done") {
		t.Fatal("buffered submission applied while paused")
	}
}

func assertPausedWorkerResultNotDone(t *testing.T, f factory.Factory) {
	t.Helper()
	assertPausedWorkerResultBuffered(t, f)
}

func assertTaskDoneOnce(t *testing.T, f factory.Factory) {
	t.Helper()
	snap, err := f.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot after resume: %v", err)
	}
	if count := countTokensAtPlace(snap, "task:done"); count != 1 {
		t.Fatalf("task:done token count = %d, want 1", count)
	}
}

func assertNoInFlightDispatches(t *testing.T, f factory.Factory) {
	t.Helper()
	snap, err := f.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot after resume: %v", err)
	}
	if snap.InFlightCount != 0 {
		t.Fatalf("inFlightCount = %d, want 0 after resume", snap.InFlightCount)
	}
}

func waitForFactoryState(t *testing.T, f factory.Factory, want interfaces.FactoryState, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		snap, err := f.GetEngineStateSnapshot(context.Background())
		if err != nil {
			t.Fatalf("GetEngineStateSnapshot: %v", err)
		}
		if snap.FactoryState == string(want) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	snap, err := f.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	t.Fatalf("factory state = %q, want %q before timeout", snap.FactoryState, want)
}

func waitForWorkAtPlace(t *testing.T, f factory.Factory, placeID string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		snap, err := f.GetEngineStateSnapshot(context.Background())
		if err != nil {
			t.Fatalf("GetEngineStateSnapshot: %v", err)
		}
		if hasWorkAtPlace(snap, placeID) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for work at %s", placeID)
}

func hasWorkAtPlace(snap *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], placeID string) bool {
	for _, token := range snap.Marking.Tokens {
		if token.PlaceID == placeID {
			return true
		}
	}
	return false
}

func countTokensAtPlace(snap *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], placeID string) int {
	count := 0
	for _, token := range snap.Marking.Tokens {
		if token.PlaceID == placeID {
			count++
		}
	}
	return count
}

func submitTaskWithWorkID(t *testing.T, f factory.Factory, workID, traceID string) {
	t.Helper()
	if _, err := submitWorkRequests(context.Background(), f, []work.SubmitRequest{{
		WorkID:     workID,
		WorkTypeID: "task",
		TraceID:    traceID,
	}}); err != nil {
		t.Fatalf("SubmitWorkRequest for %q: %v", workID, err)
	}
}

func assertWorkNotAtDonePlace(t *testing.T, f factory.Factory, workID string) {
	t.Helper()
	snap, err := f.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	if markingContainsWorkAtPlace(&snap.Marking, workID, "task:done") {
		t.Fatalf("marking = %#v, want work %q to remain unprocessed before resume", snap.Marking.Tokens, workID)
	}
}

func assertWorksNotAtDonePlace(t *testing.T, f factory.Factory, workIDs []string) {
	t.Helper()
	for _, workID := range workIDs {
		assertWorkNotAtDonePlace(t, f, workID)
	}
}

func waitForWorkDoneAfterResume(t *testing.T, f factory.Factory, workID string) {
	t.Helper()
	waitForAggregateSnapshotWithTimeout(t, f, 2*time.Second, func(snap *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) bool {
		return markingContainsWorkAtPlace(&snap.Marking, workID, "task:done")
	})
}

func waitForQuiescentWorksAtDone(t *testing.T, f factory.Factory, workIDs []string) *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net] {
	t.Helper()
	return waitForAggregateSnapshotWithTimeout(t, f, 5*time.Second, func(snap *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) bool {
		return allWorksAtDonePlace(&snap.Marking, workIDs) && snap.InFlightCount == 0
	})
}

func waitForAggregateSnapshotWithTimeout(
	t *testing.T,
	f factory.Factory,
	timeout time.Duration,
	match func(*interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) bool,
) *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net] {
	t.Helper()

	deadline := time.Now().Add(timeout)
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
	t.Fatalf("timed out waiting for aggregate snapshot after %s; last status=%q in_flight=%d tick=%d",
		timeout,
		last.RuntimeStatus,
		last.InFlightCount,
		last.TickCount,
	)
	return nil
}

func assertDispatchOrder(t *testing.T, history []interfaces.CompletedDispatch, wantWorkIDs []string) {
	t.Helper()
	gotOrder := workIDsFromDispatchHistory(history)
	for i, wantWorkID := range wantWorkIDs {
		if gotOrder[i] != wantWorkID {
			t.Fatalf("dispatch history order = %v, want %v", gotOrder, wantWorkIDs)
		}
	}
}

func resumeFactory(t *testing.T, f factory.Factory) {
	t.Helper()
	if err := f.Resume(context.Background()); err != nil {
		t.Fatalf("Resume: %v", err)
	}
}
