package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

type variancePackageFixture struct {
	total   int
	covered int
}

func TestLoadCoverageVarianceProfilesAndRenderReport(t *testing.T) {
	t.Parallel()

	_, report, _ := newVarianceReportFixture(t)
	data, err := renderCoverageVarianceReport(report)
	if err != nil {
		t.Fatalf("renderCoverageVarianceReport() error = %v", err)
	}
	wantParts := []string{
		"**Sampling commit:** `913a667d2-full`",
		"Run count:** 5 complete profiles",
		"Profile labels:** `run-01`, `run-02`, `run-03`, `run-04`, `run-05`",
		"3/5 | 4/5 | 2/5 | 5/5 | 4/5 | 40.0000% | 100.0000% | 60.0000 pp | 40.00% | 60.00% | -20.0000 pp",
		"1/2 | 1/2 | 1/2 | 2/2 | 1/2 | 50.0000% | 100.0000% | 50.0000 pp | 50.00% | exception | n/a pp",
	}
	for _, want := range wantParts {
		if !strings.Contains(string(data), want) {
			t.Fatalf("variance report does not contain %q:\n%s", want, data)
		}
	}
	for _, absent := range []string{
		"Measured remedy classification",
		"loadedsource",
		"dispatch_planning",
		"Supplied operator evidence (not part of the new sample set)",
	} {
		if strings.Contains(string(data), absent) {
			t.Fatalf("generic variance report contains unrelated annotation %q:\n%s", absent, data)
		}
	}
	genericSecond, err := renderCoverageVarianceReport(report)
	if err != nil {
		t.Fatalf("renderCoverageVarianceReport() generic second error = %v", err)
	}
	if !bytes.Equal(data, genericSecond) {
		t.Fatalf("generic variance report is not byte-stable:\nfirst=%s\nsecond=%s", data, genericSecond)
	}
}

func TestRenderCoverageVarianceReportWithAnnotations(t *testing.T) {
	t.Parallel()

	root, report, packages := newVarianceReportFixture(t)
	annotationPath := filepath.Join(root, "annotations.json")
	annotationData := fmt.Sprintf(`{
  "summary": "The supplied annotation is limited to the measured sample set.",
  "remedies": [
    {"package": %q, "classification": "deterministic functional exercise", "observedEvidence": "3/5 in the sample set", "remedy": "Retain the existing floor."}
  ],
  "suppliedEvidence": [
    {"packages": [%q], "text": "Supplied context for pkg/config remains separate from measured samples."}
  ]
}
`, packages[0], packages[0])
	if err := os.WriteFile(annotationPath, []byte(annotationData), 0o600); err != nil {
		t.Fatalf("write annotation fixture: %v", err)
	}
	annotations, err := readCoverageVarianceAnnotations(annotationPath, report)
	if err != nil {
		t.Fatalf("readCoverageVarianceAnnotations() error = %v", err)
	}
	report.annotations = annotations
	report.command += " -variance-annotations " + filepath.ToSlash(annotationPath)
	annotated, err := renderCoverageVarianceReport(report)
	if err != nil {
		t.Fatalf("renderCoverageVarianceReport() with annotations error = %v", err)
	}
	for _, want := range []string{
		"Annotation input:",
		"Measured remedy classification",
		"The supplied annotation is limited to the measured sample set.",
		"Supplied operator evidence (not part of the new sample set)",
		"Supplied context for pkg/config remains separate from measured samples.",
	} {
		if !strings.Contains(string(annotated), want) {
			t.Fatalf("annotated variance report does not contain %q:\n%s", want, annotated)
		}
	}
	second, err := renderCoverageVarianceReport(report)
	if err != nil {
		t.Fatalf("renderCoverageVarianceReport() second error = %v", err)
	}
	if !bytes.Equal(annotated, second) {
		t.Fatalf("annotated variance report is not byte-stable:\nfirst=%s\nsecond=%s", annotated, second)
	}
}

