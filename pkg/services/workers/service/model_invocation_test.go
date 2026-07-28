package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	factorydefinitioncomposition "github.com/portpowered/infinite-you/internal/testutil/factorydefinitionfixtures"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	platformrandom "github.com/portpowered/infinite-you/pkg/platform/random"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	modelinference "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/services/workers/agypty"
	workeragentrun "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations/executor/agentrun"
	workerprovider "github.com/portpowered/infinite-you/pkg/services/workers/provider"
	workerservice "github.com/portpowered/infinite-you/pkg/services/workers/service"
)

var deterministicRetryRandom = platformrandom.SourceFunc(func(int64) (int64, error) {
	return 0, nil
})

func TestService_InvokeModel_ReturnsCanonicalContentAndBindings(t *testing.T) {
	t.Parallel()

	audioPath := filepath.Join(t.TempDir(), "speech.wav")
	runtimeCfg := mustLoadedCatalogConfig(t, catalogFactoryConfig(true))
	svc := mustConstructWorkerService(t, workerServiceFixture{
		RuntimeConfig: func() interfaces.RuntimeConfigLookup { return runtimeCfg },
		ModelRuntime:  readyModelRuntime(),
		ModelInvocationExecutor: func(_ interfaces.RuntimeConfigLookup, _ *interfaces.FactoryConfig, workerName string) (workers.WorkstationRequestExecutor, error) {
			return stubInvocationExecutor{
				workerName: workerName,
				output:     mustMarshalAudioContentResponse(t, audioPath),
			}, nil
		},
	})

	result, err := svc.InvokeModel(context.Background(), "OMNIVOICE_Q4_K_M", modelinference.Request{
		Operation: "TTS",
		Content:   []work.WorkContentPart{invokeTextPart("hello world")},
		Options:   &modelinference.Options{ResponseMode: modelinference.ResponseModeAudioStream},
	})
	if err != nil {
		t.Fatalf("InvokeModel: %v", err)
	}
	if result.ModelName != "OMNIVOICE_Q4_K_M" || result.Worker != "voice-local" {
		t.Fatalf("result identity = %#v, want OMNIVOICE voice-local", result)
	}
	if len(result.Bindings) != 1 || result.Bindings[0].Slot != "text" {
		t.Fatalf("bindings = %#v, want one text binding", result.Bindings)
	}
	if len(result.Content) != 1 || result.Content[0].Type != work.WorkContentPartTypeAudio ||
		result.StreamFile != audioPath || result.StreamContentType != "audio/wav" {
		t.Fatalf("result content = %#v stream=%q type=%q, want audio output", result.Content, result.StreamFile, result.StreamContentType)
	}
}

func TestService_InvokeModel_ReturnsNotFoundWhenModelDoesNotExist(t *testing.T) {
	t.Parallel()

	runtimeCfg := mustLoadedCatalogConfig(t, &interfaces.FactoryConfig{Name: "factory"})
	svc := mustConstructWorkerService(t, workerServiceFixture{
		RuntimeConfig: func() interfaces.RuntimeConfigLookup { return runtimeCfg },
	})

	_, err := svc.InvokeModel(context.Background(), "MISSING", modelinference.Request{Operation: "TTS"})
	if err == nil || !errors.Is(err, modelinference.ErrNotFound) {
		t.Fatalf("InvokeModel error = %v, want ErrModelNotFound", err)
	}
}

func TestService_InvokeModel_ReturnsManagedRuntimeMissingWhenCacheNotReady(t *testing.T) {
	t.Parallel()

	runtimeCfg := mustLoadedCatalogConfig(t, catalogFactoryConfig(true))
	svc := mustConstructWorkerService(t, workerServiceFixture{
		RuntimeConfig: func() interfaces.RuntimeConfigLookup { return runtimeCfg },
		ModelRuntime:  missingModelRuntime(),
	})

	_, err := svc.InvokeModel(context.Background(), "OMNIVOICE_Q4_K_M", modelinference.Request{
		Operation: "TTS",
		Content:   []work.WorkContentPart{invokeTextPart("hello world")},
	})
	if err == nil || !errors.Is(err, modelinference.ErrMissing) {
		t.Fatalf("InvokeModel error = %v, want managed runtime missing", err)
	}
}

