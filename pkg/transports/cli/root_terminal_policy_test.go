package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	workcli "github.com/portpowered/infinite-you/pkg/services/work/transports/cli/work"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	workersessionscli "github.com/portpowered/infinite-you/pkg/services/worker_sessions/transports/cli/worker_sessions"
	runcli "github.com/portpowered/infinite-you/pkg/transports/cli/run"
	submitcli "github.com/portpowered/infinite-you/pkg/transports/cli/submit"
	"github.com/portpowered/infinite-you/pkg/transports/cli/terminalpolicy"
)

const (
	terminalPolicySecretPrompt = "SECRET_PROMPT_BODY_do-not-emit-712407"
	terminalPolicySecretToken  = "ghp_secretToken712407932abcdef"
)

func TestRootCommand_ResolvesTerminalPolicyForVerboseSubmit(t *testing.T) {
	originalSubmit := submitWork
	defer func() {
		submitWork = originalSubmit
	}()

	var got submitcli.SubmitConfig
	submitWork = func(cfg submitcli.SubmitConfig) error {
		got = cfg
		return nil
	}

	payloadPath := filepath.Join(t.TempDir(), "payload.md")
	if err := os.WriteFile(payloadPath, []byte("payload"), 0o600); err != nil {
		t.Fatalf("write payload: %v", err)
	}

	root := newLegacyTestRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{
		"--verbose",
		"submit",
		"--name", "policy-test",
		"--work-type-name", "task",
		"--payload", payloadPath,
	})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute submit --verbose: %v", err)
	}
	if !got.Verbose {
		t.Fatal("expected verbose submit config from resolved terminal policy")
	}
	if got.Diagnostics == nil {
		t.Fatal("expected diagnostics writer when verbose policy is resolved")
	}
}

func TestRootCommand_ResolvesQuietRunPolicyForDiagnosticsAndLogger(t *testing.T) {
	originalRunCLI := runCLI
	defer func() {
		runCLI = originalRunCLI
	}()

	var got runcli.RunConfig
	runCLI = func(_ context.Context, cfg runcli.RunConfig) error {
		got = cfg
		return nil
	}

	dir := t.TempDir()
	root := newLegacyTestRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{
		"run",
		"--dir", dir,
		"--no-record",
		"--quiet",
	})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute run --quiet: %v", err)
	}
	if got.TerminalPolicy.Mode() != terminalpolicy.ModeQuiet {
		t.Fatalf("terminal policy mode = %q, want %q", got.TerminalPolicy.Mode(), terminalpolicy.ModeQuiet)
	}
	if got.StartupOutput != nil {
		t.Fatal("expected quiet run policy to suppress startup output wiring")
	}
	if got.Diagnostics != nil {
		t.Fatal("expected quiet run policy to suppress diagnostics writer")
	}
	if got.ProgressOutput != nil || got.ProgressIsTTY {
		t.Fatalf("quiet progress channel = (%T, %t), want (nil, false)", got.ProgressOutput, got.ProgressIsTTY)
	}
	if got.Verbose {
		t.Fatal("expected quiet run policy to disable verbose runtime logging")
	}
}

func TestRootCommand_QuietRunOperationalFailureSuppressesTerminalOutput(t *testing.T) {
	originalRunCLI := runCLI
	defer func() {
		runCLI = originalRunCLI
	}()

	runCLI = func(_ context.Context, _ runcli.RunConfig) error {
		return fmt.Errorf("quiet operational failure baseline")
	}

	dir := t.TempDir()
	var stdout, stderr bytes.Buffer
	root := newLegacyTestRootCommand()
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{
		"run",
		"--dir", dir,
		"--no-record",
		"--quiet",
	})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected operational failure")
	}
	if !strings.Contains(err.Error(), "quiet operational failure baseline") {
		t.Fatalf("error = %q, want failure returned to caller", err.Error())
	}
	if stdout.String() != "" {
		t.Fatalf("stdout = %q, want empty quiet failure terminal output", stdout.String())
	}
	if stderr.String() != "" {
		t.Fatalf("stderr = %q, want empty quiet failure terminal output", stderr.String())
	}
}

