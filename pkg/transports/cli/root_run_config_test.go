package cli

import (
	"context"
	"io"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	runcli "github.com/portpowered/infinite-you/pkg/transports/cli/run"
	workcli "github.com/portpowered/infinite-you/pkg/transports/cli/work"
)

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

	root := newLegacyTestRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{
		"--json",
		"--server", "http://127.0.0.1:9090",
		"work",
		"list",
		"--state", "review",
		"--state-type", "PROCESSING",
		"--work-type", "story",
		"--terminal",
		"--counts",
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
	if got.WorkTypeName != "story" {
		t.Fatalf("work type = %q, want story", got.WorkTypeName)
	}
	if !got.Terminal || got.NonTerminal {
		t.Fatalf("terminality = terminal %t non-terminal %t, want terminal only", got.Terminal, got.NonTerminal)
	}
	if !got.Counts {
		t.Fatal("expected counts request")
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

func TestWorkListCommand_TerminalityFlagsRejectBeforeHandler(t *testing.T) {
	originalListWork := listWork
	defer func() {
		listWork = originalListWork
	}()

	called := false
	listWork = func(workcli.ListConfig) error {
		called = true
		return nil
	}
	root := newLegacyTestRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"work", "list", "--terminal", "--non-terminal"})

	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "terminal") || !strings.Contains(err.Error(), "non-terminal") {
		t.Fatalf("mutually-exclusive flags error = %v, want terminality validation error", err)
	}
	if called {
		t.Fatal("list handler was called after terminality validation")
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

	root := newLegacyTestRootCommand()
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
	root := newLegacyTestRootCommand()
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

	root := newLegacyTestRootCommand()
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

	root := newLegacyTestRootCommand()
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

	root := newLegacyTestRootCommand()
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

	root := newTransportNamedFactoryRoot(t, packagedGoalFactoryName)
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

func TestRunCommand_TextInvocationDefaultsToResponseStream(t *testing.T) {
	originalRunCLI := runCLI
	defer func() { runCLI = originalRunCLI }()

	var got runcli.RunConfig
	runCLI = func(_ context.Context, cfg runcli.RunConfig) error {
		got = cfg
		return nil
	}
	root := newTransportNamedFactoryRoot(t, packagedGoalFactoryName)
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"run", "--named", "@you/goal", "ship the goal"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute default run output: %v", err)
	}
	if got.InvocationOutputMode != runcli.InvocationOutputResponseStream {
		t.Fatalf("InvocationOutputMode = %q, want response stream", got.InvocationOutputMode)
	}
}

func TestRunCommand_ExplicitPrimaryOutputRetainsPrimaryMode(t *testing.T) {
	originalRunCLI := runCLI
	defer func() { runCLI = originalRunCLI }()

	var got runcli.RunConfig
	runCLI = func(_ context.Context, cfg runcli.RunConfig) error {
		got = cfg
		return nil
	}
	root := newTransportNamedFactoryRoot(t, packagedGoalFactoryName)
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"run", "--named", "@you/goal", "--output", "primary", "ship the goal"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute explicit primary output: %v", err)
	}
	if got.InvocationOutputMode != runcli.InvocationOutputPrimaryResult {
		t.Fatalf("InvocationOutputMode = %q, want primary", got.InvocationOutputMode)
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

	root := newLegacyTestRootCommand()
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

	root := newLegacyTestRootCommand()
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

func TestParseRunCommandArgs_WithMockWorkersOptionalPath(t *testing.T) {
	tests := []struct {
		name          string
		args          []string
		wantFlagValue string
		wantRemainder []string
	}{
		{
			name:          "bare flag before signature args",
			args:          []string{"--with-mock-workers", "--to", "fix the bug"},
			wantFlagValue: defaultMockWorkersConfigPathSentinel,
			wantRemainder: []string{"--to", "fix the bug"},
		},
		{
			name:          "bare flag after signature args",
			args:          []string{"--to", "fix the bug", "--with-mock-workers"},
			wantFlagValue: defaultMockWorkersConfigPathSentinel,
			wantRemainder: []string{"--to", "fix the bug"},
		},
		{
			name:          "explicit config path leaves signature args",
			args:          []string{"--with-mock-workers", "mock-workers.json", "--to", "fix the bug"},
			wantFlagValue: "mock-workers.json",
			wantRemainder: []string{"--to", "fix the bug"},
		},
		{
			name:          "config path after boolean run flags remains attached to mock workers",
			args:          []string{"--with-mock-workers", "--no-record", "--quiet", "mock-workers.json", "fix the bug"},
			wantFlagValue: "mock-workers.json",
			wantRemainder: []string{"fix the bug"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := newLegacyTestRootCommand()
			runCmd, _, err := root.Find([]string{"run"})
			if err != nil {
				t.Fatalf("find run command: %v", err)
			}
			flag := runCmd.Flags().Lookup("with-mock-workers")
			if flag == nil {
				t.Fatal("expected --with-mock-workers flag")
			}
			remainder, err := parseRunCommandArgs(runCmd, test.args)
			if err != nil {
				t.Fatalf("parseRunCommandArgs(%v) error = %v", test.args, err)
			}
			if !flag.Changed {
				t.Fatal("expected --with-mock-workers to be marked changed")
			}
			if got := flag.Value.String(); got != test.wantFlagValue {
				t.Fatalf("with-mock-workers value = %q, want %q", got, test.wantFlagValue)
			}
			if !reflect.DeepEqual(remainder, test.wantRemainder) {
				t.Fatalf("remainder = %#v, want %#v", remainder, test.wantRemainder)
			}
		})
	}
}

func TestRunCommand_BareWithMockWorkersDoesNotTreatSignatureFlagAsConfigPath(t *testing.T) {
	originalRunCLI := runCLI
	defer func() {
		runCLI = originalRunCLI
	}()

	tests := []struct {
		name string
		args []string
	}{
		{
			name: "bare flag before --to",
			args: []string{"run", "--with-mock-workers", "--to", "fix the bug"},
		},
		{
			name: "bare flag after --to",
			args: []string{"run", "--to", "fix the bug", "--with-mock-workers"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var got runcli.RunConfig
			runCalled := false
			runCLI = func(_ context.Context, cfg runcli.RunConfig) error {
				runCalled = true
				got = cfg
				return nil
			}

			root := newLegacyTestRootCommand()
			root.SetOut(io.Discard)
			root.SetErr(io.Discard)
			root.SetArgs(test.args)

			err := root.Execute()
			if err == nil {
				t.Fatal("expected unknown signature flag rejection")
			}
			if !strings.Contains(err.Error(), "unknown flag: --to") {
				t.Fatalf("error = %q, want unknown flag: --to (not mock-workers config path)", err.Error())
			}
			if runCalled {
				t.Fatalf("run started with MockWorkersConfigPath=%q; signature args must remain untouched", got.MockWorkersConfigPath)
			}
		})
	}
}
