package linear

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.uber.org/zap"

	factorydefinitioncomposition "github.com/portpowered/infinite-you/internal/testutil/factorydefinitionfixtures"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

func checkpointStoreForTest(t testing.TB) CheckpointStore {
	t.Helper()
	store, err := NewCheckpointStore(platformfilesystem.Local{})
	if err != nil {
		t.Fatalf("NewCheckpointStore() error = %v", err)
	}
	return store
}

type renameFailCheckpointFileSystem struct {
	CheckpointFileSystem
	err error
}

func (f renameFailCheckpointFileSystem) Rename(string, string) error { return f.err }

func TestCheckpointStore_RestartCorruptionNotFoundAndAtomicFailure(t *testing.T) {
	if _, err := NewCheckpointStore(nil); err == nil {
		t.Fatal("NewCheckpointStore(nil) error = nil")
	}
	store := checkpointStoreForTest(t)
	path := filepath.Join(t.TempDir(), "checkpoint.json")

	missing, err := store.Load(path)
	if err != nil || missing != (Checkpoint{}) {
		t.Fatalf("Load(missing) = (%#v, %v), want empty checkpoint", missing, err)
	}
	if err := os.WriteFile(path, []byte("not-json"), 0o600); err != nil {
		t.Fatalf("write corrupt checkpoint: %v", err)
	}
	if _, err := store.Load(path); err == nil || !strings.Contains(err.Error(), "decode hosted linear checkpoint") {
		t.Fatalf("Load(corrupt) error = %v", err)
	}

	first := Checkpoint{IssueID: "issue-1", UpdatedAt: "2026-01-01T00:00:00Z"}
	second := Checkpoint{IssueID: "issue-2", UpdatedAt: "2026-01-02T00:00:00Z"}
	if err := store.Save(path, first); err != nil {
		t.Fatalf("Save(first) error = %v", err)
	}
	if err := store.Save(path, second); err != nil {
		t.Fatalf("Save(second) error = %v", err)
	}
	restarted, err := store.Load(path)
	if err != nil || restarted != second {
		t.Fatalf("Load(after restart) = (%#v, %v), want %#v", restarted, err, second)
	}

	renameErr := errors.New("rename checkpoint failed")
	failingStore, err := NewCheckpointStore(renameFailCheckpointFileSystem{
		CheckpointFileSystem: platformfilesystem.Local{}, err: renameErr,
	})
	if err != nil {
		t.Fatalf("NewCheckpointStore(failing) error = %v", err)
	}
	if err := failingStore.Save(path, first); !errors.Is(err, renameErr) {
		t.Fatalf("Save(rename failure) error = %v, want %v", err, renameErr)
	}
	unchanged, err := store.Load(path)
	if err != nil || unchanged != second {
		t.Fatalf("Load(after failed commit) = (%#v, %v), want prior %#v", unchanged, err, second)
	}
}

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

	var submitted []work.WorkRequest
	result, err := RunPollCycle(
		context.Background(),
		Client{Endpoint: server.URL, HTTPClient: server.Client(), Logger: zap.NewNop()},
		nil,
		workstation,
		worker,
		func(_ context.Context, request work.WorkRequest) error {
			submitted = append(submitted, request)
			return nil
		},
		checkpointStoreForTest(t),
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

	assertSubmittedHostedLinearIssues(t, submitted[0], result.Submissions)
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
		func(_ context.Context, request work.WorkRequest) error {
			submitCalls++
			if len(request.Works) != 1 || request.Works[0].WorkID != "linear:issue-new" {
				t.Fatalf("submitted request = %#v, want only newest issue above checkpoint", request)
			}
			return nil
		},
		checkpointStoreForTest(t),
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
		func(_ context.Context, request work.WorkRequest) error {
			t.Fatalf("unexpected resubmission: %#v", request)
			return nil
		},
		checkpointStoreForTest(t),
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

	var submitted []work.WorkRequest
	result, err := RunPollCycle(
		context.Background(),
		Client{Endpoint: server.URL, HTTPClient: server.Client(), Logger: zap.NewNop()},
		nil,
		workstation,
		worker,
		func(_ context.Context, request work.WorkRequest) error {
			submitted = append(submitted, request)
			return nil
		},
		checkpointStoreForTest(t),
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
	if len(submitted[0].Works) != 1 || submitted[0].Works[0].WorkID != "linear:issue-match" {
		t.Fatalf("submitted Work Request = %#v, want only filtered issue", submitted[0])
	}
	if len(result.Submissions) != 1 || result.Submissions[0].WorkID != "linear:issue-match" {
		t.Fatalf("cycle submissions = %#v, want only filtered issue", result.Submissions)
	}
	checkpoint := readLinearCheckpointForTest(t, checkpointPath)
	if checkpoint.IssueID != "issue-match" || checkpoint.UpdatedAt != "2026-05-22T08:00:00Z" {
		t.Fatalf("checkpoint = %#v, want newest filtered issue fingerprint", checkpoint)
	}
}

func TestClientFetchIssuesPageRequiresInjectedHTTPClient(t *testing.T) {
	_, err := (Client{Endpoint: "https://linear.invalid"}).fetchIssuesPage(
		context.Background(), "secret", "", linearIssueFilter{},
	)
	if err == nil || !strings.Contains(err.Error(), "HTTP client is required") {
		t.Fatalf("fetchIssuesPage() error = %v, want injected-client failure", err)
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
	runtimeCfg, err := factorydefinitioncomposition.NewLoadedSource(
		factoryDir,
		&interfaces.FactoryConfig{},
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("NewLoadedFactoryConfig: %v", err)
	}

	envName := SecretEnvName("secrets/linear-api-key")
	if envName == "" || !strings.Contains(envName, "SECRETS_LINEAR_API_KEY") {
		t.Fatalf("SecretEnvName = %q, want normalized name", envName)
	}
	t.Setenv(envName, "env-secret")
	resolver := NewSecretResolver(os.Getenv, os.ReadFile)
	got, err := resolver(context.Background(), runtimeCfg, "secrets/linear-api-key")
	if err != nil {
		t.Fatalf("ResolveSecretRef env: %v", err)
	}
	if got != "env-secret" {
		t.Fatalf("resolved env secret = %q, want env-secret", got)
	}

	t.Setenv(envName, "")
	got, err = resolver(context.Background(), runtimeCfg, "secrets/linear-api-key")
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
) *interfaces.FactoryWorkerConfig {
	worker := &interfaces.FactoryWorkerConfig{
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

func assertSubmittedHostedLinearIssues(
	t *testing.T,
	request work.WorkRequest,
	submissions []work.SubmitRequest,
) {
	t.Helper()

	if request.Type != work.WorkRequestTypeFactoryRequestBatch {
		t.Fatalf("submitted Work Request type = %q, want %q", request.Type, work.WorkRequestTypeFactoryRequestBatch)
	}
	if len(request.Works) != 2 || len(submissions) != 2 {
		t.Fatalf("submitted works = %d and cycle submissions = %d, want 2 each", len(request.Works), len(submissions))
	}
	if request.Works[0].WorkID != "linear:issue-old" || request.Works[1].WorkID != "linear:issue-new" {
		t.Fatalf("Work Request IDs = [%s %s], want oldest-first filtered issues", request.Works[0].WorkID, request.Works[1].WorkID)
	}
	if request.RequestID == "" || submissions[0].RequestID != request.RequestID || submissions[1].RequestID != request.RequestID {
		t.Fatalf("request ID = %q, cycle submission IDs = [%q %q], want one shared non-empty ID", request.RequestID, submissions[0].RequestID, submissions[1].RequestID)
	}
	if request.Works[1].Tags["linear_issue_identifier"] != "ENG-101" || submissions[1].Tags["linear_issue_identifier"] != "ENG-101" {
		t.Fatalf("Work Request tags = %#v and cycle tags = %#v, want identifier tag", request.Works[1].Tags, submissions[1].Tags)
	}

	var payload map[string]any
	if err := json.Unmarshal(submissions[1].Payload, &payload); err != nil {
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
