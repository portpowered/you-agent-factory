package run_scoped_server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"github.com/portpowered/infinite-you/tests/functional/transport/terminalportlock"
)

const (
	goalFactoryName        = "@you/goal"
	goalWorkstationName    = "execute-goal"
	wantInvocationResponse = "mock worker accepted"
)

// TestRunScopedServerAndSiteOwnNamedAndFileInvocationLifecycles proves hosted invocation cleanup across selectors.
func TestRunScopedServerAndSiteOwnNamedAndFileInvocationLifecycles(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		site        bool
		file        bool
		stdin       string
		input       []string
		wantBrowser int32
	}{
		{name: "named positional server", input: []string{"server-scoped goal"}},
		{
			name: "file stdin site", site: true, file: true,
			stdin: "site-scoped goal\n", input: []string{"-"}, wantBrowser: 1,
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			homeDir := t.TempDir()
			workingDirectory := t.TempDir()
			var listenerStarts, listenerStops, browserCalls atomic.Int32
			providerRunner := support.NewShapedProviderCommandRunner(platformprocess.CommandResult{
				Stdout: []byte("{\"decision\":\"accepted\",\"feedback\":\"\",\"output\":\"mock worker accepted\"}"),
			})
			process, err := support.BuildProcessWithContext(t.Context(), serviceedges.Edges{
				APIServerStarter: func(ctx context.Context, request platformhttpserver.StartRequest) error {
					listenerStarts.Add(1)
					request.OnBound(platformhttpserver.Binding{Port: request.Port})
					<-ctx.Done()
					listenerStops.Add(1)
					return ctx.Err()
				},
				BrowserOpener: func(context.Context, string) error {
					browserCalls.Add(1)
					return nil
				},
				ProviderCommandRunner: providerRunner,
			})
			if err != nil {
				t.Fatalf("BuildProcess() error = %v", err)
			}
			environment := append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)
			factoryDir := initializeGoalFactory(t, process, environment, workingDirectory, homeDir)
			selection := []string{"--named", goalFactoryName}
			if test.file {
				selection = []string{"--factory", filepath.Join(factoryDir, "factory.json")}
			}
			mode := "--with-server"
			if test.site {
				mode = "--with-site"
			}
			args := append([]string{"you", "run"}, selection...)
			args = append(args,
				"--executor-provider", "codex",
				"--executor-model", "gpt-5-codex",
				"--no-record", mode,
			)
			args = append(args, test.input...)
			stdout, stderr := execute(
				t, process, environment, workingDirectory, args, test.stdin,
			)
			if !strings.Contains(stdout, "[0] factory started") ||
				!strings.Contains(stdout, "--- primary result ---") ||
				!strings.HasSuffix(stdout, wantInvocationResponse) {
				t.Fatalf("invocation stdout=%q stderr=%q", stdout, stderr)
			}
			if !strings.Contains(stderr, "dispatch ") ||
				!strings.Contains(stderr, "active at "+goalWorkstationName) ||
				!strings.Contains(stderr, "worker ") {
				t.Fatalf("invocation progress stderr=%q", stderr)
			}
			if listenerStarts.Load() != 1 || listenerStops.Load() != 1 {
				t.Fatalf(
					"listener lifecycle = starts:%d stops:%d, want exactly one joined server",
					listenerStarts.Load(),
					listenerStops.Load(),
				)
			}
			if browserCalls.Load() != test.wantBrowser {
				t.Fatalf("browser calls = %d, want %d", browserCalls.Load(), test.wantBrowser)
			}
		})
	}
}

