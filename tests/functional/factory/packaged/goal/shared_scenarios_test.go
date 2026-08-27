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

const packagedGoalSharedFixtureShutdownTimeout = 15 * time.Second

var packagedGoalSelectorSequence uint64

// TestPackagedGoalSharedScenarios is the lexical parent for the session-safe
// Goal scenarios. It owns one root-built process and one HTTP server for the
// child run; each child still owns a separate explicit Factory Session.
func TestPackagedGoalSharedScenarios(t *testing.T) {
	fixture := newPackagedGoalSharedFixture(t)
	t.Cleanup(func() { fixture.ledger.assertClean(t) })

	t.Run("AcceptCompletesWithSummary", func(t *testing.T) {
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
	})

	t.Run("UnknownDecisionFails", func(t *testing.T) {
		runner := newPackagedGoalFailingProviderRunner(t)
		scenario := fixture.openScenario(t, "failure", runner)
		response := postPackagedGoalScenarioInvocation(t, scenario, "invoke packaged goal with failing worker")
		assertPackagedGoalInvocationFailedWithRuntimeDetails(t, response)
		if got := runner.CallCount(); got != 1 {
			t.Fatalf("failing provider invocation count = %d, want 1", got)
		}
		assertPackagedGoalScenarioWitnesses(t, scenario, response, "failed")
	})
}

type packagedGoalSharedFixture struct {
	rootDir    string
	factoryDir string
	baseURL    string
	process    support.ApplicationProcess
	provider   *packagedGoalSelectorRouter
	ledger     *packagedGoalResourceLedger
}

