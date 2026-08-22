package main

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestCoverageDefaultFloorIsLaneSpecific pins the two lane defaults so a change
// to one cannot silently move the other.
func TestCoverageDefaultFloorIsLaneSpecific(t *testing.T) {
	t.Parallel()

	if got := laneDefaultCoverageFloor("unit").String(); got != "50.00" {
		t.Fatalf("unit lane default = %s, want 50.00", got)
	}
	if got := laneDefaultCoverageFloor("functional").String(); got != "15.00" {
		t.Fatalf("functional lane default = %s, want 15.00", got)
	}
	unitManifest := coverageManifest{Lane: "unit"}
	if got := unitManifest.defaultFloor().String(); got != "50.00" {
		t.Fatalf("unit manifest without defaultFloorPercent = %s, want the unit lane default", got)
	}
	functionalManifest := coverageManifest{Lane: "functional"}
	if got := functionalManifest.defaultFloor().String(); got != "15.00" {
		t.Fatalf("functional manifest without defaultFloorPercent = %s, want the functional lane default", got)
	}
	declared := coverageManifest{Lane: "functional", DefaultFloorPercent: json.RawMessage("25.00")}
	if got := declared.defaultFloor().String(); got != "25.00" {
		t.Fatalf("declared defaultFloorPercent = %s, want 25.00", got)
	}
}

// TestCheckCoverageDefaultFloorEvaluatesUnlistedPackagesPerLane proves the four
// default-floor cases independently for each lane. The unit and functional rows
// share the same measured totals wherever the lanes must disagree, so a default
// bleeding from one lane into the other fails this test.
func TestCheckCoverageDefaultFloorEvaluatesUnlistedPackagesPerLane(t *testing.T) {
	t.Parallel()

	unlisted := modulePath + "/pkg/platform/unlisted"
	tests := []struct {
		name         string
		lane         string
		defaultFloor string
		entries      []coverageManifestEntry
		totals       packageCoverageTotals
		wantFailure  string
		rejectText   string
	}{
		{
			name:        "unit unlisted below the lane default fails",
			lane:        "unit",
			totals:      packageCoverageTotals{coveredStatements: 40, totalStatements: 100},
			wantFailure: "package coverage regression: package=" + unlisted + " lane=unit floor-source=lane-default expected-minimum=50.00% actual=40.0000% delta=-10.0000 percentage-points covered=40/100 statements",
		},
		{
			name:   "unit unlisted at the lane default passes",
			lane:   "unit",
			totals: packageCoverageTotals{coveredStatements: 50, totalStatements: 100},
		},
		{
			name:   "unit unlisted above the lane default passes",
			lane:   "unit",
			totals: packageCoverageTotals{coveredStatements: 90, totalStatements: 100},
		},
		{
			name:        "unit explicit floor above the lane default still applies",
			lane:        "unit",
			entries:     []coverageManifestEntry{{Package: unlisted, Minimum: json.RawMessage("80.00")}},
			totals:      packageCoverageTotals{coveredStatements: 60, totalStatements: 100},
			wantFailure: "package coverage regression: package=" + unlisted + " lane=unit expected-minimum=80.00%",
			rejectText:  "floor-source=lane-default",
		},
		{
			name:        "functional unlisted below the lane default fails",
			lane:        "functional",
			totals:      packageCoverageTotals{coveredStatements: 0, totalStatements: 100},
			wantFailure: "package coverage regression: package=" + unlisted + " lane=functional floor-source=lane-default expected-minimum=15.00% actual=0.0000% delta=-15.0000 percentage-points covered=0/100 statements",
		},
		{
			name:   "functional unlisted at the lane default passes",
			lane:   "functional",
			totals: packageCoverageTotals{coveredStatements: 15, totalStatements: 100},
		},
		{
			name:   "functional unlisted above the lane default passes",
			lane:   "functional",
			totals: packageCoverageTotals{coveredStatements: 40, totalStatements: 100},
		},
		{
			name:         "functional unlisted below a declared default fails",
			lane:         "functional",
			defaultFloor: "25.00",
			totals:       packageCoverageTotals{coveredStatements: 10, totalStatements: 100},
			wantFailure:  "package coverage regression: package=" + unlisted + " lane=functional floor-source=lane-default expected-minimum=25.00% actual=10.0000% delta=-15.0000 percentage-points covered=10/100 statements",
		},
		{
			name:        "functional explicit floor above the lane default still applies",
			lane:        "functional",
			entries:     []coverageManifestEntry{{Package: unlisted, Minimum: json.RawMessage("60.00")}},
			totals:      packageCoverageTotals{coveredStatements: 40, totalStatements: 100},
			wantFailure: "package coverage regression: package=" + unlisted + " lane=functional expected-minimum=60.00%",
			rejectText:  "floor-source=lane-default",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			manifest := coverageManifest{Version: coverageManifestVersion, Lane: tt.lane, Packages: tt.entries}
			if tt.defaultFloor != "" {
				manifest.DefaultFloorPercent = json.RawMessage(tt.defaultFloor)
			}
			failures, _ := checkCoverageManifestWithEpsilon(
				manifest,
				map[string]packageCoverageTotals{unlisted: tt.totals},
				"minimums.json",
				0,
			)
			joined := strings.Join(failures, "\n")
			if tt.wantFailure == "" {
				if len(failures) != 0 {
					t.Fatalf("failures = %v, want none", failures)
				}
				return
			}
			if len(failures) != 1 {
				t.Fatalf("failures = %v, want exactly one", failures)
			}
			if !strings.Contains(joined, tt.wantFailure) {
				t.Fatalf("failure = %q, want containing %q", joined, tt.wantFailure)
			}
			if tt.rejectText != "" && strings.Contains(joined, tt.rejectText) {
				t.Fatalf("failure = %q, did not expect %q", joined, tt.rejectText)
			}
		})
	}
}

