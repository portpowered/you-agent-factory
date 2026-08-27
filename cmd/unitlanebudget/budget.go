package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

const (
	unitTimingVersion            = 2
	requiredSamples              = 3
	minimumImprovementPercent    = 25.0
	maximumRunAboveMedianPercent = 10.0
	requiredCachedPackages       = 0
	requiredUnknownPackages      = 0
	unitTimingCacheExecuted      = "executed"
	unitTimingCacheCached        = "cached"
	unitTimingCacheUnknown       = "unknown"
	unitTimingOutcomePass        = "pass"
	canonicalTimingEntrypoint    = "make test-unit-fresh"
	latencyBudgetSchemaPath      = "docs/internal/baselines/go-unit-lane-latency-budget.schema.json"
)

type timingRunner struct {
	Provider     string `json:"provider"`
	Image        string `json:"image"`
	ImageVersion string `json:"imageVersion"`
	OS           string `json:"os"`
	Architecture string `json:"architecture"`
	CPUModel     string `json:"cpuModel"`
}

type timingRun struct {
	Commit                   string       `json:"commit"`
	GoVersion                string       `json:"goVersion"`
	Command                  string       `json:"command"`
	UnitDefaultJobs          int          `json:"unitDefaultJobs"`
	ComputedLaneBudget       int          `json:"computedLaneBudget"`
	Runner                   timingRunner `json:"runner"`
	EnvironmentInvalidations []string     `json:"environmentInvalidations"`
}

type timingPackage struct {
	Package string   `json:"package"`
	Seconds float64  `json:"seconds"`
	Outcome string   `json:"outcome"`
	Cache   string   `json:"cache"`
	Tests   []string `json:"tests"`
}

type timingSummary struct {
	Version                  int             `json:"version"`
	Complete                 bool            `json:"complete"`
	Run                      timingRun       `json:"run"`
	WallSeconds              float64         `json:"wallSeconds"`
	PackageElapsedSecondsSum float64         `json:"packageElapsedSecondsSum"`
	PackageCount             int             `json:"packageCount"`
	ExpectedPackageCount     int             `json:"expectedPackageCount"`
	TestCount                int             `json:"testCount"`
	Packages                 []timingPackage `json:"packages"`
}

type latencyBudget struct {
	Version             int                  `json:"version"`
	Owner               string               `json:"owner"`
	Entrypoint          string               `json:"entrypoint"`
	Reference           budgetReference      `json:"reference"`
	HistoricalReference historicalReference  `json:"historicalReference"`
	ReferenceCI         cohortExpectation    `json:"referenceCI"`
	Candidate           candidateExpectation `json:"candidate"`
	Policy              budgetPolicy         `json:"policy"`
}

type historicalReference struct {
	BaseCommit         string       `json:"baseCommit"`
	MeasurementCommit  string       `json:"measurementCommit"`
	Runner             timingRunner `json:"runner"`
	GoVersion          string       `json:"goVersion"`
	UnitDefaultJobs    int          `json:"unitDefaultJobs"`
	ComputedLaneBudget int          `json:"computedLaneBudget"`
	Samples            []float64    `json:"samples"`
	MedianWallSeconds  float64      `json:"medianWallSeconds"`
	PackageCount       int          `json:"packageCount"`
	TestCount          int          `json:"testCount"`
	InventorySHA256    string       `json:"inventorySha256"`
}

type cohortExpectation struct {
	Commit          string `json:"commit"`
	PackageCount    int    `json:"packageCount"`
	TestCount       int    `json:"testCount"`
	InventorySHA256 string `json:"inventorySha256"`
}

type candidateExpectation struct {
	InventorySource string `json:"inventorySource"`
	PackageCount    int    `json:"packageCount"`
	TestCount       int    `json:"testCount"`
	InventorySHA256 string `json:"inventorySha256"`
}

