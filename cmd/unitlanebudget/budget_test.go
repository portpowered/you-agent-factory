package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestRetainedV1BudgetRemainsSchemaValidAndHistoricallyPopulated(t *testing.T) {
	repositoryRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("repository root: %v", err)
	}
	schemaPath := filepath.Join(repositoryRoot, filepath.FromSlash(latencyBudgetSchemaPath))
	budgetPath := filepath.Join(repositoryRoot, filepath.FromSlash("docs/internal/baselines/go-unit-lane-latency-budget.v1.json"))
	data, err := os.ReadFile(budgetPath)
	if err != nil {
		t.Fatalf("read populated budget: %v", err)
	}
	if err := validateLatencyBudgetDocument(schemaPath, data); err != nil {
		t.Fatalf("validate populated budget against Draft 2020-12 schema: %v", err)
	}
	var document struct {
		Version   int `json:"version"`
		Reference struct {
			MedianWallSeconds float64  `json:"medianWallSeconds"`
			PackageInventory  []string `json:"packageInventory"`
			TestInventory     []string `json:"testInventory"`
		} `json:"reference"`
	}
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatalf("decode retained v1 budget: %v", err)
	}
	const reviewedCurrentHeadTestCount = 18156
	if document.Version != 1 || document.Reference.MedianWallSeconds != 239.612 || len(document.Reference.PackageInventory) != 444 || len(document.Reference.TestInventory) != reviewedCurrentHeadTestCount {
		t.Fatalf("retained v1 budget reference = version %d, median %.3f, packages %d, tests %d; want 1/239.612/444/%d", document.Version, document.Reference.MedianWallSeconds, len(document.Reference.PackageInventory), len(document.Reference.TestInventory), reviewedCurrentHeadTestCount)
	}
}

func TestValidateBaselineAcceptsThreeComparableSamples(t *testing.T) {
	samples := comparableSamples(10, 10, 10)
	report, err := validateBaseline(samples)
	if err != nil {
		t.Fatalf("validateBaseline() error = %v", err)
	}
	if report.Mode != "baseline" || report.MedianWallSeconds != 10 || report.PackageCount != 2 || report.TestCount != 3 {
		t.Fatalf("baseline report = %+v, want median/inventory summary", report)
	}
}

func TestDecodeJSONFileRejectsTrailingDocuments(t *testing.T) {
	path := t.TempDir() + "/sample.json"
	if err := writeText(path, "{} {}\n"); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	var sample timingSummary
	err := decodeJSONFile(path, &sample)
	if err == nil || !strings.Contains(err.Error(), "multiple documents") {
		t.Fatalf("decodeJSONFile() error = %v, want multiple-document failure", err)
	}
}

func TestSplitSamplePathsRejectsDuplicateEvidencePaths(t *testing.T) {
	_, err := splitSamplePaths("one.json,two.json,one.json")
	if err == nil || !strings.Contains(err.Error(), "expected distinct path") {
		t.Fatalf("splitSamplePaths() error = %v, want duplicate-path failure", err)
	}
	_, err = splitSamplePaths("one.json,./one.json,three.json")
	if err == nil || !strings.Contains(err.Error(), "expected distinct path") {
		t.Fatalf("splitSamplePaths() alias error = %v, want duplicate-path failure", err)
	}
}

func TestSplitSamplePathsRejectsWrongSampleCountBeforeReadingFiles(t *testing.T) {
	for _, raw := range []string{"one.json,two.json", "one.json,two.json,three.json,four.json"} {
		_, err := splitSamplePaths(raw)
		if err == nil || !strings.Contains(err.Error(), "sample count: expected exactly 3") {
			t.Fatalf("splitSamplePaths(%q) error = %v, want exact sample-count failure", raw, err)
		}
	}
}

func TestValidateSampleSetRequiresExactlyThreeSamples(t *testing.T) {
	for _, count := range []int{2, 4} {
		t.Run(fmt.Sprintf("%d samples", count), func(t *testing.T) {
			_, err := validateBaseline(comparableSamples(makeWalls(count)...))
			if err == nil || !strings.Contains(err.Error(), "sample count: expected exactly 3") {
				t.Fatalf("validateBaseline() error = %v, want exact sample-count failure", err)
			}
		})
	}
}