func TestService_InvokeModel_ReturnsUnavailableWhenRuntimeMissing(t *testing.T) {
	t.Parallel()

	svc, err := workerservice.New(
		nil, nil, nil, nil, nil, nil, false, "", nil, nil, nil, nil, nil,
		nil, nil, nil, nil, nil, nil, nil, nil, nil, "", nil, nil, deterministicRetryRandom, platformfilesystem.Local{}, platformfilesystem.Local{},
	)
	if svc != nil || err == nil {
		t.Fatalf("New = (%v, %v), want missing session runtime construction error", svc, err)
	}
}

func TestService_InvokeModel_ReturnsErrorWhenExecutorMissing(t *testing.T) {
	t.Parallel()

	runtimeCfg := mustLoadedCatalogConfig(t, catalogFactoryConfig(true))
	svc := mustConstructWorkerService(t, workerServiceFixture{
		RuntimeConfig: func() interfaces.RuntimeConfigLookup { return runtimeCfg },
		ModelRuntime:  readyModelRuntime(),
		ModelInvocationExecutor: func(
			interfaces.RuntimeConfigLookup,
			*interfaces.FactoryConfig,
			string,
		) (workers.WorkstationRequestExecutor, error) {
			return nil, errors.New("Worker application is required")
		},
	})
	_, err := svc.InvokeModel(context.Background(), "OMNIVOICE_Q4_K_M", directInvokeRequest(t))
	if err == nil || !strings.Contains(err.Error(), "Worker application is required") {
		t.Fatalf("InvokeModel error = %v, want missing Worker application", err)
	}
}

func TestService_InvokeModel_LogsInvocationReadiness(t *testing.T) {
	t.Parallel()

	runtimeCfg := mustLoadedCatalogConfig(t, catalogFactoryConfig(true))
	core, logs := observer.New(zapcore.DebugLevel)
	logger := zap.New(core)
	svc := mustConstructWorkerService(t, workerServiceFixture{
		RuntimeConfig: func() interfaces.RuntimeConfigLookup { return runtimeCfg },
		ModelRuntime:  missingModelRuntime(),
		Logger:        logger,
	})

	_, err := svc.InvokeModel(context.Background(), "OMNIVOICE_Q4_K_M", modelinference.Request{
		Operation: "TTS",
		Content:   []work.WorkContentPart{invokeTextPart("hello world")},
	})
	if err == nil || !errors.Is(err, modelinference.ErrMissing) {
		t.Fatalf("InvokeModel error = %v, want managed runtime missing", err)
	}
	if got := logs.FilterMessage("managed runtime invocation blocked").Len(); got != 1 {
		t.Fatalf("blocked readiness logs = %d, want 1", got)
	}
	if got := logs.FilterMessage("managed runtime invocation readiness satisfied").Len(); got != 0 {
		t.Fatalf("successful readiness logs = %d, want 0", got)
	}
}