type budgetReference struct {
	BaseCommit         string    `json:"baseCommit"`
	RunnerImage        string    `json:"runnerImage"`
	GoVersion          string    `json:"goVersion"`
	UnitDefaultJobs    int       `json:"unitDefaultJobs"`
	ComputedLaneBudget int       `json:"computedLaneBudget"`
	Samples            []float64 `json:"samples"`
	MedianWallSeconds  float64   `json:"medianWallSeconds"`
	PackageInventory   []string  `json:"packageInventory"`
	TestInventory      []string  `json:"testInventory"`
}

type budgetPolicy struct {
	RequiredConsecutiveSamples   int      `json:"requiredConsecutiveSamples"`
	MinimumImprovementPercent    float64  `json:"minimumImprovementPercent"`
	MaximumRunAboveMedianPercent float64  `json:"maximumRunAboveMedianPercent"`
	RequiredCachedPackages       int      `json:"requiredCachedPackages"`
	RequiredUnknownPackages      int      `json:"requiredUnknownPackages"`
	RequiredRunnerIdentityFields []string `json:"requiredRunnerIdentityFields"`
	InventoryPolicy              string   `json:"inventoryPolicy"`
	InvalidSamplePolicy          string   `json:"invalidSamplePolicy"`
}

type budgetReport struct {
	Mode                     string
	SampleWalls              []float64
	MedianWallSeconds        float64
	ReferenceMedianSeconds   float64
	ImprovementPercent       float64
	MaximumRunAboveMedianPct float64
	PackageCount             int
	TestCount                int
	CachedPackages           int
	UnknownPackages          int
	ManifestPath             string
}

type validationProblems struct {
	items []string
}

func (problems *validationProblems) add(format string, args ...any) {
	problems.items = append(problems.items, fmt.Sprintf(format, args...))
}

func (problems validationProblems) err() error {
	if len(problems.items) == 0 {
		return nil
	}
	lines := []string{"unit-lane latency budget validation failed:"}
	for _, item := range problems.items {
		lines = append(lines, "- "+item)
	}
	lines = append(lines, "Rerun: make test-unit-latency-budget")
	return errors.New(strings.Join(lines, "\n"))
}

func checkerError(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("unit-lane latency budget validation failed:\n- %w\nRerun: make test-unit-latency-budget", err)
}

func splitSamplePaths(raw string) ([]string, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, errors.New("samples: expected three comma-separated timing JSON paths; actual empty")
	}
	parts := strings.Split(raw, ",")
	if len(parts) != requiredSamples {
		return nil, fmt.Errorf("sample count: expected exactly %d comma-separated paths, actual %d", requiredSamples, len(parts))
	}
	paths := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for index, part := range parts {
		path := strings.TrimSpace(part)
		if path == "" {
			return nil, fmt.Errorf("sample path %d: expected nonempty path, actual empty", index+1)
		}
		identity := filepath.Clean(path)
		if absolute, err := filepath.Abs(identity); err == nil {
			identity = absolute
		}
		if _, exists := seen[identity]; exists {
			return nil, fmt.Errorf("sample path %d: expected distinct path, actual duplicate %q", index+1, path)
		}
		seen[identity] = struct{}{}
		paths = append(paths, path)
	}
	return paths, nil
}

func loadTimingSamples(paths []string) ([]timingSummary, error) {
	samples := make([]timingSummary, 0, len(paths))
	for _, path := range paths {
		var sample timingSummary
		if err := decodeJSONFile(path, &sample); err != nil {
			return nil, fmt.Errorf("sample %q: expected valid v2 JSON, actual %w", path, err)
		}
		samples = append(samples, sample)
	}
	return samples, nil
}

func loadLatencyBudget(path string) (latencyBudget, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return latencyBudget{}, fmt.Errorf("budget %q: read JSON: %w", path, err)
	}
	if err := validateLatencyBudgetDocument(budgetSchemaPathForData(path, data), data); err != nil {
		return latencyBudget{}, fmt.Errorf("budget %q: schema validation: %w", path, err)
	}
	var budget latencyBudget
	if err := decodeJSONBytes(data, &budget); err != nil {
		return budget, fmt.Errorf("budget %q: expected valid budget JSON, actual %w", path, err)
	}
	return budget, nil
}

