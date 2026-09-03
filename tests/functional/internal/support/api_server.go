package support

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factorysessionshttp "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/http"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// Functional servers may be started concurrently by separate Go test
// packages. A larger ceiling prevents scheduler and antivirus variance on
// Windows from becoming a test failure without slowing the success path.
const functionalServerReadyTimeout = 15 * time.Second

const workerSessionReplayPollInterval = 10 * time.Millisecond

// FunctionalAPIServerConfig describes customer process inputs and replaceable
// external boundaries. Product/runtime configuration is supplied through Args
// exactly as it is for a real CLI invocation.
type FunctionalAPIServerConfig struct {
	FactoryDir                   string
	FactoryConfigPath            string
	WorkingDirectory             string
	UseMockWorkers               bool
	MockWorkersConfig            *workers.MockWorkersConfig
	WaitForServiceModeRuntime    bool
	ResponseEventRetentionLimits *factorysessions.ResponseEventRetentionLimits
	// ServerReadyTimeout overrides the bounded startup wait for scenarios whose
	// process initialization includes a large first-time packaged catalog.
	ServerReadyTimeout time.Duration
	Args               []string
	Env                []string
	ProviderOverride   providers.Service
	Edges              serviceedges.Edges
	// BeforeStart prepares scenario-owned durable state through the same
	// root-built process that will host the server. The callback runs after
	// invocation-local environment setup and before the server command starts.
	BeforeStart func(testing.TB, Process, root.Input)
}

// FunctionalAPIServer owns one daemon invocation on a reusable root Process.
type FunctionalAPIServer struct {
	process         *ProcessCommand
	processRoot     Process
	api             *ProcessAPIServer
	url             string
	closeProcess    func(context.Context) error
	closeOnce       sync.Once
	closeErr        error
	recordingReader recordings.WorkerRecordingReader
}

// ConfigureWorkerCommands installs typed functional command edges before the
// root process is constructed.
func ConfigureWorkerCommands(
	t *testing.T,
	edges *serviceedges.Edges,
	providerRunner, scriptRunner platformprocess.CommandRunner,
) {
	t.Helper()
	edges.ProviderCommandRunner = providerRunner
	edges.ScriptCommandRunner = scriptRunner
}

