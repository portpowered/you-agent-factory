package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

const (
	latencyBudgetVersionV2     = 2
	referenceCIManifestVersion = 1
	referenceCIManifestSchema  = "docs/internal/baselines/go-unit-lane-reference-ci-evidence.schema.json"
	unknownManifestValue       = "unknown"
	emptyFileSHA256            = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
)

var (
	sha256Pattern      = regexp.MustCompile(`^[0-9a-f]{64}$`)
	runOrdinalPattern  = regexp.MustCompile(`(?:^|[/\\_-])run-([0-9]+)(?:[.\\/_-]|$)`)
	identityFieldNames = []string{"provider", "image", "imageVersion", "os", "architecture", "cpuModel"}
)

type timingEvidence struct {
	Path    string
	Bytes   []byte
	Hash    string
	Summary timingSummary
	Loaded  bool
}

type referenceCIManifest struct {
	Schema                         string         `json:"schema"`
	Version                        int            `json:"version"`
	Status                         string         `json:"status"`
	ReferenceCommit                string         `json:"referenceCommit"`
	CandidateCommit                string         `json:"candidateCommit"`
	Runner                         timingRunner   `json:"runner"`
	GoVersion                      string         `json:"goVersion"`
	UnitDefaultJobs                int            `json:"unitDefaultJobs"`
	ComputedLaneBudget             int            `json:"computedLaneBudget"`
	Reference                      manifestCohort `json:"reference"`
	Candidate                      manifestCohort `json:"candidate"`
	MinimumImprovementPercent      float64        `json:"minimumImprovementPercent"`
	ActualImprovementPercent       float64        `json:"actualImprovementPercent"`
	MaximumRunAboveMedianPercent   float64        `json:"maximumRunAboveMedianPercent"`
	ActualMaximumRunAboveMedianPct float64        `json:"actualMaximumRunAboveMedianPercent"`
	Diagnostics                    []string       `json:"diagnostics"`
}

type manifestSample struct {
	Ordinal     int     `json:"ordinal"`
	Path        string  `json:"path"`
	Command     string  `json:"command"`
	SHA256      string  `json:"sha256"`
	WallSeconds float64 `json:"wallSeconds"`
}

type manifestCohort struct {
	Samples         []manifestSample `json:"samples"`
	MedianWall      float64          `json:"medianWallSeconds"`
	PackageCount    int              `json:"packageCount"`
	TestCount       int              `json:"testCount"`
	InventorySHA256 string           `json:"inventorySha256"`
	CachedPackages  int              `json:"cachedPackages"`
	UnknownPackages int              `json:"unknownPackages"`
}

func validateV2BudgetShape(problems *validationProblems, budget latencyBudget) {
	if budget.Version != latencyBudgetVersionV2 {
		problems.add("budget version: expected %d, actual %d", latencyBudgetVersionV2, budget.Version)
	}
	if budget.Owner != "backend-unit-lane" {
		problems.add("budget owner: expected %q, actual %q", "backend-unit-lane", budget.Owner)
	}
	if budget.Entrypoint != canonicalTimingEntrypoint {
		problems.add("budget entrypoint: expected %q, actual %q", canonicalTimingEntrypoint, budget.Entrypoint)
	}
	validateHistoricalReferenceShape(problems, budget.HistoricalReference)
	validateCohortExpectationShape(problems, "referenceCI", budget.ReferenceCI)
	if strings.TrimSpace(budget.Candidate.InventorySource) == "" {
		problems.add("candidate inventorySource: expected nonempty path, actual %q", budget.Candidate.InventorySource)
	}
	validateCountAndHash(problems, "candidate", budget.Candidate.PackageCount, budget.Candidate.TestCount, budget.Candidate.InventorySHA256)

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
	if !slices.Equal(policy.RequiredRunnerIdentityFields, identityFieldNames) {
		problems.add("policy requiredRunnerIdentityFields: expected %s, actual %s", compactJSON(identityFieldNames), compactJSON(policy.RequiredRunnerIdentityFields))
	}
	if policy.InventoryPolicy != "exact-with-reviewed-diff" {
		problems.add("policy inventoryPolicy: expected %q, actual %q", "exact-with-reviewed-diff", policy.InventoryPolicy)
	}
	if policy.InvalidSamplePolicy != "retain-and-fail-unless-predeclared-invalidation-matches" {
		problems.add("policy invalidSamplePolicy: expected declared retention policy, actual %q", policy.InvalidSamplePolicy)
	}
}

