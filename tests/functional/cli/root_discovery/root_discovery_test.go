package root_discovery_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	providercontract "github.com/portpowered/infinite-you/pkg/services/providers/inference"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// TestManifestProjectedRepresentativeHandlersAcceptCanonicalInputs proves the
// generated Session and Work leaves reach their typed transport handlers through
// the public process root with the canonical argument and flag shapes.
func TestManifestProjectedRepresentativeHandlersAcceptCanonicalInputs(t *testing.T) {
	process, err := root.BuildProcess(t.Context(), serviceedges.Edges{})
	if err != nil {
		t.Fatalf("BuildProcess() error = %v", err)
	}
	workingDirectory := t.TempDir()
	unavailableServer := "http://127.0.0.1:1"
	cases := [][]string{
		{"you", "--server", unavailableServer, "session", "list", "--json"},
		{"you", "--server", unavailableServer, "session", "show", "session-1", "--json"},
		{"you", "--server", unavailableServer, "session", "delete", "session-1", "--json"},
		{"you", "--server", unavailableServer, "session", "pause", "session-1", "--json"},
		{"you", "--server", unavailableServer, "session", "resume", "session-1", "--json"},
		{"you", "--server", unavailableServer, "session", "dispatches", "session-1", "--json"},
		{"you", "--server", unavailableServer, "work", "list", "--json"},
		{"you", "--server", unavailableServer, "work", "show", "work-1", "--json"},
		{"you", "--server", unavailableServer, "work", "move", "work-1", "complete", "--json"},
	}
	for _, args := range cases {
		_, _, executeErr := executeFactoryArgs(
			t,
			process,
			workingDirectory,
			args,
			false,
			t.Context(),
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
			runCurrentFactoryFailureCase(t, test.prepare, test.wantCode)
		})
	}
}

// TestServerCurrentFactoryFailsBeforeProductActivation proves server validation precedes product activation.
func TestServerCurrentFactoryFailsBeforeProductActivation(t *testing.T) {
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
			runCurrentFactoryFailureCaseForCommand(t, "server", test.prepare, test.wantCode)
		})
	}
}

// TestRunScopedSiteMissingCurrentFactoryFailsBeforeProductActivation proves site selection does not bypass validation.
func TestRunScopedSiteMissingCurrentFactoryFailsBeforeProductActivation(t *testing.T) {
	workingDirectory := t.TempDir()
	var effects atomic.Int32
	process, err := root.BuildProcess(t.Context(), serviceedges.Edges{
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
	stdout, stderr, executeErr := executeFactoryArgs(
		t,
		process,
		workingDirectory,
		[]string{"you", "run", "--no-record", "--with-site"},
		false,
		t.Context(),
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

	var effects atomic.Int32
	process, err := root.BuildProcess(t.Context(), serviceedges.Edges{
		APIServerStarter: func(context.Context, platformhttpserver.StartRequest) error {
			effects.Add(1)
			return nil
		},
		BrowserOpener: func(context.Context, string) error {
			effects.Add(1)
			return nil
		},
		RuntimeHostObserver: func(factorysessions.RuntimeHostBinding) {
			effects.Add(1)
		},
		FactorySessionIDGenerator: func() string {
			effects.Add(1)
			return "unexpected-session"
		},
		ProviderOverride: countingProvider{calls: &effects},
	})
	if err != nil {
		t.Fatalf("BuildProcess() error = %v", err)
	}

	stdout, stderr, executeErr := executeFactoryCommand(t, process, workingDirectory, command, false)
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
	for iteration := 0; iteration < 3; iteration++ {
		t.Run(fmt.Sprintf("iteration-%d", iteration), func(t *testing.T) {
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
	process, err := root.BuildProcess(t.Context(), serviceedges.Edges{
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
		ProviderOverride: countingProvider{calls: &providerCalls},
	})
	if err != nil {
		t.Fatalf("BuildProcess() error = %v", err)
	}

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
	})
	if err != nil {
		t.Fatalf("NewStarter() error = %v", err)
	}
	var browserCalls, readinessCalls atomic.Int32
	process, err := root.BuildProcess(t.Context(), serviceedges.Edges{
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
	var response factoryapi.ErrorResponse
	if err := json.Unmarshal([]byte(stderr), &response); err != nil {
		t.Fatalf("server stderr is not exactly one ErrorResponse: %v\n%s", err, stderr)
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
	if err := os.WriteFile(filepath.Join(factoryDir, ".current-factory"), []byte("alternate\n"), 0o600); err != nil {
		t.Fatalf("write ignored current pointer: %v", err)
	}

	var effects atomic.Int32
	process, err := root.BuildProcess(t.Context(), serviceedges.Edges{
		APIServerStarter: func(context.Context, platformhttpserver.StartRequest) error {
			effects.Add(1)
			return nil
		},
		BrowserOpener: func(context.Context, string) error {
			effects.Add(1)
			return nil
		},
		RuntimeHostObserver: func(factorysessions.RuntimeHostBinding) {
			effects.Add(1)
		},
		ProviderOverride: countingProvider{calls: &effects},
	})
	if err != nil {
		t.Fatalf("BuildProcess() error = %v", err)
	}

	stdout, stderr, executeErr := executeCurrentFactory(t, process, workingDirectory)
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

// TestCurrentFactoryRunScopedServerStopsAtIdleAndSiteOpensAfterReadiness proves one-shot hosting and site readiness.
func TestCurrentFactoryRunScopedServerStopsAtIdleAndSiteOpensAfterReadiness(t *testing.T) {
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
			process, err := root.BuildProcess(t.Context(), serviceedges.Edges{
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
	process, err := root.BuildProcess(t.Context(), serviceedges.Edges{
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

func executeCurrentFactory(
	t *testing.T,
	process interface{ Execute(root.Input) error },
	workingDirectory string,
) (string, string, error) {
	return executeFactoryCommand(t, process, workingDirectory, "run", false)
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
	calls *atomic.Int32
}

var _ providercontract.Provider = countingProvider{}

func (provider countingProvider) Infer(context.Context, workers.ProviderInferenceRequest) (workers.InferenceResponse, error) {
	provider.calls.Add(1)
	return workers.InferenceResponse{}, nil
}
