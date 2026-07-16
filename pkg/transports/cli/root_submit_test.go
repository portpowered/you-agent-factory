package cli

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/config/configinit"
	platformmetrics "github.com/portpowered/infinite-you/pkg/platform/metrics"
	runcli "github.com/portpowered/infinite-you/pkg/transports/cli/run"
	submitcli "github.com/portpowered/infinite-you/pkg/transports/cli/submit"
)

func TestSubmitCommand_HelpAdvertisesRequiredFlags(t *testing.T) {
	root := NewRootCommand()
	submitCmd, _, err := root.Find([]string{"submit"})
	if err != nil {
		t.Fatalf("find submit: %v", err)
	}

	for _, name := range []string{"name", "work-type-name", "payload"} {
		f := submitCmd.Flags().Lookup(name)
		if f == nil {
			t.Errorf("expected --%s flag on submit command", name)
			continue
		}
	}
	for _, name := range []string{"factory", "factory-id", "work-type-id"} {
		if f := submitCmd.Flags().Lookup(name); f != nil {
			t.Fatalf("submit command should not expose --%s", name)
		}
	}

	var out bytes.Buffer
	root = NewRootCommand()
	root.SetOut(&out)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"submit", "--help"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute submit --help: %v", err)
	}

	help := out.String()
	if !strings.Contains(help, "--name") {
		t.Fatalf("submit help should list --name:\n%s", help)
	}
	if !strings.Contains(help, "authored request name") {
		t.Fatalf("submit help should describe --name:\n%s", help)
	}
	if !strings.Contains(help, "--work-type-name") {
		t.Fatalf("submit help should list --work-type-name:\n%s", help)
	}
	if !strings.Contains(help, "work type name to submit to") {
		t.Fatalf("submit help should describe work type names:\n%s", help)
	}
	for _, disallowed := range []string{"--work-type-id", "--factory-id", "--factory"} {
		if strings.Contains(help, disallowed) {
			t.Fatalf("submit help should not list %s:\n%s", disallowed, help)
		}
	}
}

func TestSubmitCommand_WorkTypeIDFlagIsRejected(t *testing.T) {
	originalSubmitWork := submitWork
	defer func() {
		submitWork = originalSubmitWork
	}()

	called := false
	submitWork = func(cfg submitcli.SubmitConfig) error {
		called = true
		return nil
	}

	root := NewRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{
		"submit",
		"--work-type-id", "legacy-task",
		"--payload", "request.md",
	})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected removed --work-type-id flag to fail")
	}
	if !strings.Contains(err.Error(), "unknown flag: --work-type-id") {
		t.Fatalf("removed flag error = %q, want unknown flag", err.Error())
	}
	if called {
		t.Fatal("submit command should not run when --work-type-id is supplied")
	}
}

func TestSubmitCommand_MissingWorkTypeNameReturnsLocalValidationError(t *testing.T) {
	originalSubmitWork := submitWork
	defer func() {
		submitWork = originalSubmitWork
	}()

	called := false
	submitWork = func(cfg submitcli.SubmitConfig) error {
		called = true
		return submitcli.Submit(cfg)
	}

	root := NewRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"submit", "--name", "request-name", "--payload", "work.json"})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected missing work type name to fail")
	}
	if !called {
		t.Fatal("expected submit validation to run")
	}
	if got := err.Error(); got != "--work-type-name is required" {
		t.Fatalf("missing work type error = %q, want --work-type-name is required", got)
	}
}

func TestSubmitCommand_MissingNameReturnsLocalValidationError(t *testing.T) {
	originalSubmitWork := submitWork
	defer func() {
		submitWork = originalSubmitWork
	}()

	called := false
	submitWork = func(cfg submitcli.SubmitConfig) error {
		called = true
		return submitcli.Submit(cfg)
	}

	root := NewRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"submit", "--work-type-name", "tasks", "--payload", "work.json"})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected missing name to fail")
	}
	if !called {
		t.Fatal("expected submit validation to run")
	}
	if got := err.Error(); got != "--name is required" {
		t.Fatalf("missing name error = %q, want --name is required", got)
	}
}

