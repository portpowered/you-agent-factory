//go:build backendconformance

package artifacts_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"strings"
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

func TestDecodeCharacterizesApprovedWindowsCUDAVariantClosedMatrixRejection(t *testing.T) {
	t.Parallel()

	candidate := approvedWindowsCUDAManifest(t)
	before := append([]byte(nil), candidate...)
	_, err := artifacts.Decode(candidate)
	if !errors.Is(err, artifacts.ErrUnsupportedPlatform) {
		t.Fatalf("Decode(candidate) error = %v, want closed target matrix rejection", err)
	}
	var failure *artifacts.Failure
	if !errors.As(err, &failure) || failure.Kind != artifacts.FailureUnsupportedPlatform {
		t.Fatalf("Decode(candidate) error = %#v, want typed unsupported target failure", err)
	}
	if failure.Field != "artifacts[9].target" || failure.Value != "windows-amd64-cuda" {
		t.Fatalf("target failure = %#v, want the approved CUDA target at artifacts[9]", failure)
	}
	if !strings.Contains(failure.Detail, "outside the supported") {
		t.Fatalf("target failure detail = %q, want the current closed target matrix reason", failure.Detail)
	}
	if !bytes.Equal(candidate, before) {
		t.Fatal("Decode mutated the candidate manifest after rejecting the CUDA variant")
	}
}

func approvedWindowsCUDAManifest(t *testing.T) []byte {
	t.Helper()

	manifestBytes, err := os.ReadFile("default-manifest.json")
	if err != nil {
		t.Fatalf("read checked-in default manifest: %v", err)
	}
	var document map[string]any
	if err := json.Unmarshal(manifestBytes, &document); err != nil {
		t.Fatalf("decode checked-in default manifest: %v", err)
	}
	entries := document["artifacts"].([]any)
	for _, value := range entries {
		entry := value.(map[string]any)
		backend := entry["backend"].(map[string]any)
		target := entry["target"].(map[string]any)
		if backend["id"] != "localai-llamacpp" || target["id"] != "windows-amd64" {
			continue
		}

		encoded, err := json.Marshal(entry)
		if err != nil {
			t.Fatalf("copy Windows CPU artifact fixture: %v", err)
		}
		var candidate map[string]any
		if err := json.Unmarshal(encoded, &candidate); err != nil {
			t.Fatalf("decode Windows CPU artifact fixture copy: %v", err)
		}
		candidate["id"] = "localai-llamacpp/windows-amd64-cuda"
		candidateTarget := candidate["target"].(map[string]any)
		candidateTarget["id"] = "windows-amd64-cuda"
		candidateTarget["accelerators"] = []any{"cuda"}
		archive := candidate["artifact"].(map[string]any)
		oldName := archive["name"].(string)
		newName := strings.Replace(oldName, "windows-amd64", "windows-amd64-cuda", 1)
		archive["name"] = newName
		archive["location"] = strings.Replace(
			archive["location"].(string), oldName, newName, 1,
		)
		archive["sha256"] = strings.Repeat("d", 64)
		document["artifacts"] = append(entries, candidate)
		result, err := json.Marshal(document)
		if err != nil {
			t.Fatalf("encode approved Windows CUDA fixture: %v", err)
		}
		return result
	}
	t.Fatal("checked-in manifest has no localai-llamacpp/windows-amd64 baseline to copy")
	return nil
}
