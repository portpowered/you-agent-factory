package root_composition_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	"github.com/portpowered/infinite-you/pkg/root"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

type rootCompositionProcessCommand struct {
	cancel context.CancelFunc
	done   chan struct{}

	mu            sync.Mutex
	err           error
	errorAccepted bool
}

func startRootCompositionProcessCommand(
	process support.ApplicationProcess,
	input root.Input,
) *rootCompositionProcessCommand {
	parent := input.Context
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	input.Context = ctx
	command := &rootCompositionProcessCommand{cancel: cancel, done: make(chan struct{})}
	go func() {
		err := process.Execute(input)
		command.mu.Lock()
		command.err = err
		command.mu.Unlock()
		close(command.done)
	}()
	return command
}

func (command *rootCompositionProcessCommand) stop() error {
	if command == nil {
		return nil
	}
	command.cancel()
	err := command.wait()
	if err != nil && !errors.Is(err, context.Canceled) && !command.errorWasAccepted() {
		return fmt.Errorf("shared Process.Execute returned during shutdown: %w", err)
	}
	return nil
}

func (command *rootCompositionProcessCommand) wait() error {
	if command == nil {
		return nil
	}
	select {
	case <-command.done:
		return command.Err()
	case <-time.After(5 * time.Second):
		return errors.New("timed out waiting for shared Process.Execute shutdown")
	}
}

func (command *rootCompositionProcessCommand) AcceptError() {
	if command == nil {
		return
	}
	command.mu.Lock()
	command.errorAccepted = true
	command.mu.Unlock()
}

func (command *rootCompositionProcessCommand) errorWasAccepted() bool {
	if command == nil {
		return false
	}
	command.mu.Lock()
	defer command.mu.Unlock()
	return command.errorAccepted
}

func (command *rootCompositionProcessCommand) Err() error {
	if command == nil {
		return nil
	}
	command.mu.Lock()
	defer command.mu.Unlock()
	return command.err
}

type rootCompositionServer struct {
	api          *support.ProcessAPIServer
	baseURL      string
	sessionID    string
	inputs       *support.CapturedInputs
	command      *rootCompositionProcessCommand
	shutdown     func()
	shutdownOnce sync.Once
	stopOnce     sync.Once
	stop         func() error
	stopErr      error
	restore      func()
	restoreOnce  sync.Once
}

func startRootCompositionServer(
	t testing.TB,
	fixture *rootCompositionFixture,
	api *support.ProcessAPIServer,
	args []string,
	env []string,
	workingDirectory string,
) *rootCompositionServer {
	t.Helper()
	route, err := fixture.routes.routeForPath(workingDirectory)
	if err != nil {
		t.Fatalf("select route for shared Factory Session: %v", err)
	}
	if rootCompositionArgsRequireDirectProcess(args) {
		fixture.stopSharedHostForDirectProcess(t)
		return startRootCompositionDirectServer(t, fixture, route, api, args, env, workingDirectory)
	}

	baseURL := fixture.startSharedHost(t)
	opened := support.OpenFactorySessionAt(t, baseURL, route.workingDir)
	if opened.Session == nil || strings.TrimSpace(opened.Session.Id) == "" || opened.Session.Id == "~default" {
		t.Fatalf("shared Factory Session response = %#v, want unique explicit session", opened)
	}
	stop, err := route.cleanup.register("shared Factory Session "+opened.Session.Id, func() error {
		return closeRootCompositionSession(baseURL, opened.Session.Id)
	})
	if err != nil {
		_ = closeRootCompositionSession(baseURL, opened.Session.Id)
		t.Fatalf("register shared Factory Session cleanup: %v", err)
	}
	return &rootCompositionServer{
		api:       fixture.sharedHostAPI(),
		baseURL:   baseURL,
		sessionID: opened.Session.Id,
		stop:      stop,
	}
}

