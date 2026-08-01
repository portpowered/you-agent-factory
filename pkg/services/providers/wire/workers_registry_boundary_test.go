package wire_test

import (
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

func TestProvidersWireDoesNotPublishWorkersProviderRegistry(t *testing.T) {
	t.Parallel()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test file path")
	}
	root := filepath.Dir(file)
	entries, err := filepath.Glob(filepath.Join(root, "*.go"))
	if err != nil {
		t.Fatalf("glob providers/wire: %v", err)
	}
	forbidden := []string{
		"NewWorkersRegistry",
		"workers.ProviderRegistry",
		"var _ workers.ProviderRegistry",
	}
	for _, source := range entries {
		if strings.HasSuffix(source, "_test.go") {
			continue
		}
		content, err := os.ReadFile(source)
		if err != nil {
			t.Fatalf("read %s: %v", source, err)
		}
		body := string(content)
		for _, item := range forbidden {
			if strings.Contains(body, item) {
				t.Fatalf("%s still contains Workers registry edge %q", source, item)
			}
		}
	}
}

func TestProvidersCanonicalEffectCodeDoesNotImportWorkersContracts(t *testing.T) {
	t.Parallel()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test file path")
	}
	providersWireRoot := filepath.Dir(file)
	roots := []string{
		providersWireRoot,
		filepath.Join(providersWireRoot, "..", "internal", "service", "effects.go"),
		filepath.Join(providersWireRoot, "..", "internal", "services", "execution", "wire"),
		filepath.Join(providersWireRoot, "..", "internal", "services", "execution", "internal", "adapters"),
	}
	const workersImportPrefix = "github.com/portpowered/infinite-you/pkg/services/workers"
	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
				return nil
			}
			file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
			if err != nil {
				return err
			}
			for _, specification := range file.Imports {
				importPath, err := strconv.Unquote(specification.Path.Value)
				if err != nil {
					return err
				}
				if importPath == workersImportPrefix || strings.HasPrefix(importPath, workersImportPrefix+"/") {
					return fmt.Errorf("%s imports Workers contract %q", path, importPath)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}
