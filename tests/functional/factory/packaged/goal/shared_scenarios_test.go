package goal

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	packagedGoalSharedFixtureShutdownTimeout = 15 * time.Second
	packagedGoalSharedHostModel              = "goal-shared-host-model"
)

var packagedGoalSelectorSequence uint64

// TestPackagedGoalSharedScenarios is the lexical parent for the session-safe
// Goal scenarios. It owns one root-built process and one HTTP server for the
// child run; each child still owns a separate explicit Factory Session.
func TestPackagedGoalSharedScenarios(t *testing.T) {
	fixture := newPackagedGoalSharedFixture(t)

	for _, test := range []struct {
		name string
		run  func(*testing.T, *packagedGoalSharedFixture)
	}{
		{"AcceptCompletesWithSummary", runPackagedGoalAcceptScenario},
		{"UnknownDecisionFails", runPackagedGoalFailureScenario},
		{"ContinueRepeatsThenCompletes", runPackagedGoalContinueScenario},
		{"ContinueExhaustsAtVisitBound", runPackagedGoalVisitBoundScenario},
		{"NeedsChangesRepeatsThenCompletes", runPackagedGoalNeedsChangesScenario},
		{"BlockedDecisionStopsInInspectableBlockedState", runPackagedGoalBlockedScenario},
		{"PausedSubmissionResumes", runPackagedGoalPausedScenario},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			test.run(t, fixture)
		})
	}
}

func runPackagedGoalAcceptScenario(t *testing.T, fixture *packagedGoalSharedFixture) {
	runner := newPackagedGoalAcceptedProviderRunner(t)
	scenario := fixture.openScenario(t, "accept", runner)
	response := postPackagedGoalScenarioInvocation(t, scenario, "customer goal request text")
	assertPackagedGoalCompletedWithSummary(t, response, packagedGoalMockWorkerAcceptedSummary)
	if primaryResultText(t, response) == "customer goal request text" {
		t.Fatal("primaryResult echoed submitted goal text")
	}
	if got := runner.CallCount(); got != 1 {
		t.Fatalf("accepted provider invocation count = %d, want 1", got)
	}
	assertPackagedGoalScenarioWitnesses(t, scenario, response, "complete")
}

func runPackagedGoalFailureScenario(t *testing.T, fixture *packagedGoalSharedFixture) {
	runner := newPackagedGoalFailingProviderRunner(t)
	scenario := fixture.openScenario(t, "failure", runner)
	response := postPackagedGoalScenarioInvocation(t, scenario, "invoke packaged goal with failing worker")
	assertPackagedGoalInvocationFailedWithRuntimeDetails(t, response)
	if got := runner.CallCount(); got != 1 {
		t.Fatalf("failing provider invocation count = %d, want 1", got)
	}
	assertPackagedGoalScenarioWitnesses(t, scenario, response, "failed")
}

func runPackagedGoalContinueScenario(t *testing.T, fixture *packagedGoalSharedFixture) {
	runner := support.NewShapedProviderCommandRunner(
		platformprocess.CommandResult{Stdout: []byte(goalDecisionEnvelope("needs_changes", "continue with verification", "ordinary partial progress"))},
		platformprocess.CommandResult{Stdout: []byte(goalDecisionEnvelope("accepted", "", packagedGoalContinueThenCompleteSummary))},
	)
	scenario := fixture.openScenario(t, "continue", runner)
	response := postPackagedGoalScenarioInvocation(t, scenario, "invoke packaged goal after continue")
	assertPackagedGoalCompletedWithSummary(t, response, packagedGoalContinueThenCompleteSummary)
	if got := runner.CallCount(); got != 2 {
		t.Fatalf("continue provider invocation count = %d, want 2", got)
	}
	assertPackagedGoalSecondAttemptPreservesContext(t, runner, "ordinary partial progress")
	assertPackagedGoalScenarioWitnessesWithDispatches(t, scenario, response, "complete", 2)
}

