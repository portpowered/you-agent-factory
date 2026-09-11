package provider_sessions_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	rolloutRoutePrefix          = "rollout-route="
	rolloutSuccessRoute         = "controlled-rollout-success"
	rolloutFailureRoute         = "controlled-rollout-failure"
	rolloutSuccessProviderID    = "rollout-controlled-success-provider"
	rolloutFailureProviderID    = "rollout-controlled-failure-provider"
	rolloutSuccessFinalResponse = "Codex fixture answer COMPLETE"
	rolloutFixtureLLMSource     = "hf://fixture/provider-sessions-rollout/gemma-4-E4B-it-Q4_K_M.gguf@0000000000000000000000000000000000000000"
	rolloutScenarioTimeout      = 60 * time.Second
)

var rolloutSharedFixtureState struct {
	sync.Once
	fixture *rolloutSharedFixture
}

// TestMain owns only the package-shared process. Scenario Factory Sessions,
// homes, routes, streams, and work directories remain owned by their parallel
// subtests and are cleaned before this process is closed.
func TestMain(m *testing.M) {
	exitCode := m.Run()
	if err := closeRolloutSharedFixture(); err != nil {
		fmt.Fprintf(os.Stderr, "controlled rollout shared fixture cleanup failed: %v\n", err)
		exitCode = 1
	}
	os.Exit(exitCode)
}

// TestControlledRolloutTerminalMatrix keeps the customer-visible terminal
// success and failure journeys in the functional lane. The provider effect is
// controlled at the ProviderCommandRunner edge; no executable or OS process is
// involved in either scenario.
func TestControlledRolloutTerminalMatrix(t *testing.T) {
	fixture := rolloutSharedProcess(t)
	cases := []controlledRolloutCase{
		{
			name:                 "success",
			route:                rolloutSuccessRoute,
			providerSessionID:    rolloutSuccessProviderID,
			wantWorkState:        "done",
			wantOutcome:          factoryapi.WorkOutcomeAccepted,
			wantInferenceOutcome: factoryapi.InferenceOutcomeSucceeded,
		},
		{
			name:                 "failure",
			route:                rolloutFailureRoute,
			providerSessionID:    rolloutFailureProviderID,
			wantWorkState:        "failed",
			wantOutcome:          factoryapi.WorkOutcomeFailed,
			wantInferenceOutcome: factoryapi.InferenceOutcomeFailed,
		},
	}
	for _, testCase := range cases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			runControlledRolloutCase(t, fixture, testCase)
		})
	}
}

type controlledRolloutCase struct {
	name                 string
	route                string
	providerSessionID    string
	wantWorkState        string
	wantOutcome          factoryapi.WorkOutcome
	wantInferenceOutcome factoryapi.InferenceOutcome
}

type rolloutSharedFixture struct {
	rootDir     string
	hostFactory string
	homeDir     string
	baseURL     string

	process support.ApplicationProcess
	hosted  *rolloutHostedCommand
	runner  *rolloutCommandRunner
}

type rolloutHostedCommand struct {
	cancel context.CancelFunc
	done   chan error
}

func rolloutSharedProcess(t *testing.T) *rolloutSharedFixture {
	t.Helper()
	rolloutSharedFixtureState.Do(func() {
		rolloutSharedFixtureState.fixture = newRolloutSharedFixture(t)
	})
	if rolloutSharedFixtureState.fixture == nil {
		t.Fatal("controlled rollout shared fixture is unavailable")
	}
	return rolloutSharedFixtureState.fixture
}

