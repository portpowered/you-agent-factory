package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"testing"

	managedruntime "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
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
			wantIs: managedruntime.ErrMissing,
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
			wantIs: managedruntime.ErrLoading,
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
			wantIs: managedruntime.ErrFailed,
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
			wantIs: managedruntime.ErrUnsupported,
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
			) (managedruntime.Result, error) {
				return managedruntime.Result{}, tt.invokeErr
			}))

			err := invokeForTest(t, InvokeConfig{Context: context.Background(),
				ModelName:  "OMNIVOICE_Q4_K_M",
				Operation:  "TTS",
				Text:       "hello world",
				FactoryDir: t.TempDir(),
				Server:     failureBaselineUnreachableServer,
				OutputPath: filepath.Join(t.TempDir(), "speech.wav"),
				Logger:     zap.NewNop(),
				Output:     io.Discard,
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
			wantIs:        managedruntime.ErrMissing,
			wantReadiness: managedruntime.ReadinessStateMissing,
			wantLifecycle: managedruntime.LifecycleStateNotInstalled,
		},
		{
			name:          "loading",
			readiness:     factoryapi.ManagedRuntimeReadinessStateLOADING,
			lifecycle:     factoryapi.ManagedRuntimeLifecycleStateLOADING,
			wantIs:        managedruntime.ErrLoading,
			wantReadiness: managedruntime.ReadinessStateLoading,
			wantLifecycle: managedruntime.LifecycleStateLoading,
		},
		{
			name:          "failed",
			readiness:     factoryapi.ManagedRuntimeReadinessStateFAILED,
			lifecycle:     factoryapi.ManagedRuntimeLifecycleStateNOTINSTALLED,
			wantIs:        managedruntime.ErrFailed,
			wantReadiness: managedruntime.ReadinessStateFailed,
			wantLifecycle: managedruntime.LifecycleStateNotInstalled,
		},
		{
			name:          "unsupported",
			readiness:     factoryapi.ManagedRuntimeReadinessStateUNSUPPORTED,
			lifecycle:     factoryapi.ManagedRuntimeLifecycleStateNOTINSTALLED,
			wantIs:        managedruntime.ErrUnsupported,
			wantReadiness: managedruntime.ReadinessStateUnsupported,
			wantLifecycle: managedruntime.LifecycleStateNotInstalled,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			invokeErr := (managedruntime.Runtime{
				Identity:       "OMNIVOICE_Q4_K_M",
				ReadinessState: managedruntime.ReadinessState(tt.readiness),
				LifecycleState: managedruntime.LifecycleState(tt.lifecycle),
			}).InvocationError()
			installStubModelBootstrapRunner(t, readyStubModelBootstrapRunner(func(
				_ context.Context,
				_ string,
				_ factoryapi.ModelInvocationRequest,
			) (managedruntime.Result, error) {
				return managedruntime.Result{}, invokeErr
			}))

			err := invokeForTest(t, InvokeConfig{Context: context.Background(),
				ModelName:  "OMNIVOICE_Q4_K_M",
				Operation:  "TTS",
				Text:       "hello world",
				FactoryDir: t.TempDir(),
				Server:     failureBaselineUnreachableServer,
				OutputPath: filepath.Join(t.TempDir(), "speech.wav"),
				Logger:     zap.NewNop(),
				Output:     io.Discard,
			})
			if err == nil {
				t.Fatal("expected managed runtime readiness failure")
			}
			if !errors.Is(err, tt.wantIs) {
				t.Fatalf("error = %v, want errors.Is %v", err, tt.wantIs)
			}
			var readinessErr *managedruntime.InvocationError
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
	readinessErr := (managedruntime.Runtime{
		Identity:       "OMNIVOICE_Q4_K_M",
		ReadinessState: managedruntime.ReadinessState(readiness),
		LifecycleState: managedruntime.LifecycleState(lifecycle),
	}).InvocationError()
	message := readinessErr.Error()
	switch readiness {
	case factoryapi.ManagedRuntimeReadinessStateMISSING:
		message = "model \"OMNIVOICE_Q4_K_M\" is not available: pull or install the managed runtime before invoking"
	case factoryapi.ManagedRuntimeReadinessStateLOADING:
		message = "model \"OMNIVOICE_Q4_K_M\" is still loading: wait for the managed runtime to finish loading and retry the invocation"
	}
	return &managedruntime.InferenceFailure{
		Class: readinessFailureClass(readiness), Message: message,
		ModelName: "OMNIVOICE_Q4_K_M", WorkerName: "voice-local", Operation: "TTS", Cause: readinessErr,
	}
}

func readinessFailureClass(readiness factoryapi.ManagedRuntimeReadinessState) managedruntime.InferenceFailureClass {
	switch readiness {
	case factoryapi.ManagedRuntimeReadinessStateMISSING:
		return managedruntime.InferenceFailureClassMissingModel
	case factoryapi.ManagedRuntimeReadinessStateLOADING:
		return managedruntime.InferenceFailureClassLoadingModel
	default:
		return managedruntime.InferenceFailureClassRuntimeFailure
	}
}

func TestInvokeGenericMapsBackendPreflightFailure(t *testing.T) {
	t.Parallel()

	scope, err := (managedruntime.RuntimeScopeRef{}).Parse("asset-estimate:failure")
	if err != nil {
		t.Fatalf("parse runtime scope: %v", err)
	}
	catalog := managedruntime.Detail{Summary: managedruntime.Summary{
		Operations: []managedruntime.Operation{{
			Name:    managedruntime.OperationOMNI,
			Outputs: []managedruntime.OperationSlot{{Name: "text", Modality: managedruntime.ModalityText}},
		}},
	}}
	root := &genericCLIModelsService{
		catalog:      catalog,
		preflightErr: fmt.Errorf("controlled HEAD failed: %w", managedruntime.ErrAssetBackendNotReady),
	}
	service := &rootService{models: root}
	handled, err := service.invokeGenericInScope(
		InvokeConfig{Context: context.Background(), Output: io.Discard, JSON: true},
		scope, "model", managedruntime.OperationOMNI, "hello", catalog,
	)
	if !handled || err == nil {
		t.Fatalf("invokeGenericInScope = handled:%v error:%v, want typed failure", handled, err)
	}
	var coded interface {
		CLIErrorCode() string
		CLIErrorMessage() string
	}
	if !errors.As(err, &coded) || coded.CLIErrorCode() != "MODEL_BACKEND_NOT_READY" || coded.CLIErrorMessage() != "managed model backend is unavailable" {
		t.Fatalf("mapped preflight error = %v, want backend readiness diagnostic", err)
	}
}