func TestService_InvokeModel_PropagatesCancellationAndDeadlines(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		context func(t *testing.T) context.Context
		wantErr error
	}{
		{
			name: "cancellation",
			context: func(t *testing.T) context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx
			},
			wantErr: context.Canceled,
		},
		{
			name: "deadline",
			context: func(t *testing.T) context.Context {
				ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
				t.Cleanup(cancel)
				return ctx
			},
			wantErr: context.DeadlineExceeded,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := tt.context(t)
			runtimeCfg := mustLoadedCatalogConfig(t, catalogFactoryConfig(true))
			svc := mustConstructWorkerService(t, workerServiceFixture{
				RuntimeConfig: func() interfaces.RuntimeConfigLookup { return runtimeCfg },
				ModelRuntime:  readyModelRuntime(),
				ModelInvocationExecutor: func(interfaces.RuntimeConfigLookup, *interfaces.FactoryConfig, string) (workers.WorkstationRequestExecutor, error) {
					return invocationExecutorFunc(func(got context.Context, _ workerexecution.WorkstationExecutionRequest) (workerexecution.WorkResult, error) {
						if got != ctx {
							t.Fatal("executor received a different context")
						}
						return workerexecution.WorkResult{}, got.Err()
					}), nil
				},
			})

			_, err := svc.InvokeModel(ctx, "OMNIVOICE_Q4_K_M", directInvokeRequest(t))
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("InvokeModel error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestService_InvokeModel_ClassifiesExecutorAndFailedResultFailures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		execute   invocationExecutorFunc
		wantClass workers.InferenceFailureClass
	}{
		{
			name: "provider timeout",
			execute: func(context.Context, workerexecution.WorkstationExecutionRequest) (workerexecution.WorkResult, error) {
				return workerexecution.WorkResult{}, workerprovider.NewProviderError(workerexecution.WorkFailureTypeTimeout, "provider timed out", context.DeadlineExceeded)
			},
			wantClass: workers.InferenceFailureClassTimeout,
		},
		{
			name: "provider failure",
			execute: func(context.Context, workerexecution.WorkstationExecutionRequest) (workerexecution.WorkResult, error) {
				return workerexecution.WorkResult{}, workerprovider.NewProviderError(workerexecution.WorkFailureTypeUnknown, "provider failed", errors.New("provider failure"))
			},
			wantClass: workers.InferenceFailureClassRuntimeFailure,
		},
		{
			name: "failed work result",
			execute: func(context.Context, workerexecution.WorkstationExecutionRequest) (workerexecution.WorkResult, error) {
				return workerexecution.WorkResult{
					Outcome: workerexecution.OutcomeFailed,
					Error:   "provider timed out",
					FailureMetadata: &workerexecution.WorkFailureMetadata{
						Type: workerexecution.WorkFailureTypeTimeout,
					},
				}, nil
			},
			wantClass: workers.InferenceFailureClassTimeout,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runtimeCfg := mustLoadedCatalogConfig(t, catalogFactoryConfig(true))
			svc := mustConstructWorkerService(t, workerServiceFixture{
				RuntimeConfig: func() interfaces.RuntimeConfigLookup { return runtimeCfg },
				ModelRuntime:  readyModelRuntime(),
				ModelInvocationExecutor: func(interfaces.RuntimeConfigLookup, *interfaces.FactoryConfig, string) (workers.WorkstationRequestExecutor, error) {
					return tt.execute, nil
				},
			})

			_, err := svc.InvokeModel(context.Background(), "OMNIVOICE_Q4_K_M", directInvokeRequest(t))
			failure, ok := workers.AsInferenceFailure(err)
			if !ok || failure.Class != tt.wantClass {
				t.Fatalf("InvokeModel error = %v, want %s InferenceFailure", err, tt.wantClass)
			}
		})
	}
}

func TestService_InvokeModel_ReturnsUnsupportedModeWhenAudioStreamMissingOutput(t *testing.T) {
	t.Parallel()

	runtimeCfg := mustLoadedCatalogConfig(t, catalogFactoryConfig(true))
	svc := mustConstructWorkerService(t, workerServiceFixture{
		RuntimeConfig: func() interfaces.RuntimeConfigLookup { return runtimeCfg },
		ModelRuntime:  readyModelRuntime(),
		ModelInvocationExecutor: func(_ interfaces.RuntimeConfigLookup, _ *interfaces.FactoryConfig, workerName string) (workers.WorkstationRequestExecutor, error) {
			return stubInvocationExecutor{
				workerName: workerName,
				output:     `[]`,
			}, nil
		},
	})

	_, err := svc.InvokeModel(context.Background(), "OMNIVOICE_Q4_K_M", modelinference.Request{
		Operation: "TTS",
		Content:   []work.WorkContentPart{invokeTextPart("hello world")},
		Options:   &modelinference.Options{ResponseMode: modelinference.ResponseModeAudioStream},
	})
	if err == nil || !errors.Is(err, modelinference.ErrUnsupportedResponseMode) {
		t.Fatalf("InvokeModel error = %v, want unsupported audio stream mode", err)
	}
}

func TestService_InvokeModel_UsesFactoryRunnerID(t *testing.T) {
	t.Parallel()

	runtimeCfg := mustLoadedCatalogConfig(t, catalogFactoryConfig(true))
	var capturedRunnerID string
	svc := mustConstructWorkerService(t, workerServiceFixture{
		RuntimeConfig:   func() interfaces.RuntimeConfigLookup { return runtimeCfg },
		ModelRuntime:    readyModelRuntime(),
		FactoryRunnerID: "runner-42",
		ModelInvocationExecutor: func(_ interfaces.RuntimeConfigLookup, _ *interfaces.FactoryConfig, workerName string) (workers.WorkstationRequestExecutor, error) {
			return capturingInvocationExecutor{
				workerName:       workerName,
				capturedRunnerID: &capturedRunnerID,
				output:           mustMarshalAudioContentResponse(t, filepath.Join(t.TempDir(), "speech.wav")),
			}, nil
		},
	})

	_, err := svc.InvokeModel(context.Background(), "OMNIVOICE_Q4_K_M", modelinference.Request{
		Operation: "TTS",
		Content:   []work.WorkContentPart{invokeTextPart("hello world")},
	})
	if err != nil {
		t.Fatalf("InvokeModel: %v", err)
	}
	if capturedRunnerID != "runner-42" {
		t.Fatalf("runner id = %q, want runner-42", capturedRunnerID)
	}
}

