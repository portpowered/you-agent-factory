package root_composition_test

import (
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
	"sync/atomic"
	"testing"
	"time"

	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	runcli "github.com/portpowered/infinite-you/pkg/transports/cli/run"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	sharedModelsProcessTimeout = 30 * time.Second
	sharedModelsHTTPTimeout    = 5 * time.Second

	sharedModelsCatalogSessionFactoryName = "models-shared-catalog-session"
	sharedModelsUnknownSessionFactoryName = "models-shared-unknown-session"
)

// sharedModelsFixture owns the one root-built process and API transport for
// the identical rich-catalog scenarios. Each scenario still opens and closes
// its own explicit Factory Session in a distinct test-owned Factory directory;
// the session-scoped current-Factory route proves those two routed inputs stay
// distinct while the scenarios overlap. Only immutable process wiring and the
// read-only catalog fixture are shared. The public Models routes are bound to
// the process runtime and have no session selector, so they intentionally remain
// the behavior witness for the original process-level model contract.
type sharedModelsFixture struct {
	rootDir  string
	homeDir  string
	cacheDir string
	baseURL  string
	env      []string

	process    support.ApplicationProcess
	command    *sharedModelsProcessCommand
	apiStarted atomic.Bool
	apiStopped chan struct{}

	scenarioMu    sync.Mutex
	sessionIDs    map[string]string
	activeSession map[string]string
	sessionNames  map[string]string
	sessionPaths  map[string]string
	sessionDirs   []string
	rootBuilds    atomic.Int64
	apiStarts     atomic.Int64
	opens         atomic.Int64
	closes        atomic.Int64
}

type sharedModelsProcessCommand struct {
	cancel context.CancelFunc
	done   chan error
}

// sharedModelsSessionBarrier makes the race target prove that both explicit
// Factory Sessions are live before either scenario performs its Models
// observations. It has no timeout because the test context is the lifecycle
// signal and the package test command owns the outer test deadline.
type sharedModelsSessionBarrier struct {
	want         int32
	arrivals     atomic.Int32
	completions  atomic.Int32
	release      chan struct{}
	complete     chan struct{}
	releaseOnce  sync.Once
	completeOnce sync.Once
}

func newSharedModelsSessionBarrier(want int32) *sharedModelsSessionBarrier {
	return &sharedModelsSessionBarrier{
		want:     want,
		release:  make(chan struct{}),
		complete: make(chan struct{}),
	}
}

func (barrier *sharedModelsSessionBarrier) wait(t *testing.T) {
	t.Helper()
	if barrier == nil {
		return
	}
	if barrier.arrivals.Add(1) == barrier.want {
		barrier.releaseOnce.Do(func() { close(barrier.release) })
	}
	select {
	case <-barrier.release:
	case <-t.Context().Done():
		t.Fatalf("shared Models session barrier canceled before both scenarios overlapped")
	}
}

func (barrier *sharedModelsSessionBarrier) completeScenario() {
	if barrier == nil {
		return
	}
	if barrier.completions.Add(1) == barrier.want {
		barrier.completeOnce.Do(func() { close(barrier.complete) })
	}
}

func (barrier *sharedModelsSessionBarrier) waitForScenarios(t *testing.T) {
	t.Helper()
	if barrier == nil {
		return
	}
	select {
	case <-barrier.complete:
	case <-t.Context().Done():
		t.Fatalf("shared Models scenarios did not complete while sessions were live")
	}
}

var sharedModelsFixtureState struct {
	sync.Once
	fixture *sharedModelsFixture
	err     error
}

