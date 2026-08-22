package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestPlanCoverageManifestUpdateUsesMinimumAndPreservesException(t *testing.T) {
	t.Parallel()

	alpha := modulePath + "/pkg/config"
	beta := modulePath + "/pkg/service"
	gamma := modulePath + "/pkg/factory"
	manifest := coverageManifest{
		Version: coverageManifestVersion,
		Lane:    "functional",
		Packages: []coverageManifestEntry{
			{Package: alpha, Minimum: json.RawMessage("50.00")},
			{Package: beta, Exception: &coverageManifestException{
				Kind: "measurement", Justification: "no measurable statements", Owner: "backend-quality",
				Deadline: "2027-07-15", RemovalGate: "profile reports statements",
			}},
		},
	}
	samples := coverageManifestSampleSet([]map[string]packageCoverageTotals{
		{
			alpha: {coveredStatements: 1, totalStatements: 3},
			beta:  {coveredStatements: 1, totalStatements: 4},
			gamma: {coveredStatements: 1, totalStatements: 2},
		},
		{
			alpha: {coveredStatements: 2, totalStatements: 3},
			beta:  {coveredStatements: 2, totalStatements: 4},
			gamma: {coveredStatements: 2, totalStatements: 2},
		},
		{
			alpha: {coveredStatements: 3, totalStatements: 3},
			beta:  {coveredStatements: 1, totalStatements: 4},
			gamma: {coveredStatements: 2, totalStatements: 2},
		},
		{
			alpha: {coveredStatements: 2, totalStatements: 3},
			beta:  {coveredStatements: 3, totalStatements: 4},
			gamma: {coveredStatements: 2, totalStatements: 2},
		},
		{
			alpha: {coveredStatements: 3, totalStatements: 3},
			beta:  {coveredStatements: 2, totalStatements: 4},
			gamma: {coveredStatements: 2, totalStatements: 2},
		},
	})

	minimums, err := minimumCoverageTotals(samples)
	if err != nil {
		t.Fatalf("minimumCoverageTotals() error = %v", err)
	}
	updated, updates, err := planCoverageManifestUpdate(manifest, minimums)
	if err != nil {
		t.Fatalf("planCoverageManifestUpdate() error = %v", err)
	}
	wantUpdates := []string{
		"package coverage update: package=" + alpha + " lane=functional status=lowered old=50.00% candidate=33.33%",
		"package coverage update: package=" + gamma + " lane=functional status=added old=missing candidate=50.00%",
		"package coverage update: package=" + beta + " lane=functional status=unchanged old=exception candidate=25.00%",
	}
	if got := coverageManifestUpdateStrings(updates); !slices.Equal(got, wantUpdates) {
		t.Fatalf("updates = %v, want %v", got, wantUpdates)
	}
	if got := string(updated.Packages[0].Minimum); got != "33.33" {
		t.Fatalf("minimum = %s, want exact minimum-sample floor 33.33", got)
	}
	if int64(coverageFloor(3333))*3 > 1*10000 {
		t.Fatal("two-decimal floor exceeds exact 1/3 sample ratio")
	}
	var preservedException *coverageManifestException
	for _, entry := range updated.Packages {
		if entry.Package == beta {
			preservedException = entry.Exception
			break
		}
	}
	if got := preservedException; got == nil || got.Kind != "measurement" || got.Justification != "no measurable statements" || got.Owner != "backend-quality" || got.Deadline != "2027-07-15" || got.RemovalGate != "profile reports statements" {
		t.Fatalf("exception = %+v, want all original fields preserved", got)
	}

	idempotent, secondUpdates, err := planCoverageManifestUpdate(updated, minimums)
	if err != nil {
		t.Fatalf("idempotent plan error = %v", err)
	}
	firstData, _ := renderCoverageManifest(updated)
	secondData, _ := renderCoverageManifest(idempotent)
	if string(firstData) != string(secondData) {
		t.Fatalf("idempotent plan changed manifest:\n%s\n---\n%s", firstData, secondData)
	}
	for _, update := range secondUpdates {
		if update.Status != "unchanged" {
			t.Fatalf("idempotent status = %s for %s, want unchanged", update.Status, update.Package)
		}
	}
}

