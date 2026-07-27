package inference

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestRunnerDelegatesToCompositionWhenModelsDeclines(t *testing.T) {
	modelsEdge := &captureModelsService{
		result: models.LocalInvocationResult{Handled: false},
	}
	delegate := &captureDelegateRunner{
		result: workers.RunnerExecutionResult{Content: "provider output"},
	}
	inferenceRunner, err := New(validConfig(), Dependencies{
		Models:   modelsEdge,
		Delegate: delegate,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	request := validRequest()
	result, err := inferenceRunner.Execute(t.Context(), request)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Content != "provider output" {
		t.Fatalf("result content = %q, want delegate-owned output", result.Content)
	}
	if modelsEdge.Calls() != 1 {
		t.Fatalf("Models calls = %d, want 1", modelsEdge.Calls())
	}
	if delegate.Calls() != 1 {
		t.Fatalf("delegate calls = %d, want 1", delegate.Calls())
	}
	if delegate.Request().ModelOperation != request.ModelOperation {
		t.Fatalf("delegate request = %#v, want cloned runner request", delegate.Request())
	}
}

func TestRunnerRejectsInvalidBindingsBeforeModelsInvocation(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*workers.RunnerExecutionRequest)
	}{
		{
			name: "empty binding slot",
			mutate: func(request *workers.RunnerExecutionRequest) {
				request.ModelBindings[0].Slot = ""
			},
		},
		{
			name: "invalid binding source",
			mutate: func(request *workers.RunnerExecutionRequest) {
				request.ModelBindings[0].Source = "UNKNOWN"
			},
		},
		{
			name: "whitespace binding slot",
			mutate: func(request *workers.RunnerExecutionRequest) {
				request.ModelBindings[0].Slot = "   "
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			modelsEdge := &captureModelsService{}
			inferenceRunner := newTestRunner(t, modelsEdge)
			request := validRequest()
			test.mutate(&request)
			_, err := inferenceRunner.Execute(t.Context(), request)
			assertFailureType(t, err, workers.WorkFailureTypePermanentBadRequest)
			if modelsEdge.Calls() != 0 {
				t.Fatalf("Models calls = %d, want 0 before validation rejection", modelsEdge.Calls())
			}
		})
	}
}

func TestRunnerNormalizesUnavailableModelReadinessFailure(t *testing.T) {
	readinessErr := &models.InvocationError{
		Identity:       "WHISPER",
		ReadinessState: models.ReadinessStateMissing,
		Cause:          models.ErrMissing,
	}
	modelsEdge := &captureModelsService{
		result: models.LocalInvocationResult{Handled: true},
		err:    readinessErr,
	}
	inferenceRunner := newTestRunner(t, modelsEdge)

	_, err := inferenceRunner.Execute(t.Context(), validRequest())
	failure, ok := workers.AsInferenceFailure(err)
	if !ok || failure.Class != workers.InferenceFailureClassMissingModel {
		t.Fatalf("Execute() error = %#v, want missing-model InferenceFailure", err)
	}
	if !errors.Is(err, models.ErrMissing) {
		t.Fatalf("Execute() error = %v, want Models ErrMissing cause", err)
	}
	if modelsEdge.Calls() != 1 {
		t.Fatalf("Models calls = %d, want 1", modelsEdge.Calls())
	}
}

func TestRunnerPreservesHandledModelsSuccessContent(t *testing.T) {
	modelsEdge := &captureModelsService{
		result: models.LocalInvocationResult{Handled: true, Content: "  model output  "},
	}
	inferenceRunner, err := New(validConfig(), Dependencies{Models: modelsEdge})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	result, err := inferenceRunner.Execute(t.Context(), validRequest())
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Content != "  model output  " {
		t.Fatalf("result content = %q, want Models-owned handled output", result.Content)
	}
}