func StartFunctionalAPIServer(t *testing.T, cfg FunctionalAPIServerConfig) *FunctionalAPIServer {
	t.Helper()

	edges := cfg.Edges
	if cfg.ProviderOverride != nil {
		edges.ProviderOverride = cfg.ProviderOverride
	}
	if cfg.ResponseEventRetentionLimits != nil {
		edges.FactorySessionResponseEventRetentionLimits = cfg.ResponseEventRetentionLimits
	}

	api := NewProcessAPIServer()
	edges.APIServerStarter = api.Start
	process := BuildProcess(t, edges)
	recordingReader := process.WorkerRecordingReader()
	var closeProcess func(context.Context) error
	if closer, ok := process.(interface{ Close(context.Context) error }); ok {
		closeProcess = closer.Close
	}

	args := append([]string{"you", "run"}, functionalRunArgs(t, cfg)...)
	inputs := FakeInputs(context.Background(), args)
	// Match a customer invoking `you run` from the selected Factory directory.
	// This keeps invocation-relative durable state and packaged source
	// resolution aligned with the public CLI contract without changing the
	// process-wide working directory.
	inputs.WorkingDirectory = cfg.WorkingDirectory
	if inputs.WorkingDirectory == "" {
		inputs.WorkingDirectory = cfg.FactoryDir
	}
	if cfg.Env == nil {
		home := t.TempDir()
		inputs.Env = withFunctionalEnvironment(inputs.Env, "HOME", home)
		inputs.Env = withFunctionalEnvironment(inputs.Env, "USERPROFILE", home)
	} else {
		inputs.Env = append([]string(nil), cfg.Env...)
	}
	server := &FunctionalAPIServer{
		api:             api,
		processRoot:     process,
		closeProcess:    closeProcess,
		recordingReader: recordingReader,
	}
	if closeProcess != nil {
		t.Cleanup(func() { server.Close(t) })
	}
	if cfg.BeforeStart != nil {
		cfg.BeforeStart(t, process, inputs.Input)
	}
	t.Cleanup(func() {
		if !t.Failed() {
			return
		}
		if stderr := strings.TrimSpace(inputs.Stderr()); stderr != "" {
			t.Logf("daemon stderr:\n%s", stderr)
		}
		if stdout := strings.TrimSpace(inputs.Stdout()); stdout != "" {
			t.Logf("daemon stdout:\n%s", stdout)
		}
	})
	command := StartProcessCommand(t, process, inputs.Input)
	server.process = command
	readyTimeout := functionalServerReadyTimeout
	if cfg.ServerReadyTimeout > 0 {
		readyTimeout = cfg.ServerReadyTimeout
	}
	baseURL, err := api.WaitForBaseURL(readyTimeout)
	if err != nil {
		t.Fatal(err)
	}
	server.url = baseURL
	if cfg.WaitForServiceModeRuntime {
		statusURL := strings.TrimSuffix(server.url, "/") + "/status"
		if sessionID := functionalArgumentValue(cfg.Args, "--session"); sessionID != "" {
			statusURL = strings.TrimSuffix(server.url, "/") + "/factory-sessions/" + url.PathEscape(sessionID) + "/status"
		}
		_, err := waitForStatusAt(statusURL, functionalServerReadyTimeout, func(status factoryapi.StatusResponse) bool {
			return status.RuntimeStatus != ""
		})
		if err != nil {
			t.Fatalf("timed out waiting for service-mode runtime at %s: %v", statusURL, err)
		}
	}
	return server
}

// Close releases the root process resources after Stop has canceled its
// customer invocation. Tests that need to reuse durable project state can
// call it before opening another process against the same directory.
func (fs *FunctionalAPIServer) Close(t testing.TB) {
	t.Helper()
	if fs == nil || fs.closeProcess == nil {
		return
	}
	fs.closeOnce.Do(func() {
		closeCtx, cancelClose := context.WithTimeout(context.Background(), processCommandStopTimeout)
		defer cancelClose()
		fs.closeErr = fs.closeProcess(closeCtx)
	})
	if fs.closeErr != nil {
		t.Errorf("close application process: %v", fs.closeErr)
	}
}

func withFunctionalEnvironment(environment []string, name, value string) []string {
	prefix := name + "="
	out := make([]string, 0, len(environment)+1)
	for _, entry := range environment {
		if strings.EqualFold(strings.SplitN(entry, "=", 2)[0]+"=", prefix) {
			continue
		}
		out = append(out, entry)
	}
	return append(out, prefix+value)
}

func functionalRunArgs(t *testing.T, cfg FunctionalAPIServerConfig) []string {
	t.Helper()
	args := []string{
		"--continuously",
		"--with-server",
		"--quiet",
	}
	if strings.TrimSpace(cfg.FactoryConfigPath) != "" {
		args = append(args, "--factory", cfg.FactoryConfigPath)
	} else {
		args = append(args, "--dir", cfg.FactoryDir)
	}
	if !containsFunctionalArgument(cfg.Args, "--record") {
		args = append(args, "--no-record")
	}
	mockWorkersConfig := cfg.MockWorkersConfig
	if cfg.UseMockWorkers && mockWorkersConfig == nil {
		mockWorkersConfig = workers.NewEmptyMockWorkersConfig()
	}
	if mockWorkersConfig != nil {
		path := filepath.Join(t.TempDir(), "mock-workers.json")
		payload, err := json.Marshal(mockWorkersConfig)
		if err != nil {
			t.Fatalf("marshal mock workers config: %v", err)
		}
		if err := os.WriteFile(path, payload, 0o600); err != nil {
			t.Fatalf("write mock workers config: %v", err)
		}
		args = append(args, "--with-mock-workers", path)
	}
	return append(args, cfg.Args...)
}