func budgetSchemaPath(budgetPath string) string {
	name := filepath.Base(budgetPath)
	schemaName := "go-unit-lane-latency-budget.schema.json"
	if strings.Contains(name, ".v2.") {
		schemaName = "go-unit-lane-latency-budget.v2.schema.json"
	}
	adjacent := filepath.Join(filepath.Dir(budgetPath), schemaName)
	if _, err := os.Stat(adjacent); err == nil {
		return adjacent
	}
	if schemaName == "go-unit-lane-latency-budget.v2.schema.json" {
		return filepath.FromSlash("docs/internal/baselines/go-unit-lane-latency-budget.v2.schema.json")
	}
	return latencyBudgetSchemaPath
}

func budgetSchemaPathForData(budgetPath string, data []byte) string {
	var envelope struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal(data, &envelope); err == nil && envelope.Version == 2 {
		name := filepath.Join(filepath.Dir(budgetPath), "go-unit-lane-latency-budget.v2.schema.json")
		if _, err := os.Stat(name); err == nil {
			return name
		}
		return filepath.FromSlash("docs/internal/baselines/go-unit-lane-latency-budget.v2.schema.json")
	}
	return budgetSchemaPath(budgetPath)
}

func decodeJSONFile(path string, destination any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read JSON: %w", err)
	}
	return decodeJSONBytes(data, destination)
}

func decodeJSONBytes(data []byte, destination any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return fmt.Errorf("decode JSON: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return errors.New("decode JSON: multiple documents are not allowed")
		}
		return fmt.Errorf("decode JSON trailer: %w", err)
	}
	return nil
}

func validateLatencyBudgetDocument(schemaPath string, instanceData []byte) error {
	schemaData, err := os.ReadFile(schemaPath)
	if err != nil {
		return fmt.Errorf("read schema: %w", err)
	}
	schemaDocument, err := jsonschema.UnmarshalJSON(bytes.NewReader(schemaData))
	if err != nil {
		return fmt.Errorf("decode schema: %w", err)
	}
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	const resourceID = "unit-lane-latency-budget-schema"
	if err := compiler.AddResource(resourceID, schemaDocument); err != nil {
		return fmt.Errorf("register schema: %w", err)
	}
	compiled, err := compiler.Compile(resourceID)
	if err != nil {
		return fmt.Errorf("compile schema: %w", err)
	}
	instance, err := jsonschema.UnmarshalJSON(bytes.NewReader(instanceData))
	if err != nil {
		return fmt.Errorf("decode instance: %w", err)
	}
	if err := compiled.Validate(instance); err != nil {
		return fmt.Errorf("instance does not conform: %w", err)
	}
	return nil
}

func validateBaseline(samples []timingSummary) (budgetReport, error) {
	problems, details := validateSampleSet(samples)
	if err := problems.err(); err != nil {
		return budgetReport{}, err
	}
	return budgetReport{
		Mode:              "baseline",
		SampleWalls:       details.walls,
		MedianWallSeconds: details.median,
		PackageCount:      details.packageCount,
		TestCount:         details.testCount,
		CachedPackages:    details.cached,
		UnknownPackages:   details.unknown,
	}, nil
}

type sampleDetails struct {
	walls        []float64
	median       float64
	packageCount int
	testCount    int
	cached       int
	unknown      int
}

