package details

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	codexGoldenExpectedProviderSessionDetailFile = "expected-provider-session-detail.json"
	codexGoldenRolloutFile                       = "rollout.jsonl"
	codexMissingTranscriptSessionID              = "session_fixture_codex_missing_transcript"
)

// TestCodexProviderSessionDetailsLoadFromGoldenMetadata loads a sanitized Codex
// success rollout through the public Provider Session detail surface and proves
// the response structurally matches checked-in expected Provider Session metadata.
//golden: docs/temp/functional/provider-sessions/codex/success/manifest.json
func TestCodexProviderSessionDetailsLoadFromGoldenMetadata(t *testing.T) {
	repoRoot := testutil.MustRepoRoot(t)
	caseDir := filepath.Join(repoRoot, filepath.FromSlash(support.ProviderSessionFixturePath("codex", "success")))

	loaded, err := support.LoadProviderSessionCase(caseDir)
	if err != nil {
		t.Fatalf("LoadProviderSessionCase: %v", err)
	}
	if loaded.Manifest.ID != "codex-message-tool-success" {
		t.Fatalf("manifest.ID = %q, want codex-message-tool-success", loaded.Manifest.ID)
	}

	var request struct {
		SessionID string `json:"session_id"`
	}
	if err := json.Unmarshal(loaded.Request, &request); err != nil {
		t.Fatalf("decode request.json: %v", err)
	}
	if request.SessionID == "" {
		t.Fatal("request.session_id must be non-empty")
	}

	rolloutPath := filepath.Join(caseDir, codexGoldenRolloutFile)
	rolloutContent, err := os.ReadFile(rolloutPath)
	if err != nil {
		t.Fatalf("read %s: %v", codexGoldenRolloutFile, err)
	}

	homeDir := t.TempDir()
	writeCodexGoldenRolloutFixture(t, codexSessionsRoot(homeDir), request.SessionID, string(rolloutContent))

	server := startCodexProviderSessionDetailServer(t, homeDir, serviceedges.Edges{})
	defer server.Stop(t)

	detail := support.GetJSON[factoryapi.ProviderSessionDetailResponse](
		t,
		codexProviderSessionDetailURL(server.URL(), request.SessionID),
	)
	if detail.ProviderSession.Id != request.SessionID {
		t.Fatalf("detail provider session id = %q, want %q", detail.ProviderSession.Id, request.SessionID)
	}
	if detail.ProviderSession.Provider != factoryapi.Codex {
		t.Fatalf("detail provider = %q, want codex", detail.ProviderSession.Provider)
	}
	if detail.ProviderSession.Kind != factoryapi.LoadableProviderSessionKindSessionID {
		t.Fatalf("detail kind = %q, want session_id", detail.ProviderSession.Kind)
	}
	if len(detail.Transcript) == 0 {
		t.Fatal("provider session detail transcript is empty, want readable success-session content")
	}

	observed := observeCodexProviderSessionDetailGolden(detail)
	if err := compareOrUpdateCodexProviderSessionDetailGolden(loaded, observed); err != nil {
		var updated *support.ProviderSessionGoldensUpdatedError
		if errors.As(err, &updated) {
			t.Fatalf("%v", err)
		}
		t.Fatalf("compareOrUpdateCodexProviderSessionDetailGolden: %v", err)
	}
}

// TestCodexProviderSessionMissingTranscriptReturnsNotFound proves that requesting
// Codex Provider Session details for a session_id with no stored rollout returns
// a distinguishable not-found outcome instead of fabricating session detail.
func TestCodexProviderSessionMissingTranscriptReturnsNotFound(t *testing.T) {
	homeDir := t.TempDir()

	server := startCodexProviderSessionDetailServer(t, homeDir, serviceedges.Edges{})
	defer server.Stop(t)

	body := getCodexProviderSessionDetailErrorBody(
		t,
		server.URL(),
		codexMissingTranscriptSessionID,
		http.StatusNotFound,
	)
	if !strings.Contains(body, "provider session not found") {
		t.Fatalf("error body = %q, want not-found message", body)
	}

	var failure factoryapi.ErrorResponse
	if err := json.Unmarshal([]byte(body), &failure); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if failure.Code != factoryapi.ErrorResponseCodeNOTFOUND {
		t.Fatalf("error code = %q, want NOT_FOUND", failure.Code)
	}
	if failure.Message == "" {
		t.Fatal("error message is empty, want customer-readable not-found diagnostic")
	}
	if strings.Contains(body, `"transcript"`) || strings.Contains(body, `"providerSession"`) {
		t.Fatalf("not-found response fabricated provider session detail: %s", body)
	}

	assertCodexProviderSessionErrorBodySafe(t, "missing-transcript", body, homeDir)
}

