package tts

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const packagedTTSSharedFixtureTimeout = 30 * time.Second

type packagedTTSProviderOutcome interface {
	Infer(context.Context, workerexecution.ProviderInferenceRequest) (workerexecution.InferenceResponse, error)
	callCount() int
	lastRequest() *workerexecution.ProviderInferenceRequest
	lastAudioPath() string
	ownedArtifactRoot() string
}

// packagedTTSInferenceBarrier makes the concurrent isolation witness
// deterministic: both model calls must cross the shared provider boundary
// before either controlled outcome is released.
type packagedTTSInferenceBarrier struct {
	mu       sync.Mutex
	entered  int
	expected int
	released bool
	release  chan struct{}
}

func newPackagedTTSInferenceBarrier(expected int) *packagedTTSInferenceBarrier {
	return &packagedTTSInferenceBarrier{
		expected: expected,
		release:  make(chan struct{}),
	}
}

func (barrier *packagedTTSInferenceBarrier) wait(ctx context.Context) error {
	if barrier == nil {
		return nil
	}
	barrier.mu.Lock()
	barrier.entered++
	if barrier.entered >= barrier.expected && !barrier.released {
		barrier.released = true
		close(barrier.release)
	}
	barrier.mu.Unlock()
	select {
	case <-barrier.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// packagedTTSSharedProvider routes each controlled model outcome by the
// request identity carried through the real Providers boundary. The registry
// is synchronized because the same process is deliberately usable by the
// selected concurrent-isolation witness.
type packagedTTSSharedProvider struct {
	testutil.ProviderServiceAdapter

	mu      sync.Mutex
	routes  map[string]packagedTTSProviderOutcome
	barrier *packagedTTSInferenceBarrier
}

func newPackagedTTSSharedProvider() *packagedTTSSharedProvider {
	provider := &packagedTTSSharedProvider{routes: make(map[string]packagedTTSProviderOutcome)}
	provider.ProviderServiceAdapter.InferFunc = provider.Infer
	return provider
}

func (provider *packagedTTSSharedProvider) register(
	selector string,
	outcome packagedTTSProviderOutcome,
) error {
	selector = strings.TrimSpace(selector)
	if selector == "" {
		return fmt.Errorf("TTS route selector is required")
	}
	if outcome == nil {
		return fmt.Errorf("TTS route %q has no provider outcome", selector)
	}
	provider.mu.Lock()
	defer provider.mu.Unlock()
	if _, exists := provider.routes[selector]; exists {
		return fmt.Errorf("TTS route selector %q is already registered", selector)
	}
	provider.routes[selector] = outcome
	return nil
}

func (provider *packagedTTSSharedProvider) unregister(selector string) error {
	selector = strings.TrimSpace(selector)
	provider.mu.Lock()
	defer provider.mu.Unlock()
	if _, exists := provider.routes[selector]; !exists {
		return fmt.Errorf("TTS route selector %q is not registered", selector)
	}
	delete(provider.routes, selector)
	return nil
}

func (provider *packagedTTSSharedProvider) routeCount() int {
	provider.mu.Lock()
	defer provider.mu.Unlock()
	return len(provider.routes)
}

func (provider *packagedTTSSharedProvider) setInferenceBarrier(
	barrier *packagedTTSInferenceBarrier,
) {
	provider.mu.Lock()
	defer provider.mu.Unlock()
	provider.barrier = barrier
}

func (provider *packagedTTSSharedProvider) Infer(
	ctx context.Context,
	request workerexecution.ProviderInferenceRequest,
) (workerexecution.InferenceResponse, error) {
	selector := strings.TrimSpace(request.Correlation.RequestID)
	if selector == "" {
		selector = strings.TrimSpace(request.Dispatch.Execution.RequestID)
	}
	provider.mu.Lock()
	outcome := provider.routes[selector]
	barrier := provider.barrier
	provider.mu.Unlock()
	if outcome == nil {
		return workerexecution.InferenceResponse{}, fmt.Errorf(
			"no packaged TTS route registered for request %q",
			selector,
		)
	}
	if err := barrier.wait(ctx); err != nil {
		return workerexecution.InferenceResponse{}, err
	}
	return outcome.Infer(ctx, request)
}

// packagedTTSSharedHTTPServer counts the one loopback transport start owned
// by the parent group while delegating all server behavior to the functional
// support transport.
type packagedTTSSharedHTTPServer struct {
	server *support.ProcessAPIServer

	mu     sync.Mutex
	starts int
}

func newPackagedTTSSharedHTTPServer() *packagedTTSSharedHTTPServer {
	return &packagedTTSSharedHTTPServer{server: support.NewProcessAPIServer()}
}

func (server *packagedTTSSharedHTTPServer) start(
	ctx context.Context,
	request platformhttpserver.StartRequest,
) error {
	server.mu.Lock()
	server.starts++
	server.mu.Unlock()
	return server.server.Start(ctx, request)
}

func (server *packagedTTSSharedHTTPServer) startCount() int {
	server.mu.Lock()
	defer server.mu.Unlock()
	return server.starts
}

type packagedTTSSharedFixture struct {
	rootDir         string
	baseFactoryDir  string
	baseURL         string
	process         support.ApplicationProcess
	command         *support.ProcessCommand
	api             *packagedTTSSharedHTTPServer
	provider        *packagedTTSSharedProvider
	mu              sync.Mutex
	childSessionIDs []string
	artifactRoots   []string
	processBuilds   int
	closeOnce       sync.Once
	finalizeOnce    sync.Once
}

func newPackagedTTSSharedFixture(t *testing.T) *packagedTTSSharedFixture {
	t.Helper()

	rootDir := t.TempDir()
	homeDir := filepath.Join(rootDir, "home")
	workingDir := filepath.Join(rootDir, "work")
	if err := os.MkdirAll(homeDir, 0o755); err != nil {
		t.Fatalf("create shared TTS home: %v", err)
	}
	if err := os.MkdirAll(workingDir, 0o755); err != nil {
		t.Fatalf("create shared TTS working directory: %v", err)
	}

	api := newPackagedTTSSharedHTTPServer()
	provider := newPackagedTTSSharedProvider()
	process := support.BuildProcess(t, serviceedges.Edges{
		APIServerStarter: api.start,
		ProviderOverride: provider,
	})
	fixture := &packagedTTSSharedFixture{
		rootDir:       rootDir,
		process:       process,
		api:           api,
		provider:      provider,
		processBuilds: 1,
	}

	env := append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)
	fixture.baseFactoryDir = support.InstallPackagedFactoryWithProcess(
		t,
		process,
		env,
		workingDir,
		factorydefinitions.PackagedTTSFactoryName,
	)
	support.CleanupProcess(t, process)

	inputs := support.FakeInputs(context.Background(), []string{
		"you", "run",
		"--dir", fixture.baseFactoryDir,
		"--continuously",
		"--with-server",
		"--quiet",
		"--no-record",
	})
	inputs.Input.Env = env
	inputs.Input.WorkingDirectory = fixture.baseFactoryDir
	fixture.command = support.StartProcessCommand(t, process, inputs.Input)
	t.Cleanup(func() { fixture.finalize(t) })

	fixture.baseURL = api.server.WaitForURL(t)
	support.WaitForStatus(t, fixture.baseURL, packagedTTSSharedFixtureTimeout, func(status factoryapi.StatusResponse) bool {
		return strings.TrimSpace(status.RuntimeStatus) != ""
	})
	return fixture
}

type packagedTTSSharedScenario struct {
	fixture      *packagedTTSSharedFixture
	homeDir      string
	factoryDir   string
	sessionID    string
	selector     string
	provider     packagedTTSProviderOutcome
	artifactRoot string
	cleanupOnce  sync.Once
}

type packagedTTSSharedEvidence struct {
	name         string
	sessionID    string
	requestID    string
	workID       string
	modelRequest string
	traceID      string
	dispatchID   string
	eventIDs     []string
	artifactPath string
	artifactRoot string
	homeDir      string
	factoryDir   string
}

func (fixture *packagedTTSSharedFixture) openPackagedScenario(
	t *testing.T,
	selector string,
	optionalVoiceAndFormat bool,
) *packagedTTSSharedScenario {
	t.Helper()
	homeDir := t.TempDir()
	factoryDir := support.CopyFactoryAsNamed(
		t,
		fixture.baseFactoryDir,
		homeDir,
		"@test/tts",
	)
	if optionalVoiceAndFormat {
		overwritePackagedTTSFactoryWithOptionalVoiceAndFormatTopology(t, factoryDir)
	} else {
		overwritePackagedTTSFactoryWithProviderFakeTopology(t, factoryDir)
	}
	return fixture.openScenario(
		t,
		homeDir,
		factoryDir,
		selector,
		newPackagedTTSFakeProvider(t, []byte(packagedTTSFakeAudioFixture)),
	)
}

func (fixture *packagedTTSSharedFixture) openPackagedFailureScenario(
	t *testing.T,
	selector, failureMessage string,
	optionalVoiceAndFormat bool,
) *packagedTTSSharedScenario {
	t.Helper()
	homeDir := t.TempDir()
	factoryDir := support.CopyFactoryAsNamed(
		t,
		fixture.baseFactoryDir,
		homeDir,
		"@test/tts",
	)
	if optionalVoiceAndFormat {
		overwritePackagedTTSFactoryWithOptionalVoiceAndFormatTopology(t, factoryDir)
	} else {
		overwritePackagedTTSFactoryWithProviderFakeTopology(t, factoryDir)
	}
	return fixture.openScenario(
		t,
		homeDir,
		factoryDir,
		selector,
		newPackagedTTSFailingFakeProvider(t, failureMessage),
	)
}

func (fixture *packagedTTSSharedFixture) openGenericScenario(
	t *testing.T,
	selector string,
) *packagedTTSSharedScenario {
	t.Helper()
	homeDir := t.TempDir()
	factoryDir := scaffoldFactoryTTSAudioDispatch(t)
	return fixture.openScenario(
		t,
		homeDir,
		factoryDir,
		selector,
		newPackagedTTSFakeProvider(t, []byte(packagedTTSFakeAudioFixture)),
	)
}

func (fixture *packagedTTSSharedFixture) openGenericFailureScenario(
	t *testing.T,
	selector, failureMessage string,
) *packagedTTSSharedScenario {
	t.Helper()
	homeDir := t.TempDir()
	factoryDir := scaffoldFactoryTTSAudioDispatch(t)
	return fixture.openScenario(
		t,
		homeDir,
		factoryDir,
		selector,
		newPackagedTTSFailingFakeProvider(t, failureMessage),
	)
}

func (fixture *packagedTTSSharedFixture) openScenario(
	t *testing.T,
	homeDir, factoryDir, selector string,
	outcome packagedTTSProviderOutcome,
) *packagedTTSSharedScenario {
	t.Helper()
	opened := support.OpenFactorySessionAt(t, fixture.baseURL, factoryDir)
	if opened.Session == nil || strings.TrimSpace(opened.Session.Id) == "" {
		t.Fatalf("opened shared TTS Factory Session = %#v, want identity", opened)
	}
	sessionID := opened.Session.Id
	if sessionID == factorysessions.DefaultSessionID {
		t.Fatalf("shared TTS child session id = %q, want explicit non-default session", sessionID)
	}
	if err := fixture.provider.register(selector, outcome); err != nil {
		support.CloseFactorySessionAt(t, fixture.baseURL, sessionID)
		t.Fatalf("register shared TTS route %q: %v", selector, err)
	}

	scenario := &packagedTTSSharedScenario{
		fixture: fixture, homeDir: homeDir, factoryDir: factoryDir,
		sessionID: sessionID, selector: selector, provider: outcome,
		artifactRoot: outcome.ownedArtifactRoot(),
	}
	fixture.addActiveScenario(sessionID, scenario.artifactRoot)
	t.Cleanup(func() { scenario.cleanup(t) })
	return scenario
}

func (scenario *packagedTTSSharedScenario) cleanup(t testing.TB) {
	if scenario == nil {
		return
	}
	scenario.cleanupOnce.Do(func() {
		support.CloseFactorySessionAt(t, scenario.fixture.baseURL, scenario.sessionID)
		if err := scenario.fixture.provider.unregister(scenario.selector); err != nil {
			t.Errorf("unregister shared TTS route %q: %v", scenario.selector, err)
		}
		removePackagedTTSOwnedPath(t, scenario.artifactRoot)
		removePackagedTTSOwnedPath(t, scenario.factoryDir)
		removePackagedTTSOwnedPath(t, scenario.homeDir)
		scenario.fixture.removeActiveScenario(scenario.sessionID, scenario.artifactRoot)
	})
}

func (fixture *packagedTTSSharedFixture) addActiveScenario(sessionID, artifactRoot string) {
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	fixture.childSessionIDs = append(fixture.childSessionIDs, sessionID)
	fixture.artifactRoots = append(fixture.artifactRoots, artifactRoot)
}

func (fixture *packagedTTSSharedFixture) removeActiveScenario(sessionID, artifactRoot string) {
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	fixture.childSessionIDs = removeString(fixture.childSessionIDs, sessionID)
	fixture.artifactRoots = removeString(fixture.artifactRoots, artifactRoot)
}

func (fixture *packagedTTSSharedFixture) activeScenarioCounts() (int, int) {
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	return len(fixture.childSessionIDs), len(fixture.artifactRoots)
}

func removePackagedTTSOwnedPath(t testing.TB, path string) {
	t.Helper()
	if strings.TrimSpace(path) == "" {
		return
	}
	if err := os.RemoveAll(path); err != nil {
		t.Errorf("remove shared TTS owned path %q: %v", path, err)
		return
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("shared TTS owned path %q remains after cleanup; stat error: %v", path, err)
	}
}

func (fixture *packagedTTSSharedFixture) close(t testing.TB) {
	t.Helper()
	fixture.closeOnce.Do(func() {
		if fixture.command != nil {
			fixture.command.Stop(t)
		}
		closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := fixture.process.Close(closeCtx); err != nil {
			t.Errorf("close shared TTS application process: %v", err)
		}
	})
}

func (fixture *packagedTTSSharedFixture) finalize(t testing.TB) {
	t.Helper()
	fixture.finalizeOnce.Do(func() {
		fixture.close(t)
		assertPackagedTTSListenerClosed(t, fixture.baseURL)
		removePackagedTTSOwnedPath(t, fixture.rootDir)
	})
}

func assertPackagedTTSListenerClosed(t testing.TB, baseURL string) {
	t.Helper()
	client := &http.Client{Timeout: 250 * time.Millisecond}
	endpoint := strings.TrimSuffix(baseURL, "/") + "/status"
	deadline := time.Now().Add(5 * time.Second)
	var lastResponse string
	for time.Now().Before(deadline) {
		response, err := client.Get(endpoint)
		if err != nil {
			return
		}
		body, readErr := io.ReadAll(response.Body)
		response.Body.Close()
		lastResponse = fmt.Sprintf("status=%d body=%q readError=%v", response.StatusCode, strings.TrimSpace(string(body)), readErr)
		time.Sleep(10 * time.Millisecond)
	}
	t.Errorf("shared TTS listener at %s remains reachable after process cleanup: %s", endpoint, lastResponse)
}

func removeString(values []string, target string) []string {
	for index, value := range values {
		if value == target {
			return append(values[:index], values[index+1:]...)
		}
	}
	return values
}

func TestPackagedTTSSharedScenarios(t *testing.T) {
	fixture := newPackagedTTSSharedFixture(t)
	if err := fixture.assertDuplicateRouteRejected(t); err != nil {
		t.Fatal(err)
	}
	fixture.assertInvalidSessionOpenRejected(t)

	var evidence []packagedTTSSharedEvidence
	t.Run("required_input", func(t *testing.T) {
		evidence = append(evidence, runPackagedTTSSharedRequiredInput(t, fixture))
	})
	t.Run("success_work_events", func(t *testing.T) {
		evidence = append(evidence, runPackagedTTSSharedWorkEvents(t, fixture))
	})
	t.Run("optional_voice_format", func(t *testing.T) {
		evidence = append(evidence, runPackagedTTSSharedOptionalVoiceAndFormat(t, fixture))
	})
	t.Run("generic_failure", func(t *testing.T) {
		evidence = append(evidence, runPackagedTTSSharedGenericFailure(t, fixture))
	})
	t.Run("packaged_model_failure", func(t *testing.T) {
		evidence = append(evidence, runPackagedTTSSharedPackagedModelFailure(t, fixture))
	})
	t.Run("concurrent_success_failure_isolation", func(t *testing.T) {
		runPackagedTTSSharedConcurrentIsolation(t, fixture)
	})

	if t.Failed() {
		return
	}
	fixture.assertSharedEligibleEvidence(t, evidence)
	fixture.finalize(t)
}

func runPackagedTTSSharedRequiredInput(
	t *testing.T,
	fixture *packagedTTSSharedFixture,
) packagedTTSSharedEvidence {
	t.Helper()
	text := "functional shared packaged tts required input"
	requestID := "tts-shared-required-input"
	scenario := fixture.openPackagedScenario(t, requestID, false)
	response := postPackagedTTSInvocationAt(t, fixture.baseURL, scenario.sessionID, requestID, text)
	if response.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("shared required-input status = %q, want COMPLETED; response = %#v", response.Status, response)
	}
	assertPackagedTTSInvocationResponseIdentityForSession(t, response, scenario.sessionID, requestID)
	support.WaitForSessionTerminalStatus(t, fixture.baseURL, scenario.sessionID, packagedTTSSharedFixtureTimeout)

	assertPackagedTTSProviderRequestForSession(
		t,
		scenario.provider.lastRequest(),
		text,
		"execute-tts",
		scenario.sessionID,
		requestID,
	)
	if scenario.provider.callCount() != 1 {
		t.Fatalf("shared required-input provider calls = %d, want one", scenario.provider.callCount())
	}
	artifactPath := scenario.provider.lastAudioPath()
	assertPackagedTTSAudioBytes(t, artifactPath)

	listed := listPackagedTTSSessionWork(t, fixture.baseURL, scenario.sessionID)
	outputWork := packagedTTSCompletedMetadataWork(t, listed, artifactPath, response.TraceId)
	audio := packagedTTSExpectedAudioPart(t, artifactPath)
	events := support.GetFactoryEventsForSessionAt(t, fixture.baseURL, scenario.sessionID)
	assertPackagedTTSSuccessEventsForSession(
		t, events, scenario.sessionID, outputWork, text, audio, artifactPath, response.TraceId,
	)
	assertPackagedTTSResponseCorrelatesWithEventsForSession(t, response, events, scenario.sessionID)
	return sharedTTSSharedEvidence(t, "required_input", scenario, requestID, outputWork, events, artifactPath)
}

