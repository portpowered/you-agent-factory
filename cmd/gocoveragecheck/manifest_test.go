package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func retainedPolicyFixture() (coverageManifest, map[string]packageCoverageTotals) {
	manifest := coverageManifest{Version: 1, Lane: functionalCoverageSuite}
	totals := make(map[string]packageCoverageTotals)
	for importPath, floor := range map[string]coverageFloor{
		modulePath + "/pkg/wire":             8006,
		modulePath + "/pkg/platform/metrics": 7466,
		modulePath + "/pkg/services/factory_definitions/transports/mapping/validationentry":        6567,
		modulePath + "/pkg/services/work/internal/services/state_access/wire":                      7647,
		modulePath + "/pkg/services/automations/internal/services/reconciliation/internal/service": 3571,
		modulePath + "/pkg/transports/http":                                                        6559,
		modulePath + "/pkg/transports/cli/clihttp":                                                 6707,
	} {
		manifest.Packages = append(manifest.Packages, coverageManifestEntry{Package: importPath, Minimum: json.RawMessage(floor.String())})
		totals[importPath] = packageCoverageTotals{coveredStatements: int(floor), totalStatements: 10000}
	}
	slices.SortFunc(manifest.Packages, func(a, b coverageManifestEntry) int { return strings.Compare(a.Package, b.Package) })
	return manifest, totals
}

func TestRetainedFunctionalFloorsRejectLoweredManifest(t *testing.T) {
	t.Parallel()
	manifest, totals := retainedPolicyFixture()
	for index := range manifest.Packages {
		entry := &manifest.Packages[index]
		switch entry.Package {
		case modulePath + "/pkg/wire":
			entry.Minimum = json.RawMessage("71.49")
			totals[entry.Package] = packageCoverageTotals{coveredStatements: 7149, totalStatements: 10000}
		case modulePath + "/pkg/platform/metrics":
			entry.Minimum = json.RawMessage("63.15")
			totals[entry.Package] = packageCoverageTotals{coveredStatements: 6315, totalStatements: 10000}
		}
	}
	if failures, _ := checkCoverageManifestWithEpsilon(manifest, totals, "policy.json", 0); len(failures) != 0 {
		t.Fatalf("raw lowered floors should pass, got %v", failures)
	}
	failures := checkRetainedFunctionalFloors(manifest, totals, nil, t.TempDir())
	if len(failures) != 2 || !strings.Contains(strings.Join(failures, "\n"), "retained-minimum=80.06%") || !strings.Contains(strings.Join(failures, "\n"), "retained-minimum=74.66%") {
		t.Fatalf("retained deficits = %v", failures)
	}
}

func TestRetainedFunctionalFloorsAcceptRestoration(t *testing.T) {
	t.Parallel()
	manifest, totals := retainedPolicyFixture()
	if failures := checkRetainedFunctionalFloors(manifest, totals, nil, t.TempDir()); len(failures) != 0 {
		t.Fatalf("exact restoration rejected: %v", failures)
	}
	manifest.Packages[len(manifest.Packages)-1].Minimum = json.RawMessage("90.00")
	if failures := checkRetainedFunctionalFloors(manifest, totals, nil, t.TempDir()); len(failures) != 1 || !strings.Contains(failures[0], "stronger existing floor") {
		t.Fatalf("stronger floor bypassed: %v", failures)
	}
	for importPath, actual := range totals {
		actual.coveredStatements--
		totals[importPath] = actual
	}
	if failures := checkRetainedFunctionalFloors(manifest, totals, nil, t.TempDir()); len(failures) != 7 {
		t.Fatalf("just-below exact thresholds admitted: %v", failures)
	}
}

func TestRetainedFunctionalFloorsCommandPolicy(t *testing.T) {
	t.Parallel()
	for _, variant := range []string{"restored", "lowered", "epsilon", "hold", "total-only", "no-manifest", "advisory"} {
		t.Run(variant, func(t *testing.T) {
			t.Parallel()
			cfg, result, root := retainedCommandPolicyFixture(t, variant)
			got, err := applyCoverageManifestGate(cfg, result, root, nil)
			if err != nil {
				t.Fatal(err)
			}
			if variant == "restored" {
				if len(got.packageMinimumFailures) != 0 {
					t.Fatalf("restoration rejected: %v", got.packageMinimumFailures)
				}
			} else if variant == "advisory" {
				if len(got.packageMinimumFailures) != 0 || !strings.Contains(strings.Join(got.packageMinimumWarnings, "\n"), "retained functional coverage regression:") {
					t.Fatalf("advisory findings lost: %+v", got)
				}
			} else if !strings.Contains(strings.Join(got.packageMinimumFailures, "\n"), "retained functional coverage regression:") {
				t.Fatalf("%s bypassed retained admission", variant)
			}
		})
	}
}