func startCodexProviderSessionDetailServer(
	t *testing.T,
	homeDir string,
	edges serviceedges.Edges,
) *support.FunctionalAPIServer {
	t.Helper()

	if edges.ProviderSessionResolveHomeDirectory == nil {
		edges.ProviderSessionResolveHomeDirectory = func() (string, error) { return homeDir, nil }
	}
	dir := support.ScaffoldSingleStepFactory(t, "codex-provider-session-detail")
	return support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		UseMockWorkers:            true,
		WaitForServiceModeRuntime: true,
		Edges:                     edges,
		Env:                       []string{"HOME=" + homeDir, "USERPROFILE=" + homeDir},
	})
}

func codexSessionsRoot(homeDir string) string {
	return filepath.Join(homeDir, ".codex", "sessions")
}

func getCodexProviderSessionDetailErrorBody(
	t *testing.T,
	baseURL, sessionID string,
	wantStatus int,
) string {
	t.Helper()

	response, err := http.Get(codexProviderSessionDetailURL(baseURL, sessionID))
	if err != nil {
		t.Fatalf("GET provider session detail: %v", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read provider session detail body: %v", err)
	}
	if response.StatusCode != wantStatus {
		t.Fatalf(
			"GET provider session detail status = %d, want %d: %s",
			response.StatusCode,
			wantStatus,
			strings.TrimSpace(string(body)),
		)
	}
	return string(body)
}

func assertCodexProviderSessionErrorBodySafe(t *testing.T, caseID, body, homeDir string) {
	t.Helper()
	if err := support.ValidateProviderSessionFixtureContent(caseID, "provider-session-detail", []byte(body)); err != nil {
		t.Fatalf("provider session response leaked forbidden material: %v\nbody=%s", err, body)
	}
	if strings.Contains(body, homeDir) || strings.Contains(body, codexSessionsRoot(homeDir)) {
		t.Fatalf("provider session response leaked configured host path: %s", body)
	}
}

func codexProviderSessionDetailURL(baseURL, sessionID string) string {
	query := url.Values{}
	query.Set("provider", string(factoryapi.Codex))
	query.Set("kind", string(factoryapi.LoadableProviderSessionKindSessionID))
	query.Set("id", sessionID)
	return strings.TrimSuffix(baseURL, "/") + "/provider-sessions/detail?" + query.Encode()
}

func writeCodexGoldenRolloutFixture(t *testing.T, root, sessionID, content string) {
	t.Helper()
	writeCodexGoldenRolloutFixtureAt(t, root, "2026/07/27", sessionID, content)
}

func writeCodexGoldenRolloutFixtureAt(t *testing.T, root, relativeDir, sessionID, content string) {
	t.Helper()
	dir := filepath.Join(root, filepath.FromSlash(relativeDir))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir codex rollout fixture: %v", err)
	}
	path := filepath.Join(dir, "rollout-"+sessionID+".jsonl")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write codex rollout fixture: %v", err)
	}
}

