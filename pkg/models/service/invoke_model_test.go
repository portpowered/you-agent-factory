package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	modelhost "github.com/portpowered/infinite-you/pkg/models/host"
	managedruntime "github.com/portpowered/infinite-you/pkg/models/managedruntime"
	modelsservice "github.com/portpowered/infinite-you/pkg/models/service"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	"github.com/portpowered/infinite-you/pkg/work"
	"github.com/portpowered/infinite-you/pkg/workers"
	workerprovider "github.com/portpowered/infinite-you/pkg/workers/provider"
)

func TestService_InvokeModel_ReturnsCanonicalContentAndBindings(t *testing.T) {
	t.Parallel()

	audioPath := filepath.Join(t.TempDir(), "speech.wav")
	runtimeCfg := mustLoadedCatalogConfig(t, catalogFactoryConfig(true))
	svc := mustConstructModelService(t, modelsservice.Dependencies{
		RuntimeConfig: func() *factoryconfig.LoadedFactoryConfig { return runtimeCfg },
		ModelHost:     readyInvokeHost{},
		ModelInvocationExecutor: func(_ *factoryconfig.LoadedFactoryConfig, _ *interfaces.FactoryConfig, workerName string) (workers.WorkstationRequestExecutor, error) {
			return stubInvocationExecutor{
				workerName: workerName,
				output:     mustMarshalAudioContentResponse(t, audioPath),
			}, nil
		},
	})

	mode := factoryapi.AUDIOSTREAM
	result, err := svc.InvokeModel(context.Background(), "OMNIVOICE_Q4_K_M", factoryapi.ModelInvocationRequest{
		Operation: "TTS",
		Content: &factoryapi.WorkContent{
			mustGeneratedInvokeTextPart(t, "hello world"),
		},
		Options: &factoryapi.ModelInvocationOptions{ResponseMode: &mode},
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
	svc := mustConstructModelService(t, modelsservice.Dependencies{
		RuntimeConfig: func() *factoryconfig.LoadedFactoryConfig { return runtimeCfg },
	})

	_, err := svc.InvokeModel(context.Background(), "MISSING", factoryapi.ModelInvocationRequest{Operation: "TTS"})
	if err == nil || !errors.Is(err, apisurface.ErrModelNotFound) {
		t.Fatalf("InvokeModel error = %v, want ErrModelNotFound", err)
	}
}

func TestService_InvokeModel_ReturnsManagedRuntimeMissingWhenCacheNotReady(t *testing.T) {
	t.Parallel()

	runtimeCfg := mustLoadedCatalogConfig(t, catalogFactoryConfig(true))
	svc := mustConstructModelService(t, modelsservice.Dependencies{
		RuntimeConfig: func() *factoryconfig.LoadedFactoryConfig { return runtimeCfg },
		ModelHost:     missingCacheInspectHost{},
	})

	_, err := svc.InvokeModel(context.Background(), "OMNIVOICE_Q4_K_M", factoryapi.ModelInvocationRequest{
		Operation: "TTS",
		Content: &factoryapi.WorkContent{
			mustGeneratedInvokeTextPart(t, "hello world"),
		},
	})
	if err == nil || !apisurface.IsManagedRuntimeMissing(err) {
		t.Fatalf("InvokeModel error = %v, want managed runtime missing", err)
	}
}

func TestService_InvokeModel_ReturnsUnavailableWhenRuntimeMissing(t *testing.T) {
	t.Parallel()

	svc, err := modelsservice.NewService(modelsservice.Dependencies{})
	if svc != nil || !errors.Is(err, modelsservice.ErrInvalidDependencies) {
		t.Fatalf("NewService = (%v, %v), want missing runtime construction error", svc, err)
	}
}

func TestService_InvokeModel_ReturnsErrorWhenExecutorMissing(t *testing.T) {
	t.Parallel()

	runtimeCfg := mustLoadedCatalogConfig(t, catalogFactoryConfig(true))
	svc, err := modelsservice.NewService(modelsservice.Dependencies{
		RuntimeConfig: func() *factoryconfig.LoadedFactoryConfig { return runtimeCfg },
		ModelHost:     readyInvokeHost{},
	})
	if svc != nil || !errors.Is(err, modelsservice.ErrInvalidDependencies) || !strings.Contains(err.Error(), "model asset puller") {
		t.Fatalf("NewService = (%v, %v), want missing required collaborator", svc, err)
	}
}

func TestService_InvokeModel_LogsInvocationReadiness(t *testing.T) {
	t.Parallel()

	runtimeCfg := mustLoadedCatalogConfig(t, catalogFactoryConfig(true))
	core, logs := observer.New(zapcore.DebugLevel)
	logger := zap.New(core)
	svc := mustConstructModelService(t, modelsservice.Dependencies{
		RuntimeConfig: func() *factoryconfig.LoadedFactoryConfig { return runtimeCfg },
		ModelHost:     missingCacheInspectHost{},
		Logger:        logger,
	})

	_, err := svc.InvokeModel(context.Background(), "OMNIVOICE_Q4_K_M", factoryapi.ModelInvocationRequest{
		Operation: "TTS",
		Content: &factoryapi.WorkContent{
			mustGeneratedInvokeTextPart(t, "hello world"),
		},
	})
	if err == nil || !apisurface.IsManagedRuntimeMissing(err) {
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
			svc := mustConstructModelService(t, modelsservice.Dependencies{
				RuntimeConfig: func() *factoryconfig.LoadedFactoryConfig { return runtimeCfg },
				ModelHost:     readyInvokeHost{},
				ModelInvocationExecutor: func(*factoryconfig.LoadedFactoryConfig, *interfaces.FactoryConfig, string) (workers.WorkstationRequestExecutor, error) {
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
		wantClass apisurface.InferenceFailureClass
	}{
		{
			name: "provider timeout",
			execute: func(context.Context, workerexecution.WorkstationExecutionRequest) (workerexecution.WorkResult, error) {
				return workerexecution.WorkResult{}, workerprovider.NewProviderError(workerexecution.WorkFailureTypeTimeout, "provider timed out", context.DeadlineExceeded)
			},
			wantClass: apisurface.InferenceFailureClassTimeout,
		},
		{
			name: "provider failure",
			execute: func(context.Context, workerexecution.WorkstationExecutionRequest) (workerexecution.WorkResult, error) {
				return workerexecution.WorkResult{}, workerprovider.NewProviderError(workerexecution.WorkFailureTypeUnknown, "provider failed", errors.New("provider failure"))
			},
			wantClass: apisurface.InferenceFailureClassRuntimeFailure,
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
			wantClass: apisurface.InferenceFailureClassTimeout,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runtimeCfg := mustLoadedCatalogConfig(t, catalogFactoryConfig(true))
			svc := mustConstructModelService(t, modelsservice.Dependencies{
				RuntimeConfig: func() *factoryconfig.LoadedFactoryConfig { return runtimeCfg },
				ModelHost:     readyInvokeHost{},
				ModelInvocationExecutor: func(*factoryconfig.LoadedFactoryConfig, *interfaces.FactoryConfig, string) (workers.WorkstationRequestExecutor, error) {
					return tt.execute, nil
				},
			})

			_, err := svc.InvokeModel(context.Background(), "OMNIVOICE_Q4_K_M", directInvokeRequest(t))
			failure, ok := apisurface.AsInferenceFailure(err)
			if !ok || failure.Class != tt.wantClass {
				t.Fatalf("InvokeModel error = %v, want %s InferenceFailure", err, tt.wantClass)
			}
		})
	}
}

func TestService_InvokeModel_ReturnsUnsupportedModeWhenAudioStreamMissingOutput(t *testing.T) {
	t.Parallel()

	runtimeCfg := mustLoadedCatalogConfig(t, catalogFactoryConfig(true))
	mode := factoryapi.AUDIOSTREAM
	svc := mustConstructModelService(t, modelsservice.Dependencies{
		RuntimeConfig: func() *factoryconfig.LoadedFactoryConfig { return runtimeCfg },
		ModelHost:     readyInvokeHost{},
		ModelInvocationExecutor: func(_ *factoryconfig.LoadedFactoryConfig, _ *interfaces.FactoryConfig, workerName string) (workers.WorkstationRequestExecutor, error) {
			return stubInvocationExecutor{
				workerName: workerName,
				output:     `[]`,
			}, nil
		},
	})

	_, err := svc.InvokeModel(context.Background(), "OMNIVOICE_Q4_K_M", factoryapi.ModelInvocationRequest{
		Operation: "TTS",
		Content: &factoryapi.WorkContent{
			mustGeneratedInvokeTextPart(t, "hello world"),
		},
		Options: &factoryapi.ModelInvocationOptions{ResponseMode: &mode},
	})
	if err == nil || !errors.Is(err, apisurface.ErrModelInvocationUnsupportedMode) {
		t.Fatalf("InvokeModel error = %v, want unsupported audio stream mode", err)
	}
}

func TestService_InvokeModel_UsesFactoryRunnerID(t *testing.T) {
	t.Parallel()

	runtimeCfg := mustLoadedCatalogConfig(t, catalogFactoryConfig(true))
	var capturedRunnerID string
	svc := mustConstructModelService(t, modelsservice.Dependencies{
		RuntimeConfig:   func() *factoryconfig.LoadedFactoryConfig { return runtimeCfg },
		ModelHost:       readyInvokeHost{},
		FactoryRunnerID: "runner-42",
		ModelInvocationExecutor: func(_ *factoryconfig.LoadedFactoryConfig, _ *interfaces.FactoryConfig, workerName string) (workers.WorkstationRequestExecutor, error) {
			return capturingInvocationExecutor{
				workerName:       workerName,
				capturedRunnerID: &capturedRunnerID,
				output:           mustMarshalAudioContentResponse(t, filepath.Join(t.TempDir(), "speech.wav")),
			}, nil
		},
	})

	_, err := svc.InvokeModel(context.Background(), "OMNIVOICE_Q4_K_M", factoryapi.ModelInvocationRequest{
		Operation: "TTS",
		Content: &factoryapi.WorkContent{
			mustGeneratedInvokeTextPart(t, "hello world"),
		},
	})
	if err != nil {
		t.Fatalf("InvokeModel: %v", err)
	}
	if capturedRunnerID != "runner-42" {
		t.Fatalf("runner id = %q, want runner-42", capturedRunnerID)
	}
}

type readyInvokeHost struct{}

func (readyInvokeHost) ResolveIdentity(_ context.Context, _ *factoryconfig.LoadedFactoryConfig, modelName string) (modelhost.Identity, error) {
	return modelhost.Identity{Name: modelName, Locality: managedruntime.LocalityLocal}, nil
}

func (readyInvokeHost) InspectReadiness(_ context.Context, _ *factoryconfig.LoadedFactoryConfig, modelName string) (modelhost.ReadinessSnapshot, error) {
	return modelhost.ReadinessSnapshot{
		Identity:       modelhost.Identity{Name: modelName, Locality: managedruntime.LocalityLocal},
		ReadinessState: managedruntime.ReadinessStateReady,
		LifecycleState: managedruntime.LifecycleStateInstalled,
	}, nil
}

func (readyInvokeHost) Pull(context.Context, *factoryconfig.LoadedFactoryConfig, string) (modelhost.PullSnapshot, error) {
	return modelhost.PullSnapshot{}, errors.New("pull unavailable in test host")
}

func (readyInvokeHost) AcquireLease(context.Context, *factoryconfig.LoadedFactoryConfig, string, modelhost.LeaseOptions) (modelhost.Lease, error) {
	return modelhost.Lease{}, errors.New("lease unavailable in test host")
}

func (readyInvokeHost) ReleaseLease(context.Context, string) error {
	return errors.New("lease unavailable in test host")
}

func (readyInvokeHost) Unload(context.Context, *factoryconfig.LoadedFactoryConfig, string) error {
	return errors.New("unload unavailable in test host")
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

func mustGeneratedInvokeTextPart(t *testing.T, text string) factoryapi.WorkContentPart {
	t.Helper()
	var part factoryapi.WorkContentPart
	if err := part.FromWorkTextContentPart(factoryapi.WorkTextContentPart{
		Type: factoryapi.WorkContentPartTypeTextUpper,
		Text: text,
		Slot: stringPtr("text"),
	}); err != nil {
		t.Fatalf("build text content part: %v", err)
	}
	return part
}

func directInvokeRequest(t *testing.T) factoryapi.ModelInvocationRequest {
	t.Helper()
	return factoryapi.ModelInvocationRequest{
		Operation: "TTS",
		Content: &factoryapi.WorkContent{
			mustGeneratedInvokeTextPart(t, "hello world"),
		},
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

func stringPtr(value string) *string {
	return &value
}
