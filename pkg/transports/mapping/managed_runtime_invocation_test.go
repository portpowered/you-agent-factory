package apisurface

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	managedruntime "github.com/portpowered/infinite-you/pkg/models/managedruntime"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"
	workerprovider "github.com/portpowered/infinite-you/pkg/workers/provider"
)

func TestInvocationResponseFromResult_MapsDomainTerminalStatus(t *testing.T) {
	response := InvocationResponseFromResult(interfaces.FactoryInvocationResult{
		RequestID: "request-1",
		TraceID:   "trace-1",
		Status:    interfaces.InvocationTerminalStatusTimedOut,
		ErrorCode: string(interfaces.InvocationErrorCodeTimedOut),
	})

	if response.Status != factoryapi.InvocationTerminalStatusTimedOut {
		t.Fatalf("status = %q, want TIMED_OUT", response.Status)
	}
	if response.ErrorCode == nil || *response.ErrorCode != factoryapi.INVOCATIONTIMEDOUT {
		t.Fatalf("error code = %#v, want INVOCATION_TIMED_OUT", response.ErrorCode)
	}
}

func TestInvocationErrorFromManagedRuntime_ReadyAllowsInvocation(t *testing.T) {
	err := InvocationErrorFromManagedRuntime(factoryapi.ManagedRuntime{
		Identity:       "OMNIVOICE_Q4_K_M",
		ReadinessState: factoryapi.ManagedRuntimeReadinessStateREADY,
		LifecycleState: factoryapi.ManagedRuntimeLifecycleStateINSTALLED,
	})
	if err != nil {
		t.Fatalf("error = %v, want nil for READY", err)
	}
}

func TestInvocationErrorFromManagedRuntime_MissingUsesManagedVocabulary(t *testing.T) {
	err := InvocationErrorFromManagedRuntime(factoryapi.ManagedRuntime{
		Identity:       "OMNIVOICE_Q4_K_M",
		ReadinessState: factoryapi.ManagedRuntimeReadinessStateMISSING,
		LifecycleState: factoryapi.ManagedRuntimeLifecycleStateNOTINSTALLED,
	})
	if err == nil {
		t.Fatal("error = nil, want managed runtime missing")
	}
	if !errors.Is(err, ErrManagedRuntimeMissing) {
		t.Fatalf("error = %v, want ErrManagedRuntimeMissing", err)
	}
	var readinessErr *ManagedRuntimeInvocationError
	if !errors.As(err, &readinessErr) {
		t.Fatalf("error = %T, want *ManagedRuntimeInvocationError", err)
	}
	if readinessErr.ReadinessState != managedruntime.ReadinessStateMissing ||
		readinessErr.LifecycleState != managedruntime.LifecycleStateNotInstalled {
		t.Fatalf("readiness = (%s, %s), want MISSING NOT_INSTALLED", readinessErr.ReadinessState, readinessErr.LifecycleState)
	}
}

func TestInvocationErrorFromManagedRuntime_LoadingAndFailed(t *testing.T) {
	loadingErr := InvocationErrorFromManagedRuntime(factoryapi.ManagedRuntime{
		Identity:       "OMNIVOICE_Q4_K_M",
		ReadinessState: factoryapi.ManagedRuntimeReadinessStateLOADING,
		LifecycleState: factoryapi.ManagedRuntimeLifecycleStateLOADING,
	})
	if !errors.Is(loadingErr, ErrManagedRuntimeLoading) {
		t.Fatalf("loading error = %v, want ErrManagedRuntimeLoading", loadingErr)
	}

	failedErr := InvocationErrorFromManagedRuntime(factoryapi.ManagedRuntime{
		Identity:       "OMNIVOICE_Q4_K_M",
		ReadinessState: factoryapi.ManagedRuntimeReadinessStateFAILED,
		LifecycleState: factoryapi.ManagedRuntimeLifecycleStateNOTINSTALLED,
	})
	if !errors.Is(failedErr, ErrManagedRuntimeFailed) {
		t.Fatalf("failed error = %v, want ErrManagedRuntimeFailed", failedErr)
	}
}

func TestClassifyInferenceFailure_MissingModelReturnsPullGuidance(t *testing.T) {
	ctx := InferenceFailureContext{
		ModelName:  "OMNIVOICE_Q4_K_M",
		WorkerName: "tts-worker",
		Operation:  "TTS",
	}
	failure, ok := ClassifyInferenceFailure(
		InvocationErrorFromManagedRuntime(factoryapi.ManagedRuntime{
			Identity:       "OMNIVOICE_Q4_K_M",
			ReadinessState: factoryapi.ManagedRuntimeReadinessStateMISSING,
			LifecycleState: factoryapi.ManagedRuntimeLifecycleStateNOTINSTALLED,
		}),
		ctx,
	)
	if !ok || failure.Class != InferenceFailureClassMissingModel {
		t.Fatalf("failure = %#v, want missing_model", failure)
	}
	if !strings.Contains(failure.Message, "pull or install") {
		t.Fatalf("message = %q, want pull/install guidance", failure.Message)
	}
}