func runPackagedTTSSharedWorkEvents(
	t *testing.T,
	fixture *packagedTTSSharedFixture,
) packagedTTSSharedEvidence {
	t.Helper()
	text := "functional shared factory tts audio projection"
	requestID := "tts-shared-success-work-events"
	workID := "tts-shared-work-events"
	scenario := fixture.openGenericScenario(t, requestID)
	upserted := upsertPackagedTTSSessionWorkRequest(t, fixture.baseURL, scenario.sessionID, requestID, workID, text)
	if upserted.RequestId != requestID || len(upserted.Works) != 1 || upserted.Works[0].WorkId != workID {
		t.Fatalf("shared Work request response = %#v, want %q/%q", upserted, requestID, workID)
	}
	support.WaitForSessionTerminalStatus(t, fixture.baseURL, scenario.sessionID, packagedTTSSharedFixtureTimeout)

	assertPackagedTTSProviderRequestForSession(
		t,
		scenario.provider.lastRequest(),
		text,
		"tts-dispatch",
		scenario.sessionID,
		requestID,
	)
	if scenario.provider.callCount() != 1 {
		t.Fatalf("shared Work/events provider calls = %d, want one", scenario.provider.callCount())
	}
	artifactPath := scenario.provider.lastAudioPath()
	assertPackagedTTSAudioBytes(t, artifactPath)

	listed := listPackagedTTSSessionWork(t, fixture.baseURL, scenario.sessionID)
	outputWork := factoryTTSCompletedWork(t, listed)
	audio := factoryTTSAudioPart(t, outputWork)
	events := support.GetFactoryEventsForSessionAt(t, fixture.baseURL, scenario.sessionID)
	assertFactoryTTSSuccessEvents(t, events, scenario.sessionID, outputWork, text, audio)
	return sharedTTSSharedEvidence(t, "success_work_events", scenario, requestID, outputWork, events, artifactPath)
}

