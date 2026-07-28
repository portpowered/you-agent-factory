package internal_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var distributionFoldedPublicSiblingSuffixes = []string{
	"/packagedinstallation",
	"/packages/packageassets",
	"/packages/promptassets",
	"/packages/goal",
	"/packages/review",
	"/packages/subagent",
	"/packages/tts",
}

var distributionConsumerScanRoots = []string{
	"../../../wire",
}

func TestDistributionConsumerBoundary_ProcessWireDoesNotImportEmptiedDistributionSiblings(t *testing.T) {
	t.Parallel()

	for _, root := range distributionConsumerScanRoots {
		root := root
		t.Run(root, func(t *testing.T) {
			t.Parallel()
			assertDirectoryTreeDoesNotImportDistributionFoldedSiblings(t, root)
		})
	}
}

func assertDirectoryTreeDoesNotImportDistributionFoldedSiblings(t *testing.T, root string) {
	t.Helper()

	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if entry.Name() == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		assertGoFileDoesNotImportDistributionFoldedSiblings(t, path)
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
}

func assertGoFileDoesNotImportDistributionFoldedSiblings(t *testing.T, path string) {
	t.Helper()

	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, path, nil, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	for _, importSpec := range file.Imports {
		importPath := strings.Trim(importSpec.Path.Value, `"`)
		for _, suffix := range distributionFoldedPublicSiblingSuffixes {
			if importPath == factoryDefinitionsModule+suffix ||
				strings.HasPrefix(importPath, factoryDefinitionsModule+suffix+"/") {
				t.Fatalf(
					"%s must not import emptied distribution sibling %s; construct distribute behavior through factory_definitions/service or factory_definitions/wire",
					path,
					importPath,
				)
			}
		}
	}
}