func newRolloutSharedFixture(t *testing.T) *rolloutSharedFixture {
	t.Helper()
	rootDir, err := os.MkdirTemp("", "controlled-rollout-functional-")
	if err != nil {
		t.Fatalf("create controlled rollout package root: %v", err)
	}
	hostFactory := filepath.Join(rootDir, "host-factory")
	homeDir := filepath.Join(rootDir, "shared-home")
	if err := copyRolloutFactory(support.LegacyFixtureDir(t, "executor_success"), hostFactory); err != nil {
		t.Fatalf("copy controlled rollout host Factory: %v", err)
	}
	if err := os.MkdirAll(homeDir, 0o755); err != nil {
		t.Fatalf("create controlled rollout shared home: %v", err)
	}
	support.WriteOperatorModelSourceOverride(t, homeDir, "llm", rolloutFixtureLLMSource)
	support.WriteAgentConfig(t, hostFactory, "worker", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "fixture-model"))

	runner := newRolloutCommandRunner(t)
	server := support.NewProcessAPIServer()
	process, err := support.BuildProcessWithContext(context.Background(), serviceedges.Edges{
		APIServerStarter:                   server.Start,
		ProviderCommandRunner:              runner,
		FactorySessionResolveHomeDirectory: func() (string, error) { return homeDir, nil },
	})
	if err != nil {
		t.Fatalf("build controlled rollout shared process: %v", err)
	}

	inputs := support.FakeInputs(context.Background(), []string{
		"you", "run", "--dir", hostFactory, "--continuously", "--with-server", "--quiet", "--no-record",
	})
	inputs.Input.Env = rolloutEnvironment(homeDir)
	inputs.Input.WorkingDirectory = hostFactory
	hosted := startRolloutHostedCommand(process, inputs.Input)
	baseURL := server.WaitForURL(t)
	return &rolloutSharedFixture{
		rootDir: rootDir, hostFactory: hostFactory, homeDir: homeDir, baseURL: baseURL,
		process: process, hosted: hosted, runner: runner,
	}
}

func startRolloutHostedCommand(process support.ApplicationProcess, input root.Input) *rolloutHostedCommand {
	ctx, cancel := context.WithCancel(context.Background())
	input.Context = ctx
	command := &rolloutHostedCommand{cancel: cancel, done: make(chan error, 1)}
	go func() { command.done <- process.Execute(input) }()
	return command
}

func (command *rolloutHostedCommand) stop() error {
	if command == nil {
		return nil
	}
	command.cancel()
	select {
	case err := <-command.done:
		if err != nil && !errors.Is(err, context.Canceled) {
			return fmt.Errorf("stop controlled rollout hosted process: %w", err)
		}
		return nil
	case <-time.After(10 * time.Second):
		return errors.New("timed out waiting for controlled rollout hosted process shutdown")
	}
}

func closeRolloutSharedFixture() error {
	fixture := rolloutSharedFixtureState.fixture
	if fixture == nil {
		return nil
	}
	var errs []error
	if err := fixture.hosted.stop(); err != nil {
		errs = append(errs, err)
	}
	closeContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	if err := fixture.process.Close(closeContext); err != nil {
		errs = append(errs, fmt.Errorf("close controlled rollout process: %w", err))
	}
	cancel()
	if err := fixture.runner.close(); err != nil {
		errs = append(errs, err)
	}
	if err := os.RemoveAll(fixture.rootDir); err != nil {
		errs = append(errs, fmt.Errorf("remove controlled rollout package root: %w", err))
	}
	return errors.Join(errs...)
}

func runControlledRolloutCase(t *testing.T, fixture *rolloutSharedFixture, testCase controlledRolloutCase) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), rolloutScenarioTimeout)
	defer cancel()

	factoryDir := t.TempDir()
	if err := copyRolloutFactory(support.LegacyFixtureDir(t, "executor_success"), factoryDir); err != nil {
		t.Fatalf("copy %s Factory: %v", testCase.name, err)
	}
	homeDir := t.TempDir()
	support.WriteOperatorModelSourceOverride(t, homeDir, "llm", rolloutFixtureLLMSource)
	support.WriteAgentConfig(t, factoryDir, "worker", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "fixture-model"))
	support.WriteWorkstationConfig(t, factoryDir, "process", "---\ntype: MODEL_WORKSTATION\n---\n"+rolloutRoutePrefix+"{{ (index .Inputs 0).Name }}\n")

	callsBefore := fixture.runner.callCount(testCase.route)
	if err := fixture.runner.registerRoute(testCase.route); err != nil {
		t.Fatalf("register %s provider route: %v", testCase.name, err)
	}
	t.Cleanup(func() {
		if err := fixture.runner.closeRoute(testCase.route); err != nil {
			t.Errorf("close %s provider route: %v", testCase.name, err)
		}
	})

	opened := support.OpenFactorySessionAt(t, fixture.baseURL, factoryDir)
	if opened.Session == nil || opened.Session.Id == "" || opened.Session.IsDefault {
		t.Fatalf("%s opened Factory Session = %#v, want explicit non-default identity", testCase.name, opened.Session)
	}
	sessionID := opened.Session.Id
	closed := false
	t.Cleanup(func() {
		if closed {
			return
		}
		support.CloseFactorySessionAt(t, fixture.baseURL, sessionID)
	})

	eventStream := support.OpenFactoryEventStreamAt(t, support.SessionEventsURL(fixture.baseURL, sessionID))
	workID := submitControlledRolloutWork(t, ctx, fixture.process, rolloutEnvironment(homeDir), factoryDir, fixture.baseURL, sessionID, testCase.route)
	events := readRolloutEventsUntilDispatchResponse(t, ctx, eventStream, workID)
	work := listRolloutWork(t, fixture.baseURL, sessionID)
	session := getRolloutSession(t, fixture.baseURL, sessionID)

	assertControlledRolloutWork(t, testCase, workID, work, session)
	assertControlledRolloutEvents(t, testCase, workID, events)
	if got := fixture.runner.callCount(testCase.route) - callsBefore; got != 1 {
		t.Fatalf("%s scenario-owned provider calls = %d, want exactly one", testCase.name, got)
	}
	if got := fixture.runner.activeCallCount(testCase.route); got != 0 {
		t.Fatalf("%s active provider calls after terminal observation = %d, want zero", testCase.name, got)
	}

	support.CloseFactorySessionAt(t, fixture.baseURL, sessionID)
	closed = true
	assertRolloutSessionAbsent(t, fixture.baseURL, sessionID)
}

