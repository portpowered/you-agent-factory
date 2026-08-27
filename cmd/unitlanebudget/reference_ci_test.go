package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func validateReferenceCI(budget latencyBudget, historical, reference, candidate []timingSummary) (budgetReport, error) {
	report, problems := evaluateReferenceCI(budget, historical, reference, candidate, "")
	return report, problems.err()
}

func validateReferenceCIWithCandidateCommit(budget latencyBudget, historical, reference, candidate []timingSummary, expectedCandidateCommit string) (budgetReport, error) {
	report, problems := evaluateReferenceCI(budget, historical, reference, candidate, expectedCandidateCommit)
	return report, problems.err()
}

func TestCommittedBudgetV2SchemaAndInstancePassDraft202012Compiler(t *testing.T) {
	budgetPath := filepath.Join("..", "..", "docs", "internal", "baselines", "go-unit-lane-latency-budget.v2.json")
	data, err := os.ReadFile(budgetPath)
	if err != nil {
		t.Fatalf("read v2 budget: %v", err)
	}
	v2SchemaPath := filepath.Join("..", "..", "docs", "internal", "baselines", "go-unit-lane-latency-budget.v2.schema.json")
	if err := validateLatencyBudgetDocument(v2SchemaPath, data); err != nil {
		t.Fatalf("validate v2 budget against Draft 2020-12 schema: %v", err)
	}
	budget, err := loadLatencyBudget(budgetPath)
	if err != nil {
		t.Fatalf("load v2 budget: %v", err)
	}
	var problems validationProblems
	validateV2BudgetShape(&problems, budget)
	if err := problems.err(); err != nil {
		t.Fatalf("v2 budget semantic validation: %v", err)
	}
	if budget.HistoricalReference.PackageCount != 444 || budget.HistoricalReference.TestCount != 18122 || budget.HistoricalReference.MedianWallSeconds != 239.612 {
		t.Fatalf("historical reference = %+v, want 444/18122/239.612", budget.HistoricalReference)
	}
	if budget.ReferenceCI.Commit != "9e19e26e0fb6df47cfdd4c4d4469ce712aae04ff" {
		t.Fatalf("reference-CI expectation = %+v, want reachable historical base commit", budget.ReferenceCI)
	}
	if budget.HistoricalReference.MeasurementCommit != "ba8ef900ee29347295ac7657742fd1aab42f064c" {
		t.Fatalf("historical measurement commit = %q, want retained audit commit", budget.HistoricalReference.MeasurementCommit)
	}
	if budget.Candidate.PackageCount != 444 || budget.Candidate.TestCount != 18156 || budget.Candidate.InventorySHA256 != "451f3276fb95d5998dcba67cf65b039c0791351d15bdbe505c27a160a8bb6ede" {
		t.Fatalf("candidate expectation = %+v, want reconciled 444/18156 inventory", budget.Candidate)
	}
}

func TestRetainedHistoricalBaselineAuditRemainsByteIdentical(t *testing.T) {
	paths := []string{
		filepath.Join("..", "..", "docs", "internal", "development", "plans", "unit-test-optimization-c01-wire-timeout-witness", "baseline-make-run-1-replacement.v2.json"),
		filepath.Join("..", "..", "docs", "internal", "development", "plans", "unit-test-optimization-c01-wire-timeout-witness", "baseline-make-run-2.v2.json"),
		filepath.Join("..", "..", "docs", "internal", "development", "plans", "unit-test-optimization-c01-wire-timeout-witness", "baseline-make-run-3.v2.json"),
	}
	expectedHashes := []string{
		"ba7e1364ed5c88d66071d4cac4b2bf027571044ef7d159b16d25435f7fc95d8a",
		"d30fdc0215d50a14c0a4cef65b234fde68a680e4015e37b5d9a463c9f361723f",
		"e4288d9085e19ea3e7f8a87e0ad67ca52b38a255e0bc1e1a569ad59fbd008d98",
	}
	samples, err := loadTimingSamples(paths)
	if err != nil {
		t.Fatalf("load retained historical samples: %v", err)
	}
	report, err := validateBaseline(samples)
	if err != nil {
		t.Fatalf("validate retained historical samples: %v", err)
	}
	if report.PackageCount != 444 || report.TestCount != 18122 || report.MedianWallSeconds != 239.612 {
		t.Fatalf("historical report = %+v, want 444 packages, 18122 tests, 239.612 seconds", report)
	}
	if actual := inventorySHA256(samples[0]); actual != "508aabd7976efa0fdd16d322a24a0da3f18d6b9fed1725ea3773f4e99da01897" {
		t.Fatalf("historical inventory hash = %s, want reviewed hash", actual)
	}
	for index, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read retained sample %d: %v", index+1, err)
		}
		if actual := sha256Hex(data); actual != expectedHashes[index] {
			t.Fatalf("retained sample %d hash = %s, want %s", index+1, actual, expectedHashes[index])
		}
	}
}

