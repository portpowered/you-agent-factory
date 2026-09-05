package omni_artifact_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	models "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const omniModelSource = "hf://unsloth/gemma-4-E4B-it-GGUF/gemma-4-E4B-it-Q4_K_M.gguf@bfc15c382204943c3a8fff0c750b94ae2364d7a3"

const omniFactoryFunctionalTimeout = 60 * time.Second

var sharedOmniFactoryProcess *omniFactoryProcessGroup

func TestMain(m *testing.M) {
	code := m.Run()
	if sharedOmniFactoryProcess != nil {
		if err := sharedOmniFactoryProcess.close(); err != nil {
			fmt.Fprintf(os.Stderr, "close shared OMNI Factory process: %v\n", err)
			if code == 0 {
				code = 1
			}
		}
	}
	os.Exit(code)
}

// TestFactorySessionOmniArtifactJourney keeps the story's executable spine on
// the public Factory Session boundary: one root-built Process executes an
// explicit --session run, and the only model effect replaced is the
// provider-neutral protocol edge.
func TestFactorySessionOmniArtifactJourney(t *testing.T) {
	t.Parallel()

	const wantText = "Exact Unicode: café, 東京, and 🌍"
	fixture := newFactoryFixture(t, wantText)
	response := fixture.invoke(t, "Return the exact fixture")
	if response.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("invocation status = %q, want COMPLETED: %#v", response.Status, response)
	}
	if response.PrimaryResult == nil || len(*response.PrimaryResult) != 1 {
		t.Fatalf("primary result = %#v, want one text part", response.PrimaryResult)
	}
	part, err := (*response.PrimaryResult)[0].AsWorkTextContentPart()
	if err != nil || part.Text != wantText {
		t.Fatalf("primary result part = %#v, err=%v, want exact %q", part, err, wantText)
	}
	if fixture.protocol.Calls() != 1 {
		t.Fatalf("protocol calls = %d, want one", fixture.protocol.Calls())
	}
	publicWork := fixture.listWork(t)
	events := fixture.recording(t)
	journey := assertSuccessfulJourney(t, events, "Return the exact fixture", wantText)
	assertPublicWorkProjection(t, publicWork, journey, wantText)
	t.Logf("Factory Session=%s request=%s trace=%s work=%s artifact=%s mime=%s size=%d protocolCalls=%d",
		fixture.sessionID, response.RequestId, response.TraceId, requiredString(journey.outputWork.WorkId),
		journey.artifactID, requiredString(journey.outputPart.ContentType), len([]byte(wantText)), fixture.protocol.Calls())
}

func TestFactorySessionOmniArtifactReplayPreservesOrderAndLineage(t *testing.T) {
	t.Parallel()

	const wantText = "Replay exact: café, 東京, and 🌍"
	fixture := newFactoryFixture(t, wantText)
	live := fixture.invoke(t, "Replay this exact fixture")
	liveWork := fixture.listWork(t)
	liveEvents := fixture.recording(t)
	liveJourney := assertSuccessfulJourney(t, liveEvents, "Replay this exact fixture", wantText)
	replayed := fixture.replay(t)
	if fixture.protocol.Calls() != 1 {
		t.Fatalf("protocol calls after replay = %d, want one live call and no replay call", fixture.protocol.Calls())
	}
	liveProjection := replayEventProjection(t, liveEvents)
	replayProjection := replayEventProjection(t, replayed.events)
	if !jsonEqual(t, liveProjection, replayProjection) {
		t.Fatalf("replayed canonical events differ from live recording")
	}
	replayedJourney := assertReplayJourney(t, replayed.events, "Replay this exact fixture", wantText)
	assertPublicWorkEquivalent(t, liveWork, replayed.work, "live and replay public Work")
	if replayedJourney.artifactID != liveJourney.artifactID || requiredString(replayedJourney.outputWork.WorkId) != requiredString(liveJourney.outputWork.WorkId) {
		t.Fatalf("replayed Work/artifact identity = (%q,%q), want live (%q,%q)", requiredString(replayedJourney.outputWork.WorkId), replayedJourney.artifactID, requiredString(liveJourney.outputWork.WorkId), liveJourney.artifactID)
	}
	if live.Status != factoryapi.InvocationTerminalStatusCompleted || live.RequestId != requiredString(liveJourney.workRequest.Context.RequestId) || live.TraceId != firstTraceID(t, liveJourney.workRequest) {
		t.Fatalf("live response = %#v, want completed response with canonical request/trace identity", live)
	}
	assertEventLineage(t, liveEvents, "live and replay source")
}

