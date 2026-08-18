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

var functionalVerdictCoverPackages = []string{
	modulePath + "/pkg/config",
	modulePath + "/pkg/service",
	modulePath + "/pkg/wire",
}

// fakeFunctionalCoverageCommand writes a profile with one below-floor package,
// one near-floor package, and one comfortably passing package so the ordered
// verdict block has something to order.
func fakeFunctionalCoverageCommand(invocation commandInvocation) (string, string, error) {
	if invocation.name != "go" || len(invocation.args) == 0 || invocation.args[0] != "test" {
		return "", "", fmt.Errorf("unexpected command %q %v", invocation.name, invocation.args)
	}
	profilePath := helperCoverProfilePath(invocation.args[1:])
	if profilePath == "" {
		return "", "", fmt.Errorf("missing -coverprofile")
	}
	if err := writeFakeCoverageProfile(profilePath, strings.Join([]string{
		"mode: count",
		modulePath + "/pkg/config/config.go:1.1,2.1 1 1",
		modulePath + "/pkg/config/config.go:11.1,12.1 2 0",
		modulePath + "/pkg/config/load.go:21.1,22.1 1 0",
		modulePath + "/pkg/service/factory.go:1.1,2.1 9 1",
		modulePath + "/pkg/service/factory.go:31.1,32.1 1 0",
		modulePath + "/pkg/wire/wire.go:1.1,2.1 10 3",
		"",
	}, "\n")); err != nil {
		return "", "", err
	}
	// go test reports the per-package coverage percentage on its own line. The
	// functional lane must not forward it once per package.
	return "coverage: 71.4% of statements in " + strings.Join(functionalVerdictCoverPackages, ", ") + "\n", "", nil
}

func functionalVerdictManifest(t *testing.T, configMinimum string) string {
	t.Helper()
	return writePackageMinimumManifestWithEntries(t, "functional", []manifestPackageSpec{
		{importPath: modulePath + "/pkg/config", minimum: configMinimum},
		{importPath: modulePath + "/pkg/service", minimum: "89.00"},
		{importPath: modulePath + "/pkg/wire", minimum: "50.00"},
	})
}

func functionalVerdictConfig(manifestPath string, jsonPath string) config {
	return config{
		min:             0,
		suite:           "functional",
		coverpkg:        strings.Join(functionalVerdictCoverPackages, ","),
		packages:        "./tests/functional/...",
		packageManifest: manifestPath,
		jsonOutput:      jsonPath,
	}
}

func captureFunctionalVerdictRun(t *testing.T, cfg config) (string, string, error) {
	t.Helper()
	originalCommandRunner := commandRunner
	originalStdout := stdoutWriter
	originalStderr := stderrWriter
	t.Cleanup(func() {
		commandRunner = originalCommandRunner
		stdoutWriter = originalStdout
		stderrWriter = originalStderr
	})

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	commandRunner = fakeFunctionalCoverageCommand
	stdoutWriter = &stdout
	stderrWriter = &stderr

	err := execute(cfg)
	return stdout.String(), stderr.String(), err
}

func TestFunctionalCoverageVerdictOrdersViolationsBeforeNearFloorAndTally(t *testing.T) {
	jsonPath := filepath.Join(t.TempDir(), "coverage-summary.json")
	// pkg/config measures 25.0% against an 80.00 floor: a floor violation.
	cfg := functionalVerdictConfig(functionalVerdictManifest(t, "80.00"), jsonPath)

	stdout, _, err := captureFunctionalVerdictRun(t, cfg)
	if err == nil {
		t.Fatalf("execute() error = nil, want a non-zero floor-violation outcome; stdout=%s", stdout)
	}
	if !strings.Contains(err.Error(), "package coverage regression: package="+modulePath+"/pkg/config") {
		t.Fatalf("execute() error = %v, want the pkg/config floor regression", err)
	}

	violation := strings.Index(stdout, "  floor violation: package="+modulePath+"/pkg/config ")
	nearFloor := strings.Index(stdout, "  near floor: package="+modulePath+"/pkg/service ")
	tally := strings.Index(stdout, "  tally: ")
	if violation < 0 || nearFloor < 0 || tally < 0 {
		t.Fatalf("verdict block missing sections (violation=%d nearFloor=%d tally=%d):\n%s", violation, nearFloor, tally, stdout)
	}
	if !(violation < nearFloor && nearFloor < tally) {
		t.Fatalf("verdict block order = violation@%d nearFloor@%d tally@%d, want violation first and tally last:\n%s", violation, nearFloor, tally, stdout)
	}
	if !strings.Contains(stdout, "delta=-55.0000 percentage-points covered=1/4 statements uncovered-blocks=2") {
		t.Fatalf("floor violation line missing floor/actual/delta/blocks detail:\n%s", stdout)
	}
	if !strings.Contains(stdout, "  tally: measured-packages=3 gated-packages=3 below-floor=1 near-floor=1 gate-failures=1\n") {
		t.Fatalf("verdict tally line missing or wrong:\n%s", stdout)
	}
	// A comfortably passing package is neither a violation nor near its floor.
	if strings.Contains(stdout, modulePath+"/pkg/wire") {
		t.Fatalf("verdict block named a package with ample headroom:\n%s", stdout)
	}
}

