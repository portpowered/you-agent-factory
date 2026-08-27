package codex

import (
	"context"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	workservice "github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

type codexConductorFixture struct {
	process       support.ApplicationProcess
	command       *codexPackageProcessCommand
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

	packageFixture := ensureCodexPackageFixture(t)
	packageFixture.beginGroup(t, "conductor")
	return &codexConductorFixture{
		process:    packageFixture.process,
		command:    packageFixture.command,
		baseURL:    packageFixture.baseURL,
		hostDir:    packageFixture.hostDir,
		apiStopped: packageFixture.apiStopped,
		router:     packageFixture.router,
		identities: packageFixture.identities,
		apiStarts:  packageFixture.apiStarts,
		scenarios:  packageFixture.conductorScenarios,
		ledger:     make(map[string]codexScenarioObservation, len(packageFixture.conductorScenarios)),
	}
}

func newCodexScenariosAt(t *testing.T, rootDir string) []codexConductorScenario {
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
		dir := copyCodexFixtureDir(
			t,
			support.LegacyFixtureDir(t, "executor_success"),
			rootDir,
			"conductor-"+fixture.name,
		)
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

func resetCodexConductorScenario(t *testing.T, scenario codexConductorScenario) {
	t.Helper()
	overwriteCodexFixtureDir(
		t,
		support.LegacyFixtureDir(t, "executor_success"),
		scenario.factoryDir,
	)
	support.WriteAgentConfig(t, scenario.factoryDir, "worker", support.BuildModelWorkerConfig(
		modelprovider.ProviderCodex,
		codexConductorModel,
	))
	testutil.WriteSeedRequest(t, scenario.factoryDir, workservice.SubmitRequest{
		RequestID:  scenario.requestID,
		WorkID:     scenario.workID,
		Name:       scenario.workID,
		WorkTypeID: "task",
		TraceID:    scenario.traceID,
		Payload:    []byte(`{"title":"codex conductor shared process"}`),
	})
}

func newCodexHostDirAt(t *testing.T, rootDir string) string {
	t.Helper()

	hostDir := copyCodexFixtureDir(
		t,
		support.LegacyFixtureDir(t, "executor_success"),
		rootDir,
		"host",
	)
	support.WriteAgentConfig(t, hostDir, "worker", support.BuildModelWorkerConfig(
		modelprovider.ProviderCodex,
		codexConductorModel,
	))
	return hostDir
}

func configureCodexProcessInputs(
	t *testing.T,
	inputs *support.CapturedInputs,
	hostDir string,
	homeDir string,
) {
	t.Helper()

	if err := os.MkdirAll(homeDir, 0o755); err != nil {
		t.Fatalf("create shared Codex home: %v", err)
	}
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
