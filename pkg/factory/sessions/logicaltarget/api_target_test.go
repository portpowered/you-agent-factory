package logicaltarget

import (
	"testing"
)

func TestRuntimeLogicalTarget_DefaultNamedAndProvider(t *testing.T) {
	t.Parallel()

	defaultRef, err := NormalizeDefaultTarget("scope-1", t.TempDir())
	if err != nil {
		t.Fatalf("NormalizeDefaultTarget: %v", err)
	}
	defaultTarget := RuntimeLogicalTarget(defaultRef)
	if defaultTarget.Kind != string(KindDefault) {
		t.Fatalf("default kind = %q", defaultTarget.Kind)
	}
	if defaultTarget.NamedTarget != nil || defaultTarget.ProviderBoundary != nil {
		t.Fatalf("default target should not include named or provider fields: %#v", defaultTarget)
	}

	namedRef, err := NormalizeNamedTarget("scope-1", t.TempDir(), "goal")
	if err != nil {
		t.Fatalf("NormalizeNamedTarget: %v", err)
	}
	namedTarget := RuntimeLogicalTarget(namedRef)
	if namedTarget.Kind != string(KindNamed) {
		t.Fatalf("named kind = %q", namedTarget.Kind)
	}
	if namedTarget.NamedTarget == nil || *namedTarget.NamedTarget != namedRef.NamedTarget {
		t.Fatalf("namedTarget = %#v, want %q", namedTarget.NamedTarget, namedRef.NamedTarget)
	}

	providerRef, err := NormalizeProviderTarget("scope-1", t.TempDir(), ProviderBoundary{
		Provider: "cursor",
		Kind:     "agent",
		Boundary: "workspace-1",
	})
	if err != nil {
		t.Fatalf("NormalizeProviderTarget: %v", err)
	}
	providerTarget := RuntimeLogicalTarget(providerRef)
	if providerTarget.Kind != string(KindProvider) {
		t.Fatalf("provider kind = %q", providerTarget.Kind)
	}
	if providerTarget.ProviderBoundary == nil {
		t.Fatal("providerBoundary is nil")
	}
	if providerTarget.ProviderBoundary.Boundary != "workspace-1" {
		t.Fatalf("provider boundary = %q", providerTarget.ProviderBoundary.Boundary)
	}
}
