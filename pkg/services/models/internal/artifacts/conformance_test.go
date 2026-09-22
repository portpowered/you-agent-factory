//go:build backendconformance

package artifacts_test

import (
	"os"
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

func TestDecodeAcceptsApprovedWindowsCUDAVariant(t *testing.T) {
	t.Parallel()

	candidate := approvedWindowsCUDAManifest(t)
	manifest, err := artifacts.Decode(candidate)
	if err != nil {
		t.Fatalf("Decode(candidate) error = %v, want the approved CUDA variant to decode", err)
	}
	if manifest.ArtifactCount() != 10 {
		t.Fatalf("ArtifactCount = %d, want the nine baselines plus one CUDA variant", manifest.ArtifactCount())
	}
	for _, descriptor := range manifest.Artifacts() {
		if descriptor.ID != "localai-llamacpp/windows-amd64-cuda" {
			continue
		}
		if descriptor.Target.OperatingSystem != "windows" || descriptor.Target.Architecture != "amd64" || len(descriptor.Target.Accelerators) != 1 || descriptor.Target.Accelerators[0] != "cuda" {
			t.Fatalf("CUDA descriptor target = %#v, want windows/amd64/cuda", descriptor.Target)
		}
		return
	}
	t.Fatal("decoded manifest omitted the approved CUDA descriptor")
}

func TestSelectFindsApprovedWindowsCUDAArtifactWithoutChangingCPUSelection(t *testing.T) {
	t.Parallel()

	manifest, err := artifacts.Decode(approvedWindowsCUDAManifest(t))
	if err != nil {
		t.Fatalf("Decode(candidate) error = %v", err)
	}
	cuda, err := manifest.Select(artifacts.SelectionRequest{
		Backend: "localai-llamacpp", OperatingSystem: "windows", Architecture: "amd64",
		ProtocolRevision: manifest.ProtocolRevision(), Accelerator: "cuda",
	})
	if err != nil {
		t.Fatalf("Select(cuda) error = %v", err)
	}
	if cuda.ID != "localai-llamacpp/windows-amd64-cuda" {
		t.Fatalf("Select(cuda) ID = %q, want approved CUDA variant", cuda.ID)
	}
	cpu, err := manifest.Select(artifacts.SelectionRequest{
		Backend: "localai-llamacpp", OperatingSystem: "windows", Architecture: "amd64",
		ProtocolRevision: manifest.ProtocolRevision(), Accelerator: "cpu",
	})
	if err != nil {
		t.Fatalf("Select(cpu) error = %v", err)
	}
	if cpu.ID != "localai-llamacpp/windows-amd64" {
		t.Fatalf("Select(cpu) ID = %q, want existing Windows CPU baseline", cpu.ID)
	}
}

func approvedWindowsCUDAManifest(t *testing.T) []byte {
	t.Helper()
	manifestBytes, err := os.ReadFile("testdata/windows-cuda-variant-manifest.json")
	if err != nil {
		t.Fatalf("read approved Windows CUDA fixture: %v", err)
	}
	return manifestBytes
}