func runPackagedTTSSharedOptionalVoiceAndFormat(
	t *testing.T,
	fixture *packagedTTSSharedFixture,
) packagedTTSSharedEvidence {
	t.Helper()
	text := "functional shared packaged tts optional voice format"
	requestID := "tts-shared-optional-voice-format"
	voice := "alloy"
	format := "mp3"
	scenario := fixture.openPackagedScenario(t, requestID, true)
	response := postPackagedTTSInvocationWithArgsAt(
		t,
		fixture.baseURL,
		scenario.sessionID,
		requestID,
		map[string]any{"text": text, "voice": voice, "format": format},
	)
	if response.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("shared optional voice/format status = %q, want COMPLETED; response = %#v", response.Status, response)
	}
	assertPackagedTTSInvocationResponseIdentityForSession(t, response, scenario.sessionID, requestID)
	support.WaitForSessionTerminalStatus(t, fixture.baseURL, scenario.sessionID, packagedTTSSharedFixtureTimeout)

	request := scenario.provider.lastRequest()
	assertPackagedTTSProviderRequestForSession(t, request, text, "execute-tts", scenario.sessionID, requestID)
	if request == nil {
		t.Fatal("shared optional voice/format provider request is nil")
	}
	if voiceBinding, ok := modelBindingJSON(request.ModelBindings, "voice"); !ok {
		t.Fatalf("shared model bindings = %#v, want voice slot binding", request.ModelBindings)
	} else if got := stringValueFromBindingJSON(voiceBinding, "name"); got != voice {
		t.Fatalf("shared voice binding name = %q, want %q", got, voice)
	}
	if formatBinding, ok := modelBindingJSON(request.ModelBindings, "format"); !ok {
		t.Fatalf("shared model bindings = %#v, want format slot binding", request.ModelBindings)
	} else if got := stringValueFromBindingJSON(formatBinding, "name"); got != format {
		t.Fatalf("shared format binding name = %q, want %q", got, format)
	}
	if scenario.provider.callCount() != 1 {
		t.Fatalf("shared optional voice/format provider calls = %d, want one", scenario.provider.callCount())
	}
	artifactPath := scenario.provider.lastAudioPath()
	assertPackagedTTSAudioBytes(t, artifactPath)

	listed := listPackagedTTSSessionWork(t, fixture.baseURL, scenario.sessionID)
	outputWork := packagedTTSCompletedMetadataWork(t, listed, artifactPath, response.TraceId)
	audio := packagedTTSExpectedAudioPart(t, artifactPath)
	events := support.GetFactoryEventsForSessionAt(t, fixture.baseURL, scenario.sessionID)
	assertPackagedTTSSuccessEventsForSession(
		t, events, scenario.sessionID, outputWork, text, audio, artifactPath, response.TraceId,
	)
	assertPackagedTTSResponseCorrelatesWithEventsForSession(t, response, events, scenario.sessionID)
	return sharedTTSSharedEvidence(t, "optional_voice_format", scenario, requestID, outputWork, events, artifactPath)
}

