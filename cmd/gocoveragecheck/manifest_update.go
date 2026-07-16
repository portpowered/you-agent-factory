package main

import (
	"bytes"
	"encoding/json"
	"errors"
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

func updateCoverageManifestFile(filename string, lane string, totals map[string]packageCoverageTotals, measuredPackages []string) ([]coverageManifestUpdate, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("read %s go coverage manifest for update: %w", lane, err)
	}
	manifest, err := readCoverageManifestForUpdate(data, lane, measuredPackages)
	if err != nil {
		return nil, err
	}
	updated, updates, err := planCoverageManifestUpdate(manifest, totals, measuredPackages)
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

func planCoverageManifestUpdate(manifest coverageManifest, totals map[string]packageCoverageTotals, measuredPackages []string) (coverageManifest, []coverageManifestUpdate, error) {
	existing := make(map[string]coverageManifestEntry, len(manifest.Packages))
	for _, entry := range manifest.Packages {
		existing[entry.Package] = entry
	}
	packages := slices.Clone(measuredPackages)
	slices.Sort(packages)
	updated := coverageManifest{Version: manifest.Version, Lane: manifest.Lane, Packages: make([]coverageManifestEntry, 0, len(packages))}
	updates := make([]coverageManifestUpdate, 0, len(packages))
	rejected := false
	for _, importPath := range packages {
		entry, found := existing[importPath]
		if !found {
			var err error
			entry, err = newCoverageManifestEntry(manifest.Lane, importPath, totals[importPath])
			if err != nil {
				return manifest, updates, fmt.Errorf("update %s coverage manifest for new package %s: %w", manifest.Lane, importPath, err)
			}
			updated.Packages = append(updated.Packages, entry)
			updates = append(updates, coverageManifestUpdate{Package: importPath, Lane: manifest.Lane, Status: "added", Old: "missing", Candidate: coverageManifestEntryValue(entry)})
			continue
		}
		updated.Packages = append(updated.Packages, entry)
		if entry.Exception != nil {
			updates = append(updates, coverageManifestUpdate{Package: importPath, Lane: manifest.Lane, Status: "unchanged", Old: "exception", Candidate: coverageCandidateValue(totals[importPath])})
			continue
		}
		oldFloor, _ := parseCoverageFloor(entry.Minimum)
		candidate, floorErr := coverageFloorFromTotals(totals[importPath])
		if floorErr != nil || candidate < oldFloor {
			candidateValue := "unmeasurable"
			if floorErr == nil {
				candidateValue = candidate.String() + "%"
			}
			updates = append(updates, coverageManifestUpdate{Package: importPath, Lane: manifest.Lane, Status: "rejected", Old: oldFloor.String() + "%", Candidate: candidateValue})
			rejected = true
			continue
		}
		status := "unchanged"
		if candidate > oldFloor {
			status = "raised"
			updated.Packages[len(updated.Packages)-1].Minimum = json.RawMessage(candidate.String())
		}
		updates = append(updates, coverageManifestUpdate{Package: importPath, Lane: manifest.Lane, Status: status, Old: oldFloor.String() + "%", Candidate: candidate.String() + "%"})
	}
	if rejected {
		return manifest, updates, errors.New("update go coverage manifest: rejected one or more floor decreases; restore coverage before ratcheting the manifest")
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