func validateHistoricalReferenceShape(problems *validationProblems, reference historicalReference) {
	if !commitPattern.MatchString(reference.BaseCommit) {
		problems.add("historicalReference baseCommit: expected 40 lowercase hexadecimal characters, actual %q", reference.BaseCommit)
	}
	if !commitPattern.MatchString(reference.MeasurementCommit) {
		problems.add("historicalReference measurementCommit: expected 40 lowercase hexadecimal characters, actual %q", reference.MeasurementCommit)
	}
	validateRunnerShape(problems, "historicalReference runner", reference.Runner)
	if !goVersionPattern.MatchString(reference.GoVersion) {
		problems.add("historicalReference goVersion: expected goX.Y.Z, actual %q", reference.GoVersion)
	}
	if reference.UnitDefaultJobs < 1 || reference.ComputedLaneBudget < 1 {
		problems.add("historicalReference jobs/budget: expected positive values, actual jobs=%d budget=%d", reference.UnitDefaultJobs, reference.ComputedLaneBudget)
	}
	if len(reference.Samples) != requiredSamples || !allPositiveFinite(reference.Samples) {
		problems.add("historicalReference samples: expected exactly three finite values > 0, actual %s", compactJSON(reference.Samples))
	} else {
		actualMedian := median(reference.Samples)
		if !nearlyEqual(actualMedian, reference.MedianWallSeconds) {
			problems.add("historicalReference medianWallSeconds: expected recomputed %.3f, actual %.3f", actualMedian, reference.MedianWallSeconds)
		}
	}
	if !isFinitePositive(reference.MedianWallSeconds) {
		problems.add("historicalReference medianWallSeconds: expected finite value > 0, actual %v", reference.MedianWallSeconds)
	}
	validateCountAndHash(problems, "historicalReference", reference.PackageCount, reference.TestCount, reference.InventorySHA256)
}

func validateCohortExpectationShape(problems *validationProblems, name string, expectation cohortExpectation) {
	if !commitPattern.MatchString(expectation.Commit) {
		problems.add("%s commit: expected 40 lowercase hexadecimal characters, actual %q", name, expectation.Commit)
	}
	validateCountAndHash(problems, name, expectation.PackageCount, expectation.TestCount, expectation.InventorySHA256)
}

func validateCountAndHash(problems *validationProblems, name string, packageCount, testCount int, inventoryHash string) {
	if packageCount < 1 {
		problems.add("%s packageCount: expected positive value, actual %d", name, packageCount)
	}
	if testCount < 1 {
		problems.add("%s testCount: expected positive value, actual %d", name, testCount)
	}
	if !sha256Pattern.MatchString(inventoryHash) {
		problems.add("%s inventorySha256: expected 64 lowercase hexadecimal characters, actual %q", name, inventoryHash)
	}
}

func validateRunnerShape(problems *validationProblems, name string, runner timingRunner) {
	if strings.TrimSpace(runner.Provider) == "" || strings.TrimSpace(runner.Image) == "" || strings.TrimSpace(runner.ImageVersion) == "" || strings.TrimSpace(runner.OS) == "" || strings.TrimSpace(runner.Architecture) == "" || strings.TrimSpace(runner.CPUModel) == "" {
		problems.add("%s: expected complete provider/image/version/os/architecture/cpuModel identity, actual %s", name, compactJSON(runner))
	}
}

func loadTimingEvidence(paths []string, cohort string) ([]timingEvidence, []string) {
	evidence := make([]timingEvidence, 0, len(paths))
	diagnostics := make([]string, 0)
	for index, path := range paths {
		item := timingEvidence{Path: path, Hash: emptyFileSHA256}
		data, err := os.ReadFile(path)
		if err != nil {
			diagnostics = append(diagnostics, fmt.Sprintf("%s sample %d path: expected readable timing JSON, actual %v", cohort, index+1, err))
			evidence = append(evidence, item)
			continue
		}
		item.Bytes = data
		item.Hash = sha256Hex(data)
		if err := decodeJSONBytes(data, &item.Summary); err != nil {
			diagnostics = append(diagnostics, fmt.Sprintf("%s sample %d: expected valid v2 timing JSON, actual %v", cohort, index+1, err))
			evidence = append(evidence, item)
			continue
		}
		item.Loaded = true
		evidence = append(evidence, item)
	}
	return evidence, diagnostics
}

