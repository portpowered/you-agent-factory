package sessioncli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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

const sessionCLIProcessCloseTimeout = 5 * time.Second

var (
	sharedSessionCLICompositionOnce  sync.Once
	sharedSessionCLICompositionValue *sessionCLIComposition
	sharedSessionCLICompositionErr   error
	sharedSessionCLIScenarioSlot     = make(chan struct{}, 1)
	sharedSessionCLITopology         sessionCLITopologyLedger
)

// sessionCLIComposition owns the immutable root wiring and schema-level HTTP
// responder shared by this package. Process.Execute still receives fresh
// invocation-local input for every top-level case.
type sessionCLIComposition struct {
	process  support.ApplicationProcess
	server   *httptest.Server
	requests *sessionRequests

	closeOnce sync.Once
	closeErr  error
}

// sessionCLITopologyLedger records the package-owned resources without
// reaching into production state. The request ledger is reset at each case
// boundary, while the root and responder are closed once by TestMain.
type sessionCLITopologyLedger struct {
	sync.Mutex
	rootBuilds       int
	rootCloses       int
	responderStarts  int
	responderCloses  int
	scenariosStarted int
	scenariosEnded   int
}

// TestMain closes the package-scoped root and responder after all CLI cases
// have released their invocation-local state.
func TestMain(m *testing.M) {
	exitCode := m.Run()
	if sharedSessionCLICompositionValue != nil {
		if err := sharedSessionCLICompositionValue.close(); err != nil {
			fmt.Fprintf(os.Stderr, "close shared Session CLI composition: %v\n", err)
			if exitCode == 0 {
				exitCode = 1
			}
		}
	}
	if err := sharedSessionCLITopology.cleanupError(); err != nil {
		fmt.Fprintf(os.Stderr, "Session CLI cleanup accounting: %v\n", err)
		if exitCode == 0 {
			exitCode = 1
		}
	}
	fmt.Fprintf(os.Stderr, "GATE-CLI topology: %s\n", sharedSessionCLITopology.summary())
	os.Exit(exitCode)
}

func sharedSessionCLIComposition(t testing.TB) *sessionCLIComposition {
	t.Helper()
	sharedSessionCLICompositionOnce.Do(func() {
		sharedSessionCLICompositionValue, sharedSessionCLICompositionErr = newSessionCLIComposition()
	})
	if sharedSessionCLICompositionErr != nil {
		t.Fatalf("start shared Session CLI composition: %v", sharedSessionCLICompositionErr)
	}
	if sharedSessionCLICompositionValue == nil {
		t.Fatal("shared Session CLI composition is unavailable")
	}
	return sharedSessionCLICompositionValue
}

func newSessionCLIComposition() (*sessionCLIComposition, error) {
	requests := &sessionRequests{}
	server := httptest.NewServer(http.HandlerFunc(requests.handle))
	sharedSessionCLITopology.recordResponderStarted()

	process, err := support.BuildProcessWithContext(context.Background(), serviceedges.Edges{})
	if err != nil {
		server.Close()
		sharedSessionCLITopology.recordResponderClosed()
		return nil, fmt.Errorf("build root process: %w", err)
	}
	sharedSessionCLITopology.recordRootBuilt()
	return &sessionCLIComposition{
		process:  process,
		server:   server,
		requests: requests,
	}, nil
}

func (composition *sessionCLIComposition) beginScenario(t testing.TB) *sessionCLIInvocation {
	t.Helper()
	sharedSessionCLIScenarioSlot <- struct{}{}
	composition.requests.reset()
	sharedSessionCLITopology.recordScenarioStarted()
	t.Cleanup(func() {
		composition.requests.reset()
		sharedSessionCLITopology.recordScenarioEnded()
		<-sharedSessionCLIScenarioSlot
	})
	return &sessionCLIInvocation{
		composition:      composition,
		home:             t.TempDir(),
		workingDirectory: t.TempDir(),
	}
}