// TestModelsSharedProcessEligibleScenarios is the explicit race-test target
// for the shared mutable session ledger. The two subtests use the same rich
// catalog definition and process while retaining their original HTTP and CLI
// witnesses.
func TestModelsSharedProcessEligibleScenarios(t *testing.T) {
	fixture := ensureSharedModelsFixture(t)
	barrier := newSharedModelsSessionBarrier(2)
	scenarios := []struct {
		name string
		run  func(*testing.T)
	}{
		{
			name: "catalog discovery projects worker capabilities and Factory precedence",
			run: func(t *testing.T) {
				runModelsCatalogDiscoveryProjectsWorkerCapabilitiesAndFactoryPrecedenceWithBarrier(
					t, barrier, sharedModelsCatalogSessionFactoryName,
				)
			},
		},
		{
			name: "unknown detail keeps the public not-found contract",
			run: func(t *testing.T) {
				runModelsCatalogDiscoveryMapsUnknownDetailThroughHTTPWithBarrier(
					t, barrier, sharedModelsUnknownSessionFactoryName,
				)
			},
		},
	}
	t.Cleanup(func() {
		if got := fixture.rootBuilds.Load(); got != 1 {
			t.Errorf("shared Models root builds = %d, want exactly one", got)
		}
		if got := fixture.apiStarts.Load(); got != 1 {
			t.Errorf("shared Models API starts = %d, want exactly one", got)
		}
		if got := sharedModelsEnvironmentValue(fixture.env, runcli.ModelCacheDirEnvironment); got != fixture.cacheDir {
			t.Errorf("shared Models cache selector = %q, want fixture-owned cache %q", got, fixture.cacheDir)
		}
		if _, err := os.Stat(filepath.Join(fixture.cacheDir, "OMNIVOICE_Q4_K_M", ".managed-cache.json")); err != nil {
			t.Errorf("shared Models fixture cache is not available at %q: %v", fixture.cacheDir, err)
		}
		if got, want := fixture.opens.Load(), fixture.closes.Load(); got != want {
			t.Errorf("shared Models Factory Session opens = %d, closes = %d", got, want)
		}
		fixture.scenarioMu.Lock()
		uniqueSessions := len(fixture.sessionIDs)
		fixture.scenarioMu.Unlock()
		if got := uniqueSessions; got != int(fixture.opens.Load()) {
			t.Errorf("shared Models unique Factory Session IDs = %d, want %d", got, fixture.opens.Load())
		}
	})
	for _, scenario := range scenarios {
		scenario := scenario
		t.Run(scenario.name, func(t *testing.T) {
			t.Parallel()
			scenario.run(t)
		})
	}
}

func ensureSharedModelsFixture(t *testing.T) *sharedModelsFixture {
	t.Helper()
	sharedModelsFixtureState.Do(func() {
		sharedModelsFixtureState.fixture, sharedModelsFixtureState.err = newSharedModelsFixture(t)
	})
	if sharedModelsFixtureState.err != nil {
		t.Fatalf("initialize shared Models fixture: %v", sharedModelsFixtureState.err)
	}
	return sharedModelsFixtureState.fixture
}

