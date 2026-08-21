package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// packageTargetSourceClass identifies the source class that makes an open
// package-target edge observable. The class is part of the finding identity so
// a test-only observation cannot become production liveness by being counted
// in the same bucket.
type packageTargetSourceClass string

const (
	packageTargetProductionSourceClass packageTargetSourceClass = "production"
	packageTargetTestOnlySourceClass   packageTargetSourceClass = "test-only"
)

const (
	packageTargetTestOnlyBaselineVersion = 1
	packageTargetTestOnlyBaselineStage   = "pss-package-target-test-only"
	packageTargetTestOnlyDeletionGate    = "delete the exact test-only package-target edge after its owning migration lands, then remove this baseline entry"
)

type packageTargetFinding struct {
	PackagePath string
	Destination string
	Successor   string
	Class       packageTargetSourceClass
}

type packageTargetTestOnlyBaseline struct {
	Version int                                  `json:"version"`
	Entries []packageTargetTestOnlyBaselineEntry `json:"entries"`
}

type packageTargetTestOnlyBaselineEntry struct {
	PackagePath  string `json:"packagePath"`
	Destination  string `json:"destination"`
	Successor    string `json:"successor"`
	Class        string `json:"class"`
	Stage        string `json:"stage"`
	DeletionGate string `json:"deletionGate"`
}

// scanPackageTargetFindings observes only the package paths named by the open
// move ledger. This keeps the ledger deletion-only and avoids reintroducing a
// row for every live package merely because test files are visible now.
func scanPackageTargetFindings(repoRoot string, moves []PackageMapping) ([]packageTargetFinding, error) {
	findings := make([]packageTargetFinding, 0, len(moves))
	for _, row := range moves {
		classes, err := packageTargetSourceClasses(repoRoot, row.PackagePath)
		if err != nil {
			return nil, err
		}
		for class := range classes {
			findings = append(findings, packageTargetFinding{
				PackagePath: row.PackagePath,
				Destination: row.Destination,
				Successor:   row.Successor,
				Class:       class,
			})
		}
	}
	slices.SortFunc(findings, comparePackageTargetFindings)
	return findings, nil
}

// packageTargetSourceClasses examines only direct files in one package
// directory. Nested directories are separate Go packages and are represented
// by their own open-move rows when migration intent exists for them.
func packageTargetSourceClasses(repoRoot, packagePath string) (map[packageTargetSourceClass]struct{}, error) {
	classes := map[packageTargetSourceClass]struct{}{}
	directory := filepath.Join(repoRoot, filepath.FromSlash(packagePath))
	entries, err := os.ReadDir(directory)
	if err != nil {
		if os.IsNotExist(err) {
			return classes, nil
		}
		return nil, fmt.Errorf("read package-target source directory %s: %w", packagePath, err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
			continue
		}
		if strings.HasSuffix(entry.Name(), "_test.go") {
			classes[packageTargetTestOnlySourceClass] = struct{}{}
			continue
		}
		classes[packageTargetProductionSourceClass] = struct{}{}
	}
	return classes, nil
}

func comparePackageTargetFindings(left, right packageTargetFinding) int {
	if comparison := strings.Compare(left.PackagePath, right.PackagePath); comparison != 0 {
		return comparison
	}
	if comparison := strings.Compare(left.Destination, right.Destination); comparison != 0 {
		return comparison
	}
	if comparison := strings.Compare(left.Successor, right.Successor); comparison != 0 {
		return comparison
	}
	return strings.Compare(string(left.Class), string(right.Class))
}

func packageTargetFindingKey(finding packageTargetFinding) string {
	return strings.Join([]string{
		filepath.ToSlash(finding.PackagePath),
		finding.Destination,
		finding.Successor,
		string(finding.Class),
	}, "\x00")
}

func packageTargetBaselineEntryKey(entry packageTargetTestOnlyBaselineEntry) string {
	return strings.Join([]string{
		filepath.ToSlash(entry.PackagePath),
		entry.Destination,
		entry.Successor,
		entry.Class,
	}, "\x00")
}

func loadPackageTargetTestOnlyBaseline(path string) (packageTargetTestOnlyBaseline, error) {
	payload, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return packageTargetTestOnlyBaseline{}, nil
		}
		return packageTargetTestOnlyBaseline{}, fmt.Errorf("read test-only package-target baseline %s: %w", path, err)
	}
	var baseline packageTargetTestOnlyBaseline
	if err := json.Unmarshal(payload, &baseline); err != nil {
		return packageTargetTestOnlyBaseline{}, fmt.Errorf("decode test-only package-target baseline %s: %w", path, err)
	}
	if baseline.Version != packageTargetTestOnlyBaselineVersion {
		return packageTargetTestOnlyBaseline{}, fmt.Errorf(
			"test-only package-target baseline version = %d, want %d",
			baseline.Version,
			packageTargetTestOnlyBaselineVersion,
		)
	}
	if len(baseline.Entries) == 0 {
		return packageTargetTestOnlyBaseline{}, fmt.Errorf(
			"test-only package-target baseline %s is empty; delete the file to record zero debt",
			path,
		)
	}
	return baseline, nil
}

