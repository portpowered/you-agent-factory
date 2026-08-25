package wire

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners/internal/inference"
)

const inferenceFixtureExecutionFailure = "fixture execution failure"

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
	modelsEdge inference.LocalInvoker,
	delegate workers.Runner,
) runners.InferenceDependencies {
	return runners.InferenceDependencies{
		Models:   modelsEdge,
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
