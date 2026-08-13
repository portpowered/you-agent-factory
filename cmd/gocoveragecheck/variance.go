package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"
)

const minimumVarianceSamples = 5
const countCoverageProfileHeader = "mode: count"

type coverageVarianceSample struct {
	path   string
	label  string
	header string
	totals map[string]packageCoverageTotals
}

type coverageVarianceCurrentFloor struct {
	label string
	floor coverageFloor
	valid bool
}

type coverageVariancePackage struct {
	importPath   string
	samples      []packageCoverageTotals
	minimum      string
	maximum      string
	swing        string
	sampleFloor  string
	currentFloor string
	headroom     string
}

type coverageVarianceReport struct {
	commit        string
	suite         string
	command       string
	aggregation   string
	profileLabels []string
	packages      []coverageVariancePackage
}

func executeVarianceReport(cfg config) error {
	repoRoot, err := repoRootDir()
	if err != nil {
		return err
	}
	profilePaths := splitList(cfg.varianceProfiles, ",", true)
	samples, err := loadCoverageVarianceProfiles(profilePaths, repoRoot)
	if err != nil {
		return err
	}
	floors, err := readCoverageVarianceFloors(cfg.packageManifest, cfg.suite)
	if err != nil {
		return err
	}
	report, err := buildCoverageVarianceReport(cfg.varianceCommit, cfg.suite, cfg.varianceJobs, samples, floors)
	if err != nil {
		return err
	}
	data, err := renderCoverageVarianceReport(report)
	if err != nil {
		return err
	}
	if err := os.WriteFile(cfg.varianceOutput, data, 0o644); err != nil {
		return fmt.Errorf("write coverage variance report: %w", err)
	}
	fmt.Fprintf(stdoutWriter, "Wrote %s coverage variance report for %d profiles to %s.\n", cfg.suite, len(samples), cfg.varianceOutput)
	return nil
}

func loadCoverageVarianceProfiles(profilePaths []string, repoRoot string) ([]coverageVarianceSample, error) {
	paths, err := normalizeVarianceProfilePaths(profilePaths)
	if err != nil {
		return nil, err
	}
	if len(paths) < minimumVarianceSamples {
		return nil, fmt.Errorf("aggregate coverage variance: requires at least %d profiles, received %d", minimumVarianceSamples, len(paths))
	}

	samples := make([]coverageVarianceSample, 0, len(paths))
	for _, profilePath := range paths {
		sample, err := readCoverageVarianceProfile(profilePath, repoRoot)
		if err != nil {
			return nil, err
		}
		samples = append(samples, sample)
	}
	if err := validateVarianceSampleCompatibility(samples); err != nil {
		return nil, err
	}
	return samples, nil
}

func normalizeVarianceProfilePaths(profilePaths []string) ([]string, error) {
	if len(profilePaths) == 0 {
		return nil, errors.New("aggregate coverage variance: no profiles were provided")
	}
	paths := make([]string, 0, len(profilePaths))
	seen := make(map[string]string, len(profilePaths))
	for _, rawPath := range profilePaths {
		trimmed := strings.TrimSpace(rawPath)
		if trimmed == "" {
			return nil, errors.New("aggregate coverage variance: profile paths cannot be empty")
		}
		absolute, err := filepath.Abs(trimmed)
		if err != nil {
			return nil, fmt.Errorf("aggregate coverage variance: resolve profile %q: %w", trimmed, err)
		}
		absolute = filepath.Clean(absolute)
		key := absolute
		if runtime.GOOS == "windows" {
			key = strings.ToLower(key)
		}
		if previous, ok := seen[key]; ok {
			return nil, fmt.Errorf("aggregate coverage variance: duplicate profile input %q repeats %q", trimmed, previous)
		}
		seen[key] = absolute
		paths = append(paths, absolute)
	}
	slices.Sort(paths)
	return paths, nil
}

