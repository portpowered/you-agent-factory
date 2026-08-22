package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrepareUnitCoverageImportFilePreservesTestFreeCoveragePackages(t *testing.T) {
	packageDir := t.TempDir()
	testPackage := modulePath + "/pkg/coverageimports/testful"
	listings := []coveragePackageListing{
		{
			importPath:  modulePath + "/pkg/coverageimports/generated",
			directory:   filepath.Join(packageDir, "generated"),
			packageName: "generated",
			goFiles:     1,
		},
		{
			importPath:  testPackage,
			directory:   packageDir,
			packageName: "testful",
			goFiles:     1,
			testGoFiles: []string{"testful_test.go"},
		},
		{
			importPath:  modulePath + "/pkg/coverageimports/testless",
			directory:   filepath.Join(packageDir, "testless"),
			packageName: "testless",
			goFiles:     1,
		},
	}

	cleanup, err := prepareUnitCoverageImportFile([]string{testPackage}, listings)
	if err != nil {
		t.Fatalf("prepareUnitCoverageImportFile() error = %v", err)
	}
	files, err := filepath.Glob(filepath.Join(packageDir, "gocoveragecheck_coverage_imports_*_test.go"))
	if err != nil {
		t.Fatalf("find temporary coverage import file: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("temporary coverage import files = %v, want one", files)
	}
	contents, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatalf("read temporary coverage import file: %v", err)
	}
	text := string(contents)
	if !strings.HasPrefix(text, "package testful\n\nimport (\n") {
		t.Fatalf("temporary coverage import file = %q, want target package declaration", text)
	}
	for _, importPath := range []string{
		modulePath + "/pkg/coverageimports/generated",
		modulePath + "/pkg/coverageimports/testless",
	} {
		if !strings.Contains(text, "_ \""+importPath+"\"") {
			t.Fatalf("temporary coverage import file = %q, missing %s", text, importPath)
		}
	}
	if strings.Contains(text, testPackage) {
		t.Fatalf("temporary coverage import file imported selected test package: %q", text)
	}

	if err := cleanup(); err != nil {
		t.Fatalf("temporary coverage import cleanup: %v", err)
	}
	if _, err := os.Stat(files[0]); !os.IsNotExist(err) {
		t.Fatalf("temporary coverage import file stat error after cleanup = %v, want not-exist", err)
	}
}

func TestPrepareUnitCoverageImportFileFailsClosedOnMissingMetadata(t *testing.T) {
	_, err := prepareUnitCoverageImportFile(
		[]string{modulePath + "/pkg/coverageimports/testful"},
		[]coveragePackageListing{
			{
				importPath:  modulePath + "/pkg/coverageimports/testful",
				packageName: "testful",
				goFiles:     1,
				testGoFiles: []string{"testful_test.go"},
			},
			{
				importPath: modulePath + "/pkg/coverageimports/testless",
				goFiles:    1,
			},
		},
	)
	if err == nil || !strings.Contains(err.Error(), "Dir and Name are required") {
		t.Fatalf("prepareUnitCoverageImportFile() error = %v, want incomplete target metadata diagnostic", err)
	}
}
