package sessioncli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

var resolvedSessionCLIProcess support.ApplicationProcess

func TestMain(m *testing.M) {
	process, err := support.BuildProcessWithContext(context.Background(), serviceedges.Edges{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "build resolved-session CLI process: %v\n", err)
		os.Exit(1)
	}
	resolvedSessionCLIProcess = process
	exitCode := m.Run()
	closeContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := process.Close(closeContext); err != nil {
		fmt.Fprintf(os.Stderr, "close resolved-session CLI process: %v\n", err)
		exitCode = 1
	}
	os.Exit(exitCode)
}

// TestBuildProcessRoutesEverySessionLeafThroughResolvedProductionComposition proves
// every public session CLI leaf command executes through root.BuildProcess against
// the resolved production composition without bypassing the customer process boundary.
func TestBuildProcessRoutesEverySessionLeafThroughResolvedProductionComposition(t *testing.T) {
	t.Parallel()
	var requests sessionRequests
	server := httptest.NewServer(http.HandlerFunc(requests.handle))
	defer server.Close()

	port := testServerPort(t, server.URL)
	process := resolvedSessionCLIProcess
	home := t.TempDir()
	workingDirectory := t.TempDir()
	invocations := [][]string{
		{"you", "--json", "session", "create", "--dir", workingDirectory,
			"--port", port, "--target-kind", "named", "--target-name", "alpha"},
		{"you", "--server", server.URL, "--json", "session", "delete", "session-delete"},
		{"you", "--server", server.URL, "--json", "--debug", "session", "list"},
		{"you", "--server", server.URL, "--json", "session", "show", "session-show"},
		{"you", "--remote", "--server", server.URL, "--json", "session", "pause"},
		{"you", "--remote", "--server", server.URL, "--json", "session", "resume", "session-resume"},
	}
	for _, args := range invocations {
		var stdout, stderr bytes.Buffer
		if err := process.Execute(root.Input{
			Args: args, Env: testHomeEnvironment(home),
			Stdout: &stdout, Stderr: &stderr,
			Context: context.Background(), WorkingDirectory: workingDirectory,
		}); err != nil {
			t.Fatalf("Process.Execute(%v) error = %v; stderr=%q", args, err, stderr.String())
		}
		if stdout.Len() == 0 {
			t.Fatalf("Process.Execute(%v) stdout is empty", args)
		}
	}

	var failedShowOutput bytes.Buffer
	err := process.Execute(root.Input{
		Args: []string{
			"you", "--server", server.URL, "session", "show",
		},
		Env: testHomeEnvironment(home), Stdout: &failedShowOutput,
		Context: context.Background(), WorkingDirectory: workingDirectory,
	})
	if err == nil {
		t.Fatal("Process.Execute(default session show) error = nil")
	}
	if failedShowOutput.Len() != 0 {
		t.Fatalf("default session show stdout = %q, want empty", failedShowOutput.String())
	}

	requests.assert(t)
	before := requests.count()
	var rejectedOutput bytes.Buffer
	err = process.Execute(root.Input{
		Args: []string{
			"you", "--server", server.URL, "session", "show",
			"--port", port, "session-rejected",
		},
		Env: testHomeEnvironment(home), Stdout: &rejectedOutput,
		Context: context.Background(), WorkingDirectory: workingDirectory,
	})
	if err == nil {
		t.Fatal("Process.Execute(deprecated port) error = nil")
	}
	if requests.count() != before {
		t.Fatalf("deprecated port request count = %d, want %d", requests.count(), before)
	}
	if rejectedOutput.Len() != 0 {
		t.Fatalf("deprecated port stdout = %q, want empty", rejectedOutput.String())
	}
}