func TestSubmitCommand_MissingPayloadReturnsLocalValidationError(t *testing.T) {
	originalSubmitWork := submitWork
	defer func() {
		submitWork = originalSubmitWork
	}()

	called := false
	submitWork = func(cfg submitcli.SubmitConfig) error {
		called = true
		return submitcli.Submit(cfg)
	}

	root := NewRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"submit", "--name", "request-name", "--work-type-name", "tasks"})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected missing payload to fail")
	}
	if !called {
		t.Fatal("expected submit validation to run")
	}
	if got := err.Error(); got != "--payload is required" {
		t.Fatalf("missing payload error = %q, want --payload is required", got)
	}
}

func TestSubmitCommand_WorkTypeNameFlagMapsToSubmitConfig(t *testing.T) {
	originalSubmitWork := submitWork
	defer func() {
		submitWork = originalSubmitWork
	}()

	var got submitcli.SubmitConfig
	submitWork = func(cfg submitcli.SubmitConfig) error {
		got = cfg
		return nil
	}

	root := NewRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{
		"submit",
		"--name", "request-name",
		"--work-type-name", "tasks",
		"--payload", "request.md",
	})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute submit --work-type-name: %v", err)
	}

	if got.Name != "request-name" {
		t.Fatalf("name = %q, want request-name", got.Name)
	}
	if got.WorkTypeName != "tasks" {
		t.Fatalf("work type name = %q, want tasks", got.WorkTypeName)
	}
	if got.Payload != "request.md" {
		t.Fatalf("payload = %q, want request.md", got.Payload)
	}
	if got.Server != "http://localhost:7437" {
		t.Fatalf("server = %q, want http://localhost:7437", got.Server)
	}
}

func TestSubmitCommand_PortFlagRejected(t *testing.T) {
	root := NewRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{
		"submit",
		"--name", "request-name",
		"--work-type-name", "tasks",
		"--payload", "request.md",
		"--port", "7437",
	})

	if execErr := root.Execute(); execErr == nil {
		t.Fatal("expected --port rejection")
	} else if !strings.Contains(execErr.Error(), "--server") {
		t.Fatalf("error = %v, want --server guidance", execErr)
	}
}

func TestSubmitCommand_DefaultServerMapsToSharedLocalURI(t *testing.T) {
	originalSubmitWork := submitWork
	defer func() {
		submitWork = originalSubmitWork
	}()

	var got submitcli.SubmitConfig
	submitWork = func(cfg submitcli.SubmitConfig) error {
		got = cfg
		return nil
	}

	root := NewRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{
		"submit",
		"--name", "request-name",
		"--work-type-name", "tasks",
		"--payload", "request.md",
	})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute submit: %v", err)
	}

	if got.Server != "http://localhost:7437" {
		t.Fatalf("server = %q, want http://localhost:7437", got.Server)
	}
}

func TestSubmitBatchCommand_HelpDocumentsBatchIngressModes(t *testing.T) {
	root := NewRootCommand()
	batchCmd, _, err := root.Find([]string{"submit", "batch"})
	if err != nil {
		t.Fatalf("find submit batch: %v", err)
	}

	for _, name := range []string{"file", "dry-run", "session"} {
		if f := batchCmd.Flags().Lookup(name); f == nil {
			t.Errorf("expected --%s flag on submit batch command", name)
		}
	}
	for _, name := range []string{"name", "work-type-name", "payload", "work-type-id"} {
		if f := batchCmd.Flags().Lookup(name); f != nil {
			t.Fatalf("submit batch should not expose --%s", name)
		}
	}

	var out bytes.Buffer
	root = NewRootCommand()
	root.SetOut(&out)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"submit", "batch", "--help"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute submit batch --help: %v", err)
	}

	help := out.String()
	for _, want := range []string{
		"FACTORY_REQUEST_BATCH",
		"you docs batch-inputs",
		"--file",
		"filesystem path",
		"stdin",
		"inline",
		"--dry-run",
		"--session",
		"--server",
		"--json",
		"--verbose",
		"pipe",
		"shell argument length",
		"file or pipe",
	} {
		if !strings.Contains(help, want) {
			t.Fatalf("submit batch help missing %q:\n%s", want, help)
		}
	}
	for _, disallowed := range []string{"--name", "--work-type-name", "--payload", "--work-type-id"} {
		if strings.Contains(help, disallowed) {
			t.Fatalf("submit batch help should not list %s:\n%s", disallowed, help)
		}
	}
}

