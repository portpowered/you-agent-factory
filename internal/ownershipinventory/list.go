package ownershipinventory

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// FindRepositoryRoot walks upward from the working directory until it finds the
// ownership-inventory freeze marker or go.mod plus pkg/.
func FindRepositoryRoot() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get working directory: %w", err)
	}
	dir := wd
	for {
		inventory := filepath.Join(dir, filepath.FromSlash(InventoryRelativePath))
		if _, err := os.Stat(inventory); err == nil {
			return dir, nil
		}
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			if _, err := os.Stat(filepath.Join(dir, "pkg")); err == nil {
				return dir, nil
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("repository root not found from %s", wd)
		}
		dir = parent
	}
}

// ListProductionPackages returns every production Go package path under pkg,
// stable-sorted by slash-separated packagePath byte order.
func ListProductionPackages(root string) ([]string, error) {
	pkgRoot := filepath.Join(root, "pkg")
	info, err := os.Stat(pkgRoot)
	if err != nil {
		return nil, fmt.Errorf("stat pkg: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("pkg is not a directory")
	}

	seen := map[string]struct{}{}
	err = filepath.WalkDir(pkgRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			name := entry.Name()
			if name == "testdata" || name == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(entry.Name(), ".go") {
			return nil
		}
		// Skip files that are clearly not part of a package build input? Keep all
		// .go-bearing directories, including tests: production import paths include
		// packages that only expose *_test helpers as distinct directories.
		dir := filepath.Dir(path)
		rel, err := filepath.Rel(root, dir)
		if err != nil {
			return err
		}
		packagePath := filepath.ToSlash(rel)
		if !strings.HasPrefix(packagePath, "pkg/") && packagePath != "pkg" {
			return nil
		}
		seen[packagePath] = struct{}{}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk pkg: %w", err)
	}

	out := make([]string, 0, len(seen))
	for packagePath := range seen {
		out = append(out, packagePath)
	}
	slices.Sort(out)
	return out, nil
}
