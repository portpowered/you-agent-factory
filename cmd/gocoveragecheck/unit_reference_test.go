package main

import (
	"encoding/json"
	"errors"
	"io"
	"maps"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"testing"
)

const unitReferenceFailureEnvironment = "GO_COVERAGECHECK_UNIT_REFERENCE_FAIL"

type unitCoverageReferenceFixture struct {
	root             string
	packages         []string
	internalPackage  string
	externalPackage  string
	testlessPackage  string
	generatedPackage string
	platformPackage  string
}

type unitReferenceGoListPackage struct {
	ImportPath   string
	GoFiles      []string
	TestGoFiles  []string
	XTestGoFiles []string
}

// unitCoverageReference is the observable baseline that later execution-set
// changes must preserve. The coverage profile is already canonicalized by the
// command, while the timing summary supplies the top-level test inventory.
type unitCoverageReference struct {
	exitCode           int
	timing             functionalTimingSummaryJSON
	timingPackages     []string
	testOccurrences    map[string]int
	testOutcomes       map[string]string
	coveragePackages   []string
	coveragePackageArg string
	coverageProfile    []byte
	coverageBlocks     map[string]coverageBlock
	coverageSummary    *coverageSummaryJSON
	manifestPath       string
	testInvocationArgs []string
	stdout             string
	stderr             string
}

func TestUnitCoverageReferenceCapturesUnfilteredContract(t *testing.T) {
	fixture := writeUnitCoverageReferenceFixture(t)
	assertUnitCoverageReferenceMetadata(t, fixture)

	passing := captureUnitCoverageReference(t, fixture, "passing", false, "", false)
	filteredPassing := captureUnitCoverageReference(t, fixture, "passing", false, "", true)
	assertPassingUnitCoverageReference(t, fixture, passing)
	assertUnitCoverageReferenceExecutionSets(t, fixture, passing, filteredPassing)
	assertUnitCoverageReferenceEquivalent(t, "passing", passing, filteredPassing)

	floorFailure := captureUnitCoverageReference(t, fixture, "floor-failure", false, fixture.internalPackage, false)
	filteredFloorFailure := captureUnitCoverageReference(t, fixture, "floor-failure", false, fixture.internalPackage, true)
	assertUnitCoverageReferenceHasSameExecution(t, fixture, passing, floorFailure)
	assertUnitCoverageReferenceEquivalent(t, "floor-failure", floorFailure, filteredFloorFailure)
	if floorFailure.exitCode != 1 {
		t.Fatalf("floor-failure exit code = %d, want 1", floorFailure.exitCode)
	}
	if floorFailure.coverageSummary == nil || len(floorFailure.coverageSummary.PackageFloorFindings) != 1 {
		t.Fatalf("floor-failure summary = %+v, want one package-floor finding", floorFailure.coverageSummary)
	}
	if !strings.Contains(floorFailure.coverageSummary.PackageFloorFindings[0], fixture.internalPackage) {
		t.Fatalf("floor-failure findings = %v, want internal package diagnostic", floorFailure.coverageSummary.PackageFloorFindings)
	}
	if !strings.Contains(floorFailure.stderr, "package coverage regression: package="+fixture.internalPackage) {
		t.Fatalf("floor-failure stderr = %q, want manifest floor diagnostic", floorFailure.stderr)
	}

	testFailure := captureUnitCoverageReference(t, fixture, "test-failure", true, "", false)
	filteredTestFailure := captureUnitCoverageReference(t, fixture, "test-failure", true, "", true)
	assertUnitCoverageReferenceEquivalent(t, "test-failure", testFailure, filteredTestFailure)
	if testFailure.exitCode != 1 {
		t.Fatalf("test-failure exit code = %d, want 1", testFailure.exitCode)
	}
	if testFailure.coverageSummary == nil || testFailure.coverageSummary.Complete || len(testFailure.coverageBlocks) == 0 {
		t.Fatalf("test-failure partial coverage = summary=%+v blocks=%d, want incomplete diagnostic coverage", testFailure.coverageSummary, len(testFailure.coverageBlocks))
	}
	if !strings.Contains(testFailure.coverageSummary.MeasurementReason, "go test coverage did not complete") {
		t.Fatalf("test-failure measurement reason = %q, want incomplete-run diagnostic", testFailure.coverageSummary.MeasurementReason)
	}
	if len(testFailure.coverageSummary.PackageFloorFindings) != 0 {
		t.Fatalf("test-failure floor findings = %v, want floors not evaluated", testFailure.coverageSummary.PackageFloorFindings)
	}
	if testFailure.timing.TestFailCount != 1 || testFailure.timing.TestPassCount != 3 {
		t.Fatalf("test-failure timing counts = pass=%d fail=%d, want 3/1", testFailure.timing.TestPassCount, testFailure.timing.TestFailCount)
	}
	failedTest := fixture.internalPackage + "#TestReferenceFailure"
	if testFailure.testOutcomes[failedTest] != timingOutcomeFail || testFailure.testOccurrences[failedTest] != 1 {
		t.Fatalf("test-failure outcome/count for %s = %q/%d, want fail/1", failedTest, testFailure.testOutcomes[failedTest], testFailure.testOccurrences[failedTest])
	}
	if !strings.Contains(testFailure.stderr, "coverage not evaluated") {
		t.Fatalf("test-failure stderr = %q, want coverage-not-evaluated warning", testFailure.stderr)
	}
}

