package gemini

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestRootBuiltProcessExecutesThroughSharedSupport proves the shared harness executes a root-built process.
func TestRootBuiltProcessExecutesThroughSharedSupport(t *testing.T) {
	process := support.BuildProcess(t, serviceedges.Edges{})
	inputs := support.FakeInputs(t.Context(), []string{"you", "--help"})

	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf("Process.Execute(--help) error = %v\nstderr:\n%s", err, inputs.Stderr())
	}
	if !strings.Contains(inputs.Stdout(), "Run and manage") {
		t.Fatalf("Process.Execute(--help) stdout = %q, want root command help", inputs.Stdout())
	}
}

// TestGeminiConductorSuccessThroughRootBuildProcess proves successful Gemini execution through the product graph.
func TestGeminiConductorSuccessThroughRootBuildProcess(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	support.WriteAgentConfig(t, dir, "worker", support.BuildModelWorkerConfig(
		modelprovider.ProviderGemini,
		"gemini-2.5-flash",
	))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"gemini conductor success"}`))

	runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{
		Stdout: []byte("gemini functional answer COMPLETE"),
	})

	_, listed, _ := support.RunFactoryToCompletionWithEdgesAndObservations(t, dir, serviceedges.Edges{
		ProviderCommandRunner: runner,
	}, 20*time.Second)

	if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 1 {
		t.Fatalf("terminal place tokens = %d, want 1 completed work item; listed=%#v", got, listed)
	}
	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 0 {
		t.Fatalf("failed place tokens = %d, want 0", got)
	}
	if runner.CallCount() != 1 {
		t.Fatalf("gemini command runner calls = %d, want 1 through conductor path", runner.CallCount())
	}
	request := runner.LastRequest()
	if request.Command != "gemini" {
		t.Fatalf("command = %q, want gemini (conductor-selected built-in)", request.Command)
	}
	if !containsArgPair(request.Args, "--model", "gemini-2.5-flash") {
		t.Fatalf("args = %#v, want --model gemini-2.5-flash", request.Args)
	}
}

// TestGeminiClassifierRejectsStructuredLabelThroughRootBuildProcess proves unsupported structured labels fail at the boundary.
func TestGeminiClassifierRejectsStructuredLabelThroughRootBuildProcess(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	configureClassifierFixture(t, dir)
	support.WriteWorkstationConfig(t, dir, "process", `---
type: CLASSIFIER_WORKSTATION
---
Classify the work.
`)
	workerConfig := strings.Replace(
		support.BuildModelWorkerConfig(modelprovider.ProviderGemini, "gemini-2.5-flash"),
		"stopToken: COMPLETE\n",
		"",
		1,
	)
	support.WriteAgentConfig(t, dir, "worker", workerConfig)
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"gemini classifier"}`))

	runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{Stdout: []byte(`{"label":"approved"}`)})
	_, listed, _ := support.RunFactoryToCompletionWithEdgesAndObservations(t, dir, serviceedges.Edges{
		ProviderCommandRunner: runner,
	}, 20*time.Second)

	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 1 {
		t.Fatalf("failed place tokens = %d, want 1 invalid classifier result; listed=%#v", got, listed)
	}
	if runner.CallCount() != 1 {
		t.Fatalf("gemini command runner calls = %d, want 1", runner.CallCount())
	}
}

// TestGeminiConductorPreservesConfiguredEnvironment proves configured environment reaches Gemini execution.
func TestGeminiConductorPreservesConfiguredEnvironment(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	support.WriteAgentConfig(t, dir, "worker", support.BuildModelWorkerConfig(
		modelprovider.ProviderGemini,
		"gemini-2.5-flash",
	))
	support.WriteWorkstationConfig(t, dir, "process", `---
type: MODEL_WORKSTATION
env:
  GEMINI_CONTEXT_FIXTURE: configured
---
Test workstation.
`)
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"gemini conductor context"}`))

	runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{
		Stdout: []byte("gemini context answer COMPLETE"),
	})
	_, listed, _ := support.RunFactoryToCompletionWithEdgesAndObservations(t, dir, serviceedges.Edges{
		ProviderCommandRunner: runner,
	}, 20*time.Second)

	if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 1 {
		t.Fatalf("terminal place tokens = %d, want 1 completed work item; listed=%#v", got, listed)
	}
	request := runner.LastRequest()
	if !containsEnv(request.Env, "GEMINI_CONTEXT_FIXTURE=configured") {
		t.Fatalf("command environment omitted configured Gemini context")
	}
}

// TestGeminiRejectsUnsupportedWorkingDirectoryBeforeProviderIO proves invalid working directories fail before provider IO.
func TestGeminiRejectsUnsupportedWorkingDirectoryBeforeProviderIO(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	support.WriteAgentConfig(t, dir, "worker", support.BuildModelWorkerConfig(
		modelprovider.ProviderGemini,
		"gemini-2.5-flash",
	))
	support.WriteWorkstationConfig(t, dir, "process", `---
