package responsestreamremovalgate

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const legacyCompatPackageRelativePath = "pkg/factory/sessions/responsestream/compat"

var legacyCompatImportMarkers = []string{
	"responsestream/compat",
	"compat.MapFragment",
}

var retiredPrivateContractProductionSymbols = []string{
	`"recordType":"progress"`,
	`"recordType":"compaction"`,
	`"recordType":"primary_result"`,
	`"recordType": "progress"`,
	`"recordType": "compaction"`,
	`"recordType": "primary_result"`,
}

// AssertLegacyCompatMapperDeleted proves the retired responsestream/compat mapper
// package is gone and no production package still imports it.
func AssertLegacyCompatMapperDeleted(repoRoot string) error {
	if strings.TrimSpace(repoRoot) == "" {
		return fmt.Errorf("repo root is required")
	}
	compatPath := filepath.Join(repoRoot, filepath.FromSlash(legacyCompatPackageRelativePath))
	if info, err := os.Stat(compatPath); err == nil && info.IsDir() {
		return fmt.Errorf("legacy compat mapper package still exists at %s", legacyCompatPackageRelativePath)
	}
	if err := assertNoLegacyCompatImports(repoRoot); err != nil {
		return err
	}
	return nil
}

func assertNoLegacyCompatImports(repoRoot string) error {
	shouldSkipDir := func(scanRoot, path string, explicitSkips ...string) bool {
		rel, err := filepath.Rel(scanRoot, path)
		if err != nil {
			return false
		}
		rel = filepath.ToSlash(filepath.Clean(rel))
		if rel == "." {
			return false
		}
		for _, part := range strings.Split(rel, "/") {
			if strings.HasPrefix(part, ".") {
				return true
			}
		}
		for _, skip := range explicitSkips {
			if rel == filepath.ToSlash(filepath.Clean(skip)) {
				return true
			}
		}
		return false
	}

	pkgRoot := filepath.Join(repoRoot, "pkg")
	err := filepath.WalkDir(pkgRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if shouldSkipDir(pkgRoot, path, "generated") {
				return filepath.SkipDir
			}
			if strings.HasSuffix(path, filepath.Join("responsestream", "removalgate")) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := string(contents)
		for _, marker := range legacyCompatImportMarkers {
			if strings.Contains(text, marker) {
				rel, relErr := filepath.Rel(repoRoot, path)
				if relErr != nil {
					rel = path
				}
				return fmt.Errorf("%s still references retired legacy compat mapper marker %q", rel, marker)
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("scan pkg for legacy compat mapper imports: %w", err)
	}
	return nil
}

// AssertNoRetiredPrivateContractSymbolsInProductionSurfaces scans supported
// CLI/API transport production code for retired private-contract parser symbols.
func AssertNoRetiredPrivateContractSymbolsInProductionSurfaces(repoRoot string) error {
	shouldSkipDir := func(scanRoot, path string, explicitSkips ...string) bool {
		rel, err := filepath.Rel(scanRoot, path)
		if err != nil {
			return false
		}
		rel = filepath.ToSlash(filepath.Clean(rel))
		if rel == "." {
			return false
		}
		for _, part := range strings.Split(rel, "/") {
			if strings.HasPrefix(part, ".") {
				return true
			}
		}
		for _, skip := range explicitSkips {
			if rel == filepath.ToSlash(filepath.Clean(skip)) {
				return true
			}
		}
		return false
	}

	for _, relRoot := range productionSurfaceRoots {
		absRoot := filepath.Join(repoRoot, filepath.FromSlash(relRoot))
		err := filepath.WalkDir(absRoot, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				if shouldSkipDir(
					absRoot,
					path,
					"generated",
					"contracttests",
					"servertests",
				) {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			contents, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			text := string(contents)
			for _, symbol := range retiredPrivateContractProductionSymbols {
				if strings.Contains(text, symbol) {
					rel, relErr := filepath.Rel(repoRoot, path)
					if relErr != nil {
						rel = path
					}
					return fmt.Errorf("%s contains retired private-contract symbol %q", rel, symbol)
				}
			}
			return nil
		})
		if err != nil {
			return fmt.Errorf("scan %s: %w", relRoot, err)
		}
	}
	return nil
}
