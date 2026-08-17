package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
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
	Version int    `json:"version"`
	Lane    string `json:"lane"`
	// DefaultFloorPercent is the floor applied to a measured package with no
	// entry in Packages. It is optional; when absent the lane's built-in
	// default applies. Explicit Packages entries always win over it.
	DefaultFloorPercent json.RawMessage         `json:"defaultFloorPercent,omitempty"`
	Packages            []coverageManifestEntry `json:"packages"`
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

type coverageManifestValidationError struct {
	err error
}

func (err *coverageManifestValidationError) Error() string {
	return err.err.Error()
}

func (err *coverageManifestValidationError) Unwrap() error {
	return err.err
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
		entry, required, err := newCoverageManifestEntry(lane, importPath, totals[importPath])
		if err != nil {
			return coverageManifest{}, fmt.Errorf("generate %s coverage manifest for %s: %w", lane, importPath, err)
		}
		if !required {
			continue
		}
		entries = append(entries, entry)
	}
	return coverageManifest{
		Version:             coverageManifestVersion,
		Lane:                lane,
		DefaultFloorPercent: laneDefaultCoverageFloorRaw(lane),
		Packages:            entries,
	}, nil
}

// newCoverageManifestEntry builds the manifest row for importPath and reports
// whether the package requires a row at all. A package whose profile reports no
// measurable statements passes vacuously and needs no row, so none is minted
// for it. A service root always declares a row, because per-service
// completeness is satisfied by that root entry.
func newCoverageManifestEntry(lane string, importPath string, totals packageCoverageTotals) (coverageManifestEntry, bool, error) {
	floor, err := coverageFloorFromTotals(totals)
	if err == nil {
		return coverageManifestEntry{Package: importPath, Minimum: json.RawMessage(floor.String())}, true, nil
	}
	if totals.totalStatements != 0 || totals.coveredStatements != 0 {
		return coverageManifestEntry{}, false, err
	}
	if isCoverageManifestServiceRoot(importPath) {
		return coverageManifestServiceRootEntry(importPath), true, nil
	}
	return coverageManifestEntry{}, false, nil
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
	return readCoverageManifestAt(data, expectedLane, measuredPackages, time.Now().UTC())
}

func readCoverageManifestWithTotals(data []byte, expectedLane string, measuredPackages []string, measuredTotals map[string]packageCoverageTotals) (coverageManifest, error) {
	return readCoverageManifestAtModeWithTotals(data, expectedLane, measuredPackages, time.Now().UTC(), true, measuredTotals)
}

func readCoverageManifestAt(data []byte, expectedLane string, measuredPackages []string, now time.Time) (coverageManifest, error) {
	return readCoverageManifestAtMode(data, expectedLane, measuredPackages, now, true)
}

func readCoverageManifestForUpdate(data []byte, expectedLane string, measuredPackages []string) (coverageManifest, error) {
	return readCoverageManifestAtMode(data, expectedLane, measuredPackages, time.Now().UTC(), false)
}

func readCoverageManifestAtMode(data []byte, expectedLane string, measuredPackages []string, now time.Time, requireComplete bool) (coverageManifest, error) {
	return readCoverageManifestAtModeWithTotals(data, expectedLane, measuredPackages, now, requireComplete, nil)
}

