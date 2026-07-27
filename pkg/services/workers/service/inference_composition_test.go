package service

import (
	"context"
	"sync"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestResolveInferenceRunnerDelegatesWhenModelsDeclines(t *testing.T) {
	t.Parallel()

	inner := &compositionDelegateRunner{
		result: workers.RunnerExecutionResult{Content: "provider output"},
	}
	modelsEdge := &compositionModelsService{
		result: models.LocalInvocationResult{Handled: false},
	}
	factoryCfg := &interfaces.FactoryConfig{
		Resources: []interfaces.ResourceConfig{{
			ID: "resource-1", Name: "gpu", Type: "MODEL", Capacity: 1, Model: "WHISPER",
		}},
	}
	workerCfg := &interfaces.FactoryWorkerConfig{
		Name:          "whisper-worker",
		Type:          interfaces.WorkerTypeInference,
		Model:         "WHISPER",
		ModelLocality: models.RuntimeModelLocalityLocal,
	}

	runner := resolveInferenceRunner(inner, modelsEdge, factoryCfg, workerCfg)
	request := workers.RunnerExecutionRequest{
		RunnerID:       workers.RunnerIDCodex,
		ModelOperation: "transcribe",
		Dispatch: work.WorkDispatch{
			DispatchID:      "dispatch-1",
			WorkstationName: "inference-workstation",
		},
	}
	result, err := runner.Execute(t.Context(), request)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Content != "provider output" {
		t.Fatalf("result content = %q, want delegate-owned output", result.Content)
	}
	if inner.calls != 1 {
		t.Fatalf("delegate calls = %d, want 1", inner.calls)
	}
	if inner.request.RunnerID != workers.RunnerIDCodex {
		t.Fatalf("delegate runner id = %q, want codex", inner.request.RunnerID)
	}
	if modelsEdge.calls != 1 {
		t.Fatalf("Models calls = %d, want 1", modelsEdge.calls)
	}
	if got := modelsEdge.request.Worker.Name; got != "whisper-worker" {
		t.Fatalf("Models worker name = %q, want whisper-worker", got)
	}
	if got := modelsEdge.request.Resources[0].Name; got != "gpu" {
		t.Fatalf("Models resource name = %q, want gpu", got)
	}
}

func TestResolveInferenceRunnerReturnsHandledModelsContent(t *testing.T) {
	t.Parallel()

	inner := &compositionDelegateRunner{}
	modelsEdge := &compositionModelsService{
		result: models.LocalInvocationResult{Handled: true, Content: "local output"},
	}
	runner := resolveInferenceRunner(
		inner,
		modelsEdge,
		&interfaces.FactoryConfig{},
		&interfaces.FactoryWorkerConfig{
			Name:          "whisper-worker",
			Type:          interfaces.WorkerTypeInference,
			Model:         "WHISPER",
			ModelLocality: models.RuntimeModelLocalityLocal,
		},
	)

	result, err := runner.Execute(t.Context(), workers.RunnerExecutionRequest{
		RunnerID:       workers.RunnerIDCodex,
		ModelOperation: "transcribe",
		Dispatch:       work.WorkDispatch{DispatchID: "dispatch-2"},
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Content != "local output" {
		t.Fatalf("result content = %q, want local output", result.Content)
	}
	if inner.calls != 0 {
		t.Fatalf("delegate calls = %d, want 0", inner.calls)
	}
}

func TestResolveInferenceRunnerSkipsCompositionWhenDependenciesMissing(t *testing.T) {
	t.Parallel()

	inner := &compositionDelegateRunner{
		result: workers.RunnerExecutionResult{Content: "native"},
	}
	runner := resolveInferenceRunner(inner, nil, &interfaces.FactoryConfig{}, &interfaces.FactoryWorkerConfig{Name: "worker"})
	if runner != inner {
		t.Fatal("resolveInferenceRunner() replaced runner without Models service")
	}
	result, err := runner.Execute(t.Context(), workers.RunnerExecutionRequest{})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Content != "native" {
		t.Fatalf("result content = %q, want native output", result.Content)
	}
}

type compositionModelsService struct {
	testModelsService
	mu      sync.Mutex
	calls   int
	request models.LocalInvocationRequest
	result  models.LocalInvocationResult
}

func (service *compositionModelsService) InvokeLocal(
	_ context.Context,
	request models.LocalInvocationRequest,
) (models.LocalInvocationResult, error) {
	service.mu.Lock()
	defer service.mu.Unlock()
	service.calls++
	service.request = request
	return service.result, nil
}

type compositionDelegateRunner struct {
	mu      sync.Mutex
	calls   int
	request workers.RunnerExecutionRequest
	result  workers.RunnerExecutionResult
}

func (runner *compositionDelegateRunner) Execute(
	_ context.Context,
	request workers.RunnerExecutionRequest,
) (workers.RunnerExecutionResult, error) {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	runner.calls++
	runner.request = request
	return runner.result, nil
}
