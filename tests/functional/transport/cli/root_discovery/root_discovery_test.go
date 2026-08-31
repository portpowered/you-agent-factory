package root_discovery_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func startupOutputValue(output, label string) string {
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, label) {
			return strings.TrimSpace(strings.TrimPrefix(line, label))
		}
	}
	return ""
}

func pathUnderDirectory(path, directory string) bool {
	path = filepath.Clean(path)
	directory = filepath.Clean(directory)
	if path == directory {
		return true
	}
	return strings.HasPrefix(path, directory+string(os.PathSeparator))
}

// TestManifestProjectedRepresentativeHandlersAcceptCanonicalInputs proves the
// generated Session and Work leaves reach their typed transport handlers through
// the public process root with the canonical argument and flag shapes.
func TestManifestProjectedRepresentativeHandlersAcceptCanonicalInputs(t *testing.T) {
	t.Parallel()

	fixture := rootDiscoverySharedFixtureForTest(t)
	workingDirectory := t.TempDir()
	unavailableServer := "http://127.0.0.1:1"
	cases := [][]string{
		{"you", "--server", unavailableServer, "session", "list", "--json"},
		{"you", "--server", unavailableServer, "session", "show", "session-1", "--json"},
		{"you", "--server", unavailableServer, "session", "delete", "session-1", "--json"},
		{"you", "--remote", "--server", unavailableServer, "session", "pause", "session-1", "--json"},
		{"you", "--remote", "--server", unavailableServer, "session", "resume", "session-1", "--json"},
		{"you", "--server", unavailableServer, "work", "list", "--json"},
		{"you", "--server", unavailableServer, "work", "show", "work-1", "--json"},
		{"you", "--server", unavailableServer, "work", "move", "work-1", "complete", "--json"},
		{"you", "--server", unavailableServer, "worker-sessions", "list", "--json"},
	}
	for _, args := range cases {
		_, _, executeErr := fixture.executeArgs(
			t,
			workingDirectory,
			args,
			false,
			t.Context(),
			nil,
		)
		if executeErr == nil {
			t.Fatalf("%v unexpectedly succeeded against unavailable server", args)
		}
		for _, rejected := range []string{"unknown command", "unknown flag", "accepts "} {
			if strings.Contains(executeErr.Error(), rejected) {
				t.Fatalf("%v rejected before its handler: %v", args, executeErr)
			}
		}
	}
}

// TestCurrentFactoryFailsBeforeProductActivation proves invalid Current Factory selection is side-effect free.
func TestCurrentFactoryFailsBeforeProductActivation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		prepare  func(*testing.T, string)
		wantCode factoryapi.ErrorResponseCode
	}{
		{
			name: "missing exact factory json ignores current pointer",
			prepare: func(t *testing.T, factoryDir string) {
				t.Helper()
				if err := os.MkdirAll(filepath.Join(factoryDir, "alternate"), 0o755); err != nil {
					t.Fatalf("create alternate Factory: %v", err)
				}
				if err := os.WriteFile(filepath.Join(factoryDir, ".current-factory"), []byte("alternate\n"), 0o600); err != nil {
					t.Fatalf("write current pointer: %v", err)
				}
				if err := os.WriteFile(filepath.Join(factoryDir, "alternate", "factory.json"), []byte(`{}`), 0o600); err != nil {
					t.Fatalf("write alternate Factory: %v", err)
				}
			},
			wantCode: "CURRENT_FACTORY_NOT_FOUND",
		},
		{
			name: "invalid exact factory json",
			prepare: func(t *testing.T, factoryDir string) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(factoryDir, "factory.json"), []byte(`{"name":`), 0o600); err != nil {
					t.Fatalf("write invalid Current Factory: %v", err)
				}
			},
			wantCode: "CURRENT_FACTORY_INVALID",
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			runCurrentFactoryFailureCase(t, test.prepare, test.wantCode)
		})
	}
}