func readCoverageManifestAtModeWithTotals(data []byte, expectedLane string, measuredPackages []string, now time.Time, requireComplete bool, measuredTotals map[string]packageCoverageTotals) (coverageManifest, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var manifest coverageManifest
	if err := decoder.Decode(&manifest); err != nil {
		return coverageManifest{}, fmt.Errorf("parse go coverage manifest: %w", err)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return coverageManifest{}, err
	}
	if err := validateCoverageManifestAtModeWithTotals(manifest, expectedLane, measuredPackages, now, requireComplete, measuredTotals); err != nil {
		return coverageManifest{}, &coverageManifestValidationError{err: err}
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
	return validateCoverageManifestAt(manifest, expectedLane, measuredPackages, time.Now().UTC())
}

func validateCoverageManifestAt(manifest coverageManifest, expectedLane string, measuredPackages []string, now time.Time) error {
	return validateCoverageManifestAtMode(manifest, expectedLane, measuredPackages, now, true)
}

func validateCoverageManifestAtWithTotals(manifest coverageManifest, expectedLane string, measuredPackages []string, now time.Time, measuredTotals map[string]packageCoverageTotals) error {
	return validateCoverageManifestAtModeWithTotals(manifest, expectedLane, measuredPackages, now, true, measuredTotals)
}

func validateCoverageManifestAtMode(manifest coverageManifest, expectedLane string, measuredPackages []string, now time.Time, requireComplete bool) error {
	return validateCoverageManifestAtModeWithTotals(manifest, expectedLane, measuredPackages, now, requireComplete, nil)
}

func validateCoverageManifestAtModeWithTotals(manifest coverageManifest, expectedLane string, measuredPackages []string, now time.Time, requireComplete bool, measuredTotals map[string]packageCoverageTotals) error {
	if manifest.Version != coverageManifestVersion {
		return fmt.Errorf("validate go coverage manifest: version %d is unsupported; expected %d", manifest.Version, coverageManifestVersion)
	}
	if err := validateCoverageLane(manifest.Lane); err != nil {
		return err
	}
	if manifest.Lane != expectedLane {
		return fmt.Errorf("validate go coverage manifest: lane %q does not match active lane %q", manifest.Lane, expectedLane)
	}
	if err := validateCoverageManifestDefaultFloor(manifest); err != nil {
		return err
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
		if entry.Exception != nil {
			deadline, _ := time.Parse("2006-01-02", entry.Exception.Deadline)
			if deadline.Before(dateOnlyUTC(now)) {
				return fmt.Errorf("validate go coverage manifest: expired exception for package %q: deadline %s passed; satisfy removal gate: %s", entry.Package, entry.Exception.Deadline, entry.Exception.RemovalGate)
			}
		}
		seen[entry.Package] = struct{}{}
		previous = entry.Package
	}
	// A measured package with no entry is not a completeness gap: it resolves to
	// the lane default floor. Completeness only requires that every measured
	// service declares a floor at its root.
	if requireComplete {
		return validateCoverageManifestServiceRoots(expectedLane, measuredPackages, seen, measuredTotals)
	}
	return nil
}

func dateOnlyUTC(value time.Time) time.Time {
	year, month, day := value.UTC().Date()
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

func readCoverageManifestFile(filename string, lane string, measuredPackages []string) (coverageManifest, error) {
	return readCoverageManifestFileWithTotals(filename, lane, measuredPackages, nil)
}

func readCoverageManifestFileWithTotals(filename string, lane string, measuredPackages []string, measuredTotals map[string]packageCoverageTotals) (coverageManifest, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return coverageManifest{}, fmt.Errorf("read %s go coverage manifest: %w", lane, err)
	}
	var manifest coverageManifest
	if measuredTotals == nil {
		manifest, err = readCoverageManifest(data, lane, measuredPackages)
	} else {
		manifest, err = readCoverageManifestWithTotals(data, lane, measuredPackages, measuredTotals)
	}
	if err != nil {
		return coverageManifest{}, err
	}
	return manifest, nil
}

func checkCoverageManifest(manifest coverageManifest, totals map[string]packageCoverageTotals, manifestPath string) []string {
	failures, _ := checkCoverageManifestWithEpsilon(manifest, totals, manifestPath, 0)
	return failures
}

func formatUnmeasuredCoverageManifestDiagnostics(manifest coverageManifest, measuredTotals map[string]packageCoverageTotals) []string {
	diagnostics := make([]string, 0)
	for _, entry := range manifest.Packages {
		if _, measured := measuredTotals[entry.Package]; measured {
			continue
		}
		diagnostics = append(diagnostics, fmt.Sprintf(
			"coverage not evaluated: package=%s lane=%s (no measurement in profile)",
			entry.Package,
			manifest.Lane,
		))
	}
	return diagnostics
}

func checkCoverageManifestWithEpsilon(manifest coverageManifest, totals map[string]packageCoverageTotals, manifestPath string, epsilon float64) ([]string, []string) {
	return checkCoverageManifestWithEpsilonAndBlocks(manifest, totals, manifestPath, epsilon, nil)
}

func checkCoverageManifestWithEpsilonAndBlocks(manifest coverageManifest, totals map[string]packageCoverageTotals, manifestPath string, epsilon float64, coverageBlocks map[string]coverageBlock) ([]string, []string) {
	failures := make([]string, 0)
	warnings := make([]string, 0)
	for _, entry := range manifest.Packages {
		if entry.Exception != nil {
			continue
		}
		minimum, _ := parseCoverageFloor(entry.Minimum)
		actual := totals[entry.Package]
		// A package with no measurable statements passes vacuously: the floor
		// has no denominator to be evaluated against.
		if coveragePassesFloor(minimum, actual) {
			continue
		}
		actualPercent := 0.0
		if actual.totalStatements > 0 {
			actualPercent = float64(actual.coveredStatements) * 100 / float64(actual.totalStatements)
		}
		expectedPercent := float64(minimum) / 100
		if coverageFloorDriftWithinEpsilon(minimum, actual, epsilon) {
			warnings = append(warnings, fmt.Sprintf(
				"package coverage warning: tolerated drift: package=%s lane=%s expected-minimum=%s%% actual=%.4f%% delta=%+.4f percentage-points epsilon=%.4f percentage-points",
				entry.Package, manifest.Lane, minimum.String(), actualPercent, actualPercent-expectedPercent, epsilon,
			))
			continue
		}
		diagnostic := fmt.Sprintf(
			"package coverage regression: package=%s lane=%s expected-minimum=%s%% actual=%.4f%% delta=%+.4f percentage-points covered=%d/%d statements",
			entry.Package,
			manifest.Lane,
			minimum.String(),
			actualPercent,
			actualPercent-expectedPercent,
			actual.coveredStatements,
			actual.totalStatements,
		)
		if uncovered := formatUncoveredCoverageBlocks(coverageBlocks, entry.Package); uncovered != "" {
			diagnostic += "; " + uncovered
		}
		failures = append(failures, diagnostic+fmt.Sprintf(
			"; restore coverage before running `go run ./cmd/gocoveragecheck -suite %s -profile <coverage-profile> -update-manifest %s`",
			manifest.Lane, manifestPath,
		))
	}
	failures = append(failures, checkCoverageDefaultFloor(manifest, totals, manifestPath, coverageBlocks)...)
	return failures, warnings
}

func coverageFloorDriftWithinEpsilon(minimum coverageFloor, actual packageCoverageTotals, epsilon float64) bool {
	if actual.totalStatements <= 0 || epsilon <= 0 {
		return false
	}

	expected := new(big.Rat).SetFrac64(int64(minimum), 100)
	measured := new(big.Rat).SetFrac64(int64(actual.coveredStatements)*100, int64(actual.totalStatements))
	drift := new(big.Rat).Sub(expected, measured)
	epsilonRatio, ok := new(big.Rat).SetString(strconv.FormatFloat(epsilon, 'f', -1, 64))
	if !ok {
		return false
	}
	return drift.Cmp(epsilonRatio) <= 0
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
