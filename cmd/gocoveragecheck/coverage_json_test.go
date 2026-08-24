package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestExecuteWritesDeterministicJSONCoverageTotals(t *testing.T) {
	originalCommandRunner := commandRunner
	originalStdout := stdoutWriter
	originalStderr := stderrWriter
	defer func() {
		commandRunner = originalCommandRunner
		stdoutWriter = originalStdout
		stderrWriter = originalStderr
	}()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	commandRunner = fakeGoCoverageCommandPassing
	stdoutWriter = &stdout
	stderrWriter = &stderr

	jsonPath := filepath.Join(t.TempDir(), "coverage-summary.json")
	cfg := config{
		min: 80,
		coverpkg: strings.Join([]string{
			modulePath + "/pkg/config",
			modulePath + "/pkg/service",
		}, ","),
		packages:        "./pkg/config",
		packageBaseline: emptyPackageCoverageBaselinePath(t),
		jsonOutput:      jsonPath,
	}
	if err := execute(cfg); err != nil {
		t.Fatalf("execute() error = %v", err)
	}
	if err := execute(cfg); err != nil {
		t.Fatalf("second execute() error = %v", err)
	}

	first, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Fatalf("read coverage summary json: %v", err)
	}
	if err := execute(cfg); err != nil {
		t.Fatalf("third execute() error = %v", err)
	}
	second, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Fatalf("read coverage summary json after rerun: %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Fatalf("coverage summary json was not deterministic:\nfirst=%s\nsecond=%s", first, second)
	}

	var summary coverageSummaryJSON
	if err := json.Unmarshal(first, &summary); err != nil {
		t.Fatalf("decode coverage summary json: %v\n%s", err, first)
	}
	if summary.CoveredStatements != 8 {
		t.Fatalf("coveredStatements = %d, want 8", summary.CoveredStatements)
	}
	if summary.MeasurableStatements != 8 {
		t.Fatalf("measurableStatements = %d, want 8", summary.MeasurableStatements)
	}
	if summary.CoveragePercent != 100.0 {
		t.Fatalf("coveragePercent = %v, want 100.0", summary.CoveragePercent)
	}
	if len(summary.Packages) != 2 {
		t.Fatalf("packages len = %d, want 2", len(summary.Packages))
	}

	got := stdout.String()
	if !strings.Contains(got, "Go coverage 100.0% meets minimum 80.0%.") {
		t.Fatalf("execute() stdout = %q, want success message retained with json output", got)
	}
	if stderr.Len() != 0 {
		t.Fatalf("execute() stderr = %q, want empty stderr", stderr.String())
	}
}

func TestCoverageSummaryRetainsFloorPolicyAndDiagnostics(t *testing.T) {
	summary := buildCoverageSummaryJSON(coverageResult{
		packageFloorPolicy: coverageFloorPolicyAdvisory,
		packageMinimumWarnings: []string{
			"package coverage regression: package=pkg/example lane=unit expected-minimum=80.00% actual=40.0000% delta=-40.0000 percentage-points",
		},
		manifestCompletenessWarnings: []string{
			"coverage manifest missing entry: package=pkg/services/example lane=unit",
		},
		unmeasuredPackageDiagnostics: []string{
			"coverage not evaluated: package=pkg/unmeasured lane=unit (no measurement in profile)",
		},
	})

	if summary.PackageFloorPolicy != coverageFloorPolicyAdvisory {
		t.Fatalf("packageFloorPolicy = %q, want %q", summary.PackageFloorPolicy, coverageFloorPolicyAdvisory)
	}
	if len(summary.PackageFloorFindings) != 1 || !strings.Contains(summary.PackageFloorFindings[0], "package=pkg/example") {
		t.Fatalf("packageFloorFindings = %v, want retained floor diagnostic", summary.PackageFloorFindings)
	}
	if len(summary.ManifestDiagnostics) != 2 || !strings.Contains(summary.ManifestDiagnostics[0], "missing entry") || !strings.Contains(summary.ManifestDiagnostics[1], "no measurement") {
		t.Fatalf("manifestDiagnostics = %v, want retained manifest diagnostics", summary.ManifestDiagnostics)
	}
}