func TestFunctionalCoverageRunSuppressesPerPackageCoverageLines(t *testing.T) {
	jsonPath := filepath.Join(t.TempDir(), "coverage-summary.json")
	cfg := functionalVerdictConfig(functionalVerdictManifest(t, "80.00"), jsonPath)

	stdout, _, err := captureFunctionalVerdictRun(t, cfg)
	if err == nil {
		t.Fatalf("execute() error = nil, want a non-zero floor-violation outcome")
	}
	for _, coverPackage := range functionalVerdictCoverPackages {
		if raw := coverPackage + "\tcoverage: "; strings.Contains(stdout, raw) {
			t.Fatalf("functional stdout still streams the raw per-package line %q:\n%s", raw, stdout)
		}
	}
	if strings.Contains(stdout, "coverage: 71.4% of statements") {
		t.Fatalf("functional stdout still streams the go test per-package coverage line:\n%s", stdout)
	}

	// Suppression is log-only: every measured package still reaches the JSON
	// artifact the CI job uploads and renders.
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Fatalf("read coverage summary json: %v", err)
	}
	var summary coverageSummaryJSON
	if err := json.Unmarshal(data, &summary); err != nil {
		t.Fatalf("decode coverage summary json: %v\n%s", err, data)
	}
	if len(summary.Packages) != len(functionalVerdictCoverPackages) {
		t.Fatalf("coverage summary packages = %d, want %d: %s", len(summary.Packages), len(functionalVerdictCoverPackages), data)
	}
	for _, coverPackage := range functionalVerdictCoverPackages {
		found := false
		for _, entry := range summary.Packages {
			if entry.Package == coverPackage {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("coverage summary json dropped package %q: %s", coverPackage, data)
		}
	}
}

func TestFunctionalCoverageVerdictPassesWhenEveryPackageMeetsItsFloor(t *testing.T) {
	jsonPath := filepath.Join(t.TempDir(), "coverage-summary.json")
	// pkg/config measures 25.0%; a 20.00 floor clears it.
	cfg := functionalVerdictConfig(functionalVerdictManifest(t, "20.00"), jsonPath)

	stdout, _, err := captureFunctionalVerdictRun(t, cfg)
	if err != nil {
		t.Fatalf("execute() error = %v, want a zero-exit outcome; stdout=%s", err, stdout)
	}
	if !strings.Contains(stdout, "  floor violations: none\n") {
		t.Fatalf("verdict block missing the explicit no-violation line:\n%s", stdout)
	}
	if !strings.Contains(stdout, "  tally: measured-packages=3 gated-packages=3 below-floor=0 near-floor=1 gate-failures=0\n") {
		t.Fatalf("passing verdict tally line missing or wrong:\n%s", stdout)
	}
	if !strings.Contains(stdout, "Go coverage") || !strings.Contains(stdout, "meets minimum") {
		t.Fatalf("passing run lost its success message:\n%s", stdout)
	}
}