func TestFactorySessionOmniArtifactMetadataIsSemantic(t *testing.T) {
	t.Parallel()

	const wantText = "Semantic Unicode: café, 東京, and 🌍"
	fixture := newFactoryFixture(t, wantText)
	response := fixture.invoke(t, "Return semantic metadata")
	publicWork := fixture.listWork(t)
	journey := assertSuccessfulJourney(t, fixture.recording(t), "Return semantic metadata", wantText)
	assertPublicWorkProjection(t, publicWork, journey, wantText)
	resultPart := workTextPart(t, response.PrimaryResult, "primary result")
	if resultPart.Type != factoryapi.WorkContentPartTypeText || resultPart.Text != wantText {
		t.Fatalf("primary result part = %#v, want exact semantic text", resultPart)
	}
	if resultPart.ContentType == nil || *resultPart.ContentType != "text/plain" {
		t.Fatalf("primary result content type = %#v, want text/plain", resultPart.ContentType)
	}
	if resultPart.ArtifactId == nil || *resultPart.ArtifactId != journey.artifactID {
		t.Fatalf("primary result artifact id = %#v, want %q", resultPart.ArtifactId, journey.artifactID)
	}
	if got, want := len([]byte(resultPart.Text)), len([]byte(wantText)); got != want {
		t.Fatalf("UTF-8 result size = %d, want %d bytes", got, want)
	}
	if journey.outputWork.State == nil || journey.outputWork.State.Type != factoryapi.WorkStateTypeTERMINAL {
		t.Fatalf("output Work state = %#v, want terminal", journey.outputWork.State)
	}
	if journey.outputWork.Content == nil || len(*journey.outputWork.Content) != 1 {
		t.Fatalf("output Work content = %#v, want one materialized part", journey.outputWork.Content)
	}
	outputPart := workTextPart(t, journey.outputWork.Content, "output Work")
	if outputPart.Text != wantText || outputPart.ArtifactId == nil || *outputPart.ArtifactId != journey.artifactID || outputPart.ContentType == nil || *outputPart.ContentType != "text/plain" {
		t.Fatalf("output Work part = %#v, want one text/plain artifact-bearing part", outputPart)
	}
}

func TestFactorySessionOmniArtifactLineageIsPreserved(t *testing.T) {
	t.Parallel()

	const wantText = "Lineage exact: café, 東京, and 🌍"
	fixture := newFactoryFixture(t, wantText)
	response := fixture.invoke(t, "Preserve this lineage")
	publicWork := fixture.listWork(t)
	events := fixture.recording(t)
	journey := assertSuccessfulJourney(t, events, "Preserve this lineage", wantText)
	assertPublicWorkProjection(t, publicWork, journey, wantText)
	if response.RequestId != requiredString(journey.workRequest.Context.RequestId) || response.TraceId != firstTraceID(t, journey.workRequest) {
		t.Fatalf("response identity request=%q trace=%q, want event request=%q trace=%q", response.RequestId, response.TraceId, requiredString(journey.workRequest.Context.RequestId), firstTraceID(t, journey.workRequest))
	}
	assertEventLineage(t, events, "lineage")
	if journey.outputWork.WorkId == nil || journey.outputWork.CurrentChainingTraceId == nil || journey.outputWork.ChainingTraceDepth == nil {
		t.Fatalf("output Work lineage = %#v, want work/current-trace/depth identities", journey.outputWork)
	}
}

func TestFactorySessionOmniArtifactBackendFailureIsAtomic(t *testing.T) {
	t.Parallel()

	fixture := newFactoryFixture(t, "unused after rejection")
	fixture.protocol.SetError(errors.New("fixture backend rejection"))
	err, stderr := fixture.invokeExpectFailure(t, "Reject this backend attempt")
	if err == nil {
		t.Fatal("backend rejection error = nil, want failed Factory attempt")
	}
	events := fixture.recording(t)
	if fixture.protocol.Calls() != 1 {
		t.Fatalf("protocol calls = %d, want one rejected attempt", fixture.protocol.Calls())
	}
	for _, event := range events {
		if event.Type == factoryapi.FactoryEventTypeArtifactCreated {
			t.Fatalf("atomic materialization failure emitted artifact event: %#v", event)
		}
	}
	if event := optionalFactoryEvent(events, factoryapi.FactoryEventTypeModelResponse); event != nil {
		payload, decodeErr := event.Payload.AsModelResponseEventPayload()
		if decodeErr != nil {
			t.Fatalf("decode failed MODEL_RESPONSE: %v", decodeErr)
		}
		if payload.OutputContent != nil {
			t.Fatalf("failed MODEL_RESPONSE output = %#v, want no successful output", payload.OutputContent)
		}
	}
	dispatch := requiredFactoryEvent(t, events, factoryapi.FactoryEventTypeDispatchResponse)
	dispatchPayload, decodeErr := dispatch.Payload.AsDispatchResponseEventPayload()
	if decodeErr != nil {
		t.Fatalf("decode failed DISPATCH_RESPONSE: %v", decodeErr)
	}
	if dispatchPayload.Outcome == factoryapi.WorkOutcomeAccepted {
		t.Fatalf("failed DISPATCH_RESPONSE = %#v, want failed route", dispatchPayload)
	}
	if dispatchPayload.OutputWork != nil {
		for _, outputWork := range *dispatchPayload.OutputWork {
			if outputWork.State != nil && outputWork.State.Type == factoryapi.WorkStateTypeTERMINAL {
				t.Fatalf("failed DISPATCH_RESPONSE recorded terminal output Work = %#v", outputWork)
			}
			if outputWork.State != nil && outputWork.State.Type != factoryapi.WorkStateTypeFAILED {
				t.Fatalf("failed DISPATCH_RESPONSE Work state = %#v, want FAILED", outputWork.State)
			}
			if outputWork.Content != nil {
				for _, part := range *outputWork.Content {
					textPart, partErr := part.AsWorkTextContentPart()
					if partErr == nil && textPart.ArtifactId != nil && *textPart.ArtifactId != "" {
						t.Fatalf("failed DISPATCH_RESPONSE recorded artifact-bearing content = %#v", textPart)
					}
				}
			}
		}
	}
	if strings.Contains(stderr, "fixture backend rejection") {
		t.Fatalf("raw fixture failure leaked through CLI diagnostics: %q", stderr)
	}
}

