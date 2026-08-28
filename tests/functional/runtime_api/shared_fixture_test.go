package runtime_api

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/portpowered/infinite-you/internal/testutil"
	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	runtimeAPIPackageFixtureTimeout       = 15 * time.Second
	runtimeAPISessionCleanupTimeout       = 10 * time.Second
	runtimeAPIPackageShutdownTimeout      = 5 * time.Second
	runtimeAPIPackageListenerProbeTimeout = 2 * time.Second
	runtimeAPIWindowsConnectionRefused    = syscall.Errno(10061)
)

var (
	runtimeAPIFixtureOnce sync.Once
	runtimeAPIFixtureMu   sync.Mutex
	runtimeAPIFixtureVal  *runtimeAPIPackageFixture
	runtimeAPIFixtureErr  error
)

// TestMain gives the eligible runtime API cohort one package-scoped lifecycle.
// The fixture is lazy so isolated tests and short runs do not pay for a daemon
// they do not exercise.
func TestMain(m *testing.M) {
	code := m.Run()

	runtimeAPIFixtureMu.Lock()
	fixture := runtimeAPIFixtureVal
	runtimeAPIFixtureMu.Unlock()
	if fixture != nil {
		if err := fixture.close(); err != nil {
			fmt.Fprintf(os.Stderr, "runtime API package fixture cleanup: %v\n", err)
			if code == 0 {
				code = 1
			}
		}
	}

	os.Exit(code)
}

type runtimeAPIPackageFixture struct {
	rootDir string
	hostDir string
	baseURL string

	process support.ApplicationProcess
	command *runtimeAPIProcessCommand

	apiStarts     atomic.Int64
	processStarts atomic.Int64
	ledger        *runtimeAPICleanupLedger
	closeOnce     sync.Once
	closeErr      error

	providerRouter *runtimeAPIProviderRouter
	commandRouter  *runtimeAPICommandRouter
	scriptRouter   *runtimeAPICommandRouter
}

// runtimeAPICleanupLedger records resources owned by the package fixture or a
// shared scenario. A successful release removes the resource from the active
// set; counts remain so a missing or duplicated release cannot look clean.
type runtimeAPICleanupLedger struct {
	mu sync.Mutex

	nextStreamID  uint64
	sessions      map[string]struct{}
	streams       map[uint64]struct{}
	sessionOpens  int
	sessionCloses int
	streamOpens   int
	streamCloses  int
}

func newRuntimeAPICleanupLedger() *runtimeAPICleanupLedger {
	return &runtimeAPICleanupLedger{
		sessions: make(map[string]struct{}),
		streams:  make(map[uint64]struct{}),
	}
}

func (ledger *runtimeAPICleanupLedger) trackSession(id string) (func() error, error) {
	if ledger == nil {
		return func() error { return nil }, nil
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, errors.New("runtime API session ID is empty")
	}
	ledger.mu.Lock()
	if _, exists := ledger.sessions[id]; exists {
		ledger.mu.Unlock()
		return nil, fmt.Errorf("runtime API session %q is already tracked", id)
	}
	ledger.sessions[id] = struct{}{}
	ledger.sessionOpens++
	ledger.mu.Unlock()

	var once sync.Once
	var releaseErr error
	return func() error {
		once.Do(func() {
			ledger.mu.Lock()
			defer ledger.mu.Unlock()
			if _, exists := ledger.sessions[id]; !exists {
				releaseErr = fmt.Errorf("runtime API session %q was not tracked", id)
				return
			}
			delete(ledger.sessions, id)
			ledger.sessionCloses++
		})
		return releaseErr
	}, nil
}

func (ledger *runtimeAPICleanupLedger) trackStream() func() {
	if ledger == nil {
		return func() {}
	}
	ledger.mu.Lock()
	ledger.nextStreamID++
	id := ledger.nextStreamID
	ledger.streams[id] = struct{}{}
	ledger.streamOpens++
	ledger.mu.Unlock()

	var once sync.Once
	return func() {
		once.Do(func() {
			ledger.mu.Lock()
			defer ledger.mu.Unlock()
			if _, exists := ledger.streams[id]; !exists {
				return
			}
			delete(ledger.streams, id)
			ledger.streamCloses++
		})
	}
}

func (ledger *runtimeAPICleanupLedger) leakError() error {
	if ledger == nil {
		return nil
	}
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	if len(ledger.sessions) == 0 && len(ledger.streams) == 0 &&
		ledger.sessionOpens == ledger.sessionCloses && ledger.streamOpens == ledger.streamCloses {
		return nil
	}
	activeSessions := make([]string, 0, len(ledger.sessions))
	for id := range ledger.sessions {
		activeSessions = append(activeSessions, id)
	}
	return fmt.Errorf(
		"runtime API cleanup ledger is not empty: active sessions=%v streams=%d session opens/closes=%d/%d stream opens/closes=%d/%d",
		activeSessions,
		len(ledger.streams),
		ledger.sessionOpens,
		ledger.sessionCloses,
		ledger.streamOpens,
		ledger.streamCloses,
	)
}

type runtimeAPIScenario struct {
	provider       any
	providerRunner platformprocess.CommandRunner
	scriptRunner   platformprocess.CommandRunner
	models         []string
}

func (fs *functionalAPIServer) URL() string {
	if fs == nil {
		return ""
	}
	if fs.shared != nil {
		return fs.shared.baseURL
	}
	if fs.FunctionalAPIServer != nil {
		return fs.FunctionalAPIServer.URL()
	}
	return ""
}

func (fs *functionalAPIServer) sessionURL(path string) string {
	if fs == nil || fs.sessionID == "" {
		return ""
	}
	return strings.TrimSuffix(fs.URL(), "/") + "/factory-sessions/" + url.PathEscape(fs.sessionID) + path
}

