package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	"github.com/portpowered/infinite-you/pkg/factory/packages/goal"
	modelscli "github.com/portpowered/infinite-you/pkg/transports/cli/models"
	runcli "github.com/portpowered/infinite-you/pkg/transports/cli/run"
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

func assertGoalQuietLeakContractForbidden(t *testing.T, output string) {
	t.Helper()

	for _, forbidden := range goalQuietLeakForbiddenMarkers {
		if strings.Contains(output, forbidden) {
			t.Fatalf("output = %q, want no quiet-leak marker %q", output, forbidden)
		}
	}
}

func assertGoalQuietTerminalMute(t *testing.T, stdout, stderr string) {
	t.Helper()

	if stdout != "" {
		t.Fatalf("stdout = %q, want empty quiet operational-failure terminal output", stdout)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty quiet operational-failure terminal output", stderr)
	}
	assertGoalQuietLeakContractForbidden(t, stdout+stderr)
}

func TestFailureBaseline_QuietLeak_InvalidTopologySuppressesTerminalOnOperationalFailure(t *testing.T) {
	dir := t.TempDir()
	factoryPath := filepath.Join(dir, "factory.json")
	if err := os.WriteFile(factoryPath, []byte(goalFailureBaselineInvalidTopologyJSON), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	var stdout, stderr bytes.Buffer
	root := newComposedTestRootCommand(t)
	root.SetOut(&stdout)
	root.SetErr(&stderr)
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
	assertGoalQuietTerminalMute(t, stdout.String(), stderr.String())
}

func TestFailureBaseline_QuietLeak_RunBatchQuietSuppressesTerminalOnOperationalFailure(t *testing.T) {
	originalRunCLI := runCLI
	defer func() {
		runCLI = originalRunCLI
	}()

	dir := t.TempDir()
	workPath := filepath.Join(dir, "work.json")
	if err := os.WriteFile(workPath, []byte(`{"type":"FACTORY_REQUEST_BATCH","works":[]}`), 0o644); err != nil {
		t.Fatalf("write work file: %v", err)
	}

	runCLI = func(_ context.Context, _ runcli.RunConfig) error {
		return fmt.Errorf("operational failure: batch run rejected")
	}

	var stdout, stderr bytes.Buffer
	root := NewRootCommand()
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{
		"run",
		"--dir", dir,
		"--work", workPath,
		"--no-record",
		"--quiet",
	})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected operational failure")
	}
	if !strings.Contains(err.Error(), "operational failure") {
		t.Fatalf("error = %q, want operational failure returned to caller", err.Error())
	}
	assertGoalQuietTerminalMute(t, stdout.String(), stderr.String())
}

func TestFailureBaseline_QuietLeak_RunFactoryQuietSuppressesTerminalOnInvocationFailure(t *testing.T) {
	originalRunCLI := runCLI
	defer func() {
		runCLI = originalRunCLI
	}()

	dir := t.TempDir()
	factoryPath := writePortableFactoryWithDefaultHandling(t, dir)

	runCLI = func(_ context.Context, _ runcli.RunConfig) error {
		return &runcli.InvocationError{
			Code:    runcli.InvocationErrorCodeFailed,
			Message: "quiet operational failure baseline",
		}
	}

	var stdout, stderr bytes.Buffer
	root := NewRootCommand()
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{
		"run",
		"--factory", factoryPath,
		"--no-record",
		"--quiet",
		"quiet operational failure baseline prompt",
	})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected invocation failure")
	}
	if !strings.Contains(err.Error(), runcli.InvocationErrorCodeFailed) {
		t.Fatalf("error = %q, want stable invocation failure code", err.Error())
	}
	assertGoalQuietTerminalMute(t, stdout.String(), stderr.String())
}