func partitionPackageTargetFindings(
	findings []packageTargetFinding,
	moves []PackageMapping,
	baseline packageTargetTestOnlyBaseline,
) (productionStale []PackageMapping, testOnlyUnrecorded []packageTargetFinding, testOnlyStale []packageTargetTestOnlyBaselineEntry, err error) {
	baselineByKey := make(map[string]packageTargetTestOnlyBaselineEntry, len(baseline.Entries))
	for index, entry := range baseline.Entries {
		if validationErr := validatePackageTargetTestOnlyBaselineEntry(index, entry); validationErr != nil {
			return nil, nil, nil, validationErr
		}
		key := packageTargetBaselineEntryKey(entry)
		if _, duplicate := baselineByKey[key]; duplicate {
			return nil, nil, nil, fmt.Errorf(
				"test-only package-target baseline contains duplicate entry for %s -> %s",
				entry.PackagePath,
				entry.Destination,
			)
		}
		baselineByKey[key] = entry
	}
	if !slices.IsSortedFunc(baseline.Entries, func(left, right packageTargetTestOnlyBaselineEntry) int {
		return strings.Compare(packageTargetBaselineEntryKey(left), packageTargetBaselineEntryKey(right))
	}) {
		return nil, nil, nil, fmt.Errorf("test-only package-target baseline entries must be stable-sorted by exact identity")
	}

	seenTestOnly := make(map[string]struct{}, len(findings))
	productionRows := make(map[string]PackageMapping, len(findings))
	for _, finding := range findings {
		switch finding.Class {
		case packageTargetProductionSourceClass:
			productionRows[finding.PackagePath] = PackageMapping{
				PackagePath: finding.PackagePath,
				Destination: finding.Destination,
				Successor:   finding.Successor,
			}
		case packageTargetTestOnlySourceClass:
			key := packageTargetFindingKey(finding)
			seenTestOnly[key] = struct{}{}
			if _, recorded := baselineByKey[key]; !recorded {
				testOnlyUnrecorded = append(testOnlyUnrecorded, finding)
			}
		}
	}

	for key, entry := range baselineByKey {
		if _, observed := seenTestOnly[key]; !observed {
			testOnlyStale = append(testOnlyStale, entry)
		}
	}

	// A row without a production source is stale production migration intent.
	// Test-only evidence remains independently ratcheted above, but it cannot
	// make an open move row satisfy the production liveness requirement.
	for _, row := range moves {
		if _, production := productionRows[row.PackagePath]; production {
			continue
		}
		productionStale = append(productionStale, row)
	}

	slices.SortFunc(productionStale, func(left, right PackageMapping) int {
		return strings.Compare(packageTargetRowKey(left), packageTargetRowKey(right))
	})
	slices.SortFunc(testOnlyUnrecorded, comparePackageTargetFindings)
	slices.SortFunc(testOnlyStale, func(left, right packageTargetTestOnlyBaselineEntry) int {
		return strings.Compare(packageTargetBaselineEntryKey(left), packageTargetBaselineEntryKey(right))
	})
	return productionStale, testOnlyUnrecorded, testOnlyStale, nil
}

func packageTargetRowKey(row PackageMapping) string {
	return strings.Join([]string{row.PackagePath, row.Destination, row.Successor}, "\x00")
}

func packageTargetStaleRowPaths(rows []PackageMapping) []string {
	paths := make([]string, 0, len(rows))
	for _, row := range rows {
		paths = append(paths, row.PackagePath)
	}
	return paths
}

func validatePackageTargetTestOnlyBaselineEntry(index int, entry packageTargetTestOnlyBaselineEntry) error {
	prefix := fmt.Sprintf("test-only package-target baseline entries[%d]", index)
	if strings.TrimSpace(entry.PackagePath) == "" ||
		strings.TrimSpace(entry.Destination) == "" ||
		strings.TrimSpace(entry.Successor) == "" {
		return fmt.Errorf("%s requires packagePath, destination, and successor", prefix)
	}
	if entry.PackagePath != strings.TrimSpace(entry.PackagePath) ||
		entry.Destination != strings.TrimSpace(entry.Destination) ||
		entry.Successor != strings.TrimSpace(entry.Successor) {
		return fmt.Errorf("%s identities must not contain leading or trailing whitespace", prefix)
	}
	if !strings.HasPrefix(entry.PackagePath, "pkg/") || strings.Contains(entry.PackagePath, "\\") {
		return fmt.Errorf("%s packagePath must be an exact slash-separated repository-relative pkg/ path", prefix)
	}
	if entry.Class != string(packageTargetTestOnlySourceClass) {
		return fmt.Errorf("%s class = %q, want explicit %q", prefix, entry.Class, packageTargetTestOnlySourceClass)
	}
	for field, value := range map[string]string{
		"packagePath": entry.PackagePath,
		"destination": entry.Destination,
		"successor":   entry.Successor,
	} {
		if strings.ContainsAny(value, "*?[]") {
			return fmt.Errorf("%s %s must be exact and cannot contain wildcards", prefix, field)
		}
	}
	if entry.Stage != packageTargetTestOnlyBaselineStage {
		return fmt.Errorf("%s stage = %q, want %q", prefix, entry.Stage, packageTargetTestOnlyBaselineStage)
	}
	if entry.DeletionGate != packageTargetTestOnlyDeletionGate {
		return fmt.Errorf("%s has an invalid deletion gate", prefix)
	}
	return nil
}

