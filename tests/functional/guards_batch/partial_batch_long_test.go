//go:build functionallong

package guards_batch

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil"
	"github.com/portpowered/infinite-you/pkg/workers"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestPartialBatch_SomeTokensFail(t *testing.T) {
	support.SkipLongFunctional(t, "slow partial-batch failure sweep")
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "partial_failure"))

	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "token-a"}`))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "token-b"}`))

	provider := testutil.NewMockProvider(
		interfaces.InferenceResponse{Content: "Task done. COMPLETE"},
		interfaces.InferenceResponse{Content: "Task incomplete, no stop token"},
	)

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	h.RunUntilComplete(t, 10*time.Second)

	h.Assert().
		PlaceTokenCount("task:complete", 1).
		PlaceTokenCount("task:failed", 1).
		HasNoTokenInPlace("task:init").
		TokenCount(2)
}

func TestPartialBatch_SomeTokensRejected_RoutedViaRejectionArcs(t *testing.T) {
	support.SkipLongFunctional(t, "slow partial-batch rejection sweep")
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "partial_rejection"))

	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "token-accepted"}`))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "token-rejected"}`))

	provider := testutil.NewMockProvider(
		interfaces.InferenceResponse{Content: "Work accepted. COMPLETE"},
		interfaces.InferenceResponse{Content: "Work needs review, no stop token"},
	)

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	h.RunUntilComplete(t, 10*time.Second)

	h.Assert().
		PlaceTokenCount("task:complete", 1).
		PlaceTokenCount("task:rejected", 1).
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:failed").
		TokenCount(2)
}

func TestPartialBatch_TemplateResolvesFromTags(t *testing.T) {
	support.SkipLongFunctional(t, "slow partial-batch template sweep")
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "service_parameterized_success"))

	testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
		WorkTypeID: "task",
		Payload:    []byte(`{"title": "template test"}`),
		Tags:       map[string]string{"branch": "feature-abc"},
	})

	writeAgentConfig(t, dir, "exec-worker", `---
type: MODEL_WORKER
model: gpt-5-codex
modelProvider: codex
stopToken: COMPLETE
---
Process the task input.
`)
	runner := testutil.NewProviderCommandRunner(
		workers.CommandResult{Stdout: []byte("Work done. COMPLETE")},
	)

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProviderCommandRunner(runner),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	h.RunUntilComplete(t, 10*time.Second)

	h.Assert().
		PlaceTokenCount("task:complete", 1).
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:failed")

	if runner.CallCount() != 1 {
		t.Fatalf("expected provider runner called 1 time, got %d", runner.CallCount())
	}
	call := runner.LastRequest()
	if call.Command != string(workers.ModelProviderCodex) {
		t.Fatalf("expected command %q, got %q", workers.ModelProviderCodex, call.Command)
	}
	assertArgsContainSequence(t, call.Args, []string{"--model", "gpt-5-codex"})
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

	testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
		Name:       "provider-exit-failure",
		WorkID:     "work-provider-exit-failure",
		WorkTypeID: "task",
		TraceID:    "trace-provider-exit-failure",
		Payload:    []byte("provider exit failure payload"),
	})

	writeAgentConfig(t, dir, "worker-a", `---
type: MODEL_WORKER
model: test-model
modelProvider: claude
stopToken: COMPLETE
---
Process the input task.
`)
	runner := testutil.NewProviderCommandRunner(
		workers.CommandResult{
			Stdout:   []byte("provider stdout before failure"),
			Stderr:   providerErrorCorpusEntryForTest(t, "claude_authentication_error").CommandResult().Stderr,
			ExitCode: 1,
		},
	)

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProviderCommandRunner(runner),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	h.RunUntilComplete(t, 10*time.Second)

	h.Assert().
		PlaceTokenCount("task:failed", 1).
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:complete")

	if runner.CallCount() != 1 {
		t.Fatalf("expected provider runner called 1 time, got %d", runner.CallCount())
	}
	call := runner.LastRequest()
	if call.Command != string(workers.ModelProviderClaude) {
		t.Fatalf("expected command %q, got %q", workers.ModelProviderClaude, call.Command)
	}
	assertArgsContainSequence(t, call.Args, []string{"--worktree", "provider-exit-failure"})

	snap := h.Marking()
	for _, tok := range snap.Tokens {
		if tok.PlaceID == "task:failed" {
			return
		}
	}

	t.Fatal("no token found in task:failed")
}

