package support

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
)

func TestLoadProviderSessionCase_LoadsTrackedHarnessFixture(t *testing.T) {
	repoRoot := testutil.MustRepoRoot(t)
	caseDir := filepath.Join(repoRoot, filepath.FromSlash(ProviderSessionFixturePath("harness", "load-smoke")))

	loaded, err := LoadProviderSessionCase(caseDir)
	if err != nil {
		t.Fatalf("LoadProviderSessionCase: %v", err)
	}

	if loaded.Manifest.ID != "harness-load-smoke" {
		t.Fatalf("manifest.ID = %q, want harness-load-smoke", loaded.Manifest.ID)
	}
	if loaded.Manifest.Provider != "harness" {
		t.Fatalf("manifest.Provider = %q, want harness", loaded.Manifest.Provider)
	}

	var request map[string]any
	if err := json.Unmarshal(loaded.Request, &request); err != nil {
		t.Fatalf("decode loaded request: %v", err)
	}
	if got, _ := request["session_id"].(string); got != "session_fixture_load_smoke" {
		t.Fatalf("request.session_id = %q, want session_fixture_load_smoke", got)
	}

	if loaded.Process.Provider != "harness" || loaded.Process.Model != "fixture-model" {
		t.Fatalf("process provider/model = %q/%q, want harness/fixture-model", loaded.Process.Provider, loaded.Process.Model)
	}
	if loaded.Process.ExitCode == nil || *loaded.Process.ExitCode != 0 {
		t.Fatalf("process.exitCode = %#v, want 0", loaded.Process.ExitCode)
	}
	if !loaded.Process.StdoutStream || loaded.Process.StderrStream {
		t.Fatalf("process stream flags stdout=%v stderr=%v, want true/false", loaded.Process.StdoutStream, loaded.Process.StderrStream)
	}
	if loaded.Process.WorkingDirectoryRole != "factory-root" {
		t.Fatalf("process.workingDirectoryRole = %q, want factory-root", loaded.Process.WorkingDirectoryRole)
	}
	if loaded.Process.TimeoutCancelClass != "none" || loaded.Process.TerminalErrorClass != "none" {
		t.Fatalf("process classes = %q/%q, want none/none", loaded.Process.TimeoutCancelClass, loaded.Process.TerminalErrorClass)
	}
	if len(loaded.Process.Argv) == 0 {
		t.Fatal("process.argv must expose sanitized argv")
	}

	if loaded.Stdout.MediaType != ProviderSessionStdoutJSONL {
		t.Fatalf("stdout media type = %q, want %q", loaded.Stdout.MediaType, ProviderSessionStdoutJSONL)
	}
	if len(loaded.Stdout.Records) != 2 {
		t.Fatalf("stdout records = %d, want 2", len(loaded.Stdout.Records))
	}
	if strings.TrimSpace(loaded.Stderr) != "" {
		t.Fatalf("stderr = %q, want empty", loaded.Stderr)
	}

	var expectedSession map[string]any
	if err := json.Unmarshal(loaded.Expected.ProviderSession, &expectedSession); err != nil {
		t.Fatalf("decode expected provider session: %v", err)
	}
	if got, _ := expectedSession["providerSessionId"].(string); got != "session_fixture_load_smoke" {
		t.Fatalf("expected providerSessionId = %q, want session_fixture_load_smoke", got)
	}
	if len(loaded.Expected.ResponseEvents) != 2 {
		t.Fatalf("expected response events = %d, want 2", len(loaded.Expected.ResponseEvents))
	}
	var expectedResult map[string]any
	if err := json.Unmarshal(loaded.Expected.InvocationResult, &expectedResult); err != nil {
		t.Fatalf("decode expected invocation result: %v", err)
	}
	if got, _ := expectedResult["content"].(string); got != "Hello" {
		t.Fatalf("expected invocation content = %q, want Hello", got)
	}
}

func TestLoadProviderSessionStdoutArtifact_SupportsJSONAndTextShapes(t *testing.T) {
	t.Run("json", func(t *testing.T) {
		caseDir := writeProviderSessionLoadCase(t, func(manifest *ProviderSessionGoldenManifest, files map[string][]byte) {
			manifest.StdoutFile = "stdout.json"
			files["stdout.json"] = []byte(`{"type":"result","session_id":"session_fixture_json","finish_reason":"stop"}` + "\n")
			delete(files, "stdout.jsonl")
		})

		loaded, err := LoadProviderSessionCase(caseDir)
		if err != nil {
			t.Fatalf("LoadProviderSessionCase: %v", err)
		}
		if loaded.Stdout.MediaType != ProviderSessionStdoutJSON {
			t.Fatalf("media type = %q, want %q", loaded.Stdout.MediaType, ProviderSessionStdoutJSON)
		}
		var decoded map[string]any
		if err := json.Unmarshal(loaded.Stdout.JSON, &decoded); err != nil {
			t.Fatalf("decode stdout json: %v", err)
		}
		if got, _ := decoded["session_id"].(string); got != "session_fixture_json" {
			t.Fatalf("stdout.session_id = %q, want session_fixture_json", got)
		}
	})

	t.Run("text", func(t *testing.T) {
		caseDir := writeProviderSessionLoadCase(t, func(manifest *ProviderSessionGoldenManifest, files map[string][]byte) {
			manifest.StdoutFile = "stdout.txt"
			files["stdout.txt"] = []byte("plain provider stdout\n")
			delete(files, "stdout.jsonl")
		})

		loaded, err := LoadProviderSessionCase(caseDir)
		if err != nil {
			t.Fatalf("LoadProviderSessionCase: %v", err)
		}
		if loaded.Stdout.MediaType != ProviderSessionStdoutText {
			t.Fatalf("media type = %q, want %q", loaded.Stdout.MediaType, ProviderSessionStdoutText)
		}
		if loaded.Stdout.Text != "plain provider stdout\n" {
			t.Fatalf("stdout text = %q, want plain provider stdout\\n", loaded.Stdout.Text)
		}
	})
}