func TestFactorySessionOmniArtifactReleaseIsExactlyOnce(t *testing.T) {
	t.Parallel()
	t.Run("success", func(t *testing.T) {
		t.Parallel()
		fixture := newFactoryFixture(t, "release success")
		fixture.invoke(t, "Release success")
		fixture.assertRelease(t)
	})
	t.Run("backend error", func(t *testing.T) {
		t.Parallel()
		fixture := newFactoryFixture(t, "unused backend error")
		fixture.protocol.SetError(errors.New("fixture backend error"))
		_, _ = fixture.invokeExpectFailure(t, "Release backend error")
		fixture.assertRelease(t)
	})
	t.Run("timeout", func(t *testing.T) {
		t.Parallel()
		fixture := newFactoryFixtureWithConfig(t, "unused timeout", "50ms")
		fixture.protocol.BlockUntil(make(chan struct{}))
		_, _ = fixture.invokeExpectFailure(t, "Release timeout")
		fixture.assertRelease(t)
		fixture.assertNoSuccessfulOutput(t, fixture.recording(t))
	})
	t.Run("cancellation has no late completion", func(t *testing.T) {
		t.Parallel()
		fixture := newFactoryFixture(t, "unused cancellation")
		fixture.protocol.BlockUntil(make(chan struct{}))
		fixture.startLive(t, "Release cancellation")
		fixture.submit(t, "Release cancellation")
		fixture.protocol.WaitForCall(t)
		fixture.cancel(t)
		fixture.protocol.WaitForCancellation(t)
		fixture.stopLive(t)
		fixture.assertRelease(t)
		fixture.assertNoSuccessfulOutput(t, fixture.recording(t))
	})
}

var (
	sharedOmniFactoryProcessOnce sync.Once
	sharedOmniFactoryProcessErr  error
)

type omniFactoryProcessGroup struct {
	rootDir     string
	home        string
	process     support.ApplicationProcess
	modelServer *httptest.Server
	protocol    *omniProtocolRouter
	launcher    *modelHostLauncher

	serversMu sync.Mutex
	servers   map[int]*support.ProcessAPIServer
	nextPort  atomic.Int64
}

func sharedOmniFactory(t *testing.T) *omniFactoryProcessGroup {
	t.Helper()
	sharedOmniFactoryProcessOnce.Do(func() {
		sharedOmniFactoryProcess, sharedOmniFactoryProcessErr = newOmniFactoryProcessGroup()
	})
	if sharedOmniFactoryProcessErr != nil {
		t.Fatalf("build shared OMNI Factory process: %v", sharedOmniFactoryProcessErr)
	}
	if sharedOmniFactoryProcess == nil {
		t.Fatal("shared OMNI Factory process is nil")
	}
	return sharedOmniFactoryProcess
}

func newOmniFactoryProcessGroup() (*omniFactoryProcessGroup, error) {
	rootDir, err := os.MkdirTemp("", "you-functional-omni-artifact-")
	if err != nil {
		return nil, fmt.Errorf("create shared fixture root: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(rootDir) }
	home := filepath.Join(rootDir, "home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		cleanup()
		return nil, fmt.Errorf("create shared fixture home: %w", err)
	}
	if err := writeBuiltinModelCacheAt(home); err != nil {
		cleanup()
		return nil, fmt.Errorf("write shared model cache: %w", err)
	}
	selection := llamaBackendSelection()
	if err := writeBackendCacheAt(home, selection); err != nil {
		cleanup()
		return nil, fmt.Errorf("write shared backend cache: %w", err)
	}
	modelServer := newOmniModelServer()
	group := &omniFactoryProcessGroup{
		rootDir: rootDir, home: home, modelServer: modelServer,
		protocol: &omniProtocolRouter{routes: make(map[string]*omniProtocolFixture)},
		launcher: &modelHostLauncher{endpoint: modelServer.URL},
		servers:  make(map[int]*support.ProcessAPIServer),
	}
	assetFiles := modelAssetFileSystem{home: home}
	edges := serviceedges.Edges{
		APIServerStarter:               group.startAPIServer,
		ModelAssetHTTPClient:           rejectingAssetHTTP{},
		ModelAssetHostPlatform:         models.AssetHostPlatform{OperatingSystem: "linux", Architecture: "amd64"},
		ModelAssetMakeDirectories:      assetFiles.MkdirAll,
		ModelAssetInspectPath:          assetFiles.Stat,
		ModelAssetResolveHomeDirectory: assetFiles.UserHomeDir,
		ModelAssetResolveEnvironment:   func(string) string { return "" },
		ModelAssetWriteFile:            assetFiles.WriteFile,
		ModelAssetRenamePath:           assetFiles.Rename,
		ModelAssetRemovePath:           assetFiles.Remove,
		ModelAssetReadFile:             assetFiles.ReadFile,
		ModelAssetReadDirectory:        assetFiles.ReadDir,
		ModelAssetCreateFile:           assetFiles.Create,
		ModelAssetOpenFile:             assetFiles.Open,
		ModelResolveBackendArtifact: func(context.Context, serviceedges.ModelBackendArtifactSelectionRequest) (serviceedges.ModelBackendArtifactSelection, error) {
			return selection, nil
		},
		ModelHostProcessLauncher:      group.launcher,
		ModelHostProtocolNegotiator:   modelHostProtocolNegotiator{},
		ModelHostCompatibilityChecker: modelHostCompatibilityChecker{},
		ModelHostHTTPClient:           modelServer.Client(),
		ModelRuntimeHTTPClient:        modelServer.Client(),
		ModelInvocationProtocolClient: group.protocol,
	}
	group.process, err = support.BuildProcessWithContext(context.Background(), edges)
	if err != nil {
		modelServer.Close()
		cleanup()
		return nil, fmt.Errorf("build root process: %w", err)
	}
	return group, nil
}

func newOmniModelServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusOK)
	}))
}