func writeUnitCoverageReferenceFixture(t *testing.T) unitCoverageReferenceFixture {
	t.Helper()

	fixture := unitCoverageReferenceFixture{root: t.TempDir()}
	fixture.internalPackage = modulePath + "/pkg/unitcoveragefixture/internal"
	fixture.externalPackage = modulePath + "/pkg/unitcoveragefixture/external"
	fixture.testlessPackage = modulePath + "/pkg/unitcoveragefixture/testless"
	fixture.generatedPackage = modulePath + "/pkg/unitcoveragefixture/generated"
	fixture.platformPackage = modulePath + "/pkg/unitcoveragefixture/platform"
	fixture.packages = []string{
		fixture.externalPackage,
		fixture.generatedPackage,
		fixture.internalPackage,
		fixture.platformPackage,
		fixture.testlessPackage,
	}
	slices.Sort(fixture.packages)

	writeUnitReferenceModule(t, fixture.root)
	writeUnitReferenceInternalPackage(t, fixture.root)
	writeUnitReferenceExternalPackage(t, fixture.root)
	writeUnitReferenceTestFreePackages(t, fixture.root)
	writeUnitReferencePlatformPackage(t, fixture.root)
	return fixture
}

func writeUnitReferenceModule(t *testing.T, root string) {
	writeUnitReferenceFile(t, root, "go.mod", "module "+modulePath+"\n\ngo 1.25.0\n")
}

func writeUnitReferenceInternalPackage(t *testing.T, root string) {
	writeUnitReferenceFile(t, root, "pkg/unitcoveragefixture/internal/value.go", "package internal\n\nfunc Value() int { return 7 }\n\nfunc Uncovered() int { return 9 }\n")
	writeUnitReferenceFile(t, root, "pkg/unitcoveragefixture/internal/value_test.go", `package internal

import (
	"os"
	"testing"
)

func TestInternalCoverageReference(t *testing.T) {
	if Value() != 7 {
		t.Fatal("internal value changed")
	}
}

func TestReferenceFailure(t *testing.T) {
	if os.Getenv("`+unitReferenceFailureEnvironment+`") == "1" {
		t.Fatal("reference fixture requested a test failure")
	}
}
`)
}

func writeUnitReferenceExternalPackage(t *testing.T, root string) {
	writeUnitReferenceFile(t, root, "pkg/unitcoveragefixture/external/value.go", "package external\n\nfunc Value() int { return 11 }\n")
	writeUnitReferenceFile(t, root, "pkg/unitcoveragefixture/external/value_external_test.go", `package external_test

import (
	"testing"

	"`+modulePath+`/pkg/unitcoveragefixture/external"
)

func TestExternalCoverageReference(t *testing.T) {
	if external.Value() != 11 {
		t.Fatal("external value changed")
	}
}
`)
}