func validateSampleSet(samples []timingSummary) (validationProblems, sampleDetails) {
	var problems validationProblems
	details := sampleDetails{walls: make([]float64, 0, len(samples))}
	if len(samples) != requiredSamples {
		problems.add("sample count: expected exactly %d, actual %d", requiredSamples, len(samples))
	}
	for index, sample := range samples {
		validateTimingSummary(&problems, index+1, sample)
		details.walls = append(details.walls, sample.WallSeconds)
		cached, unknown := cacheCounts(sample.Packages)
		details.cached += cached
		details.unknown += unknown
		if index == 0 {
			details.packageCount = len(sample.Packages)
			details.testCount = sample.TestCount
			continue
		}
		compareSampleIdentity(&problems, 1, index+1, samples[0].Run, sample.Run)
		compareSampleInventory(&problems, 1, index+1, samples[0], sample)
	}
	if len(details.walls) == requiredSamples && allPositiveFinite(details.walls) {
		details.median = median(details.walls)
	}
	return problems, details
}

func validateTimingSummary(problems *validationProblems, ordinal int, sample timingSummary) {
	prefix := fmt.Sprintf("sample %d", ordinal)
	if sample.Version != unitTimingVersion {
		problems.add("%s version: expected %d, actual %d", prefix, unitTimingVersion, sample.Version)
	}
	if !sample.Complete {
		problems.add("%s complete: expected true, actual false", prefix)
	}
	if !isFinitePositive(sample.WallSeconds) {
		problems.add("%s wallSeconds: expected finite value > 0, actual %v", prefix, sample.WallSeconds)
	}
	if !isFiniteNonnegative(sample.PackageElapsedSecondsSum) {
		problems.add("%s packageElapsedSecondsSum: expected finite value >= 0, actual %v", prefix, sample.PackageElapsedSecondsSum)
	}
	validateRunIdentity(problems, prefix, sample.Run)
	if sample.PackageCount != len(sample.Packages) {
		problems.add("%s packageCount: expected %d, actual %d", prefix, len(sample.Packages), sample.PackageCount)
	}
	if sample.ExpectedPackageCount != len(sample.Packages) {
		problems.add("%s expectedPackageCount: expected %d, actual %d", prefix, len(sample.Packages), sample.ExpectedPackageCount)
	}
	if len(sample.Packages) == 0 {
		problems.add("%s packages: expected nonempty inventory, actual empty", prefix)
	}
	validatePackages(problems, prefix, sample.Packages, sample.TestCount)
	if isFiniteNonnegative(sample.PackageElapsedSecondsSum) {
		expectedPackageSum := roundedPackageElapsedSecondsSum(sample.Packages)
		if !nearlyEqual(sample.PackageElapsedSecondsSum, expectedPackageSum) {
			problems.add("%s packageElapsedSecondsSum: expected recomputed %.3f, actual %.3f", prefix, expectedPackageSum, sample.PackageElapsedSecondsSum)
		}
	}
}

func validateRunIdentity(problems *validationProblems, prefix string, run timingRun) {
	if !commitPattern.MatchString(run.Commit) {
		problems.add("%s run.commit: expected 40 lowercase hexadecimal characters, actual %q", prefix, run.Commit)
	}
	if !goVersionPattern.MatchString(run.GoVersion) {
		problems.add("%s run.goVersion: expected goX.Y.Z, actual %q", prefix, run.GoVersion)
	}
	command := normalizedTimingCommand(run.Command)
	if command == "" {
		problems.add("%s run.command: expected nonempty command, actual empty", prefix)
	} else {
		expectedCommand := canonicalTimingEntrypoint + " UNIT_DEFAULT_JOBS=" + strconv.Itoa(run.UnitDefaultJobs) + " UNIT_TIMING_OUTPUT=<timing-output>"
		if command != expectedCommand {
			problems.add("%s run.command: expected canonical %q, actual %q", prefix, expectedCommand, command)
		}
	}
	if run.UnitDefaultJobs < 1 {
		problems.add("%s run.unitDefaultJobs: expected positive value, actual %d", prefix, run.UnitDefaultJobs)
	}
	if run.ComputedLaneBudget < 1 {
		problems.add("%s run.computedLaneBudget: expected positive value, actual %d", prefix, run.ComputedLaneBudget)
	}
	if strings.TrimSpace(run.Runner.Provider) == "" || strings.TrimSpace(run.Runner.Image) == "" || strings.TrimSpace(run.Runner.ImageVersion) == "" || strings.TrimSpace(run.Runner.OS) == "" || strings.TrimSpace(run.Runner.Architecture) == "" || strings.TrimSpace(run.Runner.CPUModel) == "" {
		problems.add("%s run.runner: expected complete provider/image/version/os/architecture/cpuModel identity, actual %s", prefix, compactJSON(run.Runner))
	}
	for index, invalidation := range run.EnvironmentInvalidations {
		if strings.TrimSpace(invalidation) == "" {
			problems.add("%s run.environmentInvalidations[%d]: expected nonempty reason, actual empty", prefix, index)
		}
	}
	if len(run.EnvironmentInvalidations) > 0 {
		problems.add("%s run.environmentInvalidations: expected [] (no predeclared invalidation), actual %s", prefix, compactJSON(run.EnvironmentInvalidations))
	}
}

