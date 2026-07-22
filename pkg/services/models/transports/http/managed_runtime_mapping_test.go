package http

import (
	"context"
	"errors"
	"net/http"
	"testing"

	workerconfig "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestManagedRuntimeToGeneratedPreservesModelVocabulary(t *testing.T) {
	required := true
	result := managedRuntimeToGenerated(models.Runtime{
		Identity: "OMNIVOICE_Q4_K_M", ReadinessState: models.ReadinessStateReady,
		LifecycleState: models.LifecycleStateInstalled, Locality: models.LocalityLocal,
		SupportedOperations: []models.Operation{{Name: "TTS", Inputs: []models.OperationSlot{{
			Name: "text", ContentTypes: []string{"TEXT"}, Required: &required,
		}}}},
		Diagnostics: map[string]string{"sourceKind": "MANAGED_MIRROR"},
	})
	if result.ReadinessState != factoryapi.ManagedRuntimeReadinessStateREADY ||
		result.LifecycleState != factoryapi.ManagedRuntimeLifecycleStateINSTALLED ||
		result.Locality != factoryapi.WorkerModelLocalityLocal {
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
	if got := managedRuntimePullHTTPStatus(failure); got != http.StatusGatewayTimeout {
		t.Fatalf("status = %d, want %d", got, http.StatusGatewayTimeout)
	}
	cause := errors.Join(context.DeadlineExceeded, models.ErrSourceFetchFailed)
	err := &models.PullError{Result: failure, Cause: cause}
	var classified *models.PullError
	if !errors.As(err, &classified) || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("pull error = %v, want classified deadline failure", err)
	}
}

func TestInferenceFailureMapping(t *testing.T) {
	failure := &workers.InferenceFailure{Class: workers.InferenceFailureClassLoadingModel, Message: "loading"}
	if got := inferenceFailureHTTPStatus(failure); got != http.StatusConflict {
		t.Fatalf("status = %d, want %d", got, http.StatusConflict)
	}
	if got := inferenceFailureErrorCode(failure); got != "MODEL_RUNTIME_LOADING" {
		t.Fatalf("code = %q, want MODEL_RUNTIME_LOADING", got)
	}
}