// startRootCompositionDirectServer runs one normal or replay CLI invocation
// through the package process while the caller owns the only active
// Process.Execute command. It is used by the behavior witnesses whose exact
// assertion is tied to the direct CLI/default-session lifecycle; API-session
// scenarios use startRootCompositionServer's long-lived host path instead.
func startRootCompositionDirectServer(
	t testing.TB,
	fixture *rootCompositionFixture,
	route *rootCompositionRoute,
	api *support.ProcessAPIServer,
	args []string,
	env []string,
	workingDirectory string,
) *rootCompositionServer {
	return startRootCompositionDirectServerWithGate(
		t, fixture, route, api, args, env, workingDirectory, nil,
	)
}

// startRootCompositionDirectServerWithGate exposes a bound API before letting
// Process.Execute leave server startup. This preserves the direct CLI
// behavior while allowing a caller to attach a public stream before the first
// scheduled dispatch emits its initial progress marker.
func startRootCompositionDirectServerWithGate(
	t testing.TB,
	fixture *rootCompositionFixture,
	route *rootCompositionRoute,
	api *support.ProcessAPIServer,
	args []string,
	env []string,
	workingDirectory string,
	release <-chan struct{},
) *rootCompositionServer {
	t.Helper()
	if api == nil {
		api = support.NewProcessAPIServer()
	}
	shutdown := make(chan struct{})
	api.HoldShutdownUntilSignaled(shutdown)
	bound := make(chan struct{})
	boundURL := make(chan string, 1)
	route.mu.Lock()
	originalStarter := route.apiStarter
	starter := originalStarter
	if starter == nil {
		starter = func(ctx context.Context, request platformhttpserver.StartRequest) error {
			return api.Start(ctx, request)
		}
	}
	route.apiStarter = func(ctx context.Context, request platformhttpserver.StartRequest) error {
		if release != nil {
			onBound := request.OnBound
			request.OnBound = func(binding platformhttpserver.Binding) {
				// The runtime's readiness observer may wait for the lifecycle
				// runner that is waiting for APIServerStarter to return. Keep
				// that callback out of the gate's synchronous path so the
				// caller can attach the public response stream first.
				close(bound)
				boundURL <- fmt.Sprintf("http://127.0.0.1:%d", binding.Port)
				if onBound != nil {
					go onBound(binding)
				}
				select {
				case <-release:
				case <-ctx.Done():
				}
			}
		}
		return starter(ctx, request)
	}
	route.mu.Unlock()
	restore := func() {
		route.mu.Lock()
		route.apiStarter = originalStarter
		route.mu.Unlock()
	}
	inputs := support.FakeInputs(t.Context(), append([]string(nil), args...))
	inputs.Input.WorkingDirectory = workingDirectory
	if env == nil {
		inputs.Input.Env = append(os.Environ(), "HOME="+route.homeDir, "USERPROFILE="+route.homeDir)
	} else {
		inputs.Input.Env = append([]string(nil), env...)
	}
	inputs.Input.Context = withRootCompositionRouteContext(inputs.Input.Context, route)
	command := startRootCompositionProcessCommand(fixture.process, inputs.Input)
	var baseURL string
	var waitErr error
	if release != nil {
		select {
		case <-bound:
			baseURL = <-boundURL
		case <-time.After(15 * time.Second):
			waitErr = errors.New("timed out waiting for gated process API server")
		}
	} else {
		baseURL, waitErr = api.WaitForBaseURL(15 * time.Second)
	}
	if waitErr != nil {
		t.Fatalf("start direct Factory Session server: %v; command error=%v chain=%s stdout=%q stderr=%q last replay=%q replay reads=%d", waitErr, command.Err(), rootCompositionErrorChain(command.Err()), inputs.Stdout(), inputs.Stderr(), fixture.effects.lastReplayPath(), fixture.effects.liveSnapshot().replayRecording)
	}
	return &rootCompositionServer{
		api:       api,
		baseURL:   baseURL,
		sessionID: factorysessions.DefaultSessionID,
		inputs:    inputs,
		command:   command,
		shutdown:  func() { close(shutdown) },
		stop:      command.stop,
		restore:   restore,
	}
}

func rootCompositionErrorChain(err error) string {
	var parts []string
	for err != nil && len(parts) < 12 {
		parts = append(parts, fmt.Sprintf("%T: %v", err, err))
		err = errors.Unwrap(err)
	}
	return strings.Join(parts, " <- ")
}