func containsFunctionalArgument(args []string, name string) bool {
	for _, arg := range args {
		if arg == name || strings.HasPrefix(arg, name+"=") {
			return true
		}
	}
	return false
}

func functionalArgumentValue(args []string, name string) string {
	for index, arg := range args {
		if arg == name && index+1 < len(args) {
			return strings.TrimSpace(args[index+1])
		}
		if strings.HasPrefix(arg, name+"=") {
			return strings.TrimSpace(strings.TrimPrefix(arg, name+"="))
		}
	}
	return ""
}

func (fs *FunctionalAPIServer) URL() string {
	if fs == nil {
		return ""
	}
	return fs.url
}

// WorkerRecordingReader exposes the detached recording read capability of the
// same root-built process that hosts this functional server.
func (fs *FunctionalAPIServer) WorkerRecordingReader() recordings.WorkerRecordingReader {
	if fs == nil {
		return nil
	}
	return fs.recordingReader
}

func (fs *FunctionalAPIServer) Done() <-chan struct{} {
	if fs == nil || fs.process == nil {
		closed := make(chan struct{})
		close(closed)
		return closed
	}
	return fs.process.Done()
}

func (fs *FunctionalAPIServer) Stop(t *testing.T) {
	t.Helper()
	if fs != nil && fs.process != nil {
		fs.process.Stop(t)
	}
}

// Execute runs a public CLI operation through the same root-built process
// that owns the live server. This keeps ordinary customer flows on the CLI
// boundary without constructing a second application graph or reaching for
// the server's HTTP implementation from a domain-owned functional test.
func (fs *FunctionalAPIServer) Execute(t testing.TB, input root.Input) error {
	t.Helper()
	if fs == nil || fs.processRoot == nil {
		return fmt.Errorf("functional server process is unavailable")
	}
	return fs.processRoot.Execute(input)
}

// GetFactoryEvents reads the canonical public session event stream's committed
// retained history through its public retained-count boundary.
func (fs *FunctionalAPIServer) GetFactoryEvents(t *testing.T) []factoryapi.FactoryEvent {
	t.Helper()
	return GetFactoryEventsAt(t, fs.URL())
}

// GetFactoryEventsAfter reads retained Factory Event history after an
// acknowledged reconnect cursor through the public session events endpoint.
func (fs *FunctionalAPIServer) GetFactoryEventsAfter(
	t *testing.T,
	cursor FactoryEventReadCursor,
) []factoryapi.FactoryEvent {
	t.Helper()
	return GetFactoryEventsAfterAt(t, fs.URL(), cursor)
}

// FactoryEventReadCursor identifies an acknowledged Factory Event reconnect
// point for retained-history reads through the public events stream.
type FactoryEventReadCursor struct {
	AfterEventID  string
	AfterSequence *int
}

// ReconnectSequenceForFactoryEvent returns the ordering point clients should
// acknowledge with after_sequence for session-scoped Factory Event streams.
func ReconnectSequenceForFactoryEvent(event factoryapi.FactoryEvent) int {
	if event.Context.SessionSequence != nil {
		return *event.Context.SessionSequence
	}
	return event.Context.Sequence
}

func factoryEventsURLWithCursor(baseURL string, cursor FactoryEventReadCursor) string {
	return SessionEventsURLWithCursor(baseURL, factorysessions.DefaultSessionID, cursor)
}

// GetFactoryEventsAfterAt reads retained Factory Event history after an
// acknowledged reconnect cursor through the public session events endpoint.
func GetFactoryEventsAfterAt(
	t testing.TB,
	baseURL string,
	cursor FactoryEventReadCursor,
) []factoryapi.FactoryEvent {
	t.Helper()
	return readFactoryEventsFromURL(t, factoryEventsURLWithCursor(baseURL, cursor))
}

