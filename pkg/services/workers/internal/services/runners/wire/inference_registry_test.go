package wire

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners/internal/inference"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners/internal/testkit"
)

const inferenceFixtureExecutionFailure = "fixture execution failure"

func TestNewInferenceRegistryIsInertAndResolvesDetachedMetadata(t *testing.T) {
	var invokeCalls atomic.Int32
	modelsEdge := &inferenceConformanceModels{calls: &invokeCalls}
	dependencies := inferenceDependencies(modelsEdge, nil)
	config := inferenceRegistryConfig()
	config.Worker.Model = "WHISPER"
	config.Resources[0].Name = "gpu"

	registry, err := NewInferenceRegistry(config, dependencies)
	if err != nil {
		t.Fatalf("NewInferenceRegistry() error = %v", err)
	}
	config.Worker.Model = "mutated"
	config.Resources[0].Name = "mutated"
	mutatedModels := &inferenceConformanceModels{
		calls: &invokeCalls,
		onInvoke: func() {
			t.Fatal("mutated dependency was retained")
		},
	}
	dependencies.Models = mutatedModels.InvokeLocal
	assertInferenceEffectCalls(t, "construction", &invokeCalls, 0)

	first, err := registry.Resolve(runners.ResolutionRequest{
		Identity: inference.Identity,
		RequiredCapabilities: []workers.RunnerOptionalCapability{
			workers.RunnerOptionalCapabilityWorkingDirectory,
			workers.RunnerOptionalCapabilityWorktree,
		},
	})
	if err != nil {
		t.Fatalf("Resolve(inference) error = %v", err)
	}
	if first.Identity != inference.Identity ||
		first.Metadata.ID != inference.Identity ||
		first.Metadata.DisplayName != "Inference" ||
		first.Runner == nil {
		t.Fatalf("Resolve(inference) = %#v, want complete Inference binding", first)
	}
	assertInferenceEffectCalls(t, "resolution", &invokeCalls, 0)

	first.Metadata.Capabilities.Baseline[0] = "mutated"
	first.Metadata.Capabilities.Optional[0].Detail = "mutated"
	second, err := registry.Resolve(runners.ResolutionRequest{Identity: inference.Identity})
	if err != nil {
		t.Fatalf("second Resolve(inference) error = %v", err)
	}
	if reflect.DeepEqual(first.Metadata, second.Metadata) {
		t.Fatalf("second metadata = %#v, want detached registry snapshot", second.Metadata)
	}

	_, err = second.Runner.Execute(t.Context(), inferenceRequest())
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got := modelsEdge.Request().Worker.Model; got != "WHISPER" {
		t.Fatalf("worker model = %q, want construction snapshot", got)
	}
	if got := modelsEdge.Request().Resources[0].Name; got != "gpu" {
		t.Fatalf("resource name = %q, want construction snapshot", got)
	}
	if invokeCalls.Load() != 1 {
		t.Fatalf("Models calls = %d, want original injected effect once", invokeCalls.Load())
	}
}

func TestNewInferenceRegistryRejectsInvalidInferenceConfiguration(t *testing.T) {
	_, err := NewInferenceRegistry(
		runners.InferenceConfig{},
		inferenceDependencies(&inferenceConformanceModels{}, nil),
	)
	if err == nil {
		t.Fatal("NewInferenceRegistry() error = nil, want invalid inference configuration")
	}
}

func TestInferenceRunnerThroughRegistryConformsToCommonContract(t *testing.T) {
	modelsEdge := &inferenceConformanceModels{}
	delegate := &inferenceConformanceDelegate{}
	registry, err := NewInferenceRegistry(
		inferenceRegistryConfig(),
		inferenceDependencies(modelsEdge, delegate),
	)
	if err != nil {
		t.Fatalf("NewInferenceRegistry() error = %v", err)
	}
	binding, err := registry.Resolve(runners.ResolutionRequest{Identity: inference.Identity})
	if err != nil {
		t.Fatalf("Resolve(inference) error = %v", err)
	}

	valid := inferenceRequest()
	baseline, err := binding.Runner.Execute(t.Context(), valid)
	if err != nil {
		t.Fatalf("baseline Execute() error = %v", err)
	}
	invalid := workers.CloneProviderInferenceRequest(valid)
	invalid.RunnerID = ""
	unsupported := workers.CloneProviderInferenceRequest(valid)
	unsupported.RequiredOptionalCapabilities = []workers.RunnerOptionalCapability{
		workers.RunnerOptionalCapabilityImageInput,
	}
	failure := workers.CloneProviderInferenceRequest(valid)
	failure.ModelOperation = inferenceFixtureExecutionFailure

	testkit.Run(t, testkit.Subject{
		Runner:             binding.Runner,
		ValidRequest:       valid,
		InvalidRequest:     invalid,
		UnsupportedRequest: unsupported,
		FailureRequest:     failure,
		ExpectedResult:     baseline,
		AssertCaptured: func(t *testing.T) {
			t.Helper()
			request := modelsEdge.Request()
			if request.ModelOperation != "transcribe" {
				t.Fatalf("captured model operation = %q, want caller-owned snapshot", request.ModelOperation)
			}
			if request.ModelBindings[0].Content[0].Text != "fixture-audio" {
				t.Fatalf("captured binding = %#v, want caller-owned snapshot", request.ModelBindings[0].Content)
			}
		},
	})
}

