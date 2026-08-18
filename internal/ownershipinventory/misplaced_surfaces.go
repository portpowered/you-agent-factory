package ownershipinventory

import (
	"slices"
	"strings"
)

const (
	MisplacedGuardKindStandard     = "standard"
	MisplacedGuardKindAllowlist    = "allowlist"
	MisplacedGuardKindPackageGuard = "package_guard"
	MisplacedGuardKindBaseline     = "baseline"
	MisplacedGuardKindDiagnostic   = "diagnostic"

	MisplacedConcernProviderInference = "provider_inference"
	MisplacedConcernHostedPolling     = "hosted_polling"
)

// RequiredMisplacedGuardIDs returns the stable-sorted IDs for any normative
// surface that still assigns provider inference or hosted polling to Workers.
func RequiredMisplacedGuardIDs() []string {
	ids := make([]string, 0, len(committedMisplacedGuards()))
	for _, entry := range committedMisplacedGuards() {
		ids = append(ids, entry.ID)
	}
	slices.Sort(ids)
	return ids
}

// BuildMisplacedGuards returns the stable-sorted misplaced-guard burn-down list.
func BuildMisplacedGuards() []MisplacedGuardEntry {
	out := append([]MisplacedGuardEntry{}, committedMisplacedGuards()...)
	slices.SortFunc(out, compareMisplacedGuards)
	return out
}

func committedMisplacedGuards() []MisplacedGuardEntry {
	// SAC S0.1's eight previously misassigned surfaces now point at their
	// durable owners. Keep this ledger empty so a future stale assignment is a
	// visible inventory defect instead of being mistaken for planned debt.
	return []MisplacedGuardEntry{}
}

func compareMisplacedGuards(a, b MisplacedGuardEntry) int {
	return strings.Compare(a.ID, b.ID)
}