func (group *omniFactoryProcessGroup) registerAPIServer() (*support.ProcessAPIServer, int) {
	port := 25000 + int(group.nextPort.Add(1))
	server := support.NewProcessAPIServer()
	group.serversMu.Lock()
	group.servers[port] = server
	group.serversMu.Unlock()
	return server, port
}

func (group *omniFactoryProcessGroup) unregisterAPIServer(port int) {
	group.serversMu.Lock()
	delete(group.servers, port)
	group.serversMu.Unlock()
}

func (group *omniFactoryProcessGroup) startAPIServer(ctx context.Context, request platformhttpserver.StartRequest) error {
	group.serversMu.Lock()
	server := group.servers[request.Port]
	group.serversMu.Unlock()
	if server == nil {
		return fmt.Errorf("OMNI Factory API server route not found for port %d", request.Port)
	}
	return server.Start(ctx, request)
}

func (group *omniFactoryProcessGroup) close() error {
	if group == nil {
		return nil
	}
	var closeErr error
	if group.process != nil {
		ctx, cancel := context.WithTimeout(context.Background(), omniFactoryFunctionalTimeout)
		closeErr = group.process.Close(ctx)
		cancel()
	}
	starts, stops := group.launcher.Counts()
	if starts != stops {
		closeErr = errors.Join(closeErr, fmt.Errorf("managed model host starts=%d stops=%d after shared process close", starts, stops))
	}
	if group.modelServer != nil {
		group.modelServer.Close()
	}
	if err := os.RemoveAll(group.rootDir); err != nil {
		closeErr = errors.Join(closeErr, err)
	}
	return closeErr
}

type factoryFixture struct {
	home          string
	runtimeLogDir string
	sessionID     string
	dir           string
	recordingPath string
	protocol      *omniProtocolFixture
	process       support.Process
	launcher      *modelHostLauncher
	modelEndpoint string
	group         *omniFactoryProcessGroup
	command       *support.ProcessCommand
	inputs        *support.CapturedInputs
	baseURL       string
	api           *support.ProcessAPIServer
	listenPort    int
}

func newFactoryFixture(t *testing.T, response string) *factoryFixture {
	t.Helper()
	return newFactoryFixtureWithConfig(t, response, "")
}

func newFactoryFixtureWithConfig(t *testing.T, response, executionLimit string) *factoryFixture {
	t.Helper()
	group := sharedOmniFactory(t)
	home := t.TempDir()
	if err := writeBuiltinModelCacheAt(home); err != nil {
		t.Fatalf("seed builtin model cache: %v", err)
	}
	if err := writeBackendCacheAt(home, llamaBackendSelection()); err != nil {
		t.Fatalf("seed backend model cache: %v", err)
	}
	protocol := &omniProtocolFixture{
		response: response, called: make(chan struct{}), canceled: make(chan struct{}),
	}
	sessionID := uuid.NewString()
	modelEndpoint := group.modelServer.URL + "/scenario-" + uuid.NewString()
	dir := support.ScaffoldFactory(t, omniFactoryConfig(modelEndpoint))
	workstation := "---\ntype: INFERENCE_RUN\n"
	if executionLimit != "" {
		workstation += "limits:\n  maxRetries: 1\n  maxExecutionTime: " + executionLimit + "\n"
	}
	workstation += "---\nReturn the model result.\n"
	support.WriteWorkstationConfig(t, dir, "execute-llm", workstation)
	recordingPath := filepath.Join(t.TempDir(), "omni-artifact-recording.json")
	return &factoryFixture{
		home: home, runtimeLogDir: filepath.Join(home, "runtime-logs"), sessionID: sessionID, dir: dir,
		recordingPath: recordingPath, protocol: protocol, process: group.process,
		launcher: group.launcher, modelEndpoint: modelEndpoint, group: group,
	}
}

func (fixture *factoryFixture) invoke(t *testing.T, prompt string) factoryapi.InvocationResponse {
	t.Helper()
	fixture.startLive(t, prompt)
	submitted := fixture.submit(t, prompt)
	support.WaitForSessionTerminalStatus(t, fixture.baseURL, fixture.sessionID, omniFactoryFunctionalTimeout)
	listed := fixture.listWork(t)
	if len(listed.Results) != 1 {
		t.Fatalf("public Work list = %#v, want one completed Work", listed)
	}
	return factoryapi.InvocationResponse{
		RequestId: submitted.RequestId, TraceId: submitted.TraceId,
		SessionId: submitted.SessionId, WorkId: submitted.WorkId,
		Status:        factoryapi.InvocationTerminalStatusCompleted,
		PrimaryResult: listed.Results[0].Content,
	}
}

