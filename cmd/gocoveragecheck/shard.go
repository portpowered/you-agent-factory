package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
)

// Functional coverage sharding splits one selected package set across N
// independent `go test` runs and rejoins them in a single gate run.
//
// Shard mode (-shard-index/-shard-count) executes only its slice of the
// selected packages and writes its profile, timing summary, and a manifest
// naming the full selected set and its own slice. It never evaluates floors.
//
// Aggregate mode (-merge-shards) runs no tests. It recomputes the selected set
// itself, requires every shard manifest, proves the shard slices are disjoint
// and their union equals that set, merges the profiles and timings, and then
// runs the unchanged evaluation and package-floor gate.

const (
	shardManifestVersion = 1
	shardTimingsVersion  = 1

	shardManifestFile = "shard-manifest.json"
	shardProfileFile  = "coverage.out"
	shardTimingFile   = "timing.json"

	// shardArtifactPrefix names each shard's directory below -merge-shards,
	// matching the actions/download-artifact layout of one directory per
	// uploaded artifact.
	shardArtifactPrefix = "functional-coverage-shard-"

	// defaultShardPackageOverheadSeconds approximates the per-package compile
	// and link cost that every instrumented test binary pays regardless of how
	// long its tests run.
	defaultShardPackageOverheadSeconds = 4.0
)

// shardTimings is the checked-in per-package elapsed table used to balance
// shards. Packages missing from it are weighted at the table median, so a new
// package still lands somewhere sensible before the table is refreshed.
type shardTimings struct {
	Version         int                `json:"version"`
	Source          string             `json:"source,omitempty"`
	OverheadSeconds float64            `json:"overheadSeconds"`
	Packages        map[string]float64 `json:"packages"`
}

type shardManifest struct {
	Version          int      `json:"version"`
	ShardIndex       int      `json:"shardIndex"`
	ShardCount       int      `json:"shardCount"`
	SelectedSHA256   string   `json:"selectedSha256"`
	SelectedPackages []string `json:"selectedPackages"`
	ShardPackages    []string `json:"shardPackages"`
	AssignedWeight   float64  `json:"assignedWeightSeconds"`
}

type shardResult struct {
	index    int
	dir      string
	manifest shardManifest
}

func (cfg config) shardRunMode() bool {
	return cfg.shardCount > 0 && cfg.shardIndex >= 0 && strings.TrimSpace(cfg.mergeShards) == ""
}

func (cfg config) shardAggregateMode() bool {
	return strings.TrimSpace(cfg.mergeShards) != ""
}

func validateShardConfig(cfg config) error {
	run := cfg.shardCount > 0 || strings.TrimSpace(cfg.shardTimings) != "" || strings.TrimSpace(cfg.shardManifestOutput) != ""
	aggregate := cfg.shardAggregateMode()
	if !run && !aggregate {
		return nil
	}
	if cfg.suite != functionalCoverageSuite {
		return fmt.Errorf("configure functional shards: -suite must be %q (got %q)", functionalCoverageSuite, cfg.suite)
	}
	if strings.TrimSpace(cfg.functionalQuarantine) == "" {
		return errors.New("configure functional shards: -functional-quarantine is required so every shard selects from the same quarantined set")
	}
	if strings.TrimSpace(cfg.packages) != "" {
		return errors.New("configure functional shards: -packages overrides package selection and cannot be sharded")
	}
	if cfg.shardCount < 1 {
		return errors.New("configure functional shards: -shard-count must be at least 1")
	}
	if aggregate {
		if cfg.shardIndex >= 0 {
			return errors.New("configure functional shards: -merge-shards cannot be combined with -shard-index")
		}
		if strings.TrimSpace(cfg.profile) == "" {
			return errors.New("configure functional shards: -merge-shards requires -profile for the merged coverage profile")
		}
		return nil
	}
	if cfg.shardIndex < 0 || cfg.shardIndex >= cfg.shardCount {
		return fmt.Errorf("configure functional shards: -shard-index must be in [0,%d) (got %d)", cfg.shardCount, cfg.shardIndex)
	}
	if strings.TrimSpace(cfg.shardManifestOutput) == "" {
		return errors.New("configure functional shards: -shard-manifest-output is required in shard mode")
	}
	if strings.TrimSpace(cfg.profile) == "" {
		return errors.New("configure functional shards: -profile is required in shard mode")
	}
	return nil
}