func newSharedModelsFixture(t *testing.T) (_ *sharedModelsFixture, err error) {
	t.Helper()
	rootDir, err := os.MkdirTemp("", "c06-models-shared-")
	if err != nil {
		return nil, fmt.Errorf("create shared Models package root: %w", err)
	}
	fixture := &sharedModelsFixture{
		rootDir:       rootDir,
		sessionIDs:    make(map[string]string),
		activeSession: make(map[string]string),
		sessionNames:  make(map[string]string),
		sessionPaths:  make(map[string]string),
		apiStopped:    make(chan struct{}),
	}
	defer func() {
		if err != nil {
			if cleanupErr := fixture.close(); cleanupErr != nil {
				err = errors.Join(err, cleanupErr)
			}
		}
	}()

	if err := writeSharedModelsFactory(rootDir, richCatalogFactoryConfig()); err != nil {
		return nil, err
	}
	c06Ledger.factoryRoots.Add(1)

	homeDir, err := os.MkdirTemp("", "c06-models-shared-home-")
	if err != nil {
		return nil, fmt.Errorf("create shared Models home: %w", err)
	}
	fixture.homeDir = homeDir
	cacheDir := filepath.Join(homeDir, ".agent-factory", "models")
	writeCachedOmniVoiceAssets(t, cacheDir)
	fixture.cacheDir = cacheDir

	api := support.NewProcessAPIServer()
	var apiStopOnce sync.Once
	process, buildErr := support.BuildProcessWithContext(context.Background(), serviceedges.Edges{
		APIServerStarter: func(ctx context.Context, request platformhttpserver.StartRequest) error {
			fixture.apiStarted.Store(true)
			fixture.apiStarts.Add(1)
			c06Ledger.apiServers.Add(1)
			err := api.Start(ctx, request)
			apiStopOnce.Do(func() { close(fixture.apiStopped) })
			return err
		},
	})
	if buildErr != nil {
		return nil, fmt.Errorf("BuildProcess: %w", buildErr)
	}
	fixture.process = process
	fixture.rootBuilds.Add(1)
	c06Ledger.rootBuilds.Add(1)

	inputs := support.FakeInputs(context.Background(), []string{
		"you", "run", "--dir", rootDir, "--continuously", "--with-server", "--quiet", "--no-record",
	})
	inputs.WorkingDirectory = rootDir
	inputs.Env = sharedModelsEnvironment(inputs.Env, homeDir, cacheDir)
	fixture.env = append([]string(nil), inputs.Env...)
	fixture.command = startSharedModelsProcess(process, inputs)

	baseURL, waitErr := api.WaitForBaseURL(sharedModelsProcessTimeout)
	if waitErr != nil {
		return nil, fmt.Errorf("wait for shared Models API: %w", waitErr)
	}
	fixture.baseURL = baseURL
	// The runtime status is the assembled process's public readiness signal; use
	// the shared observation helper so the first Models request cannot race
	// asynchronous runtime projection startup.
	support.WaitForStatus(t, baseURL, sharedModelsProcessTimeout, func(status factoryapi.StatusResponse) bool {
		return status.RuntimeStatus != ""
	})
	return fixture, nil
}

func writeSharedModelsFactory(rootDir string, config map[string]any) error {
	if _, ok := config["name"]; !ok {
		config["name"] = "models-shared"
	}
	payload, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("marshal shared Models factory: %w", err)
	}
	if err := os.WriteFile(filepath.Join(rootDir, interfaces.FactoryConfigFile), payload, 0o644); err != nil {
		return fmt.Errorf("write shared Models factory: %w", err)
	}
	return nil
}

func sharedModelsEnvironment(environment []string, homeDir, cacheDir string) []string {
	result := replaceEnvironmentValue(environment, "HOME", homeDir)
	result = replaceEnvironmentValue(result, "USERPROFILE", homeDir)
	result = replaceEnvironmentValue(result, "XDG_CACHE_HOME", homeDir)
	return replaceEnvironmentValue(result, runcli.ModelCacheDirEnvironment, cacheDir)
}

func sharedModelsEnvironmentValue(environment []string, name string) string {
	for _, entry := range environment {
		key, value, found := strings.Cut(entry, "=")
		if found && strings.EqualFold(key, name) {
			return value
		}
	}
	return ""
}

func (fixture *sharedModelsFixture) createSessionFactory(t *testing.T, name string) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "c06-models-shared-session-")
	if err != nil {
		t.Fatalf("create shared Models session Factory %q: %v", name, err)
	}
	fixture.scenarioMu.Lock()
	fixture.sessionDirs = append(fixture.sessionDirs, dir)
	fixture.scenarioMu.Unlock()
	c06Ledger.factoryRoots.Add(1)
	config := richCatalogFactoryConfig()
	config["name"] = name
	if err := writeSharedModelsFactory(dir, config); err != nil {
		t.Fatalf("write shared Models session Factory %q: %v", name, err)
	}
	return dir
}

func startSharedModelsProcess(
	process support.ApplicationProcess,
	inputs *support.CapturedInputs,
) *sharedModelsProcessCommand {
	parent := inputs.Input.Context
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	input := inputs.Input
	input.Context = ctx
	command := &sharedModelsProcessCommand{cancel: cancel, done: make(chan error, 1)}
	go func() {
		command.done <- process.Execute(input)
	}()
	return command
}

