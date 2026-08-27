package codex

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

type codexWorktreeFixture struct {
	process    support.ApplicationProcess
	command    *support.ProcessCommand
	baseURL    string
	hostDir    string
	apiStopped <-chan struct{}
	router     *codexCommandRouter
	identities *codexIdentityGenerator
	apiStarts  *atomic.Int32
	scenarios  []codexWorktreeScenario
	opened     atomic.Int32
	closed     atomic.Int32

	ledgerMu sync.Mutex
	ledger   map[string]codexWorktreeObservation
}

type codexWorktreeScenario struct {
	name         string
	repoRoot     string
	factoryDir   string
	workName     string
	workID       string
	requestID    string
	traceID      string
	checkoutPath string
	runner       *codexScenarioCommandRunner
}

type codexWorktreeObservation struct {
	sessionID    string
	workID       string
	requestID    string
	checkoutPath string
	worktree     string
}

type codexWorktreeCase struct {
	name                string
	seedClaudeWorktrees bool
	wantParentRel       string
	workName            string
	workID              string
	requestID           string
	traceID             string
}

// TestCodexWorktreeWorkstationDispatch_MaterializesCheckoutAndOmitsCLIWorktreeFlag
// proves both local-real Git parent layouts through one root-built process. Each
// scenario owns a separate repository and opens an explicit non-default Factory
// Session so process wiring is shared while runtime and checkout state remain
// isolated.
func TestCodexWorktreeWorkstationDispatch_MaterializesCheckoutAndOmitsCLIWorktreeFlag(t *testing.T) {
	fixture := newCodexWorktreeFixture(t)
	t.Cleanup(func() {
		fixture.assertSharedIdentityLedger(t)
	})

	for _, scenario := range fixture.scenarios {
		scenario := scenario
		t.Run(scenario.name, func(t *testing.T) {
			t.Parallel()
			fixture.runScenario(t, scenario)
		})
	}
	t.Cleanup(func() {
		fixture.assertSharedProcessCleanup(t)
	})
}

func newCodexWorktreeFixture(t *testing.T) *codexWorktreeFixture {
	t.Helper()

	identities := &codexIdentityGenerator{}
	scenarios := newCodexWorktreeScenarios(t)
	routes := make([]codexCommandRoute, 0, len(scenarios))
	for _, scenario := range scenarios {
		routes = append(routes, codexCommandRoute{
			selector: scenario.checkoutPath,
			label:    scenario.name,
			runner:   scenario.runner,
		})
	}
	router, err := newCodexCommandRouter(routes)
	if err != nil {
		t.Fatalf("newCodexCommandRouter: %v", err)
	}

	hostDir := newCodexHostDir(t)
	process, command, apiStopped, apiStarts, baseURL := newCodexProcess(
		t,
		hostDir,
		router,
		identities,
	)
	return &codexWorktreeFixture{
		process:    process,
		command:    command,
		baseURL:    baseURL,
		hostDir:    hostDir,
		apiStopped: apiStopped,
		router:     router,
		identities: identities,
		apiStarts:  apiStarts,
		scenarios:  scenarios,
		ledger:     make(map[string]codexWorktreeObservation, len(scenarios)),
	}
}

func newCodexWorktreeScenarios(t *testing.T) []codexWorktreeScenario {
	t.Helper()

	cases := []codexWorktreeCase{
		{
			name:          "DefaultDotWorktreesParent",
			wantParentRel: ".worktrees",
			workName:      "codex-worktree-feature-default",
			workID:        "codex-c04-worktree-default-work",
			requestID:     "codex-c04-worktree-default-request",
			traceID:       "codex-c04-worktree-default-trace",
		},
		{
			name:                "ExistingClaudeWorktreesParent",
			seedClaudeWorktrees: true,
			wantParentRel:       ".claude/worktrees",
			workName:            "codex-worktree-feature-claude",
			workID:              "codex-c04-worktree-claude-work",
			requestID:           "codex-c04-worktree-claude-request",
			traceID:             "codex-c04-worktree-claude-trace",
		},
	}

	scenarios := make([]codexWorktreeScenario, 0, len(cases))
	for _, fixture := range cases {
		scenarios = append(scenarios, newCodexWorktreeScenario(t, fixture))
	}
	return scenarios
}