func loadedTimingSummaries(evidence []timingEvidence) []timingSummary {
	summaries := make([]timingSummary, 0, len(evidence))
	for _, item := range evidence {
		if item.Loaded {
			summaries = append(summaries, item.Summary)
		}
	}
	return summaries
}

func validateSamplePathSequence(problems *validationProblems, cohort string, paths []string) {
	ordinals := make([]int, 0, len(paths))
	recognized := false
	for _, path := range paths {
		match := runOrdinalPattern.FindStringSubmatch(filepath.ToSlash(path))
		if len(match) == 0 {
			ordinals = append(ordinals, 0)
			continue
		}
		recognized = true
		var ordinal int
		if _, err := fmt.Sscanf(match[1], "%d", &ordinal); err != nil {
			ordinal = 0
		}
		ordinals = append(ordinals, ordinal)
	}
	if !recognized {
		return
	}
	for index, ordinal := range ordinals {
		if ordinal != index+1 {
			problems.add("%s sample sequence: expected ordinal %d at position %d, actual path %q (ordinals %s)", cohort, index+1, index+1, paths[index], compactJSON(ordinals))
			return
		}
	}
}

func runFinal(cfg budgetConfig) error {
	historicalPaths, err := splitSamplePaths(cfg.historicalSamples)
	if err != nil {
		return checkerError(fmt.Errorf("historical samples: %w", err))
	}
	referencePaths, err := splitSamplePaths(cfg.referenceSamples)
	if err != nil {
		return checkerError(fmt.Errorf("reference samples: %w", err))
	}
	candidatePaths, err := splitSamplePaths(cfg.samples)
	if err != nil {
		return checkerError(fmt.Errorf("candidate samples: %w", err))
	}
	if strings.TrimSpace(cfg.manifest) == "" {
		return checkerError(errors.New("manifest: expected required output path in final mode, actual empty"))
	}

	var setupProblems validationProblems
	validateSamplePathSequence(&setupProblems, "historical", historicalPaths)
	validateSamplePathSequence(&setupProblems, "reference", referencePaths)
	validateSamplePathSequence(&setupProblems, "candidate", candidatePaths)

	budget, budgetErr := loadLatencyBudget(cfg.budgetPath)
	if budgetErr != nil {
		setupProblems.add("budget: expected valid v2 budget at %q, actual %v", cfg.budgetPath, budgetErr)
	} else if budget.Version == latencyBudgetVersionV2 {
		validateCandidateInventorySource(&setupProblems, budget.Candidate)
	}
	historicalEvidence, historicalLoadProblems := loadTimingEvidence(historicalPaths, "historical")
	referenceEvidence, referenceLoadProblems := loadTimingEvidence(referencePaths, "reference")
	candidateEvidence, candidateLoadProblems := loadTimingEvidence(candidatePaths, "candidate")

	report, validationProblems := evaluateReferenceCI(
		budget,
		loadedTimingSummaries(historicalEvidence),
		loadedTimingSummaries(referenceEvidence),
		loadedTimingSummaries(candidateEvidence),
		cfg.candidateCommit,
	)
	diagnostics := make([]string, 0, len(setupProblems.items)+len(historicalLoadProblems)+len(referenceLoadProblems)+len(candidateLoadProblems)+len(validationProblems.items))
	diagnostics = append(diagnostics, setupProblems.items...)
	diagnostics = append(diagnostics, historicalLoadProblems...)
	diagnostics = append(diagnostics, referenceLoadProblems...)
	diagnostics = append(diagnostics, candidateLoadProblems...)
	diagnostics = append(diagnostics, validationProblems.items...)
	manifest := newReferenceCIManifest(budget, referenceEvidence, candidateEvidence, report, diagnostics, cfg.candidateCommit)
	if err := writeReferenceCIManifest(cfg.manifest, manifest); err != nil {
		return errors.Join(diagnosticsError(diagnostics), checkerError(err))
	}
	if len(diagnostics) > 0 {
		return diagnosticsError(diagnostics)
	}

	report.ManifestPath = cfg.manifest
	_, err = fmt.Fprint(stdoutWriter, renderBudgetReport(report))
	return err
}

func diagnosticsError(items []string) error {
	if len(items) == 0 {
		return nil
	}
	return (validationProblems{items: items}).err()
}

