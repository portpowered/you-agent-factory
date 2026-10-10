package execution_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

type selectedOperationTimer struct {
	platformclock.Timer
	delay   time.Duration
	stopped atomic.Bool
}

func (timer *selectedOperationTimer) Stop() bool {
	timer.stopped.Store(true)
	return timer.Timer.Stop()
}

type selectedOperationScheduler struct {
	*platformclock.Deterministic
	timers chan *selectedOperationTimer
	polls  chan *selectedOperationTimer
}

// Legacy callers expose time and timers, without opting into Runtime's replay
// tick ownership. The test controller alone advances this operation source.
type selectedLegacyClock struct{ platformclock.TimerSource }

func (scheduler *selectedOperationScheduler) NewTimer(delay time.Duration) platformclock.Timer {
	// Readiness is outside the held provider/retry operation being observed.
	if delay == 10*time.Millisecond {
		ready := platformclock.NewDeterministic(scheduler.Now(), delay)
		timer := ready.NewTimer(delay)
		ready.SetTick(1)
		return timer
	}
	timer := &selectedOperationTimer{Timer: scheduler.Deterministic.NewTimer(delay), delay: delay}
	if delay == 250*time.Millisecond {
		scheduler.polls <- timer
	}
	// Observe the retry cells' exact configured backoffs. Retention,
	// supervision, startup deadlines and heartbeats remain held on this same
	// source; they are not retry-registration signals. A wrong retry delay
	// therefore fails the registration ceiling instead of advancing another
	// component's timer. Five minutes is the AGY default attempt deadline.
	if delay != 100*time.Millisecond && delay != 200*time.Millisecond && delay != 5*time.Minute {
		return timer
	}
	scheduler.timers <- timer
	return timer
}

type selectedCommandCall struct {
	request  platformprocess.CommandRequest
	reply    chan platformprocess.CommandResult
	canceled chan error
}

type selectedCommandRunner struct{ calls chan selectedCommandCall }

func (runner *selectedCommandRunner) Run(ctx context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	call := selectedCommandCall{request: request, reply: make(chan platformprocess.CommandResult, 1), canceled: make(chan error, 1)}
	select {
	case runner.calls <- call:
	case <-ctx.Done():
		return platformprocess.CommandResult{}, ctx.Err()
	}
	select {
	case result := <-call.reply:
		return result, nil
	case <-ctx.Done():
		call.canceled <- ctx.Err()
		return platformprocess.CommandResult{}, ctx.Err()
	}
}

func selectedAwait[T any](t *testing.T, channel <-chan T) T {
	t.Helper()
	// The channel is the synchronization boundary; host time is only a failure
	// ceiling for a missing observation, never the operation's scheduler.
	select {
	case value := <-channel:
		return value
	//nolint:testsleep // Channel observations synchronize operations; host time only bounds a missing signal.
	case <-time.After(30 * time.Second):
		t.Fatal("selected operation did not reach its observation boundary")
		var zero T
		return zero
	}
}

func selectedAwaitCompletion(t *testing.T, scheduler *selectedOperationScheduler, run selectedInvocation, advance func(time.Duration)) error {
	t.Helper()
	// Completion events can precede subscription delivery. Drive the public
	// waiter's registered fallback polls after the command reply, without
	// substituting host time or advancing held retry waits during their proof.
	//nolint:testsleep // Selected polls drive progress; host time only bounds a missing terminal observation.
	ceiling := time.NewTimer(30 * time.Second)
	defer ceiling.Stop()
	for {
		select {
		case err := <-run.done:
			return err
		case poll := <-scheduler.polls:
			if !poll.stopped.Load() {
				advance(poll.delay)
			}
		case <-ceiling.C:
			t.Fatal("public invocation did not complete after selected waiter polls")
			return nil
		}
	}
}

func selectedAwaitRetry(t *testing.T, scheduler *selectedOperationScheduler, runner *selectedCommandRunner, run selectedInvocation) *selectedOperationTimer {
	t.Helper()
	// Channel observations synchronize the operation. Host time only bounds a
	// missing signal so a wiring regression cannot hang the functional lane.
	select {
	case timer := <-scheduler.timers:
		return timer
	case err := <-run.done:
		t.Fatalf("invocation ended before selected retry: %v; output: %s", err, run.stdout())
	case call := <-runner.calls:
		t.Fatalf("command bypassed selected retry wait: %s %v", call.request.Command, call.request.Args)
	//nolint:testsleep // Timer registration synchronizes progress; host time only bounds a missing signal.
	case <-time.After(30 * time.Second):
		t.Fatal("selected retry timer was not registered")
	}
	return nil
}

