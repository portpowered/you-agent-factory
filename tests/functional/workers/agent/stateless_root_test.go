package agent_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestBuildProcessExecutesProviderAttemptThroughRuntimeRoot proves the
// application process still composes the canonical Workers runtime around the
// Providers root and reaches the injected provider command edge.
func TestBuildProcessExecutesProviderAttemptThroughRuntimeRoot(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"runtime provider root"}`))
	runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{
		Stdout: support.CodexSuccessStdout("functional-runtime-provider-output COMPLETE"),
	})

	_, listed := support.RunFactoryToCompletionWithEdgesAndWork(
		t,
		dir,
		serviceedges.Edges{ProviderCommandRunner: runner},
		20*time.Second,
	)
	if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 1 {
		t.Fatalf("completed work = %d, want 1; listed=%#v", got, listed)
	}
	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 0 {
		t.Fatalf("failed work = %d, want 0; listed=%#v", got, listed)
	}
	if runner.CallCount() != 1 {
		t.Fatalf("provider command calls = %d, want one", runner.CallCount())
	}
}

// detachedRootScriptRunner is the single external command effect a detached
// Workers attempt is allowed to reach. It records every request so the test can
// assert the attempt actually left the Workers root through the injected edge.
type detachedRootScriptRunner struct {
	stdout   string
	calls    atomic.Int32
	commands chan string
}

func newDetachedRootScriptRunner(stdout string) *detachedRootScriptRunner {
	return &detachedRootScriptRunner{stdout: stdout, commands: make(chan string, 8)}
}

func (runner *detachedRootScriptRunner) Run(
	ctx context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	runner.calls.Add(1)
	select {
	case runner.commands <- request.Command:
	default:
	}
	if err := ctx.Err(); err != nil {
		return platformprocess.CommandResult{}, err
	}
	return platformprocess.CommandResult{Stdout: []byte(runner.stdout)}, nil
}

func (runner *detachedRootScriptRunner) callCount() int32 { return runner.calls.Load() }

func (runner *detachedRootScriptRunner) observedCommands() []string {
	observed := make([]string, 0, len(runner.commands))
	for {
		select {
		case command := <-runner.commands:
			observed = append(observed, command)
		default:
			return observed
		}
	}
}

// detachedRootOpeningRecorder counts the Factory Session and Factory Runtime
// opening effects. A detached Workers root must never reach any of them, so
// every counter staying at zero is the observable form of "this execution ran
// outside Runtime and Session opening".
type detachedRootOpeningRecorder struct {
	sessionID       atomic.Int32
	runtimeID       atomic.Int32
	responseEventID atomic.Int32
	runtimeHost     atomic.Int32
	invocations     atomic.Int32
}

func (recorder *detachedRootOpeningRecorder) generateSessionID() string {
	recorder.sessionID.Add(1)
	return "detached-root-should-not-open-session"
}

func (recorder *detachedRootOpeningRecorder) generateRuntimeID() string {
	recorder.runtimeID.Add(1)
	return "detached-root-should-not-open-runtime"
}

func (recorder *detachedRootOpeningRecorder) generateResponseEventID() string {
	recorder.responseEventID.Add(1)
	return "detached-root-should-not-publish-response-event"
}

func (recorder *detachedRootOpeningRecorder) observeRuntimeHost(factorysessions.RuntimeHostBinding) {
	recorder.runtimeHost.Add(1)
}

func (recorder *detachedRootOpeningRecorder) RecordInvocationMetric(factorysessions.InvocationMetric) {
	recorder.invocations.Add(1)
}

func (recorder *detachedRootOpeningRecorder) openingEffects() map[string]int32 {
	return map[string]int32{
		"factory session id":  recorder.sessionID.Load(),
		"runtime instance id": recorder.runtimeID.Load(),
		"response event id":   recorder.responseEventID.Load(),
		"runtime host":        recorder.runtimeHost.Load(),
		"invocation metric":   recorder.invocations.Load(),
	}
}

func (recorder *detachedRootOpeningRecorder) edges(
	runner platformprocess.CommandRunner,
) serviceedges.Edges {
	return serviceedges.Edges{
		ScriptCommandRunner:                      runner,
		FactorySessionIDGenerator:                recorder.generateSessionID,
		FactorySessionRuntimeInstanceIDGenerator: recorder.generateRuntimeID,
		FactorySessionResponseEventIDGenerator:   recorder.generateResponseEventID,
		RuntimeHostObserver:                      recorder.observeRuntimeHost,
		InvocationMetricsRecorder:                recorder,
	}
}

func detachedRootExecuteRequest(attemptID, command string) workers.ExecuteRequest {
	return workers.ExecuteRequest{
		Correlation: workers.ExecutionCorrelation{
			FactorySessionID: "detached-root-session",
			RuntimeID:        "detached-root-runtime",
			GenerationID:     "detached-root-generation",
			DispatchID:       "detached-root-dispatch",
			AttemptID:        attemptID,
		},
		Target: workers.ExecutionTarget{
			WorkerName: "script-worker",
			RunnerID:   "script",
			Command:    command,
		},
	}
}

// TestBuildStatelessWorkersExecutesDetachedAttemptThroughRoot is the public
// caller guard for the detached Workers root. pkg/root already covers the same
// composition from inside its own package, but no test outside pkg/root drove
// root.BuildStatelessWorkers as a customer would, so the promise that a
// detached attempt stays outside Factory Runtime and Factory Session opening
// had no observable guard at the public boundary.
//
// Only edges.Edges replaces external effects: the script command runner is the
// single allowed effect, and the Sessions/Runtime opening ports are supplied
// purely so their invocation counts are observable. Nothing in this test waits
// on a clock.
func TestBuildStatelessWorkersExecutesDetachedAttemptThroughRoot(t *testing.T) {
	t.Parallel()

	const detachedOutput = "detached-root-script-output"
	recorder := &detachedRootOpeningRecorder{}
	runner := newDetachedRootScriptRunner(detachedOutput)

	service, err := root.BuildStatelessWorkers(t.Context(), recorder.edges(runner))
	if err != nil {
		t.Fatalf("root.BuildStatelessWorkers() error = %v", err)
	}
	if service == nil {
		t.Fatal("root.BuildStatelessWorkers() service = nil, want the detached Workers root")
	}
	for effect, count := range recorder.openingEffects() {
		if count != 0 {
			t.Fatalf("%s effect calls during detached construction = %d, want 0", effect, count)
		}
	}

	result, err := service.Execute(
		t.Context(),
		detachedRootExecuteRequest("detached-root-attempt", "detached-root-command"),
	)
	if err != nil {
		t.Fatalf("detached Workers Execute() error = %v", err)
	}
	if result.Outcome != workers.ExecutionOutcomeAccepted {
		t.Fatalf("detached Workers Execute() outcome = %q, want %q", result.Outcome, workers.ExecutionOutcomeAccepted)
	}
	if len(result.Output.Primary) != 1 || result.Output.Primary[0].Text != detachedOutput {
		t.Fatalf("detached Workers Execute() output = %#v, want one %q part", result.Output.Primary, detachedOutput)
	}
	if got := runner.callCount(); got != 1 {
		t.Fatalf("script command runner calls = %d, want exactly one detached attempt", got)
	}
	if got := runner.observedCommands(); len(got) != 1 || got[0] != "detached-root-command" {
		t.Fatalf("script command runner commands = %#v, want the detached attempt command", got)
	}

	for effect, count := range recorder.openingEffects() {
		if count != 0 {
			t.Fatalf(
				"%s effect calls after a detached attempt = %d, want 0; the detached root must not open a Factory Runtime or Factory Session",
				effect,
				count,
			)
		}
	}
}

// TestBuildStatelessWorkersReleasesDetachedAttemptOnCancellation proves the
// public detached root terminalizes a canceled attempt as a cancellation
// instead of hanging, reporting success, or leaking the external command
// effect, and that the canceled attempt still opens no Factory Runtime or
// Factory Session. Cancellation is delivered through the caller's own context,
// so no sleep or timeout padding is used as synchronization.
func TestBuildStatelessWorkersReleasesDetachedAttemptOnCancellation(t *testing.T) {
	t.Parallel()

	recorder := &detachedRootOpeningRecorder{}
	runner := newDetachedRootScriptRunner("cancelled-detached-output")

	service, err := root.BuildStatelessWorkers(t.Context(), recorder.edges(runner))
	if err != nil {
		t.Fatalf("root.BuildStatelessWorkers() error = %v", err)
	}

	canceledCtx, cancel := context.WithCancel(t.Context())
	cancel()

	result, err := service.Execute(
		canceledCtx,
		detachedRootExecuteRequest("cancelled-root-attempt", "cancelled-root-command"),
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled detached Execute() error = %v, want context.Canceled", err)
	}
	if result.Outcome == workers.ExecutionOutcomeAccepted {
		t.Fatalf("canceled detached Execute() outcome = %q, want a non-accepted outcome", result.Outcome)
	}
	if got := runner.callCount(); got != 0 {
		t.Fatalf("script command calls for a canceled detached attempt = %d, want 0", got)
	}

	for effect, count := range recorder.openingEffects() {
		if count != 0 {
			t.Fatalf(
				"%s effect calls after a canceled detached attempt = %d, want 0",
				effect,
				count,
			)
		}
	}
}