func TestClassifyInferenceFailure_LoadingModelReturnsWaitGuidance(t *testing.T) {
	ctx := InferenceFailureContext{ModelName: "OMNIVOICE_Q4_K_M", Operation: "TTS"}
	failure, ok := ClassifyInferenceFailure(
		fmt.Errorf("%w: download in progress", ErrManagedRuntimeLoading),
		ctx,
	)
	if !ok || failure.Class != InferenceFailureClassLoadingModel {
		t.Fatalf("failure = %#v, want loading_model", failure)
	}
	if !strings.Contains(failure.Message, "wait") || !strings.Contains(failure.Message, "retry") {
		t.Fatalf("message = %q, want wait/retry guidance", failure.Message)
	}
}

func TestClassifyInferenceFailure_UnsupportedOperationIdentifiesTarget(t *testing.T) {
	ctx := InferenceFailureContext{
		ModelName:  "OMNIVOICE_Q4_K_M",
		WorkerName: "tts-worker",
		Operation:  "EMBED",
	}
	failure, ok := ClassifyInferenceFailure(
		fmt.Errorf("%w: worker %q for model %q does not support operation %q", ErrModelInvocationUnsupportedOperation, ctx.WorkerName, ctx.ModelName, ctx.Operation),
		ctx,
	)
	if !ok || failure.Class != InferenceFailureClassUnsupportedOperation {
		t.Fatalf("failure = %#v, want unsupported_operation", failure)
	}
	for _, want := range []string{"tts-worker", "OMNIVOICE_Q4_K_M", "EMBED"} {
		if !strings.Contains(failure.Message, want) {
			t.Fatalf("message = %q, want %q", failure.Message, want)
		}
	}
}

func TestClassifyInferenceFailure_TimeoutUsesActionableMessage(t *testing.T) {
	ctx := InferenceFailureContext{ModelName: "OMNIVOICE_Q4_K_M", Operation: "TTS"}
	failure, ok := ClassifyInferenceFailure(
		workerprovider.NewProviderError(workerexecution.WorkFailureTypeTimeout, "execution timeout", context.DeadlineExceeded),
		ctx,
	)
	if !ok || failure.Class != InferenceFailureClassTimeout {
		t.Fatalf("failure = %#v, want timeout", failure)
	}
	if !strings.Contains(failure.Message, "timed out") || !strings.Contains(failure.Message, "retry") {
		t.Fatalf("message = %q, want timeout retry guidance", failure.Message)
	}
}

func TestClassifyInferenceFailure_RuntimeFailureSuppressesRawSubprocessLogs(t *testing.T) {
	ctx := InferenceFailureContext{
		ModelName:  "OMNIVOICE_Q4_K_M",
		WorkerName: "tts-worker",
		Operation:  "TTS",
	}
	raw := strings.Repeat("subprocess transcript token ", 200) + "exited with code 1"
	failure, ok := ClassifyInferenceFailure(errors.New(raw), ctx)
	if !ok || failure.Class != InferenceFailureClassRuntimeFailure {
		t.Fatalf("failure = %#v, want runtime_failure", failure)
	}
	if strings.Contains(failure.Message, "subprocess transcript token") {
		t.Fatalf("message leaked raw subprocess output: %q", failure.Message)
	}
}

func TestClassifyInferenceWorkResultFailure_UsesFailureMetadata(t *testing.T) {
	ctx := InferenceFailureContext{ModelName: "OMNIVOICE_Q4_K_M", Operation: "TTS"}
	failure, ok := ClassifyInferenceWorkResultFailure(workerexecution.WorkResult{
		Outcome: workerexecution.OutcomeFailed,
		Error:   "execution timeout",
		FailureMetadata: &workerexecution.WorkFailureMetadata{
			Type: workerexecution.WorkFailureTypeTimeout,
		},
	}, ctx)
	if !ok || failure.Class != InferenceFailureClassTimeout {
		t.Fatalf("failure = %#v, want timeout", failure)
	}
}

func TestInferenceFailureHTTPMapping(t *testing.T) {
	tests := []struct {
		class      InferenceFailureClass
		wantStatus int
		wantCode   string
	}{
		{InferenceFailureClassMissingModel, http.StatusNotFound, "MODEL_NOT_AVAILABLE"},
		{InferenceFailureClassLoadingModel, http.StatusConflict, "MODEL_RUNTIME_LOADING"},
		{InferenceFailureClassUnsupportedOperation, http.StatusBadRequest, "BAD_REQUEST"},
		{InferenceFailureClassTimeout, http.StatusGatewayTimeout, "MODEL_INFERENCE_TIMEOUT"},
		{InferenceFailureClassRuntimeFailure, http.StatusInternalServerError, "MODEL_INFERENCE_RUNTIME_FAILURE"},
	}
	for _, tt := range tests {
		t.Run(string(tt.class), func(t *testing.T) {
			failure := &InferenceFailure{Class: tt.class, Message: "test"}
			if got := InferenceFailureHTTPStatus(failure); got != tt.wantStatus {
				t.Fatalf("status = %d, want %d", got, tt.wantStatus)
			}
			if got := InferenceFailureErrorCode(failure); got != tt.wantCode {
				t.Fatalf("code = %q, want %q", got, tt.wantCode)
			}
		})
	}
}