// TestRunScopedServerOwnsRawJavaScriptLifecycleAfterReadiness proves raw JavaScript hosting shares one lifecycle.
func TestRunScopedServerOwnsRawJavaScriptLifecycleAfterReadiness(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name        string
		mode        string
		wantBrowser int32
	}{
		{name: "server", mode: "--with-server"},
		{name: "site", mode: "--with-site", wantBrowser: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			workingDirectory := t.TempDir()
			workflowPath := filepath.Join(workingDirectory, "workflow.js")
			if err := os.WriteFile(workflowPath, []byte(`return "hosted JavaScript";`), 0o600); err != nil {
				t.Fatalf("write workflow: %v", err)
			}
			var listenerStarts, listenerStops, browserCalls atomic.Int32
			process, err := support.BuildProcessWithContext(t.Context(), serviceedges.Edges{
				APIServerStarter: func(ctx context.Context, request platformhttpserver.StartRequest) error {
					listenerStarts.Add(1)
					assertDashboardHandler(t, request.Handler)
					request.OnBound(platformhttpserver.Binding{Port: request.Port})
					<-ctx.Done()
					listenerStops.Add(1)
					return ctx.Err()
				},
				BrowserOpener: func(context.Context, string) error {
					browserCalls.Add(1)
					return nil
				},
			})
			if err != nil {
				t.Fatalf("BuildProcess() error = %v", err)
			}
			homeDir := t.TempDir()
			environment := append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)
			stdout, stderr := execute(t, process, environment, workingDirectory, []string{
				"you", "run", "--factory", workflowPath, "--with-mock-workers", test.mode,
			}, "")
			if stderr != "" || !strings.Contains(stdout, "completed (SUCCEEDED)") {
				t.Fatalf("JavaScript stdout=%q stderr=%q", stdout, stderr)
			}
			if listenerStarts.Load() != 1 || listenerStops.Load() != 1 {
				t.Fatalf(
					"listener lifecycle = starts:%d stops:%d, want exactly one joined server",
					listenerStarts.Load(), listenerStops.Load(),
				)
			}
			if browserCalls.Load() != test.wantBrowser {
				t.Fatalf("browser calls = %d, want %d", browserCalls.Load(), test.wantBrowser)
			}
		})
	}
}

// TestRunScopedRawJavaScriptServerReportsUnavailableWorkerSessionOwner proves
// the root-built direct-JavaScript HTTP host preserves the public structured
// error when a route has no live Worker Sessions owner. Direct JavaScript
// hosting intentionally binds only the durable Factory Sessions handler, so
// this is the functional exception for the unavailable-owner composition
// without constructing transporthttp.NewServer in a functional test.
func TestRunScopedRawJavaScriptServerReportsUnavailableWorkerSessionOwner(t *testing.T) {
	t.Parallel()
	workingDirectory := t.TempDir()
	workflowPath := filepath.Join(workingDirectory, "workflow.js")
	if err := os.WriteFile(workflowPath, []byte(`return "hosted JavaScript";`), 0o600); err != nil {
		t.Fatalf("write workflow: %v", err)
	}

	type probeResult struct {
		status   int
		response factoryapi.ErrorResponse
		err      error
	}
	probes := make(chan probeResult, 1)
	process, err := support.BuildProcessWithContext(t.Context(), serviceedges.Edges{
		APIServerStarter: func(ctx context.Context, request platformhttpserver.StartRequest) error {
			server := httptest.NewServer(request.Handler)
			defer server.Close()

			client := &http.Client{Timeout: 5 * time.Second}
			response, requestErr := client.Get(server.URL + "/factory-sessions/~default/worker-sessions/worker-missing/events")
			if requestErr != nil {
				probes <- probeResult{err: requestErr}
			} else {
				defer response.Body.Close()
				var payload factoryapi.ErrorResponse
				decodeErr := json.NewDecoder(response.Body).Decode(&payload)
				probes <- probeResult{status: response.StatusCode, response: payload, err: decodeErr}
			}

			if request.OnBound != nil {
				request.OnBound(platformhttpserver.Binding{Port: request.Port})
			}
			<-ctx.Done()
			return ctx.Err()
		},
	})
	if err != nil {
		t.Fatalf("BuildProcess() error = %v", err)
	}

	homeDir := t.TempDir()
	environment := append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)
	stdout, stderr := execute(t, process, environment, workingDirectory, []string{
		"you", "run", "--factory", workflowPath, "--no-record", "--with-server",
	}, "")
	if stderr != "" || !strings.Contains(stdout, "completed (SUCCEEDED)") {
		t.Fatalf("JavaScript stdout=%q stderr=%q", stdout, stderr)
	}

	probe := <-probes
	if probe.err != nil {
		t.Fatalf("GET unavailable Worker Session route: %v", probe.err)
	}
	if probe.status != http.StatusInternalServerError {
		t.Fatalf("unavailable Worker Session route status = %d, want %d", probe.status, http.StatusInternalServerError)
	}
	if probe.response.Code != factoryapi.ErrorResponseCodeINTERNALERROR {
		t.Fatalf("unavailable Worker Session route error code = %q, want %q", probe.response.Code, factoryapi.ErrorResponseCodeINTERNALERROR)
	}
	if probe.response.Family != factoryapi.ErrorFamilyInternalServerError {
		t.Fatalf("unavailable Worker Session route error family = %q, want %q", probe.response.Family, factoryapi.ErrorFamilyInternalServerError)
	}
}