func (fixture *factoryFixture) submit(t *testing.T, prompt string) factoryapi.SubmitWorkResponse {
	t.Helper()
	part := factoryapi.WorkContentPart{}
	if err := part.FromWorkTextContentPart(factoryapi.WorkTextContentPart{
		Type:        factoryapi.WorkContentPartTypeText,
		Text:        prompt,
		ContentType: stringPointer("text/plain"),
	}); err != nil {
		t.Fatalf("build public prompt Work content: %v", err)
	}
	content := factoryapi.WorkContent{part}
	return support.SubmitSessionWorkAt(t, fixture.baseURL, fixture.sessionID, factoryapi.SubmitWorkRequest{
		Content:      &content,
		WorkTypeName: "task",
	})
}

func (fixture *factoryFixture) cancel(t *testing.T) {
	t.Helper()
	endpoint := strings.TrimSuffix(fixture.baseURL, "/") + "/factory-sessions/" + url.PathEscape(fixture.sessionID) + "/cancel"
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, endpoint, strings.NewReader(`{}`))
	if err != nil {
		t.Fatalf("build Factory Session cancel request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("POST %s: %v", endpoint, err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("POST %s status = %d: %s", endpoint, response.StatusCode, strings.TrimSpace(string(body)))
	}
	var result factoryapi.FactorySessionLifecycleControlResponse
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatalf("decode POST %s: %v", endpoint, err)
	}
	if result.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted &&
		result.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeNoOp {
		t.Fatalf("Factory Session cancel response = %#v, want ACCEPTED or NO_OP", result)
	}
}

func (fixture *factoryFixture) invokeExpectFailure(t *testing.T, prompt string) (error, string) {
	t.Helper()
	fixture.startLive(t, prompt)
	fixture.submit(t, prompt)
	status := support.WaitForSessionTerminalStatus(t, fixture.baseURL, fixture.sessionID, omniFactoryFunctionalTimeout)
	command := fixture.command
	command.AcceptError()
	fixture.stopLive(t)
	stderr := fixture.inputs.Stderr()
	if status.Categories.Failed == 0 {
		t.Fatalf("Factory Session status = %#v, want FAILED", status.Categories)
	}
	if err := command.Err(); err != nil {
		return err, stderr
	}
	return fmt.Errorf("Factory Session reported FAILED status without a Process.Execute error: %#v", status.Categories), stderr
}

func (fixture *factoryFixture) recording(t *testing.T) []factoryapi.FactoryEvent {
	t.Helper()
	fixture.stopLive(t)
	data, err := os.ReadFile(fixture.recordingPath)
	if err != nil {
		t.Fatalf("read Factory recording: %v", err)
	}
	var artifact struct {
		Events        []factoryapi.FactoryEvent `json:"events"`
		SchemaVersion string                    `json:"schemaVersion"`
	}
	if err := json.Unmarshal(data, &artifact); err != nil {
		t.Fatalf("decode Factory recording: %v", err)
	}
	if artifact.SchemaVersion == "" {
		t.Fatal("Factory recording schema version is empty")
	}
	if len(artifact.Events) == 0 {
		t.Fatal("Factory recording contains no canonical events")
	}
	return artifact.Events
}

func (fixture *factoryFixture) listWork(t *testing.T) factoryapi.ListWorkResponse {
	t.Helper()
	if fixture.baseURL == "" {
		t.Fatal("Factory Session API URL is empty")
	}
	return support.GetJSON[factoryapi.ListWorkResponse](
		t,
		support.SessionWorkURL(fixture.baseURL, fixture.sessionID, "/work"),
	)
}

func (fixture *factoryFixture) startLive(t *testing.T, prompt string) {
	t.Helper()
	if fixture.command != nil {
		t.Fatal("Factory Session invocation already started")
	}
	fixture.group.protocol.Register(prompt, fixture.protocol)
	fixture.api, fixture.listenPort = fixture.group.registerAPIServer()
	fixture.inputs = support.FakeInputs(t.Context(), []string{
		"you", "--json", "run", "--factory", filepath.Join(fixture.dir, "factory.json"),
		"--session", fixture.sessionID, "--record", fixture.recordingPath, "--output", "primary",
		"--runtime-log-dir", fixture.runtimeLogDir,
		"--continuously", "--with-server", "--listen", fmt.Sprintf("127.0.0.1:%d", fixture.listenPort),
	})
	fixture.inputs.Input.Env = functionalHomeEnvironment(fixture.home)
	fixture.inputs.Input.WorkingDirectory = fixture.dir
	fixture.command = support.StartProcessCommand(t, fixture.process, fixture.inputs.Input)
	baseURL, err := fixture.api.WaitForBaseURL(omniFactoryFunctionalTimeout)
	if err != nil {
		t.Fatalf("wait for Factory API: %v stdout=%q stderr=%q", err, fixture.inputs.Stdout(), fixture.inputs.Stderr())
	}
	fixture.baseURL = baseURL
}

func (fixture *factoryFixture) stopLive(t *testing.T) {
	t.Helper()
	if fixture.command == nil {
		return
	}
	fixture.command.Stop(t)
	fixture.command = nil
	fixture.group.unregisterAPIServer(fixture.listenPort)
}

type replayProjection struct {
	events    []factoryapi.FactoryEvent
	work      factoryapi.ListWorkResponse
	rawEvents string
	rawWork   string
}

