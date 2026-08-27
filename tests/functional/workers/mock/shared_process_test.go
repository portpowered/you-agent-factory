package mock

import (
	"context"
	"encoding/json"
	"errors"
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

	"github.com/google/uuid"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"github.com/portpowered/infinite-you/tests/internal/functionalevidence"
)

const (
	futureMockWorkerName      = "future-mocked-worker"
	futureMockWorkstationName = "future-mock-process"
	futureMockWorkType        = "future-mock-task"
	futureMockWorkID          = "future-mock-work"

	sharedHostWorker      = "shared-workers-mock-host-worker"
	sharedHostWorkstation = "shared-workers-mock-host-process"
	sharedHostWorkType    = "shared-workers-mock-host-task"
)

// TestSharedProcessWorkersMock keeps the Workers mock selection/routing
// scenarios on one root-built customer host. Each server-backed scenario gets
// a distinct public Factory Session and a fresh command-edge delegate. The
// plain-batch drain rows run last, after the hosted invocation is stopped, so
// they can reuse the same root without concurrently activating another
// default runtime. The resource-set CLI evidence belongs to this one
// package-scoped group because its executable behavior is exercised by the
// LiveCapacityIncrease child row below.
func TestSharedProcessWorkersMock(t *testing.T) {
	fixture := newSharedWorkersMockFixture(t)

	tests := []struct {
		name string
		run  func(*testing.T, *sharedWorkersMockFixture)
	}{
		{name: "NamedAgy", run: testNamedAgyMockPreservesDispatchMetadataAndCompletionLog},
		{name: "ScriptClassifier", run: testScriptWorkerClassifierRoutesWithoutModelCalls},
		{name: "MockWorkersReplace", run: testMockWorkersReplaceOnlyNamedChildren},
		{name: "UnknownWorker", run: testUnknownWorkerOverrideFailsActionably},
		{name: "FutureFields", run: testFutureMockWorkerFieldsAreIgnoredAndDispatchBehaviorIsPreserved},
		{name: "MockWorkerFailure", run: testMockWorkerFailureReturnsStablePublicFailure},
		{name: "RootSelection", run: testMockWorkerSelectedThroughCustomerProcess},
		{name: "ServiceConfigAlignment", run: testServiceConfigOverrideAlignmentCustomerProcess},
		{name: "ExpectedArtifacts", run: testExpectedArtifactsEnforceThroughSharedProcess},
		{name: "MockUsage", run: testMockWorkerUsageIsVisibleAndPriceableThroughSharedProcess},
		{name: "JavaScriptLiveCapacity", run: testJavaScriptLiveResourceCapacityIncreaseWakesWaitingChildren},
		{name: "LiveCapacityIncrease", run: testLiveResourceCapacityIncreaseAdmitsWaitingMockDispatch},
		{name: "LiveCapacitySafeReduction", run: testLiveResourceCapacityReductionPreservesActiveWork},
		{name: "LiveCapacityUnsafeReduction", run: testLiveResourceCapacityRejectsReductionBelowActiveUse},
		{name: "LiveCapacityRecording", run: testLiveResourceCapacityRecordingReplayAndCursor},
		{name: "PlainBatchDrainReportsStrandedWork", run: testPlainBatchDrainReportsStrandedWork},
		{name: "PlainBatchDrainCounterexamples", run: testPlainBatchDrainPreservesFiniteAndContinuousCounterexamples},
		{name: "PlainBatchDrainRejectsPreActivationCancellation", run: testPlainBatchDrainRejectsCancellationBeforeRuntimeActivation},
		{name: "PlainBatchDrainStopsAfterWorkerActivationCancellation", run: testPlainBatchDrainStopsAfterWorkerActivationCancellation},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test.run(t, fixture)
		})
	}
	functionalevidence.Covers(t, "cli/you.session.resource.set")
}

type sharedWorkersMockFixture struct {
	server               *support.FunctionalAPIServer
	providerEdge         *sharedWorkersMockCommandRunner
	scriptEdge           *sharedWorkersMockCommandRunner
	runtimeLogDir        string
	sessionIDGenerator   *sharedWorkersMockSessionIDGenerator
	inputDirectoryWalker *sharedWorkersMockInputDirectoryWalker
	activationMu         sync.Mutex
	localReady           bool
}

type sharedWorkersMockCommandRunner struct {
	mu       sync.RWMutex
	delegate platformprocess.CommandRunner
}