// TestRunScopedServerUsesProductionListenerAndReportsFallback proves the
// customer CLI path binds, reports, and joins the concrete HTTP server.
func TestRunScopedServerUsesProductionListenerAndReportsFallback(t *testing.T) {
	t.Parallel()
	busyListener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve requested loopback port: %v", err)
	}
	defer busyListener.Close()
	requestedPort := busyListener.Addr().(*net.TCPAddr).Port
	if requestedPort >= 65535 {
		t.Skip("OS selected the terminal TCP port; no higher fallback candidate exists")
	}

	workingDirectory := t.TempDir()
	workflowPath := filepath.Join(workingDirectory, "workflow.js")
	if err := os.WriteFile(workflowPath, []byte(`return "hosted JavaScript";`), 0o600); err != nil {
		t.Fatalf("write workflow: %v", err)
	}
	process, err := support.BuildProcessWithContext(t.Context(), serviceedges.Edges{})
	if err != nil {
		t.Fatalf("BuildProcess() error = %v", err)
	}
	homeDir := t.TempDir()
	environment := append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)
	requestedURL := "http://127.0.0.1:" + strconv.Itoa(requestedPort)
	stdout, stderr := execute(t, process, environment, workingDirectory, []string{
		"you", "--server", requestedURL, "run", "--factory", workflowPath,
		"--with-mock-workers", "--with-server",
	}, "")
	if !strings.Contains(stderr, "--server is deprecated") || strings.Count(stderr, "warning:") != 1 ||
		!strings.Contains(stdout, "completed (SUCCEEDED)") {
		t.Fatalf("JavaScript stdout=%q stderr=%q", stdout, stderr)
	}

	var actualURL string
	for _, line := range strings.Split(stdout, "\n") {
		if value, ok := strings.CutPrefix(line, "Dashboard URL: "); ok {
			actualURL = strings.TrimSpace(value)
			break
		}
	}
	parsed, err := url.Parse(actualURL)
	if err != nil || parsed.Hostname() != "127.0.0.1" {
		t.Fatalf("reported dashboard URL = %q, parse error = %v", actualURL, err)
	}
	actualPort, err := strconv.Atoi(parsed.Port())
	if err != nil || actualPort <= requestedPort {
		t.Fatalf("reported dashboard URL = %q, want fallback above %d", actualURL, requestedPort)
	}
	rebound, err := net.Listen("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(actualPort)))
	if err != nil {
		t.Fatalf("production listener remained bound after completion: %v", err)
	}
	_ = rebound.Close()
}

// TestRunScopedServerUsesExactListenAddress proves --listen binds the requested
// loopback port without entering the legacy ascending fallback path.
func TestRunScopedServerUsesExactListenAddress(t *testing.T) {
	t.Parallel()
	workingDirectory := t.TempDir()
	workflowPath := filepath.Join(workingDirectory, "workflow.js")
	if err := os.WriteFile(workflowPath, []byte(`return "hosted JavaScript";`), 0o600); err != nil {
		t.Fatalf("write workflow: %v", err)
	}
	requestedPort := reserveExactPort(t)
	process, err := support.BuildProcessWithContext(t.Context(), serviceedges.Edges{})
	if err != nil {
		t.Fatalf("BuildProcess() error = %v", err)
	}
	homeDir := t.TempDir()
	environment := append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)
	stdout, stderr := execute(t, process, environment, workingDirectory, []string{
		"you", "--server", "http://127.0.0.1:65534", "run", "--factory", workflowPath, "--with-server",
		"--listen", "127.0.0.1:" + strconv.Itoa(requestedPort),
	}, "")
	const wantWarning = "warning: --listen takes precedence over --server for the local listener; use --listen for the listener and reserve --server for the factory API endpoint\n"
	if stderr != wantWarning || !strings.Contains(stdout, "completed (SUCCEEDED)") {
		t.Fatalf("JavaScript stdout=%q stderr=%q", stdout, stderr)
	}
	wantURL := "Dashboard URL: http://127.0.0.1:" + strconv.Itoa(requestedPort) + "/dashboard/ui"
	if !strings.Contains(stdout, wantURL) {
		t.Fatalf("stdout = %q, want exact listener URL %q", stdout, wantURL)
	}
	rebound, err := net.Listen("tcp4", "127.0.0.1:"+strconv.Itoa(requestedPort))
	if err != nil {
		t.Fatalf("exact listener remained bound after completion: %v", err)
	}
	_ = rebound.Close()
}