func newCodexWorktreeScenario(t *testing.T, fixture codexWorktreeCase) codexWorktreeScenario {
	t.Helper()

	repoRoot := initGitRepositoryForCodexWorktreeFunctionalTest(t)
	factoryDir := filepath.Join(repoRoot, "factory")
	if err := os.MkdirAll(factoryDir, 0o755); err != nil {
		t.Fatalf("create factory dir: %v", err)
	}
	if fixture.seedClaudeWorktrees {
		if err := os.MkdirAll(filepath.Join(factoryDir, ".claude", "worktrees"), 0o755); err != nil {
			t.Fatalf("seed .claude/worktrees: %v", err)
		}
	}

	writeCodexWorktreeFactoryConfig(t, factoryDir)
	support.WriteAgentConfig(t, factoryDir, "worker-a", `---
type: MODEL_WORKER
modelProvider: codex
model: test-model
stopToken: COMPLETE
---
Process the input task.
`)
	writeCodexWorktreeWorkstationAgents(t, factoryDir)
	testutil.WriteSeedRequest(t, factoryDir, work.SubmitRequest{
		RequestID:  fixture.requestID,
		Name:       fixture.workName,
		WorkID:     fixture.workID,
		WorkTypeID: "task",
		TraceID:    fixture.traceID,
		Payload:    []byte("codex worktree workstation payload"),
	})

	checkoutPath := filepath.Join(factoryDir, filepath.FromSlash(fixture.wantParentRel), fixture.workName)
	runner := newCodexScenarioCommandRunner([]platformprocess.CommandResult{{
		Stdout: []byte(
			`{"type":"item.completed","item":{"id":"message-final","type":"agent_message","text":"Done. COMPLETE"}}` + "\n",
		),
	}}, nil)
	return codexWorktreeScenario{
		name:         fixture.name,
		repoRoot:     repoRoot,
		factoryDir:   factoryDir,
		workName:     fixture.workName,
		workID:       fixture.workID,
		requestID:    fixture.requestID,
		traceID:      fixture.traceID,
		checkoutPath: checkoutPath,
		runner:       runner,
	}
}

func (fixture *codexWorktreeFixture) runScenario(
	t *testing.T,
	scenario codexWorktreeScenario,
) {
	t.Helper()

	opened := support.OpenFactorySessionAt(t, fixture.baseURL, scenario.factoryDir)
	if opened.Session == nil {
		t.Fatalf("%s open response missing session: %#v", scenario.name, opened)
	}
	sessionID := opened.Session.Id
	if sessionID == "" || sessionID == factorysessions.DefaultSessionID {
		t.Fatalf("%s session id = %q, want unique non-default explicit session", scenario.name, sessionID)
	}
	fixture.opened.Add(1)

	closed := false
	closeSession := func() {
		if closed {
			return
		}
		support.CloseFactorySessionAt(t, fixture.baseURL, sessionID)
		closed = true
		fixture.closed.Add(1)
	}
	worktreeCleaned := false
	t.Cleanup(func() {
		// A failed assertion may leave the gated provider call waiting. Release it
		// before attempting session or process cleanup so the shared process cannot
		// retain a worker goroutine or a Git checkout.
		scenario.runner.Release()
		closeSession()
		if !worktreeCleaned {
			cleanupCodexWorktreeScenario(t, scenario)
		}
	})

	// The command edge is released only after the explicit session exists. This
	// keeps concurrent variants order-independent while retaining local-real Git.
	scenario.runner.Release()
	support.WaitForSessionTerminalStatus(t, fixture.baseURL, sessionID, codexConductorRunTimeout)
	listed := listCodexSessionWork(t, fixture.baseURL, sessionID)
	events := support.GetFactoryEventsForSessionAt(t, fixture.baseURL, sessionID)
	assertCodexWorktreeWorkCompleted(t, scenario, listed)
	assertCodexWorktreeCommand(t, fixture.router, scenario)
	request := assertCodexWorktreeEvents(t, scenario, sessionID, events)

	if _, err := os.Stat(scenario.checkoutPath); err != nil {
		t.Fatalf("materialized checkout missing at %s: %v", scenario.checkoutPath, err)
	}
	if got := support.StringPointerValue(request.Worktree); got != scenario.workName {
		t.Fatalf("model request worktree = %q, want %q", got, scenario.workName)
	}
	if got := support.StringPointerValue(request.WorkingDirectory); got != scenario.checkoutPath {
		t.Fatalf("model request workingDirectory = %q, want %q", got, scenario.checkoutPath)
	}

	closeSession()
	assertCodexSessionDeleted(t, fixture.baseURL, sessionID)
	cleanupCodexWorktreeScenario(t, scenario)
	worktreeCleaned = true
	fixture.recordObservation(codexWorktreeObservation{
		sessionID:    sessionID,
		workID:       scenario.workID,
		requestID:    scenario.requestID,
		checkoutPath: scenario.checkoutPath,
		worktree:     scenario.workName,
	})
}

