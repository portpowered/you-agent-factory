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
// one near-floor package, and one comfortably passing package so the complete
// ordered verdict block has something to order.
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

	configLine := strings.Index(stdout, "  package="+modulePath+"/pkg/config coverage=25.0% floor=80.0% delta=-55.0pp status=FAIL lane=functional\n")
	serviceLine := strings.Index(stdout, "  package="+modulePath+"/pkg/service coverage=90.0% floor=89.0% delta=+1.0pp status=PASS lane=functional\n")
	wireLine := strings.Index(stdout, "  package="+modulePath+"/pkg/wire coverage=100.0% floor=50.0% delta=+50.0pp status=PASS lane=functional\n")
	tally := strings.Index(stdout, "  tally: ")
	if configLine < 0 || serviceLine < 0 || wireLine < 0 || tally < 0 {
		t.Fatalf("verdict block missing sections (config=%d service=%d wire=%d tally=%d):\n%s", configLine, serviceLine, wireLine, tally, stdout)
	}
	if !(configLine < serviceLine && serviceLine < wireLine && wireLine < tally) {
		t.Fatalf("verdict block order = config@%d service@%d wire@%d tally@%d, want headroom order and tally last:\n%s", configLine, serviceLine, wireLine, tally, stdout)
	}
	if strings.Contains(stdout, "uncovered blocks:") || strings.Contains(stdout, "file.go:") {
		t.Fatalf("default verdict exposed uncovered source detail:\n%s", stdout)
	}
	if !strings.Contains(stdout, "  tally: measured-packages=3 gated-packages=3 below-floor=1 near-floor=1 gate-failures=1\n") {
		t.Fatalf("verdict tally line missing or wrong:\n%s", stdout)
	}
	if strings.Contains(stdout, "  near floor: package=") || strings.Contains(stdout, "not shown") {
		t.Fatalf("verdict block retained the elided near-floor format:\n%s", stdout)
	}
}

