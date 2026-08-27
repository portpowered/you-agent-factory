package claude

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

type claudeDefaultLaneFixture struct {
	process       support.ApplicationProcess
	command       *support.ProcessCommand
	api           *support.ProcessAPIServer
	baseURL       string
	hostDir       string
	apiStopped    <-chan struct{}
	router        *claudeCommandRouter
	identities    *claudeIdentityGenerator
	apiStarts     *atomic.Int32
	scenarios     []claudeScenario
	opened        atomic.Int32
	closed        atomic.Int32
	streamsOpened atomic.Int32
	streamsClosed atomic.Int32

	ledgerMu sync.Mutex
	ledger   map[string]claudeScenarioObservation
}

type claudeScenario struct {
	name              string
	factoryDir        string
	model             string
	workID            string
	requestID         string
	traceID           string
	providerSessionID string
	runner            *claudeScenarioCommandRunner
	golden            *support.ProviderSessionCase
	wantWorkState     string
	wantOutcome       factoryapi.WorkOutcome
	wantFailure       string
	wantProviderCalls int
	wantDispatches    int
}

type claudeScenarioObservation struct {
	sessionID         string
	workID            string
	requestID         string
	dispatchIDs       []string
	providerSessionID string
	responseEventIDs  []string
}

type claudeScenarioFixture struct {
	name              string
	model             string
	requestID         string
	workID            string
	traceID           string
	providerSessionID string
	results           []platformprocess.CommandResult
	runErr            error
	golden            *support.ProviderSessionCase
	wantWorkState     string
	wantOutcome       factoryapi.WorkOutcome
	wantFailure       string
	wantProviderCalls int
	wantDispatches    int
}

func newClaudeDefaultLaneFixture(t *testing.T) *claudeDefaultLaneFixture {
	t.Helper()

	identities := &claudeIdentityGenerator{}
	fixtures := loadClaudeScenarioFixtures(t)
	hostDir := newClaudeHostDir(t)
	routes, scenarios := newClaudeScenarios(t, fixtures)
	router, err := newClaudeCommandRouter(routes)
	if err != nil {
		t.Fatalf("newClaudeCommandRouter: %v", err)
	}

	process, command, api, apiStopped, apiStarts, baseURL := newClaudeProcess(
		t,
		hostDir,
		router,
		identities,
	)
	return &claudeDefaultLaneFixture{
		process:    process,
		command:    command,
		api:        api,
		baseURL:    baseURL,
		hostDir:    hostDir,
		apiStopped: apiStopped,
		router:     router,
		identities: identities,
		apiStarts:  apiStarts,
		scenarios:  scenarios,
		ledger:     make(map[string]claudeScenarioObservation, len(scenarios)),
	}
}

func loadClaudeScenarioFixtures(t *testing.T) []claudeScenarioFixture {
	t.Helper()

	structuredFailureGolden := loadClaudeGoldenCase(t, claudeGoldenStructuredFailureCase)
	assertClaudeGoldenManifest(t, structuredFailureGolden, "claude-structured-failure")
	timeoutGolden := loadClaudeGoldenCase(t, claudeGoldenTimeoutCase)
	assertClaudeGoldenManifest(t, timeoutGolden, "claude-timeout")
	structuredFailureResult := claudeGoldenCommandResult(structuredFailureGolden, 1)
	timeoutResult := claudeGoldenCommandResult(timeoutGolden, 124)

	return []claudeScenarioFixture{
		{
			name:              "Success",
			model:             claudeConductorModel,
			requestID:         "claude-c03-success-request",
			workID:            "claude-c03-success-work",
			traceID:           "claude-c03-success-trace",
			providerSessionID: "claude-c03-success-provider-session",
			results: []platformprocess.CommandResult{{Stdout: []byte(
				`{"type":"result","subtype":"success","is_error":false,"result":"claude functional answer COMPLETE","session_id":"claude-c03-success-provider-session"}` + "\n",
			)}},
			wantWorkState:     "task:done",
			wantOutcome:       factoryapi.WorkOutcomeAccepted,
			wantProviderCalls: 1,
			wantDispatches:    1,
		},
		{
			name:              "Cancellation",
			model:             claudeConductorModel,
			requestID:         "claude-c03-cancellation-request",
			workID:            "claude-c03-cancellation-work",
			traceID:           "claude-c03-cancellation-trace",
			results:           []platformprocess.CommandResult{{}},
			runErr:            context.Canceled,
			wantWorkState:     "task:failed",
			wantOutcome:       factoryapi.WorkOutcomeFailed,
			wantFailure:       claudeCancellationMessage,
			wantProviderCalls: 1,
			wantDispatches:    1,
		},
		{
			name:              "StructuredFailure",
			model:             structuredFailureGolden.Process.Model,
			requestID:         "claude-c03-structured-failure-request",
			workID:            "claude-c03-structured-failure-work",
			traceID:           "claude-c03-structured-failure-trace",
			providerSessionID: "claude-golden-structured-failure-session",
			results:           []platformprocess.CommandResult{structuredFailureResult},
			golden:            &structuredFailureGolden,
			wantWorkState:     "task:failed",
			wantOutcome:       factoryapi.WorkOutcomeFailed,
			wantFailure:       "Reduce the request size below 20 MB.",
			wantProviderCalls: 1,
			wantDispatches:    1,
		},
		{
			name:              "Timeout",
			model:             timeoutGolden.Process.Model,
			requestID:         "claude-c03-timeout-request",
			workID:            "claude-c03-timeout-work",
			traceID:           "claude-c03-timeout-trace",
			providerSessionID: "claude-golden-timeout-session",
			results:           repeatedClaudeCommandResult(timeoutResult, claudeGoldenTimeoutCommandInvocations),
			golden:            &timeoutGolden,
			wantWorkState:     "task:failed",
			wantOutcome:       factoryapi.WorkOutcomeFailed,
			wantFailure:       "provider invocation timed out",
			wantProviderCalls: claudeGoldenTimeoutCommandInvocations,
			wantDispatches:    3,
		},
	}
}

