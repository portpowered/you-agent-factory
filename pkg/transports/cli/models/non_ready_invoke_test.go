package models

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	factoryresource "github.com/portpowered/infinite-you/pkg/factory/resource"
	managedruntime "github.com/portpowered/infinite-you/pkg/models/managedruntime"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	workerconfig "github.com/portpowered/infinite-you/pkg/workers/config"
	"go.uber.org/zap"
)

func TestInvoke_NonReadyManagedOutcomes_StubBootstrapPreservesReadinessFailureClasses(t *testing.T) {
	tests := []struct {
		name           string
		invokeErr      error
		wantIs         error
		wantContains   []string
		wantNotContain string
	}{
		{
			name: "missing_inference_failure",
			invokeErr: classifiedBootstrapInvokeFailure(
				factoryapi.ManagedRuntimeReadinessStateMISSING,
				factoryapi.ManagedRuntimeLifecycleStateNOTINSTALLED,
			),
			wantIs: apisurface.ErrManagedRuntimeMissing,
			wantContains: []string{
				"pull or install",
				"OMNIVOICE_Q4_K_M",
			},
			wantNotContain: "models endpoint not reachable",
		},
		{
			name: "loading_inference_failure",
			invokeErr: classifiedBootstrapInvokeFailure(
				factoryapi.ManagedRuntimeReadinessStateLOADING,
				factoryapi.ManagedRuntimeLifecycleStateLOADING,
			),
			wantIs: apisurface.ErrManagedRuntimeLoading,
			wantContains: []string{
				"still loading",
				"OMNIVOICE_Q4_K_M",
			},
			wantNotContain: "models endpoint not reachable",
		},
		{
			name: "failed_inference_failure",
			invokeErr: classifiedBootstrapInvokeFailure(
				factoryapi.ManagedRuntimeReadinessStateFAILED,
				factoryapi.ManagedRuntimeLifecycleStateNOTINSTALLED,
			),
			wantIs: apisurface.ErrManagedRuntimeFailed,
			wantContains: []string{
				"readiness is FAILED",
				"resolve the managed runtime failure",
			},
			wantNotContain: "models endpoint not reachable",
		},
		{
			name: "unsupported_inference_failure",
			invokeErr: classifiedBootstrapInvokeFailure(
				factoryapi.ManagedRuntimeReadinessStateUNSUPPORTED,
				factoryapi.ManagedRuntimeLifecycleStateNOTINSTALLED,
			),
			wantIs: apisurface.ErrManagedRuntimeUnsupported,
			wantContains: []string{
				"readiness is UNSUPPORTED",
				"supported managed runtime",
			},
			wantNotContain: "models endpoint not reachable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			installStubModelBootstrapRunner(t, readyStubModelBootstrapRunner(func(
				_ context.Context,
				_ string,
				_ factoryapi.ModelInvocationRequest,
			) (apisurface.ModelInvocationResult, error) {
				return apisurface.ModelInvocationResult{}, tt.invokeErr
			}))

			err := Invoke(InvokeConfig{
				BuildInvocation: testModelInvocationBuilder,
				ModelName:       "OMNIVOICE_Q4_K_M",
				Operation:       "TTS",
				Text:            "hello world",
				FactoryDir:      t.TempDir(),
				Server:          failureBaselineUnreachableServer,
				JSON:            true,
				Logger:          zap.NewNop(),
				Output:          io.Discard,
			})
			if err == nil {
				t.Fatal("expected readiness-gated invoke failure")
			}
			if tt.wantIs != nil && !errors.Is(err, tt.wantIs) {
				t.Fatalf("error = %v, want errors.Is %v", err, tt.wantIs)
			}
			for _, want := range tt.wantContains {
				if !strings.Contains(err.Error(), want) {
					t.Fatalf("error = %q, want substring %q", err.Error(), want)
				}
			}
			if tt.wantNotContain != "" && strings.Contains(err.Error(), tt.wantNotContain) {
				t.Fatalf("error = %q, want to avoid transport failure %q", err.Error(), tt.wantNotContain)
			}
		})
	}
}