// TestRemotePlacementDispatchesThroughSelectedServer proves a dual-placement
// run sends its normalized prompt to the selected server without starting a
// local listener or invoking the local runtime.
func TestRemotePlacementDispatchesThroughSelectedServer(t *testing.T) {
	t.Parallel()
	var gotRequest factoryapi.FactorySessionExecutionRequest
	var startRequests atomic.Int32
	var resultRequests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/factory-sessions/async":
			if err := json.NewDecoder(r.Body).Decode(&gotRequest); err != nil {
				t.Fatalf("decode remote durable execution request: %v", err)
			}
			startRequests.Add(1)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(factoryapi.FactorySessionExecutionResponse{
				SessionId:        "dur-sess-remote",
				Status:           factoryapi.FactorySessionDurableLifecycleStatusQueued,
				OrchestratorKind: factoryapi.PETRI,
			})
		case r.Method == http.MethodGet && r.URL.Path == "/factory-sessions/dur-sess-remote/results":
			resultRequests.Add(1)
			var part factoryapi.WorkContentPart
			if err := part.FromWorkTextContentPart(factoryapi.WorkTextContentPart{
				Type: factoryapi.WorkContentPartTypeText,
				Text: "remote result",
			}); err != nil {
				t.Fatalf("build remote result content: %v", err)
			}
			primaryResult := factoryapi.WorkContent{part}
			sessionStatus := factoryapi.FactorySessionDurableLifecycleStatusSucceeded
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(factoryapi.FactorySessionResult{
				SessionId:     "dur-sess-remote",
				ResultStatus:  factoryapi.FactorySessionResultStatusFinal,
				SessionStatus: &sessionStatus,
				PrimaryResult: &primaryResult,
			})
		default:
			t.Fatalf("remote request = %s %s, want durable start or result endpoint", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	workingDirectory := t.TempDir()
	factoryPath := filepath.Join(workingDirectory, "factory.json")
	if err := os.WriteFile(factoryPath, []byte(remotePlacementFactoryJSON), 0o600); err != nil {
		t.Fatalf("write remote factory: %v", err)
	}
	var localStarts atomic.Int32
	process, err := support.BuildProcessWithContext(t.Context(), serviceedges.Edges{
		APIServerStarter: func(context.Context, platformhttpserver.StartRequest) error {
			localStarts.Add(1)
			return nil
		},
	})
	if err != nil {
		t.Fatalf("BuildProcess() error = %v", err)
	}
	homeDir := t.TempDir()
	environment := append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)
	stdout, stderr := execute(t, process, environment, workingDirectory, []string{
		"you", "--remote", "--server", server.URL, "run", "--factory", factoryPath,
		"--no-record", "--output", "primary", "same normalized request",
	}, "")
	if stderr != "" || stdout != "remote result" {
		t.Fatalf("remote stdout=%q stderr=%q, want remote result and no diagnostics", stdout, stderr)
	}
	if startRequests.Load() != 1 || resultRequests.Load() != 1 || localStarts.Load() != 0 {
		t.Fatalf("dispatch effects = durable starts:%d results:%d local listeners:%d, want 1/1/0", startRequests.Load(), resultRequests.Load(), localStarts.Load())
	}
	if gotRequest.Source.Kind != factoryapi.FactorySessionExecutionSourceKindFactoryInline || gotRequest.Source.FactoryInline == nil {
		t.Fatalf("remote source = %#v, want normalized inline Factory source", gotRequest.Source)
	}
	if gotRequest.Args == nil || (*gotRequest.Args)["prompt"] != "same normalized request" {
		t.Fatalf("remote normalized args = %#v, want prompt argument", gotRequest.Args)
	}
}