func TestRootCommand_ResolvesVerboseRunPolicyForDiagnostics(t *testing.T) {
	originalRunCLI := runCLI
	defer func() {
		runCLI = originalRunCLI
	}()

	var got runcli.RunConfig
	runCLI = func(_ context.Context, cfg runcli.RunConfig) error {
		got = cfg
		return nil
	}

	dir := t.TempDir()
	var stderr bytes.Buffer
	root := newLegacyTestRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(&stderr)
	root.SetArgs([]string{
		"run",
		"--dir", dir,
		"--no-record",
		"--verbose",
	})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute run --verbose: %v", err)
	}
	if got.TerminalPolicy.Mode() != terminalpolicy.ModeVerbose {
		t.Fatalf("terminal policy mode = %q, want %q", got.TerminalPolicy.Mode(), terminalpolicy.ModeVerbose)
	}
	if got.Diagnostics == nil {
		t.Fatal("expected verbose run policy to wire diagnostics writer")
	}
	if !got.Verbose {
		t.Fatal("expected verbose run policy to enable runtime verbose logging")
	}
}

func TestRootCommand_NormalModeSuppressesSubmitDiagnostics(t *testing.T) {
	originalSubmit := submitWork
	defer func() {
		submitWork = originalSubmit
	}()

	var got submitcli.SubmitConfig
	submitWork = func(cfg submitcli.SubmitConfig) error {
		got = cfg
		return nil
	}

	payloadPath := filepath.Join(t.TempDir(), "payload.md")
	if err := os.WriteFile(payloadPath, []byte("payload"), 0o600); err != nil {
		t.Fatalf("write payload: %v", err)
	}

	root := newLegacyTestRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{
		"submit",
		"--name", "policy-test",
		"--work-type-name", "task",
		"--payload", payloadPath,
	})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute submit: %v", err)
	}
	if got.Verbose {
		t.Fatal("expected normal mode to keep verbose disabled")
	}
	if got.Diagnostics != nil {
		t.Fatal("expected normal mode to suppress diagnostics writer")
	}
}

func TestRootCommand_NormalModeRunWiresTerminalMutedLogger(t *testing.T) {
	originalRunCLI := runCLI
	defer func() {
		runCLI = originalRunCLI
	}()

	var got runcli.RunConfig
	runCLI = func(_ context.Context, cfg runcli.RunConfig) error {
		got = cfg

		oldStderr := os.Stderr
		readPipe, writePipe, err := os.Pipe()
		if err != nil {
			t.Fatalf("pipe stderr: %v", err)
		}
		os.Stderr = writePipe
		got.Logger.Warn("normal mode structured leak probe")
		if err := writePipe.Close(); err != nil {
			t.Fatalf("close stderr writer: %v", err)
		}
		os.Stderr = oldStderr

		captured, err := io.ReadAll(readPipe)
		if err != nil {
			t.Fatalf("read captured stderr: %v", err)
		}
		if len(captured) != 0 {
			t.Fatalf("stderr = %q, want no structured terminal output for normal run logger", captured)
		}
		return nil
	}

	dir := t.TempDir()
	root := newLegacyTestRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{
		"run",
		"--dir", dir,
		"--no-record",
	})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute run: %v", err)
	}
	if got.TerminalPolicy.Mode() != terminalpolicy.ModeNormal {
		t.Fatalf("terminal policy mode = %q, want %q", got.TerminalPolicy.Mode(), terminalpolicy.ModeNormal)
	}
	if got.StartupOutput == nil {
		t.Fatal("expected normal run policy to wire human startup output")
	}
	if got.Diagnostics != nil {
		t.Fatal("expected normal run policy to suppress diagnostics writer")
	}
	if got.ProgressOutput == nil {
		t.Fatal("expected normal run policy to wire the stderr progress channel")
	}
}

