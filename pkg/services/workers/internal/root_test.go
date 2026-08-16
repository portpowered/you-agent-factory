package internal_test

import (
	"context"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/workers"
	workersinternal "github.com/portpowered/infinite-you/pkg/services/workers/internal"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers/internal/execution"
	runtimeassembly "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runtime_assembly"
	workstationswire "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations/wire"
)

type recordingRuntimeAssembly struct {
	request workers.RuntimeBuildRequest
	result  workers.RuntimeBuildResult
	err     error
}

var _ runtimeassembly.Service = (*recordingRuntimeAssembly)(nil)

func (assembly *recordingRuntimeAssembly) Build(
	_ context.Context,
	request workers.RuntimeBuildRequest,
) (workers.RuntimeBuildResult, error) {
	assembly.request = request
	return assembly.result, assembly.err
}

func TestNewRootConstructsPublishedWorkersService(t *testing.T) {
	t.Parallel()

	root, err := workersinternal.NewRoot(&recordingRuntimeAssembly{}, workstationswire.NewService())
	if err != nil {
		t.Fatalf("NewRoot() error = %v", err)
	}
	if root == nil {
		t.Fatal("NewRoot() returned nil service")
	}
	var published workers.Service = root
	if published == nil {
		t.Fatal("constructed root is nil")
	}
}

func TestDetachedValuesCloneAndFallback(t *testing.T) {
	t.Parallel()

	config := &workers.MockWorkersConfig{MockWorkers: []workers.MockWorkerConfig{{
		WorkerName: "worker",
		RunType:    workers.MockWorkerRunTypeScript,
		ScriptConfig: &workers.MockWorkerScriptConfig{
			Args: []string{"original"}, Env: map[string]string{"KEY": "original"},
		},
	}}}
	ctx := workerexecution.WithMockWorkersConfig(context.Background(), config)
	loaded := workerexecution.MockWorkersConfigFromContext(ctx)
	loaded.MockWorkers[0].ScriptConfig.Args[0] = "changed"
	loaded.MockWorkers[0].ScriptConfig.Env["KEY"] = "changed"
	if config.MockWorkers[0].ScriptConfig.Args[0] != "original" || config.MockWorkers[0].ScriptConfig.Env["KEY"] != "original" {
		t.Fatal("mock configuration was not detached")
	}

	policy := workers.OutputPolicy{Format: "decision-envelope", DecisionEnvelope: true}
	if got := workerexecution.MockWorkerOutputPolicyFromContext(workerexecution.WithMockWorkerOutputPolicy(ctx, policy)); got != policy {
		t.Fatalf("output policy = %#v, want %#v", got, policy)
	}
	publisher := workers.ProgressPublisher(func(workers.ProgressFragment) {})
	if got := workerexecution.ProgressPublisherFromContext(workerexecution.WithProgressPublisher(context.Background(), publisher), nil); got == nil {
		t.Fatal("request publisher = nil")
	}
	fallback := workers.ProgressPublisher(func(workers.ProgressFragment) {})
	if got := workerexecution.ProgressPublisherFromContext(context.Background(), fallback); got == nil {
		t.Fatal("fallback publisher = nil")
	}
	if workerexecution.WithMockWorkersConfig(nil, config) != nil || workerexecution.WithProgressPublisher(nil, publisher) != nil {
		t.Fatal("nil context should remain nil")
	}
}

func TestNewRootRejectsMissingOwners(t *testing.T) {
	t.Parallel()

	if _, err := workersinternal.NewRoot(nil, workstationswire.NewService()); err == nil {
		t.Fatal("NewRoot(nil assembly) error = nil, want missing runtime assembly")
	}
	if _, err := workersinternal.NewRoot(&recordingRuntimeAssembly{}, nil); err == nil {
		t.Fatal("NewRoot(nil workstations) error = nil, want missing workstations owner")
	}
}

func TestNewRootBuildRuntimeDelegatesWithoutLifecycle(t *testing.T) {
	t.Parallel()

	want := workers.RuntimeBuildResult{
		RunnerSelection: workers.ResolvedRunnerSelection{
			RunnerID: workers.RunnerIDAntigravity,
			Source:   workers.RunnerSelectionSourceFactory,
		},
	}
	assembly := &recordingRuntimeAssembly{result: want}
	root, err := workersinternal.NewRoot(assembly, workstationswire.NewService())
	if err != nil {
		t.Fatalf("NewRoot() error = %v", err)
	}

	got, err := root.BuildRuntime(t.Context(), workers.RuntimeBuildRequest{
		RunnerID: workers.RunnerIDAntigravity,
		Roles: []workers.RuntimeBuildRoleRequest{{
			Name: "writer",
			Kind: workers.RuntimeBuildRoleKindWorker,
		}},
	})
	if err != nil {
		t.Fatalf("BuildRuntime() error = %v", err)
	}
	if got.RunnerSelection != want.RunnerSelection {
		t.Fatalf("BuildRuntime() = %#v, want runner selection %#v", got, want.RunnerSelection)
	}
}