func newSharedWorkersMockCommandRunner() *sharedWorkersMockCommandRunner {
	return &sharedWorkersMockCommandRunner{}
}

func (runner *sharedWorkersMockCommandRunner) set(delegate platformprocess.CommandRunner) {
	runner.mu.Lock()
	runner.delegate = delegate
	runner.mu.Unlock()
}

func (runner *sharedWorkersMockCommandRunner) Run(
	ctx context.Context,
	req platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	runner.mu.RLock()
	delegate := runner.delegate
	runner.mu.RUnlock()
	if delegate == nil {
		return platformprocess.CommandResult{}, errors.New("shared workers mock command runner delegate is not configured")
	}
	return delegate.Run(ctx, req)
}

type sharedWorkersMockSessionIDGenerator struct {
	mu     sync.Mutex
	cancel context.CancelFunc
	id     string
	lastID string
}

func (generator *sharedWorkersMockSessionIDGenerator) armCancellation(
	cancel context.CancelFunc,
	id string,
) {
	generator.mu.Lock()
	generator.cancel = cancel
	generator.id = id
	generator.mu.Unlock()
}

func (generator *sharedWorkersMockSessionIDGenerator) Generate() string {
	generator.mu.Lock()
	cancel := generator.cancel
	id := generator.id
	generator.cancel = nil
	generator.id = ""
	if id == "" {
		id = uuid.NewString()
	}
	generator.lastID = id
	generator.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	return id
}

func (generator *sharedWorkersMockSessionIDGenerator) lastGeneratedID() string {
	generator.mu.Lock()
	defer generator.mu.Unlock()
	return generator.lastID
}

type sharedWorkersMockInputDirectoryWalker struct {
	mu       sync.Mutex
	cancel   context.CancelFunc
	cancelID string
	lastID   string
}

func (walker *sharedWorkersMockInputDirectoryWalker) armCancellation(
	cancel context.CancelFunc,
	cancelID string,
) {
	walker.mu.Lock()
	walker.cancel = cancel
	walker.cancelID = cancelID
	walker.mu.Unlock()
}

func (walker *sharedWorkersMockInputDirectoryWalker) Walk(
	directory string,
	walk fs.WalkDirFunc,
) error {
	walker.mu.Lock()
	cancel := walker.cancel
	cancelID := walker.cancelID
	walker.cancel = nil
	walker.cancelID = ""
	walker.lastID = cancelID
	walker.mu.Unlock()
	if cancel != nil {
		cancel()
		return nil
	}
	return filepath.WalkDir(directory, walk)
}

func (walker *sharedWorkersMockInputDirectoryWalker) lastCancellationID() string {
	walker.mu.Lock()
	defer walker.mu.Unlock()
	return walker.lastID
}

func newSharedWorkersMockFixture(t *testing.T) *sharedWorkersMockFixture {
	t.Helper()

	hostDir := support.ScaffoldFactory(t, map[string]any{
		"name": "shared-workers-mock-host",
		"workTypes": []map[string]any{{
			"name": sharedHostWorkType,
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "done", "type": "TERMINAL"},
			},
		}},
		"resources": []map[string]any{{
			"id":       liveCapacityResourceID,
			"name":     liveCapacityResourceName,
			"capacity": 1,
		}},
		"workers": []map[string]string{{"name": sharedHostWorker}},
		"workstations": []map[string]any{{
			"name":      sharedHostWorkstation,
			"worker":    sharedHostWorker,
			"inputs":    []map[string]string{{"workType": sharedHostWorkType, "state": "init"}},
			"outputs":   []map[string]string{{"workType": sharedHostWorkType, "state": "done"}},
			"onFailure": []map[string]string{{"workType": sharedHostWorkType, "state": "done"}},
		}},
	})
	support.WriteAgentConfig(t, hostDir, sharedHostWorker, support.BuildModelWorkerConfig(
		modelprovider.ProviderCodex,
		"shared-workers-mock-host-model",
	))

	providerEdge := newSharedWorkersMockCommandRunner()
	scriptEdge := newSharedWorkersMockCommandRunner()
	sessionIDGenerator := &sharedWorkersMockSessionIDGenerator{}
	inputDirectoryWalker := &sharedWorkersMockInputDirectoryWalker{}
	runtimeLogDir := t.TempDir()
	mockWorkersPath := writeSharedMockWorkersConfig(t)
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                hostDir,
		WaitForServiceModeRuntime: true,
		Env:                       sharedWorkersMockEnvironment(t, writeSharedWorkersMockOperatorHome(t)),
		Args: []string{
			"--with-mock-workers", mockWorkersPath,
			"--runtime-log-dir", runtimeLogDir,
		},
		Edges: serviceedges.Edges{
			ProviderCommandRunner:              providerEdge,
			ScriptCommandRunner:                scriptEdge,
			FactorySessionIDGenerator:          sessionIDGenerator.Generate,
			FactoryRuntimeInputDirectoryWalker: inputDirectoryWalker.Walk,
		},
	})

	return &sharedWorkersMockFixture{
		server:               server,
		providerEdge:         providerEdge,
		scriptEdge:           scriptEdge,
		runtimeLogDir:        runtimeLogDir,
		sessionIDGenerator:   sessionIDGenerator,
		inputDirectoryWalker: inputDirectoryWalker,
	}
}

