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
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

const functionalServerReadyTimeout = 5 * time.Second

// FunctionalAPIServerConfig describes customer process inputs and replaceable
// external boundaries. Product/runtime configuration is supplied through Args
// exactly as it is for a real CLI invocation.
type FunctionalAPIServerConfig struct {
	FactoryDir                string
	FactoryConfigPath         string
	WorkingDirectory          string
	UseMockWorkers            bool
	MockWorkersConfig         *workers.MockWorkersConfig
	WaitForServiceModeRuntime bool
	Args                      []string
	Env                       []string
	Edges                     serviceedges.Edges
}

// FunctionalAPIServer owns one daemon invocation on a reusable root Process.
type FunctionalAPIServer struct {
	process *ProcessCommand
	api     *ProcessAPIServer
	url     string
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

	api := NewProcessAPIServer()
	edges.APIServerStarter = api.Start
	process := BuildProcess(t, edges)

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
	server := &FunctionalAPIServer{process: command, api: api}
	server.url = api.WaitForURL(t)
	if cfg.WaitForServiceModeRuntime {
		WaitForStatus(t, server.url, functionalServerReadyTimeout, func(status factoryapi.StatusResponse) bool {
			return status.RuntimeStatus != ""
		})
	}
	return server
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

// StartFunctionalAPIServiceModeServer starts the standard customer-facing
// service process.
func StartFunctionalAPIServiceModeServer(t *testing.T, factoryDir string, useMockWorkers bool) *FunctionalAPIServer {
	t.Helper()
	return StartFunctionalAPIServer(t, FunctionalAPIServerConfig{
		FactoryDir:                factoryDir,
		UseMockWorkers:            useMockWorkers,
		WaitForServiceModeRuntime: true,
	})
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

func (fs *FunctionalAPIServer) URL() string {
	if fs == nil {
		return ""
	}
	return fs.url
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

// WaitForExitError observes an expected terminal process failure through the
// canonical root invocation without having cleanup report it a second time.
func (fs *FunctionalAPIServer) WaitForExitError(t testing.TB, timeout time.Duration) error {
	t.Helper()
	if fs == nil || fs.process == nil {
		t.Fatal("WaitForExitError requires a running process")
	}
	select {
	case <-fs.process.Done():
		err := fs.process.Err()
		fs.process.AcceptError()
		return err
	case <-time.After(timeout):
		t.Fatalf("timed out waiting for process failure")
		return nil
	}
}

// GetFactoryEvents reads the canonical public session event stream. The
// endpoint first replays retained history, so a short quiet period yields a
// stable observation without reaching into the runtime service graph.
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
	endpoint := DefaultSessionEventsURL(baseURL)
	params := url.Values{}
	if cursor.AfterEventID != "" {
		params.Set("after_event_id", cursor.AfterEventID)
	}
	if cursor.AfterSequence != nil {
		params.Set("after_sequence", strconv.Itoa(*cursor.AfterSequence))
	}
	if encoded := params.Encode(); encoded != "" {
		endpoint += "?" + encoded
	}
	return endpoint
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
	return readFactoryEventsFromURL(t, DefaultSessionEventsURL(baseURL))
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

	var collected []factoryapi.FactoryEvent
	deadline := time.NewTimer(functionalServerReadyTimeout)
	defer deadline.Stop()
	var quiet *time.Timer
	var quietC <-chan time.Time
	for {
		select {
		case event := <-events:
			collected = append(collected, event)
			if quiet == nil {
				quiet = time.NewTimer(25 * time.Millisecond)
			} else {
				if !quiet.Stop() {
					select {
					case <-quiet.C:
					default:
					}
				}
				quiet.Reset(25 * time.Millisecond)
			}
			quietC = quiet.C
		case err := <-errs:
			t.Fatalf("read factory events: %v", err)
		case <-quietC:
			return collected
		case <-deadline.C:
			t.Fatalf("timed out reading factory event history")
		}
	}
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
	var quiet *time.Timer
	var quietC <-chan time.Time
	for {
		select {
		case event := <-events:
			collected = append(collected, event)
			if quiet == nil {
				quiet = time.NewTimer(25 * time.Millisecond)
			} else {
				if !quiet.Stop() {
					select {
					case <-quiet.C:
					default:
					}
				}
				quiet.Reset(25 * time.Millisecond)
			}
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