type selectedInvocation struct {
	done   chan error
	stdout func() string
}

func startSelectedInvocation(t *testing.T, process support.Process, ctx context.Context) selectedInvocation {
	t.Helper()
	return startSelectedConfiguredInvocation(t, process, ctx, support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "gpt-5-codex"))
}

func startSelectedConfiguredInvocation(t *testing.T, process support.Process, ctx context.Context, config string) selectedInvocation {
	t.Helper()
	ctx, cancel := context.WithCancel(ctx)
	dir := scaffoldSelectedFactory(t, config)
	home := t.TempDir()
	input := support.FakeInputs(ctx, []string{"you", "--json", "run", "--factory", filepath.Join(dir, interfaces.FactoryConfigFile), "--session", uuid.NewString(), "--no-record", "selected operation"})
	input.WorkingDirectory = dir
	input.Env = []string{"HOME=" + home, "USERPROFILE=" + home, "APPDATA=" + filepath.Join(home, "appdata"), "LOCALAPPDATA=" + filepath.Join(home, "localappdata"), "XDG_CONFIG_HOME=" + filepath.Join(home, "config"), "XDG_CACHE_HOME=" + filepath.Join(home, "cache"), "XDG_STATE_HOME=" + filepath.Join(home, "state"), "XDG_DATA_HOME=" + filepath.Join(home, "data")}
	done := make(chan error, 1)
	joined := make(chan struct{})
	t.Cleanup(func() {
		cancel()
		selectedAwait(t, joined)
	})
	go func() {
		defer close(joined)
		done <- process.Execute(input.Input)
	}()
	return selectedInvocation{done: done, stdout: input.Stdout}
}

func scaffoldSelectedFactory(t *testing.T, config string) string {
	t.Helper()
	// Bound the Factory dispatch budget separately from Workers' three
	// provider commands. Retryable failures otherwise requeue the Work under
	// the default workstation budget, legitimately issuing another dispatch.
	dir := support.ScaffoldFactory(t, map[string]any{
		"workTypes": []map[string]any{{
			"name": "task", "handlingBehavior": []string{"DEFAULT"},
			"states": []map[string]string{{"name": "init", "type": "INITIAL"}, {"name": "complete", "type": "TERMINAL"}, {"name": "failed", "type": "FAILED"}},
		}},
		"workers": []map[string]string{{"name": "worker-a"}},
		"workstations": []map[string]any{{
			"name": "process", "worker": "worker-a",
			"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
			"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
			"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
			"limits":    map[string]int{"maxRetries": 1},
		}},
	})
	support.WriteAgentConfig(t, dir, "worker-a", config)
	return dir
}

func TestSelectedProviderDeadline(t *testing.T) {
	t.Parallel()
	for _, legacy := range []bool{false, true} {
		name := "explicit scheduler beats timer-capable fact clock"
		if legacy {
			name = "legacy timer-capable clock supplies omitted scheduler"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			base := time.Date(2040, 1, 1, 0, 0, 0, 0, time.UTC)
			facts := platformclock.NewDeterministic(base, time.Second)
			scheduler := &selectedOperationScheduler{Deterministic: platformclock.NewDeterministic(base.Add(time.Hour), time.Millisecond), timers: make(chan *selectedOperationTimer, 32), polls: make(chan *selectedOperationTimer, 256)}
			runner := &selectedCommandRunner{calls: make(chan selectedCommandCall, 16)}
			edges := serviceedges.Edges{Clock: facts, ProcessScheduler: scheduler, ProviderCommandRunner: runner}
			if legacy {
				edges.Clock, edges.ProcessScheduler = selectedLegacyClock{scheduler}, nil
			}
			process := support.BuildProcess(t, edges)
			tick := 0
			advance := func(delay time.Duration) { tick += int(delay / time.Millisecond); scheduler.SetTick(tick) }
			runSelectedProviderDeadline(t, process, facts, scheduler, runner, advance)
		})
	}
}