func (fs *functionalAPIServer) workURL(path string) string {
	if fs != nil && fs.shared != nil {
		return fs.sessionURL(path)
	}
	return support.DefaultSessionWorkURL(fs.URL(), path)
}

func (fs *functionalAPIServer) eventsURL() string {
	if fs != nil && fs.shared != nil {
		return support.SessionEventsURL(fs.URL(), fs.sessionID)
	}
	return support.DefaultSessionEventsURL(fs.URL())
}

func (fs *functionalAPIServer) responseEventsURL() string {
	if fs != nil && fs.shared != nil {
		return support.SessionResponseEventsURL(fs.URL(), fs.sessionID)
	}
	return support.SessionResponseEventsURL(fs.URL(), "~default")
}

func (fs *functionalAPIServer) statusURL() string {
	if fs != nil && fs.shared != nil {
		return fs.sessionURL("/status")
	}
	return strings.TrimSuffix(fs.URL(), "/") + "/status"
}

func (fs *functionalAPIServer) StatusURL() string {
	return fs.statusURL()
}

func (fs *functionalAPIServer) Session(t *testing.T) factoryapi.FactorySession {
	t.Helper()
	if fs != nil && fs.shared != nil {
		response := support.GetJSON[factoryapi.FactorySessionGetResponse](t, fs.sessionURL(""))
		session, err := response.AsFactorySession()
		if err != nil {
			t.Fatalf("decode shared Factory Session: %v", err)
		}
		return session
	}
	return support.GetDefaultSession(t, fs.URL())
}

func (fs *functionalAPIServer) GetFactoryEvents(t *testing.T) []factoryapi.FactoryEvent {
	t.Helper()
	if fs != nil && fs.shared != nil {
		return support.GetFactoryEventsForSessionAt(t, fs.URL(), fs.sessionID)
	}
	return support.GetFactoryEventsAt(t, fs.URL())
}

func (fs *functionalAPIServer) openEventStream(t *testing.T) *factoryEventHTTPStream {
	t.Helper()
	stream := openFactoryEventHTTPStream(t, fs.eventsURL())
	if fs != nil && fs.shared != nil {
		fs.shared.trackStream(stream)
	}
	return stream
}

func startSharedFunctionalServer(
	t *testing.T,
	factoryDir string,
	scenario runtimeAPIScenario,
) *functionalAPIServer {
	t.Helper()

	fixture := sharedRuntimeAPIFixture(t)
	provider := runtimeAPIProviderForScenario(t, scenario)
	if provider == nil && scenario.providerRunner == nil {
		// A mock-worker scenario does not call Providers, but registering a
		// fail-closed route keeps an accidental real invocation from escaping
		// the controlled package fixture.
		provider = testutil.NativeProvider{}
	}

	var unregisterProvider func()
	if provider != nil {
		unregisterProvider = fixture.providerRouter.register(factoryDir, scenario.models, provider)
	}
	var unregisterCommand func()
	if scenario.providerRunner != nil {
		unregisterCommand = fixture.commandRouter.register(factoryDir, scenario.providerRunner)
	}
	var unregisterScript func()
	if scenario.scriptRunner != nil {
		unregisterScript = fixture.scriptRouter.register(factoryDir, scenario.scriptRunner)
	}
	// Register route cleanup before opening the session. If session creation
	// fails, testing.T cleanup still resets every scenario-owned effect lane.
	t.Cleanup(func() {
		if unregisterScript != nil {
			unregisterScript()
		}
		if unregisterCommand != nil {
			unregisterCommand()
		}
		if unregisterProvider != nil {
			unregisterProvider()
		}
	})

	opened, err := openRuntimeAPIFactorySession(fixture.baseURL, factoryDir)
	if err != nil {
		t.Fatalf("open shared runtime API Factory Session: %v", err)
	}
	sessionID := opened.Session.Id
	if sessionID == "" || sessionID == "~default" {
		cleanupErr := closeRuntimeAPIFactorySession(fixture.baseURL, sessionID)
		if cleanupErr != nil {
			t.Errorf("cleanup invalid shared runtime API Factory Session %q: %v", sessionID, cleanupErr)
		}
		t.Fatalf("shared runtime API Factory Session ID = %q, want unique explicit session", sessionID)
	}
	releaseSession, err := fixture.trackSession(sessionID)
	if err != nil {
		cleanupErr := closeRuntimeAPIFactorySession(fixture.baseURL, sessionID)
		t.Fatalf("track shared runtime API Factory Session %q: %v; cleanup error: %v", sessionID, err, cleanupErr)
	}
	server := &functionalAPIServer{
		shared:    fixture,
		sessionID: sessionID,
	}
	t.Cleanup(func() {
		// Close the live session before releasing its effect route. This lets
		// cancellation reach any in-flight controlled provider call.
		if err := closeRuntimeAPIFactorySession(fixture.baseURL, sessionID); err != nil {
			t.Errorf("close shared runtime API Factory Session %q: %v", sessionID, err)
			return
		}
		if err := releaseSession(); err != nil {
			t.Errorf("release shared runtime API Factory Session %q: %v", sessionID, err)
		}
	})
	return server
}

func (fixture *runtimeAPIPackageFixture) trackSession(id string) (func() error, error) {
	if fixture == nil {
		return func() error { return nil }, nil
	}
	return fixture.ledger.trackSession(id)
}

func (fixture *runtimeAPIPackageFixture) trackStream(stream *factoryEventHTTPStream) {
	if fixture == nil || stream == nil {
		return
	}
	stream.setCloseHook(fixture.ledger.trackStream())
}