func (composition *sessionCLIComposition) close() error {
	if composition == nil {
		return nil
	}
	composition.closeOnce.Do(func() {
		var closeErrors []error
		if composition.process != nil {
			closeContext, cancel := context.WithTimeout(context.Background(), sessionCLIProcessCloseTimeout)
			closeErrors = append(closeErrors, composition.process.Close(closeContext))
			cancel()
			sharedSessionCLITopology.recordRootClosed()
		}
		if composition.server != nil {
			composition.server.Close()
			sharedSessionCLITopology.recordResponderClosed()
		}
		if got := composition.requests.count(); got != 0 {
			closeErrors = append(closeErrors, fmt.Errorf(
				"request ledger retained %d records after scenario cleanup", got,
			))
		}
		composition.closeErr = errors.Join(closeErrors...)
	})
	return composition.closeErr
}

func (ledger *sessionCLITopologyLedger) recordRootBuilt() {
	ledger.Lock()
	ledger.rootBuilds++
	ledger.Unlock()
}

func (ledger *sessionCLITopologyLedger) recordRootClosed() {
	ledger.Lock()
	ledger.rootCloses++
	ledger.Unlock()
}

func (ledger *sessionCLITopologyLedger) recordResponderStarted() {
	ledger.Lock()
	ledger.responderStarts++
	ledger.Unlock()
}

func (ledger *sessionCLITopologyLedger) recordResponderClosed() {
	ledger.Lock()
	ledger.responderCloses++
	ledger.Unlock()
}

func (ledger *sessionCLITopologyLedger) recordScenarioStarted() {
	ledger.Lock()
	ledger.scenariosStarted++
	ledger.Unlock()
}

func (ledger *sessionCLITopologyLedger) recordScenarioEnded() {
	ledger.Lock()
	ledger.scenariosEnded++
	ledger.Unlock()
}

func (ledger *sessionCLITopologyLedger) cleanupError() error {
	ledger.Lock()
	defer ledger.Unlock()
	var cleanupErrors []error
	if ledger.rootBuilds != ledger.rootCloses {
		cleanupErrors = append(cleanupErrors, fmt.Errorf(
			"root builds=%d closes=%d", ledger.rootBuilds, ledger.rootCloses,
		))
	}
	if ledger.responderStarts != ledger.responderCloses {
		cleanupErrors = append(cleanupErrors, fmt.Errorf(
			"responder starts=%d closes=%d", ledger.responderStarts, ledger.responderCloses,
		))
	}
	if ledger.scenariosStarted != ledger.scenariosEnded {
		cleanupErrors = append(cleanupErrors, fmt.Errorf(
			"scenarios started=%d ended=%d", ledger.scenariosStarted, ledger.scenariosEnded,
		))
	}
	if ledger.scenariosStarted > 0 && ledger.rootBuilds != 1 {
		cleanupErrors = append(cleanupErrors, fmt.Errorf(
			"root builds=%d, want exactly one shared build", ledger.rootBuilds,
		))
	}
	if ledger.scenariosStarted > 0 && ledger.responderStarts != 1 {
		cleanupErrors = append(cleanupErrors, fmt.Errorf(
			"responder starts=%d, want exactly one shared responder", ledger.responderStarts,
		))
	}
	return errors.Join(cleanupErrors...)
}

func (ledger *sessionCLITopologyLedger) summary() string {
	ledger.Lock()
	defer ledger.Unlock()
	return fmt.Sprintf(
		"root_builds=%d root_closes=%d responder_starts=%d responder_closes=%d scenarios=%d/%d",
		ledger.rootBuilds,
		ledger.rootCloses,
		ledger.responderStarts,
		ledger.responderCloses,
		ledger.scenariosStarted,
		ledger.scenariosEnded,
	)
}

type sessionCLIInvocation struct {
	composition      *sessionCLIComposition
	home             string
	workingDirectory string
}

type sessionCLIResult struct {
	stdout bytes.Buffer
	stderr bytes.Buffer
	err    error
}

func (invocation *sessionCLIInvocation) execute(args []string) sessionCLIResult {
	var result sessionCLIResult
	result.err = invocation.composition.process.Execute(root.Input{
		Args:             args,
		Env:              testHomeEnvironment(invocation.home),
		Stdout:           &result.stdout,
		Stderr:           &result.stderr,
		Context:          context.Background(),
		WorkingDirectory: invocation.workingDirectory,
	})
	return result
}

