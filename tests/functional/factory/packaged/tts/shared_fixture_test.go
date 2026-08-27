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

// packagedTTSSharedProvider routes each controlled model outcome by the
// request identity carried through the real Providers boundary. The registry
// is synchronized because the same process is deliberately usable by the
// selected concurrent-isolation story that follows this one.
type packagedTTSSharedProvider struct {
	testutil.ProviderServiceAdapter

	mu     sync.Mutex
	routes map[string]*packagedTTSFakeProvider
}

func newPackagedTTSSharedProvider() *packagedTTSSharedProvider {
	provider := &packagedTTSSharedProvider{routes: make(map[string]*packagedTTSFakeProvider)}
	provider.ProviderServiceAdapter.InferFunc = provider.Infer
	return provider
}

func (provider *packagedTTSSharedProvider) register(
	selector string,
	fake *packagedTTSFakeProvider,
) error {
	selector = strings.TrimSpace(selector)
	if selector == "" {
		return fmt.Errorf("TTS route selector is required")
	}
	if fake == nil {
		return fmt.Errorf("TTS route %q has no provider outcome", selector)
	}
	provider.mu.Lock()
	defer provider.mu.Unlock()
	if _, exists := provider.routes[selector]; exists {
		return fmt.Errorf("TTS route selector %q is already registered", selector)
	}
	provider.routes[selector] = fake
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

func (provider *packagedTTSSharedProvider) Infer(
	ctx context.Context,
	request workerexecution.ProviderInferenceRequest,
) (workerexecution.InferenceResponse, error) {
	selector := strings.TrimSpace(request.Correlation.RequestID)
	if selector == "" {
		selector = strings.TrimSpace(request.Dispatch.Execution.RequestID)
	}
	provider.mu.Lock()
	fake := provider.routes[selector]
	provider.mu.Unlock()
	if fake == nil {
		return workerexecution.InferenceResponse{}, fmt.Errorf(
			"no packaged TTS route registered for request %q",
			selector,
		)
	}
	return fake.Infer(ctx, request)
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
	api             *packagedTTSSharedHTTPServer
	provider        *packagedTTSSharedProvider
	childSessionIDs []string
	artifactRoots   []string
	processBuilds   int
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
	support.StartProcessCommand(t, process, inputs.Input)

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
	provider     *packagedTTSFakeProvider
	artifactRoot string
}

type packagedTTSSharedEvidence struct {
	name         string
	sessionID    string
	requestID    string
	workID       string
	modelRequest string
	traceID      string
	dispatchID   string
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
	return fixture.openScenario(t, homeDir, factoryDir, selector)
}

func (fixture *packagedTTSSharedFixture) openGenericScenario(
	t *testing.T,
	selector string,
) *packagedTTSSharedScenario {
	t.Helper()
	homeDir := t.TempDir()
	factoryDir := scaffoldFactoryTTSAudioDispatch(t)
	return fixture.openScenario(t, homeDir, factoryDir, selector)
}

func (fixture *packagedTTSSharedFixture) openScenario(
	t *testing.T,
	homeDir, factoryDir, selector string,
) *packagedTTSSharedScenario {
	t.Helper()
	fake := newPackagedTTSFakeProvider(t, []byte(packagedTTSFakeAudioFixture))
	opened := support.OpenFactorySessionAt(t, fixture.baseURL, factoryDir)
	if opened.Session == nil || strings.TrimSpace(opened.Session.Id) == "" {
		t.Fatalf("opened shared TTS Factory Session = %#v, want identity", opened)
	}
	sessionID := opened.Session.Id
	if sessionID == factorysessions.DefaultSessionID {
		t.Fatalf("shared TTS child session id = %q, want explicit non-default session", sessionID)
	}
	if err := fixture.provider.register(selector, fake); err != nil {
		support.CloseFactorySessionAt(t, fixture.baseURL, sessionID)
		t.Fatalf("register shared TTS route %q: %v", selector, err)
	}

	scenario := &packagedTTSSharedScenario{
		fixture: fixture, homeDir: homeDir, factoryDir: factoryDir,
		sessionID: sessionID, selector: selector, provider: fake,
		artifactRoot: fake.artifactRoot,
	}
	fixture.childSessionIDs = append(fixture.childSessionIDs, sessionID)
	fixture.artifactRoots = append(fixture.artifactRoots, fake.artifactRoot)
	t.Cleanup(func() {
		support.CloseFactorySessionAt(t, fixture.baseURL, sessionID)
		if err := fixture.provider.unregister(selector); err != nil {
			t.Errorf("unregister shared TTS route %q: %v", selector, err)
		}
		fixture.removeActiveScenario(sessionID, fake.artifactRoot)
	})
	return scenario
}

func (fixture *packagedTTSSharedFixture) removeActiveScenario(sessionID, artifactRoot string) {
	fixture.childSessionIDs = removeString(fixture.childSessionIDs, sessionID)
	fixture.artifactRoots = removeString(fixture.artifactRoots, artifactRoot)
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

	if t.Failed() {
		return
	}
	fixture.assertSharedSuccessEvidence(t, evidence)
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
	return packagedTTSSharedEvidence{
		name: name, sessionID: scenario.sessionID, requestID: requestID,
		workID:       support.StringPointerValue(outputWork.WorkId),
		modelRequest: modelResponse.ModelRequestId,
		traceID:      factoryTTSRequiredTraceID(t, observed.workRequest),
		dispatchID:   factoryTTSRequiredContextID(t, observed.dispatchRequest, "dispatch"),
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

func (fixture *packagedTTSSharedFixture) assertSharedSuccessEvidence(
	t *testing.T,
	evidence []packagedTTSSharedEvidence,
) {
	t.Helper()
	if fixture.processBuilds != 1 || fixture.api.startCount() != 1 {
		t.Fatalf("shared TTS process starts = root:%d http:%d, want one each", fixture.processBuilds, fixture.api.startCount())
	}
	if len(evidence) != 3 {
		t.Fatalf("shared TTS evidence count = %d, want three successful child scenarios", len(evidence))
	}
	if len(fixture.childSessionIDs) != 0 || len(fixture.artifactRoots) != 0 {
		t.Fatalf("shared TTS active child resources = sessions:%d artifacts:%d, want zero", len(fixture.childSessionIDs), len(fixture.artifactRoots))
	}
	seen := make(map[string]string, len(evidence))
	for _, item := range evidence {
		for label, value := range map[string]string{
			"session": item.sessionID, "request": item.requestID, "work": item.workID,
			"model request": item.modelRequest, "trace": item.traceID,
			"dispatch": item.dispatchID, "artifact": item.artifactPath,
		} {
			if strings.TrimSpace(value) == "" {
				t.Fatalf("shared %s %s identity is empty", item.name, label)
			}
			if previous, exists := seen[value]; exists {
				t.Fatalf("shared TTS %s identity %q is reused by %s and %s", label, value, previous, item.name)
			}
			seen[value] = item.name
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