// TestServerCurrentFactoryFailsBeforeProductActivation proves server validation precedes product activation.
func TestServerCurrentFactoryFailsBeforeProductActivation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		prepare  func(*testing.T, string)
		wantCode factoryapi.ErrorResponseCode
	}{
		{
			name:     "missing exact factory json",
			prepare:  func(*testing.T, string) {},
			wantCode: "CURRENT_FACTORY_NOT_FOUND",
		},
		{
			name: "invalid exact factory json",
			prepare: func(t *testing.T, factoryDir string) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(factoryDir, "factory.json"), []byte(`{"name":`), 0o600); err != nil {
					t.Fatalf("write invalid Current Factory: %v", err)
				}
			},
			wantCode: "CURRENT_FACTORY_INVALID",
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			runCurrentFactoryFailureCaseForCommand(t, "server", test.prepare, test.wantCode)
		})
	}
}

// TestRunScopedSiteMissingCurrentFactoryFailsBeforeProductActivation proves site selection does not bypass validation.
func TestRunScopedSiteMissingCurrentFactoryFailsBeforeProductActivation(t *testing.T) {
	t.Parallel()

	workingDirectory := t.TempDir()
	fixture := rootDiscoverySharedFixtureForTest(t)
	var effects atomic.Int32
	stdout, stderr, executeErr := fixture.executeArgsWithSessionID(
		t,
		workingDirectory,
		[]string{"you", "run", "--no-record", "--with-site"},
		false,
		t.Context(),
		&effects,
	)
	if executeErr == nil {
		t.Fatalf("missing Current Factory succeeded; stdout=%q stderr=%q", stdout, stderr)
	}
	if effects.Load() != 0 {
		t.Fatalf("pre-session site failure product effects = %d, want 0", effects.Load())
	}
}

func runCurrentFactoryFailureCase(
	t *testing.T,
	prepare func(*testing.T, string),
	wantCode factoryapi.ErrorResponseCode,
) {
	runCurrentFactoryFailureCaseForCommand(t, "run", prepare, wantCode)
}

func runCurrentFactoryFailureCaseForCommand(
	t *testing.T,
	command string,
	prepare func(*testing.T, string),
	wantCode factoryapi.ErrorResponseCode,
) {
	t.Helper()
	workingDirectory := t.TempDir()
	factoryDir := filepath.Join(workingDirectory, "factory")
	if err := os.MkdirAll(factoryDir, 0o755); err != nil {
		t.Fatalf("create Current Factory directory: %v", err)
	}
	prepare(t, factoryDir)

	fixture := rootDiscoverySharedFixtureForTest(t)
	var effects atomic.Int32
	stdout, stderr, executeErr := fixture.executeCommandWithSessionID(
		t,
		workingDirectory,
		command,
		false,
		t.Context(),
		&effects,
	)
	if executeErr == nil {
		t.Fatalf("Process.Execute(Current Factory) succeeded; stdout=%q stderr=%q", stdout, stderr)
	}
	var response factoryapi.ErrorResponse
	if err := json.Unmarshal(bytes.TrimSpace([]byte(stderr)), &response); err != nil {
		t.Fatalf("stderr is not one ErrorResponse: %v\n%s", err, stderr)
	}
	if response.Code != wantCode {
		t.Fatalf("ErrorResponse = %#v, want code %s", response, wantCode)
	}
	if stdout != "" {
		t.Fatalf("Current Factory failure stdout = %q, want empty", stdout)
	}
	if effects.Load() != 0 {
		t.Fatalf("Current Factory failure product effects = %d, want 0", effects.Load())
	}
}

// TestServerNonTTYReadinessGatesBrowserAndCancellationJoinsOwnedServer proves
// that server-owned browser startup is independent of stdout TTY state.
func TestServerNonTTYReadinessGatesBrowserAndCancellationJoinsOwnedServer(t *testing.T) {
	t.Parallel()

	for iteration := 0; iteration < 3; iteration++ {
		t.Run(fmt.Sprintf("iteration-%d", iteration), func(t *testing.T) {
			t.Parallel()

			runServerLifecycleCase(t)
		})
	}
}

