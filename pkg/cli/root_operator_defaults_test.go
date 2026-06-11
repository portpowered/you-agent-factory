package cli

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	runcli "github.com/portpowered/infinite-you/pkg/cli/run"
	"github.com/portpowered/infinite-you/pkg/config/operatorconfig"
	"github.com/spf13/cobra"
)

func TestRunCommand_UnknownRunnerFlagReturnsCobraError(t *testing.T) {
	root := NewRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"run", "--runner", "codex"})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected unknown --runner flag error")
	}
	if !strings.Contains(err.Error(), "unknown flag") || !strings.Contains(err.Error(), "--runner") {
		t.Fatalf("error = %q, want unknown --runner flag error", err.Error())
	}
}

func TestRootAndRunHelp_ShowDefaultWorkerModelFlagsAndHideRunner(t *testing.T) {
	root := NewRootCommand()
	runCmd, _, err := root.Find([]string{"run"})
	if err != nil {
		t.Fatalf("find run: %v", err)
	}

	for name, cmd := range map[string]*cobra.Command{"root": root, "run": runCmd} {
		if strings.Contains(cmd.Long, "--runner") {
			t.Fatalf("%s long help still mentions --runner:\n%s", name, cmd.Long)
		}
		if cmd.Root().PersistentFlags().Lookup("default-worker-model-provider") == nil {
			t.Fatalf("%s missing --default-worker-model-provider flag", name)
		}
		if cmd.Root().PersistentFlags().Lookup("default-worker-model") == nil {
			t.Fatalf("%s missing --default-worker-model flag", name)
		}
	}
}

func TestRootCommand_DefaultWorkerModelProviderFlagMapsToRunConfig(t *testing.T) {
	originalRunCLI := runCLI
	defer func() {
		runCLI = originalRunCLI
	}()

	var got runcli.RunConfig
	runCLI = func(_ context.Context, cfg runcli.RunConfig) error {
		got = cfg
		return nil
	}

	root := NewRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"--default-worker-model-provider", "codex", "run", "--no-record"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute root with default provider flag: %v", err)
	}
	if got.OperatorDefaults.WorkerModelProvider != "CODEX" {
		t.Fatalf("provider = %q, want CODEX", got.OperatorDefaults.WorkerModelProvider)
	}
	if got.OperatorDefaults.WorkerModelProviderSource != operatorconfig.SourceFlag {
		t.Fatalf("provider source = %q, want flag", got.OperatorDefaults.WorkerModelProviderSource)
	}
}

func TestRootCommand_DefaultWorkerModelFlagMapsToRunConfig(t *testing.T) {
	originalRunCLI := runCLI
	defer func() {
		runCLI = originalRunCLI
	}()

	var got runcli.RunConfig
	runCLI = func(_ context.Context, cfg runcli.RunConfig) error {
		got = cfg
		return nil
	}

	root := NewRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"run", "--default-worker-model", "gpt-5-codex", "--no-record"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute run with default model flag: %v", err)
	}
	if got.OperatorDefaults.WorkerModel != "gpt-5-codex" {
		t.Fatalf("model = %q, want gpt-5-codex", got.OperatorDefaults.WorkerModel)
	}
	if got.OperatorDefaults.WorkerModelSource != operatorconfig.SourceFlag {
		t.Fatalf("model source = %q, want flag", got.OperatorDefaults.WorkerModelSource)
	}
}

func TestRootCommand_NoArgsHonorsDefaultWorkerModelFlags(t *testing.T) {
	originalRunCLI := runCLI
	defer func() {
		runCLI = originalRunCLI
	}()

	var got runcli.RunConfig
	runCLI = func(_ context.Context, cfg runcli.RunConfig) error {
		got = cfg
		return nil
	}

	root := NewRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"--default-worker-model-provider", "codex", "--default-worker-model", "gpt-5-codex"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute root no args with default model flags: %v", err)
	}
	if got.OperatorDefaults.WorkerModelProvider != "CODEX" {
		t.Fatalf("provider = %q, want CODEX", got.OperatorDefaults.WorkerModelProvider)
	}
	if got.OperatorDefaults.WorkerModel != "gpt-5-codex" {
		t.Fatalf("model = %q, want gpt-5-codex", got.OperatorDefaults.WorkerModel)
	}
}

func TestRootCommand_DefaultProviderFlagRejectsUnresolvedSymbolicDefault(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	root := NewRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"run", "--default-worker-model-provider", "DEFAULT", "--no-record"})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected unresolved DEFAULT provider error")
	}
	if !strings.Contains(err.Error(), "DEFAULT requires a concrete provider") {
		t.Fatalf("error = %q, want unresolved DEFAULT guidance", err.Error())
	}
}

func TestRootCommand_DefaultProviderFlagResolvesSymbolicDefaultFromFile(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	configPath := operatorconfig.DefaultConfigPath(homeDir)
	if err := os.MkdirAll(filepath.Dir(configPath), 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(configPath, []byte(`{
		"defaults": {
			"workerModelProvider": "codex"
		}
	}`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	originalRunCLI := runCLI
	defer func() {
		runCLI = originalRunCLI
	}()

	var got runcli.RunConfig
	runCLI = func(_ context.Context, cfg runcli.RunConfig) error {
		got = cfg
		return nil
	}

	root := NewRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"run", "--default-worker-model-provider", "DEFAULT", "--no-record"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute run with DEFAULT provider flag: %v", err)
	}
	if got.OperatorDefaults.WorkerModelProvider != "CODEX" {
		t.Fatalf("provider = %q, want CODEX", got.OperatorDefaults.WorkerModelProvider)
	}
	if got.OperatorDefaults.WorkerModelProviderSource != operatorconfig.SourceFlag {
		t.Fatalf("provider source = %q, want flag", got.OperatorDefaults.WorkerModelProviderSource)
	}
}

func TestRunCommand_VerboseDiagnosticsIncludeOperatorDefaultPrecedence(t *testing.T) {
	originalRunCLI := runCLI
	defer func() {
		runCLI = originalRunCLI
	}()

	var diagnostics bytes.Buffer
	runCLI = func(_ context.Context, cfg runcli.RunConfig) error {
		if cfg.Diagnostics == nil {
			t.Fatal("expected diagnostics writer")
		}
		_, err := cfg.Diagnostics.Write([]byte(cfg.OperatorDefaults.DiagnosticsLine() + "\n"))
		return err
	}

	root := NewRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(&diagnostics)
	root.SetArgs([]string{"run", "--verbose", "--default-worker-model-provider", "codex", "--default-worker-model", "gpt-5-codex", "--no-record"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute run with verbose operator defaults: %v", err)
	}

	got := diagnostics.String()
	for _, want := range []string{
		"operatorDefaults precedence=file < env < flag",
		"provider=CODEX",
		"providerSource=flag",
		"model=gpt-5-codex",
		"modelSource=flag",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("diagnostics missing %q:\n%s", want, got)
		}
	}
}
