package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestCoverageFloorFromTotalsTruncatesDownward(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		totals packageCoverageTotals
		want   string
	}{
		{name: "repeating ratio", totals: packageCoverageTotals{coveredStatements: 2, totalStatements: 3}, want: "66.66"},
		{name: "exact ratio", totals: packageCoverageTotals{coveredStatements: 4, totalStatements: 5}, want: "80.00"},
		{name: "zero covered", totals: packageCoverageTotals{totalStatements: 7}, want: "0.00"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := coverageFloorFromTotals(tt.totals)
			if err != nil {
				t.Fatalf("coverageFloorFromTotals() error = %v", err)
			}
			if got.String() != tt.want {
				t.Fatalf("coverageFloorFromTotals() = %s, want %s", got, tt.want)
			}
			if int64(got)*int64(tt.totals.totalStatements) > int64(tt.totals.coveredStatements)*10000 {
				t.Fatalf("floor %s exceeds exact measured ratio", got)
			}
		})
	}
}

func TestCoverageFloorFromTotalsRejectsUnmeasurableAndInvalidCounts(t *testing.T) {
	t.Parallel()

	for _, totals := range []packageCoverageTotals{
		{},
		{coveredStatements: -1, totalStatements: 10},
		{coveredStatements: 11, totalStatements: 10},
	} {
		if _, err := coverageFloorFromTotals(totals); err == nil {
			t.Fatalf("coverageFloorFromTotals(%+v) unexpectedly succeeded", totals)
		}
	}
}

func TestNewCoverageManifestIsSortedAndByteDeterministic(t *testing.T) {
	t.Parallel()

	packages := []string{modulePath + "/pkg/service", modulePath + "/pkg/config"}
	totals := map[string]packageCoverageTotals{
		packages[0]: {coveredStatements: 2, totalStatements: 3},
		packages[1]: {coveredStatements: 4, totalStatements: 5},
	}
	first, err := newCoverageManifest("unit", totals, packages)
	if err != nil {
		t.Fatalf("newCoverageManifest() error = %v", err)
	}
	second, err := newCoverageManifest("unit", totals, slices.Clone(packages))
	if err != nil {
		t.Fatalf("newCoverageManifest() second error = %v", err)
	}
	firstData, err := renderCoverageManifest(first)
	if err != nil {
		t.Fatalf("renderCoverageManifest() error = %v", err)
	}
	secondData, err := renderCoverageManifest(second)
	if err != nil {
		t.Fatalf("renderCoverageManifest() second error = %v", err)
	}
	if string(firstData) != string(secondData) {
		t.Fatalf("manifest rendering is not deterministic:\n%s\n---\n%s", firstData, secondData)
	}
	wantOrder := []string{modulePath + "/pkg/config", modulePath + "/pkg/service"}
	if got := packageImportPaths([]packageCoverageSummary{{importPath: first.Packages[0].Package}, {importPath: first.Packages[1].Package}}); !slices.Equal(got, wantOrder) {
		t.Fatalf("manifest package order = %v, want %v", got, wantOrder)
	}
	if !strings.Contains(string(firstData), `"minimum": 66.66`) {
		t.Fatalf("manifest = %s, want fixed two-decimal numeric floor", firstData)
	}
}

func TestNewCoverageManifestUsesExplicitExceptionForUnmeasurablePackage(t *testing.T) {
	t.Parallel()

	importPath := modulePath + "/cmd/factory"
	manifest, err := newCoverageManifest("unit", map[string]packageCoverageTotals{}, []string{importPath})
	if err != nil {
		t.Fatalf("newCoverageManifest() error = %v", err)
	}
	entry := manifest.Packages[0]
	if len(entry.Minimum) != 0 {
		t.Fatalf("minimum = %s, want no fabricated floor", entry.Minimum)
	}
	if entry.Exception == nil || entry.Exception.Kind != "measurement" || entry.Exception.Deadline != unmeasurablePackageDeadline {
		t.Fatalf("exception = %+v, want structured measurement exception", entry.Exception)
	}
	if got, want := entry.Exception.Justification, "The active unit coverage profile contains no measurable statements for this package."; got != want {
		t.Fatalf("exception justification = %q, want %q", got, want)
	}
	if strings.Contains(entry.Exception.Justification, "declaration-only") {
		t.Fatalf("exception justification %q invents an uninspected package property", entry.Exception.Justification)
	}
	if err := validateCoverageManifest(manifest, "unit", []string{importPath}); err != nil {
		t.Fatalf("generated manifest does not validate: %v", err)
	}
}