func openRuntimeAPIFactorySession(baseURL, folderPath string) (factoryapi.OpenFactorySessionResponse, error) {
	payload, err := json.Marshal(factoryapi.OpenFactorySessionRequest{FolderPath: folderPath})
	if err != nil {
		return factoryapi.OpenFactorySessionResponse{}, fmt.Errorf("marshal open Factory Session request: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), runtimeAPISessionCleanupTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		strings.TrimSuffix(baseURL, "/")+"/factory-sessions",
		strings.NewReader(string(payload)),
	)
	if err != nil {
		return factoryapi.OpenFactorySessionResponse{}, fmt.Errorf("build open Factory Session request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return factoryapi.OpenFactorySessionResponse{}, fmt.Errorf("POST open Factory Session: %w", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return factoryapi.OpenFactorySessionResponse{}, fmt.Errorf("read open Factory Session response: %w", err)
	}
	if response.StatusCode != http.StatusOK {
		return factoryapi.OpenFactorySessionResponse{}, fmt.Errorf(
			"POST /factory-sessions status = %d: %s",
			response.StatusCode,
			strings.TrimSpace(string(body)),
		)
	}
	var opened factoryapi.OpenFactorySessionResponse
	if err := json.Unmarshal(body, &opened); err != nil {
		return factoryapi.OpenFactorySessionResponse{}, fmt.Errorf("decode open Factory Session response: %w", err)
	}
	if opened.Session == nil || strings.TrimSpace(opened.Session.Id) == "" {
		return factoryapi.OpenFactorySessionResponse{}, errors.New("open Factory Session response has no session ID")
	}
	return opened, nil
}

func closeRuntimeAPIFactorySession(baseURL, sessionID string) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return errors.New("close Factory Session requires a session ID")
	}
	if err := terminateRuntimeAPIFactorySession(baseURL, sessionID); err != nil {
		if errors.Is(err, errRuntimeAPIFactorySessionGone) {
			return nil
		}
		return err
	}
	if err := waitRuntimeAPIFactorySessionStopped(baseURL, sessionID); err != nil {
		return err
	}
	return deleteRuntimeAPIFactorySession(baseURL, sessionID)
}

var errRuntimeAPIFactorySessionGone = errors.New("Factory Session is already gone")

func terminateRuntimeAPIFactorySession(baseURL, sessionID string) error {
	status, body, err := runtimeAPIFactorySessionRequest(
		http.MethodPost,
		strings.TrimSuffix(baseURL, "/")+"/factory-sessions/"+url.PathEscape(sessionID)+"/terminate",
		[]byte("{}"),
	)
	if err != nil {
		return fmt.Errorf("terminate Factory Session %q: %w", sessionID, err)
	}
	if status >= http.StatusOK && status < http.StatusMultipleChoices {
		return nil
	}
	if status == http.StatusConflict && strings.Contains(string(body), `"outcome":"TERMINAL_SESSION"`) {
		return nil
	}
	if status == http.StatusNotFound {
		return errRuntimeAPIFactorySessionGone
	}
	return fmt.Errorf("terminate Factory Session %q status = %d: %s", sessionID, status, strings.TrimSpace(string(body)))
}

func waitRuntimeAPIFactorySessionStopped(baseURL, sessionID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), runtimeAPISessionCleanupTimeout)
	defer cancel()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	var lastStatus factoryapi.StatusResponse
	var lastErr error
	for {
		status, body, err := runtimeAPIFactorySessionRequestWithContext(
			ctx,
			http.MethodGet,
			strings.TrimSuffix(baseURL, "/")+"/factory-sessions/"+url.PathEscape(sessionID)+"/status",
			nil,
		)
		if err == nil && status == http.StatusOK {
			if decodeErr := json.Unmarshal(body, &lastStatus); decodeErr == nil {
				if lastStatus.RuntimeStatus == string(interfaces.RuntimeStatusIdle) ||
					lastStatus.RuntimeStatus == string(interfaces.RuntimeStatusFinished) {
					return nil
				}
				lastErr = nil
			} else {
				lastErr = fmt.Errorf("decode status: %w", decodeErr)
			}
		} else if err != nil {
			lastErr = err
		} else {
			lastErr = fmt.Errorf("status = %d: %s", status, strings.TrimSpace(string(body)))
		}

		// The public status endpoint is the lifecycle signal; polling it avoids
		// a fixed sleep while allowing the runtime to finish cancellation and
		// release its session-owned resources deterministically.
		select {
		case <-ctx.Done():
			return fmt.Errorf("wait for Factory Session %q to stop: last status=%#v error=%v: %w", sessionID, lastStatus, lastErr, ctx.Err())
		case <-ticker.C:
		}
	}
}

func deleteRuntimeAPIFactorySession(baseURL, sessionID string) error {
	status, body, err := runtimeAPIFactorySessionRequest(
		http.MethodDelete,
		strings.TrimSuffix(baseURL, "/")+"/factory-sessions/"+url.PathEscape(sessionID),
		nil,
	)
	if err != nil {
		return fmt.Errorf("delete Factory Session %q: %w", sessionID, err)
	}
	if status == http.StatusNoContent || status == http.StatusNotFound {
		return nil
	}
	return fmt.Errorf("delete Factory Session %q status = %d: %s", sessionID, status, strings.TrimSpace(string(body)))
}

func runtimeAPIFactorySessionRequest(method, endpoint string, body []byte) (int, []byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), runtimeAPISessionCleanupTimeout)
	defer cancel()
	return runtimeAPIFactorySessionRequestWithContext(ctx, method, endpoint, body)
}

func runtimeAPIFactorySessionRequestWithContext(ctx context.Context, method, endpoint string, body []byte) (int, []byte, error) {
	var reader io.Reader
	if body != nil {
		reader = strings.NewReader(string(body))
	}
	request, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return 0, nil, fmt.Errorf("build %s %s request: %w", method, endpoint, err)
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return 0, nil, err
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		return response.StatusCode, nil, fmt.Errorf("read %s %s response: %w", method, endpoint, err)
	}
	return response.StatusCode, responseBody, nil
}

