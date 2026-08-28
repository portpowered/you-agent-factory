package http

import (
	"context"
	"errors"
	"net/http"
	"testing"

	workerconfig "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestManagedRuntimeToGeneratedPreservesModelVocabulary(t *testing.T) {
	required := true
	revision := "rev-1"
	cachePath := "/tmp/models/OMNIVOICE_Q4_K_M/rev-1"
	cacheBytes := int64(1234)
	result := managedRuntimeToGenerated(models.Runtime{
		Identity: "OMNIVOICE_Q4_K_M", ReadinessState: models.ReadinessStateReady,
		LifecycleState: models.LifecycleStateInstalled, Locality: models.LocalityLocal,
		Revision: &revision, CachePath: &cachePath, CacheBytes: &cacheBytes,
		SupportedOperations: []models.Operation{{Name: "TTS", Inputs: []models.OperationSlot{{
			Name: "text", ContentTypes: []string{"TEXT"}, Required: &required,
		}}}},
		Diagnostics: map[string]string{"sourceKind": "MANAGED_MIRROR"},
	})
	if result.ReadinessState != factoryapi.ManagedRuntimeReadinessStateREADY ||
		result.LifecycleState != factoryapi.ManagedRuntimeLifecycleStateINSTALLED ||
		result.Locality != factoryapi.WorkerModelLocalityLocal ||
		result.Revision == nil || *result.Revision != revision ||
		result.CachePath == nil || *result.CachePath != cachePath ||
		result.CacheBytes == nil || *result.CacheBytes != cacheBytes {
		t.Fatalf("managed runtime = %#v, want READY/INSTALLED/LOCAL", result)
	}
	if result.Diagnostics == nil || (*result.Diagnostics)["sourceKind"] != "MANAGED_MIRROR" {
		t.Fatalf("diagnostics = %#v, want source kind", result.Diagnostics)
	}
}

func TestManagedRuntimePullMapping(t *testing.T) {
	result := models.PullResult{
		ModelName: "OMNIVOICE_Q4_K_M", ProviderLocality: workerconfig.ModelLocalityLocal,
		Outcome: "PULLED", CachePath: "/tmp/cache", Revision: "rev1",
		DownloadedFiles: []models.DownloadedFile{{Path: "model.gguf", Bytes: 10}},
	}
	response := modelPullResponseFromService(result)
	if response.ManagedRuntimePull.PullOutcome != factoryapi.ManagedRuntimePullOutcomeINSTALLEDSUCCESSFULLY ||
		response.ManagedRuntimePull.ReadinessState != factoryapi.ManagedRuntimeReadinessStateREADY {
		t.Fatalf("managed pull = %#v, want installed and ready", response.ManagedRuntimePull)
	}

	failure := models.PullResult{ManagedPullOutcome: "TIMED_OUT", ReadinessState: "FAILED"}
	cause := errors.Join(context.DeadlineExceeded, models.ErrSourceFetchFailed)
	err := &models.PullError{Result: failure, Cause: cause}
	var classified *models.PullError
	if !errors.As(err, &classified) || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("pull error = %v, want classified deadline failure", err)
	}
	httpFailure := models.PullResult{
		ModelName:          "voice",
		ManagedPullOutcome: "SOURCE_FETCH_FAILED",
		ReadinessState:     "FAILED",
		PullDiagnostics: models.PullDiagnostics{
			ModelName: "voice", ResolvedRepository: "owner/repo", Revision: "rev-1",
			File: "weights.gguf", Operation: "download asset",
			RequestURL:         "https://assets.example.test/owner/repo/weights.gguf?download=true",
			UpstreamStatusCode: http.StatusBadGateway,
		},
	}
	mappedResponse := modelPullResponseFromService(httpFailure)
	diagnostics := mappedResponse.ManagedRuntimePull.PullDiagnostics
	if diagnostics == nil || diagnostics.ResolvedRepository == nil || *diagnostics.ResolvedRepository != "owner/repo" ||
		diagnostics.RequestUrl == nil || *diagnostics.RequestUrl != "https://assets.example.test/owner/repo/weights.gguf?download=true" ||
		diagnostics.UpstreamStatusCode == nil || *diagnostics.UpstreamStatusCode != int32(http.StatusBadGateway) {
		t.Fatalf("mapped pull diagnostics = %#v repo=%q url=%q status=%v, want structured HTTP facts", diagnostics, stringValue(diagnostics.ResolvedRepository), stringValue(diagnostics.RequestUrl), int32Value(diagnostics.UpstreamStatusCode))
	}
}

func TestInferenceFailureMapping(t *testing.T) {
	failure := &models.InferenceFailure{Class: models.InferenceFailureClassLoadingModel, Message: "loading"}
	if got := inferenceFailureHTTPStatus(failure); got != http.StatusConflict {
		t.Fatalf("status = %d, want %d", got, http.StatusConflict)
	}
	if got := inferenceFailureErrorCode(failure); got != "MODEL_RUNTIME_LOADING" {
		t.Fatalf("code = %q, want MODEL_RUNTIME_LOADING", got)
	}
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func int32Value(value *int32) int32 {
	if value == nil {
		return 0
	}
	return *value
}