func writeUnitReferenceTestFreePackages(t *testing.T, root string) {
	writeUnitReferenceFile(t, root, "pkg/unitcoveragefixture/testless/value.go", "package testless\n\nfunc Value() int { return 13 }\n")
	writeUnitReferenceFile(t, root, "pkg/unitcoveragefixture/generated/generated.go", `// Code generated by the unit coverage reference fixture; DO NOT EDIT.
package generated

func Value() int { return 17 }
`)
}

func writeUnitReferencePlatformPackage(t *testing.T, root string) {
	writeUnitReferenceFile(t, root, "pkg/unitcoveragefixture/platform/value.go", "package platform\n\nfunc Value() int { return 19 }\n")
	writeUnitReferenceFile(t, root, "pkg/unitcoveragefixture/platform/value_windows_test.go", `//go:build windows

package platform

import "testing"

func TestPlatformCoverageReference(t *testing.T) {
	if Value() != 19 {
		t.Fatal("windows platform value changed")
	}
}
`)
	writeUnitReferenceFile(t, root, "pkg/unitcoveragefixture/platform/value_unix_test.go", `//go:build !windows

package platform

import "testing"

func TestPlatformCoverageReference(t *testing.T) {
	if Value() != 19 {
		t.Fatal("unix platform value changed")
	}
}
`)
}

func writeUnitReferenceFile(t *testing.T, root string, relativePath string, contents string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relativePath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create unit reference fixture directory: %v", err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write unit reference fixture file %s: %v", relativePath, err)
	}
}

func assertUnitCoverageReferenceMetadata(t *testing.T, fixture unitCoverageReferenceFixture) {
	t.Helper()
	for _, targetOS := range []string{"linux", "windows"} {
		listed := listUnitReferencePackages(t, fixture.root, targetOS)
		assertUnitReferencePackageMetadata(t, listed, fixture, targetOS)
	}
}

func listUnitReferencePackages(t *testing.T, root string, targetOS string) map[string]unitReferenceGoListPackage {
	t.Helper()
	command := exec.Command("go", "list", "-json", "./pkg/...")
	command.Dir = root
	command.Env = replaceUnitReferenceEnvironment(os.Environ(), "GOOS", targetOS)
	output, err := command.Output()
	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			t.Fatalf("go list unit reference fixture for %s: %v\n%s", targetOS, err, exitError.Stderr)
		}
		t.Fatalf("go list unit reference fixture for %s: %v", targetOS, err)
	}

	decoder := json.NewDecoder(strings.NewReader(string(output)))
	packages := make(map[string]unitReferenceGoListPackage)
	for {
		var listed unitReferenceGoListPackage
		err := decoder.Decode(&listed)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("decode go list unit reference fixture for %s: %v", targetOS, err)
		}
		packages[listed.ImportPath] = listed
	}
	return packages
}

func replaceUnitReferenceEnvironment(environment []string, name string, value string) []string {
	result := make([]string, 0, len(environment)+1)
	found := false
	prefix := name + "="
	for _, entry := range environment {
		if strings.HasPrefix(entry, prefix) {
			result = append(result, prefix+value)
			found = true
			continue
		}
		result = append(result, entry)
	}
	if !found {
		result = append(result, prefix+value)
	}
	return result
}

func assertUnitReferencePackageMetadata(t *testing.T, listed map[string]unitReferenceGoListPackage, fixture unitCoverageReferenceFixture, targetOS string) {
	t.Helper()
	for _, packagePath := range fixture.packages {
		if _, ok := listed[packagePath]; !ok {
			t.Fatalf("go list %s packages = %v, missing %s", targetOS, sortedUnitReferencePackageNames(listed), packagePath)
		}
	}
	internal := listed[fixture.internalPackage]
	if !slices.Contains(internal.TestGoFiles, "value_test.go") || len(internal.XTestGoFiles) != 0 {
		t.Fatalf("internal metadata for %s = %+v, want internal test only", targetOS, internal)
	}
	external := listed[fixture.externalPackage]
	if len(external.TestGoFiles) != 0 || !slices.Contains(external.XTestGoFiles, "value_external_test.go") {
		t.Fatalf("external metadata for %s = %+v, want external test only", targetOS, external)
	}
	for _, packagePath := range []string{fixture.testlessPackage, fixture.generatedPackage} {
		metadata := listed[packagePath]
		if len(metadata.TestGoFiles) != 0 || len(metadata.XTestGoFiles) != 0 || len(metadata.GoFiles) == 0 {
			t.Fatalf("test-free metadata for %s/%s = %+v, want production files and no tests", targetOS, packagePath, metadata)
		}
	}
	platform := listed[fixture.platformPackage]
	wantPlatformTest := "value_unix_test.go"
	if targetOS == "windows" {
		wantPlatformTest = "value_windows_test.go"
	}
	if len(platform.TestGoFiles) != 1 || platform.TestGoFiles[0] != wantPlatformTest || len(platform.XTestGoFiles) != 0 {
		t.Fatalf("platform metadata for %s = %+v, want selected %s", targetOS, platform, wantPlatformTest)
	}
}