func TestValidateReferenceCIAcceptsComparableLiveCohorts(t *testing.T) {
	budget, historical, reference, candidate := comparableReferenceCIFixture()
	report, err := validateReferenceCI(budget, historical, reference, candidate)
	if err != nil {
		t.Fatalf("validateReferenceCI() error = %v", err)
	}
	if report.ReferenceMedianSeconds != 100 || report.MedianWallSeconds != 70 || report.ImprovementPercent != 30 || report.MaximumRunAboveMedianPct != 0 {
		t.Fatalf("report = %+v, want live medians and thresholds", report)
	}
}

func TestValidateReferenceCIRejectsEveryMaterialCrossCohortMismatch(t *testing.T) {
	cases := map[string]func([]timingSummary, []timingSummary, []timingSummary){
		"provider": func(_, _, candidate []timingSummary) {
			for index := range candidate {
				candidate[index].Run.Runner.Provider = "other-provider"
			}
		},
		"image": func(_, _, candidate []timingSummary) {
			for index := range candidate {
				candidate[index].Run.Runner.Image = "other-image"
			}
		},
		"image version": func(_, _, candidate []timingSummary) {
			for index := range candidate {
				candidate[index].Run.Runner.ImageVersion = "other-image-version"
			}
		},
		"OS": func(_, _, candidate []timingSummary) {
			for index := range candidate {
				candidate[index].Run.Runner.OS = "other-os"
			}
		},
		"architecture": func(_, _, candidate []timingSummary) {
			for index := range candidate {
				candidate[index].Run.Runner.Architecture = "arm64"
			}
		},
		"CPU model": func(_, _, candidate []timingSummary) {
			for index := range candidate {
				candidate[index].Run.Runner.CPUModel = "other-cpu"
			}
		},
		"Go version": func(_, _, candidate []timingSummary) {
			for index := range candidate {
				candidate[index].Run.GoVersion = "go1.26.0"
			}
		},
		"jobs": func(_, _, candidate []timingSummary) {
			for index := range candidate {
				candidate[index].Run.UnitDefaultJobs = 3
			}
		},
		"lane budget": func(_, _, candidate []timingSummary) {
			for index := range candidate {
				candidate[index].Run.ComputedLaneBudget = 3
			}
		},
		"command": func(_, _, candidate []timingSummary) {
			for index := range candidate {
				candidate[index].Run.Command = "go test ./pkg/..."
			}
		},
		"reference commit": func(reference, _, _ []timingSummary) {
			for index := range reference {
				reference[index].Run.Commit = strings.Repeat("c", 40)
			}
		},
		"candidate commit": func(_, _, candidate []timingSummary) {
			for index := range candidate {
				candidate[index].Run.Commit = strings.Repeat("c", 40)
			}
		},
	}

	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			budget, historical, reference, candidate := comparableReferenceCIFixture()
			mutate(historical, reference, candidate)
			var err error
			if name == "candidate commit" {
				_, err = validateReferenceCIWithCandidateCommit(budget, historical, reference, candidate, strings.Repeat("b", 40))
			} else {
				_, err = validateReferenceCI(budget, historical, reference, candidate)
			}
			if err == nil || !strings.Contains(err.Error(), "expected") || !strings.Contains(err.Error(), "actual") {
				t.Fatalf("validateReferenceCI() error = %v, want expected/actual mismatch", err)
			}
			if name == "CPU model" && !strings.Contains(err.Error(), "cpuModel") {
				t.Fatalf("CPU mismatch error = %v, want cpuModel diagnostic", err)
			}
		})
	}
}

func TestValidateReferenceCIRejectsInvalidSampleAndInventoryEvidence(t *testing.T) {
	cases := map[string]func([]timingSummary){
		"incomplete":     func(samples []timingSummary) { samples[0].Complete = false },
		"failed outcome": func(samples []timingSummary) { samples[0].Packages[0].Outcome = "fail" },
		"cached":         func(samples []timingSummary) { samples[0].Packages[0].Cache = unitTimingCacheCached },
		"unknown cache":  func(samples []timingSummary) { samples[0].Packages[0].Cache = unitTimingCacheUnknown },
		"duplicate package": func(samples []timingSummary) {
			samples[0].Packages[1].Package = samples[0].Packages[0].Package
		},
		"inventory hash": func(samples []timingSummary) {
			samples[0].Packages[0].Tests[0] = "TestChanged"
			for index := 1; index < len(samples); index++ {
				samples[index].Packages[0].Tests[0] = "TestChanged"
			}
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			budget, historical, reference, candidate := comparableReferenceCIFixture()
			mutate(candidate)
			if _, err := validateReferenceCI(budget, historical, reference, candidate); err == nil {
				t.Fatal("validateReferenceCI() unexpectedly passed invalid evidence")
			}
		})
	}
}