var (
	commitPattern       = regexp.MustCompile(`^[0-9a-f]{40}$`)
	goVersionPattern    = regexp.MustCompile(`^go[0-9]+\.[0-9]+\.[0-9]+$`)
	timingOutputPattern = regexp.MustCompile(`(?i)(unit_timing_output|--timing-output)(=|\s+)("[^"]*"|\S+)`)
)

func validatePackages(problems *validationProblems, prefix string, packages []timingPackage, testCount int) {
	names := make([]string, 0, len(packages))
	actualTests := 0
	seen := make(map[string]struct{}, len(packages))
	for _, packageTiming := range packages {
		names = append(names, packageTiming.Package)
		if strings.TrimSpace(packageTiming.Package) == "" {
			problems.add("%s package: expected nonempty name, actual empty", prefix)
		}
		if _, exists := seen[packageTiming.Package]; exists {
			problems.add("%s package inventory: expected unique names, actual duplicate %q", prefix, packageTiming.Package)
		}
		seen[packageTiming.Package] = struct{}{}
		if !isFiniteNonnegative(packageTiming.Seconds) {
			problems.add("%s package %q seconds: expected finite value >= 0, actual %v", prefix, packageTiming.Package, packageTiming.Seconds)
		}
		if packageTiming.Outcome != unitTimingOutcomePass {
			problems.add("%s package %q outcome: expected %q, actual %q", prefix, packageTiming.Package, unitTimingOutcomePass, packageTiming.Outcome)
		}
		if packageTiming.Cache != unitTimingCacheExecuted {
			problems.add("%s package %q cache: expected %q, actual %q", prefix, packageTiming.Package, unitTimingCacheExecuted, packageTiming.Cache)
		}
		if !slices.IsSorted(packageTiming.Tests) || hasDuplicates(packageTiming.Tests) {
			problems.add("%s package %q tests: expected unique sorted names, actual %s", prefix, packageTiming.Package, compactJSON(packageTiming.Tests))
		}
		for _, testName := range packageTiming.Tests {
			if strings.TrimSpace(testName) == "" {
				problems.add("%s package %q test inventory: expected nonempty name, actual empty", prefix, packageTiming.Package)
			}
		}
		actualTests += len(packageTiming.Tests)
	}
	if !slices.IsSorted(names) || hasDuplicates(names) {
		problems.add("%s package inventory order: expected unique sorted package names, actual %s", prefix, compactJSON(names))
	}
	if testCount != actualTests {
		problems.add("%s testCount: expected %d, actual %d", prefix, actualTests, testCount)
	}
}

func compareSampleIdentity(problems *validationProblems, expectedOrdinal, actualOrdinal int, expected, actual timingRun) {
	expected = normalizedRun(expected)
	actual = normalizedRun(actual)
	if !reflect.DeepEqual(expected, actual) {
		problems.add("sample %d/ sample %d identity: expected %s, actual %s", expectedOrdinal, actualOrdinal, compactJSON(expected), compactJSON(actual))
	}
}

