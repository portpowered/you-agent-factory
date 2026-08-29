package submit_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

var packageSubmitFixture *submitFixture

func TestMain(m *testing.M) {
	ledger := newSubmitLifecycleLedger()
	providerRunner := &submitProviderCommandRunner{}
	apiStarter := &submitAPIServerStarter{ready: make(chan *support.ProcessAPIServer, 16), ledger: ledger}
	process, err := support.BuildProcessWithContext(context.Background(), serviceedges.Edges{
		APIServerStarter:      apiStarter.Start,
		ProviderCommandRunner: providerRunner,
	})
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "build submit functional process: %v\n", err)
		os.Exit(1)
	}
	packageSubmitFixture = &submitFixture{
		process: process,
		runner:  providerRunner,
		starter: apiStarter,
		ledger:  ledger,
	}
	ledger.processStarted()

	code := m.Run()
	closeErr := process.Close(context.Background())
	ledger.processClosed()
	if closeErr != nil {
		_, _ = fmt.Fprintf(os.Stderr, "close submit functional process: %v\n", closeErr)
		code = 1
	}
	if cleanErr := ledger.assertClean(); cleanErr != nil {
		_, _ = fmt.Fprintf(os.Stderr, "submit functional fixture leak: %v\n", cleanErr)
		code = 1
	}
	_, _ = fmt.Fprintf(os.Stderr, "submit functional lifecycle ledger: %s\n", ledger.summary())
	os.Exit(code)
}

type submitFixture struct {
	process support.ApplicationProcess
	runner  *submitProviderCommandRunner
	starter *submitAPIServerStarter
	ledger  *submitLifecycleLedger
}

type submitLifecycleLedger struct {
	mu sync.Mutex

	processStarts int
	processCloses int
	activeCalls   int
	activeOutputs int
	roots         map[string]struct{}

	activeHTTPServers int
	httpServerStarts  int
	httpServerCloses  int
	sessionStarts     int
	sessionCloses     int
}

func newSubmitLifecycleLedger() *submitLifecycleLedger {
	return &submitLifecycleLedger{roots: make(map[string]struct{})}
}

func (l *submitLifecycleLedger) processStarted() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.processStarts++
}

func (l *submitLifecycleLedger) processClosed() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.processCloses++
}

func (l *submitLifecycleLedger) invocationStarted() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.activeCalls++
	l.activeOutputs++
}

func (l *submitLifecycleLedger) invocationFinished() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.activeCalls--
	l.activeOutputs--
}

func (l *submitLifecycleLedger) rootCreated(path string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.roots[path] = struct{}{}
}

func (l *submitLifecycleLedger) rootRemoved(path string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.roots, path)
}

func (l *submitLifecycleLedger) httpServerStarted() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.activeHTTPServers++
	l.httpServerStarts++
}

func (l *submitLifecycleLedger) httpServerClosed() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.activeHTTPServers--
	l.httpServerCloses++
}

func (l *submitLifecycleLedger) sessionStarted() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.sessionStarts++
}

func (l *submitLifecycleLedger) sessionClosed() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.sessionCloses++
}

func (l *submitLifecycleLedger) assertClean() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.processStarts != 1 || l.processCloses != 1 {
		return fmt.Errorf("process lifecycle = %d starts/%d closes, want 1/1", l.processStarts, l.processCloses)
	}
	if l.activeCalls != 0 || l.activeOutputs != 0 {
		return fmt.Errorf("active invocation resources = %d calls/%d outputs, want 0/0", l.activeCalls, l.activeOutputs)
	}
	if l.activeHTTPServers != 0 {
		return fmt.Errorf("active controlled HTTP servers = %d, want 0", l.activeHTTPServers)
	}
	if l.httpServerStarts != l.httpServerCloses {
		return fmt.Errorf("HTTP server lifecycle = %d starts/%d closes, want balanced", l.httpServerStarts, l.httpServerCloses)
	}
	if l.sessionStarts != l.sessionCloses {
		return fmt.Errorf("Factory Session lifecycle = %d opens/%d closes, want balanced", l.sessionStarts, l.sessionCloses)
	}
	if len(l.roots) != 0 {
		return fmt.Errorf("unremoved invocation roots = %v", mapsKeys(l.roots))
	}
	return nil
}

