package codex

import (
	"context"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	workservice "github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

type codexConductorFixture struct {
	process       support.ApplicationProcess
	command       *support.ProcessCommand
	baseURL       string
	hostDir       string
	apiStopped    <-chan struct{}
	router        *codexCommandRouter
	identities    *codexIdentityGenerator
	apiStarts     *atomic.Int32
	scenarios     []codexConductorScenario
	opened        atomic.Int32
	closed        atomic.Int32
	streamsOpened atomic.Int32
	streamsClosed atomic.Int32

	ledgerMu sync.Mutex
	ledger   map[string]codexScenarioObservation
}

type codexConductorScenario struct {
	name              string
	factoryDir        string
	model             string
	workID            string
	requestID         string
	traceID           string
	providerSessionID string
	runner            *codexScenarioCommandRunner
	wantWorkState     string
	wantOutcome       factoryapi.WorkOutcome
	wantFailure       string
	wantProviderCalls int
	wantDispatches    int
}

type codexScenarioObservation struct {
	sessionID         string
	workID            string
	requestID         string
	dispatchIDs       []string
	providerSessionID string
	responseEventIDs  []string
}

func newCodexConductorFixture(t *testing.T) *codexConductorFixture {
	t.Helper()

	identities := &codexIdentityGenerator{}
	hostDir := newCodexHostDir(t)
	scenarios := newCodexScenarios(t)
	routes := make([]codexCommandRoute, 0, len(scenarios))
	for _, scenario := range scenarios {
		routes = append(routes, codexCommandRoute{
			selector: scenario.factoryDir,
			label:    scenario.name,
			runner:   scenario.runner,
		})
	}
	router, err := newCodexCommandRouter(routes)
	if err != nil {
		t.Fatalf("newCodexCommandRouter: %v", err)
	}

	process, command, apiStopped, apiStarts, baseURL := newCodexProcess(
		t,
		hostDir,
		router,
		identities,
	)
	return &codexConductorFixture{
		process:    process,
		command:    command,
		baseURL:    baseURL,
		hostDir:    hostDir,
		apiStopped: apiStopped,
		router:     router,
		identities: identities,
		apiStarts:  apiStarts,
		scenarios:  scenarios,
		ledger:     make(map[string]codexScenarioObservation, len(scenarios)),
	}
}

func newCodexScenarios(t *testing.T) []codexConductorScenario {
	t.Helper()

	fixtures := []struct {
		name              string
		requestID         string
		workID            string
		traceID           string
		providerSessionID string
		result            platformprocess.CommandResult
		runErr            error
		wantWorkState     string
		wantOutcome       factoryapi.WorkOutcome
		wantFailure       string
	}{
		{
			name:              "Success",
			requestID:         "codex-c04-success-request",
			workID:            "codex-c04-success-work",
			traceID:           "codex-c04-success-trace",
			providerSessionID: "codex-c04-success-provider-session",
			result: platformprocess.CommandResult{Stdout: []byte(
				`{"type":"thread.started","thread_id":"codex-c04-success-provider-session"}` + "\n" +
					`{"type":"item.completed","item":{"id":"message-final","type":"agent_message","text":"codex functional answer COMPLETE"}}` + "\n",
			)},
			wantWorkState: "task:done",
			wantOutcome:   factoryapi.WorkOutcomeAccepted,
		},
		{
			name:          "Cancellation",
			requestID:     "codex-c04-cancellation-request",
			workID:        "codex-c04-cancellation-work",
			traceID:       "codex-c04-cancellation-trace",
			runErr:        context.Canceled,
			wantWorkState: "task:failed",
			wantOutcome:   factoryapi.WorkOutcomeFailed,
			wantFailure:   codexCancellationMessage,
		},
	}

	scenarios := make([]codexConductorScenario, 0, len(fixtures))
	for _, fixture := range fixtures {
		dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
		support.WriteAgentConfig(t, dir, "worker", support.BuildModelWorkerConfig(
			modelprovider.ProviderCodex,
			codexConductorModel,
		))
		testutil.WriteSeedRequest(t, dir, workservice.SubmitRequest{
			RequestID:  fixture.requestID,
			WorkID:     fixture.workID,
			Name:       fixture.workID,
			WorkTypeID: "task",
			TraceID:    fixture.traceID,
			Payload:    []byte(`{"title":"codex conductor shared process"}`),
		})

		runner := newCodexScenarioCommandRunner(
			[]platformprocess.CommandResult{fixture.result},
			fixture.runErr,
		)
		scenarios = append(scenarios, codexConductorScenario{
			name:              fixture.name,
			factoryDir:        dir,
			model:             codexConductorModel,
			workID:            fixture.workID,
			requestID:         fixture.requestID,
			traceID:           fixture.traceID,
			providerSessionID: fixture.providerSessionID,
			runner:            runner,
			wantWorkState:     fixture.wantWorkState,
			wantOutcome:       fixture.wantOutcome,
			wantFailure:       fixture.wantFailure,
			wantProviderCalls: 1,
			wantDispatches:    1,
		})
	}
	return scenarios
}

func newCodexHostDir(t *testing.T) string {
	t.Helper()

	hostDir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	support.WriteAgentConfig(t, hostDir, "worker", support.BuildModelWorkerConfig(
		modelprovider.ProviderCodex,
		codexConductorModel,
	))
	return hostDir
}

func newCodexProcess(
	t *testing.T,
	hostDir string,
	router *codexCommandRouter,
	identities *codexIdentityGenerator,
) (
	support.ApplicationProcess,
	*support.ProcessCommand,
	<-chan struct{},
	*atomic.Int32,
	string,
) {
	t.Helper()

	api := support.NewProcessAPIServer()
	apiStopped := make(chan struct{})
	var apiStarts atomic.Int32
	process := support.BuildProcess(t, serviceedges.Edges{
		ProviderCommandRunner: router,
		APIServerStarter: func(ctx context.Context, request platformhttpserver.StartRequest) error {
			apiStarts.Add(1)
			defer close(apiStopped)
			return api.Start(ctx, request)
		},
		FactorySessionIDGenerator:                identities.nextSessionID,
		FactorySessionRuntimeInstanceIDGenerator: identities.nextRuntimeID,
		FactorySessionResponseEventIDGenerator:   identities.nextResponseEventID,
	})
	support.CleanupProcess(t, process)

	inputs := support.FakeInputs(t.Context(), []string{
		"you", "run",
		"--dir", hostDir,
		"--continuously",
		"--with-server",
		"--server", "http://127.0.0.1:1",
		"--quiet",
		"--no-record",
	})
	configureCodexProcessInputs(t, inputs, hostDir)
	command := support.StartProcessCommand(t, process, inputs.Input)
	baseURL := api.WaitForURL(t)
	assertCodexHostDefaultSession(t, baseURL)
	return process, command, apiStopped, &apiStarts, baseURL
}

func configureCodexProcessInputs(
	t *testing.T,
	inputs *support.CapturedInputs,
	hostDir string,
) {
	t.Helper()

	homeDir := t.TempDir()
	inputs.Input.Env = append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)
	inputs.Input.WorkingDirectory = hostDir
	t.Cleanup(func() {
		if !t.Failed() {
			return
		}
		if stderr := strings.TrimSpace(inputs.Stderr()); stderr != "" {
			t.Logf("daemon stderr:\n%s", stderr)
		}
		if stdout := strings.TrimSpace(inputs.Stdout()); stdout != "" {
			t.Logf("daemon stdout:\n%s", stdout)
		}
	})
}

func assertCodexHostDefaultSession(t *testing.T, baseURL string) {
	t.Helper()

	defaultSession := support.GetDefaultSession(t, baseURL)
	if !defaultSession.IsDefault || strings.TrimSpace(defaultSession.Id) == "" {
		t.Fatalf("host default session = %#v, want a live default session with a runtime identity", defaultSession)
	}
}
