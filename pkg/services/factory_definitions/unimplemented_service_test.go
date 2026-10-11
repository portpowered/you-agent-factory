package factorydefinitions_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func TestAtomicFactoryWriteFailure_ErrorUnwrapIs(t *testing.T) {
	t.Parallel()

	var nilFailure *factorydefinitions.AtomicFactoryWriteFailure
	if got := nilFailure.Error(); got != factorydefinitions.ErrAtomicFactoryWriteFailed.Error() {
		t.Fatalf("nil Error: got %q", got)
	}
	if !errors.Is(nilFailure.Unwrap(), factorydefinitions.ErrAtomicFactoryWriteFailed) {
		t.Fatalf("nil Unwrap: got %v", nilFailure.Unwrap())
	}

	cause := fmt.Errorf("disk full")
	withCause := &factorydefinitions.AtomicFactoryWriteFailure{
		Name:              "alpha",
		PreviousPreserved: true,
		Cause:             cause,
	}
	if !strings.Contains(withCause.Error(), cause.Error()) {
		t.Fatalf("Error with cause: got %q", withCause.Error())
	}
	if !errors.Is(withCause, factorydefinitions.ErrAtomicFactoryWriteFailed) {
		t.Fatal("expected errors.Is ErrAtomicFactoryWriteFailed")
	}
	if !errors.Is(withCause, cause) {
		t.Fatal("expected errors.Is cause via Unwrap")
	}

	preservedOnly := &factorydefinitions.AtomicFactoryWriteFailure{
		Name:              "beta",
		PreviousPreserved: true,
	}
	if !strings.Contains(preservedOnly.Error(), `"beta"`) {
		t.Fatalf("Error preserved: got %q", preservedOnly.Error())
	}
	if preservedOnly.Unwrap() != factorydefinitions.ErrAtomicFactoryWriteFailed {
		t.Fatalf("Unwrap preserved: got %v", preservedOnly.Unwrap())
	}

	bare := &factorydefinitions.AtomicFactoryWriteFailure{}
	if bare.Error() != factorydefinitions.ErrAtomicFactoryWriteFailed.Error() {
		t.Fatalf("bare Error: got %q", bare.Error())
	}
}

func TestFactoryDefinitionValidationFailure_ErrorUnwrapIs(t *testing.T) {
	t.Parallel()

	var nilFailure *factorydefinitions.FactoryDefinitionValidationFailure
	if got := nilFailure.Error(); got != factorydefinitions.ErrFactoryDefinitionValidationFailed.Error() {
		t.Fatalf("nil Error: got %q", got)
	}
	if !errors.Is(nilFailure.Unwrap(), factorydefinitions.ErrFactoryDefinitionValidationFailed) {
		t.Fatalf("nil Unwrap: got %v", nilFailure.Unwrap())
	}

	cause := fmt.Errorf("decode failed")
	withCause := &factorydefinitions.FactoryDefinitionValidationFailure{Cause: cause}
	if !strings.Contains(withCause.Error(), cause.Error()) {
		t.Fatalf("Error with cause: got %q", withCause.Error())
	}
	if !errors.Is(withCause, factorydefinitions.ErrFactoryDefinitionValidationFailed) {
		t.Fatal("expected errors.Is ErrFactoryDefinitionValidationFailed")
	}
	if !errors.Is(withCause, cause) {
		t.Fatal("expected errors.Is cause via Unwrap")
	}

	withFindings := &factorydefinitions.FactoryDefinitionValidationFailure{
		Validation: factorydefinitions.ValidationResult{
			Targets: []factorydefinitions.ValidationTarget{{
				Code:     "factory.topology.invalid",
				Severity: factorydefinitions.ValidationSeverityError,
				Message:  "bad topology",
			}, {
				Code:     "factory.layout.hint",
				Severity: factorydefinitions.ValidationSeverityHint,
				Message:  "hint only",
			}},
		},
	}
	if !strings.Contains(withFindings.Error(), "1 error findings") {
		t.Fatalf("Error with findings: got %q", withFindings.Error())
	}
	if withFindings.Unwrap() != factorydefinitions.ErrFactoryDefinitionValidationFailed {
		t.Fatalf("Unwrap findings: got %v", withFindings.Unwrap())
	}

	bare := &factorydefinitions.FactoryDefinitionValidationFailure{}
	if bare.Error() != factorydefinitions.ErrFactoryDefinitionValidationFailed.Error() {
		t.Fatalf("bare Error: got %q", bare.Error())
	}
}

