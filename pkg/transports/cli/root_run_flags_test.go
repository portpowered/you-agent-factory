package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	startupcli "github.com/portpowered/infinite-you/pkg/initializer/process"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workcli "github.com/portpowered/infinite-you/pkg/services/work/transports/cli/work"
	factorycli "github.com/portpowered/infinite-you/pkg/transports/cli/factory"
	runcli "github.com/portpowered/infinite-you/pkg/transports/cli/run"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/spf13/cobra"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
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

func TestRunCommand_LocalSessionSelectsIsolatedSession(t *testing.T) {
	originalRunCLI := runCLI
	defer func() { runCLI = originalRunCLI }()

	const sessionID = "session-explicit"
	var captured runcli.RunConfig
	runCLI = func(_ context.Context, cfg runcli.RunConfig) error {
		captured = cfg
		return nil
	}

	root := newLegacyTestRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"run", "--session", sessionID, "--no-record", "customer request"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute local run with explicit session: %v", err)
	}
	if captured.FactorySessionID != sessionID {
		t.Fatalf("FactorySessionID = %q, want %q", captured.FactorySessionID, sessionID)
	}
}

func TestRunCommand_LocalBatchSessionSelectsIsolatedSession(t *testing.T) {
	const sessionID = "session-batch-explicit"
	var captured runcli.RunConfig
	factory := withTestInjectedPlatformRoles(CommandFactory{})
	root := factory.NewCommand(
		func() (string, error) { return t.TempDir(), nil },
		func(string) (string, bool) { return "", false },
		startupcli.Functions{
			InitializeSystemFunc: func(context.Context, string) error { return nil },
			RunFunc: func(_ context.Context, _ startupcli.RunIntent, selection startupcli.RunSelection) error {
				captured = testRunConfig(selection)
				return nil
			},
		},
	)
	root.SetContext(startupcli.WithWorkingDirectory(context.Background(), t.TempDir()))
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"run", "--session", sessionID, "--no-record", "--work", "batch.json"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute local batch with explicit session: %v", err)
	}
	if captured.FactorySessionID != sessionID {
		t.Fatalf("FactorySessionID = %q, want %q", captured.FactorySessionID, sessionID)
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

func TestFactoryShowCommand_JSONVerboseKeepsStdoutParseableAndDiagnosticsOnStderr(t *testing.T) {
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
		if _, err := fmt.Fprintln(cfg.Diagnostics, "diagnostic: factory show"); err != nil {
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
	root.SetArgs([]string{"--json", "factory", "show", "--verbose"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute factory show --json --verbose: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("stdout is not parseable JSON: %v\n%s", err, stdout.String())
	}
	if payload["name"] != "default" {
		t.Fatalf("stdout JSON = %#v, want default factory name", payload)
	}
	if got := stderr.String(); !strings.Contains(got, "diagnostic: factory show") {
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
	exerciseBatchColdStartCLICharacterization(t)
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

func TestRemoteRunRejectsLocalHostingBeforeRunSideEffects(t *testing.T) {
	const wantMessage = "--remote selects a running server through --server and cannot be combined with --with-server or --with-site; remove --remote for local hosting and use --listen <host:port> to choose an exact local bind"
	tests := []struct {
		name string
		args []string
	}{
		{
			name: "persistent flags before run with server",
			args: []string{"--remote", "--server", "http://selected.example:9443", "run", "--with-server", "--no-record", "same request"},
		},
		{
			name: "persistent flags before run with site",
			args: []string{"--remote", "--server", "http://selected.example:9443", "run", "--with-site", "--no-record", "same request"},
		},
		{
			name: "persistent flags after run with server",
			args: []string{"run", "--with-server", "--no-record", "same request", "--remote", "--server", "http://selected.example:9443"},
		},
		{
			name: "persistent flags after run with site",
			args: []string{"run", "--with-site", "--no-record", "same request", "--remote", "--server", "http://selected.example:9443"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			remoteCalls := 0
			localRunCalls := 0
			browserCalls := 0
			factory := withTestInjectedPlatformRoles(CommandFactory{
				remoteInvocation: rootRemoteInvocationFunc(func(context.Context, runcli.RemoteInvocationRequest) (factoryapi.FactorySessionExecutionResponse, error) {
					remoteCalls++
					return factoryapi.FactorySessionExecutionResponse{}, nil
				}),
			})
			factory.browserOpener = func(context.Context, string) error {
				browserCalls++
				return nil
			}
			root := factory.NewCommand(os.UserHomeDir, os.LookupEnv, startupcli.Functions{
				InitializeSystemFunc: func(context.Context, string) error {
					t.Fatal("system initialization should not run for rejected hosting placement")
					return nil
				},
				RunFunc: func(context.Context, startupcli.RunIntent, startupcli.RunSelection) error {
					localRunCalls++
					return nil
				},
			})
			var stdout, stderr bytes.Buffer
			root.SetOut(&stdout)
			root.SetErr(&stderr)
			root.SetArgs(test.args)

			err := root.Execute()
			if err == nil {
				t.Fatal("remote/local hosting conflict error = nil")
			}
			var invocationErr *runcli.InvocationError
			if !errors.As(err, &invocationErr) {
				t.Fatalf("error = %v, want InvocationError", err)
			}
			if invocationErr.Code != runcli.RemoteLocalHostingConflictCode || invocationErr.Message != wantMessage {
				t.Fatalf("invocation error = %#v, want stable placement conflict", invocationErr)
			}
			var response struct {
				Code    string `json:"code"`
				Family  string `json:"family"`
				Message string `json:"message"`
			}
			if err := json.Unmarshal(stderr.Bytes(), &response); err != nil {
				t.Fatalf("stderr = %q, want one JSON ErrorResponse: %v", stderr.String(), err)
			}
			if response.Code != runcli.RemoteLocalHostingConflictCode || response.Family != string(factoryapi.ErrorFamilyBadRequest) || response.Message != wantMessage {
				t.Fatalf("ErrorResponse = %#v, want stable bad-request placement conflict", response)
			}
			if stdout.Len() != 0 {
				t.Fatalf("stdout = %q, want empty on rejected placement", stdout.String())
			}
			if remoteCalls != 0 || localRunCalls != 0 || browserCalls != 0 {
				t.Fatalf("side effects = remote %d, local run %d, browser %d; want all zero", remoteCalls, localRunCalls, browserCalls)
			}
		})
	}
}

func TestRunHelpDocumentsRemotePlacementAndLocalHosting(t *testing.T) {
	factory := withTestInjectedPlatformRoles(CommandFactory{})
	root := factory.NewCommand(os.UserHomeDir, os.LookupEnv, startupcli.Functions{})
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"run", "--help"})
	if err := root.Execute(); err != nil {
		t.Fatalf("run --help: %v\nstderr=%s", err, stderr.String())
	}
	for _, marker := range []string{
		"--remote",
		"--server",
		"--listen",
		"conflicts with --remote",
		"you --remote --server <uri> run",
	} {
		if !strings.Contains(stdout.String(), marker) {
			t.Fatalf("run help missing %q:\n%s", marker, stdout.String())
		}
	}
}

func TestRemoteRunDispatchesExactNormalizedRequestWithoutOpeningLocalRun(t *testing.T) {
	var got runcli.RemoteInvocationRequest
	remote := rootRemoteInvocationFunc(func(_ context.Context, request runcli.RemoteInvocationRequest) (factoryapi.FactorySessionExecutionResponse, error) {
		got = request
		return factoryapi.FactorySessionExecutionResponse{
			SessionId: "dur-sess-root", Status: factoryapi.FactorySessionDurableLifecycleStatusQueued,
		}, nil
	})
	factory := withTestInjectedPlatformRoles(CommandFactory{remoteInvocation: remote})
	factory.prepareInvocationInput = programmedRemoteArgumentsInput("same request")

	localRunCalls := 0
	root := factory.NewCommand(os.UserHomeDir, os.LookupEnv, startupcli.Functions{
		RunFunc: func(context.Context, startupcli.RunIntent, startupcli.RunSelection) error {
			localRunCalls++
			return nil
		},
	})
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	factoryPath, err := filepath.Abs(filepath.Join("factory", "factory.json"))
	if err != nil {
		t.Fatalf("resolve fixture path: %v", err)
	}
	selectedServer := "http://selected.example:9443/base"
	root.SetArgs([]string{
		"--remote", "--server", selectedServer, "run",
		"--factory", factoryPath, "--output", "primary", "same request",
	})
	if err := root.Execute(); err != nil {
		t.Fatalf("remote run: %v\nstderr=%s", err, stderr.String())
	}
	if localRunCalls != 0 {
		t.Fatalf("local run calls = %d, want zero", localRunCalls)
	}
	if got.Server != selectedServer {
		t.Fatalf("remote server = %q, want %q", got.Server, selectedServer)
	}
	if got.Request.Args == nil || (*got.Request.Args)["prompt"] != "same request" {
		t.Fatalf("normalized remote request args = %#v, want same request", got.Request.Args)
	}
	if got.Request.Source.Kind != factoryapi.FactorySessionExecutionSourceKindFactoryInline {
		t.Fatalf("remote source kind = %q, want inline Factory source", got.Request.Source.Kind)
	}
	if stdout.String() != "Factory session dur-sess-root accepted (QUEUED).\n" {
		t.Fatalf("stdout = %q, want durable acceptance", stdout.String())
	}
}

func TestRunServerPlacementRejectsRemoteLocalOnlyCommandBeforeRun(t *testing.T) {
	globals := &cliGlobalOptions{remote: true}
	options := withTestInjectedPlatformRoles(CommandFactory{})
	commands, err := buildRunServerProductionCommands(
		globals, &cliDiagnosticsOptions{}, &cliOperatorDefaultsOptions{}, options,
	)
	if err != nil {
		t.Fatalf("buildRunServerProductionCommands: %v", err)
	}
	root := &cobra.Command{Use: "you"}
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.AddCommand(commands.Server)
	root.SetArgs([]string{"server"})
	err = root.Execute()
	if err == nil {
		t.Fatal("remote local-only server command error = nil")
	}
	want := `command "you server" supports local placement only; remove --remote`
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("error = %q, want actionable placement error containing %q", err, want)
	}
}

func TestRemoteRunFailureDoesNotFallBackToLocalRun(t *testing.T) {
	remote := rootRemoteInvocationFunc(func(context.Context, runcli.RemoteInvocationRequest) (factoryapi.FactorySessionExecutionResponse, error) {
		return factoryapi.FactorySessionExecutionResponse{}, errors.New("selected remote failed")
	})
	factory := withTestInjectedPlatformRoles(CommandFactory{remoteInvocation: remote})
	factory.prepareInvocationInput = programmedRemoteArgumentsInput("same request")
	localRunCalls := 0
	root := factory.NewCommand(os.UserHomeDir, os.LookupEnv, startupcli.Functions{
		RunFunc: func(context.Context, startupcli.RunIntent, startupcli.RunSelection) error {
			localRunCalls++
			return nil
		},
	})
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	factoryPath, err := filepath.Abs(filepath.Join("factory", "factory.json"))
	if err != nil {
		t.Fatalf("resolve fixture path: %v", err)
	}
	root.SetArgs([]string{
		"--remote", "--server", "http://selected.example:9443", "run",
		"--factory", factoryPath, "--output", "primary", "same request",
	})
	if err := root.Execute(); err == nil {
		t.Fatal("remote failure error = nil")
	}
	if localRunCalls != 0 {
		t.Fatalf("local run calls after remote failure = %d, want zero", localRunCalls)
	}
}

func programmedRemoteArgumentsInput(text string) rootInvocationInputScript {
	return programmedInvocationInput(work.PreparedInvocationInput{
		Source: work.InputSourcePositionalText,
		ResolvedInput: &work.ResolvedInput{
			Source:  work.InputSourcePositionalText,
			Text:    text,
			Content: []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: text}},
		},
		NormalizedArguments: &work.NormalizedArguments{
			Arguments: map[string]work.NormalizedArgument{
				"prompt": {Values: []string{text}},
			},
		},
	}, nil)
}

