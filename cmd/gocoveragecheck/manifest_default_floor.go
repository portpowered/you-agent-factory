package main

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
)

// Per-lane default coverage floors. A measured package with no manifest entry
// is held to its lane's default floor instead of failing the manifest
// completeness check, so adding a package no longer requires a manifest edit.
//
// Both values come from the measured distribution of existing floors: the unit
// lane's tenth percentile is 50.00, so a new package that reaches the default
// sits in the bottom decile rather than failing on arrival.
//
// The functional default was 0.00, justified on the grounds that a positive
// default would fail ordinary packages en masse. That is a statement about
// blast radius, not about correctness, and it made the functional gate assert
// nothing for any unregistered package. Functional tests exist to prove
// systematic correctness end to end, so a package no functional flow reaches is
// precisely the package worth reporting: if nothing exercises it, there is no
// evidence the system works when it is involved.
//
// The cost of the old default is concrete. pkg/services/costs and its
// internal/service and transports/http packages all sat at zero functional
// coverage, and a price table that defaulted to zero models produced no cost
// figure at all for a month while every unit test passed. Nothing exercised
// that path end to end, and nothing was required to.
//
// 15.00 is the tenth percentile of the functional floors of packages that ARE
// exercised (the whole-population tenth percentile is 0.00, which is itself the
// finding). It is deliberately a low bar meaning "some flow genuinely reaches
// this package", not unit-like density, and it is expected to ratchet upward.
const (
	unitLaneDefaultFloorPercent       = "50.00"
	functionalLaneDefaultFloorPercent = "15.00"
)

// laneDefaultCoverageFloor reports the built-in default floor for lane. It is
// the floor used when a manifest omits defaultFloorPercent.
func laneDefaultCoverageFloor(lane string) coverageFloor {
	if lane == "functional" {
		floor, _ := parseCoverageFloor(json.RawMessage(functionalLaneDefaultFloorPercent))
		return floor
	}
	floor, _ := parseCoverageFloor(json.RawMessage(unitLaneDefaultFloorPercent))
	return floor
}

// laneDefaultCoverageFloorRaw renders lane's built-in default floor in the
// manifest's fixed two-decimal form.
func laneDefaultCoverageFloorRaw(lane string) json.RawMessage {
	if lane == "functional" {
		return json.RawMessage(functionalLaneDefaultFloorPercent)
	}
	return json.RawMessage(unitLaneDefaultFloorPercent)
}

// defaultFloor reports the floor applied to a measured package that has no
// entry in this manifest. An explicit manifest-level defaultFloorPercent wins;
// otherwise the lane's built-in default applies.
func (manifest coverageManifest) defaultFloor() coverageFloor {
	if len(manifest.DefaultFloorPercent) > 0 && string(manifest.DefaultFloorPercent) != "null" {
		if floor, err := parseCoverageFloor(manifest.DefaultFloorPercent); err == nil {
			return floor
		}
	}
	return laneDefaultCoverageFloor(manifest.Lane)
}

// manifestPackageSet reports the import paths carrying an explicit manifest
// entry. Those entries always win over the lane default.
func manifestPackageSet(manifest coverageManifest) map[string]struct{} {
	listed := make(map[string]struct{}, len(manifest.Packages))
	for _, entry := range manifest.Packages {
		listed[entry.Package] = struct{}{}
	}
	return listed
}

// checkCoverageDefaultFloor evaluates every measured package with no manifest
// entry against the manifest's default floor. A package whose profile reports
// no measurable statements passes vacuously and is not reported.
func checkCoverageDefaultFloor(
	manifest coverageManifest,
	totals map[string]packageCoverageTotals,
	manifestPath string,
	coverageBlocks map[string]coverageBlock,
) []string {
	listed := manifestPackageSet(manifest)
	floor := manifest.defaultFloor()
	failures := make([]string, 0)
	for _, importPath := range slices.Sorted(maps.Keys(totals)) {
		if _, explicit := listed[importPath]; explicit {
			continue
		}
		actual := totals[importPath]
		if coveragePassesFloor(floor, actual) {
			continue
		}
		failures = append(failures, formatDefaultFloorRegression(manifest, importPath, floor, actual, manifestPath, coverageBlocks))
	}
	return failures
}

// coveragePassesFloor reports whether actual satisfies floor. A package with no
// measurable statements passes: the floor cannot be evaluated against an empty
// denominator, and the coverage profile already records that fact.
func coveragePassesFloor(floor coverageFloor, actual packageCoverageTotals) bool {
	if actual.totalStatements <= 0 {
		return true
	}
	return int64(actual.coveredStatements)*10000 >= int64(floor)*int64(actual.totalStatements)
}

func formatDefaultFloorRegression(
	manifest coverageManifest,
	importPath string,
	floor coverageFloor,
	actual packageCoverageTotals,
	manifestPath string,
	coverageBlocks map[string]coverageBlock,
) string {
	actualPercent := float64(actual.coveredStatements) * 100 / float64(actual.totalStatements)
	expectedPercent := float64(floor) / 100
	diagnostic := fmt.Sprintf(
		"package coverage regression: package=%s lane=%s floor-source=lane-default expected-minimum=%s%% actual=%.4f%% delta=%+.4f percentage-points covered=%d/%d statements",
		importPath,
		manifest.Lane,
		floor.String(),
		actualPercent,
		actualPercent-expectedPercent,
		actual.coveredStatements,
		actual.totalStatements,
	)
	if uncovered := formatUncoveredCoverageBlocks(coverageBlocks, importPath); uncovered != "" {
		diagnostic += "; " + uncovered
	}
	return diagnostic + fmt.Sprintf(
		"; this package has no entry in %s, so it is held to the %s lane default floor of %s%%; raise its coverage or record an explicit minimum for it",
		manifestPath, manifest.Lane, floor.String(),
	)
}

// coverageManifestGatedPackages reports the gate applied to each measured
// package, including packages resolved through the lane default floor rather
// than an explicit entry.
func coverageManifestGatedPackages(manifest coverageManifest, totals map[string]packageCoverageTotals) map[string]packageCoverageGate {
	gates := packageGatesFromManifest(manifest)
	floor := manifest.defaultFloor()
	for importPath, actual := range totals {
		if _, explicit := gates[importPath]; explicit {
			continue
		}
		if actual.totalStatements <= 0 {
			continue
		}
		percent := float64(floor) / 100
		gates[importPath] = packageCoverageGate{Floor: &percent}
	}
	return gates
}