func TestRootCommand_TerminalPolicyNeverLeaksPromptOrSecretsAcrossModes(t *testing.T) {
	t.Run("quiet operational failure", func(t *testing.T) {
		assertTerminalPolicySecretLeakContract(t, []string{
			"run",
			"--factory", writeInvalidGoalFactory(t),
			"--no-record",
			"--quiet",
			terminalPolicySecretPrompt,
		})
	})

	t.Run("normal operational failure", func(t *testing.T) {
		assertTerminalPolicySecretLeakContract(t, []string{
			"run",
			"--factory", writeInvalidGoalFactory(t),
			"--no-record",
			terminalPolicySecretPrompt,
		})
	})

	t.Run("verbose operational failure", func(t *testing.T) {
		assertTerminalPolicySecretLeakContract(t, []string{
			"run",
			"--factory", writeInvalidGoalFactory(t),
			"--no-record",
			"--verbose",
			terminalPolicySecretPrompt,
		})
	})
}

func TestRootCommand_SubmitDiagnosticsNeverLeakPromptOrSecretsAcrossModes(t *testing.T) {
	modes := []struct {
		name string
		args []string
	}{
		{name: "normal", args: nil},
		{name: "verbose", args: []string{"--verbose"}},
	}

	for _, mode := range modes {
		t.Run(mode.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusCreated)
				_, _ = w.Write([]byte(`{"traceId":"trace-terminal-policy"}`))
			}))
			defer srv.Close()

			payloadPath := filepath.Join(t.TempDir(), "secret-payload.md")
			if err := os.WriteFile(payloadPath, []byte("# "+terminalPolicySecretPrompt+"\n\n"+terminalPolicySecretToken), 0o644); err != nil {
				t.Fatal(err)
			}

			originalSubmit := submitWork
			defer func() {
				submitWork = originalSubmit
			}()

			submitWork = func(submitcli.SubmitConfig) error {
				return nil
			}

			var stdout, stderr bytes.Buffer
			root := newLegacyTestRootCommand()
			root.SetOut(&stdout)
			root.SetErr(&stderr)
			args := append([]string{}, mode.args...)
			args = append(args,
				"submit",
				"--name", "terminal-policy-secret-test",
				"--work-type-name", "task",
				"--payload", payloadPath,
				"--server", srv.URL,
			)
			root.SetArgs(args)

			if err := root.Execute(); err != nil {
				t.Fatalf("execute submit: %v", err)
			}

			assertNoTerminalPolicySecrets(t, stdout.String()+stderr.String())
		})
	}
}

