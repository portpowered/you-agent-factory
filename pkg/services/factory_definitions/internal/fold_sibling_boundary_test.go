package internal_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const factoryDefinitionsModule = "github.com/portpowered/infinite-you/pkg/services/factory_definitions"

var foldedPublicSiblingSuffixes = []string{
	"/definition",
	"/authoredlayout",
	"/portableconfig",
	"/loading",
	"/loadedsource",
	"/snapshotcapture",
	"/packagedinstallation",
	"/packages/packageassets",
	"/packages/promptassets",
	"/packages/goal",
	"/packages/review",
	"/packages/subagent",
	"/packages/tts",
	"/namedfactories",
	"/resource",
}

var foldSiblingScanRoots = []string{
	"../wire",
	".",
}

func TestFoldSiblingBoundary_OwnerPackagesDoNotImportEmptiedPublicSiblings(t *testing.T) {
	t.Parallel()

	for _, root := range foldSiblingScanRoots {
		root := root
		t.Run(root, func(t *testing.T) {
			t.Parallel()
			assertDirectoryTreeDoesNotImportFoldedSiblings(t, root)
		})
	}
}

func assertDirectoryTreeDoesNotImportFoldedSiblings(t *testing.T, root string) {
	t.Helper()

	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if entry.Name() == "testdata" || entry.Name() == "services" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		assertGoFileDoesNotImportFoldedSiblings(t, path)
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
}

func assertGoFileDoesNotImportFoldedSiblings(t *testing.T, path string) {
	t.Helper()

	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, path, nil, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	for _, importSpec := range file.Imports {
		importPath := strings.Trim(importSpec.Path.Value, `"`)
		for _, suffix := range foldedPublicSiblingSuffixes {
			if importPath == factoryDefinitionsModule+suffix ||
				strings.HasPrefix(importPath, factoryDefinitionsModule+suffix+"/") {
				t.Fatalf(
					"%s must not import emptied public sibling %s; use internal/services destinations",
					path,
					importPath,
				)
			}
		}
	}
}