func sortedUnitReferencePackageNames(packages map[string]unitReferenceGoListPackage) []string {
	names := make([]string, 0, len(packages))
	for name := range packages {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

func captureUnitCoverageReference(t *testing.T, fixture unitCoverageReferenceFixture, label string, failTest bool, floorPackage string, filtered bool) unitCoverageReference {
	t.Helper()
	previousFailureValue, hadFailureValue := os.LookupEnv(unitReferenceFailureEnvironment)
	if err := os.Setenv(unitReferenceFailureEnvironment, map[bool]string{true: "1", false: "0"}[failTest]); err != nil {
		t.Fatalf("set unit reference failure environment: %v", err)
	}
	defer func() {
		if err := restoreUnitReferenceEnvironment(unitReferenceFailureEnvironment, previousFailureValue, hadFailureValue); err != nil {
			t.Errorf("restore unit reference failure environment: %v", err)
		}
	}()
	previousGOOSValue, hadGOOSValue := os.LookupEnv("GOOS")
	if err := os.Setenv("GOOS", runtime.GOOS); err != nil {
		t.Fatalf("set unit reference target environment: %v", err)
	}
	defer func() {
		if err := restoreUnitReferenceEnvironment("GOOS", previousGOOSValue, hadGOOSValue); err != nil {
			t.Errorf("restore unit reference target environment: %v", err)
		}
	}()

	workingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatalf("get unit reference working directory: %v", err)
	}
	if err := os.Chdir(fixture.root); err != nil {
		t.Fatalf("enter unit reference fixture: %v", err)
	}
	defer func() {
		if err := os.Chdir(workingDirectory); err != nil {
			t.Fatalf("restore unit reference working directory: %v", err)
		}
	}()

	mode := "unfiltered"
	if filtered {
		mode = "filtered"
	}
	artifactRoot := filepath.Join(fixture.root, "artifacts", label+"-"+mode)
	if err := os.MkdirAll(artifactRoot, 0o755); err != nil {
		t.Fatalf("create unit reference artifacts: %v", err)
	}
	manifestPath := writeUnitReferenceManifest(t, fixture, artifactRoot, floorPackage)
	profilePath := filepath.Join(artifactRoot, "coverage.out")
	coverageSummaryPath := filepath.Join(artifactRoot, "coverage-summary.json")
	timingSummaryPath := filepath.Join(artifactRoot, "timing-summary.json")

	originalRunner := commandRunner
	var testInvocationArgs []string
	runner := func(invocation commandInvocation) (string, string, error) {
		if invocation.name == "go" && len(invocation.args) > 0 && invocation.args[0] == "test" {
			testInvocationArgs = append([]string(nil), invocation.args...)
		}
		return originalRunner(invocation)
	}
	args := []string{
		"-suite=unit",
		"-min=0",
		"-jobs=1",
		"-short=false",
		"-package-manifest=" + manifestPath,
		"-profile=" + profilePath,
		"-json-output=" + coverageSummaryPath,
		"-timing-output=" + timingSummaryPath,
	}
	if !filtered {
		args = append(args, "-packages="+strings.Join(fixture.packages, " "))
	}
	stdout, stderr, exitCode := runMainForTest(t, args, runner)

	var timing functionalTimingSummaryJSON
	readUnitReferenceJSON(t, timingSummaryPath, &timing)
	reference := unitCoverageReference{
		exitCode:           exitCode,
		timing:             timing,
		timingPackages:     unitReferenceTimingPackages(timing),
		testOccurrences:    unitReferenceTestOccurrences(timing),
		testOutcomes:       unitReferenceTestOutcomes(timing),
		coveragePackageArg: unitReferenceCoverageArgument(testInvocationArgs),
		manifestPath:       manifestPath,
		testInvocationArgs: testInvocationArgs,
		stdout:             stdout,
		stderr:             stderr,
	}
	reference.coverageSummary, reference.coveragePackages, reference.coverageProfile, reference.coverageBlocks = readUnitReferenceArtifacts(t, coverageSummaryPath, profilePath, fixture.root)
	return reference
}

func readUnitReferenceArtifacts(t *testing.T, summaryPath string, profilePath string, repoRoot string) (*coverageSummaryJSON, []string, []byte, map[string]coverageBlock) {
	t.Helper()
	data, err := os.ReadFile(summaryPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil, nil, nil
	}
	if err != nil {
		t.Fatalf("read unit reference coverage summary: %v", err)
	}
	var summary coverageSummaryJSON
	if err := json.Unmarshal(data, &summary); err != nil {
		t.Fatalf("decode unit reference coverage summary: %v", err)
	}
	profile, err := os.ReadFile(profilePath)
	if err != nil {
		t.Fatalf("read unit reference canonical coverage profile: %v", err)
	}
	blocks, err := readCoverageProfileBlocks(profilePath, repoRoot)
	if err != nil {
		t.Fatalf("read unit reference canonical coverage blocks: %v", err)
	}
	return &summary, sortedUnitReferencePackageNamesFromSummary(summary.Packages), profile, blocks
}

func restoreUnitReferenceEnvironment(name string, previousValue string, wasSet bool) error {
	if wasSet {
		return os.Setenv(name, previousValue)
	}
	return os.Unsetenv(name)
}

func writeUnitReferenceManifest(t *testing.T, fixture unitCoverageReferenceFixture, artifactRoot string, floorPackage string) string {
	t.Helper()
	entries := make([]coverageManifestEntry, 0, len(fixture.packages))
	for _, packagePath := range fixture.packages {
		minimum := "0.00"
		if packagePath == floorPackage {
			minimum = "100.00"
		}
		entries = append(entries, coverageManifestEntry{Package: packagePath, Minimum: json.RawMessage(minimum)})
	}
	manifest := coverageManifest{
		Version:             coverageManifestVersion,
		Lane:                unitCoverageSuite,
		DefaultFloorPercent: json.RawMessage("0.00"),
		Packages:            entries,
	}
	data, err := renderCoverageManifest(manifest)
	if err != nil {
		t.Fatalf("render unit reference manifest: %v", err)
	}
	path := filepath.Join(artifactRoot, "coverage-manifest.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write unit reference manifest: %v", err)
	}
	return path
}

