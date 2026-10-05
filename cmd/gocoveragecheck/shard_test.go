package main

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func testShardPackages(count int) []string {
	packages := make([]string, 0, count)
	for index := 0; index < count; index++ {
		packages = append(packages, fmt.Sprintf("example/tests/functional/p%03d", index))
	}
	return packages
}

func testShardWeights(packages []string) map[string]float64 {
	weights := make(map[string]float64, len(packages))
	for index, pkg := range packages {
		weights[pkg] = float64((index*37)%90 + 5)
	}
	return weights
}

func TestAssignShardsPartitionsExactlyAndDeterministically(t *testing.T) {
	packages := testShardPackages(60)
	weights := testShardWeights(packages)

	first, loads := assignShards(packages, 4, weights)
	reversed := slices.Clone(packages)
	slices.Reverse(reversed)
	second, _ := assignShards(reversed, 4, weights)

	if fmt.Sprint(first) != fmt.Sprint(second) {
		t.Fatalf("assignment depends on input order:\n%v\n%v", first, second)
	}
	var union []string
	heaviest := 0.0
	for _, shard := range first {
		union = append(union, shard...)
	}
	for _, weight := range weights {
		heaviest = math.Max(heaviest, weight)
	}
	slices.Sort(union)
	if !slices.Equal(union, packages) {
		t.Fatalf("shards do not partition the package set: got %d packages, want %d", len(union), len(packages))
	}
	if spread := slices.Max(loads) - slices.Min(loads); spread > heaviest {
		t.Fatalf("greedy longest-first spread %.1f exceeds the heaviest package %.1f", spread, heaviest)
	}
}

func TestShardWeightsFallBackToMedianPlusOverhead(t *testing.T) {
	timings := shardTimings{
		OverheadSeconds: 2,
		Packages:        map[string]float64{"a": 10, "b": 30, "c": 20},
	}
	weights := shardWeights([]string{"a", "new"}, timings)
	if weights["a"] != 12 {
		t.Fatalf("recorded weight = %v, want 12", weights["a"])
	}
	if weights["new"] != 22 {
		t.Fatalf("unrecorded package weight = %v, want median 20 + overhead 2", weights["new"])
	}
	if got := shardWeights([]string{"a"}, shardTimings{})["a"]; got != defaultShardPackageOverheadSeconds {
		t.Fatalf("no-table weight = %v, want the default overhead", got)
	}
}

func buildTestManifests(t *testing.T, packages []string, count int) []shardManifest {
	t.Helper()
	manifests := make([]shardManifest, 0, count)
	for index := 0; index < count; index++ {
		manifest, err := buildShardManifest(config{shardCount: count, shardIndex: index}, packages)
		if err != nil {
			t.Fatal(err)
		}
		manifests = append(manifests, manifest)
	}
	return manifests
}

func TestBuildShardManifestSlicesAgreeAcrossShards(t *testing.T) {
	packages := testShardPackages(23)
	manifests := buildTestManifests(t, packages, 4)
	if err := verifyShardPartition(manifests, 4, packages); err != nil {
		t.Fatalf("shards built from one set must partition it: %v", err)
	}
	if _, err := buildShardManifest(config{shardCount: 30, shardIndex: 0}, packages); err == nil {
		t.Fatal("expected an error when there are fewer packages than shards")
	}
}