func TestValidateReferenceCIRejectsNonconsecutiveNamedSamples(t *testing.T) {
	var problems validationProblems
	validateSamplePathSequence(&problems, "candidate", []string{"run-1.v2.json", "run-3.v2.json", "run-4.v2.json"})
	if err := problems.err(); err == nil || !strings.Contains(err.Error(), "sample sequence") || !strings.Contains(err.Error(), "expected") || !strings.Contains(err.Error(), "actual") {
		t.Fatalf("sample sequence diagnostics = %v, want expected/actual failure", err)
	}
}

func TestWriteReferenceCIManifestReplacesOutputAtomically(t *testing.T) {
	budget, historical, reference, candidate := comparableReferenceCIFixture()
	report, err := validateReferenceCI(budget, historical, reference, candidate)
	if err != nil {
		t.Fatalf("validate fixture: %v", err)
	}
	path := filepath.Join(t.TempDir(), "reference-ci-manifest.v1.json")
	referenceEvidence := evidenceForSamples(t, reference)
	candidateEvidence := evidenceForSamples(t, candidate)
	manifest := newReferenceCIManifest(budget, referenceEvidence, candidateEvidence, report, nil, "")
	if err := writeReferenceCIManifest(path, manifest); err != nil {
		t.Fatalf("write pass manifest: %v", err)
	}
	first, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read pass manifest: %v", err)
	}
	var decoded referenceCIManifest
	if err := decodeJSONBytes(first, &decoded); err != nil {
		t.Fatalf("decode pass manifest: %v", err)
	}
	if decoded.Status != "pass" || len(decoded.Reference.Samples) != 3 || len(decoded.Candidate.Samples) != 3 {
		t.Fatalf("manifest = %+v, want pass with 3+3 samples", decoded)
	}
	if decoded.Reference.Samples[0].SHA256 != sha256Hex(referenceEvidence[0].Bytes) || decoded.Candidate.Samples[0].SHA256 != sha256Hex(candidateEvidence[0].Bytes) {
		t.Fatalf("manifest sample hashes = %q/%q, want exact input byte hashes", decoded.Reference.Samples[0].SHA256, decoded.Candidate.Samples[0].SHA256)
	}
	if err := validateLatencyBudgetDocument(referenceCIManifestSchemaPath(), first); err != nil {
		t.Fatalf("validate pass manifest schema: %v", err)
	}

	manifest.Diagnostics = []string{"reference/candidate runner.cpuModel: expected \"test-cpu\", actual \"other-cpu\""}
	manifest.Status = "fail"
	if err := writeReferenceCIManifest(path, manifest); err != nil {
		t.Fatalf("replace fail manifest: %v", err)
	}
	second, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fail manifest: %v", err)
	}
	if string(first) == string(second) || !strings.Contains(string(second), "other-cpu") {
		t.Fatalf("replacement manifest did not retain new diagnostics: %s", second)
	}
}

func TestLoadTimingEvidenceRetainsAvailableInputsWhenOneIsMissing(t *testing.T) {
	directory := t.TempDir()
	samples := comparableSamples(100, 100, 100)
	paths := []string{filepath.Join(directory, "run-1.v2.json"), filepath.Join(directory, "run-2.v2.json"), filepath.Join(directory, "run-3.v2.json")}
	for index := range samples {
		if index == 1 {
			continue
		}
		data, err := json.Marshal(samples[index])
		if err != nil {
			t.Fatalf("marshal sample: %v", err)
		}
		if err := os.WriteFile(paths[index], data, 0o600); err != nil {
			t.Fatalf("write sample: %v", err)
		}
	}
	evidence, diagnostics := loadTimingEvidence(paths, "candidate")
	if len(evidence) != 3 || len(diagnostics) != 1 || loadedTimingSummaries(evidence) == nil || len(loadedTimingSummaries(evidence)) != 2 {
		t.Fatalf("evidence=%d diagnostics=%d loaded=%d, want retained 3/1/2", len(evidence), len(diagnostics), len(loadedTimingSummaries(evidence)))
	}
	if !strings.Contains(diagnostics[0], "candidate sample 2") || !strings.Contains(diagnostics[0], "expected") || !strings.Contains(diagnostics[0], "actual") {
		t.Fatalf("missing-input diagnostic = %q, want expected/actual context", diagnostics[0])
	}
}