func readUnitReferenceJSON(t *testing.T, path string, target any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read unit reference JSON %s: %v", path, err)
	}
	if err := json.Unmarshal(data, target); err != nil {
		t.Fatalf("decode unit reference JSON %s: %v", path, err)
	}
}

func unitReferenceTimingPackages(summary functionalTimingSummaryJSON) []string {
	packages := make([]string, 0, len(summary.Packages))
	for _, packageTiming := range summary.Packages {
		packages = append(packages, packageTiming.Package)
	}
	slices.Sort(packages)
	return packages
}

func unitReferenceTestOccurrences(summary functionalTimingSummaryJSON) map[string]int {
	occurrences := make(map[string]int, len(summary.Tests))
	for _, test := range summary.Tests {
		occurrences[test.Package+"#"+test.Test]++
	}
	return occurrences
}

func unitReferenceTestOutcomes(summary functionalTimingSummaryJSON) map[string]string {
	outcomes := make(map[string]string, len(summary.Tests))
	for _, test := range summary.Tests {
		outcomes[test.Package+"#"+test.Test] = test.Outcome
	}
	return outcomes
}

func unitReferenceCoverageArgument(args []string) string {
	for _, arg := range args {
		if !strings.HasPrefix(arg, "-coverpkg=") {
			continue
		}
		return strings.TrimPrefix(arg, "-coverpkg=")
	}
	return ""
}

