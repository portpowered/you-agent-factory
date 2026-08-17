package main

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// staleMeasurementException mirrors the auto-generated exception rows this lane
// retires, so the test proves the checker no longer needs them.
func staleMeasurementException(lane string) *coverageManifestException {
	return &coverageManifestException{
		Kind:          "measurement",
		Justification: "The active " + lane + " coverage profile contains no measurable statements for this package.",
		Owner:         "backend-quality",
		Deadline:      "2027-07-15",
		RemovalGate:   "The " + lane + " coverage profile reports at least one measurable statement for this package.",
	}
}

// TestZeroStatementPackagePassesWithoutAnyManifestEntry proves the vacuous pass:
// a package whose profile reports no measurable statements passes with no
// manifest row, and is not reported as a completeness gap.
func TestZeroStatementPackagePassesWithoutAnyManifestEntry(t *testing.T) {
	t.Parallel()

	for _, lane := range []string{"unit", "functional"} {
		t.Run(lane, func(t *testing.T) {
			t.Parallel()

			unmeasurable := modulePath + "/pkg/platform/contracts"
			manifest := coverageManifest{Version: coverageManifestVersion, Lane: lane}
			failures, warnings := checkCoverageManifestWithEpsilon(
				manifest,
				map[string]packageCoverageTotals{unmeasurable: {}},
				"minimums.json",
				0,
			)
			if len(failures) != 0 || len(warnings) != 0 {
				t.Fatalf("failures = %v, warnings = %v, want none", failures, warnings)
			}
			if err := validateCoverageManifestAt(manifest, lane, []string{unmeasurable}, time.Date(2026, 8, 17, 0, 0, 0, 0, time.UTC)); err != nil {
				t.Fatalf("validateCoverageManifestAt() error = %v, want no completeness gap", err)
			}
		})
	}
}

// TestZeroStatementPackagePassesWithAStaleMeasurementException proves the stale
// rows this lane deletes are harmless while they remain: an entry that is
// present still passes.
func TestZeroStatementPackagePassesWithAStaleMeasurementException(t *testing.T) {
	t.Parallel()

	unmeasurable := modulePath + "/pkg/platform/contracts"
	manifest := coverageManifest{
		Version: coverageManifestVersion,
		Lane:    "unit",
		Packages: []coverageManifestEntry{
			{Package: unmeasurable, Exception: staleMeasurementException("unit")},
		},
	}
	failures, warnings := checkCoverageManifestWithEpsilon(
		manifest,
		map[string]packageCoverageTotals{unmeasurable: {}},
		"minimums.json",
		0,
	)
	if len(failures) != 0 || len(warnings) != 0 {
		t.Fatalf("failures = %v, warnings = %v, want none", failures, warnings)
	}
	if err := validateCoverageManifestAt(manifest, "unit", []string{unmeasurable}, time.Date(2026, 8, 17, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("validateCoverageManifestAt() error = %v, want a stale measurement exception to validate", err)
	}
}

// TestRemovingAMeasurementExceptionKeepsAZeroStatementPackagePassing proves
// deleting a stale measurement row does not convert the package into a
// completeness failure or a coverage regression.
func TestRemovingAMeasurementExceptionKeepsAZeroStatementPackagePassing(t *testing.T) {
	t.Parallel()

	unmeasurable := modulePath + "/pkg/platform/contracts"
	totals := map[string]packageCoverageTotals{unmeasurable: {}}
	measured := []string{unmeasurable}
	now := time.Date(2026, 8, 17, 0, 0, 0, 0, time.UTC)

	with := coverageManifest{
		Version: coverageManifestVersion,
		Lane:    "functional",
		Packages: []coverageManifestEntry{
			{Package: unmeasurable, Exception: staleMeasurementException("functional")},
		},
	}
	withFailures, _ := checkCoverageManifestWithEpsilon(with, totals, "minimums.json", 0)
	if len(withFailures) != 0 {
		t.Fatalf("failures with the exception = %v, want none", withFailures)
	}

	without := with
	without.Packages = nil
	withoutFailures, _ := checkCoverageManifestWithEpsilon(without, totals, "minimums.json", 0)
	if len(withoutFailures) != 0 {
		t.Fatalf("failures after removing the exception = %v, want none", withoutFailures)
	}
	if err := validateCoverageManifestAt(without, "functional", measured, now); err != nil {
		t.Fatalf("validateCoverageManifestAt() error = %v, want removal to leave no completeness gap", err)
	}
}

// TestZeroStatementPackagePassesEvenWithAnExplicitFloor proves the vacuous pass
// is independent of the default floor from the first story: an explicit
// numeric floor is also not evaluated against an empty denominator. This is
// what lets a service root declare a floor before its root package carries any
// measurable statement.
func TestZeroStatementPackagePassesEvenWithAnExplicitFloor(t *testing.T) {
	t.Parallel()

	serviceRoot := modulePath + "/pkg/services/work"
	manifest := coverageManifest{
		Version: coverageManifestVersion,
		Lane:    "unit",
		Packages: []coverageManifestEntry{
			{Package: serviceRoot, Minimum: json.RawMessage("50.00")},
		},
	}
	failures, warnings := checkCoverageManifestWithEpsilon(
		manifest,
		map[string]packageCoverageTotals{serviceRoot: {}},
		"minimums.json",
		0,
	)
	if len(failures) != 0 || len(warnings) != 0 {
		t.Fatalf("failures = %v, warnings = %v, want none", failures, warnings)
	}
}

// TestUpdateManifestMintsNoRowForAnUnmeasurablePackage proves the manifest
// update path does not re-create the rows this lane retires.
func TestUpdateManifestMintsNoRowForAnUnmeasurablePackage(t *testing.T) {
	t.Parallel()

	unmeasurable := modulePath + "/pkg/platform/contracts"
	measurable := modulePath + "/pkg/config"
	manifest := coverageManifest{
		Version:             coverageManifestVersion,
		Lane:                "unit",
		DefaultFloorPercent: json.RawMessage("50.00"),
	}
	updated, updates, err := planCoverageManifestUpdate(manifest, map[string]packageCoverageTotals{
		unmeasurable: {},
		measurable:   {coveredStatements: 8, totalStatements: 10},
	})
	if err != nil {
		t.Fatalf("planCoverageManifestUpdate() error = %v", err)
	}
	if len(updated.Packages) != 1 || updated.Packages[0].Package != measurable {
		t.Fatalf("updated packages = %+v, want only the measurable package", updated.Packages)
	}
	if string(updated.DefaultFloorPercent) != "50.00" {
		t.Fatalf("defaultFloorPercent = %s, want the update to preserve it", updated.DefaultFloorPercent)
	}
	for _, update := range updates {
		if strings.Contains(update.String(), unmeasurable) {
			t.Fatalf("updates = %v, did not expect a row for an unmeasurable package", updates)
		}
	}
}
