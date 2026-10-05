package codex

import (
	"context"
	"encoding/json"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	codexFunctionalSessionID            = "session-functional-root"
	codexFunctionalDetachedSessionID    = "session-detached-shared"
	codexFunctionalMissingSessionID     = "session-missing-root"
	codexFunctionalMalformedSessionID   = "session-malformed-root"
	codexFunctionalBoundedWalkSessionID = "session-bounded-root"
	codexFunctionalCanceledSessionID    = "session-canceled-root"
	codexFunctionalOutsideSessionID     = "session-outside-root"
	codexFunctionalOversizedSessionID   = "session-oversized-root"
)

// TestCodexHistoricalInspectionCancelledDiscoveryThroughRootBuildProcess proves
// injected discovery cancellation surfaces a safe root outcome without host-path
// leakage when reached only through root.BuildProcess and public contracts.
func TestCodexHistoricalInspectionCancelledDiscoveryThroughRootBuildProcess(t *testing.T) {
	// The discovery override is process-wide during root construction. It
	// cannot coexist with successful discovery in the shared process without a
	// mutable route or a cancellation policy that would affect other scenarios.
	homeDir := t.TempDir()
	writeCodexRolloutFixture(
		t,
		codexSessionsRoot(homeDir),
		codexFunctionalCanceledSessionID,
		`{"type":"session_meta"}`+"\n",
	)

	server := startCodexHistoricalInspectionServer(t, homeDir, serviceedges.Edges{
		ProviderSessionCodexWalkDirectory: func(string, fs.WalkDirFunc) error {
			return context.Canceled
		},
	})
	defer server.Stop(t)

	body := getCodexProviderSessionDetailErrorBody(
		t,
		server.URL(),
		codexFunctionalCanceledSessionID,
		http.StatusInternalServerError,
	)
	if !strings.Contains(body, "failed to load provider session details") {
		t.Fatalf("error body = %q, want safe load failure", body)
	}
	assertCodexProviderSessionErrorBodySafe(t, "canceled-discovery", body, homeDir)
}

type codexSharedContainmentFixture struct {
	outsideDir string
	available  bool
	err        error
}

func prepareCodexSharedContainmentFixture(t *testing.T, homeDir, sessionID string) codexSharedContainmentFixture {
	t.Helper()
	outsideDir := t.TempDir()
	outsidePath := filepath.Join(outsideDir, "rollout-"+sessionID+".jsonl")
	if err := os.WriteFile(outsidePath, []byte(`{"type":"session_meta"}`+"\n"), 0o600); err != nil {
		t.Fatalf("write outside fixture: %v", err)
	}
	linkDir := filepath.Join(codexSessionsRoot(homeDir), "2026", "07", "27")
	if err := os.MkdirAll(linkDir, 0o755); err != nil {
		t.Fatalf("mkdir link dir: %v", err)
	}
	linkPath := filepath.Join(linkDir, "rollout-"+sessionID+".jsonl")
	if err := os.Symlink(outsidePath, linkPath); err != nil {
		if runtime.GOOS == "windows" {
			return codexSharedContainmentFixture{outsideDir: outsideDir, err: err}
		}
		t.Fatalf("create symlink: %v", err)
	}
	return codexSharedContainmentFixture{outsideDir: outsideDir, available: true}
}

func assertCodexSharedDetachedHistory(t *testing.T, fixture *codexSharedProcessFixture) {
	t.Helper()
	first := getCodexProviderSessionDetail(t, fixture.baseURL, codexFunctionalDetachedSessionID)
	second := getCodexProviderSessionDetail(t, fixture.baseURL, codexFunctionalDetachedSessionID)
	if len(first.Transcript) == 0 || len(second.Transcript) == 0 ||
		first.Transcript[0].Text == nil || second.Transcript[0].Text == nil {
		t.Fatal("expected transcript text in both shared inspections")
	}
	if *first.Transcript[0].Text != *second.Transcript[0].Text {
		t.Fatalf("repeated shared inspections differ: %#v vs %#v", first.Transcript[0], second.Transcript[0])
	}
	mutated := "mutated transcript"
	first.Transcript[0].Text = &mutated
	if *second.Transcript[0].Text == mutated {
		t.Fatal("mutating first shared inspection affected second inspection transcript")
	}
}

func assertCodexSharedMissingHistory(t *testing.T, fixture *codexSharedProcessFixture) {
	t.Helper()
	body := getCodexProviderSessionDetailErrorBody(
		t,
		fixture.baseURL,
		codexFunctionalMissingSessionID,
		http.StatusNotFound,
	)
	if !strings.Contains(body, "provider session not found") {
		t.Fatalf("shared missing-session error body = %q, want not-found message", body)
	}
	assertCodexProviderSessionErrorBodySafe(t, "missing-session", body, fixture.homeDir)
}

