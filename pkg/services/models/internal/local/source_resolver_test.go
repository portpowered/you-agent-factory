package local

import (
	"testing"

	models "github.com/portpowered/infinite-you/pkg/services/models"
)

func TestDefaultManagedRuntimeSourceResolver_ClassifiesConfiguredSources(t *testing.T) {
	t.Parallel()
	resolver := DefaultManagedRuntimeSourceResolver()

	upstream := resolver.Resolve("OMNIVOICE_Q4_K_M", &modelRuntimeResource{
		Type:  models.RuntimeResourceTypeModel,
		Model: "OMNIVOICE_Q4_K_M",
	})
	if upstream.SourceKind != ManagedRuntimeSourceKindUpstreamRepository {
		t.Fatalf("upstream source kind = %q, want %q", upstream.SourceKind, ManagedRuntimeSourceKindUpstreamRepository)
	}
	if upstream.SourceID != "upstream-repository:OMNIVOICE_Q4_K_M" {
		t.Fatalf("upstream source id = %q, want upstream-repository identity", upstream.SourceID)
	}

	mirror := resolver.Resolve("OMNIVOICE_Q4_K_M", &modelRuntimeResource{
		Type:     models.RuntimeResourceTypeModel,
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