func newVarianceReportFixture(t *testing.T) (string, coverageVarianceReport, []string) {
	t.Helper()

	root := t.TempDir()
	packages := []string{modulePath + "/pkg/config", modulePath + "/pkg/service"}
	covered := [][]int{{3, 1}, {4, 1}, {2, 1}, {5, 2}, {4, 1}}
	paths := make([]string, 0, len(covered))
	for index, values := range covered {
		path := filepath.Join(root, fmt.Sprintf("run-%02d.out", index+1))
		writeVarianceProfile(t, path, map[string]variancePackageFixture{
			packages[0]: {total: 5, covered: values[0]},
			packages[1]: {total: 2, covered: values[1]},
		})
		paths = append(paths, path)
	}
	slices.Reverse(paths)

	samples, err := loadCoverageVarianceProfiles(paths, root)
	if err != nil {
		t.Fatalf("loadCoverageVarianceProfiles() error = %v", err)
	}
	report, err := buildCoverageVarianceReport("913a667d2-full", "functional", 2, samples, map[string]coverageVarianceCurrentFloor{
		packages[0]: {label: "60.00%", floor: coverageFloor(6000), valid: true},
		packages[1]: {label: "exception"},
	})
	if err != nil {
		t.Fatalf("buildCoverageVarianceReport() error = %v", err)
	}
	return root, report, packages
}

func TestLoadCoverageVarianceProfilesRejectsInvalidSampleSets(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	configPackage := modulePath + "/pkg/config"
	servicePackage := modulePath + "/pkg/service"
	writeProfile := func(name string, includeService bool, serviceTotal int) string {
		path := filepath.Join(root, name)
		fixtures := map[string]variancePackageFixture{
			configPackage: {total: 2, covered: 1},
		}
		if includeService {
			fixtures[servicePackage] = variancePackageFixture{total: serviceTotal, covered: 1}
		}
		writeVarianceProfile(t, path, fixtures)
		return path
	}

	validPaths := make([]string, 0, minimumVarianceSamples)
	for index := 0; index < minimumVarianceSamples; index++ {
		validPaths = append(validPaths, writeProfile(fmt.Sprintf("valid-%02d.out", index), true, 3))
	}
	tests := []struct {
		name  string
		paths []string
		want  string
	}{
		{name: "too few", paths: validPaths[:4], want: "requires at least 5 profiles"},
		{name: "duplicate input", paths: append(slices.Clone(validPaths[:4]), validPaths[0]), want: "duplicate profile input"},
		{name: "missing profile", paths: append(slices.Clone(validPaths[:4]), filepath.Join(root, "missing.out")), want: "is unreadable"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := loadCoverageVarianceProfiles(tc.paths, root)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("loadCoverageVarianceProfiles() error = %v, want %q", err, tc.want)
			}
		})
	}

	incompatible := slices.Clone(validPaths)
	writeProfile("incompatible.out", false, 0)
	incompatible[4] = filepath.Join(root, "incompatible.out")
	if _, err := loadCoverageVarianceProfiles(incompatible, root); err == nil || !strings.Contains(err.Error(), "incompatible package universe") {
		t.Fatalf("package-universe validation error = %v, want incompatible package universe", err)
	}

	inconsistent := slices.Clone(validPaths)
	writeProfile("inconsistent.out", true, 4)
	inconsistent[4] = filepath.Join(root, "inconsistent.out")
	if _, err := loadCoverageVarianceProfiles(inconsistent, root); err == nil || !strings.Contains(err.Error(), "inconsistent total statements") {
		t.Fatalf("total validation error = %v, want inconsistent total statements", err)
	}
}

func TestReadCoverageVarianceAnnotationsRejectsAbsentSamplePackage(t *testing.T) {
	t.Parallel()

	configPackage := modulePath + "/pkg/config"
	servicePackage := modulePath + "/pkg/service"
	report := coverageVarianceReport{
		packages: []coverageVariancePackage{{importPath: configPackage}},
	}
	annotationPath := filepath.Join(t.TempDir(), "annotations.json")
	annotationData := fmt.Sprintf(`{
  "remedies": [
    {"package": %q, "classification": "inherent concurrent variance", "observedEvidence": "not measured", "remedy": "Reject unrelated annotation input."}
  ]
}
`, servicePackage)
	if err := os.WriteFile(annotationPath, []byte(annotationData), 0o600); err != nil {
		t.Fatalf("write annotation fixture: %v", err)
	}
	if _, err := readCoverageVarianceAnnotations(annotationPath, report); err == nil || !strings.Contains(err.Error(), "absent from the measured sample set") {
		t.Fatalf("readCoverageVarianceAnnotations() error = %v, want absent-package validation", err)
	}
}