func TestRunnerProjectsLocalInvocationRequest(t *testing.T) {
	modelsEdge := &captureModelsService{
		result: models.LocalInvocationResult{Handled: true, Content: "ok"},
	}
	resources := []models.LocalResource{{
		ID: "resource-1", Name: "gpu", Type: "MODEL", Capacity: 2, Model: "WHISPER",
	}}
	inferenceRunner, err := New(Config{
		Worker: models.LocalWorker{
			Name: "whisper-worker", Type: interfaces.WorkerTypeInference,
			Model: "WHISPER", ModelLocality: models.RuntimeModelLocalityLocal,
			Resources: []models.LocalResource{{ID: "worker-resource", Name: "slot"}},
		},
		Resources: resources,
	}, Dependencies{Models: modelsEdge})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	request := validRequest()
	if _, err := inferenceRunner.Execute(t.Context(), request); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	captured := modelsEdge.Request()
	want := models.LocalInvocationRequest{
		Holder: "dispatch-1",
		Worker: models.LocalWorker{
			Name: "whisper-worker", Type: interfaces.WorkerTypeInference,
			Model: "WHISPER", ModelLocality: models.RuntimeModelLocalityLocal,
			Resources: []models.LocalResource{{ID: "worker-resource", Name: "slot"}},
		},
		Resources: resources,
		Dispatch:  request.Dispatch,
		ModelOperation: "transcribe",
		ModelBindings: []models.ResolvedModelOperationBinding{{
			Slot: "audio", Source: string(workers.ModelOperationBindingSourceInput),
			Content: []work.WorkContentPart{{
				Type: work.WorkContentPartTypeAudio, Text: "fixture-audio",
			}},
		}},
		WorkingDirectory: "explicit-work-dir",
	}
	if !reflect.DeepEqual(captured, want) {
		t.Fatalf("InvokeLocal request = %#v, want %#v", captured, want)
	}
}

func TestRunnerUsesWorktreeWhenWorkingDirectoryAbsent(t *testing.T) {
	modelsEdge := &captureModelsService{
		result: models.LocalInvocationResult{Handled: true, Content: "ok"},
	}
	inferenceRunner := newTestRunner(t, modelsEdge)
	request := validRequest()
	request.WorkingDirectory = ""
	request.Worktree = "worktree-dir"

	if _, err := inferenceRunner.Execute(t.Context(), request); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got := modelsEdge.Request().WorkingDirectory; got != "worktree-dir" {
		t.Fatalf("working directory = %q, want worktree-dir", got)
	}
}

func TestRunnerUsesDispatchWorkstationAsHolderFallback(t *testing.T) {
	modelsEdge := &captureModelsService{
		result: models.LocalInvocationResult{Handled: true, Content: "ok"},
	}
	inferenceRunner := newTestRunner(t, modelsEdge)
	request := validRequest()
	request.Dispatch.DispatchID = ""

	if _, err := inferenceRunner.Execute(t.Context(), request); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got := modelsEdge.Request().Holder; got != "request-workstation" {
		t.Fatalf("holder = %q, want request-workstation fallback", got)
	}
}

func TestNewRejectsMissingConfigurationAndEffects(t *testing.T) {
	tests := []struct {
		name   string
		config Config
		deps   Dependencies
	}{
		{
			name: "models service",
			config: validConfig(),
			deps:   Dependencies{},
		},
		{
			name:   "worker name",
			config: Config{Worker: models.LocalWorker{Type: interfaces.WorkerTypeInference, Model: "WHISPER"}},
			deps:   Dependencies{Models: &captureModelsService{}},
		},
		{
			name: "managed runtime model",
			config: Config{
				Worker: models.LocalWorker{
					Name: "worker", Type: interfaces.WorkerTypeInference,
					ModelLocality: models.RuntimeModelLocalityLocal,
				},
			},
			deps: Dependencies{Models: &captureModelsService{}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := New(test.config, test.deps)
			assertFailureType(t, err, workers.WorkFailureTypeMisconfigured)
		})
	}
}

