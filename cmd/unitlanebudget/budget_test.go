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

func TestCommittedBudgetSchemaAndInstancePassDraft202012Compiler(t *testing.T) {
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
	budget, err := loadLatencyBudget(budgetPath)
	if err != nil {
		t.Fatalf("load populated budget: %v", err)
	}
	var problems validationProblems
	validateBudgetShape(&problems, budget)
	if err := problems.err(); err != nil {
		t.Fatalf("populated budget semantic validation: %v", err)
	}
	// The retained timing samples remain the historical baseline.
	// Final mode uses the reviewed current-head inventory from the latest
	// complete Ubuntu unit-lane capture.
	const reviewedCurrentHeadTestCount = 18282
	if budget.Reference.MedianWallSeconds != 239.612 || len(budget.Reference.PackageInventory) != 446 || len(budget.Reference.TestInventory) != reviewedCurrentHeadTestCount {
		t.Fatalf("loaded budget reference = median %.3f, packages %d, tests %d; want 239.612/446/%d", budget.Reference.MedianWallSeconds, len(budget.Reference.PackageInventory), len(budget.Reference.TestInventory), reviewedCurrentHeadTestCount)
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

func TestValidateFinalRecomputesMedianAndImprovement(t *testing.T) {
	budget := comparableBudget([]float64{10, 11, 12}, 11)
	samples := comparableSamples(7, 7, 7)
	for index := range samples {
		samples[index].Run.Commit = strings.Repeat("b", 40)
	}
	report, err := validateFinal(budget, samples)
	if err != nil {
		t.Fatalf("validateFinal() error = %v", err)
	}
	if report.ReferenceMedianSeconds != 11 || report.MedianWallSeconds != 7 {
		t.Fatalf("report medians = %+v, want reference 11 and candidate 7", report)
	}
	if report.ImprovementPercent != improvementPercent(11, 7) || report.MaximumRunAboveMedianPct != 0 {
		t.Fatalf("report percentages = %+v, want recomputed values", report)
	}
}

func TestValidateFinalAcceptsInclusiveThresholds(t *testing.T) {
	budget := comparableBudget([]float64{100, 100, 100}, 100)
	samples := comparableSamples(75, 75, 82.5)
	for index := range samples {
		samples[index].Run.Commit = strings.Repeat("b", 40)
	}
	report, err := validateFinal(budget, samples)
	if err != nil {
		t.Fatalf("validateFinal() error = %v, want inclusive boundaries to pass", err)
	}
	if !nearlyEqual(report.ImprovementPercent, 25) || !nearlyEqual(report.MaximumRunAboveMedianPct, 10) {
		t.Fatalf("threshold report = %+v, want exactly 25%% improvement and 10%% spread", report)
	}
}

func TestValidateFinalAcceptsReviewedInventoryUpdate(t *testing.T) {
	samples := comparableSamples(70, 70, 70)
	for index := range samples {
		samples[index].Run.Commit = strings.Repeat("b", 40)
		samples[index].Packages[0].Package = "example.test/pkg/gamma"
		samples[index].Packages[0].Tests = []string{"TestGamma"}
		samples[index].Packages[0], samples[index].Packages[1] = samples[index].Packages[1], samples[index].Packages[0]
	}
	budget := comparableBudget([]float64{100, 100, 100}, 100)
	budget.Reference.PackageInventory, budget.Reference.TestInventory = inventories(samples[0])
	if _, err := validateFinal(budget, samples); err != nil {
		t.Fatalf("validateFinal() error = %v, want reviewed inventory update to be accepted", err)
	}
}

func TestValidateFinalRejectsEveryMaterialInvalidState(t *testing.T) {
	baseBudget := comparableBudget([]float64{10, 10, 10}, 10)
	cases := []struct {
		name   string
		mutate func([]timingSummary)
		want   string
	}{
		{name: "incomplete", mutate: func(samples []timingSummary) { samples[0].Complete = false }, want: "complete: expected true, actual false"},
		{name: "cached", mutate: func(samples []timingSummary) { samples[0].Packages[0].Cache = unitTimingCacheCached }, want: "cache: expected \"executed\", actual \"cached\""},
		{name: "unknown cache", mutate: func(samples []timingSummary) { samples[0].Packages[0].Cache = unitTimingCacheUnknown }, want: "cache: expected \"executed\", actual \"unknown\""},
		{name: "failed outcome", mutate: func(samples []timingSummary) { samples[0].Packages[0].Outcome = "fail" }, want: "outcome: expected \"pass\", actual \"fail\""},
		{name: "identity drift", mutate: func(samples []timingSummary) { samples[1].Run.Commit = strings.Repeat("c", 40) }, want: "identity: expected"},
		{name: "runner drift", mutate: func(samples []timingSummary) { samples[1].Run.Runner.ImageVersion = "different-runner" }, want: "identity: expected"},
		{name: "runner provider drift", mutate: func(samples []timingSummary) { samples[1].Run.Runner.Provider = "different-provider" }, want: "identity: expected"},
		{name: "runner OS drift", mutate: func(samples []timingSummary) { samples[1].Run.Runner.OS = "windows" }, want: "identity: expected"},
		{name: "runner architecture drift", mutate: func(samples []timingSummary) { samples[1].Run.Runner.Architecture = "arm64" }, want: "identity: expected"},
		{name: "runner CPU drift", mutate: func(samples []timingSummary) { samples[1].Run.Runner.CPUModel = "different-cpu" }, want: "identity: expected"},
		{name: "toolchain drift", mutate: func(samples []timingSummary) { samples[1].Run.GoVersion = "go1.26.0" }, want: "identity: expected"},
		{name: "jobs drift", mutate: func(samples []timingSummary) { samples[1].Run.UnitDefaultJobs = 3 }, want: "identity: expected"},
		{name: "computed budget drift", mutate: func(samples []timingSummary) { samples[1].Run.ComputedLaneBudget = 3 }, want: "identity: expected"},
		{name: "invalidation", mutate: func(samples []timingSummary) {
			samples[0].Run.EnvironmentInvalidations = []string{"host capability unavailable"}
		}, want: "environmentInvalidations: expected []"},
		{name: "inventory drift", mutate: func(samples []timingSummary) { samples[1].Packages[0].Tests[0] = "TestDifferent" }, want: "test inventory: expected"},
		{name: "missing package inventory", mutate: func(samples []timingSummary) { samples[0].Packages = samples[0].Packages[:1] }, want: "packageCount: expected 1, actual 2"},
		{name: "duplicate package inventory", mutate: func(samples []timingSummary) { samples[0].Packages[1].Package = samples[0].Packages[0].Package }, want: "package inventory: expected unique names"},
		{name: "duplicate test inventory", mutate: func(samples []timingSummary) { samples[0].Packages[1].Tests[1] = samples[0].Packages[1].Tests[0] }, want: "tests: expected unique sorted"},
		{name: "reordered package inventory", mutate: func(samples []timingSummary) {
			samples[0].Packages[0], samples[0].Packages[1] = samples[0].Packages[1], samples[0].Packages[0]
		}, want: "package inventory order: expected unique sorted"},
		{name: "reordered test inventory", mutate: func(samples []timingSummary) {
			samples[0].Packages[1].Tests[0], samples[0].Packages[1].Tests[1] = samples[0].Packages[1].Tests[1], samples[0].Packages[1].Tests[0]
		}, want: "tests: expected unique sorted"},
		{name: "direct unitlane command", mutate: func(samples []timingSummary) { samples[0].Run.Command = "go run ./cmd/unitlane -jobs 2 -count=1" }, want: "run.command: expected canonical"},
		{name: "package sum drift", mutate: func(samples []timingSummary) { samples[0].PackageElapsedSecondsSum = 999 }, want: "packageElapsedSecondsSum: expected recomputed"},
		{name: "below improvement", mutate: func(samples []timingSummary) {
			for i := range samples {
				samples[i].WallSeconds = 8
			}
		}, want: "median improvement: expected >= 25.00%, actual 20.00%"},
		{name: "above spread", mutate: func(samples []timingSummary) {
			samples[0].WallSeconds = 7
			samples[1].WallSeconds = 7
			samples[2].WallSeconds = 9
		}, want: "maximum run above median: expected <= 10.00%"},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			samples := comparableSamples(7, 7, 7)
			for index := range samples {
				samples[index].Run.Commit = strings.Repeat("b", 40)
			}
			testCase.mutate(samples)
			_, err := validateFinal(baseBudget, samples)
			if err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("validateFinal() error = %v, want substring %q", err, testCase.want)
			}
			if !strings.Contains(err.Error(), "expected") || !strings.Contains(err.Error(), "actual") || !strings.Contains(err.Error(), "Rerun: make test-unit-latency-budget") {
				t.Fatalf("validation diagnostics = %v, want expected/actual and rerun command", err)
			}
		})
	}
}