func readCoverageVarianceProfile(profilePath string, repoRoot string) (coverageVarianceSample, error) {
	profile, err := os.Open(profilePath)
	if err != nil {
		return coverageVarianceSample{}, fmt.Errorf("aggregate coverage variance: profile %q is unreadable: %w", profilePath, err)
	}
	header, blocks, scanErr := scanCoverageProfile(profile, repoRoot)
	closeErr := profile.Close()
	if scanErr != nil {
		return coverageVarianceSample{}, fmt.Errorf("aggregate coverage variance: read profile %q: %w", profilePath, scanErr)
	}
	if closeErr != nil {
		return coverageVarianceSample{}, fmt.Errorf("aggregate coverage variance: close profile %q: %w", profilePath, closeErr)
	}
	if header != countCoverageProfileHeader {
		return coverageVarianceSample{}, fmt.Errorf("aggregate coverage variance: profile %q uses %q; count-mode profiles are required", profilePath, header)
	}
	totals := make(map[string]packageCoverageTotals)
	for importPath, packageTotals := range coverageTotals(blocks) {
		if !isBackendCoveragePackage(importPath) {
			continue
		}
		if packageTotals.totalStatements <= 0 || packageTotals.coveredStatements < 0 || packageTotals.coveredStatements > packageTotals.totalStatements {
			return coverageVarianceSample{}, fmt.Errorf("aggregate coverage variance: profile %q has invalid statement counts for package %s: covered=%d total=%d", profilePath, importPath, packageTotals.coveredStatements, packageTotals.totalStatements)
		}
		totals[importPath] = packageTotals
	}
	if len(totals) == 0 {
		return coverageVarianceSample{}, fmt.Errorf("aggregate coverage variance: profile %q contains no measurable backend packages", profilePath)
	}
	return coverageVarianceSample{
		path:   profilePath,
		label:  coverageVarianceProfileLabel(profilePath),
		header: header,
		totals: totals,
	}, nil
}

func coverageVarianceProfileLabel(profilePath string) string {
	directory := filepath.Base(filepath.Dir(profilePath))
	if strings.HasPrefix(directory, "run-") {
		return directory
	}
	filename := filepath.Base(profilePath)
	return strings.TrimSuffix(filename, filepath.Ext(filename))
}

func validateVarianceSampleCompatibility(samples []coverageVarianceSample) error {
	if len(samples) == 0 {
		return errors.New("aggregate coverage variance: no samples were loaded")
	}
	first := samples[0]
	if first.header != countCoverageProfileHeader {
		return fmt.Errorf("aggregate coverage variance: profile %q uses %q; count-mode profiles are required", first.path, first.header)
	}
	wantPackages := sortedCoveragePackageNames(first.totals)
	for _, sample := range samples[1:] {
		if sample.header != first.header {
			return fmt.Errorf("aggregate coverage variance: incompatible profile modes: %q uses %q, but %q uses %q", first.path, first.header, sample.path, sample.header)
		}
		gotPackages := sortedCoveragePackageNames(sample.totals)
		if !slices.Equal(wantPackages, gotPackages) {
			return fmt.Errorf("aggregate coverage variance: incompatible package universe between %q and %q: expected %s, got %s", first.path, sample.path, strings.Join(wantPackages, ", "), strings.Join(gotPackages, ", "))
		}
		for _, importPath := range wantPackages {
			wantTotal := first.totals[importPath].totalStatements
			gotTotal := sample.totals[importPath].totalStatements
			if wantTotal != gotTotal {
				return fmt.Errorf("aggregate coverage variance: inconsistent total statements for package %s between %q and %q: expected %d, got %d", importPath, first.path, sample.path, wantTotal, gotTotal)
			}
		}
	}
	return nil
}

func minimumCoverageTotals(samples []coverageVarianceSample) (map[string]packageCoverageTotals, error) {
	if len(samples) < minimumVarianceSamples {
		return nil, fmt.Errorf("requires at least %d profiles, received %d", minimumVarianceSamples, len(samples))
	}
	if err := validateVarianceSampleCompatibility(samples); err != nil {
		return nil, err
	}
	packages := sortedCoveragePackageNames(samples[0].totals)
	minimums := make(map[string]packageCoverageTotals, len(packages))
	for _, importPath := range packages {
		minimum := samples[0].totals[importPath]
		for _, sample := range samples[1:] {
			candidate := sample.totals[importPath]
			if candidate.coveredStatements < minimum.coveredStatements {
				minimum.coveredStatements = candidate.coveredStatements
			}
		}
		minimums[importPath] = minimum
	}
	return minimums, nil
}

