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
	"github.com/portpowered/infinite-you/pkg/packagedfactories/goal"
)

// Hermetic S02 failure-baseline fixtures for one-shot goal/model CLI paths. Each
// case locks a customer-visible failure contract without live providers or
// external network services beyond local loopback transport failure.

const goalFailureBaselineUnreachableServer = "http://127.0.0.1:1"

const goalFailureBaselineInvalidTopologyJSON = `{
  "name": "@you/goal",
  "workTypes": [{
    "name": "goal",
    "handlingBehavior": ["DEFAULT"],
    "states": [
      {"name": "init", "type": "INITIAL"},
      {"name": "plan", "type": "PROCESSING"},
      {"name": "complete", "type": "TERMINAL"},
      {"name": "failed", "type": "FAILED"}
    ]
  }],
  "workers": [{"name": "goal-planner", "type": "AGENT_WORKER"}],
  "workstations": [{
    "name": "plan-goal",
    "type": "AGENT_RUN",
    "worker": "goal-planner",
    "inputs": [{"workType": "goal", "state": "init"}],
    "outputs": [{"workType": "goal", "state": "missing-plan-state"}],
    "onFailure": [{"workType": "goal", "state": "failed"}]
  }]
}`

var goalQuietLeakForbiddenMarkers = []string{
	"Factory initiated",
	"Dashboard URL",
	"Runtime log",
	"Opening dashboard",
	"Factory:",
	"Recording saved",
}

var goalQuietLeakExpectedMarkers = []string{
	"Factory initiated",
	"Dashboard URL",
	"Runtime log",
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

func assertGoalQuietLeakContractPresent(t *testing.T, output string) {
	t.Helper()

	if strings.TrimSpace(output) == "" {
		t.Fatal("output is empty, want non-empty quiet-leak terminal chatter documenting today's contract")
	}
	for _, marker := range goalQuietLeakExpectedMarkers {
		if strings.Contains(output, marker) {
			return
		}
	}
	t.Fatalf("output = %q, want at least one quiet-leak marker among %v", output, goalQuietLeakExpectedMarkers)
}

func TestFailureBaseline_NoServer_ModelsInvokeCommandReportsUnreachableEndpoint(t *testing.T) {
	outputPath := filepath.Join(t.TempDir(), "speech.wav")
	root := NewRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{
		"models", "invoke", "OMNIVOICE_Q4_K_M",
		"--operation", "TTS",
		"--text", "hello world",
		"--output", outputPath,
		"--server", goalFailureBaselineUnreachableServer,
	})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected unreachable error")
	}
	want := "models endpoint not reachable at http://127.0.0.1:1/models/OMNIVOICE_Q4_K_M/invocations"
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("error = %q, want %q", err.Error(), want)
	}
}

func TestFailureBaseline_NoServer_ModelsListCommandReportsUnreachableEndpoint(t *testing.T) {
	root := NewRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{
		"models", "list",
		"--server", goalFailureBaselineUnreachableServer,
	})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected unreachable error")
	}
	want := "models endpoint not reachable at http://127.0.0.1:1/models"
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("error = %q, want %q", err.Error(), want)
	}
}

func TestFailureBaseline_AbsentDefault_RunCommandRejectsUnresolvedDefaultProvider(t *testing.T) {
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

func TestFailureBaseline_AbsentDefault_RunNamedGoalLeavesOperatorDefaultsEmptyWithoutConfig(t *testing.T) {
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
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{
		"run",
		"--named", goal.PackagedFactoryName,
		"--no-record",
		"--quiet",
		"absent-default baseline probe",
	})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute run --named %s: %v", goal.PackagedFactoryName, err)
	}
	if got.NamedFactoryName != goal.PackagedFactoryName {
		t.Fatalf("named factory = %q, want %q", got.NamedFactoryName, goal.PackagedFactoryName)
	}
	if got.OperatorDefaults.WorkerModelProvider != "" {
		t.Fatalf("operator provider = %q, want empty without configured defaults", got.OperatorDefaults.WorkerModelProvider)
	}
	if got.OperatorDefaults.WorkerModel != "" {
		t.Fatalf("operator model = %q, want empty without configured defaults", got.OperatorDefaults.WorkerModel)
	}
}

func TestFailureBaseline_InvalidTopology_RunFactoryCommandRejectsGoalShapedGraphReferences(t *testing.T) {
	dir := t.TempDir()
	factoryPath := filepath.Join(dir, "factory.json")
	if err := os.WriteFile(factoryPath, []byte(goalFailureBaselineInvalidTopologyJSON), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	root := NewRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{
		"run",
		"--factory", factoryPath,
		"--no-record",
		"--quiet",
		"invalid-topology-baseline",
	})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected invalid goal topology to fail before invocation")
	}
	if !strings.Contains(err.Error(), "invalid graph references") {
		t.Fatalf("error = %q, want invalid graph references guidance", err.Error())
	}
	if !strings.Contains(err.Error(), "blocking validation targets") {
		t.Fatalf("error = %q, want blocking validation target count", err.Error())
	}
}