func writeInvalidGoalFactory(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	factoryPath := filepath.Join(dir, "factory.json")
	if err := os.WriteFile(factoryPath, []byte(goalFailureBaselineInvalidTopologyJSON), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return factoryPath
}

func assertTerminalPolicySecretLeakContract(t *testing.T, args []string) {
	t.Helper()

	var stdout, stderr bytes.Buffer
	root := newComposedTestRootCommand(t)
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs(args)

	err := root.Execute()
	if err == nil {
		t.Fatal("expected operational failure")
	}
	if !strings.Contains(err.Error(), "invalid graph references") {
		t.Fatalf("error = %q, want invalid topology failure", err.Error())
	}

	assertNoTerminalPolicySecrets(t, stdout.String()+stderr.String())
}

func assertNoTerminalPolicySecrets(t *testing.T, capture string) {
	t.Helper()

	for _, forbidden := range []string{
		terminalPolicySecretPrompt,
		terminalPolicySecretToken,
	} {
		if strings.Contains(capture, forbidden) {
			t.Fatalf("terminal or diagnostics capture leaked %q:\n%s", forbidden, capture)
		}
	}
}

// Work-family production cutover tests (merged from root_work_test.go for pkg-file-count/backend-size).
func TestProductionWorkCommandUsesGeneratedFamily(t *testing.T) {
	work := productionWorkCommand(&cliGlobalOptions{}, &cliDiagnosticsOptions{})
	if work == nil {
		t.Fatal("productionWorkCommand() = nil, want work command")
	}
	if work.RunE != nil {
		t.Fatal("generated work parent must remain non-runnable")
	}
	approval, _, err := work.Find([]string{"approval"})
	if err != nil {
		t.Fatalf("Find(approval) error = %v", err)
	}
	if approval.RunE != nil {
		t.Fatal("generated work approval parent must remain non-runnable")
	}
	for _, path := range []string{"list", "watch", "show", "move", "visualize"} {
		if _, _, err := work.Find([]string{path}); err != nil {
			t.Fatalf("generated work tree missing %q: %v", path, err)
		}
	}
}

func TestProductionWorkCommandAttachesHandwrittenRunE(t *testing.T) {
	work := productionWorkCommand(&cliGlobalOptions{}, &cliDiagnosticsOptions{})
	list, _, err := work.Find([]string{"list"})
	if err != nil {
		t.Fatalf("Find(list) error = %v", err)
	}
	if list.RunE == nil {
		t.Fatal("generated work list must attach handwritten RunE")
	}
	watch, _, err := work.Find([]string{"watch"})
	if err != nil {
		t.Fatalf("Find(watch) error = %v", err)
	}
	if watch.RunE == nil {
		t.Fatal("generated work watch must attach handwritten RunE")
	}
	show, _, err := work.Find([]string{"show"})
	if err != nil {
		t.Fatalf("Find(show) error = %v", err)
	}
	if show.RunE == nil {
		t.Fatal("generated work show must attach handwritten RunE")
	}
	move, _, err := work.Find([]string{"move"})
	if err != nil {
		t.Fatalf("Find(move) error = %v", err)
	}
	if move.RunE == nil {
		t.Fatal("generated work move must attach handwritten RunE")
	}
	visualize, _, err := work.Find([]string{"visualize"})
	if err != nil {
		t.Fatalf("Find(visualize) error = %v", err)
	}
	if visualize.RunE == nil {
		t.Fatal("generated work visualize must attach handwritten RunE")
	}
}

func TestProductionRootUsesGeneratedWorkFamilyCutover(t *testing.T) {
	root := newLegacyTestRootCommand()
	work, _, err := root.Find([]string{"work"})
	if err != nil {
		t.Fatalf("Find(work) error = %v", err)
	}
	if work.RunE != nil {
		t.Fatal("you work must remain non-runnable through generated cutover")
	}
	approval, _, err := work.Find([]string{"approval"})
	if err != nil {
		t.Fatalf("Find(work approval) error = %v", err)
	}
	if approval.RunE != nil {
		t.Fatal("you work approval must remain non-runnable through generated cutover")
	}
	for _, path := range []string{"list", "watch", "show", "move", "visualize"} {
		leaf, _, err := root.Find([]string{"work", path})
		if err != nil {
			t.Fatalf("Find(work %s) error = %v", path, err)
		}
		if leaf.RunE == nil {
			t.Fatalf("you work %s must attach handwritten RunE through generated cutover", path)
		}
	}
}
func TestProductionWorkResolvedHandlerExecutesWatch(t *testing.T) {
	var got workcli.WatchConfig
	root := withTestInjectedPlatformRoles(CommandFactory{
		WatchWork: func(cfg workcli.WatchConfig) error {
			got = cfg
			_, err := io.WriteString(cfg.Output, "watched\n")
			return err
		},
	}).NewCommand(nil, nil, nil)

	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"--server", "https://factory.example", "--verbose", "--debug", "work", "watch", "--session", "session-alpha", "--follow"})
	if err := root.Execute(); err != nil {
		t.Fatalf("work watch Execute() error = %v", err)
	}

	if got.Context == nil || got.Server != "https://factory.example" || got.SessionID != "session-alpha" ||
		!got.SessionIDExplicit || !got.Follow || !got.Verbose || !got.Debug || got.Output != &stdout || got.Diagnostics != &stderr {
		t.Fatalf("watch config = %#v, want production stable-input mapping", got)
	}
	if stdout.String() != "watched\n" || stderr.Len() != 0 {
		t.Fatalf("watch output = %q, diagnostics = %q", stdout.String(), stderr.String())
	}
}