func loadShardTimings(path string) (shardTimings, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return shardTimings{}, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return shardTimings{}, fmt.Errorf("read functional shard timings: %w", err)
	}
	var timings shardTimings
	if err := json.Unmarshal(data, &timings); err != nil {
		return shardTimings{}, fmt.Errorf("decode functional shard timings %s: %w", path, err)
	}
	if timings.Version != shardTimingsVersion {
		return shardTimings{}, fmt.Errorf("functional shard timings %s: unsupported version %d", path, timings.Version)
	}
	for pkg, seconds := range timings.Packages {
		if seconds < 0 || math.IsNaN(seconds) || math.IsInf(seconds, 0) {
			return shardTimings{}, fmt.Errorf("functional shard timings %s: package %s has invalid seconds %v", path, pkg, seconds)
		}
	}
	return timings, nil
}

// shardWeights returns one balancing weight per package: recorded elapsed
// seconds (or the table median when unrecorded) plus the fixed per-package
// overhead. Without any table every package weighs the same.
func shardWeights(packages []string, timings shardTimings) map[string]float64 {
	overhead := timings.OverheadSeconds
	if overhead <= 0 {
		overhead = defaultShardPackageOverheadSeconds
	}
	known := make([]float64, 0, len(timings.Packages))
	for _, seconds := range timings.Packages {
		known = append(known, seconds)
	}
	fallback := 0.0
	if len(known) > 0 {
		slices.Sort(known)
		fallback = known[len(known)/2]
	}
	weights := make(map[string]float64, len(packages))
	for _, pkg := range packages {
		seconds, ok := timings.Packages[pkg]
		if !ok {
			seconds = fallback
		}
		weights[pkg] = seconds + overhead
	}
	return weights
}

// assignShards partitions packages into count slices with greedy
// longest-processing-time-first assignment. Ties break on package path and
// then on the lowest shard index, so the result depends only on the package
// set and the weights, never on discovery order.
func assignShards(packages []string, count int, weights map[string]float64) ([][]string, []float64) {
	shards := make([][]string, count)
	loads := make([]float64, count)
	ordered := sortedUniqueStrings(packages)
	slices.SortStableFunc(ordered, func(left, right string) int {
		switch {
		case weights[left] > weights[right]:
			return -1
		case weights[left] < weights[right]:
			return 1
		default:
			return strings.Compare(left, right)
		}
	})
	for _, pkg := range ordered {
		lightest := 0
		for index := 1; index < count; index++ {
			if loads[index] < loads[lightest] {
				lightest = index
			}
		}
		shards[lightest] = append(shards[lightest], pkg)
		loads[lightest] += weights[pkg]
	}
	for index := range shards {
		slices.Sort(shards[index])
	}
	return shards, loads
}

func packageSetDigest(packages []string) string {
	sum := sha256.Sum256([]byte(strings.Join(sortedUniqueStrings(packages), "\n")))
	return hex.EncodeToString(sum[:])
}

// buildShardManifest assigns the selected packages and returns this shard's
// slice. A shard that would receive no package is an error: an empty shard is
// a silent loss of parallelism and usually a misconfigured count.
func buildShardManifest(cfg config, selected []string) (shardManifest, error) {
	timings, err := loadShardTimings(cfg.shardTimings)
	if err != nil {
		return shardManifest{}, err
	}
	selected = sortedUniqueStrings(selected)
	if len(selected) < cfg.shardCount {
		return shardManifest{}, fmt.Errorf("shard functional coverage: %d selected packages cannot fill %d shards", len(selected), cfg.shardCount)
	}
	assigned, loads := assignShards(selected, cfg.shardCount, shardWeights(selected, timings))
	return shardManifest{
		Version:          shardManifestVersion,
		ShardIndex:       cfg.shardIndex,
		ShardCount:       cfg.shardCount,
		SelectedSHA256:   packageSetDigest(selected),
		SelectedPackages: selected,
		ShardPackages:    assigned[cfg.shardIndex],
		AssignedWeight:   math.Round(loads[cfg.shardIndex]*1000) / 1000,
	}, nil
}