func TestPartialBatch_RetryableProviderFailuresRetryThroughScriptWrapPath(t *testing.T) {
	support.SkipLongFunctional(t, "slow partial-batch retryable-provider sweep")
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "worktree_passthrough"))

	testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
		Name:       "provider-retry-success",
		WorkID:     "work-provider-retry-success",
		WorkTypeID: "task",
		TraceID:    "trace-provider-retry-success",
		Payload:    []byte("provider retry payload"),
	})

	writeAgentConfig(t, dir, "worker-a", `---
type: MODEL_WORKER
model: test-model
modelProvider: claude
stopToken: COMPLETE
---
Process the input task.
`)
	runner := testutil.NewProviderCommandRunner(
		providerErrorCorpusEntryForTest(t, "claude_internal_server_api_error").CommandResult(),
		providerErrorCorpusEntryForTest(t, "claude_internal_server_api_error").CommandResult(),
		workers.CommandResult{Stdout: []byte("Done. COMPLETE")},
	)

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProviderCommandRunner(runner),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	h.RunUntilComplete(t, 10*time.Second)

	h.Assert().
		PlaceTokenCount("task:complete", 1).
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:failed")

	if runner.CallCount() != 3 {
		t.Fatalf("expected provider runner called 3 times, got %d", runner.CallCount())
	}
	call := runner.LastRequest()
	if call.Command != string(workers.ModelProviderClaude) {
		t.Fatalf("expected command %q, got %q", workers.ModelProviderClaude, call.Command)
	}
	assertArgsContainSequence(t, call.Args, []string{"--worktree", "provider-retry-success"})
}

func TestPartialBatch_ThrottledProviderFailureWithoutAuthoredGuardEventuallyFails(t *testing.T) {
	support.SkipLongFunctional(t, "slow partial-batch throttled-provider failure sweep")
	h, runner := throttledProviderFailureHarness(t)
	h.RunUntilComplete(t, 5*time.Second)

	h.Assert().
		PlaceTokenCount("task:failed", 1).
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:complete")

	if runner.CallCount() != 4 {
		t.Fatalf("expected provider runner called 4 times, got %d", runner.CallCount())
	}

	assertThrottledWorkFailedAfterRetries(t, h)
}

func throttledProviderFailureHarness(t *testing.T) (*testutil.ServiceTestHarness, *testutil.ProviderCommandRunner) {
	t.Helper()

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "worktree_passthrough"))

	testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
		Name:       "provider-throttle-requeue",
		WorkID:     "work-provider-throttle-requeue",
		WorkTypeID: "task",
		TraceID:    "trace-provider-throttle-requeue",
		Payload:    []byte("provider throttle payload"),
	})

	writeAgentConfig(t, dir, "worker-a", `---
type: MODEL_WORKER
model: test-model
modelProvider: claude
stopToken: COMPLETE
---
Process the input task.
`)
	runner := testutil.NewProviderCommandRunner(
		providerErrorCorpusEntryForTest(t, "claude_rate_limit_error").RepeatedCommandResults(3)...,
	)

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProviderCommandRunner(runner),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	return h, runner
}

func assertThrottledWorkFailedAfterRetries(t *testing.T, h *testutil.ServiceTestHarness) {
	t.Helper()

	snap := h.Marking()
	var failed *interfaces.Token
	for _, tok := range snap.Tokens {
		if tok.PlaceID == "task:failed" && tok.Color.WorkID == "work-provider-throttle-requeue" {
			failed = tok
			break
		}
	}
	if failed == nil {
		t.Fatal("expected failed token in task:failed")
	}
	if failed.Color.WorkID != "work-provider-throttle-requeue" {
		t.Fatalf("failed token work id = %q, want %q", failed.Color.WorkID, "work-provider-throttle-requeue")
	}

	engineState, err := h.GetEngineStateSnapshot()
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot() error = %v", err)
	}
	if len(engineState.DispatchHistory) == 0 {
		t.Fatal("expected failed throttle path to record dispatch history")
	}
	last := engineState.DispatchHistory[len(engineState.DispatchHistory)-1]
	if last.TransitionID != "process" {
		t.Fatalf("last dispatch transition = %q, want %q", last.TransitionID, "process")
	}
}