func TestWorkerSessionsListCommandMapsManifestInputsToOperation(t *testing.T) {
	var got workersessionscli.ListConfig
	factory := withTestInjectedPlatformRoles(CommandFactory{
		ListWorkerSessions: func(config workersessionscli.ListConfig) error {
			got = config
			return nil
		},
	})
	root := factory.NewCommand(nil, nil, nil)
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{
		"--json", "--server", "http://factory.test:7437",
		"worker-sessions", "list", "--work-id", "work-1",
		"--session", "session-1", "--output", "json",
	})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute worker-sessions list: %v", err)
	}
	if got.WorkID == "" {
		t.Fatal("worker sessions list operation was not invoked")
	}
	if got.WorkID != "work-1" || got.SessionID != "session-1" || got.Server != "http://factory.test:7437" {
		t.Fatalf("operation config = %#v, want manifest values", got)
	}
	if got.OutputFormat != "json" || !got.JSON {
		t.Fatalf("output config = %#v, want json output", got)
	}
}

func TestWorkerSessionsInvokeCommandMapsManifestInputsToOperation(t *testing.T) {
	var got workersessionscli.InvokeConfig
	factory := withTestInjectedPlatformRoles(CommandFactory{
		InvokeWorkerSession: func(config workersessionscli.InvokeConfig) error {
			got = config
			return nil
		},
	})
	root := factory.NewCommand(nil, nil, nil)
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{
		"--json", "--server", "http://factory.test:7437",
		"worker-sessions", "invoke", "--request-id", "request-1", "--worker-session-id", "session-1",
		"--dispatch-id", "dispatch-1", "--workstation", "coding", "--provider", "codex",
		"--model", "model-1", "--user-message", "hello", "--async", "--output", "json", "follow-up", "now",
	})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute worker-sessions invoke: %v", err)
	}
	assertWorkerSessionsInvokeConfig(t, got)
}

func TestWorkerSessionsContinueCommandMapsManifestInputsToOperation(t *testing.T) {
	var got workersessionscli.ContinueConfig
	factory := withTestInjectedPlatformRoles(CommandFactory{
		ContinueWorkerSession: func(config workersessionscli.ContinueConfig) error {
			got = config
			return nil
		},
	})
	root := factory.NewCommand(nil, nil, nil)
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{
		"--json", "--server", "http://factory.test:7437",
		"worker-sessions", "continue", "source-1", "--request-id", "request-1",
		"--successor-worker-session-id", "successor-1", "--user-message", "hello",
		"--async", "--output", "json", "follow", "up",
	})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute worker-sessions continue: %v", err)
	}
	if got.SourceWorkerSessionID != "source-1" || got.RequestID != "request-1" ||
		got.SuccessorWorkerSessionID != "successor-1" || got.FollowUpInput != "hello" ||
		got.Server != "http://factory.test:7437" || got.Remote || !got.Async || got.OutputFormat != "json" || !got.JSON {
		t.Fatalf("continue config = %#v, want manifest values", got)
	}
	if len(got.Prompt) != 2 || got.Prompt[0] != "follow" || got.Prompt[1] != "up" {
		t.Fatalf("continue prompt = %#v, want positional follow-up input", got.Prompt)
	}
}

