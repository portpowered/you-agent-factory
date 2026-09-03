package inference_test

import (
	"bufio"
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
	"testing"
	"time"

	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryinterfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	sharedInferenceScenarioTimeout = 20 * time.Second
)

var sharedInferenceGroup = &inferenceProcessGroup{}

// TestMain owns the one process group used by the controlled P003 inference
// scenarios. The process is intentionally started lazily so package selectors
// that only exercise construction tests do not pay for a service-mode host.
func TestMain(m *testing.M) {
	code := m.Run()
	closeErr := sharedInferenceGroup.close()
	if closeErr != nil {
		fmt.Fprintf(os.Stderr, "close shared inference process group: %v\n", closeErr)
		if code == 0 {
			code = 1
		}
	}
	os.Exit(code)
}

type inferenceProcessGroup struct {
	once     sync.Once
	setupErr error

	process  support.ApplicationProcess
	serverMu sync.RWMutex
	server   *support.ProcessAPIServer

	rootDir          string
	hostDir          string
	homeDir          string
	baseURL          string
	daemon           *inferenceDaemon
	commands         *inferenceCommandRouter
	scripts          *inferenceCommandRouter
	override         *inferenceProviderOverride
	workerRecordings *inferenceWorkerRecordingRouter

	externals         map[string]*inferenceIntegrationRouter
	sessions          map[string]struct{}
	sessionMu         sync.Mutex
	openedSessionIDs  []string
	deletedSessionIDs []string
}

type inferenceDaemon struct {
	cancel context.CancelFunc
	done   chan error
}

func (group *inferenceProcessGroup) ensure(t *testing.T) {
	t.Helper()
	group.once.Do(group.setup)
	if group.setupErr != nil {
		t.Fatalf("set up shared inference process group: %v", group.setupErr)
	}
}

func (group *inferenceProcessGroup) setup() {
	group.rootDir, group.setupErr = os.MkdirTemp("", "you-inference-shared-")
	if group.setupErr != nil {
		return
	}
	group.hostDir = filepath.Join(group.rootDir, "host-factory")
	group.homeDir = filepath.Join(group.rootDir, "home")
	if err := writeSharedInferenceHostFactory(group.hostDir); err != nil {
		group.setupErr = err
		return
	}
	if err := os.MkdirAll(group.homeDir, 0o755); err != nil {
		group.setupErr = fmt.Errorf("create shared process home: %w", err)
		return
	}

	group.commands = &inferenceCommandRouter{routes: make(map[string]inferenceCommandRoute)}
	group.scripts = &inferenceCommandRouter{routes: make(map[string]inferenceCommandRoute)}
	group.override = newInferenceProviderOverride()
	group.workerRecordings = &inferenceWorkerRecordingRouter{
		fallback:    newWSRFT004RecordingStore(),
		bySession:   make(map[string]recordings.WorkerRecordingWriter),
		byWorker:    make(map[string]inferenceWorkerRecordingRoute),
		byRecording: make(map[string]inferenceWorkerRecordingRoute),
	}
	group.externals = make(map[string]*inferenceIntegrationRouter)
	providerDefinitions := []struct {
		id    string
		alias string
	}{
		{id: "selected.provider", alias: "selected"},
		{id: "alternate.provider", alias: "alternate"},
		{id: "global.default.provider", alias: "global-default"},
		{id: "worker.override.provider", alias: "worker-override"},
		{id: "registered.provider", alias: "registered"},
	}
	for _, provider := range providerDefinitions {
		group.externals[provider.id] = &inferenceIntegrationRouter{identity: provider.id}
	}
	group.sessions = make(map[string]struct{})
	group.server = support.NewProcessAPIServer()

	registrations := make([]sharedInferenceProviderRegistration, 0, len(group.externals))
	for _, provider := range providerDefinitions {
		integration := group.externals[provider.id]
		registrations = append(registrations, sharedInferenceProviderRegistration{
			Manifest:    sharedInferenceExternalManifest(provider.id, provider.alias),
			Integration: integration,
		})
	}
	process, err := support.BuildProcessWithContext(context.Background(), serviceedges.Edges{
		APIServerStarter:      group.startAPIServer,
		ProviderCommandRunner: group.commands,
		ScriptCommandRunner:   group.scripts,
		ProviderOverride:      group.override,
		WorkerRecordingWriter: group.workerRecordings,
		ProviderRegistrations: registrations,
	})
	if err != nil {
		group.setupErr = err
		return
	}
	group.process = process
	group.setupErr = group.startDaemon()
}

