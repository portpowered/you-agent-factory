package runtimeapifixture

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
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
	runtimeAPIFixtureVal  *PackageFixture
	runtimeAPIFixtureErr  error
)

type PackageFixture struct {
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

type Scenario struct {
	Provider       any
	ProviderRunner platformprocess.CommandRunner
	ScriptRunner   platformprocess.CommandRunner
	Models         []string
}

type SessionHandle struct {
	fixture   *PackageFixture
	sessionID string
}

func (handle *SessionHandle) Fixture() *PackageFixture {
	if handle == nil {
		return nil
	}
	return handle.fixture
}

func (handle *SessionHandle) SessionID() string {
	if handle == nil {
		return ""
	}
	return handle.sessionID
}

// StartSharedFunctionalServer opens a unique Factory Session on the package
// fixture and registers only the scenario's external-effect lanes.
func StartSharedFunctionalServer(t *testing.T, factoryDir string, scenario Scenario) *SessionHandle {
	t.Helper()

	fixture := sharedPackageFixture(t)
	provider := providerForScenario(t, scenario)
	if provider == nil && scenario.ProviderRunner == nil {
		// A mock-worker scenario does not call Providers, but registering a
		// fail-closed route keeps an accidental real invocation from escaping
		// the controlled package fixture.
		provider = testutil.NativeProvider{}
	}

	var unregisterProvider func()
	if provider != nil {
		unregisterProvider = fixture.providerRouter.register(factoryDir, scenario.Models, provider)
	}
	var unregisterCommand func()
	if scenario.ProviderRunner != nil {
		unregisterCommand = fixture.commandRouter.register(factoryDir, scenario.ProviderRunner)
	}
	var unregisterScript func()
	if scenario.ScriptRunner != nil {
		unregisterScript = fixture.scriptRouter.register(factoryDir, scenario.ScriptRunner)
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
	releaseSession, err := fixture.TrackSession(sessionID)
	if err != nil {
		cleanupErr := closeRuntimeAPIFactorySession(fixture.baseURL, sessionID)
		t.Fatalf("track shared runtime API Factory Session %q: %v; cleanup error: %v", sessionID, err, cleanupErr)
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
	return &SessionHandle{fixture: fixture, sessionID: sessionID}
}

func (fixture *PackageFixture) TrackSession(id string) (func() error, error) {
	if fixture == nil {
		return func() error { return nil }, nil
	}
	return fixture.ledger.trackSession(id)
}

func (fixture *PackageFixture) TrackStream() func() {
	if fixture == nil {
		return func() {}
	}
	return fixture.ledger.trackStream()
}

func (fixture *PackageFixture) BaseURL() string {
	if fixture == nil {
		return ""
	}
	return fixture.baseURL
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

func providerForScenario(t *testing.T, scenario Scenario) providers.Service {
	t.Helper()
	if scenario.ProviderRunner != nil {
		return newRuntimeAPICommandProvider(scenario.ProviderRunner)
	}
	switch provider := scenario.Provider.(type) {
	case nil:
		return nil
	case providers.Service:
		return provider
	case interface {
		Infer(context.Context, workers.ProviderInferenceRequest) (workers.InferenceResponse, error)
	}:
		return support.ProviderServiceFromInference(provider)
	default:
		t.Fatalf("unsupported shared runtime API provider %T", scenario.Provider)
		return nil
	}
}

func sharedPackageFixture(t *testing.T) *PackageFixture {
	t.Helper()
	runtimeAPIFixtureOnce.Do(func() {
		fixture, err := newPackageFixture()
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

// CloseSharedFixture releases the package-scoped Process.Execute fixture after
// all tests have released their explicit Factory Sessions and effect lanes.
func CloseSharedFixture() error {
	runtimeAPIFixtureMu.Lock()
	fixture := runtimeAPIFixtureVal
	runtimeAPIFixtureMu.Unlock()
	if fixture == nil {
		return nil
	}
	return fixture.Close()
}

func newPackageFixture() (*PackageFixture, error) {
	rootDir, err := os.MkdirTemp("", "infinite-you-runtime-api-")
	if err != nil {
		return nil, err
	}
	cleanupOnError := func(cause error) (*PackageFixture, error) {
		return nil, errors.Join(cause, removeRuntimeAPIRoot(rootDir))
	}

	hostDir := filepath.Join(rootDir, "host")
	if err := writeRuntimeAPIFixtureFactory(hostDir, sharedHostFactoryConfig()); err != nil {
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

	fixture := &PackageFixture{
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
			fixture.Close(),
		)
	}
	fixture.baseURL = baseURL

	return fixture, nil
}

func sharedHostFactoryConfig() map[string]any {
	return map[string]any{
		"name": "model-transport-smoke",
		"workTypes": []map[string]any{{
			"name": "task",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]any{{
			"name":          "tts-worker",
			"type":          interfaces.WorkerTypeModel,
			"model":         "OMNIVOICE_Q4_K_M",
			"modelProvider": "CODEX",
			"modelLocality": interfaces.ModelLocalityCloud,
			"operations": []map[string]any{{
				"name": "TTS",
				"inputs": []map[string]any{{
					"name":         "text",
					"contentTypes": []string{interfaces.ModelOperationContentTypeText},
					"required":     true,
				}},
				"outputs": []map[string]any{{
					"name":         "audio",
					"contentTypes": []string{interfaces.ModelOperationContentTypeAudio},
				}},
			}},
		}},
	}
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

func (fixture *PackageFixture) Close() error {
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

func (fixture *PackageFixture) cleanupLedgerError() error {
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

var _ platformprocess.CommandRunner = (*runtimeAPICommandRouter)(nil)
