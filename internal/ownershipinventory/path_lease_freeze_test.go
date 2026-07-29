package ownershipinventory_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/ownershipinventory"
	"github.com/portpowered/infinite-you/internal/psslease"
)

func TestPathLeaseFreezeArtifactExists(t *testing.T) {
	root := repositoryRoot(t)
	path := filepath.Join(root, ownershipinventory.PathLeaseFreezeRelativePath)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("path-lease freeze artifact missing at %s: %v", ownershipinventory.PathLeaseFreezeRelativePath, err)
	}
}

func TestFrozenPathLeaseFreezeMatchesBuild(t *testing.T) {
	root := repositoryRoot(t)
	want := ownershipinventory.BuildPathLeaseFreeze()
	got, err := ownershipinventory.LoadPathLeaseFreeze(root)
	if err != nil {
		t.Fatalf("LoadPathLeaseFreeze() error = %v", err)
	}
	if got.FreezeID != want.FreezeID {
		t.Fatalf("freezeId = %q, want %q", got.FreezeID, want.FreezeID)
	}
	if got.SourceInventory != want.SourceInventory {
		t.Fatalf("sourceInventory = %q, want %q", got.SourceInventory, want.SourceInventory)
	}
	if got.FormatVersion != psslease.FormatVersion {
		t.Fatalf("formatVersion = %q, want FND-10 %q", got.FormatVersion, psslease.FormatVersion)
	}
	if !slices.Equal(packetIDs(got), packetIDs(want)) {
		t.Fatalf("packet ids = %#v, want %#v", packetIDs(got), packetIDs(want))
	}
	report := ownershipinventory.ValidatePathLeaseFreeze(got)
	if !report.OK() {
		t.Fatalf("ValidatePathLeaseFreeze() failed on frozen artifact: %#v", report)
	}
}

func TestValidatePathLeaseFreezeRequiresCatalogedPackets(t *testing.T) {
	freeze := ownershipinventory.BuildPathLeaseFreeze()
	freeze.Packets = freeze.Packets[:1]

	report := ownershipinventory.ValidatePathLeaseFreeze(freeze)
	if report.OK() {
		t.Fatal("ValidatePathLeaseFreeze() unexpectedly passed without PSS-F02")
	}
	if !slices.Contains(report.MissingPackets, "PSS-F02") {
		t.Fatalf("missing packets = %#v, want PSS-F02", report.MissingPackets)
	}
}

func TestValidatePathLeaseFreezeFailsWhenExclusivePathsEmpty(t *testing.T) {
	freeze := ownershipinventory.BuildPathLeaseFreeze()
	for i := range freeze.Packets {
		if freeze.Packets[i].PacketID == "PSS-F01" {
			freeze.Packets[i].ExclusivePaths = nil
		}
	}

	report := ownershipinventory.ValidatePathLeaseFreeze(freeze)
	if report.OK() {
		t.Fatal("ValidatePathLeaseFreeze() unexpectedly passed with empty exclusivePaths")
	}
	if len(report.EmptyExclusivePathPackets) == 0 {
		t.Fatalf("empty exclusive path packets empty; report=%#v", report)
	}
	if !slices.Contains(report.EmptyExclusivePathPackets, "PSS-F01") {
		t.Fatalf("empty exclusive path packets = %#v, want PSS-F01", report.EmptyExclusivePathPackets)
	}
}

func TestValidatePathLeaseFreezeFailsOnOverlappingActiveLeases(t *testing.T) {
	freeze := ownershipinventory.BuildPathLeaseFreeze()
	for i := range freeze.Packets {
		switch freeze.Packets[i].PacketID {
		case "PSS-F01":
			freeze.Packets[i].State = psslease.StateActive
			freeze.Packets[i].ExclusivePaths = []string{"docs/internal/projects/packaged-service-structure/"}
		case "PSS-F02":
			freeze.Packets[i].State = psslease.StateActive
			freeze.Packets[i].ExclusivePaths = []string{
				"docs/internal/projects/packaged-service-structure/owner-boundary-enforcement.md",
			}
		}
	}

	report := ownershipinventory.ValidatePathLeaseFreeze(freeze)
	if report.OK() {
		t.Fatal("ValidatePathLeaseFreeze() unexpectedly passed with overlapping active leases")
	}
	if !report.OverlappingActiveLeases {
		t.Fatalf("expected overlapping active leases; report=%#v", report)
	}
	if report.OverlapDetail == "" {
		t.Fatal("expected overlap detail naming colliding packets/paths")
	}
}

