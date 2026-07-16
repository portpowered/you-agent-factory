package service_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	workertaxonomy "github.com/portpowered/infinite-you/pkg/workers/taxonomy"

	workerconfig "github.com/portpowered/infinite-you/pkg/workers/config"

	"github.com/jonboulle/clockwork"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"github.com/portpowered/infinite-you/internal/testutil/runtimefixtures"
	"github.com/portpowered/infinite-you/pkg/config"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	"github.com/portpowered/infinite-you/pkg/work"
	"github.com/portpowered/infinite-you/pkg/workers"
	hostedworkers "github.com/portpowered/infinite-you/pkg/workers/hosted"
	workersservice "github.com/portpowered/infinite-you/pkg/workers/service"
)

func TestStartHostedLinearPoller_SubmitsIssuesThroughWorkersService(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"data": {
				"issues": {
					"nodes": [
						{
							"id": "issue-new",
							"identifier": "ENG-101",
							"title": "Newest issue",
							"description": "First",
							"updatedAt": "2026-05-22T07:10:00Z",
							"url": "https://linear.app/example/issue/ENG-101",
							"team": {"id": "team-1", "key": "ENG", "name": "Engineering"},
							"state": {"id": "state-1", "name": "Todo", "type": "unstarted"},
							"assignee": null
						}
					],
					"pageInfo": {"hasNextPage": false, "endCursor": ""}
				}
			}
		}`))
	}))
	defer server.Close()

	factoryDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(factoryDir, "secrets"), 0o755); err != nil {
		t.Fatalf("mkdir secrets: %v", err)
	}
	if err := os.WriteFile(filepath.Join(factoryDir, "secrets", "linear-api-key"), []byte("runtime-linear-key\n"), 0o600); err != nil {
		t.Fatalf("write secret file: %v", err)
	}

	submitted := &recordingSubmitter{}
	poller := hostedLinearPollerWorkstation()
	worker := hostedLinearPollerWorker()
	runtimeCfg := newHostedPollerLoadedRuntimeConfig(t, factoryDir, poller, worker)

	svc := workersservice.New(workersservice.Config{
		Logger: zap.NewNop(),
		HostedWorkers: hostedworkers.Config{
			Logger: zap.NewNop(), HTTPClient: server.Client(), LinearEndpoint: server.URL,
		},
	})

	sidecarCtx, cancel := context.WithCancel(context.Background())
	var sidecars sync.WaitGroup
	t.Cleanup(func() {
		cancel()
		sidecars.Wait()
	})
	svc.StartHostedLinearPoller(sidecarCtx, &sidecars, runtimeCfg, poller, worker, submitted.submit)

	waitForPollerSubmission(t, submitted, 1, time.Second)
	calls, _ := submitted.snapshot()
	if calls != 1 {
		t.Fatalf("submit calls = %d, want 1", calls)
	}
}

func TestStartHostedLinearPoller_StopsOnContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"issues":{"nodes":[],"pageInfo":{"hasNextPage":false,"endCursor":""}}}}`))
	}))
	defer server.Close()

	factoryDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(factoryDir, "secrets"), 0o755); err != nil {
		t.Fatalf("mkdir secrets: %v", err)
	}
	if err := os.WriteFile(filepath.Join(factoryDir, "secrets", "linear-api-key"), []byte("runtime-linear-key\n"), 0o600); err != nil {
		t.Fatalf("write secret file: %v", err)
	}

	logCore, observedLogs := observer.New(zap.InfoLevel)
	poller := hostedLinearPollerWorkstation()
	worker := hostedLinearPollerWorker()
	runtimeCfg := newHostedPollerLoadedRuntimeConfig(t, factoryDir, poller, worker)

	svc := workersservice.New(workersservice.Config{
		Logger: zap.New(logCore),
		HostedWorkers: hostedworkers.Config{
			Logger: zap.New(logCore), HTTPClient: server.Client(), LinearEndpoint: server.URL,
		},
	})

	sidecarCtx, cancel := context.WithCancel(context.Background())
	var sidecars sync.WaitGroup
	svc.StartHostedLinearPoller(sidecarCtx, &sidecars, runtimeCfg, poller, worker, func(context.Context, work.WorkRequest) error {
		return nil
	})

	waitForObservedLogMessage(t, observedLogs, "hosted linear poller started", time.Second)
	cancel()
	sidecars.Wait()

	stopped := observedLogs.FilterMessage("hosted linear poller stopped").All()
	if len(stopped) != 1 {
		t.Fatalf("hosted linear poller stopped log count = %d, want 1", len(stopped))
	}
	if got, ok := stopped[0].ContextMap()["reason"].(string); !ok || got != "context canceled" {
		t.Fatalf("hosted linear poller stop reason = %#v, want context canceled", stopped[0].ContextMap()["reason"])
	}
}

