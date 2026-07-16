// backendsizecheck:ignore-file consolidated root run and operator-default CLI tests remain together until dedicated CLI test seams split.
// pkgmaintcheck:ignore-file-lines consolidated root run and operator-default CLI tests remain together until dedicated CLI test seams split.
package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/config/configinit"
	"github.com/portpowered/infinite-you/pkg/config/operatorconfig"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	factorysessionexecution "github.com/portpowered/infinite-you/pkg/factory/sessions/execution"
	"github.com/portpowered/infinite-you/pkg/factory/sessions/execution/fixtures"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	"github.com/portpowered/infinite-you/pkg/service"
	"github.com/portpowered/infinite-you/pkg/transports/cli/cliserver"
	factorycli "github.com/portpowered/infinite-you/pkg/transports/cli/factory"
	runcli "github.com/portpowered/infinite-you/pkg/transports/cli/run"
	sessionexecutioncli "github.com/portpowered/infinite-you/pkg/transports/cli/sessionexecution"
	workcli "github.com/portpowered/infinite-you/pkg/transports/cli/work"
	"github.com/spf13/cobra"
)

func TestMain(m *testing.M) {
	homeDir, err := os.MkdirTemp("", "you-cli-test-home-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "create cli test home: %v\n", err)
		os.Exit(1)
	}
	defer func() {
		_ = os.RemoveAll(homeDir)
	}()

	os.Setenv("HOME", homeDir)
	os.Setenv("USERPROFILE", homeDir)
	os.Setenv("HOMEDRIVE", filepath.VolumeName(homeDir))
	os.Setenv("HOMEPATH", string(os.PathSeparator))

	os.Exit(m.Run())
}

func TestGeneratedRunSubmitFamilyForParityUsesProductionHandlerBindings(t *testing.T) {
	root, err := NewGeneratedRunSubmitFamilyCommandForParity()
	if err != nil {
		t.Fatalf("NewGeneratedRunSubmitFamilyCommandForParity() error = %v", err)
	}
	for _, path := range []struct {
		args []string
		name string
	}{
		{args: []string{"run"}, name: "run"},
		{args: []string{"submit"}, name: "submit"},
		{args: []string{"submit", "batch"}, name: "batch"},
	} {
		cmd, _, findErr := root.Find(path.args)
		if findErr != nil {
			t.Fatalf("Find(%v) error = %v", path.args, findErr)
		}
		if cmd.Name() != path.name || cmd.PreRunE == nil || cmd.RunE == nil {
			t.Fatalf("Find(%v) = %q lifecycle=(%t,%t)", path.args, cmd.Name(), cmd.PreRunE != nil, cmd.RunE != nil)
		}
	}
}

func TestProductionRunSubmitFamilyCutoverEnabled(t *testing.T) {
	if !useGeneratedRunSubmitFamily {
		t.Fatal("useGeneratedRunSubmitFamily = false, want production cutover enabled")
	}

	root := NewRootCommandWithOptions(RootCommandOptions{})
	for _, path := range [][]string{{"run"}, {"submit"}, {"submit", "batch"}} {
		cmd, remaining, err := root.Find(path)
		if err != nil {
			t.Fatalf("Find(%v) error = %v", path, err)
		}
		if len(remaining) != 0 {
			t.Fatalf("Find(%v) remaining = %v, want none", path, remaining)
		}
		if cmd.PreRunE == nil || cmd.RunE == nil {
			t.Fatalf("Find(%v) lifecycle = (%t, %t), want retained PreRunE and RunE", path, cmd.PreRunE != nil, cmd.RunE != nil)
		}
	}

	assertDirectCommandCount(t, root, "run", 1)
	assertDirectCommandCount(t, root, "submit", 1)
	submitCmd, _, err := root.Find([]string{"submit"})
	if err != nil {
		t.Fatalf("find submit: %v", err)
	}
	assertDirectCommandCount(t, submitCmd, "batch", 1)
}

func TestRunSubmitFamilyRollbackBuildsRetainedCommands(t *testing.T) {
	options := normalizeRootCommandOptions(RootCommandOptions{})
	globals := &cliGlobalOptions{server: cliserver.DefaultBaseURI}
	diagnostics := &cliDiagnosticsOptions{}
	operatorDefaults := &cliOperatorDefaultsOptions{}

	commands, err := buildRunSubmitProductionCommands(
		globals, diagnostics, operatorDefaults, options, false,
	)
	if err != nil {
		t.Fatalf("buildRunSubmitProductionCommands(legacy) error = %v", err)
	}
	if commands.Run.Name() != "run" || commands.Submit.Name() != "submit" {
		t.Fatalf("legacy commands = (%q, %q), want (run, submit)", commands.Run.Name(), commands.Submit.Name())
	}
	if commands.Run.PreRunE == nil || commands.Run.RunE == nil || commands.Submit.PreRunE == nil || commands.Submit.RunE == nil {
		t.Fatal("legacy rollback commands must retain handwritten lifecycles")
	}
	assertDirectCommandCount(t, commands.Submit, "batch", 1)
}

func assertDirectCommandCount(t *testing.T, parent *cobra.Command, name string, want int) {
	t.Helper()
	count := 0
	for _, command := range parent.Commands() {
		if command.Name() == name {
			count++
		}
	}
	if count != want {
		t.Fatalf("%s direct %q command count = %d, want %d", parent.CommandPath(), name, count, want)
	}
}

func newComposedTestRootCommand(t *testing.T) *cobra.Command {
	t.Helper()
	return NewRootCommandWithOptions(RootCommandOptions{
		RunFactory: func(ctx context.Context, cfg runcli.RunConfig) error {
			application, err := runcli.BuildApplication(ctx, cfg, nil, func(
				buildCtx context.Context,
				serviceCfg *service.FactoryServiceConfig,
			) (runcli.InvocationRunner, error) {
				svc, err := service.BuildFactoryService(buildCtx, service.NormalizeInvocationBootstrapConfig(serviceCfg))
				if err != nil {
					return nil, err
				}
				return service.NewInvocationBootstrap(svc)
			})
			if err != nil {
				return err
			}
			return application.Run(ctx)
		},
		BuildSessionExecution: func(_ context.Context, _ sessionexecutioncli.ServiceRequest) (sessionexecutioncli.ServiceOwner, error) {
			catalogPath := filepath.Join("..", "..", "..", filepath.FromSlash(fixtures.ContractFixtureCatalogRelativePath))
			service, err := factorysessionexecution.NewFakeServiceFromContractFixtures(catalogPath)
			return rootRunExecutionOwner{Service: service}, err
		},
	})
}