type rootRemoteInvocationFunc func(context.Context, runcli.RemoteInvocationRequest) (factoryapi.FactorySessionExecutionResponse, error)

func (fn rootRemoteInvocationFunc) StartFactorySession(ctx context.Context, request runcli.RemoteInvocationRequest) (factoryapi.FactorySessionExecutionResponse, error) {
	return fn(ctx, request)
}

var profileSelectedBatchSystemInitializationCases = []struct {
	name         string
	cfg          runcli.RunConfig
	changedFlag  string
	wantDeferred bool
}{
	{
		name: "finite mock no-record batch",
		cfg: runcli.RunConfig{
			WorkFile:                "one-work.json",
			MockWorkersEnabled:      true,
			DisableDefaultRecording: true,
			Port:                    7437,
		},
		wantDeferred: true,
	},
	{
		name: "explicit recording",
		cfg: runcli.RunConfig{
			WorkFile:                "one-work.json",
			MockWorkersEnabled:      true,
			DisableDefaultRecording: false,
			RecordPath:              "recording.jsonl",
		},
	},
	{
		name: "replay",
		cfg: runcli.RunConfig{
			WorkFile:           "one-work.json",
			MockWorkersEnabled: true,
			ReplayPath:         "recording.jsonl",
		},
	},
	{
		name: "server",
		cfg: runcli.RunConfig{
			WorkFile:                "one-work.json",
			MockWorkersEnabled:      true,
			DisableDefaultRecording: true,
			WithServer:              true,
		},
	},
	{
		name: "continuous",
		cfg: runcli.RunConfig{
			WorkFile:                "one-work.json",
			MockWorkersEnabled:      true,
			DisableDefaultRecording: true,
			Continuously:            true,
		},
	},
	{
		name: "real workers",
		cfg: runcli.RunConfig{
			WorkFile:                "one-work.json",
			DisableDefaultRecording: true,
		},
	},
	{
		name: "bootstrap",
		cfg: runcli.RunConfig{
			WorkFile:                "one-work.json",
			MockWorkersEnabled:      true,
			DisableDefaultRecording: true,
			Bootstrap:               true,
		},
	},
	{
		name: "named Factory",
		cfg: runcli.RunConfig{
			WorkFile:                "one-work.json",
			MockWorkersEnabled:      true,
			DisableDefaultRecording: true,
			NamedFactoryName:        "@you/goal",
		},
	},
	{
		name: "listener",
		cfg: runcli.RunConfig{
			WorkFile:                "one-work.json",
			MockWorkersEnabled:      true,
			DisableDefaultRecording: true,
			ListenExplicit:          true,
		},
	},
	{
		name:        "explicit factory path",
		changedFlag: "factory",
		cfg: runcli.RunConfig{
			WorkFile:                "one-work.json",
			FactoryConfigPath:       "factory.json",
			MockWorkersEnabled:      true,
			DisableDefaultRecording: true,
		},
	},
	{
		name:        "explicit factory directory",
		changedFlag: "dir",
		cfg: runcli.RunConfig{
			WorkFile:                "one-work.json",
			Dir:                     "factory",
			MockWorkersEnabled:      true,
			DisableDefaultRecording: true,
		},
	},
}