func TestLoadTimingSamplesRetainsMissingInputAndFailsClosed(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing.v2.json")
	_, err := loadTimingSamples([]string{missing, missing + ".second", missing + ".third"})
	if err == nil || !strings.Contains(err.Error(), missing) {
		t.Fatalf("loadTimingSamples() error = %v, want missing path diagnostic", err)
	}
}

func TestReferenceScaleCheckerOverhead(t *testing.T) {
	base := syntheticReferenceScaleSample(100)
	referenceCommit := strings.Repeat("a", 40)
	candidateCommit := strings.Repeat("b", 40)
	historical := make([]timingSummary, requiredSamples)
	reference := make([]timingSummary, requiredSamples)
	candidate := make([]timingSummary, requiredSamples)
	for index := range historical {
		historical[index] = cloneTimingSummary(t, base)
		historical[index].Run.Commit = referenceCommit
		reference[index] = cloneTimingSummary(t, base)
		reference[index].Run.Commit = referenceCommit
		candidate[index] = cloneTimingSummary(t, base)
		candidate[index].Run.Commit = candidateCommit
		candidate[index].WallSeconds = 70
	}
	inventoryHash := inventorySHA256(reference[0])
	budget := latencyBudget{
		Version:    latencyBudgetVersionV2,
		Owner:      "backend-unit-lane",
		Entrypoint: canonicalTimingEntrypoint,
		HistoricalReference: historicalReference{
			BaseCommit:         strings.Repeat("c", 40),
			MeasurementCommit:  referenceCommit,
			Runner:             reference[0].Run.Runner,
			GoVersion:          reference[0].Run.GoVersion,
			UnitDefaultJobs:    reference[0].Run.UnitDefaultJobs,
			ComputedLaneBudget: reference[0].Run.ComputedLaneBudget,
			Samples:            []float64{100, 100, 100},
			MedianWallSeconds:  100,
			PackageCount:       base.PackageCount,
			TestCount:          base.TestCount,
			InventorySHA256:    inventoryHash,
		},
		ReferenceCI: cohortExpectation{
			Commit:          referenceCommit,
			PackageCount:    base.PackageCount,
			TestCount:       base.TestCount,
			InventorySHA256: inventoryHash,
		},
		Candidate: candidateExpectation{
			InventorySource: "fixture-reconciliation.v1.json",
			PackageCount:    base.PackageCount,
			TestCount:       base.TestCount,
			InventorySHA256: inventoryHash,
		},
		Policy: budgetPolicy{
			RequiredConsecutiveSamples:   requiredSamples,
			MinimumImprovementPercent:    minimumImprovementPercent,
			MaximumRunAboveMedianPercent: maximumRunAboveMedianPercent,
			RequiredCachedPackages:       requiredCachedPackages,
			RequiredUnknownPackages:      requiredUnknownPackages,
			RequiredRunnerIdentityFields: append([]string(nil), identityFieldNames...),
			InventoryPolicy:              "exact-with-reviewed-diff",
			InvalidSamplePolicy:          "retain-and-fail-unless-predeclared-invalidation-matches",
		},
	}
	start := time.Now()
	report, problems := evaluateReferenceCI(budget, historical, reference, candidate, candidateCommit)
	if err := problems.err(); err != nil {
		t.Fatalf("validate synthetic reference-scale cohorts: %v", err)
	}
	if report.ReferenceMedianSeconds != 100 || report.MedianWallSeconds != 70 {
		t.Fatalf("reference-scale report = %+v, want live medians 100/70", report)
	}
	if report := renderBudgetReport(budgetReport{Mode: "baseline", SampleWalls: []float64{70, 70, 70}, MedianWallSeconds: 70, PackageCount: 444, TestCount: 18122}); report == "" {
		t.Fatal("render synthetic reference-scale report returned empty output")
	}
	elapsed := time.Since(start)
	t.Logf("GATE-OVERHEAD-002 elapsed=%s reference=239.612s threshold=2.396s attempts=1", elapsed)
	if elapsed >= 2396*time.Millisecond {
		t.Fatalf("reference-scale capture/check overhead = %s, want < 2.396s", elapsed)
	}
}

