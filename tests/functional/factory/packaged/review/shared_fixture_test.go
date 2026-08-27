package review

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const packagedReviewFixtureShutdownTimeout = 15 * time.Second

// packagedReviewSharedFixture owns one root-built process and one continuous
// API host for compatible public-outcome scenarios. Each scenario owns a
// copied packaged Factory and explicit Factory Session; the command edge routes
// by that scenario's unique Factory/workspace selector.
type packagedReviewSharedFixture struct {
	rootDir        string
	factoryDir     string
	baseURL        string
	process        support.ApplicationProcess
	providerRunner *packagedReviewSelectorRunner
	cancel         context.CancelFunc
	done           chan error
}

type packagedReviewSelectorRunner struct {
	mu        sync.RWMutex
	delegates map[string]platformprocess.CommandRunner
}

func (runner *packagedReviewSelectorRunner) register(
	selector string,
	delegate platformprocess.CommandRunner,
) {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	if runner.delegates == nil {
		runner.delegates = make(map[string]platformprocess.CommandRunner)
	}
	runner.delegates[packagedReviewSelectorPath(selector)] = delegate
}

func (runner *packagedReviewSelectorRunner) unregister(selector string) {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	delete(runner.delegates, packagedReviewSelectorPath(selector))
}

func (runner *packagedReviewSelectorRunner) Run(
	ctx context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	selector := packagedReviewSelectorPath(request.WorkDir)
	runner.mu.RLock()
	delegate := runner.delegates[selector]
	runner.mu.RUnlock()
	if delegate == nil {
		return platformprocess.CommandResult{}, fmt.Errorf(
			"no packaged Review provider command runner registered for selector %q",
			selector,
		)
	}
	return delegate.Run(ctx, request)
}

func packagedReviewSelectorPath(path string) string {
	return filepath.Clean(path)
}

var (
	packagedReviewFixtureOnce sync.Once
	packagedReviewFixture     *packagedReviewSharedFixture
	packagedReviewFixtureErr  error
)

func TestMain(m *testing.M) {
	code := m.Run()
	if packagedReviewFixture != nil {
		if err := packagedReviewFixture.close(); err != nil {
			fmt.Fprintf(os.Stderr, "close shared packaged Review fixture: %v\n", err)
			if code == 0 {
				code = 1
			}
		}
	}
	os.Exit(code)
}

func sharedPackagedReviewFixture(t *testing.T) *packagedReviewSharedFixture {
	t.Helper()
	packagedReviewFixtureOnce.Do(func() {
		packagedReviewFixture, packagedReviewFixtureErr = startPackagedReviewFixture()
	})
	if packagedReviewFixtureErr != nil {
		t.Fatalf("start shared packaged Review fixture: %v", packagedReviewFixtureErr)
	}
	if packagedReviewFixture == nil {
		t.Fatal("shared packaged Review fixture is unavailable")
	}
	support.WaitForStatus(t, packagedReviewFixture.baseURL, packagedReviewFixtureShutdownTimeout, func(status factoryapi.StatusResponse) bool {
		return strings.TrimSpace(status.RuntimeStatus) != ""
	})
	return packagedReviewFixture
}

