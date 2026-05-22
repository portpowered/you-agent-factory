package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestRunHostedLinearPollCycle_SubmitsFilteredIssuesAndPersistsCheckpoint(t *testing.T) {
	var authHeaders []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeaders = append(authHeaders, r.Header.Get("Authorization"))
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
							"assignee": {"id": "user-1", "name": "Alex", "email": "alex@example.com"}
						},
						{
							"id": "issue-skip",
							"identifier": "OPS-4",
							"title": "Skip issue",
							"description": "",
							"updatedAt": "2026-05-22T07:05:00Z",
							"url": "https://linear.app/example/issue/OPS-4",
							"team": {"id": "team-2", "key": "OPS", "name": "Operations"},
							"state": {"id": "state-1", "name": "Todo", "type": "unstarted"},
							"assignee": null
						},
						{
							"id": "issue-old",
							"identifier": "ENG-55",
							"title": "Older issue",
							"description": "Second",
							"updatedAt": "2026-05-22T07:00:00Z",
							"url": "https://linear.app/example/issue/ENG-55",
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

	worker := hostedLinearWorkerConfigForTest(
		interfaces.HostedLinearWorkerMappingConfig{WorkType: "story", State: "init"},
		func(cfg *interfaces.HostedLinearWorkerConfig) {
			cfg.TeamIDs = []string{"team-1"}
			cfg.StateIDs = []string{"state-1"}
			cfg.Claim = &interfaces.HostedLinearWorkerClaimConfig{AssigneeField: "ownerEmail"}
		},
	)
	workstation := interfaces.FactoryWorkstationConfig{Name: "linear-ingress"}
	checkpointPath := filepath.Join(t.TempDir(), "checkpoint.json")

	var submitted []interfaces.WorkRequest
	result, err := runHostedLinearPollCycle(
		context.Background(),
		linearPollerClient{endpoint: server.URL, client: server.Client(), logger: zap.NewNop()},
		nil,
		workstation,
		worker,
		func(_ context.Context, request interfaces.WorkRequest) error {
			submitted = append(submitted, request)
			return nil
		},
		checkpointPath,
		"linear-secret-key",
		zap.NewNop(),
	)
	if err != nil {
		t.Fatalf("runHostedLinearPollCycle: %v", err)
	}
	if !result.foundNewer {
		t.Fatal("expected hosted linear cycle to report newer issues")
	}
	if len(submitted) != 1 {
		t.Fatalf("submitted requests = %d, want 1", len(submitted))
	}
	if got := authHeaders; len(got) != 1 || got[0] != "linear-secret-key" {
		t.Fatalf("authorization headers = %#v, want raw API key once", got)
	}

	normalized := normalizeSubmittedLinearWorkRequest(t, submitted[0])
	assertNormalizedHostedLinearIssues(t, normalized)
	checkpoint := readLinearCheckpointForTest(t, checkpointPath)
	if checkpoint.IssueID != "issue-new" || checkpoint.UpdatedAt != "2026-05-22T07:10:00Z" {
		t.Fatalf("checkpoint = %#v, want newest issue fingerprint", checkpoint)
	}
}

