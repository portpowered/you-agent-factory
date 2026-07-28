package cursor

import (
	"context"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	_ "modernc.org/sqlite"
)

const (
	cursorInspectionWorkspaceHash = "cursor-root-inspection-workspace"
	cursorInspectionSessionID     = "cursor-root-inspection-session"
)

// TestCursorHistoricalInspectionThroughRootBuildProcess_ReturnsDeterministicNormalizedDetail
// proves a valid Cursor store inspected through the public provider-session
// detail surface returns canonical identity and normalized facts in stable order.
func TestCursorHistoricalInspectionThroughRootBuildProcess_ReturnsDeterministicNormalizedDetail(t *testing.T) {
	homeDir := t.TempDir()
	writeCursorInspectionNormalizedStore(t, homeDir, cursorInspectionSessionID)

	server := startCursorInspectionRootServer(t, homeDir, serviceedges.Edges{})
	defer server.Stop(t)

	endpoint := cursorProviderSessionDetailURL(server.URL(), factoryapi.Cursor, cursorInspectionSessionID)
	first := support.GetJSON[factoryapi.ProviderSessionDetailResponse](t, endpoint)
	second := support.GetJSON[factoryapi.ProviderSessionDetailResponse](t, endpoint)

	if first.ProviderSession.Id != cursorInspectionSessionID ||
		first.ProviderSession.Provider != factoryapi.Cursor {
		t.Fatalf("ProviderSession = %#v, want cursor %q", first.ProviderSession, cursorInspectionSessionID)
	}
	if len(first.Transcript) < 5 {
		t.Fatalf("Transcript = %#v, want representative normalized entries", first.Transcript)
	}
	wantTypes := []factoryapi.ProviderSessionTranscriptEntryType{
		factoryapi.UserMessage,
		factoryapi.AssistantMessage,
		factoryapi.Reasoning,
		factoryapi.ToolCall,
		factoryapi.ToolOutput,
	}
	for index, want := range wantTypes {
		if first.Transcript[index].Type != want {
			t.Fatalf("Transcript[%d].Type = %q, want %q", index, first.Transcript[index].Type, want)
		}
	}
	if first.Parse.TokenUsage == nil || first.Parse.TokenUsage.TotalTokens == nil ||
		*first.Parse.TokenUsage.TotalTokens != 18 {
		t.Fatalf("TokenUsage = %#v, want total 18", first.Parse.TokenUsage)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("repeated root-built inspections differ:\nfirst=%#v\nsecond=%#v", first, second)
	}

	if first.Transcript[0].Text != nil {
		*first.Transcript[0].Text = "mutated"
	}
	third := support.GetJSON[factoryapi.ProviderSessionDetailResponse](t, endpoint)
	if !reflect.DeepEqual(second, third) {
		t.Fatalf("mutating one observed result affected a later inspection")
	}
}

