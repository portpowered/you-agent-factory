package service_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
	"github.com/portpowered/infinite-you/internal/testutil/factorydefinitionfixtures"
	scriptpollers "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/script_pollers"
	scriptpollerswire "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/script_pollers/wire"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestRunScriptPoller_TimesOutWithoutSubmit(t *testing.T) {
	t.Parallel()

	workRequestJSON := []byte(`{
		"requestId":"linear-issue-batch-timeout",
		"type":"FACTORY_REQUEST_BATCH",
		"works":[{"name":"issue-timeout","workTypeName":"task","payload":{"id":"ISSUE-TIMEOUT"}}]
	}`)
	runner := &sequenceCommandRunner{
		outcomes: []runOutcome{{
			waitForCancel: true,
			result:        workers.CommandResult{Stdout: workRequestJSON},
		}},
	}
	submitted := &recordingSubmitter{}
	svc := newScriptPollersServiceWithOptions(scriptPollersServiceOptions{
		runner: runner,
		executionPolicy: factorydefinitionfixtures.WorkstationExecutionPolicy{
			Resolve: func(*interfaces.FactoryWorkstationConfig) (time.Duration, error) {
				return time.Millisecond, nil
			},
		},
	})
	poller := newCanonicalScriptPollerWorkstation()
	worker := newCanonicalScriptPollerWorker()
	runtimeCfg := newScriptPollerLoadedRuntimeConfig(t, t.TempDir(), poller, worker)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err := svc.RunScriptPoller(ctx, runner, runtimeCfg, poller, worker, submitted.submit)
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("RunScriptPoller error = %v, want timeout", err)
	}
	if submitted.calls != 0 {
		t.Fatalf("submit calls = %d, want 0 on timeout before stdout parse", submitted.calls)
	}
}

func TestRunScriptPoller_UsesWorkerTimeoutWithoutSubmit(t *testing.T) {
	t.Parallel()

	runner := &sequenceCommandRunner{
		outcomes: []runOutcome{{waitForCancel: true}},
	}
	submitted := &recordingSubmitter{}
	svc := newScriptPollersService(runner)
	poller := newCanonicalScriptPollerWorkstation()
	worker := newCanonicalScriptPollerWorker()
	worker.Timeout = "1ms"
	runtimeCfg := newScriptPollerLoadedRuntimeConfig(t, t.TempDir(), poller, worker)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err := svc.RunScriptPoller(ctx, runner, runtimeCfg, poller, worker, submitted.submit)
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("RunScriptPoller error = %v, want timeout", err)
	}
	if submitted.calls != 0 {
		t.Fatalf("submit calls = %d, want 0 on worker timeout", submitted.calls)
	}
}

func TestStartScriptPoller_RestartsAfterNonZeroExit(t *testing.T) {
	t.Parallel()

	fakeClock := clockwork.NewFakeClock()
	runner := &sequenceCommandRunner{
		outcomes: []runOutcome{
			{result: workers.CommandResult{ExitCode: 2}},
			{waitForCancel: true},
		},
	}
	logCore, observedLogs := observer.New(zap.InfoLevel)
	svc := newScriptPollersServiceWithOptions(scriptPollersServiceOptions{
		runner: runner,
		clock:  fakeClock,
		logger: zap.New(logCore),
	})
	poller := newCanonicalScriptPollerWorkstation()
	worker := newCanonicalScriptPollerWorker()
	runtimeCfg := newScriptPollerLoadedRuntimeConfig(t, t.TempDir(), poller, worker)

	runCtx, cancelRun := context.WithCancel(context.Background())
	var sidecars sync.WaitGroup
	svc.StartScriptPoller(
		runCtx,
		&sidecars,
		runtimeCfg,
		poller,
		worker,
		func(_ context.Context, _ work.WorkRequest) error { return nil },
	)
	t.Cleanup(func() {
		cancelRun()
		sidecars.Wait()
	})

	waitForScriptPollerRunnerCalls(t, runner, 1, time.Second)
	waitForFakeClockWaiters(t, fakeClock, 1)
	fakeClock.Advance(scriptpollers.ScriptPollerRestartBackoffMin)
	waitForScriptPollerRunnerCalls(t, runner, 2, time.Second)

	if observedLogs.FilterMessage("script poller restarting").Len() == 0 {
		t.Fatal("expected restart log after non-zero exit")
	}
	entry := observedLogs.FilterMessage("script poller restarting").All()[0]
	if got := entry.ContextMap()["error"]; got == nil || !strings.Contains(got.(string), "exited with code 2") {
		t.Fatalf("restart error = %#v, want non-zero exit context", got)
	}
}

