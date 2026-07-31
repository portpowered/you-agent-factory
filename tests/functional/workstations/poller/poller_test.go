package poller

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	pollerWorkTypeName      = "story"
	pollerOutputStateName   = "queued"
	pollerExternalWorkID    = "external-issue-101"
	pollerExternalRequestID = "poller-external-batch-1"
	pollerScriptCommand     = "factory/scripts/poller.sh"
	pollerWorkstationName   = "poll-tasks"
	pollerWorkerName        = "script-poller"
)

// TestPollerCreatesWorkFromExternalItems proves a POLLER workstation running
// through root.BuildProcess admits Work when a controlled external poll source
// returns identifiable items. The scenario replaces only the script poller
// command edge and observes admission through the public Work listing.
func TestPollerCreatesWorkFromExternalItems(t *testing.T) {
	dir := scaffoldScriptPollerFactory(t)
	support.ClearSeedInputs(t, dir)

	runner := newPollerIngressCommandRunner(t, pollerExternalWorkRequestJSON(t))
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: dir,
		Edges: serviceedges.Edges{
			ScriptCommandRunner: runner,
		},
	})
	defer server.Stop(t)

	outputLocation := support.WorkCustomerLocation(pollerWorkTypeName, pollerOutputStateName)
	listed := waitForListedWorkAtCustomerState(t, server.URL(), outputLocation, 1, 10*time.Second)
	if got := support.CountWorkAtCustomerState(listed, outputLocation); got != 1 {
		t.Fatalf("CountWorkAtCustomerState(%q) = %d, want 1; listed=%#v", outputLocation, got, listed)
	}
	if !support.HasWorkAtCustomerState(listed, pollerExternalWorkID, outputLocation) {
		t.Fatalf("listed work = %#v, want work %q at %q", listed.Results, pollerExternalWorkID, outputLocation)
	}
	if runner.callCount() < 1 {
		t.Fatalf("poller command calls = %d, want at least one external poll invocation", runner.callCount())
	}
}

// TestPollerEmptyResultCreatesNoWork proves a POLLER workstation does not admit
// Work when the external poll source returns a successful empty result. The
// scenario replaces only the script poller command edge, waits for the empty
// poll cycle to finish through a deterministic observation edge, and asserts
// the public Work listing at the poller output location is unchanged.
func TestPollerEmptyResultCreatesNoWork(t *testing.T) {
	dir := scaffoldScriptPollerFactory(t)
	support.ClearSeedInputs(t, dir)

	runner := newPollerIngressCommandRunner(t, nil)
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: dir,
		Edges: serviceedges.Edges{
			ScriptCommandRunner: runner,
		},
	})
	defer server.Stop(t)

	outputLocation := support.WorkCustomerLocation(pollerWorkTypeName, pollerOutputStateName)
	baseline := support.ListDefaultSessionWork(t, server.URL())
	if got := support.CountWorkAtCustomerState(baseline, outputLocation); got != 0 {
		t.Fatalf("baseline CountWorkAtCustomerState(%q) = %d, want 0 before empty poll", outputLocation, got)
	}

	waitForPollCycle(t, runner, 10*time.Second)

	listed := support.ListDefaultSessionWork(t, server.URL())
	if got := support.CountWorkAtCustomerState(listed, outputLocation); got != 0 {
		t.Fatalf("CountWorkAtCustomerState(%q) = %d, want 0 after empty poll; listed=%#v", outputLocation, got, listed.Results)
	}
	if runner.callCount() < 1 {
		t.Fatalf("poller command calls = %d, want at least one empty poll invocation", runner.callCount())
	}
}