func runServerLifecycleCase(t *testing.T) {
	t.Helper()
	workingDirectory := t.TempDir()
	factoryDir := filepath.Join(workingDirectory, "factory")
	if err := os.MkdirAll(factoryDir, 0o755); err != nil {
		t.Fatalf("create Current Factory directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(factoryDir, "factory.json"), []byte(idleCurrentFactoryJSON), 0o600); err != nil {
		t.Fatalf("write Current Factory: %v", err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	serverStopped := make(chan struct{})
	var bound, browserCalls, providerCalls atomic.Int32
	// Isolated: each iteration proves listener readiness and joins its own
	// cancellation-owned server; a shared root would conflate those witnesses.
	process, err := support.BuildProcessWithContext(t.Context(), serviceedges.Edges{
		APIServerStarter: func(serverCtx context.Context, request platformhttpserver.StartRequest) error {
			if request.Handler == nil || request.OnBound == nil {
				return errors.New("incomplete server start request")
			}
			bound.Store(1)
			request.OnBound(platformhttpserver.Binding{Port: request.Port})
			<-serverCtx.Done()
			close(serverStopped)
			return serverCtx.Err()
		},
		BrowserOpener: func(_ context.Context, target string) error {
			if bound.Load() == 0 {
				return errors.New("browser opened before listener readiness")
			}
			if !strings.Contains(target, "/dashboard/ui") {
				return fmt.Errorf("browser target = %q, want dashboard URL", target)
			}
			browserCalls.Add(1)
			cancel()
			return nil
		},
		ProviderOverride: newCountingProvider(&providerCalls),
	})
	if err != nil {
		t.Fatalf("BuildProcess() error = %v", err)
	}
	support.CleanupProcess(t, process)

	stdout, stderr, executeErr := executeFactoryCommandWithContext(
		t, process, workingDirectory, "server", false, ctx,
	)
	if executeErr != nil && !errors.Is(executeErr, context.Canceled) {
		t.Fatalf("Process.Execute(server) error = %v; stdout=%q stderr=%q", executeErr, stdout, stderr)
	}
	if stderr != "" {
		t.Fatalf("server stderr = %q, want empty", stderr)
	}
	if browserCalls.Load() != 1 {
		t.Fatalf("browser calls = %d, want 1", browserCalls.Load())
	}
	if providerCalls.Load() != 0 {
		t.Fatalf("provider calls = %d, want 0", providerCalls.Load())
	}
	select {
	case <-serverStopped:
	case <-time.After(5 * time.Second):
		t.Fatal("server cancellation did not join the owned listener")
	}
	for _, expected := range []string{"Factory initiated: " + factoryDir, "Dashboard URL:"} {
		if !strings.Contains(stdout, expected) {
			t.Fatalf("server stdout omitted %q:\n%s", expected, stdout)
		}
	}
}

// TestServerBindExhaustionWritesDeclaredErrorWithoutResidualEffects proves terminal bind failures leave no lifecycle effects.
func TestServerBindExhaustionWritesDeclaredErrorWithoutResidualEffects(t *testing.T) {
	t.Parallel()

	workingDirectory := t.TempDir()
	factoryDir := filepath.Join(workingDirectory, "factory")
	if err := os.MkdirAll(factoryDir, 0o755); err != nil {
		t.Fatalf("create Current Factory directory: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(factoryDir, "factory.json"),
		[]byte(idleCurrentFactoryJSON),
		0o600,
	); err != nil {
		t.Fatalf("write Current Factory: %v", err)
	}

	var attempts []string
	starter, err := platformhttpserver.NewStarter(func(_ string, address string) (net.Listener, error) {
		attempts = append(attempts, address)
		return nil, errors.New("address unavailable")
	}, nil, nil)
	if err != nil {
		t.Fatalf("NewStarter() error = %v", err)
	}
	var browserCalls, readinessCalls atomic.Int32
	// Isolated: this injects a unique listener factory and asserts exact bind
	// attempts plus the absence of readiness effects after terminal failure.
	process, err := support.BuildProcessWithContext(t.Context(), serviceedges.Edges{
		APIServerStarter: starter,
		BrowserOpener: func(context.Context, string) error {
			browserCalls.Add(1)
			return nil
		},
		RuntimeHostObserver: func(factorysessions.RuntimeHostBinding) {
			readinessCalls.Add(1)
		},
	})
	if err != nil {
		t.Fatalf("BuildProcess() error = %v", err)
	}
	support.CleanupProcess(t, process)

	stdout, stderr, executeErr := executeFactoryArgs(
		t,
		process,
		workingDirectory,
		[]string{"you", "--server", "http://127.0.0.1:65534", "server"},
		false,
		t.Context(),
	)
	if executeErr == nil {
		t.Fatalf("Process.Execute(server) error = nil; stdout=%q stderr=%q", stdout, stderr)
	}
	for _, forbidden := range []string{"Factory initiated:", "Dashboard URL:"} {
		if strings.Contains(stdout, forbidden) {
			t.Fatalf("server stdout exposed readiness %q before bind failure:\n%s", forbidden, stdout)
		}
	}
	stderrLines := strings.Split(strings.TrimSpace(stderr), "\n")
	if len(stderrLines) != 2 || !strings.Contains(stderrLines[0], "--server is deprecated") || !strings.Contains(stderrLines[0], "--listen") {
		t.Fatalf("server stderr = %q, want one migration warning followed by one ErrorResponse", stderr)
	}
	var response factoryapi.ErrorResponse
	if err := json.Unmarshal([]byte(stderrLines[1]), &response); err != nil {
		t.Fatalf("server stderr ErrorResponse is invalid: %v\n%s", err, stderr)
	}
	if response.Code != factoryapi.ErrorResponseCode("SERVER_BIND_FAILED") {
		t.Fatalf("ErrorResponse = %#v, want SERVER_BIND_FAILED", response)
	}
	if got, want := strings.Join(attempts, ","), "127.0.0.1:65534,127.0.0.1:65535"; got != want {
		t.Fatalf("listener attempts = %q, want %q", got, want)
	}
	if browserCalls.Load() != 0 || readinessCalls.Load() != 0 {
		t.Fatalf(
			"post-failure effects = browser:%d readiness:%d, want none",
			browserCalls.Load(),
			readinessCalls.Load(),
		)
	}
}

// TestCurrentFactoryRunsToIdleWithoutStartingServer proves ordinary Current Factory runs remain serverless.
func TestCurrentFactoryRunsToIdleWithoutStartingServer(t *testing.T) {
	t.Parallel()

	workingDirectory := t.TempDir()
	fixture := rootDiscoverySharedFixtureForTest(t)
	factoryDir := filepath.Join(workingDirectory, "factory")
	if err := os.MkdirAll(factoryDir, 0o755); err != nil {
		t.Fatalf("create Current Factory directory: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(factoryDir, "factory.json"),
		[]byte(idleCurrentFactoryJSON),
		0o600,
	); err != nil {
		t.Fatalf("write Current Factory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(factoryDir, ".current-factory"), []byte("alternate\n"), 0o600); err != nil {
		t.Fatalf("write ignored current pointer: %v", err)
	}

	var effects atomic.Int32
	stdout, stderr, executeErr := fixture.executeCommand(
		t,
		workingDirectory,
		"run",
		false,
		t.Context(),
		&effects,
	)
	if executeErr != nil {
		t.Fatalf("Process.Execute(Current Factory) error = %v; stdout=%q stderr=%q", executeErr, stdout, stderr)
	}
	if stderr != "" {
		t.Fatalf("Current Factory stderr = %q, want empty", stderr)
	}
	for _, expected := range []string{
		"Factory initiated: " + factoryDir,
		"Dashboard server disabled",
	} {
		if !strings.Contains(stdout, expected) {
			t.Fatalf("Current Factory stdout omitted %q:\n%s", expected, stdout)
		}
	}
	if effects.Load() != 0 {
		t.Fatalf("idle Current Factory external effects = %d, want no listener, browser, or provider call", effects.Load())
	}
}

// TestLocalRunDisclosesHomeBeforeSystemInitializationAccess proves the
// customer process writes the resolved home before the injected initialization
// path inspects an artifact beneath that home.
func TestLocalRunDisclosesHomeBeforeSystemInitializationAccess(t *testing.T) {
	workingDirectory := t.TempDir()
	factoryDir := filepath.Join(workingDirectory, "factory")
	if err := os.MkdirAll(factoryDir, 0o755); err != nil {
		t.Fatalf("create Factory directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(factoryDir, "factory.json"), []byte(idleCurrentFactoryJSON), 0o600); err != nil {
		t.Fatalf("write Factory config: %v", err)
	}

	homeDir := t.TempDir()
	events := make([]string, 0, 2)
	stdout := &orderedStartupOutput{events: &events}
	var stderr bytes.Buffer
	// Isolated: HOME and startup-artifact ordering are invocation-owned
	// environment witnesses that must not share process initialization state.
	process, err := support.BuildProcessWithContext(t.Context(), serviceedges.Edges{
		SystemInitializationInspectPath: func(path string) (fs.FileInfo, error) {
			events = append(events, "initialize:"+path)
			return nil, fs.ErrNotExist
		},
	})
	if err != nil {
		t.Fatalf("BuildProcess() error = %v", err)
	}
	support.CleanupProcess(t, process)
	stdinIsTTY := true
	stdoutIsTTY := false
	err = process.Execute(root.Input{
		Args:             []string{"you", "run", "--dir", factoryDir, "--no-record"},
		Env:              append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir),
		Stdin:            strings.NewReader(""),
		Stdout:           stdout,
		Stderr:           &stderr,
		Context:          t.Context(),
		WorkingDirectory: workingDirectory,
		StdinIsTTY:       &stdinIsTTY,
		StdoutIsTTY:      &stdoutIsTTY,
	})
	if err != nil {
		t.Fatalf("Process.Execute(local run) error = %v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
	}
	homeIndex := -1
	initializationIndex := -1
	for index, event := range events {
		if event == "home" && homeIndex < 0 {
			homeIndex = index
		}
		if strings.HasPrefix(event, "initialize:") && initializationIndex < 0 {
			initializationIndex = index
		}
	}
	if homeIndex < 0 || initializationIndex < 0 || homeIndex > initializationIndex {
		t.Fatalf("startup events = %#v, want home followed by initialization access", events)
	}
	if got, want := stdout.String(), "Home directory: "+homeDir+"\n"; !strings.HasPrefix(got, want) {
		t.Fatalf("stdout = %q, want prefix %q", got, want)
	}
	for _, label := range []string{"Runtime log: ", "Runtime metrics: "} {
		path := startupOutputValue(stdout.String(), label)
		if path == "" {
			t.Fatalf("stdout = %q, want %s startup artifact", stdout.String(), label)
		}
		if !pathUnderDirectory(path, homeDir) {
			t.Fatalf("%s path = %q, want beneath resolved home %q", label, path, homeDir)
		}
		if _, statErr := os.Stat(path); statErr != nil {
			t.Fatalf("stat %s artifact %q: %v", label, path, statErr)
		}
	}
}

type orderedStartupOutput struct {
	bytes.Buffer
	events      *[]string
	homeSeen    chan struct{}
	startupSeen chan struct{}
	homeOnce    sync.Once
	startupOnce sync.Once
}

func (output *orderedStartupOutput) Write(data []byte) (int, error) {
	text := string(data)
	if strings.Contains(text, "Home directory:") {
		*output.events = append(*output.events, "home")
		if output.homeSeen != nil {
			output.homeOnce.Do(func() { close(output.homeSeen) })
		}
	}
	if strings.Contains(text, "Factory initiated:") && output.startupSeen != nil {
		output.startupOnce.Do(func() { close(output.startupSeen) })
	}
	return output.Buffer.Write(data)
}

// TestServerDisclosesHomeBeforeSystemInitializationAccess exercises the
// ordinary server command through the real process graph, including listener
// readiness and the system-initialization filesystem edge.
func TestServerDisclosesHomeBeforeSystemInitializationAccess(t *testing.T) {
	workingDirectory := t.TempDir()
	factoryDir := filepath.Join(workingDirectory, "factory")
	if err := os.MkdirAll(factoryDir, 0o755); err != nil {
		t.Fatalf("create Factory directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(factoryDir, "factory.json"), []byte(idleCurrentFactoryJSON), 0o600); err != nil {
		t.Fatalf("write Factory config: %v", err)
	}

	homeDir := t.TempDir()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	events := make([]string, 0, 3)
	startupSeen := make(chan struct{})
	stdout := &orderedStartupOutput{events: &events, startupSeen: startupSeen}
	var stderr bytes.Buffer
	// Isolated: this combines HOME/startup ordering with a listener bind, so its
	// readiness and cancellation witness must remain invocation-owned.
	process, err := support.BuildProcessWithContext(t.Context(), serviceedges.Edges{
		SystemInitializationInspectPath: func(path string) (fs.FileInfo, error) {
			events = append(events, "initialize:"+path)
			return nil, fs.ErrNotExist
		},
		APIServerStarter: func(serverCtx context.Context, request platformhttpserver.StartRequest) error {
			events = append(events, "bind")
			request.OnBound(platformhttpserver.Binding{Port: request.Port})
			select {
			case <-startupSeen:
				cancel()
			case <-time.After(5 * time.Second):
				return errors.New("system initialization did not reach the server startup boundary")
			}
			return serverCtx.Err()
		},
	})
	if err != nil {
		t.Fatalf("BuildProcess() error = %v", err)
	}
	support.CleanupProcess(t, process)
	stdinIsTTY := true
	stdoutIsTTY := false
	err = process.Execute(root.Input{
		Args:             []string{"you", "server"},
		Env:              append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir),
		Stdin:            strings.NewReader(""),
		Stdout:           stdout,
		Stderr:           &stderr,
		Context:          ctx,
		WorkingDirectory: workingDirectory,
		StdinIsTTY:       &stdinIsTTY,
		StdoutIsTTY:      &stdoutIsTTY,
	})
	if err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("Process.Execute(server) error = %v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
	}
	homeIndex := -1
	initializationIndex := -1
	for index, event := range events {
		if event == "home" && homeIndex < 0 {
			homeIndex = index
		}
		if strings.HasPrefix(event, "initialize:") && initializationIndex < 0 {
			initializationIndex = index
		}
	}
	if homeIndex < 0 || initializationIndex < 0 || homeIndex > initializationIndex {
		t.Fatalf("startup events = %#v, want home followed by initialization access", events)
	}
	if got, want := stdout.String(), "Home directory: "+homeDir+"\n"; !strings.HasPrefix(got, want) {
		t.Fatalf("stdout = %q, want prefix %q", got, want)
	}
}

// TestCurrentFactoryRunScopedServerStopsAtIdleAndSiteOpensAfterReadiness proves one-shot hosting and site readiness.
func TestCurrentFactoryRunScopedServerStopsAtIdleAndSiteOpensAfterReadiness(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		flag        string
		wantBrowser int32
	}{
		{name: "server", flag: "--with-server"},
		{name: "site", flag: "--with-site", wantBrowser: 1},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			workingDirectory := t.TempDir()
			factoryDir := filepath.Join(workingDirectory, "factory")
			if err := os.MkdirAll(factoryDir, 0o755); err != nil {
				t.Fatalf("create Current Factory directory: %v", err)
			}
			if err := os.WriteFile(
				filepath.Join(factoryDir, "factory.json"),
				[]byte(idleCurrentFactoryJSON),
				0o600,
			); err != nil {
				t.Fatalf("write Current Factory: %v", err)
			}

			serverStopped := make(chan struct{})
			var bound, browserCalls atomic.Int32
			// Isolated per mode: the assertion covers one-shot listener lifetime,
			// readiness, and browser behavior for this invocation.
			process, err := support.BuildProcessWithContext(t.Context(), serviceedges.Edges{
				APIServerStarter: func(ctx context.Context, request platformhttpserver.StartRequest) error {
					bound.Store(1)
					request.OnBound(platformhttpserver.Binding{Port: request.Port})
					<-ctx.Done()
					close(serverStopped)
					return ctx.Err()
				},
				BrowserOpener: func(context.Context, string) error {
					if bound.Load() == 0 {
						return errors.New("browser opened before listener readiness")
					}
					browserCalls.Add(1)
					return nil
				},
			})
			if err != nil {
				t.Fatalf("BuildProcess() error = %v", err)
			}
			support.CleanupProcess(t, process)

			stdout, stderr, executeErr := executeFactoryArgs(
				t,
				process,
				workingDirectory,
				[]string{"you", "run", "--no-record", test.flag},
				false,
				t.Context(),
			)
			if executeErr != nil {
				t.Fatalf("Process.Execute(%s) error = %v; stdout=%q stderr=%q", test.flag, executeErr, stdout, stderr)
			}
			if stderr != "" {
				t.Fatalf("%s stderr = %q, want empty", test.flag, stderr)
			}
			if browserCalls.Load() != test.wantBrowser {
				t.Fatalf("%s browser calls = %d, want %d", test.flag, browserCalls.Load(), test.wantBrowser)
			}
			select {
			case <-serverStopped:
			case <-time.After(5 * time.Second):
				t.Fatalf("%s did not join its owned listener after idle", test.flag)
			}
		})
	}
}

// TestContinuousRunScopedServerKeepsListenerUntilCancellation proves continuous hosting follows invocation cancellation.
func TestContinuousRunScopedServerKeepsListenerUntilCancellation(t *testing.T) {
	t.Parallel()

	workingDirectory := t.TempDir()
	factoryDir := filepath.Join(workingDirectory, "factory")
	if err := os.MkdirAll(factoryDir, 0o755); err != nil {
		t.Fatalf("create Current Factory directory: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(factoryDir, "factory.json"),
		[]byte(idleCurrentFactoryJSON),
		0o600,
	); err != nil {
		t.Fatalf("write Current Factory: %v", err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	serverStopped := make(chan struct{})
	// Isolated: continuous hosting must retain this invocation's listener until
	// its cancellation and prove that listener shutdown joins deterministically.
	process, err := support.BuildProcessWithContext(t.Context(), serviceedges.Edges{
		APIServerStarter: func(serverCtx context.Context, request platformhttpserver.StartRequest) error {
			request.OnBound(platformhttpserver.Binding{Port: request.Port})
			cancel()
			<-serverCtx.Done()
			close(serverStopped)
			return serverCtx.Err()
		},
		BrowserOpener: func(context.Context, string) error {
			return errors.New("--with-server must not open a browser")
		},
	})
	if err != nil {
		t.Fatalf("BuildProcess() error = %v", err)
	}
	support.CleanupProcess(t, process)
	stdout, stderr, executeErr := executeFactoryArgs(
		t,
		process,
		workingDirectory,
		[]string{"you", "run", "--no-record", "--continuously", "--with-server"},
		false,
		ctx,
	)
	if executeErr != nil && !errors.Is(executeErr, context.Canceled) {
		t.Fatalf("continuous run error = %v; stdout=%q stderr=%q", executeErr, stdout, stderr)
	}
	if stderr != "" {
		t.Fatalf("continuous run stderr = %q, want empty", stderr)
	}
	select {
	case <-serverStopped:
	case <-time.After(5 * time.Second):
		t.Fatal("continuous cancellation did not join the owned listener")
	}
}

func executeFactoryCommand(
	t *testing.T,
	process interface{ Execute(root.Input) error },
	workingDirectory string,
	command string,
	stdoutIsTTY bool,
) (string, string, error) {
	return executeFactoryCommandWithContext(t, process, workingDirectory, command, stdoutIsTTY, t.Context())
}

func executeFactoryCommandWithContext(
	t *testing.T,
	process interface{ Execute(root.Input) error },
	workingDirectory string,
	command string,
	stdoutIsTTY bool,
	ctx context.Context,
) (string, string, error) {
	t.Helper()
	args := []string{"you", command}
	if command == "run" {
		args = append(args, "--no-record")
	}
	return executeFactoryArgs(t, process, workingDirectory, args, stdoutIsTTY, ctx)
}

func executeFactoryArgs(
	t *testing.T,
	process interface{ Execute(root.Input) error },
	workingDirectory string,
	args []string,
	stdoutIsTTY bool,
	ctx context.Context,
) (string, string, error) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	stdinIsTTY := true
	home := t.TempDir()
	err := process.Execute(root.Input{
		Args:             args,
		Env:              append(os.Environ(), "HOME="+home, "USERPROFILE="+home),
		Stdin:            strings.NewReader(""),
		Stdout:           &stdout,
		Stderr:           &stderr,
		Context:          ctx,
		WorkingDirectory: workingDirectory,
		StdinIsTTY:       &stdinIsTTY,
		StdoutIsTTY:      &stdoutIsTTY,
	})
	return stdout.String(), stderr.String(), err
}

const idleCurrentFactoryJSON = `{
  "name": "current",
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

type countingProvider struct {
	testutil.NativeProvider
	calls *atomic.Int32
}

func newCountingProvider(calls *atomic.Int32) countingProvider {
	provider := countingProvider{calls: calls}
	provider.NativeProvider.ExecuteFunc = provider.Execute
	return provider
}

func (provider countingProvider) Execute(context.Context, providers.ExecuteRequest) (providers.ExecuteResult, error) {
	provider.calls.Add(1)
	return providers.ExecuteResult{}, nil
}