func TestRunFinalRetainsSchemaValidFailureManifestForMissingSamples(t *testing.T) {
	directory := t.TempDir()
	missingPaths := []string{
		filepath.Join(directory, "reference", "run-1.v2.json"),
		filepath.Join(directory, "reference", "run-2.v2.json"),
		filepath.Join(directory, "reference", "run-3.v2.json"),
	}
	cfg := budgetConfig{
		budgetPath:        filepath.Join("..", "..", "docs", "internal", "baselines", "go-unit-lane-latency-budget.v2.json"),
		historicalSamples: strings.Join(missingPaths, ","),
		referenceSamples:  strings.Join(missingPaths, ","),
		samples:           strings.Join(missingPaths, ","),
		manifest:          filepath.Join(directory, "reference-ci-manifest.v1.json"),
	}
	if err := runFinal(cfg); err == nil {
		t.Fatal("runFinal() unexpectedly passed missing samples")
	}
	data, err := os.ReadFile(cfg.manifest)
	if err != nil {
		t.Fatalf("read retained failure manifest: %v", err)
	}
	var manifest referenceCIManifest
	if err := decodeJSONBytes(data, &manifest); err != nil {
		t.Fatalf("decode retained failure manifest: %v", err)
	}
	if manifest.Status != "fail" || len(manifest.Diagnostics) == 0 {
		t.Fatalf("failure manifest = %+v, want fail with diagnostics", manifest)
	}
	if err := validateLatencyBudgetDocument(referenceCIManifestSchemaPath(), data); err != nil {
		t.Fatalf("failure manifest schema validation: %v", err)
	}
}

func TestRunFinalAcceptsComparableFixturesAndWritesManifest(t *testing.T) {
	directory := t.TempDir()
	budget, historical, reference, candidate := comparableReferenceCIFixture()
	sourcePath := filepath.Join(directory, "reconciliation.v1.json")
	fixtureHash := inventorySHA256(candidate[0])
	source := map[string]any{
		"schema":  "you-agent-factory.unit-lane-inventory-reconciliation.v1",
		"version": 1,
		"comparison": map[string]any{
			"final": map[string]any{
				"packageCount":    2,
				"testCount":       3,
				"inventorySha256": fixtureHash,
			},
			"exactReconstruction": map[string]any{
				"reconstructedSetSize":         3,
				"reconstructedInventorySha256": fixtureHash,
			},
		},
	}
	writeFixtureJSON(t, sourcePath, source)
	budget.Candidate.InventorySource = sourcePath
	budgetData, err := json.Marshal(budget)
	if err != nil {
		t.Fatalf("marshal fixture budget: %v", err)
	}
	var budgetDocument map[string]json.RawMessage
	if err := json.Unmarshal(budgetData, &budgetDocument); err != nil {
		t.Fatalf("decode fixture budget: %v", err)
	}
	delete(budgetDocument, "reference")
	budgetData, err = json.Marshal(budgetDocument)
	if err != nil {
		t.Fatalf("render fixture budget: %v", err)
	}
	budgetPath := filepath.Join(directory, "budget.v2.json")
	writeFixtureBytes(t, budgetPath, budgetData)
	schemaSource := filepath.Join("..", "..", "docs", "internal", "baselines", "go-unit-lane-latency-budget.v2.schema.json")
	schemaData, err := os.ReadFile(schemaSource)
	if err != nil {
		t.Fatalf("read v2 schema: %v", err)
	}
	writeFixtureBytes(t, filepath.Join(directory, "go-unit-lane-latency-budget.v2.schema.json"), schemaData)

	historicalPaths := writeTimingFixtures(t, filepath.Join(directory, "historical"), historical)
	referencePaths := writeTimingFixtures(t, filepath.Join(directory, "reference"), reference)
	candidatePaths := writeTimingFixtures(t, filepath.Join(directory, "candidate"), candidate)
	manifestPath := filepath.Join(directory, "reference-ci-manifest.v1.json")
	output := new(bytes.Buffer)
	originalWriter := stdoutWriter
	stdoutWriter = output
	t.Cleanup(func() { stdoutWriter = originalWriter })
	if err := runFinal(budgetConfig{
		budgetPath:        budgetPath,
		historicalSamples: strings.Join(historicalPaths, ","),
		referenceSamples:  strings.Join(referencePaths, ","),
		samples:           strings.Join(candidatePaths, ","),
		manifest:          manifestPath,
	}); err != nil {
		t.Fatalf("runFinal() error = %v", err)
	}
	if !strings.Contains(output.String(), "Median improvement: 30.00%") || !strings.Contains(output.String(), manifestPath) {
		t.Fatalf("runFinal output = %q, want live result and manifest path", output.String())
	}
	manifestData, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read final manifest: %v", err)
	}
	var manifest referenceCIManifest
	if err := decodeJSONBytes(manifestData, &manifest); err != nil {
		t.Fatalf("decode final manifest: %v", err)
	}
	if manifest.Status != "pass" || manifest.Reference.MedianWall != 100 || manifest.Candidate.MedianWall != 70 {
		t.Fatalf("final manifest = %+v, want live pass medians", manifest)
	}
}

