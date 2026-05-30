package hostedworkers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
	"github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestStartLinearPoller_SubmitsIssuesThroughSubmitter(t *testing.T) {
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

	var submitCalls int
	var submittedWorkID string
	submitter := Submitter(func(_ context.Context, request interfaces.WorkRequest) error {
		submitCalls++
		if len(request.Works) > 0 {
			submittedWorkID = request.Works[0].WorkID
		}
		return nil
	})

	poller := interfaces.FactoryWorkstationConfig{
		Name:           "linear-ingress",
		Kind:           interfaces.WorkstationKindPoller,
		WorkerTypeName: "linear-poller",
	}
	worker := &interfaces.WorkerConfig{
		Name:     "linear-poller",
		Type:     interfaces.WorkerTypeHosted,
		Provider: interfaces.HostedWorkerProviderLinear,
		Auth:     &interfaces.HostedWorkerAuthConfig{SecretRef: "secrets/linear-api-key"},
		Linear: &interfaces.HostedLinearWorkerConfig{
			PollInterval: "1h",
			Mapping: interfaces.HostedLinearWorkerMappingConfig{
				WorkType: "story",
				State:    "init",
			},
		},
	}
	runtimeCfg, err := config.NewLoadedFactoryConfig(
		factoryDir,
		&interfaces.FactoryConfig{
			Workers:      []interfaces.WorkerConfig{{Name: worker.Name}},
			Workstations: []interfaces.FactoryWorkstationConfig{poller},
		},
		nil,
	)
	if err != nil {
		t.Fatalf("NewLoadedFactoryConfig: %v", err)
	}

	pollerCfg := Config{
		Logger:         zap.NewNop(),
		HTTPClient:     server.Client(),
		LinearEndpoint: server.URL,
	}

	sidecarCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var sidecars sync.WaitGroup
	StartLinearPoller(sidecarCtx, &sidecars, pollerCfg, runtimeCfg, poller, worker, submitter)

	waitForSubmitCalls(t, &submitCalls, 1, time.Second)
	cancel()
	sidecars.Wait()

	if submitCalls != 1 {
		t.Fatalf("submit calls = %d, want 1", submitCalls)
	}
	if submittedWorkID != "linear:issue-new" {
		t.Fatalf("submitted work id = %q, want linear:issue-new", submittedWorkID)
	}
}

func TestStartLinearPoller_StopsAndLogsLifecycle(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"data": {
				"issues": {
					"nodes": [],
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

	logCore, observedLogs := observer.New(zap.InfoLevel)
	poller := interfaces.FactoryWorkstationConfig{
		Name:           "linear-ingress",
		Kind:           interfaces.WorkstationKindPoller,
		WorkerTypeName: "linear-poller",
	}
	worker := &interfaces.WorkerConfig{
		Name:     "linear-poller",
		Type:     interfaces.WorkerTypeHosted,
		Provider: interfaces.HostedWorkerProviderLinear,
		Auth:     &interfaces.HostedWorkerAuthConfig{SecretRef: "secrets/linear-api-key"},
		Linear: &interfaces.HostedLinearWorkerConfig{
			PollInterval: "1h",
			Mapping: interfaces.HostedLinearWorkerMappingConfig{
				WorkType: "story",
				State:    "init",
			},
		},
	}
	runtimeCfg, err := config.NewLoadedFactoryConfig(factoryDir, &interfaces.FactoryConfig{}, nil)
	if err != nil {
		t.Fatalf("NewLoadedFactoryConfig: %v", err)
	}

	pollerCfg := Config{
		Logger:         zap.New(logCore),
		HTTPClient:     server.Client(),
		LinearEndpoint: server.URL,
	}

	sidecarCtx, cancel := context.WithCancel(context.Background())
	var sidecars sync.WaitGroup
	StartLinearPoller(sidecarCtx, &sidecars, pollerCfg, runtimeCfg, poller, worker, func(context.Context, interfaces.WorkRequest) error {
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

func TestStartLinearPoller_RestartsOnMissingAuthConfig(t *testing.T) {
	fakeClock := clockwork.NewFakeClock()
	logCore, observedLogs := observer.New(zap.InfoLevel)
	poller := interfaces.FactoryWorkstationConfig{
		Name:           "linear-ingress",
		Kind:           interfaces.WorkstationKindPoller,
		WorkerTypeName: "linear-poller",
	}
	worker := &interfaces.WorkerConfig{
		Name:     "linear-poller",
		Type:     interfaces.WorkerTypeHosted,
		Provider: interfaces.HostedWorkerProviderLinear,
		Linear: &interfaces.HostedLinearWorkerConfig{
			PollInterval: "1h",
			Mapping: interfaces.HostedLinearWorkerMappingConfig{
				WorkType: "story",
				State:    "init",
			},
		},
	}
	runtimeCfg, err := config.NewLoadedFactoryConfig(t.TempDir(), &interfaces.FactoryConfig{}, nil)
	if err != nil {
		t.Fatalf("NewLoadedFactoryConfig: %v", err)
	}

	pollerCfg := Config{
		Logger: zap.New(logCore),
		Clock:  fakeClock,
	}

	sidecarCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var sidecars sync.WaitGroup
	StartLinearPoller(sidecarCtx, &sidecars, pollerCfg, runtimeCfg, poller, worker, func(context.Context, interfaces.WorkRequest) error {
		return nil
	})

	waitForObservedLogMessage(t, observedLogs, "hosted linear poller restarting", time.Second)
	restartEntry := observedLogs.FilterMessage("hosted linear poller restarting").All()[0]
	if got := restartEntry.ContextMap()["error"]; got == nil || !strings.Contains(fieldString(got), "missing auth.secretRef") {
		t.Fatalf("restart error = %#v, want missing auth.secretRef context", got)
	}

	waitForFakeClockWaiters(t, fakeClock, 1)
	fakeClock.Advance(restartBackoffMin)
	waitForObservedLogMessage(t, observedLogs, "hosted linear poller restarting", time.Second)
	if observedLogs.FilterMessage("hosted linear poller restarting").Len() < 2 {
		t.Fatalf("restart log count = %d, want at least 2 after backoff", observedLogs.FilterMessage("hosted linear poller restarting").Len())
	}
}

func waitForSubmitCalls(t *testing.T, submitCalls *int, want int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if *submitCalls >= want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %d submit call(s); got %d", want, *submitCalls)
}

func waitForObservedLogMessage(t *testing.T, logs *observer.ObservedLogs, message string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if logs.FilterMessage(message).Len() > 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for log message %q", message)
}

func waitForFakeClockWaiters(t *testing.T, fakeClock *clockwork.FakeClock, waiters int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := fakeClock.BlockUntilContext(ctx, waiters); err != nil {
		t.Fatalf("timed out waiting for %d fake-clock waiter(s): %v", waiters, err)
	}
}

func fieldString(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case error:
		return typed.Error()
	default:
		return ""
	}
}
