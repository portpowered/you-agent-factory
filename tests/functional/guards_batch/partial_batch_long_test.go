//go:build functionallong

package guards_batch

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestPartialBatch_SomeTokensFail(t *testing.T) {
	support.SkipLongFunctional(t, "slow partial-batch failure sweep")
	enterSharedGuardsScenario(t)
	dir := sharedGuardsScenario(t, "partial_failure")

	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "token-a"}`))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "token-b"}`))

	_, listed := runSharedGuardsFactoryToCompletionWithRouteAndWork(t, dir, sharedGuardsRouteConfig{
		provider: sharedGuardsProviderSequence(
			sharedGuardsProviderOutput("Task done. COMPLETE"),
			sharedGuardsProviderOutput("Task incomplete, no stop token"),
		),
	}, 10*time.Second)
	assertGuardSessionPlaces(t, listed, map[string]int{"task:complete": 1, "task:failed": 1, "task:init": 0})
	assertSharedGuardsProviderCalls(t, dir, 2)
}

func TestPartialBatch_SomeTokensRejected_RoutedViaRejectionArcs(t *testing.T) {
	support.SkipLongFunctional(t, "slow partial-batch rejection sweep")
	enterSharedGuardsScenario(t)
	dir := sharedGuardsScenario(t, "partial_rejection")

	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "token-accepted"}`))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "token-rejected"}`))

	_, listed := runSharedGuardsFactoryToCompletionWithRouteAndWork(t, dir, sharedGuardsRouteConfig{
		provider: sharedGuardsProviderSequence(
			sharedGuardsProviderOutput("Work accepted. COMPLETE"),
			sharedGuardsProviderOutput("Work needs review, no stop token"),
		),
	}, 10*time.Second)
	assertGuardSessionPlaces(t, listed, map[string]int{
		"task:complete": 1, "task:rejected": 1, "task:init": 0, "task:failed": 0,
	})
	assertSharedGuardsProviderCalls(t, dir, 2)
}

func TestPartialBatch_TemplateResolvesFromTags(t *testing.T) {
	support.SkipLongFunctional(t, "slow partial-batch template sweep")
	enterSharedGuardsScenario(t)
	dir := sharedGuardsScenario(t, "service_parameterized_success")

	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkTypeID: "task",
		Payload:    []byte(`{"title": "template test"}`),
		Tags:       map[string]string{"branch": "feature-abc"},
	})

	support.WriteAgentConfig(t, dir, "exec-worker", `---
type: MODEL_WORKER
model: gpt-5-codex
modelProvider: codex
stopToken: COMPLETE
---
Process the task input.
`)
	_, listed := runSharedGuardsFactoryToCompletionWithRouteAndWork(t, dir, sharedGuardsRouteConfig{
		provider: sharedGuardsProviderSequence(sharedGuardsProviderOutput("Work done. COMPLETE")),
	}, 10*time.Second)
	assertGuardSessionPlaces(t, listed, map[string]int{"task:complete": 1, "task:init": 0, "task:failed": 0})

	requests := sharedGuardsProviderRequests(t, dir)
	if len(requests) != 1 {
		t.Fatalf("expected provider runner called 1 time, got %d", len(requests))
	}
	call := requests[0]
	if call.Command != string(modelprovider.ProviderCodex) {
		t.Fatalf("expected command %q, got %q", modelprovider.ProviderCodex, call.Command)
	}
	support.AssertArgsContainSequence(t, call.Args, []string{"--model", "gpt-5-codex"})
	if got := call.Args[len(call.Args)-1]; got != "-" {
		t.Fatalf("expected codex stdin placeholder '-', got %q", got)
	}
	if len(call.Stdin) == 0 {
		t.Fatal("expected codex prompt to be streamed over stdin")
	}
}

func TestPartialBatch_ProviderExitFailureRoutesTokenToFailedWithContext(t *testing.T) {
	support.SkipLongFunctional(t, "slow partial-batch provider-exit sweep")
	enterSharedGuardsScenario(t)
	dir := sharedGuardsScenario(t, "worktree_passthrough")

	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		Name:       "provider-exit-failure",
		WorkID:     "work-provider-exit-failure",
		WorkTypeID: "task",
		TraceID:    "trace-provider-exit-failure",
		Payload:    []byte("provider exit failure payload"),
	})

	support.WriteAgentConfig(t, dir, "worker-a", `---
type: MODEL_WORKER
model: test-model
modelProvider: claude
stopToken: COMPLETE
---
Process the input task.
`)
	_, listed := runSharedGuardsFactoryToCompletionWithRouteAndWork(t, dir, sharedGuardsRouteConfig{
		provider: sharedGuardsProviderSequence(sharedGuardsCommandResult(
			support.ProviderErrorCommandResult(t, "claude_authentication_error"),
		)),
	}, 10*time.Second)
	assertGuardSessionPlaces(t, listed, map[string]int{"task:failed": 1, "task:init": 0, "task:complete": 0})

	requests := sharedGuardsProviderRequests(t, dir)
	if len(requests) != 1 {
		t.Fatalf("expected provider runner called 1 time, got %d", len(requests))
	}
	call := requests[0]
	if call.Command != string(modelprovider.ProviderClaude) {
		t.Fatalf("expected command %q, got %q", modelprovider.ProviderClaude, call.Command)
	}
	support.AssertArgsContainSequence(t, call.Args, []string{"--worktree", "provider-exit-failure"})

}

