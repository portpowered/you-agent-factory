package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"slices"
)

type coverageManifestUpdate struct {
	Package   string
	Lane      string
	Status    string
	Old       string
	Candidate string
}

func (update coverageManifestUpdate) String() string {
	return fmt.Sprintf(
		"package coverage update: package=%s lane=%s status=%s old=%s candidate=%s",
		update.Package, update.Lane, update.Status, update.Old, update.Candidate,
	)
}

func executeSampledManifestUpdate(cfg config) error {
	repoRoot, err := repoRootDir()
	if err != nil {
		return err
	}
	samples, err := loadCoverageVarianceProfiles(splitList(cfg.updateProfiles, ",", true), repoRoot)
	if err != nil {
		return fmt.Errorf("update %s coverage manifest from sample set: %w", cfg.suite, err)
	}
	updates, err := updateCoverageManifestFile(cfg.updateManifest, cfg.suite, samples)
	if err != nil {
		return err
	}
	for _, update := range updates {
		fmt.Fprintln(stdoutWriter, update.String())
	}
	fmt.Fprintf(stdoutWriter, "Updated %s coverage manifest from %d complete profiles at %s.\n", cfg.suite, len(samples), cfg.updateManifest)
	return nil
}

func updateCoverageManifestFile(filename string, lane string, samples []coverageVarianceSample) ([]coverageManifestUpdate, error) {
	minimums, err := minimumCoverageTotals(samples)
	if err != nil {
		return nil, fmt.Errorf("update %s coverage manifest: %w", lane, err)
	}
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("read %s go coverage manifest for update: %w", lane, err)
	}
	manifest, err := readCoverageManifestForSampleUpdate(data, lane, sortedCoveragePackageNames(minimums))
	if err != nil {
		return nil, err
	}
	updated, updates, err := planCoverageManifestUpdate(manifest, minimums)
	if err != nil {
		return updates, err
	}
	updatedData, err := renderCoverageManifest(updated)
	if err != nil {
		return updates, err
	}
	if bytes.Equal(data, updatedData) {
		return updates, nil
	}
	info, err := os.Stat(filename)
	if err != nil {
		return updates, fmt.Errorf("stat go coverage manifest for update: %w", err)
	}
	if err := os.WriteFile(filename, updatedData, info.Mode().Perm()); err != nil {
		return updates, fmt.Errorf("write go coverage manifest update: %w", err)
	}
	return updates, nil
}

func readCoverageManifestForSampleUpdate(data []byte, lane string, sampledPackages []string) (coverageManifest, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var unvalidated coverageManifest
	if err := decoder.Decode(&unvalidated); err != nil {
		return coverageManifest{}, fmt.Errorf("parse go coverage manifest: %w", err)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return coverageManifest{}, err
	}

	allPackages := make(map[string]struct{}, len(sampledPackages)+len(unvalidated.Packages))
	for _, importPath := range sampledPackages {
		allPackages[importPath] = struct{}{}
	}
	for _, entry := range unvalidated.Packages {
		allPackages[entry.Package] = struct{}{}
	}
	measuredPackages := make([]string, 0, len(allPackages))
	for importPath := range allPackages {
		measuredPackages = append(measuredPackages, importPath)
	}
	slices.Sort(measuredPackages)
	manifest, err := readCoverageManifestForUpdate(data, lane, measuredPackages)
	if err != nil {
		return coverageManifest{}, err
	}
	sampled := make(map[string]struct{}, len(sampledPackages))
	for _, importPath := range sampledPackages {
		sampled[importPath] = struct{}{}
	}
	for _, entry := range manifest.Packages {
		if entry.Exception != nil {
			continue
		}
		if _, ok := sampled[entry.Package]; !ok {
			return coverageManifest{}, fmt.Errorf(
				"update %s coverage manifest: numeric package %q is absent from the compatible sample package set",
				lane, entry.Package,
			)
		}
	}
	return manifest, nil
}