func assertPassingUnitCoverageReference(t *testing.T, fixture unitCoverageReferenceFixture, reference unitCoverageReference) {
	t.Helper()
	if reference.exitCode != 0 {
		t.Fatalf("passing reference exit code = %d, want 0; stderr=%q", reference.exitCode, reference.stderr)
	}
	if !reference.timing.Complete || reference.timing.ExpectedPackageCount != len(fixture.packages) || reference.timing.PackageCount != len(fixture.packages) {
		t.Fatalf("passing reference timing package state = %+v, want complete coverage for %d packages", reference.timing, len(fixture.packages))
	}
	if reference.timing.TestCount != 4 || reference.timing.TestPassCount != 4 || reference.timing.TestFailCount != 0 || reference.timing.TestSkipCount != 0 {
		t.Fatalf("passing reference timing test counts = %+v, want four passing top-level tests", reference.timing)
	}
	assertUnitCoverageReferenceHasSameExecution(t, fixture, reference, reference)
	if reference.coverageSummary == nil || !reference.coverageSummary.Complete {
		t.Fatalf("passing reference coverage summary = %+v, want complete summary", reference.coverageSummary)
	}
	if reference.coverageSummary.PackageFloorPolicy != coverageFloorPolicyBlocking {
		t.Fatalf("passing reference floor policy = %q, want blocking", reference.coverageSummary.PackageFloorPolicy)
	}
	if len(reference.coverageSummary.PackageFloorFindings) != 0 {
		t.Fatalf("passing reference floor findings = %v, want none", reference.coverageSummary.PackageFloorFindings)
	}
	assertUnitReferenceCoverageSummary(t, fixture, reference)
	if len(reference.coverageBlocks) == 0 || len(reference.coverageProfile) == 0 {
		t.Fatal("passing reference did not retain canonical coverage blocks/profile")
	}
	if strings.Contains(string(reference.coverageProfile), fixture.root) {
		t.Fatalf("passing reference profile retained temporary fixture path: %q", reference.coverageProfile)
	}
	if !strings.Contains(string(reference.coverageProfile), "mode: count\n") {
		t.Fatalf("passing reference profile = %q, want count mode", reference.coverageProfile)
	}
	if !strings.Contains(reference.stdout, "Go coverage ") || strings.Contains(reference.stderr, "coverage regression") {
		t.Fatalf("passing reference output = stdout %q stderr %q, want success without floor failure", reference.stdout, reference.stderr)
	}
}

func assertUnitReferenceCoverageSummary(t *testing.T, fixture unitCoverageReferenceFixture, reference unitCoverageReference) {
	t.Helper()
	summary := reference.coverageSummary
	if len(summary.ManifestDiagnostics) != 0 {
		t.Fatalf("passing reference manifest diagnostics = %v, want none", summary.ManifestDiagnostics)
	}
	if len(summary.Packages) != len(fixture.packages) {
		t.Fatalf("passing reference package verdict count = %d, want %d", len(summary.Packages), len(fixture.packages))
	}
	expectedTotals := coverageTotals(reference.coverageBlocks)
	seen := make(map[string]struct{}, len(summary.Packages))
	covered, measurable := 0, 0
	for _, packageSummary := range summary.Packages {
		if _, duplicate := seen[packageSummary.Package]; duplicate {
			t.Fatalf("passing reference repeated package verdict for %s", packageSummary.Package)
		}
		seen[packageSummary.Package] = struct{}{}
		totals := expectedTotals[packageSummary.Package]
		if packageSummary.CoveredStatements != totals.coveredStatements || packageSummary.MeasurableStatements != totals.totalStatements {
			t.Fatalf("passing reference package %s measurements = %d/%d, want %d/%d", packageSummary.Package, packageSummary.CoveredStatements, packageSummary.MeasurableStatements, totals.coveredStatements, totals.totalStatements)
		}
		if packageSummary.PackageFloor == nil || *packageSummary.PackageFloor != 0 || packageSummary.MeasurementException != nil {
			t.Fatalf("passing reference package %s gate = %+v/%+v, want explicit zero floor", packageSummary.Package, packageSummary.PackageFloor, packageSummary.MeasurementException)
		}
		covered += totals.coveredStatements
		measurable += totals.totalStatements
	}
	if !slices.Equal(sortedUnitReferencePackageNamesFromSummary(summary.Packages), fixture.packages) {
		t.Fatalf("passing reference measured packages = %v, want %v", sortedUnitReferencePackageNamesFromSummary(summary.Packages), fixture.packages)
	}
	if summary.CoveredStatements != covered || summary.MeasurableStatements != measurable {
		t.Fatalf("passing reference aggregate counts = %d/%d, want %d/%d", summary.CoveredStatements, summary.MeasurableStatements, covered, measurable)
	}
	wantPercent := 0.0
	if measurable > 0 {
		wantPercent = math.Round(float64(covered)*100/float64(measurable)*10) / 10
	}
	if summary.CoveragePercent != wantPercent {
		t.Fatalf("passing reference total coverage = %.1f, want %.1f", summary.CoveragePercent, wantPercent)
	}
}