func runtimeAPIProviderForScenario(t *testing.T, scenario runtimeAPIScenario) providers.Service {
	t.Helper()
	if scenario.providerRunner != nil {
		return newRuntimeAPICommandProvider(scenario.providerRunner)
	}
	switch provider := scenario.provider.(type) {
	case nil:
		return nil
	case providers.Service:
		return provider
	case interface {
		Infer(context.Context, workers.ProviderInferenceRequest) (workers.InferenceResponse, error)
	}:
		return support.ProviderServiceFromInference(provider)
	default:
		t.Fatalf("unsupported shared runtime API provider %T", scenario.provider)
		return nil
	}
}

func sharedRuntimeAPIFixture(t *testing.T) *runtimeAPIPackageFixture {
	t.Helper()
	runtimeAPIFixtureOnce.Do(func() {
		fixture, err := newRuntimeAPIPackageFixture()
		runtimeAPIFixtureMu.Lock()
		runtimeAPIFixtureVal = fixture
		runtimeAPIFixtureErr = err
		runtimeAPIFixtureMu.Unlock()
	})
	runtimeAPIFixtureMu.Lock()
	fixture := runtimeAPIFixtureVal
	err := runtimeAPIFixtureErr
	runtimeAPIFixtureMu.Unlock()
	if err != nil {
		t.Fatalf("construct shared runtime API fixture: %v", err)
	}
	return fixture
}

func newRuntimeAPIPackageFixture() (*runtimeAPIPackageFixture, error) {
	rootDir, err := os.MkdirTemp("", "infinite-you-runtime-api-")
	if err != nil {
		return nil, err
	}
	cleanupOnError := func(cause error) (*runtimeAPIPackageFixture, error) {
		return nil, errors.Join(cause, removeRuntimeAPIRoot(rootDir))
	}

	hostDir := filepath.Join(rootDir, "host")
	if err := writeRuntimeAPIFixtureFactory(hostDir, providerBackedModelTransportSmokeConfig()); err != nil {
		return cleanupOnError(err)
	}
	workerPath := filepath.Join(hostDir, "workers", "tts-worker", "AGENTS.md")
	if err := os.MkdirAll(filepath.Dir(workerPath), 0o755); err != nil {
		return cleanupOnError(fmt.Errorf("create fixture worker directory: %w", err))
	}
	if err := os.WriteFile(workerPath, []byte("---\ntype: MODEL_WORKER\nmodel: OMNIVOICE_Q4_K_M\nmodelProvider: CODEX\n---\nDo the work.\n"), 0o644); err != nil {
		return cleanupOnError(fmt.Errorf("write fixture worker configuration: %w", err))
	}
	homeDir := filepath.Join(rootDir, "home")
	if err := os.MkdirAll(homeDir, 0o755); err != nil {
		return cleanupOnError(fmt.Errorf("create fixture home: %w", err))
	}

	fixture := &runtimeAPIPackageFixture{
		rootDir:        rootDir,
		hostDir:        hostDir,
		ledger:         newRuntimeAPICleanupLedger(),
		providerRouter: newRuntimeAPIProviderRouter(),
		commandRouter:  newRuntimeAPICommandRouter("provider"),
		scriptRouter:   newRuntimeAPICommandRouter("script"),
	}
	api := support.NewProcessAPIServer()
	edges := serviceedges.Edges{
		APIServerStarter: platformhttpserver.Starter(func(ctx context.Context, request platformhttpserver.StartRequest) error {
			fixture.apiStarts.Add(1)
			return api.Start(ctx, request)
		}),
		ProviderOverride:          fixture.providerRouter,
		ProviderCommandRunner:     fixture.commandRouter,
		ScriptCommandRunner:       fixture.scriptRouter,
		FactorySessionIDGenerator: uuid.NewString,
	}
	process, err := support.BuildProcessWithContext(context.Background(), edges)
	if err != nil {
		return cleanupOnError(fmt.Errorf("build production process: %w", err))
	}
	fixture.process = process

	inputs := support.FakeInputs(context.Background(), []string{
		"you", "run", "--continuously", "--with-server", "--quiet", "--dir", hostDir, "--no-record",
	})
	inputs.Input.WorkingDirectory = hostDir
	inputs.Input.Env = runtimeAPIEnvironment(inputs.Input.Env, homeDir)
	fixture.processStarts.Add(1)
	fixture.command = startRuntimeAPIProcessCommand(process, inputs.Input)
	baseURL, err := api.WaitForBaseURL(runtimeAPIPackageFixtureTimeout)
	if err != nil {
		return nil, errors.Join(
			fmt.Errorf("wait for package API listener: %w", err),
			fixture.close(),
		)
	}
	fixture.baseURL = baseURL

	return fixture, nil
}

func writeRuntimeAPIFixtureFactory(dir string, cfg map[string]any) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create factory directory: %w", err)
	}
	if _, ok := cfg["name"]; !ok {
		cfg["name"] = "runtime-api-shared-host"
	}
	payload, err := json.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal factory configuration: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, interfaces.FactoryConfigFile), payload, 0o644); err != nil {
		return fmt.Errorf("write factory configuration: %w", err)
	}
	workstations, _ := cfg["workstations"].([]map[string]any)
	for _, workstation := range workstations {
		name, _ := workstation["name"].(string)
		if name == "" {
			continue
		}
		path := filepath.Join(dir, "workstations", name, "AGENTS.md")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return fmt.Errorf("create workstation configuration directory: %w", err)
		}
		if err := os.WriteFile(path, []byte("---\ntype: MODEL_WORKSTATION\n---\nDo the work.\n"), 0o644); err != nil {
			return fmt.Errorf("write workstation configuration: %w", err)
		}
	}
	return nil
}

func runtimeAPIEnvironment(environment []string, homeDir string) []string {
	result := make([]string, 0, len(environment)+2)
	for _, entry := range environment {
		name := strings.ToUpper(strings.SplitN(entry, "=", 2)[0])
		if name == "HOME" || name == "USERPROFILE" {
			continue
		}
		result = append(result, entry)
	}
	result = append(result, "HOME="+homeDir, "USERPROFILE="+homeDir)
	return result
}