// TestPollerRecoverableFailureRetriesWithoutDuplicates proves a POLLER
// workstation retries after a recoverable external poll failure and admits each
// external item exactly once. The scenario replaces only the script poller
// command edge and controllable process clock, injects a transient command
// failure followed by a successful poll batch, and observes admission through
// public Work listings without duplicate submission for the same external item.
func TestPollerRecoverableFailureRetriesWithoutDuplicates(t *testing.T) {
	dir := scaffoldScriptPollerFactory(t)
	support.ClearSeedInputs(t, dir)

	fakeClock := clockwork.NewFakeClock()
	runner := newPollerRetrySequenceCommandRunner(t, []pollerIngressRunOutcome{
		{err: errors.New("transient poll source unavailable")},
		{stdout: pollerExternalWorkRequestJSON(t)},
	})
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: dir,
		Edges: serviceedges.Edges{
			ScriptCommandRunner: runner,
			Clock:               fakeClock,
		},
	})
	defer server.Stop(t)

	outputLocation := support.WorkCustomerLocation(pollerWorkTypeName, pollerOutputStateName)
	baseline := support.ListDefaultSessionWork(t, server.URL())
	if got := support.CountWorkAtCustomerState(baseline, outputLocation); got != 0 {
		t.Fatalf("baseline CountWorkAtCustomerState(%q) = %d, want 0 before retry scenario", outputLocation, got)
	}

	waitForPollCycle(t, runner, 10*time.Second)
	afterFailure := support.ListDefaultSessionWork(t, server.URL())
	if got := support.CountWorkAtCustomerState(afterFailure, outputLocation); got != 0 {
		t.Fatalf("CountWorkAtCustomerState(%q) = %d after failed poll, want 0; listed=%#v", outputLocation, got, afterFailure.Results)
	}

	waitForFakeClockWaiters(t, fakeClock, 1)
	fakeClock.Advance(100 * time.Millisecond)

	waitForRunnerCalls(t, runner, 2, 10*time.Second)
	listed := waitForListedWorkAtCustomerState(t, server.URL(), outputLocation, 1, 10*time.Second)
	if got := support.CountWorkAtCustomerState(listed, outputLocation); got != 1 {
		t.Fatalf("CountWorkAtCustomerState(%q) = %d, want 1 after successful retry; listed=%#v", outputLocation, got, listed.Results)
	}
	if !support.HasWorkAtCustomerState(listed, pollerExternalWorkID, outputLocation) {
		t.Fatalf("listed work = %#v, want work %q at %q", listed.Results, pollerExternalWorkID, outputLocation)
	}

	waitForRunnerCalls(t, runner, 3, 10*time.Second)
	stable := support.ListDefaultSessionWork(t, server.URL())
	if got := support.CountWorkAtCustomerState(stable, outputLocation); got != 1 {
		t.Fatalf("CountWorkAtCustomerState(%q) = %d after post-success restart, want 1 (no duplicate); listed=%#v", outputLocation, got, stable.Results)
	}
	if runner.callCount() < 2 {
		t.Fatalf("poller command calls = %d, want at least failed poll and successful retry", runner.callCount())
	}
}

func scaffoldScriptPollerFactory(t *testing.T) string {
	t.Helper()
	return support.ScaffoldFactory(t, map[string]any{
		"name": "poller-external-items",
		"workTypes": []map[string]any{{
			"name": pollerWorkTypeName,
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": pollerOutputStateName, "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]string{{
			"name":    pollerWorkerName,
			"type":    "SCRIPT_WORKER",
			"command": pollerScriptCommand,
		}},
		"workstations": []map[string]any{{
			"name":      pollerWorkstationName,
			"behavior":  "POLLER",
			"worker":    pollerWorkerName,
			"inputs":    []map[string]string{{"workType": pollerWorkTypeName, "state": "init"}},
			"outputs":   []map[string]string{{"workType": pollerWorkTypeName, "state": pollerOutputStateName}},
			"onFailure": []map[string]string{{"workType": pollerWorkTypeName, "state": "failed"}},
		}},
	})
}

func pollerExternalWorkRequestJSON(t *testing.T) []byte {
	t.Helper()
	payload, err := json.Marshal(map[string]any{
		"requestId": pollerExternalRequestID,
		"type":      "FACTORY_REQUEST_BATCH",
		"works": []map[string]any{{
			"name":         "external-issue-101",
			"workId":       pollerExternalWorkID,
			"workTypeName": pollerWorkTypeName,
			"payload": map[string]string{
				"id":    "ISSUE-101",
				"title": "External ingress item",
			},
		}},
	})
	if err != nil {
		t.Fatalf("marshal poller work request: %v", err)
	}
	return payload
}

type pollerPollCycleObserver interface {
	callCount() int
	pollCycleDone() <-chan struct{}
}

func waitForPollCycle(t *testing.T, runner pollerPollCycleObserver, timeout time.Duration) {
	t.Helper()

	select {
	case <-runner.pollCycleDone():
	case <-time.After(timeout):
		t.Fatalf("timed out waiting for poller poll cycle; calls=%d", runner.callCount())
	}
}

func waitForRunnerCalls(t *testing.T, runner pollerPollCycleObserver, want int, timeout time.Duration) {
	t.Helper()

	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		if runner.callCount() >= want {
			return
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatalf("timed out waiting for %d poller command call(s); got %d", want, runner.callCount())
		}
	}
}