func sortedUnitReferencePackageNamesFromSummary(packages []packageCoverageJSON) []string {
	names := make([]string, 0, len(packages))
	for _, packageSummary := range packages {
		names = append(names, packageSummary.Package)
	}
	slices.Sort(names)
	return names
}

func assertUnitCoverageReferenceExecutionSets(t *testing.T, fixture unitCoverageReferenceFixture, unfiltered unitCoverageReference, filtered unitCoverageReference) {
	t.Helper()
	if !slices.Equal(unfiltered.timingPackages, fixture.packages) {
		t.Fatalf("unfiltered timing packages = %v, want coverage fixture packages %v", unfiltered.timingPackages, fixture.packages)
	}
	wantFiltered := []string{fixture.externalPackage, fixture.internalPackage, fixture.platformPackage}
	slices.Sort(wantFiltered)
	if !slices.Equal(filtered.timingPackages, wantFiltered) {
		t.Fatalf("filtered timing packages = %v, want exactly build-selected test packages %v", filtered.timingPackages, wantFiltered)
	}
	for name, reference := range map[string]unitCoverageReference{
		"unfiltered": unfiltered,
		"filtered":   filtered,
	} {
		if !slices.Equal(reference.coveragePackages, fixture.packages) {
			t.Fatalf("%s coverage packages = %v, want unchanged measurement universe %v", name, reference.coveragePackages, fixture.packages)
		}
		if reference.coveragePackageArg == "" {
			t.Fatalf("%s go test args = %v, want a coverpkg argument", name, reference.testInvocationArgs)
		}
	}
}

func assertUnitCoverageReferenceEquivalent(t *testing.T, label string, expected unitCoverageReference, actual unitCoverageReference) {
	t.Helper()
	if actual.exitCode != expected.exitCode {
		t.Fatalf("%s exit status changed: expected=%d actual=%d", label, expected.exitCode, actual.exitCode)
	}
	if !maps.Equal(actual.testOccurrences, expected.testOccurrences) {
		t.Fatalf("%s top-level test occurrence counts changed: expected=%v actual=%v", label, expected.testOccurrences, actual.testOccurrences)
	}
	if !maps.Equal(actual.testOutcomes, expected.testOutcomes) {
		t.Fatalf("%s top-level test outcomes changed: expected=%v actual=%v", label, expected.testOutcomes, actual.testOutcomes)
	}
	if actual.timing.Version != expected.timing.Version || actual.timing.Complete != expected.timing.Complete ||
		actual.timing.TestCount != expected.timing.TestCount || actual.timing.TestPassCount != expected.timing.TestPassCount ||
		actual.timing.TestFailCount != expected.timing.TestFailCount || actual.timing.TestSkipCount != expected.timing.TestSkipCount {
		t.Fatalf("%s timing test verdict changed: expected=%+v actual=%+v", label, expected.timing, actual.timing)
	}
	if !slices.Equal(actual.coveragePackages, expected.coveragePackages) {
		t.Fatalf("%s coverage measurement universe changed: expected=%v actual=%v", label, expected.coveragePackages, actual.coveragePackages)
	}
	if actual.coveragePackageArg != expected.coveragePackageArg {
		t.Fatalf("%s coverpkg argument changed: expected=%q actual=%q", label, expected.coveragePackageArg, actual.coveragePackageArg)
	}
	blocksEqual := maps.Equal(actual.coverageBlocks, expected.coverageBlocks)
	if !blocksEqual {
		t.Fatalf("%s canonical coverage blocks or counts changed: blocks_equal=%t expected_blocks=%d actual_blocks=%d expected_totals=%v actual_totals=%v", label, blocksEqual, len(expected.coverageBlocks), len(actual.coverageBlocks), coverageTotals(expected.coverageBlocks), coverageTotals(actual.coverageBlocks))
	}
	if !maps.Equal(coverageTotals(actual.coverageBlocks), coverageTotals(expected.coverageBlocks)) {
		t.Fatalf("%s per-package coverage totals changed", label)
	}
	if !reflect.DeepEqual(normalizeUnitReferenceSummary(actual.coverageSummary, actual.manifestPath), normalizeUnitReferenceSummary(expected.coverageSummary, expected.manifestPath)) {
		t.Fatalf("%s coverage verdict, manifest, or floor decision changed:\nexpected=%+v\nactual=%+v", label, expected.coverageSummary, actual.coverageSummary)
	}
}