func TestDetailedDiagnosticsOnlyChangesJSONFindingPresentation(t *testing.T) {
	floor := 80.0
	detailedFinding := "package coverage regression: package=pkg/example lane=unit expected-minimum=80.00% actual=40.0000% delta=-40.0000 percentage-points covered=2/5 statements; uncovered blocks: pkg/example/file.go:41 (2 statements); restore coverage"
	result := coverageResult{
		actual:                 40,
		packageFloorPolicy:     coverageFloorPolicyBlocking,
		packageMinimumFailures: []string{detailedFinding},
		packageSummaries:       []packageCoverageSummary{{importPath: "pkg/example", coverage: 40}},
		packageTotals: map[string]packageCoverageTotals{
			"pkg/example": {coveredStatements: 2, totalStatements: 5},
		},
		packageGates: map[string]packageCoverageGate{
			"pkg/example": {Floor: &floor},
		},
	}

	compact := buildCoverageSummaryJSON(result)
	detailedResult := result
	detailedResult.detailedDiagnostics = true
	detailed := buildCoverageSummaryJSON(detailedResult)

	wantCompactFinding := "package coverage regression: package=pkg/example lane=unit expected-minimum=80.00% actual=40.0000% delta=-40.0000 percentage-points covered=2/5 statements; restore coverage"
	if compact.PackageFloorFindings[0] != wantCompactFinding {
		t.Fatalf("default package finding = %q, want compact finding %q", compact.PackageFloorFindings[0], wantCompactFinding)
	}
	if detailed.PackageFloorFindings[0] != detailedFinding {
		t.Fatalf("detailed package finding = %q, want full finding %q", detailed.PackageFloorFindings[0], detailedFinding)
	}
	if !reflect.DeepEqual(compact.Packages, detailed.Packages) {
		t.Fatalf("package measurement changed with detailed diagnostics: compact=%+v detailed=%+v", compact.Packages, detailed.Packages)
	}
	if compact.CoveredStatements != detailed.CoveredStatements ||
		compact.MeasurableStatements != detailed.MeasurableStatements ||
		compact.CoveragePercent != detailed.CoveragePercent ||
		compact.PackageFloorPolicy != detailed.PackageFloorPolicy {
		t.Fatalf("coverage summary changed with detailed diagnostics: compact=%+v detailed=%+v", compact, detailed)
	}
}

func TestExecuteOmitsJSONFileWhenJSONOutputOptionAbsent(t *testing.T) {
	originalCommandRunner := commandRunner
	originalStdout := stdoutWriter
	originalStderr := stderrWriter
	defer func() {
		commandRunner = originalCommandRunner
		stdoutWriter = originalStdout
		stderrWriter = originalStderr
	}()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	commandRunner = fakeGoCoverageCommandPassing
	stdoutWriter = &stdout
	stderrWriter = &stderr

	jsonPath := filepath.Join(t.TempDir(), "coverage-summary.json")
	err := execute(config{
		min: 80,
		coverpkg: strings.Join([]string{
			modulePath + "/pkg/config",
			modulePath + "/pkg/service",
		}, ","),
		packages: "./pkg/config",
	})
	if err != nil {
		t.Fatalf("execute() error = %v", err)
	}
	if _, err := os.Stat(jsonPath); !os.IsNotExist(err) {
		t.Fatalf("unexpected json file at %s with err=%v", jsonPath, err)
	}

	got := stdout.String()
	if !strings.Contains(got, "total: (statements) 100.0%") {
		t.Fatalf("execute() stdout = %q, want total coverage line", got)
	}
	if !strings.Contains(got, "package="+modulePath+"/pkg/config coverage=100.0% floor=80.0% delta=+20.0pp status=PASS lane=unit") {
		t.Fatalf("execute() stdout = %q, want package summary for pkg/config", got)
	}
	if !strings.Contains(got, "package="+modulePath+"/pkg/service coverage=100.0% floor=80.0% delta=+20.0pp status=PASS lane=unit") {
		t.Fatalf("execute() stdout = %q, want package summary for pkg/service", got)
	}
	wantSuccess := "Go coverage 100.0% meets minimum 80.0%."
	if !strings.Contains(got, wantSuccess) {
		t.Fatalf("execute() stdout = %q, want success message %q", got, wantSuccess)
	}
	if stderr.Len() != 0 {
		t.Fatalf("execute() stderr = %q, want empty stderr", stderr.String())
	}
}

