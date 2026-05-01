package providers

import (
	"strings"
	"testing"

	factoryapi "github.com/portpowered/agent-factory/pkg/api/generated"
	"github.com/portpowered/agent-factory/pkg/interfaces"
	"github.com/portpowered/agent-factory/pkg/testutil"
	"github.com/portpowered/agent-factory/pkg/workers"
	"github.com/portpowered/agent-factory/tests/functional/internal/support"
)

func providerErrorCorpusEntryForTest(t *testing.T, name string) workers.ProviderErrorCorpusEntry {
	t.Helper()

	corpus, err := workers.LoadProviderErrorCorpus()
	if err != nil {
		t.Fatalf("workers.LoadProviderErrorCorpus() error = %v", err)
	}
	entry, ok := corpus.Entry(name)
	if !ok {
		t.Fatalf("provider error corpus entry %q not found", name)
	}
	return entry
}

func providerErrorCorpusEntryLabel(entry workers.ProviderErrorCorpusEntry) string {
	if entry.UpstreamSourceCase == "" {
		return entry.Name
	}
	return entry.Name + " [" + entry.UpstreamSourceCase + "]"
}

func providerErrorCorpusLastErrorLine(t *testing.T, entry workers.ProviderErrorCorpusEntry) string {
	t.Helper()

	var lastMatch string
	for _, stream := range []string{entry.Stderr, entry.Stdout} {
		for _, line := range strings.Split(stream, "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "ERROR:") {
				lastMatch = trimmed
			}
		}
	}
	if lastMatch == "" {
		t.Fatalf("provider error corpus entry %q contains no ERROR: line", providerErrorCorpusEntryLabel(entry))
	}
	return lastMatch
}

func providerErrorSmokeWork(name, payload string) testutil.ProviderErrorSmokeWork {
	return testutil.ProviderErrorSmokeWork{
		Name:       name,
		WorkID:     "work-" + name,
		WorkTypeID: "task",
		TraceID:    "trace-" + name,
		Payload:    []byte(payload),
	}
}

func assertProviderCommandMatchesLane(t *testing.T, req workers.CommandRequest, provider workers.ModelProvider, workName, model string) {
	t.Helper()

	if req.Command != string(provider) {
		t.Fatalf("provider command = %q, want %q", req.Command, provider)
	}
	support.AssertArgsContainSequence(t, req.Args, []string{"--model", model})

	switch provider {
	case workers.ModelProviderClaude:
		support.AssertArgsContainSequence(t, req.Args, []string{"--worktree", workName})
		if len(req.Stdin) != 0 {
			t.Fatalf("expected claude prompt in args, got stdin %q", string(req.Stdin))
		}
	case workers.ModelProviderCodex:
		if got := req.Args[len(req.Args)-1]; got != "-" {
			t.Fatalf("expected codex stdin placeholder '-', got %q", got)
		}
		if len(req.Stdin) == 0 {
			t.Fatal("expected codex prompt over stdin")
		}
	default:
		t.Fatalf("unsupported provider %q", provider)
	}
}

func assertDispatchHistoryMatchesWork(t *testing.T, dispatch interfaces.CompletedDispatch, work testutil.ProviderErrorSmokeWork) {
	t.Helper()

	if len(dispatch.ConsumedTokens) == 0 {
		t.Fatal("dispatch consumed no tokens")
	}

	consumed := dispatch.ConsumedTokens[0]
	if consumed.Color.WorkID != work.WorkID {
		t.Fatalf("dispatch consumed WorkID = %q, want %q", consumed.Color.WorkID, work.WorkID)
	}
	if consumed.Color.TraceID != work.TraceID {
		t.Fatalf("dispatch consumed TraceID = %q, want %q", consumed.Color.TraceID, work.TraceID)
	}
	if consumed.Color.Name != work.Name {
		t.Fatalf("dispatch consumed Name = %q, want %q", consumed.Color.Name, work.Name)
	}
}

