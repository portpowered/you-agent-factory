package agy

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	platformpty "github.com/portpowered/infinite-you/pkg/platform/pty"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// The supported host boundary fails closed if canonical execution ever selects
// the legacy PTY path. Counters are scenario-owned and safe during cleanup.
type canonicalAgyPTYObserver struct {
	allocations atomic.Int64
	starts      atomic.Int64
}

func (host *canonicalAgyPTYObserver) Allocate(context.Context) (platformpty.Allocation, error) {
	host.allocations.Add(1)
	return nil, errors.New("unexpected legacy AGY PTY allocation")
}

func (host *canonicalAgyPTYObserver) Start(platformpty.ProcessLaunch, platformpty.Allocation) (platformpty.Process, io.ReadCloser, error) {
	host.starts.Add(1)
	return nil, nil, errors.New("unexpected legacy AGY PTY start")
}

func (host *canonicalAgyPTYObserver) assertUnused(t testing.TB) {
	t.Helper()
	if allocated, started := host.allocations.Load(), host.starts.Load(); allocated != 0 || started != 0 {
		t.Fatalf("legacy PTY effects: Allocate=%d Start=%d, want zero", allocated, started)
	}
}

// This real root/Process/runtime/Work journey controls only the external AGY
// command effect. It does not prove executable discovery, authentication,
// network inference or OS-process release (owned by AGY-LIVE-SMOKE/I01).
func TestAgyCanonicalCommandRunnerExecutesWithZeroPTYEffects(t *testing.T) {
	t.Parallel()
	workDir := filepath.Join(t.TempDir(), "work")
	copyAgyDirectory(t, support.LegacyFixtureDir(t, "executor_success"), workDir)
	support.WriteAgentConfig(t, workDir, "worker", agyGoldenWorkerConfigWithStopToken("COMPLETE"))
	const prompt = "FI_T08_COMMAND_INPUT"
	const output = "FI_T08_COMMAND_OK COMPLETE"
	support.WriteWorkstationConfig(t, workDir, "process", agyGoldenWorkstationConfig(prompt, ""))
	testutil.WriteSeedFile(t, workDir, "task", []byte(`{"title":"canonical command selection"}`))
	runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{
		Stdout: []byte(`{"event":"result","result":{"conversation_id":"fi-t08-command","status":"SUCCESS","response":"FI_T08_COMMAND_OK COMPLETE","duration_seconds":1,"num_turns":1,"usage":{"input_tokens":1,"output_tokens":1,"thinking_tokens":0,"cache_read_tokens":0,"total_tokens":2}}}` + "\n"),
	})
	host := &canonicalAgyPTYObserver{}
	// Allocate the public identity before runtime activation. A private process
	// is necessary for this immutable command-plus-PTY edge shape; existing AGY
	// peers share a different graph without the PTY observer.
	sessionID := uuid.NewString()
	server, command := startCanonicalAgyScenario(t, workDir, sessionID, runner, host)
	support.WaitForSessionTerminalStatus(t, server.URL(), sessionID, agySharedInvocationTimeout)
	listed := support.GetJSON[factoryapi.ListWorkResponse](t, server.URL()+"/factory-sessions/"+sessionID+"/work")
	assertAgyGoldenWorkCompleted(t, listed)
	events := support.GetFactoryEventsForSessionAt(t, server.URL(), sessionID)
	assertAgySingleDispatch(t, events, factoryapi.WorkOutcomeAccepted)
	assertAgySingleDispatchOutput(t, events, factoryapi.WorkOutcomeAccepted, output)
	assertAgyCanonicalCommandScope(t, runner, events, sessionID, workDir, prompt)
	host.assertUnused(t)
	support.CloseFactorySessionAt(t, server.URL(), sessionID)
	command.Stop(t)
	server.Close(t)
	host.assertUnused(t)
	if runner.CallCount() != 1 {
		t.Fatal("provider command effect occurred after owned session cleanup")
	}
}

