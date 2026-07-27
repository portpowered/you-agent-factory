package script_pollers_test

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"go/parser"
	"go/token"
)

const scriptPollersImportPrefix = "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/script_pollers"

func TestPrivateOwnerReachedOnlyThroughAutomationsComposition(t *testing.T) {
	t.Parallel()

	repositoryRoot := findRepositoryRoot(t)
	allowedImporterPrefixes := []string{
		filepath.ToSlash(filepath.Join("pkg", "services", "automations", "service")),
		filepath.ToSlash(filepath.Join("pkg", "services", "automations", "internal", "services", "script_pollers")),
	}

	var violations []string
	err := filepath.WalkDir(repositoryRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", "node_modules", "vendor":
				return filepath.SkipDir
			default:
				return nil
			}
		}
		if !strings.HasSuffix(entry.Name(), ".go") {
			return nil
		}

		relativePath := filepath.ToSlash(path[len(repositoryRoot)+1:])
		if strings.HasPrefix(relativePath, "pkg/services/automations/internal/services/script_pollers/") {
			return nil
		}
		for _, allowed := range allowedImporterPrefixes {
			if strings.HasPrefix(relativePath, allowed) {
				return nil
			}
		}

		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, spec := range file.Imports {
			importPath, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				return err
			}
			if importPath == scriptPollersImportPrefix ||
				strings.HasPrefix(importPath, scriptPollersImportPrefix+"/") {
				violations = append(violations, relativePath+" imports "+importPath)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan repository imports: %v", err)
	}
	if len(violations) > 0 {
		t.Fatalf(
			"script_pollers private owner imports outside Automations composition:\n%s",
			strings.Join(violations, "\n"),
		)
	}
}

func findRepositoryRoot(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("repository root not found from test working directory")
		}
		dir = parent
	}
}