func TestSubmitCommand_HelpMentionsBatchSubcommand(t *testing.T) {
	var out bytes.Buffer
	root := NewRootCommand()
	root.SetOut(&out)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"submit", "--help"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute submit --help: %v", err)
	}

	help := out.String()
	for _, want := range []string{
		"submit batch",
		"FACTORY_REQUEST_BATCH",
		"you docs batch-inputs",
	} {
		if !strings.Contains(help, want) {
			t.Fatalf("submit help missing %q:\n%s", want, help)
		}
	}
}

func TestSubmitBatchCommand_InvokesSubmitBatchHandler(t *testing.T) {
	originalSubmitBatch := submitBatch
	defer func() {
		submitBatch = originalSubmitBatch
	}()

	called := false
	submitBatch = func(cfg submitcli.BatchConfig) error {
		called = true
		if cfg.DryRun {
			t.Fatal("dry-run should default false")
		}
		return nil
	}

	root := NewRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"submit", "batch", "batch.json"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute submit batch: %v", err)
	}
	if !called {
		t.Fatal("expected submit batch handler to run")
	}
}

// Run-command server and named-packaged-factory invocation tests (merged from
// root_run_server_test.go for pkg-file-count).

func TestRunCommand_DefaultServerEnablesAutoPortAndLocalBind(t *testing.T) {
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
	if got.Port != 7437 {
		t.Fatalf("port = %d, want 7437", got.Port)
	}
	if got.BindHost != "localhost" {
		t.Fatalf("bind host = %q, want localhost", got.BindHost)
	}
	if !got.AutoPort {
		t.Fatal("expected default --server to enable automatic port resolution")
	}
}

func TestRunCommand_ExplicitServerDerivesBindPortAndDisablesAutoPort(t *testing.T) {
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
	root.SetArgs([]string{"run", "--server", "http://127.0.0.1:9090"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute run --server: %v", err)
	}
	if got.Port != 9090 {
		t.Fatalf("port = %d, want 9090", got.Port)
	}
	if got.BindHost != "127.0.0.1" {
		t.Fatalf("bind host = %q, want 127.0.0.1", got.BindHost)
	}
	if got.AutoPort {
		t.Fatal("expected explicit --server to disable automatic port resolution")
	}
}

func TestRunCommand_NonLocalServerRejected(t *testing.T) {
	root := NewRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"run", "--server", "https://remote.example.com:7443"})

	if execErr := root.Execute(); execErr == nil {
		t.Fatal("expected non-local --server rejection")
	} else if !strings.Contains(execErr.Error(), "not a local bind target") {
		t.Fatalf("error = %v, want local bind guidance", execErr)
	}
}

func TestRunCommand_PortFlagRejected(t *testing.T) {
	root := NewRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"run", "--port", "7437"})

	if execErr := root.Execute(); execErr == nil {
		t.Fatal("expected --port rejection")
	} else if !strings.Contains(execErr.Error(), "--server") {
		t.Fatalf("error = %v, want --server guidance", execErr)
	}
}