func (fixture *sharedWorkersMockFixture) useCommandRunners(
	provider platformprocess.CommandRunner,
	script platformprocess.CommandRunner,
) {
	fixture.providerEdge.set(provider)
	fixture.scriptEdge.set(script)
}

// prepareLocalActivation closes the one continuous host before one of the four
// documented no-server exception rows is admitted. The root process itself
// remains alive and reusable; only its active default runtime is stopped.
// Keeping this transition serialized prevents a second invocation from racing
// the host teardown and fails closed if a future caller forgets to prepare the
// local lane.
func (fixture *sharedWorkersMockFixture) prepareLocalActivation(t *testing.T) {
	t.Helper()
	fixture.activationMu.Lock()
	defer fixture.activationMu.Unlock()
	if fixture.localReady {
		return
	}
	fixture.server.Stop(t)
	select {
	case <-fixture.server.Done():
		fixture.localReady = true
	default:
		t.Errorf("shared workers mock host did not stop before local activation")
	}
}

func (fixture *sharedWorkersMockFixture) executeLocal(
	t testing.TB,
	input root.Input,
) error {
	t.Helper()
	// The local CLI has no public Factory Session selector. These calls are
	// therefore intentionally limited to the plain-batch exception rows, whose
	// own input/HOME/runtime identities and Process.Execute completion provide
	// the isolation and cleanup boundary.
	return (&sharedWorkersMockLocalProcess{fixture: fixture, tb: t}).Execute(input)
}

type sharedWorkersMockLocalProcess struct {
	fixture *sharedWorkersMockFixture
	tb      testing.TB
}

func (process *sharedWorkersMockLocalProcess) Execute(input root.Input) error {
	if process == nil || process.fixture == nil || process.fixture.server == nil {
		return errors.New("shared workers mock local process is unavailable")
	}
	process.fixture.activationMu.Lock()
	defer process.fixture.activationMu.Unlock()
	if !process.fixture.localReady {
		return errors.New("shared workers mock local activation was not prepared")
	}
	return process.fixture.server.Execute(process.tb, input)
}

type sharedWorkersMockSession struct {
	fixture *sharedWorkersMockFixture
	id      string
	closed  bool
}

func (fixture *sharedWorkersMockFixture) openSession(t *testing.T, factoryDir string) *sharedWorkersMockSession {
	t.Helper()
	opened := support.OpenFactorySessionAt(t, fixture.server.URL(), factoryDir)
	session := &sharedWorkersMockSession{fixture: fixture, id: opened.Session.Id}
	t.Cleanup(func() {
		session.close(t)
	})
	return session
}

func (session *sharedWorkersMockSession) terminalObservations(
	t *testing.T,
	timeout time.Duration,
) (factoryapi.ListWorkResponse, []factoryapi.FactoryEvent) {
	t.Helper()
	support.WaitForSessionTerminalStatus(t, session.fixture.server.URL(), session.id, timeout)
	listed := support.GetJSON[factoryapi.ListWorkResponse](t, session.workURL())
	events := support.GetFactoryEventsForSessionAt(t, session.fixture.server.URL(), session.id)
	return listed, events
}

func (session *sharedWorkersMockSession) workURL() string {
	return strings.TrimSuffix(session.fixture.server.URL(), "/") +
		"/factory-sessions/" + url.PathEscape(session.id) + "/work"
}