func TestStartPollersForRuntime_StartsScriptAndHostedPollers(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"data": {
				"issues": {
					"nodes": [
						{
							"id": "issue-new",
							"identifier": "ENG-101",
							"title": "Newest issue",
							"description": "From hosted poller",
							"updatedAt": "2026-05-22T07:10:00Z",
							"url": "https://linear.app/example/issue/ENG-101",
							"team": {"id": "team-1", "key": "ENG", "name": "Engineering"},
							"state": {"id": "state-1", "name": "Todo", "type": "unstarted"},
							"assignee": null
						}
					],
					"pageInfo": {"hasNextPage": false, "endCursor": ""}
				}
			}
		}`))
	}))
	defer server.Close()

	factoryDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(factoryDir, "secrets"), 0o755); err != nil {
		t.Fatalf("mkdir secrets: %v", err)
	}
	if err := os.WriteFile(filepath.Join(factoryDir, "secrets", "linear-api-key"), []byte("runtime-linear-key\n"), 0o600); err != nil {
		t.Fatalf("write secret file: %v", err)
	}

	workRequestJSON := []byte(`{
		"requestId":"script-batch-1",
		"type":"FACTORY_REQUEST_BATCH",
		"works":[{"name":"issue-script","workTypeName":"task","payload":{"id":"SCRIPT-1"}}]
	}`)
	runner := &pollerSequenceCommandRunner{
		outcomes: []pollerRunOutcome{{result: workers.CommandResult{Stdout: workRequestJSON}}},
	}
	submitted := &recordingSubmitter{}

	scriptPoller := newCanonicalScriptPollerWorkstation()
	scriptWorker := newCanonicalScriptPollerWorker()
	hostedPoller := hostedLinearPollerWorkstation()
	hostedWorker := hostedLinearPollerWorker()

	factoryCfg := &interfaces.FactoryConfig{
		Workers: []workerconfig.Config{
			{Name: scriptWorker.Name},
			{Name: hostedWorker.Name},
		},
		Workstations: []interfaces.FactoryWorkstationConfig{scriptPoller, hostedPoller},
	}
	loaded, err := config.NewLoadedFactoryConfig(factoryDir, factoryCfg, runtimefixtures.RuntimeDefinitionLookupFixture{
		Workers: map[string]*workerconfig.Config{
			scriptWorker.Name: scriptWorker,
			hostedWorker.Name: hostedWorker,
		},
		Workstations: map[string]*interfaces.FactoryWorkstationConfig{
			scriptPoller.Name: &scriptPoller,
			hostedPoller.Name: &hostedPoller,
		},
	})
	if err != nil {
		t.Fatalf("NewLoadedFactoryConfig: %v", err)
	}

	fakeClock := clockwork.NewFakeClock()
	svc := workersservice.New(workersservice.Config{
		Logger:        zap.NewNop(),
		Clock:         fakeClock,
		CommandRunner: runner,
		HostedWorkers: hostedworkers.Config{
			Logger: zap.NewNop(), Clock: fakeClock, HTTPClient: server.Client(), LinearEndpoint: server.URL,
		},
	})

	sidecarCtx, cancel := context.WithCancel(context.Background())
	var sidecars sync.WaitGroup
	t.Cleanup(func() {
		cancel()
		sidecars.Wait()
	})
	svc.StartPollersForRuntime(sidecarCtx, &sidecars, factoryCfg, loaded, submitted.submit)

	waitForPollerSubmission(t, submitted, 2, 2*time.Second)
	calls, _ := submitted.snapshot()
	if calls < 2 {
		t.Fatalf("submit calls = %d, want at least 2 (script + hosted)", calls)
	}
}

func hostedLinearPollerWorkstation() interfaces.FactoryWorkstationConfig {
	return interfaces.FactoryWorkstationConfig{
		Name:           "linear-ingress",
		Kind:           workertaxonomy.WorkstationKindPoller,
		WorkerTypeName: "linear-poller",
	}
}

func hostedLinearPollerWorker() *workerconfig.Config {
	return &workerconfig.Config{
		Name:     "linear-poller",
		Type:     workertaxonomy.WorkerTypeHosted,
		Provider: workertaxonomy.HostedWorkerProviderLinear,
		Auth:     &workerconfig.HostedWorkerAuthConfig{SecretRef: "secrets/linear-api-key"},
		Linear: &workerconfig.HostedLinearWorkerConfig{
			PollInterval: "1h",
			Mapping: workerconfig.HostedLinearWorkerMappingConfig{
				WorkType: "story",
				State:    "init",
			},
		},
	}
}

func newHostedPollerLoadedRuntimeConfig(
	t *testing.T,
	factoryDir string,
	poller interfaces.FactoryWorkstationConfig,
	worker *workerconfig.Config,
) *config.LoadedFactoryConfig {
	t.Helper()
	loaded, err := config.NewLoadedFactoryConfig(factoryDir, &interfaces.FactoryConfig{
		Workers:      []workerconfig.Config{{Name: worker.Name}},
		Workstations: []interfaces.FactoryWorkstationConfig{poller},
	}, runtimefixtures.RuntimeDefinitionLookupFixture{
		Workers:      map[string]*workerconfig.Config{worker.Name: worker},
		Workstations: map[string]*interfaces.FactoryWorkstationConfig{poller.Name: &poller},
	})
	if err != nil {
		t.Fatalf("NewLoadedFactoryConfig: %v", err)
	}
	return loaded
}

func waitForPollerSubmission(t *testing.T, submitted *recordingSubmitter, want int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		calls, _ := submitted.snapshot()
		if calls >= want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	calls, _ := submitted.snapshot()
	t.Fatalf("timed out waiting for %d poller submission(s); got %d", want, calls)
}

func waitForObservedLogMessage(t *testing.T, observed *observer.ObservedLogs, message string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if len(observed.FilterMessage(message).All()) > 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for log message %q", message)
}