func (group *inferenceProcessGroup) startDaemon() error {
	server := group.currentServer()
	if server == nil {
		return errors.New("shared inference API server is not configured")
	}
	ctx, cancel := context.WithCancel(context.Background())
	inputs := support.FakeInputs(ctx, []string{
		"you", "run",
		"--dir", group.hostDir,
		"--continuously",
		"--with-server",
		"--server", "http://127.0.0.1:1",
		"--quiet",
	})
	inputs.Input.Env = sharedInferenceProcessEnvironment(group.homeDir)
	inputs.Input.WorkingDirectory = group.hostDir
	daemon := &inferenceDaemon{cancel: cancel, done: make(chan error, 1)}
	group.daemon = daemon
	go func() {
		daemon.done <- group.process.Execute(inputs.Input)
	}()

	baseURL, err := server.WaitForBaseURL(15 * time.Second)
	if err != nil {
		cancel()
		select {
		case <-daemon.done:
		case <-time.After(10 * time.Second):
		}
		group.daemon = nil
		return err
	}
	select {
	case err := <-daemon.done:
		group.daemon = nil
		if err == nil {
			return errors.New("shared inference daemon exited before readiness")
		} else {
			return fmt.Errorf("shared inference daemon exited before readiness: %w", err)
		}
	default:
	}
	group.baseURL = baseURL
	return nil
}

func (group *inferenceProcessGroup) close() error {
	if group.process == nil {
		return nil
	}
	if err := group.stopDaemon(); err != nil {
		return err
	}
	closeCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err := group.process.Close(closeCtx)
	if removeErr := os.RemoveAll(group.rootDir); err == nil {
		err = removeErr
	}
	return err
}

func (group *inferenceProcessGroup) stopDaemon() error {
	daemon := group.daemon
	if daemon == nil {
		return nil
	}
	daemon.cancel()
	select {
	case err := <-daemon.done:
		group.daemon = nil
		if err != nil && !errors.Is(err, context.Canceled) {
			return fmt.Errorf("stop shared inference daemon: %w", err)
		}
		return nil
	case <-time.After(10 * time.Second):
		return errors.New("timed out stopping shared inference daemon")
	}
}

func (group *inferenceProcessGroup) currentServer() *support.ProcessAPIServer {
	group.serverMu.RLock()
	defer group.serverMu.RUnlock()
	return group.server
}

func (group *inferenceProcessGroup) setServer(server *support.ProcessAPIServer) {
	group.serverMu.Lock()
	group.server = server
	group.serverMu.Unlock()
}

func (group *inferenceProcessGroup) startAPIServer(
	ctx context.Context,
	request platformhttpserver.StartRequest,
) error {
	server := group.currentServer()
	if server == nil {
		return errors.New("shared inference API server is not configured")
	}
	return server.Start(ctx, request)
}

type sharedInferenceScenario struct {
	commandRunner         platformprocess.CommandRunner
	scriptRunner          platformprocess.CommandRunner
	providerOverride      providers.Service
	providerRegistrations []sharedInferenceProviderRegistration
	workerRecordingWriter recordings.WorkerRecordingWriter
	submittedWork         *factoryapi.SubmitWorkRequest
	submittedWorks        []factoryapi.SubmitWorkRequest
	env                   []string
	scenarioName          string
	captureResponse       bool
	captureWorkerEvents   bool
}