// GetFactoryEventsAfterForSessionAt reads retained Factory Event history after
// an acknowledged reconnect cursor for one explicitly selected Factory Session.
func GetFactoryEventsAfterForSessionAt(
	t testing.TB,
	baseURL, sessionID string,
	cursor FactoryEventReadCursor,
) []factoryapi.FactoryEvent {
	t.Helper()
	return readFactoryEventsFromURL(t, SessionEventsURLWithCursor(baseURL, sessionID, cursor))
}

// FactoryEventsInvalidCursorError is the typed 400 payload for an invalid
// Factory Event reconnect cursor together with the raw response body.
type FactoryEventsInvalidCursorError struct {
	Response factoryapi.ErrorResponse
	Body     string
}

// GetFactoryEventsInvalidCursorErrorAt requests retained Factory Event history
// with an invalid reconnect cursor and returns the typed 400 error payload.
func GetFactoryEventsInvalidCursorErrorAt(
	t testing.TB,
	baseURL string,
	cursor FactoryEventReadCursor,
) FactoryEventsInvalidCursorError {
	t.Helper()
	return readFactoryEventsInvalidCursorErrorFromURL(t, factoryEventsURLWithCursor(baseURL, cursor))
}

// ProbeFactoryEventStreamRecoveryAt issues the JSON reconnect probe for an
// invalid or valid Factory Event cursor through the public session events
// endpoint.
func ProbeFactoryEventStreamRecoveryAt(
	t testing.TB,
	baseURL string,
	cursor FactoryEventReadCursor,
) factoryapi.FactorySessionEventStreamRecovery {
	t.Helper()
	return readFactoryEventStreamRecoveryFromURL(t, factoryEventsURLWithCursor(baseURL, cursor))
}

// GetFactoryEventsAt reads retained Factory Event history from a public
// session endpoint without requiring the FunctionalAPIServer wrapper.
func GetFactoryEventsAt(t testing.TB, baseURL string) []factoryapi.FactoryEvent {
	t.Helper()
	return GetFactoryEventsForSessionAt(t, baseURL, factorysessions.DefaultSessionID)
}

// GetFactoryEventsForSessionAt reads the committed retained Factory Event
// history for one explicitly selected session. The public stream's retained
// count header makes this a bounded snapshot read; it never waits for stream
// quietness.
func GetFactoryEventsForSessionAt(t testing.TB, baseURL, sessionID string) []factoryapi.FactoryEvent {
	t.Helper()
	return readFactoryEventsFromURL(t, SessionEventsURL(baseURL, sessionID))
}

// GetWorkerSessionEventsByIDAt drains the public provider-neutral Worker
// Session stream through its retained replay summary. The Worker Session ID
// comes from the public list projection, so this path remains usable when no
// Provider Session reference was emitted.
func GetWorkerSessionEventsByIDAt(t testing.TB, baseURL, workerSessionID string) []factoryapi.WorkerSessionEvent {
	return GetWorkerSessionEventsForSessionByIDAt(
		t, baseURL, factorysessions.DefaultSessionID, workerSessionID,
	)
}

// GetWorkerSessionEventsForSessionByIDAt drains one explicitly selected
// Factory Session's provider-neutral Worker Session stream.
func GetWorkerSessionEventsForSessionByIDAt(
	t testing.TB,
	baseURL, sessionID, workerSessionID string,
) []factoryapi.WorkerSessionEvent {
	t.Helper()
	if strings.TrimSpace(workerSessionID) == "" {
		t.Fatal("worker session id is empty")
	}
	ctx, cancel := context.WithTimeout(context.Background(), functionalServerReadyTimeout)
	defer cancel()
	endpoint := strings.TrimSuffix(baseURL, "/") +
		"/factory-sessions/" + url.PathEscape(sessionID) +
		"/worker-sessions/" + url.PathEscape(workerSessionID) + "/events?replayOnly=true"
	events, err := waitForCompleteWorkerSessionReplay(ctx, endpoint)
	if err != nil {
		t.Fatalf("GET Worker Session events: %v", err)
	}
	return events
}