func TestRunnerRejectsInvalidRequestBeforeModelsInvocation(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*workers.RunnerExecutionRequest)
	}{
		{
			name: "wrong runner identity",
			mutate: func(request *workers.RunnerExecutionRequest) {
				request.RunnerID = "script"
			},
		},
		{
			name: "missing model operation",
			mutate: func(request *workers.RunnerExecutionRequest) {
				request.ModelOperation = ""
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			modelsEdge := &captureModelsService{}
			inferenceRunner := newTestRunner(t, modelsEdge)
			request := validRequest()
			test.mutate(&request)
			_, err := inferenceRunner.Execute(t.Context(), request)
			assertFailureType(t, err, workers.WorkFailureTypePermanentBadRequest)
			if modelsEdge.Calls() != 0 {
				t.Fatalf("Models calls = %d, want 0 before validation rejection", modelsEdge.Calls())
			}
		})
	}
}

func TestRunnerSnapshotsCallerOwnedDataBeforeModelsInvocation(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	modelsEdge := &captureModelsService{
		result: models.LocalInvocationResult{Handled: true, Content: "stable"},
		onInvoke: func() {
			close(entered)
			<-release
		},
	}
	config := validConfig()
	inferenceRunner, err := New(config, Dependencies{Models: modelsEdge})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	config.Worker.Model = "mutated"
	request := validRequest()

	done := make(chan error, 1)
	go func() {
		_, executeErr := inferenceRunner.Execute(context.Background(), request)
		done <- executeErr
	}()
	<-entered
	request.ModelBindings[0].Content[0].Text = "mutated"
	request.ModelBindings[0].Content[0].Metadata = map[string]any{"mutated": true}
	request.EnvVars["RUNTIME"] = "mutated"
	request.Dispatch.Execution.RequestID = "mutated"
	request.InputTokens[0].(map[string]any)["nested"] = "mutated"
	close(release)
	if err := <-done; err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	captured := modelsEdge.Request()
	if captured.ModelBindings[0].Content[0].Text != "fixture-audio" {
		t.Fatalf("captured binding = %#v, want caller-owned snapshot", captured.ModelBindings[0].Content)
	}
	if captured.Dispatch.Execution.RequestID != "request-1" {
		t.Fatalf("captured dispatch request ID = %q, want request-1", captured.Dispatch.Execution.RequestID)
	}
}

func TestRunnerSuccessResultsStayDetachedAcrossRepeatedAndConcurrentExecutions(t *testing.T) {
	modelsEdge := &captureModelsService{
		result: models.LocalInvocationResult{Handled: true, Content: "stable"},
	}
	inferenceRunner := newTestRunner(t, modelsEdge)

	first, err := inferenceRunner.Execute(t.Context(), validRequest())
	if err != nil {
		t.Fatalf("first Execute() error = %v", err)
	}
	first.Content = "mutated"

	const executions = 16
	errs := make(chan error, executions)
	var wait sync.WaitGroup
	for range executions {
		wait.Add(1)
		go func() {
			defer wait.Done()
			result, executeErr := inferenceRunner.Execute(context.Background(), validRequest())
			if executeErr != nil {
				errs <- executeErr
				return
			}
			if result.Content != "stable" {
				errs <- errors.New("concurrent result was not detached")
			}
		}()
	}
	wait.Wait()
	close(errs)
	for executeErr := range errs {
		t.Fatal(executeErr)
	}
}

func TestRunnerPreservesPreCanceledContextWithoutCallingModels(t *testing.T) {
	modelsEdge := &captureModelsService{}
	inferenceRunner := newTestRunner(t, modelsEdge)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := inferenceRunner.Execute(ctx, validRequest())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Execute() error = %v, want context.Canceled", err)
	}
	if modelsEdge.Calls() != 0 {
		t.Fatalf("Models calls = %d, want 0 after pre-cancellation", modelsEdge.Calls())
	}
}

