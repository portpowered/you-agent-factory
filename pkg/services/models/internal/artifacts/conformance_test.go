//go:build backendconformance

package artifacts_test

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/models/internal/artifacts"
)

func TestArtifactsReturnsDetachedDescriptorsInManifestOrder(t *testing.T) {
	t.Parallel()

	manifest, err := artifacts.DefaultManifest()
	if err != nil {
		t.Fatalf("DefaultManifest: %v", err)
	}
	first := manifest.Artifacts()
	second := manifest.Artifacts()
	if len(first) != manifest.ArtifactCount() || len(second) != len(first) {
		t.Fatalf("Artifacts lengths = (%d, %d), want %d", len(first), len(second), manifest.ArtifactCount())
	}
	if len(first) == 0 || first[0].Backend.ID != "localai-llamacpp" || first[0].Target.ID != "darwin-arm64" {
		t.Fatalf("first artifact = %#v, want localai-llamacpp/darwin-arm64", first[0])
	}
	first[0].Target.Accelerators[0] = "mutated"
	if second[0].Target.Accelerators[0] == "mutated" {
		t.Fatal("mutating one Artifacts result changed a later result")
	}
}