func compareSampleInventory(problems *validationProblems, expectedOrdinal, actualOrdinal int, expected, actual timingSummary) {
	expectedPackages, expectedTests := inventories(expected)
	actualPackages, actualTests := inventories(actual)
	if !slices.Equal(expectedPackages, actualPackages) {
		problems.add("sample %d/ sample %d package inventory: expected %s, actual %s", expectedOrdinal, actualOrdinal, compactJSON(expectedPackages), compactJSON(actualPackages))
	}
	if !slices.Equal(expectedTests, actualTests) {
		problems.add("sample %d/ sample %d test inventory: expected %s, actual %s", expectedOrdinal, actualOrdinal, compactJSON(expectedTests), compactJSON(actualTests))
	}
}

func normalizedRun(run timingRun) timingRun {
	run.Command = normalizedTimingCommand(run.Command)
	if len(run.EnvironmentInvalidations) == 0 {
		run.EnvironmentInvalidations = []string{}
	}
	return run
}

func normalizedTimingCommand(command string) string {
	return strings.TrimSpace(timingOutputPattern.ReplaceAllString(command, "$1$2<timing-output>"))
}

func validateFinal(budget latencyBudget, samples []timingSummary) (budgetReport, error) {
	problems, details := validateSampleSet(samples)
	validateBudgetShape(&problems, budget)
	if len(samples) > 0 {
		validateReferenceIdentity(&problems, budget.Reference, samples[0])
		compareReferenceInventory(&problems, budget.Reference, samples[0])
	}
	if len(budget.Reference.Samples) == requiredSamples && allPositiveFinite(budget.Reference.Samples) {
		referenceMedian := median(budget.Reference.Samples)
		if !nearlyEqual(referenceMedian, budget.Reference.MedianWallSeconds) {
			problems.add("reference medianWallSeconds: expected recomputed %.3f, actual %.3f", referenceMedian, budget.Reference.MedianWallSeconds)
		}
		if details.median > 0 {
			improvement := improvementPercent(referenceMedian, details.median)
			if improvement+0.000000001 < budget.Policy.MinimumImprovementPercent {
				problems.add("median improvement: expected >= %.2f%%, actual %.2f%%", budget.Policy.MinimumImprovementPercent, improvement)
			}
			maximum := maximumRunAboveMedian(details.walls, details.median)
			if maximum > budget.Policy.MaximumRunAboveMedianPercent+0.000000001 {
				problems.add("maximum run above median: expected <= %.2f%%, actual %.2f%%", budget.Policy.MaximumRunAboveMedianPercent, maximum)
			}
			return budgetReport{Mode: "final", SampleWalls: details.walls, MedianWallSeconds: details.median, ReferenceMedianSeconds: referenceMedian, ImprovementPercent: improvement, MaximumRunAboveMedianPct: maximum, PackageCount: details.packageCount, TestCount: details.testCount, CachedPackages: details.cached, UnknownPackages: details.unknown}, problems.err()
		}
	}
	return budgetReport{}, problems.err()
}