func startCanonicalAgyScenario(
	t *testing.T,
	workDir, sessionID string,
	runner *testutil.ProviderCommandRunner,
	host *canonicalAgyPTYObserver,
) (*canonicalAgyScenario, *support.ProcessCommand) {
	t.Helper()
	idleDir := filepath.Join(t.TempDir(), "idle")
	copyAgyDirectory(t, support.LegacyFixtureDir(t, "executor_success"), idleDir)
	commandEnv := agySharedEnvironment(t.TempDir())
	hostEnv := agySharedEnvironment(t.TempDir())
	hostSessionID := uuid.NewString()
	if hostSessionID == sessionID {
		t.Fatal("host and command sessions must be distinct")
	}
	// Register before support cleanup so this observer runs after all owned
	// commands and the process have joined, including on partial setup failure.
	t.Cleanup(func() {
		host.assertUnused(t)
		if runner.CallCount() != 1 {
			t.Errorf("command calls after owned cleanup = %d, want one", runner.CallCount())
		}
	})
	api := support.NewProcessAPIServer()
	process := support.BuildProcess(t, serviceedges.Edges{
		ProviderCommandRunner: runner, AgyPTYHost: host, APIServerStarter: api.Start,
	})
	host.assertUnused(t)
	if runner.CallCount() != 0 {
		t.Fatal("process construction executed provider command")
	}
	// Bootstrap both private profiles before the host readiness clock.
	for _, profile := range []struct {
		env []string
		dir string
	}{{hostEnv, idleDir}, {commandEnv, workDir}} {
		support.InitializeCustomerHomeWithProcess(t, process, profile.env, profile.dir)
		host.assertUnused(t)
		if runner.CallCount() != 0 {
			t.Fatal("profile bootstrap executed provider command")
		}
	}
	hostInputs := support.FakeInputs(context.Background(), []string{
		"you", "run", "--dir", idleDir, "--session", hostSessionID,
		"--continuously", "--with-server", "--quiet", "--no-record",
	})
	hostInputs.Input.Env = hostEnv
	hostInputs.Input.WorkingDirectory = idleDir
	hostCommand := support.StartProcessCommand(t, process, hostInputs.Input)
	server := &canonicalAgyScenario{process: process, host: hostCommand, url: api.WaitForURL(t)}
	if runner.CallCount() != 0 {
		t.Fatal("idle host consumed scenario command effect")
	}
	host.assertUnused(t)
	inputs := support.FakeInputs(context.Background(), []string{
		"you", "run", "--dir", workDir, "--session", sessionID,
		"--continuously", "--quiet", "--no-record",
	})
	inputs.Input.Env = commandEnv
	inputs.Input.WorkingDirectory = workDir
	return server, support.StartProcessCommand(t, process, inputs.Input)
}

type canonicalAgyScenario struct {
	process support.ApplicationProcess
	host    *support.ProcessCommand
	url     string
}

func (scenario *canonicalAgyScenario) URL() string { return scenario.url }

func (scenario *canonicalAgyScenario) Close(t testing.TB) {
	t.Helper()
	scenario.host.Stop(t)
	//nolint:testsleep // Bound shutdown failure after the host command joins; scenario completion is observed before cleanup.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := scenario.process.Close(ctx); err != nil {
		t.Errorf("close canonical AGY process: %v", err)
	}
}

func assertAgyCanonicalCommandScope(
	t *testing.T,
	runner *testutil.ProviderCommandRunner,
	events []factoryapi.FactoryEvent,
	sessionID, workDir, prompt string,
) {
	t.Helper()
	if runner.CallCount() != 1 {
		t.Fatalf("command calls = %d, want one", runner.CallCount())
	}
	request := runner.LastRequest()
	assertAgyGoldenCommand(t, request, workDir, prompt, "")
	if request.ExecutionScopeID != sessionID {
		t.Fatalf("command execution scope = %q, want owned session %q", request.ExecutionScopeID, sessionID)
	}
	if len(events) == 0 {
		t.Fatal("owned session has no Factory Events")
	}
	for _, event := range events {
		if event.Context.SessionId != nil && *event.Context.SessionId != sessionID {
			t.Fatalf("Factory Event %q is outside owned session", event.Id)
		}
		switch event.Type {
		case factoryapi.FactoryEventTypeWorkRequest, factoryapi.FactoryEventTypeDispatchRequest, factoryapi.FactoryEventTypeDispatchResponse:
			if event.Context.SessionId == nil {
				t.Fatalf("Work/dispatch event %q has no owned session identity", event.Id)
			}
		}
	}
}