type rootRunExecutionOwner struct {
	factorysessionexecution.Service
}

func (rootRunExecutionOwner) Close() error { return nil }

func TestRunCommand_VerboseFlag(t *testing.T) {
	root := NewRootCommand()
	runCmd, _, err := root.Find([]string{"run"})
	if err != nil {
		t.Fatalf("find run: %v", err)
	}

	vFlag := runCmd.Flag("verbose")
	if vFlag == nil {
		t.Fatal("expected --verbose flag on run command")
	}
	if vFlag.DefValue != "false" {
		t.Errorf("default verbose = %q, want %q", vFlag.DefValue, "false")
	}
	if vFlag.Shorthand != "v" {
		t.Errorf("verbose shorthand = %q, want %q", vFlag.Shorthand, "v")
	}
}

func TestRootCommand_SharedDiagnosticsFlagsAvailableOnCoveredCommands(t *testing.T) {
	root := NewRootCommand()
	commands := [][]string{
		{},
		{"run"},
		{"submit"},
		{"work", "list"},
		{"factory", "query"},
		{"factory", "list"},
		{"factory", "create"},
		{"factory", "replace-current"},
		{"factory", "update", "staging"},
		{"factory", "delete", "staging"},
		{"models", "list"},
		{"models", "inspect"},
		{"models", "invoke"},
		{"models", "pull"},
		{"factory", "config", "flatten"},
		{"factory", "config", "expand"},
		{"factory", "config", "validate"},
		{"init"},
		{"docs", "config"},
	}

	for _, path := range commands {
		cmd := root
		if len(path) > 0 {
			found, _, err := root.Find(path)
			if err != nil {
				t.Fatalf("find %v: %v", path, err)
			}
			cmd = found
		}
		for name, shorthand := range map[string]string{"verbose": "v", "debug": "d"} {
			flag := cmd.Flag(name)
			if flag == nil {
				t.Fatalf("%v missing shared --%s flag", path, name)
			}
			if flag.DefValue != "false" {
				t.Fatalf("%v --%s default = %q, want false", path, name, flag.DefValue)
			}
			if flag.Shorthand != shorthand {
				t.Fatalf("%v --%s shorthand = %q, want %q", path, name, flag.Shorthand, shorthand)
			}
		}
	}
}

func TestRunCommand_RecordFlagsDocumentDefaultRecordingBehavior(t *testing.T) {
	root := NewRootCommand()
	runCmd, _, err := root.Find([]string{"run"})
	if err != nil {
		t.Fatalf("find run: %v", err)
	}

	recordFlag := runCmd.Flags().Lookup("record")
	if recordFlag == nil {
		t.Fatal("expected --record flag on run command")
	}
	if !strings.Contains(recordFlag.Usage, "default live runs record automatically unless --no-record is used") {
		t.Fatalf("--record usage = %q, want default-recording guidance", recordFlag.Usage)
	}
	if !strings.Contains(recordFlag.Usage, "replay artifacts are sensitive") {
		t.Fatalf("--record usage = %q, want sensitivity guidance", recordFlag.Usage)
	}

	noRecordFlag := runCmd.Flags().Lookup("no-record")
	if noRecordFlag == nil {
		t.Fatal("expected --no-record flag on run command")
	}
	if noRecordFlag.DefValue != "false" {
		t.Fatalf("--no-record default = %q, want false", noRecordFlag.DefValue)
	}
	if !strings.Contains(noRecordFlag.Usage, "disable the default replay artifact for this invocation") {
		t.Fatalf("--no-record usage = %q", noRecordFlag.Usage)
	}
	if !strings.Contains(runCmd.Long, "Normal live runs record by default unless you pass --no-record.") {
		t.Fatal("expected run command long help text to document default recording")
	}
	if !strings.Contains(runCmd.Long, "Replay artifacts are sensitive and can contain prompts, payloads, stdout, stderr, and diagnostic metadata.") {
		t.Fatal("expected run command long help text to document replay artifact sensitivity")
	}

	replayFlag := runCmd.Flags().Lookup("replay")
	if replayFlag == nil {
		t.Fatal("expected --replay flag on run command")
	}
	if !strings.Contains(replayFlag.Usage, "existing sensitive replay artifact") {
		t.Fatalf("--replay usage = %q, want sensitivity guidance", replayFlag.Usage)
	}
}

func TestRunCommand_FactoryPromptRejectsEmptyStdinWithStableCode(t *testing.T) {
	originalRunCLI := runCLI
	defer func() {
		runCLI = originalRunCLI
	}()

	dir := t.TempDir()
	factoryPath := writePortableFactoryWithDefaultHandling(t, dir)

	runCalled := false
	runCLI = func(context.Context, runcli.RunConfig) error {
		runCalled = true
		return nil
	}

	root := NewRootCommand()
	root.SetIn(strings.NewReader(""))
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"run", "--factory", factoryPath, "-"})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected empty stdin rejection")
	}
	if !strings.Contains(err.Error(), "INVOCATION_INPUT_EMPTY") {
		t.Fatalf("error = %q, want stable empty stdin code", err.Error())
	}
	if runCalled {
		t.Fatal("run should not start for empty factory stdin")
	}
}

func assertRunStdoutFreeOfOperatorChatter(t *testing.T, stdout string) {
	t.Helper()
	for _, forbidden := range []string{
		"Factory initiated",
		"Dashboard URL",
		"Runtime log",
		"Opening dashboard",
		"Factory:",
		"Recording saved",
	} {
		if strings.Contains(stdout, forbidden) {
			t.Fatalf("stdout = %q, want no %q chatter", stdout, forbidden)
		}
	}
}