func (l *submitLifecycleLedger) summary() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return fmt.Sprintf(
		"process=%d/%d invocations=%d outputs=%d http=%d/%d sessions=%d/%d roots=%d",
		l.processStarts, l.processCloses, l.activeCalls, l.activeOutputs,
		l.httpServerStarts, l.httpServerCloses, l.sessionStarts, l.sessionCloses, len(l.roots),
	)
}

func mapsKeys(values map[string]struct{}) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	return keys
}

func (f *submitFixture) tempDir(t testing.TB) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "functional-submit-")
	if err != nil {
		t.Fatalf("create submit fixture root: %v", err)
	}
	f.ledger.rootCreated(dir)
	t.Cleanup(func() {
		if err := os.RemoveAll(dir); err != nil {
			t.Errorf("remove submit fixture root %s: %v", dir, err)
			return
		}
		if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
			if err == nil {
				t.Errorf("submit fixture root %s still exists after cleanup", dir)
			} else {
				t.Errorf("stat removed submit fixture root %s: %v", dir, err)
			}
			return
		}
		f.ledger.rootRemoved(dir)
	})
	return dir
}

type submitInvocation struct {
	input   root.Input
	stdout  *bytes.Buffer
	stderr  *bytes.Buffer
	finish  sync.Once
	fixture *submitFixture
}

func (f *submitFixture) newInvocation(
	t testing.TB,
	args []string,
	ctx context.Context,
	stdin string,
	stdinIsTTY bool,
	workingDirectory string,
	stdout io.Writer,
) *submitInvocation {
	t.Helper()
	if ctx == nil {
		ctx = context.Background()
	}
	if workingDirectory == "" {
		workingDirectory = f.tempDir(t)
	}
	home := f.tempDir(t)
	cache := f.tempDir(t)
	if stdout == nil {
		stdout = &bytes.Buffer{}
	}
	stdoutBuffer, _ := stdout.(*bytes.Buffer)
	stderr := &bytes.Buffer{}
	stdinTTY := stdinIsTTY
	invocation := &submitInvocation{fixture: f, stdout: stdoutBuffer, stderr: stderr}
	invocation.input = root.Input{
		Args:             append([]string(nil), args...),
		Env:              submitEnvironment(home, cache),
		Stdin:            strings.NewReader(stdin),
		Stdout:           stdout,
		Stderr:           stderr,
		Context:          ctx,
		WorkingDirectory: workingDirectory,
		StdinIsTTY:       &stdinTTY,
	}
	f.ledger.invocationStarted()
	t.Cleanup(invocation.finishInvocation)
	return invocation
}

func (invocation *submitInvocation) finishInvocation() {
	invocation.finish.Do(invocation.fixture.ledger.invocationFinished)
}

func (f *submitFixture) execute(
	t testing.TB,
	args []string,
	ctx context.Context,
	stdin string,
	stdinIsTTY bool,
) submitCommandResult {
	t.Helper()
	invocation := f.newInvocation(t, args, ctx, stdin, stdinIsTTY, "", nil)
	err := f.process.Execute(invocation.input)
	invocation.finishInvocation()
	return submitCommandResult{stdout: invocation.stdoutString(), stderr: invocation.stderr.String(), err: err}
}

func (f *submitFixture) executeWithWriter(
	t testing.TB,
	args []string,
	ctx context.Context,
	stdin string,
	stdinIsTTY bool,
	stdout io.Writer,
) submitCommandResult {
	t.Helper()
	invocation := f.newInvocation(t, args, ctx, stdin, stdinIsTTY, "", stdout)
	err := f.process.Execute(invocation.input)
	invocation.finishInvocation()
	return submitCommandResult{stdout: invocation.stdoutString(), stderr: invocation.stderr.String(), err: err}
}

func (invocation *submitInvocation) stdoutString() string {
	if invocation.stdout == nil {
		return ""
	}
	return invocation.stdout.String()
}

type submitCommandResult struct {
	stdout string
	stderr string
	err    error
}

type submitAsyncCommand struct {
	invocation *submitInvocation
	cancel     context.CancelFunc
	done       chan struct{}
	err        error
}