func TestReadCoverageManifestValidatesContract(t *testing.T) {
	t.Parallel()

	configPackage := modulePath + "/pkg/config"
	servicePackage := modulePath + "/pkg/service"
	valid := `{"version":1,"lane":"unit","packages":[{"package":"` + configPackage + `","minimum":80.00},{"package":"` + servicePackage + `","exception":{"kind":"measurement","justification":"coverage profile omits generated bridge statements","owner":"backend-quality","deadline":"2026-12-31","removalGate":"profile records bridge statements"}}]}`
	if _, err := readCoverageManifest([]byte(valid), "unit", []string{configPackage, servicePackage}); err != nil {
		t.Fatalf("readCoverageManifest() error = %v", err)
	}

	tests := []struct {
		name     string
		manifest string
		measured []string
		want     string
	}{
		{name: "unknown lane", manifest: strings.Replace(valid, `"unit"`, `"long"`, 1), measured: []string{configPackage, servicePackage}, want: "unknown lane"},
		{name: "duplicate", manifest: `{"version":1,"lane":"unit","packages":[{"package":"` + configPackage + `","minimum":80.00},{"package":"` + configPackage + `","minimum":81.00}]}`, measured: []string{configPackage}, want: "duplicate package"},
		{name: "outside measured set", manifest: valid, measured: []string{configPackage}, want: "outside the unit measured package set"},
		{name: "missing measured package", manifest: `{"version":1,"lane":"unit","packages":[{"package":"` + configPackage + `","minimum":80.00}]}`, measured: []string{configPackage, servicePackage}, want: "has no manifest entry"},
		{name: "unsorted", manifest: `{"version":1,"lane":"unit","packages":[{"package":"` + servicePackage + `","minimum":80.00},{"package":"` + configPackage + `","minimum":80.00}]}`, measured: []string{configPackage, servicePackage}, want: "must be sorted"},
		{name: "invalid percentage", manifest: `{"version":1,"lane":"unit","packages":[{"package":"` + configPackage + `","minimum":100.01}]}`, measured: []string{configPackage}, want: "between 0.00 and 100.00"},
		{name: "imprecise percentage", manifest: `{"version":1,"lane":"unit","packages":[{"package":"` + configPackage + `","minimum":80.0}]}`, measured: []string{configPackage}, want: "exactly two decimal places"},
		{name: "both entry forms", manifest: `{"version":1,"lane":"unit","packages":[{"package":"` + configPackage + `","minimum":80.00,"exception":{"kind":"migration"}}]}`, measured: []string{configPackage}, want: "exactly one"},
		{name: "malformed exception", manifest: `{"version":1,"lane":"unit","packages":[{"package":"` + configPackage + `","exception":{"kind":"coverage","justification":"low coverage","owner":"team","deadline":"soon","removalGate":"more tests"}}]}`, measured: []string{configPackage}, want: "must be measurement or migration"},
		{name: "unknown field", manifest: `{"version":1,"lane":"unit","unknown":true,"packages":[]}`, measured: nil, want: "unknown field"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := readCoverageManifest([]byte(tt.manifest), "unit", tt.measured)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("readCoverageManifest() error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestRenderCoverageManifestProducesValidJSON(t *testing.T) {
	t.Parallel()

	manifest, err := newCoverageManifest("functional", map[string]packageCoverageTotals{
		modulePath + "/pkg/config": {coveredStatements: 1, totalStatements: 3},
	}, []string{modulePath + "/pkg/config"})
	if err != nil {
		t.Fatalf("newCoverageManifest() error = %v", err)
	}
	data, err := renderCoverageManifest(manifest)
	if err != nil {
		t.Fatalf("renderCoverageManifest() error = %v", err)
	}
	if !json.Valid(data) {
		t.Fatalf("renderCoverageManifest() returned invalid JSON: %s", data)
	}
	if _, err := readCoverageManifest(data, "functional", []string{modulePath + "/pkg/config"}); err != nil {
		t.Fatalf("generated manifest does not validate: %v", err)
	}
}

func TestCreateCoverageManifestIsCreateOnly(t *testing.T) {
	t.Parallel()

	filename := filepath.Join(t.TempDir(), "manifest.json")
	importPath := modulePath + "/pkg/config"
	totals := map[string]packageCoverageTotals{importPath: {coveredStatements: 4, totalStatements: 5}}
	if err := createCoverageManifest(filename, "unit", totals, []string{importPath}); err != nil {
		t.Fatalf("createCoverageManifest() error = %v", err)
	}
	first, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("read generated manifest: %v", err)
	}
	if err := createCoverageManifest(filename, "unit", totals, []string{importPath}); err == nil {
		t.Fatal("createCoverageManifest() overwrote an existing manifest")
	}
	second, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("read preserved manifest: %v", err)
	}
	if string(first) != string(second) {
		t.Fatalf("existing manifest changed after rejected create:\n%s\n---\n%s", first, second)
	}
}

func TestReadCoverageManifestRejectsExpiredException(t *testing.T) {
	t.Parallel()

	importPath := modulePath + "/pkg/config"
	data := []byte(`{"version":1,"lane":"unit","packages":[{"package":"` + importPath + `","exception":{"kind":"measurement","justification":"profile defect","owner":"backend-quality","deadline":"2026-07-14","removalGate":"profile contains statements"}}]}`)
	_, err := readCoverageManifestAt(data, "unit", []string{importPath}, time.Date(2026, 7, 15, 18, 0, 0, 0, time.FixedZone("test", -7*60*60)))
	if err == nil || !strings.Contains(err.Error(), "expired exception for package \""+importPath+"\"") {
		t.Fatalf("readCoverageManifestAt() error = %v, want expired-exception diagnostic", err)
	}
}

func TestCheckCoverageManifestControlledProfilesForBothLanes(t *testing.T) {
	t.Parallel()

	alpha := modulePath + "/pkg/config"
	beta := modulePath + "/pkg/service"
	for _, lane := range []string{"unit", "functional"} {
		lane := lane
		t.Run(lane, func(t *testing.T) {
			t.Parallel()

			manifest := coverageManifest{
				Version: coverageManifestVersion,
				Lane:    lane,
				Packages: []coverageManifestEntry{
					{Package: alpha, Minimum: json.RawMessage("80.00")},
					{Package: beta, Minimum: json.RawMessage("75.00")},
				},
			}
			missing := manifest
			missing.Packages = missing.Packages[:1]
			err := validateCoverageManifestAt(missing, lane, []string{alpha, beta}, time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC))
			if err == nil || !strings.Contains(err.Error(), "measured "+lane+" package \""+beta+"\" has no manifest entry") {
				t.Fatalf("missing-entry error = %v, want lane-specific closed failure", err)
			}

			if failures := checkCoverageManifest(manifest, map[string]packageCoverageTotals{
				alpha: {coveredStatements: 4, totalStatements: 5},
				beta:  {coveredStatements: 3, totalStatements: 4},
			}, "minimums.json"); len(failures) != 0 {
				t.Fatalf("equality failures = %v, want none", failures)
			}

			failures := checkCoverageManifest(manifest, map[string]packageCoverageTotals{
				alpha: {coveredStatements: 79, totalStatements: 100},
				beta:  {coveredStatements: 74, totalStatements: 100},
			}, "minimums.json")
			if len(failures) != 2 {
				t.Fatalf("regression failures = %v, want two", failures)
			}
			if !strings.Contains(failures[0], "package="+alpha+" lane="+lane+" expected-minimum=80.00% actual=79.0000% delta=-1.0000") ||
				!strings.Contains(failures[0], "-update-manifest minimums.json") {
				t.Fatalf("first regression = %q, want complete actionable diagnostic", failures[0])
			}
			if !strings.Contains(failures[1], "package="+beta+" lane="+lane+" expected-minimum=75.00% actual=74.0000% delta=-1.0000") {
				t.Fatalf("second regression = %q, want complete diagnostic", failures[1])
			}
			if !slices.IsSorted(failures) {
				t.Fatalf("regression failures are not in stable package order: %v", failures)
			}
		})
	}
}