func runPackagedTTSSharedGenericFailure(
	t *testing.T,
	fixture *packagedTTSSharedFixture,
) packagedTTSSharedEvidence {
	t.Helper()
	text := "functional shared factory tts failure"
	requestID := "tts-shared-generic-failure"
	workID := "tts-shared-generic-failure-work"
	failureMessage := "tts backend failed"
	scenario := fixture.openGenericFailureScenario(t, requestID, failureMessage)
	upserted := upsertPackagedTTSSessionWorkRequest(t, fixture.baseURL, scenario.sessionID, requestID, workID, text)
	if upserted.RequestId != requestID || len(upserted.Works) != 1 || upserted.Works[0].WorkId != workID {
		t.Fatalf("shared failure Work request response = %#v, want %q/%q", upserted, requestID, workID)
	}
	support.WaitForSessionTerminalStatus(t, fixture.baseURL, scenario.sessionID, packagedTTSSharedFixtureTimeout)

	request := scenario.provider.lastRequest()
	assertPackagedTTSProviderRequestForSession(t, request, text, "tts-dispatch", scenario.sessionID, requestID)
	if scenario.provider.callCount() != 1 {
		t.Fatalf("shared generic failure provider calls = %d, want one", scenario.provider.callCount())
	}
	assertPackagedTTSNoArtifact(t, scenario.provider)

	listed := listPackagedTTSSessionWork(t, fixture.baseURL, scenario.sessionID)
	failedWork := factoryTTSFailedWork(t, listed)
	events := support.GetFactoryEventsForSessionAt(t, fixture.baseURL, scenario.sessionID)
	observed := collectFactoryTTSDispatchEvents(t, events, scenario.sessionID)
	workID = *failedWork.WorkId
	traceID := factoryTTSRequiredTraceID(t, observed.workRequest)
	dispatchID := factoryTTSRequiredContextID(t, observed.dispatchRequest, "dispatch")
	assertFactoryTTSContextCorrelation(t, observed, workID, requestID, traceID, dispatchID)
	assertFactoryTTSWorkRequest(t, observed.workRequest, workID, text)
	assertFactoryTTSDispatchRequest(t, observed.dispatchRequest, workID)
	assertFactoryTTSFailureModelEvents(t, observed, failureMessage)
	assertFactoryTTSFailureDispatchResponse(t, observed.dispatchResponse, workID, text, failureMessage)
	assertPackagedTTSNoArtifactEvents(t, events)
	return sharedTTSSharedEvidence(t, "generic_failure", scenario, requestID, failedWork, events, "")
}