type: MODEL_WORKSTATION
workingDirectory: .
---
Test workstation.
`)
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"gemini rejects working directory"}`))

	runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{
		Stdout: []byte("provider must not run"),
	})
	_, listed, _ := support.RunFactoryToCompletionWithEdgesAndObservations(t, dir, serviceedges.Edges{
		ProviderCommandRunner: runner,
	}, 20*time.Second)

	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 1 {
		t.Fatalf("failed place tokens = %d, want 1 unsupported-capability failure; listed=%#v", got, listed)
	}
	if runner.CallCount() != 0 {
		t.Fatalf("gemini command runner calls = %d, want no provider I/O", runner.CallCount())
	}
}

// TestGeminiRejectsUnsupportedStructuredOutputBeforeProviderIO proves unsupported output fails before provider IO.
func TestGeminiRejectsUnsupportedStructuredOutputBeforeProviderIO(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	support.WriteAgentConfig(t, dir, "worker", support.BuildModelWorkerConfig(
		modelprovider.ProviderGemini,
		"gemini-2.5-flash",
	))
	support.WriteWorkstationConfig(t, dir, "process", `---
type: MODEL_WORKSTATION
outputSchema: '{}'
worktree: .
---
Test workstation.
`)
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"gemini rejects structured output"}`))

	runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{
		Stdout: []byte("provider must not run"),
	})
	_, listed, _ := support.RunFactoryToCompletionWithEdgesAndObservations(t, dir, serviceedges.Edges{
		ProviderCommandRunner: runner,
	}, 20*time.Second)

	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 1 {
		t.Fatalf("failed place tokens = %d, want 1 unsupported-capability failure; listed=%#v", got, listed)
	}
	if runner.CallCount() != 0 {
		t.Fatalf("gemini command runner calls = %d, want no provider I/O", runner.CallCount())
	}
}

// TestGeminiConductorPreservesConfiguredSkipPermissions proves permission policy reaches Gemini execution.
func TestGeminiConductorPreservesConfiguredSkipPermissions(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	workerConfig := strings.Replace(
		support.BuildModelWorkerConfig(modelprovider.ProviderGemini, "gemini-2.5-flash"),
		"stopToken: COMPLETE",
		"skipPermissions: true\nstopToken: COMPLETE",
		1,
	)
	support.WriteAgentConfig(t, dir, "worker", workerConfig)
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"gemini skip permissions"}`))

	runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{
		Stdout: []byte("gemini policy answer COMPLETE"),
	})
	_, listed, _ := support.RunFactoryToCompletionWithEdgesAndObservations(t, dir, serviceedges.Edges{
		ProviderCommandRunner: runner,
	}, 20*time.Second)

	if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 1 {
		t.Fatalf("terminal place tokens = %d, want 1 completed work item; listed=%#v", got, listed)
	}
	args := runner.LastRequest().Args
	if !containsArgPair(args, "--approval-mode", "yolo") ||
		!containsArgPair(args, "--sandbox", "false") {
		t.Fatalf("args = %#v, want configured Gemini skip-permissions flags", args)
	}
}

