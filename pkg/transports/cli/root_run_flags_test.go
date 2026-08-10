package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"

	factorycli "github.com/portpowered/infinite-you/pkg/transports/cli/factory"
	runcli "github.com/portpowered/infinite-you/pkg/transports/cli/run"
	workcli "github.com/portpowered/infinite-you/pkg/transports/cli/work"
)

func TestRootCommand_NoArgsPrintsHelpWithoutRun(t *testing.T) {
	originalRunCLI := runCLI
	defer func() {
		runCLI = originalRunCLI
	}()

	runCalls := 0
	runCLI = func(_ context.Context, _ runcli.RunConfig) error {
		runCalls++
		return nil
	}

	var out bytes.Buffer
	root := newLegacyTestRootCommand()
	root.SetOut(&out)
	root.SetErr(io.Discard)
	root.SetArgs([]string{})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute root no args: %v", err)
	}
	if runCalls != 0 {
		t.Fatalf("run calls = %d, want 0", runCalls)
	}
	if output := out.String(); !strings.Contains(output, "Available Commands:") ||
		!strings.Contains(output, "Run and manage CPN-based workflow factories") {
		t.Fatalf("root no-argument output is not concise discovery help:\n%s", output)
	}
}

func TestRootCommand_NoArgsDoesNotChangeExplicitRun(t *testing.T) {
	originalRunCLI := runCLI
	defer func() {
		runCLI = originalRunCLI
	}()

	var captured []runcli.RunConfig
	runCLI = func(_ context.Context, cfg runcli.RunConfig) error {
		captured = append(captured, cfg)
		mode := "batch"
		if cfg.Continuously {
			mode = "continuous"
		}
		if cfg.StartupOutput != nil {
			fmt.Fprintf(
				cfg.StartupOutput,
				"service startup reached: mode=%s bootstrap=%t open-dashboard=%t\n",
				mode,
				cfg.Bootstrap,
				cfg.OpenDashboard,
			)
		}
		return nil
	}

	var rootOut bytes.Buffer
	rootDefault := newLegacyTestRootCommand()
	rootDefault.SetOut(&rootOut)
	rootDefault.SetErr(io.Discard)
	rootDefault.SetArgs([]string{})
	if err := rootDefault.Execute(); err != nil {
		t.Fatalf("execute root no args: %v", err)
	}

	var explicitOut bytes.Buffer
	explicitRun := newLegacyTestRootCommand()
	explicitRun.SetOut(&explicitOut)
	explicitRun.SetErr(io.Discard)
	explicitRun.SetArgs([]string{"run"})
	if err := explicitRun.Execute(); err != nil {
		t.Fatalf("execute explicit run: %v", err)
	}

	if len(captured) != 1 {
		t.Fatalf("captured run configs = %d, want 1", len(captured))
	}

	explicit := captured[0]
	if explicit.Continuously || explicit.Bootstrap || explicit.OpenDashboard {
		t.Fatalf("explicit run should not inherit OOTB-only defaults: %#v", explicit)
	}
	if got := rootOut.String(); !strings.Contains(got, "Available Commands:") {
		t.Fatalf("no-args observable output = %q, want discovery help", got)
	}
	if got := explicitOut.String(); !strings.Contains(got, "service startup reached: mode=batch bootstrap=false open-dashboard=false") {
		t.Fatalf("explicit run observable startup output = %q, want explicit service startup", got)
	}
}

func TestRunCommand_DebugFlag(t *testing.T) {
	root := newLegacyTestRootCommand()
	runCmd, _, err := root.Find([]string{"run"})
	if err != nil {
		t.Fatalf("find run: %v", err)
	}

	dFlag := runCmd.Flag("debug")
	if dFlag == nil {
		t.Fatal("expected --debug flag on run command")
	}
	if dFlag.DefValue != "false" {
		t.Errorf("default debug = %q, want %q", dFlag.DefValue, "false")
	}
	if dFlag.Shorthand != "d" {
		t.Errorf("debug shorthand = %q, want %q", dFlag.Shorthand, "d")
	}
}

func TestRunCommand_DebugImpliesVerboseRunConfig(t *testing.T) {
	originalRunCLI := runCLI
	defer func() {
		runCLI = originalRunCLI
	}()

	var got runcli.RunConfig
	runCLI = func(_ context.Context, cfg runcli.RunConfig) error {
		got = cfg
		return nil
	}

	root := newLegacyTestRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"run", "--debug"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute run --debug: %v", err)
	}

	if !got.Verbose {
		t.Fatal("expected --debug to imply verbose run behavior")
	}
	if got.Logger == nil {
		t.Fatal("expected run command to set debug-capable logger")
	}
}

