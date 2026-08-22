package main

import (
	"fmt"
	"os"
	"slices"
	"strings"
)

// prepareUnitCoverageImportFile adds a temporary, test-function-free internal
// test file to one selected unit-test package. Blank imports make Go's
// coverage instrumenter load every test-free measured package into that test
// binary, so those packages retain their zero-count coverage blocks without
// receiving their own go test invocation. The file is removed by the returned
// cleanup after the invocation plan completes.
func prepareUnitCoverageImportFile(testPackages []string, listings []coveragePackageListing) (func() error, error) {
	selected := make(map[string]struct{}, len(testPackages))
	for _, importPath := range testPackages {
		selected[importPath] = struct{}{}
	}

	toImport := make([]coveragePackageListing, 0)
	var target *coveragePackageListing
	for index := range listings {
		listing := listings[index]
		if !isBackendCoveragePackage(listing.importPath) || listing.goFiles == 0 {
			continue
		}
		if _, isTestPackage := selected[listing.importPath]; isTestPackage {
			if target == nil || len(listing.testGoFiles) > 0 {
				candidate := listing
				target = &candidate
			}
			continue
		}
		toImport = append(toImport, listing)
	}
	if len(toImport) == 0 {
		return func() error { return nil }, nil
	}
	if target == nil {
		return nil, fmt.Errorf("prepare unit coverage imports: no build-selected test package is available to carry %d test-free package imports", len(toImport))
	}
	if strings.TrimSpace(target.directory) == "" || strings.TrimSpace(target.packageName) == "" {
		return nil, fmt.Errorf("prepare unit coverage imports: incomplete go list metadata for test package %q (Dir and Name are required)", target.importPath)
	}
	for _, listing := range toImport {
		if strings.TrimSpace(listing.importPath) == "" {
			return nil, fmt.Errorf("prepare unit coverage imports: incomplete go list metadata for measured package %q", listing.importPath)
		}
	}

	slices.SortFunc(toImport, func(left, right coveragePackageListing) int {
		return strings.Compare(left.importPath, right.importPath)
	})
	var source strings.Builder
	fmt.Fprintf(&source, "package %s\n\nimport (\n", target.packageName)
	for _, listing := range toImport {
		fmt.Fprintf(&source, "\t_ %q\n", listing.importPath)
	}
	source.WriteString(")\n")

	file, err := os.CreateTemp(target.directory, "gocoveragecheck_coverage_imports_*_test.go")
	if err != nil {
		return nil, fmt.Errorf("prepare unit coverage imports: create temporary test file in %q: %w", target.directory, err)
	}
	filename := file.Name()
	if _, err := file.WriteString(source.String()); err != nil {
		_ = file.Close()
		_ = os.Remove(filename)
		return nil, fmt.Errorf("prepare unit coverage imports: write temporary test file %q: %w", filename, err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(filename)
		return nil, fmt.Errorf("prepare unit coverage imports: close temporary test file %q: %w", filename, err)
	}

	return func() error {
		if err := os.Remove(filename); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove temporary unit coverage imports %q: %w", filename, err)
		}
		return nil
	}, nil
}