func TestExecuteJSONReportsPackageFloorsFromManifest(t *testing.T) {
	originalCommandRunner := commandRunner
	originalStdout := stdoutWriter
	originalStderr := stderrWriter
	defer func() {
		commandRunner = originalCommandRunner
		stdoutWriter = originalStdout
		stderrWriter = originalStderr
	}()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	commandRunner = fakeGoCoverageCommandPassing
	stdoutWriter = &stdout
	stderrWriter = &stderr

	configPackage := modulePath + "/pkg/config"
	servicePackage := modulePath + "/pkg/service"
	manifestPath := writePackageMinimumManifestWithEntries(t, "unit", []manifestPackageSpec{
		{importPath: configPackage, minimum: "66.66"},
		{importPath: servicePackage, minimum: "80.00"},
	})
	jsonPath := filepath.Join(t.TempDir(), "coverage-summary.json")

	err := execute(config{
		suite:           "unit",
		min:             80,
		coverpkg:        strings.Join([]string{configPackage, servicePackage}, ","),
		packages:        "./pkg/config",
		packageManifest: manifestPath,
		jsonOutput:      jsonPath,
	})
	if err != nil {
		t.Fatalf("execute() error = %v", err)
	}

	data, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Fatalf("read coverage summary json: %v", err)
	}
	var summary coverageSummaryJSON
	if err := json.Unmarshal(data, &summary); err != nil {
		t.Fatalf("decode coverage summary json: %v\n%s", err, data)
	}
	if len(summary.Packages) != 2 {
		t.Fatalf("packages len = %d, want 2\n%s", len(summary.Packages), data)
	}
	if summary.Packages[0].Package != configPackage || summary.Packages[1].Package != servicePackage {
		t.Fatalf("package order = [%s %s], want deterministic [%s %s]", summary.Packages[0].Package, summary.Packages[1].Package, configPackage, servicePackage)
	}

	configEntry := summary.Packages[0]
	if configEntry.CoveredStatements != 3 || configEntry.MeasurableStatements != 3 {
		t.Fatalf("pkg/config statements = %d/%d, want 3/3", configEntry.CoveredStatements, configEntry.MeasurableStatements)
	}
	if configEntry.CoveragePercent != 100.0 {
		t.Fatalf("pkg/config coveragePercent = %v, want 100.0", configEntry.CoveragePercent)
	}
	if configEntry.PackageFloor == nil || *configEntry.PackageFloor != 66.66 {
		t.Fatalf("pkg/config packageFloor = %v, want 66.66", configEntry.PackageFloor)
	}
	if configEntry.MeasurementException != nil {
		t.Fatalf("pkg/config measurementException = %+v, want nil", configEntry.MeasurementException)
	}

	serviceEntry := summary.Packages[1]
	if serviceEntry.CoveredStatements != 5 || serviceEntry.MeasurableStatements != 5 {
		t.Fatalf("pkg/service statements = %d/%d, want 5/5", serviceEntry.CoveredStatements, serviceEntry.MeasurableStatements)
	}
	if serviceEntry.PackageFloor == nil || *serviceEntry.PackageFloor != 80.0 {
		t.Fatalf("pkg/service packageFloor = %v, want 80", serviceEntry.PackageFloor)
	}
	if serviceEntry.MeasurementException != nil {
		t.Fatalf("pkg/service measurementException = %+v, want nil", serviceEntry.MeasurementException)
	}
	if stderr.Len() != 0 {
		t.Fatalf("execute() stderr = %q, want empty stderr", stderr.String())
	}
}

func TestExecuteJSONReportsHeldFloorWithoutDisablingBlockingPolicy(t *testing.T) {
	_, stderr := stubCoverageExecute(t, fakeGoCoverageCommandWithMeasuredZeroConfig)

	configPackage := modulePath + "/pkg/config"
	manifestPath := writePackageMinimumManifestWithEntries(t, "unit", []manifestPackageSpec{
		{
			importPath: configPackage,
			minimum:    "80.00",
			floorHold: &coverageManifestFloorHold{
				Justification: "The current-main baseline is below the existing unit floor while matching-lane tests are restored.",
				Owner:         "coverage-remediation",
				Deadline:      "2027-07-15",
				RemovalGate:   "Matching unit coverage reaches the existing floor and this hold is removed.",
			},
		},
	})
	jsonPath := filepath.Join(t.TempDir(), "coverage-summary.json")

	err := execute(config{
		suite:           "unit",
		min:             0,
		coverpkg:        configPackage,
		packages:        "./pkg/config",
		packageManifest: manifestPath,
		jsonOutput:      jsonPath,
	})
	if err != nil {
		t.Fatalf("execute() error = %v, want the staged hold to keep the blocking lane green", err)
	}
	if strings.Contains(stderr.String(), "COVERAGE FLOOR POLICY: advisory") || strings.Contains(stderr.String(), "report-only") {
		t.Fatalf("held blocking run emitted advisory policy guidance: %q", stderr.String())
	}
	if !strings.Contains(stderr.String(), "package coverage hold: package="+configPackage) ||
		!strings.Contains(stderr.String(), "expected-minimum=80.00% actual=0.0000% delta=-80.0000 percentage-points") {
		t.Fatalf("held blocking run lost the complete remediation diagnostic: %q", stderr.String())
	}

	summary := readCoverageSummaryJSONFile(t, jsonPath)
	if summary.PackageFloorPolicy != coverageFloorPolicyBlocking {
		t.Fatalf("packageFloorPolicy = %q, want blocking", summary.PackageFloorPolicy)
	}
	if len(summary.Packages) != 1 || !summary.Packages[0].PackageFloorHeld {
		t.Fatalf("package floor hold = %+v, want one held package", summary.Packages)
	}
}

