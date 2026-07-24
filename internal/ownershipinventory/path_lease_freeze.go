package ownershipinventory

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/portpowered/infinite-you/internal/psslease"
)

const (
	// PathLeaseFreezeRelativePath is the PSS-F01 initial path-lease freeze
	// artifact referenced by the FND-10 packet catalog.
	PathLeaseFreezeRelativePath = "docs/internal/projects/packaged-service-structure/ownership-path-lease-freeze.json"

	// PathLeaseFreezeID identifies the initial freeze published from the
	// ownership inventory.
	PathLeaseFreezeID = "pss-f01-initial-path-lease-freeze"

	// PSSF02FirstCheckerLeasePath is the first PSS-F02 owner-boundary checker
	// slice unlocked by this freeze.
	PSSF02FirstCheckerLeasePath = "docs/internal/projects/packaged-service-structure/owner-boundary-enforcement.md"

	PathLeaseFreezePacketSortKeyDescription = "packetId ascending byte order"
)

// RequiredPathLeaseFreezePacketIDs are the ownership-inventory freeze and the
// first PSS-F02 foundation checker slice derived from that freeze.
var RequiredPathLeaseFreezePacketIDs = []string{
	"PSS-F01",
	"PSS-F02",
}

// PortfolioHoldExclusion records a live external portfolio hold that the
// initial freeze must not claim as an exclusive changed-path lease.
type PortfolioHoldExclusion struct {
	ID    string   `json:"id"`
	Paths []string `json:"paths"`
	Note  string   `json:"note"`
}

// PathLeaseFreeze is the PSS-F01 initial exclusive changed-path lease freeze.
// Packet shape and overlap mechanics reuse the FND-10 pss-path-lease-packet-
// manifest/v1 contract via internal/psslease.
type PathLeaseFreeze struct {
	FormatVersion           string                   `json:"formatVersion"`
	FreezeID                string                   `json:"freezeId"`
	SourceInventory         string                   `json:"sourceInventory"`
	SortKey                 string                   `json:"sortKey"`
	Packets                 []psslease.Packet        `json:"packets"`
	PortfolioHoldExclusions []PortfolioHoldExclusion `json:"portfolioHoldExclusions"`
}

// PathLeaseFreezeReport is the focused path-lease freeze validation result.
type PathLeaseFreezeReport struct {
	MissingPackets            []string
	EmptyExclusivePathPackets []string
	PortfolioHoldConflicts    []string
	InvalidFormatVersion      bool
	MissingSourceInventory    bool
	UnstablePacketSort        bool
	OverlappingActiveLeases   bool
	OverlapDetail             string
	ManifestError             string
}

// OK reports whether path-lease freeze validation found no defects.
func (r PathLeaseFreezeReport) OK() bool {
	return len(r.MissingPackets) == 0 &&
		len(r.EmptyExclusivePathPackets) == 0 &&
		len(r.PortfolioHoldConflicts) == 0 &&
		!r.InvalidFormatVersion &&
		!r.MissingSourceInventory &&
		!r.UnstablePacketSort &&
		!r.OverlappingActiveLeases &&
		r.ManifestError == ""
}

// BuildPathLeaseFreeze returns the deterministic initial freeze derived from
// the ownership-inventory packet and the first PSS-F02 checker slice.
func BuildPathLeaseFreeze() PathLeaseFreeze {
	packets := []psslease.Packet{
		{
			PacketID:   "PSS-F01",
			LeaseClass: "ownership-inventory",
			ExclusivePaths: []string{
				InventoryRelativePath,
				"docs/internal/baselines/README.md",
				PathLeaseFreezeRelativePath,
				"internal/ownershipinventory/",
				"cmd/ownershipinventoryfreeze/",
			},
			State:         psslease.StateActive,
			Prerequisites: []string{"FND-01", "FND-10"},
		},
		{
			PacketID:   "PSS-F02",
			LeaseClass: "architecture-rule",
			ExclusivePaths: []string{
				PSSF02FirstCheckerLeasePath,
			},
			State:         psslease.StateReady,
			Prerequisites: []string{"PSS-F01"},
		},
	}
	slices.SortFunc(packets, func(a, b psslease.Packet) int {
		return strings.Compare(a.PacketID, b.PacketID)
	})
	return PathLeaseFreeze{
		FormatVersion:           psslease.FormatVersion,
		FreezeID:                PathLeaseFreezeID,
		SourceInventory:         InventoryRelativePath,
		SortKey:                 PathLeaseFreezePacketSortKeyDescription,
		Packets:                 packets,
		PortfolioHoldExclusions: defaultPortfolioHoldExclusions(),
	}
}

func defaultPortfolioHoldExclusions() []PortfolioHoldExclusion {
	return []PortfolioHoldExclusion{
		{
			ID: "cli-manifest",
			Paths: []string{
				"cmd/climanifestgen/",
				"contracts/cli/",
				"cmd/factory/",
			},
			Note: "Schema CLI / Clean CLI portfolio hold owns canonical CLI manifest/generation (PR #1262); PSS-I03 stays held.",
		},
		{
			ID: "provider-conductor",
			Paths: []string{
				"pkg/services/workers/provider/",
				"pkg/services/workers/cliprovider/",
				"pkg/services/workers/agypty/",
			},
			Note: "Standardized Providers neutral-conductor lane owns provider composition; do not claim those holds in this freeze.",
		},
	}
}