func TestValidatePathLeaseFreezeRejectsPortfolioHoldClaims(t *testing.T) {
	freeze := ownershipinventory.BuildPathLeaseFreeze()
	for i := range freeze.Packets {
		if freeze.Packets[i].PacketID == "PSS-F01" {
			freeze.Packets[i].ExclusivePaths = append(
				freeze.Packets[i].ExclusivePaths,
				"cmd/climanifestgen/",
			)
		}
	}

	report := ownershipinventory.ValidatePathLeaseFreeze(freeze)
	if report.OK() {
		t.Fatal("ValidatePathLeaseFreeze() unexpectedly passed while claiming CLI-manifest portfolio hold")
	}
	if len(report.PortfolioHoldConflicts) == 0 {
		t.Fatalf("portfolio hold conflicts empty; report=%#v", report)
	}
	joined := strings.Join(report.PortfolioHoldConflicts, "\n")
	if !strings.Contains(joined, "cli-manifest") || !strings.Contains(joined, "cmd/climanifestgen/") {
		t.Fatalf("portfolio conflicts = %#v, want cli-manifest + cmd/climanifestgen/", report.PortfolioHoldConflicts)
	}
}

func TestValidatePathLeaseFreezeRejectsProviderConductorClaims(t *testing.T) {
	freeze := ownershipinventory.BuildPathLeaseFreeze()
	for i := range freeze.Packets {
		if freeze.Packets[i].PacketID == "PSS-F02" {
			freeze.Packets[i].ExclusivePaths = append(
				freeze.Packets[i].ExclusivePaths,
				"pkg/services/providers/internal/services/execution/internal/provider/",
			)
		}
	}

	report := ownershipinventory.ValidatePathLeaseFreeze(freeze)
	if report.OK() {
		t.Fatal("ValidatePathLeaseFreeze() unexpectedly passed while claiming provider-conductor portfolio hold")
	}
	joined := strings.Join(report.PortfolioHoldConflicts, "\n")
	if !strings.Contains(joined, "provider-conductor") {
		t.Fatalf("portfolio conflicts = %#v, want provider-conductor", report.PortfolioHoldConflicts)
	}
}

func TestPathLeaseFreezeUnblocksPSSF02WithoutOverlappingF01(t *testing.T) {
	freeze := ownershipinventory.BuildPathLeaseFreeze()
	f01 := packetByID(t, freeze, "PSS-F01")
	f02 := packetByID(t, freeze, "PSS-F02")

	if f02.State != psslease.StateReady {
		t.Fatalf("PSS-F02 state = %q, want ready so the freeze unblocks the first checker slice", f02.State)
	}
	if f01.State != psslease.StateActive {
		t.Fatalf("PSS-F01 state = %q, want active for the ownership-inventory freeze lease", f01.State)
	}
	if !slices.Contains(f02.Prerequisites, "PSS-F01") {
		t.Fatalf("PSS-F02 prerequisites = %#v, want PSS-F01", f02.Prerequisites)
	}

	report := ownershipinventory.ValidatePathLeaseFreeze(freeze)
	if !report.OK() {
		t.Fatalf("ValidatePathLeaseFreeze() failed: %#v", report)
	}

	hasInventoryPath := false
	hasFreezePath := false
	for _, path := range f01.ExclusivePaths {
		if path == ownershipinventory.InventoryRelativePath {
			hasInventoryPath = true
		}
		if path == ownershipinventory.PathLeaseFreezeRelativePath {
			hasFreezePath = true
		}
		if strings.Contains(path, "cmd/climanifestgen") ||
			strings.Contains(path, "contracts/cli") ||
			strings.Contains(path, "cmd/factory/") ||
			strings.Contains(path, "pkg/services/providers/internal/services/execution/internal/provider") {
			t.Fatalf("PSS-F01 exclusive path %q claims a live portfolio hold", path)
		}
	}
	if !hasInventoryPath || !hasFreezePath {
		t.Fatalf("PSS-F01 exclusivePaths = %#v, want inventory + freeze artifacts", f01.ExclusivePaths)
	}

	hasCheckerDoc := false
	for _, path := range f02.ExclusivePaths {
		if path == ownershipinventory.PSSF02FirstCheckerLeasePath {
			hasCheckerDoc = true
		}
	}
	if !hasCheckerDoc {
		t.Fatalf("PSS-F02 exclusivePaths = %#v, want first checker slice %q", f02.ExclusivePaths, ownershipinventory.PSSF02FirstCheckerLeasePath)
	}
}

func packetIDs(freeze ownershipinventory.PathLeaseFreeze) []string {
	ids := make([]string, 0, len(freeze.Packets))
	for _, packet := range freeze.Packets {
		ids = append(ids, packet.PacketID)
	}
	return ids
}

func packetByID(t *testing.T, freeze ownershipinventory.PathLeaseFreeze, packetID string) psslease.Packet {
	t.Helper()
	for _, packet := range freeze.Packets {
		if packet.PacketID == packetID {
			return packet
		}
	}
	t.Fatalf("packet %q not found", packetID)
	return psslease.Packet{}
}