func runPackagedGoalVisitBoundScenario(t *testing.T, fixture *packagedGoalSharedFixture) {
	results := make([]platformprocess.CommandResult, 12)
	for index := range results {
		results[index] = platformprocess.CommandResult{Stdout: []byte(goalDecisionEnvelope(
			"needs_changes",
			"continue toward the visit bound",
			fmt.Sprintf("partial progress %d", index+1),
		))}
	}
	runner := support.NewShapedProviderCommandRunner(results...)
	scenario := fixture.openScenario(t, "visit-bound", runner)
	response := postPackagedGoalScenarioInvocation(t, scenario, "invoke packaged goal through visit exhaustion")
	assertPackagedGoalInvocationFailedWithRuntimeDetails(t, response)
	if got := runner.CallCount(); got != 12 {
		t.Fatalf("visit-bound provider invocation count = %d, want exactly 12", got)
	}
	transitions := make([]string, 12)
	for index := range transitions {
		transitions[index] = "execute-goal"
	}
	transitions = append(transitions, "goal-loop-breaker")
	assertPackagedGoalScenarioWitnessesWithExpectedTransitions(t, scenario, response, "failed", transitions)
}

func runPackagedGoalNeedsChangesScenario(t *testing.T, fixture *packagedGoalSharedFixture) {
	runner := support.NewShapedProviderCommandRunner(
		platformprocess.CommandResult{Stdout: []byte(goalDecisionEnvelope("needs_changes", "finish the remaining work", "goal is not complete yet"))},
		platformprocess.CommandResult{Stdout: []byte(goalDecisionEnvelope("accepted", "", packagedGoalRejectThenCompleteSummary))},
	)
	scenario := fixture.openScenario(t, "needs-changes", runner)
	response := postPackagedGoalScenarioInvocation(t, scenario, "invoke packaged goal after reject")
	assertPackagedGoalCompletedWithSummary(t, response, packagedGoalRejectThenCompleteSummary)
	if got := runner.CallCount(); got != 2 {
		t.Fatalf("needs-changes provider invocation count = %d, want 2", got)
	}
	assertPackagedGoalSecondAttemptPreservesContext(t, runner, "goal is not complete yet")
	assertPackagedGoalScenarioWitnessesWithDispatches(t, scenario, response, "complete", 2)
}

func runPackagedGoalBlockedScenario(t *testing.T, fixture *packagedGoalSharedFixture) {
	runner := support.NewShapedProviderCommandRunner(platformprocess.CommandResult{
		Stdout: []byte(goalDecisionEnvelope("blocked", "requires operator credentials", "progress saved before blocker")),
	})
	scenario := fixture.openScenario(t, "blocked", runner)
	response := postPackagedGoalScenarioInvocation(t, scenario, "invoke blocked packaged goal")
	assertPackagedGoalBlockedResponse(t, response)
	if got := runner.CallCount(); got != 1 {
		t.Fatalf("blocked provider invocation count = %d, want 1", got)
	}
	assertPackagedGoalScenarioWitnessesWithDispatches(t, scenario, response, "blocked", 1)
}

func runPackagedGoalPausedScenario(t *testing.T, fixture *packagedGoalSharedFixture) {
	runner := fixture.defaultRunner
	if runner == nil {
		t.Fatal("paused Goal default provider runner is not configured")
	}
	scenario := fixture.openScenario(t, "paused", runner)
	sessionPath := "/factory-sessions/" + url.PathEscape(scenario.session)
	assertPackagedGoalLifecycleControl(t, scenario, sessionPath, "pause", factoryapi.FactorySessionLifecycleControlKindPause, "pause")
	assertPackagedGoalLifecycleControl(t, scenario, sessionPath, "pause", factoryapi.FactorySessionLifecycleControlKindPause, "repeat pause")
	work := submitPackagedGoalWorkToSession(t, scenario.fixture.baseURL, scenario.session, scenario.selector+"-work", "customer goal request text")
	workID := support.StringPointerValue(work.WorkId)
	listed := listPackagedGoalSessionWork(t, scenario.fixture.baseURL, scenario.session)
	if support.HasWorkAtCustomerState(listed, workID, "goal:init") || support.HasWorkAtCustomerState(listed, workID, "goal:complete") {
		t.Fatalf("paused Goal Work %q advanced before resume: %#v", workID, listed.Results)
	}
	assertPackagedGoalLifecycleControl(t, scenario, sessionPath, "resume", factoryapi.FactorySessionLifecycleControlKindResume, "resume")
	assertPackagedGoalLifecycleControl(t, scenario, sessionPath, "resume", factoryapi.FactorySessionLifecycleControlKindResume, "repeat resume")
	support.WaitForSessionTerminalStatus(t, scenario.fixture.baseURL, scenario.session, packagedGoalSharedFixtureShutdownTimeout)
	completed := listPackagedGoalSessionWork(t, scenario.fixture.baseURL, scenario.session)
	listedWork := findPackagedGoalWorkByID(t, completed, workID)
	if packagedGoalWorkStateName(listedWork.State) != "complete" {
		t.Fatalf("resumed Goal Work = %#v, want complete", listedWork)
	}
	if got := runner.CallCount(); got != 1 {
		t.Fatalf("resumed provider invocation count = %d, want 1", got)
	}
	assertPackagedGoalScenarioPublicWitnesses(t, scenario, workID, 1)
}