// LoadPathLeaseFreeze reads the frozen path-lease freeze artifact.
func LoadPathLeaseFreeze(root string) (PathLeaseFreeze, error) {
	path := filepath.Join(root, filepath.FromSlash(PathLeaseFreezeRelativePath))
	payload, err := os.ReadFile(path)
	if err != nil {
		return PathLeaseFreeze{}, fmt.Errorf("read path-lease freeze %s: %w", PathLeaseFreezeRelativePath, err)
	}
	var freeze PathLeaseFreeze
	if err := json.Unmarshal(payload, &freeze); err != nil {
		return PathLeaseFreeze{}, fmt.Errorf("parse path-lease freeze %s: %w", PathLeaseFreezeRelativePath, err)
	}
	return freeze, nil
}

// WritePathLeaseFreeze writes the path-lease freeze artifact.
func WritePathLeaseFreeze(root string, freeze PathLeaseFreeze) error {
	path := filepath.Join(root, filepath.FromSlash(PathLeaseFreezeRelativePath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir path-lease freeze: %w", err)
	}
	payload, err := json.MarshalIndent(freeze, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal path-lease freeze: %w", err)
	}
	payload = append(payload, '\n')
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		return fmt.Errorf("write path-lease freeze: %w", err)
	}
	return nil
}

// ValidatePathLeaseFreeze checks freeze completeness, FND-10 mechanics, and
// portfolio-hold exclusions.
func ValidatePathLeaseFreeze(freeze PathLeaseFreeze) PathLeaseFreezeReport {
	report := PathLeaseFreezeReport{}
	if freeze.FormatVersion != psslease.FormatVersion {
		report.InvalidFormatVersion = true
	}
	if freeze.SourceInventory != InventoryRelativePath {
		report.MissingSourceInventory = true
	}
	if freeze.SortKey != PathLeaseFreezePacketSortKeyDescription {
		report.UnstablePacketSort = true
	}

	ids := make([]string, 0, len(freeze.Packets))
	present := map[string]psslease.Packet{}
	for _, packet := range freeze.Packets {
		ids = append(ids, packet.PacketID)
		present[packet.PacketID] = packet
		if len(normalizeFreezePaths(packet.ExclusivePaths)) == 0 {
			report.EmptyExclusivePathPackets = append(report.EmptyExclusivePathPackets, packet.PacketID)
		}
	}
	if !slices.IsSorted(ids) {
		report.UnstablePacketSort = true
	}
	for _, packetID := range RequiredPathLeaseFreezePacketIDs {
		if _, ok := present[packetID]; !ok {
			report.MissingPackets = append(report.MissingPackets, packetID)
		}
	}
	slices.Sort(report.MissingPackets)
	slices.Sort(report.EmptyExclusivePathPackets)

	manifest := &psslease.Manifest{
		FormatVersion: freeze.FormatVersion,
		Packets:       freeze.Packets,
	}
	if err := psslease.ValidateManifest(manifest); err != nil {
		report.ManifestError = err.Error()
		// Still surface empty-path packets from the freeze-required set when
		// ValidateManifest rejects for the same reason.
	}
	if err := psslease.ValidateLeaseHolders(manifest); err != nil {
		report.OverlappingActiveLeases = true
		report.OverlapDetail = err.Error()
	}

	exclusions := freeze.PortfolioHoldExclusions
	if len(exclusions) == 0 {
		exclusions = defaultPortfolioHoldExclusions()
	}
	for _, packet := range freeze.Packets {
		for _, path := range normalizeFreezePaths(packet.ExclusivePaths) {
			for _, hold := range exclusions {
				for _, excluded := range hold.Paths {
					if freezePathsOverlap(path, excluded) {
						report.PortfolioHoldConflicts = append(
							report.PortfolioHoldConflicts,
							fmt.Sprintf("%s claims %q which overlaps portfolio hold %s path %q", packet.PacketID, path, hold.ID, excluded),
						)
					}
				}
			}
		}
	}
	slices.Sort(report.PortfolioHoldConflicts)
	report.PortfolioHoldConflicts = slices.Compact(report.PortfolioHoldConflicts)
	return report
}

func normalizeFreezePaths(paths []string) []string {
	out := make([]string, 0, len(paths))
	for _, path := range paths {
		trimmed := strings.TrimSpace(strings.ReplaceAll(path, "\\", "/"))
		if trimmed == "" {
			continue
		}
		out = append(out, trimmed)
	}
	return out
}

func freezePathsOverlap(left, right string) bool {
	left = strings.TrimSpace(strings.ReplaceAll(left, "\\", "/"))
	right = strings.TrimSpace(strings.ReplaceAll(right, "\\", "/"))
	if left == "" || right == "" {
		return false
	}
	if left == right {
		return true
	}
	return freezePathPrefix(left, right) || freezePathPrefix(right, left)
}

func freezePathPrefix(prefix, path string) bool {
	if prefix == path {
		return true
	}
	if !strings.HasPrefix(path, prefix) {
		return false
	}
	if strings.HasSuffix(prefix, "/") {
		return true
	}
	remainder := path[len(prefix):]
	return strings.HasPrefix(remainder, "/")
}