func (fixture *factoryFixture) replay(t *testing.T) replayProjection {
	t.Helper()
	replaySessionID := uuid.NewString()
	api, port := fixture.group.registerAPIServer()
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "--json", "run", "--dir", fixture.dir, "--session", replaySessionID,
		"--replay", fixture.recordingPath, "--no-record", "--output", "primary",
		"--runtime-log-dir", fixture.runtimeLogDir,
		"--continuously", "--with-server", "--listen", fmt.Sprintf("127.0.0.1:%d", port),
	})
	inputs.Input.Env = functionalHomeEnvironment(fixture.home)
	inputs.Input.WorkingDirectory = fixture.dir
	command := support.StartProcessCommand(t, fixture.process, inputs.Input)
	baseURL := api.WaitForURL(t)
	support.WaitForSessionTerminalStatus(t, baseURL, replaySessionID, omniFactoryFunctionalTimeout)
	projection := replayProjection{
		events: support.GetFactoryEventsForSessionAt(t, baseURL, replaySessionID),
		work: support.GetJSON[factoryapi.ListWorkResponse](
			t, support.SessionWorkURL(baseURL, replaySessionID, "/work"),
		),
		rawEvents: readPublicEventsHTTPBody(t, support.SessionEventsURL(baseURL, replaySessionID)),
		rawWork:   readPublicHTTPBody(t, support.SessionWorkURL(baseURL, replaySessionID, "/work")),
	}
	command.Stop(t)
	fixture.group.unregisterAPIServer(port)
	return projection
}

type successfulJourney struct {
	workRequest      factoryapi.FactoryEvent
	dispatchRequest  factoryapi.FactoryEvent
	modelRequest     factoryapi.FactoryEvent
	modelResponse    factoryapi.FactoryEvent
	dispatchResponse factoryapi.FactoryEvent
	sessionCompleted factoryapi.FactoryEvent
	inputWork        factoryapi.Work
	outputWork       factoryapi.Work
	outputPart       factoryapi.WorkTextContentPart
	artifactID       string
}

func assertSuccessfulJourney(t *testing.T, events []factoryapi.FactoryEvent, prompt, wantText string) successfulJourney {
	t.Helper()
	journey := assertJourney(t, events, prompt, wantText, true)
	return journey
}

func assertReplayJourney(t *testing.T, events []factoryapi.FactoryEvent, prompt, wantText string) successfulJourney {
	t.Helper()
	return assertJourney(t, events, prompt, wantText, false)
}

func assertJourney(t *testing.T, events []factoryapi.FactoryEvent, prompt, wantText string, requireSessionCompletion bool) successfulJourney {
	t.Helper()
	journey := inspectJourney(t, events)
	if requireSessionCompletion && journey.sessionCompleted.Type != factoryapi.FactoryEventTypeSessionCompleted {
		t.Fatalf("canonical events have no SESSION_COMPLETED event")
	}
	if journey.inputWork.Content == nil || len(*journey.inputWork.Content) != 1 {
		t.Fatalf("input Work content = %#v, want one text part", journey.inputWork.Content)
	}
	inputPart := workTextPart(t, journey.inputWork.Content, "input Work")
	if inputPart.Text != prompt {
		t.Fatalf("input Work text = %q, want %q", inputPart.Text, prompt)
	}
	modelResponse, err := journey.modelResponse.Payload.AsModelResponseEventPayload()
	if err != nil {
		t.Fatalf("decode MODEL_RESPONSE: %v", err)
	}
	if modelResponse.Outcome != factoryapi.InferenceOutcomeSucceeded || modelResponse.OutputContent == nil || len(*modelResponse.OutputContent) != 1 {
		t.Fatalf("MODEL_RESPONSE = %#v, want successful one-part output", modelResponse)
	}
	modelPart := workTextPart(t, modelResponse.OutputContent, "MODEL_RESPONSE")
	if modelPart.Text != wantText {
		t.Fatalf("MODEL_RESPONSE text = %q, want %q", modelPart.Text, wantText)
	}
	if modelPart.ArtifactId == nil || *modelPart.ArtifactId == "" {
		t.Fatal("MODEL_RESPONSE output has no opaque artifact id")
	}
	if modelPart.ContentType == nil || *modelPart.ContentType != "text/plain" {
		t.Fatalf("MODEL_RESPONSE content type = %#v, want text/plain", modelPart.ContentType)
	}
	if modelPart.Type != factoryapi.WorkContentPartTypeText {
		t.Fatalf("MODEL_RESPONSE content discriminator = %q, want text", modelPart.Type)
	}
	if journey.outputPart.Text != wantText || journey.outputPart.ArtifactId == nil || *journey.outputPart.ArtifactId != *modelPart.ArtifactId {
		t.Fatalf("output Work part = %#v, want exact model artifact-bearing result %#v", journey.outputPart, modelPart)
	}
	journey.artifactID = *modelPart.ArtifactId
	return journey
}

