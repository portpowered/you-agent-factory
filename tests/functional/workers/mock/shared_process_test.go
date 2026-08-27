package mock

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	futureMockWorkerName      = "future-mocked-worker"
	futureMockWorkstationName = "future-mock-process"
	futureMockWorkType        = "future-mock-task"
	futureMockWorkID          = "future-mock-work"

	sharedHostWorker      = "shared-workers-mock-host-worker"
	sharedHostWorkstation = "shared-workers-mock-host-process"
	sharedHostWorkType    = "shared-workers-mock-host-task"
)

// TestSharedProcessWorkersMock keeps the Workers mock selection/routing
// scenarios on one root-built customer host. Each scenario gets a distinct
// public Factory Session and a fresh command-edge delegate.
func TestSharedProcessWorkersMock(t *testing.T) {
	fixture := newSharedWorkersMockFixture(t)

	tests := []struct {
		name string
		run  func(*testing.T, *sharedWorkersMockFixture)
	}{
		{name: "NamedAgy", run: testNamedAgyMockPreservesDispatchMetadataAndCompletionLog},
		{name: "ScriptClassifier", run: testScriptWorkerClassifierRoutesWithoutModelCalls},
		{name: "MockWorkersReplace", run: testMockWorkersReplaceOnlyNamedChildren},
		{name: "UnknownWorker", run: testUnknownWorkerOverrideFailsActionably},
		{name: "FutureFields", run: testFutureMockWorkerFieldsAreIgnoredAndDispatchBehaviorIsPreserved},
		{name: "MockWorkerFailure", run: testMockWorkerFailureReturnsStablePublicFailure},
		{name: "RootSelection", run: testMockWorkerSelectedThroughCustomerProcess},
		{name: "ServiceConfigAlignment", run: testServiceConfigOverrideAlignmentCustomerProcess},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test.run(t, fixture)
		})
	}
}

type sharedWorkersMockFixture struct {
	server        *support.FunctionalAPIServer
	providerEdge  *sharedWorkersMockCommandRunner
	scriptEdge    *sharedWorkersMockCommandRunner
	runtimeLogDir string
}

type sharedWorkersMockCommandRunner struct {
	mu       sync.RWMutex
	delegate platformprocess.CommandRunner
}

func newSharedWorkersMockCommandRunner() *sharedWorkersMockCommandRunner {
	return &sharedWorkersMockCommandRunner{}
}

func (runner *sharedWorkersMockCommandRunner) set(delegate platformprocess.CommandRunner) {
	runner.mu.Lock()
	runner.delegate = delegate
	runner.mu.Unlock()
}

func (runner *sharedWorkersMockCommandRunner) Run(
	ctx context.Context,
	req platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	runner.mu.RLock()
	delegate := runner.delegate
	runner.mu.RUnlock()
	if delegate == nil {
		return platformprocess.CommandResult{}, errors.New("shared workers mock command runner delegate is not configured")
	}
	return delegate.Run(ctx, req)
}

func newSharedWorkersMockFixture(t *testing.T) *sharedWorkersMockFixture {
	t.Helper()

	hostDir := support.ScaffoldFactory(t, map[string]any{
		"name": "shared-workers-mock-host",
		"workTypes": []map[string]any{{
			"name": sharedHostWorkType,
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "done", "type": "TERMINAL"},
			},
		}},
		"workers": []map[string]string{{"name": sharedHostWorker}},
		"workstations": []map[string]any{{
			"name":      sharedHostWorkstation,
			"worker":    sharedHostWorker,
			"inputs":    []map[string]string{{"workType": sharedHostWorkType, "state": "init"}},
			"outputs":   []map[string]string{{"workType": sharedHostWorkType, "state": "done"}},
			"onFailure": []map[string]string{{"workType": sharedHostWorkType, "state": "done"}},
		}},
	})
	support.WriteAgentConfig(t, hostDir, sharedHostWorker, support.BuildModelWorkerConfig(
		modelprovider.ProviderCodex,
		"shared-workers-mock-host-model",
	))

	providerEdge := newSharedWorkersMockCommandRunner()
	scriptEdge := newSharedWorkersMockCommandRunner()
	runtimeLogDir := t.TempDir()
	mockWorkersPath := writeSharedMockWorkersConfig(t)
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                hostDir,
		WaitForServiceModeRuntime: true,
		Args: []string{
			"--with-mock-workers", mockWorkersPath,
			"--runtime-log-dir", runtimeLogDir,
		},
		Edges: serviceedges.Edges{
			ProviderCommandRunner: providerEdge,
			ScriptCommandRunner:   scriptEdge,
		},
	})

	return &sharedWorkersMockFixture{
		server:        server,
		providerEdge:  providerEdge,
		scriptEdge:    scriptEdge,
		runtimeLogDir: runtimeLogDir,
	}
}

func (fixture *sharedWorkersMockFixture) useCommandRunners(
	provider platformprocess.CommandRunner,
	script platformprocess.CommandRunner,
) {
	fixture.providerEdge.set(provider)
	fixture.scriptEdge.set(script)
}

type sharedWorkersMockSession struct {
	fixture *sharedWorkersMockFixture
	id      string
	closed  bool
}

func (fixture *sharedWorkersMockFixture) openSession(t *testing.T, factoryDir string) *sharedWorkersMockSession {
	t.Helper()
	opened := support.OpenFactorySessionAt(t, fixture.server.URL(), factoryDir)
	session := &sharedWorkersMockSession{fixture: fixture, id: opened.Session.Id}
	t.Cleanup(func() {
		session.close(t)
	})
	return session
}

