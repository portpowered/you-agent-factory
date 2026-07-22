package hostedworkers

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
	factorydefinitioncomposition "github.com/portpowered/infinite-you/internal/testutil/factorydefinitionfixtures"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	hostedlinear "github.com/portpowered/infinite-you/pkg/services/workers/services/hosted_logic/linear"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

type Config struct {
	Logger         *zap.Logger
	Clock          clockwork.Clock
	HTTPClient     *http.Client
	SecretResolver hostedlinear.SecretResolver
	Checkpoints    hostedlinear.CheckpointStore
	LinearEndpoint string
}

func startLinearPollerWithConfig(
	ctx context.Context,
	sidecars *sync.WaitGroup,
	cfg Config,
	runtimeConfig interfaces.RuntimeConfigLookup,
	workstation interfaces.FactoryWorkstationConfig,
	worker *interfaces.FactoryWorkerConfig,
	submitter Submitter,
) error {
	if cfg.Logger == nil {
		cfg.Logger = zap.NewNop()
	}
	if cfg.Clock == nil {
		cfg.Clock = clockwork.NewFakeClock()
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{Timeout: hostedlinear.DefaultRequestTimeout}
	}
	if cfg.SecretResolver == nil {
		cfg.SecretResolver = hostedlinear.NewSecretResolver(os.Getenv, os.ReadFile)
	}
	if cfg.Checkpoints == nil {
		var err error
		cfg.Checkpoints, err = hostedlinear.NewCheckpointStore(platformfilesystem.Local{})
		if err != nil {
			return err
		}
	}
	return StartLinearPoller(
		ctx, sidecars, cfg.Logger, cfg.Clock, cfg.HTTPClient,
		cfg.SecretResolver, cfg.Checkpoints, cfg.LinearEndpoint,
		runtimeConfig, workstation, worker, submitter,
	)
}

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

	var submitCalls atomic.Int32
	var submittedWorkID string
	submitter := Submitter(func(_ context.Context, request work.WorkRequest) error {
		submitCalls.Add(1)
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
	worker := &interfaces.FactoryWorkerConfig{
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
	runtimeCfg, err := factorydefinitioncomposition.NewLoadedSource(
		factoryDir,
		&interfaces.FactoryConfig{
			Workers:      []interfaces.FactoryWorkerConfig{{Name: worker.Name}},
			Workstations: []interfaces.FactoryWorkstationConfig{poller},
		},
		nil,
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
	startLinearPollerWithConfig(sidecarCtx, &sidecars, pollerCfg, runtimeCfg, poller, worker, submitter)

	waitForSubmitCalls(t, &submitCalls, 1, time.Second)
	cancel()
	sidecars.Wait()

	if got := submitCalls.Load(); got != 1 {
		t.Fatalf("submit calls = %d, want 1", got)
	}
	if submittedWorkID != "linear:issue-new" {
		t.Fatalf("submitted work id = %q, want linear:issue-new", submittedWorkID)
	}
}

func TestStartHostedLinearPoller_StopsOnContextCancellation(t *testing.T) {
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
	worker := &interfaces.FactoryWorkerConfig{
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
	runtimeCfg, err := factorydefinitioncomposition.NewLoadedSource(
		factoryDir,
		&interfaces.FactoryConfig{},
		nil,
		nil,
	)
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
	startLinearPollerWithConfig(sidecarCtx, &sidecars, pollerCfg, runtimeCfg, poller, worker, func(context.Context, work.WorkRequest) error {
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

func TestStartLinearPoller_RejectsMissingAuthBeforeStarting(t *testing.T) {
	poller := interfaces.FactoryWorkstationConfig{
		Name:           "linear-ingress",
		Kind:           interfaces.WorkstationKindPoller,
		WorkerTypeName: "linear-poller",
	}
	worker := &interfaces.FactoryWorkerConfig{
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
	runtimeCfg, err := factorydefinitioncomposition.NewLoadedSource(
		t.TempDir(),
		&interfaces.FactoryConfig{},
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("NewLoadedFactoryConfig: %v", err)
	}

	var sidecars sync.WaitGroup
	err = startLinearPollerWithConfig(context.Background(), &sidecars, Config{}, runtimeCfg, poller, worker, func(context.Context, work.WorkRequest) error {
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "auth.secretRef is required") {
		t.Fatalf("StartLinearPoller() error = %v, want auth dependency validation", err)
	}
	sidecars.Wait()
}

func TestNewLinearPoller_RequiresExternalEffects(t *testing.T) {
	runtimeCfg, err := factorydefinitioncomposition.NewLoadedSource(
		t.TempDir(),
		&interfaces.FactoryConfig{},
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("NewLoadedFactoryConfig: %v", err)
	}
	worker := &interfaces.FactoryWorkerConfig{
		Name: "linear-poller", Auth: &interfaces.HostedWorkerAuthConfig{SecretRef: "linear-key"},
		Linear: &interfaces.HostedLinearWorkerConfig{},
	}
	clock := clockwork.NewFakeClock()
	client := &http.Client{Timeout: hostedlinear.DefaultRequestTimeout}
	resolver := hostedlinear.NewSecretResolver(os.Getenv, os.ReadFile)
	checkpoints, err := hostedlinear.NewCheckpointStore(platformfilesystem.Local{})
	if err != nil {
		t.Fatalf("NewCheckpointStore() error = %v", err)
	}
	for _, tc := range []struct {
		name        string
		clock       Clock
		client      hostedlinear.HTTPDoer
		resolver    hostedlinear.SecretResolver
		checkpoints hostedlinear.CheckpointStore
		want        string
	}{
		{name: "clock", client: client, resolver: resolver, checkpoints: checkpoints, want: "clock is required"},
		{name: "HTTP client", clock: clock, resolver: resolver, checkpoints: checkpoints, want: "HTTP client is required"},
		{name: "secret resolver", clock: clock, client: client, checkpoints: checkpoints, want: "secret resolver is required"},
		{name: "checkpoint store", clock: clock, client: client, resolver: resolver, want: "checkpoint store is required"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewLinearPoller(
				zap.NewNop(), tc.clock, tc.client, tc.resolver, tc.checkpoints, "", runtimeCfg,
				interfaces.FactoryWorkstationConfig{}, worker,
				func(context.Context, work.WorkRequest) error { return nil },
			)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("NewLinearPoller() error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestStartLinearPoller_RedactsResolvedSecretFromProviderErrors(t *testing.T) {
	const secret = "resolved-linear-secret"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = fmt.Fprintf(w, `{"errors":[{"message":"provider echoed %s"}]}`, secret)
	}))
	defer server.Close()

	logCore, observedLogs := observer.New(zap.InfoLevel)
	factoryDir := t.TempDir()
	pollerCfg, runtimeCfg, workstation, worker := hostedLinearPollerFixtureForTest(t, factoryDir, server, nil)
	pollerCfg.Logger = zap.New(logCore)
	pollerCfg.Clock = clockwork.NewFakeClock()
	pollerCfg.SecretResolver = func(context.Context, workers.HostedRuntimePaths, string) (string, error) {
		return secret, nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	var sidecars sync.WaitGroup
	if err := startLinearPollerWithConfig(ctx, &sidecars, pollerCfg, runtimeCfg, workstation, worker, func(context.Context, work.WorkRequest) error { return nil }); err != nil {
		t.Fatalf("StartLinearPoller() error = %v", err)
	}
	waitForObservedLogMessage(t, observedLogs, "hosted linear poller restarting", time.Second)
	cancel()
	sidecars.Wait()
	logged := fieldString(observedLogs.FilterMessage("hosted linear poller restarting").All()[0].ContextMap()["error"])
	if strings.Contains(logged, secret) || !strings.Contains(logged, "[REDACTED]") {
		t.Fatalf("logged provider error = %q, want resolved secret redacted", logged)
	}
}

func TestStartLinearPoller_KeepsPollingOverTime(t *testing.T) {
	fakeClock := clockwork.NewFakeClock()
	var requestCount int
	var requestMu sync.Mutex

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestMu.Lock()
		requestCount++
		call := requestCount
		requestMu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		switch call {
		case 1:
			_, _ = w.Write([]byte(mockLinearIssuesGraphQLResponse(mockLinearIssue{
				ID: "issue-1", Identifier: "ENG-1", Title: "First issue", UpdatedAt: "2026-05-22T07:00:00Z",
			})))
		default:
			_, _ = w.Write([]byte(mockLinearIssuesGraphQLResponse(mockLinearIssue{
				ID: "issue-2", Identifier: "ENG-2", Title: "Second issue", UpdatedAt: "2026-05-22T08:00:00Z",
			})))
		}
	}))
	defer server.Close()

	factoryDir := t.TempDir()
	writeHostedLinearSecretForTest(t, factoryDir)

	var submitCalls atomic.Int32
	var submittedWorkIDs []string
	var submitMu sync.Mutex
	submitter := Submitter(func(_ context.Context, request work.WorkRequest) error {
		submitMu.Lock()
		defer submitMu.Unlock()
		submitCalls.Add(1)
		for _, work := range request.Works {
			submittedWorkIDs = append(submittedWorkIDs, work.WorkID)
		}
		return nil
	})

	pollerCfg, runtimeCfg, poller, worker := hostedLinearPollerFixtureForTest(
		t,
		factoryDir,
		server,
		func(cfg *interfaces.HostedLinearWorkerConfig) {
			cfg.PollInterval = "50ms"
		},
	)
	pollerCfg.Clock = fakeClock

	sidecarCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var sidecars sync.WaitGroup
	startLinearPollerWithConfig(sidecarCtx, &sidecars, pollerCfg, runtimeCfg, poller, worker, submitter)

	waitForSubmitCalls(t, &submitCalls, 1, time.Second)
	waitForFakeClockWaiters(t, fakeClock, 1)
	fakeClock.Advance(50 * time.Millisecond)
	waitForSubmitCalls(t, &submitCalls, 2, time.Second)

	cancel()
	sidecars.Wait()

	submitMu.Lock()
	defer submitMu.Unlock()
	if len(submittedWorkIDs) != 2 {
		t.Fatalf("submitted work IDs = %#v, want two poll cycles", submittedWorkIDs)
	}
	if submittedWorkIDs[0] != "linear:issue-1" || submittedWorkIDs[1] != "linear:issue-2" {
		t.Fatalf("submitted work IDs = %#v, want linear:issue-1 then linear:issue-2", submittedWorkIDs)
	}
}

func TestStartLinearPoller_RestartsOnProviderHTTPFailure(t *testing.T) {
	fakeClock := clockwork.NewFakeClock()
	var requestCount int
	var requestMu sync.Mutex
	logCore, observedLogs := observer.New(zap.InfoLevel)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestMu.Lock()
		requestCount++
		call := requestCount
		requestMu.Unlock()

		if call == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"errors":[{"message":"temporary provider failure"}]}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(mockLinearIssuesGraphQLResponse(mockLinearIssue{
			ID: "issue-recovered", Identifier: "ENG-99", Title: "Recovered issue", UpdatedAt: "2026-05-22T09:00:00Z",
		})))
	}))
	defer server.Close()

	factoryDir := t.TempDir()
	writeHostedLinearSecretForTest(t, factoryDir)

	var submitCalls atomic.Int32
	submitter := Submitter(func(_ context.Context, request work.WorkRequest) error {
		submitCalls.Add(1)
		return nil
	})

	pollerCfg, runtimeCfg, poller, worker := hostedLinearPollerFixtureForTest(t, factoryDir, server, nil)
	pollerCfg.Logger = zap.New(logCore)
	pollerCfg.Clock = fakeClock

	sidecarCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var sidecars sync.WaitGroup
	startLinearPollerWithConfig(sidecarCtx, &sidecars, pollerCfg, runtimeCfg, poller, worker, submitter)

	waitForObservedLogMessage(t, observedLogs, "hosted linear poller restarting", time.Second)
	restartEntry := observedLogs.FilterMessage("hosted linear poller restarting").All()[0]
	if got := fieldString(restartEntry.ContextMap()["error"]); got == "" || !strings.Contains(got, "temporary provider failure") {
		t.Fatalf("restart error = %#v, want provider HTTP-edge failure", restartEntry.ContextMap()["error"])
	}

	waitForFakeClockWaiters(t, fakeClock, 1)
	fakeClock.Advance(restartBackoffMin)
	waitForSubmitCalls(t, &submitCalls, 1, time.Second)

	cancel()
	sidecars.Wait()
}

type mockLinearIssue struct {
	ID          string
	Identifier  string
	Title       string
	UpdatedAt   string
	Description string
}

func mockLinearIssuesGraphQLResponse(issues ...mockLinearIssue) string {
	nodes := make([]string, 0, len(issues))
	for _, issue := range issues {
		description := issue.Description
		if description == "" {
			description = issue.Title
		}
		nodes = append(nodes, fmt.Sprintf(`{
			"id": %q,
			"identifier": %q,
			"title": %q,
			"description": %q,
			"updatedAt": %q,
			"url": %q,
			"team": {"id": "team-1", "key": "ENG", "name": "Engineering"},
			"state": {"id": "state-1", "name": "Todo", "type": "unstarted"},
			"assignee": null
		}`, issue.ID, issue.Identifier, issue.Title, description, issue.UpdatedAt,
			fmt.Sprintf("https://linear.app/example/issue/%s", issue.Identifier)))
	}
	return fmt.Sprintf(`{
		"data": {
			"issues": {
				"nodes": [%s],
				"pageInfo": {"hasNextPage": false, "endCursor": ""}
			}
		}
	}`, strings.Join(nodes, ","))
}

func writeHostedLinearSecretForTest(t *testing.T, factoryDir string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(factoryDir, "secrets"), 0o755); err != nil {
		t.Fatalf("mkdir secrets: %v", err)
	}
	if err := os.WriteFile(filepath.Join(factoryDir, "secrets", "linear-api-key"), []byte("runtime-linear-key\n"), 0o600); err != nil {
		t.Fatalf("write secret file: %v", err)
	}
}

func hostedLinearPollerFixtureForTest(
	t *testing.T,
	factoryDir string,
	server *httptest.Server,
	mutateLinear func(*interfaces.HostedLinearWorkerConfig),
) (
	Config,
	interfaces.MutableLoadedFactorySource,
	interfaces.FactoryWorkstationConfig,
	*interfaces.FactoryWorkerConfig,
) {
	t.Helper()
	poller := interfaces.FactoryWorkstationConfig{
		Name:           "linear-ingress",
		Kind:           interfaces.WorkstationKindPoller,
		WorkerTypeName: "linear-poller",
	}
	linearCfg := &interfaces.HostedLinearWorkerConfig{
		PollInterval: "1h",
		Mapping: interfaces.HostedLinearWorkerMappingConfig{
			WorkType: "story",
			State:    "init",
		},
	}
	if mutateLinear != nil {
		mutateLinear(linearCfg)
	}
	worker := &interfaces.FactoryWorkerConfig{
		Name:     "linear-poller",
		Type:     interfaces.WorkerTypeHosted,
		Provider: interfaces.HostedWorkerProviderLinear,
		Auth:     &interfaces.HostedWorkerAuthConfig{SecretRef: "secrets/linear-api-key"},
		Linear:   linearCfg,
	}
	runtimeCfg, err := factorydefinitioncomposition.NewLoadedSource(
		factoryDir,
		&interfaces.FactoryConfig{
			Workers:      []interfaces.FactoryWorkerConfig{{Name: worker.Name}},
			Workstations: []interfaces.FactoryWorkstationConfig{poller},
		},
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("NewLoadedFactoryConfig: %v", err)
	}
	return Config{
		Logger:         zap.NewNop(),
		HTTPClient:     server.Client(),
		LinearEndpoint: server.URL,
	}, runtimeCfg, poller, worker
}

func waitForSubmitCalls(t *testing.T, submitCalls *atomic.Int32, want int32, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if submitCalls.Load() >= want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %d submit call(s); got %d", want, submitCalls.Load())
}

func waitForObservedLogMessage(t *testing.T, logs *observer.ObservedLogs, message string, timeout time.Duration) {
	t.Helper()
	waitForObservedLogCount(t, logs, message, 1, timeout)
}

func waitForObservedLogCount(t *testing.T, logs *observer.ObservedLogs, message string, want int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if logs.FilterMessage(message).Len() >= want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %d log message(s) %q; got %d", want, message, logs.FilterMessage(message).Len())
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
