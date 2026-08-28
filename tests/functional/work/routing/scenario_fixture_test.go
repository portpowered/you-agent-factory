package routing

import (
	"context"
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

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

type workRoutingScenarioCommandRunner struct {
	mu         sync.Mutex
	name       string
	results    []platformprocess.CommandResult
	errors     []error
	index      int
	requests   []platformprocess.CommandRequest
	active     atomic.Int32
	callStarts atomic.Int64
	callCloses atomic.Int64
}

func newWorkRoutingScenarioCommandRunner(
	name string,
	results []platformprocess.CommandResult,
	errors []error,
) *workRoutingScenarioCommandRunner {
	clonedResults := make([]platformprocess.CommandResult, len(results))
	for index, result := range results {
		clonedResults[index] = cloneWorkRoutingCommandResult(result)
	}
	return &workRoutingScenarioCommandRunner{
		name:    name,
		results: clonedResults,
		errors:  append([]error(nil), errors...),
	}
}

func (runner *workRoutingScenarioCommandRunner) Run(
	ctx context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	runner.callStarts.Add(1)
	defer runner.callCloses.Add(1)
	runner.active.Add(1)
	defer runner.active.Add(-1)
	select {
	case <-ctx.Done():
		return platformprocess.CommandResult{}, ctx.Err()
	default:
	}

	runner.mu.Lock()
	defer runner.mu.Unlock()
	callIndex := runner.index
	runner.index++
	runner.requests = append(runner.requests, cloneWorkRoutingCommandRequest(request))
	if callIndex >= len(runner.results) {
		return platformprocess.CommandResult{}, fmt.Errorf(
			"Work routing scenario %q command result queue exhausted at call %d",
			runner.name, callIndex+1,
		)
	}
	result := cloneWorkRoutingCommandResult(runner.results[callIndex])
	if callIndex < len(runner.errors) && runner.errors[callIndex] != nil {
		return result, runner.errors[callIndex]
	}
	return result, nil
}

func (runner *workRoutingScenarioCommandRunner) callCount() int {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return len(runner.requests)
}

func (runner *workRoutingScenarioCommandRunner) requestsSnapshot() []platformprocess.CommandRequest {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	requests := make([]platformprocess.CommandRequest, len(runner.requests))
	for index, request := range runner.requests {
		requests[index] = cloneWorkRoutingCommandRequest(request)
	}
	return requests
}

func (runner *workRoutingScenarioCommandRunner) activeCallCount() int {
	return int(runner.active.Load())
}

func (runner *workRoutingScenarioCommandRunner) callStats() (starts, closes, active int64) {
	return runner.callStarts.Load(), runner.callCloses.Load(), int64(runner.activeCallCount())
}

func cloneWorkRoutingCommandRequest(request platformprocess.CommandRequest) platformprocess.CommandRequest {
	request.Args = append([]string(nil), request.Args...)
	request.Stdin = append([]byte(nil), request.Stdin...)
	request.Env = append([]string(nil), request.Env...)
	return request
}

func cloneWorkRoutingCommandResult(result platformprocess.CommandResult) platformprocess.CommandResult {
	result.Stdout = append([]byte(nil), result.Stdout...)
	result.Stderr = append([]byte(nil), result.Stderr...)
	return result
}

type workRoutingScenario struct {
	fixture         *workRoutingPackageFixture
	id              string
	rootDir         string
	factoryDir      string
	runner          *workRoutingScenarioCommandRunner
	sessionID       string
	registered      bool
	routeRegistered bool
	routeClosed     bool
	closed          bool
}

func (fixture *workRoutingPackageFixture) newScenario(
	t *testing.T,
	id string,
	sourceFixture string,
	runner *workRoutingScenarioCommandRunner,
) *workRoutingScenario {
	t.Helper()
	rootDir, err := os.MkdirTemp(fixture.rootDir, "scenario-")
	if err != nil {
		t.Fatalf("create Work routing scenario root: %v", err)
	}
	scenario := &workRoutingScenario{
		fixture: fixture,
		id:      id,
		rootDir: rootDir,
		runner:  runner,
	}
	// Register ownership before copying or opening anything so a setup failure
	// still removes the scenario root and any route that was already acquired.
	t.Cleanup(func() { scenario.close(t) })
	factoryDir, err := copyWorkRoutingFixtureDir(
		support.LegacyFixtureDir(t, sourceFixture),
		filepath.Join(rootDir, "factory"),
	)
	if err != nil {
		t.Fatalf("copy Work routing scenario Factory %q: %v", id, err)
	}
	scenario.factoryDir = factoryDir
	if err := fixture.lifecycle.registerScenario(id, rootDir, factoryDir); err != nil {
		t.Fatalf("register Work routing scenario %q lifecycle: %v", id, err)
	}
	scenario.registered = true
	selectors := []string{rootDir, factoryDir}
	if err := fixture.provider.register(id, selectors, runner); err != nil {
		t.Fatalf("register Work routing scenario %q: %v", id, err)
	}
	scenario.routeRegistered = true
	return scenario
}

func (scenario *workRoutingScenario) open(t *testing.T) {
	t.Helper()
	opened := workRoutingReadValue(t, scenario.fixture, scenario.id+"/session-open", func() factoryapi.OpenFactorySessionResponse {
		return support.OpenFactorySessionAt(t, scenario.fixture.baseURL, scenario.factoryDir)
	})
	scenario.sessionID = opened.Session.Id
	if scenario.sessionID == factorysessions.DefaultSessionID {
		t.Fatalf("Work routing scenario %q opened the default Factory Session", scenario.id)
	}
	if err := scenario.fixture.lifecycle.openSession(scenario.id, scenario.sessionID); err != nil {
		t.Fatalf("register Work routing scenario %q lifecycle: %v", scenario.id, err)
	}
}

func (scenario *workRoutingScenario) close(t testing.TB) {
	t.Helper()
	if scenario == nil || scenario.closed {
		return
	}
	scenario.closed = true
	sessionAbsent := true
	if scenario.sessionID != "" {
		workRoutingRead(t, scenario.fixture, scenario.id+"/session-close", func() {
			support.CloseFactorySessionAt(t, scenario.fixture.baseURL, scenario.sessionID)
		})
		workRoutingRead(t, scenario.fixture, scenario.id+"/session-absence", func() {
			assertWorkRoutingSessionAbsent(t, scenario.fixture.baseURL, scenario.sessionID)
		})
	} else {
		sessionAbsent = false
	}
	if scenario.runner != nil {
		if active := scenario.runner.activeCallCount(); active != 0 {
			t.Errorf("owner=%q resource=controlled-call active=%d before route cleanup", scenario.id, active)
		}
	}
	if scenario.routeRegistered && !scenario.routeClosed {
		if err := scenario.fixture.provider.unregister(scenario.id); err != nil {
			t.Errorf("unregister Work routing scenario %q route: %v", scenario.id, err)
		}
		scenario.routeClosed = true
	}
	rootRemoved := false
	if err := os.RemoveAll(scenario.rootDir); err != nil {
		t.Errorf("CLEAN-001 remove Work routing scenario root %q: %v", scenario.rootDir, err)
	} else if _, err := os.Stat(scenario.rootDir); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("CLEAN-001 Work routing scenario root %q remains: %v", scenario.rootDir, err)
	} else {
		rootRemoved = true
	}
	factoryRemoved := false
	if scenario.factoryDir != "" {
		if _, err := os.Stat(scenario.factoryDir); errors.Is(err, os.ErrNotExist) {
			factoryRemoved = true
		} else if err != nil {
			t.Errorf("CLEAN-001 Work routing Factory path %q absence probe: %v", scenario.factoryDir, err)
		} else {
			t.Errorf("CLEAN-001 Work routing Factory path %q remains after cleanup", scenario.factoryDir)
		}
	}
	if scenario.registered {
		if err := scenario.fixture.lifecycle.closeScenario(scenario.id, sessionAbsent, rootRemoved, factoryRemoved); err != nil {
			t.Errorf("record Work routing scenario %q cleanup: %v", scenario.id, err)
		}
	}
}