func TestWorkListCommand_SharedDiagnosticsFlagsMapToConfig(t *testing.T) {
	originalListWork := listWork
	defer func() {
		listWork = originalListWork
	}()

	var got workcli.ListConfig
	listWork = func(cfg workcli.ListConfig) error {
		got = cfg
		return nil
	}

	var stderr bytes.Buffer
	root := newLegacyTestRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(&stderr)
	root.SetArgs([]string{"work", "list", "--debug"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute work list --debug: %v", err)
	}

	if !got.Verbose {
		t.Fatal("expected --debug to imply verbose command diagnostics")
	}
	if !got.Debug {
		t.Fatal("expected debug config")
	}
	if got.Diagnostics != &stderr {
		t.Fatalf("diagnostics writer = %#v, want configured stderr writer", got.Diagnostics)
	}
	if got.Output == nil {
		t.Fatal("expected stdout writer")
	}
}

func TestFactoryQueryCommand_JSONVerboseKeepsStdoutParseableAndDiagnosticsOnStderr(t *testing.T) {
	originalQueryFactory := queryFactory
	defer func() {
		queryFactory = originalQueryFactory
	}()

	queryFactory = func(cfg factorycli.QueryConfig) error {
		if !cfg.Verbose {
			t.Fatal("expected verbose config")
		}
		if cfg.Diagnostics == nil {
			t.Fatal("expected diagnostics writer")
		}
		if _, err := fmt.Fprintln(cfg.Diagnostics, "diagnostic: factory query"); err != nil {
			return err
		}
		_, err := fmt.Fprintln(cfg.Output, `{"name":"default"}`)
		return err
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	root := newLegacyTestRootCommand()
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"--json", "factory", "query", "--verbose"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute factory query --json --verbose: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("stdout is not parseable JSON: %v\n%s", err, stdout.String())
	}
	if payload["name"] != "default" {
		t.Fatalf("stdout JSON = %#v, want default factory name", payload)
	}
	if got := stderr.String(); !strings.Contains(got, "diagnostic: factory query") {
		t.Fatalf("stderr = %q, want diagnostics", got)
	}
}

func TestRunCommand_ContinuouslyFlag(t *testing.T) {
	root := newLegacyTestRootCommand()
	runCmd, _, err := root.Find([]string{"run"})
	if err != nil {
		t.Fatalf("find run: %v", err)
	}

	flag := runCmd.Flags().Lookup("continuously")
	if flag == nil {
		t.Fatal("expected --continuously flag on run command")
	}
	if flag.DefValue != "false" {
		t.Errorf("default continuously = %q, want %q", flag.DefValue, "false")
	}
	if flag.Usage != "keep the factory alive while idle until cancelled" {
		t.Errorf("continuously usage = %q", flag.Usage)
	}
	if runCmd.Long == "" {
		t.Fatal("expected run command long help text")
	}
	if !strings.Contains(runCmd.Long, "you run --work ./docs/examples/startup-work.json") {
		t.Fatal("expected run command long help text to provide explicit default Work")
	}
	if !strings.Contains(runCmd.Long, "factory/inputs/task/default") {
		t.Fatal("expected run command long help text to mention default task input path")
	}
	if !strings.Contains(runCmd.Example, "run --work ./docs/examples/startup-work.json") {
		t.Fatal("expected run command examples to provide explicit default Work")
	}
}

func TestRunCommand_RecordAndReplayFlags(t *testing.T) {
	root := newLegacyTestRootCommand()
	runCmd, _, err := root.Find([]string{"run"})
	if err != nil {
		t.Fatalf("find run: %v", err)
	}

	for _, name := range []string{"record", "replay"} {
		flag := runCmd.Flags().Lookup(name)
		if flag == nil {
			t.Fatalf("expected --%s flag on run command", name)
		}
		if flag.DefValue != "" {
			t.Errorf("--%s default = %q, want empty", name, flag.DefValue)
		}
	}
}

func TestRunCommand_WithMockWorkersFlag(t *testing.T) {
	root := newLegacyTestRootCommand()
	runCmd, _, err := root.Find([]string{"run"})
	if err != nil {
		t.Fatalf("find run: %v", err)
	}

	flag := runCmd.Flags().Lookup("with-mock-workers")
	if flag == nil {
		t.Fatal("expected --with-mock-workers flag on run command")
	}
	if flag.DefValue != "" {
		t.Errorf("default with-mock-workers = %q, want empty", flag.DefValue)
	}
	if flag.NoOptDefVal == "" {
		t.Error("with-mock-workers should define an internal optional-value default")
	}
	if !strings.Contains(flag.Usage, "optional mock-workers JSON config path") {
		t.Errorf("with-mock-workers usage = %q", flag.Usage)
	}
	if !strings.Contains(runCmd.Long, "--with-mock-workers") {
		t.Fatal("expected run command long help text to mention --with-mock-workers")
	}
}

func TestRunCommand_SkipPermissionsFlag(t *testing.T) {
	root := newLegacyTestRootCommand()
	runCmd, _, err := root.Find([]string{"run"})
	if err != nil {
		t.Fatalf("find run: %v", err)
	}

	flag := runCmd.Flags().Lookup("skip-permissions")
	if flag == nil {
		t.Fatal("expected --skip-permissions flag on run command")
	}
	if flag.DefValue != "false" {
		t.Errorf("default skip-permissions = %q, want false", flag.DefValue)
	}
	if !strings.Contains(flag.Usage, "invocation-only unsafe permission bypass") {
		t.Errorf("skip-permissions usage = %q", flag.Usage)
	}
	if !strings.Contains(runCmd.Long, "--skip-permissions") {
		t.Fatal("expected run command long help text to mention --skip-permissions")
	}
}

func TestRunCommand_WorktreeFlagMapsToRunConfig(t *testing.T) {
	originalRunCLI := runCLI
	defer func() {
		runCLI = originalRunCLI
	}()

	var got runcli.RunConfig
	runCLI = func(_ context.Context, cfg runcli.RunConfig) error {
		got = cfg
		return nil
	}

	root := newLegacyTestRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"run", "--worktree", "feature-login"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute run --worktree: %v", err)
	}
	if got.Worktree != "feature-login" {
		t.Fatalf("worktree = %q, want feature-login", got.Worktree)
	}
}