func retainedCommandPolicyFixture(t *testing.T, variant string) (config, coverageResult, string) {
	t.Helper()
	manifest, totals := retainedPolicyFixture()
	for _, service := range []string{"automations", "factory_definitions", "work"} {
		importPath := coverageServiceRootPrefix + service
		manifest.Packages = append(manifest.Packages, coverageManifestEntry{Package: importPath, Minimum: json.RawMessage("0.00")})
		totals[importPath] = packageCoverageTotals{}
	}
	slices.SortFunc(manifest.Packages, func(a, b coverageManifestEntry) int { return strings.Compare(a.Package, b.Package) })
	wire := &manifest.Packages[len(manifest.Packages)-1]
	if variant != "restored" && variant != "no-manifest" {
		totals[wire.Package] = packageCoverageTotals{coveredStatements: 7149, totalStatements: 10000}
	}
	if variant == "lowered" || variant == "total-only" || variant == "advisory" {
		wire.Minimum = json.RawMessage("71.49")
	}
	if variant == "hold" {
		manifest.FloorHolds = []coverageManifestFloorHold{{Package: wire.Package, Owner: "owner", Deadline: "2099-01-01", Justification: "temporary hold", RemovalGate: "restore coverage"}}
	}
	root := t.TempDir()
	data, err := renderCoverageManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	filename := filepath.Join(root, "policy.json")
	if err := os.WriteFile(filename, data, 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := config{suite: functionalCoverageSuite, packageManifest: filename, totalOnly: variant == "total-only"}
	if variant == "no-manifest" {
		cfg.packageManifest = ""
		cfg.totalOnly = true
	}
	if variant == "epsilon" {
		cfg.packageFloorEpsilon = 100
	}
	if variant == "advisory" {
		cfg.packageFloorPolicy = coverageFloorPolicyAdvisory
	}
	packages := make([]string, 0, len(totals))
	for importPath := range totals {
		packages = append(packages, importPath)
	}
	result := coverageResult{packageTotals: totals, packageSummaries: summarizePackageCoverageFromTotals(totals, packages)}
	return cfg, result, root
}

func TestRetainedFunctionalFloorsRejectHoldsAndScopeExclusions(t *testing.T) {
	t.Parallel()
	for _, variant := range []string{"missing-row", "missing-profile", "zero-statements", "measurement-exception", "generic-hold", "lowered-row-with-restored-measurement"} {
		t.Run(variant, func(t *testing.T) {
			t.Parallel()
			manifest, totals := retainedPolicyFixture()
			entry := &manifest.Packages[len(manifest.Packages)-1]
			importPath := entry.Package
			switch variant {
			case "missing-row":
				manifest.Packages = manifest.Packages[:len(manifest.Packages)-1]
			case "missing-profile":
				delete(totals, importPath)
			case "zero-statements":
				totals[importPath] = packageCoverageTotals{}
			case "measurement-exception":
				entry.Minimum = nil
				entry.Exception = &coverageManifestException{Kind: "measurement"}
			case "generic-hold":
				manifest.FloorHolds = []coverageManifestFloorHold{{Package: importPath}}
				totals[importPath] = packageCoverageTotals{coveredStatements: 1, totalStatements: 100}
			case "lowered-row-with-restored-measurement":
				entry.Minimum = json.RawMessage("71.49")
			}
			failures := checkRetainedFunctionalFloors(manifest, totals, nil, t.TempDir())
			if len(failures) != 1 || !strings.Contains(failures[0], "package="+importPath+" ") {
				t.Fatalf("%s admitted or wrong diagnostics: %v", variant, failures)
			}
		})
	}
}

func retainedDispositionFixture(t *testing.T) (coverageManifestEntry, map[string]coverageBlock, string) {
	t.Helper()
	root := t.TempDir()
	file := "pkg/wire/fixture.go"
	filename := filepath.Join(root, filepath.FromSlash(file))
	if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
		t.Fatal(err)
	}
	data := []byte("package wire\nfunc fixture() {}\n")
	if err := os.WriteFile(filename, data, 0o644); err != nil {
		t.Fatal(err)
	}
	entry := coverageManifestEntry{Package: modulePath + "/pkg/wire", Minimum: json.RawMessage("71.49")}
	blocks := make(map[string]coverageBlock)
	// More than the diagnostic display limit must still be validated in full.
	for index := 1; index <= maxUncoveredCoverageBlocks+1; index++ {
		rangeSpec := fmt.Sprintf("%d.1,%d.2", index, index)
		block := coverageBlock{canonicalPath: modulePath + "/" + file, importPath: entry.Package, rangeSpec: rangeSpec, statementCount: 1}
		blocks[block.canonicalPath+":"+rangeSpec] = block
		entry.DeadCodeDispositions = append(entry.DeadCodeDispositions, coverageManifestDeadCodeDisposition{
			File: file, Range: rangeSpec, Statements: 1, SourceSHA256: fmt.Sprintf("%x", sha256.Sum256(data)),
			Classification: "unreachable", Justification: "The canonical caller rejects an absent owner before this guard can execute.",
			ReviewReference: "https://github.com/portpowered/you-agent-factory/pull/3143#discussion_r1",
		})
	}
	return entry, blocks, root
}