func configureProviderErrorLoopBreaker(t *testing.T, dir string) {
	t.Helper()

	updateScriptFixtureFactory(t, dir, func(cfg map[string]any) {
		workstations, ok := cfg["workstations"].([]any)
		if !ok {
			t.Fatalf("factory.json workstations = %T, want []any", cfg["workstations"])
		}
		cfg["workstations"] = append(workstations, map[string]any{
			"name": "provider-error-loop-breaker",
			"type": "LOGICAL_MOVE",
			"inputs": []any{
				map[string]any{
					"workType": "task",
					"state":    "init",
				},
			},
			"outputs": []any{
				map[string]any{
					"workType": "task",
					"state":    "failed",
				},
			},
			"guards": []any{
				map[string]any{
					"type":        "visit_count",
					"workstation": "process",
					"maxVisits":   2,
				},
			},
		})
	})
}

func assertCodexHighDemandLoopBreakerOutcome(
	t *testing.T,
	h *testutil.ServiceTestHarness,
	runner *testutil.ProviderCommandRunner,
	outcome testutil.ProviderErrorSmokeOutcome,
	work testutil.ProviderErrorSmokeWork,
) {
	t.Helper()

	h.Assert().
		PlaceTokenCount("task:failed", 1).
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:complete")

	if runner.CallCount() != 6 {
		t.Fatalf("provider command count = %d, want 6 across two exhausted dispatches", runner.CallCount())
	}
	if len(outcome.EngineState.ActiveThrottlePauses) != 0 {
		t.Fatalf("active throttle pauses = %d, want 0", len(outcome.EngineState.ActiveThrottlePauses))
	}

	assertProviderErrorLoopBreakerHistory(t, outcome, work)
	assertRetryableInternalServerRequeueDispatch(t, outcome.Dispatches[0], work, "first")
	assertRetryableInternalServerRequeueDispatch(t, outcome.Dispatches[1], work, "second")
}

func assertProviderErrorLoopBreakerHistory(
	t *testing.T,
	outcome testutil.ProviderErrorSmokeOutcome,
	work testutil.ProviderErrorSmokeWork,
) {
	t.Helper()

	if got := outcome.Token.History.TotalVisits["process"]; got != 2 {
		t.Fatalf("TotalVisits[process] = %d, want 2 after bounded retry exhaustion", got)
	}
	if got := outcome.Token.History.ConsecutiveFailures["process"]; got != 2 {
		t.Fatalf("ConsecutiveFailures[process] = %d, want 2 after bounded retry exhaustion", got)
	}
	if got := len(outcome.Token.History.FailureLog); got != 2 {
		t.Fatalf("FailureLog length = %d, want 2 after bounded retry exhaustion", got)
	}
	if len(outcome.Dispatches) != 3 {
		t.Fatalf("dispatch count = %d, want 2 failed provider dispatches plus 1 guarded loop-breaker dispatch", len(outcome.Dispatches))
	}
	loopBreaker := outcome.Dispatches[len(outcome.Dispatches)-1]
	if loopBreaker.WorkstationName != "provider-error-loop-breaker" {
		t.Fatalf("final workstation = %q, want provider-error-loop-breaker", loopBreaker.WorkstationName)
	}
	if loopBreaker.Outcome != interfaces.OutcomeAccepted {
		t.Fatalf("final loop-breaker outcome = %s, want %s", loopBreaker.Outcome, interfaces.OutcomeAccepted)
	}
	if !dispatchHasOutputMutationToPlace(loopBreaker, work.WorkTypeID+":failed", work.WorkID) {
		t.Fatalf("final loop-breaker mutations = %#v, want route to %s:failed", loopBreaker.OutputMutations, work.WorkTypeID)
	}
}

func assertRetryableInternalServerRequeueDispatch(
	t *testing.T,
	dispatch interfaces.CompletedDispatch,
	work testutil.ProviderErrorSmokeWork,
	dispatchName string,
) {
	t.Helper()

	assertDispatchHistoryMatchesWork(t, dispatch, work)
	assertProviderFailureIsRetryableInternalServer(t, dispatch.ProviderFailure)
	if !dispatchHasOutputMutationToPlace(dispatch, work.WorkTypeID+":init", work.WorkID) {
		t.Fatalf(
			"%s dispatch mutations = %#v, want retryable requeue to %s:init",
			dispatchName,
			dispatch.OutputMutations,
			work.WorkTypeID,
		)
	}
}

func assertContainsAll(t *testing.T, got string, want []string) {
	t.Helper()

	for _, fragment := range want {
		if !strings.Contains(got, fragment) {
			t.Fatalf("expected %q to contain %q", got, fragment)
		}
	}
}