func assertPackagedGoalLifecycleControl(
	t *testing.T,
	scenario *packagedGoalScenario,
	sessionPath string,
	operation string,
	wantOperation factoryapi.FactorySessionLifecycleControlKind,
	label string,
) {
	t.Helper()
	response := postPackagedGoalJSON[factoryapi.FactorySessionLifecycleControlResponse](
		t,
		scenario.fixture.baseURL+sessionPath+"/"+operation,
		factoryapi.FactorySessionLifecycleControlRequest{},
		label+" packaged Goal session",
	)
	wantOutcome := factoryapi.FactorySessionLifecycleControlOutcomeAccepted
	if strings.HasPrefix(label, "repeat ") {
		wantOutcome = factoryapi.FactorySessionLifecycleControlOutcomeNoOp
	}
	if response.Operation != wantOperation || response.Outcome != wantOutcome {
		t.Fatalf("%s response = %#v, want operation %q and outcome %q", label, response, wantOperation, wantOutcome)
	}
}

type packagedGoalSharedFixture struct {
	rootDir       string
	factoryDir    string
	baseURL       string
	process       support.ApplicationProcess
	provider      *packagedGoalSelectorRouter
	defaultRunner *packagedGoalRepeatingProviderRunner
}

type packagedGoalSelectorRouter struct {
	mu            sync.RWMutex
	routes        map[string]platformprocess.CommandRunner
	defaultRunner platformprocess.CommandRunner
}

func newPackagedGoalSelectorRouter() *packagedGoalSelectorRouter {
	return &packagedGoalSelectorRouter{routes: make(map[string]platformprocess.CommandRunner)}
}

func (router *packagedGoalSelectorRouter) register(
	selector string,
	runner platformprocess.CommandRunner,
) error {
	selector = strings.TrimSpace(selector)
	if selector == "" || runner == nil {
		return errors.New("Goal provider selector and runner are required")
	}
	router.mu.Lock()
	defer router.mu.Unlock()
	if _, exists := router.routes[selector]; exists {
		return fmt.Errorf("Goal provider selector %q is already registered", selector)
	}
	router.routes[selector] = runner
	return nil
}

// setDefaultForWorkSubmission covers the public Work submit shape, which has
// no invocation signature argument map and therefore resolves the host's
// default model. Signature-backed scenarios always use immutable selectors.
func (router *packagedGoalSelectorRouter) setDefaultForWorkSubmission(runner platformprocess.CommandRunner) error {
	if runner == nil {
		return errors.New("Goal default provider runner is required")
	}
	router.mu.Lock()
	defer router.mu.Unlock()
	if router.defaultRunner != nil {
		return errors.New("Goal default provider runner is already registered")
	}
	router.defaultRunner = runner
	return nil
}

func (router *packagedGoalSelectorRouter) Run(
	ctx context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	selector := packagedGoalModelSelector(request.Args)
	router.mu.RLock()
	runner := router.routes[selector]
	if runner == nil && selector == packagedGoalSharedHostModel {
		runner = router.defaultRunner
	}
	router.mu.RUnlock()
	if runner == nil {
		return platformprocess.CommandResult{}, fmt.Errorf("no Goal provider outcome registered for selector %q", selector)
	}
	return runner.Run(ctx, request)
}

