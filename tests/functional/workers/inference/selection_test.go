package inference_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	modelproviders "github.com/portpowered/infinite-you/packages/model-providers"
	platformlogging "github.com/portpowered/infinite-you/pkg/platform/logging"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/models"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	inference "github.com/portpowered/infinite-you/pkg/services/providers/wire"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerswire "github.com/portpowered/infinite-you/pkg/services/workers/wire"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestExplicitProviderAndModelReachSelectedProviderEdge proves that when a worker
// declares an explicit provider and model, root.BuildProcess dispatch invokes the
// matching registered provider-process edge, completes factory dispatch through
// that edge, and does not invoke a different registered provider edge for the
// same work.
func TestExplicitProviderAndModelReachSelectedProviderEdge(t *testing.T) {
	const (
		selectedProviderID     = "selected.provider"
		selectedProviderAlias  = "selected"
		alternateProviderID    = "alternate.provider"
		alternateProviderAlias = "alternate"
		explicitModel          = "explicit-selection-model"
	)

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	writeExplicitSelectionWorker(t, dir, selectedProviderAlias, explicitModel)
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"explicit provider selection"}`))

	selectedIntegration := inference.ProgressingExternalIntegration(
		selectedProviderID,
		"structured progress COMPLETE",
	)
	alternateIntegration := inference.ProgressingExternalIntegration(
		alternateProviderID,
		"alternate provider must not run",
	)

	selectedManifest := externalProviderManifest(t, selectedProviderID, selectedProviderAlias)
	alternateManifest := externalProviderManifest(t, alternateProviderID, alternateProviderAlias)

	_, listed, _ := support.RunFactoryToCompletionWithEdgesAndObservations(t, dir, serviceedges.Edges{
		ProviderRegistrations: []inference.Registration{
			{Manifest: selectedManifest, Integration: selectedIntegration},
			{Manifest: alternateManifest, Integration: alternateIntegration},
		},
	}, 20*time.Second)

	if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 1 {
		t.Fatalf("terminal place tokens = %d, want 1 completed work item", got)
	}
	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 0 {
		t.Fatalf("failed place tokens = %d, want 0", got)
	}

	selectedStats := selectedIntegration.Stats()
	if selectedStats.InvokeCalls != 1 {
		t.Fatalf("selected provider invoke calls = %d, want 1", selectedStats.InvokeCalls)
	}
	if selectedStats.ProgressWrites < 1 {
		t.Fatalf(
			"selected provider progress writes = %d, want at least 1 through the conductor response writer",
			selectedStats.ProgressWrites,
		)
	}
	if selectedStats.TerminalCloses != 1 {
		t.Fatalf("selected provider terminal closes = %d, want exactly one terminal outcome", selectedStats.TerminalCloses)
	}
	alternateStats := alternateIntegration.Stats()
	if alternateStats.InvokeCalls != 0 {
		t.Fatalf(
			"alternate provider invoke calls = %d, want 0 when worker selected %q",
			alternateStats.InvokeCalls,
			selectedProviderAlias,
		)
	}
	if alternateStats.ProgressWrites != 0 || alternateStats.TerminalCloses != 0 {
		t.Fatalf(
			"alternate provider side effects = progress:%d terminal:%d, want inert when not selected",
			alternateStats.ProgressWrites,
			alternateStats.TerminalCloses,
		)
	}
}

// TestWorkerProviderOverridesGlobalDefault proves that when a global default
// provider is configured and both the default and worker provider edges are
// registered, a worker-authored modelProvider dispatches through the worker
// provider edge and leaves the global default provider edge inert for that work.
func TestWorkerProviderOverridesGlobalDefault(t *testing.T) {
	const (
		defaultProviderID    = "global.default.provider"
		defaultProviderAlias = "global-default"
		workerProviderID     = "worker.override.provider"
		workerProviderAlias  = "worker-override"
		workerModel          = "worker-override-model"
		globalDefaultModel   = "global-default-model"
	)

	t.Setenv(operatorsettings.EnvDefaultWorkerModelProvider, defaultProviderAlias)
	t.Setenv(operatorsettings.EnvDefaultWorkerModel, globalDefaultModel)

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	writeExplicitSelectionWorker(t, dir, workerProviderAlias, workerModel)
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"worker provider overrides global default"}`))

	defaultIntegration := inference.ProgressingExternalIntegration(
		defaultProviderID,
		"global default provider must not run",
	)
	workerIntegration := inference.ProgressingExternalIntegration(
		workerProviderID,
		"structured progress COMPLETE",
	)

	defaultManifest := externalProviderManifest(t, defaultProviderID, defaultProviderAlias)
	workerManifest := externalProviderManifest(t, workerProviderID, workerProviderAlias)

	_, listed, _ := support.RunFactoryToCompletionWithEdgesAndObservations(t, dir, serviceedges.Edges{
		ProviderRegistrations: []inference.Registration{
			{Manifest: defaultManifest, Integration: defaultIntegration},
			{Manifest: workerManifest, Integration: workerIntegration},
		},
	}, 20*time.Second)

	if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 1 {
		t.Fatalf("terminal place tokens = %d, want 1 completed work item", got)
	}
	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 0 {
		t.Fatalf("failed place tokens = %d, want 0", got)
	}

	workerStats := workerIntegration.Stats()
	if workerStats.InvokeCalls != 1 {
		t.Fatalf("worker provider invoke calls = %d, want 1", workerStats.InvokeCalls)
	}
	if workerStats.ProgressWrites < 1 {
		t.Fatalf(
			"worker provider progress writes = %d, want at least 1 through the conductor response writer",
			workerStats.ProgressWrites,
		)
	}
	if workerStats.TerminalCloses != 1 {
		t.Fatalf("worker provider terminal closes = %d, want exactly one terminal outcome", workerStats.TerminalCloses)
	}

	defaultStats := defaultIntegration.Stats()
	if defaultStats.InvokeCalls != 0 {
		t.Fatalf(
			"global default provider invoke calls = %d, want 0 when worker selected %q",
			defaultStats.InvokeCalls,
			workerProviderAlias,
		)
	}
	if defaultStats.ProgressWrites != 0 || defaultStats.TerminalCloses != 0 {
		t.Fatalf(
			"global default provider side effects = progress:%d terminal:%d, want inert when worker provider overrides",
			defaultStats.ProgressWrites,
			defaultStats.TerminalCloses,
		)
	}
}