type workerServiceFixture struct {
	RuntimeConfig           func() interfaces.RuntimeConfigLookup
	ModelRuntime            modelinference.Runtime
	Logger                  *zap.Logger
	FactoryRunnerID         string
	ModelInvocationExecutor workerservice.ModelInvocationExecutor
}

func mustConstructWorkerService(t *testing.T, fixture workerServiceFixture) *workerservice.Service {
	t.Helper()
	var runtimeConfig interfaces.RuntimeConfigLookup
	if fixture.RuntimeConfig != nil {
		runtimeConfig = fixture.RuntimeConfig()
	}
	sessions := currentRuntimeResolver{
		runtime: &factorysessions.LiveRuntime{
			RuntimeConfig: runtimeConfig,
		},
	}
	svc, err := workerservice.New(
		sessions,
		workerModelService{runtimeConfig: fixture.RuntimeConfig, runtime: fixture.ModelRuntime},
		inertCommandRunner{},
		inertCommandRunner{},
		&agypty.MockAllocator{},
		selectedTestLogger(fixture.Logger),
		false,
		fixture.FactoryRunnerID,
		nil,
		nil,
		time.Now,
		os.Environ,
		os.Getwd,
		fixture.ModelInvocationExecutor,
		nil,
		nil,
		nil,
		func(string) (map[string]string, error) { return map[string]string{}, nil },
		func(path string) (string, error) { return path, nil },
		platformprocess.HostExecutableLocator{},
		platformfilesystem.Local{},
		platformfilesystem.Local{},
		"linux",
		inertWorktreePreparer{},
		workeragentrun.NewLibraryHarnessAdapter(platformfilesystem.Local{}),
		deterministicRetryRandom,
		platformfilesystem.Local{},
		platformfilesystem.Local{},
	)
	if err != nil {
		t.Fatalf("New Worker service: %v", err)
	}
	return svc
}

type inertCommandRunner struct{}

type inertWorktreePreparer struct{}

func (inertWorktreePreparer) Prepare(context.Context, string, string) (workers.FactoryWorktreePreparation, error) {
	return workers.FactoryWorktreePreparation{}, nil
}

func (inertCommandRunner) Run(context.Context, workers.CommandRequest) (workers.CommandResult, error) {
	return workers.CommandResult{}, nil
}

func selectedTestLogger(logger *zap.Logger) *zap.Logger {
	if logger != nil {
		return logger
	}
	return zap.NewNop()
}

type currentRuntimeResolver struct {
	runtime *factorysessions.LiveRuntime
}

func (r currentRuntimeResolver) CurrentRuntime() *factorysessions.LiveRuntime {
	return r.runtime
}

type workerModelService struct {
	runtimeConfig func() interfaces.RuntimeConfigLookup
	runtime       modelinference.Runtime
}

func (workerModelService) OpenRuntimeScope(
	context.Context,
	modelinference.OpenRuntimeScopeRequest,
) (modelinference.OpenRuntimeScopeResult, error) {
	return modelinference.OpenRuntimeScopeResult{}, modelinference.ErrUnsupportedOperation
}

func (workerModelService) CloseRuntimeScope(
	context.Context,
	modelinference.CloseRuntimeScopeRequest,
) (modelinference.CloseRuntimeScopeResult, error) {
	return modelinference.CloseRuntimeScopeResult{}, modelinference.ErrUnsupportedOperation
}

func (workerModelService) ListCatalog(
	context.Context,
	modelinference.ListModelsRequest,
) (modelinference.ListModelsResult, error) {
	return modelinference.ListModelsResult{}, modelinference.ErrUnsupportedOperation
}

func (workerModelService) GetCatalogModel(
	context.Context,
	modelinference.GetModelRequest,
) (modelinference.GetModelResult, error) {
	return modelinference.GetModelResult{}, modelinference.ErrUnsupportedOperation
}