func TestFailureBaseline_NoServer_ModelsInvokeCommandUsesBootstrapInsteadOfUnreachableEndpoint(t *testing.T) {
	originalInvokeModel := invokeModel
	defer func() {
		invokeModel = originalInvokeModel
	}()

	var got modelscli.InvokeConfig
	invokeModel = func(cfg modelscli.InvokeConfig) error {
		got = cfg
		return nil
	}

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

	if err := root.Execute(); err != nil {
		t.Fatalf("execute models invoke: %v", err)
	}
	if got.Logger == nil {
		t.Fatal("expected invoke to receive a logger for bootstrap invoke")
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

func corruptGoalFactoryExecuteOutputStateForTest(t *testing.T, factoryDir, stateName string) {
	t.Helper()

	factoryPath := filepath.Join(factoryDir, interfaces.FactoryConfigFile)
	data, err := os.ReadFile(factoryPath)
	if err != nil {
		t.Fatalf("ReadFile(factory.json): %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal(factory.json): %v", err)
	}
	workstations, ok := raw["workstations"].([]any)
	if !ok || len(workstations) == 0 {
		t.Fatal("factory.json workstations missing")
	}
	var workstation map[string]any
	for _, entry := range workstations {
		candidate, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		if candidate["name"] == goal.PackagedExecuteWorkstationName {
			workstation = candidate
			break
		}
	}
	if workstation == nil {
		t.Fatal("factory.json execute-goal workstation not found")
	}
	outputs, ok := workstation["outputs"].([]any)
	if !ok || len(outputs) == 0 {
		t.Fatal("factory.json workstation outputs missing")
	}
	output, ok := outputs[0].(map[string]any)
	if !ok {
		t.Fatal("factory.json workstation output[0] is not an object")
	}
	output["state"] = stateName
	updated, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		t.Fatalf("Marshal(factory.json): %v", err)
	}
	if err := os.WriteFile(factoryPath, updated, 0o644); err != nil {
		t.Fatalf("WriteFile(factory.json): %v", err)
	}
}

func TestRunNamedGoalResolutionDefersCorruptedFactoryValidationToRuntime(t *testing.T) {
	originalRunCLI := runCLI
	defer func() {
		runCLI = originalRunCLI
	}()

	env := setupNamedGoalCLIEnv(t)
	runCalled := false
	runCLI = func(_ context.Context, cfg runcli.RunConfig) error {
		runCalled = true
		if cfg.NamedFactoryName != goal.PackagedFactoryName {
			t.Fatalf("named factory = %q, want %q", cfg.NamedFactoryName, goal.PackagedFactoryName)
		}
		return nil
	}

	env.root.SetArgs([]string{
		"run",
		"--named", goal.PackagedFactoryName,
		"--no-record",
		"--quiet",
		"invalid-topology-materialized-baseline",
	})
	if err := env.root.Execute(); err != nil {
		t.Fatalf("execute first run --named %s: %v", goal.PackagedFactoryName, err)
	}
	if !runCalled {
		t.Fatal("expected first named goal run to reach runCLI after materialization")
	}

	factoryDir := materializedGoalDir(env.homeDir)
	corruptGoalFactoryExecuteOutputStateForTest(t, factoryDir, "missing-output-state")

	runCalled = false
	env.root.SetArgs([]string{
		"run",
		"--named", goal.PackagedFactoryName,
		"--no-record",
		"--quiet",
		"invalid-topology-upgrade-baseline",
	})
	if err := env.root.Execute(); err != nil {
		t.Fatalf("resolve corrupted installed factory: %v", err)
	}
	if !runCalled {
		t.Fatal("expected read-only named resolution to defer topology validation to runtime")
	}
}

func TestRunNamedGoalResolutionDefersInvalidInstalledTargetValidationToRuntime(t *testing.T) {
	originalRunCLI := runCLI
	defer func() {
		runCLI = originalRunCLI
	}()
	runCalled := false
	runCLI = func(context.Context, runcli.RunConfig) error {
		runCalled = true
		return nil
	}

	env := setupNamedGoalCLIEnv(t)
	factoryDir := materializedGoalDir(env.homeDir)
	if err := os.MkdirAll(factoryDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", factoryDir, err)
	}
	factoryPath := filepath.Join(factoryDir, interfaces.FactoryConfigFile)
	if err := os.WriteFile(factoryPath, []byte(goalFailureBaselineInvalidTopologyJSON), 0o644); err != nil {
		t.Fatalf("WriteFile(%q): %v", factoryPath, err)
	}

	env.root.SetArgs([]string{
		"run",
		"--named", goal.PackagedFactoryName,
		"--no-record",
		"--quiet",
		"invalid-topology-existing-target-baseline",
	})
	if err := env.root.Execute(); err != nil {
		t.Fatalf("resolve invalid installed target: %v", err)
	}
	if !runCalled {
		t.Fatal("expected read-only named resolution to defer topology validation to runtime")
	}
}

func TestFailureBaseline_InvalidTopology_RunFactoryCommandRejectsGoalShapedGraphReferences(t *testing.T) {
	dir := t.TempDir()
	factoryPath := filepath.Join(dir, "factory.json")
	if err := os.WriteFile(factoryPath, []byte(goalFailureBaselineInvalidTopologyJSON), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	root := newComposedTestRootCommand(t)
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
	if !strings.Contains(err.Error(), "Blocking findings:") {
		t.Fatalf("error = %q, want blocking findings section", err.Error())
	}
	if strings.Count(err.Error(), "you factory config validate") != 1 {
		t.Fatalf("error = %q, want exactly one recovery command", err.Error())
	}
}

func TestFailureBaseline_QuietLeak_RunBatchQuietSuppressesStartupChatter(t *testing.T) {
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
	if got.StartupOutput != nil {
		t.Fatal("expected batch quiet run to suppress startup output wiring")
	}
	if got.CleanInvocation {
		t.Fatal("expected dir/work batch quiet run to keep operator startup output mode")
	}
	if stdout.String() != "" {
		t.Fatalf("stdout = %q, want empty quiet success terminal output", stdout.String())
	}
	assertGoalQuietLeakContractForbidden(t, stdout.String())
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