func normalizeUnitReferenceSummary(summary *coverageSummaryJSON, manifestPath string) *coverageSummaryJSON {
	if summary == nil {
		return nil
	}
	normalized := *summary
	if summary.Complete {
		normalized.MeasurementReason = ""
	} else {
		// A failed go test includes package-result prose in this diagnostic;
		// importing test-free packages changes that prose's percentage even
		// though the partial canonical measurements and decisions are equal.
		normalized.MeasurementReason = "<incomplete coverage>"
	}
	normalized.PackageFloorFindings = normalizeUnitReferenceDiagnostics(summary.PackageFloorFindings, manifestPath)
	normalized.ManifestDiagnostics = normalizeUnitReferenceDiagnostics(summary.ManifestDiagnostics, manifestPath)
	return &normalized
}

func normalizeUnitReferenceDiagnostics(diagnostics []string, manifestPath string) []string {
	normalized := slices.Clone(diagnostics)
	for index, diagnostic := range normalized {
		normalized[index] = strings.ReplaceAll(diagnostic, manifestPath, "<coverage-manifest>")
	}
	return normalized
}

func assertUnitCoverageReferenceHasSameExecution(t *testing.T, fixture unitCoverageReferenceFixture, expected unitCoverageReference, actual unitCoverageReference) {
	t.Helper()
	if !slices.Equal(actual.timingPackages, fixture.packages) {
		t.Fatalf("reference timing packages = %v, want unfiltered packages %v", actual.timingPackages, fixture.packages)
	}
	wantTests := map[string]int{
		fixture.internalPackage + "#TestInternalCoverageReference": 1,
		fixture.internalPackage + "#TestReferenceFailure":          1,
		fixture.externalPackage + "#TestExternalCoverageReference": 1,
		fixture.platformPackage + "#TestPlatformCoverageReference": 1,
	}
	if !maps.Equal(actual.testOccurrences, wantTests) {
		t.Fatalf("reference test occurrences = %v, want exactly once for %v", actual.testOccurrences, wantTests)
	}
	if len(actual.testInvocationArgs) == 0 || slices.Contains(actual.testInvocationArgs, "-count=1") {
		t.Fatalf("reference go test args = %v, want Go default count", actual.testInvocationArgs)
	}
	if !maps.Equal(actual.testOccurrences, expected.testOccurrences) {
		t.Fatalf("reference test inventory changed: expected=%v actual=%v", expected.testOccurrences, actual.testOccurrences)
	}
	if expected.coverageSummary != nil && actual.coverageSummary != nil {
		if actual.coverageSummary.CoveredStatements != expected.coverageSummary.CoveredStatements ||
			actual.coverageSummary.MeasurableStatements != expected.coverageSummary.MeasurableStatements ||
			actual.coverageSummary.CoveragePercent != expected.coverageSummary.CoveragePercent {
			t.Fatalf("reference aggregate coverage changed: expected=%+v actual=%+v", expected.coverageSummary, actual.coverageSummary)
		}
		if !slices.Equal(actual.coverageProfile, expected.coverageProfile) || !maps.Equal(actual.coverageBlocks, expected.coverageBlocks) {
			t.Fatalf("reference canonical coverage changed between equivalent executions")
		}
	}
}