func TestRunScopedServerRejectsUnavailableExactListenAddress(t *testing.T) {
	t.Parallel()
	busyListener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve exact listener port: %v", err)
	}
	defer busyListener.Close()
	requestedPort := busyListener.Addr().(*net.TCPAddr).Port
	workingDirectory := t.TempDir()
	workflowPath := filepath.Join(workingDirectory, "workflow.js")
	if err := os.WriteFile(workflowPath, []byte(`return "unreachable";`), 0o600); err != nil {
		t.Fatalf("write workflow: %v", err)
	}
	process, err := support.BuildProcessWithContext(t.Context(), serviceedges.Edges{})
	if err != nil {
		t.Fatalf("BuildProcess() error = %v", err)
	}
	homeDir := t.TempDir()
	environment := append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)
	var stdout, stderr bytes.Buffer
	err = process.Execute(root.Input{
		Args: []string{
			"you", "run", "--factory", workflowPath, "--with-server",
			"--listen", "127.0.0.1:" + strconv.Itoa(requestedPort),
		},
		Env: environment, Context: t.Context(), WorkingDirectory: workingDirectory,
		Stdout: &stdout, Stderr: &stderr,
	})
	if err == nil || !strings.Contains(err.Error(), "SERVER_BIND_FAILED") {
		t.Fatalf("exact bind error = %v, want SERVER_BIND_FAILED; stdout=%q stderr=%q", err, stdout.String(), stderr.String())
	}
	if strings.Contains(stdout.String(), "Dashboard URL:") || !strings.Contains(stderr.String(), `"code":"SERVER_BIND_FAILED"`) {
		t.Fatalf("stdout=%q stderr=%q, want no readiness and one structured bind failure", stdout.String(), stderr.String())
	}
}

// TestRunScopedServerRejectsRemoteBindTargetAtCLIBoundary proves remote bind
// targets fail at the customer CLI boundary before listener startup.
func TestRunScopedServerRejectsRemoteBindTargetAtCLIBoundary(t *testing.T) {
	t.Parallel()
	workingDirectory := t.TempDir()
	workflowPath := filepath.Join(workingDirectory, "workflow.js")
	if err := os.WriteFile(workflowPath, []byte(`return "unreachable";`), 0o600); err != nil {
		t.Fatalf("write workflow: %v", err)
	}
	homeDir := t.TempDir()
	environment := append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)
	process, err := support.BuildProcessWithContext(t.Context(), serviceedges.Edges{})
	if err != nil {
		t.Fatalf("BuildProcess() error = %v", err)
	}
	var stdout, stderr bytes.Buffer
	err = process.Execute(root.Input{
		Args: []string{
			"you", "--server", "https://remote.example.com:7443",
			"run", "--factory", workflowPath, "--with-mock-workers", "--with-server",
		},
		Env:              environment,
		Context:          t.Context(),
		WorkingDirectory: workingDirectory,
		Stdout:           &stdout,
		Stderr:           &stderr,
	})
	if err == nil || !strings.Contains(err.Error(), "not a local bind target") {
		t.Fatalf(
			"bind error = %v, want local-target guidance; stdout=%q stderr=%q",
			err, stdout.String(), stderr.String(),
		)
	}
}

// TestRemotePlacementRejectsLocalHostingBeforeInitialization proves the
// public process rejects contradictory placement before any local or remote
// runtime effect can start, regardless of persistent-flag position.
func TestRemotePlacementRejectsLocalHostingBeforeInitialization(t *testing.T) {
	t.Parallel()
	const wantCode = factoryapi.ErrorResponseCode("REMOTE_LOCAL_HOSTING_CONFLICT")
	const wantMessage = "--remote selects a running server through --server and cannot be combined with --with-server or --with-site; remove --remote for local hosting and use --listen <host:port> to choose an exact local bind"

	var effects atomic.Int32
	process, err := support.BuildProcessWithContext(t.Context(), serviceedges.Edges{
		APIServerStarter: func(context.Context, platformhttpserver.StartRequest) error {
			effects.Add(1)
			return nil
		},
		BrowserOpener: func(context.Context, string) error {
			effects.Add(1)
			return nil
		},
		FactorySessionIDGenerator: func() string {
			effects.Add(1)
			return "unexpected-session"
		},
	})
	if err != nil {
		t.Fatalf("BuildProcess() error = %v", err)
	}

	for _, test := range []struct {
		name string
		args []string
	}{
		{
			name: "persistent flags before run",
			args: []string{"you", "--remote", "--server", "https://selected.example:7443", "run", "--with-server"},
		},
		{
			name: "persistent flags after run",
			args: []string{"you", "run", "--with-site", "--remote", "--server", "https://selected.example:7443"},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			var stdout, stderr bytes.Buffer
			homeDir := t.TempDir()
			stdinIsTTY := true
			stdoutIsTTY := false
			err := process.Execute(root.Input{
				Args:             test.args,
				Env:              append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir),
				Stdin:            strings.NewReader(""),
				Stdout:           &stdout,
				Stderr:           &stderr,
				Context:          t.Context(),
				WorkingDirectory: t.TempDir(),
				StdinIsTTY:       &stdinIsTTY,
				StdoutIsTTY:      &stdoutIsTTY,
			})
			if err == nil {
				t.Fatal("remote/local hosting conflict unexpectedly succeeded")
			}
			var response factoryapi.ErrorResponse
			if decodeErr := json.Unmarshal([]byte(strings.TrimSpace(stderr.String())), &response); decodeErr != nil {
				t.Fatalf("stderr = %q, want one ErrorResponse: %v", stderr.String(), decodeErr)
			}
			if response.Code != wantCode || response.Family != factoryapi.ErrorFamilyBadRequest || response.Message != wantMessage {
				t.Fatalf("ErrorResponse = %#v, want stable placement conflict", response)
			}
			if stdout.Len() != 0 {
				t.Fatalf("stdout = %q, want empty", stdout.String())
			}
		})
	}
	if effects.Load() != 0 {
		t.Fatalf("placement conflict effects = %d, want no listener, browser, or session effects", effects.Load())
	}
}

