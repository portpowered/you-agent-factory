//go:build functionallong

package backendconformance

import (
	"context"
	"net/http"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/models/internal/artifacts"
)

// TestPublishedBackendArtifactLocations is intentionally isolated from the
// ordinary PR guard. It becomes meaningful after the immutable backend release
// described by the checked-in manifest has been published.
func TestPublishedBackendArtifactLocations(t *testing.T) {
	manifest, err := artifacts.DefaultManifest()
	if err != nil {
		t.Fatalf("decode default backend manifest: %v", err)
	}

	descriptors := manifest.Artifacts()
	entries := make([]PublishedArtifact, 0, len(descriptors))
	for _, descriptor := range descriptors {
		entries = append(entries, PublishedArtifact{
			BackendID: descriptor.Backend.ID,
			TargetID:  descriptor.Target.ID,
			Location:  descriptor.Artifact.Location,
			SizeBytes: descriptor.Artifact.SizeBytes,
			SHA256:    descriptor.Artifact.SHA256,
		})
	}

	client := &http.Client{Timeout: PublishedArtifactRequestTimeout}
	if err := VerifyPublishedArtifactLocations(context.Background(), client, entries); err != nil {
		t.Fatal(err)
	}
}