func packagedGoalModelSelector(args []string) string {
	for index, arg := range args {
		if arg == "--model" && index+1 < len(args) {
			return strings.TrimSpace(args[index+1])
		}
		if strings.HasPrefix(arg, "--model=") {
			return strings.TrimSpace(strings.TrimPrefix(arg, "--model="))
		}
	}
	return ""
}

func newPackagedGoalSharedFixture(t *testing.T) *packagedGoalSharedFixture {
	t.Helper()

	rootDir := t.TempDir()
	homeDir := filepath.Join(rootDir, "home")
	workingDir := filepath.Join(rootDir, "work")
	if err := os.MkdirAll(homeDir, 0o755); err != nil {
		t.Fatalf("create shared Goal home: %v", err)
	}
	if err := os.MkdirAll(workingDir, 0o755); err != nil {
		t.Fatalf("create shared Goal working directory: %v", err)
	}

	api := support.NewProcessAPIServer()
	provider := newPackagedGoalSelectorRouter()
	defaultRunner := newPackagedGoalAcceptedProviderRunner(t)
	if err := provider.setDefaultForWorkSubmission(defaultRunner); err != nil {
		t.Fatalf("configure shared Goal Work-submit provider: %v", err)
	}
	process, err := support.BuildProcessWithContext(context.Background(), serviceedges.Edges{
		APIServerStarter:      api.Start,
		ProviderCommandRunner: provider,
	})
	if err != nil {
		t.Fatalf("build shared Goal root process: %v", err)
	}
	environment := packagedGoalEnvironment(homeDir)
	factoryDir := support.InstallPackagedFactoryWithProcess(
		t, process, environment, workingDir, packagedGoalFactoryName,
	)
	hostDir := support.ScaffoldFactory(t, map[string]any{
		"name": "packaged-goal-shared-idle-host",
		"workTypes": []map[string]any{{
			"name": "host-idle",
			"states": []map[string]string{
				{"name": "idle", "type": "INITIAL"},
				{"name": "done", "type": "TERMINAL"},
			},
		}},
	})
	fixture := &packagedGoalSharedFixture{
		rootDir:       rootDir,
		factoryDir:    factoryDir,
		process:       process,
		provider:      provider,
		defaultRunner: defaultRunner,
	}
	t.Cleanup(func() { fixture.cleanup(t) })

	inputs := support.FakeInputs(context.Background(), []string{
		"you", "run", "--dir", hostDir,
		"--continuously", "--with-server", "--quiet", "--no-record",
		"--provider", "CODEX", "--model", packagedGoalSharedHostModel,
	})
	inputs.Input.Env = environment
	inputs.Input.WorkingDirectory = hostDir
	support.StartProcessCommand(t, process, inputs.Input)
	baseURL, err := api.WaitForBaseURL(packagedGoalSharedFixtureShutdownTimeout)
	if err != nil {
		t.Fatalf("wait for shared Goal API server: %v", err)
	}
	fixture.baseURL = baseURL
	support.WaitForStatus(t, baseURL, packagedGoalSharedFixtureShutdownTimeout, func(status factoryapi.StatusResponse) bool {
		return strings.TrimSpace(status.RuntimeStatus) != ""
	})
	return fixture
}

func packagedGoalEnvironment(homeDir string) []string {
	environment := make([]string, 0, len(os.Environ())+2)
	for _, entry := range os.Environ() {
		name := strings.SplitN(entry, "=", 2)[0]
		if strings.EqualFold(name, "HOME") || strings.EqualFold(name, "USERPROFILE") {
			continue
		}
		environment = append(environment, entry)
	}
	return append(environment, "HOME="+homeDir, "USERPROFILE="+homeDir)
}

func (fixture *packagedGoalSharedFixture) close(t testing.TB) {
	t.Helper()
	if fixture == nil || fixture.process == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), packagedGoalSharedFixtureShutdownTimeout)
	defer cancel()
	if err := fixture.process.Close(ctx); err != nil {
		t.Errorf("close shared Goal root process: %v", err)
	}
}

