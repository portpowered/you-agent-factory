package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// packageTargetSourceClass identifies the kind of Go source that makes an
// unfinished package-target row observable. Keeping the class on the finding
// prevents test-only evidence from being folded into production counts.
type packageTargetSourceClass string

const (
	packageTargetProductionSourceClass packageTargetSourceClass = "production"
	packageTargetTestOnlySourceClass   packageTargetSourceClass = "test-only"
)

type packageTargetFinding struct {
	PackagePath string
	Destination string
	Successor   string
	Class       packageTargetSourceClass
}

// packageTargetSourceClasses examines direct Go files in one package
// directory. Nested directories are separate packages and remain represented
// by their own move rows. Classification happens while the directory is being
// discovered, before any production-only view can discard _test.go files.
func packageTargetSourceClasses(repoRoot, packagePath string) (map[packageTargetSourceClass]struct{}, error) {
	if strings.Contains(packagePath, "\\") {
		return nil, fmt.Errorf("package path %q must use slash separators", packagePath)
	}
	if !strings.HasPrefix(packagePath, "pkg/") {
		return nil, fmt.Errorf("package path %q must be repository-relative under pkg/", packagePath)
	}

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

// scanPackageTargetFindings observes only package paths named by the
// unfinished-move ledger. This preserves the ledger's deletion-only role while
// making both source classes visible for each relevant package-target edge.
func scanPackageTargetFindings(repoRoot string, moves []PackageMapping) ([]packageTargetFinding, error) {
	findings := make([]packageTargetFinding, 0, len(moves)*2)
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

func hasPackageTargetClass(findings []packageTargetFinding, packagePath string, class packageTargetSourceClass) bool {
	for _, finding := range findings {
		if finding.PackagePath == packagePath && finding.Class == class {
			return true
		}
	}
	return false
}

// packageTargetProductionStaleRows retains the existing blocking path for a
// move row whose package directory has disappeared. A visible test-only source
// is reported separately and is intentionally non-blocking; it does not turn
// into a production finding or a baseline entry.
func packageTargetProductionStaleRows(moves []PackageMapping, findings []packageTargetFinding) []PackageMapping {
	stale := make([]PackageMapping, 0)
	for _, row := range moves {
		if hasPackageTargetClass(findings, row.PackagePath, packageTargetProductionSourceClass) ||
			hasPackageTargetClass(findings, row.PackagePath, packageTargetTestOnlySourceClass) {
			continue
		}
		stale = append(stale, row)
	}
	slices.SortFunc(stale, func(left, right PackageMapping) int {
		return strings.Compare(packageTargetRowKey(left), packageTargetRowKey(right))
	})
	return stale
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

func writePackageTargetViolationCountsForFindings(writer io.Writer, productionStale []PackageMapping, findings []packageTargetFinding) {
	testOnly := 0
	for _, finding := range findings {
		if finding.Class == packageTargetTestOnlySourceClass {
			testOnly++
		}
	}
	fmt.Fprintf(
		writer,
		"[agent-factory:package-target-manifest] dependency violation counts: production=%d test-only=%d\n",
		len(productionStale),
		testOnly,
	)
}

func writePackageTargetTestOnlyObservations(writer io.Writer, findings []packageTargetFinding) {
	for _, finding := range findings {
		if finding.Class != packageTargetTestOnlySourceClass {
			continue
		}
		fmt.Fprintf(
			writer,
			"[agent-factory:package-target-manifest] test-only observation: %s -> %s (successor %s) [class=%s]\n",
			finding.PackagePath,
			finding.Destination,
			finding.Successor,
			finding.Class,
		)
	}
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
