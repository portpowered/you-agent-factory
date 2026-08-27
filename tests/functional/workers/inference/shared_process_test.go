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
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	platformreplay "github.com/portpowered/infinite-you/pkg/platform/replay"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryinterfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	providerswire "github.com/portpowered/infinite-you/pkg/services/providers/wire"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	recordingswire "github.com/portpowered/infinite-you/pkg/services/recordings/wire"
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
	if err := sharedInferenceGroup.close(); err != nil {
		fmt.Fprintf(os.Stderr, "close shared inference process group: %v\n", err)
		if code == 0 {
			code = 1
		}
	}
	os.Exit(code)
}

type inferenceProcessGroup struct {
	once     sync.Once
	setupErr error

	mu       sync.Mutex
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

	externals map[string]*inferenceIntegrationRouter
	sessions  map[string]struct{}
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
	group.override = &inferenceProviderOverride{}
	workerRecordingFallback, err := recordingswire.NewWorkerRecordingFileWriter(
		platformreplay.NewLocal(runtime.GOOS),
		filepath.Join(group.rootDir, "worker-recordings"),
	)
	if err != nil {
		group.setupErr = fmt.Errorf("create shared Worker recording fallback: %w", err)
		return
	}
	group.workerRecordings = &inferenceWorkerRecordingRouter{fallback: workerRecordingFallback}
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

	registrations := make([]providerswire.Registration, 0, len(group.externals))
	for _, provider := range providerDefinitions {
		integration := group.externals[provider.id]
		registrations = append(registrations, providerswire.Registration{
			Manifest:    sharedInferenceExternalManifest(provider.id, provider.alias),
			Integration: integration,
		})
	}
	defaultProviders, err := providerswire.NewService(
		providerswire.WithCommandRunner(group.commands),
		providerswire.WithWorkersCommandRunner(group.commands),
		providerswire.WithAgyCommandRunner(group.commands),
		providerswire.WithRegistrations(registrations...),
	)
	if err != nil {
		group.setupErr = fmt.Errorf("create shared default Providers service: %w", err)
		return
	}
	group.override.defaultService = defaultProviders
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
	providerRegistrations []providerswire.Registration
	workerRecordingWriter recordings.WorkerRecordingWriter
	env                   []string
	captureResponse       bool
	captureWorkerEvents   bool
	stopDaemonForExecute  bool
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
	group.mu.Lock()
	defer group.mu.Unlock()
	var responseRelease chan struct{}
	responseReleased := false
	if scenario.captureResponse && scenario.commandRunner != nil {
		responseRelease = make(chan struct{})
		scenario.commandRunner = &inferenceResponseCaptureRunner{
			delegate: scenario.commandRunner,
			release:  responseRelease,
		}
	}

	group.commands.set(dir, scenario.commandRunner, scenario.env)
	group.scripts.set(dir, scenario.scriptRunner, nil)
	group.override.set(scenario.providerOverride)
	group.workerRecordings.set(scenario.workerRecordingWriter)
	group.setExternalRegistrations(scenario.providerRegistrations)
	defer func() {
		group.commands.clear(dir)
		group.scripts.clear(dir)
		group.override.set(nil)
		group.workerRecordings.set(nil)
		group.setExternalRegistrations(nil)
		if responseRelease != nil && !responseReleased {
			close(responseRelease)
		}
	}()

	opened := support.OpenFactorySessionAt(t, group.baseURL, dir)
	sessionID := opened.Session.Id
	if sessionID == factorysessions.DefaultSessionID {
		t.Fatalf("opened Factory Session ID = %q, want an explicit non-default session", sessionID)
	}
	if _, exists := group.sessions[sessionID]; exists {
		t.Fatalf("Factory Session ID %q was reused by the shared process group", sessionID)
	}
	group.sessions[sessionID] = struct{}{}
	defer support.CloseFactorySessionAt(t, group.baseURL, sessionID)
	var responseStream *support.FactoryResponseEventStream
	if scenario.captureResponse {
		responseStream = support.OpenFactoryResponseEventStreamAt(
			t,
			support.SessionResponseEventsURL(group.baseURL, sessionID),
		)
		defer responseStream.Close()
	}
	if responseRelease != nil {
		close(responseRelease)
		responseReleased = true
	}

	support.WaitForSessionTerminalStatus(t, group.baseURL, sessionID, timeout)

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
		result.workerEvents = readSharedInferenceWorkerEvents(t, group.baseURL, sessionID, result.work)
	}
	return result
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
	for {
		frame, ok := stream.TryNextFrame(100 * time.Millisecond)
		if !ok {
			break
		}
		events = append(events, frame.Event)
	}
	return events
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
	group.mu.Lock()
	defer group.mu.Unlock()
	group.override.set(scenario.providerOverride)
	group.workerRecordings.set(scenario.workerRecordingWriter)
	group.setExternalRegistrations(scenario.providerRegistrations)
	defer func() {
		group.override.set(nil)
		group.workerRecordings.set(nil)
		group.setExternalRegistrations(nil)
	}()
	fn(group.process)
}