func TestFunctionalCoverageVerdictNamesHeldFloorWithoutCallingItAnActiveViolation(t *testing.T) {
	jsonPath := filepath.Join(t.TempDir(), "coverage-summary.json")
	configPackage := modulePath + "/pkg/config"
	manifestPath := writePackageMinimumManifestWithEntries(t, "functional", []manifestPackageSpec{
		{
			importPath: configPackage,
			minimum:    "80.00",
			floorHold: &coverageManifestFloorHold{
				Justification: "current-main baseline is being restored",
				Owner:         "coverage-remediation",
				Deadline:      "2027-07-15",
				RemovalGate:   "matching functional tests restore the existing floor",
			},
		},
		{importPath: modulePath + "/pkg/service", minimum: "89.00"},
		{importPath: modulePath + "/pkg/wire", minimum: "50.00"},
	})

	stdout, stderr, err := captureFunctionalVerdictRun(t, functionalVerdictConfig(manifestPath, jsonPath))
	if err != nil {
		t.Fatalf("execute() error = %v, want the staged hold to keep the blocking lane green; stdout=%s stderr=%s", err, stdout, stderr)
	}
	if strings.Contains(stderr, "advisory") || strings.Contains(stderr, "report-only") {
		t.Fatalf("blocking staged run emitted advisory guidance: %s", stderr)
	}
	if !strings.Contains(stdout, "  package="+configPackage+" coverage=25.0% floor=80.0% delta=-55.0pp status=HOLD lane=functional\n") {
		t.Fatalf("verdict did not distinguish the held package from active violations:\n%s", stdout)
	}
	if strings.Contains(stdout, "floor hold:") || strings.Contains(stdout, "uncovered-blocks=") {
		t.Fatalf("held verdict exposed verbose floor diagnostics:\n%s", stdout)
	}
	if !strings.Contains(stdout, "below-floor=0 near-floor=1 gate-failures=0") {
		t.Fatalf("verdict tally treated the held package as an active failure:\n%s", stdout)
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
	for _, want := range []string{
		"  package=" + modulePath + "/pkg/config coverage=25.0% floor=20.0% delta=+5.0pp status=PASS lane=functional\n",
		"  package=" + modulePath + "/pkg/service coverage=90.0% floor=89.0% delta=+1.0pp status=PASS lane=functional\n",
		"  package=" + modulePath + "/pkg/wire coverage=100.0% floor=50.0% delta=+50.0pp status=PASS lane=functional\n",
	} {
		if strings.Count(stdout, want) != 1 {
			t.Fatalf("verdict line count for %q = %d, want one:\n%s", want, strings.Count(stdout, want), stdout)
		}
	}
	if !strings.Contains(stdout, "  tally: measured-packages=3 gated-packages=3 below-floor=0 near-floor=1 gate-failures=0\n") {
		t.Fatalf("passing verdict tally line missing or wrong:\n%s", stdout)
	}
	if strings.Contains(stdout, "  near floor: package=") || strings.Contains(stdout, "not shown") {
		t.Fatalf("passing verdict retained the elided near-floor format:\n%s", stdout)
	}
	if !strings.Contains(stdout, "Go coverage") || !strings.Contains(stdout, "meets minimum") {
		t.Fatalf("passing run lost its success message:\n%s", stdout)
	}
}

func TestUnitCoverageRunUsesCompactVerdictWithoutJSONArtifact(t *testing.T) {
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
	if !strings.Contains(got, "Unit package coverage verdict:\n") {
		t.Fatalf("unit lane did not render its compact verdict:\n%s", got)
	}
	if !strings.Contains(got, "package="+configPackage+" coverage=100.0% floor=100.0% delta=+0.0pp status=PASS lane=unit") {
		t.Fatalf("unit lane compact verdict omitted the measured package:\n%s", got)
	}
	if strings.Contains(got, "\tcoverage: ") {
		t.Fatalf("unit lane still printed a raw per-package coverage line:\n%s", got)
	}
}

func TestCoverageVerdictReportsEveryGatedPackage(t *testing.T) {
	originalStdout := stdoutWriter
	defer func() { stdoutWriter = originalStdout }()
	var stdout bytes.Buffer
	stdoutWriter = &stdout

	floor := 50.0
	result := coverageResult{
		packageGates:  make(map[string]packageCoverageGate),
		packageTotals: make(map[string]packageCoverageTotals),
	}
	const packageCount = 13
	for index := range packageCount {
		importPath := fmt.Sprintf("%s/pkg/near%02d", modulePath, index)
		result.packageSummaries = append(result.packageSummaries, packageCoverageSummary{importPath: importPath, coverage: 51})
		result.packageGates[importPath] = packageCoverageGate{Floor: &floor}
		result.packageTotals[importPath] = packageCoverageTotals{coveredStatements: 51, totalStatements: 100}
	}

	writeCoverageVerdict("Functional", result, nil)

	got := stdout.String()
	if count := strings.Count(got, "  package="); count != packageCount {
		t.Fatalf("package verdict lines = %d, want all %d rows:\n%s", count, packageCount, got)
	}
	if strings.Contains(got, "  near floor: package=") || strings.Contains(got, "not shown") {
		t.Fatalf("complete verdict retained the elided near-floor format:\n%s", got)
	}
	if !strings.Contains(got, "near-floor=13") {
		t.Fatalf("tally did not count every near-floor package:\n%s", got)
	}
}

func TestCoverageVerdictReportsUngatedPackagesAfterGatedRows(t *testing.T) {
	originalStdout := stdoutWriter
	defer func() { stdoutWriter = originalStdout }()
	var stdout bytes.Buffer
	stdoutWriter = &stdout

	floor := 80.0
	gated := modulePath + "/pkg/gated"
	ungated := modulePath + "/pkg/report-only"
	result := coverageResult{
		packageSummaries: []packageCoverageSummary{
			{importPath: ungated, coverage: 5},
			{importPath: gated, coverage: 25},
		},
		packageTotals: map[string]packageCoverageTotals{
			gated: {coveredStatements: 25, totalStatements: 100},
		},
		packageGates: map[string]packageCoverageGate{
			gated: {Floor: &floor},
		},
	}

	writeCoverageVerdict("Functional", result, nil)

	got := stdout.String()
	wantGated := "  package=" + gated + " coverage=25.0% floor=80.0% delta=-55.0pp status=FAIL lane=functional\n"
	wantUngated := "  package=" + ungated + " coverage=5.0% floor=n/a status=report-only lane=functional\n"
	gatedIndex := strings.Index(got, wantGated)
	ungatedIndex := strings.Index(got, wantUngated)
	if gatedIndex < 0 || ungatedIndex < 0 || gatedIndex > ungatedIndex {
		t.Fatalf("verdict rows = gated@%d report-only@%d, want gated first:\n%s", gatedIndex, ungatedIndex, got)
	}
	if !strings.Contains(got, "measured-packages=2 gated-packages=1 below-floor=1 near-floor=0 gate-failures=0") {
		t.Fatalf("verdict tally did not distinguish report-only package:\n%s", got)
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
		"FAIL\t" + modulePath + "/tests/functional/alpha\tcoverage: 42.7% of statements in " + modulePath + "/pkg/config\n",
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
		"FAIL\t" + modulePath + "/tests/functional/alpha\tcoverage: 42.7% of statements in " + modulePath + "/pkg/config\n",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("functional stream dropped %q:\n%s", want, got)
		}
	}
	for _, line := range strings.Split(got, "\n") {
		if strings.HasPrefix(line, "coverage: 42.7% of statements in ") {
			t.Fatalf("functional stream forwarded the standalone per-package coverage line:\n%s", got)
		}
	}
}