func runSharedInferenceFactoryToCompletion(
	t *testing.T,
	dir string,
	scenario sharedInferenceScenario,
	timeout time.Duration,
) (factoryapi.FactorySession, factoryapi.ListWorkResponse, []factoryapi.FactoryEvent) {
	t.Helper()
	result := runSharedInferenceFactory(t, dir, scenario, timeout)
	return result.session, result.work, result.events
}

func runSharedInferenceFactoryWithStreams(
	t *testing.T,
	dir string,
	scenario sharedInferenceScenario,
	timeout time.Duration,
) (factoryapi.FactorySession, factoryapi.ListWorkResponse, []factoryapi.FactoryEvent, []factoryapi.FactoryResponseEvent, []factoryapi.WorkerSessionEvent) {
	t.Helper()
	scenario.captureResponse = true
	scenario.captureWorkerEvents = true
	result := runSharedInferenceFactory(t, dir, scenario, timeout)
	return result.session, result.work, result.events, result.responseEvents, result.workerEvents
}

type sharedInferenceFactoryResult struct {
	session        factoryapi.FactorySession
	work           factoryapi.ListWorkResponse
	events         []factoryapi.FactoryEvent
	responseEvents []factoryapi.FactoryResponseEvent
	workerEvents   []factoryapi.WorkerSessionEvent
}

func runSharedInferenceFactory(
	t *testing.T,
	dir string,
	scenario sharedInferenceScenario,
	timeout time.Duration,
) sharedInferenceFactoryResult {
	t.Helper()
	group := sharedInferenceGroup
	group.ensure(t)
	releaseOverride, err := group.override.bind(dir, scenario.providerOverride)
	if err != nil {
		t.Fatalf("bind shared inference provider override: %v", err)
	}
	releaseExternalProviders, err := group.bindExternalRegistrations(scenario.providerRegistrations)
	if err != nil {
		t.Fatalf("bind shared inference provider routes: %v", err)
	}
	releaseProvider := prepareSharedInferenceScenario(t, group, dir, scenario)
	defer func() {
		group.commands.clear(dir)
		group.scripts.clear(dir)
		releaseExternalProviders()
		releaseOverride()
		releaseProvider()
	}()

	sessionID := openSharedInferenceSession(t, group, dir)
	sessionClosed := false
	defer func() {
		if !sessionClosed {
			closeSharedInferenceSession(t, group, sessionID)
		}
	}()
	updateSharedInferenceRouteContext(t, group, dir, scenario, sessionID)
	if scenario.workerRecordingWriter != nil && sharedInferenceScenarioSubmitsWork(scenario) {
		group.workerRecordings.setSession(sessionID, scenario.workerRecordingWriter)
		defer group.workerRecordings.clearSession(sessionID)
	}
	var responseStream *support.FactoryResponseEventStream
	if scenario.captureResponse {
		responseStream = support.OpenFactoryResponseEventStreamAt(
			t,
			support.SessionResponseEventsURL(group.baseURL, sessionID),
		)
		defer responseStream.Close()
	}
	if scenario.submittedWork != nil {
		support.SubmitSessionWorkAt(t, group.baseURL, sessionID, *scenario.submittedWork)
	}
	for _, request := range scenario.submittedWorks {
		support.SubmitSessionWorkAt(t, group.baseURL, sessionID, request)
	}
	releaseProvider()

	// The terminal status is the public completion boundary for the whole
	// session, including provider retry policy. Read the retained Factory
	// Events only after that boundary, then request each finite Worker Session
	// replay exactly once; no quiet-period or retry polling is needed.
	support.WaitForSessionTerminalStatus(t, group.baseURL, sessionID, timeout)

	result := collectSharedInferenceFactoryResult(t, group, sessionID, scenario, responseStream, timeout)
	closeSharedInferenceSession(t, group, sessionID)
	sessionClosed = true
	if responseStream != nil {
		responseStream.WaitClosed(timeout)
		result.responseEvents = append(result.responseEvents, drainSharedInferenceResponseEvents(t, responseStream)...)
	}
	return result
}