func TestRetainedFunctionalFloorsRequireExactDispositions(t *testing.T) {
	t.Parallel()
	entry, blocks, root := retainedDispositionFixture(t)
	actual := packageCoverageTotals{coveredStatements: 74, totalStatements: 100}
	before := coverageTotals(blocks)
	if err := checkRetainedFunctionalPackage(entry, 8006, actual, blocks, root); err != nil {
		t.Fatalf("complete source-bound disposition rejected: %v", err)
	}
	if got := coverageTotals(blocks)[entry.Package]; got != before[entry.Package] {
		t.Fatalf("raw statement totals rewritten: %v -> %v", before, got)
	}
	manifest := coverageManifest{Version: 1, Lane: functionalCoverageSuite, Packages: []coverageManifestEntry{entry}}
	data, err := renderCoverageManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := readCoverageManifest(data, functionalCoverageSuite, []string{entry.Package})
	if err != nil {
		t.Fatalf("disposition schema round trip: %v", err)
	}
	if len(decoded.Packages[0].DeadCodeDispositions) != len(entry.DeadCodeDispositions) {
		t.Fatal("dispositions lost on round trip")
	}
	diagnostics := retainedDispositionEligibilityDiagnostics(decoded, nil)
	if len(diagnostics) != 1 || !strings.Contains(diagnostics[0], "independent review") || !strings.Contains(diagnostics[0], "source-bound-blocks=26") {
		t.Fatalf("eligibility findings = %v", diagnostics)
	}
}

func TestRetainedFunctionalFloorsDispositionPreservesStrongerFloors(t *testing.T) {
	t.Parallel()
	entry, blocks, root := retainedDispositionFixture(t)
	manifest, totals := retainedPolicyFixture()
	manifest.Packages[len(manifest.Packages)-1] = entry
	totals[entry.Package] = packageCoverageTotals{totalStatements: len(blocks)}
	retainedFailures := checkRetainedFunctionalFloors(manifest, totals, blocks, root)
	if len(retainedFailures) != 0 {
		t.Fatalf("exact disposition rejected: %v", retainedFailures)
	}
	failures, _ := checkCoverageManifestWithEpsilonAndBlocks(manifest, totals, "policy.json", 0, blocks)
	if len(failures) != 1 {
		t.Fatalf("raw floor finding = %v", failures)
	}
	if got := filterDispositionFloorFindings(manifest, failures, retainedFailures); len(got) != 0 {
		t.Fatalf("valid alternative still blocked: %v", got)
	}
	manifest.Packages[len(manifest.Packages)-1].Minimum = json.RawMessage("90.00")
	failures, _ = checkCoverageManifestWithEpsilonAndBlocks(manifest, totals, "policy.json", 0, blocks)
	if got := filterDispositionFloorFindings(manifest, failures, retainedFailures); len(got) != 1 {
		t.Fatal("stronger numeric floor was bypassed")
	}
	if got := filterDispositionFloorFindings(manifest, failures, []string{"retained regression: package=" + entry.Package + " invalid"}); len(got) != 1 {
		t.Fatal("invalid disposition bypassed admission")
	}
}

