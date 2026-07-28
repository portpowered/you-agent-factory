package details

import (
	"database/sql"
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
	_ "modernc.org/sqlite"
)

const (
	cursorGoldenSuccessWorkspaceHash              = "cursor-fixture-workspace"
	cursorGoldenExpectedProviderSessionDetailFile = "expected-provider-session-detail.json"
	cursorUnavailableContentSessionID             = "cursor-fixture-unavailable-content"
	cursorMissingSessionID                        = "cursor-fixture-missing-session"
)

// TestCursorProviderSessionDetailsLoadFromGoldenMetadata proves Cursor Provider
// Session detail activates through the public GET /provider-sessions/detail surface
// after runtime lifecycle starts on a process composed only via
// support.StartFunctionalAPIServer (root.BuildProcess + edges.Edges). It loads a
// sanitized Cursor success store and proves identity/provider/kind plus readable
// transcript structurally match checked-in expected Provider Session metadata.
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
	assertProviderSessionDetailIdentity(
		t,
		detail,
		request.SessionID,
		factoryapi.Cursor,
		factoryapi.LoadableProviderSessionKindSessionID,
	)
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

// TestCursorProviderSessionUnavailableContentRemainsInspectable proves that a
// Cursor Provider Session whose store contains encrypted or otherwise unavailable
// blob content still returns an inspectable public detail response with identity
// and unavailable parse facts instead of fabricated plaintext transcript.
func TestCursorProviderSessionUnavailableContentRemainsInspectable(t *testing.T) {
	homeDir := t.TempDir()
	writeCursorUnavailableContentStorageFixture(t, homeDir, cursorUnavailableContentSessionID)

	server := startCursorProviderSessionDetailServer(t, homeDir, serviceedges.Edges{})
	defer server.Stop(t)

	detail := support.GetJSON[factoryapi.ProviderSessionDetailResponse](
		t,
		cursorProviderSessionDetailURL(server.URL(), cursorUnavailableContentSessionID),
	)
	assertProviderSessionDetailIdentity(
		t,
		detail,
		cursorUnavailableContentSessionID,
		factoryapi.Cursor,
		factoryapi.LoadableProviderSessionKindSessionID,
	)
	if detail.Parse.UnknownEventCount == 0 && len(detail.Parse.UnknownEvents) == 0 {
		t.Fatalf("parse summary = %#v, want unavailable unknown-event diagnostics", detail.Parse)
	}
	for _, entry := range detail.Transcript {
		if entry.Text != nil && strings.TrimSpace(*entry.Text) != "" {
			t.Fatalf("transcript entry = %#v, want no fabricated plaintext from unavailable blobs", entry)
		}
	}

	encoded, err := json.Marshal(detail)
	if err != nil {
		t.Fatalf("marshal detail: %v", err)
	}
	assertCursorProviderSessionDetailBodySafe(t, "unavailable-content", string(encoded), homeDir)
}

// TestCursorProviderSessionMissingIDReturnsNotFound proves that requesting
// Cursor Provider Session details for a session_id that does not exist returns
// a distinguishable not-found outcome instead of fabricating session detail.
func TestCursorProviderSessionMissingIDReturnsNotFound(t *testing.T) {
	homeDir := t.TempDir()

	server := startCursorProviderSessionDetailServer(t, homeDir, serviceedges.Edges{})
	defer server.Stop(t)

	body := getCursorProviderSessionDetailErrorBody(
		t,
		server.URL(),
		cursorMissingSessionID,
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

	assertCursorProviderSessionDetailBodySafe(t, "missing-session", body, homeDir)
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

func getCursorProviderSessionDetailErrorBody(
	t *testing.T,
	baseURL, sessionID string,
	wantStatus int,
) string {
	t.Helper()

	response, err := http.Get(cursorProviderSessionDetailURL(baseURL, sessionID))
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

func writeCursorUnavailableContentStorageFixture(t *testing.T, homeDir, sessionID string) {
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

	if _, err := db.Exec(`CREATE TABLE blobs (key TEXT PRIMARY KEY, value TEXT)`); err != nil {
		t.Fatalf("create cursor storage blobs table: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO blobs (key, value) VALUES (?, ?)`,
		"encrypted-blob",
		string([]byte{0x00, 0x01, 0x02, 0x03, 0xff, 0xfe, 0xfd, 0xfc, 0xfb, 0xfa}),
	); err != nil {
		t.Fatalf("insert encrypted cursor storage blob: %v", err)
	}
}

func assertCursorProviderSessionDetailBodySafe(t *testing.T, caseID, body, homeDir string) {
	t.Helper()
	if err := support.ValidateProviderSessionFixtureContent(caseID, "provider-session-detail", []byte(body)); err != nil {
		t.Fatalf("provider session response leaked forbidden material: %v\nbody=%s", err, body)
	}
	cursorChatsRoot := filepath.Join(homeDir, ".cursor", "chats")
	if strings.Contains(body, homeDir) || strings.Contains(body, cursorChatsRoot) {
		t.Fatalf("provider session response leaked configured host path: %s", body)
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
