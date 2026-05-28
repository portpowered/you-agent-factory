package cli

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"reflect"
	"strconv"
	"strings"
	"testing"

	runcli "github.com/portpowered/infinite-you/pkg/cli/run"
	workcli "github.com/portpowered/infinite-you/pkg/cli/work"
	"github.com/portpowered/infinite-you/pkg/logging"
)

func TestRunCommand_VerboseFlag(t *testing.T) {
	root := NewRootCommand()
	runCmd, _, err := root.Find([]string{"run"})
	if err != nil {
		t.Fatalf("find run: %v", err)
	}

	vFlag := runCmd.Flags().Lookup("verbose")
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
	explicit.Logger = nil
	explicit.StartupOutput = nil
	if !reflect.DeepEqual(noArgs, explicit) {
		t.Fatalf("no-args and explicit run configs diverge outside documented defaults:\nno-args: %#v\nrun:     %#v", noArgs, explicit)
	}
}

func TestRootCommand_HelpDocumentsOOTBQuickstart(t *testing.T) {
	var out bytes.Buffer
	root := NewRootCommand()
	root.SetOut(&out)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"--help"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute root --help: %v", err)
	}

	help := out.String()
	for _, want := range []string{
		"Running you with no arguments starts the out-of-the-box flow",
		"factory/inputs/task/default",
		"http://localhost:7437/dashboard/ui",
		"printf \"Fix the lint issues\\n\" > factory/inputs/task/default/fix-lint.md",
		"docs",
		"Print packaged markdown reference topics",
		"you docs workstation",
	} {
		if !strings.Contains(help, want) {
			t.Fatalf("root help missing %q:\n%s", want, help)
		}
	}
	for _, disallowed := range []string{"goreleaser", "GoReleaser"} {
		if strings.Contains(help, disallowed) {
			t.Fatalf("root help should not include release tooling instruction %q:\n%s", disallowed, help)
		}
	}
}

func TestRootCommand_HelpDocumentsDiagnosticsContract(t *testing.T) {
	var out bytes.Buffer
	root := NewRootCommand()
	root.SetOut(&out)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"--help"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute root --help: %v", err)
	}

	help := out.String()
	for _, want := range []string{
		"Default command output is customer-facing",
		"Verbose and debug diagnostics are for troubleshooting",
		"JSON stdout remains parseable",
		"must not include full prompts",
		"full work payloads",
		"access tokens",
		"full model input text",
		"full successful response bodies",
		"sensitive generated content",
	} {
		if !strings.Contains(help, want) {
			t.Fatalf("root help missing diagnostics contract %q:\n%s", want, help)
		}
	}
}

func TestRunCommand_DebugFlag(t *testing.T) {
	root := NewRootCommand()
	runCmd, _, err := root.Find([]string{"run"})
	if err != nil {
		t.Fatalf("find run: %v", err)
	}

	dFlag := runCmd.Flags().Lookup("debug")
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
	if !strings.Contains(runCmd.Long, "run you with no arguments") {
		t.Fatal("expected run command long help text to point users to no-arg default flow")
	}
	if !strings.Contains(runCmd.Long, "factory/inputs/task/default") {
		t.Fatal("expected run command long help text to mention default task input path")
	}
	if !strings.Contains(runCmd.Example, "factory/inputs/task/default") {
		t.Fatal("expected run command examples to mention default task input path")
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
		"work",
		"list",
		"--state-name", "review",
		"--state-type", "PROCESSING",
		"--sort-by", "state.type",
		"--max-results", "25",
		"--next-token", "cursor-1",
		"--json",
		"--port", "9090",
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
	if got.Port != 9090 {
		t.Fatalf("port = %d, want 9090", got.Port)
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

func TestWorkListCommand_DefaultPortMapsToSharedLocalPort(t *testing.T) {
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

	if got.Port != 7437 {
		t.Fatalf("port = %d, want 7437", got.Port)
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
		{name: "runtime-log-dir", def: "", usageIn: "directory for structured runtime log files"},
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
	if !strings.Contains(runCmd.Long, "Runtime logs are structured JSON rolling files") {
		t.Fatal("expected run command long help text to document runtime log behavior")
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
		"--workflow", "workflow-1",
		"--work", "work.json",
		"--port", "0",
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
	if got.Workflow != "workflow-1" {
		t.Errorf("workflow = %q, want %q", got.Workflow, "workflow-1")
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
	if got.Port != 0 {
		t.Errorf("port = %d, want %d", got.Port, 0)
	}
	if got.AutoPort {
		t.Fatal("expected explicit --port to disable automatic port resolution")
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
		t.Fatalf("runtime log dir = %q, want logs/runtime", got.RuntimeLogDir)
	}
	want := logging.RuntimeLogConfig{MaxSize: 11, MaxBackups: 12, MaxAge: 13, Compress: true}
	if got.RuntimeLogConfig != want {
		t.Fatalf("runtime log config = %#v, want %#v", got.RuntimeLogConfig, want)
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