// TestBuildProcessRoutesEverySessionLeafThroughResolvedProductionComposition proves
// every public session CLI leaf command executes through root.BuildProcess against
// the resolved production composition without bypassing the customer process boundary.
func TestBuildProcessRoutesEverySessionLeafThroughResolvedProductionComposition(t *testing.T) {
	composition := sharedSessionCLIComposition(t)
	invocation := composition.beginScenario(t)

	port := testServerPort(t, composition.server.URL)
	invocations := [][]string{
		{"you", "--json", "session", "create", "--dir", invocation.workingDirectory,
			"--port", port, "--target-kind", "named", "--target-name", "alpha"},
		{"you", "--server", composition.server.URL, "--json", "session", "delete", "session-delete"},
		{"you", "--server", composition.server.URL, "--json", "--debug", "session", "list"},
		{"you", "--server", composition.server.URL, "--json", "session", "show", "session-show"},
		{"you", "--remote", "--server", composition.server.URL, "--json", "session", "pause"},
		{"you", "--remote", "--server", composition.server.URL, "--json", "session", "resume", "session-resume"},
	}
	for _, args := range invocations {
		result := invocation.execute(args)
		if result.err != nil {
			t.Fatalf("Process.Execute(%v) error = %v; stderr=%q", args, result.err, result.stderr.String())
		}
		if result.stdout.Len() == 0 {
			t.Fatalf("Process.Execute(%v) stdout is empty", args)
		}
	}

	failedShow := invocation.execute([]string{
		"you", "--server", composition.server.URL, "session", "show",
	})
	if failedShow.err == nil {
		t.Fatal("Process.Execute(default session show) error = nil")
	}
	if failedShow.stdout.Len() != 0 {
		t.Fatalf("default session show stdout = %q, want empty", failedShow.stdout.String())
	}

	composition.requests.assert(t)
	before := composition.requests.count()
	rejected := invocation.execute([]string{
		"you", "--server", composition.server.URL, "session", "show",
		"--port", port, "session-rejected",
	})
	if rejected.err == nil {
		t.Fatal("Process.Execute(deprecated port) error = nil")
	}
	if composition.requests.count() != before {
		t.Fatalf("deprecated port request count = %d, want %d", composition.requests.count(), before)
	}
	if rejected.stdout.Len() != 0 {
		t.Fatalf("deprecated port stdout = %q, want empty", rejected.stdout.String())
	}
}

// TestBuildProcessRejectsDeprecatedPortBeforeSubmitDispatch proves submit rejects
// deprecated --port wiring before any dispatch attempt and directs callers to --server.
func TestBuildProcessRejectsDeprecatedPortBeforeSubmitDispatch(t *testing.T) {
	composition := sharedSessionCLIComposition(t)
	invocation := composition.beginScenario(t)
	// Keep the payload in the already fresh case-owned working directory so the
	// deprecated-option witness does not need a third temporary root.
	payloadPath := filepath.Join(invocation.workingDirectory, "request.md")
	if err := os.WriteFile(payloadPath, []byte("must not be submitted"), 0o600); err != nil {
		t.Fatalf("write payload: %v", err)
	}
	result := invocation.execute([]string{
		"you", "submit", "--port", "9090", "--name", "rejected",
		"--work-type-name", "task", "--payload", payloadPath,
	})
	if result.err == nil || !strings.Contains(result.err.Error(), "--port is no longer supported; use --server") {
		t.Fatalf("deprecated port error = %v", result.err)
	}
	if composition.requests.count() != 0 {
		t.Fatalf("deprecated submit request count = %d, want 0", composition.requests.count())
	}
	if result.stdout.Len() != 0 {
		t.Fatalf("deprecated port stdout = %q, want empty", result.stdout.String())
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

func (requests *sessionRequests) reset() {
	requests.mu.Lock()
	requests.requests = nil
	requests.mu.Unlock()
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