func (command *sharedModelsProcessCommand) stop(ctx context.Context) error {
	if command == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	command.cancel()
	select {
	case err := <-command.done:
		if err != nil && !errors.Is(err, context.Canceled) {
			return fmt.Errorf("stop shared Models process command: %w", err)
		}
		return nil
	case <-ctx.Done():
		return fmt.Errorf("timed out waiting for shared Models process command shutdown: %w", ctx.Err())
	}
}

func (fixture *sharedModelsFixture) withSession(
	t *testing.T,
	label string,
	factoryName string,
	observe func(string, string),
) {
	t.Helper()

	factoryDir := fixture.createSessionFactory(t, factoryName)
	opened := support.OpenFactorySessionAt(t, fixture.baseURL, factoryDir)
	sessionID := opened.Session.Id
	if sessionID == factorysessions.DefaultSessionID {
		t.Fatalf("shared Models scenario %q opened the default Factory Session", label)
	}
	fixture.opens.Add(1)
	c06Ledger.sharedSessionOpens.Add(1)
	fixture.scenarioMu.Lock()
	previous, exists := fixture.sessionIDs[sessionID]
	if !exists {
		fixture.sessionIDs[sessionID] = label
		fixture.activeSession[sessionID] = label
		fixture.sessionNames[sessionID] = factoryName
		fixture.sessionPaths[sessionID] = factoryDir
	}
	fixture.scenarioMu.Unlock()
	if exists {
		t.Fatalf("shared Models Factory Session ID %q reused by %q and %q", sessionID, previous, label)
	}
	// Register bookkeeping cleanup separately so it still runs if a public
	// teardown assertion calls t.Fatal while the session is being closed.
	defer func() {
		fixture.scenarioMu.Lock()
		delete(fixture.activeSession, sessionID)
		fixture.scenarioMu.Unlock()
		fixture.closes.Add(1)
		c06Ledger.sharedSessionCloses.Add(1)
	}()
	defer func() {
		support.CloseFactorySessionAt(t, fixture.baseURL, sessionID)
		assertSharedModelsSessionDeleted(t, fixture.baseURL, sessionID)
		defaultSession := getSharedModelsSession(t, fixture.baseURL, factorysessions.DefaultSessionID)
		if !defaultSession.IsDefault || strings.TrimSpace(defaultSession.Id) == "" {
			t.Fatalf("shared Models default Factory Session after %q = %#v", label, defaultSession)
		}
	}()

	observe(sessionID, factoryDir)
}

func (fixture *sharedModelsFixture) activeOtherSession(currentID string) (sessionID, factoryName, factoryDir string, ok bool) {
	fixture.scenarioMu.Lock()
	defer fixture.scenarioMu.Unlock()
	for candidateID := range fixture.activeSession {
		if candidateID == currentID {
			continue
		}
		return candidateID, fixture.sessionNames[candidateID], fixture.sessionPaths[candidateID], true
	}
	return "", "", "", false
}

func assertSharedModelsSessionRoute(
	t testing.TB,
	baseURL, sessionID, expectedFactoryDir string,
) {
	t.Helper()
	session := getSharedModelsSession(t, baseURL, sessionID)
	if session.Id != sessionID || session.IsDefault {
		t.Fatalf("shared Models Factory Session identity = %#v, want non-default %q", session, sessionID)
	}
	if session.FolderPath != expectedFactoryDir {
		t.Fatalf("shared Models Factory Session %q folder path = %q, want %q", sessionID, session.FolderPath, expectedFactoryDir)
	}
	factory := support.GetJSON[factoryapi.Factory](
		t,
		strings.TrimSuffix(baseURL, "/")+"/factory-sessions/"+url.PathEscape(sessionID)+"/factory",
	)
	if factory.Name != factoryapi.FactoryName(factorysessions.CurrentFactoryName) {
		t.Fatalf(
			"shared Models Factory Session %q current Factory name=%q directory=%q source=%q metadata=%v, want current-factory name %q",
			sessionID, factory.Name, sharedModelsStringPointerValue(factory.FactoryDirectory), sharedModelsStringPointerValue(factory.SourceDirectory), factory.Metadata, factorysessions.CurrentFactoryName,
		)
	}
	if sharedModelsStringPointerValue(factory.FactoryDirectory) != expectedFactoryDir ||
		sharedModelsStringPointerValue(factory.SourceDirectory) != expectedFactoryDir {
		t.Fatalf(
			"shared Models Factory Session %q current Factory directories = (%q, %q), want session Factory directory %q",
			sessionID, sharedModelsStringPointerValue(factory.FactoryDirectory), sharedModelsStringPointerValue(factory.SourceDirectory), expectedFactoryDir,
		)
	}
}

func sharedModelsStringPointerValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func assertSharedModelsSessionRouteIsolation(
	t testing.TB,
	fixture *sharedModelsFixture,
	sessionID, expectedFactoryDir string,
) {
	t.Helper()
	fixture.scenarioMu.Lock()
	ownName, ownNameOK := fixture.sessionNames[sessionID]
	registeredFactoryDir, pathOK := fixture.sessionPaths[sessionID]
	fixture.scenarioMu.Unlock()
	if !ownNameOK || !pathOK {
		t.Fatalf("shared Models session %q has no registered Factory route", sessionID)
	}
	if registeredFactoryDir != expectedFactoryDir {
		t.Fatalf("shared Models session %q Factory directory = %q, want %q", sessionID, registeredFactoryDir, expectedFactoryDir)
	}
	assertSharedModelsSessionRoute(t, fixture.baseURL, sessionID, expectedFactoryDir)

	otherID, otherName, otherDir, ok := fixture.activeOtherSession(sessionID)
	if !ok {
		t.Fatalf("shared Models session %q has no overlapping peer session", sessionID)
	}
	if otherName == ownName || otherDir == expectedFactoryDir {
		t.Fatalf(
			"shared Models session routes are not unique: own=(%q,%q,%q), other=(%q,%q,%q)",
			sessionID, ownName, expectedFactoryDir, otherID, otherName, otherDir,
		)
	}
	assertSharedModelsSessionRoute(t, fixture.baseURL, otherID, otherDir)
}

func assertSharedModelsSessionDeleted(t *testing.T, baseURL, sessionID string) {
	t.Helper()
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + url.PathEscape(sessionID)
	ctx, cancel := context.WithTimeout(context.Background(), sharedModelsHTTPTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		t.Fatalf("build GET deleted shared Models Factory Session %q: %v", sessionID, err)
	}
	client := &http.Client{Timeout: sharedModelsHTTPTimeout}
	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("GET deleted shared Models Factory Session %q: %v", sessionID, err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("GET deleted shared Models Factory Session %q status = %d, want 404", sessionID, response.StatusCode)
	}
}

func getSharedModelsSession(t testing.TB, baseURL, sessionID string) factoryapi.FactorySession {
	t.Helper()
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + url.PathEscape(sessionID)
	ctx, cancel := context.WithTimeout(context.Background(), sharedModelsHTTPTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		t.Fatalf("build GET shared Models Factory Session %q: %v", sessionID, err)
	}
	client := &http.Client{Timeout: sharedModelsHTTPTimeout}
	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("GET shared Models Factory Session %q: %v", sessionID, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("GET shared Models Factory Session %q status = %d: %s", sessionID, response.StatusCode, strings.TrimSpace(string(body)))
	}
	var payload factoryapi.FactorySessionGetResponse
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatalf("decode shared Models Factory Session %q: %v", sessionID, err)
	}
	session, err := payload.AsFactorySession()
	if err != nil {
		t.Fatalf("decode shared Models Factory Session %q payload: %v", sessionID, err)
	}
	return session
}