func TestVerifyShardPartitionFailsClosed(t *testing.T) {
	packages := testShardPackages(12)
	tests := []struct {
		name    string
		mutate  func([]shardManifest) []shardManifest
		selects []string
		want    string
	}{
		{"missing shard", func(m []shardManifest) []shardManifest { return m[:2] }, packages, "shard 2 manifest is missing"},
		{"duplicate shard", func(m []shardManifest) []shardManifest { return append(m, m[0]) }, packages, "appears more than once"},
		{"overlap", func(m []shardManifest) []shardManifest {
			m[1].ShardPackages = append(slices.Clone(m[1].ShardPackages), m[0].ShardPackages[0])
			return m
		}, packages, "is assigned to shards 0 and 1"},
		{"dropped package", func(m []shardManifest) []shardManifest {
			m[2].ShardPackages = m[2].ShardPackages[1:]
			return m
		}, packages, "was not run by any shard"},
		{"extra package", func(m []shardManifest) []shardManifest {
			m[0].ShardPackages = append(slices.Clone(m[0].ShardPackages), "example/tests/functional/extra")
			return m
		}, packages, "not in the selected set"},
		{"stale selection", func(m []shardManifest) []shardManifest { return m }, packages[:11], "different package set"},
		{"wrong count", func(m []shardManifest) []shardManifest { m[0].ShardCount = 4; return m }, packages, "declares shard-count 4"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := verifyShardPartition(test.mutate(buildTestManifests(t, packages, 3)), 3, test.selects)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want it to contain %q", err, test.want)
			}
		})
	}
}

func TestRestrictSelectionKeepsGroupRunPatterns(t *testing.T) {
	selection := functionalCoverageSelection{
		Groups: []coverageRunGroup{
			{Packages: []string{"a", "b"}},
			{Packages: []string{"c", "d"}, RunPattern: "^(?:TestOne)$"},
		},
		SelectedTests: map[string][]string{"a": {"TestA"}, "b": {"TestB"}, "c": {"TestOne"}, "d": {"TestOne", "TestTwo"}},
	}
	restricted := restrictSelectionToPackages(selection, []string{"b", "d"})
	if restricted.SelectedPackageCount != 2 || restricted.SelectedTestCount != 3 {
		t.Fatalf("counts = %d packages, %d tests", restricted.SelectedPackageCount, restricted.SelectedTestCount)
	}
	if len(restricted.Groups) != 2 || restricted.Groups[1].RunPattern != "^(?:TestOne)$" {
		t.Fatalf("groups lost their run pattern: %+v", restricted.Groups)
	}
	if got := selectedFunctionalPackages(restricted); !slices.Equal(got, []string{"b", "d"}) {
		t.Fatalf("packages = %v", got)
	}
	if _, ok := restricted.SelectedTests["a"]; ok {
		t.Fatal("restricted selection must not expect tests from another shard")
	}
}

func TestMergeShardTimingSummaries(t *testing.T) {
	pass := func(pkg string, seconds float64) functionalPackageTimingJSON {
		return functionalPackageTimingJSON{Package: pkg, Seconds: seconds, Outcome: timingOutcomePass}
	}
	first := functionalTimingSummaryJSON{Complete: true, WallSeconds: 10, PackageElapsedSecondsSum: 5, Packages: []functionalPackageTimingJSON{pass("a", 5)}, Tests: []functionalTestTimingJSON{{Package: "a", Test: "TestA", Outcome: timingOutcomePass}}}
	second := functionalTimingSummaryJSON{Complete: true, WallSeconds: 12, PackageElapsedSecondsSum: 7, Packages: []functionalPackageTimingJSON{pass("b", 7)}, Tests: []functionalTestTimingJSON{{Package: "b", Test: "TestB", Outcome: timingOutcomeSkip}}}

	merged := mergeShardTimingSummaries([]functionalTimingSummaryJSON{first, second}, []string{"a", "b"})
	if !merged.Complete || merged.WallSeconds != 12 || merged.PackageElapsedSecondsSum != 12 || merged.PackageCount != 2 || merged.TestPassCount != 1 || merged.TestSkipCount != 1 {
		t.Fatalf("unexpected merge: %+v", merged)
	}

	missing := mergeShardTimingSummaries([]functionalTimingSummaryJSON{first, second}, []string{"a", "b", "c"})
	if missing.Complete || !strings.Contains(missing.CaptureReason, "package c has no timing record") {
		t.Fatalf("a selected package without timing must make the merge incomplete: %+v", missing)
	}
	second.Complete = false
	second.CaptureReason = "ended early"
	if merged := mergeShardTimingSummaries([]functionalTimingSummaryJSON{first, second}, []string{"a", "b"}); merged.Complete {
		t.Fatal("an incomplete shard summary must make the merge incomplete")
	}
	if duplicate := mergeShardTimingSummaries([]functionalTimingSummaryJSON{first, first}, []string{"a"}); duplicate.Complete {
		t.Fatal("a package reported by two shards must make the merge incomplete")
	}
}