func assertCodexSharedMalformedHistory(t *testing.T, fixture *codexSharedProcessFixture) {
	t.Helper()
	detail := getCodexProviderSessionDetail(t, fixture.baseURL, codexFunctionalMalformedSessionID)
	if detail.Parse.MalformedLineCount != 1 || len(detail.Parse.ParseErrors) != 1 {
		t.Fatalf("shared parse summary = %#v, want one malformed-line diagnostic", detail.Parse)
	}
	if detail.Parse.ParseErrors[0].Message != "truncated JSON event record" {
		t.Fatalf("shared parse error = %#v, want truncated JSON diagnostic", detail.Parse.ParseErrors[0])
	}
	if len(detail.Transcript) != 0 {
		t.Fatalf("shared malformed transcript = %#v, want no fabricated entries", detail.Transcript)
	}
	encoded, err := json.Marshal(detail)
	if err != nil {
		t.Fatalf("marshal shared malformed detail: %v", err)
	}
	assertCodexProviderSessionErrorBodySafe(t, "malformed-jsonl", string(encoded), fixture.homeDir)
}

func assertCodexSharedOversizedHistory(t *testing.T, fixture *codexSharedProcessFixture) {
	t.Helper()
	detail := getCodexProviderSessionDetail(t, fixture.baseURL, codexFunctionalOversizedSessionID)
	if len(detail.Transcript) != 2 || detail.Transcript[0].Text == nil || *detail.Transcript[0].Text != "before" || detail.Transcript[1].Text == nil || *detail.Transcript[1].Text != "after" {
		t.Fatalf("oversized history lost valid neighbors: %#v", detail.Transcript)
	}
	if len(detail.Parse.ParseErrors) != 1 || detail.Parse.ParseErrors[0].LineNumber != 2 || detail.Parse.ParseErrors[0].Message != "inspection record limit reached" {
		t.Fatalf("oversized history missing safe line diagnostic: %#v", detail.Parse)
	}

}

func assertCodexSharedBoundedHistory(t *testing.T, fixture *codexSharedProcessFixture) {
	t.Helper()
	body := getCodexProviderSessionDetailErrorBody(
		t,
		fixture.baseURL,
		codexFunctionalBoundedWalkSessionID,
		http.StatusInternalServerError,
	)
	if !strings.Contains(body, "failed to load provider session details") {
		t.Fatalf("shared bounded-walk error body = %q, want safe failure", body)
	}
	assertCodexProviderSessionErrorBodySafe(t, "bounded-walk", body, fixture.homeDir)
}

func assertCodexSharedContainmentHistory(t *testing.T, fixture *codexSharedProcessFixture) {
	t.Helper()
	if !fixture.containmentAvailable {
		t.Skipf("symlink capability unavailable: %v", fixture.containmentCapabilityErr)
	}
	body := getCodexProviderSessionDetailErrorBody(
		t,
		fixture.baseURL,
		codexFunctionalOutsideSessionID,
		http.StatusInternalServerError,
	)
	if !strings.Contains(body, "failed to load provider session details") {
		t.Fatalf("shared containment error body = %q, want safe load failure", body)
	}
	assertCodexProviderSessionErrorBodySafe(t, "containment-rejection", body, fixture.homeDir)
}

func startCodexHistoricalInspectionServer(
	t *testing.T,
	homeDir string,
	edges serviceedges.Edges,
) *support.FunctionalAPIServer {
	t.Helper()

	if edges.ProviderSessionResolveHomeDirectory == nil {
		edges.ProviderSessionResolveHomeDirectory = func() (string, error) { return homeDir, nil }
	}
	dir := support.ScaffoldSingleStepFactory(t, "codex-historical-inspection")
	return support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		WaitForServiceModeRuntime: true,
		Edges:                     edges,
		Env:                       []string{"HOME=" + homeDir, "USERPROFILE=" + homeDir},
	})
}

func codexSessionsRoot(homeDir string) string {
	return filepath.Join(homeDir, ".codex", "sessions")
}

func codexProviderSessionDetailURL(baseURL, sessionID string) string {
	query := url.Values{}
	query.Set("provider", string(factoryapi.Codex))
	query.Set("kind", string(factoryapi.LoadableProviderSessionKindSessionID))
	query.Set("id", sessionID)
	return strings.TrimSuffix(baseURL, "/") + "/provider-sessions/detail?" + query.Encode()
}