func TestInvoke_NonReadyManagedOutcomes_StubBootstrapPreservesManagedRuntimeVocabulary(t *testing.T) {
	tests := []struct {
		name          string
		readiness     factoryapi.ManagedRuntimeReadinessState
		lifecycle     factoryapi.ManagedRuntimeLifecycleState
		wantIs        error
		wantReadiness managedruntime.ReadinessState
		wantLifecycle managedruntime.LifecycleState
	}{
		{
			name:          "missing",
			readiness:     factoryapi.ManagedRuntimeReadinessStateMISSING,
			lifecycle:     factoryapi.ManagedRuntimeLifecycleStateNOTINSTALLED,
			wantIs:        apisurface.ErrManagedRuntimeMissing,
			wantReadiness: managedruntime.ReadinessStateMissing,
			wantLifecycle: managedruntime.LifecycleStateNotInstalled,
		},
		{
			name:          "loading",
			readiness:     factoryapi.ManagedRuntimeReadinessStateLOADING,
			lifecycle:     factoryapi.ManagedRuntimeLifecycleStateLOADING,
			wantIs:        apisurface.ErrManagedRuntimeLoading,
			wantReadiness: managedruntime.ReadinessStateLoading,
			wantLifecycle: managedruntime.LifecycleStateLoading,
		},
		{
			name:          "failed",
			readiness:     factoryapi.ManagedRuntimeReadinessStateFAILED,
			lifecycle:     factoryapi.ManagedRuntimeLifecycleStateNOTINSTALLED,
			wantIs:        apisurface.ErrManagedRuntimeFailed,
			wantReadiness: managedruntime.ReadinessStateFailed,
			wantLifecycle: managedruntime.LifecycleStateNotInstalled,
		},
		{
			name:          "unsupported",
			readiness:     factoryapi.ManagedRuntimeReadinessStateUNSUPPORTED,
			lifecycle:     factoryapi.ManagedRuntimeLifecycleStateNOTINSTALLED,
			wantIs:        apisurface.ErrManagedRuntimeUnsupported,
			wantReadiness: managedruntime.ReadinessStateUnsupported,
			wantLifecycle: managedruntime.LifecycleStateNotInstalled,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			invokeErr := apisurface.InvocationErrorFromManagedRuntime(factoryapi.ManagedRuntime{
				Identity:       "OMNIVOICE_Q4_K_M",
				ReadinessState: tt.readiness,
				LifecycleState: tt.lifecycle,
			})
			installStubModelBootstrapRunner(t, readyStubModelBootstrapRunner(func(
				_ context.Context,
				_ string,
				_ factoryapi.ModelInvocationRequest,
			) (apisurface.ModelInvocationResult, error) {
				return apisurface.ModelInvocationResult{}, invokeErr
			}))

			err := Invoke(InvokeConfig{
				BuildInvocation: testModelInvocationBuilder,
				ModelName:       "OMNIVOICE_Q4_K_M",
				Operation:       "TTS",
				Text:            "hello world",
				FactoryDir:      t.TempDir(),
				Server:          failureBaselineUnreachableServer,
				JSON:            true,
				Logger:          zap.NewNop(),
				Output:          io.Discard,
			})
			if err == nil {
				t.Fatal("expected managed runtime readiness failure")
			}
			if !errors.Is(err, tt.wantIs) {
				t.Fatalf("error = %v, want errors.Is %v", err, tt.wantIs)
			}
			var readinessErr *apisurface.ManagedRuntimeInvocationError
			if !errors.As(err, &readinessErr) {
				t.Fatalf("error = %T, want *ManagedRuntimeInvocationError", err)
			}
			if readinessErr.ReadinessState != tt.wantReadiness {
				t.Fatalf("readiness = %s, want %s", readinessErr.ReadinessState, tt.wantReadiness)
			}
			if readinessErr.LifecycleState != tt.wantLifecycle {
				t.Fatalf("lifecycle = %s, want %s", readinessErr.LifecycleState, tt.wantLifecycle)
			}
			if !strings.Contains(err.Error(), string(tt.wantReadiness)) {
				t.Fatalf("error = %q, want readiness token %q", err.Error(), tt.wantReadiness)
			}
			if strings.Contains(err.Error(), "models endpoint not reachable") {
				t.Fatalf("error = %q, want bootstrap readiness failure instead of transport failure", err.Error())
			}
		})
	}
}

func classifiedBootstrapInvokeFailure(
	readiness factoryapi.ManagedRuntimeReadinessState,
	lifecycle factoryapi.ManagedRuntimeLifecycleState,
) error {
	readinessErr := apisurface.InvocationErrorFromManagedRuntime(factoryapi.ManagedRuntime{
		Identity:       "OMNIVOICE_Q4_K_M",
		ReadinessState: readiness,
		LifecycleState: lifecycle,
	})
	failure, ok := apisurface.ClassifyInferenceFailure(readinessErr, apisurface.InferenceFailureContext{
		ModelName:  "OMNIVOICE_Q4_K_M",
		WorkerName: "voice-local",
		Operation:  "TTS",
	})
	if !ok || failure == nil {
		panic("expected classified inference failure")
	}
	return failure
}

func nonReadyLocalModelFactoryConfig() map[string]any {
	return map[string]any{
		"name": "factory",
		"resources": []map[string]any{{
			"name":       "omnivoice-cache",
			"type":       factoryresource.TypeModel,
			"capacity":   1,
			"model":      "OMNIVOICE_Q4_K_M",
			"backend":    "LLAMACPP",
			"loadPolicy": "ON_DEMAND",
		}},
		"workers": []map[string]any{{
			"name":          "voice-local",
			"type":          interfaces.WorkerTypeModel,
			"modelProvider": "CODEX",
			"model":         "OMNIVOICE_Q4_K_M",
			"modelLocality": workerconfig.ModelLocalityLocal,
			"resources":     []map[string]any{{"name": "omnivoice-cache", "capacity": 1}},
			"operations": []map[string]any{{
				"name": "TTS",
				"inputs": []map[string]any{{
					"name":         "text",
					"contentTypes": []string{workerconfig.ModelOperationContentTypeText},
					"required":     true,
				}},
				"outputs": []map[string]any{{
					"name":         "audio",
					"contentTypes": []string{workerconfig.ModelOperationContentTypeAudio},
				}},
			}},
		}},
	}
}