func TestNewRootExecuteDelegatesDetachedAttempt(t *testing.T) {
	t.Parallel()

	execute := &recordingExecuteCapability{
		result: workers.ExecuteResult{
			Correlation: workers.ExecutionCorrelation{
				FactorySessionID: "session-root",
				RuntimeID:        "runtime-root",
				GenerationID:     "generation-root",
				DispatchID:       "dispatch-1",
				AttemptID:        "attempt-1",
			},
			Outcome: workers.ExecutionOutcomeAccepted,
		},
	}
	root, err := workersinternal.NewRoot(
		&recordingRuntimeAssembly{},
		workstationswire.NewService(),
		execute,
	)
	if err != nil {
		t.Fatalf("NewRoot() error = %v", err)
	}

	got, err := root.Execute(t.Context(), workers.ExecuteRequest{
		Correlation: workers.ExecutionCorrelation{
			FactorySessionID: "session-root",
			RuntimeID:        "runtime-root",
			GenerationID:     "generation-root",
			DispatchID:       "dispatch-1",
			AttemptID:        "attempt-1",
		},
		Target: workers.ExecutionTarget{RunnerID: workers.RunnerIDCodex},
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got.Correlation != execute.result.Correlation ||
		got.Outcome != execute.result.Outcome {
		t.Fatalf("Execute() = %#v, want %#v", got, execute.result)
	}
	if execute.calls != 1 {
		t.Fatalf("Execute() calls = %d, want 1", execute.calls)
	}
}

func TestWorkersRootExposesDetachedPromptCapabilities(t *testing.T) {
	t.Parallel()

	execute := &promptExecuteCapability{}
	service, err := workersinternal.NewRoot(
		&recordingRuntimeAssembly{},
		workstationswire.NewService(),
		execute,
	)
	if err != nil {
		t.Fatalf("NewRoot() error = %v", err)
	}
	root, ok := service.(*workersinternal.Root)
	if !ok {
		t.Fatalf("Workers service type = %T, want *internal.Root", service)
	}

	contract := root.BuildPromptTemplateContract(1, []string{"factory/docs/guide.md"})
	if contract.InputCount != 1 || len(contract.AvailableVariables) == 0 {
		t.Fatalf("prompt contract = %#v, want selected input variables", contract)
	}
	validation := root.ValidatePromptTemplate("{{ .Context.Project }}", 1, nil)
	if !validation.Valid || len(validation.Diagnostics) != 0 {
		t.Fatalf("prompt validation = %#v, want valid detached template", validation)
	}

	rendered, err := root.RenderPrompt("hello", nil, &workers.Context{ProjectID: "project-1"})
	if err != nil {
		t.Fatalf("RenderPrompt() error = %v", err)
	}
	if rendered != "rendered:hello" || execute.renderCalls != 1 {
		t.Fatalf("RenderPrompt() = %q with %d calls, want rendered prompt once", rendered, execute.renderCalls)
	}
	fields, err := root.ResolveTemplateFields(
		"{{.Context.WorkDir}}",
		map[string]string{"TOKEN": "{{.Context.Project}}"},
		nil,
		&workers.Context{WorkDirectory: "/workspace", ProjectID: "project-1"},
		"",
	)
	if err != nil {
		t.Fatalf("ResolveTemplateFields() error = %v", err)
	}
	if fields.WorkingDirectory != "/workspace" || fields.Env["TOKEN"] != "project-1" {
		t.Fatalf("resolved fields = %#v, want detached context values", fields)
	}
	if !root.RuntimeOwnsModelEventRecording() {
		t.Fatal("RuntimeOwnsModelEventRecording() = false, want true")
	}
}

func TestWorkersRootPromptCapabilityErrorsRemainDetached(t *testing.T) {
	t.Parallel()

	service, err := workersinternal.NewRoot(
		&recordingRuntimeAssembly{},
		workstationswire.NewService(),
	)
	if err != nil {
		t.Fatalf("NewRoot() error = %v", err)
	}
	root := service.(*workersinternal.Root)
	if _, err := root.RenderPrompt("hello", nil, nil); err == nil {
		t.Fatal("RenderPrompt() error = nil, want unavailable renderer")
	}
	if _, err := root.ResolveTemplateFields("{{.Missing}}", nil, nil, nil, ""); err == nil {
		t.Fatal("ResolveTemplateFields() error = nil, want unavailable execution service")
	}
	var nilRoot *workersinternal.Root
	if _, err := nilRoot.RenderPrompt("hello", nil, nil); err == nil {
		t.Fatal("nil Root RenderPrompt() error = nil, want unavailable service")
	}
}

type recordingExecuteCapability struct {
	result workers.ExecuteResult
	calls  int
}

func (capability *recordingExecuteCapability) Execute(
	context.Context,
	workers.ExecuteRequest,
) (workers.ExecuteResult, error) {
	capability.calls++
	return capability.result, nil
}

type promptExecuteCapability struct {
	renderCalls int
}

func (capability *promptExecuteCapability) Execute(context.Context, workers.ExecuteRequest) (workers.ExecuteResult, error) {
	return workers.ExecuteResult{}, nil
}

func (capability *promptExecuteCapability) RenderPrompt(template string, _ []workers.Token, _ *workers.Context) (string, error) {
	capability.renderCalls++
	return "rendered:" + template, nil
}