func claudeGoldenCommandResult(
	golden support.ProviderSessionCase,
	fallbackExitCode int,
) platformprocess.CommandResult {
	exitCode := fallbackExitCode
	if golden.Process.ExitCode != nil {
		exitCode = *golden.Process.ExitCode
	}
	return platformprocess.CommandResult{
		Stdout:   append([]byte(nil), golden.Stdout.Raw...),
		Stderr:   []byte(golden.Stderr),
		ExitCode: exitCode,
	}
}

func repeatedClaudeCommandResult(result platformprocess.CommandResult, count int) []platformprocess.CommandResult {
	results := make([]platformprocess.CommandResult, count)
	for index := range results {
		results[index] = cloneClaudeCommandResult(result)
	}
	return results
}

func newClaudeHostDir(t *testing.T) string {
	t.Helper()

	hostDir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	support.WriteAgentConfig(t, hostDir, "worker", support.BuildModelWorkerConfig(
		modelprovider.ProviderClaude,
		claudeConductorModel,
	))
	return hostDir
}

func newClaudeScenarios(
	t *testing.T,
	fixtures []claudeScenarioFixture,
) ([]claudeCommandRoute, []claudeScenario) {
	t.Helper()

	routes := make([]claudeCommandRoute, 0, len(fixtures))
	scenarios := make([]claudeScenario, 0, len(fixtures))
	for index := range fixtures {
		scenario, route := newClaudeScenario(t, fixtures[index])
		routes = append(routes, route)
		scenarios = append(scenarios, scenario)
	}
	return routes, scenarios
}

func newClaudeScenario(
	t *testing.T,
	fixture claudeScenarioFixture,
) (claudeScenario, claudeCommandRoute) {
	t.Helper()

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	workerConfig := support.BuildModelWorkerConfig(modelprovider.ProviderClaude, fixture.model)
	if fixture.golden != nil {
		workerConfig = strings.Replace(workerConfig, "stopToken: COMPLETE", "skipPermissions: true\nstopToken: COMPLETE", 1)
	}
	support.WriteAgentConfig(t, dir, "worker", workerConfig)
	testutil.WriteSeedRequest(t, dir, workservice.SubmitRequest{
		RequestID:  fixture.requestID,
		WorkID:     fixture.workID,
		Name:       fixture.workID,
		WorkTypeID: "task",
		TraceID:    fixture.traceID,
		Payload:    []byte(`{"title":"claude default lane"}`),
	})

	runner := newClaudeScenarioCommandRunner(fixture.results, fixture.runErr)
	scenario := claudeScenario{
		name:              fixture.name,
		factoryDir:        dir,
		model:             fixture.model,
		workID:            fixture.workID,
		requestID:         fixture.requestID,
		traceID:           fixture.traceID,
		providerSessionID: fixture.providerSessionID,
		runner:            runner,
		golden:            fixture.golden,
		wantWorkState:     fixture.wantWorkState,
		wantOutcome:       fixture.wantOutcome,
		wantFailure:       fixture.wantFailure,
		wantProviderCalls: fixture.wantProviderCalls,
		wantDispatches:    fixture.wantDispatches,
	}
	return scenario, claudeCommandRoute{
		selector: dir,
		label:    fixture.name,
		runner:   runner,
	}
}

func newClaudeProcess(
	t *testing.T,
	hostDir string,
	router *claudeCommandRouter,
	identities *claudeIdentityGenerator,
) (
	support.ApplicationProcess,
	*support.ProcessCommand,
	*support.ProcessAPIServer,
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
	configureClaudeProcessInputs(t, inputs, hostDir)
	command := support.StartProcessCommand(t, process, inputs.Input)
	baseURL := api.WaitForURL(t)
	assertClaudeHostDefaultSession(t, baseURL)
	return process, command, api, apiStopped, &apiStarts, baseURL
}

func configureClaudeProcessInputs(
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

func assertClaudeHostDefaultSession(t *testing.T, baseURL string) {
	t.Helper()

	defaultSession := support.GetDefaultSession(t, baseURL)
	if !defaultSession.IsDefault || strings.TrimSpace(defaultSession.Id) == "" {
		t.Fatalf("host default session = %#v, want a live default session with a runtime identity", defaultSession)
	}
}