func TestReadCoverageVarianceFloorsPreservesNumericAndExceptionEntries(t *testing.T) {
	t.Parallel()

	configPackage := modulePath + "/pkg/config"
	servicePackage := modulePath + "/pkg/service"
	manifestPath := filepath.Join(t.TempDir(), "functional-minimums.json")
	manifest := fmt.Sprintf(`{
  "version": 1,
  "lane": "functional",
  "packages": [
    {"package": %q, "minimum": 67.34},
    {"package": %q, "exception": {"kind": "measurement", "justification": "no measurable statements", "owner": "backend-quality", "deadline": "2027-07-15", "removalGate": "profile reports statements"}}
  ]
}
`, configPackage, servicePackage)
	if err := os.WriteFile(manifestPath, []byte(manifest), 0o600); err != nil {
		t.Fatalf("write manifest fixture: %v", err)
	}
	floors, err := readCoverageVarianceFloors(manifestPath, "functional")
	if err != nil {
		t.Fatalf("readCoverageVarianceFloors() error = %v", err)
	}
	if got := floors[configPackage]; !got.valid || got.label != "67.34%" {
		t.Fatalf("numeric floor = %+v, want 67.34%%", got)
	}
	if got := floors[servicePackage]; got.valid || got.label != "exception" {
		t.Fatalf("exception floor = %+v, want preserved exception", got)
	}
}

func TestExecuteWritesVarianceReportOnlyAfterAllProfilesValidate(t *testing.T) {
	root := t.TempDir()
	paths := make([]string, 0, minimumVarianceSamples)
	for index := 0; index < minimumVarianceSamples; index++ {
		path := filepath.Join(root, fmt.Sprintf("profile-%02d.out", index))
		writeVarianceProfile(t, path, map[string]variancePackageFixture{
			modulePath + "/pkg/config": {total: 3, covered: index%3 + 1},
		})
		paths = append(paths, path)
	}
	output := filepath.Join(root, "variance.md")
	if err := execute(config{
		suite:            "functional",
		varianceProfiles: strings.Join(paths, ","),
		varianceOutput:   output,
		varianceCommit:   "913a667d22f020a2f89580839688d0c659e7fe3b",
	}); err != nil {
		t.Fatalf("execute() error = %v", err)
	}
	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatalf("read variance report: %v", err)
	}
	if !strings.Contains(string(data), "Run count:** 5 complete profiles") {
		t.Fatalf("variance report = %s, want five-profile summary", data)
	}
}

func writeVarianceProfile(t *testing.T, path string, packages map[string]variancePackageFixture) {
	t.Helper()
	packageNames := make([]string, 0, len(packages))
	for importPath := range packages {
		packageNames = append(packageNames, importPath)
	}
	slices.Sort(packageNames)
	lines := []string{"mode: count"}
	for _, importPath := range packageNames {
		fixture := packages[importPath]
		suffix := strings.TrimPrefix(importPath, modulePath+"/")
		filePath := modulePath + "/" + suffix + "/fixture.go"
		for block := 0; block < fixture.total; block++ {
			executionCount := 0
			if block < fixture.covered {
				executionCount = 1
			}
			lineNumber := block + 1
			lines = append(lines, fmt.Sprintf("%s:%d.1,%d.2 1 %d", filePath, lineNumber, lineNumber, executionCount))
		}
	}
	if err := os.WriteFile(path, []byte(strings.Join(append(lines, ""), "\n")), 0o600); err != nil {
		t.Fatalf("write profile %s: %v", path, err)
	}
}
