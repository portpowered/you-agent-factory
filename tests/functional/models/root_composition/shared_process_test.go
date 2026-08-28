package root_composition_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const sharedModelsProcessTimeout = 30 * time.Second

// sharedModelsFixture owns the one root-built process and API transport for
// the identical rich-catalog scenarios. Each scenario still opens and closes
// its own explicit Factory Session; only immutable process wiring and the
// read-only catalog fixture are shared. The public Models routes are bound to
// the process runtime and have no session selector, so the explicit session is
// a lifecycle-isolation witness while the original process-level model routes
// remain the behavior witness.
type sharedModelsFixture struct {
	rootDir string
	homeDir string
	baseURL string
	env     []string

	process    support.ApplicationProcess
	command    *sharedModelsProcessCommand
	apiStarted atomic.Bool
	apiStopped chan struct{}

	scenarioMu    sync.Mutex
	sessionIDs    map[string]string
	activeSession map[string]string
	rootBuilds    atomic.Int64
	apiStarts     atomic.Int64
	opens         atomic.Int64
	closes        atomic.Int64
}

type sharedModelsProcessCommand struct {
	cancel context.CancelFunc
	done   chan error
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
	t.Run("catalog discovery projects worker capabilities and Factory precedence", func(t *testing.T) {
		runModelsCatalogDiscoveryProjectsWorkerCapabilitiesAndFactoryPrecedence(t)
	})
	t.Run("unknown detail keeps the public not-found contract", func(t *testing.T) {
		runModelsCatalogDiscoveryMapsUnknownDetailThroughHTTP(t)
	})

	fixture := ensureSharedModelsFixture(t)
	if got := fixture.rootBuilds.Load(); got != 1 {
		t.Fatalf("shared Models root builds = %d, want exactly one", got)
	}
	if got := fixture.apiStarts.Load(); got != 1 {
		t.Fatalf("shared Models API starts = %d, want exactly one", got)
	}
	if got, want := fixture.opens.Load(), fixture.closes.Load(); got != want {
		t.Fatalf("shared Models Factory Session opens = %d, closes = %d", got, want)
	}
	if got := len(fixture.sessionIDs); got != int(fixture.opens.Load()) {
		t.Fatalf("shared Models unique Factory Session IDs = %d, want %d", got, fixture.opens.Load())
	}
}

func ensureSharedModelsFixture(t *testing.T) *sharedModelsFixture {
	t.Helper()
	sharedModelsFixtureState.Do(func() {
		sharedModelsFixtureState.fixture, sharedModelsFixtureState.err = newSharedModelsFixture()
	})
	if sharedModelsFixtureState.err != nil {
		t.Fatalf("initialize shared Models fixture: %v", sharedModelsFixtureState.err)
	}
	return sharedModelsFixtureState.fixture
}