func TestLoadProviderSessionCase_RejectsIncompleteProcessMetadata(t *testing.T) {
	caseDir := writeProviderSessionLoadCase(t, func(_ *ProviderSessionGoldenManifest, files map[string][]byte) {
		files["process.json"] = []byte(`{
  "argv": ["provider-cli"],
  "provider": "harness",
  "model": "fixture-model",
  "exitCode": 0,
  "stdoutStream": true,
  "stderrStream": false,
  "workingDirectoryRole": "factory-root",
  "timeoutCancelClass": "none"
}` + "\n")
	})

	_, err := LoadProviderSessionCase(caseDir)
	assertProviderSessionLoadError(t, err, "codex-message-tool-success", "process", "terminalErrorClass")
}

func TestLoadProviderSessionCase_RejectsUnsanitizedMaterialBeforeReturningArtifacts(t *testing.T) {
	caseDir := writeProviderSessionLoadCase(t, func(_ *ProviderSessionGoldenManifest, files map[string][]byte) {
		files["request.json"] = []byte(`{"headers":{"Authorization":"Bearer sk-live-loader-secret-token"}}` + "\n")
	})

	_, err := LoadProviderSessionCase(caseDir)
	assertProviderSessionSanitizeError(t, err, "codex-message-tool-success", ProviderSessionForbiddenCredential, "", "headers.Authorization")
}

func writeProviderSessionLoadCase(t *testing.T, mutate func(*ProviderSessionGoldenManifest, map[string][]byte)) string {
	t.Helper()

	manifest := validProviderSessionManifest()
	exitCode := 0
	files := map[string][]byte{
		"request.json": []byte(`{"prompt":"fixture prompt","session_id":"session_fixture_001"}` + "\n"),
		"process.json": mustJSONBytes(t, ProviderSessionProcessMetadata{
			Argv:                 []string{"provider-cli", "--model", "fixture-model"},
			Provider:             "codex",
			Model:                "fixture-model",
			ExitCode:             &exitCode,
			StdoutStream:         true,
			StderrStream:         false,
			WorkingDirectoryRole: "factory-root",
			TimeoutCancelClass:   "none",
			TerminalErrorClass:   "none",
		}),
		"stdout.jsonl":                      []byte(`{"type":"result","session_id":"session_fixture_001","finish_reason":"stop"}` + "\n"),
		"stderr.txt":                        []byte(""),
		"expected-provider-session.json":    []byte(`{"provider":"codex","providerSessionId":"session_fixture_001","status":"completed"}` + "\n"),
		"expected-response-events.ndjson":   []byte(`{"type":"message.completed","itemId":"item_fixture_1"}` + "\n"),
		"expected-invocation-result.json":   []byte(`{"ok":true,"content":"done","finishReason":"stop"}` + "\n"),
	}
	if mutate != nil {
		mutate(&manifest, files)
	}

	caseDir := t.TempDir()
	raw, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(caseDir, ProviderSessionGoldenManifestFile), append(raw, '\n'), 0o644); err != nil {
		t.Fatalf("write manifest.json: %v", err)
	}

	pointers := []string{
		manifest.RequestFile,
		manifest.ProcessFile,
		manifest.StdoutFile,
		manifest.StderrFile,
		manifest.ExpectedProviderSessionFile,
		manifest.ExpectedResponseEventsFile,
		manifest.ExpectedInvocationResultFile,
	}
	for _, name := range pointers {
		if strings.TrimSpace(name) == "" || filepath.IsAbs(name) || strings.Contains(name, "..") {
			continue
		}
		content, ok := files[name]
		if !ok {
			content = []byte("{}\n")
		}
		path := filepath.Join(caseDir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir for %s: %v", name, err)
		}
		if err := os.WriteFile(path, content, 0o644); err != nil {
			t.Fatalf("write fixture %s: %v", name, err)
		}
	}
	return caseDir
}

func mustJSONBytes(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatalf("marshal json: %v", err)
	}
	return append(raw, '\n')
}

func assertProviderSessionLoadError(t *testing.T, err error, wantCaseID, wantRole, wantField string) {
	t.Helper()
	if err == nil {
		t.Fatal("expected load error, got nil")
	}
	var loadErr *ProviderSessionLoadError
	if !errors.As(err, &loadErr) {
		t.Fatalf("error type = %T (%v), want *ProviderSessionLoadError", err, err)
	}
	message := loadErr.Error()
	if wantCaseID != "" && !strings.Contains(message, wantCaseID) {
		t.Fatalf("error %q does not name case id %q", message, wantCaseID)
	}
	if wantRole != "" && loadErr.Role != wantRole {
		t.Fatalf("error role = %q, want %q (message=%q)", loadErr.Role, wantRole, message)
	}
	if wantField != "" && loadErr.Field != wantField {
		t.Fatalf("error field = %q, want %q (message=%q)", loadErr.Field, wantField, message)
	}
}