func createPackageTargetTestOnlyBaseline(
	path string,
	findings []packageTargetFinding,
	stdout io.Writer,
) error {
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("refusing to overwrite existing test-only package-target baseline: %s", filepath.ToSlash(path))
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat test-only package-target baseline: %w", err)
	}
	entries := make([]packageTargetTestOnlyBaselineEntry, 0)
	for _, finding := range findings {
		if finding.Class != packageTargetTestOnlySourceClass {
			continue
		}
		entries = append(entries, packageTargetTestOnlyBaselineEntry{
			PackagePath:  finding.PackagePath,
			Destination:  finding.Destination,
			Successor:    finding.Successor,
			Class:        string(packageTargetTestOnlySourceClass),
			Stage:        packageTargetTestOnlyBaselineStage,
			DeletionGate: packageTargetTestOnlyDeletionGate,
		})
	}
	if len(entries) == 0 {
		return fmt.Errorf("refusing to create empty test-only package-target baseline: no test-only package-target debt exists")
	}
	slices.SortFunc(entries, func(left, right packageTargetTestOnlyBaselineEntry) int {
		return strings.Compare(packageTargetBaselineEntryKey(left), packageTargetBaselineEntryKey(right))
	})
	payload, err := json.MarshalIndent(packageTargetTestOnlyBaseline{Version: packageTargetTestOnlyBaselineVersion, Entries: entries}, "", "  ")
	if err != nil {
		return fmt.Errorf("encode test-only package-target baseline: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create test-only package-target baseline directory: %w", err)
	}
	if err := os.WriteFile(path, append(payload, '\n'), 0o644); err != nil {
		return fmt.Errorf("write test-only package-target baseline: %w", err)
	}
	fmt.Fprintf(stdout, "[agent-factory:package-target-manifest] created %s with %d exact deletion-only test-only edge(s)\n", filepath.ToSlash(path), len(entries))
	return nil
}

func writePackageTargetObservationCounts(writer io.Writer, findings []packageTargetFinding) {
	production, testOnly := 0, 0
	for _, finding := range findings {
		switch finding.Class {
		case packageTargetProductionSourceClass:
			production++
		case packageTargetTestOnlySourceClass:
			testOnly++
		}
	}
	fmt.Fprintf(writer, "[agent-factory:package-target-manifest] package-target observations: production=%d test-only=%d\n", production, testOnly)
}

func writePackageTargetViolationCounts(writer io.Writer, productionStale []PackageMapping, testOnlyUnrecorded []packageTargetFinding, testOnlyStale []packageTargetTestOnlyBaselineEntry) {
	fmt.Fprintf(
		writer,
		"[agent-factory:package-target-manifest] dependency violation counts: production=%d test-only=%d\n",
		len(productionStale),
		len(testOnlyUnrecorded)+len(testOnlyStale),
	)
}

func writeStaleProductionPackageTargetRow(writer io.Writer, row PackageMapping) {
	fmt.Fprintf(
		writer,
		"[agent-factory:package-target-manifest] stale production package-target row: %s -> %s (successor %s) [class=production]\n",
		row.PackagePath,
		row.Destination,
		row.Successor,
	)
}

func writePackageTargetFinding(writer io.Writer, label string, finding packageTargetFinding) {
	fmt.Fprintf(
		writer,
		"[agent-factory:package-target-manifest] %s: %s -> %s (successor %s) [class=%s]\n",
		label,
		finding.PackagePath,
		finding.Destination,
		finding.Successor,
		finding.Class,
	)
}

func writeStalePackageTargetTestOnlyBaselineEntry(writer io.Writer, entry packageTargetTestOnlyBaselineEntry, path string) {
	fmt.Fprintf(
		writer,
		"[agent-factory:package-target-manifest] stale test-only package-target baseline entry: %s -> %s (successor %s) [class=%s]; remove this exact entry from %s\n",
		entry.PackagePath,
		entry.Destination,
		entry.Successor,
		entry.Class,
		path,
	)
}