func rootCompositionArgsRequireDirectProcess(args []string) bool {
	for _, arg := range args {
		if arg == "--replay" || strings.HasPrefix(arg, "--replay=") ||
			arg == "--resume" || strings.HasPrefix(arg, "--resume=") {
			return true
		}
	}
	return false
}

func (server *rootCompositionServer) URL(t testing.TB) string {
	t.Helper()
	if server == nil || server.api == nil {
		t.Fatal("shared root composition server is unavailable")
	}
	if strings.TrimSpace(server.baseURL) == "" {
		t.Fatal("shared root composition server URL is empty")
	}
	return server.baseURL
}

func (server *rootCompositionServer) SessionID(t testing.TB) string {
	t.Helper()
	if server == nil || strings.TrimSpace(server.sessionID) == "" {
		t.Fatal("shared root composition Factory Session is unavailable")
	}
	return server.sessionID
}

func getRootCompositionSession(t testing.TB, baseURL, sessionID string) factoryapi.FactorySession {
	t.Helper()
	response := support.GetJSON[factoryapi.FactorySessionGetResponse](
		t,
		strings.TrimSuffix(baseURL, "/")+"/factory-sessions/"+url.PathEscape(sessionID),
	)
	session, err := response.AsFactorySession()
	if err != nil {
		t.Fatalf("decode shared Factory Session %q: %v", sessionID, err)
	}
	return session
}

func rootCompositionSessionWorkURL(baseURL, sessionID, suffix string) string {
	return strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + url.PathEscape(sessionID) + suffix
}

