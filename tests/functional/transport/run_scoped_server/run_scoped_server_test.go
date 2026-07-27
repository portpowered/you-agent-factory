package run_scoped_server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

const (
	goalFactoryName        = "@you/goal"
	goalWorkstationName    = "execute-goal"
	wantInvocationResponse = "mock worker accepted"
)

func TestRunScopedServerAndSiteOwnNamedAndFileInvocationLifecycles(t *testing.T) {
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
			homeDir := t.TempDir()
			workingDirectory := t.TempDir()
			var listenerStarts, listenerStops, browserCalls atomic.Int32
			process, err := root.BuildProcess(t.Context(), serviceedges.Edges{
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
			factoryDir := initializeGoalFactory(t, process, environment, workingDirectory, homeDir)
			mockWorkersPath := writeMockWorkersConfig(t)
			selection := []string{"--named", goalFactoryName}
			if test.file {
				selection = []string{"--factory", filepath.Join(factoryDir, "factory.json")}
			}
			mode := "--with-server"
			if test.site {
				mode = "--with-site"
			}
			args := append([]string{"you", "run"}, selection...)
			args = append(args, "--with-mock-workers", mockWorkersPath, "--no-record", mode)
			args = append(args, test.input...)
			stdout, stderr := execute(
				t, process, environment, workingDirectory, args, test.stdin,
			)
			if stderr != "" || stdout != wantInvocationResponse {
				t.Fatalf("invocation stdout=%q stderr=%q", stdout, stderr)
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

func TestRunScopedServerOwnsRawJavaScriptLifecycleAfterReadiness(t *testing.T) {
	for _, test := range []struct {
		name        string
		mode        string
		wantBrowser int32
	}{
		{name: "server", mode: "--with-server"},
		{name: "site", mode: "--with-site", wantBrowser: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			workingDirectory := t.TempDir()
			workflowPath := filepath.Join(workingDirectory, "workflow.js")
			if err := os.WriteFile(workflowPath, []byte(`return "hosted JavaScript";`), 0o600); err != nil {
				t.Fatalf("write workflow: %v", err)
			}
			var listenerStarts, listenerStops, browserCalls atomic.Int32
			process, err := root.BuildProcess(t.Context(), serviceedges.Edges{
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

func TestRunScopedServerOwnsReplayLifecycle(t *testing.T) {
	homeDir := t.TempDir()
	workingDirectory := t.TempDir()
	var listenerStarts, listenerStops, browserCalls atomic.Int32
	process, err := root.BuildProcess(t.Context(), serviceedges.Edges{
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