// TestRemotePlacementRejectsLocalOnlyServerCommand proves remote placement
// remains explicit for commands that can only own local listener state.
func TestRemotePlacementRejectsLocalOnlyServerCommand(t *testing.T) {
	t.Parallel()
	var listenerStarts atomic.Int32
	process, err := support.BuildProcessWithContext(t.Context(), serviceedges.Edges{
		APIServerStarter: func(context.Context, platformhttpserver.StartRequest) error {
			listenerStarts.Add(1)
			return nil
		},
	})
	if err != nil {
		t.Fatalf("BuildProcess() error = %v", err)
	}

	stdout, stderr, executeErr := executeFactoryArgsForRunScopedTest(
		t,
		process,
		[]string{"you", "--remote", "--server", "https://selected.example:7443", "server"},
	)
	if executeErr == nil || !strings.Contains(executeErr.Error(), "supports local placement only") {
		t.Fatalf("remote server error = %v, want local-placement guidance; stdout=%q stderr=%q", executeErr, stdout, stderr)
	}
	if listenerStarts.Load() != 0 {
		t.Fatalf("listener starts = %d, want 0", listenerStarts.Load())
	}
}

// TestRemotePlacementRejectsLocalOnlyFactoryCommand proves manifest-projected
// local-only commands fail at the generic placement boundary before their
// handler can inspect the requested file.
func TestRemotePlacementRejectsLocalOnlyFactoryCommand(t *testing.T) {
	t.Parallel()
	process, err := support.BuildProcessWithContext(t.Context(), serviceedges.Edges{})
	if err != nil {
		t.Fatalf("BuildProcess() error = %v", err)
	}

	stdout, stderr, executeErr := executeFactoryArgsForRunScopedTest(
		t,
		process,
		[]string{
			"you", "--remote", "--server", "https://selected.example:7443",
			"factory", "config", "validate", filepath.Join(t.TempDir(), "factory.json"),
		},
	)
	if executeErr == nil || !strings.Contains(executeErr.Error(), "supports local placement only") {
		t.Fatalf("remote local-only factory error = %v, want placement guidance; stdout=%q stderr=%q", executeErr, stdout, stderr)
	}
}

// TestRunRejectsMalformedExactListenAddress proves --listen is parsed as an
// exact local host:port before the listener or Factory runtime starts.
func TestRunRejectsMalformedExactListenAddress(t *testing.T) {
	t.Parallel()
	var listenerStarts atomic.Int32
	process, err := support.BuildProcessWithContext(t.Context(), serviceedges.Edges{
		APIServerStarter: func(context.Context, platformhttpserver.StartRequest) error {
			listenerStarts.Add(1)
			return nil
		},
	})
	if err != nil {
		t.Fatalf("BuildProcess() error = %v", err)
	}

	stdout, stderr, executeErr := executeFactoryArgsForRunScopedTest(
		t,
		process,
		[]string{"you", "run", "--with-server", "--listen", "127.0.0.1"},
	)
	if executeErr == nil || !strings.Contains(executeErr.Error(), "invalid --listen address") {
		t.Fatalf("malformed listen error = %v, want exact-address guidance; stdout=%q stderr=%q", executeErr, stdout, stderr)
	}
	if listenerStarts.Load() != 0 {
		t.Fatalf("listener starts = %d, want 0", listenerStarts.Load())
	}
}

