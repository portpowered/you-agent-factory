package workers

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/models"
)

func TestClassifyInferenceFailureOwnsReadinessAndExecutionPolicy(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		err       error
		wantClass InferenceFailureClass
		wantText  string
	}{
		{
			name:      "missing managed runtime",
			err:       &models.InvocationError{Identity: "model-a", ReadinessState: models.ReadinessStateMissing, Cause: models.ErrMissing},
			wantClass: InferenceFailureClassMissingModel, wantText: "pull or install",
		},
		{
			name: "loading managed runtime", err: fmt.Errorf("%w: in progress", models.ErrLoading),
			wantClass: InferenceFailureClassLoadingModel, wantText: "retry",
		},
		{
			name: "unsupported operation", err: fmt.Errorf("%w: does not support operation", models.ErrUnsupportedOperation),
			wantClass: InferenceFailureClassUnsupportedOperation, wantText: "does not support operation",
		},
		{
			name: "provider timeout", err: NewProviderError(WorkFailureTypeTimeout, "execution timeout", context.DeadlineExceeded),
			wantClass: InferenceFailureClassTimeout, wantText: "retry",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			failure, ok := ClassifyInferenceFailure(tt.err, InferenceFailureContext{ModelName: "model-a", WorkerName: "worker-a", Operation: "TTS"})
			if !ok || failure.Class != tt.wantClass || !strings.Contains(failure.Message, tt.wantText) {
				t.Fatalf("failure = %#v, want class %q containing %q", failure, tt.wantClass, tt.wantText)
			}
		})
	}
}

func TestClassifyInferenceFailureUsesOwnerTargetAndPreservesCause(t *testing.T) {
	t.Parallel()
	cause := NewProviderError(WorkFailureTypeTimeout, "provider timed out", context.DeadlineExceeded)
	failure, ok := ClassifyInferenceFailure(&models.TargetError{
		ModelName: "model-a", WorkerName: "worker-a", Operation: "TTS", Cause: cause,
	}, InferenceFailureContext{ModelName: "request-model", Operation: "request-operation"})
	if !ok || failure.ModelName != "model-a" || failure.WorkerName != "worker-a" || failure.Operation != "TTS" {
		t.Fatalf("failure target = %#v, want owner target", failure)
	}
	if !errors.Is(failure, context.DeadlineExceeded) {
		t.Fatalf("failure = %v, want deadline cause", failure)
	}
}

func TestClassifyInferenceFailureSuppressesRawSubprocessDiagnostics(t *testing.T) {
	t.Parallel()
	raw := strings.Repeat("subprocess transcript token ", 200) + "exited with code 1"
	failure, ok := ClassifyInferenceFailure(errors.New(raw), InferenceFailureContext{ModelName: "model-a", Operation: "TTS"})
	if !ok || failure.Class != InferenceFailureClassRuntimeFailure {
		t.Fatalf("failure = %#v, want runtime failure", failure)
	}
	if strings.Contains(failure.Message, "transcript token") {
		t.Fatalf("customer message leaked raw subprocess diagnostics: %q", failure.Message)
	}
}

func TestClassifyInferenceWorkResultFailureOwnsFailureMetadataPolicy(t *testing.T) {
	t.Parallel()
	failure, ok := ClassifyInferenceWorkResultFailure(WorkResult{
		Outcome: OutcomeFailed, Error: "execution timeout",
		FailureMetadata: &WorkFailureMetadata{Type: WorkFailureTypeTimeout},
	}, InferenceFailureContext{ModelName: "model-a", Operation: "TTS"})
	if !ok || failure.Class != InferenceFailureClassTimeout {
		t.Fatalf("failure = %#v, want timeout", failure)
	}
}
