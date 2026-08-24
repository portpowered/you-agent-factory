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
	request  models.InvokeModelRequest
	calls    *atomic.Int32
	onInvoke func()
}

func (service *inferenceConformanceModels) InvokeModel(
	_ context.Context,
	request models.InvokeModelRequest,
) (models.InvokeModelResult, error) {
	service.mu.Lock()
	defer service.mu.Unlock()
	if service.calls != nil {
		service.calls.Add(1)
	}
	service.request = request
	if service.onInvoke != nil {
		service.onInvoke()
	}
	if request.Operation == inferenceFixtureExecutionFailure {
		return models.InvokeModelResult{}, workers.NewProviderError(
			workers.WorkFailureTypeInternalServerError,
			"runner execution failed",
			errors.New("deterministic fixture failure"),
		)
	}
	return models.InvokeModelResult{
		Status: models.ModelInvocationStatusCompleted,
		Outputs: []models.InferenceOutput{{
			Name: "text", Modality: models.ModalityText,
			ContentType: "text/plain", MediaType: "text/plain", Content: "fixture output",
		}},
	}, nil
}

func (service *inferenceConformanceModels) Request() models.InvokeModelRequest {
	service.mu.Lock()
	defer service.mu.Unlock()
	return service.request
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
	modelsEdge inference.ModelInvoker,
	delegate workers.Runner,
) runners.InferenceDependencies {
	return runners.InferenceDependencies{
		Models:   modelsEdge,
		Delegate: delegate,
	}
}

func inferenceRegistryConfig() runners.InferenceConfig {
	scope, err := (models.RuntimeScopeRef{}).Parse("factory-session:inference-registry")
	if err != nil {
		panic(err)
	}
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
		Scope: scope,
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