func validateBudgetShape(problems *validationProblems, budget latencyBudget) {
	if budget.Version == 2 {
		validateV2BudgetShape(problems, budget)
		return
	}
	if budget.Version != 1 {
		problems.add("budget version: expected 1, actual %d", budget.Version)
	}
	if budget.Owner != "backend-unit-lane" {
		problems.add("budget owner: expected %q, actual %q", "backend-unit-lane", budget.Owner)
	}
	if budget.Entrypoint != "make test-unit-fresh" {
		problems.add("budget entrypoint: expected %q, actual %q", "make test-unit-fresh", budget.Entrypoint)
	}
	policy := budget.Policy
	if policy.RequiredConsecutiveSamples != requiredSamples {
		problems.add("policy requiredConsecutiveSamples: expected %d, actual %d", requiredSamples, policy.RequiredConsecutiveSamples)
	}
	if policy.MinimumImprovementPercent != minimumImprovementPercent {
		problems.add("policy minimumImprovementPercent: expected %.2f, actual %.2f", minimumImprovementPercent, policy.MinimumImprovementPercent)
	}
	if policy.MaximumRunAboveMedianPercent != maximumRunAboveMedianPercent {
		problems.add("policy maximumRunAboveMedianPercent: expected %.2f, actual %.2f", maximumRunAboveMedianPercent, policy.MaximumRunAboveMedianPercent)
	}
	if policy.RequiredCachedPackages != requiredCachedPackages || policy.RequiredUnknownPackages != requiredUnknownPackages {
		problems.add("policy cache allowances: expected cached=%d unknown=%d, actual cached=%d unknown=%d", requiredCachedPackages, requiredUnknownPackages, policy.RequiredCachedPackages, policy.RequiredUnknownPackages)
	}
	if policy.InventoryPolicy != "exact-with-reviewed-diff" {
		problems.add("policy inventoryPolicy: expected %q, actual %q", "exact-with-reviewed-diff", policy.InventoryPolicy)
	}
	if policy.InvalidSamplePolicy != "retain-and-fail-unless-predeclared-invalidation-matches" {
		problems.add("policy invalidSamplePolicy: expected declared retention policy, actual %q", policy.InvalidSamplePolicy)
	}
	validateReferenceShape(problems, budget.Reference)
}

func validateReferenceShape(problems *validationProblems, reference budgetReference) {
	if !commitPattern.MatchString(reference.BaseCommit) {
		problems.add("reference baseCommit: expected 40 lowercase hexadecimal characters, actual %q", reference.BaseCommit)
	}
	if strings.TrimSpace(reference.RunnerImage) == "" || !goVersionPattern.MatchString(reference.GoVersion) {
		problems.add("reference runner/go identity: expected nonempty runnerImage and goX.Y.Z, actual runnerImage=%q goVersion=%q", reference.RunnerImage, reference.GoVersion)
	}
	if reference.UnitDefaultJobs < 1 || reference.ComputedLaneBudget < 1 {
		problems.add("reference jobs/budget: expected positive values, actual jobs=%d budget=%d", reference.UnitDefaultJobs, reference.ComputedLaneBudget)
	}
	if len(reference.Samples) != requiredSamples || !allPositiveFinite(reference.Samples) {
		problems.add("reference samples: expected exactly three finite values > 0, actual %s", compactJSON(reference.Samples))
	}
	if !isFinitePositive(reference.MedianWallSeconds) {
		problems.add("reference medianWallSeconds: expected finite value > 0, actual %v", reference.MedianWallSeconds)
	}
	if len(reference.PackageInventory) == 0 || hasBlankStrings(reference.PackageInventory) || hasDuplicates(reference.PackageInventory) || !slices.IsSorted(reference.PackageInventory) {
		problems.add("reference packageInventory: expected nonempty unique sorted values, actual %s", compactJSON(reference.PackageInventory))
	}
	if len(reference.TestInventory) == 0 || hasBlankStrings(reference.TestInventory) || hasDuplicates(reference.TestInventory) || !slices.IsSorted(reference.TestInventory) {
		problems.add("reference testInventory: expected nonempty unique sorted values, actual %s", compactJSON(reference.TestInventory))
	}
}

func validateReferenceIdentity(problems *validationProblems, reference budgetReference, sample timingSummary) {
	if sample.Run.Runner.Image != reference.RunnerImage {
		problems.add("reference runner image: expected %q, actual %q", reference.RunnerImage, sample.Run.Runner.Image)
	}
	if sample.Run.GoVersion != reference.GoVersion {
		problems.add("reference Go version: expected %q, actual %q", reference.GoVersion, sample.Run.GoVersion)
	}
	if sample.Run.UnitDefaultJobs != reference.UnitDefaultJobs || sample.Run.ComputedLaneBudget != reference.ComputedLaneBudget {
		problems.add("reference jobs/budget: expected jobs=%d budget=%d, actual jobs=%d budget=%d", reference.UnitDefaultJobs, reference.ComputedLaneBudget, sample.Run.UnitDefaultJobs, sample.Run.ComputedLaneBudget)
	}
}