func TestExecuteJSONReportsMeasurementExceptionFromManifest(t *testing.T) {
	_, stderr := stubCoverageExecute(t, fakeGoCoverageCommandPassing)

	configPackage := modulePath + "/pkg/config"
	servicePackage := modulePath + "/pkg/service"
	initializerPackage := modulePath + "/pkg/initializer"
	wantException := unitUnmeasurableMeasurementException()
	manifestPath := writePackageMinimumManifestWithEntries(t, "unit", []manifestPackageSpec{
		{importPath: configPackage, minimum: "100.00"},
		{importPath: initializerPackage, exception: wantException},
		{importPath: servicePackage, minimum: "100.00"},
	})
	jsonPath := filepath.Join(t.TempDir(), "coverage-summary.json")

	err := execute(config{
		suite: "unit",
		min:   80,
		coverpkg: strings.Join([]string{
			configPackage,
			initializerPackage,
			servicePackage,
		}, ","),
		packages:        "./pkg/config",
		packageManifest: manifestPath,
		jsonOutput:      jsonPath,
	})
	if err != nil {
		t.Fatalf("execute() error = %v", err)
	}

	summary := readCoverageSummaryJSONFile(t, jsonPath)
	if len(summary.Packages) != 3 {
		t.Fatalf("packages len = %d, want 3", len(summary.Packages))
	}
	wantOrder := []string{configPackage, initializerPackage, servicePackage}
	for index, wantPackage := range wantOrder {
		if summary.Packages[index].Package != wantPackage {
			t.Fatalf("packages[%d] = %q, want %q", index, summary.Packages[index].Package, wantPackage)
		}
	}

	assertUnmeasurablePackageJSON(t, summary.Packages[1], wantException)
	if summary.Packages[0].MeasurementException != nil || summary.Packages[2].MeasurementException != nil {
		t.Fatalf("measurable packages unexpectedly carried measurementException: %+v / %+v", summary.Packages[0].MeasurementException, summary.Packages[2].MeasurementException)
	}
	if summary.Packages[0].PackageFloor == nil || *summary.Packages[0].PackageFloor != 100.0 {
		t.Fatalf("pkg/config packageFloor = %v, want 100", summary.Packages[0].PackageFloor)
	}
	wantDiagnostic := "coverage not evaluated: package=" + initializerPackage + " lane=unit (no measurement in profile)\n"
	if got := stderr.String(); got != wantDiagnostic {
		t.Fatalf("execute() stderr = %q, want unmeasured-package diagnostic %q", got, wantDiagnostic)
	}
}

func TestExecuteWritesJSONWhenOverallFloorFails(t *testing.T) {
	originalCommandRunner := commandRunner
	originalStdout := stdoutWriter
	originalStderr := stderrWriter
	defer func() {
		commandRunner = originalCommandRunner
		stdoutWriter = originalStdout
		stderrWriter = originalStderr
	}()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	commandRunner = fakeGoCoverageCommandPassing
	stdoutWriter = &stdout
	stderrWriter = &stderr

	jsonPath := filepath.Join(t.TempDir(), "coverage-summary.json")
	err := execute(config{
		min: 100.1,
		coverpkg: strings.Join([]string{
			modulePath + "/pkg/config",
			modulePath + "/pkg/service",
		}, ","),
		packages:        "./pkg/config",
		packageBaseline: emptyPackageCoverageBaselinePath(t),
		jsonOutput:      jsonPath,
	})
	if err == nil {
		t.Fatal("execute() unexpectedly succeeded")
	}
	wantFailure := "go coverage 100.0% is below minimum 100.1%"
	if err.Error() != wantFailure {
		t.Fatalf("execute() error = %q, want %q", err.Error(), wantFailure)
	}

	data, readErr := os.ReadFile(jsonPath)
	if readErr != nil {
		t.Fatalf("read coverage summary json after floor failure: %v", readErr)
	}
	var summary coverageSummaryJSON
	if decodeErr := json.Unmarshal(data, &summary); decodeErr != nil {
		t.Fatalf("decode coverage summary json: %v\n%s", decodeErr, data)
	}
	if summary.CoveredStatements != 8 || summary.MeasurableStatements != 8 {
		t.Fatalf("overall statements = %d/%d, want 8/8", summary.CoveredStatements, summary.MeasurableStatements)
	}
	if summary.CoveragePercent != 100.0 {
		t.Fatalf("coveragePercent = %v, want 100.0", summary.CoveragePercent)
	}
	if len(summary.Packages) != 2 {
		t.Fatalf("packages len = %d, want 2", len(summary.Packages))
	}

	// The complete per-package set is asserted on the JSON summary above. With
	// that artifact written, stdout carries the collapsed lane verdict rather
	// than one raw coverage line per measured package.
	got := stdout.String()
	if !strings.Contains(got, "Unit package coverage verdict:") {
		t.Fatalf("execute() stdout = %q, want the lane verdict retained on floor failure", got)
	}
	if !strings.Contains(got, "tally: measured-packages=2 gated-packages=2 below-floor=0 near-floor=0 gate-failures=1") {
		t.Fatalf("execute() stdout = %q, want the verdict tally to count the gate failure", got)
	}
	if strings.Contains(got, "meets minimum") {
		t.Fatalf("execute() stdout = %q, did not expect success message", got)
	}
	if stderr.Len() != 0 {
		t.Fatalf("execute() stderr = %q, want empty stderr", stderr.String())
	}
}