func TestCloneRuntimeSnapshotDetachesNestedValues(t *testing.T) {
	version := factorydefinitions.FactoryVersion{}
	snapshot := factorydefinitions.RuntimeSnapshot{
		FactoryDir:        "/factories/example",
		RuntimeBaseDir:    "/runtime/example",
		DefinitionVersion: &version,
		EffectiveFactory: factorydefinitions.FactoryConfig{
			Name:         "example",
			Workers:      []factorydefinitions.FactoryWorkerConfig{{Name: "worker-a"}},
			Workstations: []factorydefinitions.FactoryWorkstationConfig{{Name: "workstation-a"}},
		},
		Workers:      []factorydefinitions.FactoryWorkerConfig{{Name: "worker-b"}},
		Workstations: []factorydefinitions.FactoryWorkstationConfig{{Name: "workstation-b"}},
		AutomationSources: []factorydefinitions.RuntimeAutomationSource{{
			ID:          "source-a",
			Workstation: factorydefinitions.FactoryWorkstationConfig{Name: "source-workstation"},
			Worker:      &factorydefinitions.FactoryWorkerConfig{Name: "source-worker"},
		}},
		PromptSources: []factorydefinitions.RuntimePromptSource{{Name: "prompt-a"}},
		BundledFiles:  []factorydefinitions.PortableBundledFileReplacement{{TargetPath: "README.md"}},
	}

	cloned, err := snapshot.Clone()
	if err != nil {
		t.Fatalf("RuntimeSnapshot.Clone() error = %v", err)
	}
	cloned.EffectiveFactory.Workers[0].Name = "mutated-effective-worker"
	cloned.Workers[0].Name = "mutated-runtime-worker"
	cloned.Workstations[0].Name = "mutated-runtime-workstation"
	cloned.AutomationSources[0].Workstation.Name = "mutated-source-workstation"
	cloned.AutomationSources[0].Worker.Name = "mutated-source-worker"
	cloned.PromptSources[0].Name = "mutated-prompt"
	cloned.BundledFiles[0].TargetPath = "mutated.md"
	cloned.DefinitionVersion = nil

	if snapshot.EffectiveFactory.Workers[0].Name != "worker-a" || snapshot.Workers[0].Name != "worker-b" || snapshot.Workstations[0].Name != "workstation-b" {
		t.Fatal("RuntimeSnapshot.Clone() shared worker or workstation values")
	}
	if snapshot.AutomationSources[0].Workstation.Name != "source-workstation" || snapshot.AutomationSources[0].Worker.Name != "source-worker" {
		t.Fatal("RuntimeSnapshot.Clone() shared automation source values")
	}
	if snapshot.PromptSources[0].Name != "prompt-a" || snapshot.BundledFiles[0].TargetPath != "README.md" || snapshot.DefinitionVersion == nil {
		t.Fatal("RuntimeSnapshot.Clone() shared scalar slices or version")
	}
}

func TestRuntimeSnapshotResolutionErrorCarriesTypedCauses(t *testing.T) {
	var nilError *factorydefinitions.RuntimeSnapshotResolutionError
	if nilError.Error() != factorydefinitions.ErrRuntimeSnapshotResolutionFailed.Error() {
		t.Fatalf("nil Error() = %q", nilError.Error())
	}
	if !errors.Is(nilError.Unwrap(), factorydefinitions.ErrRuntimeSnapshotResolutionFailed) {
		t.Fatalf("nil Unwrap() = %v", nilError.Unwrap())
	}

	cause := errors.New("invalid source")
	for _, code := range []factorydefinitions.RuntimeSnapshotDiagnosticCode{
		factorydefinitions.RuntimeSnapshotDiagnosticInvalidRequest,
		factorydefinitions.RuntimeSnapshotDiagnosticInvalidDefinition,
		factorydefinitions.RuntimeSnapshotDiagnosticUnavailable,
		factorydefinitions.RuntimeSnapshotDiagnosticCanceled,
	} {
		err := &factorydefinitions.RuntimeSnapshotResolutionError{
			Diagnostic: factorydefinitions.RuntimeSnapshotDiagnostic{Code: code, Message: "source failed"},
			Cause:      cause,
		}
		if !strings.Contains(err.Error(), "source failed") || !errors.Is(err, cause) || !errors.Is(err, factorydefinitions.ErrRuntimeSnapshotResolutionFailed) {
			t.Fatalf("error for code %q = %v, missing cause or umbrella", code, err)
		}
	}
	if err := (&factorydefinitions.RuntimeSnapshotResolutionError{
		Diagnostic: factorydefinitions.RuntimeSnapshotDiagnostic{Code: factorydefinitions.RuntimeSnapshotDiagnosticInvalidRequest},
	}).Error(); !strings.Contains(err, "invalid-request") {
		t.Fatalf("empty-message error = %q, want diagnostic code", err)
	}

	checks := []struct {
		code   factorydefinitions.RuntimeSnapshotDiagnosticCode
		target error
	}{
		{factorydefinitions.RuntimeSnapshotDiagnosticInvalidRequest, factorydefinitions.ErrInvalidRuntimeSnapshotRequest},
		{factorydefinitions.RuntimeSnapshotDiagnosticInvalidDefinition, factorydefinitions.ErrInvalidRuntimeSnapshotDefinition},
		{factorydefinitions.RuntimeSnapshotDiagnosticUnavailable, factorydefinitions.ErrRuntimeSnapshotResolverUnavailable},
	}
	for _, check := range checks {
		err := &factorydefinitions.RuntimeSnapshotResolutionError{Diagnostic: factorydefinitions.RuntimeSnapshotDiagnostic{Code: check.code}}
		if !errors.Is(err, check.target) {
			t.Fatalf("errors.Is(code=%q, target=%v) = false", check.code, check.target)
		}
	}
}

func (fakeDefinitionsPeer) ResolveRuntimeSnapshot(
	ctx context.Context,
	request factorydefinitions.ResolveRuntimeSnapshotRequest,
) (factorydefinitions.ResolveRuntimeSnapshotResult, error) {
	return factorydefinitions.ResolveRuntimeSnapshotResult{}, &factorydefinitions.RuntimeSnapshotResolutionError{
		Diagnostic: factorydefinitions.RuntimeSnapshotDiagnostic{
			Code:    factorydefinitions.RuntimeSnapshotDiagnosticUnavailable,
			Message: "test snapshot resolver unavailable",
		},
		Cause: factorydefinitions.ErrRuntimeSnapshotResolverUnavailable,
	}
}