func prepareSharedInferenceScenario(
	t *testing.T,
	group *inferenceProcessGroup,
	dir string,
	scenario sharedInferenceScenario,
) func() {
	t.Helper()
	var providerRelease chan struct{}
	providerReleased := false
	// A response stream must be subscribed before a fast provider completes.
	// Likewise, a scenario that proves multiple coexisting Works must finish
	// their public admission before the first provider result can settle the
	// session. One deterministic command-edge gate covers both boundaries.
	if scenario.commandRunner != nil && (scenario.captureResponse || len(scenario.submittedWorks) > 1) {
		providerRelease = make(chan struct{})
		scenario.commandRunner = &inferenceResponseCaptureRunner{
			delegate: scenario.commandRunner,
			release:  providerRelease,
		}
	}
	// Register the exact WorkDir selector before opening the session. Opening a
	// session starts its hosted runtime, so the seed Work can reach the command
	// edge before the open request returns.
	routeContext := sharedInferenceRouteContext(scenario, "opening", dir)
	if err := group.commands.set(dir, scenario.commandRunner, scenario.env, routeContext); err != nil {
		t.Fatalf("register shared inference command route: %v", err)
	}
	if err := group.scripts.set(dir, scenario.scriptRunner, nil, routeContext); err != nil {
		t.Fatalf("register shared inference script route: %v", err)
	}
	return func() {
		if providerRelease != nil && !providerReleased {
			close(providerRelease)
			providerReleased = true
		}
	}
}

func openSharedInferenceSession(
	t *testing.T,
	group *inferenceProcessGroup,
	dir string,
) string {
	t.Helper()
	opened := support.OpenFactorySessionAt(t, group.baseURL, dir)
	sessionID := opened.Session.Id
	if sessionID == factorysessions.DefaultSessionID {
		t.Fatalf("opened Factory Session ID = %q, want an explicit non-default session", sessionID)
	}
	group.sessionMu.Lock()
	defer group.sessionMu.Unlock()
	if _, exists := group.sessions[sessionID]; exists {
		t.Fatalf("Factory Session ID %q was reused by the shared process group", sessionID)
	}
	group.sessions[sessionID] = struct{}{}
	group.openedSessionIDs = append(group.openedSessionIDs, sessionID)
	return sessionID
}

// closeSharedInferenceSession records the public deletion only after the
// existing termination-and-delete helper has completed successfully. Its
// census lock protects only the package ledger while peer sessions remain live.
func closeSharedInferenceSession(t *testing.T, group *inferenceProcessGroup, sessionID string) {
	t.Helper()
	support.CloseFactorySessionAt(t, group.baseURL, sessionID)
	group.sessionMu.Lock()
	defer group.sessionMu.Unlock()
	group.deletedSessionIDs = append(group.deletedSessionIDs, sessionID)
}

func updateSharedInferenceRouteContext(
	t *testing.T,
	group *inferenceProcessGroup,
	dir string,
	scenario sharedInferenceScenario,
	sessionID string,
) {
	t.Helper()
	routeContext := sharedInferenceRouteContext(scenario, sessionID, dir)
	if err := group.commands.updateContext(dir, routeContext); err != nil {
		t.Fatalf("update shared inference command route context: %v", err)
	}
	if err := group.scripts.updateContext(dir, routeContext); err != nil {
		t.Fatalf("update shared inference script route context: %v", err)
	}
}

func collectSharedInferenceFactoryResult(
	t *testing.T,
	group *inferenceProcessGroup,
	sessionID string,
	scenario sharedInferenceScenario,
	responseStream *support.FactoryResponseEventStream,
	timeout time.Duration,
) sharedInferenceFactoryResult {
	t.Helper()
	result := sharedInferenceFactoryResult{}
	if responseStream != nil {
		result.responseEvents = readSharedInferenceResponseEvents(t, responseStream, timeout)
	}
	response := support.GetJSON[factoryapi.FactorySessionGetResponse](
		t,
		strings.TrimSuffix(group.baseURL, "/")+"/factory-sessions/"+url.PathEscape(sessionID),
	)
	var err error
	result.session, err = response.AsFactorySession()
	if err != nil {
		t.Fatalf("decode explicit Factory Session %q: %v", sessionID, err)
	}
	result.work = support.GetJSON[factoryapi.ListWorkResponse](
		t,
		strings.TrimSuffix(group.baseURL, "/")+"/factory-sessions/"+url.PathEscape(sessionID)+"/work",
	)
	result.events = support.GetFactoryEventsForSessionAt(t, group.baseURL, sessionID)
	if scenario.captureWorkerEvents {
		result.workerEvents = collectSharedInferenceWorkerEvents(t, group, sessionID, result.events, result.work)
	}
	return result
}