func TestRetainedFunctionalFloorsRejectIncompleteOrStaleDispositions(t *testing.T) {
	t.Parallel()
	for _, variant := range []string{"missing-last", "duplicate", "foreign", "range", "statements", "digest", "source-change", "reachable", "unknown", "blank-justification", "generic-scope", "blank-review", "malformed-review", "traversal", "absolute", "backslash", "missing-source", "covered-block"} {
		t.Run(variant, func(t *testing.T) {
			t.Parallel()
			entry, blocks, root := retainedDispositionFixture(t)
			mutateDispositionFixture(t, variant, &entry, blocks, root)
			if err := checkRetainedFunctionalPackage(entry, 8006, packageCoverageTotals{coveredStatements: 74, totalStatements: 100}, blocks, root); err == nil {
				t.Fatalf("%s disposition admitted", variant)
			}
			if err := checkRetainedFunctionalPackage(entry, 8006, packageCoverageTotals{coveredStatements: 90, totalStatements: 100}, blocks, root); err == nil {
				t.Fatalf("%s stale/invalid disposition admitted after restoration", variant)
			}
		})
	}
}

func mutateDispositionFixture(t *testing.T, variant string, entry *coverageManifestEntry, blocks map[string]coverageBlock, root string) {
	t.Helper()
	record := &entry.DeadCodeDispositions[0]
	switch variant {
	case "missing-last":
		entry.DeadCodeDispositions = entry.DeadCodeDispositions[:maxUncoveredCoverageBlocks]
	case "duplicate":
		entry.DeadCodeDispositions = append(entry.DeadCodeDispositions, *record)
	case "source-change":
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(record.File)), []byte("package wire\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	case "missing-source":
		if err := os.Remove(filepath.Join(root, filepath.FromSlash(record.File))); err != nil {
			t.Fatal(err)
		}
	case "covered-block":
		key := modulePath + "/" + record.File + ":" + record.Range
		block := blocks[key]
		block.executionCount = 1
		blocks[key] = block
	default:
		mutateDispositionRecord(variant, record)
	}
}

func mutateDispositionRecord(variant string, record *coverageManifestDeadCodeDisposition) {
	switch variant {
	case "foreign":
		record.File = "pkg/platform/metrics/fixture.go"
	case "range":
		record.Range = "99.1,99.2"
	case "statements":
		record.Statements++
	case "digest":
		record.SourceSHA256 = strings.Repeat("0", 64)
	case "reachable":
		record.Classification = "reachable"
	case "unknown":
		record.Classification = "generated"
	case "blank-justification":
		record.Justification = " "
	case "generic-scope":
		record.Justification = "scope exclusion"
	case "blank-review":
		record.ReviewReference = " "
	case "malformed-review":
		record.ReviewReference = "not a review"
	case "traversal":
		record.File = "pkg/wire/../wire/fixture.go"
	case "absolute":
		record.File = "/pkg/wire/fixture.go"
	case "backslash":
		record.File = `pkg\wire\fixture.go`
	}
}

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

func TestNewCoverageManifestOmitsUnmeasurablePackagesAndDeclaresServiceRoots(t *testing.T) {
	t.Parallel()

	commandPackage := modulePath + "/cmd/factory"
	serviceRoot := modulePath + "/pkg/services/work"
	serviceInternal := modulePath + "/pkg/services/work/internal"
	measured := []string{commandPackage, serviceRoot, serviceInternal}
	manifest, err := newCoverageManifest("unit", map[string]packageCoverageTotals{}, measured)
	if err != nil {
		t.Fatalf("newCoverageManifest() error = %v", err)
	}

	entries := make(map[string]coverageManifestEntry, len(manifest.Packages))
	for _, entry := range manifest.Packages {
		entries[entry.Package] = entry
	}
	if _, ok := entries[commandPackage]; ok {
		t.Fatalf("manifest = %+v, want no row for an unmeasurable non-service package", manifest.Packages)
	}
	if _, ok := entries[serviceInternal]; ok {
		t.Fatalf("manifest = %+v, want no row for an unmeasurable package inside a declared service", manifest.Packages)
	}
	root, ok := entries[serviceRoot]
	if !ok {
		t.Fatalf("manifest = %+v, want a root row declaring the service", manifest.Packages)
	}
	if root.Exception != nil {
		t.Fatalf("service root entry = %+v, want an explicit floor rather than an exception", root)
	}
	if got := string(root.Minimum); got != unmeasuredServiceRootFloorPercent {
		t.Fatalf("service root minimum = %s, want the inert declaration floor %s", got, unmeasuredServiceRootFloorPercent)
	}
	if err := validateCoverageManifest(manifest, "unit", measured); err != nil {
		t.Fatalf("generated manifest does not validate: %v", err)
	}
}