func TestRunCommand_FactoryPromptRejectsAmbiguousPositionalAndStdin(t *testing.T) {
	originalRunCLI := runCLI
	defer func() {
		runCLI = originalRunCLI
	}()

	dir := t.TempDir()
	factoryPath := writePortableFactoryWithDefaultHandling(t, dir)

	runCalled := false
	runCLI = func(context.Context, runcli.RunConfig) error {
		runCalled = true
		return nil
	}

	root := NewRootCommand()
	root.SetIn(strings.NewReader("Fix from stdin\n"))
	root.SetOut(io.Discard)
	var stderr bytes.Buffer
	root.SetErr(&stderr)
	root.SetArgs([]string{"run", "--factory", factoryPath, "--quiet", "Fix from args", "-"})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected ambiguous positional and stdin prompt rejection")
	}
	for _, want := range []string{"INVOCATION_INPUT_SOURCE_CONFLICT", "positional_text", "stdin_text"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error = %q, want %q", err.Error(), want)
		}
	}
	for _, want := range []string{"INVOCATION_INPUT_SOURCE_CONFLICT", "positional_text", "stdin_text"} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("stderr = %q, want %q", stderr.String(), want)
		}
	}
	if runCalled {
		t.Fatal("run should not start for ambiguous factory prompt input")
	}
}

func TestRunCommand_FactoryPromptRejectsWorkFlagConflict(t *testing.T) {
	originalRunCLI := runCLI
	defer func() {
		runCLI = originalRunCLI
	}()

	dir := t.TempDir()
	factoryPath := writePortableFactoryWithDefaultHandling(t, dir)

	runCalled := false
	runCLI = func(context.Context, runcli.RunConfig) error {
		runCalled = true
		return nil
	}

	root := NewRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"run", "--factory", factoryPath, "--work", "work.json", "Fix the lint issues"})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected conflict between positional prompt and --work")
	}
	if !strings.Contains(err.Error(), "cannot be used with --work") {
		t.Fatalf("error = %q, want --work conflict message", err.Error())
	}
	if runCalled {
		t.Fatal("run should not start when prompt conflicts with --work")
	}
}

func TestRunCommand_PositionalPromptRequiresFactoryFlag(t *testing.T) {
	originalRunCLI := runCLI
	defer func() {
		runCLI = originalRunCLI
	}()

	runCalled := false
	runCLI = func(context.Context, runcli.RunConfig) error {
		runCalled = true
		return nil
	}

	root := NewRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"run", "--dir", "factory", "Fix the lint issues"})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected positional prompt without --factory to fail")
	}
	if !strings.Contains(err.Error(), "require --factory") {
		t.Fatalf("error = %q, want --factory requirement", err.Error())
	}
	if runCalled {
		t.Fatal("run should not start for positional prompt without --factory")
	}
}

func TestRunCommand_CleanInvocationFailureWritesPlaintextToStderr(t *testing.T) {
	originalRunCLI := runCLI
	defer func() {
		runCLI = originalRunCLI
	}()

	dir := t.TempDir()
	factoryPath := writePortableFactoryWithDefaultHandling(t, dir)

	var stdout, stderr bytes.Buffer
	runCLI = func(context.Context, runcli.RunConfig) error {
		return &runcli.InvocationError{
			Code:    runcli.InvocationErrorCodeFailed,
			Message: "clean invocation failed: mock worker rejected",
		}
	}

	root := NewRootCommand()
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"run", "--factory", factoryPath, "Fix the lint issues"})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected clean invocation failure")
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	if got := stderr.String(); got != "RUN_INVOCATION_FAILED: clean invocation failed: mock worker rejected\n" {
		t.Fatalf("stderr = %q", got)
	}
}

func TestRunCommand_CleanInvocationJSONFailureWritesSingleErrorObjectToStderr(t *testing.T) {
	originalRunCLI := runCLI
	defer func() {
		runCLI = originalRunCLI
	}()

	dir := t.TempDir()
	factoryPath := writePortableFactoryWithDefaultHandling(t, dir)

	var stdout, stderr bytes.Buffer
	runCLI = func(context.Context, runcli.RunConfig) error {
		return &runcli.InvocationError{
			Code:    runcli.InvocationErrorCodeTimeout,
			Message: "clean invocation timed out",
		}
	}

	root := NewRootCommand()
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"--json", "run", "--factory", factoryPath, "Fix the lint issues"})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected clean invocation timeout")
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}

	var payload map[string]string
	if decodeErr := json.Unmarshal(stderr.Bytes(), &payload); decodeErr != nil {
		t.Fatalf("stderr is not one JSON object: %v\n%s", decodeErr, stderr.String())
	}
	if payload["code"] != runcli.InvocationErrorCodeTimeout {
		t.Fatalf("code = %q, want %q", payload["code"], runcli.InvocationErrorCodeTimeout)
	}
	if payload["message"] != "clean invocation timed out" {
		t.Fatalf("message = %q", payload["message"])
	}
}

func TestRootCommand_NoArgsStartsContinuousRun(t *testing.T) {
	originalRunCLI := runCLI
	defer func() {
		runCLI = originalRunCLI
	}()

	var got runcli.RunConfig
	runCLI = func(_ context.Context, cfg runcli.RunConfig) error {
		got = cfg
		return nil
	}

	var out bytes.Buffer
	root := NewRootCommand()
	root.SetOut(&out)
	root.SetErr(io.Discard)
	root.SetArgs([]string{})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute root no args: %v", err)
	}

	if !got.Continuously {
		t.Fatal("expected no-arg invocation to use continuous mode")
	}
	if !got.Bootstrap {
		t.Fatal("expected no-arg invocation to enable bootstrap mode")
	}
	if !got.OpenDashboard {
		t.Fatal("expected no-arg invocation to enable dashboard auto-open")
	}
	if got.Dir != "factory" {
		t.Errorf("dir = %q, want %q", got.Dir, "factory")
	}
	if got.Port != 7437 {
		t.Errorf("port = %d, want %d", got.Port, 7437)
	}
	if !got.AutoPort {
		t.Fatal("expected no-arg invocation to auto-resolve the dashboard port")
	}
	if got.StartupOutput == nil {
		t.Fatal("expected no-arg invocation to configure startup output")
	}
}

