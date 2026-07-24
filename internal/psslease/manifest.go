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

	StateBlocked     = "blocked"
	StateReady       = "ready"
	StateActive      = "active"
	StateReview      = "review"
	StateIntegration = "integration"
	StateDone        = "done"
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
	return ValidateLeaseHolders(manifest)
}

// ValidateLeaseHolders rejects overlapping exclusive path claims among packets
// that are already in lease-holding states. Non-holding states never conflict
// by path alone.
//
// Path overlap uses a documented prefix rule: after slash normalization, two
// paths overlap when they are equal, or when one is a directory/file prefix of
// the other (matched on path segment boundaries so "pkg/foo" does not collide
// with "pkg/foobar").
func ValidateLeaseHolders(manifest *Manifest) error {
	if manifest == nil {
		return fmt.Errorf("validate pss lease holders: manifest is nil")
	}
	if err := ValidateManifest(manifest); err != nil {
		return err
	}

	holders := make([]Packet, 0, len(manifest.Packets))
	for _, packet := range manifest.Packets {
		if IsLeaseHoldingState(packet.State) {
			holders = append(holders, packet)
		}
	}
	for i := 0; i < len(holders); i++ {
		for j := i + 1; j < len(holders); j++ {
			left := holders[i]
			right := holders[j]
			overlap := overlappingExclusivePaths(left.ExclusivePaths, right.ExclusivePaths)
			if len(overlap) == 0 {
				continue
			}
			return fmt.Errorf(
				"validate pss lease holders: overlapping exclusive paths between %q and %q: %s",
				strings.TrimSpace(left.PacketID),
				strings.TrimSpace(right.PacketID),
				strings.Join(overlap, ", "),
			)
		}
	}
	return nil
}

// SetPacketState applies a planner state update to packetID after the
// pre-dispatch overlap gate. Callers edit program-metadata manifests (and
// co-located fixtures) only; this helper does not touch OpenAPI, CLI, or
// provider surfaces.
//
// Documented planner lifecycle: blocked|ready → active → review|integration → done.
func SetPacketState(manifest *Manifest, packetID, targetState string) error {
	if manifest == nil {
		return fmt.Errorf("set pss packet state: manifest is nil")
	}
	packetID = strings.TrimSpace(packetID)
	if packetID == "" {
		return fmt.Errorf("set pss packet state: missing packetId")
	}
	if err := ValidateDispatchCandidate(manifest, packetID, targetState); err != nil {
		return err
	}

	for index := range manifest.Packets {
		if strings.TrimSpace(manifest.Packets[index].PacketID) != packetID {
			continue
		}
		prior := manifest.Packets[index].State
		manifest.Packets[index].State = targetState
		if err := ValidateLeaseHolders(manifest); err != nil {
			manifest.Packets[index].State = prior
			return err
		}
		return nil
	}
	return fmt.Errorf("set pss packet state: unknown packetId %q", packetID)
}

// ValidateDispatchCandidate rejects promoting packetID into targetState when
// that transition would create a lease-holding overlap with an existing holder.
// Non-holding target states always pass the overlap gate.
func ValidateDispatchCandidate(manifest *Manifest, packetID, targetState string) error {
	if manifest == nil {
		return fmt.Errorf("validate pss dispatch candidate: manifest is nil")
	}
	packetID = strings.TrimSpace(packetID)
	if packetID == "" {
		return fmt.Errorf("validate pss dispatch candidate: missing packetId")
	}
	if _, ok := AllowedStates[targetState]; !ok {
		return fmt.Errorf("validate pss dispatch candidate: packet %q: unknown packet state %q", packetID, targetState)
	}
	if err := ValidateManifest(manifest); err != nil {
		return err
	}
	if !IsLeaseHoldingState(targetState) {
		return nil
	}

	var candidate *Packet
	for index := range manifest.Packets {
		if strings.TrimSpace(manifest.Packets[index].PacketID) == packetID {
			candidate = &manifest.Packets[index]
			break
		}
	}
	if candidate == nil {
		return fmt.Errorf("validate pss dispatch candidate: unknown packetId %q", packetID)
	}

	for _, packet := range manifest.Packets {
		otherID := strings.TrimSpace(packet.PacketID)
		if otherID == packetID {
			continue
		}
		if !IsLeaseHoldingState(packet.State) {
			continue
		}
		overlap := overlappingExclusivePaths(candidate.ExclusivePaths, packet.ExclusivePaths)
		if len(overlap) == 0 {
			continue
		}
		return fmt.Errorf(
			"validate pss dispatch candidate: overlapping exclusive paths between %q and %q: %s",
			packetID,
			otherID,
			strings.Join(overlap, ", "),
		)
	}
	return nil
}

func overlappingExclusivePaths(left, right []string) []string {
	leftPaths := normalizeExclusivePaths(left)
	rightPaths := normalizeExclusivePaths(right)
	overlap := make([]string, 0)
	seen := make(map[string]struct{})
	add := func(path string) {
		if _, ok := seen[path]; ok {
			return
		}
		seen[path] = struct{}{}
		overlap = append(overlap, path)
	}
	for _, leftPath := range leftPaths {
		for _, rightPath := range rightPaths {
			if !pathsOverlap(leftPath, rightPath) {
				continue
			}
			// Include both claims so diagnostics name every overlapping path
			// prefix/file involved in the conflict.
			add(leftPath)
			add(rightPath)
		}
	}
	return overlap
}

// pathsOverlap reports whether two exclusive path claims collide under the
// documented path-prefix rule.
func pathsOverlap(left, right string) bool {
	left = normalizePath(left)
	right = normalizePath(right)
	if left == "" || right == "" {
		return false
	}
	if left == right {
		return true
	}
	return isPathPrefix(left, right) || isPathPrefix(right, left)
}

func isPathPrefix(prefix, path string) bool {
	if prefix == path {
		return true
	}
	if !strings.HasPrefix(path, prefix) {
		return false
	}
	// Directory leases (trailing slash) cover everything beneath them.
	if strings.HasSuffix(prefix, "/") {
		return true
	}
	// File/directory segment boundary: "pkg/foo" covers "pkg/foo/..." but not
	// "pkg/foobar".
	remainder := path[len(prefix):]
	return strings.HasPrefix(remainder, "/")
}

func normalizeExclusivePaths(paths []string) []string {
	if len(paths) == 0 {
		return nil
	}
	normalized := make([]string, 0, len(paths))
	for _, path := range paths {
		trimmed := normalizePath(path)
		if trimmed == "" {
			continue
		}
		normalized = append(normalized, trimmed)
	}
	return normalized
}

func normalizePath(path string) string {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return ""
	}
	return strings.ReplaceAll(trimmed, "\\", "/")
}