func startPackagedReviewFixture() (*packagedReviewSharedFixture, error) {
	rootDir, err := os.MkdirTemp("", "you-functional-packaged-review-")
	if err != nil {
		return nil, fmt.Errorf("create fixture root: %w", err)
	}
	cleanupRoot := func() { _ = os.RemoveAll(rootDir) }
	homeDir := filepath.Join(rootDir, "home")
	workingDir := filepath.Join(rootDir, "work")
	if err := os.MkdirAll(homeDir, 0o755); err != nil {
		cleanupRoot()
		return nil, fmt.Errorf("create fixture home: %w", err)
	}
	if err := os.MkdirAll(workingDir, 0o755); err != nil {
		cleanupRoot()
		return nil, fmt.Errorf("create fixture working directory: %w", err)
	}

	api := support.NewProcessAPIServer()
	providerRunner := &packagedReviewSelectorRunner{}
	process, err := support.BuildProcessWithContext(context.Background(), serviceedges.Edges{
		APIServerStarter:      api.Start,
		ProviderCommandRunner: providerRunner,
	})
	if err != nil {
		cleanupRoot()
		return nil, fmt.Errorf("build root process: %w", err)
	}

	env := packagedReviewFixtureEnvironment(homeDir)
	if err := initializePackagedReviewHome(process, env, workingDir); err != nil {
		closePackagedReviewProcess(process)
		cleanupRoot()
		return nil, fmt.Errorf("initialize packaged Factory home: %w", err)
	}
	runtimeFactoryDir := filepath.Join(
		homeDir,
		".you-agent-factory",
		"factories",
		filepath.FromSlash(factorydefinitions.PackagedReviewFactoryName),
	)
	if _, err := os.Stat(filepath.Join(runtimeFactoryDir, factorydefinitions.FactoryConfigFile)); err != nil {
		closePackagedReviewProcess(process)
		cleanupRoot()
		return nil, fmt.Errorf("find packaged Review Factory at %s: %w", runtimeFactoryDir, err)
	}
	// The continuous runtime writes lifecycle files below its Factory directory.
	// Copy a static template before starting it so per-scenario copies never
	// race with runtime artifact creation or observe a half-written file.
	templateDir := filepath.Join(rootDir, "factory-template")
	if err := os.CopyFS(templateDir, os.DirFS(runtimeFactoryDir)); err != nil {
		closePackagedReviewProcess(process)
		cleanupRoot()
		return nil, fmt.Errorf("snapshot packaged Review Factory: %w", err)
	}

	commandContext, cancel := context.WithCancel(context.Background())
	inputs := support.FakeInputs(commandContext, []string{
		"you", "run",
		"--dir", runtimeFactoryDir,
		"--continuously",
		"--with-server",
		"--quiet",
		"--no-record",
	})
	inputs.Input.Env = env
	inputs.Input.WorkingDirectory = runtimeFactoryDir
	done := make(chan error, 1)
	go func() { done <- process.Execute(inputs.Input) }()

	baseURL, err := api.WaitForBaseURL(packagedReviewFixtureShutdownTimeout)
	if err != nil {
		cancel()
		waitForPackagedReviewCommand(done)
		closePackagedReviewProcess(process)
		cleanupRoot()
		return nil, fmt.Errorf("wait for API server: %w", err)
	}
	return &packagedReviewSharedFixture{
		rootDir:        rootDir,
		factoryDir:     templateDir,
		baseURL:        baseURL,
		process:        process,
		providerRunner: providerRunner,
		cancel:         cancel,
		done:           done,
	}, nil
}

func initializePackagedReviewHome(
	process support.Process,
	env []string,
	workingDir string,
) error {
	missingFactory := filepath.Join(workingDir, "missing-initialization-factory.json")
	inputs := support.FakeInputs(context.Background(), []string{
		"you", "run", "--factory", missingFactory,
	})
	inputs.Input.Env = env
	inputs.Input.WorkingDirectory = workingDir
	err := process.Execute(inputs.Input)
	if err == nil || !strings.Contains(err.Error(), filepath.Base(missingFactory)) {
		return fmt.Errorf(
			"missing Factory probe error = %v, stdout=%q, stderr=%q",
			err,
			inputs.Stdout(),
			inputs.Stderr(),
		)
	}
	return nil
}

func packagedReviewFixtureEnvironment(homeDir string) []string {
	return []string{
		"HOME=" + homeDir,
		"USERPROFILE=" + homeDir,
		operatorsettings.EnvDefaultWorkerModelProvider + "=CODEX",
		operatorsettings.EnvDefaultWorkerModel + "=operator-configured-model",
	}
}

func waitForPackagedReviewCommand(done <-chan error) {
	select {
	case <-done:
	case <-time.After(packagedReviewFixtureShutdownTimeout):
		// The done channel is the deterministic lifecycle observation. This
		// timer is only a bounded startup-failure/teardown safety ceiling: a
		// failed command must not leave fixture setup hanging forever while its
		// context and injected API server unwind. It is never scenario
		// synchronization.
	}
}