func (scenario *workRoutingScenario) closeRoute(t testing.TB) {
	t.Helper()
	if scenario == nil || !scenario.routeRegistered || scenario.routeClosed {
		return
	}
	if err := scenario.fixture.provider.unregister(scenario.id); err != nil {
		t.Errorf("unregister Work routing scenario %q route: %v", scenario.id, err)
	}
	scenario.routeClosed = true
}

func (scenario *workRoutingScenario) observe(
	t *testing.T,
	timeout time.Duration,
) (factoryapi.FactorySession, factoryapi.ListWorkResponse, []factoryapi.FactoryEvent) {
	t.Helper()
	workRoutingRead(t, scenario.fixture, scenario.id+"/session-terminal", func() {
		support.WaitForSessionTerminalStatus(t, scenario.fixture.baseURL, scenario.sessionID, timeout)
	})
	session := workRoutingReadValue(t, scenario.fixture, scenario.id+"/session-read", func() factoryapi.FactorySession {
		return getWorkRoutingSession(t, scenario.fixture.baseURL, scenario.sessionID)
	})
	listed := workRoutingReadValue(t, scenario.fixture, scenario.id+"/work-read", func() factoryapi.ListWorkResponse {
		return listWorkRoutingSession(t, scenario.fixture.baseURL, scenario.sessionID)
	})
	events := workRoutingReadValue(t, scenario.fixture, scenario.id+"/factory-events-read", func() []factoryapi.FactoryEvent {
		return support.GetFactoryEventsForSessionAt(t, scenario.fixture.baseURL, scenario.sessionID)
	})
	return session, listed, events
}