func planCoverageManifestUpdate(manifest coverageManifest, minimums map[string]packageCoverageTotals) (coverageManifest, []coverageManifestUpdate, error) {
	existing := make(map[string]coverageManifestEntry, len(manifest.Packages))
	allPackages := make(map[string]struct{}, len(manifest.Packages)+len(minimums))
	for _, entry := range manifest.Packages {
		existing[entry.Package] = entry
		allPackages[entry.Package] = struct{}{}
	}
	for importPath := range minimums {
		allPackages[importPath] = struct{}{}
	}
	packages := make([]string, 0, len(allPackages))
	for importPath := range allPackages {
		packages = append(packages, importPath)
	}
	slices.Sort(packages)

	updated := coverageManifest{
		Version:             manifest.Version,
		Lane:                manifest.Lane,
		DefaultFloorPercent: manifest.DefaultFloorPercent,
		FloorHolds:          slices.Clone(manifest.FloorHolds),
		Packages:            make([]coverageManifestEntry, 0, len(packages)),
	}
	updates := make([]coverageManifestUpdate, 0, len(packages))
	for _, importPath := range packages {
		if !coverageRequirementApplies(manifest.Lane, importPath) {
			continue
		}
		entry, found := existing[importPath]
		if !found {
			totals, ok := minimums[importPath]
			if !ok {
				return manifest, updates, fmt.Errorf("update %s coverage manifest: package %q has no compatible sample measurement", manifest.Lane, importPath)
			}
			entry, required, err := newCoverageManifestEntry(manifest.Lane, importPath, totals)
			if err != nil {
				return manifest, updates, fmt.Errorf("update %s coverage manifest for new package %s: %w", manifest.Lane, importPath, err)
			}
			if !required {
				continue
			}
			updated.Packages = append(updated.Packages, entry)
			updates = append(updates, coverageManifestUpdate{
				Package: importPath, Lane: manifest.Lane, Status: "added", Old: "missing", Candidate: coverageManifestEntryValue(entry),
			})
			continue
		}

		updated.Packages = append(updated.Packages, entry)
		if entry.Exception != nil {
			candidate := "unmeasurable"
			if totals, ok := minimums[importPath]; ok {
				candidate = coverageCandidateValue(totals)
			}
			updates = append(updates, coverageManifestUpdate{
				Package: importPath, Lane: manifest.Lane, Status: "unchanged", Old: "exception", Candidate: candidate,
			})
			continue
		}

		totals, ok := minimums[importPath]
		if !ok {
			return manifest, updates, fmt.Errorf("update %s coverage manifest: numeric package %q has no compatible sample measurement", manifest.Lane, importPath)
		}
		oldFloor, err := parseCoverageFloor(entry.Minimum)
		if err != nil {
			return manifest, updates, fmt.Errorf("update %s coverage manifest for package %s: %w", manifest.Lane, importPath, err)
		}
		candidate, err := coverageFloorFromTotals(totals)
		if err != nil {
			return manifest, updates, fmt.Errorf("update %s coverage manifest for package %s: %w", manifest.Lane, importPath, err)
		}
		status := "unchanged"
		switch {
		case candidate < oldFloor:
			status = "lowered"
		case candidate > oldFloor:
			status = "raised"
		}
		if status != "unchanged" {
			updated.Packages[len(updated.Packages)-1].Minimum = json.RawMessage(candidate.String())
		}
		updates = append(updates, coverageManifestUpdate{
			Package: importPath, Lane: manifest.Lane, Status: status, Old: oldFloor.String() + "%", Candidate: candidate.String() + "%",
		})
	}
	return updated, updates, nil
}

func coverageCandidateValue(totals packageCoverageTotals) string {
	floor, err := coverageFloorFromTotals(totals)
	if err != nil {
		return "unmeasurable"
	}
	return floor.String() + "%"
}

func coverageManifestEntryValue(entry coverageManifestEntry) string {
	if entry.Exception != nil {
		return "exception"
	}
	floor, _ := parseCoverageFloor(entry.Minimum)
	return floor.String() + "%"
}
