package acceptance

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"github.com/portpowered/infinite-you/tests/internal/functionalevidence"
)

// F02/F10/F11 share one host with a distinct immutable clock edge. The two
// sessions intentionally coexist: the same logical advance must expire only
// the authored deadline. Neither direct START creates a Factory Session.
func TestSubagentExecutionDeadlines(t *testing.T) {
	t.Parallel()
	functionalevidence.Covers(t, "cli/you.worker-sessions.start", "rest/startWorkerSession", "rest/getWorkerSessionObservationByWorkerSessionId")
	root := t.TempDir()
	hostDir, home, err := prepareInvokeContinuePackageRoot(t, root)
	if err != nil {
		t.Fatal(err)
	}
	clock := &subagentDeadlineClock{Deterministic: platformclock.NewDeterministic(time.Unix(1800000000, 0), time.Second), armed: make(chan struct{}, 1)}
	var scenarios []invokeContinueScenario
	route := &invokeContinueStaticCommandRoute{}
	runners := make(map[string]*t7GatedProviderRunner)
	for _, name := range []string{"unbounded", "deadline"} {
		runner := &t7GatedProviderRunner{}
		runner.reset()
		runners[name] = runner
		if err := appendInvokeContinueScenario(root, &scenarios, &route.routes, name, runner, runner, nil, nil, nil, nil); err != nil {
			t.Fatal(err)
		}
	}
	host, err := startInvokeContinuePackageProcessWithEdges(t, hostDir, home, route, serviceedges.Edges{Clock: clock, ProcessScheduler: clock})
	if err != nil {
		t.Fatal(err)
	}
	// Fixture bootstrap is not part of execution-deadline policy. Existing
	// bootstrap timers remain on wall time; newly admitted sessions use logical
	// time from this point. No provider route or production service is switched.
	clock.controlled.Store(true)
	t.Cleanup(func() {
		if err := host.command.stop(); err != nil {
			t.Error(err)
		}
		closeCtx, closeCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer closeCancel()
		if err := host.process.Close(closeCtx); err != nil {
			t.Error(err)
		}
	})
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	for _, runner := range runners {
		defer t7ReleaseAndJoin(t, ctx, runner)()
	}
	caller, detach := context.WithCancel(ctx)
	start := support.FakeInputs(caller, []string{"you", "--server", host.baseURL, "--json", "worker-sessions", "start",
		"--request-id", "unbounded-request", "--worker-session-id", "unbounded", "--provider", "codex", "--model", "controlled-model",
		"--working-root", scenarios[0].workingDirectory, "no execution deadline"})
	start.Input.Env, start.Input.WorkingDirectory = scenarios[0].environment(), scenarios[0].workingDirectory
	if err := host.process.Execute(start.Input); err != nil || !strings.Contains(start.Stdout(), `"accepted":true`) {
		t.Fatalf("CLI START = %v %s %s", err, start.Stdout(), start.Stderr())
	}
	detach()
	t19AwaitSignal(t, ctx, runners["unbounded"].started, "unbounded provider active after CLI returns")
	document := map[string]any{"requestId": "deadline-request", "workerSessionId": "deadline", "prompt": "explicit execution deadline",
		"provider": "codex", "model": "controlled-model", "workingRoot": scenarios[1].workingDirectory, "timeoutMillis": 5000}
	status, body := t7HTTP(t, ctx, http.MethodPost, host.baseURL+"/worker-sessions", document)
	if status != http.StatusAccepted {
		t.Fatalf("deadline START = %d %s", status, body)
	}
	t19AwaitSignal(t, ctx, runners["deadline"].started, "deadline provider active")
	t19AwaitSignal(t, ctx, clock.armed, "execution deadline registered")
	clock.SetTick(4)
	assertSubagentDeadlineObservation(t, ctx, host.baseURL, "deadline", "RUNNING", "")
	clock.SetTick(5)
	t19AwaitSignal(t, ctx, runners["deadline"].stopped, "deadline canceled selected provider")
	// Durable READ uses the existing committed execution-failure kind and
	// exact timeout detail; live-only reconciliation metadata is not recorded.
	assertSubagentDeadlineObservation(t, ctx, host.baseURL, "deadline", "FAILED", "family=retryable type=timeout")
	assertSubagentTerminalStartReplay(t, ctx, host.baseURL, document, runners["deadline"])
	clock.SetTick(21 * 60)
	assertSubagentDeadlineObservation(t, ctx, host.baseURL, "unbounded", "RUNNING", "")
	select {
	case <-runners["unbounded"].stopped:
		t.Fatal("omitted execution duration canceled the provider")
	default:
	}
	if runners["deadline"].CallCount() != 1 || runners["unbounded"].CallCount() != 1 {
		t.Fatal("execution deadline or caller cancellation relaunched a provider")
	}
	completeSubagentUnboundedSession(t, ctx, host.baseURL, scenarios[0].workingDirectory, runners["unbounded"])
}

