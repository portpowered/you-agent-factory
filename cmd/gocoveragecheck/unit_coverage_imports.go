package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

const unitCoverageImportFilename = "gocoveragecheck_coverage_imports_test.go"

type unitCoverageImportCarrier struct {
	listing coveragePackageListing
	imports []string
}

// prepareUnitCoverageImportFile adds temporary, test-function-free internal
// test files to selected unit-test packages. Blank imports make Go's coverage
// instrumenter load every test-free measured package into an existing test
// binary, so those packages retain their zero-count coverage blocks without
// receiving their own go test invocation. Imports are grouped by a legal
// carrier because Go's internal-package visibility rules still apply to test
// source. The files are removed by the returned cleanup after the invocation
// plan completes.
func prepareUnitCoverageImportFile(testPackages []string, listings []coveragePackageListing) (func() error, error) {
	selected := make(map[string]struct{}, len(testPackages))
	for _, importPath := range testPackages {
		selected[importPath] = struct{}{}
	}
	selectedDependencies := make(map[string]struct{})
	for _, listing := range listings {
		if _, isTestPackage := selected[listing.importPath]; !isTestPackage {
			continue
		}
		for _, dependency := range listing.deps {
			selectedDependencies[dependency] = struct{}{}
		}
	}

	toImport := make([]coveragePackageListing, 0)
	carriers := make([]unitCoverageImportCarrier, 0)
	for index := range listings {
		listing := listings[index]
		if !isBackendCoveragePackage(listing.importPath) || listing.goFiles == 0 {
			continue
		}
		if _, isTestPackage := selected[listing.importPath]; isTestPackage {
			carriers = append(carriers, unitCoverageImportCarrier{listing: listing})
			continue
		}
		if _, alreadyImported := selectedDependencies[listing.importPath]; alreadyImported {
			continue
		}
		toImport = append(toImport, listing)
	}
	if len(toImport) == 0 {
		return func() error { return nil }, nil
	}
	if len(carriers) == 0 {
		return nil, fmt.Errorf("prepare unit coverage imports: no build-selected test package is available to carry %d test-free package imports", len(toImport))
	}
	slices.SortFunc(carriers, func(left, right unitCoverageImportCarrier) int {
		return strings.Compare(left.listing.importPath, right.listing.importPath)
	})
	slices.SortFunc(toImport, func(left, right coveragePackageListing) int {
		return strings.Compare(left.importPath, right.importPath)
	})

	for _, listing := range toImport {
		carrierIndex, ok := chooseUnitCoverageImportCarrier(listing.importPath, listing.deps, carriers)
		if !ok {
			return nil, fmt.Errorf("prepare unit coverage imports: no selected test package can legally import test-free package %q", listing.importPath)
		}
		carriers[carrierIndex].imports = append(carriers[carrierIndex].imports, listing.importPath)
	}

	filenames := make([]string, 0, len(carriers))
	cleanup := func() error {
		var cleanupErr error
		for _, filename := range filenames {
			if err := os.Remove(filename); err != nil && !os.IsNotExist(err) {
				cleanupErr = errors.Join(cleanupErr, fmt.Errorf("remove temporary unit coverage imports %q: %w", filename, err))
			}
		}
		return cleanupErr
	}

	for _, carrier := range carriers {
		if len(carrier.imports) == 0 {
			continue
		}
		if strings.TrimSpace(carrier.listing.directory) == "" || strings.TrimSpace(carrier.listing.packageName) == "" {
			return cleanup, fmt.Errorf("prepare unit coverage imports: incomplete go list metadata for test package %q (Dir and Name are required)", carrier.listing.importPath)
		}
		var source strings.Builder
		fmt.Fprintf(&source, "package %s\n\nimport (\n", carrier.listing.packageName)
		for _, importPath := range carrier.imports {
			fmt.Fprintf(&source, "\t_ %q\n", importPath)
		}
		source.WriteString(")\n")

		filename := filepath.Join(carrier.listing.directory, unitCoverageImportFilename)
		file, err := os.OpenFile(filename, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err != nil {
			return cleanup, fmt.Errorf("prepare unit coverage imports: create temporary test file in %q: %w", carrier.listing.directory, err)
		}
		filenames = append(filenames, filename)
		if _, err := file.WriteString(source.String()); err != nil {
			_ = file.Close()
			return cleanup, fmt.Errorf("prepare unit coverage imports: write temporary test file %q: %w", filename, err)
		}
		if err := file.Close(); err != nil {
			return cleanup, fmt.Errorf("prepare unit coverage imports: close temporary test file %q: %w", filename, err)
		}
	}

	return cleanup, nil
}

func chooseUnitCoverageImportCarrier(importPath string, deps []string, carriers []unitCoverageImportCarrier) (int, bool) {
	bestIndex := -1
	bestPrefixLength := -1
	bestImportCount := -1
	for index, carrier := range carriers {
		if !canUnitCoverageImport(carrier.listing.importPath, importPath) {
			continue
		}
		if slices.Contains(deps, carrier.listing.importPath) {
			continue
		}
		prefixLength := commonImportPathPrefixLength(carrier.listing.importPath, importPath)
		importCount := len(carrier.imports)
		if prefixLength > bestPrefixLength || (prefixLength == bestPrefixLength && importCount > bestImportCount) {
			bestIndex = index
			bestPrefixLength = prefixLength
			bestImportCount = importCount
		}
	}
	return bestIndex, bestIndex >= 0
}

func commonImportPathPrefixLength(left, right string) int {
	leftParts := strings.Split(left, "/")
	rightParts := strings.Split(right, "/")
	length := 0
	for length < len(leftParts) && length < len(rightParts) && leftParts[length] == rightParts[length] {
		length++
	}
	return length
}

func canUnitCoverageImport(importer, imported string) bool {
	if importer == imported {
		return false
	}
	parts := strings.Split(imported, "/")
	internalIndex := -1
	for index, part := range parts {
		if part == "internal" {
			internalIndex = index
		}
	}
	if internalIndex < 0 {
		return true
	}
	parent := strings.Join(parts[:internalIndex], "/")
	return importer == parent || strings.HasPrefix(importer, parent+"/")
}