func closePackagedReviewProcess(process support.ApplicationProcess) {
	if process == nil {
		return
	}
	closeContext, cancel := context.WithTimeout(context.Background(), packagedReviewFixtureShutdownTimeout)
	defer cancel()
	_ = process.Close(closeContext)
}

func (fixture *packagedReviewSharedFixture) close() error {
	if fixture == nil {
		return nil
	}
	fixture.cancel()
	var errs []error
	select {
	case err := <-fixture.done:
		if err != nil && !errors.Is(err, context.Canceled) {
			errs = append(errs, fmt.Errorf("continuous command: %w", err))
		}
	case <-time.After(packagedReviewFixtureShutdownTimeout):
		// The done channel is the deterministic lifecycle observation. This
		// timer is only a bounded teardown safety ceiling for a command that
		// failed to honor cancellation; without it TestMain could hang while
		// closing the injected API server. It is not normal scenario waiting.
		errs = append(errs, errors.New("timed out waiting for continuous command shutdown"))
	}
	closeContext, cancel := context.WithTimeout(context.Background(), packagedReviewFixtureShutdownTimeout)
	if err := fixture.process.Close(closeContext); err != nil {
		errs = append(errs, fmt.Errorf("close root process: %w", err))
	}
	cancel()
	if err := os.RemoveAll(fixture.rootDir); err != nil {
		errs = append(errs, fmt.Errorf("remove fixture root: %w", err))
	}
	return errors.Join(errs...)
}

type packagedReviewScenario struct {
	fixture    *packagedReviewSharedFixture
	homeDir    string
	factoryDir string
	workspace  string
	selector   string
	sessionID  string
	requestID  string
}

func openPackagedReviewScenario(
	t *testing.T,
	runner *packagedReviewCommandRunner,
	name string,
	configure func(*testing.T, string),
) *packagedReviewScenario {
	t.Helper()
	fixture := sharedPackagedReviewFixture(t)
	homeDir := t.TempDir()
	factoryDir := support.CopyFactoryAsNamed(
		t,
		fixture.factoryDir,
		homeDir,
		factorydefinitions.PackagedReviewFactoryName,
	)
	if configure != nil {
		configure(t, factoryDir)
	}
	workspace := factoryDir
	selector := filepath.Clean(workspace)
	fixture.providerRunner.register(selector, runner)
	t.Cleanup(func() { fixture.providerRunner.unregister(selector) })
	opened := support.OpenFactorySessionAt(t, fixture.baseURL, factoryDir)
	if opened.Session == nil || opened.Session.Id == "" {
		t.Fatal("opened packaged Review session has no id")
	}
	if opened.Session.Id == factorysessions.DefaultSessionID {
		t.Fatalf("opened packaged Review session = %q, want explicit non-default session", opened.Session.Id)
	}
	sessionID := opened.Session.Id
	requestID := "packaged-review-" + strings.NewReplacer(" ", "-", "/", "-", "_", "-").Replace(name)
	t.Cleanup(func() {
		support.CloseFactorySessionAt(t, fixture.baseURL, sessionID)
	})
	return &packagedReviewScenario{
		fixture:    fixture,
		homeDir:    homeDir,
		factoryDir: factoryDir,
		workspace:  workspace,
		selector:   selector,
		sessionID:  sessionID,
		requestID:  requestID,
	}
}

