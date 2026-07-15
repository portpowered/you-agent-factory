package modeltests

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/service"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"
	workerprovider "github.com/portpowered/infinite-you/pkg/workers/provider"
)

func TestInvokeModel_ClassifiesMissingManagedRuntimeWithPullGuidance(t *testing.T) {
	cacheDir := t.TempDir()
	writeManagedCacheMetadata(t, cacheDir)
	svc := buildModelCatalogServiceWithOptions(t, modelCatalogConfig(true), service.FactoryServiceConfig{
		ModelCacheDir: cacheDir,
	})

	_, err := svc.InvokeModel(context.Background(), "OMNIVOICE_Q4_K_M", factoryapi.ModelInvocationRequest{
		Operation: "TTS",
		Content: &factoryapi.WorkContent{
			mustGeneratedServiceTextPart(t, "hello world"),
		},
	})
	failure, ok := apisurface.AsInferenceFailure(err)
	if !ok || failure.Class != apisurface.InferenceFailureClassMissingModel {
		t.Fatalf("error = %v, want missing_model InferenceFailure", err)
	}
	if !strings.Contains(failure.Message, "pull or install") {
		t.Fatalf("message = %q, want pull/install guidance", failure.Message)
	}
	if !errors.Is(err, apisurface.ErrManagedRuntimeMissing) {
		t.Fatalf("error = %v, want wrapped managed runtime missing", err)
	}
}

func TestInvokeModel_ClassifiesUnsupportedOperationWithTargetIdentity(t *testing.T) {
	svc := buildModelCatalogServiceWithOptions(t, cloudModelInvocationConfig(), service.FactoryServiceConfig{})

	_, err := svc.InvokeModel(context.Background(), "OMNIVOICE_Q4_K_M", factoryapi.ModelInvocationRequest{
		Operation: "EMBED",
		Content: &factoryapi.WorkContent{
			mustGeneratedServiceTextPart(t, "hello world"),
		},
	})
	failure, ok := apisurface.AsInferenceFailure(err)
	if !ok || failure.Class != apisurface.InferenceFailureClassUnsupportedOperation {
		t.Fatalf("error = %v, want unsupported_operation InferenceFailure", err)
	}
	for _, want := range []string{"OMNIVOICE_Q4_K_M", "EMBED", "tts-worker"} {
		if !strings.Contains(failure.Message, want) {
			t.Fatalf("message = %q, want %q", failure.Message, want)
		}
	}
}

func TestInvokeModel_ClassifiesProviderTimeoutWithoutRawLogs(t *testing.T) {
	svc := buildModelCatalogServiceWithOptions(t, cloudModelInvocationConfig(), service.FactoryServiceConfig{
		ProviderOverride: timeoutProvider{},
	})

	_, err := svc.InvokeModel(context.Background(), "OMNIVOICE_Q4_K_M", factoryapi.ModelInvocationRequest{
		Operation: "TTS",
		Content: &factoryapi.WorkContent{
			mustGeneratedServiceTextPart(t, "hello world"),
		},
	})
	failure, ok := apisurface.AsInferenceFailure(err)
	if !ok || failure.Class != apisurface.InferenceFailureClassTimeout {
		t.Fatalf("error = %v, want timeout InferenceFailure", err)
	}
	if !strings.Contains(failure.Message, "timed out") {
		t.Fatalf("message = %q, want timeout guidance", failure.Message)
	}
}

func TestInvokeModel_ClassifiesProviderRuntimeFailureWithoutRawLogs(t *testing.T) {
	svc := buildModelCatalogServiceWithOptions(t, cloudModelInvocationConfig(), service.FactoryServiceConfig{
		ProviderOverride: runtimeFailureProvider{},
	})

	_, err := svc.InvokeModel(context.Background(), "OMNIVOICE_Q4_K_M", factoryapi.ModelInvocationRequest{
		Operation: "TTS",
		Content: &factoryapi.WorkContent{
			mustGeneratedServiceTextPart(t, "hello world"),
		},
	})
	failure, ok := apisurface.AsInferenceFailure(err)
	if !ok || failure.Class != apisurface.InferenceFailureClassRuntimeFailure {
		t.Fatalf("error = %v, want runtime_failure InferenceFailure", err)
	}
	if strings.Contains(failure.Message, "subprocess transcript") {
		t.Fatalf("message leaked raw subprocess output: %q", failure.Message)
	}
}

type timeoutProvider struct{}

func (timeoutProvider) Infer(_ context.Context, _ workerexecution.ProviderInferenceRequest) (workerexecution.InferenceResponse, error) {
	return workerexecution.InferenceResponse{}, workerprovider.NewProviderError(
		workerexecution.WorkFailureTypeTimeout,
		"execution timeout",
		nil,
	)
}

type runtimeFailureProvider struct{}

func (runtimeFailureProvider) Infer(_ context.Context, _ workerexecution.ProviderInferenceRequest) (workerexecution.InferenceResponse, error) {
	return workerexecution.InferenceResponse{}, workerprovider.NewProviderError(
		workerexecution.WorkFailureTypeUnknown,
		"ERROR: subprocess transcript should not reach customers",
		nil,
	)
}
