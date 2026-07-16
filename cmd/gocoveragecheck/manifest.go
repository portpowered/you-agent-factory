package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"
)

const coverageManifestVersion = 1

const unmeasurablePackageDeadline = "2027-07-15"

type coverageFloor int64

type coverageManifest struct {
	Version  int                     `json:"version"`
	Lane     string                  `json:"lane"`
	Packages []coverageManifestEntry `json:"packages"`
}

type coverageManifestEntry struct {
	Package   string                     `json:"package"`
	Minimum   json.RawMessage            `json:"minimum,omitempty"`
	Exception *coverageManifestException `json:"exception,omitempty"`
}

type coverageManifestException struct {
	Kind          string `json:"kind"`
	Justification string `json:"justification"`
	Owner         string `json:"owner"`
	Deadline      string `json:"deadline"`
	RemovalGate   string `json:"removalGate"`
}

func createCoverageManifest(filename string, lane string, totals map[string]packageCoverageTotals, packages []string) error {
	manifest, err := newCoverageManifest(lane, totals, packages)
	if err != nil {
		return err
	}
	data, err := renderCoverageManifest(manifest)
	if err != nil {
		return err
	}
	file, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return fmt.Errorf("create go coverage manifest: %w", err)
	}
	written := false
	defer func() {
		_ = file.Close()
		if !written {
			_ = os.Remove(filename)
		}
	}()
	if _, err := file.Write(data); err != nil {
		return fmt.Errorf("write go coverage manifest: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close go coverage manifest: %w", err)
	}
	written = true
	return nil
}

func newCoverageManifest(lane string, totals map[string]packageCoverageTotals, packages []string) (coverageManifest, error) {
	if err := validateCoverageLane(lane); err != nil {
		return coverageManifest{}, err
	}
	packages = append([]string(nil), packages...)
	slices.Sort(packages)
	entries := make([]coverageManifestEntry, 0, len(packages))
	for _, importPath := range packages {
		floor, err := coverageFloorFromTotals(totals[importPath])
		if err != nil {
			if totals[importPath].totalStatements == 0 {
				entries = append(entries, coverageManifestEntry{
					Package: importPath,
					Exception: &coverageManifestException{
						Kind:          "measurement",
						Justification: "The Go coverage profile contains no measurable statements for this declaration-only package.",
						Owner:         "backend-quality",
						Deadline:      unmeasurablePackageDeadline,
						RemovalGate:   fmt.Sprintf("The %s coverage profile reports at least one measurable statement for this package.", lane),
					},
				})
				continue
			}
			return coverageManifest{}, fmt.Errorf("generate %s coverage manifest for %s: %w", lane, importPath, err)
		}
		entries = append(entries, coverageManifestEntry{
			Package: importPath,
			Minimum: json.RawMessage(floor.String()),
		})
	}
	return coverageManifest{Version: coverageManifestVersion, Lane: lane, Packages: entries}, nil
}

func coverageFloorFromTotals(totals packageCoverageTotals) (coverageFloor, error) {
	if totals.totalStatements <= 0 {
		return 0, errors.New("cannot calculate a floor without measured statements")
	}
	if totals.coveredStatements < 0 || totals.coveredStatements > totals.totalStatements {
		return 0, fmt.Errorf("invalid statement counts: covered=%d total=%d", totals.coveredStatements, totals.totalStatements)
	}
	return coverageFloor(int64(totals.coveredStatements) * 10000 / int64(totals.totalStatements)), nil
}

func (floor coverageFloor) String() string {
	return fmt.Sprintf("%d.%02d", floor/100, floor%100)
}

func renderCoverageManifest(manifest coverageManifest) ([]byte, error) {
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("render go coverage manifest: %w", err)
	}
	return append(data, '\n'), nil
}

func readCoverageManifest(data []byte, expectedLane string, measuredPackages []string) (coverageManifest, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var manifest coverageManifest
	if err := decoder.Decode(&manifest); err != nil {
		return coverageManifest{}, fmt.Errorf("parse go coverage manifest: %w", err)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return coverageManifest{}, err
	}
	if err := validateCoverageManifest(manifest, expectedLane, measuredPackages); err != nil {
		return coverageManifest{}, err
	}
	return manifest, nil
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err == io.EOF {
		return nil
	} else if err != nil {
		return fmt.Errorf("parse go coverage manifest trailing data: %w", err)
	}
	return errors.New("parse go coverage manifest: multiple JSON values")
}

