//go:build functionallong

package providers

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	timeoutCompanionSignalTimeout = 10 * time.Second
	timeoutCompanionRunTimeout    = 20 * time.Second
)

// portos:func-length-exception owner=agent-factory reason=script-timeout-companion-long-smoke review=2026-07-22 removal=split-timeout-runner-setup-requeue-observation-and-completion-assertions-before-next-timeout-companion-change
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
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkID:     workID,
		WorkTypeID: "task",
		TraceID:    traceID,
		Payload:    []byte("timeout companion payload"),
	})

	runner := newTimeoutThenReleaseCommandRunner()
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: dir,
		Edges: serviceedges.Edges{
			ScriptCommandRunner: runner,
		},
	})

	waitForTimeoutCompanionRetryStarted(t, runner)

	close(runner.releaseCh)
	support.WaitForTerminalStatus(t, server.URL(), timeoutCompanionRunTimeout)
	session := support.GetDefaultSession(t, server.URL())
	assertSessionPlaces(t, listed, map[string]int{"task:done": 1, "task:init": 0, "task:failed": 0})
	assertListedWorkIdentity(t, support.ListDefaultSessionWork(t, server.URL()), "done", workID, "task", traceID, nil)
	assertDispatchOutcomeSequence(t, server.GetFactoryEvents(t), []factoryapi.WorkOutcome{
		factoryapi.WorkOutcomeFailed,
		factoryapi.WorkOutcomeAccepted,
	}, "execution timeout")
	if runner.CallCount() < 2 {
		t.Fatalf("timeout companion runner call count = %d, want at least 2", runner.CallCount())
	}
	server.Stop(t)
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

func (r *timeoutThenReleaseCommandRunner) Run(ctx context.Context, _ platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	r.mu.Lock()
	r.callCount++
	call := r.callCount
	r.mu.Unlock()

	if call == 1 {
		<-ctx.Done()
		r.firstTimeout.Do(func() { close(r.firstTimeoutCh) })
		return platformprocess.CommandResult{}, ctx.Err()
	}
	if call == 2 {
		r.retryStart.Do(func() { close(r.retryStartCh) })
	}

	select {
	case <-r.releaseCh:
		return platformprocess.CommandResult{Stdout: []byte("script-output-after-timeout-retry")}, nil
	case <-ctx.Done():
		return platformprocess.CommandResult{}, ctx.Err()
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