// TestCursorHistoricalInspectionThroughRootBuildProcess_PropagatesMissingAndContainmentFailures
// proves missing-session and containment-rejection scenarios surface accepted safe
// root outcomes without sensitive diagnostic leakage.
func TestCursorHistoricalInspectionThroughRootBuildProcess_PropagatesMissingAndContainmentFailures(t *testing.T) {
	t.Run("missing session", func(t *testing.T) {
		homeDir := t.TempDir()
		if err := os.MkdirAll(cursorChatsRoot(homeDir), 0o755); err != nil {
			t.Fatalf("mkdir cursor chats root: %v", err)
		}
		server := startCursorInspectionRootServer(t, homeDir, serviceedges.Edges{})
		defer server.Stop(t)

		const secretRoot = `C:\private\cursor-token-root`
		body := getProviderSessionDetailBody(
			t,
			cursorProviderSessionDetailURL(server.URL(), factoryapi.Cursor, "missing-session"),
			http.StatusNotFound,
		)
		if strings.Contains(body, secretRoot) || strings.Contains(body, "store.db") ||
			strings.Contains(body, homeDir) {
			t.Fatalf("missing-session response leaked host storage detail: %s", body)
		}
		var failure factoryapi.ErrorResponse
		if err := json.Unmarshal([]byte(body), &failure); err != nil {
			t.Fatalf("decode error response: %v", err)
		}
		if failure.Code != factoryapi.ErrorResponseCodeNOTFOUND {
			t.Fatalf("error code = %q, want NOT_FOUND", failure.Code)
		}
	})

	t.Run("containment rejection", func(t *testing.T) {
		homeDir := t.TempDir()
		writeCursorInspectionMinimalStore(t, homeDir, "contained-session")
		chatsRoot := cursorChatsRoot(homeDir)
		absoluteRoot, err := filepath.Abs(chatsRoot)
		if err != nil {
			t.Fatalf("abs chats root: %v", err)
		}
		outside := filepath.Join(t.TempDir(), "replacement.db")
		server := startCursorInspectionRootServer(t, homeDir, serviceedges.Edges{
			ProviderSessionCursorResolveSymlinks: func(path string) (string, error) {
				if filepath.Clean(path) == filepath.Clean(absoluteRoot) {
					return absoluteRoot, nil
				}
				return outside, nil
			},
		})
		defer server.Stop(t)

		body := getProviderSessionDetailBody(
			t,
			cursorProviderSessionDetailURL(server.URL(), factoryapi.Cursor, "contained-session"),
			http.StatusBadRequest,
		)
		if strings.Contains(body, outside) || strings.Contains(body, homeDir) {
			t.Fatalf("containment response leaked host storage detail: %s", body)
		}
	})
}