func runPackagedTTSSharedPackagedModelFailure(
	t *testing.T,
	fixture *packagedTTSSharedFixture,
) packagedTTSSharedEvidence {
	t.Helper()
	text := "functional shared packaged tts model failure"
	requestID := "tts-shared-packaged-model-failure"
	failureMessage := "omnivoice invoke failed: exit status 1"
	scenario := fixture.openPackagedFailureScenario(t, requestID, failureMessage, false)
	response := postPackagedTTSInvocationAt(t, fixture.baseURL, scenario.sessionID, requestID, text)
	if response.Status != factoryapi.InvocationTerminalStatusFailed {
		t.Fatalf("shared packaged model failure status = %q, want FAILED; response = %#v", response.Status, response)
	}
	if response.ErrorCode == nil || *response.ErrorCode != factoryapi.INVOCATIONTTSGENERATIONFAILED {
		t.Fatalf("shared packaged model failure errorCode = %#v, want INVOCATION_TTS_GENERATION_FAILED", response.ErrorCode)
	}
	if primaryResultContainsTTSArtifactMetadata(t, response.PrimaryResult) {
		t.Fatalf("shared packaged model failure primary result = %#v, want no success-shaped artifact metadata", response.PrimaryResult)
	}
	assertPackagedTTSInvocationResponseIdentityForSession(t, response, scenario.sessionID, requestID)
	support.WaitForSessionTerminalStatus(t, fixture.baseURL, scenario.sessionID, packagedTTSSharedFixtureTimeout)

	request := scenario.provider.lastRequest()
	assertPackagedTTSProviderRequestForSession(t, request, text, "execute-tts", scenario.sessionID, requestID)
	if scenario.provider.callCount() != 1 {
		t.Fatalf("shared packaged model failure provider calls = %d, want one", scenario.provider.callCount())
	}
	assertPackagedTTSNoArtifact(t, scenario.provider)

	listed := listPackagedTTSSessionWork(t, fixture.baseURL, scenario.sessionID)
	failedWork := factoryTTSFailedWork(t, listed)
	events := support.GetFactoryEventsForSessionAt(t, fixture.baseURL, scenario.sessionID)
	observed := collectFactoryTTSDispatchEvents(t, events, scenario.sessionID)
	workID := *failedWork.WorkId
	traceID := factoryTTSRequiredTraceID(t, observed.workRequest)
	dispatchID := factoryTTSRequiredContextID(t, observed.dispatchRequest, "dispatch")
	assertFactoryTTSContextCorrelation(t, observed, workID, requestID, traceID, dispatchID)
	assertPackagedTTSWorkRequest(t, observed.workRequest, workID, text)
	assertPackagedTTSDispatchRequest(t, observed.dispatchRequest, workID)
	assertFactoryTTSFailureModelEvents(t, observed, failureMessage)
	assertPackagedTTSFailureDispatchResponse(t, observed.dispatchResponse, workID, text, failureMessage)
	assertPackagedTTSResponseCorrelatesWithEventsForSession(t, response, events, scenario.sessionID)
	assertPackagedTTSNoArtifactEvents(t, events)
	return sharedTTSSharedEvidence(t, "packaged_model_failure", scenario, requestID, failedWork, events, "")
}

type packagedTTSConcurrentInvocationResult struct {
	name     string
	response factoryapi.InvocationResponse
	err      error
}

