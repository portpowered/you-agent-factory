package hostedlinear

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

	"github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factory/requests"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"go.uber.org/zap"
)

func TestRunPollCycle_SubmitsFilteredIssuesAndPersistsCheckpoint(t *testing.T) {
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

	worker := linearWorkerConfigForTest(
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
	result, err := RunPollCycle(
		context.Background(),
		Client{Endpoint: server.URL, HTTPClient: server.Client(), Logger: zap.NewNop()},
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
		t.Fatalf("RunPollCycle: %v", err)
	}
	if !result.FoundNewer {
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

func TestRunPollCycle_StopsAtCheckpointAndSkipsResubmission(t *testing.T) {
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

	worker := linearWorkerConfigForTest(
		interfaces.HostedLinearWorkerMappingConfig{WorkType: "story", State: "init"},
		nil,
	)
	workstation := interfaces.FactoryWorkstationConfig{Name: "linear-ingress"}
	checkpointPath := filepath.Join(t.TempDir(), "checkpoint.json")
	writeLinearCheckpointForTest(t, checkpointPath, Checkpoint{
		IssueID:   "issue-old",
		UpdatedAt: "2026-05-22T07:00:00Z",
	})

	submitCalls := 0
	first, err := RunPollCycle(
		context.Background(),
		Client{Endpoint: server.URL, HTTPClient: server.Client(), Logger: zap.NewNop()},
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
		t.Fatalf("first RunPollCycle: %v", err)
	}
	if !first.FoundNewer || submitCalls != 1 {
		t.Fatalf("first cycle foundNewer=%t submitCalls=%d, want true and 1", first.FoundNewer, submitCalls)
	}

	second, err := RunPollCycle(
		context.Background(),
		Client{Endpoint: server.URL, HTTPClient: server.Client(), Logger: zap.NewNop()},
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
		t.Fatalf("second RunPollCycle: %v", err)
	}
	if second.FoundNewer {
		t.Fatal("expected second cycle to stop at checkpoint with no newer issues")
	}
}

func TestRunPollCycle_PushesFiltersIntoProviderQueryForBoundedResume(t *testing.T) {
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

	worker := linearWorkerConfigForTest(
		interfaces.HostedLinearWorkerMappingConfig{WorkType: "story", State: "init"},
		func(cfg *interfaces.HostedLinearWorkerConfig) {
			cfg.TeamIDs = []string{"team-match"}
			cfg.StateIDs = []string{"state-match"}
		},
	)
	workstation := interfaces.FactoryWorkstationConfig{Name: "linear-ingress"}
	checkpointPath := filepath.Join(t.TempDir(), "checkpoint.json")
	writeLinearCheckpointForTest(t, checkpointPath, Checkpoint{
		IssueID:   "issue-older-match",
		UpdatedAt: "2026-05-22T07:00:00Z",
	})

	var submitted []interfaces.WorkRequest
	result, err := RunPollCycle(
		context.Background(),
		Client{Endpoint: server.URL, HTTPClient: server.Client(), Logger: zap.NewNop()},
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
		t.Fatalf("RunPollCycle: %v", err)
	}
	if !strings.Contains(capturedQuery, `team: { id: { in: ["team-match"] } }`) {
		t.Fatalf("query = %q, want team filter pushed into provider request", capturedQuery)
	}
	if !strings.Contains(capturedQuery, `state: { id: { in: ["state-match"] } }`) {
		t.Fatalf("query = %q, want state filter pushed into provider request", capturedQuery)
	}
	if !result.FoundNewer {
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

func TestResolveSecretRef_PrefersEnvThenRuntimeFile(t *testing.T) {
	factoryDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(factoryDir, "secrets"), 0o755); err != nil {
		t.Fatalf("mkdir secrets: %v", err)
	}
	if err := os.WriteFile(filepath.Join(factoryDir, "secrets", "linear-api-key"), []byte("file-secret"), 0o600); err != nil {
		t.Fatalf("write secret file: %v", err)
	}
	runtimeCfg, err := config.NewLoadedFactoryConfig(factoryDir, &interfaces.FactoryConfig{}, nil)
	if err != nil {
		t.Fatalf("NewLoadedFactoryConfig: %v", err)
	}

	envName := SecretEnvName("secrets/linear-api-key")
	if envName == "" || !strings.Contains(envName, "SECRETS_LINEAR_API_KEY") {
		t.Fatalf("SecretEnvName = %q, want normalized name", envName)
	}
	t.Setenv(envName, "env-secret")
	got, err := ResolveSecretRef(context.Background(), runtimeCfg, "secrets/linear-api-key")
	if err != nil {
		t.Fatalf("ResolveSecretRef env: %v", err)
	}
	if got != "env-secret" {
		t.Fatalf("resolved env secret = %q, want env-secret", got)
	}

	t.Setenv(envName, "")
	got, err = ResolveSecretRef(context.Background(), runtimeCfg, "secrets/linear-api-key")
	if err != nil {
		t.Fatalf("ResolveSecretRef file: %v", err)
	}
	if got != "file-secret" {
		t.Fatalf("resolved file secret = %q, want file-secret", got)
	}
}

func linearWorkerConfigForTest(
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

	normalized, err := requests.NormalizeWorkRequest(request, interfaces.WorkRequestNormalizeOptions{})
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

func writeLinearCheckpointForTest(t *testing.T, checkpointPath string, checkpoint Checkpoint) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(checkpointPath), 0o755); err != nil {
		t.Fatalf("create checkpoint dir: %v", err)
	}
	data, err := json.Marshal(checkpoint)
	if err != nil {
		t.Fatalf("marshal checkpoint: %v", err)
	}
	if err := os.WriteFile(checkpointPath, data, 0o600); err != nil {
		t.Fatalf("write checkpoint: %v", err)
	}
}

func readLinearCheckpointForTest(t *testing.T, checkpointPath string) Checkpoint {
	t.Helper()

	checkpointData, err := os.ReadFile(checkpointPath)
	if err != nil {
		t.Fatalf("read checkpoint: %v", err)
	}

	var checkpoint Checkpoint
	if err := json.Unmarshal(checkpointData, &checkpoint); err != nil {
		t.Fatalf("decode checkpoint: %v", err)
	}
	return checkpoint
}