func runSelectedProviderDeadline(t *testing.T, process support.Process, facts *platformclock.Deterministic, scheduler *selectedOperationScheduler, runner *selectedCommandRunner, advance func(time.Duration)) {
	t.Helper()
	run := startSelectedConfiguredInvocation(t, process, t.Context(), support.BuildModelWorkerConfig(modelprovider.ProviderAntigravity, "gemini-3.6-flash-low"))
	for attempt := 0; attempt < 3; attempt++ {
		t.Logf("deadline attempt %d: awaiting command", attempt+1)
		call := selectedAwait(t, runner.calls)
		if call.request.Command != "agy" || !strings.Contains(strings.Join(call.request.Args, " "), "--print-timeout 5m") {
			t.Fatalf("configured provider command = %s %v", call.request.Command, call.request.Args)
		}
		t.Log("awaiting configured deadline registration")
		timer := selectedAwait(t, scheduler.timers)
		if timer.delay != 5*time.Minute {
			t.Fatalf("provider deadline = %s", timer.delay)
		}
		facts.SetTick(3600 + attempt*3600)
		select {
		case err := <-call.canceled:
			t.Fatalf("fact clock expired selected attempt: %v", err)
		default:
		}
		advance(timer.delay - time.Millisecond)
		select {
		case err := <-call.canceled:
			t.Fatalf("provider expired before its selected deadline: %v", err)
		default:
		}
		advance(time.Millisecond)
		t.Log("awaiting selected deadline cancellation")
		if err := selectedAwait(t, call.canceled); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("selected provider timeout = %v", err)
		}
		if attempt < 2 {
			t.Log("awaiting deadline retry registration")
			retry := selectedAwaitRetry(t, scheduler, runner, run)
			if !timer.stopped.Load() {
				t.Fatal("expired attempt timer was not stopped before retry")
			}
			assertSelectedRetryHeld(t, facts, runner, retry, time.Duration(attempt+1)*100*time.Millisecond)
			advance(retry.delay)
		}
	}
	if err := selectedAwaitCompletion(t, scheduler, run, advance); err == nil {
		t.Fatal("expired provider attempts returned success")
	}
	response := support.DecodeInvocationResponseJSON(t, run.stdout())
	if response.Status != factoryapi.InvocationTerminalStatusFailed {
		t.Fatalf("provider deadline terminal result = %#v", response)
	}
	t.Log("S05/S09 provider/S10 legacy: held configured deadline expires only on its selected scheduler, maps to deadline exceeded, stops before retry and yields bounded public failure")
}

func TestSelectedEffectsOmittedDefaults(t *testing.T) {
	t.Parallel()
	runner := &selectedCommandRunner{calls: make(chan selectedCommandCall, 1)}
	process := support.BuildProcess(t, serviceedges.Edges{ProviderCommandRunner: runner})
	run := startSelectedInvocation(t, process, t.Context())
	selectedAwait(t, runner.calls).reply <- platformprocess.CommandResult{Stdout: support.CodexSuccessStdout("default effects COMPLETE")}
	if err := selectedAwait(t, run.done); err != nil {
		t.Fatal(err)
	}
	response := support.DecodeInvocationResponseJSON(t, run.stdout())
	if response.Status != factoryapi.InvocationTerminalStatusCompleted || !strings.Contains(run.stdout(), "default effects COMPLETE") {
		t.Fatalf("default effects result = %#v", response)
	}
	t.Log("S10 default: omitted fact clock and scheduler preserve public provider success")
}

func selectedRetryFailure() platformprocess.CommandResult {
	// Timeouts use the bounded attempt policy. Native upstream outages use
	// the separate, longer backpressure window and are not this cell's failure.
	return platformprocess.CommandResult{ExitCode: 1, Stdout: []byte("{\"type\":\"thread.started\",\"thread_id\":\"selected-continuation\"}\n{\"type\":\"turn.failed\",\"error\":{\"message\":\"request timed out\"}}\n")}
}

func assertSelectedRetryHeld(t *testing.T, facts *platformclock.Deterministic, runner *selectedCommandRunner, timer *selectedOperationTimer, delay time.Duration) {
	t.Helper()
	if timer.delay != delay {
		t.Fatalf("retry delay = %s, want %s", timer.delay, delay)
	}
	facts.SetTick(3600)
	select {
	case <-timer.C():
		t.Fatal("fact clock released selected backoff")
	default:
	}
	select {
	case call := <-runner.calls:
		t.Fatalf("provider retried before backoff advancement: %v", call.request.Args)
	default:
	}
}