func (workerModelService) GetModelReadiness(
	context.Context,
	modelinference.GetModelReadinessRequest,
) (modelinference.GetModelReadinessResult, error) {
	return modelinference.GetModelReadinessResult{}, modelinference.ErrUnsupportedOperation
}

func (workerModelService) PrepareModelAssets(
	context.Context,
	modelinference.PrepareModelAssetsRequest,
) (modelinference.PrepareModelAssetsResult, error) {
	return modelinference.PrepareModelAssetsResult{}, modelinference.ErrUnsupportedOperation
}

func (workerModelService) InspectModelAssets(
	context.Context,
	modelinference.InspectModelAssetsRequest,
) (modelinference.InspectModelAssetsResult, error) {
	return modelinference.InspectModelAssetsResult{}, modelinference.ErrUnsupportedOperation
}

func (workerModelService) RemoveModelAssets(
	context.Context,
	modelinference.RemoveModelAssetsRequest,
) (modelinference.RemoveModelAssetsResult, error) {
	return modelinference.RemoveModelAssetsResult{}, modelinference.ErrUnsupportedOperation
}

func (workerModelService) EnsureModelHost(
	context.Context,
	modelinference.EnsureModelHostRequest,
) (modelinference.EnsureModelHostResult, error) {
	return modelinference.EnsureModelHostResult{}, modelinference.ErrUnsupportedOperation
}

func (workerModelService) InspectModelHost(
	context.Context,
	modelinference.InspectModelHostRequest,
) (modelinference.InspectModelHostResult, error) {
	return modelinference.InspectModelHostResult{}, modelinference.ErrUnsupportedOperation
}

func (workerModelService) StopModelHost(
	context.Context,
	modelinference.StopModelHostRequest,
) (modelinference.StopModelHostResult, error) {
	return modelinference.StopModelHostResult{}, modelinference.ErrUnsupportedOperation
}

func (workerModelService) AcquireModelLease(
	context.Context,
	modelinference.AcquireModelLeaseRequest,
) (modelinference.AcquireModelLeaseResult, error) {
	return modelinference.AcquireModelLeaseResult{}, modelinference.ErrUnsupportedOperation
}

func (workerModelService) GetModelLease(
	context.Context,
	modelinference.GetModelLeaseRequest,
) (modelinference.GetModelLeaseResult, error) {
	return modelinference.GetModelLeaseResult{}, modelinference.ErrUnsupportedOperation
}

func (workerModelService) ReleaseModelLease(
	context.Context,
	modelinference.ReleaseModelLeaseRequest,
) (modelinference.ReleaseModelLeaseResult, error) {
	return modelinference.ReleaseModelLeaseResult{}, modelinference.ErrUnsupportedOperation
}

func (workerModelService) InvokeModelWithLease(
	context.Context,
	modelinference.InvokeModelRequest,
) (modelinference.InvokeModelResult, error) {
	return modelinference.InvokeModelResult{}, modelinference.ErrUnsupportedOperation
}

func (workerModelService) CancelInvocation(
	context.Context,
	modelinference.CancelInvocationRequest,
) (modelinference.CancelInvocationResult, error) {
	return modelinference.CancelInvocationResult{}, modelinference.ErrUnsupportedOperation
}

func (s workerModelService) ForRuntime(modelinference.RuntimeBinding) (modelinference.Service, error) {
	return s, nil
}

func (workerModelService) ListModels(context.Context) (modelinference.List, error) {
	return modelinference.List{}, nil
}

func (workerModelService) GetModel(context.Context, string) (modelinference.Detail, error) {
	return modelinference.Detail{}, nil
}

func (workerModelService) PullModel(context.Context, string) (modelinference.PullResult, error) {
	return modelinference.PullResult{}, nil
}

func (workerModelService) PullModelForScope(
	context.Context,
	modelinference.PullModelRequest,
) (modelinference.PullResult, error) {
	return modelinference.PullResult{}, nil
}

func (workerModelService) InvokeLocal(context.Context, modelinference.LocalInvocationRequest) (modelinference.LocalInvocationResult, error) {
	return modelinference.LocalInvocationResult{}, nil
}

func (workerModelService) AcquireLease(context.Context, modelinference.AcquireLeaseRequest) (modelinference.HostLease, error) {
	return modelinference.HostLease{}, nil
}

func (workerModelService) ReleaseLease(context.Context, modelinference.ReleaseLeaseRequest) error {
	return nil
}