func completeSubagentUnboundedSession(t *testing.T, ctx context.Context, baseURL, directory string, runner *t7GatedProviderRunner) {
	t.Helper()
	close(runner.release)
	t19AwaitSignal(t, ctx, runner.stopped, "unbounded provider completed by explicit fixture release")
	assertSubagentDeadlineObservation(t, ctx, baseURL, "unbounded", "COMPLETED", "")
	assertSubagentTerminalStartReplay(t, ctx, baseURL, map[string]any{
		"requestId": "unbounded-request", "workerSessionId": "unbounded", "prompt": "no execution deadline",
		"provider": "codex", "model": "controlled-model", "workingRoot": directory,
	}, runner)
}

func assertSubagentTerminalStartReplay(t *testing.T, ctx context.Context, baseURL string, document map[string]any, runner *t7GatedProviderRunner) {
	t.Helper()
	status, body := t7HTTP(t, ctx, http.MethodPost, baseURL+"/worker-sessions", document)
	var result struct {
		ID       string `json:"workerSessionId"`
		Accepted bool   `json:"accepted"`
	}
	if err := json.Unmarshal([]byte(body), &result); err != nil {
		t.Fatal(err)
	}
	if status != http.StatusAccepted || result.ID != document["workerSessionId"] || !result.Accepted || runner.CallCount() != 1 {
		t.Fatalf("terminal START replay = %d %s, provider calls = %d", status, body, runner.CallCount())
	}
}

type subagentDeadlineClock struct {
	*platformclock.Deterministic
	armed      chan struct{}
	controlled atomic.Bool
}

func (clock *subagentDeadlineClock) Now() time.Time {
	if !clock.controlled.Load() {
		return time.Now()
	}
	return clock.Deterministic.Now()
}

func (clock *subagentDeadlineClock) After(duration time.Duration) <-chan time.Time {
	return clock.NewTimer(duration).C()
}

func (clock *subagentDeadlineClock) NewTimer(duration time.Duration) platformclock.Timer {
	if !clock.controlled.Load() {
		return platformclock.Real{}.NewTimer(duration)
	}
	timer := clock.Deterministic.NewTimer(duration)
	if duration == 5*time.Second {
		select {
		case clock.armed <- struct{}{}:
		default:
		}
	}
	return timer
}

func assertSubagentDeadlineObservation(t *testing.T, ctx context.Context, baseURL, id, state, cause string) {
	t.Helper()
	type observation struct {
		ID      string `json:"workerSessionId"`
		State   string `json:"state"`
		Failure *struct {
			Kind   string `json:"kind"`
			Detail string `json:"detail"`
		} `json:"failure"`
	}
	// Command cancellation and durable public projection publication are separate
	// production actions. The finite observation API exposes no completion signal
	// for this commit; bounded reads wait only for the requested public state.
	result, err := support.WaitForObservation(10*time.Second, func() (observation, error) {
		status, body := t7HTTP(t, ctx, http.MethodGet, baseURL+"/worker-sessions/"+id, nil)
		var result observation
		if status != http.StatusOK {
			return result, fmt.Errorf("READ = %d %s", status, body)
		}
		err := json.Unmarshal([]byte(body), &result)
		return result, err
	}, func(result observation) bool { return result.State == state })
	if err != nil || result.ID != id {
		t.Fatalf("READ %s = %+v: %v", id, result, err)
	}
	if cause == "" && result.Failure != nil || cause != "" && (result.Failure == nil || result.Failure.Kind != "WORKERS_EXECUTION_FAILURE" || result.Failure.Detail != cause) {
		t.Fatalf("READ %s failure = %+v, want %q", id, result.Failure, cause)
	}
}