func TestRunCommand_SkipPermissionsFlagMapsToRunConfig(t *testing.T) {
	originalRunCLI := runCLI
	defer func() {
		runCLI = originalRunCLI
	}()

	var got runcli.RunConfig
	runCLI = func(_ context.Context, cfg runcli.RunConfig) error {
		got = cfg
		return nil
	}

	root := newLegacyTestRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"run", "--skip-permissions"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute run --skip-permissions: %v", err)
	}
	if got.InvocationSkipPermissionsOverride == nil {
		t.Fatal("expected --skip-permissions to set invocation override")
	}
	if !*got.InvocationSkipPermissionsOverride {
		t.Fatal("expected invocation skip-permissions override to be true")
	}
}

func TestRunCommand_WithoutSkipPermissionsLeavesInvocationOverrideUnset(t *testing.T) {
	originalRunCLI := runCLI
	defer func() {
		runCLI = originalRunCLI
	}()

	var got runcli.RunConfig
	runCLI = func(_ context.Context, cfg runcli.RunConfig) error {
		got = cfg
		return nil
	}

	root := newLegacyTestRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"run"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute run: %v", err)
	}
	if got.InvocationSkipPermissionsOverride != nil {
		t.Fatalf("invocation skip-permissions override = %#v, want nil when flag omitted", got.InvocationSkipPermissionsOverride)
	}
}

func TestRunCommand_WorkflowFlagRejected(t *testing.T) {
	originalRunCLI := runCLI
	defer func() {
		runCLI = originalRunCLI
	}()

	runCalled := false
	runCLI = func(context.Context, runcli.RunConfig) error {
		runCalled = true
		return nil
	}

	root := newLegacyTestRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"run", "--workflow", "workflow-1"})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected run-level --workflow to be rejected")
	}
	if !strings.Contains(err.Error(), "unknown flag: --workflow") {
		t.Fatalf("error = %q, want unknown flag --workflow", err.Error())
	}
	if runCalled {
		t.Fatal("run command should not execute when --workflow is unsupported")
	}
}

func TestRunCommand_RetiredMockExecutionAliasRejected(t *testing.T) {
	originalRunCLI := runCLI
	defer func() {
		runCLI = originalRunCLI
	}()

	runCalled := false
	runCLI = func(context.Context, runcli.RunConfig) error {
		runCalled = true
		return nil
	}

	root := newLegacyTestRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	retiredFlag := "--" + strings.Join([]string{"dry", "run"}, "-")
	root.SetArgs([]string{"run", retiredFlag})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected retired mock-execution alias to be rejected")
	}
	if !strings.Contains(err.Error(), "unknown flag: "+retiredFlag) {
		t.Fatalf("error = %q, want unknown retired flag", err.Error())
	}
	if runCalled {
		t.Fatal("run command should not execute when retired mock-execution alias is unsupported")
	}
}

func TestRunCommand_QuietFlag(t *testing.T) {
	root := newLegacyTestRootCommand()
	runCmd, _, err := root.Find([]string{"run"})
	if err != nil {
		t.Fatalf("find run: %v", err)
	}

	flag := runCmd.Flags().Lookup("quiet")
	if flag == nil {
		t.Fatal("expected --quiet flag on run command")
	}
	if flag.DefValue != "false" {
		t.Errorf("default quiet = %q, want %q", flag.DefValue, "false")
	}
	if flag.Usage != "suppress dashboard output for quiet or CI-oriented runs" {
		t.Errorf("quiet usage = %q", flag.Usage)
	}
	if !strings.Contains(runCmd.Long, "--quiet") {
		t.Fatal("expected run command long help text to mention --quiet")
	}
}