func (fixture *runtimeAPIPackageFixture) close() error {
	if fixture == nil {
		return nil
	}
	fixture.closeOnce.Do(func() {
		var result error
		if fixture.command != nil {
			if err := fixture.command.stop(); err != nil {
				result = errors.Join(result, fmt.Errorf("stop Process.Execute: %w", err))
			}
		}
		if fixture.process != nil {
			ctx, cancel := context.WithTimeout(context.Background(), runtimeAPIPackageShutdownTimeout)
			if err := fixture.process.Close(ctx); err != nil {
				result = errors.Join(result, fmt.Errorf("close application process: %w", err))
			}
			cancel()
		}
		if fixture.baseURL != "" && fixture.apiStarts.Load() != 1 {
			result = errors.Join(result, fmt.Errorf("API listener starts = %d, want 1", fixture.apiStarts.Load()))
		}
		if err := runtimeAPIListenerClosed(fixture.baseURL, fixture.apiStarts.Load()); err != nil {
			result = errors.Join(result, err)
		}
		if fixture.baseURL != "" && fixture.processStarts.Load() != 1 {
			result = errors.Join(result, fmt.Errorf("Process.Execute starts = %d, want 1", fixture.processStarts.Load()))
		}
		if err := fixture.cleanupLedgerError(); err != nil {
			result = errors.Join(result, err)
		}
		if fixture.rootDir != "" {
			if err := removeRuntimeAPIRoot(fixture.rootDir); err != nil {
				result = errors.Join(result, err)
			}
		}
		fixture.closeErr = result
	})
	return fixture.closeErr
}

func (fixture *runtimeAPIPackageFixture) cleanupLedgerError() error {
	if fixture == nil {
		return nil
	}
	var result error
	if err := fixture.ledger.leakError(); err != nil {
		result = errors.Join(result, err)
	}
	if active, models := fixture.providerRouter.routeCounts(); active != 0 || models != 0 {
		result = errors.Join(result, fmt.Errorf("provider edge lanes remain: routes=%d model routes=%d", active, models))
	}
	if active := fixture.commandRouter.routeCount(); active != 0 {
		result = errors.Join(result, fmt.Errorf("provider command edge lanes remain: routes=%d", active))
	}
	if active := fixture.scriptRouter.routeCount(); active != 0 {
		result = errors.Join(result, fmt.Errorf("script command edge lanes remain: routes=%d", active))
	}
	return result
}

func runtimeAPIListenerClosed(baseURL string, starts int64) error {
	if strings.TrimSpace(baseURL) == "" {
		if starts == 0 {
			return nil
		}
		return fmt.Errorf("listener cleanup probe has no base URL after %d listener starts", starts)
	}
	ctx, cancel := context.WithTimeout(context.Background(), runtimeAPIPackageListenerProbeTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		strings.TrimSuffix(baseURL, "/")+"/status",
		nil,
	)
	if err != nil {
		return fmt.Errorf("build runtime API listener cleanup probe: %w", err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		if runtimeAPIConnectionWasRefused(err) {
			return nil
		}
		return fmt.Errorf("runtime API listener cleanup probe did not prove closure: %w", err)
	}
	defer response.Body.Close()
	return fmt.Errorf("runtime API listener remained reachable after shutdown with status %d", response.StatusCode)
}

func runtimeAPIConnectionWasRefused(err error) bool {
	var operationError *net.OpError
	if !errors.As(err, &operationError) || operationError.Op != "dial" {
		return false
	}
	return errors.Is(err, syscall.ECONNREFUSED) || errors.Is(err, runtimeAPIWindowsConnectionRefused)
}

func removeRuntimeAPIRoot(path string) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	if err := os.RemoveAll(path); err != nil {
		return fmt.Errorf("remove runtime API fixture root %s: %w", path, err)
	}
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("runtime API fixture root %s remains after cleanup", path)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("verify runtime API fixture root %s removal: %w", path, err)
	}
	return nil
}

type runtimeAPIProcessCommand struct {
	cancel context.CancelFunc
	done   chan struct{}

	mu          sync.Mutex
	terminalErr error
	stopOnce    sync.Once
	stopErr     error
}

func startRuntimeAPIProcessCommand(process support.ApplicationProcess, input root.Input) *runtimeAPIProcessCommand {
	parent := input.Context
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	input.Context = ctx
	command := &runtimeAPIProcessCommand{cancel: cancel, done: make(chan struct{})}
	go func() {
		err := process.Execute(input)
		command.mu.Lock()
		command.terminalErr = err
		command.mu.Unlock()
		close(command.done)
	}()
	return command
}

func (command *runtimeAPIProcessCommand) stop() error {
	if command == nil {
		return nil
	}
	command.stopOnce.Do(func() {
		command.cancel()
		select {
		case <-command.done:
			if err := command.terminalError(); err != nil && !errors.Is(err, context.Canceled) {
				command.stopErr = fmt.Errorf("Process.Execute returned during shutdown: %w", err)
			}
		case <-time.After(runtimeAPIPackageShutdownTimeout):
			command.stopErr = fmt.Errorf("timed out waiting %s for Process.Execute shutdown", runtimeAPIPackageShutdownTimeout)
		}
	})
	return command.stopErr
}

func (command *runtimeAPIProcessCommand) terminalError() error {
	if command == nil {
		return nil
	}
	command.mu.Lock()
	defer command.mu.Unlock()
	return command.terminalErr
}

type runtimeAPIProviderRouter struct {
	mu     sync.RWMutex
	routes map[string]runtimeAPIProviderRoute
	models map[string]string
}

type runtimeAPIProviderRoute struct {
	factoryDir string
	models     []string
	provider   providers.Service
	token      *struct{}
}