func (session *sharedWorkersMockSession) current(t testing.TB) factoryapi.FactorySession {
	t.Helper()
	response := support.GetJSON[factoryapi.FactorySessionGetResponse](
		t,
		strings.TrimSuffix(session.fixture.server.URL(), "/")+
			"/factory-sessions/"+url.PathEscape(session.id),
	)
	current, err := response.AsFactorySession()
	if err != nil {
		t.Fatalf("decode Factory Session %q: %v", session.id, err)
	}
	return current
}

func (session *sharedWorkersMockSession) events(t testing.TB) []factoryapi.FactoryEvent {
	t.Helper()
	return support.GetFactoryEventsForSessionAt(t, session.fixture.server.URL(), session.id)
}

func (session *sharedWorkersMockSession) eventsAfter(
	t testing.TB,
	cursor support.FactoryEventReadCursor,
) []factoryapi.FactoryEvent {
	t.Helper()
	return support.GetFactoryEventsAfterForSessionAt(t, session.fixture.server.URL(), session.id, cursor)
}

func (fixture *sharedWorkersMockFixture) trackSession(
	t testing.TB,
	sessionID string,
) *sharedWorkersMockSession {
	t.Helper()
	if strings.TrimSpace(sessionID) == "" {
		t.Fatal("Factory Session ID is empty")
	}
	session := &sharedWorkersMockSession{fixture: fixture, id: sessionID}
	t.Cleanup(func() { session.close(t) })
	return session
}

func (fixture *sharedWorkersMockFixture) executeCLI(
	t testing.TB,
	factoryDir string,
	args ...string,
) (*support.CapturedInputs, error) {
	t.Helper()
	inputs := support.FakeInputs(t.Context(), append([]string{"you"}, args...))
	inputs.Input.WorkingDirectory = factoryDir
	inputs.Input.Env = sharedWorkersMockEnvironment(t, t.TempDir())
	err := fixture.server.Execute(t, inputs.Input)
	return inputs, err
}

func (session *sharedWorkersMockSession) close(t testing.TB) {
	t.Helper()
	if session == nil || session.closed {
		return
	}
	support.CloseFactorySessionAt(t, session.fixture.server.URL(), session.id)
	session.closed = true
}

func (session *sharedWorkersMockSession) closeAndAssertGone(t *testing.T) {
	t.Helper()
	session.close(t)
	assertSharedWorkersMockSessionGone(t, session.fixture.server.URL(), session.id)
}

func assertSharedWorkersMockSessionGone(t *testing.T, baseURL, sessionID string) {
	t.Helper()
	client := &http.Client{Timeout: 2 * time.Second}
	base := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + url.PathEscape(sessionID)
	assertSharedWorkersMockEndpointStatus(t, client, base, http.StatusNotFound)
	for _, suffix := range []string{"/work", "/events"} {
		endpoint := base + suffix
		status := assertSharedWorkersMockEndpointStatus(t, client, endpoint, 0)
		if status >= http.StatusOK && status < http.StatusMultipleChoices {
			t.Fatalf("GET deleted Factory Session endpoint %s status = %d, want non-success", endpoint, status)
		}
	}
}

func assertSharedWorkersMockEndpointStatus(
	t *testing.T,
	client *http.Client,
	endpoint string,
	wantStatus int,
) int {
	t.Helper()
	request, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		t.Fatalf("build deleted Factory Session request %s: %v", endpoint, err)
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("GET deleted Factory Session endpoint %s: %v", endpoint, err)
	}
	_, _ = io.Copy(io.Discard, response.Body)
	response.Body.Close()
	if wantStatus != 0 && response.StatusCode != wantStatus {
		t.Fatalf("GET deleted Factory Session endpoint %s status = %d, want %d", endpoint, response.StatusCode, wantStatus)
	}
	return response.StatusCode
}