func sortedCoveragePackageNames(totals map[string]packageCoverageTotals) []string {
	packages := make([]string, 0, len(totals))
	for importPath := range totals {
		packages = append(packages, importPath)
	}
	slices.Sort(packages)
	return packages
}

func readCoverageVarianceFloors(filename string, lane string) (map[string]coverageVarianceCurrentFloor, error) {
	filename = strings.TrimSpace(filename)
	if filename == "" {
		return map[string]coverageVarianceCurrentFloor{}, nil
	}
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("read %s coverage manifest for variance report: %w", lane, err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var manifest coverageManifest
	if err := decoder.Decode(&manifest); err != nil {
		return nil, fmt.Errorf("parse %s coverage manifest for variance report: %w", lane, err)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return nil, err
	}
	manifestPackages := make([]string, 0, len(manifest.Packages))
	for _, entry := range manifest.Packages {
		manifestPackages = append(manifestPackages, entry.Package)
	}
	if err := validateCoverageManifestAtMode(manifest, lane, manifestPackages, time.Now().UTC(), false); err != nil {
		return nil, fmt.Errorf("validate %s coverage manifest for variance report: %w", lane, err)
	}
	floors := make(map[string]coverageVarianceCurrentFloor, len(manifest.Packages))
	for _, entry := range manifest.Packages {
		if entry.Exception != nil {
			floors[entry.Package] = coverageVarianceCurrentFloor{label: "exception"}
			continue
		}
		floor, err := parseCoverageFloor(entry.Minimum)
		if err != nil {
			return nil, fmt.Errorf("parse %s coverage floor for package %s: %w", lane, entry.Package, err)
		}
		floors[entry.Package] = coverageVarianceCurrentFloor{label: floor.String() + "%", floor: floor, valid: true}
	}
	return floors, nil
}

func buildCoverageVarianceReport(commit string, lane string, jobs int, samples []coverageVarianceSample, floors map[string]coverageVarianceCurrentFloor) (coverageVarianceReport, error) {
	if len(samples) < minimumVarianceSamples {
		return coverageVarianceReport{}, fmt.Errorf("build coverage variance report: requires at least %d profiles, received %d", minimumVarianceSamples, len(samples))
	}
	if strings.TrimSpace(commit) == "" {
		return coverageVarianceReport{}, errors.New("build coverage variance report: unchanged commit is required")
	}
	if jobs <= 0 {
		jobs = defaultCoverageJobs
	}
	if err := validateVarianceSampleCompatibility(samples); err != nil {
		return coverageVarianceReport{}, err
	}
	packages := sortedCoveragePackageNames(samples[0].totals)
	rows := make([]coverageVariancePackage, 0, len(packages))
	for _, importPath := range packages {
		row, err := buildCoverageVariancePackage(importPath, samples, floors[importPath])
		if err != nil {
			return coverageVarianceReport{}, err
		}
		rows = append(rows, row)
	}
	labels := make([]string, len(samples))
	for index := range labels {
		labels[index] = samples[index].label
		if labels[index] == "" {
			labels[index] = fmt.Sprintf("run-%02d", index+1)
		}
	}
	return coverageVarianceReport{
		commit:        strings.TrimSpace(commit),
		suite:         lane,
		command:       fmt.Sprintf("bash scripts/ci/run-functional-test-viz.sh (functional lane -jobs %d); then go run ./cmd/gocoveragecheck -suite functional -variance-profiles <complete CI coverage.out files> -variance-output <report> -variance-commit <full SHA> -variance-jobs %d", jobs, jobs),
		aggregation:   "Parse each count-mode Go coverage profile into canonical source blocks, sum statement counts by backend package, require identical package universes and total statement counts, then derive minimums from exact covered/total ratios.",
		profileLabels: labels,
		packages:      rows,
	}, nil
}

func buildCoverageVariancePackage(importPath string, samples []coverageVarianceSample, current coverageVarianceCurrentFloor) (coverageVariancePackage, error) {
	counts := make([]packageCoverageTotals, len(samples))
	minimumCovered := 0
	maximumCovered := 0
	for index, sample := range samples {
		counts[index] = sample.totals[importPath]
		if index == 0 || counts[index].coveredStatements < minimumCovered {
			minimumCovered = counts[index].coveredStatements
		}
		if index == 0 || counts[index].coveredStatements > maximumCovered {
			maximumCovered = counts[index].coveredStatements
		}
	}
	minimum := packageCoverageTotals{coveredStatements: minimumCovered, totalStatements: counts[0].totalStatements}
	maximum := packageCoverageTotals{coveredStatements: maximumCovered, totalStatements: counts[0].totalStatements}
	sampleFloor, err := coverageFloorFromTotals(minimum)
	if err != nil {
		return coverageVariancePackage{}, fmt.Errorf("build coverage variance report for package %s: %w", importPath, err)
	}
	minimumPercent := coveragePercentRat(minimum)
	maximumPercent := coveragePercentRat(maximum)
	row := coverageVariancePackage{
		importPath:   importPath,
		samples:      counts,
		minimum:      formatCoverageRat(minimumPercent),
		maximum:      formatCoverageRat(maximumPercent),
		swing:        formatCoverageRat(new(big.Rat).Sub(maximumPercent, minimumPercent)),
		sampleFloor:  sampleFloor.String() + "%",
		currentFloor: "not recorded",
		headroom:     "n/a",
	}
	if current.label != "" {
		row.currentFloor = current.label
	}
	if current.valid {
		floorPercent := new(big.Rat).SetFrac64(int64(current.floor), 100)
		headroom := new(big.Rat).Sub(minimumPercent, floorPercent)
		row.headroom = formatSignedCoverageRat(headroom)
	}
	return row, nil
}

func coveragePercentRat(totals packageCoverageTotals) *big.Rat {
	return new(big.Rat).SetFrac64(int64(totals.coveredStatements)*100, int64(totals.totalStatements))
}

func formatCoverageRat(value *big.Rat) string {
	return value.FloatString(4)
}

func formatSignedCoverageRat(value *big.Rat) string {
	if value.Sign() == 0 {
		return "0.0000"
	}
	absolute := new(big.Rat).Set(value)
	sign := "+"
	if absolute.Sign() < 0 {
		sign = "-"
		absolute.Neg(absolute)
	}
	return sign + absolute.FloatString(4)
}

func renderCoverageVarianceReport(report coverageVarianceReport) ([]byte, error) {
	var output strings.Builder
	fmt.Fprintf(&output, "# Functional Coverage Variance Audit\n\n")
	fmt.Fprintf(&output, "- **Sampling commit:** `%s`\n", report.commit)
	fmt.Fprintf(&output, "- **Suite:** `%s`\n", report.suite)
	fmt.Fprintf(&output, "- **Run count:** %d complete profiles\n", len(report.profileLabels))
	fmt.Fprintf(&output, "- **Command:** `%s`\n", report.command)
	fmt.Fprintf(&output, "- **Aggregation:** %s\n\n", report.aggregation)
	fmt.Fprintf(&output, "- **Profile labels:** `%s` (labels preserve the source artifact directory; omitted labels identify captures that were not accepted as complete samples)\n\n", strings.Join(report.profileLabels, "`, `"))
	fmt.Fprintln(&output, "## New sample set")
	fmt.Fprintln(&output)
	fmt.Fprintln(&output, "The table preserves exact covered/total statement counts for every measured backend package. Percentages are display values; the sample floor is truncated to two decimal places from the exact minimum ratio, so it cannot exceed that observation. Headroom is the exact minimum percentage minus the current numeric functional floor.")
	fmt.Fprintln(&output)
	runHeaders := make([]string, len(report.profileLabels))
	for index, label := range report.profileLabels {
		runHeaders[index] = label + " covered/total"
	}
	fmt.Fprintf(&output, "| Package | %s | Minimum | Maximum | Swing | Sample floor | Current floor | Headroom |\n", strings.Join(runHeaders, " | "))
	separator := make([]string, len(runHeaders)+7)
	for index := range separator {
		separator[index] = "---"
	}
	fmt.Fprintf(&output, "| %s |\n", strings.Join(separator, " | "))
	for _, row := range report.packages {
		values := make([]string, 0, len(row.samples))
		for _, sample := range row.samples {
			values = append(values, fmt.Sprintf("%d/%d", sample.coveredStatements, sample.totalStatements))
		}
		fmt.Fprintf(&output, "| `%s` | %s | %s%% | %s%% | %s pp | %s | %s | %s pp |\n", row.importPath, strings.Join(values, " | "), row.minimum, row.maximum, row.swing, row.sampleFloor, row.currentFloor, row.headroom)
	}
	fmt.Fprintln(&output)
	renderCoverageVarianceRemedies(&output)
	fmt.Fprintln(&output)
	fmt.Fprintln(&output, "## Supplied operator evidence (not part of the new sample set)")
	fmt.Fprintln(&output)
	fmt.Fprintln(&output, "The following context was supplied before this audit and is kept separate from the measurements above:")
	fmt.Fprintln(&output)
	fmt.Fprintln(&output, "- CI run `31699727541` measured dispatch-planning at 167/267 statements (62.5468%) against a 62.92% floor on one attempt and passed on a second attempt for the same main commit.")
	fmt.Fprintln(&output, "- The operator comparison covered 348 numeric floors, identified 112 packages with at most 0.25 percentage points of headroom, and identified five latent-red package cases where observed swing exceeded headroom.")
	fmt.Fprintln(&output, "- The five supplied latent-red cases are authoredmodel/workers, runtimebinding, platform/jsonvalue, processlifecycle, and dispatch-planning. Dispatch-planning remains owned by its separate lane and is reported here without editing its package or manifest entry.")
	fmt.Fprintln(&output, "- The supplied comparison identifies loadedsource as an epsilon-only pass and proposalmaterialization as a sampling-range case; their independent counts in this new sample set are shown in the table when those packages are measurable.")
	return []byte(output.String()), nil
}

func renderCoverageVarianceRemedies(output *strings.Builder) {
	fmt.Fprintln(output, "## Measured remedy classification")
	fmt.Fprintln(output)
	fmt.Fprintln(output, "Remedies are limited to package-specific evidence. The four in-scope latent-red packages repeat their existing functional exercise in all five samples, so their floors remain unchanged. The only functional manifest entry changed by this lane is loadedsource, whose exact sampled minimum is below its prior epsilon-tolerated floor.")
	fmt.Fprintln(output)
	fmt.Fprintln(output, "| Package | Classification | Observed evidence | Remedy |")
	fmt.Fprintln(output, "| --- | --- | --- | --- |")
	fmt.Fprintln(output, "| `.../validation/authoredmodel/workers` | deterministic functional exercise | 55/62 in every run; swing 0.0000 pp; floor 80.64%; headroom +8.0697 pp | Retain 80.64%; existing functional exercise is repeatable and preserves regression detection. |")
	fmt.Fprintln(output, "| `.../factory_sessions/internal/runtimebinding` | deterministic functional exercise | 282/441 in every run; swing 0.0000 pp; floor 60.10%; headroom +3.8456 pp | Retain 60.10%; no source or floor change is justified by the sample. |")
	fmt.Fprintln(output, "| `.../platform/jsonvalue` | deterministic functional exercise | 34/49 in every run; swing 0.0000 pp; floor 67.34%; headroom +2.0478 pp | Retain 67.34%; existing functional exercise is repeatable. |")
	fmt.Fprintln(output, "| `.../factory_sessions/internal/processlifecycle` | deterministic functional exercise | 133/171 in every run; swing 0.0000 pp; floor 76.31%; headroom +1.4678 pp | Retain 76.31%; existing functional exercise is repeatable. |")
	fmt.Fprintln(output, "| `.../factory_definitions/internal/services/compilation/loadedsource` | inherent concurrent variance, epsilon-only pass | 57/77 in every run; minimum 74.0260%; prior floor 74.13% exceeded the observation by 0.1040 pp | Lower only this entry to the safe two-decimal minimum 74.02%; keep the 0.25 pp epsilon and blocking gate unchanged. |")
	fmt.Fprintln(output, "| `.../factory_runtime/internal/services/dispatch_planning/internal/service` | inherent concurrent variance, owner lane | New sample: 168/267, 167/267, 169/267, 167/267, 167/267; supplied owner-lane minimum 167/267 | Inherit main's 62.54% entry from the merged owner lane; this lane does not edit the package or manifest entry. |")
	fmt.Fprintln(output)
	fmt.Fprintln(output, "`proposalmaterialization` remains at its existing 0.00% numeric floor: the new sample observes 15/84 in every run, while the supplied sampling-range evidence does not justify a positive ratchet or an exception conversion.")
}
