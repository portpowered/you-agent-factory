package analyzers

import (
	"path/filepath"
	"strings"

	"github.com/portpowered/infinite-you/internal/testlanes"
	"golang.org/x/tools/go/analysis"
)

// TestLane classifies compiler-selected test sources with the shared lane policy.
var TestLane = &analysis.Analyzer{
	Name: "testlane",
	Doc:  "require primary lane ownership for compiled Go test packages",
	Run:  runTestLane,
}

func sourceName(unit, name string) string {
	return strings.TrimSuffix(unit, "_test") + "/" + filepath.Base(name)
}

func runTestLane(pass *analysis.Pass) (any, error) {
	unit, ok := unitKey(pass)
	if !ok {
		return nil, nil
	}
	path := strings.TrimSuffix(unit, "_test")
	if under(path, "tests/adhoc") {
		return nil, nil
	}
	for _, file := range pass.Files {
		if !strings.HasSuffix(pass.Fset.Position(file.Pos()).Filename, "_test.go") {
			continue
		}
		if _, owned := testlanes.ForImportPath(testlanes.ModulePath + "/" + path); !owned {
			pass.Reportf(file.Package, "test-lane-unowned: %s; assign a primary lane in internal/testlanes", path)
		}
		break
	}
	return nil, nil
}