func collectSharedInferenceWorkerEvents(
	t *testing.T,
	group *inferenceProcessGroup,
	sessionID string,
	events []factoryapi.FactoryEvent,
	work factoryapi.ListWorkResponse,
) []factoryapi.WorkerSessionEvent {
	t.Helper()
	var workerEvents []factoryapi.WorkerSessionEvent
	for _, workerSessionID := range sharedInferenceWorkerSessionIDs(t, events) {
		workerEvents = append(workerEvents, readSharedInferenceWorkerSessionReplay(
			t, group.baseURL, sessionID, workerSessionID,
		)...)
	}
	if len(workerEvents) > 0 {
		return workerEvents
	}
	return readSharedInferenceWorkerEvents(t, group.baseURL, sessionID, work)
}

func sharedInferenceWorkerSessionIDs(
	t *testing.T,
	events []factoryapi.FactoryEvent,
) []string {
	t.Helper()
	workerSessionIDs := make([]string, 0)
	seenWorkerSessionIDs := make(map[string]struct{})
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeDispatchWorkerSessionAssociation {
			continue
		}
		payload, err := event.Payload.AsDispatchWorkerSessionAssociationEventPayload()
		if err != nil {
			t.Fatalf("decode shared inference Worker Session association: %v", err)
		}
		if payload.WorkerSessionId == "" {
			t.Fatalf("shared inference Worker Session association has empty Worker Session ID: %#v", event)
		}
		if _, exists := seenWorkerSessionIDs[payload.WorkerSessionId]; !exists {
			seenWorkerSessionIDs[payload.WorkerSessionId] = struct{}{}
			workerSessionIDs = append(workerSessionIDs, payload.WorkerSessionId)
		}
	}
	return workerSessionIDs
}

func readSharedInferenceResponseEvents(
	t *testing.T,
	stream *support.FactoryResponseEventStream,
	timeout time.Duration,
) []factoryapi.FactoryResponseEvent {
	t.Helper()
	deadline := time.Now().Add(timeout)
	events := make([]factoryapi.FactoryResponseEvent, 0)
	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			t.Fatalf("waiting for terminal Factory Response Event exceeded %s", timeout)
		}
		frame := stream.NextFrame(remaining)
		events = append(events, frame.Event)
		if sharedInferenceTerminalResponseEvent(frame.Event) {
			break
		}
	}
	return events
}

func drainSharedInferenceResponseEvents(
	t *testing.T,
	stream *support.FactoryResponseEventStream,
) []factoryapi.FactoryResponseEvent {
	t.Helper()
	events := make([]factoryapi.FactoryResponseEvent, 0)
	for {
		result := stream.TryNextFrameResult(0)
		switch result.Outcome {
		case support.FactoryResponseEventStreamOutcomeFrame:
			events = append(events, result.Frame.Event)
		case support.FactoryResponseEventStreamOutcomeReadError:
			t.Fatalf("drain closed Factory Response Event stream: %s", result.Diagnostic())
		default:
			return events
		}
	}
}

func sharedInferenceTerminalResponseEvent(event factoryapi.FactoryResponseEvent) bool {
	if event.Kind == factoryapi.FactoryResponseEventKindError {
		return event.Phase == factoryapi.FactoryResponseEventPhaseFailed ||
			event.Phase == factoryapi.FactoryResponseEventPhaseCanceled
	}
	return event.Kind == factoryapi.FactoryResponseEventKindRun &&
		(event.Phase == factoryapi.FactoryResponseEventPhaseCompleted ||
			event.Phase == factoryapi.FactoryResponseEventPhaseFailed ||
			event.Phase == factoryapi.FactoryResponseEventPhaseCanceled)
}