func TestWorkerSessionsInterruptCommandMapsManifestInputs(t *testing.T) {
	var interrupt workersessionscli.InterruptConfig
	factory := withTestInjectedPlatformRoles(CommandFactory{
		InterruptWorkerSession: func(config workersessionscli.InterruptConfig) error {
			interrupt = config
			return nil
		},
	})
	root := factory.NewCommand(nil, nil, nil)
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{
		"--json", "--server", "http://factory.test:7437",
		"worker-sessions", "interrupt", "source-1", "--request-id", "request-1",
		"--successor-worker-session-id", "successor-1", "--replacement-message", "hello",
		"--async", "--output", "json", "replace", "input",
	})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute worker-sessions interrupt: %v", err)
	}
	if interrupt.SourceWorkerSessionID != "source-1" || interrupt.RequestID != "request-1" ||
		interrupt.SuccessorWorkerSessionID != "successor-1" || interrupt.ReplacementMessage != "hello" ||
		interrupt.Server != "http://factory.test:7437" || interrupt.Remote || !interrupt.Async ||
		interrupt.OutputFormat != "json" || !interrupt.JSON || len(interrupt.Prompt) != 2 {
		t.Fatalf("interrupt config = %#v, want manifest values", interrupt)
	}
}

func TestWorkerSessionsControlCommandsMapManifestInputs(t *testing.T) {
	for _, action := range []workersessions.ControlAction{
		workersessions.ControlActionPause, workersessions.ControlActionResume,
		workersessions.ControlActionCancel, workersessions.ControlActionTerminate,
	} {
		t.Run(strings.ToLower(string(action)), func(t *testing.T) {
			var control workersessionscli.ControlConfig
			operations := CommandFactory{}
			operation := func(config workersessionscli.ControlConfig) error {
				control = config
				return nil
			}
			switch action {
			case workersessions.ControlActionPause:
				operations.PauseWorkerSession = operation
			case workersessions.ControlActionResume:
				operations.ResumeWorkerSession = operation
			case workersessions.ControlActionCancel:
				operations.CancelWorkerSession = operation
			case workersessions.ControlActionTerminate:
				operations.TerminateWorkerSession = operation
			}
			root := withTestInjectedPlatformRoles(operations).NewCommand(nil, nil, nil)
			root.SetOut(io.Discard)
			root.SetErr(io.Discard)
			root.SetArgs([]string{
				"--json", "--server", "http://factory.test:7437", "worker-sessions",
				strings.ToLower(string(action)), "session-1", "--output", "json",
			})
			if err := root.Execute(); err != nil {
				t.Fatalf("execute worker-sessions %s: %v", action, err)
			}
			if control.WorkerSessionID != "session-1" || control.Action != action ||
				control.Server != "http://factory.test:7437" || control.Remote ||
				control.OutputFormat != "json" || !control.JSON {
				t.Fatalf("%s config = %#v, want manifest values", action, control)
			}
		})
	}
}

func TestWorkerSessionsInterruptAndControlCommandsRequireOperations(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "interrupt",
			args: []string{"worker-sessions", "interrupt", "source-1", "--request-id", "request-1", "--successor-worker-session-id", "successor-1", "--replacement-message", "replacement"},
			want: "worker sessions interrupt service is required",
		},
		{
			name: "pause",
			args: []string{"worker-sessions", "pause", "session-1"},
			want: "worker sessions pause service is required",
		},
		{
			name: "resume",
			args: []string{"worker-sessions", "resume", "session-1"},
			want: "worker sessions resume service is required",
		},
		{
			name: "cancel",
			args: []string{"worker-sessions", "cancel", "session-1"},
			want: "worker sessions cancel service is required",
		},
		{
			name: "terminate",
			args: []string{"worker-sessions", "terminate", "session-1"},
			want: "worker sessions terminate service is required",
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			root := withTestInjectedPlatformRoles(CommandFactory{}).NewCommand(nil, nil, nil)
			root.SetOut(io.Discard)
			root.SetErr(io.Discard)
			root.SetArgs(test.args)
			err := root.Execute()
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("execute %s error = %v, want %q", test.name, err, test.want)
			}
		})
	}
}