func TestWorkerFromFactorySnapshotsDetachedWorkerProjection(t *testing.T) {
	worker := &interfaces.FactoryWorkerConfig{
		Name: "fixture", Type: interfaces.WorkerTypeInference, Model: "WHISPER",
		ModelLocality: models.RuntimeModelLocalityLocal,
		Resources: []interfaces.ResourceConfig{{
			ID: "resource-1", Name: "gpu", Type: "MODEL", Capacity: 1, Model: "WHISPER",
		}},
	}
	projected := WorkerFromFactory(worker)
	worker.Model = "mutated"
	worker.Resources[0].Name = "mutated"
	if projected.Model != "WHISPER" || projected.Resources[0].Name != "gpu" {
		t.Fatalf("projected worker = %#v, want detached snapshot", projected)
	}
}

func validConfig() Config {
	return Config{
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

func validRequest() workers.RunnerExecutionRequest {
	return workers.RunnerExecutionRequest{
		RunnerID:        Identity,
		WorkerType:      "request-worker",
		WorkstationType: "request-workstation",
		ModelOperation:  "transcribe",
		WorkingDirectory: "explicit-work-dir",
		Worktree:        "worktree-fallback",
		InputTokens: []any{map[string]any{
			"nested": "original",
		}},
		EnvVars: map[string]string{"RUNTIME": "request-env"},
		ModelBindings: []workers.ResolvedModelOperationBinding{{
			Slot:   "audio",
			Source: workers.ModelOperationBindingSourceInput,
			Content: []work.WorkContentPart{{
				Type: work.WorkContentPartTypeAudio,
				Text: "fixture-audio",
			}},
		}},
		Dispatch: work.WorkDispatch{
			DispatchID:      "dispatch-1",
			TransitionID:    "transition-1",
			WorkerType:      "dispatch-worker",
			WorkstationName: "request-workstation",
			InputTokens: []any{map[string]any{
				"nested": "dispatch-original",
			}},
			Execution: work.ExecutionMetadata{
				RequestID: "request-1",
			},
		},
	}
}

func newTestRunner(t *testing.T, modelsEdge *captureModelsService) workers.Runner {
	t.Helper()
	inferenceRunner, err := New(validConfig(), Dependencies{Models: modelsEdge})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return inferenceRunner
}

type captureModelsService struct {
	mu       sync.Mutex
	request  models.LocalInvocationRequest
	result   models.LocalInvocationResult
	err      error
	calls    int
	onInvoke func()
}

func (service *captureModelsService) InvokeLocal(
	_ context.Context,
	request models.LocalInvocationRequest,
) (models.LocalInvocationResult, error) {
	service.mu.Lock()
	defer service.mu.Unlock()
	service.calls++
	service.request = snapshotInvocationRequest(request)
	if service.onInvoke != nil {
		service.onInvoke()
	}
	return service.result, service.err
}

func (service *captureModelsService) Request() models.LocalInvocationRequest {
	service.mu.Lock()
	defer service.mu.Unlock()
	return snapshotInvocationRequest(service.request)
}

func (service *captureModelsService) Calls() int {
	service.mu.Lock()
	defer service.mu.Unlock()
	return service.calls
}

func snapshotInvocationRequest(request models.LocalInvocationRequest) models.LocalInvocationRequest {
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

func assertFailureType(t *testing.T, err error, want workers.WorkFailureType) {
	t.Helper()
	var failure *workers.ProviderError
	if !errors.As(err, &failure) || failure.Type != want {
		t.Fatalf("error = %#v, want ProviderError type %q", err, want)
	}
}

type captureDelegateRunner struct {
	mu      sync.Mutex
	request workers.RunnerExecutionRequest
	result  workers.RunnerExecutionResult
	err     error
	calls   int
}

func (runner *captureDelegateRunner) Execute(
	_ context.Context,
	request workers.RunnerExecutionRequest,
) (workers.RunnerExecutionResult, error) {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	runner.calls++
	runner.request = workers.CloneProviderInferenceRequest(request)
	return runner.result, runner.err
}

func (runner *captureDelegateRunner) Calls() int {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return runner.calls
}

func (runner *captureDelegateRunner) Request() workers.RunnerExecutionRequest {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return workers.CloneProviderInferenceRequest(runner.request)
}