func TestNewCoverageManifestRecordsFunctionalLaneDefaultFloor(t *testing.T) {
	t.Parallel()

	serviceRoot := modulePath + "/pkg/services/work"
	manifest, err := newCoverageManifest("functional", map[string]packageCoverageTotals{}, []string{serviceRoot})
	if err != nil {
		t.Fatalf("newCoverageManifest() error = %v", err)
	}
	if len(manifest.Packages) != 1 {
		t.Fatalf("manifest packages = %+v, want exactly the service root", manifest.Packages)
	}
	if got := string(manifest.Packages[0].Minimum); got != unmeasuredServiceRootFloorPercent {
		t.Fatalf("service root minimum = %s, want the inert declaration floor %s", got, unmeasuredServiceRootFloorPercent)
	}
	if got := string(manifest.DefaultFloorPercent); got != functionalLaneDefaultFloorPercent {
		t.Fatalf("defaultFloorPercent = %s, want %s", got, functionalLaneDefaultFloorPercent)
	}
}

func TestReadCoverageManifestValidatesContract(t *testing.T) {
	t.Parallel()

	configPackage := modulePath + "/pkg/config"
	servicePackage := modulePath + "/pkg/service"
	valid := `{"version":1,"lane":"unit","floorHolds":[{"package":"` + configPackage + `","justification":"current-main baseline is being restored","owner":"coverage-remediation","deadline":"2027-07-15","removalGate":"matching unit tests restore the existing floor"}],"packages":[{"package":"` + configPackage + `","minimum":80.00},{"package":"` + servicePackage + `","exception":{"kind":"measurement","justification":"coverage profile omits generated bridge statements","owner":"backend-quality","deadline":"2026-12-31","removalGate":"profile records bridge statements"}}]}`
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
		{name: "missing service root", manifest: `{"version":1,"lane":"unit","packages":[{"package":"` + configPackage + `","minimum":80.00}]}`, measured: []string{configPackage, modulePath + "/pkg/services/work/internal"}, want: `measured unit service "` + modulePath + `/pkg/services/work" has no root manifest entry`},
		{name: "invalid default floor", manifest: `{"version":1,"lane":"unit","defaultFloorPercent":50.0,"packages":[{"package":"` + configPackage + `","minimum":80.00}]}`, measured: []string{configPackage}, want: "defaultFloorPercent"},
		{name: "unsorted", manifest: `{"version":1,"lane":"unit","packages":[{"package":"` + servicePackage + `","minimum":80.00},{"package":"` + configPackage + `","minimum":80.00}]}`, measured: []string{configPackage, servicePackage}, want: "must be sorted"},
		{name: "invalid percentage", manifest: `{"version":1,"lane":"unit","packages":[{"package":"` + configPackage + `","minimum":100.01}]}`, measured: []string{configPackage}, want: "between 0.00 and 100.00"},
		{name: "imprecise percentage", manifest: `{"version":1,"lane":"unit","packages":[{"package":"` + configPackage + `","minimum":80.0}]}`, measured: []string{configPackage}, want: "exactly two decimal places"},
		{name: "both entry forms", manifest: `{"version":1,"lane":"unit","packages":[{"package":"` + configPackage + `","minimum":80.00,"exception":{"kind":"migration"}}]}`, measured: []string{configPackage}, want: "exactly one"},
		{name: "neither entry form", manifest: `{"version":1,"lane":"unit","packages":[{"package":"` + configPackage + `"}]}`, measured: []string{configPackage}, want: "exactly one"},
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
			// A measured package outside pkg/services with no entry is not a
			// completeness failure: it resolves to the lane default floor.
			unlisted := manifest
			unlisted.Packages = unlisted.Packages[:1]
			if err := validateCoverageManifestAt(unlisted, lane, []string{alpha, beta}, time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC)); err != nil {
				t.Fatalf("unlisted non-service package error = %v, want the %s lane default to apply", err, lane)
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
				!strings.Contains(failures[0], "covered=79/100 statements") ||
				!strings.Contains(failures[0], "-update-manifest minimums.json") {
				t.Fatalf("first regression = %q, want complete actionable diagnostic", failures[0])
			}
			if !strings.Contains(failures[1], "package="+beta+" lane="+lane+" expected-minimum=75.00% actual=74.0000% delta=-1.0000") ||
				!strings.Contains(failures[1], "covered=74/100 statements") {
				t.Fatalf("second regression = %q, want complete diagnostic", failures[1])
			}
			if !slices.IsSorted(failures) {
				t.Fatalf("regression failures are not in stable package order: %v", failures)
			}
		})
	}
}