func TestUpdateCoverageManifestFileWritesMinimumsAndIsByteIdempotent(t *testing.T) {
	alpha := modulePath + "/pkg/config"
	beta := modulePath + "/pkg/service"
	manifest := coverageManifest{Version: coverageManifestVersion, Lane: "unit", Packages: []coverageManifestEntry{
		{Package: alpha, Minimum: json.RawMessage("80.00")},
		{Package: beta, Minimum: json.RawMessage("50.00")},
	}}
	data, err := renderCoverageManifest(manifest)
	if err != nil {
		t.Fatalf("render manifest: %v", err)
	}
	filename := filepath.Join(t.TempDir(), "minimums.json")
	if err := os.WriteFile(filename, data, 0o644); err != nil {
		t.Fatalf("write manifest fixture: %v", err)
	}
	samples := coverageManifestSampleSet([]map[string]packageCoverageTotals{
		{alpha: {coveredStatements: 79, totalStatements: 100}, beta: {coveredStatements: 3, totalStatements: 5}},
		{alpha: {coveredStatements: 80, totalStatements: 100}, beta: {coveredStatements: 4, totalStatements: 5}},
		{alpha: {coveredStatements: 82, totalStatements: 100}, beta: {coveredStatements: 5, totalStatements: 5}},
		{alpha: {coveredStatements: 81, totalStatements: 100}, beta: {coveredStatements: 4, totalStatements: 5}},
		{alpha: {coveredStatements: 80, totalStatements: 100}, beta: {coveredStatements: 4, totalStatements: 5}},
	})

	updates, err := updateCoverageManifestFile(filename, "unit", samples)
	if err != nil {
		t.Fatalf("first update error = %v", err)
	}
	if got := coverageManifestUpdateStrings(updates); !slices.Equal(got, []string{
		"package coverage update: package=" + alpha + " lane=unit status=lowered old=80.00% candidate=79.00%",
		"package coverage update: package=" + beta + " lane=unit status=raised old=50.00% candidate=60.00%",
	}) {
		t.Fatalf("updates = %v, want deterministic minimum-based report", got)
	}
	first, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("read first manifest: %v", err)
	}
	if !strings.Contains(string(first), `"minimum": 79.00`) || !strings.Contains(string(first), `"minimum": 60.00`) {
		t.Fatalf("updated manifest = %s, want sampled minimum floors", first)
	}
	if _, err := updateCoverageManifestFile(filename, "unit", samples); err != nil {
		t.Fatalf("second update error = %v", err)
	}
	second, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("read second manifest: %v", err)
	}
	if string(first) != string(second) {
		t.Fatalf("second update changed bytes:\n%s\n---\n%s", first, second)
	}
}

func TestUpdateCoverageManifestRejectsInvalidSampleSetWithoutMutation(t *testing.T) {
	alpha := modulePath + "/pkg/config"
	beta := modulePath + "/pkg/service"
	manifest := coverageManifest{Version: coverageManifestVersion, Lane: "functional", Packages: []coverageManifestEntry{
		{Package: alpha, Minimum: json.RawMessage("50.00")},
		{Package: beta, Minimum: json.RawMessage("50.00")},
	}}
	data, err := renderCoverageManifest(manifest)
	if err != nil {
		t.Fatalf("render manifest: %v", err)
	}
	filename := filepath.Join(t.TempDir(), "minimums.json")
	if err := os.WriteFile(filename, data, 0o644); err != nil {
		t.Fatalf("write manifest fixture: %v", err)
	}
	tooFew := coverageManifestSampleSet([]map[string]packageCoverageTotals{
		{alpha: {coveredStatements: 1, totalStatements: 2}, beta: {coveredStatements: 1, totalStatements: 2}},
		{alpha: {coveredStatements: 1, totalStatements: 2}, beta: {coveredStatements: 1, totalStatements: 2}},
		{alpha: {coveredStatements: 1, totalStatements: 2}, beta: {coveredStatements: 1, totalStatements: 2}},
		{alpha: {coveredStatements: 1, totalStatements: 2}, beta: {coveredStatements: 1, totalStatements: 2}},
	})
	if _, err := updateCoverageManifestFile(filename, "functional", tooFew); err == nil || !strings.Contains(err.Error(), "requires at least 5 profiles") {
		t.Fatalf("too-few update error = %v, want actionable sample-count failure", err)
	}
	after, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("read manifest after rejected update: %v", err)
	}
	if string(after) != string(data) {
		t.Fatalf("rejected update mutated manifest:\n%s\n---\n%s", data, after)
	}

	missingSample := coverageManifestSampleSet([]map[string]packageCoverageTotals{
		{alpha: {coveredStatements: 1, totalStatements: 2}},
		{alpha: {coveredStatements: 1, totalStatements: 2}},
		{alpha: {coveredStatements: 1, totalStatements: 2}},
		{alpha: {coveredStatements: 1, totalStatements: 2}},
		{alpha: {coveredStatements: 1, totalStatements: 2}},
	})
	if _, err := updateCoverageManifestFile(filename, "functional", missingSample); err == nil || !strings.Contains(err.Error(), "numeric package \""+beta+"\" is absent") {
		t.Fatalf("incompatible update error = %v, want missing-package failure", err)
	}
	after, err = os.ReadFile(filename)
	if err != nil {
		t.Fatalf("read manifest after incompatible update: %v", err)
	}
	if string(after) != string(data) {
		t.Fatalf("incompatible update mutated manifest:\n%s\n---\n%s", data, after)
	}
}