func validateCandidateInventorySource(problems *validationProblems, candidate candidateExpectation) {
	path := strings.TrimSpace(candidate.InventorySource)
	data, err := readRepositoryFile(path)
	if err != nil {
		problems.add("candidate inventory source: expected readable reconciliation %q, actual %v", path, err)
		return
	}
	var document struct {
		Schema     string `json:"schema"`
		Version    int    `json:"version"`
		Comparison struct {
			Final struct {
				PackageCount    int    `json:"packageCount"`
				TestCount       int    `json:"testCount"`
				InventorySHA256 string `json:"inventorySha256"`
			} `json:"final"`
			ExactReconstruction struct {
				ReconstructedSetSize       int    `json:"reconstructedSetSize"`
				ReconstructedInventoryHash string `json:"reconstructedInventorySha256"`
			} `json:"exactReconstruction"`
		} `json:"comparison"`
	}
	if err := json.Unmarshal(data, &document); err != nil {
		problems.add("candidate inventory source: expected valid reconciliation JSON, actual %v", err)
		return
	}
	if document.Schema != "you-agent-factory.unit-lane-inventory-reconciliation.v1" {
		problems.add("candidate inventory source schema: expected %q, actual %q", "you-agent-factory.unit-lane-inventory-reconciliation.v1", document.Schema)
	}
	if document.Version != 1 {
		problems.add("candidate inventory source version: expected 1, actual %d", document.Version)
	}
	compareExpectedInt(problems, "candidate inventory source packageCount", candidate.PackageCount, document.Comparison.Final.PackageCount)
	compareExpectedInt(problems, "candidate inventory source testCount", candidate.TestCount, document.Comparison.ExactReconstruction.ReconstructedSetSize)
	compareExpectedInt(problems, "candidate inventory source final testCount", candidate.TestCount, document.Comparison.Final.TestCount)
	compareExpectedString(problems, "candidate inventory source final inventorySha256", candidate.InventorySHA256, document.Comparison.Final.InventorySHA256)
	compareExpectedString(problems, "candidate inventory source inventorySha256", candidate.InventorySHA256, document.Comparison.ExactReconstruction.ReconstructedInventoryHash)
}