// TestUnknownProviderFailsBeforeProcessStart proves that when a worker names an
// unregistered provider alias, root.BuildProcess construction leaves registered
// provider edges inert and factory startup fails with a stable validation error
// before any provider invoke or customer process lifecycle starts.
func TestUnknownProviderFailsBeforeProcessStart(t *testing.T) {
	const (
		unknownProviderAlias    = "unknown-provider"
		registeredProviderID    = "registered.provider"
		registeredProviderAlias = "registered"
		unknownModel            = "unknown-model"
	)

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	writeExplicitSelectionWorker(t, dir, unknownProviderAlias, unknownModel)

	registeredIntegration := inference.ProgressingExternalIntegration(
		registeredProviderID,
		"registered provider must not run",
	)
	registeredManifest := externalProviderManifest(t, registeredProviderID, registeredProviderAlias)

	process := support.BuildProcess(t, serviceedges.Edges{
		ProviderRegistrations: []inference.Registration{
			{Manifest: registeredManifest, Integration: registeredIntegration},
		},
	})

	constructionStats := registeredIntegration.Stats()
	if constructionStats.InvokeCalls != 0 || constructionStats.ProgressWrites != 0 ||
		constructionStats.TerminalCloses != 0 || constructionStats.DiscoverCalls != 0 ||
		constructionStats.CapabilityCalls != 0 {
		t.Fatalf(
			"construction side effects = %#v, want inert registry composition",
			constructionStats,
		)
	}

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

	runStats := registeredIntegration.Stats()
	if runStats.InvokeCalls != 0 {
		t.Fatalf(
			"registered provider invoke calls after failed startup = %d, want 0",
			runStats.InvokeCalls,
		)
	}
	if runStats.ProgressWrites != 0 || runStats.TerminalCloses != 0 {
		t.Fatalf(
			"registered provider side effects after failed startup = progress:%d terminal:%d, want inert",
			runStats.ProgressWrites,
			runStats.TerminalCloses,
		)
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

func externalProviderManifest(t *testing.T, identity, alias string) inference.Manifest {
	t.Helper()
	var catalog struct {
		Providers []inference.Manifest `json:"providers"`
	}
	if err := json.Unmarshal(modelproviders.CatalogJSON(), &catalog); err != nil {
		t.Fatalf("decode embedded provider catalog: %v", err)
	}
	manifest := catalog.Providers[0]
	manifest.ID = identity
	manifest.Aliases = []string{alias}
	manifest.ImplementationAvailability = inference.ImplementationExternallySupplied
	manifest.TechnicalSupportLevel = inference.SupportProduction
	manifest.Deprecation = nil
	manifest.MaximumExecutionCapabilities = inference.ExecutionCapabilities{
		PromptSubmission: true,
	}
	manifest.MaximumResponseFidelityCapabilities = inference.ResponseFidelityCapabilities{}
	return manifest
}

// TestWorkersWireRejectsInvalidInferenceRunner proves that public Workers
// composition rejects an invalid runner registration before publishing a
// partially usable root. The error comes from the live runner registry path,
// not from a source-only or private implementation assertion.
func TestWorkersWireRejectsInvalidInferenceRunner(t *testing.T) {
	providersRoot, err := inference.NewService()
	if err != nil {
		t.Fatalf("providerswire.NewService() error = %v", err)
	}

	root, err := workerswire.NewService(
		workerswire.AgentDependencies{
			Providers: providersRoot,
			Publish:   func(workers.ProgressFragment) {},
		},
		workerswire.ScriptConfig{Command: "script"},
		workerswire.ScriptDependencies{
			CommandRunner: runnerCompositionCommandRunner{},
			FactoryDocs: func(string) (map[string]string, error) {
				return nil, nil
			},
			Now:     func() time.Time { return time.Unix(0, 0) },
			Publish: func(workers.ProgressFragment) {},
			Record:  func(workers.ScriptEvent) {},
		},
		workerswire.InferenceConfig{
			Worker: models.LocalWorker{
				Name:          "invalid-local-worker",
				Type:          models.RuntimeWorkerTypeInference,
				ModelLocality: models.RuntimeModelLocalityLocal,
			},
		},
		workerswire.InferenceDependencies{
			Models: runnerCompositionModels{},
		},
		nil,
		platformlogging.NoopLogger{},
		func() time.Time { return time.Unix(0, 0) },
		nil,
		nil,
		nil,
		nil,
	)
	if root != nil {
		t.Fatalf("workerswire.NewService() root = %#v, want nil after invalid registration", root)
	}
	if err == nil || !errors.Is(err, workers.ErrInvalidRunnerRegistration) {
		t.Fatalf(
			"workerswire.NewService() error = %v, want invalid runner registration",
			err,
		)
	}
}

type runnerCompositionCommandRunner struct{}

func (runnerCompositionCommandRunner) Run(
	context.Context,
	platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	return platformprocess.CommandResult{}, nil
}

func (runnerCompositionCommandRunner) RunStreaming(
	context.Context,
	platformprocess.CommandRequest,
	platformprocess.OutputChunkObserver,
) (platformprocess.CommandResult, error) {
	return platformprocess.CommandResult{}, nil
}

type runnerCompositionModels struct{}

func (runnerCompositionModels) InvokeLocal(
	context.Context,
	models.LocalInvocationRequest,
) (models.LocalInvocationResult, error) {
	return models.LocalInvocationResult{}, nil
}

func (runnerCompositionModels) InvokeModel(
	context.Context,
	models.InvokeModelRequest,
) (models.InvokeModelResult, error) {
	return models.InvokeModelResult{}, nil
}