func TestCheckCoverageManifestReportsOrderedPackageUncoveredBlocks(t *testing.T) {
	t.Parallel()

	manifest, coverageBlocks, importPath := uncoveredBlockManifestFixture()
	failures, warnings := checkCoverageManifestWithEpsilonAndBlocks(
		manifest,
		map[string]packageCoverageTotals{importPath: {coveredStatements: 5, totalStatements: 10}},
		"minimums.json",
		0,
		coverageBlocks,
	)
	if len(warnings) != 0 || len(failures) != 1 {
		t.Fatalf("checkCoverageManifestWithEpsilonAndBlocks() = failures %v, warnings %v; want one failure and no warnings", failures, warnings)
	}

	failure := failures[0]
	wantDetail := "uncovered blocks: pkg/config/a.go:20 (1 statement), pkg/config/a.go:20 (2 statements), pkg/config/z.go:9 (3 statements)"
	if !strings.Contains(failure, wantDetail) {
		t.Fatalf("failure = %q, want ordered uncovered detail %q", failure, wantDetail)
	}
	if strings.Contains(failure, "covered.go") || strings.Contains(failure, "pkg/service/other.go") {
		t.Fatalf("failure = %q, did not expect covered or other-package blocks", failure)
	}
	if !strings.Contains(failure, "restore coverage before running `go run ./cmd/gocoveragecheck") {
		t.Fatalf("failure = %q, want existing remediation wording", failure)
	}
}

func TestCheckCoverageManifestKeepsHeldRegressionOutOfBlockingFailures(t *testing.T) {
	t.Parallel()

	importPath := modulePath + "/pkg/config"
	manifest := coverageManifest{
		Version: coverageManifestVersion,
		Lane:    "unit",
		FloorHolds: []coverageManifestFloorHold{{
			Package:       importPath,
			Justification: "current-main baseline is being restored",
			Owner:         "coverage-remediation",
			Deadline:      "2027-07-15",
			RemovalGate:   "matching unit tests restore the existing floor",
		}},
		Packages: []coverageManifestEntry{{Package: importPath, Minimum: json.RawMessage("80.00")}},
	}

	failures, warnings := checkCoverageManifestWithEpsilon(
		manifest,
		map[string]packageCoverageTotals{importPath: {coveredStatements: 40, totalStatements: 100}},
		"minimums.json",
		0,
	)
	if len(failures) != 0 {
		t.Fatalf("held regression failures = %v, want none", failures)
	}
	if len(warnings) != 1 {
		t.Fatalf("held regression warnings = %v, want one", warnings)
	}
	for _, want := range []string{
		"package coverage hold: package=" + importPath,
		"lane=unit",
		"expected-minimum=80.00%",
		"actual=40.0000%",
		"delta=-40.0000 percentage-points",
		"staged blocking hold",
		"matching-unit-lane coverage",
	} {
		if !strings.Contains(warnings[0], want) {
			t.Fatalf("held regression warning = %q, want %q", warnings[0], want)
		}
	}
	if strings.Contains(warnings[0], "package coverage regression") || strings.Contains(warnings[0], "report-only") {
		t.Fatalf("held regression warning used the wrong policy classification: %q", warnings[0])
	}
}

func TestCheckCoverageManifestCapsUncoveredBlocks(t *testing.T) {
	t.Parallel()

	manifest, _, importPath := uncoveredBlockManifestFixture()
	tooManyBlocks := make(map[string]coverageBlock, maxUncoveredCoverageBlocks+2)
	for index := 0; index < maxUncoveredCoverageBlocks+2; index++ {
		tooManyBlocks[fmt.Sprintf("block-%d", index)] = coverageBlock{
			canonicalPath:  fmt.Sprintf("%s/pkg/config/file-%02d.go", modulePath, index),
			importPath:     importPath,
			rangeSpec:      "1.1,2.1",
			statementCount: 1,
		}
	}
	failures, _ := checkCoverageManifestWithEpsilonAndBlocks(
		manifest,
		map[string]packageCoverageTotals{importPath: {coveredStatements: 1, totalStatements: 2}},
		"minimums.json",
		0,
		tooManyBlocks,
	)
	if len(failures) != 1 || !strings.Contains(failures[0], "... and 2 more") {
		t.Fatalf("capped failures = %v, want exact omitted-block tail", failures)
	}
	if got := strings.Count(failures[0], "(1 statement)"); got != maxUncoveredCoverageBlocks {
		t.Fatalf("capped failure = %q, contains %d block entries; want %d", failures[0], got, maxUncoveredCoverageBlocks)
	}
}