func compareReferenceInventory(problems *validationProblems, reference budgetReference, sample timingSummary) {
	packages, tests := inventories(sample)
	if !slices.Equal(reference.PackageInventory, packages) {
		problems.add("reference package inventory: expected %s, actual %s", compactJSON(reference.PackageInventory), compactJSON(packages))
	}
	if !slices.Equal(reference.TestInventory, tests) {
		problems.add("reference test inventory: expected %s, actual %s", compactJSON(reference.TestInventory), compactJSON(tests))
	}
}

func inventories(sample timingSummary) ([]string, []string) {
	packages := make([]string, 0, len(sample.Packages))
	tests := make([]string, 0, sample.TestCount)
	for _, packageTiming := range sample.Packages {
		packages = append(packages, packageTiming.Package)
		for _, testName := range packageTiming.Tests {
			tests = append(tests, packageTiming.Package+"::"+testName)
		}
	}
	slices.Sort(packages)
	slices.Sort(tests)
	return packages, tests
}

func cacheCounts(packages []timingPackage) (int, int) {
	cached, unknown := 0, 0
	for _, packageTiming := range packages {
		switch packageTiming.Cache {
		case unitTimingCacheCached:
			cached++
		case unitTimingCacheUnknown:
			unknown++
		}
	}
	return cached, unknown
}

func roundedPackageElapsedSecondsSum(packages []timingPackage) float64 {
	total := 0.0
	for _, packageTiming := range packages {
		total += packageTiming.Seconds
	}
	return math.Round(total*1000) / 1000
}

func renderBudgetReport(report budgetReport) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "Unit-lane latency budget %s\n", report.Mode)
	fmt.Fprintf(&builder, "Samples: %s\n", compactJSON(report.SampleWalls))
	fmt.Fprintf(&builder, "Median wall time: %.3fs\n", report.MedianWallSeconds)
	if report.ReferenceMedianSeconds > 0 {
		fmt.Fprintf(&builder, "Reference median wall time: %.3fs\n", report.ReferenceMedianSeconds)
		fmt.Fprintf(&builder, "Median improvement: %.2f%%\n", report.ImprovementPercent)
		fmt.Fprintf(&builder, "Maximum run above median: %.2f%%\n", report.MaximumRunAboveMedianPct)
	}
	fmt.Fprintf(&builder, "Inventory: %d packages, %d tests\n", report.PackageCount, report.TestCount)
	fmt.Fprintf(&builder, "Cache: %d cached, %d unknown\n", report.CachedPackages, report.UnknownPackages)
	if report.ManifestPath != "" {
		fmt.Fprintf(&builder, "Manifest: %s\n", report.ManifestPath)
	}
	builder.WriteString("Result: pass\n")
	return builder.String()
}

func median(values []float64) float64 {
	ordered := append([]float64(nil), values...)
	sort.Float64s(ordered)
	return ordered[len(ordered)/2]
}

func improvementPercent(reference, candidate float64) float64 {
	return (reference - candidate) / reference * 100
}

func maximumRunAboveMedian(walls []float64, medianWall float64) float64 {
	maximum := 0.0
	for _, wall := range walls {
		above := (wall - medianWall) / medianWall * 100
		if above > maximum {
			maximum = above
		}
	}
	return maximum
}

func compactJSON(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf("<unrenderable: %v>", err)
	}
	return string(data)
}

func hasDuplicates(values []string) bool {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if _, exists := seen[value]; exists {
			return true
		}
		seen[value] = struct{}{}
	}
	return false
}

func hasBlankStrings(values []string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			return true
		}
	}
	return false
}

func allPositiveFinite(values []float64) bool {
	for _, value := range values {
		if !isFinitePositive(value) {
			return false
		}
	}
	return true
}

func isFinitePositive(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value > 0
}

func isFiniteNonnegative(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= 0
}

func nearlyEqual(left, right float64) bool {
	return math.Abs(left-right) <= 0.000000001
}