// restrictSelectionToPackages keeps only the named packages in a quarantine
// selection, preserving each group's run pattern so test-level quarantine is
// applied exactly as it is in an unsharded run.
func restrictSelectionToPackages(selection functionalCoverageSelection, keep []string) functionalCoverageSelection {
	keepSet := make(map[string]struct{}, len(keep))
	for _, pkg := range keep {
		keepSet[pkg] = struct{}{}
	}
	restricted := selection
	restricted.Groups = nil
	restricted.SelectedTests = make(map[string][]string)
	restricted.SelectedPackageCount = 0
	restricted.SelectedTestCount = 0
	for _, group := range selection.Groups {
		var packages []string
		for _, pkg := range group.Packages {
			if _, ok := keepSet[pkg]; ok {
				packages = append(packages, pkg)
			}
		}
		if len(packages) == 0 {
			continue
		}
		restricted.Groups = append(restricted.Groups, coverageRunGroup{Packages: packages, RunPattern: group.RunPattern})
		restricted.SelectedPackageCount += len(packages)
		for _, pkg := range packages {
			restricted.SelectedTests[pkg] = append([]string(nil), selection.SelectedTests[pkg]...)
			restricted.SelectedTestCount += len(selection.SelectedTests[pkg])
		}
	}
	return restricted
}

func writeShardManifest(path string, manifest shardManifest) error {
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("encode functional shard manifest: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create functional shard manifest directory: %w", err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("write functional shard manifest: %w", err)
	}
	return nil
}

func readShardManifest(path string) (shardManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return shardManifest{}, fmt.Errorf("read functional shard manifest: %w", err)
	}
	var manifest shardManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return shardManifest{}, fmt.Errorf("decode functional shard manifest %s: %w", path, err)
	}
	if manifest.Version != shardManifestVersion {
		return shardManifest{}, fmt.Errorf("functional shard manifest %s: unsupported version %d", path, manifest.Version)
	}
	return manifest, nil
}

func shardArtifactDir(root string, index int) string {
	return filepath.Join(root, fmt.Sprintf("%s%d", shardArtifactPrefix, index))
}

// loadShardResults reads the manifest of every expected shard. A missing
// directory or manifest is an error, so a shard that never uploaded fails the
// gate instead of shrinking the measured set.
func loadShardResults(root string, count int) ([]shardResult, error) {
	results := make([]shardResult, 0, count)
	var problems []error
	for index := 0; index < count; index++ {
		dir := shardArtifactDir(root, index)
		manifest, err := readShardManifest(filepath.Join(dir, shardManifestFile))
		if err != nil {
			problems = append(problems, fmt.Errorf("shard %d: %w", index, err))
			continue
		}
		for _, name := range []string{shardProfileFile, shardTimingFile} {
			if info, statErr := os.Stat(filepath.Join(dir, name)); statErr != nil || info.IsDir() || info.Size() == 0 {
				problems = append(problems, fmt.Errorf("shard %d: required artifact %s is missing or empty", index, name))
			}
		}
		results = append(results, shardResult{index: index, dir: dir, manifest: manifest})
	}
	return results, errors.Join(problems...)
}