func comparableReferenceCIFixture() (latencyBudget, []timingSummary, []timingSummary, []timingSummary) {
	referenceCommit := strings.Repeat("a", 40)
	candidateCommit := strings.Repeat("b", 40)
	historical := comparableSamples(222.006, 239.612, 258.271)
	reference := comparableSamples(100, 100, 100)
	candidate := comparableSamples(70, 70, 70)
	for index := range historical {
		historical[index].Run = fixtureRun(referenceCommit)
	}
	for index := range reference {
		reference[index].Run = fixtureRun(referenceCommit)
	}
	for index := range candidate {
		candidate[index].Run = fixtureRun(candidateCommit)
	}
	fixtureInventoryHash := inventorySHA256(reference[0])
	budget := latencyBudget{
		Version:    latencyBudgetVersionV2,
		Owner:      "backend-unit-lane",
		Entrypoint: canonicalTimingEntrypoint,
		HistoricalReference: historicalReference{
			BaseCommit:         strings.Repeat("c", 40),
			MeasurementCommit:  referenceCommit,
			Runner:             historical[0].Run.Runner,
			GoVersion:          "go1.25.0",
			UnitDefaultJobs:    2,
			ComputedLaneBudget: 2,
			Samples:            []float64{222.006, 239.612, 258.271},
			MedianWallSeconds:  239.612,
			PackageCount:       2,
			TestCount:          3,
			InventorySHA256:    fixtureInventoryHash,
		},
		ReferenceCI: cohortExpectation{
			Commit:          referenceCommit,
			PackageCount:    2,
			TestCount:       3,
			InventorySHA256: fixtureInventoryHash,
		},
		Candidate: candidateExpectation{
			InventorySource: "docs/internal/development/unit-test-optimization/unit-lane-inventory-reconciliation.v1.json",
			PackageCount:    2,
			TestCount:       3,
			InventorySHA256: fixtureInventoryHash,
		},
		Policy: budgetPolicy{
			RequiredConsecutiveSamples:   3,
			MinimumImprovementPercent:    25,
			MaximumRunAboveMedianPercent: 10,
			RequiredCachedPackages:       0,
			RequiredUnknownPackages:      0,
			RequiredRunnerIdentityFields: append([]string(nil), identityFieldNames...),
			InventoryPolicy:              "exact-with-reviewed-diff",
			InvalidSamplePolicy:          "retain-and-fail-unless-predeclared-invalidation-matches",
		},
	}
	return budget, historical, reference, candidate
}

func fixtureRun(commit string) timingRun {
	run := comparableRun()
	run.Commit = commit
	return run
}

func evidenceForSamples(t *testing.T, samples []timingSummary) []timingEvidence {
	t.Helper()
	evidence := make([]timingEvidence, len(samples))
	for index, sample := range samples {
		data, err := json.Marshal(sample)
		if err != nil {
			t.Fatalf("marshal timing sample: %v", err)
		}
		evidence[index] = timingEvidence{
			Path:    filepath.Join("candidate", "run-"+strconv.Itoa(index+1)+".v2.json"),
			Bytes:   data,
			Hash:    sha256Hex(data),
			Summary: sample,
			Loaded:  true,
		}
	}
	return evidence
}

func writeTimingFixtures(t *testing.T, directory string, samples []timingSummary) []string {
	t.Helper()
	paths := make([]string, len(samples))
	for index, sample := range samples {
		paths[index] = filepath.Join(directory, "run-"+strconv.Itoa(index+1)+".v2.json")
		writeFixtureJSON(t, paths[index], sample)
	}
	return paths
}

func writeFixtureJSON(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal fixture %s: %v", path, err)
	}
	writeFixtureBytes(t, path, data)
}

func writeFixtureBytes(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create fixture directory %s: %v", path, err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write fixture %s: %v", path, err)
	}
}
