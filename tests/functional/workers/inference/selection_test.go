package inference_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestExplicitProviderAndModelReachSelectedProviderEdge proves that when a worker
// declares an explicit provider and model, root.BuildProcess dispatch invokes the
// matching provider command edge, completes factory dispatch through that edge,
// and does not invoke a different provider command for the same work.
func TestExplicitProviderAndModelReachSelectedProviderEdge(t *testing.T) {
	const (
		explicitModel = "explicit-selection-model"
	)

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	writeExplicitSelectionWorker(t, dir, string(modelprovider.ProviderCodex), explicitModel)
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"explicit provider selection"}`))

	runner := support.NewShapedProviderCommandRunner(platformprocess.CommandResult{
		Stdout: []byte("structured progress COMPLETE"),
	})

	_, listed, _ := support.RunFactoryToCompletionWithEdgesAndObservations(
		t,
		dir,
		serviceedges.Edges{ProviderCommandRunner: runner},
		20*time.Second,
	)

	if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 1 {
		t.Fatalf("terminal place tokens = %d, want 1 completed work item", got)
	}
	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 0 {
		t.Fatalf("failed place tokens = %d, want 0", got)
	}

	if got := runner.CallCount(); got != 1 {
		t.Fatalf("provider command calls = %d, want exactly one selected-provider invocation", got)
	}
	request := runner.LastRequest()
	if request.Command != string(modelprovider.ProviderCodex) {
		t.Fatalf("provider command = %q, want selected provider %q", request.Command, modelprovider.ProviderCodex)
	}
	support.AssertArgsContainSequence(t, request.Args, []string{"--model", explicitModel})
}

// TestWorkerProviderOverridesGlobalDefault proves that when a global default
// provider is configured, a worker-authored modelProvider dispatches through
// the worker provider command edge instead of the global default command.
func TestWorkerProviderOverridesGlobalDefault(t *testing.T) {
	const (
		workerModel        = "worker-override-model"
		globalDefaultModel = "global-default-model"
	)

	t.Setenv(operatorsettings.EnvDefaultWorkerModelProvider, string(modelprovider.ProviderClaude))
	t.Setenv(operatorsettings.EnvDefaultWorkerModel, globalDefaultModel)

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	writeExplicitSelectionWorker(t, dir, string(modelprovider.ProviderCodex), workerModel)
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"worker provider overrides global default"}`))

	runner := support.NewShapedProviderCommandRunner(platformprocess.CommandResult{
		Stdout: []byte("structured progress COMPLETE"),
	})

	_, listed, _ := support.RunFactoryToCompletionWithEdgesAndObservations(
		t,
		dir,
		serviceedges.Edges{ProviderCommandRunner: runner},
		20*time.Second,
	)

	if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 1 {
		t.Fatalf("terminal place tokens = %d, want 1 completed work item", got)
	}
	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 0 {
		t.Fatalf("failed place tokens = %d, want 0", got)
	}

	if got := runner.CallCount(); got != 1 {
		t.Fatalf("provider command calls = %d, want exactly one worker-provider invocation", got)
	}
	request := runner.LastRequest()
	if request.Command != string(modelprovider.ProviderCodex) {
		t.Fatalf("provider command = %q, want worker provider %q", request.Command, modelprovider.ProviderCodex)
	}
	support.AssertArgsContainSequence(t, request.Args, []string{"--model", workerModel})
}

// TestUnknownProviderFailsBeforeProcessStart proves that when a worker names an
// unregistered provider alias, root.BuildProcess construction leaves the
// provider command edge inert and factory startup fails with a stable
// validation error before any provider invoke or customer process lifecycle
// starts.
func TestUnknownProviderFailsBeforeProcessStart(t *testing.T) {
	const (
		unknownProviderAlias = "unknown-provider"
		unknownModel         = "unknown-model"
	)

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	writeExplicitSelectionWorker(t, dir, unknownProviderAlias, unknownModel)

	runner := support.NewShapedProviderCommandRunner()
	process := support.BuildProcess(t, serviceedges.Edges{ProviderCommandRunner: runner})

	inputs := support.FakeInputs(t.Context(), []string{
		"you", "run",
		"--dir", dir,
		"--continuously",
		"--with-server",
		"--server", "http://127.0.0.1:1",
		"--quiet",
		"--no-record",
	})
	homeDir := t.TempDir()
	inputs.Input.Env = append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)
	inputs.Input.WorkingDirectory = dir

	executeErr := process.Execute(inputs.Input)
	if executeErr == nil {
		t.Fatalf(
			"Process.Execute(run unknown provider) error = nil, want validation failure; stdout=%q stderr=%q",
			inputs.Stdout(),
			inputs.Stderr(),
		)
	}
	errorText := executeErr.Error()
	if !strings.Contains(errorText, `provider "`+unknownProviderAlias+`" is unknown`) &&
		!strings.Contains(errorText, "validate Factory provider selections") {
		t.Fatalf(
			"unknown provider error = %q, want stable unknown-provider validation diagnostic; stdout=%q stderr=%q",
			errorText,
			inputs.Stdout(),
			inputs.Stderr(),
		)
	}

	if got := runner.CallCount(); got != 0 {
		t.Fatalf("provider command calls after failed startup = %d, want 0", got)
	}
}

func writeExplicitSelectionWorker(t *testing.T, factoryDir, provider, model string) {
	t.Helper()
	workerPath := filepath.Join(factoryDir, "workers", "worker", "AGENTS.md")
	worker := strings.Join([]string{
		"---",
		"model: " + model,
		"modelProvider: " + provider,
		"stopToken: COMPLETE",
		"type: MODEL_WORKER",
		"---",
		"",
		"Test worker for explicit provider and model selection.",
		"",
	}, "\n")
	if err := os.WriteFile(workerPath, []byte(worker), 0o600); err != nil {
		t.Fatalf("write explicit selection worker: %v", err)
	}
}