func TestWorkerSessionsInterruptInputReaderReportsTypedInputErrors(t *testing.T) {
	values := map[string]any{
		"you.worker-sessions.interrupt.arg.0":                            "source-1",
		"you.worker-sessions.interrupt.flag.request-id":                  "request-1",
		"you.worker-sessions.interrupt.flag.successor-worker-session-id": "successor-1",
		"you.worker-sessions.interrupt.flag.replacement-message":         "replacement",
		"you.worker-sessions.interrupt.flag.output":                      "json",
		"you.worker-sessions.interrupt.arg.1":                            []string{"follow-up"},
		"you.worker-sessions.interrupt.flag.async":                       true,
	}
	for _, key := range []string{
		"you.worker-sessions.interrupt.arg.0",
		"you.worker-sessions.interrupt.flag.request-id",
		"you.worker-sessions.interrupt.flag.successor-worker-session-id",
		"you.worker-sessions.interrupt.flag.replacement-message",
		"you.worker-sessions.interrupt.flag.output",
		"you.worker-sessions.interrupt.flag.async",
	} {
		candidate := make(map[string]any, len(values))
		for id, value := range values {
			candidate[id] = value
		}
		delete(candidate, key)
		if _, err := readGeneratedWorkerSessionsInterruptInputs(candidate); err == nil {
			t.Errorf("readGeneratedWorkerSessionsInterruptInputs(missing %s) = nil error, want typed input error", key)
		}
	}
	delete(values, "you.worker-sessions.interrupt.arg.1")
	if got, err := readGeneratedWorkerSessionsInterruptInputs(values); err != nil {
		t.Fatalf("readGeneratedWorkerSessionsInterruptInputs(missing optional prompt) error = %v", err)
	} else if len(got.replacementInput) != 0 {
		t.Fatalf("optional interrupt prompt = %#v, want empty", got.replacementInput)
	}
}

func assertWorkerSessionsInvokeConfig(t *testing.T, got workersessionscli.InvokeConfig) {
	t.Helper()
	checks := map[string]bool{
		"local placement":   !got.Remote,
		"server":            got.Server == "http://factory.test:7437",
		"request ID":        got.RequestID == "request-1",
		"Worker Session ID": got.WorkerSessionID == "session-1",
		"dispatch ID":       got.DispatchID == "dispatch-1",
		"workstation":       got.WorkstationName == "coding",
		"provider":          got.Provider == "codex",
		"model":             got.Model == "model-1",
		"user message":      got.UserMessage == "hello",
		"async":             got.Async,
		"output format":     got.OutputFormat == "json",
		"JSON output":       got.JSON,
	}
	for name, ok := range checks {
		if !ok {
			t.Errorf("invoke config %s is incorrect: %#v", name, got)
		}
	}
	wantPrompt := []string{"follow-up", "now"}
	if len(got.Prompt) != len(wantPrompt) {
		t.Fatalf("invoke prompt = %#v, want positional prompt %#v", got.Prompt, wantPrompt)
	}
	for index, want := range wantPrompt {
		if got.Prompt[index] != want {
			t.Errorf("invoke prompt[%d] = %q, want %q", index, got.Prompt[index], want)
		}
	}
}

func TestWorkerSessionsShowCommandMapsManifestInputsToOperation(t *testing.T) {
	var got workersessionscli.ShowConfig
	factory := withTestInjectedPlatformRoles(CommandFactory{
		ShowWorkerSession: func(config workersessionscli.ShowConfig) error {
			got = config
			return nil
		},
	})
	root := factory.NewCommand(nil, nil, nil)
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{
		"--json", "--server", "http://factory.test:7437",
		"worker-sessions", "show", "--provider", "codex", "--kind", "session_id", "--id", "provider-session-1",
		"--session", "session-1", "--output", "json",
	})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute worker-sessions show: %v", err)
	}
	if got.Provider != "codex" || got.Kind != "session_id" || got.ID != "provider-session-1" || got.SessionID != "session-1" {
		t.Fatalf("operation config = %#v, want manifest identity values", got)
	}
	if got.Server != "http://factory.test:7437" || got.OutputFormat != "json" || !got.JSON {
		t.Fatalf("output/config = %#v, want server and json values", got)
	}
}