func TestInferenceRegistryResolveAndExecuteConcurrently(t *testing.T) {
	modelsEdge := &inferenceConformanceModels{}
	registry, err := NewInferenceRegistry(
		inferenceRegistryConfig(),
		inferenceDependencies(modelsEdge, nil),
	)
	if err != nil {
		t.Fatalf("NewInferenceRegistry() error = %v", err)
	}

	const executions = 24
	var group sync.WaitGroup
	errs := make(chan error, executions)
	for range executions {
		group.Add(1)
		go func() {
			defer group.Done()
			binding, resolveErr := registry.Resolve(runners.ResolutionRequest{
				Identity: inference.Identity,
			})
			if resolveErr != nil {
				errs <- resolveErr
				return
			}
			result, executeErr := binding.Runner.Execute(t.Context(), inferenceRequest())
			if executeErr != nil {
				errs <- executeErr
				return
			}
			if result.Content != "fixture output" {
				errs <- errors.New("unexpected detached result content")
			}
		}()
	}
	group.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("concurrent resolve/execute: %v", err)
	}
}

type inferenceConformanceModels struct {
	mu       sync.Mutex
	request  models.LocalInvocationRequest
	calls    *atomic.Int32
	onInvoke func()
}

func (service *inferenceConformanceModels) InvokeLocal(
	_ context.Context,
	request models.LocalInvocationRequest,
) (models.LocalInvocationResult, error) {
	service.mu.Lock()
	defer service.mu.Unlock()
	if service.calls != nil {
		service.calls.Add(1)
	}
	service.request = snapshotConformanceInvocationRequest(request)
	if service.onInvoke != nil {
		service.onInvoke()
	}
	if request.ModelOperation == inferenceFixtureExecutionFailure {
		return models.LocalInvocationResult{Handled: false}, nil
	}
	return models.LocalInvocationResult{
		Handled: true,
		Content: "fixture output",
	}, nil
}

func (service *inferenceConformanceModels) Request() models.LocalInvocationRequest {
	service.mu.Lock()
	defer service.mu.Unlock()
	return snapshotConformanceInvocationRequest(service.request)
}

type inferenceConformanceDelegate struct{}

func (delegate *inferenceConformanceDelegate) Execute(
	_ context.Context,
	request workers.RunnerExecutionRequest,
) (workers.RunnerExecutionResult, error) {
	if request.ModelOperation == inferenceFixtureExecutionFailure {
		return workers.RunnerExecutionResult{}, workers.NewProviderError(
			workers.WorkFailureTypeInternalServerError,
			"runner execution failed",
			errors.New("deterministic fixture failure"),
		)
	}
	return workers.RunnerExecutionResult{}, errors.New("unexpected delegate invocation")
}

func inferenceDependencies(
	modelsEdge *inferenceConformanceModels,
	delegate workers.Runner,
) runners.InferenceDependencies {
	return runners.InferenceDependencies{
		Models:   modelsEdge.InvokeLocal,
		Delegate: delegate,
	}
}

func inferenceRegistryConfig() runners.InferenceConfig {
	return runners.InferenceConfig{
		Worker: models.LocalWorker{
			Name:          "whisper-worker",
			Type:          interfaces.WorkerTypeInference,
			Model:         "WHISPER",
			ModelLocality: models.RuntimeModelLocalityLocal,
		},
		Resources: []models.LocalResource{{
			ID: "resource-1", Name: "gpu", Type: "MODEL", Capacity: 1, Model: "WHISPER",
		}},
	}
}

func inferenceRequest() workers.RunnerExecutionRequest {
	token := map[string]any{
		"color": map[string]any{
			"name":      "input",
			"work_id":   "work-1",
			"data_type": string(workers.DataTypeWork),
		},
		"nested": []any{"original"},
	}
	return workers.RunnerExecutionRequest{
		RunnerID:         inference.Identity,
		ModelOperation:   "transcribe",
		WorkingDirectory: "explicit-work-dir",
		Worktree:         "worktree-fallback",
		InputTokens:      []any{token},
		EnvVars:          map[string]string{"RUNTIME": "original"},
		ModelBindings: []workers.ResolvedModelOperationBinding{{
			Slot:   "audio",
			Source: workers.ModelOperationBindingSourceInput,
			Content: []work.WorkContentPart{{
				Type: work.WorkContentPartTypeAudio,
				Text: "fixture-audio",
				Metadata: map[string]any{
					"nested": []any{"metadata-original"},
				},
			}},
		}},
		Dispatch: work.WorkDispatch{
			DispatchID:      "dispatch-conformance",
			WorkstationName: "request-workstation",
			InputTokens: []any{map[string]any{
				"color": map[string]any{
					"name":      "input",
					"work_id":   "work-1",
					"data_type": string(workers.DataTypeWork),
				},
				"nested": []any{"dispatch-original"},
			}},
		},
		RequiredOptionalCapabilities: []workers.RunnerOptionalCapability{
			workers.RunnerOptionalCapabilityWorkingDirectory,
		},
	}
}

func snapshotConformanceInvocationRequest(
	request models.LocalInvocationRequest,
) models.LocalInvocationRequest {
	request.Resources = append([]models.LocalResource(nil), request.Resources...)
	request.ModelBindings = append([]models.ResolvedModelOperationBinding(nil), request.ModelBindings...)
	for index := range request.ModelBindings {
		request.ModelBindings[index].Content = work.CloneWorkContentParts(
			request.ModelBindings[index].Content,
		)
	}
	request.Worker.Resources = append([]models.LocalResource(nil), request.Worker.Resources...)
	request.Dispatch = work.CloneWorkDispatch(request.Dispatch)
	return request
}

func assertInferenceEffectCalls(
	t *testing.T,
	stage string,
	invokeCalls *atomic.Int32,
	want int32,
) {
	t.Helper()
	if invokeCalls.Load() != want {
		t.Fatalf("%s Models calls = %d, want %d", stage, invokeCalls.Load(), want)
	}
}
