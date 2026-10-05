package analyzers

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/portpowered/infinite-you/internal/packagedfactorycatalog"
	"golang.org/x/tools/go/analysis"
)

// PackagedFactoryCatalog retains publication validation in the shared lint lane.
var PackagedFactoryCatalog = &analysis.Analyzer{
	Name: "packagedfactorycatalog",
	Doc:  "reject invalid or drifting packaged Factory publications",
	Run: func(pass *analysis.Pass) (any, error) {
		if !catalogOwner(pass) {
			return nil, nil
		}
		return PackagedFactoryCatalogForDirectory(filepath.Dir(pass.Fset.Position(pass.Files[0].Package).Filename)).Run(pass)
	},
}

// PackagedFactoryCatalogForDirectory captures all non-Go inputs before the
// plugin's issue-cache lookup. Its content identity invalidates cached success
// on byte, path, kind or input-error changes without mutating Go sources.
func PackagedFactoryCatalogForDirectory(directory string) *analysis.Analyzer {
	snapshot, err := loadCatalogSnapshot(directory)
	return packagedFactoryCatalogSnapshot(snapshot, err)
}

func catalogOwner(pass *analysis.Pass) bool {
	unit, ok := unitKey(pass)
	return ok && unit == "internal/lint/analyzers"
}

func packagedFactoryCatalogSnapshot(source catalogSnapshot, inputError error) *analysis.Analyzer {
	// Copy once; callers and compilation units cannot change captured inputs.
	snapshot := catalogSnapshot{}
	paths := make([]string, 0, len(source))
	for name, file := range source {
		file.data = bytes.Clone(file.data)
		snapshot[name] = file
		paths = append(paths, name)
	}
	sort.Strings(paths)
	hash := sha256.New()
	for _, name := range paths {
		file := snapshot[name]
		fmt.Fprintf(hash, "%d:%s:%d:%d:", len(name), name, file.mode, len(file.data))
		_, _ = hash.Write(file.data)
	}
	fmt.Fprintf(hash, "error:%v", inputError)
	analyzer := &analysis.Analyzer{Name: fmt.Sprintf("packagedfactorycatalog_%x", hash.Sum(nil)[:12]), Doc: "reject invalid or drifting packaged Factory publications"}
	analyzer.Run = func(pass *analysis.Pass) (any, error) {
		if !catalogOwner(pass) {
			return nil, nil
		}
		for _, finding := range catalogFindings(snapshot, inputError) {
			pass.Reportf(pass.Files[0].Package, "packaged-factory-catalog: %s; run `make packaged-factory-catalog-generate`", finding)
		}
		return nil, nil
	}
	return analyzer
}

func catalogFindings(snapshot catalogSnapshot, inputError error) []string {
	if inputError != nil {
		return []string{fmt.Sprintf("input failure: %v", inputError)}
	}
	catalog, err := packagedfactorycatalog.BuildCatalog(context.Background(), snapshot, "factories", "schemas/factory.schema.json")
	if err != nil {
		return []string{fmt.Sprintf("projection failed: %v", err)}
	}
	var findings []string
	for name, expected := range catalog.Files {
		actual, exists := snapshot[name]
		switch {
		case !exists:
			findings = append(findings, "missing: "+name)
		case !actual.mode.IsRegular():
			findings = append(findings, "non-regular: "+name)
		case name != "generated/README.md" && !bytes.Equal(expected, actual.data):
			findings = append(findings, "stale: "+name)
		}
	}
	for name := range snapshot {
		if _, expected := catalog.Files[name]; strings.HasPrefix(name, "generated/") && !expected {
			findings = append(findings, "unexpected: "+name)
		}
	}
	sort.Strings(findings)
	return findings
}