// TestCheckCoverageDefaultFloorSkipsUnlistedFunctionalPackageWithoutStatements
// keeps the empty-denominator case separate from the measurable functional
// 0/100 regression above: a package with no statements remains vacuously
// passing even under the positive functional default.
func TestCheckCoverageDefaultFloorSkipsUnlistedFunctionalPackageWithoutStatements(t *testing.T) {
	t.Parallel()

	unlisted := modulePath + "/pkg/platform/unlisted"
	manifest := coverageManifest{
		Version: coverageManifestVersion,
		Lane:    "functional",
	}
	totals := map[string]packageCoverageTotals{unlisted: {}}

	if failures := checkCoverageDefaultFloor(manifest, totals, "minimums.json", nil); len(failures) != 0 {
		t.Fatalf("default-floor failures = %v, want none for a zero-statement package", failures)
	}
	gates := coverageManifestGatedPackages(manifest, totals)
	if _, ok := gates[unlisted]; ok {
		t.Fatalf("default-floor gate = %+v, want no gate for a zero-statement package", gates[unlisted])
	}
}

// TestCheckCoverageDefaultFloorNamesTheDefaultRemedy proves the diagnostic
// explains which floor was applied and how to override it.
func TestCheckCoverageDefaultFloorNamesTheDefaultRemedy(t *testing.T) {
	t.Parallel()

	unlisted := modulePath + "/pkg/platform/unlisted"
	failures, _ := checkCoverageManifestWithEpsilon(
		coverageManifest{Version: coverageManifestVersion, Lane: "unit"},
		map[string]packageCoverageTotals{unlisted: {coveredStatements: 1, totalStatements: 10}},
		"docs/internal/baselines/go-unit-coverage-package-minimums.json",
		0,
	)
	if len(failures) != 1 {
		t.Fatalf("failures = %v, want exactly one", failures)
	}
	want := "this package has no entry in docs/internal/baselines/go-unit-coverage-package-minimums.json, so it is held to the unit lane default floor of 50.00%; raise its coverage or record an explicit minimum for it"
	if !strings.Contains(failures[0], want) {
		t.Fatalf("failure = %q, want containing %q", failures[0], want)
	}
}

// TestCoverageManifestGatedPackagesNamesDefaultFloorPackages proves an unlisted
// measured package is reported with the floor it was held to, so a package that
// resolves through the lane default is still named in checker output.
func TestCoverageManifestGatedPackagesNamesDefaultFloorPackages(t *testing.T) {
	t.Parallel()

	listed := modulePath + "/pkg/config"
	unlisted := modulePath + "/pkg/platform/unlisted"
	unmeasurable := modulePath + "/pkg/platform/contracts"
	gates := coverageManifestGatedPackages(
		coverageManifest{
			Version:  coverageManifestVersion,
			Lane:     "unit",
			Packages: []coverageManifestEntry{{Package: listed, Minimum: json.RawMessage("80.00")}},
		},
		map[string]packageCoverageTotals{
			listed:       {coveredStatements: 9, totalStatements: 10},
			unlisted:     {coveredStatements: 9, totalStatements: 10},
			unmeasurable: {},
		},
	)
	if gate := gates[listed]; gate.Floor == nil || *gate.Floor != 80 {
		t.Fatalf("listed gate = %+v, want the explicit 80 floor", gate)
	}
	if gate := gates[unlisted]; gate.Floor == nil || *gate.Floor != 50 {
		t.Fatalf("unlisted gate = %+v, want the unit lane default floor", gate)
	}
	if gate, ok := gates[unmeasurable]; ok {
		t.Fatalf("unmeasurable gate = %+v, want no floor for a package with no measurable statements", gate)
	}
}