func TestExecuteSampledManifestUpdateRejectsDuplicateUnreadableProfilesWithoutMutation(t *testing.T) {
	alpha := modulePath + "/pkg/config"
	root := t.TempDir()
	paths := make([]string, 0, minimumVarianceSamples)
	for index := 0; index < minimumVarianceSamples; index++ {
		path := filepath.Join(root, "run-"+string(rune('1'+index))+".out")
		writeVarianceProfile(t, path, map[string]variancePackageFixture{alpha: {total: 2, covered: 1}})
		paths = append(paths, path)
	}
	manifestPath := filepath.Join(root, "minimums.json")
	manifest := []byte("{\n  \"version\": 1,\n  \"lane\": \"functional\",\n  \"packages\": [{\n    \"package\": \"" + alpha + "\",\n    \"minimum\": 50.00\n  }]\n}\n")
	if err := os.WriteFile(manifestPath, manifest, 0o600); err != nil {
		t.Fatalf("write manifest fixture: %v", err)
	}
	before := string(manifest)

	duplicate := slices.Clone(paths)
	duplicate[len(duplicate)-1] = duplicate[0]
	if err := execute(config{suite: "functional", updateManifest: manifestPath, updateProfiles: strings.Join(duplicate, ",")}); err == nil || !strings.Contains(err.Error(), "duplicate profile input") {
		t.Fatalf("duplicate profile error = %v, want actionable duplicate diagnostic", err)
	}
	after, _ := os.ReadFile(manifestPath)
	if string(after) != before {
		t.Fatalf("duplicate rejection mutated manifest:\n%s\n---\n%s", before, after)
	}

	missing := slices.Clone(paths)
	missing[len(missing)-1] = filepath.Join(root, "missing.out")
	if err := execute(config{suite: "functional", updateManifest: manifestPath, updateProfiles: strings.Join(missing, ",")}); err == nil || !strings.Contains(err.Error(), "is unreadable") {
		t.Fatalf("unreadable profile error = %v, want actionable unreadable diagnostic", err)
	}
	after, _ = os.ReadFile(manifestPath)
	if string(after) != before {
		t.Fatalf("unreadable rejection mutated manifest:\n%s\n---\n%s", before, after)
	}
}

func coverageManifestSampleSet(totals []map[string]packageCoverageTotals) []coverageVarianceSample {
	samples := make([]coverageVarianceSample, len(totals))
	for index, sampleTotals := range totals {
		samples[index] = coverageVarianceSample{
			path:   filepath.Join("sample-root", "run-"+string(rune('1'+index))+".out"),
			label:  "run-" + string(rune('1'+index)),
			header: countCoverageProfileHeader,
			totals: sampleTotals,
		}
	}
	return samples
}

func coverageManifestUpdateStrings(updates []coverageManifestUpdate) []string {
	values := make([]string, 0, len(updates))
	for _, update := range updates {
		values = append(values, update.String())
	}
	return values
}