// TestBuildProcessRejectsDeprecatedPortBeforeSubmitDispatch proves submit rejects
// deprecated --port wiring before any dispatch attempt and directs callers to --server.
func TestBuildProcessRejectsDeprecatedPortBeforeSubmitDispatch(t *testing.T) {
	t.Parallel()
	process := resolvedSessionCLIProcess
	home := t.TempDir()
	payloadPath := filepath.Join(t.TempDir(), "request.md")
	if err := os.WriteFile(payloadPath, []byte("must not be submitted"), 0o600); err != nil {
		t.Fatalf("write payload: %v", err)
	}
	var stdout bytes.Buffer
	err := process.Execute(root.Input{
		Args: []string{
			"you", "submit", "--port", "9090", "--name", "rejected",
			"--work-type-name", "task", "--payload", payloadPath,
		},
		Env:              testHomeEnvironment(home),
		Stdout:           &stdout,
		Context:          context.Background(),
		WorkingDirectory: home,
	})
	if err == nil || !strings.Contains(err.Error(), "--port is no longer supported; use --server") {
		t.Fatalf("deprecated port error = %v", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("deprecated port stdout = %q, want empty", stdout.String())
	}
}

type capturedRequest struct {
	method string
	path   string
	query  url.Values
	body   map[string]any
}

type sessionRequests struct {
	mu       sync.Mutex
	requests []capturedRequest
}

func (requests *sessionRequests) handle(writer http.ResponseWriter, request *http.Request) {
	captured := capturedRequest{
		method: request.Method,
		path:   request.URL.Path,
		query:  request.URL.Query(),
	}
	if request.Body != nil && request.Method == http.MethodPost &&
		request.URL.Path == "/factory-sessions" {
		_ = json.NewDecoder(request.Body).Decode(&captured.body)
	}
	requests.mu.Lock()
	requests.requests = append(requests.requests, captured)
	requests.mu.Unlock()

	writer.Header().Set("Content-Type", "application/json")
	switch {
	case request.Method == http.MethodPost && request.URL.Path == "/factory-sessions":
		_, _ = fmt.Fprint(writer, `{"session":{"factoryDir":"factory","folderPath":"workspace","id":"session-created","isDefault":false,"project":"alpha","target":{"kind":"named","name":"alpha"}}}`)
	case request.Method == http.MethodDelete:
		writer.WriteHeader(http.StatusNoContent)
	case request.Method == http.MethodGet && request.URL.Path == "/factory-sessions":
		_, _ = fmt.Fprint(writer, `{"sessions":[]}`)
	case request.Method == http.MethodGet && request.URL.Path == "/factory-sessions/session-show":
		_, _ = fmt.Fprint(writer, `{"id":"session-show","runtime":{"orchestratorKind":"JAVASCRIPT"}}`)
	case request.Method == http.MethodGet && request.URL.Path == "/factory-sessions/~default":
		http.Error(writer, "default session unavailable", http.StatusServiceUnavailable)
	case request.Method == http.MethodGet:
		_, _ = fmt.Fprint(writer, `{"sessionId":"dur-sess-1","dispatches":[]}`)
	case request.URL.Path == "/factory-sessions/~default/pause":
		_, _ = fmt.Fprint(writer, `{"sessionId":"~default","operation":"PAUSE","outcome":"ACCEPTED","status":"PAUSED"}`)
	case request.URL.Path == "/factory-sessions/session-resume/resume":
		_, _ = fmt.Fprint(writer, `{"sessionId":"session-resume","operation":"RESUME","outcome":"ACCEPTED","status":"RUNNING"}`)
	default:
		http.Error(writer, "unexpected request", http.StatusNotFound)
	}
}

func (requests *sessionRequests) count() int {
	requests.mu.Lock()
	defer requests.mu.Unlock()
	return len(requests.requests)
}

func (requests *sessionRequests) assert(t *testing.T) {
	t.Helper()
	requests.mu.Lock()
	defer requests.mu.Unlock()
	if len(requests.requests) != 7 {
		t.Fatalf("request count = %d, want 7: %#v", len(requests.requests), requests.requests)
	}
	want := []capturedRequest{
		{method: http.MethodPost, path: "/factory-sessions"},
		{method: http.MethodDelete, path: "/factory-sessions/session-delete"},
		{method: http.MethodGet, path: "/factory-sessions"},
		{method: http.MethodGet, path: "/factory-sessions/session-show"},
		{method: http.MethodPost, path: "/factory-sessions/~default/pause"},
		{method: http.MethodPost, path: "/factory-sessions/session-resume/resume"},
		{method: http.MethodGet, path: "/factory-sessions/~default"},
	}
	for index, expected := range want {
		got := requests.requests[index]
		if got.method != expected.method || got.path != expected.path {
			t.Fatalf("request[%d] = %s %s, want %s %s", index, got.method, got.path, expected.method, expected.path)
		}
	}
	create := requests.requests[0].body
	if create["folderPath"] == "" {
		t.Fatalf("create request = %#v, want resolved folder path", create)
	}
	if target, ok := create["target"].(map[string]any); !ok ||
		target["kind"] != "named" || target["name"] != "alpha" {
		t.Fatalf("create target = %#v, want named alpha", create["target"])
	}
}

func testServerPort(t *testing.T, serverURL string) string {
	t.Helper()
	parsed, err := url.Parse(serverURL)
	if err != nil {
		t.Fatalf("parse test server URL: %v", err)
	}
	_, port, err := net.SplitHostPort(parsed.Host)
	if err != nil {
		t.Fatalf("split test server host: %v", err)
	}
	if _, err := strconv.Atoi(port); err != nil {
		t.Fatalf("test server port %q: %v", port, err)
	}
	return port
}

func testHomeEnvironment(home string) []string {
	if runtime.GOOS == "windows" {
		return []string{"USERPROFILE=" + home}
	}
	if runtime.GOOS == "plan9" {
		return []string{"home=" + home}
	}
	return []string{"HOME=" + home}
}