func newRuntimeAPIProviderRouter() *runtimeAPIProviderRouter {
	return &runtimeAPIProviderRouter{routes: make(map[string]runtimeAPIProviderRoute), models: make(map[string]string)}
}

func (router *runtimeAPIProviderRouter) register(factoryDir string, models []string, provider providers.Service) func() {
	key := runtimeAPINormalizeDir(factoryDir)
	route := runtimeAPIProviderRoute{
		factoryDir: key,
		models:     append([]string(nil), models...),
		provider:   provider,
		token:      &struct{}{},
	}
	router.mu.Lock()
	if previous, ok := router.routes[key]; ok {
		for _, model := range previous.models {
			modelKey := strings.ToLower(strings.TrimSpace(model))
			if router.models[modelKey] == key {
				delete(router.models, modelKey)
			}
		}
	}
	router.routes[key] = route
	for _, model := range models {
		router.models[strings.ToLower(strings.TrimSpace(model))] = key
	}
	router.mu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			router.mu.Lock()
			defer router.mu.Unlock()
			if current, ok := router.routes[key]; ok && current.token == route.token {
				delete(router.routes, key)
				for _, model := range route.models {
					modelKey := strings.ToLower(strings.TrimSpace(model))
					if router.models[modelKey] == key {
						delete(router.models, modelKey)
					}
				}
			}
		})
	}
}

func (router *runtimeAPIProviderRouter) routeCounts() (int, int) {
	if router == nil {
		return 0, 0
	}
	router.mu.RLock()
	defer router.mu.RUnlock()
	return len(router.routes), len(router.models)
}

func (router *runtimeAPIProviderRouter) providerFor(request providers.ExecuteRequest) providers.Service {
	factoryDir := runtimeAPINormalizeDir(request.FactoryDirectory)
	model := strings.ToLower(strings.TrimSpace(request.Model))
	router.mu.RLock()
	defer router.mu.RUnlock()
	if route, ok := router.routes[factoryDir]; ok {
		return route.provider
	}
	for _, route := range router.routes {
		if runtimeAPIDirContains(route.factoryDir, factoryDir) || runtimeAPIDirContains(route.factoryDir, runtimeAPINormalizeDir(request.WorkingDirectory)) {
			return route.provider
		}
	}
	if key := router.models[model]; key != "" {
		return router.routes[key].provider
	}
	return nil
}

func (router *runtimeAPIProviderRouter) Execute(ctx context.Context, request providers.ExecuteRequest) (providers.ExecuteResult, error) {
	provider := router.providerFor(request)
	if provider == nil {
		return providers.ExecuteResult{}, providers.ExecuteFailure{
			Kind:    providers.ExecuteFailureKindMisconfigured,
			Message: "no shared runtime API provider route for factory directory",
		}
	}
	return provider.Execute(ctx, request)
}

func (router *runtimeAPIProviderRouter) ListProviders(ctx context.Context, request providers.ListProvidersRequest) (providers.ListProvidersResult, error) {
	return (testutil.NativeProvider{}).ListProviders(ctx, request)
}

func (router *runtimeAPIProviderRouter) GetProvider(ctx context.Context, request providers.GetProviderRequest) (providers.GetProviderResult, error) {
	return (testutil.NativeProvider{}).GetProvider(ctx, request)
}

func (router *runtimeAPIProviderRouter) ResolveIdentity(ctx context.Context, request providers.ResolveIdentityRequest) (providers.ResolveIdentityResult, error) {
	return (testutil.NativeProvider{}).ResolveIdentity(ctx, request)
}

func (router *runtimeAPIProviderRouter) ResolveSelection(ctx context.Context, request providers.ResolveSelectionRequest) (providers.ResolveSelectionResult, error) {
	return (testutil.NativeProvider{}).ResolveSelection(ctx, request)
}

func (router *runtimeAPIProviderRouter) ValidatePrerequisites(ctx context.Context, request providers.ValidatePrerequisitesRequest) error {
	return (testutil.NativeProvider{}).ValidatePrerequisites(ctx, request)
}

func (router *runtimeAPIProviderRouter) ControlAttempt(ctx context.Context, request providers.ControlAttemptRequest) (providers.ControlAttemptResult, error) {
	return (testutil.NativeProvider{}).ControlAttempt(ctx, request)
}

func (router *runtimeAPIProviderRouter) Continue(ctx context.Context, request providers.ContinueRequest) (providers.ContinueResult, error) {
	return (testutil.NativeProvider{}).Continue(ctx, request)
}

func (router *runtimeAPIProviderRouter) ContinueReference(ctx context.Context, request providers.ContinueReferenceRequest) (providers.ContinueReferenceResult, error) {
	return (testutil.NativeProvider{}).ContinueReference(ctx, request)
}

type runtimeAPICommandRouter struct {
	name   string
	mu     sync.RWMutex
	routes map[string]runtimeAPICommandRoute
}

type runtimeAPICommandRoute struct {
	runner platformprocess.CommandRunner
	token  *struct{}
}

func newRuntimeAPICommandRouter(name string) *runtimeAPICommandRouter {
	return &runtimeAPICommandRouter{name: name, routes: make(map[string]runtimeAPICommandRoute)}
}

func (router *runtimeAPICommandRouter) register(factoryDir string, runner platformprocess.CommandRunner) func() {
	key := runtimeAPINormalizeDir(factoryDir)
	route := runtimeAPICommandRoute{runner: runner, token: &struct{}{}}
	router.mu.Lock()
	// The route map is scenario-scoped. Replacing a route for the same
	// directory is safe, while the token below prevents an older cleanup from
	// deleting the replacement.
	router.routes[key] = route
	router.mu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			router.mu.Lock()
			if current, ok := router.routes[key]; ok && current.token == route.token {
				delete(router.routes, key)
			}
			router.mu.Unlock()
		})
	}
}

