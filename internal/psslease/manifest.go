// Package psslease defines the Packaged Service Structure changed-path lease
// and packet-state program-metadata contract used before dispatch.
package psslease

import (
	"encoding/json"
	"fmt"
	"strings"
)

const (
	// FormatVersion is the stable program-metadata format identifier.
	FormatVersion = "pss-path-lease-packet-manifest/v1"

	StateBlocked      = "blocked"
	StateReady        = "ready"
	StateActive       = "active"
	StateReview       = "review"
	StateIntegration  = "integration"
	StateDone         = "done"
)

// AllowedStates is the closed packet-state vocabulary from the PSS plan.
var AllowedStates = map[string]struct{}{
	StateBlocked:     {},
	StateReady:       {},
	StateActive:      {},
	StateReview:      {},
	StateIntegration: {},
	StateDone:        {},
}

// LeaseHoldingStates are in-flight exclusive path claimants. Overlapping
// exclusive paths among these states must be rejected before dispatch.
var LeaseHoldingStates = map[string]struct{}{
	StateActive:      {},
	StateReview:      {},
	StateIntegration: {},
}

// RequiredCatalogPacketIDs is the Phase 0 foundation wave plus shared
// foundation/integration lane IDs the lease matrix schedules against.
var RequiredCatalogPacketIDs = []string{
	"FND-01",
	"FND-02",
	"FND-03",
	"FND-04",
	"FND-05",
	"FND-06",
	"FND-07",
	"FND-08",
	"FND-09",
	"FND-10",
	"FND-11",
	"FND-12",
	"PSS-F01",
	"PSS-F02",
	"PSS-I01",
	"PSS-I02",
	"PSS-I03",
	"PSS-I04",
	"PSS-I05",
}

// Manifest is the program-metadata lease and packet-state ledger.
type Manifest struct {
	FormatVersion string   `json:"formatVersion"`
	Packets       []Packet `json:"packets"`
}

// Packet binds one cataloged work packet to an exclusive changed-path lease
// and current scheduling state.
type Packet struct {
	PacketID       string   `json:"packetId"`
	ExclusivePaths []string `json:"exclusivePaths"`
	State          string   `json:"state"`
	Prerequisites  []string `json:"prerequisites,omitempty"`
	LeaseClass     string   `json:"leaseClass,omitempty"`
}

// DecodeManifest decodes a program-metadata lease manifest.
func DecodeManifest(data []byte) (*Manifest, error) {
	manifest := &Manifest{}
	if err := json.Unmarshal(data, manifest); err != nil {
		return nil, fmt.Errorf("decode pss lease manifest: %w", err)
	}
	return manifest, nil
}

// IsLeaseHoldingState reports whether state claims an exclusive path lease.
func IsLeaseHoldingState(state string) bool {
	_, ok := LeaseHoldingStates[state]
	return ok
}

// ValidateManifest rejects unknown states, missing packet IDs, empty exclusive
// path sets, and duplicate packet IDs before any dispatch decision.
func ValidateManifest(manifest *Manifest) error {
	if manifest == nil {
		return fmt.Errorf("validate pss lease manifest: manifest is nil")
	}
	if manifest.FormatVersion != FormatVersion {
		return fmt.Errorf("validate pss lease manifest: unknown formatVersion %q", manifest.FormatVersion)
	}

	seen := make(map[string]int, len(manifest.Packets))
	for index, packet := range manifest.Packets {
		packetID := strings.TrimSpace(packet.PacketID)
		if packetID == "" {
			return fmt.Errorf("validate pss lease manifest: packets[%d]: missing packetId", index)
		}
		if first, exists := seen[packetID]; exists {
			return fmt.Errorf("validate pss lease manifest: duplicate packetId %q (first at packets[%d], again at packets[%d])", packetID, first, index)
		}
		seen[packetID] = index

		if strings.TrimSpace(packet.State) == "" {
			return fmt.Errorf("validate pss lease manifest: packet %q: missing packet state", packetID)
		}
		if _, ok := AllowedStates[packet.State]; !ok {
			return fmt.Errorf("validate pss lease manifest: packet %q: unknown packet state %q", packetID, packet.State)
		}

		paths := normalizeExclusivePaths(packet.ExclusivePaths)
		if len(paths) == 0 {
			return fmt.Errorf("validate pss lease manifest: packet %q: empty exclusivePaths", packetID)
		}
		for pathIndex, path := range paths {
			if path == "" {
				return fmt.Errorf("validate pss lease manifest: packet %q: exclusivePaths[%d] is empty", packetID, pathIndex)
			}
		}
	}
	return nil
}

// ValidateCatalog runs structural validation then requires every Phase 0
// foundation and shared-lane packet ID to appear exactly once.
func ValidateCatalog(manifest *Manifest) error {
	if err := ValidateManifest(manifest); err != nil {
		return err
	}

	present := make(map[string]struct{}, len(manifest.Packets))
	for _, packet := range manifest.Packets {
		present[strings.TrimSpace(packet.PacketID)] = struct{}{}
	}
	for _, packetID := range RequiredCatalogPacketIDs {
		if _, ok := present[packetID]; !ok {
			return fmt.Errorf("validate pss lease catalog: missing cataloged packet %q", packetID)
		}
	}
	return nil
}

func normalizeExclusivePaths(paths []string) []string {
	if len(paths) == 0 {
		return nil
	}
	normalized := make([]string, 0, len(paths))
	for _, path := range paths {
		trimmed := strings.TrimSpace(path)
		if trimmed == "" {
			continue
		}
		normalized = append(normalized, trimmed)
	}
	return normalized
}