// TestRunScopedServerReportsExhaustedTerminalPortAtCLIBoundary proves port
// exhaustion is reported through the customer CLI contract.
func TestRunScopedServerReportsExhaustedTerminalPortAtCLIBoundary(t *testing.T) {
	t.Parallel()
	workingDirectory := t.TempDir()
	workflowPath := filepath.Join(workingDirectory, "workflow.js")
	if err := os.WriteFile(workflowPath, []byte(`return "unreachable";`), 0o600); err != nil {
		t.Fatalf("write workflow: %v", err)
	}
	process, err := support.BuildProcessWithContext(t.Context(), serviceedges.Edges{})
	if err != nil {
		t.Fatalf("BuildProcess() error = %v", err)
	}

	// Coordinate with the built-executable server_binding package before
	// opening the shared terminal endpoint. The lock is cross-process because
	// go test runs each package in its own process.
	releasePortLock, err := terminalportlock.Acquire()
	if err != nil {
		t.Fatalf("acquire terminal loopback test lock: %v", err)
	}
	defer func() {
		if err := releasePortLock(); err != nil {
			t.Errorf("release terminal loopback test lock: %v", err)
		}
	}()

	busyListener, err := net.Listen("tcp4", "127.0.0.1:65535")
	if err != nil {
		t.Fatalf("reserve terminal loopback port while owning test lock: %v", err)
	}
	defer func() {
		if err := busyListener.Close(); err != nil {
			t.Errorf("close terminal loopback listener: %v", err)
		}
	}()

	homeDir := t.TempDir()
	environment := append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)
	var stdout, stderr bytes.Buffer
	err = process.Execute(root.Input{
		Args: []string{
			"you", "--server", "http://127.0.0.1:65535",
			"run", "--factory", workflowPath, "--with-mock-workers", "--with-server",
		},
		Env:              environment,
		Context:          t.Context(),
		WorkingDirectory: workingDirectory,
		Stdout:           &stdout,
		Stderr:           &stderr,
	})
	if err == nil || !strings.Contains(err.Error(), "through 65535") {
		t.Fatalf(
			"terminal-port bind error = %v, want exhaustion guidance; stdout=%q stderr=%q",
			err, stdout.String(), stderr.String(),
		)
	}
}

// TestRunScopedServerOwnsReplayLifecycle proves replay hosting joins its listener at terminal completion.
func TestRunScopedServerOwnsReplayLifecycle(t *testing.T) {
	t.Parallel()
	homeDir := t.TempDir()
	workingDirectory := t.TempDir()
	var listenerStarts, listenerStops, browserCalls atomic.Int32
	process, err := support.BuildProcessWithContext(t.Context(), serviceedges.Edges{
		APIServerStarter: func(ctx context.Context, request platformhttpserver.StartRequest) error {
			listenerStarts.Add(1)
			request.OnBound(platformhttpserver.Binding{Port: request.Port})
			<-ctx.Done()
			listenerStops.Add(1)
			return ctx.Err()
		},
		BrowserOpener: func(context.Context, string) error {
			browserCalls.Add(1)
			return nil
		},
	})
	if err != nil {
		t.Fatalf("BuildProcess() error = %v", err)
	}
	environment := append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)
	initializeGoalFactory(t, process, environment, workingDirectory, homeDir)
	mockWorkersPath := writeMockWorkersConfig(t)
	replayPath := filepath.Join(t.TempDir(), "goal.replay.json")
	_, _ = execute(t, process, environment, workingDirectory, []string{
		"you", "run", "--named", goalFactoryName,
		"--with-mock-workers", mockWorkersPath, "--record", replayPath, "record replay",
	}, "")
	stdout, stderr := execute(t, process, environment, workingDirectory, []string{
		"you", "run", "--replay", replayPath, "--with-server",
	}, "")
	if stderr != "" || stdout == "" {
		t.Fatalf("replay stdout=%q stderr=%q", stdout, stderr)
	}
	if listenerStarts.Load() != 1 || listenerStops.Load() != 1 || browserCalls.Load() != 0 {
		t.Fatalf(
			"replay lifecycle = starts:%d stops:%d browsers:%d",
			listenerStarts.Load(), listenerStops.Load(), browserCalls.Load(),
		)
	}
}