func withSharedInferenceProcess(
	t *testing.T,
	scenario sharedInferenceScenario,
	fn func(support.ApplicationProcess),
) {
	t.Helper()
	group := sharedInferenceGroup
	group.ensure(t)
	releaseExternalProviders, err := group.bindExternalRegistrations(scenario.providerRegistrations)
	if err != nil {
		t.Fatalf("bind shared inference provider routes: %v", err)
	}
	defer releaseExternalProviders()
	fn(group.process)
}

func sharedInferenceWork(title string) *factoryapi.SubmitWorkRequest {
	return &factoryapi.SubmitWorkRequest{
		WorkTypeName: "task",
		Payload:      map[string]any{"title": title},
	}
}

func sharedInferenceWorks(titles ...string) []factoryapi.SubmitWorkRequest {
	requests := make([]factoryapi.SubmitWorkRequest, len(titles))
	for index, title := range titles {
		requests[index] = *sharedInferenceWork(title)
	}
	return requests
}

func sharedInferenceScenarioSubmitsWork(scenario sharedInferenceScenario) bool {
	return scenario.submittedWork != nil || len(scenario.submittedWorks) > 0
}

func readSharedInferenceWorkerEvents(
	t *testing.T,
	baseURL string,
	sessionID string,
	work factoryapi.ListWorkResponse,
) []factoryapi.WorkerSessionEvent {
	t.Helper()
	var events []factoryapi.WorkerSessionEvent
	seen := make(map[string]struct{})
	for _, item := range work.Results {
		workID := support.StringPointerValue(item.WorkId)
		if workID == "" {
			continue
		}
		endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" +
			url.PathEscape(sessionID) + "/worker-sessions?workId=" + url.QueryEscape(workID)
		observations := support.GetJSON[factoryapi.ListWorkerSessionsResponse](t, endpoint)
		for _, observation := range observations.Sessions {
			if observation.WorkerSessionId == "" {
				continue
			}
			if _, exists := seen[observation.WorkerSessionId]; exists {
				continue
			}
			seen[observation.WorkerSessionId] = struct{}{}
			events = append(events, readSharedInferenceWorkerSessionReplay(
				t, baseURL, sessionID, observation.WorkerSessionId,
			)...)
		}
	}
	return events
}

func readSharedInferenceWorkerSessionReplay(
	t *testing.T,
	baseURL string,
	sessionID string,
	workerSessionID string,
) []factoryapi.WorkerSessionEvent {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), sharedInferenceScenarioTimeout)
	defer cancel()
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" +
		url.PathEscape(sessionID) + "/worker-sessions/" + url.PathEscape(workerSessionID) +
		"/events?replayOnly=true"
	events, summary, err := readSharedInferenceWorkerReplay(ctx, endpoint)
	if err != nil {
		t.Fatalf("GET Worker Session events: %v", err)
	}
	if summary == nil || !summary.Complete {
		t.Fatalf("Worker Session replay at %s returned incomplete summary: %#v", endpoint, summary)
	}
	return events
}