// verifyShardPartition proves the shard manifests form an exact partition of
// the independently recomputed selected set: every shard present once, all
// agreeing on count and set, slices disjoint, union equal to the set.
func verifyShardPartition(manifests []shardManifest, count int, selected []string) error {
	selected = sortedUniqueStrings(selected)
	wantDigest := packageSetDigest(selected)
	var problems []string
	seenIndex := make(map[int]struct{}, count)
	owner := make(map[string]int)
	for _, manifest := range manifests {
		if manifest.ShardCount != count {
			problems = append(problems, fmt.Sprintf("shard %d declares shard-count %d, want %d", manifest.ShardIndex, manifest.ShardCount, count))
		}
		if manifest.ShardIndex < 0 || manifest.ShardIndex >= count {
			problems = append(problems, fmt.Sprintf("shard index %d is outside [0,%d)", manifest.ShardIndex, count))
			continue
		}
		if _, dup := seenIndex[manifest.ShardIndex]; dup {
			problems = append(problems, fmt.Sprintf("shard %d appears more than once", manifest.ShardIndex))
		}
		seenIndex[manifest.ShardIndex] = struct{}{}
		if manifest.SelectedSHA256 != wantDigest || packageSetDigest(manifest.SelectedPackages) != wantDigest {
			problems = append(problems, fmt.Sprintf("shard %d selected a different package set than the aggregate (%d packages, want %d)", manifest.ShardIndex, len(manifest.SelectedPackages), len(selected)))
		}
		for _, pkg := range manifest.ShardPackages {
			if previous, taken := owner[pkg]; taken {
				problems = append(problems, fmt.Sprintf("package %s is assigned to shards %d and %d", pkg, previous, manifest.ShardIndex))
				continue
			}
			owner[pkg] = manifest.ShardIndex
		}
	}
	for index := 0; index < count; index++ {
		if _, ok := seenIndex[index]; !ok {
			problems = append(problems, fmt.Sprintf("shard %d manifest is missing", index))
		}
	}
	selectedSet := make(map[string]struct{}, len(selected))
	for _, pkg := range selected {
		selectedSet[pkg] = struct{}{}
		if _, ok := owner[pkg]; !ok {
			problems = append(problems, fmt.Sprintf("selected package %s was not run by any shard", pkg))
		}
	}
	for pkg := range owner {
		if _, ok := selectedSet[pkg]; !ok {
			problems = append(problems, fmt.Sprintf("shard ran package %s that is not in the selected set", pkg))
		}
	}
	if len(problems) > 0 {
		slices.Sort(problems)
		return fmt.Errorf("functional shard partition check failed:\n  %s", strings.Join(problems, "\n  "))
	}
	return nil
}

// mergeShardTimingSummaries combines per-shard timing summaries into the one
// summary the report consumes. It is complete only if every shard summary is
// complete and every selected package reported exactly once.
func mergeShardTimingSummaries(summaries []functionalTimingSummaryJSON, selected []string) functionalTimingSummaryJSON {
	merged := functionalTimingSummaryJSON{
		Version:              functionalTimingSummaryVersion,
		Complete:             true,
		ExpectedPackageCount: len(selected),
		Packages:             []functionalPackageTimingJSON{},
		Tests:                []functionalTestTimingJSON{},
	}
	seen := make(map[string]struct{}, len(selected))
	var reasons []string
	for index, summary := range summaries {
		if !summary.Complete {
			merged.Complete = false
			reasons = append(reasons, fmt.Sprintf("shard %d timing incomplete: %s", index, summary.CaptureReason))
		}
		merged.WallSeconds = math.Max(merged.WallSeconds, summary.WallSeconds)
		merged.PackageElapsedSecondsSum += summary.PackageElapsedSecondsSum
		for _, pkg := range summary.Packages {
			if _, dup := seen[pkg.Package]; dup {
				merged.Complete = false
				reasons = append(reasons, "package "+pkg.Package+" reported by more than one shard")
				continue
			}
			seen[pkg.Package] = struct{}{}
			merged.Packages = append(merged.Packages, pkg)
		}
		merged.Tests = append(merged.Tests, summary.Tests...)
	}
	for _, pkg := range selected {
		if _, ok := seen[pkg]; !ok {
			merged.Complete = false
			reasons = append(reasons, "package "+pkg+" has no timing record")
		}
	}
	slices.SortFunc(merged.Packages, func(left, right functionalPackageTimingJSON) int {
		return strings.Compare(left.Package, right.Package)
	})
	slices.SortFunc(merged.Tests, func(left, right functionalTestTimingJSON) int {
		if result := strings.Compare(left.Package, right.Package); result != 0 {
			return result
		}
		return strings.Compare(left.Test, right.Test)
	})
	merged.PackageElapsedSecondsSum = roundTimingSeconds(merged.PackageElapsedSecondsSum)
	merged.PackageCount = len(merged.Packages)
	merged.TestCount = len(merged.Tests)
	merged.TestPassCount, merged.TestFailCount, merged.TestSkipCount = countTimingTestOutcomes(merged.Tests)
	if len(reasons) > 0 {
		slices.Sort(reasons)
		merged.CaptureReason = strings.Join(reasons, "; ")
	}
	return merged
}