func TestExecuteWritesJSONWhenPackageFloorFails(t *testing.T) {
	originalCommandRunner := commandRunner
	originalStdout := stdoutWriter
	originalStderr := stderrWriter
	defer func() {
		commandRunner = originalCommandRunner
		stdoutWriter = originalStdout
		stderrWriter = originalStderr
	}()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	commandRunner = fakeGoCoverageCommandWithMeasuredZeroConfig
	stdoutWriter = &stdout
	stderrWriter = &stderr

	configPackage := modulePath + "/pkg/config"
	manifestPath := writePackageMinimumManifestWithEntries(t, "unit", []manifestPackageSpec{
		{importPath: configPackage, minimum: "80.00"},
	})
	jsonPath := filepath.Join(t.TempDir(), "coverage-summary.json")

	err := execute(config{
		suite:           "unit",
		min:             0,
		coverpkg:        configPackage,
		packages:        "./pkg/config",
		packageManifest: manifestPath,
		jsonOutput:      jsonPath,
	})
	if err == nil {
		t.Fatal("execute() unexpectedly succeeded")
	}
	if !strings.Contains(err.Error(), "package coverage regression: package="+configPackage+" lane=unit expected-minimum=80.00%") {
		t.Fatalf("execute() error = %q, want package floor failure", err.Error())
	}

	data, readErr := os.ReadFile(jsonPath)
	if readErr != nil {
		t.Fatalf("read coverage summary json after package floor failure: %v", readErr)
	}
	var summary coverageSummaryJSON
	if decodeErr := json.Unmarshal(data, &summary); decodeErr != nil {
		t.Fatalf("decode coverage summary json: %v\n%s", decodeErr, data)
	}
	if len(summary.Packages) != 1 {
		t.Fatalf("packages len = %d, want 1\n%s", len(summary.Packages), data)
	}
	entry := summary.Packages[0]
	if entry.Package != configPackage {
		t.Fatalf("package = %q, want %q", entry.Package, configPackage)
	}
	if entry.PackageFloor == nil || *entry.PackageFloor != 80.0 {
		t.Fatalf("packageFloor = %v, want 80", entry.PackageFloor)
	}
	if entry.CoveragePercent != 0.0 {
		t.Fatalf("coveragePercent = %v, want 0.0", entry.CoveragePercent)
	}
	if strings.Contains(stdout.String(), "meets minimum") {
		t.Fatalf("execute() stdout = %q, did not expect success message", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("execute() stderr = %q, want empty stderr for a measured package", stderr.String())
	}
}

func TestExecuteWritesIncompleteJSONWhenMeasurementFailsWithProfile(t *testing.T) {
	originalCommandRunner := commandRunner
	originalStdout := stdoutWriter
	originalStderr := stderrWriter
	defer func() {
		commandRunner = originalCommandRunner
		stdoutWriter = originalStdout
		stderrWriter = originalStderr
	}()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	commandRunner = fakeGoCoverageCommandTestFailsWithoutDetail
	stdoutWriter = &stdout
	stderrWriter = &stderr

	jsonPath := filepath.Join(t.TempDir(), "coverage-summary.json")
	err := execute(config{
		min: 80,
		coverpkg: strings.Join([]string{
			modulePath + "/pkg/config",
			modulePath + "/pkg/service",
		}, ","),
		packages:   "./pkg/config",
		profile:    filepath.Join(t.TempDir(), "coverage.out"),
		jsonOutput: jsonPath,
	})
	if err == nil {
		t.Fatal("execute() unexpectedly succeeded")
	}
	if !strings.Contains(err.Error(), "run go test coverage lane") {
		t.Fatalf("execute() error = %q, want incomplete measurement failure", err.Error())
	}
	data, readErr := os.ReadFile(jsonPath)
	if readErr != nil {
		t.Fatalf("read incomplete coverage summary json: %v", readErr)
	}
	var summary coverageSummaryJSON
	if decodeErr := json.Unmarshal(data, &summary); decodeErr != nil {
		t.Fatalf("decode incomplete coverage summary json: %v\n%s", decodeErr, data)
	}
	if summary.Complete {
		t.Fatal("incomplete coverage summary marked complete")
	}
	if len(summary.Packages) == 0 {
		t.Fatalf("incomplete coverage summary packages = %+v, want retained partial package totals", summary.Packages)
	}
	if len(summary.Packages) != 2 {
		t.Fatalf("incomplete coverage summary packages = %d, want both measured packages", len(summary.Packages))
	}
	for _, entry := range summary.Packages {
		if entry.PackageFloor != nil {
			t.Fatalf("partial package %s has floor %v, want n/a until manifest evaluation", entry.Package, *entry.PackageFloor)
		}
	}
	if summary.Packages[0].Package != modulePath+"/pkg/config" || summary.Packages[0].CoveragePercent != 100.0 {
		t.Fatalf("partial config package = %+v, want measured 100.0%%", summary.Packages[0])
	}
	if summary.Packages[1].Package != modulePath+"/pkg/service" || summary.Packages[1].CoveragePercent != 100.0 {
		t.Fatalf("partial service package = %+v, want measured 100.0%%", summary.Packages[1])
	}
	if !strings.Contains(summary.MeasurementReason, "did not complete") {
		t.Fatalf("measurement reason = %q, want incomplete-measurement explanation", summary.MeasurementReason)
	}
}

func TestExecuteFailedTestsDoNotEvaluatePackageFloors(t *testing.T) {
	stdout, stderr := stubCoverageExecute(t, fakeGoCoverageCommandTestFailsWithObservedFailures)
	configPackage := modulePath + "/pkg/config"
	manifestPath := writePackageMinimumManifest(t, "unit", configPackage, "80.00")
	outputDir := t.TempDir()
	jsonPath := filepath.Join(outputDir, "coverage-summary.json")

	err := execute(config{
		suite:              "unit",
		min:                0,
		packageFloorPolicy: coverageFloorPolicyBlocking,
		packageManifest:    manifestPath,
		coverpkg:           configPackage,
		packages:           "./pkg/config",
		profile:            filepath.Join(outputDir, "coverage.out"),
		jsonOutput:         jsonPath,
	})
	if err == nil {
		t.Fatal("execute() unexpectedly succeeded for failed coverage tests")
	}
	if strings.Contains(err.Error(), "package coverage regression") {
		t.Fatalf("execute() reclassified failed tests as a floor regression: %v", err)
	}
	wantDiagnostic := "coverage not evaluated: 2 failed tests observed; package floors were NOT checked because the coverage test run failed"
	if got := stderr.String(); !strings.Contains(got, wantDiagnostic) {
		t.Fatalf("execute() stderr = %q, want failed-test diagnostic containing %q", got, wantDiagnostic)
	}
	if strings.Contains(stdout.String(), "meets minimum") {
		t.Fatalf("execute() stdout = %q, did not expect aggregate success", stdout.String())
	}

	summary := readCoverageSummaryJSONFile(t, jsonPath)
	if summary.Complete {
		t.Fatal("failed-test coverage summary marked complete")
	}
	if summary.PackageFloorPolicy != coverageFloorPolicyBlocking {
		t.Fatalf("packageFloorPolicy = %q, want active blocking policy", summary.PackageFloorPolicy)
	}
	if len(summary.PackageFloorFindings) != 0 {
		t.Fatalf("packageFloorFindings = %v, want floors not evaluated", summary.PackageFloorFindings)
	}
	if len(summary.ManifestDiagnostics) != 0 {
		t.Fatalf("manifestDiagnostics = %v, want no manifest findings from an incomplete run", summary.ManifestDiagnostics)
	}
}

func TestExecuteIncompleteJSONRetainsAdvisoryPolicy(t *testing.T) {
	stubCoverageExecute(t, fakeGoCoverageCommandTestFailsWithoutDetail)
	outputDir := t.TempDir()
	jsonPath := filepath.Join(outputDir, "coverage-summary.json")

	err := execute(config{
		suite:              "unit",
		packageFloorPolicy: coverageFloorPolicyAdvisory,
		coverpkg:           modulePath + "/pkg/config",
		packages:           "./pkg/config",
		profile:            filepath.Join(outputDir, "coverage.out"),
		jsonOutput:         jsonPath,
	})
	if err == nil {
		t.Fatal("execute() unexpectedly succeeded for failed coverage tests")
	}

	summary := readCoverageSummaryJSONFile(t, jsonPath)
	if summary.PackageFloorPolicy != coverageFloorPolicyAdvisory {
		t.Fatalf("packageFloorPolicy = %q, want active advisory policy", summary.PackageFloorPolicy)
	}
	if len(summary.PackageFloorFindings) != 0 {
		t.Fatalf("packageFloorFindings = %v, want floors not evaluated", summary.PackageFloorFindings)
	}
}

func TestExecuteFloorFailureWithoutJSONOptionKeepsHumanDiagnostics(t *testing.T) {
	originalCommandRunner := commandRunner
	originalStdout := stdoutWriter
	originalStderr := stderrWriter
	defer func() {
		commandRunner = originalCommandRunner
		stdoutWriter = originalStdout
		stderrWriter = originalStderr
	}()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	commandRunner = fakeGoCoverageCommandPassing
	stdoutWriter = &stdout
	stderrWriter = &stderr

	jsonPath := filepath.Join(t.TempDir(), "coverage-summary.json")
	err := execute(config{
		min: 100.1,
		coverpkg: strings.Join([]string{
			modulePath + "/pkg/config",
			modulePath + "/pkg/service",
		}, ","),
		packages: "./pkg/config",
	})
	if err == nil {
		t.Fatal("execute() unexpectedly succeeded")
	}
	wantFailure := "go coverage 100.0% is below minimum 100.1%"
	if err.Error() != wantFailure {
		t.Fatalf("execute() error = %q, want %q", err.Error(), wantFailure)
	}
	if _, statErr := os.Stat(jsonPath); !os.IsNotExist(statErr) {
		t.Fatalf("unexpected json file at %s when json option absent (stat err=%v)", jsonPath, statErr)
	}

	got := stdout.String()
	if !strings.Contains(got, "total: (statements) 100.0%") {
		t.Fatalf("execute() stdout = %q, want total coverage line", got)
	}
	if !strings.Contains(got, "package="+modulePath+"/pkg/config coverage=100.0% floor=80.0% delta=+20.0pp status=PASS lane=unit") {
		t.Fatalf("execute() stdout = %q, want package summary for pkg/config", got)
	}
	if !strings.Contains(got, "package="+modulePath+"/pkg/service coverage=100.0% floor=80.0% delta=+20.0pp status=PASS lane=unit") {
		t.Fatalf("execute() stdout = %q, want package summary for pkg/service", got)
	}
	if strings.Contains(got, "meets minimum") {
		t.Fatalf("execute() stdout = %q, did not expect success message", got)
	}
	if stderr.Len() != 0 {
		t.Fatalf("execute() stderr = %q, want empty stderr", stderr.String())
	}
}

func TestBuildCoverageSummaryJSONUsesMeasuredTotalsAndPackageGates(t *testing.T) {
	t.Parallel()

	configPackage := modulePath + "/pkg/config"
	servicePackage := modulePath + "/pkg/service"
	floor := 66.66
	result := coverageResult{
		actual: 75,
		packageTotals: map[string]packageCoverageTotals{
			configPackage:  {coveredStatements: 3, totalStatements: 4},
			servicePackage: {coveredStatements: 0, totalStatements: 0},
		},
		packageSummaries: []packageCoverageSummary{
			{importPath: configPackage, coverage: 75},
			{importPath: servicePackage, coverage: 0},
		},
		packageGates: map[string]packageCoverageGate{
			configPackage: {Floor: &floor},
			servicePackage: {
				Exception: &coverageManifestException{
					Kind:          "measurement",
					Justification: "The active unit coverage profile contains no measurable statements for this package.",
					Owner:         "backend-quality",
					Deadline:      unmeasurablePackageDeadline,
					RemovalGate:   "The unit coverage profile reports at least one measurable statement for this package.",
				},
			},
		},
	}

	summary := buildCoverageSummaryJSON(result)
	if summary.CoveredStatements != 3 {
		t.Fatalf("coveredStatements = %d, want 3", summary.CoveredStatements)
	}
	if summary.MeasurableStatements != 4 {
		t.Fatalf("measurableStatements = %d, want 4", summary.MeasurableStatements)
	}
	if summary.CoveragePercent != 75.0 {
		t.Fatalf("coveragePercent = %v, want 75.0", summary.CoveragePercent)
	}
	if len(summary.Packages) != 2 {
		t.Fatalf("packages len = %d, want 2", len(summary.Packages))
	}
	if summary.Packages[0].PackageFloor == nil || *summary.Packages[0].PackageFloor != 66.66 {
		t.Fatalf("packages[0].packageFloor = %v, want 66.66", summary.Packages[0].PackageFloor)
	}
	if summary.Packages[0].MeasurementException != nil {
		t.Fatalf("packages[0].measurementException = %+v, want nil", summary.Packages[0].MeasurementException)
	}
	if summary.Packages[1].PackageFloor != nil {
		t.Fatalf("packages[1].packageFloor = %v, want null", *summary.Packages[1].PackageFloor)
	}
	if summary.Packages[1].MeasurementException == nil || summary.Packages[1].MeasurementException.Kind != "measurement" {
		t.Fatalf("packages[1].measurementException = %+v, want measurement exception", summary.Packages[1].MeasurementException)
	}

	data, err := renderCoverageSummaryJSON(summary)
	if err != nil {
		t.Fatalf("renderCoverageSummaryJSON() error = %v", err)
	}
	second, err := renderCoverageSummaryJSON(summary)
	if err != nil {
		t.Fatalf("renderCoverageSummaryJSON() second error = %v", err)
	}
	if !bytes.Equal(data, second) {
		t.Fatalf("package coverage json was not deterministic:\nfirst=%s\nsecond=%s", data, second)
	}
}

type manifestPackageSpec struct {
	importPath string
	minimum    string
	exception  *coverageManifestException
	floorHold  *coverageManifestFloorHold
}

func stubCoverageExecute(t *testing.T, runner commandRunnerFunc) (stdout *bytes.Buffer, stderr *bytes.Buffer) {
	t.Helper()
	originalCommandRunner := commandRunner
	originalStdout := stdoutWriter
	originalStderr := stderrWriter
	t.Cleanup(func() {
		commandRunner = originalCommandRunner
		stdoutWriter = originalStdout
		stderrWriter = originalStderr
	})
	stdout = &bytes.Buffer{}
	stderr = &bytes.Buffer{}
	commandRunner = runner
	stdoutWriter = stdout
	stderrWriter = stderr
	return stdout, stderr
}

func readCoverageSummaryJSONFile(t *testing.T, path string) coverageSummaryJSON {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read coverage summary json: %v", err)
	}
	var summary coverageSummaryJSON
	if err := json.Unmarshal(data, &summary); err != nil {
		t.Fatalf("decode coverage summary json: %v\n%s", err, data)
	}
	return summary
}