func TestValidateFinalRejectsReferenceIdentityDrift(t *testing.T) {
	for name, mutate := range map[string]func(*latencyBudget){
		"runner image": func(budget *latencyBudget) { budget.Reference.RunnerImage = "different-image" },
		"Go version":   func(budget *latencyBudget) { budget.Reference.GoVersion = "go1.26.0" },
		"jobs":         func(budget *latencyBudget) { budget.Reference.UnitDefaultJobs = 3 },
		"lane budget":  func(budget *latencyBudget) { budget.Reference.ComputedLaneBudget = 3 },
	} {
		t.Run(name, func(t *testing.T) {
			budget := comparableBudget([]float64{100, 100, 100}, 100)
			mutate(&budget)
			samples := comparableSamples(70, 70, 70)
			for index := range samples {
				samples[index].Run.Commit = strings.Repeat("b", 40)
			}
			_, err := validateFinal(budget, samples)
			if err == nil || !strings.Contains(err.Error(), "reference") {
				t.Fatalf("validateFinal() error = %v, want reference identity failure", err)
			}
		})
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
	base := syntheticReferenceScaleSample(70)
	budget := comparableBudget([]float64{100, 100, 100}, 100)
	budget.Reference.PackageInventory, budget.Reference.TestInventory = inventories(base)
	start := time.Now()
	samples := make([]timingSummary, 0, requiredSamples)
	for index := 0; index < requiredSamples; index++ {
		payload, err := json.Marshal(base)
		if err != nil {
			t.Fatalf("marshal reference-scale sample %d: %v", index+1, err)
		}
		var sample timingSummary
		if err := decodeJSONBytes(payload, &sample); err != nil {
			t.Fatalf("decode synthetic sample %d: %v", index+1, err)
		}
		samples = append(samples, sample)
	}
	if _, err := validateFinal(budget, samples); err != nil {
		t.Fatalf("validate synthetic reference-scale samples: %v", err)
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

func comparableBudget(samples []float64, medianWall float64) latencyBudget {
	return latencyBudget{
		Version:    1,
		Owner:      "backend-unit-lane",
		Entrypoint: "make test-unit-fresh",
		Reference: budgetReference{
			BaseCommit:         strings.Repeat("a", 40),
			RunnerImage:        "ubuntu-24.04",
			GoVersion:          "go1.25.0",
			UnitDefaultJobs:    2,
			ComputedLaneBudget: 2,
			Samples:            samples,
			MedianWallSeconds:  medianWall,
			PackageInventory:   []string{"example.test/pkg/alpha", "example.test/pkg/beta"},
			TestInventory:      []string{"example.test/pkg/alpha::TestAlpha", "example.test/pkg/beta::TestBeta", "example.test/pkg/beta::TestBeta/sub"},
		},
		Policy: budgetPolicy{
			RequiredConsecutiveSamples:   3,
			MinimumImprovementPercent:    25,
			MaximumRunAboveMedianPercent: 10,
			RequiredCachedPackages:       0,
			RequiredUnknownPackages:      0,
			InventoryPolicy:              "exact-with-reviewed-diff",
			InvalidSamplePolicy:          "retain-and-fail-unless-predeclared-invalidation-matches",
		},
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
