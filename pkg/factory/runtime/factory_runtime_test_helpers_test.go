package runtime

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	factoryboundary "github.com/portpowered/infinite-you/pkg/api"
	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/interfaces"

	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/factory/requests"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/logging"
	"github.com/portpowered/infinite-you/pkg/petri"
	"github.com/portpowered/infinite-you/pkg/testutil/runtimefixtures"
	"github.com/portpowered/infinite-you/pkg/workers"
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
	return f.SubmitWorkRequest(ctx, requests.WorkRequestFromSubmitRequests(reqs))
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

func newPassingInlineRuntime(t *testing.T) factory.Factory {
	t.Helper()
	f, err := New(
		factory.WithNet(buildSimpleNet()),
		factory.WithInlineDispatch(),
		factory.WithWorkerExecutor("mock", &passExecutor{}),
		factory.WithLogger(logging.NoopLogger{}),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return f
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
	return events
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
	request factoryapi.FactoryWorldWorkstationRequestView,
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
	if family == "" && stringValueForRuntimeTest(request.Response.FailureReason) != "" {
		t.Fatalf("failure reason = %q, want empty for successful request", stringValueForRuntimeTest(request.Response.FailureReason))
	}
	if family == "" {
		return
	}
	if stringValueForRuntimeTest(request.Response.FailureReason) != providerFailureType {
		t.Fatalf("failure reason = %q, want %q", stringValueForRuntimeTest(request.Response.FailureReason), providerFailureType)
	}
	if stringValueForRuntimeTest(request.Response.FailureMessage) != failureMessage {
		t.Fatalf("failure message = %q, want %q", stringValueForRuntimeTest(request.Response.FailureMessage), failureMessage)
	}
	if ok {
		metadata := attempt.Diagnostics.Provider.ResponseMetadata
		if metadata == nil || metadata["retry_count"] != "2" {
			t.Fatalf("response metadata = %#v, want retry_count=2 for failed request", metadata)
		}
	}
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
