package inferencefailure_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestWorkersClassifyInferenceFailureOwnsReadinessAndExecutionPolicy(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		err       error
		wantClass workers.InferenceFailureClass
		wantText  string
	}{
		{
			name:      "missing managed runtime",
			err:       &models.InvocationError{Identity: "model-a", ReadinessState: models.ReadinessStateMissing, Cause: models.ErrMissing},
			wantClass: workers.InferenceFailureClassMissingModel, wantText: "pull or install",
		},
		{
			name: "loading managed runtime", err: fmt.Errorf("%w: in progress", models.ErrLoading),
			wantClass: workers.InferenceFailureClassLoadingModel, wantText: "retry",
		},
		{
			name: "unsupported operation", err: fmt.Errorf("%w: does not support operation", models.ErrUnsupportedOperation),
			wantClass: workers.InferenceFailureClassUnsupportedOperation, wantText: "does not support operation",
		},
		{
			name: "provider timeout", err: workers.NewProviderError(workers.WorkFailureTypeTimeout, "execution timeout", context.DeadlineExceeded),
			wantClass: workers.InferenceFailureClassTimeout, wantText: "retry",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			failure, ok := workers.ClassifyInferenceFailure(tt.err, workers.InferenceFailureContext{ModelName: "model-a", WorkerName: "worker-a", Operation: "TTS"})
			if !ok || failure.Class != tt.wantClass || !strings.Contains(failure.Message, tt.wantText) {
				t.Fatalf("failure = %#v, want class %q containing %q", failure, tt.wantClass, tt.wantText)
			}
		})
	}
}

func TestWorkersClassifyInferenceFailureUsesOwnerTargetAndPreservesCause(t *testing.T) {
	t.Parallel()
	cause := workers.NewProviderError(workers.WorkFailureTypeTimeout, "provider timed out", context.DeadlineExceeded)
	failure, ok := workers.ClassifyInferenceFailure(&models.TargetError{
		ModelName: "model-a", WorkerName: "worker-a", Operation: "TTS", Cause: cause,
	}, workers.InferenceFailureContext{ModelName: "request-model", Operation: "request-operation"})
	if !ok || failure.ModelName != "model-a" || failure.WorkerName != "worker-a" || failure.Operation != "TTS" {
		t.Fatalf("failure target = %#v, want owner target", failure)
	}
	if !errors.Is(failure, context.DeadlineExceeded) {
		t.Fatalf("failure = %v, want deadline cause", failure)
	}
}

func TestWorkersClassifyInferenceFailureSuppressesRawSubprocessDiagnostics(t *testing.T) {
	t.Parallel()
	raw := strings.Repeat("subprocess transcript token ", 200) + "exited with code 1"
	failure, ok := workers.ClassifyInferenceFailure(errors.New(raw), workers.InferenceFailureContext{ModelName: "model-a", Operation: "TTS"})
	if !ok || failure.Class != workers.InferenceFailureClassRuntimeFailure {
		t.Fatalf("failure = %#v, want runtime failure", failure)
	}
	if strings.Contains(failure.Message, "transcript token") {
		t.Fatalf("customer message leaked raw subprocess diagnostics: %q", failure.Message)
	}
}

func TestClassifyInferenceWorkResultFailureOwnsFailureMetadataPolicy(t *testing.T) {
	t.Parallel()
	failure, ok := workers.ClassifyInferenceWorkResultFailure(workers.WorkResult{
		Outcome: workers.OutcomeFailed, Error: "execution timeout",
		FailureMetadata: &workers.WorkFailureMetadata{Type: workers.WorkFailureTypeTimeout},
	}, workers.InferenceFailureContext{ModelName: "model-a", Operation: "TTS"})
	if !ok || failure.Class != workers.InferenceFailureClassTimeout {
		t.Fatalf("failure = %#v, want timeout", failure)
	}
}