func (fixture *codexWorktreeFixture) recordObservation(observation codexWorktreeObservation) {
	fixture.ledgerMu.Lock()
	defer fixture.ledgerMu.Unlock()
	fixture.ledger[observation.requestID] = observation
}

func (fixture *codexWorktreeFixture) assertSharedIdentityLedger(t *testing.T) {
	t.Helper()

	fixture.ledgerMu.Lock()
	observations := make([]codexWorktreeObservation, 0, len(fixture.ledger))
	for _, observation := range fixture.ledger {
		observations = append(observations, observation)
	}
	fixture.ledgerMu.Unlock()
	if len(observations) == 0 {
		t.Fatal("shared worktree-process scenario observations are empty")
	}

	seenSessions := make(map[string]string, len(observations))
	seenWorks := make(map[string]string, len(observations))
	seenRequests := make(map[string]string, len(observations))
	seenCheckouts := make(map[string]string, len(observations))
	seenWorktrees := make(map[string]string, len(observations))
	for _, observation := range observations {
		assertCodexUniqueIdentity(t, seenSessions, observation.sessionID, observation.requestID, "Factory Session")
		assertCodexUniqueIdentity(t, seenWorks, observation.workID, observation.requestID, "Work")
		assertCodexUniqueIdentity(t, seenRequests, observation.requestID, observation.requestID, "request")
		assertCodexUniqueIdentity(t, seenCheckouts, observation.checkoutPath, observation.requestID, "checkout")
		assertCodexUniqueIdentity(t, seenWorktrees, observation.worktree, observation.requestID, "worktree")
	}

	if got := fixture.opened.Load(); got != fixture.closed.Load() {
		t.Fatalf("Factory Session opens = %d, closes = %d", got, fixture.closed.Load())
	}
	if got := fixture.apiStarts.Load(); got != 1 {
		t.Fatalf("API server starts = %d, want exactly one shared process server", got)
	}
	for _, scenario := range fixture.scenarios {
		if got := scenario.runner.ActiveCallCount(); got != 0 {
			t.Fatalf("%s active Codex command calls after scenario cleanup = %d, want 0", scenario.name, got)
		}
	}
	if len(observations) != len(fixture.scenarios) {
		// An anchored subtest intentionally exercises only the selected route; the
		// complete parent invocation proves both local-real variants together.
		return
	}
	if got := fixture.opened.Load(); got != int32(len(fixture.scenarios)) {
		t.Fatalf("Factory Session opens = %d, want %d", got, len(fixture.scenarios))
	}
	if got := fixture.identities.sessionCount(); got < uint64(len(fixture.scenarios)) {
		t.Fatalf("Factory Session IDs generated = %d, want at least %d explicit sessions", got, len(fixture.scenarios))
	}
	if got := fixture.router.callCount(); got != len(fixture.scenarios) {
		t.Fatalf("shared process routed provider calls = %d, want %d", got, len(fixture.scenarios))
	}
}

func (fixture *codexWorktreeFixture) assertSharedProcessCleanup(t *testing.T) {
	t.Helper()

	closeCtx, cancel := context.WithTimeout(context.Background(), codexConductorRunTimeout)
	defer cancel()
	fixture.command.Stop(t)
	if err := fixture.process.Close(closeCtx); err != nil {
		t.Fatalf("close shared Codex worktree application process: %v", err)
	}
	select {
	case <-fixture.apiStopped:
	case <-time.After(codexConductorRunTimeout):
		t.Fatal("shared Codex worktree API server did not close after process cleanup")
	}
	for _, scenario := range fixture.scenarios {
		if got := scenario.runner.ActiveCallCount(); got != 0 {
			t.Fatalf("%s active Codex command calls after process cleanup = %d, want 0", scenario.name, got)
		}
		cleanupCodexWorktreeScenario(t, scenario)
	}
	removeCodexWorktreePath(t, fixture.hostDir)
}