// TestUnitCoverageRunKeepsRawPerPackageCoverageLines pins the invocation that
// publishes no coverage-summary artifact. Its stdout listing is the only copy
// of the per-package measurement, so it is never collapsed.
func TestUnitCoverageRunKeepsRawPerPackageCoverageLines(t *testing.T) {
	originalCommandRunner := commandRunner
	originalStdout := stdoutWriter
	originalStderr := stderrWriter
	defer func() {
		commandRunner = originalCommandRunner
		stdoutWriter = originalStdout
		stderrWriter = originalStderr
	}()

	configPackage := modulePath + "/pkg/config"
	manifestPath := writePackageMinimumManifest(t, "unit", configPackage, "100.00")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	commandRunner = fakeGoCoverageCommandPassing
	stdoutWriter = &stdout
	stderrWriter = &stderr

	if err := execute(config{
		min:             0,
		suite:           "unit",
		coverpkg:        configPackage,
		packages:        "./pkg/config",
		packageManifest: manifestPath,
	}); err != nil {
		t.Fatalf("execute() error = %v", err)
	}

	got := stdout.String()
	if !strings.Contains(got, configPackage+"\tcoverage: 100.0% of statements\n") {
		t.Fatalf("unit lane lost its raw per-package coverage line:\n%s", got)
	}
	if strings.Contains(got, "package coverage verdict:") {
		t.Fatalf("unit lane collapsed its only copy of the measurement into a verdict block:\n%s", got)
	}
}

func TestNearFloorCoverageReportStatesOmittedPackages(t *testing.T) {
	originalStdout := stdoutWriter
	defer func() { stdoutWriter = originalStdout }()
	var stdout bytes.Buffer
	stdoutWriter = &stdout

	floor := 50.0
	result := coverageResult{
		packageGates:  make(map[string]packageCoverageGate),
		packageTotals: make(map[string]packageCoverageTotals),
	}
	for index := range nearFloorCoverageReportLimit + 3 {
		importPath := fmt.Sprintf("%s/pkg/near%02d", modulePath, index)
		result.packageSummaries = append(result.packageSummaries, packageCoverageSummary{importPath: importPath, coverage: 51})
		result.packageGates[importPath] = packageCoverageGate{Floor: &floor}
		result.packageTotals[importPath] = packageCoverageTotals{coveredStatements: 51, totalStatements: 100}
	}

	writeCoverageVerdict("Functional", result, nil)

	got := stdout.String()
	if count := strings.Count(got, "  near floor: package="); count != nearFloorCoverageReportLimit {
		t.Fatalf("near-floor lines = %d, want the %d-row cap:\n%s", count, nearFloorCoverageReportLimit, got)
	}
	if !strings.Contains(got, "  near floor: 3 more package(s) within 2.0000 percentage points not shown\n") {
		t.Fatalf("near-floor report did not state its omitted rows:\n%s", got)
	}
	if !strings.Contains(got, "near-floor=13") {
		t.Fatalf("tally did not count every near-floor package:\n%s", got)
	}
}

func TestFunctionalStreamKeepsNonCoverageOutputLines(t *testing.T) {
	var sink bytes.Buffer
	reporter := newFunctionalStreamReporter(&sink)
	writer := reporter.stdoutWriter()

	streamed := []string{
		"=== RUN   TestSomething\n",
		"coverage: 42.7% of statements in " + modulePath + "/pkg/config\n",
		"--- FAIL: TestSomething (0.12s)\n",
		"panic: test timed out after 10m0s\n",
		"ok  \t" + modulePath + "/tests/functional/alpha\t1.234s\tcoverage: 42.7% of statements\n",
	}
	for _, output := range streamed {
		event, err := json.Marshal(goTestTimingEvent{
			Action:  "output",
			Package: modulePath + "/tests/functional/alpha",
			Output:  output,
		})
		if err != nil {
			t.Fatalf("marshal stream event: %v", err)
		}
		if _, err := writer.Write(append(event, '\n')); err != nil {
			t.Fatalf("stream reporter write error = %v", err)
		}
	}

	got := sink.String()
	for _, want := range []string{
		"=== RUN   TestSomething\n",
		"--- FAIL: TestSomething (0.12s)\n",
		"panic: test timed out after 10m0s\n",
		"ok  \t" + modulePath + "/tests/functional/alpha\t1.234s\tcoverage: 42.7% of statements\n",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("functional stream dropped %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "coverage: 42.7% of statements in ") {
		t.Fatalf("functional stream forwarded the standalone per-package coverage line:\n%s", got)
	}
}