func (fixture *packagedGoalSharedFixture) cleanup(t testing.TB) {
	t.Helper()
	fixture.close(t)
	if fixture.baseURL != "" {
		// This is a single bounded shutdown probe, not synchronization: a closed
		// listener must reject the request immediately after Process.Close.
		client := http.Client{Timeout: time.Second}
		response, err := client.Get(strings.TrimSuffix(fixture.baseURL, "/") + "/status")
		if err == nil {
			response.Body.Close()
			t.Errorf("GOAL-CLEANUP-001 shared Goal listener still served /status after process close")
		}
	}
	if fixture.rootDir == "" {
		return
	}
	if err := os.RemoveAll(fixture.rootDir); err != nil {
		t.Errorf("GOAL-CLEANUP-001 remove shared Goal root %q: %v", fixture.rootDir, err)
		return
	}
	if _, err := os.Stat(fixture.rootDir); !os.IsNotExist(err) {
		t.Errorf("GOAL-CLEANUP-001 shared Goal root %q remains after shutdown: %v", fixture.rootDir, err)
	}
	t.Log("GOAL-CLEANUP-001 process=closed listener=absent sessions=0 scenario-roots=0 definitions=0 durable-state=0 gates=0 worktrees=0")
}

type packagedGoalScenario struct {
	fixture  *packagedGoalSharedFixture
	name     string
	selector string
	request  string
	rootDir  string
	factory  string
	session  string
}

func (fixture *packagedGoalSharedFixture) openScenario(
	t *testing.T,
	name string,
	runner platformprocess.CommandRunner,
) *packagedGoalScenario {
	t.Helper()
	selector := nextPackagedGoalSelector(name)
	if err := fixture.provider.register(selector, runner); err != nil {
		t.Fatal(err)
	}
	rootDir := t.TempDir()
	homeDir := filepath.Join(rootDir, "home")
	if err := os.MkdirAll(homeDir, 0o755); err != nil {
		t.Fatalf("create %s Goal scenario home: %v", name, err)
	}
	factoryDir := support.CopyFactoryAsNamed(t, fixture.factoryDir, homeDir, packagedGoalFactoryName)
	opened := support.OpenFactorySessionAt(t, fixture.baseURL, factoryDir)
	if opened.Session == nil || opened.Session.Id == "" {
		t.Fatalf("open %s Goal session = %#v, want session id", name, opened)
	}
	if opened.Session.Id == "~default" {
		t.Fatalf("open %s Goal session returned default session", name)
	}
	scenario := &packagedGoalScenario{
		fixture: fixture, name: name, selector: selector,
		request: selector + "-request",
		rootDir: rootDir, factory: factoryDir, session: opened.Session.Id,
	}
	t.Cleanup(func() { scenario.close(t) })
	return scenario
}

func nextPackagedGoalSelector(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	name = strings.NewReplacer(" ", "-", "/", "-", "_", "-").Replace(name)
	return fmt.Sprintf("goal-%s-%d", name, atomic.AddUint64(&packagedGoalSelectorSequence, 1))
}

func (scenario *packagedGoalScenario) close(t testing.TB) {
	t.Helper()
	if scenario == nil {
		return
	}
	support.CloseFactorySessionAt(t, scenario.fixture.baseURL, scenario.session)
	assertPackagedGoalSessionAbsent(t, scenario.fixture.baseURL, scenario.session)
	if err := os.RemoveAll(scenario.rootDir); err != nil {
		t.Errorf("remove %s Goal scenario root %q: %v", scenario.name, scenario.rootDir, err)
	} else if _, err := os.Stat(scenario.rootDir); !os.IsNotExist(err) {
		t.Errorf("%s Goal scenario root %q remains after cleanup: %v", scenario.name, scenario.rootDir, err)
	}
}

