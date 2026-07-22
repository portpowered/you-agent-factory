package testutil

import (
	"encoding/json"
	"os"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

// LoadReplayArtifact decodes the customer-visible artifact written at the
// recording edge. Replay validation and historical compatibility policy remain
// covered by Recordings owner tests rather than being invoked by functional
// test support.
func LoadReplayArtifact(t testing.TB, artifactPath string) *interfaces.ReplayArtifact {
	t.Helper()

	data, err := os.ReadFile(artifactPath)
	if err != nil {
		t.Fatalf("LoadReplayArtifact: read %q: %v", artifactPath, err)
	}
	var artifact interfaces.ReplayArtifact
	if err := json.Unmarshal(data, &artifact); err != nil {
		t.Fatalf("LoadReplayArtifact: decode %q: %v", artifactPath, err)
	}
	return &artifact
}
