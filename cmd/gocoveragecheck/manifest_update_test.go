package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestPlanCoverageManifestUpdateAddsRaisesAndLeavesSubPrecisionGainUnchanged(t *testing.T) {
	t.Parallel()

	alpha := modulePath + "/pkg/config"
	beta := modulePath + "/pkg/factory"
	gamma := modulePath + "/pkg/service"
	for _, lane := range []string{"unit", "functional"} {
		lane := lane
		t.Run(lane, func(t *testing.T) {
			t.Parallel()
			manifest := coverageManifest{
				Version: coverageManifestVersion,
				Lane:    lane,
				Packages: []coverageManifestEntry{
					{Package: alpha, Minimum: json.RawMessage("50.00")},
					{Package: beta, Minimum: json.RawMessage("66.66")},
				},
			}
			totals := map[string]packageCoverageTotals{
				alpha: {coveredStatements: 3, totalStatements: 5},
				beta:  {coveredStatements: 2, totalStatements: 3},
				gamma: {coveredStatements: 1, totalStatements: 4},
			}

			updated, updates, err := planCoverageManifestUpdate(manifest, totals, []string{gamma, beta, alpha})
			if err != nil {
				t.Fatalf("planCoverageManifestUpdate() error = %v", err)
			}
			wantUpdates := []string{
				"package coverage update: package=" + alpha + " lane=" + lane + " status=raised old=50.00% candidate=60.00%",
				"package coverage update: package=" + beta + " lane=" + lane + " status=unchanged old=66.66% candidate=66.66%",
				"package coverage update: package=" + gamma + " lane=" + lane + " status=added old=missing candidate=25.00%",
			}
			if got := coverageManifestUpdateStrings(updates); !slices.Equal(got, wantUpdates) {
				t.Fatalf("updates = %v, want %v", got, wantUpdates)
			}
			if got := string(updated.Packages[0].Minimum); got != "60.00" {
				t.Fatalf("raised minimum = %s, want 60.00", got)
			}
			if got := string(updated.Packages[1].Minimum); got != "66.66" {
				t.Fatalf("sub-precision minimum = %s, want preserved 66.66", got)
			}

			idempotent, secondUpdates, err := planCoverageManifestUpdate(updated, totals, []string{alpha, beta, gamma})
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
		})
	}
}

func TestUpdateCoverageManifestFileRejectsDecreaseWithoutAnyMutation(t *testing.T) {
	t.Parallel()

	alpha := modulePath + "/pkg/config"
	beta := modulePath + "/pkg/service"
	manifest := coverageManifest{
		Version: coverageManifestVersion,
		Lane:    "unit",
		Packages: []coverageManifestEntry{
			{Package: alpha, Minimum: json.RawMessage("80.00")},
			{Package: beta, Minimum: json.RawMessage("50.00")},
		},
	}
	data, err := renderCoverageManifest(manifest)
	if err != nil {
		t.Fatalf("render manifest: %v", err)
	}
	filename := filepath.Join(t.TempDir(), "minimums.json")
	if err := os.WriteFile(filename, data, 0o644); err != nil {
		t.Fatalf("write manifest fixture: %v", err)
	}

	updates, err := updateCoverageManifestFile(filename, "unit", map[string]packageCoverageTotals{
		alpha: {coveredStatements: 79, totalStatements: 100},
		beta:  {coveredStatements: 3, totalStatements: 5},
	}, []string{beta, alpha})
	if err == nil || !strings.Contains(err.Error(), "rejected one or more floor decreases") {
		t.Fatalf("updateCoverageManifestFile() error = %v, want rejected decrease", err)
	}
	wantUpdates := []string{
		"package coverage update: package=" + alpha + " lane=unit status=rejected old=80.00% candidate=79.00%",
		"package coverage update: package=" + beta + " lane=unit status=raised old=50.00% candidate=60.00%",
	}
	if got := coverageManifestUpdateStrings(updates); !slices.Equal(got, wantUpdates) {
		t.Fatalf("updates = %v, want %v", got, wantUpdates)
	}
	after, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("read manifest after rejection: %v", err)
	}
	if string(after) != string(data) {
		t.Fatalf("rejected update mutated manifest:\n%s\n---\n%s", data, after)
	}
}

func TestUpdateCoverageManifestFileWritesOnceThenIsByteIdempotent(t *testing.T) {
	t.Parallel()

	alpha := modulePath + "/pkg/config"
	beta := modulePath + "/pkg/service"
	manifest := coverageManifest{Version: coverageManifestVersion, Lane: "unit", Packages: []coverageManifestEntry{
		{Package: alpha, Minimum: json.RawMessage("50.00")},
	}}
	data, _ := renderCoverageManifest(manifest)
	filename := filepath.Join(t.TempDir(), "minimums.json")
	if err := os.WriteFile(filename, data, 0o644); err != nil {
		t.Fatalf("write manifest fixture: %v", err)
	}
	totals := map[string]packageCoverageTotals{
		alpha: {coveredStatements: 3, totalStatements: 5},
		beta:  {coveredStatements: 1, totalStatements: 4},
	}
	packages := []string{alpha, beta}
	if _, err := updateCoverageManifestFile(filename, "unit", totals, packages); err != nil {
		t.Fatalf("first update error = %v", err)
	}
	first, _ := os.ReadFile(filename)
	if _, err := updateCoverageManifestFile(filename, "unit", totals, packages); err != nil {
		t.Fatalf("second update error = %v", err)
	}
	second, _ := os.ReadFile(filename)
	if string(first) != string(second) {
		t.Fatalf("second update changed bytes:\n%s\n---\n%s", first, second)
	}
}

func coverageManifestUpdateStrings(updates []coverageManifestUpdate) []string {
	values := make([]string, 0, len(updates))
	for _, update := range updates {
		values = append(values, update.String())
	}
	return values
}
