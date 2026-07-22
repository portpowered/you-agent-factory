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

	startupcli "github.com/portpowered/infinite-you/pkg/initializer/process"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	modelscli "github.com/portpowered/infinite-you/pkg/services/models/transports/cli"
	"github.com/portpowered/infinite-you/pkg/services/work"
	runcli "github.com/portpowered/infinite-you/pkg/transports/cli/run"
	"github.com/spf13/cobra"
)

// Hermetic S02 failure-baseline fixtures for one-shot goal/model CLI paths. Each
// case locks a customer-visible failure contract without live providers or
// external network services beyond local loopback transport failure.

const goalFailureBaselineUnreachableServer = "http://127.0.0.1:1"
const packagedGoalFactoryName = "@you/goal"
const packagedGoalExecuteWorkstationName = "execute-goal"

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

const goalFailureBaselineNamedFactoryJSON = `{
  "name": "@you/goal",
  "workTypes": [{
    "name": "goal",
    "handlingBehavior": ["DEFAULT"],
    "states": [
      {"name": "init", "type": "INITIAL"},
      {"name": "complete", "type": "TERMINAL"},
      {"name": "failed", "type": "FAILED"}
    ]
  }],
  "workers": [{"name": "goal-executor", "type": "AGENT_WORKER"}],
  "workstations": [{
    "name": "execute-goal",
    "type": "AGENT_RUN",
    "worker": "goal-executor",
    "inputs": [{"workType": "goal", "state": "init"}],
    "outputs": [{"workType": "goal", "state": "complete"}],
    "onFailure": [{"workType": "goal", "state": "failed"}]
  }]
}`

type goalFailureNamedFactoryCatalog struct {
	factoryDir string
}

func (catalog goalFailureNamedFactoryCatalog) ListNamedFactories(string) ([]interfaces.NamedFactoryListEntry, error) {
	return []interfaces.NamedFactoryListEntry{{
		Name:       packagedGoalFactoryName,
		FactoryDir: catalog.factoryDir,
	}}, nil
}

func (goalFailureNamedFactoryCatalog) DeleteNamedFactory(string, string) error {
	return nil
}

func (catalog goalFailureNamedFactoryCatalog) ResolveNamedFactoryAcrossRoots(
	projectRoot string,
	globalRoot string,
	name string,
) (*interfaces.NamedFactoryResolution, error) {
	if name != packagedGoalFactoryName {
		return nil, fmt.Errorf(
			"resolve named factory %q in project root %s or global root %s: named factory %q not found",
			name,
			projectRoot,
			globalRoot,
			name,
		)
	}
	return &interfaces.NamedFactoryResolution{
		Name:               name,
		FactoryDir:         catalog.factoryDir,
		Source:             interfaces.NamedFactoryResolutionSourceGlobal,
		ProjectRoot:        projectRoot,
		GlobalRoot:         globalRoot,
		PrecedenceDecision: interfaces.NamedFactoryPrecedenceDecisionNone,
	}, nil
}

type goalFailureNamedRunEnvironment struct {
	homeDir    string
	factoryDir string
	root       *cobra.Command
}

func newGoalFailureNamedRunEnvironment(t *testing.T) goalFailureNamedRunEnvironment {
	return newGoalFailureNamedRunEnvironmentWithInvocation(t, rootInvocationInputScript{})
}