func readRepositoryFile(path string) ([]byte, error) {
	candidates := []string{path}
	if !filepath.IsAbs(path) {
		candidates = append(candidates, filepath.Join("..", "..", path))
	}
	var lastErr error
	for _, candidate := range candidates {
		data, err := os.ReadFile(candidate)
		if err == nil {
			return data, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

func evaluateReferenceCI(budget latencyBudget, historical, reference, candidate []timingSummary, expectedCandidateCommit string) (budgetReport, validationProblems) {
	var problems validationProblems
	validateV2BudgetShape(&problems, budget)

	historicalProblems, historicalDetails := validateSampleSet(historical)
	referenceProblems, referenceDetails := validateSampleSet(reference)
	candidateProblems, candidateDetails := validateSampleSet(candidate)
	appendCohortProblems(&problems, "historical", historicalProblems)
	appendCohortProblems(&problems, "reference", referenceProblems)
	appendCohortProblems(&problems, "candidate", candidateProblems)

	if budget.Version == latencyBudgetVersionV2 {
		validateExpectedHistorical(&problems, budget.HistoricalReference, historical)
		validateExpectedCohort(&problems, "reference", budget.ReferenceCI, reference)
		validateExpectedCandidate(&problems, budget.Candidate, candidate)
		validateLiveConfiguration(&problems, budget.HistoricalReference, "reference", reference)
		validateLiveConfiguration(&problems, budget.HistoricalReference, "candidate", candidate)
	}
	if expectedCandidateCommit = strings.TrimSpace(expectedCandidateCommit); expectedCandidateCommit != "" {
		if !commitPattern.MatchString(expectedCandidateCommit) {
			problems.add("candidate commit expectation: expected 40 lowercase hexadecimal characters, actual %q", expectedCandidateCommit)
		} else {
			for index, sample := range candidate {
				if sample.Run.Commit != expectedCandidateCommit {
					problems.add("candidate sample %d commit: expected %q, actual %q", index+1, expectedCandidateCommit, sample.Run.Commit)
				}
			}
		}
	}

	if len(reference) > 0 && len(candidate) > 0 {
		compareLiveIdentity(&problems, reference[0].Run, candidate[0].Run)
	}

	report := budgetReport{
		Mode:              "final",
		SampleWalls:       candidateDetails.walls,
		MedianWallSeconds: candidateDetails.median,
		PackageCount:      candidateDetails.packageCount,
		TestCount:         candidateDetails.testCount,
		CachedPackages:    candidateDetails.cached,
		UnknownPackages:   candidateDetails.unknown,
	}
	if referenceDetails.median > 0 {
		report.ReferenceMedianSeconds = referenceDetails.median
	}
	if report.ReferenceMedianSeconds > 0 && report.MedianWallSeconds > 0 {
		report.ImprovementPercent = improvementPercent(report.ReferenceMedianSeconds, report.MedianWallSeconds)
		report.MaximumRunAboveMedianPct = maximumRunAboveMedian(candidateDetails.walls, report.MedianWallSeconds)
		if report.ImprovementPercent+0.000000001 < minimumImprovementPercent {
			problems.add("median improvement: expected >= %.2f%%, actual %.2f%%", minimumImprovementPercent, report.ImprovementPercent)
		}
		if report.MaximumRunAboveMedianPct > maximumRunAboveMedianPercent+0.000000001 {
			problems.add("maximum run above median: expected <= %.2f%%, actual %.2f%%", maximumRunAboveMedianPercent, report.MaximumRunAboveMedianPct)
		}
	}
	_ = historicalDetails
	return report, problems
}

func appendCohortProblems(destination *validationProblems, cohort string, source validationProblems) {
	for _, item := range source.items {
		destination.add("%s %s", cohort, item)
	}
}

func validateExpectedHistorical(problems *validationProblems, expected historicalReference, samples []timingSummary) {
	for index, sample := range samples {
		prefix := fmt.Sprintf("historical sample %d", index+1)
		compareExpectedString(problems, prefix+" commit", expected.MeasurementCommit, sample.Run.Commit)
		compareExpectedRunner(problems, prefix+" runner", expected.Runner, sample.Run.Runner)
		compareExpectedString(problems, prefix+" goVersion", expected.GoVersion, sample.Run.GoVersion)
		compareExpectedInt(problems, prefix+" unitDefaultJobs", expected.UnitDefaultJobs, sample.Run.UnitDefaultJobs)
		compareExpectedInt(problems, prefix+" computedLaneBudget", expected.ComputedLaneBudget, sample.Run.ComputedLaneBudget)
		compareExpectedInt(problems, prefix+" packageCount", expected.PackageCount, sample.PackageCount)
		compareExpectedInt(problems, prefix+" testCount", expected.TestCount, sample.TestCount)
		compareExpectedString(problems, prefix+" inventorySha256", expected.InventorySHA256, inventorySHA256(sample))
	}
}

func validateExpectedCohort(problems *validationProblems, name string, expected cohortExpectation, samples []timingSummary) {
	for index, sample := range samples {
		prefix := fmt.Sprintf("%s sample %d", name, index+1)
		compareExpectedString(problems, prefix+" commit", expected.Commit, sample.Run.Commit)
		compareExpectedInt(problems, prefix+" packageCount", expected.PackageCount, sample.PackageCount)
		compareExpectedInt(problems, prefix+" testCount", expected.TestCount, sample.TestCount)
		compareExpectedString(problems, prefix+" inventorySha256", expected.InventorySHA256, inventorySHA256(sample))
	}
}

func validateExpectedCandidate(problems *validationProblems, expected candidateExpectation, samples []timingSummary) {
	for index, sample := range samples {
		prefix := fmt.Sprintf("candidate sample %d", index+1)
		compareExpectedInt(problems, prefix+" packageCount", expected.PackageCount, sample.PackageCount)
		compareExpectedInt(problems, prefix+" testCount", expected.TestCount, sample.TestCount)
		compareExpectedString(problems, prefix+" inventorySha256", expected.InventorySHA256, inventorySHA256(sample))
	}
}

func validateLiveConfiguration(problems *validationProblems, expected historicalReference, cohort string, samples []timingSummary) {
	for index, sample := range samples {
		prefix := fmt.Sprintf("%s sample %d", cohort, index+1)
		compareExpectedString(problems, prefix+" goVersion", expected.GoVersion, sample.Run.GoVersion)
		compareExpectedInt(problems, prefix+" unitDefaultJobs", expected.UnitDefaultJobs, sample.Run.UnitDefaultJobs)
		compareExpectedInt(problems, prefix+" computedLaneBudget", expected.ComputedLaneBudget, sample.Run.ComputedLaneBudget)
	}
}

func compareExpectedRunner(problems *validationProblems, prefix string, expected, actual timingRunner) {
	compareExpectedString(problems, prefix+".provider", expected.Provider, actual.Provider)
	compareExpectedString(problems, prefix+".image", expected.Image, actual.Image)
	compareExpectedString(problems, prefix+".imageVersion", expected.ImageVersion, actual.ImageVersion)
	compareExpectedString(problems, prefix+".os", expected.OS, actual.OS)
	compareExpectedString(problems, prefix+".architecture", expected.Architecture, actual.Architecture)
	compareExpectedString(problems, prefix+".cpuModel", expected.CPUModel, actual.CPUModel)
}

func compareLiveIdentity(problems *validationProblems, expected, actual timingRun) {
	compareExpectedRunner(problems, "reference/candidate runner", expected.Runner, actual.Runner)
	compareExpectedString(problems, "reference/candidate goVersion", expected.GoVersion, actual.GoVersion)
	compareExpectedInt(problems, "reference/candidate unitDefaultJobs", expected.UnitDefaultJobs, actual.UnitDefaultJobs)
	compareExpectedInt(problems, "reference/candidate computedLaneBudget", expected.ComputedLaneBudget, actual.ComputedLaneBudget)
	compareExpectedString(problems, "reference/candidate command", normalizedTimingCommand(expected.Command), normalizedTimingCommand(actual.Command))
}

func compareExpectedString(problems *validationProblems, field, expected, actual string) {
	if expected != actual {
		problems.add("%s: expected %q, actual %q", field, expected, actual)
	}
}

func compareExpectedInt(problems *validationProblems, field string, expected, actual int) {
	if expected != actual {
		problems.add("%s: expected %d, actual %d", field, expected, actual)
	}
}

func inventorySHA256(sample timingSummary) string {
	_, tests := inventories(sample)
	return sha256Hex([]byte(strings.Join(tests, "\n")))
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func newReferenceCIManifest(budget latencyBudget, reference, candidate []timingEvidence, report budgetReport, diagnostics []string, expectedCandidateCommit string) referenceCIManifest {
	referenceCommit := firstEvidenceCommit(reference)
	if referenceCommit == "" {
		referenceCommit = budget.ReferenceCI.Commit
	}
	candidateCommit := firstEvidenceCommit(candidate)
	if candidateCommit == "" {
		candidateCommit = strings.TrimSpace(expectedCandidateCommit)
	}
	manifest := referenceCIManifest{
		Schema:                         "you-agent-factory.unit-lane-reference-ci-evidence.v1",
		Version:                        referenceCIManifestVersion,
		Status:                         "fail",
		ReferenceCommit:                referenceCommit,
		CandidateCommit:                candidateCommit,
		Runner:                         manifestRunner(reference, candidate),
		GoVersion:                      unknownManifestValue,
		UnitDefaultJobs:                0,
		ComputedLaneBudget:             0,
		Reference:                      manifestCohortFromEvidence(reference),
		Candidate:                      manifestCohortFromEvidence(candidate),
		MinimumImprovementPercent:      minimumImprovementPercent,
		ActualImprovementPercent:       report.ImprovementPercent,
		MaximumRunAboveMedianPercent:   maximumRunAboveMedianPercent,
		ActualMaximumRunAboveMedianPct: report.MaximumRunAboveMedianPct,
		Diagnostics:                    append([]string{}, diagnostics...),
	}
	if manifest.ReferenceCommit == "" {
		manifest.ReferenceCommit = unknownManifestValue
	}
	if manifest.CandidateCommit == "" {
		manifest.CandidateCommit = unknownManifestValue
	}
	if run, ok := firstEvidenceRun(reference); ok {
		manifest.GoVersion = run.GoVersion
		manifest.UnitDefaultJobs = run.UnitDefaultJobs
		manifest.ComputedLaneBudget = run.ComputedLaneBudget
	} else if run, ok := firstEvidenceRun(candidate); ok {
		manifest.GoVersion = run.GoVersion
		manifest.UnitDefaultJobs = run.UnitDefaultJobs
		manifest.ComputedLaneBudget = run.ComputedLaneBudget
	}
	if len(manifest.Diagnostics) == 0 {
		manifest.Status = "pass"
	}
	return manifest
}

func firstEvidenceRun(evidence []timingEvidence) (timingRun, bool) {
	for _, item := range evidence {
		if item.Loaded {
			return item.Summary.Run, true
		}
	}
	return timingRun{}, false
}

func firstEvidenceCommit(evidence []timingEvidence) string {
	run, ok := firstEvidenceRun(evidence)
	if !ok {
		return ""
	}
	return run.Commit
}

func manifestRunner(reference, candidate []timingEvidence) timingRunner {
	if run, ok := firstEvidenceRun(reference); ok {
		return run.Runner
	}
	if run, ok := firstEvidenceRun(candidate); ok {
		return run.Runner
	}
	return timingRunner{Provider: unknownManifestValue, Image: unknownManifestValue, ImageVersion: unknownManifestValue, OS: unknownManifestValue, Architecture: unknownManifestValue, CPUModel: unknownManifestValue}
}

func manifestCohortFromEvidence(evidence []timingEvidence) manifestCohort {
	cohort := manifestCohort{Samples: make([]manifestSample, 0, len(evidence)), InventorySHA256: emptyFileSHA256}
	walls := make([]float64, 0, len(evidence))
	for index, item := range evidence {
		sample := manifestSample{Ordinal: index + 1, Path: item.Path, Command: "", SHA256: item.Hash, WallSeconds: 0}
		if strings.TrimSpace(sample.Path) == "" {
			sample.Path = fmt.Sprintf("<missing-sample-%d>", index+1)
		}
		if sample.SHA256 == "" {
			sample.SHA256 = emptyFileSHA256
		}
		if item.Loaded {
			sample.Command = item.Summary.Run.Command
			sample.WallSeconds = item.Summary.WallSeconds
			walls = append(walls, item.Summary.WallSeconds)
			if cohort.PackageCount == 0 {
				cohort.PackageCount = item.Summary.PackageCount
				cohort.TestCount = item.Summary.TestCount
				cohort.InventorySHA256 = inventorySHA256(item.Summary)
			}
			cached, unknown := cacheCounts(item.Summary.Packages)
			cohort.CachedPackages += cached
			cohort.UnknownPackages += unknown
		}
		cohort.Samples = append(cohort.Samples, sample)
	}
	if len(walls) == requiredSamples && allPositiveFinite(walls) {
		cohort.MedianWall = median(walls)
	}
	return cohort
}

func writeReferenceCIManifest(path string, manifest referenceCIManifest) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return errors.New("manifest: expected nonempty output path, actual empty")
	}
	if directory := filepath.Dir(path); directory != "." && directory != "" {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			return fmt.Errorf("create reference-CI manifest directory: %w", err)
		}
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("render reference-CI manifest JSON: %w", err)
	}
	data = append(data, '\n')
	if err := validateLatencyBudgetDocument(referenceCIManifestSchemaPath(), data); err != nil {
		return fmt.Errorf("reference-CI manifest schema validation: %w", err)
	}
	return atomicWriteReferenceCIFile(path, data)
}

func referenceCIManifestSchemaPath() string {
	for _, candidate := range []string{
		referenceCIManifestSchema,
		filepath.Join("..", "..", filepath.FromSlash(referenceCIManifestSchema)),
	} {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return filepath.FromSlash(referenceCIManifestSchema)
}

func atomicWriteReferenceCIFile(path string, data []byte) error {
	directory := filepath.Dir(path)
	temporary, err := os.CreateTemp(directory, ".unit-reference-ci-*.tmp")
	if err != nil {
		return fmt.Errorf("create reference-CI manifest temporary file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o644); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("set reference-CI manifest temporary file mode: %w", err)
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write reference-CI manifest temporary file: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close reference-CI manifest temporary file: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		if !os.IsExist(err) {
			return fmt.Errorf("rename reference-CI manifest temporary file: %w", err)
		}
		if removeErr := os.Remove(path); removeErr != nil {
			return fmt.Errorf("replace reference-CI manifest: %w", removeErr)
		}
		if retryErr := os.Rename(temporaryPath, path); retryErr != nil {
			return fmt.Errorf("rename reference-CI manifest temporary file: %w", retryErr)
		}
	}
	return nil
}