func TestCheckCoverageManifestToleratedDriftOmitsUncoveredBlocks(t *testing.T) {
	t.Parallel()

	manifest, coverageBlocks, importPath := uncoveredBlockManifestFixture()
	manifest.Packages = []coverageManifestEntry{{Package: importPath, Minimum: json.RawMessage("80.00")}}
	failures, warnings := checkCoverageManifestWithEpsilonAndBlocks(
		manifest,
		map[string]packageCoverageTotals{importPath: {coveredStatements: 79, totalStatements: 100}},
		"minimums.json",
		1,
		coverageBlocks,
	)
	if len(failures) != 0 || len(warnings) != 1 {
		t.Fatalf("tolerated result = failures %v, warnings %v; want no failure and one warning", failures, warnings)
	}
	if strings.Contains(warnings[0], "uncovered blocks") {
		t.Fatalf("warning = %q, did not expect uncovered-block detail", warnings[0])
	}
}

func uncoveredBlockManifestFixture() (coverageManifest, map[string]coverageBlock, string) {
	importPath := modulePath + "/pkg/config"
	otherImportPath := modulePath + "/pkg/service"
	manifest := coverageManifest{
		Version: coverageManifestVersion,
		Lane:    "unit",
		Packages: []coverageManifestEntry{
			{Package: importPath, Minimum: json.RawMessage("100.00")},
		},
	}
	coverageBlocks := map[string]coverageBlock{
		"z": {
			canonicalPath:  modulePath + "/pkg/config/z.go",
			importPath:     importPath,
			rangeSpec:      "9.1,10.1",
			statementCount: 3,
		},
		"a-late": {
			canonicalPath:  modulePath + "/pkg/config/a.go",
			importPath:     importPath,
			rangeSpec:      "20.2,21.1",
			statementCount: 2,
		},
		"a-early": {
			canonicalPath:  modulePath + "/pkg/config/a.go",
			importPath:     importPath,
			rangeSpec:      "20.1,20.2",
			statementCount: 1,
		},
		"covered": {
			canonicalPath:  modulePath + "/pkg/config/covered.go",
			importPath:     importPath,
			rangeSpec:      "1.1,2.1",
			statementCount: 4,
			executionCount: 1,
		},
		"other-package": {
			canonicalPath:  modulePath + "/pkg/service/other.go",
			importPath:     otherImportPath,
			rangeSpec:      "2.1,3.1",
			statementCount: 7,
		},
	}
	return manifest, coverageBlocks, importPath
}

func TestValidateCoverageManifestReportsEachUndeclaredServiceOnce(t *testing.T) {
	t.Parallel()

	declaredRoot := modulePath + "/pkg/services/work"
	declaredChild := modulePath + "/pkg/services/work/internal/staging"
	alphaRoot := modulePath + "/pkg/services/alpha"
	zetaRoot := modulePath + "/pkg/services/zeta"
	manifest := coverageManifest{
		Version: coverageManifestVersion,
		Lane:    "functional",
		Packages: []coverageManifestEntry{
			{Package: declaredRoot, Minimum: json.RawMessage("40.00")},
		},
	}
	err := validateCoverageManifestAtWithTotals(
		manifest,
		"functional",
		[]string{
			zetaRoot, zetaRoot + "/internal", zetaRoot + "/transports/http",
			alphaRoot + "/wire", declaredRoot, declaredChild,
		},
		time.Date(2026, 8, 13, 0, 0, 0, 0, time.UTC),
		map[string]packageCoverageTotals{
			zetaRoot: {coveredStatements: 2, totalStatements: 3},
		},
	)
	if err == nil {
		t.Fatal("validateCoverageManifestAtWithTotals() unexpectedly succeeded")
	}

	message := err.Error()
	if !strings.Contains(message, "measured functional services have no root manifest entry") {
		t.Fatalf("missing-root error = %q, want functional lane diagnostic", message)
	}
	if got := strings.Count(message, "has no root manifest entry;"); got != 2 {
		t.Fatalf("missing-root error = %q, want one line per undeclared service, got %d", message, got)
	}
	if strings.Contains(message, declaredRoot+"/") || strings.Contains(message, zetaRoot+"/") {
		t.Fatalf("missing-root error = %q, want services named once rather than per package", message)
	}
	if !strings.Contains(message, `measured functional service "`+alphaRoot+`" has no root manifest entry`) {
		t.Fatalf("missing-root error = %q, want the alpha service named by its root", message)
	}
	if !strings.Contains(message, `measured functional service "`+zetaRoot+`" has no root manifest entry; record one entry for the service root; the root package measures 2/3 statements`) {
		t.Fatalf("missing-root error = %q, want the zeta root measurement", message)
	}
	if strings.Index(message, alphaRoot) > strings.Index(message, zetaRoot) {
		t.Fatalf("undeclared services are not sorted by import path: %q", message)
	}
}

