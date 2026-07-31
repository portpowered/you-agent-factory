package codex

import (
	"context"
	"encoding/json"
	"fmt"
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
	codexFunctionalMissingSessionID     = "session-missing-root"
	codexFunctionalMalformedSessionID   = "session-malformed-root"
	codexFunctionalBoundedWalkSessionID = "session-bounded-root"
	codexFunctionalCanceledSessionID    = "session-canceled-root"
	codexFunctionalOutsideSessionID     = "session-outside-root"
)

// TestCodexHistoricalInspectionSuccessThroughRootBuildProcess proves a stored
// Codex rollout is readable through the public provider-session detail surface
// composed from root.BuildProcess.
func TestCodexHistoricalInspectionSuccessThroughRootBuildProcess(t *testing.T) {
	homeDir := t.TempDir()
	writeCodexRolloutFixture(t, codexSessionsRoot(homeDir), codexFunctionalSessionID, representativeCodexJSONL())

	server := startCodexHistoricalInspectionServer(t, homeDir, serviceedges.Edges{})
	defer server.Stop(t)

	detail := getCodexProviderSessionDetail(t, server.URL(), codexFunctionalSessionID)
	if detail.ProviderSession.Id != codexFunctionalSessionID ||
		detail.ProviderSession.Provider != factoryapi.Codex ||
		detail.ProviderSession.Kind != factoryapi.LoadableProviderSessionKindSessionID {
		t.Fatalf("provider session = %#v, want codex session_id %s", detail.ProviderSession, codexFunctionalSessionID)
	}
	if detail.Source.RelativePath != "2026/07/27/rollout-"+codexFunctionalSessionID+".jsonl" {
		t.Fatalf("source path = %q, want contained rollout path", detail.Source.RelativePath)
	}
	if len(detail.Transcript) < 4 || len(detail.Parse.FunctionCalls) != 1 || len(detail.Parse.Reasoning) != 1 {
		t.Fatalf("detail = %#v, want transcript, tool, and reasoning facts", detail)
	}
	if detail.Parse.TokenUsage == nil || detail.Parse.TokenUsage.TotalTokens == nil ||
		*detail.Parse.TokenUsage.TotalTokens != 130 {
		t.Fatalf("token usage = %#v, want total 130", detail.Parse.TokenUsage)
	}
	if detail.Transcript[0].Text == nil || !strings.Contains(*detail.Transcript[0].Text, "Inspect the failing run") {
		t.Fatalf("transcript = %#v, want user message text", detail.Transcript)
	}
}

// TestCodexHistoricalInspectionDetachedRepeatedRunsThroughRootBuildProcess proves
// repeated root-built inspections return detached equivalent detail.
func TestCodexHistoricalInspectionDetachedRepeatedRunsThroughRootBuildProcess(t *testing.T) {
	homeDir := t.TempDir()
	writeCodexRolloutFixture(t, codexSessionsRoot(homeDir), codexFunctionalSessionID, representativeCodexJSONL())

	server := startCodexHistoricalInspectionServer(t, homeDir, serviceedges.Edges{})
	defer server.Stop(t)

	first := getCodexProviderSessionDetail(t, server.URL(), codexFunctionalSessionID)
	second := getCodexProviderSessionDetail(t, server.URL(), codexFunctionalSessionID)
	if first.Transcript[0].Text == nil || second.Transcript[0].Text == nil {
		t.Fatal("expected transcript text in both inspections")
	}
	if *first.Transcript[0].Text != *second.Transcript[0].Text {
		t.Fatalf("repeated inspections differ: %#v vs %#v", first.Transcript[0], second.Transcript[0])
	}
	mutated := "mutated transcript"
	first.Transcript[0].Text = &mutated
	if *second.Transcript[0].Text == mutated {
		t.Fatal("mutating first inspection affected second inspection transcript")
	}
}

// TestCodexHistoricalInspectionMissingSessionThroughRootBuildProcess proves a
// missing Codex session returns the accepted not-found outcome without leaking
// configured host storage paths.
func TestCodexHistoricalInspectionMissingSessionThroughRootBuildProcess(t *testing.T) {
	homeDir := t.TempDir()
	server := startCodexHistoricalInspectionServer(t, homeDir, serviceedges.Edges{})
	defer server.Stop(t)

	body := getCodexProviderSessionDetailErrorBody(
		t,
		server.URL(),
		codexFunctionalMissingSessionID,
		http.StatusNotFound,
	)
	if !strings.Contains(body, "provider session not found") {
		t.Fatalf("error body = %q, want not-found message", body)
	}
	assertCodexProviderSessionErrorBodySafe(t, "missing-session", body, homeDir)
}