func TestRunCommand_RuntimeMetricsFlags(t *testing.T) {
	root := NewRootCommand()
	runCmd, _, err := root.Find([]string{"run"})
	if err != nil {
		t.Fatalf("find run: %v", err)
	}

	defaults := platformmetrics.DefaultRuntimeMetricsConfig()
	tests := []struct {
		name    string
		def     string
		usageIn string
	}{
		{name: "runtime-metrics-dir", def: "", usageIn: "root directory for structured runtime metrics JSONL files grouped by UTC start date"},
		{name: "runtime-metrics-max-size-mb", def: strconv.Itoa(defaults.MaxSize), usageIn: "rotate each runtime metrics file"},
		{name: "runtime-metrics-max-backups", def: strconv.Itoa(defaults.MaxBackups), usageIn: "maximum rotated runtime metrics files"},
		{name: "runtime-metrics-max-age-days", def: strconv.Itoa(defaults.MaxAge), usageIn: "maximum days to retain rotated runtime metrics files"},
		{name: "runtime-metrics-compress", def: "false", usageIn: "compress rotated runtime metrics files"},
	}

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
	if got := runCmd.Flags().Lookup("runtime-metrics-dir").Usage; !strings.Contains(got, "~/.you-agent-factory/metrics") {
		t.Fatalf("--runtime-metrics-dir usage = %q, want canonical default metrics path", got)
	}
	if !strings.Contains(runCmd.Long, "Runtime metrics are a separate structured JSONL operational channel") {
		t.Fatal("expected run command long help text to document separate runtime metrics channel")
	}
}

func TestRunCommand_RuntimeMetricsFlagsMapToRunConfig(t *testing.T) {
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
		"--runtime-metrics-dir", "logs/metrics",
		"--runtime-metrics-max-size-mb", "21",
		"--runtime-metrics-max-backups", "22",
		"--runtime-metrics-max-age-days", "23",
		"--runtime-metrics-compress",
	})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute run with runtime metrics flags: %v", err)
	}

	if got.RuntimeMetricsDir != "logs/metrics" {
		t.Fatalf("runtime metrics dir = %q, want unchanged root logs/metrics", got.RuntimeMetricsDir)
	}
	want := platformmetrics.RuntimeMetricsConfig{MaxSize: 21, MaxBackups: 22, MaxAge: 23, Compress: true}
	if got.RuntimeMetricsConfig != want {
		t.Fatalf("runtime metrics config = %#v, want %#v", got.RuntimeMetricsConfig, want)
	}
}

func withNamedPackagedFactoryRunRoot(t *testing.T) func() {
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
	t.Setenv("HOME", homeDir)
	t.Setenv("USERPROFILE", homeDir)
	volumeName := filepath.VolumeName(homeDir)
	t.Setenv("HOMEDRIVE", volumeName)
	t.Setenv("HOMEPATH", strings.TrimPrefix(homeDir, volumeName))
	if _, err := configinit.Init(homeDir); err != nil {
		t.Fatalf("configinit.Init: %v", err)
	}

	return func() {
		if chdirErr := os.Chdir(originalWorkingDirectory); chdirErr != nil {
			t.Fatalf("restore working directory: %v", chdirErr)
		}
	}
}

func assertNamedPackagedFactoryInvocationInput(
	t *testing.T,
	got runcli.RunConfig,
	factory string,
	wantPositional string,
	wantStdin string,
) {
	t.Helper()

	if got.NamedFactoryName != factory {
		t.Fatalf("named factory = %q, want %q", got.NamedFactoryName, factory)
	}
	if wantPositional != "" {
		if got.InvocationPositionalText == nil {
			t.Fatal("expected invocation positional text")
		}
		if gotText := *got.InvocationPositionalText; gotText != wantPositional {
			t.Fatalf("invocation positional text = %q, want %q", gotText, wantPositional)
		}
		if !got.SuppressDashboardRendering {
			t.Fatal("expected named text invocation to suppress dashboard rendering")
		}
	}
	if wantStdin != "" {
		if got.InvocationPositionalText != nil {
			t.Fatal("expected no invocation positional text for stdin run")
		}
		if got.InvocationStdinText == nil {
			t.Fatal("expected invocation stdin text")
		}
		if gotText := *got.InvocationStdinText; gotText != wantStdin {
			t.Fatalf("invocation stdin text = %q, want %q", gotText, wantStdin)
		}
		if !got.SuppressDashboardRendering {
			t.Fatal("expected named stdin invocation to suppress dashboard rendering")
		}
	}
}

