package logicaltarget_test

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/logicaltarget"
)

func TestDeriveLogicalSessionKeyID_StableForSameCanonicalReference(t *testing.T) {
	ref, err := logicaltarget.NormalizeDefaultTarget(testBackendScopeID, t.TempDir())
	if err != nil {
		t.Fatalf("NormalizeDefaultTarget: %v", err)
	}

	first := logicaltarget.DeriveLogicalSessionKeyID(ref)
	second := logicaltarget.DeriveLogicalSessionKeyID(ref)
	if first == "" || first != second {
		t.Fatalf("logicalSessionKeyID = %q and %q, want identical stable value", first, second)
	}
	if !logicaltarget.IsLogicalSessionKeyID(first) {
		t.Fatalf("logicalSessionKeyID = %q, want opaque lsk- format", first)
	}
}

func TestDeriveLogicalSessionKeyID_DistinctBoundariesDoNotCollide(t *testing.T) {
	folder := t.TempDir()
	otherFolder := t.TempDir()

	defaultRef, err := logicaltarget.NormalizeDefaultTarget(testBackendScopeID, folder)
	if err != nil {
		t.Fatalf("NormalizeDefaultTarget: %v", err)
	}
	otherScopeRef, err := logicaltarget.NormalizeDefaultTarget("other-scope", folder)
	if err != nil {
		t.Fatalf("NormalizeDefaultTarget(other scope): %v", err)
	}
	otherFolderRef, err := logicaltarget.NormalizeDefaultTarget(testBackendScopeID, otherFolder)
	if err != nil {
		t.Fatalf("NormalizeDefaultTarget(other folder): %v", err)
	}
	namedRef, err := logicaltarget.NormalizeNamedTarget(testBackendScopeID, folder, "beta")
	if err != nil {
		t.Fatalf("NormalizeNamedTarget: %v", err)
	}
	providerRef, err := logicaltarget.NormalizeProviderTarget(
		testBackendScopeID,
		folder,
		logicaltarget.ProviderBoundary{
			Provider: "cursor",
			Kind:     "session_id",
			Boundary: "workspace-1",
		},
	)
	if err != nil {
		t.Fatalf("NormalizeProviderTarget: %v", err)
	}

	keys := map[string]logicaltarget.CanonicalReference{
		logicaltarget.DeriveLogicalSessionKeyID(defaultRef):     defaultRef,
		logicaltarget.DeriveLogicalSessionKeyID(otherScopeRef):  otherScopeRef,
		logicaltarget.DeriveLogicalSessionKeyID(otherFolderRef): otherFolderRef,
		logicaltarget.DeriveLogicalSessionKeyID(namedRef):       namedRef,
		logicaltarget.DeriveLogicalSessionKeyID(providerRef):    providerRef,
	}
	if len(keys) != 5 {
		t.Fatalf("derived %d unique keys, want 5 distinct boundaries", len(keys))
	}
}

func TestDeriveLogicalSessionKeyID_EquivalentNormalizedTargetsMatch(t *testing.T) {
	folder := t.TempDir()

	absolute, err := logicaltarget.NormalizeDefaultTarget(testBackendScopeID, folder)
	if err != nil {
		t.Fatalf("NormalizeDefaultTarget: %v", err)
	}
	trimmedScope, err := logicaltarget.NormalizeDefaultTarget("  "+testBackendScopeID+"  ", folder)
	if err != nil {
		t.Fatalf("NormalizeDefaultTarget(trimmed scope): %v", err)
	}

	if logicaltarget.DeriveLogicalSessionKeyID(absolute) != logicaltarget.DeriveLogicalSessionKeyID(trimmedScope) {
		t.Fatalf(
			"equivalent default targets produced different keys: %q vs %q",
			logicaltarget.DeriveLogicalSessionKeyID(absolute),
			logicaltarget.DeriveLogicalSessionKeyID(trimmedScope),
		)
	}
}

func TestDeriveLogicalSessionKeyID_DoesNotDependOnFactorySessionAllocation(t *testing.T) {
	folder := t.TempDir()
	ref, err := logicaltarget.NormalizeNamedTarget(testBackendScopeID, folder, "beta")
	if err != nil {
		t.Fatalf("NormalizeNamedTarget: %v", err)
	}

	firstAllocation := logicaltarget.DeriveLogicalSessionKeyID(ref)
	secondAllocation := logicaltarget.DeriveLogicalSessionKeyID(ref)
	if firstAllocation != secondAllocation {
		t.Fatalf("key changed across allocations: %q vs %q", firstAllocation, secondAllocation)
	}
}

func TestIsLogicalSessionKeyID_RejectsNonOpaqueValues(t *testing.T) {
	t.Parallel()

	if logicaltarget.IsLogicalSessionKeyID("") {
		t.Fatal("IsLogicalSessionKeyID(\"\") = true, want false")
	}
	if logicaltarget.IsLogicalSessionKeyID("folder::default::") {
		t.Fatal("IsLogicalSessionKeyID(legacy join) = true, want false")
	}
}