// TestCursorHistoricalInspectionThroughRootBuildProcess_DegradesAdverseNativeDataSafely
// proves corrupt, malformed, unknown, and cancellation scenarios remain bounded
// and sanitized through the production root composition.
func TestCursorHistoricalInspectionThroughRootBuildProcess_DegradesAdverseNativeDataSafely(t *testing.T) {
	t.Run("malformed and oversized blobs retain valid rows", func(t *testing.T) {
		homeDir := t.TempDir()
		writeCursorInspectionAdverseBlobStore(t, homeDir, "adverse-blob-session")
		server := startCursorInspectionRootServer(t, homeDir, serviceedges.Edges{})
		defer server.Stop(t)

		detail := support.GetJSON[factoryapi.ProviderSessionDetailResponse](
			t,
			cursorProviderSessionDetailURL(server.URL(), factoryapi.Cursor, "adverse-blob-session"),
		)
		if len(detail.Transcript) != 1 || detail.Transcript[0].Text == nil ||
			*detail.Transcript[0].Text != "valid message" {
			t.Fatalf("Transcript = %#v, want one valid message", detail.Transcript)
		}
		if detail.Parse.MalformedLineCount == 0 && len(detail.Parse.ParseErrors) == 0 {
			t.Fatalf("Parse = %#v, want bounded malformed diagnostics", detail.Parse)
		}
		assertProviderSessionDetailRedacted(t, detail)
	})

	t.Run("production blob byte limit skips oversized payload", func(t *testing.T) {
		homeDir := t.TempDir()
		writeCursorInspectionProductionOversizedBlobStore(t, homeDir, "oversized-blob-session")
		server := startCursorInspectionRootServer(t, homeDir, serviceedges.Edges{})
		defer server.Stop(t)

		detail := support.GetJSON[factoryapi.ProviderSessionDetailResponse](
			t,
			cursorProviderSessionDetailURL(server.URL(), factoryapi.Cursor, "oversized-blob-session"),
		)
		if len(detail.Transcript) != 1 || detail.Transcript[0].Text == nil ||
			*detail.Transcript[0].Text != "valid message" {
			t.Fatalf("Transcript = %#v, want one valid message before oversized skip", detail.Transcript)
		}
		if detail.Parse.MalformedLineCount == 0 && len(detail.Parse.ParseErrors) == 0 {
			t.Fatalf("Parse = %#v, want bounded oversized-blob diagnostics", detail.Parse)
		}
		assertProviderSessionDetailRedacted(t, detail)
	})

	t.Run("unreadable database fails safely", func(t *testing.T) {
		homeDir := t.TempDir()
		writeCursorInspectionUnreadableStore(t, homeDir, "unreadable-session")
		server := startCursorInspectionRootServer(t, homeDir, serviceedges.Edges{})
		defer server.Stop(t)

		body := getProviderSessionDetailBody(
			t,
			cursorProviderSessionDetailURL(server.URL(), factoryapi.Cursor, "unreadable-session"),
			http.StatusInternalServerError,
		)
		if strings.Contains(body, homeDir) || strings.Contains(strings.ToLower(body), "select ") {
			t.Fatalf("unreadable database response leaked sensitive detail: %s", body)
		}
	})

	t.Run("unknown native records are summarized without transcript fabrication", func(t *testing.T) {
		homeDir := t.TempDir()
		writeCursorInspectionUnknownRecordStore(t, homeDir, "unknown-record-session")
		server := startCursorInspectionRootServer(t, homeDir, serviceedges.Edges{})
		defer server.Stop(t)

		detail := support.GetJSON[factoryapi.ProviderSessionDetailResponse](
			t,
			cursorProviderSessionDetailURL(server.URL(), factoryapi.Cursor, "unknown-record-session"),
		)
		if detail.Parse.UnknownEventCount == 0 && len(detail.Parse.UnknownEvents) == 0 {
			t.Fatalf("Parse = %#v, want unknown-record summary", detail.Parse)
		}
		if len(detail.Transcript) != 1 || detail.Transcript[0].Text == nil ||
			*detail.Transcript[0].Text != "known message" {
			t.Fatalf("Transcript = %#v, want only known rows reconstructed", detail.Transcript)
		}
		assertProviderSessionDetailRedacted(t, detail)
	})

	t.Run("cancellation stops further reconstruction", func(t *testing.T) {
		homeDir := t.TempDir()
		writeCursorInspectionMinimalStore(t, homeDir, "canceled-session")
		server := startCursorInspectionRootServer(t, homeDir, serviceedges.Edges{
			ProviderSessionCursorWalkDirectory: func(root string, walkFn fs.WalkDirFunc) error {
				return context.Canceled
			},
		})
		defer server.Stop(t)

		body := getProviderSessionDetailBody(
			t,
			cursorProviderSessionDetailURL(server.URL(), factoryapi.Cursor, "canceled-session"),
			http.StatusInternalServerError,
		)
		if strings.Contains(body, homeDir) {
			t.Fatalf("cancellation response leaked host storage detail: %s", body)
		}
	})
}

func startCursorInspectionRootServer(
	t *testing.T,
	homeDir string,
	edges serviceedges.Edges,
) *support.FunctionalAPIServer {
	t.Helper()

	if edges.ProviderSessionResolveHomeDirectory == nil {
		edges.ProviderSessionResolveHomeDirectory = func() (string, error) { return homeDir, nil }
	}

	dir := support.ScaffoldFactory(t, map[string]any{
		"name": "cursor-historical-inspection",
		"workTypes": []map[string]any{{
			"name": "task",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "done", "type": "TERMINAL"},
			},
		}},
		"workers": []map[string]string{{"name": "worker"}},
		"workstations": []map[string]any{{
			"name":     "process",
			"worker":   "worker",
			"behavior": "STANDARD",
			"inputs":   []map[string]string{{"workType": "task", "state": "init"}},
			"outputs":  []map[string]string{{"workType": "task", "state": "done"}},
		}},
	})

	env := append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)
	return support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		WaitForServiceModeRuntime: true,
		Env:                       env,
		Edges:                     edges,
	})
}

func cursorChatsRoot(homeDir string) string {
	return filepath.Join(homeDir, ".cursor", "chats")
}

func cursorStoreDBPath(homeDir, sessionID string) string {
	return filepath.Join(cursorChatsRoot(homeDir), cursorInspectionWorkspaceHash, sessionID, "store.db")
}

