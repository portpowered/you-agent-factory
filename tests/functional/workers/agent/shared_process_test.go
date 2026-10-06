package agent_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	agentSharedProcessTimeout = 20 * time.Second
	agentFailureMessage       = "Codex authentication failed."
	agentCancellationMessage  = "provider invocation was canceled"
	agentTimeoutMessage       = "provider invocation timed out"
)

// TestAgentSharedProcess exercises provider selection, Work outcomes, cancellation
// and recovery through one host with distinct explicit Factory Sessions.
func TestAgentSharedProcess(t *testing.T) {
	t.Parallel()
	fixture := newAgentSharedProcessFixture(t)

	for _, scenario := range fixture.scenarios {
		if scenario.name != "Invalid" {
			continue
		}
		t.Run(scenario.name, func(t *testing.T) {
			t.Run("UnknownProvider", func(t *testing.T) {
				fixture.assertUnknownProvider(t, scenario)
			})
			t.Run("MalformedConfiguration", func(t *testing.T) {
				runAgentMalformedConfigurationProbe(t)
			})
		})
	}

	fixture.start(t)
	// Recovery must run after adverse cases on this same host.
	for _, scenario := range fixture.scenarios {
		scenario := scenario
		if scenario.name == "Invalid" {
			continue
		}
		t.Run(scenario.name, func(t *testing.T) {
			fixture.runScenario(t, scenario)
		})
	}
}

type agentSharedProcessFixture struct {
	process    support.ApplicationProcess
	command    *support.ProcessCommand
	api        *support.ProcessAPIServer
	apiClosed  chan struct{}
	apiClose   sync.Once
	baseURL    string
	hostDir    string
	homeDir    string
	router     *agentSharedCommandRouter
	identities *agentSharedIdentityGenerator
	scenarios  []agentSharedScenario

	sessionsMu sync.Mutex
	opened     map[string]string
	closed     map[string]struct{}
}

type agentSharedScenario struct {
	name           string
	factoryDir     string
	model          string
	inputMarker    string
	output         string
	inputMode      agentSharedInputMode
	behavior       agentSharedScenarioBehavior
	provider       modelprovider.Provider
	runner         *agentSharedScenarioRunner
	wantOutcome    factoryapi.WorkOutcome
	wantFailure    factoryapi.WorkFailureType
	wantMessage    string
	wantCalls      int
	wantDispatches int
}

type agentSharedInputMode string

type agentSharedScenarioBehavior string

const (
	agentSharedTextInput        agentSharedInputMode = "text"
	agentSharedJSONPayloadInput agentSharedInputMode = "json-payload"
	agentSharedJSONSeedInput    agentSharedInputMode = "json-seed"

	agentSharedSuccess     agentSharedScenarioBehavior = "success"
	agentSharedHeldSuccess agentSharedScenarioBehavior = "held-success"
	agentSharedFailure     agentSharedScenarioBehavior = "failure"
	agentSharedTimeout     agentSharedScenarioBehavior = "timeout"
	agentSharedCancel      agentSharedScenarioBehavior = "cancel"
)

func newAgentSharedProcessFixture(t *testing.T) *agentSharedProcessFixture {
	t.Helper()

	hostDir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	support.ClearSeedInputs(t, hostDir)
	scenarios := newAgentSharedScenarios(t)
	router := newAgentSharedCommandRouter(t, scenarios)
	api := support.NewProcessAPIServer()
	apiClosed := make(chan struct{})
	identities := &agentSharedIdentityGenerator{}
	fixture := &agentSharedProcessFixture{
		api:        api,
		apiClosed:  apiClosed,
		hostDir:    hostDir,
		homeDir:    t.TempDir(),
		router:     router,
		identities: identities,
		scenarios:  scenarios,
		opened:     make(map[string]string, len(scenarios)),
		closed:     make(map[string]struct{}, len(scenarios)),
	}

	process, err := support.BuildProcessWithContext(context.Background(), serviceedges.Edges{
		APIServerStarter: func(ctx context.Context, request platformhttpserver.StartRequest) error {
			err := api.Start(ctx, request)
			fixture.apiClose.Do(func() { close(apiClosed) })
			return err
		},
		ProviderCommandRunner:                  router,
		FactorySessionIDGenerator:              identities.nextSessionID,
		FactorySessionResponseEventIDGenerator: identities.nextResponseEventID,
	})
	if err != nil {
		t.Fatalf("BuildProcess() error = %v", err)
	}
	fixture.process = process
	t.Cleanup(func() { fixture.close(t) })
	return fixture
}

func newAgentSharedScenarios(t *testing.T) []agentSharedScenario {
	t.Helper()
	cases := agentSharedScenarioSpecs()

	scenarios := make([]agentSharedScenario, 0, len(cases))
	for _, testCase := range cases {
		dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
		support.ClearSeedInputs(t, dir)
		support.WriteAgentConfig(t, dir, "worker", support.BuildModelWorkerConfig(
			testCase.provider,
			testCase.model,
		))
		support.WriteWorkstationConfig(t, dir, "process", "---\ntype: MODEL_WORKSTATION\n---\nAgent input: {{ (index .Inputs 0).Payload }}\n")
		if testCase.inputMode == agentSharedJSONSeedInput {
			testutil.WriteSeedFile(t, dir, "task", []byte(fmt.Sprintf(`{"marker":%q}`, testCase.inputMarker)))
		}
		scenario := agentSharedScenario{
			name:           testCase.name,
			factoryDir:     dir,
			model:          testCase.model,
			inputMarker:    testCase.inputMarker,
			output:         testCase.output,
			inputMode:      testCase.inputMode,
			behavior:       testCase.behavior,
			provider:       testCase.provider,
			wantOutcome:    factoryapi.WorkOutcomeAccepted,
			wantFailure:    testCase.failure,
			wantMessage:    testCase.message,
			wantCalls:      1,
			wantDispatches: 1,
		}
		if testCase.behavior != "" {
			scenario.runner = newAgentSharedScenarioRunner(testCase.behavior, testCase.output, testCase.message)
			if testCase.behavior != agentSharedSuccess && testCase.behavior != agentSharedHeldSuccess {
				scenario.wantOutcome = factoryapi.WorkOutcomeFailed
			}
			if testCase.behavior == agentSharedTimeout {
				scenario.wantCalls = 9
				scenario.wantDispatches = 3
			}
		} else {
			scenario.runner = newAgentSharedScenarioRunner(agentSharedSuccess, testCase.output, "")
		}
		scenarios = append(scenarios, scenario)
	}
	return scenarios
}
