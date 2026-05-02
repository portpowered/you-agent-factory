//go:build functionallong

package providers

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil"
	"github.com/portpowered/infinite-you/pkg/workers"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	timeoutCompanionSignalTimeout = 10 * time.Second
	timeoutCompanionRunTimeout    = 20 * time.Second
)

func TestIntegrationSmoke_ScriptTimeoutCompanionRequeuesBeforeLaterCompletion(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "script_executor_dir"))
	traceID := "trace-script-timeout-companion-001"
	workID := "work-script-timeout-companion-001"

	support.WriteWorkstationConfig(t, dir, "run-script", `---
type: MODEL_WORKSTATION
limits:
  maxExecutionTime: 10ms
---
Execute the script.
`)
	testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
		WorkID:     workID,
		WorkTypeID: "task",
		TraceID:    traceID,
		Payload:    []byte("timeout companion payload"),
	})

	runner := newTimeoutThenReleaseCommandRunner()
	harness := testutil.NewServiceTestHarness(t, dir,
		testutil.WithFullWorkerPoolAndScriptWrap(),
		testutil.WithRunAsync(),
		testutil.WithCommandRunner(runner),
	)

	runCtx, cancel := context.WithTimeout(context.Background(), timeoutCompanionRunTimeout)
	defer cancel()
	errCh := harness.RunInBackground(runCtx)

	waitForTimeoutCompanionRetryStarted(t, runner)

	engineState, err := harness.GetEngineStateSnapshot()
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot() error = %v", err)
	}
	workDispatches := dispatchesForWorkID(engineState.DispatchHistory, workID)
	if len(workDispatches) == 0 {
		t.Fatal("missing first timeout dispatch after retry signal")
	}
	first := workDispatches[0]
	if first.Outcome != interfaces.OutcomeFailed {
		t.Fatalf("first timeout dispatch outcome = %s, want %s", first.Outcome, interfaces.OutcomeFailed)
	}
	if first.Reason != "execution timeout" {
		t.Fatalf("first timeout dispatch reason = %q, want %q", first.Reason, "execution timeout")
	}
	if first.ProviderFailure == nil {
		t.Fatal("first timeout dispatch ProviderFailure is nil, want timeout metadata")
	}
	if first.ProviderFailure.Type != interfaces.ProviderErrorTypeTimeout {
		t.Fatalf("first timeout dispatch provider failure type = %s, want %s", first.ProviderFailure.Type, interfaces.ProviderErrorTypeTimeout)
	}
	if first.ProviderFailure.Family != interfaces.ProviderErrorFamilyRetryable {
		t.Fatalf("first timeout dispatch provider failure family = %s, want %s", first.ProviderFailure.Family, interfaces.ProviderErrorFamilyRetryable)
	}
	if len(first.OutputMutations) == 0 || first.OutputMutations[0].ToPlace != "task:init" {
		t.Fatalf("first timeout dispatch mutations = %#v, want requeue to task:init", first.OutputMutations)
	}
	if first.OutputMutations[0].Token == nil || first.OutputMutations[0].Token.Color.WorkID != workID {
		t.Fatalf("first timeout dispatch requeued token = %#v, want work %q", first.OutputMutations[0].Token, workID)
	}

	close(runner.releaseCh)
	waitForTimeoutCompanionCompletion(t, harness, errCh, cancel)

	finalState, err := harness.GetEngineStateSnapshot()
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot() error = %v", err)
	}
	if !support.HasWorkTokenInPlace(finalState.Marking, "task:done", workID) {
		t.Fatalf("missing completed token for %q in task:done; marking=%#v", workID, finalState.Marking.PlaceTokens)
	}

	finalDispatches := dispatchesForWorkID(finalState.DispatchHistory, workID)
	if len(finalDispatches) < 2 {
		t.Fatalf("final timeout companion dispatch count = %d, want at least 2", len(finalDispatches))
	}
	last := finalDispatches[len(finalDispatches)-1]
	if last.Outcome != interfaces.OutcomeAccepted {
		t.Fatalf("last timeout companion dispatch outcome = %s, want %s", last.Outcome, interfaces.OutcomeAccepted)
	}
	if runner.CallCount() < 2 {
		t.Fatalf("timeout companion runner call count = %d, want at least 2", runner.CallCount())
	}
}

type timeoutThenReleaseCommandRunner struct {
	mu             sync.Mutex
	callCount      int
	releaseCh      chan struct{}
	firstTimeoutCh chan struct{}
	retryStartCh   chan struct{}
	firstTimeout   sync.Once
	retryStart     sync.Once
}

func newTimeoutThenReleaseCommandRunner() *timeoutThenReleaseCommandRunner {
	return &timeoutThenReleaseCommandRunner{
		releaseCh:      make(chan struct{}),
		firstTimeoutCh: make(chan struct{}),
		retryStartCh:   make(chan struct{}),
	}
}

func (r *timeoutThenReleaseCommandRunner) Run(ctx context.Context, _ workers.CommandRequest) (workers.CommandResult, error) {
	r.mu.Lock()
	r.callCount++
	call := r.callCount
	r.mu.Unlock()

	if call == 1 {
		<-ctx.Done()
		r.firstTimeout.Do(func() { close(r.firstTimeoutCh) })
		return workers.CommandResult{}, ctx.Err()
	}
	if call == 2 {
		r.retryStart.Do(func() { close(r.retryStartCh) })
	}

	select {
	case <-r.releaseCh:
		return workers.CommandResult{Stdout: []byte("script-output-after-timeout-retry")}, nil
	case <-ctx.Done():
		return workers.CommandResult{}, ctx.Err()
	}
}

func (r *timeoutThenReleaseCommandRunner) CallCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.callCount
}

func waitForTimeoutCompanionRetryStarted(t *testing.T, runner *timeoutThenReleaseCommandRunner) {
	t.Helper()

	deadline := time.Now().Add(timeoutCompanionSignalTimeout)
	select {
	case <-runner.firstTimeoutCh:
	case <-time.After(timeoutCompanionSignalTimeout):
		t.Fatalf("missing first timeout signal within %s", timeoutCompanionSignalTimeout)
	}

	remaining := time.Until(deadline)
	if remaining <= 0 {
		t.Fatalf("retry dispatch did not start within %s", timeoutCompanionSignalTimeout)
	}
	select {
	case <-runner.retryStartCh:
	case <-time.After(remaining):
		t.Fatalf("missing retry dispatch signal within %s", timeoutCompanionSignalTimeout)
	}
}

func waitForTimeoutCompanionCompletion(
	t *testing.T,
	harness *testutil.ServiceTestHarness,
	errCh <-chan error,
	cancel context.CancelFunc,
) {
	t.Helper()

	select {
	case <-harness.WaitToComplete():
	case err := <-errCh:
		t.Fatalf("factory exited before timeout companion completion: %v", err)
	case <-time.After(timeoutCompanionRunTimeout):
		t.Fatalf("timed out waiting %s for timeout companion completion", timeoutCompanionRunTimeout)
	}

	cancel()
	select {
	case err := <-errCh:
		if err != nil && err != context.Canceled && err != context.DeadlineExceeded {
			t.Fatalf("timeout companion background run error = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for timeout companion background run to exit")
	}
}