func runPackagedTTSSharedConcurrentIsolation(
	t *testing.T,
	fixture *packagedTTSSharedFixture,
) {
	t.Helper()
	const (
		successRequestID = "tts-shared-concurrent-success"
		failureRequestID = "tts-shared-concurrent-failure"
		successText      = "functional shared concurrent packaged tts success"
		failureText      = "functional shared concurrent packaged tts failure"
		voice            = "alloy"
		format           = "mp3"
		failureMessage   = "omnivoice concurrent invoke failed: exit status 1"
	)
	success := fixture.openPackagedScenario(t, successRequestID, true)
	failure := fixture.openPackagedFailureScenario(t, failureRequestID, failureMessage, true)

	barrier := newPackagedTTSInferenceBarrier(2)
	fixture.provider.setInferenceBarrier(barrier)
	results := make(chan packagedTTSConcurrentInvocationResult, 2)
	go func() {
		response, err := postPackagedTTSInvocationWithArgsContext(
			t.Context(), fixture.baseURL, success.sessionID, successRequestID,
			map[string]any{"text": successText, "voice": voice, "format": format},
		)
		results <- packagedTTSConcurrentInvocationResult{name: "success", response: response, err: err}
	}()
	go func() {
		response, err := postPackagedTTSInvocationWithArgsContext(
			t.Context(), fixture.baseURL, failure.sessionID, failureRequestID,
			map[string]any{"text": failureText, "voice": voice, "format": format},
		)
		results <- packagedTTSConcurrentInvocationResult{name: "failure", response: response, err: err}
	}()

	var successResult, failureResult packagedTTSConcurrentInvocationResult
	for range 2 {
		result := <-results
		switch result.name {
		case "success":
			successResult = result
		case "failure":
			failureResult = result
		default:
			t.Fatalf("concurrent TTS result has unknown name %q", result.name)
		}
	}
	fixture.provider.setInferenceBarrier(nil)
	if successResult.err != nil {
		t.Fatalf("concurrent success invocation error = %v", successResult.err)
	}
	if failureResult.err != nil {
		t.Fatalf("concurrent failure invocation error = %v", failureResult.err)
	}
	if successResult.response.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("concurrent success response = %#v, want COMPLETED", successResult.response)
	}
	if failureResult.response.Status != factoryapi.InvocationTerminalStatusFailed {
		t.Fatalf("concurrent failure response = %#v, want FAILED", failureResult.response)
	}
	if failureResult.response.ErrorCode == nil || *failureResult.response.ErrorCode != factoryapi.INVOCATIONTTSGENERATIONFAILED {
		t.Fatalf("concurrent failure errorCode = %#v, want INVOCATION_TTS_GENERATION_FAILED", failureResult.response.ErrorCode)
	}
	assertPackagedTTSInvocationResponseIdentityForSession(t, successResult.response, success.sessionID, successRequestID)
	assertPackagedTTSInvocationResponseIdentityForSession(t, failureResult.response, failure.sessionID, failureRequestID)
	if primaryResultContainsTTSArtifactMetadata(t, failureResult.response.PrimaryResult) {
		t.Fatalf("concurrent failure primary result = %#v, want no success-shaped artifact metadata", failureResult.response.PrimaryResult)
	}

	support.WaitForSessionTerminalStatus(t, fixture.baseURL, success.sessionID, packagedTTSSharedFixtureTimeout)
	support.WaitForSessionTerminalStatus(t, fixture.baseURL, failure.sessionID, packagedTTSSharedFixtureTimeout)
	successRequest := success.provider.lastRequest()
	failureRequest := failure.provider.lastRequest()
	assertPackagedTTSProviderRequestForSession(t, successRequest, successText, "execute-tts", success.sessionID, successRequestID)
	assertPackagedTTSProviderRequestForSession(t, failureRequest, failureText, "execute-tts", failure.sessionID, failureRequestID)
	assertPackagedTTSOptionalBindings(t, successRequest, voice, format)
	assertPackagedTTSOptionalBindings(t, failureRequest, voice, format)
	if success.provider.callCount() != 1 || failure.provider.callCount() != 1 {
		t.Fatalf("concurrent provider calls = success:%d failure:%d, want one each", success.provider.callCount(), failure.provider.callCount())
	}

	successArtifactPath := success.provider.lastAudioPath()
	assertPackagedTTSAudioBytes(t, successArtifactPath)
	assertPackagedTTSNoArtifact(t, failure.provider)
	successListed := listPackagedTTSSessionWork(t, fixture.baseURL, success.sessionID)
	failureListed := listPackagedTTSSessionWork(t, fixture.baseURL, failure.sessionID)
	successWork := packagedTTSCompletedMetadataWork(t, successListed, successArtifactPath, successResult.response.TraceId)
	failureWork := factoryTTSFailedWork(t, failureListed)
	successAudio := packagedTTSExpectedAudioPart(t, successArtifactPath)
	successEvents := support.GetFactoryEventsForSessionAt(t, fixture.baseURL, success.sessionID)
	failureEvents := support.GetFactoryEventsForSessionAt(t, fixture.baseURL, failure.sessionID)
	assertPackagedTTSSuccessEventsForSession(
		t, successEvents, success.sessionID, successWork, successText, successAudio,
		successArtifactPath, successResult.response.TraceId,
	)
	assertPackagedTTSResponseCorrelatesWithEventsForSession(t, successResult.response, successEvents, success.sessionID)
	failureObserved := collectFactoryTTSDispatchEvents(t, failureEvents, failure.sessionID)
	failureWorkID := *failureWork.WorkId
	failureTraceID := factoryTTSRequiredTraceID(t, failureObserved.workRequest)
	failureDispatchID := factoryTTSRequiredContextID(t, failureObserved.dispatchRequest, "dispatch")
	assertFactoryTTSContextCorrelation(t, failureObserved, failureWorkID, failureRequestID, failureTraceID, failureDispatchID)
	assertPackagedTTSWorkRequest(t, failureObserved.workRequest, failureWorkID, failureText)
	assertPackagedTTSDispatchRequest(t, failureObserved.dispatchRequest, failureWorkID)
	assertFactoryTTSFailureModelEvents(t, failureObserved, failureMessage)
	assertPackagedTTSFailureDispatchResponse(t, failureObserved.dispatchResponse, failureWorkID, failureText, failureMessage)
	assertPackagedTTSResponseCorrelatesWithEventsForSession(t, failureResult.response, failureEvents, failure.sessionID)
	assertPackagedTTSNoArtifactEvents(t, failureEvents)

	successEvidence := sharedTTSSharedEvidence(t, "concurrent_success", success, successRequestID, successWork, successEvents, successArtifactPath)
	failureEvidence := sharedTTSSharedEvidence(t, "concurrent_failure", failure, failureRequestID, failureWork, failureEvents, "")
	assertPackagedTTSConcurrentEvidenceDisjoint(t, successEvidence, failureEvidence)
	success.cleanup(t)
	failure.cleanup(t)
}

func assertPackagedTTSOptionalBindings(
	t *testing.T,
	request *workerexecution.ProviderInferenceRequest,
	wantVoice, wantFormat string,
) {
	t.Helper()
	if voiceBinding, ok := modelBindingJSON(request.ModelBindings, "voice"); !ok {
		t.Fatalf("shared model bindings = %#v, want voice slot binding", request.ModelBindings)
	} else if got := stringValueFromBindingJSON(voiceBinding, "name"); got != wantVoice {
		t.Fatalf("shared voice binding name = %q, want %q", got, wantVoice)
	}
	if formatBinding, ok := modelBindingJSON(request.ModelBindings, "format"); !ok {
		t.Fatalf("shared model bindings = %#v, want format slot binding", request.ModelBindings)
	} else if got := stringValueFromBindingJSON(formatBinding, "name"); got != wantFormat {
		t.Fatalf("shared format binding name = %q, want %q", got, wantFormat)
	}
}

func assertPackagedTTSNoArtifact(t testing.TB, outcome packagedTTSProviderOutcome) {
	t.Helper()
	if path := outcome.lastAudioPath(); strings.TrimSpace(path) != "" {
		t.Fatalf("failed shared TTS outcome recorded audio path %q, want no artifact", path)
	}
	root := outcome.ownedArtifactRoot()
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("read failed shared TTS artifact root %q: %v", root, err)
	}
	if len(entries) != 0 {
		t.Fatalf("failed shared TTS artifact root %q contains %d entries, want zero", root, len(entries))
	}
}

