package agent_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
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