func unitUnmeasurableMeasurementException() *coverageManifestException {
	return &coverageManifestException{
		Kind:          "measurement",
		Justification: "The active unit coverage profile contains no measurable statements for this package.",
		Owner:         "backend-quality",
		Deadline:      unmeasurablePackageDeadline,
		RemovalGate:   "The unit coverage profile reports at least one measurable statement for this package.",
	}
}

func assertUnmeasurablePackageJSON(t *testing.T, entry packageCoverageJSON, want *coverageManifestException) {
	t.Helper()
	if entry.CoveredStatements != 0 || entry.MeasurableStatements != 0 {
		t.Fatalf("unmeasurable package statements = %d/%d, want 0/0", entry.CoveredStatements, entry.MeasurableStatements)
	}
	if entry.PackageFloor != nil {
		t.Fatalf("unmeasurable packageFloor = %v, want null", *entry.PackageFloor)
	}
	if entry.MeasurementException == nil {
		t.Fatal("measurementException is nil, want structured exception")
	}
	got := entry.MeasurementException
	if got.Kind != want.Kind || got.Justification != want.Justification || got.Owner != want.Owner || got.Deadline != want.Deadline || got.RemovalGate != want.RemovalGate {
		t.Fatalf("measurementException = %+v, want %+v", got, want)
	}
}