func submitControlledRolloutWork(
	t *testing.T,
	ctx context.Context,
	process support.Process,
	env []string,
	factoryDir, baseURL, sessionID, route string,
) string {
	t.Helper()
	payload, err := json.Marshal(map[string]any{
		"requestId": route,
		"type":      "FACTORY_REQUEST_BATCH",
		"works": []any{map[string]any{
			"name":         route,
			"workTypeName": "task",
			"payload":      map[string]string{"title": route},
		}},
	})
	if err != nil {
		t.Fatalf("marshal controlled rollout Work: %v", err)
	}
	inputs := executeRolloutCLI(t, ctx, process, env, factoryDir,
		"--server", baseURL, "--json", "submit", "batch", "--session", sessionID, string(payload))
	var response struct {
		WorkCount int `json:"workCount"`
		Works     []struct {
			WorkID string `json:"workId"`
		} `json:"works"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(inputs.Stdout())), &response); err != nil {
		t.Fatalf("decode controlled rollout submit response: %v\nstdout:\n%s\nstderr:\n%s", err, inputs.Stdout(), inputs.Stderr())
	}
	if response.WorkCount != 1 || len(response.Works) != 1 || strings.TrimSpace(response.Works[0].WorkID) == "" {
		t.Fatalf("controlled rollout submit response = %#v, want one Work\nstdout:\n%s", response, inputs.Stdout())
	}
	return response.Works[0].WorkID
}

func executeRolloutCLI(
	t *testing.T,
	ctx context.Context,
	process support.Process,
	env []string,
	workingDirectory string,
	args ...string,
) *support.CapturedInputs {
	t.Helper()
	inputs := support.FakeInputs(ctx, append([]string{"you"}, args...))
	inputs.Input.Env = append([]string(nil), env...)
	inputs.Input.WorkingDirectory = workingDirectory
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf("you %s: %v\nstdout:\n%s\nstderr:\n%s", strings.Join(args, " "), err, inputs.Stdout(), inputs.Stderr())
	}
	return inputs
}

func readRolloutEventsUntilDispatchResponse(
	t *testing.T,
	ctx context.Context,
	stream *support.FactoryEventStream,
	workID string,
) []factoryapi.FactoryEvent {
	t.Helper()
	observeContext, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	events := make([]factoryapi.FactoryEvent, 0, 16)
	for {
		event := stream.NextEventContext(observeContext)
		events = append(events, event)
		if event.Type != factoryapi.FactoryEventTypeDispatchResponse || !rolloutEventIncludesWork(event, workID) {
			continue
		}
		return events
	}
}

func rolloutEventIncludesWork(event factoryapi.FactoryEvent, workID string) bool {
	if event.Context.WorkIds == nil {
		return false
	}
	for _, candidate := range *event.Context.WorkIds {
		if candidate == workID {
			return true
		}
	}
	return false
}

func listRolloutWork(t *testing.T, baseURL, sessionID string) factoryapi.ListWorkResponse {
	t.Helper()
	return support.GetJSON[factoryapi.ListWorkResponse](
		t,
		support.SessionWorkURL(baseURL, sessionID, "/work"),
	)
}

func getRolloutSession(t *testing.T, baseURL, sessionID string) factoryapi.FactorySession {
	t.Helper()
	response := support.GetJSON[factoryapi.FactorySessionGetResponse](
		t,
		strings.TrimSuffix(baseURL, "/")+"/factory-sessions/"+url.PathEscape(sessionID),
	)
	session, err := response.AsFactorySession()
	if err != nil {
		t.Fatalf("decode controlled rollout Factory Session: %v", err)
	}
	return session
}

func assertControlledRolloutWork(
	t *testing.T,
	testCase controlledRolloutCase,
	workID string,
	listed factoryapi.ListWorkResponse,
	session factoryapi.FactorySession,
) {
	t.Helper()
	if len(listed.Results) != 1 || listed.Results[0].WorkId == nil || *listed.Results[0].WorkId != workID {
		t.Fatalf("%s public Work = %#v, want exactly Work %q", testCase.name, listed.Results, workID)
	}
	if got := support.CountWorkAtCustomerState(listed, support.WorkCustomerLocation("task", testCase.wantWorkState)); got != 1 {
		t.Fatalf("%s Work state count = %d, want one task:%s; listed=%#v", testCase.name, got, testCase.wantWorkState, listed.Results)
	}
	otherState := "done"
	if testCase.wantWorkState == "done" {
		otherState = "failed"
	}
	if got := support.CountWorkAtCustomerState(listed, support.WorkCustomerLocation("task", otherState)); got != 0 {
		t.Fatalf("%s Work opposite terminal state count = %d, want zero", testCase.name, got)
	}
	if session.Runtime.Progress.Categories.Initial != 0 || session.Runtime.Progress.Categories.Processing != 0 {
		t.Fatalf("%s session active progress = %+v, want no active Work", testCase.name, session.Runtime.Progress.Categories)
	}
	if testCase.wantWorkState == "done" && (session.Runtime.Progress.Categories.Terminal != 1 || session.Runtime.Progress.Categories.Failed != 0) {
		t.Fatalf("successful rollout session progress = %+v, want terminal=1 failed=0", session.Runtime.Progress.Categories)
	}
	if testCase.wantWorkState == "failed" && (session.Runtime.Progress.Categories.Terminal != 0 || session.Runtime.Progress.Categories.Failed != 1) {
		t.Fatalf("failed rollout session progress = %+v, want terminal=0 failed=1", session.Runtime.Progress.Categories)
	}
}

func assertControlledRolloutEvents(t *testing.T, testCase controlledRolloutCase, workID string, events []factoryapi.FactoryEvent) {
	t.Helper()
	dispatchResponses := make([]factoryapi.DispatchResponseEventPayload, 0, 1)
	inferenceResponses := make([]factoryapi.InferenceResponseEventPayload, 0, 1)
	for _, event := range events {
		if !rolloutEventIncludesWork(event, workID) {
			continue
		}
		switch event.Type {
		case factoryapi.FactoryEventTypeDispatchResponse:
			payload, err := event.Payload.AsDispatchResponseEventPayload()
			if err != nil {
				t.Fatalf("decode %s dispatch response: %v", testCase.name, err)
			}
			dispatchResponses = append(dispatchResponses, payload)
		case factoryapi.FactoryEventTypeInferenceResponse, factoryapi.FactoryEventTypeModelResponse:
			payload, err := support.AsInferenceResponseObservation(event)
			if err != nil {
				t.Fatalf("decode %s provider response: %v", testCase.name, err)
			}
			inferenceResponses = append(inferenceResponses, payload)
		}
	}
	if len(dispatchResponses) != 1 {
		t.Fatalf("%s public dispatch responses = %d, want exactly one", testCase.name, len(dispatchResponses))
	}
	if len(inferenceResponses) != 1 {
		t.Fatalf("%s public inference responses = %d, want exactly one", testCase.name, len(inferenceResponses))
	}
	dispatch := dispatchResponses[0]
	inference := inferenceResponses[0]
	if dispatch.Outcome != testCase.wantOutcome {
		t.Fatalf("%s dispatch outcome = %q, want %q", testCase.name, dispatch.Outcome, testCase.wantOutcome)
	}
	if inference.Outcome != testCase.wantInferenceOutcome {
		t.Fatalf("%s inference outcome = %q, want %q", testCase.name, inference.Outcome, testCase.wantInferenceOutcome)
	}
	if inference.ProviderSession == nil || inference.ProviderSession.Id == nil || *inference.ProviderSession.Id != testCase.providerSessionID {
		t.Fatalf("%s Provider Session = %#v, want %q", testCase.name, inference.ProviderSession, testCase.providerSessionID)
	}
	if testCase.wantWorkState == "done" {
		if inference.Response == nil || !strings.Contains(*inference.Response, rolloutSuccessFinalResponse) {
			t.Fatalf("successful rollout final response = %#v, want semantic marker", inference.Response)
		}
		if dispatch.Output == nil || !strings.Contains(*dispatch.Output, rolloutSuccessFinalResponse) || dispatch.FailureDetail != nil {
			t.Fatalf("successful rollout dispatch = %#v, want output without failure detail", dispatch)
		}
		return
	}
	if inference.Response != nil || inference.FailureDetail == nil || inference.FailureDetail.Reason != factoryapi.WorkFailureTypeAuthFailure {
		t.Fatalf("failed rollout inference = %#v, want typed auth failure without success response", inference)
	}
	if dispatch.Output != nil || dispatch.FailureDetail == nil || dispatch.FailureDetail.Reason != factoryapi.WorkFailureTypeAuthFailure || dispatch.ProviderFailure == nil {
		t.Fatalf("failed rollout dispatch = %#v, want typed auth failure without output", dispatch)
	}
}

func assertRolloutSessionAbsent(t *testing.T, baseURL, sessionID string) {
	t.Helper()
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + url.PathEscape(sessionID)
	response, err := http.Get(endpoint)
	if err != nil {
		t.Fatalf("closed controlled rollout Factory Session %q probe: %v", sessionID, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf(
			"closed controlled rollout Factory Session %q status = %d, want 404: %s",
			sessionID,
			response.StatusCode,
			strings.TrimSpace(string(body)),
		)
	}
}

func newRolloutCommandRunner(t *testing.T) *rolloutCommandRunner {
	t.Helper()
	success := readRolloutFixture(t, "codex", "success", "stdout.jsonl")
	success = replaceRolloutBytes(success, "session_fixture_codex_success", rolloutSuccessProviderID)
	failure := readRolloutFixture(t, "codex", "structured-failure", "stdout.jsonl")
	failure = replaceRolloutBytes(failure, "session_fixture_codex_structured_failure", rolloutFailureProviderID)
	return &rolloutCommandRunner{
		definitions: map[string]platformprocess.CommandResult{
			rolloutSuccessRoute: {Stdout: success},
			rolloutFailureRoute: {Stdout: failure, ExitCode: 1},
		},
		routes: make(map[string]platformprocess.CommandResult),
		active: make(map[string]int),
		calls:  make(map[string]int),
	}
}

type rolloutCommandRunner struct {
	mu          sync.Mutex
	definitions map[string]platformprocess.CommandResult
	routes      map[string]platformprocess.CommandResult
	active      map[string]int
	calls       map[string]int
}

func (runner *rolloutCommandRunner) Run(ctx context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	return runner.run(ctx, request, nil)
}

func (runner *rolloutCommandRunner) RunStreaming(
	ctx context.Context,
	request platformprocess.CommandRequest,
	observer platformprocess.OutputChunkObserver,
) (platformprocess.CommandResult, error) {
	return runner.run(ctx, request, observer)
}

func (runner *rolloutCommandRunner) run(
	ctx context.Context,
	request platformprocess.CommandRequest,
	observer platformprocess.OutputChunkObserver,
) (platformprocess.CommandResult, error) {
	key := rolloutRouteKey(request)
	runner.mu.Lock()
	result, registered := runner.routes[key]
	if registered {
		runner.calls[key]++
		runner.active[key]++
	}
	runner.mu.Unlock()
	if !registered {
		return platformprocess.CommandResult{}, fmt.Errorf("controlled rollout provider route %q is not registered", key)
	}
	defer func() {
		runner.mu.Lock()
		runner.active[key]--
		runner.mu.Unlock()
	}()
	for _, chunk := range rolloutChunks(result.Stdout) {
		if err := ctx.Err(); err != nil {
			return platformprocess.CommandResult{}, err
		}
		if observer != nil {
			observer(platformprocess.OutputStreamStdout, chunk)
		}
	}
	return cloneRolloutCommandResult(result), nil
}

func (runner *rolloutCommandRunner) registerRoute(key string) error {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	definition, ok := runner.definitions[key]
	if !ok {
		return fmt.Errorf("controlled rollout provider route %q is undefined", key)
	}
	if _, ok := runner.routes[key]; ok {
		return fmt.Errorf("controlled rollout provider route %q is already registered", key)
	}
	runner.routes[key] = cloneRolloutCommandResult(definition)
	return nil
}

func (runner *rolloutCommandRunner) closeRoute(key string) error {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	if _, ok := runner.routes[key]; !ok {
		return fmt.Errorf("controlled rollout provider route %q is not registered", key)
	}
	if runner.active[key] != 0 {
		return fmt.Errorf("controlled rollout provider route %q has %d active calls", key, runner.active[key])
	}
	delete(runner.routes, key)
	delete(runner.active, key)
	return nil
}

func (runner *rolloutCommandRunner) callCount(key string) int {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return runner.calls[key]
}

func (runner *rolloutCommandRunner) activeCallCount(key string) int {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return runner.active[key]
}

func (runner *rolloutCommandRunner) close() error {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	for key, count := range runner.active {
		if count != 0 {
			return fmt.Errorf("controlled rollout route %q has %d active calls during package cleanup", key, count)
		}
	}
	if len(runner.routes) != 0 {
		return fmt.Errorf("controlled rollout routes remain registered during package cleanup: %v", rolloutRouteKeys(runner.routes))
	}
	runner.definitions = nil
	runner.active = nil
	runner.calls = nil
	return nil
}

func rolloutRouteKeys(routes map[string]platformprocess.CommandResult) []string {
	keys := make([]string, 0, len(routes))
	for key := range routes {
		keys = append(keys, key)
	}
	return keys
}

func rolloutRouteKey(request platformprocess.CommandRequest) string {
	for _, line := range strings.Split(string(request.Stdin), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, rolloutRoutePrefix) {
			return strings.TrimSpace(strings.TrimPrefix(line, rolloutRoutePrefix))
		}
	}
	return ""
}

func rolloutChunks(contents []byte) [][]byte {
	const chunkSize = 64 << 10
	chunks := make([][]byte, 0, (len(contents)+chunkSize-1)/chunkSize)
	for len(contents) > 0 {
		size := chunkSize
		if len(contents) < size {
			size = len(contents)
		}
		chunks = append(chunks, append([]byte(nil), contents[:size]...))
		contents = contents[size:]
	}
	return chunks
}

func cloneRolloutCommandResult(result platformprocess.CommandResult) platformprocess.CommandResult {
	result.Stdout = append([]byte(nil), result.Stdout...)
	result.Stderr = append([]byte(nil), result.Stderr...)
	return result
}

func readRolloutFixture(t *testing.T, provider, caseName, fileName string) []byte {
	t.Helper()
	path := filepath.Join(testutil.MustRepoRoot(t), filepath.FromSlash(support.ProviderSessionFixturePath(provider, caseName, fileName)))
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read controlled rollout fixture %s: %v", path, err)
	}
	return contents
}

func replaceRolloutBytes(contents []byte, old, replacement string) []byte {
	return []byte(strings.ReplaceAll(string(contents), old, replacement))
}

func rolloutEnvironment(homeDir string) []string {
	env := append([]string(nil), os.Environ()...)
	env = append(env, "HOME="+homeDir, "USERPROFILE="+homeDir)
	return env
}

func copyRolloutFactory(sourceDir, targetDir string) error {
	return filepath.WalkDir(sourceDir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}
		if relative == "." {
			return os.MkdirAll(targetDir, 0o755)
		}
		targetPath := filepath.Join(targetDir, relative)
		if entry.IsDir() {
			return os.MkdirAll(targetPath, 0o755)
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(targetPath, contents, 0o644)
	})
}

var _ platformprocess.CommandRunner = (*rolloutCommandRunner)(nil)
var _ interface {
	platformprocess.CommandRunner
	RunStreaming(context.Context, platformprocess.CommandRequest, platformprocess.OutputChunkObserver) (platformprocess.CommandResult, error)
} = (*rolloutCommandRunner)(nil)