func invokePackagedReviewSession(
	t *testing.T,
	scenario *packagedReviewScenario,
	args map[string]any,
) factoryapi.InvocationResponse {
	t.Helper()
	// The packaged Petri Factory must run inside this already-open explicit
	// session so the assertions below can observe its canonical Work, Event,
	// dispatch, and replay history. The public run CLI has no session-target
	// flag, and its remote durable source only resolves JavaScript workflow
	// factories, so this is the narrowly scoped CLI-plus-API parity exception.
	// TestPackagedReviewSharedProcess/CLIResponseMatchesExplicitSession still
	// executes the same invocation through the root-built customer CLI and
	// compares its terminal response with this explicit-session API path.
	payload, err := json.Marshal(factoryapi.InvocationRequest{
		RequestId: &scenario.requestID,
		Args:      &args,
	})
	if err != nil {
		t.Fatalf("marshal packaged Review invocation: %v", err)
	}
	endpoint := strings.TrimSuffix(scenario.fixture.baseURL, "/") +
		"/factory-sessions/" + url.PathEscape(scenario.sessionID) + "/invocations"
	response, err := http.Post(endpoint, "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("POST packaged Review invocation: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("POST packaged Review invocation status = %d, want 200", response.StatusCode)
	}
	var decoded factoryapi.InvocationResponse
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		t.Fatalf("decode packaged Review invocation: %v", err)
	}
	support.WaitForSessionTerminalStatus(t, scenario.fixture.baseURL, scenario.sessionID, packagedReviewFixtureShutdownTimeout)
	if decoded.RequestId != scenario.requestID {
		t.Fatalf("invocation request id = %q, want unique %q", decoded.RequestId, scenario.requestID)
	}
	return decoded
}

func assertPackagedReviewSharedEvidence(
	t *testing.T,
	scenario *packagedReviewScenario,
	runner *packagedReviewCommandRunner,
	wantWorkState string,
) {
	t.Helper()
	if scenario.workspace != scenario.selector || filepath.Clean(scenario.factoryDir) != scenario.selector {
		t.Fatalf("scenario identities = factory %q workspace %q selector %q, want one unique workspace selector", scenario.factoryDir, scenario.workspace, scenario.selector)
	}
	requests := runner.Requests()
	if len(requests) == 0 {
		t.Fatal("shared Review runner recorded no provider requests")
	}
	for index, request := range requests {
		if filepath.Clean(request.WorkDir) != scenario.selector {
			t.Fatalf("provider request[%d] work dir = %q, want scenario selector %q", index, request.WorkDir, scenario.selector)
		}
	}

	workResponse := support.GetJSON[factoryapi.ListWorkResponse](
		t,
		strings.TrimSuffix(scenario.fixture.baseURL, "/")+
			"/factory-sessions/"+url.PathEscape(scenario.sessionID)+"/work",
	)
	if len(workResponse.Results) != 1 {
		t.Fatalf("session Work results = %#v, want one Review Work item", workResponse.Results)
	}
	work := workResponse.Results[0]
	if work.State == nil || work.State.Name != wantWorkState {
		t.Fatalf("session Work state = %#v, want name %q", work.State, wantWorkState)
	}
	if work.WorkId == nil || strings.TrimSpace(*work.WorkId) == "" {
		t.Fatalf("session Work identity = %#v, want non-empty Work ID", work.WorkId)
	}

	events := support.GetFactoryEventsForSessionAt(t, scenario.fixture.baseURL, scenario.sessionID)
	if len(events) < 2 {
		t.Fatalf("retained Factory Event history length = %d, want at least two events", len(events))
	}
	hasWorkRequest := false
	for _, event := range events {
		if event.Type == factoryapi.FactoryEventTypeWorkRequest {
			hasWorkRequest = true
			break
		}
	}
	if !hasWorkRequest {
		t.Fatal("retained Factory Event history has no Work Request event")
	}

	dispatches := support.ObserveDispatchEvents(t, events)
	roles := runner.Roles()
	if len(dispatches) != len(roles) {
		t.Fatalf("dispatch count = %d, provider role count = %d; dispatches = %#v", len(dispatches), len(roles), dispatches)
	}
	wantTransitions := map[string]string{
		"writer":   "execute-review-work",
		"reviewer": "review-review-work",
	}
	for index, dispatch := range dispatches {
		if got := dispatch.Request.TransitionId; got != wantTransitions[roles[index]] {
			t.Fatalf("dispatch[%d] transition = %q for role %q, want %q", index, got, roles[index], wantTransitions[roles[index]])
		}
		if dispatch.Response == nil {
			t.Fatalf("dispatch[%d] = %#v, want retained dispatch response", index, dispatch)
		}
		if !support.DispatchObservationIncludesWork(dispatch, *work.WorkId) {
			t.Fatalf("dispatch[%d] = %#v, want Work ID %q", index, dispatch, *work.WorkId)
		}
	}

	sequence := support.ReconnectSequenceForFactoryEvent(events[0])
	replayed := support.GetFactoryEventsAfterForSessionAt(t, scenario.fixture.baseURL, scenario.sessionID, support.FactoryEventReadCursor{
		AfterEventID:  events[0].Id,
		AfterSequence: &sequence,
	})
	if len(replayed) != len(events)-1 {
		t.Fatalf("retained replay event count = %d, want %d", len(replayed), len(events)-1)
	}
	for index := range replayed {
		if replayed[index].Id != events[index+1].Id {
			t.Fatalf("retained replay event %d = %q, want %q", index, replayed[index].Id, events[index+1].Id)
		}
	}
}