type packagedGoalSelectorRouter struct {
	mu     sync.RWMutex
	routes map[string]platformprocess.CommandRunner
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

func (router *packagedGoalSelectorRouter) Run(
	ctx context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	selector := packagedGoalModelSelector(request.Args)
	router.mu.RLock()
	runner := router.routes[selector]
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
	fixture := &packagedGoalSharedFixture{
		rootDir:    rootDir,
		factoryDir: factoryDir,
		process:    process,
		provider:   provider,
		ledger:     newPackagedGoalResourceLedger(),
	}
	t.Cleanup(func() { fixture.close(t) })

	inputs := support.FakeInputs(context.Background(), []string{
		"you", "run", "--dir", factoryDir,
		"--continuously", "--with-server", "--quiet", "--no-record",
		"--provider", "CODEX", "--model", "goal-shared-host-model",
	})
	inputs.Input.Env = environment
	inputs.Input.WorkingDirectory = factoryDir
	fixture.ledger.recordProcessStart()
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
	fixture.ledger.registerScenario(packagedGoalScenarioResources{
		name: name, selector: selector, session: scenario.session,
		rootDir: rootDir, factoryDir: factoryDir,
	})
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
	scenario.fixture.ledger.closeSession(scenario.session)
	if err := os.RemoveAll(scenario.rootDir); err != nil {
		t.Errorf("remove %s Goal scenario root %q: %v", scenario.name, scenario.rootDir, err)
		return
	}
	if _, err := os.Stat(scenario.rootDir); !os.IsNotExist(err) {
		t.Errorf("%s Goal scenario root %q remains after cleanup: %v", scenario.name, scenario.rootDir, err)
	}
}

type packagedGoalScenarioResources struct {
	name       string
	selector   string
	session    string
	workID     string
	rootDir    string
	factoryDir string
	closed     bool
}

type packagedGoalResourceLedger struct {
	mu            sync.Mutex
	processStarts int
	scenarios     []packagedGoalScenarioResources
}

func newPackagedGoalResourceLedger() *packagedGoalResourceLedger {
	return &packagedGoalResourceLedger{}
}

func (ledger *packagedGoalResourceLedger) recordProcessStart() {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	ledger.processStarts++
}

func (ledger *packagedGoalResourceLedger) registerScenario(resource packagedGoalScenarioResources) {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	ledger.scenarios = append(ledger.scenarios, resource)
}

func (ledger *packagedGoalResourceLedger) recordWork(session, workID string) {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	for index := range ledger.scenarios {
		if ledger.scenarios[index].session == session {
			ledger.scenarios[index].workID = workID
			return
		}
	}
}

func (ledger *packagedGoalResourceLedger) closeSession(session string) {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	for index := range ledger.scenarios {
		if ledger.scenarios[index].session == session {
			ledger.scenarios[index].closed = true
			return
		}
	}
}

func (ledger *packagedGoalResourceLedger) assertClean(t testing.TB) {
	t.Helper()
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	if ledger.processStarts != 1 {
		t.Errorf("GOAL-SPINE-001 process starts = %d, want 1", ledger.processStarts)
	}
	selectors := make(map[string]struct{}, len(ledger.scenarios))
	sessions := make(map[string]struct{}, len(ledger.scenarios))
	workIDs := make(map[string]struct{}, len(ledger.scenarios))
	factories := make(map[string]struct{}, len(ledger.scenarios))
	for _, resource := range ledger.scenarios {
		if !resource.closed {
			t.Errorf("GOAL-CLEANUP-001 session %q remains open", resource.session)
		}
		for label, value := range map[string]string{
			"selector": resource.selector, "session": resource.session,
			"work": resource.workID,
			"root": resource.rootDir, "factory": resource.factoryDir,
		} {
			if strings.TrimSpace(value) == "" {
				t.Errorf("scenario %q has empty %s identity", resource.name, label)
			}
		}
		if _, exists := selectors[resource.selector]; exists {
			t.Errorf("scenario selector %q is not unique", resource.selector)
		}
		selectors[resource.selector] = struct{}{}
		if _, exists := sessions[resource.session]; exists {
			t.Errorf("Factory Session %q is not unique", resource.session)
		}
		sessions[resource.session] = struct{}{}
		if _, exists := workIDs[resource.workID]; exists {
			t.Errorf("Work %q is not unique", resource.workID)
		}
		workIDs[resource.workID] = struct{}{}
		if _, exists := factories[resource.factoryDir]; exists {
			t.Errorf("scenario Factory definition %q is not unique", resource.factoryDir)
		}
		factories[resource.factoryDir] = struct{}{}
	}
	t.Logf("GOAL-SPINE-001 processStarts=%d scenarios=%d sessions=%d selectors=%d workIDs=%d", ledger.processStarts, len(ledger.scenarios), len(sessions), len(selectors), len(workIDs))
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
	listed := support.GetJSON[factoryapi.ListWorkResponse](
		t,
		strings.TrimSuffix(scenario.fixture.baseURL, "/")+"/factory-sessions/"+url.PathEscape(scenario.session)+"/work",
	)
	work := findPackagedGoalWork(t, listed, scenario.request)
	if work.WorkId == nil || strings.TrimSpace(*work.WorkId) == "" {
		t.Fatalf("%s Goal listed WorkId = %#v, want unique Work identity", scenario.name, work.WorkId)
	}
	workID := *work.WorkId
	scenario.fixture.ledger.recordWork(scenario.session, workID)
	if response.WorkId != nil && *response.WorkId != workID {
		t.Fatalf("%s Goal response WorkId = %q, want listed WorkId %q", scenario.name, *response.WorkId, workID)
	}
	if work.State == nil || work.State.Name != wantState {
		t.Fatalf("%s Goal Work state = %#v, want %q; work=%#v", scenario.name, work.State, wantState, work)
	}
	if work.WorkTypeName == nil || *work.WorkTypeName != "goal" {
		t.Fatalf("%s Goal Work type = %#v, want goal", scenario.name, work.WorkTypeName)
	}
	if work.WorkId == nil || *work.WorkId != workID {
		t.Fatalf("%s Goal listed WorkId = %#v, want response WorkId %q", scenario.name, work.WorkId, workID)
	}

	events := support.GetFactoryEventsForSessionAt(t, scenario.fixture.baseURL, scenario.session)
	assertPackagedGoalEventScope(t, scenario, events, workID)
	dispatches := support.ObserveDispatchEvents(t, events)
	if len(dispatches) != 1 || dispatches[0].Request.TransitionId != "execute-goal" {
		t.Fatalf("%s Goal dispatches = %#v, want one execute-goal dispatch", scenario.name, dispatches)
	}
	if !support.DispatchObservationIncludesWork(dispatches[0], workID) {
		t.Fatalf("%s Goal dispatch = %#v, want WorkId %q", scenario.name, dispatches[0], workID)
	}
	if dispatches[0].Response == nil {
		t.Fatalf("%s Goal dispatch has no public response event", scenario.name)
	}
	assertPackagedGoalRetainedReplay(t, scenario, events)
	t.Logf("GOAL-SPINE-001 scenario=%s selector=%s session=%s work=%s state=%s events=%d dispatches=%d", scenario.name, scenario.selector, scenario.session, workID, wantState, len(events), len(dispatches))
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