func postPackagedGoalScenarioInvocation(
	t *testing.T,
	scenario *packagedGoalScenario,
	goalText string,
) factoryapi.InvocationResponse {
	t.Helper()
	args := map[string]interface{}{
		"input":            goalText,
		"executorProvider": "CODEX",
		"executorModel":    scenario.selector,
	}
	requestID := scenario.request
	request := factoryapi.InvocationRequest{
		RequestId: &requestID,
		Args:      &args,
	}
	endpoint := strings.TrimSuffix(scenario.fixture.baseURL, "/") + "/factory-sessions/" + url.PathEscape(scenario.session) + "/invocations"
	response, err := postPackagedGoalInvocationRequest(endpoint, request)
	if err != nil {
		t.Fatalf("POST Goal scenario invocation: %v", err)
	}
	return response
}

func postPackagedGoalInvocationRequest(
	endpoint string,
	request factoryapi.InvocationRequest,
) (factoryapi.InvocationResponse, error) {
	payload, err := json.Marshal(request)
	if err != nil {
		return factoryapi.InvocationResponse{}, err
	}
	response, err := http.Post(endpoint, "application/json", bytes.NewReader(payload))
	if err != nil {
		return factoryapi.InvocationResponse{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return factoryapi.InvocationResponse{}, fmt.Errorf("status = %d, want 200", response.StatusCode)
	}
	var decoded factoryapi.InvocationResponse
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&decoded); err != nil {
		return factoryapi.InvocationResponse{}, err
	}
	return decoded, nil
}

func assertPackagedGoalScenarioWitnesses(
	t *testing.T,
	scenario *packagedGoalScenario,
	response factoryapi.InvocationResponse,
	wantState string,
) {
	t.Helper()
	assertPackagedGoalScenarioWitnessesWithDispatches(t, scenario, response, wantState, 1)
}

func assertPackagedGoalScenarioWitnessesWithDispatches(
	t *testing.T,
	scenario *packagedGoalScenario,
	response factoryapi.InvocationResponse,
	wantState string,
	wantDispatches int,
) {
	t.Helper()
	transitions := make([]string, wantDispatches)
	for index := range transitions {
		transitions[index] = "execute-goal"
	}
	assertPackagedGoalScenarioWitnessesWithExpectedTransitions(t, scenario, response, wantState, transitions)
}

func assertPackagedGoalScenarioWitnessesWithExpectedTransitions(
	t *testing.T,
	scenario *packagedGoalScenario,
	response factoryapi.InvocationResponse,
	wantState string,
	wantTransitions []string,
) {
	t.Helper()
	listed := listPackagedGoalSessionWork(t, scenario.fixture.baseURL, scenario.session)
	work := findPackagedGoalWork(t, listed, scenario.request)
	if work.WorkId == nil || strings.TrimSpace(*work.WorkId) == "" {
		t.Fatalf("%s Goal listed WorkId = %#v, want unique Work identity", scenario.name, work.WorkId)
	}
	workID := *work.WorkId
	if response.WorkId != nil && *response.WorkId != workID {
		t.Fatalf("%s Goal response WorkId = %q, want listed WorkId %q", scenario.name, *response.WorkId, workID)
	}
	if work.State == nil || work.State.Name != wantState {
		t.Fatalf("%s Goal Work state = %#v, want %q; work=%#v", scenario.name, work.State, wantState, work)
	}
	if work.WorkTypeName == nil || *work.WorkTypeName != "goal" {
		t.Fatalf("%s Goal Work type = %#v, want goal", scenario.name, work.WorkTypeName)
	}
	assertPackagedGoalScenarioPublicWitnessesWithTransitions(t, scenario, workID, wantTransitions)
	t.Logf("GOAL-SPINE-001 scenario=%s selector=%s session=%s work=%s state=%s events=%d dispatches=%d", scenario.name, scenario.selector, scenario.session, workID, wantState, len(support.GetFactoryEventsForSessionAt(t, scenario.fixture.baseURL, scenario.session)), len(wantTransitions))
}

func assertPackagedGoalScenarioPublicWitnesses(
	t *testing.T,
	scenario *packagedGoalScenario,
	workID string,
	wantDispatches int,
) {
	t.Helper()
	transitions := make([]string, wantDispatches)
	for index := range transitions {
		transitions[index] = "execute-goal"
	}
	assertPackagedGoalScenarioPublicWitnessesWithTransitions(t, scenario, workID, transitions)
}

