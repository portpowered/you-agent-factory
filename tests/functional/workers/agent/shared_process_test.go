package agent_test

import (
	"context"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
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
	agentSharedProcessTimeout   = 20 * time.Second
	agentForcedCleanupChildEnv  = "YOU_AGENT_FORCED_CLEANUP_CHILD"
	agentForcedCleanupReportEnv = "YOU_AGENT_FORCED_CLEANUP_REPORT"
	agentFailureMessage         = "Codex authentication failed."
	agentCancellationMessage    = "provider invocation was canceled"
	agentTimeoutMessage         = "provider invocation timed out"
)

// TestAgentSharedProcess keeps the four existing agent rows on one immutable
// root-built process. Inert composition is observed before Process.Execute is
// activated; the remaining rows use distinct explicit Factory Sessions and
// immutable provider-command routes.
func TestAgentSharedProcess(t *testing.T) {
	if os.Getenv(agentForcedCleanupChildEnv) == "1" {
		runAgentForcedCleanupChild(t)
		return
	}

	fixture := newAgentSharedProcessFixture(t)

	t.Run("Inert", func(t *testing.T) {
		fixture.assertInert(t)
	})

	for _, scenario := range fixture.scenarios {
		if scenario.name != "Invalid" {
			continue
		}
		t.Run(scenario.name, func(t *testing.T) {
			t.Run("UnknownProvider", func(t *testing.T) {
				fixture.assertUnknownProvider(t, scenario)
			})
			t.Run("MalformedConfiguration", func(t *testing.T) {
				fixture.assertMalformedConfiguration(t)
			})
		})
	}

	fixture.start(t)
	for _, scenario := range fixture.scenarios {
		scenario := scenario
		if scenario.name == "Invalid" {
			continue
		}
		t.Run(scenario.name, func(t *testing.T) {
			fixture.runScenario(t, scenario)
		})
	}
	t.Run("Cleanup", func(t *testing.T) {
		runAgentForcedCleanupParent(t)
	})
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

	processBuilds   atomic.Int32
	apiStarts       atomic.Int32
	processClosed   atomic.Bool
	processCloseMu  sync.Mutex
	processCloseErr string
	sessionsMu      sync.Mutex
	opened          map[string]string
	closed          map[string]struct{}
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

func newAgentSharedProcessFixture(t *testing.T, scenarioNames ...string) *agentSharedProcessFixture {
	t.Helper()

	scenarios := newAgentSharedScenarios(t, scenarioNames...)
	if len(scenarios) == 0 {
		t.Fatal("agent shared fixture has no selected scenarios")
	}
	// The default daemon session is idle; reuse the first characterized
	// scenario's Factory Definition instead of provisioning a second identical
	// host directory. Explicit scenario Sessions remain fresh and isolated by
	// their session identities, while the routed provider configuration stays
	// immutable for each directory.
	hostDir := scenarios[0].factoryDir
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
			fixture.apiStarts.Add(1)
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
	fixture.processBuilds.Add(1)
	t.Cleanup(func() { fixture.close(t) })
	return fixture
}

func newAgentSharedFactoryDir(t *testing.T) string {
	t.Helper()

	return support.ScaffoldFactory(t, map[string]any{
		"name": "agent-shared",
		"workTypes": []any{
			map[string]any{
				"name": "task",
				"states": []any{
					map[string]any{"name": "init", "type": "INITIAL"},
					map[string]any{"name": "done", "type": "TERMINAL"},
					map[string]any{"name": "failed", "type": "FAILED"},
				},
			},
		},
		"workers": []any{
			map[string]any{"name": "worker"},
		},
		"workstations": []map[string]any{
			{
				"name":   "process",
				"worker": "worker",
				"inputs": []any{
					map[string]any{"workType": "task", "state": "init"},
				},
				"outputs": []any{
					map[string]any{"workType": "task", "state": "done"},
				},
				"onFailure": []any{
					map[string]any{"workType": "task", "state": "failed"},
				},
			},
		},
	})
}

func newAgentSharedScenarios(t *testing.T, scenarioNames ...string) []agentSharedScenario {
	t.Helper()
	cases := agentSharedScenarioSpecs()
	selected := make(map[string]struct{}, len(scenarioNames))
	for _, name := range scenarioNames {
		selected[name] = struct{}{}
	}

	scenarios := make([]agentSharedScenario, 0, len(cases))
	for _, testCase := range cases {
		if len(selected) > 0 {
			if _, ok := selected[testCase.name]; !ok {
				continue
			}
		}
		dir := newAgentSharedFactoryDir(t)
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