func waitForFakeClockWaiters(t *testing.T, fakeClock *clockwork.FakeClock, waiters int) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := fakeClock.BlockUntilContext(ctx, waiters); err != nil {
		t.Fatalf("timed out waiting for %d fake-clock waiter(s): %v", waiters, err)
	}
}

func waitForListedWorkAtCustomerState(
	t *testing.T,
	baseURL string,
	location string,
	wantCount int,
	timeout time.Duration,
) factoryapi.ListWorkResponse {
	t.Helper()

	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	var last factoryapi.ListWorkResponse
	for {
		last = support.ListDefaultSessionWork(t, baseURL)
		if support.CountWorkAtCustomerState(last, location) >= wantCount {
			return last
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatalf(
				"timed out waiting for %d work at %s; listed=%#v",
				wantCount,
				location,
				last.Results,
			)
		}
	}
}

type pollerIngressRunOutcome struct {
	stdout   []byte
	err      error
	exitCode int
}

type pollerIngressCommandRunner struct {
	mu       sync.Mutex
	stdout   []byte
	calls    int
	pollDone chan struct{}
}

func newPollerIngressCommandRunner(t *testing.T, stdout []byte) *pollerIngressCommandRunner {
	t.Helper()
	return &pollerIngressCommandRunner{
		stdout:   append([]byte(nil), stdout...),
		pollDone: make(chan struct{}, 1),
	}
}

func (r *pollerIngressCommandRunner) Run(_ context.Context, _ platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	r.mu.Lock()
	r.calls++
	callNumber := r.calls
	stdout := append([]byte(nil), r.stdout...)
	r.mu.Unlock()

	r.signalPollCycle()

	if callNumber == 1 {
		return platformprocess.CommandResult{Stdout: stdout}, nil
	}
	return platformprocess.CommandResult{}, nil
}

func (r *pollerIngressCommandRunner) callCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls
}

func (r *pollerIngressCommandRunner) pollCycleDone() <-chan struct{} {
	return r.pollDone
}

func (r *pollerIngressCommandRunner) signalPollCycle() {
	select {
	case r.pollDone <- struct{}{}:
	default:
	}
}

type pollerRetrySequenceCommandRunner struct {
	mu       sync.Mutex
	outcomes []pollerIngressRunOutcome
	calls    int
	pollDone chan struct{}
}

func newPollerRetrySequenceCommandRunner(t *testing.T, outcomes []pollerIngressRunOutcome) *pollerRetrySequenceCommandRunner {
	t.Helper()
	copied := make([]pollerIngressRunOutcome, len(outcomes))
	for i, outcome := range outcomes {
		if outcome.stdout != nil {
			copied[i].stdout = append([]byte(nil), outcome.stdout...)
		}
		copied[i].err = outcome.err
		copied[i].exitCode = outcome.exitCode
	}
	return &pollerRetrySequenceCommandRunner{
		outcomes: copied,
		pollDone: make(chan struct{}, 8),
	}
}

func (r *pollerRetrySequenceCommandRunner) Run(_ context.Context, _ platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	r.mu.Lock()
	r.calls++
	callNumber := r.calls
	var outcome pollerIngressRunOutcome
	if callNumber-1 < len(r.outcomes) {
		outcome = r.outcomes[callNumber-1]
	}
	r.mu.Unlock()

	r.signalPollCycle()

	if outcome.err != nil {
		return platformprocess.CommandResult{}, outcome.err
	}
	return platformprocess.CommandResult{Stdout: outcome.stdout, ExitCode: outcome.exitCode}, nil
}

func (r *pollerRetrySequenceCommandRunner) callCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls
}

func (r *pollerRetrySequenceCommandRunner) pollCycleDone() <-chan struct{} {
	return r.pollDone
}

func (r *pollerRetrySequenceCommandRunner) signalPollCycle() {
	select {
	case r.pollDone <- struct{}{}:
	default:
	}
}

var _ platformprocess.CommandRunner = (*pollerIngressCommandRunner)(nil)
var _ platformprocess.CommandRunner = (*pollerRetrySequenceCommandRunner)(nil)
