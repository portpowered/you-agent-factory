package details

import (
	"database/sql"
	"encoding/json"
	"errors"
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
	_ "modernc.org/sqlite"
)

const (
	cursorGoldenSuccessWorkspaceHash             = "cursor-fixture-workspace"
	cursorGoldenExpectedProviderSessionDetailFile = "expected-provider-session-detail.json"
)

// TestCursorProviderSessionDetailsLoadFromGoldenMetadata loads a sanitized Cursor
// success store through the public Provider Session detail surface and proves
// the response structurally matches checked-in expected Provider Session metadata.
//golden: docs/temp/functional/provider-sessions/cursor/success/manifest.json
func TestCursorProviderSessionDetailsLoadFromGoldenMetadata(t *testing.T) {
	repoRoot := testutil.MustRepoRoot(t)
	caseDir := filepath.Join(repoRoot, filepath.FromSlash(support.ProviderSessionFixturePath("cursor", "success")))

	loaded, err := support.LoadProviderSessionCase(caseDir)
	if err != nil {
		t.Fatalf("LoadProviderSessionCase: %v", err)
	}
	if loaded.Manifest.ID != "cursor-text-success" {
		t.Fatalf("manifest.ID = %q, want cursor-text-success", loaded.Manifest.ID)
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

	homeDir := t.TempDir()
	writeCursorGoldenSuccessStorageFixture(t, homeDir, request.SessionID)

	server := startCursorProviderSessionDetailServer(t, homeDir, serviceedges.Edges{})
	defer server.Stop(t)

	detail := support.GetJSON[factoryapi.ProviderSessionDetailResponse](
		t,
		cursorProviderSessionDetailURL(server.URL(), request.SessionID),
	)
	if detail.ProviderSession.Id != request.SessionID {
		t.Fatalf("detail provider session id = %q, want %q", detail.ProviderSession.Id, request.SessionID)
	}
	if detail.ProviderSession.Provider != factoryapi.Cursor {
		t.Fatalf("detail provider = %q, want cursor", detail.ProviderSession.Provider)
	}
	if detail.ProviderSession.Kind != factoryapi.LoadableProviderSessionKindSessionID {
		t.Fatalf("detail kind = %q, want session_id", detail.ProviderSession.Kind)
	}
	if len(detail.Transcript) == 0 {
		t.Fatal("provider session detail transcript is empty, want readable success-session content")
	}
	hasReadableText := false
	for _, entry := range detail.Transcript {
		if entry.Text != nil && strings.TrimSpace(*entry.Text) != "" {
			hasReadableText = true
			break
		}
	}
	if !hasReadableText {
		t.Fatalf("provider session detail transcript = %#v, want readable text", detail.Transcript)
	}

	observed := observeCursorProviderSessionDetailGolden(detail)
	if err := compareOrUpdateCursorProviderSessionDetailGolden(loaded, observed); err != nil {
		var updated *support.ProviderSessionGoldensUpdatedError
		if errors.As(err, &updated) {
			t.Fatalf("%v", err)
		}
		t.Fatalf("compareOrUpdateCursorProviderSessionDetailGolden: %v", err)
	}
}

func startCursorProviderSessionDetailServer(
	t *testing.T,
	homeDir string,
	edges serviceedges.Edges,
) *support.FunctionalAPIServer {
	t.Helper()

	if edges.ProviderSessionResolveHomeDirectory == nil {
		edges.ProviderSessionResolveHomeDirectory = func() (string, error) { return homeDir, nil }
	}
	dir := support.ScaffoldSingleStepFactory(t, "cursor-provider-session-detail")
	return support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		UseMockWorkers:            true,
		WaitForServiceModeRuntime: true,
		Edges:                     edges,
		Env:                       []string{"HOME=" + homeDir, "USERPROFILE=" + homeDir},
	})
}

func cursorProviderSessionDetailURL(baseURL, sessionID string) string {
	query := url.Values{}
	query.Set("provider", string(factoryapi.Cursor))
	query.Set("kind", string(factoryapi.LoadableProviderSessionKindSessionID))
	query.Set("id", sessionID)
	return strings.TrimSuffix(baseURL, "/") + "/provider-sessions/detail?" + query.Encode()
}

func writeCursorGoldenSuccessStorageFixture(t *testing.T, homeDir, sessionID string) {
	t.Helper()

	chatsRoot := filepath.Join(homeDir, ".cursor", "chats")
	dbPath := filepath.Join(chatsRoot, cursorGoldenSuccessWorkspaceHash, sessionID, "store.db")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		t.Fatalf("mkdir cursor storage: %v", err)
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open cursor storage sqlite: %v", err)
	}
	defer func() { _ = db.Close() }()

	schema := `
CREATE TABLE blobs (key TEXT PRIMARY KEY, value TEXT);
CREATE TABLE meta (key TEXT PRIMARY KEY, value TEXT);`
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("create cursor storage tables: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO blobs (key, value) VALUES (?, ?)`,
		"bubble-success",
		`{"bubbleId":"bubble-success","chatId":"chat-success","text":"Cursor fixture answer COMPLETE","timestamp":1000,"type":1}`,
	); err != nil {
		t.Fatalf("insert cursor storage bubble: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO meta (key, value) VALUES (?, ?)`,
		"0",
		`{"createdAt":1000,"agentId":"`+sessionID+`","name":"Cursor golden success fixture session"}`,
	); err != nil {
		t.Fatalf("insert cursor storage meta: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO meta (key, value) VALUES (?, ?)`,
		"1",
		`{"usage":{"inputTokens":12,"outputTokens":34,"cacheReadTokens":5,"cacheWriteTokens":2}}`,
	); err != nil {
		t.Fatalf("insert cursor storage usage meta: %v", err)
	}
}

func observeCursorProviderSessionDetailGolden(
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
		transcript = append(transcript, record)
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
		if detail.Parse.TokenUsage.TotalTokens != nil {
			tokenUsage["totalTokens"] = *detail.Parse.TokenUsage.TotalTokens
		}
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
			"eventCount": detail.Parse.EventCount,
			"lineCount":  detail.Parse.LineCount,
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

func compareOrUpdateCursorProviderSessionDetailGolden(
	loaded support.ProviderSessionCase,
	observed json.RawMessage,
) error {
	expectedPath := filepath.Join(loaded.CaseDir, cursorGoldenExpectedProviderSessionDetailFile)
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
			Paths:  []string{cursorGoldenExpectedProviderSessionDetailFile},
		}
	}

	normalizedFields := append([]string(nil), loaded.Manifest.NormalizedFields...)
	normalizedFields = append(normalizedFields, "modifiedAt", "sizeBytes")

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
		Paths:  []string{cursorGoldenExpectedProviderSessionDetailFile},
	}
}