func getCodexProviderSessionDetail(
	t *testing.T,
	baseURL, sessionID string,
) factoryapi.ProviderSessionDetailResponse {
	t.Helper()
	return support.GetJSON[factoryapi.ProviderSessionDetailResponse](
		t,
		codexProviderSessionDetailURL(baseURL, sessionID),
	)
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

func writeCodexRolloutFixture(t *testing.T, root, sessionID, content string) {
	t.Helper()
	writeCodexRolloutFixtureAt(t, root, "2026/07/27", sessionID, content)
}

func writeCodexRolloutFixtureAt(t *testing.T, root, relativeDir, sessionID, content string) {
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

func representativeCodexJSONL() string {
	return strings.Join([]string{
		`{"timestamp":"2026-05-18T10:00:00Z","type":"turn_context"}`,
		`{"timestamp":"2026-05-18T10:00:01Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"Inspect the failing run."}]}}`,
		`{"timestamp":"2026-05-18T10:00:02Z","type":"response_item","payload":{"type":"reasoning","summary":["Checking tool output"]}}`,
		`{"timestamp":"2026-05-18T10:00:03Z","type":"response_item","payload":{"type":"function_call","call_id":"call-1","name":"exec_command","arguments":"go test ./pkg/api"}}`,
		`{"timestamp":"2026-05-18T10:00:04Z","type":"response_item","payload":{"type":"function_call_output","call_id":"call-1","output":"ok"}}`,
		`{"timestamp":"2026-05-18T10:00:05Z","type":"event_msg","payload":{"type":"agent_message","message":"The package tests passed."}}`,
		`{"timestamp":"2026-05-18T10:00:06Z","type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":100,"cached_input_tokens":40,"output_tokens":25,"reasoning_output_tokens":5,"total_tokens":130}}}}`,
	}, "\n") + "\n"
}

func assertCodexSharedAppendHistory(t *testing.T, fixture *codexSharedProcessFixture) {
	t.Helper()
	id := "session-append-tail"
	root := codexSessionsRoot(fixture.homeDir)
	writeCodexRolloutFixture(t, root, id, `{"type":"event_msg","payload":{"type":"agent_message","message":"before"}}`+"\n"+`{"type":"future"}`+"\n"+`{"type":"event_msg","payload":{"type":"agent_message","message":"after`)
	first := getCodexProviderSessionDetail(t, fixture.baseURL, id)
	if len(first.Transcript) != 1 || len(first.Parse.ParseErrors) != 1 || len(first.Parse.UnknownEvents) != 1 {
		t.Fatalf("partial tail: %#v", first)
	}
	path := filepath.Join(root, "2026", "07", "27", "rollout-"+id+".jsonl")
	writer, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, err = writer.WriteString(`"}}` + "\n")
	closeErr := writer.Close()
	if err != nil || closeErr != nil {
		t.Fatalf("complete tail: %v, %v", err, closeErr)
	}
	second := getCodexProviderSessionDetail(t, fixture.baseURL, id)
	if len(second.Transcript) != 2 || second.Transcript[1].Text == nil || *second.Transcript[1].Text != "after" || len(second.Parse.ParseErrors) != 0 {
		t.Fatalf("completed tail not visible exactly once: %#v", second)
	}
}

// This immutable storage fault is selected by a scenario-owned identity. Other
// histories continue through the real local filesystem on the same process.
type codexHistoryFaultFiles struct{}

func (codexHistoryFaultFiles) Open(path string) (io.ReadCloser, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	if filepath.Base(path) == "rollout-session-read-failure.jsonl" {
		return codexHistoryReadFailure{ReadCloser: file}, nil
	}
	return file, nil
}

func (codexHistoryFaultFiles) Stat(path string) (fs.FileInfo, error) { return os.Stat(path) }

type codexHistoryReadFailure struct{ io.ReadCloser }

func (codexHistoryReadFailure) Read([]byte) (int, error) {
	return 0, &fs.PathError{Op: "read", Path: "C:/secret/native-rollout", Err: fs.ErrPermission}
}

func assertCodexSharedEmptyHistory(t *testing.T, fixture *codexSharedProcessFixture) {
	t.Helper()
	detail := getCodexProviderSessionDetail(t, fixture.baseURL, "session-empty")
	if detail.ProviderSession.Id != "session-empty" || len(detail.Transcript) != 0 || detail.Parse.EventCount != 0 || detail.Parse.LineCount != 0 || detail.Parse.MalformedLineCount != 0 || detail.Parse.UnknownEventCount != 0 {
		t.Fatalf("empty detail fabricated facts: %#v", detail)
	}
}

func assertCodexSharedStorageFailureHistory(t *testing.T, fixture *codexSharedProcessFixture) {
	t.Helper()
	body := getCodexProviderSessionDetailErrorBody(t, fixture.baseURL, "session-read-failure", http.StatusInternalServerError)
	if !strings.Contains(body, "INTERNAL_ERROR") || strings.Contains(body, "secret") {
		t.Fatalf("unsafe storage failure: %s", body)
	}
	assertCodexProviderSessionErrorBodySafe(t, "read-failure", body, fixture.homeDir)
}

func assertCodexSharedDiagnosticBudgetHistory(t *testing.T, fixture *codexSharedProcessFixture) {
	t.Helper()
	detail := getCodexProviderSessionDetail(t, fixture.baseURL, "session-diagnostic-budget")
	if len(detail.Transcript) != 1 || detail.Transcript[0].Text == nil || *detail.Transcript[0].Text != "prefix" || len(detail.Parse.ParseErrors)+len(detail.Parse.UnknownEvents) != 256 {
		t.Fatalf("diagnostic exhaustion fabricated suffix or exceeded budget: %#v", detail)
	}
	if len(detail.Parse.ParseErrors) != 1 || detail.Parse.ParseErrors[0].Message != "inspection diagnostic limit reached" || detail.Parse.ParseErrors[0].LineNumber != 258 {
		t.Fatalf("missing terminal budget diagnostic: %#v", detail.Parse)
	}
}