func newSharedModelsFixture() (_ *sharedModelsFixture, err error) {
	rootDir, err := os.MkdirTemp("", "c06-models-shared-")
	if err != nil {
		return nil, fmt.Errorf("create shared Models package root: %w", err)
	}
	fixture := &sharedModelsFixture{
		rootDir:       rootDir,
		sessionIDs:    make(map[string]string),
		activeSession: make(map[string]string),
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

	homeDir, err := os.MkdirTemp("", "c06-models-shared-home-")
	if err != nil {
		return nil, fmt.Errorf("create shared Models home: %w", err)
	}
	fixture.homeDir = homeDir
	inputs := support.FakeInputs(context.Background(), []string{
		"you", "run", "--dir", rootDir, "--continuously", "--with-server", "--quiet", "--no-record",
	})
	inputs.WorkingDirectory = rootDir
	inputs.Env = sharedModelsEnvironment(inputs.Env, homeDir)
	fixture.env = append([]string(nil), inputs.Env...)
	fixture.command = startSharedModelsProcess(process, inputs)

	baseURL, waitErr := api.WaitForBaseURL(sharedModelsProcessTimeout)
	if waitErr != nil {
		return nil, fmt.Errorf("wait for shared Models API: %w", waitErr)
	}
	fixture.baseURL = baseURL
	if err := waitForSharedModelsRuntime(baseURL, sharedModelsProcessTimeout); err != nil {
		return nil, err
	}
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

func sharedModelsEnvironment(environment []string, homeDir string) []string {
	result := make([]string, 0, len(environment)+2)
	for _, entry := range environment {
		name := strings.SplitN(entry, "=", 2)[0]
		if strings.EqualFold(name, "HOME") || strings.EqualFold(name, "USERPROFILE") {
			continue
		}
		result = append(result, entry)
	}
	result = append(result, "HOME="+homeDir, "USERPROFILE="+homeDir)
	return result
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

func (command *sharedModelsProcessCommand) stop() error {
	if command == nil {
		return nil
	}
	command.cancel()
	select {
	case err := <-command.done:
		if err != nil && !errors.Is(err, context.Canceled) {
			return fmt.Errorf("stop shared Models process command: %w", err)
		}
		return nil
	case <-time.After(sharedModelsProcessTimeout):
		return fmt.Errorf("timed out waiting for shared Models process command shutdown")
	}
}

func waitForSharedModelsRuntime(baseURL string, timeout time.Duration) error {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	endpoint := strings.TrimSuffix(baseURL, "/") + "/status"
	for {
		response, err := http.Get(endpoint)
		if err == nil {
			var status factoryapi.StatusResponse
			decodeErr := json.NewDecoder(response.Body).Decode(&status)
			response.Body.Close()
			if response.StatusCode == http.StatusOK && decodeErr == nil && status.RuntimeStatus != "" {
				return nil
			}
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			return fmt.Errorf("timed out waiting for shared Models runtime at %s", endpoint)
		}
	}
}

func (fixture *sharedModelsFixture) withSession(
	t *testing.T,
	label string,
	observe func(string),
) {
	t.Helper()
	fixture.scenarioMu.Lock()
	defer fixture.scenarioMu.Unlock()

	opened := support.OpenFactorySessionAt(t, fixture.baseURL, fixture.rootDir)
	sessionID := opened.Session.Id
	if sessionID == factorysessions.DefaultSessionID {
		t.Fatalf("shared Models scenario %q opened the default Factory Session", label)
	}
	fixture.opens.Add(1)
	c06Ledger.sharedSessionOpens.Add(1)
	defer func() {
		support.CloseFactorySessionAt(t, fixture.baseURL, sessionID)
		delete(fixture.activeSession, sessionID)
		fixture.closes.Add(1)
		c06Ledger.sharedSessionCloses.Add(1)
		assertSharedModelsSessionDeleted(t, fixture.baseURL, sessionID)
		defaultSession := support.GetDefaultSession(t, fixture.baseURL)
		if !defaultSession.IsDefault || strings.TrimSpace(defaultSession.Id) == "" {
			t.Fatalf("shared Models default Factory Session after %q = %#v", label, defaultSession)
		}
	}()
	if previous, exists := fixture.sessionIDs[sessionID]; exists {
		t.Fatalf("shared Models Factory Session ID %q reused by %q and %q", sessionID, previous, label)
	}
	fixture.sessionIDs[sessionID] = label
	fixture.activeSession[sessionID] = label

	observe(sessionID)
}

func assertSharedModelsSessionDeleted(t *testing.T, baseURL, sessionID string) {
	t.Helper()
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + url.PathEscape(sessionID)
	response, err := http.Get(endpoint)
	if err != nil {
		t.Fatalf("GET deleted shared Models Factory Session %q: %v", sessionID, err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("GET deleted shared Models Factory Session %q status = %d, want 404", sessionID, response.StatusCode)
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
	if err := fixture.command.stop(); err != nil {
		errs = append(errs, err)
	}
	if fixture.process != nil {
		closeCtx, cancel := context.WithTimeout(context.Background(), sharedModelsProcessTimeout)
		if err := fixture.process.Close(closeCtx); err != nil {
			errs = append(errs, fmt.Errorf("close shared Models application process: %w", err))
		}
		cancel()
	}
	if fixture.apiStarted.Load() {
		select {
		case <-fixture.apiStopped:
		case <-time.After(sharedModelsProcessTimeout):
			errs = append(errs, fmt.Errorf("shared Models API server did not close after process cleanup"))
		}
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
	return errors.Join(errs...)
}