func waitForCompleteWorkerSessionReplay(
	ctx context.Context,
	endpoint string,
) ([]factoryapi.WorkerSessionEvent, error) {
	ticker := time.NewTicker(workerSessionReplayPollInterval)
	defer ticker.Stop()
	var lastSummary *factoryapi.WorkerSessionReplaySummary
	for {
		events, summary, err := readWorkerSessionReplay(ctx, endpoint)
		if err != nil {
			return nil, err
		}
		lastSummary = summary
		if summary != nil && summary.Complete {
			return events, nil
		}

		select {
		case <-ctx.Done():
			return nil, fmt.Errorf(
				"waiting for complete replay at %s; last summary=%#v: %w",
				endpoint,
				lastSummary,
				ctx.Err(),
			)
		case <-ticker.C:
		}
	}
}

func readWorkerSessionReplay(
	ctx context.Context,
	endpoint string,
) ([]factoryapi.WorkerSessionEvent, *factoryapi.WorkerSessionReplaySummary, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("build Worker Session events request: %w", err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, nil, fmt.Errorf("GET Worker Session events: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		return nil, nil, fmt.Errorf(
			"GET Worker Session events status = %d url = %s body = %s",
			response.StatusCode,
			endpoint,
			strings.TrimSpace(string(body)),
		)
	}

	var events []factoryapi.WorkerSessionEvent
	scanner := bufio.NewScanner(response.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		var event factoryapi.WorkerSessionEvent
		if err := json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data:"))), &event); err != nil {
			return nil, nil, fmt.Errorf("decode Worker Session event: %w", err)
		}
		events = append(events, event)
		if string(event.Delivery) == "REPLAY_SUMMARY" || event.ReplaySummary != nil {
			if event.ReplaySummary == nil {
				return nil, nil, fmt.Errorf("Worker Session replay summary is empty: %s", endpoint)
			}
			return events, event.ReplaySummary, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, nil, fmt.Errorf("read Worker Session events: %w", err)
	}
	return nil, nil, fmt.Errorf("Worker Session event stream ended without replay summary: %s", endpoint)
}

func readFactoryEventsInvalidCursorErrorFromURL(t testing.TB, endpoint string) FactoryEventsInvalidCursorError {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), functionalServerReadyTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		t.Fatalf("build factory events invalid cursor request: %v", err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("GET factory events invalid cursor: %v", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read factory events invalid cursor response: %v", err)
	}
	bodyText := strings.TrimSpace(string(body))
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf(
			"GET factory events invalid cursor status = %d url = %q body = %s, want 400",
			response.StatusCode,
			endpoint,
			bodyText,
		)
	}
	if strings.Contains(response.Header.Get("Content-Type"), "text/event-stream") {
		t.Fatalf("invalid cursor Content-Type = %q, want typed error response instead of SSE stream", response.Header.Get("Content-Type"))
	}
	var errResp factoryapi.ErrorResponse
	if err := json.Unmarshal(body, &errResp); err != nil {
		t.Fatalf("decode factory events invalid cursor error: %v: %s", err, bodyText)
	}
	return FactoryEventsInvalidCursorError{
		Response: errResp,
		Body:     bodyText,
	}
}

func readFactoryEventStreamRecoveryFromURL(t testing.TB, endpoint string) factoryapi.FactorySessionEventStreamRecovery {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), functionalServerReadyTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		t.Fatalf("build factory events recovery probe request: %v", err)
	}
	request.Header.Set("Accept", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("GET factory events recovery probe: %v", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read factory events recovery probe response: %v", err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf(
			"GET factory events recovery probe status = %d url = %q body = %s, want 200",
			response.StatusCode,
			endpoint,
			strings.TrimSpace(string(body)),
		)
	}
	var recovery factoryapi.FactorySessionEventStreamRecovery
	if err := json.Unmarshal(body, &recovery); err != nil {
		t.Fatalf("decode factory events recovery probe: %v: %s", err, strings.TrimSpace(string(body)))
	}
	return recovery
}