func assertPackagedTTSNoArtifactEvents(t testing.TB, events []factoryapi.FactoryEvent) {
	t.Helper()
	for _, event := range events {
		if event.Type == factoryapi.FactoryEventTypeArtifactCreated {
			t.Fatalf("failed shared TTS outcome emitted ARTIFACT_CREATED event: %#v", event)
		}
	}
}

func assertPackagedTTSConcurrentEvidenceDisjoint(
	t testing.TB,
	first, second packagedTTSSharedEvidence,
) {
	t.Helper()
	seen := make(map[string]string, 8)
	for _, item := range []packagedTTSSharedEvidence{first, second} {
		for label, value := range map[string]string{
			"session": item.sessionID, "request": item.requestID, "work": item.workID,
			"model request": item.modelRequest, "trace": item.traceID, "dispatch": item.dispatchID,
		} {
			if strings.TrimSpace(value) == "" {
				t.Fatalf("concurrent %s %s identity is empty", item.name, label)
			}
			if previous, exists := seen[value]; exists {
				t.Fatalf("concurrent TTS identity %q is shared by %s and %s", value, previous, item.name)
			}
			seen[value] = item.name
		}
		if strings.TrimSpace(item.artifactPath) != "" {
			if previous, exists := seen[item.artifactPath]; exists {
				t.Fatalf("concurrent TTS artifact %q is shared by %s and %s", item.artifactPath, previous, item.name)
			}
			seen[item.artifactPath] = item.name
		}
		for _, eventID := range item.eventIDs {
			if strings.TrimSpace(eventID) == "" {
				t.Fatalf("concurrent %s event identity is empty", item.name)
			}
			eventKey := item.sessionID + "\x00" + eventID
			if previous, exists := seen[eventKey]; exists {
				t.Fatalf("concurrent TTS event identity %q is duplicated for session %q by %s and %s", eventID, item.sessionID, previous, item.name)
			}
			seen[eventKey] = item.name
		}
	}
}

func sharedTTSSharedEvidence(
	t *testing.T,
	name string,
	scenario *packagedTTSSharedScenario,
	requestID string,
	outputWork factoryapi.Work,
	events []factoryapi.FactoryEvent,
	artifactPath string,
) packagedTTSSharedEvidence {
	t.Helper()
	observed := collectFactoryTTSDispatchEvents(t, events, scenario.sessionID)
	modelResponse, err := observed.modelRequest.Payload.AsModelRequestEventPayload()
	if err != nil {
		t.Fatalf("decode shared %s MODEL_REQUEST: %v", name, err)
	}
	if strings.TrimSpace(modelResponse.ModelRequestId) == "" {
		t.Fatalf("shared %s model request identity is empty", name)
	}
	eventIDs := make([]string, 0, len(events))
	seenEventIDs := make(map[string]struct{}, len(events))
	for _, event := range events {
		if strings.TrimSpace(event.Id) == "" {
			t.Fatalf("shared %s event identity is empty", name)
		}
		if _, exists := seenEventIDs[event.Id]; exists {
			t.Fatalf("shared %s event identity %q is duplicated", name, event.Id)
		}
		seenEventIDs[event.Id] = struct{}{}
		eventIDs = append(eventIDs, event.Id)
	}
	return packagedTTSSharedEvidence{
		name: name, sessionID: scenario.sessionID, requestID: requestID,
		workID:       support.StringPointerValue(outputWork.WorkId),
		modelRequest: modelResponse.ModelRequestId,
		traceID:      factoryTTSRequiredTraceID(t, observed.workRequest),
		dispatchID:   factoryTTSRequiredContextID(t, observed.dispatchRequest, "dispatch"),
		eventIDs:     eventIDs,
		artifactPath: artifactPath, artifactRoot: scenario.artifactRoot,
		homeDir: scenario.homeDir, factoryDir: scenario.factoryDir,
	}
}

func (fixture *packagedTTSSharedFixture) assertDuplicateRouteRejected(t *testing.T) error {
	t.Helper()
	selector := "tts-shared-duplicate-selector"
	first := newPackagedTTSFakeProvider(t, []byte(packagedTTSFakeAudioFixture))
	second := newPackagedTTSFakeProvider(t, []byte(packagedTTSFakeAudioFixture))
	if err := fixture.provider.register(selector, first); err != nil {
		return fmt.Errorf("register first duplicate-selector route: %w", err)
	}
	if err := fixture.provider.register(selector, second); err == nil {
		_ = fixture.provider.unregister(selector)
		return fmt.Errorf("duplicate TTS route selector %q was accepted", selector)
	}
	if err := fixture.provider.unregister(selector); err != nil {
		return fmt.Errorf("cleanup duplicate-selector route: %w", err)
	}
	for label, root := range map[string]string{
		"duplicate-selector artifact": first.artifactRoot,
		"rejected duplicate artifact": second.artifactRoot,
	} {
		if err := os.RemoveAll(root); err != nil {
			return fmt.Errorf("remove %s root %q: %w", label, root, err)
		}
		if _, err := os.Stat(root); !os.IsNotExist(err) {
			return fmt.Errorf("%s root %q remains; stat error: %v", label, root, err)
		}
	}
	if fixture.provider.routeCount() != 0 {
		return fmt.Errorf("duplicate TTS route cleanup changed registry count to %d, want zero", fixture.provider.routeCount())
	}
	return nil
}

func (fixture *packagedTTSSharedFixture) assertInvalidSessionOpenRejected(t *testing.T) {
	t.Helper()
	missingDir := filepath.Join(fixture.rootDir, "missing-child-factory")
	status, body := tryOpenPackagedTTSFactorySession(t, fixture.baseURL, missingDir)
	if status != http.StatusBadRequest {
		t.Fatalf("invalid shared TTS session open status = %d, want 400: %s", status, strings.TrimSpace(string(body)))
	}
	if fixture.provider.routeCount() != 0 {
		t.Fatalf("invalid shared TTS session open left %d provider routes", fixture.provider.routeCount())
	}
}