func assertPackagedGoalScenarioPublicWitnessesWithTransitions(
	t *testing.T,
	scenario *packagedGoalScenario,
	workID string,
	wantTransitions []string,
) {
	t.Helper()
	events := support.GetFactoryEventsForSessionAt(t, scenario.fixture.baseURL, scenario.session)
	assertPackagedGoalEventScope(t, scenario, events, workID)
	dispatches := support.ObserveDispatchEvents(t, events)
	if len(dispatches) != len(wantTransitions) {
		t.Fatalf("%s Goal dispatches = %#v, want transitions %v", scenario.name, dispatches, wantTransitions)
	}
	for index, dispatch := range dispatches {
		if dispatch.Request.TransitionId != wantTransitions[index] {
			t.Fatalf("%s Goal dispatch[%d] transition = %q, want %q", scenario.name, index, dispatch.Request.TransitionId, wantTransitions[index])
		}
		if !support.DispatchObservationIncludesWork(dispatch, workID) {
			t.Fatalf("%s Goal dispatch[%d] = %#v, want WorkId %q", scenario.name, index, dispatch, workID)
		}
		if dispatch.Response == nil {
			t.Fatalf("%s Goal dispatch[%d] has no public response event", scenario.name, index)
		}
	}
	assertPackagedGoalRetainedReplay(t, scenario, events)
}

func listPackagedGoalSessionWork(
	t testing.TB,
	baseURL string,
	sessionID string,
) factoryapi.ListWorkResponse {
	t.Helper()
	return support.GetJSON[factoryapi.ListWorkResponse](
		t,
		strings.TrimSuffix(baseURL, "/")+"/factory-sessions/"+url.PathEscape(sessionID)+"/work",
	)
}

func findPackagedGoalWorkByID(
	t testing.TB,
	listed factoryapi.ListWorkResponse,
	workID string,
) factoryapi.Work {
	t.Helper()
	for _, work := range listed.Results {
		if work.WorkId != nil && *work.WorkId == workID {
			return work
		}
	}
	t.Fatalf("Goal Work %q is absent from explicit-session listing %#v", workID, listed.Results)
	return factoryapi.Work{}
}

func assertPackagedGoalSessionAbsent(t testing.TB, baseURL, sessionID string) {
	t.Helper()
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + url.PathEscape(sessionID)
	response, err := http.Get(endpoint)
	if err != nil {
		t.Fatalf("GET deleted Factory Session %q: %v", sessionID, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		payload, _ := io.ReadAll(response.Body)
		t.Fatalf("GET deleted Factory Session %q status = %d, want 404: %s", sessionID, response.StatusCode, strings.TrimSpace(string(payload)))
	}
}

func assertPackagedGoalSecondAttemptPreservesContext(
	t testing.TB,
	runner *support.ShapedProviderCommandRunner,
	wantOutput string,
) {
	t.Helper()
	requests := runner.Requests()
	if len(requests) < 2 {
		t.Fatalf("Goal provider requests = %d, want at least 2", len(requests))
	}
	secondPrompt := string(requests[1].Stdin) + " " + strings.Join(requests[1].Args, " ")
	if !strings.Contains(secondPrompt, "state file's unchanged `objective` as authoritative") ||
		!strings.Contains(secondPrompt, wantOutput) {
		t.Fatalf("second Goal attempt prompt does not preserve the durable objective contract and prior output: %s", secondPrompt)
	}
}

func assertPackagedGoalBlockedResponse(t testing.TB, response factoryapi.InvocationResponse) {
	t.Helper()
	if response.Status != factoryapi.InvocationTerminalStatusFailed {
		t.Fatalf("blocked Goal invocation status = %q, want FAILED", response.Status)
	}
	if response.ErrorCode == nil || *response.ErrorCode != factoryapi.InvocationResponseErrorCode("INVOCATION_BLOCKED") {
		t.Fatalf("blocked Goal invocation errorCode = %#v, want INVOCATION_BLOCKED", response.ErrorCode)
	}
	if response.WorkState == nil || *response.WorkState != "goal:blocked" {
		t.Fatalf("blocked Goal invocation workState = %#v, want goal:blocked", response.WorkState)
	}
	if response.PrimaryResult != nil {
		t.Fatalf("blocked Goal invocation primaryResult = %#v, want nil", response.PrimaryResult)
	}
}

func findPackagedGoalWork(
	t testing.TB,
	listed factoryapi.ListWorkResponse,
	requestID string,
) factoryapi.Work {
	t.Helper()
	for _, work := range listed.Results {
		if work.RequestId != nil && *work.RequestId == requestID {
			return work
		}
	}
	t.Fatalf("Goal Work request %q is absent from explicit-session listing %#v", requestID, listed.Results)
	return factoryapi.Work{}
}

func submitPackagedGoalWorkToSession(
	t testing.TB,
	baseURL string,
	sessionID string,
	name string,
	text string,
) factoryapi.SubmitWorkResponse {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"name":         name,
		"workTypeName": "goal",
		"items": []map[string]any{{
			"type": "text",
			"text": text,
		}},
	})
	if err != nil {
		t.Fatalf("marshal packaged Goal submit request: %v", err)
	}
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + url.PathEscape(sessionID) + "/work"
	resp, err := http.Post(endpoint, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST %s Goal submit: %v", endpoint, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		payload, _ := io.ReadAll(resp.Body)
		t.Fatalf("POST %s Goal submit status = %d, want 201: %s", endpoint, resp.StatusCode, string(payload))
	}
	var submitted factoryapi.SubmitWorkResponse
	if err := json.NewDecoder(resp.Body).Decode(&submitted); err != nil {
		t.Fatalf("decode Goal submit response: %v", err)
	}
	if strings.TrimSpace(support.StringPointerValue(submitted.WorkId)) == "" {
		t.Fatalf("Goal submit response = %#v, want Work ID", submitted)
	}
	return submitted
}