// These cases intentionally advance one immutable process's shared scheduler
// in order. Other functional cohorts overlap; each invocation has its own
// explicit session, profile, output and cancellation context.
func TestSelectedSchedulerControlsProviderRetries(t *testing.T) {
	t.Parallel()
	base := time.Date(2040, 1, 1, 0, 0, 0, 0, time.UTC)
	facts := platformclock.NewDeterministic(base, time.Second)
	scheduler := &selectedOperationScheduler{Deterministic: platformclock.NewDeterministic(base.Add(time.Hour), time.Millisecond), timers: make(chan *selectedOperationTimer, 32), polls: make(chan *selectedOperationTimer, 256)}
	runner := &selectedCommandRunner{calls: make(chan selectedCommandCall, 16)}
	process := support.BuildProcess(t, serviceedges.Edges{Clock: facts, ProcessScheduler: scheduler, ProviderCommandRunner: runner})
	tick := 0
	advance := func(delay time.Duration) { tick += int(delay / time.Millisecond); scheduler.SetTick(tick) }
	runSelectedRetryBudget(t, process, facts, scheduler, runner, advance)
	runSelectedRetryCancellation(t, process, facts, scheduler, runner, advance)
}

func runSelectedRetryBudget(t *testing.T, process support.Process, facts *platformclock.Deterministic, scheduler *selectedOperationScheduler, runner *selectedCommandRunner, advance func(time.Duration)) {
	t.Helper()
	for _, persistent := range []bool{false, true} {
		run := startSelectedInvocation(t, process, t.Context())
		first := selectedAwait(t, runner.calls)
		t.Logf("first provider command: %s %v", first.request.Command, first.request.Args)
		first.reply <- selectedRetryFailure()
		for attempt, delay := range []time.Duration{100 * time.Millisecond, 200 * time.Millisecond} {
			timer := selectedAwaitRetry(t, scheduler, runner, run)
			assertSelectedRetryHeld(t, facts, runner, timer, delay)
			advance(delay)
			next := selectedAwait(t, runner.calls)
			t.Logf("selected backoff %s released next command", delay)
			if !strings.Contains(strings.Join(next.request.Args, " "), "resume selected-continuation") {
				t.Fatalf("retry lost continuation: %v", next.request.Args)
			}
			if !timer.stopped.Load() {
				t.Fatal("completed backoff timer was not stopped")
			}
			if !persistent {
				next.reply <- platformprocess.CommandResult{Stdout: support.CodexSuccessStdout("selected retry COMPLETE")}
				break
			}
			next.reply <- selectedRetryFailure()
			if attempt == 1 {
				break
			}
		}
		t.Log("awaiting public invocation result")
		err := selectedAwaitCompletion(t, scheduler, run, advance)
		response := support.DecodeInvocationResponseJSON(t, run.stdout())
		if persistent {
			if err == nil || response.Status != factoryapi.InvocationTerminalStatusFailed {
				t.Fatalf("bounded failure = %v, %#v", err, response)
			}
			select {
			case call := <-runner.calls:
				t.Fatalf("attempt budget exceeded: %v", call.request.Args)
			default:
			}
			t.Log("S07: two selected backoffs terminate after exactly three failed commands")
		} else {
			if err != nil || response.Status != factoryapi.InvocationTerminalStatusCompleted {
				t.Fatalf("retry success = %v, %#v", err, response)
			}
			t.Log("S06/S09 retry: held selected backoff preserves continuation and public success; fact timer advancement cannot release it")
		}
	}
}

func runSelectedRetryCancellation(t *testing.T, process support.Process, facts *platformclock.Deterministic, scheduler *selectedOperationScheduler, runner *selectedCommandRunner, advance func(time.Duration)) {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	run := startSelectedInvocation(t, process, ctx)
	selectedAwait(t, runner.calls).reply <- selectedRetryFailure()
	timer := selectedAwaitRetry(t, scheduler, runner, run)
	assertSelectedRetryHeld(t, facts, runner, timer, 100*time.Millisecond)
	cancel()
	if err := selectedAwait(t, run.done); err == nil {
		t.Fatal("canceled retry returned success")
	}
	if !timer.stopped.Load() {
		t.Fatal("canceled retry timer was not stopped")
	}
	peer := startSelectedInvocation(t, process, t.Context())
	selectedAwait(t, runner.calls).reply <- platformprocess.CommandResult{Stdout: support.CodexSuccessStdout("healthy peer COMPLETE")}
	if err := selectedAwaitCompletion(t, scheduler, peer, advance); err != nil {
		t.Fatal(err)
	}
	advance(100 * time.Millisecond)
	select {
	case call := <-runner.calls:
		t.Fatalf("canceled retry issued another command: %v", call.request.Args)
	default:
	}
	if !strings.Contains(peer.stdout(), "healthy peer COMPLETE") {
		t.Fatal("peer result was lost")
	}
	t.Log("S08: cancel joins selected retry without a next attempt; owned timer stops while peer completes")
}