// TestGeminiNativeFailureThroughRootBuildProcessIsSafe proves native provider failures return safely.
func TestGeminiNativeFailureThroughRootBuildProcessIsSafe(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	support.WriteAgentConfig(t, dir, "worker", support.BuildModelWorkerConfig(
		modelprovider.ProviderGemini,
		"gemini-2.5-flash",
	))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"gemini native failure"}`))

	// Use a non-retryable auth failure so the factory reaches a single terminal
	// failure without provider/workstation retry amplification.
	const leaked = "/tmp/secret-key"
	runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{
		ExitCode: 1,
		Stderr:   []byte(`{"error":{"status":"UNAUTHENTICATED","message":"token path ` + leaked + ` leaked"}}`),
	})

	_, listed, events := support.RunFactoryToCompletionWithEdgesAndObservations(t, dir, serviceedges.Edges{
		ProviderCommandRunner: runner,
	}, 20*time.Second)

	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 1 {
		t.Fatalf("failed place tokens = %d, want 1; listed=%#v", got, listed)
	}
	if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 0 {
		t.Fatalf("done place tokens = %d, want 0 after native failure", got)
	}
	if runner.CallCount() != 1 {
		t.Fatalf("gemini command runner calls = %d, want 1", runner.CallCount())
	}
	if request := runner.LastRequest(); request.Command != "gemini" {
		t.Fatalf("command = %q, want gemini", request.Command)
	}

	encoded, err := json.Marshal(events)
	if err != nil {
		t.Fatalf("marshal factory events: %v", err)
	}
	payload := string(encoded)
	if strings.Contains(payload, leaked) ||
		strings.Contains(payload, "secret-key") {
		t.Fatalf("factory events leaked unsafe Gemini failure detail: %s", payload)
	}
}

// TestGeminiCommandCancellationThroughRootBuildProcessIsCanonical proves cancellation returns the canonical outcome.
func TestGeminiCommandCancellationThroughRootBuildProcessIsCanonical(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	support.WriteAgentConfig(t, dir, "worker", support.BuildModelWorkerConfig(
		modelprovider.ProviderGemini,
		"gemini-2.5-flash",
	))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"gemini command error"}`))

	runner := &commandCancellationRunner{}
	_, listed, events := support.RunFactoryToCompletionWithEdgesAndObservations(t, dir, serviceedges.Edges{
		ProviderCommandRunner: runner,
	}, 20*time.Second)

	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 1 {
		t.Fatalf("failed place tokens = %d, want 1; listed=%#v", got, listed)
	}
	if runner.calls != 1 {
		t.Fatalf("gemini command runner calls = %d, want 1", runner.calls)
	}
	encoded, err := json.Marshal(events)
	if err != nil {
		t.Fatalf("marshal factory events: %v", err)
	}
	if strings.Contains(string(encoded), "Gemini command did not complete successfully") {
		t.Fatalf("factory events used Gemini-local cancellation fallback: %s", encoded)
	}
}

type commandCancellationRunner struct {
	calls int
}

func configureClassifierFixture(t *testing.T, dir string) {
	t.Helper()
	path := filepath.Join(dir, "factory.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read classifier fixture: %v", err)
	}
	var config map[string]any
	if err := json.Unmarshal(raw, &config); err != nil {
		t.Fatalf("decode classifier fixture: %v", err)
	}
	workstations := config["workstations"].([]any)
	workstation := workstations[0].(map[string]any)
	workstation["type"] = "CLASSIFIER_WORKSTATION"
	delete(workstation, "outputs")
	workstation["classificationRoutes"] = []map[string]any{{
		"label": "approved",
		"outputs": []map[string]any{{
			"state": "done", "workType": "task",
		}},
	}}
	updated, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("encode classifier fixture: %v", err)
	}
	if err := os.WriteFile(path, updated, 0o600); err != nil {
		t.Fatalf("write classifier fixture: %v", err)
	}
}

func (r *commandCancellationRunner) Run(context.Context, platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	r.calls++
	return platformprocess.CommandResult{}, context.Canceled
}

func containsArgPair(args []string, flag, value string) bool {
	for index := 0; index+1 < len(args); index++ {
		if args[index] == flag && args[index+1] == value {
			return true
		}
	}
	return false
}

func containsEnv(env []string, expected string) bool {
	for _, value := range env {
		if value == expected {
			return true
		}
	}
	return false
}