func waitForRootCompositionSessionStatus(
	t testing.TB,
	baseURL string,
	sessionID string,
	timeout time.Duration,
	accept func(factoryapi.StatusResponse) bool,
) factoryapi.StatusResponse {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		request, err := http.NewRequest(http.MethodGet, rootCompositionSessionWorkURL(baseURL, sessionID, "/status"), nil)
		if err == nil {
			response, requestErr := http.DefaultClient.Do(request)
			if requestErr == nil {
				body, readErr := io.ReadAll(response.Body)
				response.Body.Close()
				if readErr == nil && response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusMultipleChoices {
					var status factoryapi.StatusResponse
					if json.Unmarshal(body, &status) == nil && accept(status) {
						return status
					}
				}
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for Factory Session %q status at %s", sessionID, baseURL)
	return factoryapi.StatusResponse{}
}

func (server *rootCompositionServer) Stop(t testing.TB) {
	t.Helper()
	if server == nil {
		return
	}
	server.closeDirectSession(t)
	server.shutdownOnce.Do(func() {
		if server.shutdown != nil {
			server.shutdown()
		}
	})
	server.stopOnce.Do(func() {
		if server.stop != nil {
			server.stopErr = server.stop()
		}
	})
	if server.stopErr != nil {
		t.Errorf("stop shared root composition server: %v", server.stopErr)
	}
	if server.restore != nil {
		server.restoreOnce.Do(server.restore)
	}
}

func (server *rootCompositionServer) Finish(t testing.TB) {
	t.Helper()
	if server == nil {
		return
	}
	server.closeDirectSession(t)
	server.shutdownOnce.Do(func() {
		if server.shutdown != nil {
			server.shutdown()
		}
	})
	server.stopOnce.Do(func() {
		if server.command != nil {
			server.stopErr = server.command.wait()
		}
	})
	if server.stopErr != nil && !errors.Is(server.stopErr, context.Canceled) && !server.command.errorWasAccepted() {
		t.Errorf("finish shared root composition server: %v", server.stopErr)
	}
	if server.restore != nil {
		server.restoreOnce.Do(server.restore)
	}
}

// closeDirectSession retires the default Factory Session left behind by a
// direct CLI invocation. Shared API-session invocations already register a
// cleanup action for their explicit session; only the direct path needs this
// extra public close so the one shared process can accept the next default
// invocation in the same package run.
func (server *rootCompositionServer) closeDirectSession(t testing.TB) {
	t.Helper()
	if server == nil || server.sessionID != factorysessions.DefaultSessionID || server.baseURL == "" {
		return
	}
	if err := closeRootCompositionSession(server.baseURL, server.sessionID); err != nil {
		t.Errorf("close direct default Factory Session: %v", err)
	}
}

func (server *rootCompositionServer) Done() <-chan struct{} {
	if server == nil || server.command == nil {
		closed := make(chan struct{})
		close(closed)
		return closed
	}
	return server.command.done
}

func (server *rootCompositionServer) Err() error {
	if server == nil || server.command == nil {
		return nil
	}
	server.command.mu.Lock()
	defer server.command.mu.Unlock()
	return server.command.err
}

func closeRootCompositionSession(baseURL, sessionID string) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return errors.New("shared Factory Session ID is empty")
	}
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + url.PathEscape(sessionID) + "/terminate"
	request, err := http.NewRequest(http.MethodPost, endpoint, strings.NewReader("{}"))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return fmt.Errorf("terminate Factory Session %q: %w", sessionID, err)
	}
	body, readErr := io.ReadAll(response.Body)
	response.Body.Close()
	if readErr != nil {
		return fmt.Errorf("read terminate Factory Session %q response: %w", sessionID, readErr)
	}
	terminalSession := response.StatusCode == http.StatusConflict && strings.Contains(string(body), `"outcome":"TERMINAL_SESSION"`)
	if response.StatusCode == http.StatusNotFound {
		return nil
	}
	if !terminalSession && (response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices) {
		return fmt.Errorf("terminate Factory Session %q status = %d: %s", sessionID, response.StatusCode, strings.TrimSpace(string(body)))
	}

	deadline := time.Now().Add(10 * time.Second)
	for !terminalSession && time.Now().Before(deadline) {
		statusEndpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + url.PathEscape(sessionID) + "/status"
		statusRequest, requestErr := http.NewRequest(http.MethodGet, statusEndpoint, nil)
		if requestErr != nil {
			return requestErr
		}
		statusResponse, requestErr := http.DefaultClient.Do(statusRequest)
		if requestErr != nil {
			return fmt.Errorf("observe stopped Factory Session %q: %w", sessionID, requestErr)
		}
		statusBody, statusReadErr := io.ReadAll(statusResponse.Body)
		statusResponse.Body.Close()
		if statusReadErr != nil {
			return fmt.Errorf("read Factory Session %q status: %w", sessionID, statusReadErr)
		}
		if statusResponse.StatusCode == http.StatusNotFound {
			return nil
		}
		if statusResponse.StatusCode >= http.StatusOK && statusResponse.StatusCode < http.StatusMultipleChoices {
			var status factoryapi.StatusResponse
			if json.Unmarshal(statusBody, &status) == nil && (status.RuntimeStatus == "IDLE" || status.RuntimeStatus == "FINISHED") {
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !terminalSession && time.Now().After(deadline) {
		return fmt.Errorf("timed out waiting for Factory Session %q to stop", sessionID)
	}
	// The public contract keeps ~default as the stable default alias and does
	// not permit deleting that registry entry. Termination still releases its
	// active runtime binding, which is the resource a subsequent direct CLI
	// invocation needs to reuse this process.
	if sessionID == factorysessions.DefaultSessionID {
		return nil
	}

	deleteEndpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + url.PathEscape(sessionID)
	deleteRequest, err := http.NewRequest(http.MethodDelete, deleteEndpoint, nil)
	if err != nil {
		return err
	}
	deleteResponse, err := http.DefaultClient.Do(deleteRequest)
	if err != nil {
		return fmt.Errorf("delete Factory Session %q: %w", sessionID, err)
	}
	deleteBody, readErr := io.ReadAll(deleteResponse.Body)
	deleteResponse.Body.Close()
	if readErr != nil {
		return fmt.Errorf("read delete Factory Session %q response: %w", sessionID, readErr)
	}
	if deleteResponse.StatusCode != http.StatusNoContent && deleteResponse.StatusCode != http.StatusNotFound {
		return fmt.Errorf("delete Factory Session %q status = %d: %s", sessionID, deleteResponse.StatusCode, strings.TrimSpace(string(deleteBody)))
	}
	return nil
}
