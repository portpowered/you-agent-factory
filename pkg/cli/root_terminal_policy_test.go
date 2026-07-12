package cli

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	runcli "github.com/portpowered/infinite-you/pkg/cli/run"
	submitcli "github.com/portpowered/infinite-you/pkg/cli/submit"
	"github.com/portpowered/infinite-you/pkg/cli/terminalpolicy"
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