func waitForWorkRoutingWorkCount(
	t testing.TB,
	fixture *workRoutingPackageFixture,
	baseURL, sessionID string,
	want int,
	timeout time.Duration,
) {
	// The file watcher admits the second seed asynchronously after the session
	// returns idle; observe the public Work listing instead of adding a fixed
	// delay that would hide scheduling variance.
	t.Helper()
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		listed := workRoutingReadValue(t, fixture, "work-count/"+sessionID, func() factoryapi.ListWorkResponse {
			return listWorkRoutingSession(t, baseURL, sessionID)
		})
		if len(listed.Results) >= want {
			return
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatalf(
				"timed out waiting for %d Work items in Factory Session %q; listed=%#v",
				want,
				sessionID,
				listed,
			)
		}
	}
}

func workRoutingPathKey(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if absolute, err := filepath.Abs(path); err == nil {
		path = absolute
	}
	return filepath.Clean(path)
}

func workRoutingPathContains(root, candidate string) bool {
	relative, err := filepath.Rel(root, candidate)
	if err != nil {
		return false
	}
	return relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)))
}

func copyWorkRoutingFixtureDir(sourceDir, targetDir string) (string, error) {
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return "", err
	}
	err := filepath.WalkDir(sourceDir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}
		if relative == "." {
			return nil
		}
		target := filepath.Join(targetDir, relative)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
	if err != nil {
		return "", err
	}
	return targetDir, nil
}

func getWorkRoutingSession(t testing.TB, baseURL, sessionID string) factoryapi.FactorySession {
	t.Helper()
	response := support.GetJSON[factoryapi.FactorySessionGetResponse](
		t,
		strings.TrimSuffix(baseURL, "/")+"/factory-sessions/"+url.PathEscape(sessionID),
	)
	session, err := response.AsFactorySession()
	if err != nil {
		t.Fatalf("decode Work routing Factory Session %q: %v", sessionID, err)
	}
	return session
}

func listWorkRoutingSession(
	t testing.TB,
	baseURL, sessionID string,
) factoryapi.ListWorkResponse {
	t.Helper()
	endpoint := strings.TrimSuffix(baseURL, "/") +
		"/factory-sessions/" + url.PathEscape(sessionID) + "/work"
	return support.GetJSON[factoryapi.ListWorkResponse](t, endpoint)
}

func getWorkRoutingWorkByID(
	t testing.TB,
	baseURL, sessionID, workID string,
) factoryapi.Work {
	t.Helper()
	endpoint := strings.TrimSuffix(baseURL, "/") +
		"/factory-sessions/" + url.PathEscape(sessionID) + "/work/" + url.PathEscape(workID)
	return support.GetJSON[factoryapi.Work](t, endpoint)
}

func assertWorkRoutingSessionAbsent(t testing.TB, baseURL, sessionID string) {
	t.Helper()
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + url.PathEscape(sessionID)
	response, err := http.Get(endpoint)
	if err != nil {
		t.Fatalf("GET deleted Work routing Factory Session %q: %v", sessionID, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf(
			"GET deleted Work routing Factory Session %q status = %d, want 404: %s",
			sessionID, response.StatusCode, strings.TrimSpace(string(body)),
		)
	}
}

var _ platformprocess.CommandRunner = (*workRoutingProviderCommandRunner)(nil)
var _ platformprocess.CommandRunner = (*workRoutingScenarioCommandRunner)(nil)