func TestRootCommand_NoArgsAndExplicitRunShareHarnessConfig(t *testing.T) {
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
	rootDefault := NewRootCommand()
	rootDefault.SetOut(&rootOut)
	rootDefault.SetErr(io.Discard)
	rootDefault.SetArgs([]string{})
	if err := rootDefault.Execute(); err != nil {
		t.Fatalf("execute root no args: %v", err)
	}

	var explicitOut bytes.Buffer
	explicitRun := NewRootCommand()
	explicitRun.SetOut(&explicitOut)
	explicitRun.SetErr(io.Discard)
	explicitRun.SetArgs([]string{"run"})
	if err := explicitRun.Execute(); err != nil {
		t.Fatalf("execute explicit run: %v", err)
	}

	if len(captured) != 2 {
		t.Fatalf("captured run configs = %d, want 2", len(captured))
	}

	noArgs := captured[0]
	explicit := captured[1]
	if !noArgs.Continuously || !noArgs.Bootstrap || !noArgs.OpenDashboard {
		t.Fatalf("no-args config missing documented OOTB defaults: %#v", noArgs)
	}
	if explicit.Continuously || explicit.Bootstrap || explicit.OpenDashboard {
		t.Fatalf("explicit run should not inherit OOTB-only defaults: %#v", explicit)
	}
	if got := rootOut.String(); !strings.Contains(got, "service startup reached: mode=continuous bootstrap=true open-dashboard=true") {
		t.Fatalf("no-args observable startup output = %q, want OOTB service startup", got)
	}
	if got := explicitOut.String(); !strings.Contains(got, "service startup reached: mode=batch bootstrap=false open-dashboard=false") {
		t.Fatalf("explicit run observable startup output = %q, want explicit service startup", got)
	}

	noArgs.Continuously = false
	noArgs.Bootstrap = false
	noArgs.OpenDashboard = false
	noArgs.Logger = nil
	noArgs.StartupOutput = nil
	noArgs.Output = nil
	explicit.Logger = nil
	explicit.StartupOutput = nil
	explicit.Output = nil
	if !reflect.DeepEqual(noArgs, explicit) {
		t.Fatalf("no-args and explicit run configs diverge outside documented defaults:\nno-args: %#v\nrun:     %#v", noArgs, explicit)
	}
}

func TestRunCommand_DebugFlag(t *testing.T) {
	root := NewRootCommand()
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

	root := NewRootCommand()
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
	root := NewRootCommand()
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
	root := NewRootCommand()
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
	root := NewRootCommand()
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
	root := NewRootCommand()
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
	root := NewRootCommand()
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
	root := NewRootCommand()
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

	root := NewRootCommand()
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

	root := NewRootCommand()
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

	root := NewRootCommand()
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

	root := NewRootCommand()
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
	root := NewRootCommand()
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

func TestWorkListCommand_StateFilterFlagsMapToConfig(t *testing.T) {
	originalListWork := listWork
	defer func() {
		listWork = originalListWork
	}()

	var got workcli.ListConfig
	listWork = func(cfg workcli.ListConfig) error {
		got = cfg
		return nil
	}

	root := NewRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{
		"--json",
		"--server", "http://127.0.0.1:9090",
		"work",
		"list",
		"--state-name", "review",
		"--state-type", "PROCESSING",
		"--sort-by", "state.type",
		"--max-results", "25",
		"--next-token", "cursor-1",
	})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute work list: %v", err)
	}

	if got.StateName != "review" {
		t.Fatalf("state name = %q, want review", got.StateName)
	}
	if got.StateType != "PROCESSING" {
		t.Fatalf("state type = %q, want PROCESSING", got.StateType)
	}
	if got.SortBy != "state.type" {
		t.Fatalf("sort by = %q, want state.type", got.SortBy)
	}
	if got.Server != "http://127.0.0.1:9090" {
		t.Fatalf("server = %q, want http://127.0.0.1:9090", got.Server)
	}
	if got.MaxResults != 25 {
		t.Fatalf("max results = %d, want 25", got.MaxResults)
	}
	if got.NextToken != "cursor-1" {
		t.Fatalf("next token = %q, want cursor-1", got.NextToken)
	}
	if !got.JSON {
		t.Fatal("expected json output flag")
	}
	if got.Output == nil {
		t.Fatal("expected output writer")
	}
}

func TestWorkListCommand_DefaultServerMapsToSharedLocalURI(t *testing.T) {
	originalListWork := listWork
	defer func() {
		listWork = originalListWork
	}()

	var got workcli.ListConfig
	listWork = func(cfg workcli.ListConfig) error {
		got = cfg
		return nil
	}

	root := NewRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"work", "list"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute work list: %v", err)
	}

	if got.Server != "http://localhost:7437" {
		t.Fatalf("server = %q, want http://localhost:7437", got.Server)
	}
}