func assertEventLineage(t *testing.T, events []factoryapi.FactoryEvent, label string) {
	t.Helper()
	workRequest := requiredFactoryEvent(t, events, factoryapi.FactoryEventTypeWorkRequest)
	dispatchRequest := requiredFactoryEvent(t, events, factoryapi.FactoryEventTypeDispatchRequest)
	modelRequest := requiredFactoryEvent(t, events, factoryapi.FactoryEventTypeModelRequest)
	modelResponse := requiredFactoryEvent(t, events, factoryapi.FactoryEventTypeModelResponse)
	dispatchResponse := requiredFactoryEvent(t, events, factoryapi.FactoryEventTypeDispatchResponse)
	requestID := requiredString(workRequest.Context.RequestId)
	traceID := firstTraceID(t, workRequest)
	workID := firstWorkID(t, workRequest)
	dispatchID := requiredString(dispatchRequest.Context.DispatchId)
	if requestID == "" || dispatchID == "" || workID == "" {
		t.Fatalf("%s identity request=%q dispatch=%q work=%q, want all identities", label, requestID, dispatchID, workID)
	}
	for _, event := range []factoryapi.FactoryEvent{dispatchRequest, modelRequest, modelResponse, dispatchResponse} {
		if event.Context.RequestId != nil && *event.Context.RequestId != requestID {
			t.Fatalf("%s event %s request=%q, want %q", label, event.Type, *event.Context.RequestId, requestID)
		}
		if event.Context.DispatchId != nil && *event.Context.DispatchId != dispatchID {
			t.Fatalf("%s event %s dispatch=%q, want %q", label, event.Type, *event.Context.DispatchId, dispatchID)
		}
		if event.Context.WorkIds != nil && firstWorkID(t, event) != workID {
			t.Fatalf("%s event %s Work IDs=%#v, want %q", label, event.Type, event.Context.WorkIds, workID)
		}
		if event.Context.TraceIds != nil && firstTraceID(t, event) != traceID {
			t.Fatalf("%s event %s trace IDs=%#v, want %q", label, event.Type, event.Context.TraceIds, traceID)
		}
	}
	if source := requiredString(workRequest.Context.Source); source == "" {
		t.Fatalf("%s WORK_REQUEST source is empty", label)
	}
	if dispatchResponse.Context.PreviousChainingTraceIds == nil {
		t.Fatalf("%s completion context has no prior chaining trace IDs", label)
	}
}

func (fixture *factoryFixture) assertRelease(t *testing.T) {
	t.Helper()
	fixture.stopLive(t)
	if got := fixture.protocol.Calls(); got != 1 {
		t.Fatalf("private OMNI protocol calls = %d, want exactly one attempt", got)
	}
	startsForTarget, stopsForTarget := fixture.launcher.CountsFor(fixture.modelEndpoint)
	if startsForTarget != 1 || stopsForTarget != 1 {
		t.Fatalf("managed model host lifecycle for %q = starts %d, stops %d; want exactly one start and one stop", fixture.modelEndpoint, startsForTarget, stopsForTarget)
	}
	starts, stops := fixture.launcher.Counts()
	if stops > starts {
		t.Fatalf("managed model host releases = %d, exceed starts = %d", stops, starts)
	}
}

func (fixture *factoryFixture) assertNoSuccessfulOutput(t *testing.T, events []factoryapi.FactoryEvent) {
	t.Helper()
	if response := optionalFactoryEvent(events, factoryapi.FactoryEventTypeModelResponse); response != nil {
		payload, err := response.Payload.AsModelResponseEventPayload()
		if err != nil {
			t.Fatalf("decode failure MODEL_RESPONSE: %v", err)
		}
		if payload.Outcome == factoryapi.InferenceOutcomeSucceeded || payload.OutputContent != nil {
			t.Fatalf("failed invocation published model output: %#v", payload)
		}
	}
	if response := optionalFactoryEvent(events, factoryapi.FactoryEventTypeDispatchResponse); response != nil {
		payload, err := response.Payload.AsDispatchResponseEventPayload()
		if err != nil {
			t.Fatalf("decode failure DISPATCH_RESPONSE: %v", err)
		}
		if payload.Outcome == factoryapi.WorkOutcomeAccepted {
			t.Fatalf("failed invocation published accepted Work output: %#v", payload)
		}
		if payload.OutputWork != nil {
			for _, outputWork := range *payload.OutputWork {
				if outputWork.State != nil && outputWork.State.Type == factoryapi.WorkStateTypeTERMINAL {
					t.Fatalf("failed invocation published terminal Work output: %#v", outputWork)
				}
			}
		}
	}
}

func assertPublicWorkProjection(t *testing.T, listed factoryapi.ListWorkResponse, journey successfulJourney, wantText string) {
	t.Helper()
	if len(listed.Results) != 1 {
		t.Fatalf("public Work list results = %d, want exactly one canonical Work: %#v", len(listed.Results), listed)
	}
	work := listed.Results[0]
	if requiredString(work.WorkId) != requiredString(journey.outputWork.WorkId) {
		t.Fatalf("public Work ID = %q, want canonical output Work %q", requiredString(work.WorkId), requiredString(journey.outputWork.WorkId))
	}
	if work.State == nil || work.State.Type != factoryapi.WorkStateTypeTERMINAL {
		t.Fatalf("public Work state = %#v, want TERMINAL", work.State)
	}
	part := workTextPart(t, work.Content, "public Work")
	if part.Text != wantText {
		t.Fatalf("public Work text = %q, want %q", part.Text, wantText)
	}
	if part.ContentType == nil || *part.ContentType != "text/plain" {
		t.Fatalf("public Work content type = %#v, want text/plain", part.ContentType)
	}
	if part.ArtifactId == nil || *part.ArtifactId != journey.artifactID {
		t.Fatalf("public Work artifact ID = %#v, want %q", part.ArtifactId, journey.artifactID)
	}
	if got, want := len([]byte(part.Text)), len([]byte(wantText)); got != want {
		t.Fatalf("public Work UTF-8 size = %d, want %d bytes", got, want)
	}
}