func readSharedInferenceWorkerReplay(
	ctx context.Context,
	endpoint string,
) ([]factoryapi.WorkerSessionEvent, *factoryapi.WorkerSessionReplaySummary, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("build Worker Session events request: %w", err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, nil, fmt.Errorf("GET Worker Session events: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		return nil, nil, fmt.Errorf("GET Worker Session events status = %d body = %s", response.StatusCode, strings.TrimSpace(string(body)))
	}

	var events []factoryapi.WorkerSessionEvent
	scanner := bufio.NewScanner(response.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		var event factoryapi.WorkerSessionEvent
		if err := json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data:"))), &event); err != nil {
			return nil, nil, fmt.Errorf("decode Worker Session event: %w", err)
		}
		events = append(events, event)
		if string(event.Delivery) == "REPLAY_SUMMARY" || event.ReplaySummary != nil {
			if event.ReplaySummary == nil {
				return nil, nil, errors.New("Worker Session replay summary is empty")
			}
			return events, event.ReplaySummary, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, nil, fmt.Errorf("read Worker Session events: %w", err)
	}
	return nil, nil, errors.New("Worker Session event stream ended without replay summary")
}

func sharedInferenceProcessEnvironment(homeDir string) []string {
	environment := append([]string(nil), os.Environ()...)
	environment = setSharedInferenceEnvironment(environment, "HOME", homeDir)
	environment = setSharedInferenceEnvironment(environment, "USERPROFILE", homeDir)
	return environment
}

func setSharedInferenceEnvironment(environment []string, name, value string) []string {
	prefix := name + "="
	result := make([]string, 0, len(environment)+1)
	for _, entry := range environment {
		if strings.EqualFold(strings.SplitN(entry, "=", 2)[0]+"=", prefix) {
			continue
		}
		result = append(result, entry)
	}
	return append(result, name+"="+value)
}

func sharedInferenceRouteContext(
	scenario sharedInferenceScenario,
	sessionID string,
	dir string,
) inferenceRouteContext {
	name := strings.TrimSpace(scenario.scenarioName)
	if name == "" {
		name = cleanInferencePath(dir)
	}
	return inferenceRouteContext{scenarioName: name, sessionID: sessionID}
}

func cleanInferencePath(path string) string {
	if path == "" {
		return ""
	}
	cleaned, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	return filepath.Clean(cleaned)
}

func sharedInferenceWithExecutorProvider(config, provider string) string {
	marker := "type: MODEL_WORKER"
	return strings.Replace(config, marker, "executorProvider: "+provider+"\n"+marker, 1)
}

func writeSharedInferenceHostFactory(dir string) error {
	if err := os.MkdirAll(filepath.Join(dir, "workers", "worker"), 0o755); err != nil {
		return fmt.Errorf("create shared host worker directory: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "workstations", "process"), 0o755); err != nil {
		return fmt.Errorf("create shared host workstation directory: %w", err)
	}
	config := map[string]any{
		"name": "shared-inference-host",
		"workTypes": []map[string]any{{
			"name": "task",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "done", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]string{{"name": "worker"}},
		"workstations": []map[string]any{{
			"name":      "process",
			"worker":    "worker",
			"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
			"outputs":   []map[string]string{{"workType": "task", "state": "done"}},
			"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
		}},
	}
	encoded, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("marshal shared host factory: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, factoryinterfaces.FactoryConfigFile), encoded, 0o644); err != nil {
		return fmt.Errorf("write shared host factory: %w", err)
	}
	workerConfig := sharedInferenceWithExecutorProvider("---\nmodel: gpt-5-codex\nmodelProvider: CODEX\nstopToken: COMPLETE\ntype: MODEL_WORKER\n---\nShared host worker.\n", "CODEX")
	if err := os.WriteFile(filepath.Join(dir, "workers", "worker", "AGENTS.md"), []byte(workerConfig), 0o644); err != nil {
		return fmt.Errorf("write shared host worker: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "workstations", "process", "AGENTS.md"), []byte("---\ntype: MODEL_WORKSTATION\n---\nShared host workstation.\n"), 0o644); err != nil {
		return fmt.Errorf("write shared host workstation: %w", err)
	}
	return nil
}

var _ platformprocess.CommandRunner = (*inferenceCommandRouter)(nil)
var _ platformprocess.CommandRunner = (*inferenceResponseCaptureRunner)(nil)
var _ recordings.WorkerRecordingWriter = (*inferenceWorkerRecordingRouter)(nil)
var _ recordings.WorkerRecordingReader = (*inferenceWorkerRecordingRouter)(nil)
var _ recordings.WorkerRecordingFailureWriter = (*inferenceWorkerRecordingRouter)(nil)