func TestFailureBaseline_QuietLeak_RunBatchQuietStillEmitsStartupChatter(t *testing.T) {
	originalRunCLI := runCLI
	defer func() {
		runCLI = originalRunCLI
	}()

	dir := t.TempDir()
	workPath := filepath.Join(dir, "work.json")
	if err := os.WriteFile(workPath, []byte(`{"type":"FACTORY_REQUEST_BATCH","works":[]}`), 0o644); err != nil {
		t.Fatalf("write work file: %v", err)
	}

	var got runcli.RunConfig
	runCLI = func(_ context.Context, cfg runcli.RunConfig) error {
		got = cfg
		if cfg.StartupOutput != nil {
			io.WriteString(cfg.StartupOutput, "Factory initiated: "+cfg.Dir+"\n")
			io.WriteString(cfg.StartupOutput, "Dashboard URL: http://127.0.0.1:7437/\n")
		}
		return nil
	}

	var stdout bytes.Buffer
	root := NewRootCommand()
	root.SetOut(&stdout)
	root.SetErr(io.Discard)
	root.SetArgs([]string{
		"run",
		"--dir", dir,
		"--work", workPath,
		"--no-record",
		"--quiet",
	})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute run --dir --work --quiet: %v", err)
	}
	if !got.SuppressDashboardRendering {
		t.Fatal("expected --quiet to suppress dashboard rendering")
	}
	if got.StartupOutput == nil {
		t.Fatal("expected batch quiet run to keep startup output wired to terminal stdout")
	}
	if got.CleanInvocation {
		t.Fatal("expected dir/work batch quiet run to keep operator startup output mode")
	}
	assertGoalQuietLeakContractPresent(t, stdout.String())
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

func TestFailureBaseline_NamedPath_RunNamedMissingLocalFactoryRejectsBeforeInvocation(t *testing.T) {
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
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{
		"run",
		"--named", "missing-alpha",
		"--no-record",
		"--quiet",
		"named-path baseline prompt",
	})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected missing named factory to fail before invocation")
	}
	if !strings.Contains(err.Error(), `resolve named factory "missing-alpha"`) {
		t.Fatalf("error = %q, want named-path resolution guidance", err.Error())
	}
	if !strings.Contains(err.Error(), `named factory "missing-alpha" not found`) {
		t.Fatalf("error = %q, want named factory not found guidance", err.Error())
	}
	if runCalled {
		t.Fatal("run should not start for unresolved named factory path")
	}
}

func TestFailureBaseline_NamedPath_RunNamedGoalSurfacesPercentEncodedFactoryDir(t *testing.T) {
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
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{
		"run",
		"--named", goal.PackagedFactoryName,
		"--no-record",
		"--quiet",
		"percent-encoded-path baseline probe",
	})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute run --named %s: %v", goal.PackagedFactoryName, err)
	}
	if got.NamedFactoryName != goal.PackagedFactoryName {
		t.Fatalf("named factory = %q, want %q", got.NamedFactoryName, goal.PackagedFactoryName)
	}
	if got.Dir == "" {
		t.Fatal("expected resolved factory directory on run config")
	}
	if !strings.Contains(got.Dir, filepath.Join("@you", "goal")) {
		t.Fatalf("run dir = %q, want hierarchical @you/goal layout", got.Dir)
	}
	if got.NamedFactoryResolution == nil {
		t.Fatal("expected named-factory resolution metadata")
	}
	if got.Dir != got.NamedFactoryResolution.FactoryDir {
		t.Fatalf("run dir = %q, want resolved factory dir %q", got.Dir, got.NamedFactoryResolution.FactoryDir)
	}
	if !strings.Contains(got.NamedFactoryResolution.FactoryDir, filepath.Join("@you", "goal")) {
		t.Fatalf("resolution factory dir = %q, want hierarchical @you/goal layout", got.NamedFactoryResolution.FactoryDir)
	}
	if filepath.Base(got.Dir) != "goal" {
		t.Fatalf("run dir base = %q, want goal hierarchical leaf", filepath.Base(got.Dir))
	}
	if !strings.Contains(got.Dir, filepath.Join(".you-agent-factory", "you-agent-factories")) {
		t.Fatalf("run dir = %q, want global named-factory root layout", got.Dir)
	}
}

func TestFailureBaseline_NamedPath_RunNamedUnknownBuiltInGoalStyleNameRejectsBeforeInvocation(t *testing.T) {
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
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{
		"run",
		"--named", "@you/missing",
		"--no-record",
		"--quiet",
		"named-path baseline prompt",
	})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected unknown built-in named factory to fail before invocation")
	}
	if !strings.Contains(err.Error(), `resolve named factory "@you/missing"`) {
		t.Fatalf("error = %q, want built-in named-path resolution guidance", err.Error())
	}
	if !strings.Contains(err.Error(), "project root") || !strings.Contains(err.Error(), "global root") {
		t.Fatalf("error = %q, want cross-root named-path context", err.Error())
	}
	if runCalled {
		t.Fatal("run should not start for unknown built-in named factory path")
	}
}