// TestCodexHistoricalInspectionMalformedJSONLThroughRootBuildProcess proves
// truncated JSONL returns bounded parse diagnostics without fabricating
// transcript content or leaking host paths.
func TestCodexHistoricalInspectionMalformedJSONLThroughRootBuildProcess(t *testing.T) {
	homeDir := t.TempDir()
	content := `{"type":"turn_context"}` + "\n" +
		`{"type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"partial`
	writeCodexRolloutFixture(t, codexSessionsRoot(homeDir), codexFunctionalMalformedSessionID, content)

	server := startCodexHistoricalInspectionServer(t, homeDir, serviceedges.Edges{})
	defer server.Stop(t)

	detail := getCodexProviderSessionDetail(t, server.URL(), codexFunctionalMalformedSessionID)
	if detail.Parse.MalformedLineCount != 1 || len(detail.Parse.ParseErrors) != 1 {
		t.Fatalf("parse summary = %#v, want one malformed-line diagnostic", detail.Parse)
	}
	if detail.Parse.ParseErrors[0].Message != "truncated JSON event record" {
		t.Fatalf("parse error = %#v, want truncated JSON diagnostic", detail.Parse.ParseErrors[0])
	}
	if len(detail.Transcript) != 0 {
		t.Fatalf("transcript = %#v, want no fabricated entries", detail.Transcript)
	}
	encoded, err := json.Marshal(detail)
	if err != nil {
		t.Fatalf("marshal detail: %v", err)
	}
	assertCodexProviderSessionErrorBodySafe(t, "malformed-jsonl", string(encoded), homeDir)
}

// TestCodexHistoricalInspectionContainmentRejectionThroughRootBuildProcess proves
// symlink escapes fail safely through the root-composed detail surface.
func TestCodexHistoricalInspectionContainmentRejectionThroughRootBuildProcess(t *testing.T) {
	homeDir := t.TempDir()
	root := codexSessionsRoot(homeDir)
	outsideDir := t.TempDir()
	outsidePath := filepath.Join(outsideDir, "rollout-"+codexFunctionalOutsideSessionID+".jsonl")
	if err := os.WriteFile(outsidePath, []byte(`{"type":"session_meta"}`+"\n"), 0o600); err != nil {
		t.Fatalf("write outside fixture: %v", err)
	}
	linkDir := filepath.Join(root, "2026", "07", "27")
	if err := os.MkdirAll(linkDir, 0o755); err != nil {
		t.Fatalf("mkdir link dir: %v", err)
	}
	if err := os.Symlink(outsidePath, filepath.Join(linkDir, "rollout-"+codexFunctionalOutsideSessionID+".jsonl")); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("symlink capability unavailable: %v", err)
		}
		t.Fatalf("create symlink: %v", err)
	}

	server := startCodexHistoricalInspectionServer(t, homeDir, serviceedges.Edges{})
	defer server.Stop(t)

	body := getCodexProviderSessionDetailErrorBody(
		t,
		server.URL(),
		codexFunctionalOutsideSessionID,
		http.StatusInternalServerError,
	)
	if !strings.Contains(body, "failed to load provider session details") {
		t.Fatalf("error body = %q, want safe load failure", body)
	}
	assertCodexProviderSessionErrorBodySafe(t, "containment-rejection", body, homeDir)
}

// TestCodexHistoricalInspectionBoundedWalkThroughRootBuildProcess proves
// excessive discovery candidates terminate with a safe root outcome.
func TestCodexHistoricalInspectionBoundedWalkThroughRootBuildProcess(t *testing.T) {
	homeDir := t.TempDir()
	root := codexSessionsRoot(homeDir)
	for i := 0; i < 65; i++ {
		writeCodexRolloutFixtureAt(
			t,
			root,
			fmt.Sprintf("2026/05/%02d", i),
			codexFunctionalBoundedWalkSessionID,
			`{"type":"session_meta"}`+"\n",
		)
	}

	server := startCodexHistoricalInspectionServer(t, homeDir, serviceedges.Edges{})
	defer server.Stop(t)

	body := getCodexProviderSessionDetailErrorBody(
		t,
		server.URL(),
		codexFunctionalBoundedWalkSessionID,
		http.StatusInternalServerError,
	)
	if !strings.Contains(body, "failed to load provider session details") {
		t.Fatalf("error body = %q, want safe load failure", body)
	}
	assertCodexProviderSessionErrorBodySafe(t, "bounded-walk", body, homeDir)
}

// TestCodexHistoricalInspectionCancelledDiscoveryThroughRootBuildProcess proves
// injected discovery cancellation surfaces a safe root outcome without host-path
// leakage when reached only through root.BuildProcess and public contracts.
func TestCodexHistoricalInspectionCancelledDiscoveryThroughRootBuildProcess(t *testing.T) {
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