func TestLoadShardResultsRequiresEveryShardArtifact(t *testing.T) {
	root := t.TempDir()
	for index, manifest := range buildTestManifests(t, testShardPackages(8), 2) {
		dir := shardArtifactDir(root, index)
		if err := writeShardManifest(filepath.Join(dir, shardManifestFile), manifest); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, shardProfileFile), []byte("mode: count\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := loadShardResults(root, 2); err == nil || !strings.Contains(err.Error(), "timing.json is missing or empty") {
		t.Fatalf("missing timing artifact must fail, got %v", err)
	}
	for index := 0; index < 2; index++ {
		if err := os.WriteFile(filepath.Join(shardArtifactDir(root, index), shardTimingFile), []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := loadShardResults(root, 2); err != nil {
		t.Fatalf("complete shard set must load: %v", err)
	}
	if _, err := loadShardResults(root, 3); err == nil || !strings.Contains(err.Error(), "shard 2") {
		t.Fatalf("a third expected shard that never uploaded must fail, got %v", err)
	}
}

func TestValidateShardConfig(t *testing.T) {
	base := config{suite: functionalCoverageSuite, functionalQuarantine: "q.json", shardIndex: -1}
	shard := base
	shard.shardCount, shard.shardIndex, shard.shardManifestOutput, shard.profile = 4, 1, "m.json", "p.out"
	aggregate := base
	aggregate.shardCount, aggregate.mergeShards, aggregate.profile = 4, "dir", "p.out"

	tests := map[string]struct {
		cfg    config
		mutate func(*config)
		want   string
	}{
		"valid shard":     {cfg: shard, mutate: func(*config) {}},
		"valid aggregate": {cfg: aggregate, mutate: func(*config) {}},
		"unsharded":       {cfg: base, mutate: func(*config) {}},
		"index range":     {cfg: shard, mutate: func(c *config) { c.shardIndex = 4 }, want: "-shard-index must be in [0,4)"},
		"unit suite":      {cfg: shard, mutate: func(c *config) { c.suite = "unit" }, want: "-suite must be"},
		"no quarantine":   {cfg: shard, mutate: func(c *config) { c.functionalQuarantine = "" }, want: "-functional-quarantine is required"},
		"packages":        {cfg: shard, mutate: func(c *config) { c.packages = "./x" }, want: "cannot be sharded"},
		"no manifest":     {cfg: shard, mutate: func(c *config) { c.shardManifestOutput = "" }, want: "-shard-manifest-output"},
		"both modes":      {cfg: aggregate, mutate: func(c *config) { c.shardIndex = 0 }, want: "cannot be combined"},
		"no merged path":  {cfg: aggregate, mutate: func(c *config) { c.profile = "" }, want: "requires -profile"},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			cfg := test.cfg
			test.mutate(&cfg)
			err := validateShardConfig(cfg)
			if test.want == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

// TestCheckedInShardTimingsBalanceFourShards guards the table the workflow
// uses: it must parse and keep four shards within a few percent of each other.
func TestCheckedInShardTimingsBalanceFourShards(t *testing.T) {
	timings, err := loadShardTimings(filepath.Join("..", "..", "scripts", "ci", "functional-shard-timings.json"))
	if err != nil {
		t.Fatal(err)
	}
	packages := make([]string, 0, len(timings.Packages))
	for pkg := range timings.Packages {
		packages = append(packages, pkg)
	}
	_, loads := assignShards(packages, 4, shardWeights(packages, timings))
	maxLoad := slices.Max(loads)
	if spread := (maxLoad - slices.Min(loads)) / maxLoad; math.IsNaN(spread) || spread > 0.05 {
		t.Fatalf("four-shard load spread %.1f%% exceeds 5%% (loads %v)", spread*100, loads)
	}
}