func assertDashboardHandler(t *testing.T, handler http.Handler) {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, "/dashboard/ui", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("dashboard status = %d, want 200", response.Code)
	}
}

type process interface {
	Execute(root.Input) error
}

func initializeGoalFactory(
	t *testing.T,
	application process,
	environment []string,
	workingDirectory string,
	homeDir string,
) string {
	t.Helper()
	missingFactory := filepath.Join(workingDirectory, "missing-initialization-factory.json")
	err := application.Execute(root.Input{
		Args:             []string{"you", "run", "--factory", missingFactory},
		Env:              environment,
		Context:          t.Context(),
		WorkingDirectory: workingDirectory,
	})
	if err == nil {
		t.Fatal("missing Factory initialization unexpectedly succeeded")
	}
	factoryDir := filepath.Join(homeDir, ".you-agent-factory", "factories", "@you", "goal")
	if _, err := os.Stat(filepath.Join(factoryDir, "factory.json")); err != nil {
		t.Fatalf("installed packaged Factory %q: %v", goalFactoryName, err)
	}
	return factoryDir
}

func writeMockWorkersConfig(t *testing.T) string {
	t.Helper()
	payload, err := json.Marshal(workers.MockWorkersConfig{
		UnmatchedDispatchPolicy: workers.MockWorkerUnmatchedDispatchPolicyPassthrough,
		MockWorkers: []workers.MockWorkerConfig{{
			WorkerName:      "goal-executor",
			WorkstationName: goalWorkstationName,
			RunType:         workers.MockWorkerRunTypeAccept,
		}},
	})
	if err != nil {
		t.Fatalf("marshal mock workers config: %v", err)
	}
	path := filepath.Join(t.TempDir(), "mock-workers.json")
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		t.Fatalf("write mock workers config: %v", err)
	}
	return path
}

func execute(
	t *testing.T,
	application process,
	environment []string,
	workingDirectory string,
	args []string,
	stdin string,
) (string, string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	defer cancel()
	stdinIsTTY := true
	stdoutIsTTY := false
	var stdout, stderr bytes.Buffer
	err := application.Execute(root.Input{
		Args: args, Env: environment, Stdin: strings.NewReader(stdin),
		Stdout: &stdout, Stderr: &stderr, Context: ctx,
		WorkingDirectory: workingDirectory,
		StdinIsTTY:       &stdinIsTTY,
		StdoutIsTTY:      &stdoutIsTTY,
	})
	if err != nil {
		t.Fatalf("Process.Execute(%v) error = %v; stdout=%q stderr=%q", args, err, stdout.String(), stderr.String())
	}
	return stdout.String(), stderr.String()
}

func executeFactoryArgsForRunScopedTest(
	t *testing.T,
	application process,
	args []string,
) (string, string, error) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	stdinIsTTY := true
	stdoutIsTTY := false
	err := application.Execute(root.Input{
		Args:             args,
		Env:              append(os.Environ(), "HOME="+t.TempDir(), "USERPROFILE="+t.TempDir()),
		Stdin:            strings.NewReader(""),
		Stdout:           &stdout,
		Stderr:           &stderr,
		Context:          t.Context(),
		WorkingDirectory: t.TempDir(),
		StdinIsTTY:       &stdinIsTTY,
		StdoutIsTTY:      &stdoutIsTTY,
	})
	return stdout.String(), stderr.String(), err
}

func reserveExactPort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve exact listener port: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatalf("release exact listener port: %v", err)
	}
	return port
}

const remotePlacementFactoryJSON = `{
  "name": "remote-placement",
  "invocationSignature": {
    "parameters": [{
      "name": "prompt",
      "required": true,
      "bindings": [{"kind": "POSITIONAL", "position": 1}]
    }]
  },
  "workTypes": [
    {
      "name": "task",
      "states": [
        {"name": "init", "type": "INITIAL"},
        {"name": "complete", "type": "TERMINAL"},
        {"name": "failed", "type": "FAILED"}
      ]
    }
  ],
  "workers": [{"name": "processor"}],
  "workstations": [
    {
      "name": "process",
      "inputs": [{"workType": "task", "state": "init"}],
      "outputs": [{"workType": "task", "state": "complete"}],
      "onFailure": [{"workType": "task", "state": "failed"}],
      "worker": "processor"
    }
  ]
}`