func readShardTimingSummary(path string) (functionalTimingSummaryJSON, error) {
	var summary functionalTimingSummaryJSON
	data, err := os.ReadFile(path)
	if err != nil {
		return functionalTimingSummaryJSON{}, fmt.Errorf("read functional shard timing summary: %w", err)
	}
	if err := json.Unmarshal(data, &summary); err != nil {
		return functionalTimingSummaryJSON{}, fmt.Errorf("decode functional shard timing summary %s: %w", path, err)
	}
	return summary, nil
}

// runShardAggregate is the aggregate-mode replacement for the test run: it
// recomputes the selected set, verifies the shard partition, merges timings and
// profiles into the paths a normal run would have produced, and returns the
// canonical blocks the unchanged evaluation consumes.
func runShardAggregate(cfg config, profilePath string, repoRoot string, coverPackages []string) (map[string]coverageBlock, error) {
	skipped := skippedQuarantineVerification()
	packages, listed, started, err := resolveCoverageTestPackages(cfg, repoRoot, skipped)
	if err != nil {
		return nil, err
	}
	selected, _, err := prepareCoverageTestPackagesWithVerification(cfg, packages, runtime.GOOS, runtime.NumCPU(), repoRoot, listed, started, skipped)
	if err != nil {
		return nil, err
	}

	results, err := loadShardResults(cfg.mergeShards, cfg.shardCount)
	if err != nil {
		return nil, err
	}
	manifests := make([]shardManifest, 0, len(results))
	for _, result := range results {
		manifests = append(manifests, result.manifest)
	}
	if err := verifyShardPartition(manifests, cfg.shardCount, selected); err != nil {
		return nil, err
	}
	fmt.Fprintf(stdoutWriter, "Functional shards: count=%d selected-packages=%d partition=exact\n", cfg.shardCount, len(selected))

	summaries := make([]functionalTimingSummaryJSON, 0, len(results))
	profiles := make([]string, 0, len(results))
	for _, result := range results {
		summary, err := readShardTimingSummary(filepath.Join(result.dir, shardTimingFile))
		if err != nil {
			return nil, err
		}
		summaries = append(summaries, summary)
		profiles = append(profiles, filepath.Join(result.dir, shardProfileFile))
	}
	timing := mergeShardTimingSummaries(summaries, selected)
	if err := writeFunctionalTimingSummaryJSON(cfg.timingOutput, timing); err != nil {
		return nil, err
	}
	if !timing.Complete {
		return nil, fmt.Errorf("merge functional shard timings: %s", timing.CaptureReason)
	}
	if timing.TestFailCount > 0 {
		return nil, fmt.Errorf("merge functional shard timings: %d top-level tests failed", timing.TestFailCount)
	}
	if err := writeFunctionalTimingReport(timing); err != nil {
		return nil, err
	}
	return mergeCoverageProfilesWithBlocks(profiles, profilePath, repoRoot, coverPackages)
}

// errShardRunComplete ends a shard-mode run after its tests pass: a shard only
// produces artifacts and never evaluates floors.
var errShardRunComplete = errors.New("functional shard run complete")

// skippedQuarantineVerification stands in for the in-process selector and
// ratchet checks. Sharded runs perform them once in the dedicated quarantine
// job instead of in every shard and the aggregate.
func skippedQuarantineVerification() *functionalQuarantineSelectorVerification {
	verification := newFunctionalQuarantineSelectorVerification(0)
	verification.done <- nil
	verification.overlapCoverage = true
	verification.skipped = true
	return verification
}