func (router *runtimeAPICommandRouter) Run(ctx context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	key := runtimeAPINormalizeDir(request.WorkDir)
	router.mu.RLock()
	route, ok := router.routes[key]
	if !ok {
		for routeDir, candidate := range router.routes {
			if runtimeAPIDirContains(routeDir, key) {
				route = candidate
				ok = true
				break
			}
		}
	}
	router.mu.RUnlock()
	if !ok || route.runner == nil {
		return platformprocess.CommandResult{}, fmt.Errorf("no shared runtime API %s command route for %q", router.name, request.WorkDir)
	}
	return route.runner.Run(ctx, request)
}

func (router *runtimeAPICommandRouter) routeCount() int {
	if router == nil {
		return 0
	}
	router.mu.RLock()
	defer router.mu.RUnlock()
	return len(router.routes)
}

type runtimeAPICommandProvider struct {
	testutil.NativeProvider
	runner platformprocess.CommandRunner
}

func newRuntimeAPICommandProvider(runner platformprocess.CommandRunner) providers.Service {
	provider := &runtimeAPICommandProvider{runner: runner}
	provider.NativeProvider.ExecuteFunc = provider.Execute
	return provider
}

func (provider *runtimeAPICommandProvider) Execute(ctx context.Context, request providers.ExecuteRequest) (providers.ExecuteResult, error) {
	command := strings.TrimSpace(request.Command)
	if command == "" {
		command = request.Provider.CanonicalSessionProvider()
	}
	commandRequest := platformprocess.CommandRequest{
		Command:                  command,
		Args:                     append([]string(nil), request.Args...),
		Stdin:                    []byte(request.UserMessage),
		Env:                      append([]string(nil), request.ProcessEnvironment...),
		WorkDir:                  request.WorkingDirectory,
		ExecutionLogger:          request.ExecutionLogger,
		ProcessLifecycleObserver: request.ProcessLifecycleObserver,
	}
	result, err := provider.runner.Run(ctx, commandRequest)
	if err != nil {
		return providers.ExecuteResult{}, err
	}
	if failure := runtimeAPICommandFailure(result); failure != nil {
		return providers.ExecuteResult{}, failure
	}
	return providers.ExecuteResult{Content: runtimeAPICommandContent(command, result.Stdout)}, nil
}

func runtimeAPICommandFailure(result platformprocess.CommandResult) error {
	if result.ExitCode == 0 && len(result.Stderr) == 0 {
		return nil
	}
	message := strings.TrimSpace(string(result.Stderr))
	lower := strings.ToLower(message)
	kind := providers.ExecuteFailureKindUnknown
	switch {
	case strings.Contains(lower, "rate_limit"), strings.Contains(lower, "429"), strings.Contains(lower, "thrott"):
		kind = providers.ExecuteFailureKindThrottled
	case strings.Contains(lower, "authentication"), strings.Contains(lower, "401"):
		kind = providers.ExecuteFailureKindAuthentication
	case strings.Contains(lower, "invalid"):
		kind = providers.ExecuteFailureKindInvalidRequest
	}
	if message == "" {
		message = "provider command failed"
	}
	return providers.ExecuteFailure{Kind: kind, Message: message}
}

func runtimeAPICommandContent(command string, stdout []byte) string {
	trimmed := strings.TrimSpace(string(stdout))
	if trimmed == "" {
		return ""
	}
	scanner := bufio.NewScanner(strings.NewReader(trimmed))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var record map[string]any
		if json.Unmarshal([]byte(line), &record) != nil {
			continue
		}
		typeName, _ := record["type"].(string)
		switch strings.ToLower(strings.TrimSpace(command)) {
		case "codex":
			if typeName != "item.completed" {
				continue
			}
			item, _ := record["item"].(map[string]any)
			text, _ := item["text"].(string)
			if text != "" {
				return text
			}
		case "claude":
			if typeName != "result" {
				continue
			}
			text, _ := record["result"].(string)
			if text != "" {
				return text
			}
		}
	}
	return trimmed
}

func runtimeAPINormalizeDir(path string) string {
	if strings.TrimSpace(path) == "" {
		return ""
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	return filepath.Clean(abs)
}

func runtimeAPIDirContains(parent, child string) bool {
	parent = runtimeAPINormalizeDir(parent)
	child = runtimeAPINormalizeDir(child)
	if parent == "" || child == "" {
		return false
	}
	relative, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	return relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(os.PathSeparator)))
}

var _ providers.Service = (*runtimeAPIProviderRouter)(nil)
var _ platformprocess.CommandRunner = (*runtimeAPICommandRouter)(nil)
var _ providers.Service = (*runtimeAPICommandProvider)(nil)