func newGoalFailureNamedRunEnvironmentWithInvocation(
	t *testing.T,
	prepare rootInvocationInputScript,
) goalFailureNamedRunEnvironment {
	t.Helper()

	homeDir := t.TempDir()
	factoryDir := filepath.Join(homeDir, ".you-agent-factory", "you-agent-factories", "@you", "goal")
	if err := os.MkdirAll(factoryDir, 0o755); err != nil {
		t.Fatalf("MkdirAll named Factory fixture: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(factoryDir, interfaces.FactoryConfigFile),
		[]byte(goalFailureBaselineNamedFactoryJSON),
		0o644,
	); err != nil {
		t.Fatalf("write named Factory fixture: %v", err)
	}
	factory := withTestInjectedPlatformRoles(CommandFactory{
		namedFactoryCatalog: goalFailureNamedFactoryCatalog{factoryDir: factoryDir},
	})
	if prepare.prepare != nil {
		factory.prepareInvocationInput = prepare
	}
	root := factory.NewCommand(
		func() (string, error) { return homeDir, nil },
		os.LookupEnv,
		startupcli.Functions{
			RunFunc: func(ctx context.Context, _ startupcli.RunIntent, selection startupcli.RunSelection) error {
				return runCLI(ctx, testRunConfig(selection))
			},
		},
	)
	root.SetContext(startupcli.WithWorkingDirectory(context.Background(), t.TempDir()))
	return goalFailureNamedRunEnvironment{
		homeDir:    homeDir,
		factoryDir: factoryDir,
		root:       root,
	}
}

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

// These two cases are owner-local transport invariants: the injected Run
// operation is the causal seam under test, so they intentionally use the
// owning CommandFactory rather than pretending an observation edge authored
// the operational failure.
func TestFailureBaseline_QuietLeak_RunBatchQuietSuppressesTerminalOnOperationalFailure(t *testing.T) {
	originalRunCLI := runCLI
	defer func() { runCLI = originalRunCLI }()

	dir := t.TempDir()
	workPath := filepath.Join(dir, "work.json")
	if err := os.WriteFile(workPath, []byte(`{"type":"FACTORY_REQUEST_BATCH","works":[]}`), 0o644); err != nil {
		t.Fatalf("write work file: %v", err)
	}
	runCLI = func(_ context.Context, _ runcli.RunConfig) error {
		return fmt.Errorf("operational failure: batch run rejected")
	}

	var stdout, stderr bytes.Buffer
	root := newLegacyTestRootCommand()
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"run", "--dir", dir, "--work", workPath, "--no-record", "--quiet"})
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
	defer func() { runCLI = originalRunCLI }()

	dir := t.TempDir()
	factoryPath := writePortableFactoryWithDefaultHandling(t, dir)
	runCLI = func(_ context.Context, _ runcli.RunConfig) error {
		return &runcli.InvocationError{
			Code: runcli.InvocationErrorCodeFailed, Message: "quiet operational failure baseline",
		}
	}

	var stdout, stderr bytes.Buffer
	root := newLegacyTestRootCommand()
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{
		"run", "--factory", factoryPath, "--no-record", "--quiet", "quiet operational failure baseline prompt",
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
	root := newLegacyTestRootCommand()
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

func TestFailureBaseline_AbsentDefault_RunNamedGoalLeavesOperatorDefaultsEmptyWithoutConfig(t *testing.T) {
	originalRunCLI := runCLI
	defer func() {
		runCLI = originalRunCLI
	}()
	env := newGoalFailureNamedRunEnvironment(t)

	var got runcli.RunConfig
	runCLI = func(_ context.Context, cfg runcli.RunConfig) error {
		got = cfg
		return nil
	}

	root := env.root
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{
		"run",
		"--named", packagedGoalFactoryName,
		"--no-record",
		"--quiet",
		"absent-default baseline probe",
	})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute run --named %s: %v", packagedGoalFactoryName, err)
	}
	if got.NamedFactoryName != packagedGoalFactoryName {
		t.Fatalf("named factory = %q, want %q", got.NamedFactoryName, packagedGoalFactoryName)
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
		if candidate["name"] == packagedGoalExecuteWorkstationName {
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

	env := newGoalFailureNamedRunEnvironment(t)
	runCalled := false
	runCLI = func(_ context.Context, cfg runcli.RunConfig) error {
		runCalled = true
		if cfg.NamedFactoryName != packagedGoalFactoryName {
			t.Fatalf("named factory = %q, want %q", cfg.NamedFactoryName, packagedGoalFactoryName)
		}
		return nil
	}

	env.root.SetArgs([]string{
		"run",
		"--named", packagedGoalFactoryName,
		"--no-record",
		"--quiet",
		"invalid-topology-materialized-baseline",
	})
	if err := env.root.Execute(); err != nil {
		t.Fatalf("execute first run --named %s: %v", packagedGoalFactoryName, err)
	}
	if !runCalled {
		t.Fatal("expected first named goal run to reach runCLI after materialization")
	}

	factoryDir := env.factoryDir
	corruptGoalFactoryExecuteOutputStateForTest(t, factoryDir, "missing-output-state")

	runCalled = false
	env.root.SetArgs([]string{
		"run",
		"--named", packagedGoalFactoryName,
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

	env := newGoalFailureNamedRunEnvironment(t)
	factoryDir := env.factoryDir
	if err := os.MkdirAll(factoryDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", factoryDir, err)
	}
	factoryPath := filepath.Join(factoryDir, interfaces.FactoryConfigFile)
	if err := os.WriteFile(factoryPath, []byte(goalFailureBaselineInvalidTopologyJSON), 0o644); err != nil {
		t.Fatalf("WriteFile(%q): %v", factoryPath, err)
	}

	env.root.SetArgs([]string{
		"run",
		"--named", packagedGoalFactoryName,
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
	root := newLegacyTestRootCommand()
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
	root := newLegacyTestRootCommand()
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
	env := newGoalFailureNamedRunEnvironmentWithInvocation(t, programmedTextInvocationInput(
		work.InputSourcePositionalText,
		"quiet-leak baseline goal prompt",
	))

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
	root := env.root
	root.SetOut(&stdout)
	root.SetErr(io.Discard)
	root.SetArgs([]string{
		"run",
		"--named", packagedGoalFactoryName,
		"--no-record",
		"--quiet",
		"quiet-leak baseline goal prompt",
	})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute run --named %s --quiet: %v", packagedGoalFactoryName, err)
	}
	if got.NamedFactoryName != packagedGoalFactoryName {
		t.Fatalf("named factory = %q, want %q", got.NamedFactoryName, packagedGoalFactoryName)
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

func TestFailureBaseline_NamedPath_RunNamedGoalSurfacesPercentEncodedFactoryDir(t *testing.T) {
	originalRunCLI := runCLI
	defer func() {
		runCLI = originalRunCLI
	}()

	env := newGoalFailureNamedRunEnvironment(t)

	var got runcli.RunConfig
	runCLI = func(_ context.Context, cfg runcli.RunConfig) error {
		got = cfg
		return nil
	}

	root := env.root
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{
		"run",
		"--named", packagedGoalFactoryName,
		"--no-record",
		"--quiet",
		"percent-encoded-path baseline probe",
	})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute run --named %s: %v", packagedGoalFactoryName, err)
	}
	if got.NamedFactoryName != packagedGoalFactoryName {
		t.Fatalf("named factory = %q, want %q", got.NamedFactoryName, packagedGoalFactoryName)
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
	if !strings.Contains(got.Dir, filepath.Join(".you-agent-factory", "factories")) {
		t.Fatalf("run dir = %q, want global named-factory root layout", got.Dir)
	}
}