func assertSharedModelsSessionIdentity(t testing.TB, baseURL, expectedID string) {
	t.Helper()
	session := getSharedModelsSession(t, baseURL, expectedID)
	if session.Id != expectedID || session.IsDefault {
		t.Fatalf("shared Models Factory Session identity = %#v, want non-default %q", session, expectedID)
	}
}

func closeSharedModelsFixture() error {
	fixture := sharedModelsFixtureState.fixture
	if fixture == nil {
		return nil
	}
	return fixture.close()
}

func sharedModelsFixtureCounters() (rootBuilds, apiStarts, opens, closes int64) {
	fixture := sharedModelsFixtureState.fixture
	if fixture == nil {
		return 0, 0, 0, 0
	}
	return fixture.rootBuilds.Load(), fixture.apiStarts.Load(), fixture.opens.Load(), fixture.closes.Load()
}

func (fixture *sharedModelsFixture) close() error {
	if fixture == nil {
		return nil
	}
	var errs []error
	// Process and API shutdown are asynchronous. These context deadlines are
	// safety ceilings for their cancellation signals, not readiness polling.
	stopCtx, cancelStop := context.WithTimeout(context.Background(), sharedModelsProcessTimeout)
	if err := fixture.command.stop(stopCtx); err != nil {
		errs = append(errs, err)
	}
	cancelStop()
	if fixture.process != nil {
		closeCtx, cancel := context.WithTimeout(context.Background(), sharedModelsProcessTimeout)
		if err := fixture.process.Close(closeCtx); err != nil {
			errs = append(errs, fmt.Errorf("close shared Models application process: %w", err))
		}
		cancel()
	}
	if fixture.apiStarted.Load() {
		apiCtx, cancelAPI := context.WithTimeout(context.Background(), sharedModelsProcessTimeout)
		select {
		case <-fixture.apiStopped:
		case <-apiCtx.Done():
			errs = append(errs, fmt.Errorf("shared Models API server did not close after process cleanup: %w", apiCtx.Err()))
		}
		cancelAPI()
	}
	fixture.scenarioMu.Lock()
	if len(fixture.activeSession) != 0 {
		errs = append(errs, fmt.Errorf("shared Models active Factory Sessions after cleanup = %d", len(fixture.activeSession)))
	}
	fixture.scenarioMu.Unlock()
	if got := fixture.opens.Load(); got != fixture.closes.Load() {
		errs = append(errs, fmt.Errorf("shared Models Factory Session opens = %d, closes = %d", got, fixture.closes.Load()))
	}
	if err := os.RemoveAll(fixture.rootDir); err != nil {
		errs = append(errs, fmt.Errorf("remove shared Models package root %q: %w", fixture.rootDir, err))
	} else if _, err := os.Stat(fixture.rootDir); !errors.Is(err, os.ErrNotExist) {
		errs = append(errs, fmt.Errorf("shared Models package root %q remains after cleanup: %v", fixture.rootDir, err))
	}
	if fixture.homeDir != "" {
		if err := os.RemoveAll(fixture.homeDir); err != nil {
			errs = append(errs, fmt.Errorf("remove shared Models home %q: %w", fixture.homeDir, err))
		} else if _, err := os.Stat(fixture.homeDir); !errors.Is(err, os.ErrNotExist) {
			errs = append(errs, fmt.Errorf("shared Models home %q remains after cleanup: %v", fixture.homeDir, err))
		}
	}
	fixture.scenarioMu.Lock()
	sessionDirs := append([]string(nil), fixture.sessionDirs...)
	fixture.scenarioMu.Unlock()
	for _, sessionDir := range sessionDirs {
		if err := os.RemoveAll(sessionDir); err != nil {
			errs = append(errs, fmt.Errorf("remove shared Models session Factory %q: %w", sessionDir, err))
		} else if _, err := os.Stat(sessionDir); !errors.Is(err, os.ErrNotExist) {
			errs = append(errs, fmt.Errorf("shared Models session Factory %q remains after cleanup: %v", sessionDir, err))
		}
	}
	return errors.Join(errs...)
}