func TestStartScriptPoller_RestartsAfterCommandFailure(t *testing.T) {
	t.Parallel()

	fakeClock := clockwork.NewFakeClock()
	runErr := errors.New("shell command unavailable")
	runner := &sequenceCommandRunner{
		outcomes: []runOutcome{
			{err: runErr},
			{waitForCancel: true},
		},
	}
	logCore, observedLogs := observer.New(zap.InfoLevel)
	svc := newScriptPollersServiceWithOptions(scriptPollersServiceOptions{
		runner: runner,
		clock:  fakeClock,
		logger: zap.New(logCore),
	})
	poller := newCanonicalScriptPollerWorkstation()
	worker := newCanonicalScriptPollerWorker()
	runtimeCfg := newScriptPollerLoadedRuntimeConfig(t, t.TempDir(), poller, worker)

	runCtx, cancelRun := context.WithCancel(context.Background())
	var sidecars sync.WaitGroup
	svc.StartScriptPoller(
		runCtx,
		&sidecars,
		runtimeCfg,
		poller,
		worker,
		func(_ context.Context, _ work.WorkRequest) error { return nil },
	)
	t.Cleanup(func() {
		cancelRun()
		sidecars.Wait()
	})

	waitForScriptPollerRunnerCalls(t, runner, 1, time.Second)
	waitForFakeClockWaiters(t, fakeClock, 1)
	fakeClock.Advance(scriptpollers.ScriptPollerRestartBackoffMin)
	waitForScriptPollerRunnerCalls(t, runner, 2, time.Second)

	entry := observedLogs.FilterMessage("script poller restarting").All()
	if len(entry) == 0 {
		t.Fatal("expected restart log after command failure")
	}
	if got := entry[0].ContextMap()["error"]; got == nil || !strings.Contains(got.(string), "execution failed") {
		t.Fatalf("restart error = %#v, want execution failed context", got)
	}
}

func TestStartScriptPoller_StopReasonOnContextCancelDuringRun(t *testing.T) {
	t.Parallel()

	fakeClock := clockwork.NewFakeClock()
	runner := &sequenceCommandRunner{
		outcomes: []runOutcome{{waitForCancel: true}},
	}
	logCore, observedLogs := observer.New(zap.InfoLevel)
	svc := newScriptPollersServiceWithOptions(scriptPollersServiceOptions{
		runner: runner,
		clock:  fakeClock,
		logger: zap.New(logCore),
	})
	poller := newCanonicalScriptPollerWorkstation()
	worker := newCanonicalScriptPollerWorker()
	runtimeCfg := newScriptPollerLoadedRuntimeConfig(t, t.TempDir(), poller, worker)

	runCtx, cancelRun := context.WithCancel(context.Background())
	var sidecars sync.WaitGroup
	svc.StartScriptPoller(
		runCtx,
		&sidecars,
		runtimeCfg,
		poller,
		worker,
		func(_ context.Context, _ work.WorkRequest) error { return nil },
	)
	cancelRun()
	sidecars.Wait()

	stopped := observedLogs.FilterMessage("script poller stopped").All()
	if len(stopped) != 1 {
		t.Fatalf("script poller stopped logs = %d, want 1", len(stopped))
	}
	if got := stopped[0].ContextMap()["reason"]; got != "context canceled" {
		t.Fatalf("stop reason = %#v, want context canceled", got)
	}
}

func TestStartScriptPoller_StopsDuringBackoffWithoutAnotherRun(t *testing.T) {
	t.Parallel()

	fakeClock := clockwork.NewFakeClock()
	runner := &sequenceCommandRunner{
		outcomes: []runOutcome{
			{result: workers.CommandResult{ExitCode: 1}},
		},
	}
	svc := newScriptPollersServiceWithOptions(scriptPollersServiceOptions{
		runner: runner,
		clock:  fakeClock,
	})
	poller := newCanonicalScriptPollerWorkstation()
	worker := newCanonicalScriptPollerWorker()
	runtimeCfg := newScriptPollerLoadedRuntimeConfig(t, t.TempDir(), poller, worker)

	runCtx, cancelRun := context.WithCancel(context.Background())
	var sidecars sync.WaitGroup
	svc.StartScriptPoller(
		runCtx,
		&sidecars,
		runtimeCfg,
		poller,
		worker,
		func(_ context.Context, _ work.WorkRequest) error { return nil },
	)

	waitForScriptPollerRunnerCalls(t, runner, 1, time.Second)
	waitForFakeClockWaiters(t, fakeClock, 1)
	cancelRun()
	sidecars.Wait()

	if runner.callCount() != 1 {
		t.Fatalf("runner calls = %d, want 1 after cancel during backoff", runner.callCount())
	}
}

type scriptPollersServiceOptions struct {
	runner          workers.CommandRunner
	clock           clockwork.Clock
	logger          *zap.Logger
	executionPolicy factorydefinitionfixtures.WorkstationExecutionPolicy
}

func newScriptPollersServiceWithOptions(options scriptPollersServiceOptions) scriptpollers.Service {
	logger := options.logger
	if logger == nil {
		logger = zap.NewNop()
	}
	executionPolicy := options.executionPolicy
	if executionPolicy.Resolve == nil {
		executionPolicy = factorydefinitionfixtures.WorkstationExecutionPolicy{
			Resolve: func(*interfaces.FactoryWorkstationConfig) (time.Duration, error) {
				return 0, nil
			},
		}
	}
	deps := scriptpollers.Dependencies{
		Logger: func(workstationName, workerName string) *zap.Logger {
			return logger
		},
		CommandRunner: func() workers.CommandRunner {
			return options.runner
		},
		ExecutionPolicy: executionPolicy,
	}
	if options.clock != nil {
		clock := options.clock
		deps.Clock = func() clockwork.Clock {
			return clock
		}
	}
	return scriptpollerswire.NewService(deps)
}

func waitForFakeClockWaiters(t *testing.T, fakeClock *clockwork.FakeClock, waiters int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := fakeClock.BlockUntilContext(ctx, waiters); err != nil {
		t.Fatalf("timed out waiting for %d fake-clock waiter(s): %v", waiters, err)
	}
}

func waitForScriptPollerRunnerCalls(t *testing.T, runner *sequenceCommandRunner, want int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if runner.callCount() >= want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %d runner call(s); got %d", want, runner.callCount())
}