func assertCodexWorktreeWorkCompleted(
	t *testing.T,
	scenario codexWorktreeScenario,
	listed factoryapi.ListWorkResponse,
) {
	t.Helper()

	for _, location := range []struct {
		location string
		want     int
	}{
		{location: "task:complete", want: 1},
		{location: "task:init", want: 0},
		{location: "task:failed", want: 0},
	} {
		if got := support.CountWorkAtCustomerState(listed, location.location); got != location.want {
			t.Fatalf("%s %s token count = %d, want %d", scenario.name, location.location, got, location.want)
		}
	}

	found := 0
	for _, item := range listed.Results {
		if support.StringPointerValue(item.WorkId) != scenario.workID {
			continue
		}
		found++
		if support.StringPointerValue(item.RequestId) != scenario.requestID {
			t.Fatalf("%s Work request id = %q, want %q", scenario.name, support.StringPointerValue(item.RequestId), scenario.requestID)
		}
		if support.StringPointerValue(item.TraceId) != scenario.traceID {
			t.Fatalf("%s Work trace id = %q, want %q", scenario.name, support.StringPointerValue(item.TraceId), scenario.traceID)
		}
	}
	if found != 1 {
		t.Fatalf("%s Work identity count = %d, want exactly one %q", scenario.name, found, scenario.workID)
	}
}

func assertCodexWorktreeCommand(
	t *testing.T,
	router *codexCommandRouter,
	scenario codexWorktreeScenario,
) {
	t.Helper()

	requests := scenario.runner.Requests()
	if len(requests) != 1 {
		t.Fatalf("%s routed provider calls = %d, want 1; requests=%#v", scenario.name, len(requests), requests)
	}
	routed := router.callsFor(scenario.checkoutPath)
	if len(routed) != len(requests) {
		t.Fatalf("%s immutable route calls = %d, runner calls = %d", scenario.name, len(routed), len(requests))
	}
	for index, routedCall := range routed {
		request := routedCall.request
		if request.WorkDir != requests[index].WorkDir {
			t.Fatalf("%s router WorkDir = %q, runner WorkDir = %q", scenario.name, request.WorkDir, requests[index].WorkDir)
		}
		if request.Command != string(modelprovider.ProviderCodex) {
			t.Fatalf("%s command = %q, want %q", scenario.name, request.Command, modelprovider.ProviderCodex)
		}
		if request.WorkDir != scenario.checkoutPath {
			t.Fatalf("%s command WorkDir = %q, want materialized checkout %q", scenario.name, request.WorkDir, scenario.checkoutPath)
		}
		assertArgsDoNotContain(t, request.Args, "--worktree")
		support.AssertArgsContainSequence(t, request.Args, []string{"exec", "--json", "--model", "test-model", "-"})
	}
}

func assertCodexWorktreeEvents(
	t *testing.T,
	scenario codexWorktreeScenario,
	sessionID string,
	events []factoryapi.FactoryEvent,
) factoryapi.ModelRequestEventPayload {
	t.Helper()
	if len(events) == 0 {
		t.Fatalf("%s Factory Event stream is empty", scenario.name)
	}

	var request factoryapi.ModelRequestEventPayload
	foundRequest := false
	for _, event := range events {
		if event.Context.SessionId != nil && *event.Context.SessionId != sessionID {
			t.Fatalf("%s event %q escaped Factory Session %q", scenario.name, event.Id, sessionID)
		}
		if event.Context.RequestId != nil && *event.Context.RequestId != scenario.requestID {
			t.Fatalf("%s event %q request id = %q, want %q", scenario.name, event.Id, *event.Context.RequestId, scenario.requestID)
		}
		if event.Context.TraceIds != nil {
			for _, traceID := range *event.Context.TraceIds {
				if traceID != scenario.traceID {
					t.Fatalf("%s event %q trace id = %q, want %q", scenario.name, event.Id, traceID, scenario.traceID)
				}
			}
		}
		if event.Context.WorkIds != nil {
			for _, workID := range *event.Context.WorkIds {
				if workID != scenario.workID {
					t.Fatalf("%s event %q Work id = %q, want %q", scenario.name, event.Id, workID, scenario.workID)
				}
			}
		}
		if event.Type != factoryapi.FactoryEventTypeModelRequest {
			continue
		}
		if foundRequest {
			t.Fatalf("%s has multiple model request events", scenario.name)
		}
		decoded, err := event.Payload.AsModelRequestEventPayload()
		if err != nil {
			t.Fatalf("%s decode model request payload: %v", scenario.name, err)
		}
		request = decoded
		foundRequest = true
	}
	if !foundRequest {
		t.Fatalf("%s events missing %s: %v", scenario.name, factoryapi.FactoryEventTypeModelRequest, codexWorktreeEventTypes(events))
	}
	return request
}

