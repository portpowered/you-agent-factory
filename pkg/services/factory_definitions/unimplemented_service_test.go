package factorydefinitions_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func TestUnimplementedService_RootSliceTypedOutcomes(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	var unimplemented factorydefinitions.UnimplementedService

	if _, err := unimplemented.ListNamedFactories(ctx, factorydefinitions.ListNamedFactoriesRequest{}); err == nil {
		t.Fatal("ListNamedFactories: expected collaborator-required error")
	}
	if _, err := unimplemented.GetNamedFactory(ctx, factorydefinitions.GetNamedFactoryRequest{Name: "missing"}); !errors.Is(err, factorydefinitions.ErrNamedFactoryNotFound) {
		t.Fatalf("GetNamedFactory: got %v, want ErrNamedFactoryNotFound", err)
	}
	if _, err := unimplemented.ResolveNamedFactory(ctx, factorydefinitions.ResolveNamedFactoryRequest{Name: "missing"}); !errors.Is(err, factorydefinitions.ErrNamedFactoryNotFound) {
		t.Fatalf("ResolveNamedFactory: got %v, want ErrNamedFactoryNotFound", err)
	}
	if _, err := unimplemented.DeleteNamedFactory(ctx, factorydefinitions.DeleteNamedFactoryRequest{Name: "missing"}); !errors.Is(err, factorydefinitions.ErrNamedFactoryNotFound) {
		t.Fatalf("DeleteNamedFactory: got %v, want ErrNamedFactoryNotFound", err)
	}
	if _, err := unimplemented.GetCurrentFactoryPointer(ctx, factorydefinitions.GetCurrentFactoryPointerRequest{}); !errors.Is(err, factorydefinitions.ErrCurrentFactoryNotFound) {
		t.Fatalf("GetCurrentFactoryPointer: got %v, want ErrCurrentFactoryNotFound", err)
	}
	if _, err := unimplemented.SetCurrentFactoryPointer(ctx, factorydefinitions.SetCurrentFactoryPointerRequest{Name: "alpha"}); !errors.Is(err, factorydefinitions.ErrNamedFactoryNotFound) {
		t.Fatalf("SetCurrentFactoryPointer: got %v, want ErrNamedFactoryNotFound", err)
	}

	if _, err := unimplemented.PrepareFactoryLayout(ctx, factorydefinitions.PrepareFactoryLayoutRequest{}); !errors.Is(err, factorydefinitions.ErrMalformedFactoryLayoutPayload) {
		t.Fatalf("PrepareFactoryLayout: got %v, want ErrMalformedFactoryLayoutPayload", err)
	}
	if _, err := unimplemented.FlattenFactoryLayout(ctx, factorydefinitions.FlattenFactoryLayoutRequest{}); err == nil {
		t.Fatal("FlattenFactoryLayout: expected collaborator-required error")
	}
	if _, err := unimplemented.ExpandFactoryLayout(ctx, factorydefinitions.ExpandFactoryLayoutRequest{}); err == nil {
		t.Fatal("ExpandFactoryLayout: expected collaborator-required error")
	}

	_, createErr := unimplemented.CreateNamedFactory(ctx, factorydefinitions.CreateNamedFactoryRequest{Name: "alpha"})
	assertAtomicWriteFailure(t, createErr, true)
	_, replaceErr := unimplemented.ReplaceNamedFactory(ctx, factorydefinitions.ReplaceNamedFactoryRequest{Name: "alpha"})
	assertAtomicWriteFailure(t, replaceErr, true)

	if _, err := unimplemented.CompileEffectiveFactorySource(ctx, factorydefinitions.CompileEffectiveFactorySourceRequest{}); !errors.Is(err, factorydefinitions.ErrInvalidAuthoredFactorySource) {
		t.Fatalf("CompileEffectiveFactorySource: got %v, want ErrInvalidAuthoredFactorySource", err)
	}
	if _, err := unimplemented.ValidateStructuralFactoryDefinition(ctx, factorydefinitions.ValidateStructuralFactoryDefinitionRequest{}); !errors.Is(err, factorydefinitions.ErrInvalidFactoryDefinitionPayload) {
		t.Fatalf("ValidateStructuralFactoryDefinition: got %v, want ErrInvalidFactoryDefinitionPayload", err)
	}
	if _, err := unimplemented.ValidateEffectiveFactoryDefinition(ctx, factorydefinitions.ValidateEffectiveFactoryDefinitionRequest{}); !errors.Is(err, factorydefinitions.ErrInvalidFactoryDefinitionPayload) {
		t.Fatalf("ValidateEffectiveFactoryDefinition: got %v, want ErrInvalidFactoryDefinitionPayload", err)
	}

	if _, err := unimplemented.CaptureFactorySnapshot(ctx, factorydefinitions.CaptureFactorySnapshotRequest{}); !errors.Is(err, factorydefinitions.ErrInvalidFactorySnapshotPayload) {
		t.Fatalf("CaptureFactorySnapshot: got %v, want ErrInvalidFactorySnapshotPayload", err)
	}
	if _, err := unimplemented.PrepareFactorySnapshotImport(ctx, factorydefinitions.PrepareFactorySnapshotImportRequest{}); !errors.Is(err, factorydefinitions.ErrInvalidFactorySnapshotPayload) {
		t.Fatalf("PrepareFactorySnapshotImport: got %v, want ErrInvalidFactorySnapshotPayload", err)
	}
	if _, err := unimplemented.MaterializeFactorySnapshot(ctx, factorydefinitions.MaterializeFactorySnapshotRequest{}); !errors.Is(err, factorydefinitions.ErrUnsafeFactorySnapshotMaterialize) {
		t.Fatalf("MaterializeFactorySnapshot: got %v, want ErrUnsafeFactorySnapshotMaterialize", err)
	}

	if _, err := unimplemented.ListBuiltInPackagedFactories(ctx, factorydefinitions.ListBuiltInPackagedFactoriesRequest{}); err == nil {
		t.Fatal("ListBuiltInPackagedFactories: expected collaborator-required error")
	}
	if _, err := unimplemented.InstallPackagedFactory(ctx, factorydefinitions.InstallPackagedFactoryRequest{Name: "@you/missing"}); !errors.Is(err, factorydefinitions.ErrUnknownPackagedFactoryIdentity) {
		t.Fatalf("InstallPackagedFactory: got %v, want ErrUnknownPackagedFactoryIdentity", err)
	}
	if _, err := unimplemented.CreateFactoryScaffold(ctx, factorydefinitions.CreateFactoryScaffoldRequest{}); !errors.Is(err, factorydefinitions.ErrFactoryDistributeFailed) {
		t.Fatalf("CreateFactoryScaffold: got %v, want ErrFactoryDistributeFailed", err)
	}
}

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

func assertAtomicWriteFailure(t *testing.T, err error, wantPreserved bool) {
	t.Helper()
	if err == nil {
		t.Fatal("expected AtomicFactoryWriteFailure")
	}
	if !errors.Is(err, factorydefinitions.ErrAtomicFactoryWriteFailed) {
		t.Fatalf("got %v, want ErrAtomicFactoryWriteFailed", err)
	}
	var failure *factorydefinitions.AtomicFactoryWriteFailure
	if !errors.As(err, &failure) {
		t.Fatalf("got %T, want *AtomicFactoryWriteFailure", err)
	}
	if failure.PreviousPreserved != wantPreserved {
		t.Fatalf("PreviousPreserved=%v, want %v", failure.PreviousPreserved, wantPreserved)
	}
	if failure.Error() == "" {
		t.Fatal("expected non-empty Error()")
	}
}