func cursorProviderSessionDetailURL(
	baseURL string,
	provider factoryapi.LoadableProviderSessionProvider,
	sessionID string,
) string {
	query := url.Values{}
	query.Set("provider", string(provider))
	query.Set("kind", string(factoryapi.LoadableProviderSessionKindSessionID))
	query.Set("id", sessionID)
	return strings.TrimSuffix(baseURL, "/") + "/provider-sessions/detail?" + query.Encode()
}

func getProviderSessionDetailBody(t *testing.T, endpoint string, wantStatus int) string {
	t.Helper()
	response, err := http.Get(endpoint)
	if err != nil {
		t.Fatalf("GET %s: %v", endpoint, err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read GET %s body: %v", endpoint, err)
	}
	if response.StatusCode != wantStatus {
		t.Fatalf("GET %s status = %d, want %d: %s", endpoint, response.StatusCode, wantStatus, strings.TrimSpace(string(body)))
	}
	return string(body)
}

func assertProviderSessionDetailRedacted(t *testing.T, detail factoryapi.ProviderSessionDetailResponse) {
	t.Helper()
	encoded, err := json.Marshal(detail)
	if err != nil {
		t.Fatalf("marshal detail: %v", err)
	}
	payload := strings.ToLower(string(encoded))
	for _, forbidden := range []string{"select ", "pragma ", "password", "secret-prompt", "c:\\private"} {
		if strings.Contains(payload, forbidden) {
			t.Fatalf("provider session detail leaked sensitive content %q: %s", forbidden, payload)
		}
	}
	for _, diagnostic := range detail.Parse.ParseErrors {
		if len(diagnostic.Message) > 256 {
			t.Fatalf("diagnostic too long: %q", diagnostic.Message)
		}
	}
}

func writeCursorInspectionMinimalStore(t *testing.T, homeDir, sessionID string) {
	t.Helper()
	writeCursorInspectionStore(t, homeDir, sessionID, func(db *sql.DB) error {
		_, err := db.Exec(`
INSERT INTO blobs (key, value) VALUES ('bubble-1', '{"bubbleId":"bubble-1","text":"hello","timestamp":1000,"type":1}');
INSERT INTO meta (key, value) VALUES ('0', '{"agentId":"` + sessionID + `","createdAt":1000}');
`)
		return err
	})
}

func writeCursorInspectionNormalizedStore(t *testing.T, homeDir, sessionID string) {
	t.Helper()
	user := `{"id":"user-1","role":"user","timestamp":2000,"content":[{"type":"input_text","text":"question"}]}`
	assistant := `{"id":"assistant-1","role":"assistant","timestamp":2000,"content":[{"type":"output_text","text":"answer"},{"type":"reasoning","text":"considered","summary":"brief"},{"type":"tool_call","name":"search","tool_call_id":"call-1","arguments":{"q":"docs"},"status":"started"},{"type":"tool","name":"search","tool_call_id":"call-1","content":"found","status":"completed"},{"type":"redacted-reasoning","data":"sensitive-ciphertext"}]}`
	composer := `{"composerId":"composer-1","createdAt":1000,"fullConversationHeadersOnly":[{"bubbleId":"user-1","type":1},{"bubbleId":"assistant-1","type":2}]}`
	usage := `{"usage":{"inputTokens":10,"outputTokens":5,"cacheReadTokens":2,"cacheWriteTokens":1}}`
	writeCursorInspectionStore(t, homeDir, sessionID, func(db *sql.DB) error {
		for _, row := range []struct {
			key   string
			value string
		}{
			{key: "03-assistant-protobuf-duplicate", value: cursorInspectionProtobufJSON(assistant)},
			{key: "02-assistant-json", value: assistant},
			{key: "01-user", value: user},
			{key: "04-composer", value: composer},
		} {
			if _, err := db.Exec(`INSERT INTO blobs (key, value) VALUES (?, ?)`, row.key, row.value); err != nil {
				return fmt.Errorf("insert blob %s: %w", row.key, err)
			}
		}
		if _, err := db.Exec(`INSERT INTO meta (key, value) VALUES ('usage', ?)`, usage); err != nil {
			return err
		}
		_, err := db.Exec(`INSERT INTO meta (key, value) VALUES ('0', '{"agentId":"` + sessionID + `","createdAt":1000}')`)
		return err
	})
}

func writeCursorInspectionAdverseBlobStore(t *testing.T, homeDir, sessionID string) {
	t.Helper()
	writeCursorInspectionStore(t, homeDir, sessionID, func(db *sql.DB) error {
		_, err := db.Exec(`
INSERT INTO blobs (key, value) VALUES ('valid', '{"bubbleId":"bubble-valid","text":"valid message","timestamp":1000,"type":1}');
INSERT INTO blobs (key, value) VALUES ('malformed', 'not-json-secret-prompt');
INSERT INTO blobs (key, value) VALUES ('oversized', ?);
INSERT INTO meta (key, value) VALUES ('0', '{"agentId":"`+sessionID+`","createdAt":1000}');
`, strings.Repeat("x", 128))
		return err
	})
}

func writeCursorInspectionProductionOversizedBlobStore(t *testing.T, homeDir, sessionID string) {
	t.Helper()
	const productionBlobByteLimit = 4 * 1024 * 1024
	writeCursorInspectionStore(t, homeDir, sessionID, func(db *sql.DB) error {
		_, err := db.Exec(`
INSERT INTO blobs (key, value) VALUES ('valid', '{"bubbleId":"bubble-valid","text":"valid message","timestamp":1000,"type":1}');
INSERT INTO blobs (key, value) VALUES ('oversized', ?);
INSERT INTO meta (key, value) VALUES ('0', '{"agentId":"`+sessionID+`","createdAt":1000}');
`, strings.Repeat("x", productionBlobByteLimit+1))
		return err
	})
}

func writeCursorInspectionUnreadableStore(t *testing.T, homeDir, sessionID string) {
	t.Helper()
	path := cursorStoreDBPath(homeDir, sessionID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir unreadable store: %v", err)
	}
	if err := os.WriteFile(path, []byte("not-a-sqlite-database"), 0o600); err != nil {
		t.Fatalf("write unreadable store: %v", err)
	}
}

func writeCursorInspectionUnknownRecordStore(t *testing.T, homeDir, sessionID string) {
	t.Helper()
	writeCursorInspectionStore(t, homeDir, sessionID, func(db *sql.DB) error {
		_, err := db.Exec(`
INSERT INTO blobs (key, value) VALUES ('known', '{"bubbleId":"bubble-known","text":"known message","timestamp":1000,"type":1}');
INSERT INTO blobs (key, value) VALUES ('unknown', '{"type":"cursor-unknown-native-record","payload":"secret-prompt"}');
INSERT INTO meta (key, value) VALUES ('0', '{"agentId":"`+sessionID+`","createdAt":1000}');
`)
		return err
	})
}

func writeCursorInspectionStore(
	t *testing.T,
	homeDir, sessionID string,
	populate func(*sql.DB) error,
) {
	t.Helper()
	path := cursorStoreDBPath(homeDir, sessionID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir cursor store: %v", err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open cursor store: %v", err)
	}
	defer func() { _ = db.Close() }()
	if _, err := db.Exec(`
CREATE TABLE blobs (key TEXT PRIMARY KEY, value TEXT);
CREATE TABLE meta (key TEXT PRIMARY KEY, value TEXT);
`); err != nil {
		t.Fatalf("create cursor store tables: %v", err)
	}
	if err := populate(db); err != nil {
		t.Fatalf("populate cursor store: %v", err)
	}
}

func cursorInspectionProtobufJSON(value string) string {
	encoded := []byte{0x0a}
	encoded = binary.AppendUvarint(encoded, uint64(len(value)))
	encoded = append(encoded, value...)
	return string(encoded)
}