func cleanupCodexWorktreeScenario(t *testing.T, scenario codexWorktreeScenario) {
	t.Helper()

	if _, err := os.Stat(scenario.checkoutPath); err == nil {
		cmd := exec.Command("git", "worktree", "remove", "--force", scenario.checkoutPath)
		cmd.Dir = scenario.repoRoot
		output, runErr := cmd.CombinedOutput()
		if runErr != nil {
			t.Errorf(
				"remove Codex worktree %q from %q: %v\n%s",
				scenario.checkoutPath,
				scenario.repoRoot,
				runErr,
				strings.TrimSpace(string(output)),
			)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("inspect Codex worktree %q during cleanup: %v", scenario.checkoutPath, err)
	}
	if _, err := os.Stat(scenario.checkoutPath); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("Codex worktree %q still exists after cleanup: %v", scenario.checkoutPath, err)
	}
	removeCodexWorktreePath(t, scenario.repoRoot)
}

func removeCodexWorktreePath(t *testing.T, path string) {
	t.Helper()
	if err := os.RemoveAll(path); err != nil {
		t.Errorf("remove test-owned Codex path %q: %v", path, err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("test-owned Codex path %q still exists after cleanup: %v", path, err)
	}
}

func writeCodexWorktreeWorkstationAgents(t *testing.T, factoryDir string) {
	t.Helper()

	path := filepath.Join(factoryDir, "workstations", "process", "AGENTS.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create workstation dir: %v", err)
	}
	content := "---\ntype: MODEL_WORKSTATION\n---\nProcess {{ (index .Inputs 0).Name }}.\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write workstation AGENTS.md: %v", err)
	}
}

func writeCodexWorktreeFactoryConfig(t *testing.T, factoryDir string) {
	t.Helper()

	config := `{
  "name": "codex_worktree_workstation",
  "workTypes": [
    {
      "name": "task",
      "states": [
        { "name": "init", "type": "INITIAL" },
        { "name": "complete", "type": "TERMINAL" },
        { "name": "failed", "type": "FAILED" }
      ]
    }
  ],
  "workers": [
    { "name": "worker-a" }
  ],
  "workstations": [
    {
      "name": "process",
      "worker": "worker-a",
      "inputs": [{ "workType": "task", "state": "init" }],
      "outputs": [{ "workType": "task", "state": "complete" }],
      "onFailure": [{ "workType": "task", "state": "failed" }],
      "worktree": "{{ (index .Inputs 0).Name }}"
    }
  ]
}
`
	path := filepath.Join(factoryDir, "factory.json")
	if err := os.WriteFile(path, []byte(config), 0o644); err != nil {
		t.Fatalf("write factory.json: %v", err)
	}
}

func initGitRepositoryForCodexWorktreeFunctionalTest(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available in PATH")
	}

	repoRoot := t.TempDir()
	runGitForCodexWorktreeFunctionalTest(t, repoRoot, "init")
	runGitForCodexWorktreeFunctionalTest(t, repoRoot, "config", "user.email", "codex-worktree-functional@example.com")
	runGitForCodexWorktreeFunctionalTest(t, repoRoot, "config", "user.name", "codex worktree functional")
	runGitForCodexWorktreeFunctionalTest(t, repoRoot, "commit", "--allow-empty", "-m", "init")
	return repoRoot
}

func runGitForCodexWorktreeFunctionalTest(t *testing.T, dir string, args ...string) {
	t.Helper()

	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s in %s: %v\n%s", strings.Join(args, " "), dir, err, output)
	}
}

func assertArgsDoNotContain(t *testing.T, args []string, forbidden ...string) {
	t.Helper()

	for _, arg := range args {
		for _, item := range forbidden {
			if arg == item {
				t.Fatalf("args = %#v, want to omit %q", args, item)
			}
		}
	}
}

func codexWorktreeEventTypes(events []factoryapi.FactoryEvent) []factoryapi.FactoryEventType {
	types := make([]factoryapi.FactoryEventType, 0, len(events))
	for _, event := range events {
		types = append(types, event.Type)
	}
	return types
}