func TestPartialBatch_RetryableProviderFailuresRetryThroughScriptWrapPath(t *testing.T) {
	support.SkipLongFunctional(t, "slow partial-batch retryable-provider sweep")
	enterSharedGuardsScenario(t)
	dir := sharedGuardsScenario(t, "worktree_passthrough")

	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		Name:       "provider-retry-success",
		WorkID:     "work-provider-retry-success",
		WorkTypeID: "task",
		TraceID:    "trace-provider-retry-success",
		Payload:    []byte("provider retry payload"),
	})

	support.WriteAgentConfig(t, dir, "worker-a", `---
type: MODEL_WORKER
model: test-model
modelProvider: claude
stopToken: COMPLETE
---
Process the input task.
`)
	_, listed := runSharedGuardsFactoryToCompletionWithRouteAndWork(t, dir, sharedGuardsRouteConfig{
		provider: sharedGuardsProviderSequence(
			sharedGuardsCommandResult(support.ProviderErrorCommandResult(t, "claude_internal_server_api_error")),
			sharedGuardsCommandResult(support.ProviderErrorCommandResult(t, "claude_internal_server_api_error")),
			sharedGuardsProviderOutput("Done. COMPLETE"),
		),
	}, 10*time.Second)
	assertGuardSessionPlaces(t, listed, map[string]int{"task:complete": 1, "task:init": 0, "task:failed": 0})

	requests := sharedGuardsProviderRequests(t, dir)
	if len(requests) != 3 {
		t.Fatalf("expected provider runner called 3 times, got %d", len(requests))
	}
	call := requests[len(requests)-1]
	if call.Command != string(modelprovider.ProviderClaude) {
		t.Fatalf("expected command %q, got %q", modelprovider.ProviderClaude, call.Command)
	}
	support.AssertArgsContainSequence(t, call.Args, []string{"--worktree", "provider-retry-success"})
}

func TestPartialBatch_ThrottledProviderFailureWithoutAuthoredGuardEventuallyFails(t *testing.T) {
	support.SkipLongFunctional(t, "slow partial-batch throttled-provider failure sweep")
	dir, runner := throttledProviderFailureFixture(t)
	_, listed := support.RunFactoryToCompletionWithEdgesAndWork(t, dir, serviceedges.Edges{
		ProviderCommandRunner: runner,
	}, 5*time.Second)
	assertGuardSessionPlaces(t, listed, map[string]int{"task:failed": 1, "task:init": 0, "task:complete": 0})

	if runner.CallCount() != 4 {
		t.Fatalf("expected provider runner called 4 times, got %d", runner.CallCount())
	}

	assertListedFailedWorkID(t, listed, "work-provider-throttle-requeue")
}

func throttledProviderFailureFixture(t *testing.T) (string, *testutil.ProviderCommandRunner) {
	t.Helper()

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "worktree_passthrough"))

	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		Name:       "provider-throttle-requeue",
		WorkID:     "work-provider-throttle-requeue",
		WorkTypeID: "task",
		TraceID:    "trace-provider-throttle-requeue",
		Payload:    []byte("provider throttle payload"),
	})

	support.WriteAgentConfig(t, dir, "worker-a", `---
type: MODEL_WORKER
model: test-model
modelProvider: claude
stopToken: COMPLETE
---
Process the input task.
`)
	runner := testutil.NewProviderCommandRunner(
		support.RepeatedProviderErrorCommandResults(t, "claude_rate_limit_error", 3)...,
	)

	return dir, runner
}

func assertListedFailedWorkID(t *testing.T, response factoryapi.ListWorkResponse, wantWorkID string) {
	t.Helper()
	for _, item := range response.Results {
		if item.State != nil && item.State.Name == "failed" && item.WorkId != nil && *item.WorkId == wantWorkID {
			return
		}
	}
	t.Fatalf("listed Work missing failed work ID %q", wantWorkID)
}
