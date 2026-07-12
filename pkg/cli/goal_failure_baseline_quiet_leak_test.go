package cli

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	runcli "github.com/portpowered/infinite-you/pkg/cli/run"
	"github.com/portpowered/infinite-you/pkg/packagedfactories/goal"
)

// Hermetic S02 failure-baseline fixtures for one-shot you run --factory and
// you run --named @you/goal paths with --quiet suppression enabled.

var goalQuietLeakForbiddenMarkers = []string{
	"Factory initiated",
	"Dashboard URL",
	"Runtime log",
	"Opening dashboard",
	"Factory:",
	"Recording saved",
}

func assertGoalQuietLeakContractForbidden(t *testing.T, output string) {
	t.Helper()

	for _, forbidden := range goalQuietLeakForbiddenMarkers {
		if strings.Contains(output, forbidden) {
			t.Fatalf("output = %q, want no quiet-leak marker %q", output, forbidden)
		}
	}
}

func TestFailureBaseline_QuietLeak_RunFactoryQuietPromptKeepsStartupOutputSuppressed(t *testing.T) {
	originalRunCLI := runCLI
	defer func() {
		runCLI = originalRunCLI
	}()

	dir := t.TempDir()
	factoryPath := writePortableFactoryWithDefaultHandling(t, dir)

	var got runcli.RunConfig
	runCLI = func(_ context.Context, cfg runcli.RunConfig) error {
		got = cfg
		if cfg.StartupOutput != nil {
			io.WriteString(cfg.StartupOutput, "Factory initiated: unexpected\n")
			io.WriteString(cfg.StartupOutput, "Dashboard URL: unexpected\n")
			io.WriteString(cfg.StartupOutput, "Recording saved: unexpected\n")
		}
		return nil
	}

	var stdout bytes.Buffer
	root := NewRootCommand()
	root.SetOut(&stdout)
	root.SetErr(io.Discard)
	root.SetArgs([]string{
		"run",
		"--factory", factoryPath,
		"--no-record",
		"--quiet",
		"quiet-leak baseline prompt",
	})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute run --factory --quiet: %v", err)
	}
	if !got.SuppressDashboardRendering {
		t.Fatal("expected --quiet to suppress dashboard rendering")
	}
	if got.StartupOutput != nil {
		t.Fatalf("startup output = %#v, want nil for quiet one-shot factory prompt", got.StartupOutput)
	}
	assertGoalQuietLeakContractForbidden(t, stdout.String())
}

func TestFailureBaseline_QuietLeak_RunNamedGoalQuietBatchSuppressesOperatorChatter(t *testing.T) {
	originalRunCLI := runCLI
	defer func() {
		runCLI = originalRunCLI
	}()
	restore := withNamedPackagedFactoryRunRoot(t)
	defer restore()

	var got runcli.RunConfig
	runCLI = func(_ context.Context, cfg runcli.RunConfig) error {
		got = cfg
		if cfg.StartupOutput != nil {
			io.WriteString(cfg.StartupOutput, "Factory initiated: unexpected\n")
			io.WriteString(cfg.StartupOutput, "Dashboard URL: unexpected\n")
		}
		if cfg.Output != nil {
			io.WriteString(cfg.Output, "goal quiet baseline primary result\n")
		}
		return nil
	}

	var stdout bytes.Buffer
	root := NewRootCommand()
	root.SetOut(&stdout)
	root.SetErr(io.Discard)
	root.SetArgs([]string{
		"run",
		"--named", goal.PackagedFactoryName,
		"--no-record",
		"--quiet",
		"quiet-leak baseline goal prompt",
	})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute run --named %s --quiet: %v", goal.PackagedFactoryName, err)
	}
	if got.NamedFactoryName != goal.PackagedFactoryName {
		t.Fatalf("named factory = %q, want %q", got.NamedFactoryName, goal.PackagedFactoryName)
	}
	if !got.SuppressDashboardRendering {
		t.Fatal("expected named goal quiet batch run to suppress dashboard rendering")
	}
	if got.StartupOutput != nil {
		t.Fatalf("startup output = %#v, want nil for quiet named goal invocation", got.StartupOutput)
	}
	if got := stdout.String(); got != "goal quiet baseline primary result\n" {
		t.Fatalf("stdout = %q, want only primary invocation output", got)
	}
	assertGoalQuietLeakContractForbidden(t, stdout.String())
}