func providerErrorCommandFailure(stderr string) workers.CommandResult {
	return workers.CommandResult{
		ExitCode: 1,
		Stderr:   []byte(stderr),
	}
}

func assertRetryableInternalServerRequeueOutcome(
	t *testing.T,
	runner *testutil.ProviderCommandRunner,
	outcome testutil.ProviderErrorSmokeOutcome,
	work testutil.ProviderErrorSmokeWork,
) {
	t.Helper()

	if runner.CallCount() < 3 {
		t.Fatalf("provider command count = %d, want at least 3", runner.CallCount())
	}
	assertProviderCommandMatchesLane(t, runner.LastRequest(), workers.ModelProviderCodex, work.Name, "gpt-5-codex")

	if len(outcome.Dispatches) != 1 {
		t.Fatalf("dispatch count = %d, want 1 failed dispatch before requeue", len(outcome.Dispatches))
	}
	dispatch := outcome.Dispatches[0]
	if dispatch.Outcome != interfaces.OutcomeFailed {
		t.Fatalf("dispatch outcome = %s, want %s", dispatch.Outcome, interfaces.OutcomeFailed)
	}
	assertDispatchHistoryMatchesWork(t, dispatch, work)
	assertProviderFailureIsRetryableInternalServer(t, dispatch.ProviderFailure)
	if !dispatchHasOutputMutationToPlace(dispatch, work.WorkTypeID+":init", work.WorkID) {
		t.Fatalf("dispatch mutations = %#v, want requeue to %s:init", dispatch.OutputMutations, work.WorkTypeID)
	}
	if len(outcome.EngineState.ActiveThrottlePauses) != 0 {
		t.Fatalf("active throttle pauses = %d, want 0", len(outcome.EngineState.ActiveThrottlePauses))
	}
	if got := outcome.Token.History.TotalVisits["process"]; got != 1 {
		t.Fatalf("TotalVisits[process] = %d, want 1", got)
	}
	if got := outcome.Token.History.ConsecutiveFailures["process"]; got != 1 {
		t.Fatalf("ConsecutiveFailures[process] = %d, want 1", got)
	}
	if got := len(outcome.Token.History.FailureLog); got != 1 {
		t.Fatalf("FailureLog length = %d, want 1", got)
	}
}

func assertProviderFailureIsRetryableInternalServer(t *testing.T, failure *interfaces.ProviderFailureMetadata) {
	t.Helper()

	if failure == nil {
		t.Fatal("ProviderFailure is nil, want normalized internal_server_error metadata")
	}
	if failure.Type != interfaces.ProviderErrorTypeInternalServerError {
		t.Fatalf("provider failure type = %s, want %s", failure.Type, interfaces.ProviderErrorTypeInternalServerError)
	}
	if failure.Family != interfaces.ProviderErrorFamilyRetryable {
		t.Fatalf("provider failure family = %s, want %s", failure.Family, interfaces.ProviderErrorFamilyRetryable)
	}
}

func requireProviderErrorDispatchCompletedEventForWork(
	t *testing.T,
	events []factoryapi.FactoryEvent,
	workID string,
) factoryapi.DispatchResponseEventPayload {
	t.Helper()

	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeDispatchResponse {
			continue
		}
		payload, err := event.Payload.AsDispatchResponseEventPayload()
		if err != nil {
			t.Fatalf("decode DISPATCH_RESPONSE event %q: %v", event.Id, err)
		}
		for _, eventWorkID := range stringSliceValue(event.Context.WorkIds) {
			if eventWorkID == workID {
				return payload
			}
		}
	}

	t.Fatalf("missing DISPATCH_RESPONSE event for work %q", workID)
	return factoryapi.DispatchResponseEventPayload{}
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

func dispatchHasOutputMutationToPlace(dispatch interfaces.CompletedDispatch, placeID, workID string) bool {
	for _, mutation := range dispatch.OutputMutations {
		if mutation.ToPlace != placeID || mutation.Token == nil {
			continue
		}
		if mutation.Token.Color.WorkID == workID {
			return true
		}
	}
	return false
}

func stringSliceValue(values *[]string) []string {
	if values == nil {
		return nil
	}
	return append([]string(nil), (*values)...)
}