func TestRunCommand_RuntimeLogFlags(t *testing.T) {
	root := NewRootCommand()
	runCmd, _, err := root.Find([]string{"run"})
	if err != nil {
		t.Fatalf("find run: %v", err)
	}

	defaults := logging.DefaultRuntimeLogConfig()
	tests := []struct {
		name    string
		def     string
		usageIn string
	}{
		{name: "runtime-log-dir", def: "", usageIn: "root directory for structured runtime log files grouped by UTC start date"},
		{name: "runtime-log-max-size-mb", def: "100", usageIn: "rotate each runtime log file"},
		{name: "runtime-log-max-backups", def: "20", usageIn: "maximum rotated runtime log files"},
		{name: "runtime-log-max-age-days", def: "30", usageIn: "maximum days to retain rotated runtime log files"},
		{name: "runtime-log-compress", def: "false", usageIn: "compress rotated runtime log files"},
	}
	tests[1].def = strconv.Itoa(defaults.MaxSize)
	tests[2].def = strconv.Itoa(defaults.MaxBackups)
	tests[3].def = strconv.Itoa(defaults.MaxAge)

	for _, tc := range tests {
		flag := runCmd.Flags().Lookup(tc.name)
		if flag == nil {
			t.Fatalf("expected --%s flag on run command", tc.name)
		}
		if flag.DefValue != tc.def {
			t.Fatalf("--%s default = %q, want %q", tc.name, flag.DefValue, tc.def)
		}
		if !strings.Contains(flag.Usage, tc.usageIn) {
			t.Fatalf("--%s usage = %q, want to contain %q", tc.name, flag.Usage, tc.usageIn)
		}
	}
	if got := runCmd.Flags().Lookup("runtime-log-dir").Usage; !strings.Contains(got, "~/.you-agent-factory/logs") {
		t.Fatalf("--runtime-log-dir usage = %q, want canonical default log path", got)
	}
	if !strings.Contains(runCmd.Long, "Runtime logs are structured JSON rolling files grouped by UTC start date under the selected log root") {
		t.Fatal("expected run command long help text to document UTC-grouped runtime log behavior")
	}
	if !strings.Contains(runCmd.Long, "stdout/stderr only on command failures") {
		t.Fatal("expected run command long help text to document command output policy")
	}
}

func TestRunCommand_QuietFlagMapsToRunConfig(t *testing.T) {
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
	root.SetArgs([]string{
		"run",
		"--quiet",
		"--no-record",
		"--dir", "custom-factory",
		"--work", "work.json",
	})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute run --quiet: %v", err)
	}

	if !got.SuppressDashboardRendering {
		t.Fatal("expected --quiet to suppress dashboard rendering")
	}
	if got.Dir != "custom-factory" {
		t.Errorf("dir = %q, want %q", got.Dir, "custom-factory")
	}
	if got.Workflow != "" {
		t.Errorf("workflow = %q, want empty (run-level --workflow removed)", got.Workflow)
	}
	if got.WorkFile != "work.json" {
		t.Errorf("work file = %q, want %q", got.WorkFile, "work.json")
	}
	if !got.DisableDefaultRecording {
		t.Fatal("expected --no-record to disable default recording")
	}
	if got.RecordPath != "" {
		t.Errorf("record path = %q, want empty", got.RecordPath)
	}
	if got.Port != 7437 {
		t.Errorf("port = %d, want %d", got.Port, 7437)
	}
	if !got.AutoPort {
		t.Fatal("expected default --server to enable automatic port resolution")
	}
	if got.BindHost != "localhost" {
		t.Fatalf("bind host = %q, want localhost", got.BindHost)
	}
	if got.Logger == nil {
		t.Fatal("expected run command to set logger")
	}
}

func TestRunCommand_RecordAndNoRecordFlagsCanBePassedTogetherForDeterministicValidation(t *testing.T) {
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
	root.SetArgs([]string{
		"run",
		"--record", "record.replay.json",
		"--no-record",
	})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute run with --record and --no-record: %v", err)
	}
	if got.RecordPath != "record.replay.json" {
		t.Fatalf("record path = %q, want record.replay.json", got.RecordPath)
	}
	if !got.DisableDefaultRecording {
		t.Fatal("expected --no-record to map into RunConfig for downstream validation")
	}
}

func TestRunCommand_RuntimeLogFlagsMapToRunConfig(t *testing.T) {
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
	root.SetArgs([]string{
		"run",
		"--runtime-log-dir", "logs/runtime",
		"--runtime-log-max-size-mb", "11",
		"--runtime-log-max-backups", "12",
		"--runtime-log-max-age-days", "13",
		"--runtime-log-compress",
	})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute run with runtime log flags: %v", err)
	}

	if got.RuntimeLogDir != "logs/runtime" {
		t.Fatalf("runtime log dir = %q, want unchanged root logs/runtime", got.RuntimeLogDir)
	}
	want := logging.RuntimeLogConfig{MaxSize: 11, MaxBackups: 12, MaxAge: 13, Compress: true}
	if got.RuntimeLogConfig != want {
		t.Fatalf("runtime log config = %#v, want %#v", got.RuntimeLogConfig, want)
	}
}

func TestRunCommand_OutputResponseStreamFlagMapsToRunConfig(t *testing.T) {
	originalRunCLI := runCLI
	defer func() {
		runCLI = originalRunCLI
	}()

	var got runcli.RunConfig
	runCLI = func(_ context.Context, cfg runcli.RunConfig) error {
		got = cfg
		return nil
	}

	env := setupNamedGoalCLIEnv(t)
	root := env.root
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{
		"run",
		"--named", "@you/goal",
		"--output", "response-stream",
		"ship the goal",
	})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute run --output response-stream: %v", err)
	}
	if got.InvocationOutputMode != runcli.InvocationOutputResponseStream {
		t.Fatalf("InvocationOutputMode = %q, want %q", got.InvocationOutputMode, runcli.InvocationOutputResponseStream)
	}
}