func (f *submitFixture) startInvocation(t testing.TB, invocation *submitInvocation) *submitAsyncCommand {
	t.Helper()
	ctx, cancel := context.WithCancel(invocation.input.Context)
	invocation.input.Context = ctx
	command := &submitAsyncCommand{invocation: invocation, cancel: cancel, done: make(chan struct{})}
	go func() {
		command.err = f.process.Execute(invocation.input)
		invocation.finishInvocation()
		close(command.done)
	}()
	t.Cleanup(func() {
		command.cancel()
		<-command.done
	})
	return command
}

func (command *submitAsyncCommand) result(t testing.TB) submitCommandResult {
	t.Helper()
	<-command.done
	return submitCommandResult{
		stdout: command.invocation.stdoutString(),
		stderr: command.invocation.stderr.String(),
		err:    command.err,
	}
}

func submitEnvironment(home, cache string) []string {
	const (
		homeKey       = "HOME"
		profileKey    = "USERPROFILE"
		driveKey      = "HOMEDRIVE"
		pathKey       = "HOMEPATH"
		plan9HomeKey  = "home"
		cacheKey      = "XDG_CACHE_HOME"
		voiceCacheKey = "INFINITE_YOU_OMNIVOICE_CACHE_DIR"
	)
	blocked := map[string]struct{}{
		homeKey: {}, profileKey: {}, driveKey: {}, pathKey: {}, plan9HomeKey: {}, cacheKey: {}, voiceCacheKey: {},
	}
	environment := make([]string, 0, len(os.Environ())+4)
	for _, entry := range os.Environ() {
		name, _, ok := strings.Cut(entry, "=")
		if ok && submitEnvironmentNameBlocked(name, blocked) {
			continue
		}
		environment = append(environment, entry)
	}
	environment = append(environment, homeKey+"="+home, profileKey+"="+home, cacheKey+"="+cache, voiceCacheKey+"="+cache)
	if runtime.GOOS == "plan9" {
		environment = append(environment, plan9HomeKey+"="+home)
	}
	return environment
}

func submitEnvironmentNameBlocked(name string, blocked map[string]struct{}) bool {
	for blockedName := range blocked {
		if strings.EqualFold(name, blockedName) {
			return true
		}
	}
	return false
}

type submitProviderCommandRunner struct {
	active atomic.Int32

	mu           sync.Mutex
	calls        int
	nextCallDone chan struct{}
}

func (runner *submitProviderCommandRunner) expectNextCall(t testing.TB) <-chan struct{} {
	t.Helper()
	runner.mu.Lock()
	defer runner.mu.Unlock()
	if runner.nextCallDone != nil {
		t.Fatal("controlled provider call completion is already being observed")
	}
	runner.nextCallDone = make(chan struct{})
	return runner.nextCallDone
}

func (runner *submitProviderCommandRunner) Run(ctx context.Context, _ platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	runner.active.Add(1)
	defer runner.active.Add(-1)
	runner.mu.Lock()
	runner.calls++
	callDone := runner.nextCallDone
	runner.nextCallDone = nil
	runner.mu.Unlock()
	if callDone != nil {
		defer close(callDone)
	}
	select {
	case <-ctx.Done():
		return platformprocess.CommandResult{}, ctx.Err()
	default:
	}
	return platformprocess.CommandResult{Stdout: support.CodexSuccessStdout("not-json")}, nil
}

func (runner *submitProviderCommandRunner) callCount() int {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return runner.calls
}

func (runner *submitProviderCommandRunner) activeCount() int {
	return int(runner.active.Load())
}

type submitAPIServerStarter struct {
	ready  chan *support.ProcessAPIServer
	ledger *submitLifecycleLedger
	starts atomic.Int32
	closes atomic.Int32
}

func (starter *submitAPIServerStarter) Start(ctx context.Context, request platformhttpserver.StartRequest) error {
	api := support.NewProcessAPIServer()
	select {
	case starter.ready <- api:
	case <-ctx.Done():
		return ctx.Err()
	}
	starter.starts.Add(1)
	starter.ledger.httpServerStarted()
	err := api.Start(ctx, request)
	starter.closes.Add(1)
	starter.ledger.httpServerClosed()
	return err
}

