package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
	if !strings.Contains(got, modulePath+"/pkg/config\tcoverage: 100.0% of statements") {
		t.Fatalf("execute() stdout = %q, want package summary for pkg/config", got)
	}
	if !strings.Contains(got, modulePath+"/pkg/service\tcoverage: 100.0% of statements") {
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

func TestExecuteJSONReportsMeasurementExceptionFromManifest(t *testing.T) {
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
	initializerPackage := modulePath + "/pkg/initializer"
	manifestPath := writePackageMinimumManifestWithEntries(t, "unit", []manifestPackageSpec{
		{importPath: configPackage, minimum: "100.00"},
		{
			importPath: initializerPackage,
			exception: &coverageManifestException{
				Kind:          "measurement",
				Justification: "The active unit coverage profile contains no measurable statements for this package.",
				Owner:         "backend-quality",
				Deadline:      unmeasurablePackageDeadline,
				RemovalGate:   "The unit coverage profile reports at least one measurable statement for this package.",
			},
		},
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

	data, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Fatalf("read coverage summary json: %v", err)
	}
	var summary coverageSummaryJSON
	if err := json.Unmarshal(data, &summary); err != nil {
		t.Fatalf("decode coverage summary json: %v\n%s", err, data)
	}
	if len(summary.Packages) != 3 {
		t.Fatalf("packages len = %d, want 3\n%s", len(summary.Packages), data)
	}
	wantOrder := []string{configPackage, initializerPackage, servicePackage}
	for index, wantPackage := range wantOrder {
		if summary.Packages[index].Package != wantPackage {
			t.Fatalf("packages[%d] = %q, want %q", index, summary.Packages[index].Package, wantPackage)
		}
	}

	exceptionEntry := summary.Packages[1]
	if exceptionEntry.CoveredStatements != 0 || exceptionEntry.MeasurableStatements != 0 {
		t.Fatalf("unmeasurable package statements = %d/%d, want 0/0", exceptionEntry.CoveredStatements, exceptionEntry.MeasurableStatements)
	}
	if exceptionEntry.PackageFloor != nil {
		t.Fatalf("unmeasurable packageFloor = %v, want null", *exceptionEntry.PackageFloor)
	}
	if exceptionEntry.MeasurementException == nil {
		t.Fatalf("measurementException is nil, want structured exception\n%s", data)
	}
	gotException := exceptionEntry.MeasurementException
	if gotException.Kind != "measurement" {
		t.Fatalf("exception kind = %q, want measurement", gotException.Kind)
	}
	if gotException.Justification != "The active unit coverage profile contains no measurable statements for this package." {
		t.Fatalf("exception justification = %q", gotException.Justification)
	}
	if gotException.Owner != "backend-quality" {
		t.Fatalf("exception owner = %q, want backend-quality", gotException.Owner)
	}
	if gotException.Deadline != unmeasurablePackageDeadline {
		t.Fatalf("exception deadline = %q, want %q", gotException.Deadline, unmeasurablePackageDeadline)
	}
	if gotException.RemovalGate != "The unit coverage profile reports at least one measurable statement for this package." {
		t.Fatalf("exception removalGate = %q", gotException.RemovalGate)
	}

	if summary.Packages[0].MeasurementException != nil || summary.Packages[2].MeasurementException != nil {
		t.Fatalf("measurable packages unexpectedly carried measurementException: %+v / %+v", summary.Packages[0].MeasurementException, summary.Packages[2].MeasurementException)
	}
	if summary.Packages[0].PackageFloor == nil || *summary.Packages[0].PackageFloor != 100.0 {
		t.Fatalf("pkg/config packageFloor = %v, want 100", summary.Packages[0].PackageFloor)
	}
	if stderr.Len() != 0 {
		t.Fatalf("execute() stderr = %q, want empty stderr", stderr.String())
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

	got := stdout.String()
	if !strings.Contains(got, modulePath+"/pkg/config\tcoverage: 100.0% of statements") {
		t.Fatalf("execute() stdout = %q, want package summary retained on floor failure", got)
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
	commandRunner = fakeGoCoverageCommand
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
		t.Fatalf("execute() stderr = %q, want empty stderr", stderr.String())
	}
}

func TestExecuteDoesNotWriteJSONWhenMeasurementIncomplete(t *testing.T) {
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
	if !strings.Contains(err.Error(), "run go test coverage shard") {
		t.Fatalf("execute() error = %q, want incomplete measurement failure", err.Error())
	}
	if _, statErr := os.Stat(jsonPath); !os.IsNotExist(statErr) {
		t.Fatalf("unexpected json file at %s after incomplete measurement (stat err=%v)", jsonPath, statErr)
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
	if !strings.Contains(got, modulePath+"/pkg/config\tcoverage: 100.0% of statements") {
		t.Fatalf("execute() stdout = %q, want package summary for pkg/config", got)
	}
	if !strings.Contains(got, modulePath+"/pkg/service\tcoverage: 100.0% of statements") {
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
}

func writePackageMinimumManifestWithEntries(t *testing.T, lane string, entries []manifestPackageSpec) string {
	t.Helper()
	manifestPath := filepath.Join(t.TempDir(), lane+"-minimums.json")
	var packageBlocks []string
	for _, entry := range entries {
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
	data := fmt.Sprintf("{\n  \"version\": 1,\n  \"lane\": %q,\n  \"packages\": [\n%s\n  ]\n}\n", lane, strings.Join(packageBlocks, ",\n"))
	if err := os.WriteFile(manifestPath, []byte(data), 0o600); err != nil {
		t.Fatalf("write package minimum manifest: %v", err)
	}
	return manifestPath
}