func TestRunCommand_NamedPackagedFactoryInvocationInputSources(t *testing.T) {
	tests := []struct {
		name           string
		factory        string
		stdin          string
		args           []string
		wantPositional string
		wantStdin      string
	}{
		{
			name:           "tts positional",
			factory:        "@you/tts",
			args:           []string{"hi", "there"},
			wantPositional: "hi there",
		},
		{
			name:           "goal positional",
			factory:        "@you/goal",
			args:           []string{"Plan", "the", "sprint"},
			wantPositional: "Plan the sprint",
		},
		{
			name:      "tts piped stdin",
			factory:   "@you/tts",
			stdin:     "hi from stdin\n",
			wantStdin: "hi from stdin\n",
		},
		{
			name:      "goal piped stdin",
			factory:   "@you/goal",
			stdin:     "Ship from stdin\n",
			wantStdin: "Ship from stdin\n",
		},
		{
			name:      "tts explicit stdin",
			factory:   "@you/tts",
			stdin:     "hi from explicit stdin\n",
			args:      []string{"-"},
			wantStdin: "hi from explicit stdin\n",
		},
		{
			name:      "goal explicit stdin",
			factory:   "@you/goal",
			stdin:     "Ship from explicit stdin\n",
			args:      []string{"-"},
			wantStdin: "Ship from explicit stdin\n",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			originalRunCLI := runCLI
			defer func() {
				runCLI = originalRunCLI
			}()
			restore := withNamedPackagedFactoryRunRoot(t)
			defer restore()

			var got runcli.RunConfig
			runCLI = func(_ context.Context, cfg runcli.RunConfig) error {
				got = cfg
				return nil
			}

			root := NewRootCommand()
			if tc.stdin != "" {
				root.SetIn(strings.NewReader(tc.stdin))
			}
			root.SetOut(io.Discard)
			root.SetErr(io.Discard)
			root.SetArgs(append([]string{"run", "--named", tc.factory, "--no-record"}, tc.args...))

			if err := root.Execute(); err != nil {
				t.Fatalf("execute run --named %s: %v", tc.factory, err)
			}
			assertNamedPackagedFactoryInvocationInput(t, got, tc.factory, tc.wantPositional, tc.wantStdin)
		})
	}
}

func TestRunCommand_NamedPackagedFactoryRejectsAmbiguousInvocationInput(t *testing.T) {
	tests := []struct {
		name    string
		factory string
		stdin   string
		args    []string
	}{
		{
			name:    "tts positional and explicit stdin",
			factory: "@you/tts",
			stdin:   "Fix from stdin\n",
			args:    []string{"Fix from args", "-"},
		},
		{
			name:    "goal positional and explicit stdin",
			factory: "@you/goal",
			stdin:   "Plan from stdin\n",
			args:    []string{"Plan from args", "-"},
		},
		{
			name:    "goal positional and piped stdin",
			factory: "@you/goal",
			stdin:   "Plan from piped stdin\n",
			args:    []string{"Plan from args"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			originalRunCLI := runCLI
			defer func() {
				runCLI = originalRunCLI
			}()
			restore := withNamedPackagedFactoryRunRoot(t)
			defer restore()

			runCalled := false
			runCLI = func(context.Context, runcli.RunConfig) error {
				runCalled = true
				return nil
			}

			root := NewRootCommand()
			root.SetIn(strings.NewReader(tc.stdin))
			root.SetOut(io.Discard)
			root.SetErr(io.Discard)
			root.SetArgs(append([]string{"run", "--named", tc.factory, "--no-record"}, tc.args...))

			err := root.Execute()
			if err == nil {
				t.Fatal("expected ambiguous invocation input rejection")
			}
			for _, want := range []string{
				"INVOCATION_INPUT_SOURCE_CONFLICT",
				"positional_text",
				"stdin_text",
			} {
				if !strings.Contains(err.Error(), want) {
					t.Fatalf("error = %q, want %q", err.Error(), want)
				}
			}
			if runCalled {
				t.Fatal("run should not start for ambiguous named factory prompt input")
			}
		})
	}
}