func TestInventoriesAreSortedAndNamespaced(t *testing.T) {
	sample := comparableSamples(1, 1, 1)[0]
	packages, tests := inventories(sample)
	if !slices.Equal(packages, []string{"example.test/pkg/alpha", "example.test/pkg/beta"}) {
		t.Fatalf("packages = %v, want sorted package inventory", packages)
	}
	if !slices.Equal(tests, []string{"example.test/pkg/alpha::TestAlpha", "example.test/pkg/beta::TestBeta", "example.test/pkg/beta::TestBeta/sub"}) {
		t.Fatalf("tests = %v, want namespaced sorted test inventory", tests)
	}
}

func comparableSamples(walls ...float64) []timingSummary {
	result := make([]timingSummary, len(walls))
	for index, wall := range walls {
		result[index] = timingSummary{
			Version:                  unitTimingVersion,
			Complete:                 true,
			Run:                      comparableRun(),
			WallSeconds:              wall,
			PackageElapsedSecondsSum: 2,
			PackageCount:             2,
			ExpectedPackageCount:     2,
			TestCount:                3,
			Packages: []timingPackage{
				{Package: "example.test/pkg/alpha", Seconds: 1, Outcome: unitTimingOutcomePass, Cache: unitTimingCacheExecuted, Tests: []string{"TestAlpha"}},
				{Package: "example.test/pkg/beta", Seconds: 1, Outcome: unitTimingOutcomePass, Cache: unitTimingCacheExecuted, Tests: []string{"TestBeta", "TestBeta/sub"}},
			},
		}
	}
	return result
}

func cloneTimingSummary(t *testing.T, source timingSummary) timingSummary {
	t.Helper()
	payload, err := json.Marshal(source)
	if err != nil {
		t.Fatalf("marshal timing fixture: %v", err)
	}
	var clone timingSummary
	if err := decodeJSONBytes(payload, &clone); err != nil {
		t.Fatalf("decode timing fixture: %v", err)
	}
	return clone
}

func comparableRun() timingRun {
	return timingRun{
		Commit:             strings.Repeat("a", 40),
		GoVersion:          "go1.25.0",
		Command:            "make test-unit-fresh UNIT_DEFAULT_JOBS=2 UNIT_TIMING_OUTPUT=run.v2.json",
		UnitDefaultJobs:    2,
		ComputedLaneBudget: 2,
		Runner: timingRunner{
			Provider: "github-actions", Image: "ubuntu-24.04", ImageVersion: "20260824.1", OS: "linux", Architecture: "amd64", CPUModel: "test-cpu",
		},
		EnvironmentInvalidations: []string{},
	}
}

func writeText(path, contents string) error {
	return os.WriteFile(path, []byte(contents), 0o600)
}

func makeWalls(count int) []float64 {
	walls := make([]float64, count)
	for index := range walls {
		walls[index] = float64(index + 1)
	}
	return walls
}

func syntheticReferenceScaleSample(wall float64) timingSummary {
	const packageCount = 444
	const testCount = 18122
	packages := make([]timingPackage, 0, packageCount)
	remainingTests := testCount
	for packageIndex := 0; packageIndex < packageCount; packageIndex++ {
		testsForPackage := remainingTests / (packageCount - packageIndex)
		tests := make([]string, testsForPackage)
		for testIndex := range tests {
			tests[testIndex] = fmt.Sprintf("Test%05d", testIndex)
		}
		packages = append(packages, timingPackage{
			Package: fmt.Sprintf("example.test/pkg/%03d", packageIndex),
			Seconds: 0.001,
			Outcome: unitTimingOutcomePass,
			Cache:   unitTimingCacheExecuted,
			Tests:   tests,
		})
		remainingTests -= testsForPackage
	}
	return timingSummary{
		Version:                  unitTimingVersion,
		Complete:                 true,
		Run:                      comparableRun(),
		WallSeconds:              wall,
		PackageElapsedSecondsSum: 0.444,
		PackageCount:             packageCount,
		ExpectedPackageCount:     packageCount,
		TestCount:                testCount,
		Packages:                 packages,
	}
}