func observeCodexProviderSessionDetailGolden(
	detail factoryapi.ProviderSessionDetailResponse,
) json.RawMessage {
	transcript := make([]map[string]any, 0, len(detail.Transcript))
	for _, entry := range detail.Transcript {
		record := map[string]any{
			"order": entry.Order,
			"type":  string(entry.Type),
		}
		if entry.SourceType != nil {
			record["sourceType"] = *entry.SourceType
		}
		if entry.Text != nil {
			record["text"] = *entry.Text
		}
		if entry.Timestamp != nil {
			record["timestamp"] = entry.Timestamp.UTC().Format(time.RFC3339Nano)
		}
		if entry.CallId != nil {
			record["callId"] = *entry.CallId
		}
		if entry.Name != nil {
			record["name"] = *entry.Name
		}
		if entry.Arguments != nil {
			record["arguments"] = *entry.Arguments
		}
		if entry.Output != nil {
			record["output"] = *entry.Output
		}
		if entry.Status != nil {
			record["status"] = *entry.Status
		}
		transcript = append(transcript, record)
	}

	functionCalls := make([]map[string]any, 0, len(detail.Parse.FunctionCalls))
	for _, call := range detail.Parse.FunctionCalls {
		record := map[string]any{
			"order": call.Order,
		}
		if call.CallId != nil {
			record["callId"] = *call.CallId
		}
		if call.Name != nil {
			record["name"] = *call.Name
		}
		if call.Arguments != nil {
			record["arguments"] = *call.Arguments
		}
		if call.Output != nil {
			record["output"] = *call.Output
		}
		if call.Status != nil {
			record["status"] = *call.Status
		}
		if call.Type != "" {
			record["type"] = call.Type
		}
		if call.TurnIndex != nil {
			record["turnIndex"] = *call.TurnIndex
		}
		functionCalls = append(functionCalls, record)
	}

	var tokenUsage map[string]any
	if detail.Parse.TokenUsage != nil {
		tokenUsage = map[string]any{}
		if detail.Parse.TokenUsage.InputTokens != nil {
			tokenUsage["inputTokens"] = *detail.Parse.TokenUsage.InputTokens
		}
		if detail.Parse.TokenUsage.OutputTokens != nil {
			tokenUsage["outputTokens"] = *detail.Parse.TokenUsage.OutputTokens
		}
		if detail.Parse.TokenUsage.CachedInputTokens != nil {
			tokenUsage["cachedInputTokens"] = *detail.Parse.TokenUsage.CachedInputTokens
		}
		if detail.Parse.TokenUsage.CacheWriteTokens != nil {
			tokenUsage["cacheWriteTokens"] = *detail.Parse.TokenUsage.CacheWriteTokens
		}
		if detail.Parse.TokenUsage.ReasoningOutputTokens != nil {
			tokenUsage["reasoningOutputTokens"] = *detail.Parse.TokenUsage.ReasoningOutputTokens
		}
		if detail.Parse.TokenUsage.TotalTokens != nil {
			tokenUsage["totalTokens"] = *detail.Parse.TokenUsage.TotalTokens
		}
	}

	turns := make([]map[string]any, 0, len(detail.Parse.Turns))
	for _, turn := range detail.Parse.Turns {
		record := map[string]any{
			"index": turn.Index,
		}
		record["eventCount"] = turn.EventCount
		record["functionCallCount"] = turn.FunctionCallCount
		record["reasoningCount"] = turn.ReasoningCount
		record["responseItemCount"] = turn.ResponseItemCount
		if turn.StartedAt != nil {
			record["startedAt"] = turn.StartedAt.UTC().Format(time.RFC3339Nano)
		}
		turns = append(turns, record)
	}

	record := map[string]any{
		"providerSession": map[string]any{
			"provider": string(detail.ProviderSession.Provider),
			"kind":     string(detail.ProviderSession.Kind),
			"id":       detail.ProviderSession.Id,
		},
		"source": map[string]any{
			"relativePath": detail.Source.RelativePath,
			"sizeBytes":    detail.Source.SizeBytes,
		},
		"parse": map[string]any{
			"eventCount":    detail.Parse.EventCount,
			"lineCount":     detail.Parse.LineCount,
			"functionCalls": functionCalls,
			"turns":         turns,
		},
		"transcript": transcript,
	}
	if detail.Source.ModifiedAt != nil {
		record["source"].(map[string]any)["modifiedAt"] = detail.Source.ModifiedAt.UTC().Format(time.RFC3339Nano)
	}
	if tokenUsage != nil {
		record["parse"].(map[string]any)["tokenUsage"] = tokenUsage
	}
	return mustMarshalJSON(record)
}

func compareOrUpdateCodexProviderSessionDetailGolden(
	loaded support.ProviderSessionCase,
	observed json.RawMessage,
) error {
	expectedPath := filepath.Join(loaded.CaseDir, codexGoldenExpectedProviderSessionDetailFile)
	expected, err := os.ReadFile(expectedPath)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		if !support.ProviderSessionFunctionalGoldensUpdateEnabled() {
			return &support.ProviderSessionLoadError{
				CaseID: loaded.Manifest.ID,
				Role:   "expected-provider-session-detail",
				Path:   expectedPath,
				Detail: "required expected-provider-session-detail fixture is missing",
			}
		}
		encoded, err := json.MarshalIndent(json.RawMessage(observed), "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(expectedPath, append(encoded, '\n'), 0o644); err != nil {
			return err
		}
		return &support.ProviderSessionGoldensUpdatedError{
			CaseID: loaded.Manifest.ID,
			Paths:  []string{codexGoldenExpectedProviderSessionDetailFile},
		}
	}

	normalizedFields := append([]string(nil), loaded.Manifest.NormalizedFields...)
	normalizedFields = append(normalizedFields, "modifiedAt", "sizeBytes", "timestamp", "startedAt")

	err = support.CompareProviderSessionJSON(
		loaded.Manifest.ID,
		"expected-provider-session-detail",
		normalizedFields,
		expected,
		observed,
	)
	if err == nil {
		return nil
	}
	if !support.ProviderSessionFunctionalGoldensUpdateEnabled() {
		return err
	}
	encoded, marshalErr := json.MarshalIndent(json.RawMessage(observed), "", "  ")
	if marshalErr != nil {
		return marshalErr
	}
	if writeErr := os.WriteFile(expectedPath, append(encoded, '\n'), 0o644); writeErr != nil {
		return writeErr
	}
	return &support.ProviderSessionGoldensUpdatedError{
		CaseID: loaded.Manifest.ID,
		Paths:  []string{codexGoldenExpectedProviderSessionDetailFile},
	}
}

func mustMarshalJSON(value any) json.RawMessage {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return encoded
}
