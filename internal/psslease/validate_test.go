package psslease_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/psslease"
)

func TestValidateManifestRejectsContractViolations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		fixture string
		want    string
	}{
		{
			name:    "unknown state",
			fixture: "invalid-unknown-state.json",
			want:    `unknown packet state "running"`,
		},
		{
			name:    "missing packet id",
			fixture: "invalid-missing-packet-id.json",
			want:    "missing packetId",
		},
		{
			name:    "empty exclusive paths",
			fixture: "invalid-empty-exclusive-paths.json",
			want:    "empty exclusivePaths",
		},
		{
			name:    "duplicate packet ids",
			fixture: "invalid-duplicate-packet-ids.json",
			want:    `duplicate packetId "FND-10"`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			manifest := loadFixture(t, test.fixture)
			err := psslease.ValidateManifest(manifest)
			if err == nil {
				t.Fatal("ValidateManifest() error = nil, want failure")
			}
			if !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ValidateManifest() error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func TestValidateManifestAcceptsContractExample(t *testing.T) {
	t.Parallel()

	manifest := loadFixture(t, "valid-contract-example.json")
	if err := psslease.ValidateManifest(manifest); err != nil {
		t.Fatalf("ValidateManifest() error = %v, want nil", err)
	}

	if got := manifest.Packets[0].State; got != psslease.StateReady {
		t.Fatalf("packet state = %q, want %q", got, psslease.StateReady)
	}
	if !psslease.IsLeaseHoldingState(psslease.StateActive) {
		t.Fatal("expected active to be lease-holding")
	}
	if !psslease.IsLeaseHoldingState(psslease.StateReview) {
		t.Fatal("expected review to be lease-holding")
	}
	if !psslease.IsLeaseHoldingState(psslease.StateIntegration) {
		t.Fatal("expected integration to be lease-holding")
	}
	for _, state := range []string{psslease.StateBlocked, psslease.StateReady, psslease.StateDone} {
		if psslease.IsLeaseHoldingState(state) {
			t.Fatalf("expected %q not to be lease-holding", state)
		}
	}
}

func TestCommittedProgramMetadataManifestPassesContractValidation(t *testing.T) {
	t.Parallel()

	path := filepath.Join("..", "..", "docs", "internal", "projects", "packaged-service-structure", "path-lease-packet-manifest.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read committed manifest: %v", err)
	}
	manifest, err := psslease.DecodeManifest(data)
	if err != nil {
		t.Fatalf("DecodeManifest() error = %v", err)
	}
	if err := psslease.ValidateManifest(manifest); err != nil {
		t.Fatalf("ValidateManifest() error = %v, want nil", err)
	}
}

func loadFixture(t *testing.T, name string) *psslease.Manifest {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	manifest, err := psslease.DecodeManifest(data)
	if err != nil {
		t.Fatalf("DecodeManifest(%s) error = %v", name, err)
	}
	return manifest
}