func (s workerModelService) InspectRuntime(ctx context.Context, modelName string) (modelinference.Runtime, error) {
	if s.runtimeConfig == nil || s.runtimeConfig() == nil {
		return modelinference.Runtime{}, fmt.Errorf("factory service runtime is not available")
	}
	runtime := s.runtime
	runtime.Identity = modelName
	return runtime, runtime.InvocationError()
}

func mustLoadedCatalogConfig(
	t *testing.T,
	factoryCfg *interfaces.FactoryConfig,
) interfaces.MutableLoadedFactorySource {
	t.Helper()
	loaded, err := factorydefinitioncomposition.NewLoadedSource(
		t.TempDir(),
		factoryCfg,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("construct loaded Factory source: %v", err)
	}
	return loaded
}

func catalogFactoryConfig(_ bool) *interfaces.FactoryConfig {
	return &interfaces.FactoryConfig{
		Name: "factory",
		Workers: []interfaces.FactoryWorkerConfig{{
			Name:          "voice-local",
			Type:          interfaces.WorkerTypeModel,
			Model:         "OMNIVOICE_Q4_K_M",
			ModelLocality: interfaces.ModelLocalityLocal,
			Operations: []interfaces.ModelOperation{{
				Name: "TTS",
				Inputs: []interfaces.ModelOperationSlot{{
					Name:         "text",
					ContentTypes: []string{interfaces.ModelOperationContentTypeText},
					Required:     true,
				}},
				Outputs: []interfaces.ModelOperationSlot{{
					Name:         "audio",
					ContentTypes: []string{interfaces.ModelOperationContentTypeAudio},
				}},
			}},
		}},
	}
}

func missingModelRuntime() modelinference.Runtime {
	return modelinference.Runtime{
		Locality:       modelinference.LocalityLocal,
		ReadinessState: modelinference.ReadinessStateMissing,
		LifecycleState: modelinference.LifecycleStateNotInstalled,
	}
}

func readyModelRuntime() modelinference.Runtime {
	return modelinference.Runtime{
		Locality:       modelinference.LocalityLocal,
		ReadinessState: modelinference.ReadinessStateReady,
		LifecycleState: modelinference.LifecycleStateInstalled,
	}
}

type stubInvocationExecutor struct {
	workerName string
	output     string
}

type invocationExecutorFunc func(context.Context, workerexecution.WorkstationExecutionRequest) (workerexecution.WorkResult, error)

func (f invocationExecutorFunc) Execute(ctx context.Context, request workerexecution.WorkstationExecutionRequest) (workerexecution.WorkResult, error) {
	return f(ctx, request)
}

func (s stubInvocationExecutor) Execute(_ context.Context, request workerexecution.WorkstationExecutionRequest) (workerexecution.WorkResult, error) {
	if request.WorkerType != s.workerName {
		return workerexecution.WorkResult{}, errors.New("unexpected worker")
	}
	return workerexecution.WorkResult{
		Outcome: workerexecution.OutcomeAccepted,
		Output:  s.output,
	}, nil
}

type capturingInvocationExecutor struct {
	workerName       string
	capturedRunnerID *string
	output           string
}

func (s capturingInvocationExecutor) Execute(_ context.Context, request workerexecution.WorkstationExecutionRequest) (workerexecution.WorkResult, error) {
	if request.WorkerType != s.workerName {
		return workerexecution.WorkResult{}, errors.New("unexpected worker")
	}
	*s.capturedRunnerID = request.RunnerID
	return workerexecution.WorkResult{
		Outcome: workerexecution.OutcomeAccepted,
		Output:  s.output,
	}, nil
}

func invokeTextPart(text string) work.WorkContentPart {
	return work.WorkContentPart{Type: work.WorkContentPartTypeText, Text: text, Slot: "text"}
}

func directInvokeRequest(t *testing.T) modelinference.Request {
	t.Helper()
	return modelinference.Request{
		Operation: "TTS",
		Content:   []work.WorkContentPart{invokeTextPart("hello world")},
	}
}

func mustMarshalAudioContentResponse(t *testing.T, audioPath string) string {
	t.Helper()
	body, err := json.Marshal([]work.WorkContentPart{{
		Type:        work.WorkContentPartTypeAudio,
		File:        audioPath,
		ContentType: "audio/wav",
	}})
	if err != nil {
		t.Fatalf("marshal audio content: %v", err)
	}
	return string(body)
}
