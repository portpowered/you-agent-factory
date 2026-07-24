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

func TestValidateCatalogRejectsMissingRequiredPacket(t *testing.T) {
	t.Parallel()

	manifest := loadFixture(t, "invalid-missing-cataloged-packet.json")
	err := psslease.ValidateCatalog(manifest)
	if err == nil {
		t.Fatal("ValidateCatalog() error = nil, want missing required packet failure")
	}
	if !strings.Contains(err.Error(), `missing cataloged packet "FND-01"`) {
		t.Fatalf("ValidateCatalog() error = %v, want missing FND-01", err)
	}
}

func TestValidateCatalogRejectsMissingState(t *testing.T) {
	t.Parallel()

	manifest := loadFixture(t, "invalid-missing-packet-state.json")
	err := psslease.ValidateCatalog(manifest)
	if err == nil {
		t.Fatal("ValidateCatalog() error = nil, want missing state failure")
	}
	if !strings.Contains(err.Error(), "missing packet state") && !strings.Contains(err.Error(), "unknown packet state") {
		t.Fatalf("ValidateCatalog() error = %v, want missing/unknown state", err)
	}
}

func TestValidateCatalogRejectsEmptyExclusivePaths(t *testing.T) {
	t.Parallel()

	manifest := loadFixture(t, "invalid-catalog-empty-exclusive-paths.json")
	err := psslease.ValidateCatalog(manifest)
	if err == nil {
		t.Fatal("ValidateCatalog() error = nil, want empty exclusivePaths failure")
	}
	if !strings.Contains(err.Error(), "empty exclusivePaths") {
		t.Fatalf("ValidateCatalog() error = %v, want empty exclusivePaths", err)
	}
}

func TestValidateCatalogRejectsDuplicatePacketIDs(t *testing.T) {
	t.Parallel()

	manifest := loadFixture(t, "invalid-duplicate-packet-ids.json")
	err := psslease.ValidateCatalog(manifest)
	if err == nil {
		t.Fatal("ValidateCatalog() error = nil, want duplicate packetId failure")
	}
	if !strings.Contains(err.Error(), `duplicate packetId "FND-10"`) {
		t.Fatalf("ValidateCatalog() error = %v, want duplicate FND-10", err)
	}
}

func TestCommittedProgramMetadataManifestPassesCatalogValidation(t *testing.T) {
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
	if err := psslease.ValidateCatalog(manifest); err != nil {
		t.Fatalf("ValidateCatalog() error = %v, want nil", err)
	}

	seen := make(map[string]psslease.Packet, len(manifest.Packets))
	for _, packet := range manifest.Packets {
		seen[packet.PacketID] = packet
		if psslease.IsLeaseHoldingState(packet.State) {
			t.Fatalf("catalog packet %q state = %q; undispatched catalog defaults must not silently use lease-holding states", packet.PacketID, packet.State)
		}
	}
	for _, packetID := range psslease.RequiredCatalogPacketIDs {
		packet, ok := seen[packetID]
		if !ok {
			t.Fatalf("committed manifest missing required packet %q", packetID)
		}
		if len(packet.ExclusivePaths) == 0 {
			t.Fatalf("packet %q has empty exclusivePaths", packetID)
		}
		if strings.TrimSpace(packet.State) == "" {
			t.Fatalf("packet %q missing state", packetID)
		}
	}
}

func TestValidateLeaseHoldersRejectsOverlappingActiveLeases(t *testing.T) {
	t.Parallel()

	manifest := loadFixture(t, "invalid-overlapping-active-leases.json")
	err := psslease.ValidateLeaseHolders(manifest)
	if err == nil {
		t.Fatal("ValidateLeaseHolders() error = nil, want overlapping lease conflict")
	}
	message := err.Error()
	for _, want := range []string{"PKT-A", "PKT-B", "pkg/services/factory_runtime/"} {
		if !strings.Contains(message, want) {
			t.Fatalf("ValidateLeaseHolders() error = %v, want substring %q", err, want)
		}
	}
}

func TestValidateLeaseHoldersAcceptsDisjointActiveLeases(t *testing.T) {
	t.Parallel()

	manifest := loadFixture(t, "valid-disjoint-active-leases.json")
	if err := psslease.ValidateLeaseHolders(manifest); err != nil {
		t.Fatalf("ValidateLeaseHolders() error = %v, want nil for disjoint holders", err)
	}
}

func TestValidateLeaseHoldersAllowsNonHoldingPathOverlap(t *testing.T) {
	t.Parallel()

	manifest := loadFixture(t, "valid-blocked-ready-path-overlap.json")
	if err := psslease.ValidateLeaseHolders(manifest); err != nil {
		t.Fatalf("ValidateLeaseHolders() error = %v, want nil while overlap is only among non-holders", err)
	}
}

func TestValidateDispatchCandidateRejectsOverlapWithExistingHolder(t *testing.T) {
	t.Parallel()

	manifest := loadFixture(t, "valid-blocked-ready-path-overlap.json")
	if err := psslease.ValidateLeaseHolders(manifest); err != nil {
		t.Fatalf("precondition ValidateLeaseHolders() error = %v", err)
	}

	// Promote PKT-A to active first so it holds the overlapping prefix.
	for i := range manifest.Packets {
		if manifest.Packets[i].PacketID == "PKT-A" {
			manifest.Packets[i].State = psslease.StateActive
		}
	}
	if err := psslease.ValidateLeaseHolders(manifest); err != nil {
		t.Fatalf("ValidateLeaseHolders() after promoting PKT-A error = %v, want nil", err)
	}

	err := psslease.ValidateDispatchCandidate(manifest, "PKT-B", psslease.StateActive)
	if err == nil {
		t.Fatal("ValidateDispatchCandidate() error = nil, want rejection before PKT-B becomes active")
	}
	message := err.Error()
	for _, want := range []string{"PKT-A", "PKT-B", "docs/internal/projects/packaged-service-structure/"} {
		if !strings.Contains(message, want) {
			t.Fatalf("ValidateDispatchCandidate() error = %v, want substring %q", err, want)
		}
	}
}

func TestValidateDispatchCandidateAllowsDisjointActivation(t *testing.T) {
	t.Parallel()

	manifest := loadFixture(t, "valid-disjoint-active-leases.json")
	// Reset PKT-B to ready so we can ask whether it may become active.
	for i := range manifest.Packets {
		if manifest.Packets[i].PacketID == "PKT-B" {
			manifest.Packets[i].State = psslease.StateReady
		}
	}

	if err := psslease.ValidateDispatchCandidate(manifest, "PKT-B", psslease.StateActive); err != nil {
		t.Fatalf("ValidateDispatchCandidate() error = %v, want nil for disjoint activation", err)
	}
}

func TestCommittedProgramMetadataManifestPassesLeaseHolderValidation(t *testing.T) {
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
	if err := psslease.ValidateLeaseHolders(manifest); err != nil {
		t.Fatalf("ValidateLeaseHolders() error = %v, want nil", err)
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