func validateCoverageManifest(manifest coverageManifest, expectedLane string, measuredPackages []string) error {
	if manifest.Version != coverageManifestVersion {
		return fmt.Errorf("validate go coverage manifest: version %d is unsupported; expected %d", manifest.Version, coverageManifestVersion)
	}
	if err := validateCoverageLane(manifest.Lane); err != nil {
		return err
	}
	if manifest.Lane != expectedLane {
		return fmt.Errorf("validate go coverage manifest: lane %q does not match active lane %q", manifest.Lane, expectedLane)
	}
	measured := make(map[string]struct{}, len(measuredPackages))
	for _, importPath := range measuredPackages {
		measured[importPath] = struct{}{}
	}
	seen := make(map[string]struct{}, len(manifest.Packages))
	previous := ""
	for index, entry := range manifest.Packages {
		if entry.Package == "" {
			return fmt.Errorf("validate go coverage manifest: packages[%d].package is required", index)
		}
		if _, ok := seen[entry.Package]; ok {
			return fmt.Errorf("validate go coverage manifest: duplicate package %q", entry.Package)
		}
		if previous != "" && entry.Package < previous {
			return fmt.Errorf("validate go coverage manifest: packages must be sorted by import path; %q appears after %q", entry.Package, previous)
		}
		if _, ok := measured[entry.Package]; !ok {
			return fmt.Errorf("validate go coverage manifest: package %q is outside the %s measured package set", entry.Package, expectedLane)
		}
		if err := validateCoverageManifestEntry(entry); err != nil {
			return fmt.Errorf("validate go coverage manifest package %q: %w", entry.Package, err)
		}
		seen[entry.Package] = struct{}{}
		previous = entry.Package
	}
	for _, importPath := range measuredPackages {
		if _, ok := seen[importPath]; !ok {
			return fmt.Errorf("validate go coverage manifest: measured %s package %q has no manifest entry", expectedLane, importPath)
		}
	}
	return nil
}

func validateCoverageLane(lane string) error {
	if lane != "unit" && lane != "functional" {
		return fmt.Errorf("validate go coverage manifest: unknown lane %q; expected unit or functional", lane)
	}
	return nil
}

func validateCoverageManifestEntry(entry coverageManifestEntry) error {
	hasMinimum := len(entry.Minimum) > 0 && string(entry.Minimum) != "null"
	hasException := entry.Exception != nil
	if hasMinimum == hasException {
		return errors.New("exactly one of minimum or exception is required")
	}
	if hasMinimum {
		_, err := parseCoverageFloor(entry.Minimum)
		return err
	}
	return validateCoverageManifestException(*entry.Exception)
}

func parseCoverageFloor(raw json.RawMessage) (coverageFloor, error) {
	value := string(raw)
	parts := strings.Split(value, ".")
	if len(parts) != 2 || len(parts[1]) != 2 {
		return 0, fmt.Errorf("minimum %q must be a numeric percentage with exactly two decimal places", value)
	}
	whole, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, fmt.Errorf("minimum %q must be numeric", value)
	}
	fraction, err := strconv.Atoi(parts[1])
	if err != nil || whole < 0 || whole > 100 || fraction < 0 || fraction > 99 || (whole == 100 && fraction != 0) {
		return 0, fmt.Errorf("minimum %q must be between 0.00 and 100.00", value)
	}
	return coverageFloor(whole*100 + fraction), nil
}

func validateCoverageManifestException(exception coverageManifestException) error {
	if exception.Kind != "measurement" && exception.Kind != "migration" {
		return fmt.Errorf("exception kind %q must be measurement or migration", exception.Kind)
	}
	for name, value := range map[string]string{
		"justification": exception.Justification,
		"owner":         exception.Owner,
		"deadline":      exception.Deadline,
		"removalGate":   exception.RemovalGate,
	} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("exception %s is required", name)
		}
	}
	if _, err := time.Parse("2006-01-02", exception.Deadline); err != nil {
		return fmt.Errorf("exception deadline %q must use YYYY-MM-DD", exception.Deadline)
	}
	return nil
}

func packageImportPaths(summaries []packageCoverageSummary) []string {
	packages := make([]string, 0, len(summaries))
	for _, summary := range summaries {
		packages = append(packages, summary.importPath)
	}
	return packages
}
