package agentrun

import (
	"errors"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/modelhost"
)

func TestFailureClassForError_ModelhostLeaseDenied(t *testing.T) {
	t.Parallel()

	err := &modelhost.ReadinessError{
		Snapshot: modelhost.ReadinessSnapshot{
			Identity:       modelhost.Identity{Name: "OMNIVOICE_Q4_K_M"},
			ReadinessState: factoryapi.ManagedRuntimeReadinessStateFAILED,
			FailureClass:   modelhost.FailureClassCapacityExhausted,
		},
		Cause: modelhost.ErrCapacityExhausted,
	}
	if got := failureClassForError(err); got != FailureClassLeaseDenied {
		t.Fatalf("failureClassForError = %q, want %q", got, FailureClassLeaseDenied)
	}
}

func TestFailureClassForError_ModelhostMissingReadiness(t *testing.T) {
	t.Parallel()

	err := &modelhost.ReadinessError{
		Snapshot: modelhost.ReadinessSnapshot{
			Identity:       modelhost.Identity{Name: "OMNIVOICE_Q4_K_M"},
			ReadinessState: factoryapi.ManagedRuntimeReadinessStateMISSING,
			FailureClass:   modelhost.FailureClassMissingAssets,
		},
		Cause: modelhost.ErrRuntimeNotReady,
	}
	if got := failureClassForError(err); got != FailureClassModelNotReady {
		t.Fatalf("failureClassForError = %q, want %q", got, FailureClassModelNotReady)
	}
}

func TestFailureClassForError_ModelhostRuntimeFailure(t *testing.T) {
	t.Parallel()

	err := &modelhost.ReadinessError{
		Snapshot: modelhost.ReadinessSnapshot{
			Identity:       modelhost.Identity{Name: "OMNIVOICE_Q4_K_M"},
			ReadinessState: factoryapi.ManagedRuntimeReadinessStateFAILED,
			FailureClass:   modelhost.FailureClassProcessCrash,
		},
		Cause: modelhost.ErrProcessCrash,
	}
	if got := failureClassForError(err); got != FailureClassModelRuntime {
		t.Fatalf("failureClassForError = %q, want %q", got, FailureClassModelRuntime)
	}
}

func TestFailureClassForError_ManagedRuntimeInvocationMissing(t *testing.T) {
	t.Parallel()

	err := &apisurface.ManagedRuntimeInvocationError{
		Identity:       "OMNIVOICE_Q4_K_M",
		ReadinessState: factoryapi.ManagedRuntimeReadinessStateMISSING,
		Cause:          apisurface.ErrManagedRuntimeMissing,
	}
	if got := failureClassForError(err); got != FailureClassModelNotReady {
		t.Fatalf("failureClassForError = %q, want %q", got, FailureClassModelNotReady)
	}
}

func TestAgentRunFailureDiagnostics_IncludesRecoveryAction(t *testing.T) {
	t.Parallel()

	diagnostics := agentRunFailureDiagnostics(modelhost.ErrCapacityExhausted)
	if diagnostics[DiagnosticFailureClass] != FailureClassLeaseDenied {
		t.Fatalf("failure class = %q, want %q", diagnostics[DiagnosticFailureClass], FailureClassLeaseDenied)
	}
	if diagnostics[DiagnosticRecoveryAction] == "" {
		t.Fatal("expected recovery action for lease denial")
	}
}

func TestFailureMetadataForError_ModelhostLeaseDeniedIsThrottle(t *testing.T) {
	t.Parallel()

	metadata := failureMetadataForError(modelhost.ErrCapacityExhausted)
	if metadata == nil {
		t.Fatal("expected failure metadata")
	}
	if metadata.Family != interfaces.WorkFailureFamilyThrottle || metadata.Type != interfaces.WorkFailureTypeThrottled {
		t.Fatalf("metadata = %#v, want throttle family", metadata)
	}
}

func TestFormatAgentRunError_ModelhostErrorsUseAgentRunWording(t *testing.T) {
	t.Parallel()

	got := formatAgentRunError(errors.Join(errors.New("wrapped"), modelhost.ErrCapacityExhausted))
	if got == "" || got == "provider error" {
		t.Fatalf("formatAgentRunError = %q, want agent-run lease denial wording", got)
	}
}
