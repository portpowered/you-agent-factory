package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// listProductionPkgPackages returns every production Go package under pkg/ in
// deterministic byte-order (slash-normalized path) sort. A production package is
// a directory that contains at least one non-test .go source file. Directories
// named testdata, vendor, or dotted names are skipped. The ordered list is the
// deletion-only nonconformance ledger seed.
func listProductionPkgPackages(repoRoot string) ([]string, error) {
	pkgRoot := filepath.Join(repoRoot, "pkg")
	info, err := os.Stat(pkgRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("stat pkg root: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("pkg root is not a directory: %s", pkgRoot)
	}

	productionDirs := map[string]struct{}{}
	walkErr := filepath.WalkDir(pkgRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			name := entry.Name()
			if path != pkgRoot && (name == "testdata" || name == "vendor" || strings.HasPrefix(name, ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}
		relDir, err := filepath.Rel(repoRoot, filepath.Dir(path))
		if err != nil {
			return fmt.Errorf("resolve package path for %s: %w", path, err)
		}
		packagePath := filepath.ToSlash(relDir)
		if packagePath == "." || !strings.HasPrefix(packagePath, "pkg/") {
			return nil
		}
		productionDirs[packagePath] = struct{}{}
		return nil
	})
	if walkErr != nil {
		return nil, fmt.Errorf("walk pkg packages: %w", walkErr)
	}

	packages := make([]string, 0, len(productionDirs))
	for packagePath := range productionDirs {
		packages = append(packages, packagePath)
	}
	// Byte-order / path sort on slash-normalized repository-relative paths.
	slices.Sort(packages)
	return packages, nil
}

// validateRowsNamePackagesThatExist proves every committed migration row still
// names a package present in the live tree, so a row cannot silently outlive the
// package it describes.
//
// Completeness is deliberately asserted in one direction only. A live package
// with no row is valid, expected state: its destination is derivable from its
// own path, so adding or deleting a package inside a service needs no manifest
// edit.
func validateRowsNamePackagesThatExist(repoRoot string, packages []PackageMapping) error {
	live, err := listProductionPkgPackages(repoRoot)
	if err != nil {
		return err
	}
	liveSet := make(map[string]struct{}, len(live))
	for _, packagePath := range live {
		liveSet[packagePath] = struct{}{}
	}

	var stale []string
	for i, row := range packages {
		packagePath := row.PackagePath
		if strings.Contains(packagePath, "\\") {
			return fmt.Errorf("packages[%d] %q must use slash separators", i, packagePath)
		}
		if !strings.HasPrefix(packagePath, "pkg/") {
			return fmt.Errorf("packages[%d] %q must be repository-relative under pkg/", i, packagePath)
		}
		if _, alive := liveSet[packagePath]; !alive {
			stale = append(stale, packagePath)
		}
	}
	if len(stale) > 0 {
		slices.Sort(stale)
		return fmt.Errorf(
			"packages name %d package(s) that no longer exist in the live tree: %s\nLINT_VIOLATION_COUNT: %d",
			len(stale),
			strings.Join(stale, ", "),
			len(stale),
		)
	}
	return nil
}
