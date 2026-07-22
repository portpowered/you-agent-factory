package retiredsurfaceguard

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

// NamedFactoryMapper maps a canonical named-factory display name to an on-disk directory.
type NamedFactoryMapper func(factoriesRoot, name string) (string, error)

// ScanEncodedPathReintroductionViolations reports production named-factory mapping
// that prefers percent-encoded scoped leaf layout.
func ScanEncodedPathReintroductionViolations(mapper NamedFactoryMapper, factoriesRoot string, scopedNames []string) []Violation {
	if mapper == nil {
		return []Violation{{
			Family:  "encoded-path",
			Surface: "named-factory-mapper",
			Detail:  "named-factory mapper is required",
		}}
	}

	var violations []Violation
	for _, name := range scopedNames {
		mappedDir, err := mapper(factoriesRoot, name)
		if err != nil {
			violations = append(violations, Violation{
				Family:  "encoded-path",
				Surface: name,
				Detail:  fmt.Sprintf("map named factory dir: %v", err),
			})
			continue
		}
		if strings.Contains(mappedDir, "%2F") {
			violations = append(violations, Violation{
				Family:  "encoded-path",
				Surface: name,
				Detail:  "production named-factory mapping must not use percent-encoded scoped leaf names",
			})
		}
	}
	return violations
}

var legacyEncodedLayoutSymbols = []string{
	"LegacyLayoutSegment",
	"LegacyLayoutSegmentToName",
	"encodeScopedLegacyLayoutSegment",
	"NamedFactoryNameToLayoutSegment",
	"NamedFactoryLayoutSegmentToName",
}

// ScanEncodedPathProductionSourceViolations reports production Go files that call
// legacy encoded layout helpers outside the fixture-only helper definition.
func ScanEncodedPathProductionSourceViolations(repoRoot string) ([]Violation, error) {
	repoRoot, err := filepath.Abs(repoRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve repository root: %w", err)
	}

	scanRoots := []string{
		filepath.Join(repoRoot, "pkg", "config"),
	}
	var violations []Violation
	for _, scanRoot := range scanRoots {
		if _, statErr := os.Stat(scanRoot); statErr != nil {
			if errors.Is(statErr, os.ErrNotExist) {
				continue
			}
			return nil, fmt.Errorf("walk %s: %w", scanRoot, statErr)
		}
		err := filepath.WalkDir(scanRoot, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				switch entry.Name() {
				case "runtimetests", "testdata", "openapitests", "maptests", "exhaustiontests":
					return filepath.SkipDir
				}
				return nil
			}
			if !isProductionGoFile(entry.Name()) {
				return nil
			}
			if filepath.Base(path) == "legacy_layout_segment.go" {
				return nil
			}
			fileViolations, err := scanGoFileForLegacyEncodedSymbols(path)
			if err != nil {
				return err
			}
			violations = append(violations, fileViolations...)
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("walk %s: %w", scanRoot, err)
		}
	}
	return violations, nil
}

func scanGoFileForLegacyEncodedSymbols(path string) ([]Violation, error) {
	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, path, nil, 0)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", filepath.ToSlash(path), err)
	}

	var violations []Violation
	ast.Inspect(file, func(node ast.Node) bool {
		selector, ok := node.(*ast.SelectorExpr)
		if !ok || selector.Sel == nil {
			return true
		}
		for _, symbol := range legacyEncodedLayoutSymbols {
			if selector.Sel.Name != symbol {
				continue
			}
			violations = append(violations, Violation{
				Family:  "encoded-path",
				Surface: filepath.ToSlash(path),
				Detail:  "production source must not call legacy encoded layout helper " + symbol,
			})
		}
		ident, ok := node.(*ast.Ident)
		if !ok || ident.Obj == nil {
			return true
		}
		for _, symbol := range legacyEncodedLayoutSymbols {
			if ident.Name != symbol {
				continue
			}
			if _, ok := ident.Obj.Decl.(*ast.FuncDecl); ok {
				continue
			}
			violations = append(violations, Violation{
				Family:  "encoded-path",
				Surface: filepath.ToSlash(path),
				Detail:  "production source must not reference legacy encoded layout helper " + symbol,
			})
		}
		return true
	})
	return violations, nil
}

func isProductionGoFile(name string) bool {
	return strings.HasSuffix(name, ".go") && !strings.HasSuffix(name, "_test.go")
}