func TestRunCommand_WithMockWorkersFlagMapsToRunConfig(t *testing.T) {
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
	root.SetArgs([]string{"run", "--with-mock-workers", "mock-workers.json"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute run --with-mock-workers: %v", err)
	}

	if !got.MockWorkersEnabled {
		t.Fatal("expected --with-mock-workers to enable mock workers")
	}
	if got.MockWorkersConfigPath != "mock-workers.json" {
		t.Fatalf("mock workers config path = %q, want %q", got.MockWorkersConfigPath, "mock-workers.json")
	}
}

func TestRunCommand_WithMockWorkersFlagWithoutPathMapsToDefaultConfig(t *testing.T) {
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
	root.SetArgs([]string{"run", "--with-mock-workers"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute run --with-mock-workers without path: %v", err)
	}

	if !got.MockWorkersEnabled {
		t.Fatal("expected --with-mock-workers to enable mock workers")
	}
	if got.MockWorkersConfigPath != "" {
		t.Fatalf("mock workers config path = %q, want empty default path", got.MockWorkersConfigPath)
	}
}

func TestRunCommand_VerboseFlagMapsToRunConfig(t *testing.T) {
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
	root.SetArgs([]string{"run", "--verbose"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute run --verbose: %v", err)
	}

	if !got.Verbose {
		t.Fatal("expected --verbose to enable service verbose logging")
	}
	if got.Logger == nil {
		t.Fatal("expected run command to set logger")
	}
}

func TestRunCommand_VerboseDiagnosticsUseStderr(t *testing.T) {
	originalRunCLI := runCLI
	defer func() {
		runCLI = originalRunCLI
	}()

	runCLI = func(_ context.Context, cfg runcli.RunConfig) error {
		if !cfg.Verbose {
			t.Fatal("expected verbose run config")
		}
		if cfg.Diagnostics == nil {
			t.Fatal("expected diagnostics writer")
		}
		_, err := fmt.Fprintln(cfg.Diagnostics, "diagnostic: run startup")
		return err
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	root := NewRootCommand()
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"run", "--verbose"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute run --verbose: %v", err)
	}

	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want no diagnostic output", stdout.String())
	}
	if got := stderr.String(); !strings.Contains(got, "diagnostic: run startup") {
		t.Fatalf("stderr = %q, want run diagnostics", got)
	}
}

func TestRunCommand_NamedFactoryResolutionMetadataFlowsForInstalledGoal(t *testing.T) {
	originalRunCLI := runCLI
	defer func() {
		runCLI = originalRunCLI
	}()

	env := setupNamedGoalCLIEnv(t)

	var got runcli.RunConfig
	runCLI = func(_ context.Context, cfg runcli.RunConfig) error {
		got = cfg
		return nil
	}

	executeNamedGoalRun(t, env.root)
	assertInstalledGoalResolution(t, got, env.homeDir)
}

func TestRunCommand_NamedLoopAppliesModelFlagsAndKeepsInvocationConfiguration(t *testing.T) {
	originalRunCLI := runCLI
	defer func() {
		runCLI = originalRunCLI
	}()

	env := setupNamedGoalCLIEnv(t)
	var got runcli.RunConfig
	runCLI = func(_ context.Context, cfg runcli.RunConfig) error {
		got = cfg
		return nil
	}

	env.root.SetArgs([]string{
		"run",
		"--default-worker-model-provider", "cursor",
		"--default-worker-model", "loop-cursor-model",
		"--named", "@you/loop",
		"--no-record",
		"Check the release dashboard",
		"--period", "24h",
		"--worktree", "release-dashboard",
	})
	if err := env.root.Execute(); err != nil {
		t.Fatalf("execute run --named @you/loop with model flags: %v", err)
	}

	if got.NamedFactoryResolution == nil || got.NamedFactoryResolution.Name != "@you/loop" {
		t.Fatalf("named factory resolution = %#v, want @you/loop", got.NamedFactoryResolution)
	}
	if got.OperatorDefaults.WorkerModelProvider != "CURSOR" || got.OperatorDefaults.WorkerModel != "loop-cursor-model" {
		t.Fatalf("operator defaults = %+v, want cursor loop model from flags", got.OperatorDefaults)
	}
	if got.InvocationNormalizedArguments == nil {
		t.Fatal("expected normalized loop invocation arguments")
	}
	if values := got.InvocationNormalizedArguments.Arguments["period"].Values; !reflect.DeepEqual(values, []string{"24h"}) {
		t.Fatalf("period = %#v, want configured cadence", values)
	}
	if values := got.InvocationNormalizedArguments.Arguments["worktree"].Values; !reflect.DeepEqual(values, []string{"release-dashboard"}) {
		t.Fatalf("worktree = %#v, want configured isolation", values)
	}
}

func TestRunCommand_RepeatedInstalledGoalRunReusesDiskCopy(t *testing.T) {
	originalRunCLI := runCLI
	defer func() {
		runCLI = originalRunCLI
	}()

	env := setupNamedGoalCLIEnv(t)

	var runs []runcli.RunConfig
	runCLI = func(_ context.Context, cfg runcli.RunConfig) error {
		runs = append(runs, cfg)
		return nil
	}

	for range 2 {
		executeNamedGoalRun(t, env.root)
	}
	if len(runs) != 2 {
		t.Fatalf("run count = %d, want 2", len(runs))
	}
	assertGoalResolutionReusesInstalledCopy(t, runs[0], runs[1])

	wantMaterializedDir := materializedGoalDir(env.homeDir)
	workerPath := filepath.Join(wantMaterializedDir, interfaces.WorkersDir, "goal-executor", interfaces.FactoryAgentsFileName)
	editedBody := "You are the customer-edited @you/goal built-in.\n"
	if err := os.WriteFile(workerPath, []byte(editedBody), 0o644); err != nil {
		t.Fatalf("WriteFile(materialized goal worker body): %v", err)
	}

	executeNamedGoalRun(t, env.root)
	if len(runs) != 3 {
		t.Fatalf("run count = %d, want 3", len(runs))
	}
	if runs[2].NamedFactoryResolution.FactoryDir != wantMaterializedDir {
		t.Fatalf("third factory dir = %q, want %q", runs[2].NamedFactoryResolution.FactoryDir, wantMaterializedDir)
	}
	assertLoadedGoalWorkerBody(t, runs[2].NamedFactoryResolution.FactoryDir, editedBody)
}

func TestRunCommand_NamedFactoryResolutionMetadataFlowsIntoRunConfig(t *testing.T) {
	originalRunCLI := runCLI
	defer func() {
		runCLI = originalRunCLI
	}()

	workingDirectory := t.TempDir()
	homeDir := t.TempDir()
	projectRoot := filepath.Join(workingDirectory, "factory")
	projectPayload := []byte(`{
	  "name": "project-alpha",
	  "id": "project-alpha",
	  "workTypes": [{"name":"task","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"}]}],
	  "workers": [{"name":"executor","type":"MODEL_WORKER","body":"You are the executor."}],
	  "workstations": [{"name":"execute-project-alpha","worker":"executor","inputs":[{"workType":"task","state":"init"}],"outputs":[{"workType":"task","state":"complete"}],"type":"MODEL_WORKSTATION","body":"Implement {{ .WorkID }}."}]
	}`)
	if _, err := factoryconfig.PersistNamedFactory(projectRoot, "alpha", projectPayload); err != nil {
		t.Fatalf("PersistNamedFactory(project alpha): %v", err)
	}
	globalRoot := filepath.Join(homeDir, ".you-agent-factory", "you-agent-factories")
	globalPayload := []byte(`{
	  "name": "global-alpha",
	  "id": "global-alpha",
	  "workTypes": [{"name":"task","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"}]}],
	  "workers": [{"name":"executor","type":"MODEL_WORKER","body":"You are the executor."}],
	  "workstations": [{"name":"execute-global-alpha","worker":"executor","inputs":[{"workType":"task","state":"init"}],"outputs":[{"workType":"task","state":"complete"}],"type":"MODEL_WORKSTATION","body":"Implement {{ .WorkID }}."}]
	}`)
	if _, err := factoryconfig.PersistNamedFactory(globalRoot, "alpha", globalPayload); err != nil {
		t.Fatalf("PersistNamedFactory(global alpha): %v", err)
	}

	originalWorkingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(workingDirectory); err != nil {
		t.Fatalf("Chdir(%q): %v", workingDirectory, err)
	}
	defer func() {
		if chdirErr := os.Chdir(originalWorkingDirectory); chdirErr != nil {
			t.Fatalf("restore working directory: %v", chdirErr)
		}
	}()
	t.Setenv("HOME", homeDir)
	t.Setenv("USERPROFILE", homeDir)

	var got runcli.RunConfig
	runCLI = func(_ context.Context, cfg runcli.RunConfig) error {
		got = cfg
		return nil
	}

	root := NewRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"run", "--named", "alpha", "--no-record"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute run --named alpha: %v", err)
	}
	if got.NamedFactoryResolution == nil {
		t.Fatal("expected named-factory resolution metadata")
	}
	if got.Dir != got.NamedFactoryResolution.FactoryDir {
		t.Fatalf("run dir = %q, want resolved named-factory dir %q", got.Dir, got.NamedFactoryResolution.FactoryDir)
	}
	if got.NamedFactoryResolution.Source != factoryconfig.NamedFactoryResolutionSourceProjectLocal {
		t.Fatalf("resolution source = %q, want %q", got.NamedFactoryResolution.Source, factoryconfig.NamedFactoryResolutionSourceProjectLocal)
	}
	if got.NamedFactoryResolution.PrecedenceDecision != factoryconfig.NamedFactoryPrecedenceDecisionProjectOverGlobal {
		t.Fatalf("resolution precedence = %q, want %q", got.NamedFactoryResolution.PrecedenceDecision, factoryconfig.NamedFactoryPrecedenceDecisionProjectOverGlobal)
	}
}

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

func TestRootCommand_ExplicitEnvironmentIsIsolatedAndFlagsRetainPrecedence(t *testing.T) {
	homeDir := t.TempDir()
	configPath := operatorconfig.DefaultConfigPath(homeDir)
	if err := os.MkdirAll(filepath.Dir(configPath), 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(configPath, []byte(`{"defaults":{"workerModelProvider":"claude"}}`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	originalRunCLI := runCLI
	defer func() { runCLI = originalRunCLI }()
	var got []runcli.RunConfig
	runCLI = func(_ context.Context, cfg runcli.RunConfig) error {
		got = append(got, cfg)
		return nil
	}

	newCommand := func(environment map[string]string) *cobra.Command {
		return NewRootCommandWithOptions(RootCommandOptions{
			HomeDir: func() (string, error) { return homeDir, nil },
			LookupEnv: func(name string) (string, bool) {
				value, ok := environment[name]
				return value, ok
			},
		})
	}

	first := newCommand(map[string]string{operatorconfig.EnvDefaultWorkerModelProvider: "codex"})
	first.SetOut(io.Discard)
	first.SetErr(io.Discard)
	first.SetArgs([]string{"run", "--default-worker-model-provider", "gemini", "--no-record"})
	if err := first.Execute(); err != nil {
		t.Fatalf("first Execute() error = %v", err)
	}

	second := newCommand(map[string]string{operatorconfig.EnvDefaultWorkerModelProvider: "codex"})
	second.SetOut(io.Discard)
	second.SetErr(io.Discard)
	second.SetArgs(nil)
	if err := second.Execute(); err != nil {
		t.Fatalf("second Execute() error = %v", err)
	}

	third := newCommand(map[string]string{operatorconfig.EnvDefaultWorkerModelProvider: ""})
	third.SetOut(io.Discard)
	third.SetErr(io.Discard)
	third.SetArgs(nil)
	if err := third.Execute(); err != nil {
		t.Fatalf("third Execute() error = %v", err)
	}

	if len(got) != 3 {
		t.Fatalf("run calls = %d, want 3", len(got))
	}
	if got[0].OperatorDefaults.WorkerModelProvider != "GEMINI" || got[0].OperatorDefaults.WorkerModelProviderSource != operatorconfig.SourceFlag {
		t.Fatalf("first defaults = %+v, want GEMINI from flag", got[0].OperatorDefaults)
	}
	if got[1].OperatorDefaults.WorkerModelProvider != "CODEX" || got[1].OperatorDefaults.WorkerModelProviderSource != operatorconfig.SourceEnv {
		t.Fatalf("second defaults = %+v, want CODEX from environment", got[1].OperatorDefaults)
	}
	if got[2].OperatorDefaults.WorkerModelProvider != "CLAUDE" || got[2].OperatorDefaults.WorkerModelProviderSource != operatorconfig.SourceFile {
		t.Fatalf("third defaults = %+v, want CLAUDE from file", got[2].OperatorDefaults)
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
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("USERPROFILE", homeDir)

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
	t.Setenv("USERPROFILE", homeDir)

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
	t.Setenv("USERPROFILE", homeDir)
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

type namedGoalCLIEnv struct {
	homeDir string
	root    *cobra.Command
}

func setupNamedGoalCLIEnv(t *testing.T) namedGoalCLIEnv {
	t.Helper()

	workingDirectory := t.TempDir()
	homeDir := t.TempDir()
	originalWorkingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(workingDirectory); err != nil {
		t.Fatalf("Chdir(%q): %v", workingDirectory, err)
	}
	t.Cleanup(func() {
		if chdirErr := os.Chdir(originalWorkingDirectory); chdirErr != nil {
			t.Fatalf("restore working directory: %v", chdirErr)
		}
	})
	t.Setenv("HOME", homeDir)
	t.Setenv("USERPROFILE", homeDir)
	if _, err := configinit.Init(homeDir); err != nil {
		t.Fatalf("configinit.Init: %v", err)
	}

	root := NewRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	return namedGoalCLIEnv{homeDir: homeDir, root: root}
}

func materializedGoalDir(homeDir string) string {
	return filepath.Join(homeDir, ".you-agent-factory", "you-agent-factories", "@you", "goal")
}

func executeNamedGoalRun(t *testing.T, root *cobra.Command) {
	t.Helper()

	root.SetArgs([]string{"run", "--named", "@you/goal", "--no-record"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute run --named @you/goal: %v", err)
	}
}

func assertInstalledGoalResolution(t *testing.T, got runcli.RunConfig, homeDir string) {
	t.Helper()

	if got.NamedFactoryName != "@you/goal" {
		t.Fatalf("named factory = %q, want @you/goal", got.NamedFactoryName)
	}
	if got.NamedFactoryResolution == nil {
		t.Fatal("expected named-factory resolution metadata")
	}
	if got.Dir != got.NamedFactoryResolution.FactoryDir {
		t.Fatalf("run dir = %q, want resolved named-factory dir %q", got.Dir, got.NamedFactoryResolution.FactoryDir)
	}
	if got.NamedFactoryResolution.Name != "@you/goal" {
		t.Fatalf("resolution name = %q, want @you/goal", got.NamedFactoryResolution.Name)
	}
	if got.NamedFactoryResolution.Source != factoryconfig.NamedFactoryResolutionSourceGlobal {
		t.Fatalf("resolution source = %q, want %q", got.NamedFactoryResolution.Source, factoryconfig.NamedFactoryResolutionSourceGlobal)
	}
	if got.NamedFactoryResolution.PrecedenceDecision != factoryconfig.NamedFactoryPrecedenceDecisionNone {
		t.Fatalf("resolution precedence = %q, want %q", got.NamedFactoryResolution.PrecedenceDecision, factoryconfig.NamedFactoryPrecedenceDecisionNone)
	}

	wantInstalledDir := materializedGoalDir(homeDir)
	if got.NamedFactoryResolution.FactoryDir != wantInstalledDir {
		t.Fatalf("installed factory dir = %q, want %q", got.NamedFactoryResolution.FactoryDir, wantInstalledDir)
	}
	assertMaterializedGoalSplitLayout(t, wantInstalledDir)
}

func assertMaterializedGoalSplitLayout(t *testing.T, materializedDir string) {
	t.Helper()

	for _, path := range []string{
		filepath.Join(materializedDir, interfaces.FactoryConfigFile),
		filepath.Join(materializedDir, interfaces.WorkersDir, "goal-executor", interfaces.FactoryAgentsFileName),
		filepath.Join(materializedDir, interfaces.WorkstationsDir, "execute-goal", interfaces.FactoryAgentsFileName),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected first-use materialized goal path %s: %v", path, err)
		}
	}
	for _, dirName := range []string{interfaces.WorkersDir, interfaces.WorkstationsDir} {
		info, err := os.Stat(filepath.Join(materializedDir, dirName))
		if err != nil {
			t.Fatalf("stat materialized goal %s: %v", dirName, err)
		}
		if !info.IsDir() {
			t.Fatalf("materialized goal %s is not a directory", dirName)
		}
	}
}

func assertGoalResolutionReusesInstalledCopy(t *testing.T, first, second runcli.RunConfig) {
	t.Helper()

	if first.NamedFactoryResolution.Source != factoryconfig.NamedFactoryResolutionSourceGlobal {
		t.Fatalf("first resolution source = %q, want %q", first.NamedFactoryResolution.Source, factoryconfig.NamedFactoryResolutionSourceGlobal)
	}
	if second.NamedFactoryResolution.Source != factoryconfig.NamedFactoryResolutionSourceGlobal {
		t.Fatalf("second resolution source = %q, want %q", second.NamedFactoryResolution.Source, factoryconfig.NamedFactoryResolutionSourceGlobal)
	}
	if second.NamedFactoryResolution.FactoryDir != first.NamedFactoryResolution.FactoryDir {
		t.Fatalf("second factory dir = %q, want stable %q", second.NamedFactoryResolution.FactoryDir, first.NamedFactoryResolution.FactoryDir)
	}
}

func assertLoadedGoalWorkerBody(t *testing.T, factoryDir, editedBody string) {
	t.Helper()

	loaded, err := factoryconfig.LoadRuntimeConfigFromFactoryDir(factoryDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfigFromFactoryDir(edited goal): %v", err)
	}
	worker, ok := loaded.Worker("goal-executor")
	if !ok {
		t.Fatal("expected materialized goal worker")
	}
	if worker.Body != editedBody {
		t.Fatalf("edited goal worker body = %q, want exact edited content %q", worker.Body, editedBody)
	}
}

func TestRunCommand_VerboseDiagnosticsIncludeOperatorDefaultPrecedence(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("USERPROFILE", homeDir)

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
