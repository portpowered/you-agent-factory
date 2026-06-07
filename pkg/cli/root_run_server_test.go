package cli

import (
	"context"
	"io"
	"strconv"
	"strings"
	"testing"

	runcli "github.com/portpowered/infinite-you/pkg/cli/run"
	"github.com/portpowered/infinite-you/pkg/logging"
)

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

	defaults := logging.DefaultRuntimeMetricsConfig()
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
	want := logging.RuntimeMetricsConfig{MaxSize: 21, MaxBackups: 22, MaxAge: 23, Compress: true}
	if got.RuntimeMetricsConfig != want {
		t.Fatalf("runtime metrics config = %#v, want %#v", got.RuntimeMetricsConfig, want)
	}
}