func assertPublicWorkEquivalent(t *testing.T, live, replay factoryapi.ListWorkResponse, label string) {
	t.Helper()
	if len(live.Results) != 1 || len(replay.Results) != 1 {
		t.Fatalf("%s list sizes = live:%d replay:%d, want exactly one each", label, len(live.Results), len(replay.Results))
	}
	if !jsonEqual(t, live.Results[0], replay.Results[0]) {
		t.Fatalf("%s differs: live=%#v replay=%#v", label, live.Results[0], replay.Results[0])
	}
}

func requiredFactoryEvent(t *testing.T, events []factoryapi.FactoryEvent, eventType factoryapi.FactoryEventType) factoryapi.FactoryEvent {
	t.Helper()
	var matches []factoryapi.FactoryEvent
	for _, event := range events {
		if event.Type == eventType {
			matches = append(matches, event)
		}
	}
	if len(matches) != 1 {
		t.Fatalf("event type %s count = %d, want one", eventType, len(matches))
	}
	return matches[0]
}

func optionalFactoryEvent(events []factoryapi.FactoryEvent, eventType factoryapi.FactoryEventType) *factoryapi.FactoryEvent {
	for index := range events {
		if events[index].Type == eventType {
			return &events[index]
		}
	}
	return nil
}

func eventIndex(events []factoryapi.FactoryEvent, eventType factoryapi.FactoryEventType) int {
	for index, event := range events {
		if event.Type == eventType {
			return index
		}
	}
	return -1
}

func firstTraceID(t *testing.T, event factoryapi.FactoryEvent) string {
	t.Helper()
	if event.Context.TraceIds == nil || len(*event.Context.TraceIds) == 0 || (*event.Context.TraceIds)[0] == "" {
		t.Fatalf("event %s has no trace identity: %#v", event.Type, event.Context)
	}
	return (*event.Context.TraceIds)[0]
}

func firstTraceOrEmpty(event factoryapi.FactoryEvent) string {
	if event.Context.TraceIds == nil || len(*event.Context.TraceIds) == 0 {
		return ""
	}
	return (*event.Context.TraceIds)[0]
}

func firstWorkID(t *testing.T, event factoryapi.FactoryEvent) string {
	t.Helper()
	if event.Context.WorkIds == nil || len(*event.Context.WorkIds) == 0 || (*event.Context.WorkIds)[0] == "" {
		t.Fatalf("event %s has no Work identity: %#v", event.Type, event.Context)
	}
	return (*event.Context.WorkIds)[0]
}

func firstWorkOrEmpty(event factoryapi.FactoryEvent) string {
	if event.Context.WorkIds == nil || len(*event.Context.WorkIds) == 0 {
		return ""
	}
	return (*event.Context.WorkIds)[0]
}

func workTextPart(t *testing.T, content *factoryapi.WorkContent, label string) factoryapi.WorkTextContentPart {
	t.Helper()
	if content == nil || len(*content) != 1 {
		t.Fatalf("%s content = %#v, want one part", label, content)
	}
	part, err := (*content)[0].AsWorkTextContentPart()
	if err != nil {
		t.Fatalf("decode %s text part: %v", label, err)
	}
	return part
}

func jsonEqual(t *testing.T, left, right interface{}) bool {
	t.Helper()
	leftJSON, err := json.Marshal(left)
	if err != nil {
		t.Fatalf("marshal left JSON: %v", err)
	}
	rightJSON, err := json.Marshal(right)
	if err != nil {
		t.Fatalf("marshal right JSON: %v", err)
	}
	return string(leftJSON) == string(rightJSON)
}

type replayEventView struct {
	Type       factoryapi.FactoryEventType `json:"type"`
	ID         string                      `json:"id"`
	RequestID  string                      `json:"requestId,omitempty"`
	DispatchID string                      `json:"dispatchId,omitempty"`
	WorkIDs    []string                    `json:"workIds,omitempty"`
	TraceIDs   []string                    `json:"traceIds,omitempty"`
}

func replayEventProjection(t *testing.T, events []factoryapi.FactoryEvent) []replayEventView {
	t.Helper()
	views := make([]replayEventView, 0, len(events))
	for _, event := range events {
		switch event.Type {
		case factoryapi.FactoryEventTypeWorkRequest,
			factoryapi.FactoryEventTypeDispatchRequest,
			factoryapi.FactoryEventTypeDispatchWorkerSessionAssociation,
			factoryapi.FactoryEventTypeModelRequest,
			factoryapi.FactoryEventTypeModelResponse,
			factoryapi.FactoryEventTypeDispatchResponse:
		default:
			continue
		}
		view := replayEventView{
			Type: event.Type, ID: event.Id, RequestID: requiredString(event.Context.RequestId),
			DispatchID: requiredString(event.Context.DispatchId),
		}
		if event.Context.WorkIds != nil {
			view.WorkIDs = append([]string(nil), (*event.Context.WorkIds)...)
		}
		if event.Context.TraceIds != nil {
			view.TraceIDs = append([]string(nil), (*event.Context.TraceIds)...)
		}
		views = append(views, view)
	}
	return views
}