func (fixture *packagedTTSSharedFixture) assertSharedEligibleEvidence(
	t *testing.T,
	evidence []packagedTTSSharedEvidence,
) {
	t.Helper()
	if fixture.processBuilds != 1 || fixture.api.startCount() != 1 {
		t.Fatalf("shared TTS process starts = root:%d http:%d, want one each", fixture.processBuilds, fixture.api.startCount())
	}
	if len(evidence) != 5 {
		t.Fatalf("shared TTS eligible evidence count = %d, want five success/failure child scenarios", len(evidence))
	}
	childSessions, artifactRoots := fixture.activeScenarioCounts()
	if childSessions != 0 || artifactRoots != 0 {
		t.Fatalf("shared TTS active child resources = sessions:%d artifacts:%d, want zero", childSessions, artifactRoots)
	}
	seen := make(map[string]string, len(evidence))
	for _, item := range evidence {
		for label, value := range map[string]string{
			"session": item.sessionID, "request": item.requestID, "work": item.workID,
			"model request": item.modelRequest, "trace": item.traceID, "dispatch": item.dispatchID,
		} {
			if strings.TrimSpace(value) == "" {
				t.Fatalf("shared %s %s identity is empty", item.name, label)
			}
			if previous, exists := seen[value]; exists {
				t.Fatalf("shared TTS %s identity %q is reused by %s and %s", label, value, previous, item.name)
			}
			seen[value] = item.name
		}
		for _, eventID := range item.eventIDs {
			if strings.TrimSpace(eventID) == "" {
				t.Fatalf("shared %s event identity is empty", item.name)
			}
			eventKey := item.sessionID + "\x00" + eventID
			if previous, exists := seen[eventKey]; exists {
				t.Fatalf("shared TTS event identity %q is duplicated for session %q by %s and %s", eventID, item.sessionID, previous, item.name)
			}
			seen[eventKey] = item.name
		}
		if strings.TrimSpace(item.artifactPath) != "" {
			if previous, exists := seen[item.artifactPath]; exists {
				t.Fatalf("shared TTS artifact %q is reused by %s and %s", item.artifactPath, previous, item.name)
			}
			seen[item.artifactPath] = item.name
		}
		assertPackagedTTSSessionDeleted(t, fixture.baseURL, item.sessionID)
		if _, err := os.Stat(item.artifactRoot); !os.IsNotExist(err) {
			t.Fatalf("shared %s artifact root = %q, want removed; stat error = %v", item.name, item.artifactRoot, err)
		}
		if _, err := os.Stat(item.factoryDir); !os.IsNotExist(err) {
			t.Fatalf("shared %s Factory definition root = %q, want removed; stat error = %v", item.name, item.factoryDir, err)
		}
		if _, err := os.Stat(item.homeDir); !os.IsNotExist(err) {
			t.Fatalf("shared %s temporary state root = %q, want removed; stat error = %v", item.name, item.homeDir, err)
		}
	}
	if got := fixture.provider.routeCount(); got != 0 {
		t.Fatalf("shared TTS provider route count after child cleanup = %d, want zero", got)
	}
}

func assertPackagedTTSAudioBytes(t testing.TB, artifactPath string) {
	t.Helper()
	if strings.TrimSpace(artifactPath) == "" {
		t.Fatal("shared TTS artifact path is empty, want audio file")
	}
	got, err := os.ReadFile(artifactPath)
	if err != nil {
		t.Fatalf("read shared TTS artifact %q: %v", artifactPath, err)
	}
	if !bytes.Equal(got, []byte(packagedTTSFakeAudioFixture)) {
		t.Fatalf("shared TTS artifact bytes = %q, want exact fixture %q", got, packagedTTSFakeAudioFixture)
	}
}

func listPackagedTTSSessionWork(
	t testing.TB,
	baseURL, sessionID string,
) factoryapi.ListWorkResponse {
	t.Helper()
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + url.PathEscape(sessionID) + "/work"
	return support.GetJSON[factoryapi.ListWorkResponse](t, endpoint)
}

func upsertPackagedTTSSessionWorkRequest(
	t testing.TB,
	baseURL, sessionID, requestID, workID, text string,
) factoryapi.UpsertWorkRequestResponse {
	t.Helper()
	workType := "task"
	part := factoryapi.WorkContentPart{}
	if err := part.FromWorkTextContentPart(factoryapi.WorkTextContentPart{
		Type: factoryapi.WorkContentPartTypeText,
		Text: text,
	}); err != nil {
		t.Fatalf("build shared TTS text content: %v", err)
	}
	content := factoryapi.WorkContent{part}
	works := []factoryapi.Work{{
		Name:         "shared tts work events",
		WorkId:       &workID,
		WorkTypeName: &workType,
		Content:      &content,
	}}
	payload, err := json.Marshal(factoryapi.WorkRequest{
		RequestId: requestID,
		Type:      factoryapi.WorkRequestTypeFactoryRequestBatch,
		Works:     &works,
	})
	if err != nil {
		t.Fatalf("marshal shared TTS Work request: %v", err)
	}
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" +
		url.PathEscape(sessionID) + "/work-requests/" + url.PathEscape(requestID)
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPut, endpoint, bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("build shared TTS Work request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("PUT %s: %v", endpoint, err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("PUT %s status = %d, want success: %s", endpoint, response.StatusCode, strings.TrimSpace(string(body)))
	}
	var result factoryapi.UpsertWorkRequestResponse
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatalf("decode shared TTS Work request response: %v", err)
	}
	return result
}

func assertPackagedTTSSessionDeleted(t *testing.T, baseURL, sessionID string) {
	t.Helper()
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + url.PathEscape(sessionID)
	response, err := http.Get(endpoint)
	if err != nil {
		t.Fatalf("GET deleted shared TTS session %q: %v", sessionID, err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("GET deleted shared TTS session %q status = %d, want 404: %s", sessionID, response.StatusCode, strings.TrimSpace(string(body)))
	}
}

func tryOpenPackagedTTSFactorySession(
	t testing.TB,
	baseURL, folderPath string,
) (int, []byte) {
	t.Helper()
	payload, err := json.Marshal(factoryapi.OpenFactorySessionRequest{FolderPath: folderPath})
	if err != nil {
		t.Fatalf("marshal invalid shared TTS session request: %v", err)
	}
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions"
	response, err := http.Post(endpoint, "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("POST invalid shared TTS session request: %v", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read invalid shared TTS session response: %v", err)
	}
	return response.StatusCode, body
}
