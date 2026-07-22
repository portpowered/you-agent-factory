package directoryreplace

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPlatformDirectoryReplacementContainsNoFactoryPolicy(t *testing.T) {
	t.Parallel()

	source, err := os.ReadFile("replace.go")
	if err != nil {
		t.Fatalf("read replace.go: %v", err)
	}
	if strings.Contains(strings.ToLower(string(source)), "factory") {
		t.Fatal("Platform directory replacement must remain product-policy free; Factory diagnostics belong to Factory Definitions")
	}
}

func TestProductionDirectoryReplacementAdapterIsSelectedOnlyByWire(t *testing.T) {
	t.Parallel()

	repositoryRoot := filepath.Clean(filepath.Join("..", "..", ".."))
	importMarker := `github.com/portpowered/infinite-you/pkg/platform/directoryreplace`
	err := filepath.WalkDir(filepath.Join(repositoryRoot, "pkg"), func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		source, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(source), importMarker) {
			relative, err := filepath.Rel(repositoryRoot, path)
			if err != nil {
				return err
			}
			if !strings.HasPrefix(filepath.ToSlash(relative), "pkg/wire/") {
				t.Errorf("%s selects the directory replacement adapter outside Wire", filepath.ToSlash(relative))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan production imports: %v", err)
	}
}