func (session *sharedWorkersMockSession) terminalObservations(
	t *testing.T,
	timeout time.Duration,
) (factoryapi.ListWorkResponse, []factoryapi.FactoryEvent) {
	t.Helper()
	support.WaitForSessionTerminalStatus(t, session.fixture.server.URL(), session.id, timeout)
	listed := support.GetJSON[factoryapi.ListWorkResponse](t, session.workURL())
	events := support.GetFactoryEventsForSessionAt(t, session.fixture.server.URL(), session.id)
	return listed, events
}

func (session *sharedWorkersMockSession) workURL() string {
	return strings.TrimSuffix(session.fixture.server.URL(), "/") +
		"/factory-sessions/" + url.PathEscape(session.id) + "/work"
}

func (session *sharedWorkersMockSession) close(t testing.TB) {
	t.Helper()
	if session == nil || session.closed {
		return
	}
	support.CloseFactorySessionAt(t, session.fixture.server.URL(), session.id)
	session.closed = true
}

func (session *sharedWorkersMockSession) closeAndAssertGone(t *testing.T) {
	t.Helper()
	session.close(t)
	assertSharedWorkersMockSessionGone(t, session.fixture.server.URL(), session.id)
}

func assertSharedWorkersMockSessionGone(t *testing.T, baseURL, sessionID string) {
	t.Helper()
	client := &http.Client{Timeout: 2 * time.Second}
	base := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + url.PathEscape(sessionID)
	assertSharedWorkersMockEndpointStatus(t, client, base, http.StatusNotFound)
	for _, suffix := range []string{"/work", "/events"} {
		endpoint := base + suffix
		status := assertSharedWorkersMockEndpointStatus(t, client, endpoint, 0)
		if status >= http.StatusOK && status < http.StatusMultipleChoices {
			t.Fatalf("GET deleted Factory Session endpoint %s status = %d, want non-success", endpoint, status)
		}
	}
}

func assertSharedWorkersMockEndpointStatus(
	t *testing.T,
	client *http.Client,
	endpoint string,
	wantStatus int,
) int {
	t.Helper()
	request, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		t.Fatalf("build deleted Factory Session request %s: %v", endpoint, err)
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("GET deleted Factory Session endpoint %s: %v", endpoint, err)
	}
	_, _ = io.Copy(io.Discard, response.Body)
	response.Body.Close()
	if wantStatus != 0 && response.StatusCode != wantStatus {
		t.Fatalf("GET deleted Factory Session endpoint %s status = %d, want %d", endpoint, response.StatusCode, wantStatus)
	}
	return response.StatusCode
}

func writeSharedMockWorkersConfig(t *testing.T) string {
	t.Helper()

	payload := map[string]any{
		"unmatchedDispatchPolicy": "passthrough",
		"mockWorkers": []map[string]any{
			{
				"id":              "shared-agy-reject",
				"workerName":      "worker",
				"workstationName": "process",
				"runType":         "reject",
				"rejectConfig": map[string]any{
					"stdout":   configuredRejectStdout,
					"stderr":   configuredRejectStderr,
					"exitCode": 7,
				},
			},
			{
				"id":              "shared-script-classifier",
				"workerName":      scriptClassifierWorker,
				"workstationName": scriptClassifierWorkstation,
				"runType":         "script",
				"scriptConfig": map[string]any{
					"command": "mock-classifier-script",
				},
			},
			{
				"id":              "shared-named-replacement",
				"workerName":      mockedWorkerName,
				"workstationName": mockedWorkstationName,
				"runType":         "accept",
			},
			{
				"id":              "shared-future-fields",
				"workerName":      futureMockWorkerName,
				"workstationName": futureMockWorkstationName,
				"runType":         "accept",
				"futureNested":    map[string]any{"secret": "must not affect matching"},
			},
			{
				"id":              "shared-reject",
				"workerName":      rejectWorkerName,
				"workstationName": rejectWorkstationName,
				"runType":         "reject",
				"rejectConfig": map[string]any{
					"stdout":   configuredRejectStdout,
					"stderr":   configuredRejectStderr,
					"exitCode": 7,
				},
			},
			{
				"id":              "shared-root-selection",
				"workerName":      rootMockWorker,
				"workstationName": rootMockWorkstation,
				"runType":         "accept",
			},
		},
		"futureTopLevel": true,
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal shared mock workers config: %v", err)
	}
	path := filepath.Join(t.TempDir(), "shared-mock-workers.json")
	if err := os.WriteFile(path, payloadBytes, 0o600); err != nil {
		t.Fatalf("write shared mock workers config: %v", err)
	}
	return path
}

func requireSharedWorkersMockRuntimeLogRecord(
	t *testing.T,
	logDir string,
	eventName string,
	requestID string,
) map[string]any {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(logDir, "*", "*", "*", "*-runtime-log-*.log"))
	if err != nil {
		t.Fatalf("glob shared runtime log paths: %v", err)
	}
	for _, path := range matches {
		file, err := os.Open(path)
		if err != nil {
			t.Fatalf("open shared runtime log %s: %v", path, err)
		}
		var found map[string]any
		decoder := json.NewDecoder(file)
		for {
			var record map[string]any
			err := decoder.Decode(&record)
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				file.Close()
				t.Fatalf("decode shared runtime log record: %v", err)
			}
			if record["event_name"] == eventName && record["request_id"] == requestID {
				found = record
				break
			}
		}
		file.Close()
		if found != nil {
			return found
		}
	}
	t.Fatalf("shared runtime logs under %s did not contain event_name %q for request %q", logDir, eventName, requestID)
	return nil
}