func writeSharedMockWorkersConfig(t *testing.T) string {
	t.Helper()

	payload := map[string]any{
		"unmatchedDispatchPolicy": "passthrough",
		"mockWorkers": []map[string]any{
			{
				"id":              "shared-agy-reject",
				"workerName":      "worker",
				"workstationName": "process",
				"runType":         "reject",
				"rejectConfig": map[string]any{
					"stdout":   configuredRejectStdout,
					"stderr":   configuredRejectStderr,
					"exitCode": 7,
				},
			},
			{
				"id":              "shared-script-classifier",
				"workerName":      scriptClassifierWorker,
				"workstationName": scriptClassifierWorkstation,
				"runType":         "script",
				"scriptConfig": map[string]any{
					"command": "mock-classifier-script",
				},
			},
			{
				"id":              "shared-named-replacement",
				"workerName":      mockedWorkerName,
				"workstationName": mockedWorkstationName,
				"runType":         "accept",
			},
			{
				"id":              "shared-future-fields",
				"workerName":      futureMockWorkerName,
				"workstationName": futureMockWorkstationName,
				"runType":         "accept",
				"futureNested":    map[string]any{"secret": "must not affect matching"},
			},
			{
				"id":              "shared-reject",
				"workerName":      rejectWorkerName,
				"workstationName": rejectWorkstationName,
				"runType":         "reject",
				"rejectConfig": map[string]any{
					"stdout":   configuredRejectStdout,
					"stderr":   configuredRejectStderr,
					"exitCode": 7,
				},
			},
			{
				"id":              "shared-root-selection",
				"workerName":      rootMockWorker,
				"workstationName": rootMockWorkstation,
				"runType":         "accept",
			},
			{
				"id":              "shared-artifact-registry",
				"workerName":      artifactRegistryWorker,
				"workstationName": artifactRegistryWorkstation,
				"runType":         "script",
				"scriptConfig": map[string]any{
					"command": artifactRegistryScript,
				},
			},
			{
				"id":              "shared-mock-usage",
				"workstationName": "execute-story",
				"runType":         "accept",
				"usage": map[string]any{
					"provider":              "codex",
					"model":                 "gpt-5-codex",
					"inputTokens":           1000000,
					"cachedInputTokens":     400000,
					"outputTokens":          500000,
					"reasoningOutputTokens": 100000,
				},
			},
			{
				"id":              "shared-live-capacity-script",
				"workerName":      liveCapacityWorker,
				"workstationName": liveCapacityWorkstation,
				"runType":         "script",
				"scriptConfig": map[string]any{
					"command": liveCapacityBarrierCommand,
				},
			},
		},
		"futureTopLevel": true,
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal shared mock workers config: %v", err)
	}
	path := filepath.Join(t.TempDir(), "shared-mock-workers.json")
	if err := os.WriteFile(path, payloadBytes, 0o600); err != nil {
		t.Fatalf("write shared mock workers config: %v", err)
	}
	return path
}

func writeSharedWorkersMockOperatorHome(t *testing.T) string {
	t.Helper()
	homeDir := t.TempDir()
	configDir := filepath.Join(homeDir, ".you-agent-factory")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatalf("create shared workers mock operator config directory: %v", err)
	}
	config := []byte(`{
  "defaults": {"workerModelProvider": "codex", "workerModel": "gpt-5-codex"},
  "workerPresets": [{"id": "javascript-capacity-worker", "modelProvider": "codex", "model": "mock-capacity-model"}]
}`)
	if err := os.WriteFile(filepath.Join(configDir, "config.json"), config, 0o600); err != nil {
		t.Fatalf("write shared workers mock operator config: %v", err)
	}
	return homeDir
}

func sharedWorkersMockEnvironment(t testing.TB, homeDir string) []string {
	t.Helper()
	environment := make([]string, 0, len(os.Environ())+2)
	for _, entry := range os.Environ() {
		name := strings.SplitN(entry, "=", 2)[0]
		if strings.EqualFold(name, "HOME") || strings.EqualFold(name, "USERPROFILE") {
			continue
		}
		environment = append(environment, entry)
	}
	environment = append(environment, "HOME="+homeDir, "USERPROFILE="+homeDir)
	return environment
}

func requireSharedWorkersMockRuntimeLogRecord(
	t *testing.T,
	logDir string,
	eventName string,
	requestID string,
) map[string]any {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(logDir, "*", "*", "*", "*-runtime-log-*.log"))
	if err != nil {
		t.Fatalf("glob shared runtime log paths: %v", err)
	}
	for _, path := range matches {
		file, err := os.Open(path)
		if err != nil {
			t.Fatalf("open shared runtime log %s: %v", path, err)
		}
		var found map[string]any
		decoder := json.NewDecoder(file)
		for {
			var record map[string]any
			err := decoder.Decode(&record)
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				file.Close()
				t.Fatalf("decode shared runtime log record: %v", err)
			}
			if record["event_name"] == eventName && record["request_id"] == requestID {
				found = record
				break
			}
		}
		file.Close()
		if found != nil {
			return found
		}
	}
	t.Fatalf("shared runtime logs under %s did not contain event_name %q for request %q", logDir, eventName, requestID)
	return nil
}