func TestRunHostedLinearPollCycle_StopsAtCheckpointAndSkipsResubmission(t *testing.T) {
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
							"description": "",
							"updatedAt": "2026-05-22T07:10:00Z",
							"url": "https://linear.app/example/issue/ENG-101",
							"team": {"id": "team-1", "key": "ENG", "name": "Engineering"},
							"state": {"id": "state-1", "name": "Todo", "type": "unstarted"},
							"assignee": null
						},
						{
							"id": "issue-old",
							"identifier": "ENG-55",
							"title": "Older issue",
							"description": "",
							"updatedAt": "2026-05-22T07:00:00Z",
							"url": "https://linear.app/example/issue/ENG-55",
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

	worker := hostedLinearWorkerConfigForTest(
		interfaces.HostedLinearWorkerMappingConfig{WorkType: "story", State: "init"},
		nil,
	)
	workstation := interfaces.FactoryWorkstationConfig{Name: "linear-ingress"}
	checkpointPath := filepath.Join(t.TempDir(), "checkpoint.json")
	if err := saveLinearCheckpoint(checkpointPath, linearCheckpoint{
		IssueID:   "issue-old",
		UpdatedAt: "2026-05-22T07:00:00Z",
	}); err != nil {
		t.Fatalf("seed checkpoint: %v", err)
	}

	submitCalls := 0
	first, err := runHostedLinearPollCycle(
		context.Background(),
		linearPollerClient{endpoint: server.URL, client: server.Client(), logger: zap.NewNop()},
		nil,
		workstation,
		worker,
		func(_ context.Context, request interfaces.WorkRequest) error {
			submitCalls++
			if len(request.Works) != 1 || request.Works[0].WorkID != "linear:issue-new" {
				t.Fatalf("submitted request = %#v, want only newest issue above checkpoint", request)
			}
			return nil
		},
		checkpointPath,
		"linear-secret-key",
		zap.NewNop(),
	)
	if err != nil {
		t.Fatalf("first runHostedLinearPollCycle: %v", err)
	}
	if !first.foundNewer || submitCalls != 1 {
		t.Fatalf("first cycle foundNewer=%t submitCalls=%d, want true and 1", first.foundNewer, submitCalls)
	}

	second, err := runHostedLinearPollCycle(
		context.Background(),
		linearPollerClient{endpoint: server.URL, client: server.Client(), logger: zap.NewNop()},
		nil,
		workstation,
		worker,
		func(_ context.Context, request interfaces.WorkRequest) error {
			t.Fatalf("unexpected resubmission: %#v", request)
			return nil
		},
		checkpointPath,
		"linear-secret-key",
		zap.NewNop(),
	)
	if err != nil {
		t.Fatalf("second runHostedLinearPollCycle: %v", err)
	}
	if second.foundNewer {
		t.Fatal("expected second cycle to stop at checkpoint with no newer issues")
	}
}

func TestRunHostedLinearPollCycle_PushesFiltersIntoProviderQueryForBoundedResume(t *testing.T) {
	var capturedQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		var payload struct {
			Query string `json:"query"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		capturedQuery = payload.Query

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"data": {
				"issues": {
					"nodes": [
						{
							"id": "issue-match",
							"identifier": "ENG-144",
							"title": "Filtered issue",
							"description": "Visible only when provider-side filters apply",
							"updatedAt": "2026-05-22T08:00:00Z",
							"url": "https://linear.app/example/issue/ENG-144",
							"team": {"id": "team-match", "key": "ENG", "name": "Engineering"},
							"state": {"id": "state-match", "name": "Todo", "type": "unstarted"},
							"assignee": null
						}
					],
					"pageInfo": {"hasNextPage": false, "endCursor": ""}
				}
			}
		}`))
	}))
	defer server.Close()

	worker := hostedLinearWorkerConfigForTest(
		interfaces.HostedLinearWorkerMappingConfig{WorkType: "story", State: "init"},
		func(cfg *interfaces.HostedLinearWorkerConfig) {
			cfg.TeamIDs = []string{"team-match"}
			cfg.StateIDs = []string{"state-match"}
		},
	)
	workstation := interfaces.FactoryWorkstationConfig{Name: "linear-ingress"}
	checkpointPath := filepath.Join(t.TempDir(), "checkpoint.json")
	if err := saveLinearCheckpoint(checkpointPath, linearCheckpoint{
		IssueID:   "issue-older-match",
		UpdatedAt: "2026-05-22T07:00:00Z",
	}); err != nil {
		t.Fatalf("seed checkpoint: %v", err)
	}

	var submitted []interfaces.WorkRequest
	result, err := runHostedLinearPollCycle(
		context.Background(),
		linearPollerClient{endpoint: server.URL, client: server.Client(), logger: zap.NewNop()},
		nil,
		workstation,
		worker,
		func(_ context.Context, request interfaces.WorkRequest) error {
			submitted = append(submitted, request)
			return nil
		},
		checkpointPath,
		"linear-secret-key",
		zap.NewNop(),
	)
	if err != nil {
		t.Fatalf("runHostedLinearPollCycle: %v", err)
	}
	if !strings.Contains(capturedQuery, `team: { id: { in: ["team-match"] } }`) {
		t.Fatalf("query = %q, want team filter pushed into provider request", capturedQuery)
	}
	if !strings.Contains(capturedQuery, `state: { id: { in: ["state-match"] } }`) {
		t.Fatalf("query = %q, want state filter pushed into provider request", capturedQuery)
	}
	if !result.foundNewer {
		t.Fatal("expected hosted linear cycle to report newer filtered issues")
	}
	if len(submitted) != 1 {
		t.Fatalf("submitted requests = %d, want 1", len(submitted))
	}
	normalized := normalizeSubmittedLinearWorkRequest(t, submitted[0])
	if len(normalized) != 1 || normalized[0].WorkID != "linear:issue-match" {
		t.Fatalf("normalized submissions = %#v, want only filtered issue", normalized)
	}
	checkpoint := readLinearCheckpointForTest(t, checkpointPath)
	if checkpoint.IssueID != "issue-match" || checkpoint.UpdatedAt != "2026-05-22T08:00:00Z" {
		t.Fatalf("checkpoint = %#v, want newest filtered issue fingerprint", checkpoint)
	}
}

