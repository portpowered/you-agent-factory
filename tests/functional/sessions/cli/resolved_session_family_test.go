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
	"runtime"
	"strconv"
	"sync"
	"testing"

	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
)

func TestBuildProcessRoutesEverySessionLeafThroughResolvedProductionComposition(t *testing.T) {
	var requests sessionRequests
	server := httptest.NewServer(http.HandlerFunc(requests.handle))
	defer server.Close()

	port := testServerPort(t, server.URL)
	process, err := root.BuildProcess(context.Background(), serviceedges.Edges{})
	if err != nil {
		t.Fatalf("BuildProcess() error = %v", err)
	}
	home := t.TempDir()
	workingDirectory := t.TempDir()
	invocations := [][]string{
		{"you", "--json", "session", "create", "--dir", workingDirectory,
			"--port", port, "--target-kind", "named", "--target-name", "alpha"},
		{"you", "--json", "session", "delete", "session-delete", "--port", port},
		{"you", "--server", server.URL, "--json", "--debug", "session", "list"},
		{"you", "--server", server.URL, "--json", "session", "show", "session-show"},
		{"you", "--server", server.URL, "--json", "session", "dispatches",
			"dur-sess-1", "--phase", "execute", "--status", "SUCCEEDED"},
		{"you", "--server", server.URL, "--json", "session", "pause"},
		{"you", "--server", server.URL, "--json", "session", "resume", "session-resume"},
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
		{method: http.MethodGet, path: "/factory-sessions/dur-sess-1/dispatches"},
		{method: http.MethodPost, path: "/factory-sessions/~default/pause"},
		{method: http.MethodPost, path: "/factory-sessions/session-resume/resume"},
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
	dispatches := requests.requests[4].query
	if dispatches.Get("phase") != "execute" || dispatches.Get("status") != "SUCCEEDED" {
		t.Fatalf("dispatch query = %v", dispatches)
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