func withSharedInferenceProcessAt(
	t *testing.T,
	dir string,
	scenario sharedInferenceScenario,
	fn func(support.ApplicationProcess),
) {
	t.Helper()
	group := sharedInferenceGroup
	group.ensure(t)
	group.mu.Lock()
	defer group.mu.Unlock()
	if scenario.stopDaemonForExecute {
		if err := group.stopDaemon(); err != nil {
			t.Fatalf("stop shared inference daemon for direct execution: %v", err)
		}
		group.setServer(support.NewProcessAPIServer())
	}
	group.commands.set(dir, scenario.commandRunner, scenario.env)
	group.scripts.set(dir, scenario.scriptRunner, nil)
	group.override.set(scenario.providerOverride)
	group.workerRecordings.set(scenario.workerRecordingWriter)
	group.setExternalRegistrations(scenario.providerRegistrations)
	defer func() {
		group.commands.clear(dir)
		group.scripts.clear(dir)
		group.override.set(nil)
		group.workerRecordings.set(nil)
		group.setExternalRegistrations(nil)
		if scenario.stopDaemonForExecute {
			if err := group.startDaemon(); err != nil {
				t.Errorf("restart shared inference daemon after direct execution: %v", err)
			}
		}
	}()
	fn(group.process)
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
	var lastSummary *factoryapi.WorkerSessionReplaySummary
	for {
		events, summary, err := readSharedInferenceWorkerReplay(ctx, endpoint)
		if err != nil {
			t.Fatalf("GET Worker Session events: %v", err)
		}
		lastSummary = summary
		if summary != nil && summary.Complete {
			return events
		}
		select {
		case <-ctx.Done():
			t.Fatalf("waiting for complete Worker Session replay at %s; last summary=%#v: %v", endpoint, lastSummary, ctx.Err())
		case <-time.After(10 * time.Millisecond):
		}
	}
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

type inferenceCommandRoute struct {
	runner platformprocess.CommandRunner
	env    []string
}

type inferenceResponseCaptureRunner struct {
	delegate platformprocess.CommandRunner
	release  <-chan struct{}
}

func (runner *inferenceResponseCaptureRunner) wait(ctx context.Context) error {
	select {
	case <-runner.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (runner *inferenceResponseCaptureRunner) Run(
	ctx context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	if err := runner.wait(ctx); err != nil {
		return platformprocess.CommandResult{}, err
	}
	return runner.delegate.Run(ctx, request)
}

func (runner *inferenceResponseCaptureRunner) RunStreaming(
	ctx context.Context,
	request platformprocess.CommandRequest,
	observer platformprocess.OutputChunkObserver,
) (platformprocess.CommandResult, error) {
	if err := runner.wait(ctx); err != nil {
		return platformprocess.CommandResult{}, err
	}
	streaming, ok := runner.delegate.(interface {
		RunStreaming(context.Context, platformprocess.CommandRequest, platformprocess.OutputChunkObserver) (platformprocess.CommandResult, error)
	})
	if ok {
		return streaming.RunStreaming(ctx, request, observer)
	}
	result, err := runner.delegate.Run(ctx, request)
	if observer != nil {
		if len(result.Stdout) > 0 {
			observer(platformprocess.OutputStreamStdout, append([]byte(nil), result.Stdout...))
		}
		if len(result.Stderr) > 0 {
			observer(platformprocess.OutputStreamStderr, append([]byte(nil), result.Stderr...))
		}
	}
	return result, err
}

type inferenceCommandRouter struct {
	mu     sync.Mutex
	routes map[string]inferenceCommandRoute
}

// inferenceWorkerRecordingRouter keeps the root-built process's durable
// recording port stable while allowing one serialized scenario to observe the
// exact Worker records it produced. The fallback is intentionally real file
// persistence so scenarios that do not inspect recordings still exercise the
// production recording composition.
type inferenceWorkerRecordingRouter struct {
	mu       sync.RWMutex
	fallback recordings.WorkerRecordingWriter
	delegate recordings.WorkerRecordingWriter
}

func (router *inferenceWorkerRecordingRouter) set(delegate recordings.WorkerRecordingWriter) {
	if router == nil {
		return
	}
	router.mu.Lock()
	router.delegate = delegate
	router.mu.Unlock()
}

func (router *inferenceWorkerRecordingRouter) current() recordings.WorkerRecordingWriter {
	if router == nil {
		return nil
	}
	router.mu.RLock()
	defer router.mu.RUnlock()
	if router.delegate != nil {
		return router.delegate
	}
	return router.fallback
}

func (router *inferenceWorkerRecordingRouter) PersistWorkerRecord(
	ctx context.Context,
	record recordings.WorkerRecordingRecord,
) error {
	writer := router.current()
	if writer == nil {
		return recordings.ErrMissingWorkerRecordingWriter
	}
	return writer.PersistWorkerRecord(ctx, record)
}

func (router *inferenceWorkerRecordingRouter) PersistWorkerRecordingFailure(
	ctx context.Context,
	failure recordings.WorkerRecordingFailure,
) error {
	writer := router.current()
	failureWriter, ok := writer.(recordings.WorkerRecordingFailureWriter)
	if !ok || failureWriter == nil {
		return recordings.ErrMissingWorkerRecordingWriter
	}
	return failureWriter.PersistWorkerRecordingFailure(ctx, failure)
}

func (router *inferenceWorkerRecordingRouter) LoadWorkerRecording(
	ctx context.Context,
	recordingID string,
) (recordings.WorkerRecordingSnapshot, error) {
	reader, ok := router.current().(recordings.WorkerRecordingReader)
	if !ok || reader == nil {
		return recordings.WorkerRecordingSnapshot{}, recordings.ErrMissingWorkerRecordingReader
	}
	return reader.LoadWorkerRecording(ctx, recordingID)
}

func (router *inferenceCommandRouter) set(dir string, runner platformprocess.CommandRunner, env []string) {
	if router == nil {
		return
	}
	router.mu.Lock()
	defer router.mu.Unlock()
	router.routes[cleanInferencePath(dir)] = inferenceCommandRoute{runner: runner, env: append([]string(nil), env...)}
}

func (router *inferenceCommandRouter) clear(dir string) {
	if router == nil {
		return
	}
	router.mu.Lock()
	defer router.mu.Unlock()
	delete(router.routes, cleanInferencePath(dir))
}

func (router *inferenceCommandRouter) route(request platformprocess.CommandRequest) (inferenceCommandRoute, error) {
	router.mu.Lock()
	defer router.mu.Unlock()
	if route, ok := router.routes[cleanInferencePath(request.WorkDir)]; ok && route.runner != nil {
		return route, nil
	}
	if len(router.routes) == 1 {
		for _, route := range router.routes {
			if route.runner != nil {
				return route, nil
			}
		}
	}
	return inferenceCommandRoute{}, fmt.Errorf("shared inference command route is not registered for %q", request.WorkDir)
}

func (router *inferenceCommandRouter) Run(
	ctx context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	route, err := router.route(request)
	if err != nil {
		return platformprocess.CommandResult{}, err
	}
	request.Env = overlayInferenceEnvironment(request.Env, route.env)
	return route.runner.Run(ctx, request)
}

func (router *inferenceCommandRouter) RunStreaming(
	ctx context.Context,
	request platformprocess.CommandRequest,
	observer platformprocess.OutputChunkObserver,
) (platformprocess.CommandResult, error) {
	route, err := router.route(request)
	if err != nil {
		return platformprocess.CommandResult{}, err
	}
	request.Env = overlayInferenceEnvironment(request.Env, route.env)
	streaming, ok := route.runner.(interface {
		RunStreaming(context.Context, platformprocess.CommandRequest, platformprocess.OutputChunkObserver) (platformprocess.CommandResult, error)
	})
	if ok {
		return streaming.RunStreaming(ctx, request, observer)
	}
	result, err := route.runner.Run(ctx, request)
	if observer != nil {
		if len(result.Stdout) > 0 {
			observer(platformprocess.OutputStreamStdout, append([]byte(nil), result.Stdout...))
		}
		if len(result.Stderr) > 0 {
			observer(platformprocess.OutputStreamStderr, append([]byte(nil), result.Stderr...))
		}
	}
	return result, err
}

func overlayInferenceEnvironment(base, overlay []string) []string {
	if len(overlay) == 0 {
		return base
	}
	result := make([]string, 0, len(base)+len(overlay))
	for _, value := range base {
		name := strings.SplitN(value, "=", 2)[0]
		duplicate := false
		for _, replacement := range overlay {
			if strings.EqualFold(name, strings.SplitN(replacement, "=", 2)[0]) {
				duplicate = true
				break
			}
		}
		if !duplicate {
			result = append(result, value)
		}
	}
	return append(result, overlay...)
}

type inferenceIntegrationRouter struct {
	identity string
	mu       sync.RWMutex
	delegate providerswire.Integration
}

func (router *inferenceIntegrationRouter) set(delegate providerswire.Integration) {
	router.mu.Lock()
	router.delegate = delegate
	router.mu.Unlock()
}

func (router *inferenceIntegrationRouter) current() providerswire.Integration {
	router.mu.RLock()
	defer router.mu.RUnlock()
	return router.delegate
}

func (router *inferenceIntegrationRouter) Identity() providerswire.Identity {
	return providerswire.Identity(router.identity)
}

func (router *inferenceIntegrationRouter) MaximumCapabilities() providerswire.CapabilitySet {
	if delegate := router.current(); delegate != nil {
		return delegate.MaximumCapabilities()
	}
	return providerswire.NewCapabilitySet(providerswire.CapabilityPromptSubmission)
}

func (router *inferenceIntegrationRouter) Discover(ctx context.Context) (providerswire.Discovery, error) {
	if delegate := router.current(); delegate != nil {
		return delegate.Discover(ctx)
	}
	return providerswire.Discovery{}, nil
}

func (router *inferenceIntegrationRouter) Capabilities(ctx context.Context, request providerswire.InvocationRequest) (providerswire.CapabilitySet, error) {
	if delegate := router.current(); delegate != nil {
		return delegate.Capabilities(ctx, request)
	}
	return router.MaximumCapabilities(), nil
}

func (router *inferenceIntegrationRouter) Invoke(ctx context.Context, request providerswire.InvocationRequest, writer providerswire.ResponseWriter) error {
	if delegate := router.current(); delegate != nil {
		return delegate.Invoke(ctx, request, writer)
	}
	return fmt.Errorf("shared inference provider %q is not configured", router.identity)
}

func (group *inferenceProcessGroup) setExternalRegistrations(registrations []providerswire.Registration) {
	for _, router := range group.externals {
		router.set(nil)
	}
	for _, registration := range registrations {
		if router := group.externals[registration.Manifest.ID]; router != nil {
			router.set(registration.Integration)
		}
	}
}

type inferenceProviderOverride struct {
	mu             sync.RWMutex
	delegate       providers.Service
	defaultService providers.Service
}

func (router *inferenceProviderOverride) set(delegate providers.Service) {
	router.mu.Lock()
	router.delegate = delegate
	router.mu.Unlock()
}

func (router *inferenceProviderOverride) current() (providers.Service, error) {
	router.mu.RLock()
	defer router.mu.RUnlock()
	if router.delegate == nil {
		if router.defaultService != nil {
			return router.defaultService, nil
		}
		return nil, errors.New("shared inference provider override is not configured")
	}
	return router.delegate, nil
}

func (router *inferenceProviderOverride) ListProviders(ctx context.Context, request providers.ListProvidersRequest) (providers.ListProvidersResult, error) {
	delegate, err := router.current()
	if err != nil {
		return providers.ListProvidersResult{}, err
	}
	return delegate.ListProviders(ctx, request)
}

func (router *inferenceProviderOverride) GetProvider(ctx context.Context, request providers.GetProviderRequest) (providers.GetProviderResult, error) {
	delegate, err := router.current()
	if err != nil {
		return providers.GetProviderResult{}, err
	}
	return delegate.GetProvider(ctx, request)
}

func (router *inferenceProviderOverride) ResolveIdentity(ctx context.Context, request providers.ResolveIdentityRequest) (providers.ResolveIdentityResult, error) {
	delegate, err := router.current()
	if err != nil {
		return providers.ResolveIdentityResult{}, err
	}
	return delegate.ResolveIdentity(ctx, request)
}

func (router *inferenceProviderOverride) ResolveSelection(ctx context.Context, request providers.ResolveSelectionRequest) (providers.ResolveSelectionResult, error) {
	delegate, err := router.current()
	if err != nil {
		return providers.ResolveSelectionResult{}, err
	}
	return delegate.ResolveSelection(ctx, request)
}

func (router *inferenceProviderOverride) ValidatePrerequisites(ctx context.Context, request providers.ValidatePrerequisitesRequest) error {
	delegate, err := router.current()
	if err != nil {
		return err
	}
	return delegate.ValidatePrerequisites(ctx, request)
}

func (router *inferenceProviderOverride) Execute(ctx context.Context, request providers.ExecuteRequest) (providers.ExecuteResult, error) {
	delegate, err := router.current()
	if err != nil {
		return providers.ExecuteResult{}, err
	}
	return delegate.Execute(ctx, request)
}

func (router *inferenceProviderOverride) ControlAttempt(ctx context.Context, request providers.ControlAttemptRequest) (providers.ControlAttemptResult, error) {
	delegate, err := router.current()
	if err != nil {
		return providers.ControlAttemptResult{}, err
	}
	return delegate.ControlAttempt(ctx, request)
}

func (router *inferenceProviderOverride) Continue(ctx context.Context, request providers.ContinueRequest) (providers.ContinueResult, error) {
	delegate, err := router.current()
	if err != nil {
		return providers.ContinueResult{}, err
	}
	return delegate.Continue(ctx, request)
}

func (router *inferenceProviderOverride) ContinueReference(ctx context.Context, request providers.ContinueReferenceRequest) (providers.ContinueReferenceResult, error) {
	delegate, err := router.current()
	if err != nil {
		return providers.ContinueReferenceResult{}, err
	}
	return delegate.ContinueReference(ctx, request)
}

func sharedInferenceExternalManifest(id, alias string) providerswire.Manifest {
	return providerswire.Manifest{
		ID:                         id,
		Aliases:                    []string{alias},
		ImplementationAvailability: providerswire.ImplementationExternallySupplied,
		TechnicalSupportLevel:      providerswire.SupportProduction,
		MaximumExecutionCapabilities: providerswire.ExecutionCapabilities{
			PromptSubmission: true,
		},
	}
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
var _ providerswire.Integration = (*inferenceIntegrationRouter)(nil)
var _ providers.Service = (*inferenceProviderOverride)(nil)
