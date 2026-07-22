//go:build functionallong

package guards_batch

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestPartialBatch_SomeTokensFail(t *testing.T) {
	support.SkipLongFunctional(t, "slow partial-batch failure sweep")
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "partial_failure"))

	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "token-a"}`))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "token-b"}`))

	provider := testutil.NewMockProvider(
		workerexecution.InferenceResponse{Content: "Task done. COMPLETE"},
		workerexecution.InferenceResponse{Content: "Task incomplete, no stop token"},
	)

	session := support.RunFactoryToCompletion(t, dir, provider, 10*time.Second)
	assertGuardSessionPlaces(t, session, map[string]int{"task:complete": 1, "task:failed": 1, "task:init": 0})
}

func TestPartialBatch_SomeTokensRejected_RoutedViaRejectionArcs(t *testing.T) {
	support.SkipLongFunctional(t, "slow partial-batch rejection sweep")
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "partial_rejection"))

	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "token-accepted"}`))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "token-rejected"}`))

	provider := testutil.NewMockProvider(
		workerexecution.InferenceResponse{Content: "Work accepted. COMPLETE"},
		workerexecution.InferenceResponse{Content: "Work needs review, no stop token"},
	)

	session := support.RunFactoryToCompletion(t, dir, provider, 10*time.Second)
	assertGuardSessionPlaces(t, session, map[string]int{
		"task:complete": 1, "task:rejected": 1, "task:init": 0, "task:failed": 0,
	})
}

func TestPartialBatch_TemplateResolvesFromTags(t *testing.T) {
	support.SkipLongFunctional(t, "slow partial-batch template sweep")
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "service_parameterized_success"))

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
	runner := testutil.NewProviderCommandRunner(
		platformprocess.CommandResult{Stdout: []byte("Work done. COMPLETE")},
	)

	session := support.RunFactoryToCompletionWithEdges(t, dir, serviceedges.Edges{
		ProviderCommandRunner: runner,
	}, 10*time.Second)
	assertGuardSessionPlaces(t, session, map[string]int{"task:complete": 1, "task:init": 0, "task:failed": 0})

	if runner.CallCount() != 1 {
		t.Fatalf("expected provider runner called 1 time, got %d", runner.CallCount())
	}
	call := runner.LastRequest()
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
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "worktree_passthrough"))

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
	runner := testutil.NewProviderCommandRunner(
		platformprocess.CommandResult{
			Stdout:   []byte("provider stdout before failure"),
			Stderr:   support.ProviderErrorCommandResult(t, "claude_authentication_error").Stderr,
			ExitCode: 1,
		},
	)

	session := support.RunFactoryToCompletionWithEdges(t, dir, serviceedges.Edges{
		ProviderCommandRunner: runner,
	}, 10*time.Second)
	assertGuardSessionPlaces(t, session, map[string]int{"task:failed": 1, "task:init": 0, "task:complete": 0})

	if runner.CallCount() != 1 {
		t.Fatalf("expected provider runner called 1 time, got %d", runner.CallCount())
	}
	call := runner.LastRequest()
	if call.Command != string(modelprovider.ProviderClaude) {
		t.Fatalf("expected command %q, got %q", modelprovider.ProviderClaude, call.Command)
	}
	support.AssertArgsContainSequence(t, call.Args, []string{"--worktree", "provider-exit-failure"})

}

func TestPartialBatch_RetryableProviderFailuresRetryThroughScriptWrapPath(t *testing.T) {
	support.SkipLongFunctional(t, "slow partial-batch retryable-provider sweep")
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "worktree_passthrough"))

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
	runner := testutil.NewProviderCommandRunner(
		support.ProviderErrorCommandResult(t, "claude_internal_server_api_error"),
		support.ProviderErrorCommandResult(t, "claude_internal_server_api_error"),
		platformprocess.CommandResult{Stdout: []byte(`{"type":"result","subtype":"success","is_error":false,"result":"Done. COMPLETE","session_id":"retry-success"}` + "\n")},
	)

	session := support.RunFactoryToCompletionWithEdges(t, dir, serviceedges.Edges{
		ProviderCommandRunner: runner,
	}, 10*time.Second)
	assertGuardSessionPlaces(t, session, map[string]int{"task:complete": 1, "task:init": 0, "task:failed": 0})

	if runner.CallCount() != 3 {
		t.Fatalf("expected provider runner called 3 times, got %d", runner.CallCount())
	}
	call := runner.LastRequest()
	if call.Command != string(modelprovider.ProviderClaude) {
		t.Fatalf("expected command %q, got %q", modelprovider.ProviderClaude, call.Command)
	}
	support.AssertArgsContainSequence(t, call.Args, []string{"--worktree", "provider-retry-success"})
}

func TestPartialBatch_ThrottledProviderFailureWithoutAuthoredGuardEventuallyFails(t *testing.T) {
	support.SkipLongFunctional(t, "slow partial-batch throttled-provider failure sweep")
	dir, runner := throttledProviderFailureFixture(t)
	session, listedWork := support.RunFactoryToCompletionWithEdgesAndWork(t, dir, serviceedges.Edges{
		ProviderCommandRunner: runner,
	}, 5*time.Second)
	assertGuardSessionPlaces(t, session, map[string]int{"task:failed": 1, "task:init": 0, "task:complete": 0})

	if runner.CallCount() != 4 {
		t.Fatalf("expected provider runner called 4 times, got %d", runner.CallCount())
	}

	assertListedFailedWorkID(t, listedWork, "work-provider-throttle-requeue")
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