func assertPackagedGoalEventScope(
	t testing.TB,
	scenario *packagedGoalScenario,
	events []factoryapi.FactoryEvent,
	workID string,
) {
	t.Helper()
	wanted := map[factoryapi.FactoryEventType]bool{
		factoryapi.FactoryEventTypeWorkRequest:      false,
		factoryapi.FactoryEventTypeDispatchRequest:  false,
		factoryapi.FactoryEventTypeDispatchResponse: false,
	}
	for _, event := range events {
		if _, relevant := wanted[event.Type]; !relevant {
			continue
		}
		if event.Context.SessionId == nil || *event.Context.SessionId != scenario.session {
			t.Fatalf("%s Goal event %q session = %#v, want %q", scenario.name, event.Type, event.Context.SessionId, scenario.session)
		}
		if event.Context.WorkIds != nil {
			for _, candidate := range *event.Context.WorkIds {
				if candidate == workID {
					if _, ok := wanted[event.Type]; ok {
						wanted[event.Type] = true
					}
				}
			}
		}
	}
	for eventType, found := range wanted {
		if !found {
			t.Fatalf("%s Goal events lack scoped %s for WorkId %q", scenario.name, eventType, workID)
		}
	}
}

func assertPackagedGoalRetainedReplay(
	t *testing.T,
	scenario *packagedGoalScenario,
	events []factoryapi.FactoryEvent,
) {
	t.Helper()
	if len(events) < 2 {
		t.Fatalf("%s Goal events = %d, want retained history", scenario.name, len(events))
	}
	sequence := support.ReconnectSequenceForFactoryEvent(events[0])
	replayed := support.GetFactoryEventsAfterForSessionAt(t, scenario.fixture.baseURL, scenario.session, support.FactoryEventReadCursor{
		AfterEventID: events[0].Id, AfterSequence: &sequence,
	})
	if len(replayed) != len(events)-1 {
		t.Fatalf("%s Goal replay events = %d, want %d", scenario.name, len(replayed), len(events)-1)
	}
	for index := range replayed {
		if replayed[index].Id != events[index+1].Id {
			t.Fatalf("%s Goal replay event %d = %q, want %q", scenario.name, index, replayed[index].Id, events[index+1].Id)
		}
	}
}
