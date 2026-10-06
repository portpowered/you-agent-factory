package recording_sidecar_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// This deliberately small real-edge witness consumes a prebuilt you artifact
// and the installed Codex, never a shim. Inference is a loopback-only Responses
// fixture returning one final message; the request proves model-visible roles.
func TestLongWorkerPromptWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows CreateProcess witness")
	}
	t.Parallel()
	binary := invokeArtifactBinary(t)
	codex, err := exec.LookPath("codex.exe")
	if err != nil {
		t.Fatal("supported real Codex missing: ", err)
	}
	version, err := exec.CommandContext(t.Context(), codex, "--version").Output()
	if err != nil {
		t.Fatal(err)
	}
	project, home, temp, codexHome := newLongPromptProject(t)
	server, capture := newLongPromptResponses(t)
	config := fmt.Sprintf("model_provider = \"loopback\"\nmodel = \"long-prompt-model\"\ncli_auth_credentials_store = \"file\"\n[features]\napps = false\nplugins = false\n[model_providers.loopback]\nname = \"Loopback\"\nbase_url = %q\nwire_api = \"responses\"\nrequires_openai_auth = false\nrequest_max_retries = 0\nstream_max_retries = 0\n", server.URL)
	if err := os.WriteFile(filepath.Join(codexHome, "config.toml"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	developer := "Explain the literal project_root_markers in this document.\n  read \"quoted\" C:\\workspace\\file\tUnicode café 😀\r\n" + strings.Repeat("exact developer instructions ", 1400) + "\r\n  "
	user := "  identical user café 😀\r\n\t  "
	document, err := json.Marshal(map[string]any{
		"requestId": "long-prompt-request", "workerSessionId": "long-prompt-session",
		"execution": map[string]any{
			"workstationName": "direct", "workingDirectory": project, "workerType": "direct-worker",
			"runnerId": "codex", "executorProvider": "codex", "modelProvider": "codex", "model": "long-prompt-model",
			"skipPermissions": true, "systemPrompt": developer, "userMessage": user,
			"envVars":  map[string]string{"CODEX_HOME": codexHome},
			"dispatch": map[string]any{"dispatchId": "long-prompt-attempt", "workstationName": "direct", "workerType": "direct-worker"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(project, "execution.json")
	if err := os.WriteFile(path, document, 0o600); err != nil {
		t.Fatal(err)
	}
	env := longPromptEnvironment(home, temp, codexHome, filepath.Dir(codex))
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	t.Cleanup(cancel)
	command := exec.CommandContext(ctx, binary, "--json", "worker-sessions", "invoke", "--execution", path)
	command.Dir, command.Env = project, env
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	err = command.Run()
	capture.mu.Lock()
	captured := append([][]byte(nil), capture.requests...)
	capture.mu.Unlock()
	if err != nil || !bytes.Contains(stdout.Bytes(), []byte(`"COMPLETED"`)) {
		t.Fatalf("real Windows invocation exit=%v requests=%d stdout=%s stderr=%s", err, len(captured), &stdout, &stderr)
	}
	if len(captured) != 1 {
		t.Fatalf("inference calls=%d, want one", len(captured))
	}
	assertLongPromptModelInput(t, captured[0], developer, user)
	residue, err := filepath.Glob(filepath.Join(codexHome, "you-prompt-*.config.toml"))
	if err != nil || len(residue) != 0 {
		t.Fatalf("own profile residue=%v (%v)", residue, err)
	}
	base, err := os.ReadFile(filepath.Join(codexHome, "config.toml"))
	if err != nil || string(base) != config {
		t.Fatal("base user config was changed")
	}
	t.Logf("Codex=%s OS=%s command=%s --json worker-sessions invoke --execution <owned execution.json>; exit=0; developer sha256=%x user sha256=%x config sha256=%x fixture sha256=%x requests=1 state=COMPLETED profiles=0", strings.TrimSpace(string(version)), runtime.GOOS, binary, sha256.Sum256([]byte(developer)), sha256.Sum256([]byte(user)), sha256.Sum256([]byte(config)), sha256.Sum256([]byte(strings.Join(longPromptResponseEvents(), "\n"))))
}

type longPromptCapture struct {
	mu       sync.Mutex
	requests [][]byte
}

func newLongPromptProject(t *testing.T) (string, string, string, string) {
	t.Helper()
	project := t.TempDir()
	home, temp, codexHome := filepath.Join(project, "home"), filepath.Join(project, "temp"), filepath.Join(project, "codex-home")
	for _, path := range []string{home, temp, codexHome, filepath.Join(project, "factory"), filepath.Join(project, ".git")} {
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	definition := `{"name":"long-prompt","workTypes":[{"name":"task","states":[{"name":"init","type":"INITIAL"},{"name":"done","type":"TERMINAL"}]}],"workers":[],"workstations":[]}`
	if err := os.WriteFile(filepath.Join(project, "factory", "factory.json"), []byte(definition), 0o600); err != nil {
		t.Fatal(err)
	}
	return project, home, temp, codexHome
}

func newLongPromptResponses(t *testing.T) (*httptest.Server, *longPromptCapture) {
	t.Helper()
	capture := &longPromptCapture{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/responses" {
			http.Error(w, "unexpected path", http.StatusNotFound)
			return
		}
		raw, err := io.ReadAll(io.LimitReader(r.Body, 2<<20))
		if err != nil {
			http.Error(w, "read failure", 500)
			return
		}
		capture.mu.Lock()
		capture.requests = append(capture.requests, raw)
		count := len(capture.requests)
		capture.mu.Unlock()
		if count > 2 {
			http.Error(w, "loopback request budget exceeded", 429)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		for _, event := range longPromptResponseEvents() {
			fmt.Fprintf(w, "data: %s\n\n", event)
		}
	}))
	t.Cleanup(server.Close)
	return server, capture
}

func longPromptEnvironment(home, temp, codexHome, codexBin string) []string {
	env := cleanupEnvironment(home, temp)
	result := make([]string, 0, len(env)+2)
	for _, entry := range env {
		key, value, _ := strings.Cut(entry, "=")
		upper := strings.ToUpper(key)
		if upper == "CODEX_HOME" || strings.Contains(upper, "API_KEY") || strings.Contains(upper, "TOKEN") || strings.Contains(upper, "BASE_URL") {
			continue
		}
		if upper == "PATH" {
			entry = key + "=" + codexBin + string(os.PathListSeparator) + value
		}
		result = append(result, entry)
	}
	return append(result, "CODEX_HOME="+codexHome)
}

func assertLongPromptModelInput(t *testing.T, raw []byte, developer, user string) {
	t.Helper()
	var request struct {
		Input []struct {
			Role    string
			Content []struct{ Text string }
		}
	}
	if err := json.Unmarshal(raw, &request); err != nil {
		t.Fatal(err)
	}
	developerSeen, userSeen := false, false
	for _, input := range request.Input {
		for _, content := range input.Content {
			if input.Role == "developer" && strings.Contains(content.Text, developer) {
				developerSeen = true
			}
			if input.Role == "user" && content.Text == user {
				userSeen = true
			}
		}
	}
	if !developerSeen || !userSeen {
		t.Fatalf("model-visible byte/role preservation failed: developer=%t user=%t body=%s", developerSeen, userSeen, raw)
	}
}

func longPromptResponseEvents() []string {
	message := `{"id":"msg-long-prompt","type":"message","status":"completed","role":"assistant","content":[{"type":"output_text","text":"COMPLETE","annotations":[]}]}`
	return []string{
		`{"type":"response.created","response":{"id":"resp-long-prompt","object":"response","status":"in_progress","output":[]}}`,
		`{"type":"response.output_item.added","output_index":0,"item":{"id":"msg-long-prompt","type":"message","role":"assistant","content":[]}}`,
		`{"type":"response.output_text.delta","item_id":"msg-long-prompt","output_index":0,"content_index":0,"delta":"COMPLETE"}`,
		`{"type":"response.output_item.done","output_index":0,"item":` + message + `}`,
		`{"type":"response.completed","response":{"id":"resp-long-prompt","object":"response","status":"completed","output":[` + message + `],"usage":{"input_tokens":2,"output_tokens":1,"total_tokens":3}}}`,
	}
}
