package logicaltarget

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/factorysessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestAPILogicalTarget_DefaultNamedAndProvider(t *testing.T) {
	t.Parallel()

	defaultRef, err := NormalizeDefaultTarget("scope-1", t.TempDir())
	if err != nil {
		t.Fatalf("NormalizeDefaultTarget: %v", err)
	}
	defaultTarget := APILogicalTarget(defaultRef)
	if defaultTarget.Kind != factoryapi.FactorySessionLogicalTargetKindDefault {
		t.Fatalf("default kind = %q", defaultTarget.Kind)
	}
	if defaultTarget.NamedTarget != nil || defaultTarget.ProviderBoundary != nil {
		t.Fatalf("default target should not include named or provider fields: %#v", defaultTarget)
	}

	namedRef, err := NormalizeNamedTarget("scope-1", t.TempDir(), "goal")
	if err != nil {
		t.Fatalf("NormalizeNamedTarget: %v", err)
	}
	namedTarget := APILogicalTarget(namedRef)
	if namedTarget.Kind != factoryapi.FactorySessionLogicalTargetKindNamed {
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
	providerTarget := APILogicalTarget(providerRef)
	if providerTarget.Kind != factoryapi.FactorySessionLogicalTargetKindProvider {
		t.Fatalf("provider kind = %q", providerTarget.Kind)
	}
	if providerTarget.ProviderBoundary == nil {
		t.Fatal("providerBoundary is nil")
	}
	if providerTarget.ProviderBoundary.Boundary != "workspace-1" {
		t.Fatalf("provider boundary = %q", providerTarget.ProviderBoundary.Boundary)
	}
}

func TestAPILogicalTargetFromSession(t *testing.T) {
	t.Parallel()

	session := &factorysessions.LiveSession{
		SessionState: factorysessions.SessionState{
			FolderPath: t.TempDir(),
		},
		Target: factorysessions.TargetRef{
			Kind: factorysessions.TargetKindNamed,
			Name: "goal",
		},
	}
	target, err := APILogicalTargetFromSession("scope-1", session)
	if err != nil {
		t.Fatalf("APILogicalTargetFromSession: %v", err)
	}
	if target == nil {
		t.Fatal("target is nil")
	}
	if target.Kind != factoryapi.FactorySessionLogicalTargetKindNamed {
		t.Fatalf("kind = %q", target.Kind)
	}
}

func TestAPILogicalTargetFromSession_NilSessionAndInvalidTarget(t *testing.T) {
	t.Parallel()

	target, err := APILogicalTargetFromSession("scope-1", nil)
	if err != nil || target != nil {
		t.Fatalf("APILogicalTargetFromSession(nil) = (%#v, %v), want nil,nil", target, err)
	}

	invalidSession := &factorysessions.LiveSession{
		SessionState: factorysessions.SessionState{FolderPath: t.TempDir()},
		Target: factorysessions.TargetRef{
			Kind: factorysessions.TargetKindNamed,
			Name: "",
		},
	}
	if _, err := APILogicalTargetFromSession("scope-1", invalidSession); err == nil {
		t.Fatal("APILogicalTargetFromSession(invalid named target) = nil, want validation error")
	}
}
