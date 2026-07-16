package local

import (
	"testing"

	factoryresource "github.com/portpowered/infinite-you/pkg/factory/resource"
)

func TestDefaultManagedRuntimeSourceResolver_ClassifiesConfiguredSources(t *testing.T) {
	resolver := DefaultManagedRuntimeSourceResolver()

	upstream := resolver.Resolve("OMNIVOICE_Q4_K_M", &factoryresource.Config{
		Type:  factoryresource.TypeModel,
		Model: "OMNIVOICE_Q4_K_M",
	})
	if upstream.SourceKind != ManagedRuntimeSourceKindUpstreamRepository {
		t.Fatalf("upstream source kind = %q, want %q", upstream.SourceKind, ManagedRuntimeSourceKindUpstreamRepository)
	}
	if upstream.SourceID != "upstream-repository:OMNIVOICE_Q4_K_M" {
		t.Fatalf("upstream source id = %q, want upstream-repository identity", upstream.SourceID)
	}

	mirror := resolver.Resolve("OMNIVOICE_Q4_K_M", &factoryresource.Config{
		Type:     factoryresource.TypeModel,
		Model:    "OMNIVOICE_Q4_K_M",
		Provider: "MODELSCOPE",
	})
	if mirror.SourceKind != ManagedRuntimeSourceKindManagedMirror {
		t.Fatalf("mirror source kind = %q, want %q", mirror.SourceKind, ManagedRuntimeSourceKindManagedMirror)
	}
	if mirror.SourceID != "managed-mirror:OMNIVOICE_Q4_K_M" {
		t.Fatalf("mirror source id = %q, want managed-mirror identity", mirror.SourceID)
	}
}