func TestRuntimeAPIPackageCleanupLedgerAndEdgeLeasesAreIdempotent(t *testing.T) {
	fixture := &runtimeAPIPackageFixture{
		ledger:         newRuntimeAPICleanupLedger(),
		providerRouter: newRuntimeAPIProviderRouter(),
		commandRouter:  newRuntimeAPICommandRouter("provider"),
		scriptRouter:   newRuntimeAPICommandRouter("script"),
	}

	releaseSession, err := fixture.trackSession("session-ledger-test")
	if err != nil {
		t.Fatalf("track session: %v", err)
	}
	stream := &factoryEventHTTPStream{}
	fixture.trackStream(stream)

	providerUnregister := fixture.providerRouter.register(t.TempDir(), []string{"model-ledger-test"}, testutil.NativeProvider{})
	commandUnregister := fixture.commandRouter.register(t.TempDir(), support.NewRecordingCommandRunner("provider"))
	scriptUnregister := fixture.scriptRouter.register(t.TempDir(), support.NewRecordingCommandRunner("script"))

	if err := releaseSession(); err != nil {
		t.Fatalf("release session: %v", err)
	}
	if err := releaseSession(); err != nil {
		t.Fatalf("repeated session release: %v", err)
	}
	stream.notifyClosed()
	stream.notifyClosed()
	providerUnregister()
	providerUnregister()
	commandUnregister()
	commandUnregister()
	scriptUnregister()
	scriptUnregister()

	if err := fixture.cleanupLedgerError(); err != nil {
		t.Fatalf("cleanup ledger after idempotent releases: %v", err)
	}

	t.Run("active resources fail closed", func(t *testing.T) {
		fixture := &runtimeAPIPackageFixture{
			ledger:         newRuntimeAPICleanupLedger(),
			providerRouter: newRuntimeAPIProviderRouter(),
			commandRouter:  newRuntimeAPICommandRouter("provider"),
			scriptRouter:   newRuntimeAPICommandRouter("script"),
		}
		releaseSession, err := fixture.trackSession("session-leak-test")
		if err != nil {
			t.Fatalf("track leaked session: %v", err)
		}
		stream := &factoryEventHTTPStream{}
		fixture.trackStream(stream)
		unregister := fixture.providerRouter.register(t.TempDir(), nil, testutil.NativeProvider{})
		if err := fixture.cleanupLedgerError(); err == nil {
			t.Fatal("cleanup ledger error = nil, want active resource failure")
		}

		if err := releaseSession(); err != nil {
			t.Fatalf("release leaked session: %v", err)
		}
		stream.notifyClosed()
		unregister()
		if err := fixture.cleanupLedgerError(); err != nil {
			t.Fatalf("cleanup ledger after releasing active resources: %v", err)
		}
	})
}

// C06-ISOLATED CASE-43: cleanup itself owns process, listener, root, and
// tracked-lane teardown; injected lifecycle failures must remain visible while
// every independent cleanup action still runs.
func TestRuntimeAPIPackageFixtureCleanupIsIdempotentAndPreservesFailures(t *testing.T) {
	t.Run("normal cleanup probes listener and removes root", func(t *testing.T) {
		fixture, listener, process := newRuntimeAPICleanupTestFixture(t, nil, nil)
		listener.Close()

		if err := fixture.close(); err != nil {
			t.Fatalf("fixture close: %v", err)
		}
		if err := fixture.close(); err != nil {
			t.Fatalf("repeated fixture close: %v", err)
		}
		if got := process.closeCalls.Load(); got != 1 {
			t.Fatalf("process close calls = %d, want 1", got)
		}
		assertRuntimeAPITestRootRemoved(t, fixture.rootDir)
	})

	t.Run("injected execute and close failures remain visible", func(t *testing.T) {
		executeErr := errors.New("injected Process.Execute failure")
		closeErr := errors.New("injected application process close failure")
		fixture, listener, process := newRuntimeAPICleanupTestFixture(t, executeErr, closeErr)
		listener.Close()

		firstErr := fixture.close()
		if !errors.Is(firstErr, executeErr) {
			t.Fatalf("fixture close error = %v, want Process.Execute cause", firstErr)
		}
		if !errors.Is(firstErr, closeErr) {
			t.Fatalf("fixture close error = %v, want process Close cause", firstErr)
		}
		secondErr := fixture.close()
		if !errors.Is(secondErr, executeErr) || !errors.Is(secondErr, closeErr) {
			t.Fatalf("repeated fixture close error = %v, want both original causes", secondErr)
		}
		if got := process.closeCalls.Load(); got != 1 {
			t.Fatalf("process close calls = %d, want 1 after repeated cleanup", got)
		}
		assertRuntimeAPITestRootRemoved(t, fixture.rootDir)
	})

	t.Run("reachable listener fails the cleanup probe", func(t *testing.T) {
		listener := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
		defer listener.Close()

		err := runtimeAPIListenerClosed(listener.URL, 1)
		if err == nil || !strings.Contains(err.Error(), "remained reachable") {
			t.Fatalf("reachable listener probe error = %v, want reachability failure", err)
		}
	})
}

type runtimeAPICleanupTestProcess struct {
	executeErr error
	closeErr   error
	closeCalls atomic.Int64
}

func (process *runtimeAPICleanupTestProcess) Execute(root.Input) error {
	return process.executeErr
}

func (process *runtimeAPICleanupTestProcess) Close(context.Context) error {
	process.closeCalls.Add(1)
	return process.closeErr
}

func (*runtimeAPICleanupTestProcess) ACPServer() support.ACPServer {
	return nil
}

func (*runtimeAPICleanupTestProcess) ProviderRegistry() support.ProviderRegistry {
	return nil
}

func (*runtimeAPICleanupTestProcess) WorkerRecordingReader() recordings.WorkerRecordingReader {
	return nil
}

func newRuntimeAPICleanupTestFixture(
	t *testing.T,
	executeErr, closeErr error,
) (*runtimeAPIPackageFixture, *httptest.Server, *runtimeAPICleanupTestProcess) {
	t.Helper()
	rootDir := filepath.Join(t.TempDir(), "owned-runtime-api-root")
	if err := os.MkdirAll(rootDir, 0o755); err != nil {
		t.Fatalf("create cleanup test root: %v", err)
	}
	listener := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	process := &runtimeAPICleanupTestProcess{executeErr: executeErr, closeErr: closeErr}
	fixture := &runtimeAPIPackageFixture{
		rootDir:        rootDir,
		baseURL:        listener.URL,
		process:        process,
		ledger:         newRuntimeAPICleanupLedger(),
		providerRouter: newRuntimeAPIProviderRouter(),
		commandRouter:  newRuntimeAPICommandRouter("provider"),
		scriptRouter:   newRuntimeAPICommandRouter("script"),
	}
	fixture.apiStarts.Store(1)
	fixture.processStarts.Store(1)
	inputs := support.FakeInputs(context.Background(), []string{"you", "run"})
	fixture.command = startRuntimeAPIProcessCommand(process, inputs.Input)
	return fixture, listener, process
}

func assertRuntimeAPITestRootRemoved(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("cleanup test root stat error = %v, want path removed", err)
	}
}