func TestValidateCoverageManifestAcceptsNewPackagesInsideDeclaredService(t *testing.T) {
	t.Parallel()

	declaredRoot := modulePath + "/pkg/services/work"
	manifest := coverageManifest{
		Version: coverageManifestVersion,
		Lane:    "unit",
		Packages: []coverageManifestEntry{
			{Package: declaredRoot, Minimum: json.RawMessage("40.00")},
		},
	}
	measured := []string{declaredRoot, declaredRoot + "/internal/brand_new", declaredRoot + "/transports/cli"}
	if err := validateCoverageManifestAt(manifest, "unit", measured, time.Date(2026, 8, 13, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("validateCoverageManifestAt() error = %v, want new packages inside a declared service to pass completeness", err)
	}

	withoutRoot := manifest
	withoutRoot.Packages = nil
	err := validateCoverageManifestAt(withoutRoot, "unit", measured, time.Date(2026, 8, 13, 0, 0, 0, 0, time.UTC))
	if err == nil || !strings.Contains(err.Error(), `measured unit service "`+declaredRoot+`" has no root manifest entry`) {
		t.Fatalf("validateCoverageManifestAt() error = %v, want the service named after its root entry is removed", err)
	}
}

func TestCheckCoverageManifestAppliesRawStatementEpsilon(t *testing.T) {
	t.Parallel()

	importPath := modulePath + "/pkg/config"
	manifest := coverageManifest{
		Version: coverageManifestVersion,
		Lane:    "unit",
		Packages: []coverageManifestEntry{
			{Package: importPath, Minimum: json.RawMessage("80.00")},
		},
	}
	cases := []struct {
		name             string
		totals           packageCoverageTotals
		epsilon          float64
		wantFailure      bool
		wantWarningDelta string
	}{
		{
			name:             "inside epsilon",
			totals:           packageCoverageTotals{coveredStatements: 319, totalStatements: 400},
			epsilon:          0.25,
			wantWarningDelta: "delta=-0.2500 percentage-points",
		},
		{
			name:        "beyond epsilon",
			totals:      packageCoverageTotals{coveredStatements: 318, totalStatements: 400},
			epsilon:     0.25,
			wantFailure: true,
		},
		{
			name:    "exact floor",
			totals:  packageCoverageTotals{coveredStatements: 4, totalStatements: 5},
			epsilon: 0.25,
		},
		{
			name:        "strict zero",
			totals:      packageCoverageTotals{coveredStatements: 319, totalStatements: 400},
			epsilon:     0,
			wantFailure: true,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			failures, warnings := checkCoverageManifestWithEpsilon(
				manifest,
				map[string]packageCoverageTotals{importPath: tc.totals},
				"minimums.json",
				tc.epsilon,
			)
			if got := len(failures) > 0; got != tc.wantFailure {
				t.Fatalf("failures = %v, want failure=%t", failures, tc.wantFailure)
			}
			if tc.wantWarningDelta == "" {
				if len(warnings) != 0 {
					t.Fatalf("warnings = %v, want none", warnings)
				}
				return
			}
			if len(warnings) != 1 || !strings.Contains(warnings[0], tc.wantWarningDelta) {
				t.Fatalf("warnings = %v, want one warning containing %q", warnings, tc.wantWarningDelta)
			}
			if !strings.Contains(warnings[0], "package="+importPath+" lane=unit expected-minimum=80.00% actual=79.7500%") ||
				!strings.Contains(warnings[0], "epsilon=0.2500 percentage-points") {
				t.Fatalf("warning = %q, want package, lane, floor, actual, and epsilon", warnings[0])
			}
			if strings.Contains(warnings[0], "covered=") {
				t.Fatalf("warning = %q, did not expect exact statement counts", warnings[0])
			}
			if !strings.Contains(warnings[0], "warning") || strings.Contains(warnings[0], "update-manifest") {
				t.Fatalf("warning = %q, did not expect update remediation", warnings[0])
			}
		})
	}
}
