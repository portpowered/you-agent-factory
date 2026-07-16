package cli

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	fse "github.com/portpowered/infinite-you/pkg/factory/sessions/execution"
	runcli "github.com/portpowered/infinite-you/pkg/transports/cli/run"
	sessionexecutioncli "github.com/portpowered/infinite-you/pkg/transports/cli/sessionexecution"
	submitcli "github.com/portpowered/infinite-you/pkg/transports/cli/submit"
	"github.com/portpowered/infinite-you/pkg/transports/cli/terminalpolicy"
)

func TestWorkflowCommandClosesRuntimeExecutionOwnerExactlyOnce(t *testing.T) {
	service, err := fse.NewFakeServiceFromContractFixtures(contractFixtureCatalogPathForTerminology(t))
	if err != nil {
		t.Fatalf("NewFakeServiceFromContractFixtures() error = %v", err)
	}

	tests := []struct {
		name      string
		args      []string
		wantError bool
	}{
		{name: "success", args: []string{"workflow", "run", "--request-id", "req-petri-success-001", "--factory", "customer-support-triage"}},
		{name: "command error", args: []string{"workflow", "status", "missing-session"}, wantError: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			closeCount := 0
			root := NewRootCommandWithOptions(RootCommandOptions{
				BuildSessionExecution: func(_ context.Context, request sessionexecutioncli.ServiceRequest) (sessionexecutioncli.ServiceOwner, error) {
					if request.Provider != string(fse.ExecutionProviderJavaScriptRuntime) {
						t.Fatalf("provider = %q, want runtime provider", request.Provider)
					}
					return commandLifecycleExecutionOwner{Service: service, closeCount: &closeCount}, nil
				},
			})
			root.SetOut(io.Discard)
			root.SetErr(io.Discard)
			root.SetArgs(append(test.args, "--execution-provider", "javascript-runtime"))

			err := root.Execute()
			if test.wantError && err == nil {
				t.Fatal("Execute() error = nil, want command error")
			}
			if !test.wantError && err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			if closeCount != 1 {
				t.Fatalf("owner close count = %d, want 1", closeCount)
			}
		})
	}
}

type commandLifecycleExecutionOwner struct {
	fse.Service
	closeCount *int
}

func (owner commandLifecycleExecutionOwner) Close() error {
	*owner.closeCount++
	return nil
}

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

	root := NewRootCommand()
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
	root := NewRootCommand()
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
	root := NewRootCommand()
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
	root := NewRootCommand()
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

	root := NewRootCommand()
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
	root := NewRootCommand()
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
			root := NewRootCommand()
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
func TestUseGeneratedWorkFamilyCutoverEnabled(t *testing.T) {
	if !useGeneratedWorkFamily {
		t.Fatal("useGeneratedWorkFamily = false, want production cutover enabled")
	}
}

func TestProductionWorkCommandUsesGeneratedFamily(t *testing.T) {
	work := productionWorkCommand(&cliGlobalOptions{}, &cliDiagnosticsOptions{})
	if work == nil {
		t.Fatal("productionWorkCommand() = nil, want work command")
	}
	if work.RunE != nil {
		t.Fatal("generated work parent must remain non-runnable")
	}
	for _, path := range []string{"list", "show", "move", "visualize"} {
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
	if !useGeneratedWorkFamily {
		t.Fatal("useGeneratedWorkFamily = false, want production cutover enabled")
	}

	root := NewRootCommand()
	work, _, err := root.Find([]string{"work"})
	if err != nil {
		t.Fatalf("Find(work) error = %v", err)
	}
	if work.RunE != nil {
		t.Fatal("you work must remain non-runnable through generated cutover")
	}
	for _, path := range []string{"list", "show", "move", "visualize"} {
		leaf, _, err := root.Find([]string{"work", path})
		if err != nil {
			t.Fatalf("Find(work %s) error = %v", path, err)
		}
		if leaf.RunE == nil {
			t.Fatalf("you work %s must attach handwritten RunE through generated cutover", path)
		}
	}
}

func TestLegacyWorkFamilyConstructorsRemainCallable(t *testing.T) {
	legacy := NewLegacyWorkFamilyCommand()
	if legacy == nil {
		t.Fatal("NewLegacyWorkFamilyCommand() = nil")
	}
	if _, _, err := legacy.Find([]string{"list"}); err != nil {
		t.Fatalf("legacy work tree missing list: %v", err)
	}

	legacyRoot := NewLegacyWorkFamilyRootForParity()
	if legacyRoot == nil {
		t.Fatal("NewLegacyWorkFamilyRootForParity() = nil")
	}
	work, _, err := legacyRoot.Find([]string{"work"})
	if err != nil {
		t.Fatalf("Find(work) error = %v", err)
	}
	if _, _, err := work.Find([]string{"visualize"}); err != nil {
		t.Fatalf("legacy parity root missing visualize: %v", err)
	}
}
