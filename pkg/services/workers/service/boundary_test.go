package service_test

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// pkgmaintcheck:ignore-cyclomatic-complexity service-ownership migration preserves this decision flow; simplify branches and remove this exemption.
func TestExecutorBuilderIsOwnedByWorkerExecutionService(t *testing.T) {
	t.Parallel()

	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test source path")
	}
	ownerDir := filepath.Clean(filepath.Join(filepath.Dir(filename), "..", "internal"))
	repoRoot := filepath.Clean(filepath.Join(ownerDir, "..", "..", "..", ".."))

	var violations []string
	err := filepath.WalkDir(repoRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if entry.Name() == ".git" || entry.Name() == "node_modules" || entry.Name() == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") || strings.HasPrefix(path, ownerDir+string(filepath.Separator)) {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if bytes.Contains(content, []byte("ExecutorBuilder.Build(")) || bytes.Contains(content, []byte("ExecutorBuilder.BuildLogical(")) {
			relative, err := filepath.Rel(repoRoot, path)
			if err != nil {
				return err
			}
			violations = append(violations, filepath.ToSlash(relative))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan executor construction ownership: %v", err)
	}
	if len(violations) != 0 {
		t.Fatalf("ExecutorBuilder construction must remain in pkg/services/workers/internal; found %v", violations)
	}
}