type packagedReviewCommandRunner struct {
	mu                 sync.Mutex
	requests           []platformprocess.CommandRequest
	roles              []string
	workCalls          int
	reviewerCalls      int
	rejectReviews      int
	rejectionFeedbacks []string
	reviewOutputs      []string
	acceptedOutput     string
}

func (runner *packagedReviewCommandRunner) Run(
	_ context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	prompt := providerCommandPrompt(request)
	runner.mu.Lock()
	runner.requests = append(runner.requests, clonePackagedReviewCommandRequest(request))
	switch {
	case strings.Contains(prompt, "producing stage of an independent produce-and-review workflow"):
		runner.workCalls++
		call := runner.workCalls
		runner.roles = append(runner.roles, "writer")
		runner.mu.Unlock()
		candidate := "first candidate"
		if call > 1 {
			candidate = "revised candidate"
		}
		return packagedReviewProviderResult(candidate), nil
	case strings.Contains(prompt, "independent review stage"):
		runner.reviewerCalls++
		call := runner.reviewerCalls
		reject := call <= runner.rejectReviews
		feedback := "add the missing release date"
		if call > 0 && call <= len(runner.rejectionFeedbacks) {
			feedback = runner.rejectionFeedbacks[call-1]
		}
		reviewOutput := runner.acceptedOutput
		if call > 0 && call <= len(runner.reviewOutputs) {
			reviewOutput = runner.reviewOutputs[call-1]
		}
		if reviewOutput != "" && call <= len(runner.reviewOutputs) {
			runner.roles = append(runner.roles, "reviewer")
			runner.mu.Unlock()
			return packagedReviewProviderResult(reviewOutput), nil
		}
		if reviewOutput == "" {
			reviewOutput = "approved candidate work"
		}
		runner.roles = append(runner.roles, "reviewer")
		runner.mu.Unlock()
		if reject {
			return packagedReviewProviderResult(fmt.Sprintf(
				`{"decision":"REJECTED","feedback":%q}`,
				feedback,
			)), nil
		}
		return packagedReviewProviderResult(fmt.Sprintf(
			`{"decision":"ACCEPTED","output":%q}`,
			reviewOutput,
		)), nil
	default:
		runner.mu.Unlock()
		return platformprocess.CommandResult{}, fmt.Errorf("unexpected Review prompt: %s", prompt)
	}
}

func (runner *packagedReviewCommandRunner) Requests() []platformprocess.CommandRequest {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	requests := make([]platformprocess.CommandRequest, len(runner.requests))
	for index, request := range runner.requests {
		requests[index] = clonePackagedReviewCommandRequest(request)
	}
	return requests
}

func (runner *packagedReviewCommandRunner) Roles() []string {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return append([]string(nil), runner.roles...)
}

func (runner *packagedReviewCommandRunner) ReviewerCalls() int {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return runner.reviewerCalls
}

func packagedReviewProviderResult(output string) platformprocess.CommandResult {
	return platformprocess.CommandResult{Stdout: support.CodexSuccessStdout(output)}
}

func clonePackagedReviewCommandRequest(request platformprocess.CommandRequest) platformprocess.CommandRequest {
	request.Args = append([]string(nil), request.Args...)
	request.Stdin = append([]byte(nil), request.Stdin...)
	request.Env = append([]string(nil), request.Env...)
	return request
}