func readFactoryEventsFromURL(t testing.TB, endpoint string) []factoryapi.FactoryEvent {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		t.Fatalf("build factory events request: %v", err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("GET factory events: %v", err)
	}
	if response.StatusCode != http.StatusOK {
		defer response.Body.Close()
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("GET factory events status = %d url = %q body = %s", response.StatusCode, endpoint, strings.TrimSpace(string(body)))
	}

	retainedHeader := strings.TrimSpace(response.Header.Get(factorysessionshttp.SessionEventStreamRetainedCountHeader))
	retainedCount, err := strconv.Atoi(retainedHeader)
	if err != nil {
		defer response.Body.Close()
		t.Fatalf(
			"GET factory events url = %q: missing or invalid %s header (%q): %v",
			endpoint, factorysessionshttp.SessionEventStreamRetainedCountHeader, retainedHeader, err,
		)
	}

	events := make(chan factoryapi.FactoryEvent, 256)
	errs := make(chan error, 1)
	go func() {
		defer response.Body.Close()
		scanner := bufio.NewScanner(response.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data:") {
				continue
			}
			var event factoryapi.FactoryEvent
			if err := json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data:"))), &event); err != nil {
				errs <- fmt.Errorf("decode factory event: %w", err)
				return
			}
			events <- event
		}
		if err := scanner.Err(); err != nil && !errors.Is(err, context.Canceled) {
			errs <- err
		}
	}()

	collected := make([]factoryapi.FactoryEvent, 0, retainedCount)
	deadline := time.NewTimer(functionalServerReadyTimeout)
	defer deadline.Stop()
	for len(collected) < retainedCount {
		select {
		case event := <-events:
			collected = append(collected, event)
		case err := <-errs:
			t.Fatalf("read factory events: %v", err)
		case <-deadline.C:
			t.Fatalf(
				"timed out reading factory event history: got %d of %d retained events",
				len(collected), retainedCount,
			)
		}
	}
	return collected
}

// GetFactoryResponseEventsAt reads retained public Factory response events
// until the active stream becomes quiet.
func GetFactoryResponseEventsAt(
	t testing.TB,
	baseURL string,
	sessionID string,
) []factoryapi.FactoryResponseEvent {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), functionalServerReadyTimeout)
	defer cancel()
	endpoint := strings.TrimSuffix(baseURL, "/") +
		"/factory-sessions/" + sessionID + "/response-events"
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		t.Fatalf("build factory response events request: %v", err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("GET factory response events: %v", err)
	}
	if response.StatusCode != http.StatusOK {
		defer response.Body.Close()
		t.Fatalf("GET factory response events status = %d", response.StatusCode)
	}

	events := make(chan factoryapi.FactoryResponseEvent, 32)
	errs := make(chan error, 1)
	go func() {
		defer response.Body.Close()
		scanner := bufio.NewScanner(response.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data:") {
				continue
			}
			var event factoryapi.FactoryResponseEvent
			if err := json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data:"))), &event); err != nil {
				errs <- fmt.Errorf("decode factory response event: %w", err)
				return
			}
			events <- event
		}
		if err := scanner.Err(); err != nil && !errors.Is(err, context.Canceled) {
			errs <- err
		}
	}()

	var collected []factoryapi.FactoryResponseEvent
	deadline := time.NewTimer(functionalServerReadyTimeout)
	defer deadline.Stop()
	quiet := time.NewTimer(25 * time.Millisecond)
	defer quiet.Stop()
	quietC := quiet.C
	for {
		select {
		case event := <-events:
			collected = append(collected, event)
			if !quiet.Stop() {
				select {
				case <-quiet.C:
				default:
				}
			}
			quiet.Reset(25 * time.Millisecond)
			quietC = quiet.C
		case err := <-errs:
			t.Fatalf("read factory response events: %v", err)
		case <-quietC:
			return collected
		case <-deadline.C:
			t.Fatalf("timed out reading factory response-event history")
		}
	}
}