func TestWorkerSessionsStreamCommandMapsManifestInputsToOperation(t *testing.T) {
	var got workersessionscli.StreamConfig
	factory := withTestInjectedPlatformRoles(CommandFactory{
		StreamWorkerSession: func(config workersessionscli.StreamConfig) error {
			got = config
			return nil
		},
	})
	root := factory.NewCommand(nil, nil, nil)
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{
		"--json", "--server", "http://factory.test:7437",
		"worker-sessions", "stream", "--provider", "codex", "--kind", "session_id", "--id", "provider-session-1",
		"--session", "session-1", "--output", "json", "--follow",
	})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute worker-sessions stream: %v", err)
	}
	if got.Provider != "codex" || got.Kind != "session_id" || got.ID != "provider-session-1" || got.SessionID != "session-1" {
		t.Fatalf("operation config = %#v, want manifest identity values", got)
	}
	if got.Server != "http://factory.test:7437" || got.OutputFormat != "json" || !got.JSON {
		t.Fatalf("output/config = %#v, want server and json values", got)
	}
	if !got.Follow || got.ReplayOnly {
		t.Fatalf("stream mode config = %#v, want explicit live follow only", got)
	}
}

func TestWorkerSessionsStreamRejectsReplayOnlyFollowBeforeOperation(t *testing.T) {
	operationCalls := 0
	factory := withTestInjectedPlatformRoles(CommandFactory{
		StreamWorkerSession: func(workersessionscli.StreamConfig) error {
			operationCalls++
			return nil
		},
	})
	root := factory.NewCommand(nil, nil, nil)
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{
		"--json", "--server", "http://factory.test:7437",
		"worker-sessions", "stream", "--provider", "codex", "--kind", "session_id", "--id", "provider-session-1",
		"--replay-only", "--follow", "--output", "json",
	})

	err := root.Execute()
	var typed *workersessionscli.CLIError
	if !errors.As(err, &typed) || typed.Code != workersessionscli.StreamModeConflictCode {
		t.Fatalf("error = %v, want %s", err, workersessionscli.StreamModeConflictCode)
	}
	if operationCalls != 0 {
		t.Fatalf("conflicting stream invoked operation %d times, want 0", operationCalls)
	}
	if stdout.Len() != 0 {
		t.Fatalf("conflicting stream wrote stdout %q, want empty", stdout.String())
	}
}

func TestWorkerSessionsReadCommandMapsManifestInputsToOperation(t *testing.T) {
	var got workersessionscli.ReadConfig
	factory := withTestInjectedPlatformRoles(CommandFactory{
		ReadWorkerSession: func(config workersessionscli.ReadConfig) error {
			got = config
			return nil
		},
	})
	root := factory.NewCommand(nil, nil, nil)
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{
		"--json", "--server", "http://factory.test:7437",
		"worker-sessions", "read", "--provider", "codex", "--kind", "session_id", "--id", "provider-session-1",
		"--session", "session-1", "--output", "json",
	})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute worker-sessions read: %v", err)
	}
	if got.Provider != "codex" || got.Kind != "session_id" || got.ID != "provider-session-1" || got.SessionID != "session-1" {
		t.Fatalf("operation config = %#v, want manifest identity values", got)
	}
	if got.Server != "http://factory.test:7437" || got.OutputFormat != "json" || !got.JSON {
		t.Fatalf("output/config = %#v, want server and json values", got)
	}
}