func writePackageMinimumManifestWithEntries(t *testing.T, lane string, entries []manifestPackageSpec) string {
	t.Helper()
	manifestPath := filepath.Join(t.TempDir(), lane+"-minimums.json")
	var packageBlocks []string
	var floorHoldBlocks []string
	for _, entry := range entries {
		if entry.floorHold != nil {
			floorHoldBlocks = append(floorHoldBlocks, fmt.Sprintf(`    {
      "package": %q,
      "justification": %q,
      "owner": %q,
      "deadline": %q,
      "removalGate": %q
    }`, entry.importPath, entry.floorHold.Justification, entry.floorHold.Owner, entry.floorHold.Deadline, entry.floorHold.RemovalGate))
		}
		if entry.exception != nil {
			packageBlocks = append(packageBlocks, fmt.Sprintf(`    {
      "package": %q,
      "exception": {
        "kind": %q,
        "justification": %q,
        "owner": %q,
        "deadline": %q,
        "removalGate": %q
      }
    }`, entry.importPath, entry.exception.Kind, entry.exception.Justification, entry.exception.Owner, entry.exception.Deadline, entry.exception.RemovalGate))
			continue
		}
		packageBlocks = append(packageBlocks, fmt.Sprintf(`    {
      "package": %q,
      "minimum": %s
    }`, entry.importPath, entry.minimum))
	}
	floorHolds := ""
	if len(floorHoldBlocks) > 0 {
		floorHolds = fmt.Sprintf("\n  \"floorHolds\": [\n%s\n  ],", strings.Join(floorHoldBlocks, ",\n"))
	}
	data := fmt.Sprintf("{\n  \"version\": 1,\n  \"lane\": %q,%s\n  \"packages\": [\n%s\n  ]\n}\n", lane, floorHolds, strings.Join(packageBlocks, ",\n"))
	if err := os.WriteFile(manifestPath, []byte(data), 0o600); err != nil {
		t.Fatalf("write package minimum manifest: %v", err)
	}
	return manifestPath
}
