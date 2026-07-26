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

func validateInventory(repoRoot string, inventory []string) error {
	live, err := listProductionPkgPackages(repoRoot)
	if err != nil {
		return err
	}
	if len(inventory) == 0 {
		return fmt.Errorf("inventory must list every production pkg package; found 0 rows, live tree has %d", len(live))
	}
	if !slices.IsSorted(inventory) {
		return fmt.Errorf("inventory must be stable-sorted in byte-order path sort")
	}
	seen := make(map[string]struct{}, len(inventory))
	for i, packagePath := range inventory {
		if strings.TrimSpace(packagePath) == "" {
			return fmt.Errorf("inventory[%d] is empty", i)
		}
		if strings.Contains(packagePath, "\\") {
			return fmt.Errorf("inventory[%d] %q must use slash separators", i, packagePath)
		}
		if !strings.HasPrefix(packagePath, "pkg/") {
			return fmt.Errorf("inventory[%d] %q must be repository-relative under pkg/", i, packagePath)
		}
		if _, dup := seen[packagePath]; dup {
			return fmt.Errorf("inventory contains duplicate package path %q", packagePath)
		}
		seen[packagePath] = struct{}{}
	}
	if !slices.Equal(inventory, live) {
		missing, extra := diffStringSets(live, inventory)
		switch {
		case len(missing) > 0 && len(extra) > 0:
			return fmt.Errorf("inventory does not match live production packages; missing example %q; extra example %q", missing[0], extra[0])
		case len(missing) > 0:
			return fmt.Errorf("inventory is missing production package %q", missing[0])
		case len(extra) > 0:
			return fmt.Errorf("inventory contains unknown package %q", extra[0])
		default:
			return fmt.Errorf("inventory order does not match stable-sorted live production packages")
		}
	}
	return nil
}

func diffStringSets(want, got []string) (missing, extra []string) {
	wantSet := make(map[string]struct{}, len(want))
	gotSet := make(map[string]struct{}, len(got))
	for _, value := range want {
		wantSet[value] = struct{}{}
	}
	for _, value := range got {
		gotSet[value] = struct{}{}
	}
	for _, value := range want {
		if _, ok := gotSet[value]; !ok {
			missing = append(missing, value)
		}
	}
	for _, value := range got {
		if _, ok := wantSet[value]; !ok {
			extra = append(extra, value)
		}
	}
	slices.Sort(missing)
	slices.Sort(extra)
	return missing, extra
}