func hostedLinearWorkerConfigForTest(
	mapping interfaces.HostedLinearWorkerMappingConfig,
	mutate func(*interfaces.HostedLinearWorkerConfig),
) *interfaces.WorkerConfig {
	worker := &interfaces.WorkerConfig{
		Name:     "linear-poller",
		Type:     interfaces.WorkerTypeHosted,
		Provider: interfaces.HostedWorkerProviderLinear,
		Linear: &interfaces.HostedLinearWorkerConfig{
			Mapping: mapping,
		},
	}
	if mutate != nil {
		mutate(worker.Linear)
	}
	return worker
}

func normalizeSubmittedLinearWorkRequest(t *testing.T, request interfaces.WorkRequest) []interfaces.SubmitRequest {
	t.Helper()

	normalized, err := factory.NormalizeWorkRequest(request, interfaces.WorkRequestNormalizeOptions{})
	if err != nil {
		t.Fatalf("NormalizeWorkRequest: %v", err)
	}
	return normalized
}

func assertNormalizedHostedLinearIssues(t *testing.T, normalized []interfaces.SubmitRequest) {
	t.Helper()

	if len(normalized) != 2 {
		t.Fatalf("normalized submissions = %d, want 2 filtered issues", len(normalized))
	}
	if normalized[0].WorkID != "linear:issue-old" || normalized[1].WorkID != "linear:issue-new" {
		t.Fatalf("normalized work IDs = [%s %s], want oldest-first filtered issues", normalized[0].WorkID, normalized[1].WorkID)
	}
	if normalized[0].RequestID != normalized[1].RequestID || normalized[0].RequestID == "" {
		t.Fatalf("deterministic batch request IDs = [%q %q], want shared non-empty ID", normalized[0].RequestID, normalized[1].RequestID)
	}
	if normalized[1].Tags["linear_issue_identifier"] != "ENG-101" {
		t.Fatalf("linear tags = %#v, want identifier tag", normalized[1].Tags)
	}

	var payload map[string]any
	if err := json.Unmarshal(normalized[1].Payload, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	claims, _ := payload["claims"].(map[string]any)
	if claims["ownerEmail"] != "alex@example.com" {
		t.Fatalf("claims = %#v, want ownerEmail claim", claims)
	}
}

func readLinearCheckpointForTest(t *testing.T, checkpointPath string) linearCheckpoint {
	t.Helper()

	checkpointData, err := os.ReadFile(checkpointPath)
	if err != nil {
		t.Fatalf("read checkpoint: %v", err)
	}

	var checkpoint linearCheckpoint
	if err := json.Unmarshal(checkpointData, &checkpoint); err != nil {
		t.Fatalf("decode checkpoint: %v", err)
	}
	return checkpoint
}

func TestFactoryService_StartLiveRuntimeSidecars_StartsHostedLinearPoller(t *testing.T) {
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

	submitted := &aggregateSnapshotFactory{}
	svc := &FactoryService{
		cfg: &FactoryServiceConfig{
			RuntimeMode:            interfaces.RuntimeModeService,
			HostedPollerHTTPClient: server.Client(),
			HostedLinearEndpoint:   server.URL,
		},
		logger: zap.NewNop(),
	}
	poller := interfaces.FactoryWorkstationConfig{
		Name:           "linear-ingress",
		Kind:           interfaces.WorkstationKindPoller,
		WorkerTypeName: "linear-poller",
	}
	runtimeCfg := newLoadedFactoryConfigForServiceTest(
		t,
		factoryDir,
		&interfaces.FactoryConfig{
			Workers:      []interfaces.WorkerConfig{{Name: "linear-poller"}},
			Workstations: []interfaces.FactoryWorkstationConfig{poller},
		},
		map[string]*interfaces.WorkerConfig{
			"linear-poller": {
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
			},
		},
		map[string]*interfaces.FactoryWorkstationConfig{poller.Name: &poller},
	)
	handle := &liveRuntimeHandle{
		runtime: &replacementFactoryRuntime{
			runtimeCfg: runtimeCfg,
			factory:    submitted,
		},
	}
	sidecarCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := svc.startLiveRuntimeSidecars(sidecarCtx, handle); err != nil {
		t.Fatalf("startLiveRuntimeSidecars: %v", err)
	}
	defer svc.stopLiveRuntimeSidecars(handle)

	waitForHostedPollerSubmission(t, submitted, 1, time.Second)
	if submitted.submitCalls != 1 {
		t.Fatalf("submit calls = %d, want 1", submitted.submitCalls)
	}
	if got := submitted.submissions[0].Works[0].WorkID; got != "linear:issue-new" {
		t.Fatalf("submitted work id = %q, want linear:issue-new", got)
	}
}

func TestFactoryService_StopLiveRuntimeSidecars_StopsHostedLinearPollerAndLogsLifecycle(t *testing.T) {
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
	svc := &FactoryService{
		cfg: &FactoryServiceConfig{
			RuntimeMode:            interfaces.RuntimeModeService,
			HostedPollerHTTPClient: server.Client(),
			HostedLinearEndpoint:   server.URL,
		},
		logger: zap.New(logCore),
	}
	poller := interfaces.FactoryWorkstationConfig{
		Name:           "linear-ingress",
		Kind:           interfaces.WorkstationKindPoller,
		WorkerTypeName: "linear-poller",
	}
	runtimeCfg := newLoadedFactoryConfigForServiceTest(
		t,
		factoryDir,
		&interfaces.FactoryConfig{
			Workers:      []interfaces.WorkerConfig{{Name: "linear-poller"}},
			Workstations: []interfaces.FactoryWorkstationConfig{poller},
		},
		map[string]*interfaces.WorkerConfig{
			"linear-poller": {
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
			},
		},
		map[string]*interfaces.FactoryWorkstationConfig{poller.Name: &poller},
	)
	handle := &liveRuntimeHandle{
		runtime: &replacementFactoryRuntime{
			runtimeCfg: runtimeCfg,
			factory:    &aggregateSnapshotFactory{},
		},
	}

	if err := svc.startLiveRuntimeSidecars(context.Background(), handle); err != nil {
		t.Fatalf("startLiveRuntimeSidecars: %v", err)
	}

	waitForObservedLogMessage(t, observedLogs, "hosted linear poller started", time.Second)
	svc.stopLiveRuntimeSidecars(handle)

	stopped := observedLogs.FilterMessage("hosted linear poller stopped").All()
	if len(stopped) != 1 {
		t.Fatalf("hosted linear poller stopped log count = %d, want 1", len(stopped))
	}
	if got := fieldString(stopped[0].ContextMap()["reason"]); got != "context canceled" {
		t.Fatalf("hosted linear poller stop reason = %q, want context canceled", got)
	}
}

func TestResolveHostedSecretRef_PrefersEnvThenRuntimeFile(t *testing.T) {
	factoryDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(factoryDir, "secrets"), 0o755); err != nil {
		t.Fatalf("mkdir secrets: %v", err)
	}
	if err := os.WriteFile(filepath.Join(factoryDir, "secrets", "linear-api-key"), []byte("file-secret"), 0o600); err != nil {
		t.Fatalf("write secret file: %v", err)
	}
	runtimeCfg := newLoadedFactoryConfigForServiceTest(t, factoryDir, &interfaces.FactoryConfig{}, nil, nil)

	envName := hostedSecretEnvName("secrets/linear-api-key")
	if envName == "" || !strings.Contains(envName, "SECRETS_LINEAR_API_KEY") {
		t.Fatalf("hostedSecretEnvName = %q, want normalized name", envName)
	}
	t.Setenv(envName, "env-secret")
	got, err := resolveHostedSecretRef(context.Background(), runtimeCfg, "secrets/linear-api-key")
	if err != nil {
		t.Fatalf("resolveHostedSecretRef env: %v", err)
	}
	if got != "env-secret" {
		t.Fatalf("resolved env secret = %q, want env-secret", got)
	}

	t.Setenv(envName, "")
	got, err = resolveHostedSecretRef(context.Background(), runtimeCfg, "secrets/linear-api-key")
	if err != nil {
		t.Fatalf("resolveHostedSecretRef file: %v", err)
	}
	if got != "file-secret" {
		t.Fatalf("resolved file secret = %q, want file-secret", got)
	}
}

func waitForHostedPollerSubmission(t *testing.T, submitted *aggregateSnapshotFactory, want int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if submitted.submitCalls >= want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %d hosted poller submission(s); got %d", want, submitted.submitCalls)
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