func (starter *submitAPIServerStarter) next(t testing.TB) *support.ProcessAPIServer {
	t.Helper()
	return <-starter.ready
}

type submitHTTPServer struct {
	server *httptest.Server
	ledger *submitLifecycleLedger
	active atomic.Int32

	mu       sync.Mutex
	requests []submitHTTPRequest
}

type submitHTTPRequest struct {
	Method string
	Path   string
	Body   []byte
}

func newSubmitHTTPServer(t testing.TB, ledger *submitLifecycleLedger, handler http.HandlerFunc) *submitHTTPServer {
	t.Helper()
	controlled := &submitHTTPServer{ledger: ledger}
	controlled.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		controlled.active.Add(1)
		defer controlled.active.Add(-1)
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "read request", http.StatusBadRequest)
			return
		}
		_ = r.Body.Close()
		controlled.mu.Lock()
		controlled.requests = append(controlled.requests, submitHTTPRequest{
			Method: r.Method,
			Path:   r.URL.Path,
			Body:   append([]byte(nil), body...),
		})
		controlled.mu.Unlock()
		r.Body = io.NopCloser(bytes.NewReader(body))
		handler(w, r)
	}))
	ledger.httpServerStarted()
	t.Cleanup(func() {
		controlled.server.Close()
		if active := controlled.active.Load(); active != 0 {
			t.Errorf("controlled HTTP server active handlers after close = %d", active)
		}
		ledger.httpServerClosed()
	})
	return controlled
}

func (server *submitHTTPServer) URL() string {
	return server.server.URL
}

func (server *submitHTTPServer) requestsSnapshot() []submitHTTPRequest {
	server.mu.Lock()
	defer server.mu.Unlock()
	requests := make([]submitHTTPRequest, len(server.requests))
	copy(requests, server.requests)
	return requests
}

func submitJSONResponse(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, body)
}

func submitAcceptedResponse(requestID, traceID, workID, name, workType string) string {
	return fmt.Sprintf(`{"accepted":true,"requestId":%q,"traceId":%q,"workId":%q,"name":%q,"workTypeName":%q}`,
		requestID, traceID, workID, name, workType)
}

func submitBatchAcceptedResponse(requestID, traceID string, works ...string) string {
	var builder strings.Builder
	_, _ = fmt.Fprintf(&builder, `{"accepted":true,"requestId":%q,"traceId":%q,"works":[`, requestID, traceID)
	for index, name := range works {
		if index > 0 {
			builder.WriteByte(',')
		}
		_, _ = fmt.Fprintf(&builder, `{"name":%q,"workTypeName":"task","workId":%q}`, name, requestID+"-"+name)
	}
	builder.WriteString("]}")
	return builder.String()
}

func oneWorkBatch(requestID, name, payload string) string {
	encodedPayload, _ := json.Marshal(payload)
	return fmt.Sprintf(`{"requestId":%q,"type":"FACTORY_REQUEST_BATCH","works":[{"name":%q,"workTypeName":"task","payload":%s}]}`,
		requestID, name, encodedPayload)
}

func writeSubmitFactory(t *testing.T, dir string) {
	t.Helper()
	config := map[string]any{
		"name": "submit-functional",
		"workTypes": []any{map[string]any{
			"name": "task",
			"states": []any{
				map[string]string{"name": "init", "type": "INITIAL"},
				map[string]string{"name": "done", "type": "TERMINAL"},
				map[string]string{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []any{map[string]string{"name": "worker"}},
		"workstations": []any{map[string]any{
			"name":      "process",
			"worker":    "worker",
			"inputs":    []any{map[string]string{"workType": "task", "state": "init"}},
			"outputs":   []any{map[string]string{"workType": "task", "state": "done"}},
			"onFailure": []any{map[string]string{"workType": "task", "state": "failed"}},
		}},
	}
	encoded, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		t.Fatalf("marshal submit factory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "factory.json"), encoded, 0o600); err != nil {
		t.Fatalf("write submit factory: %v", err)
	}
	support.WriteAgentConfig(t, dir, "worker", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "test-model"))
	support.WriteWorkstationConfig(t, dir, "process", `---
type: MODEL_WORKSTATION
outputSchema: '{}'
---
Return structured JSON.
`)
}
