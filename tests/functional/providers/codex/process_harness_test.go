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
	body := getCodexProviderSessionDetailErrorBody(
		t,
		fixture.baseURL,
		codexFunctionalOversizedSessionID,
		http.StatusInternalServerError,
	)
	if !strings.Contains(body, "failed to load provider session details") {
		t.Fatalf("shared oversized-record error body = %q, want safe failure", body)
	}
	assertCodexProviderSessionErrorBodySafe(t, "oversized-record", body, fixture.homeDir)
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
		UseMockWorkers:            true,
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
