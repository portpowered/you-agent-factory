package portableconfig

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

type portableOperationFileSystem struct {
	platformfilesystem.Local
	walk func(string, fs.WalkDirFunc) error
}

func (f portableOperationFileSystem) WalkDir(root string, fn fs.WalkDirFunc) error {
	return f.walk(root, fn)
}

func TestPortableOperationConstructorsRequireDirectoryWalker(t *testing.T) {
	t.Parallel()

	constructors := map[string]func() error{
		"bundled files": func() error {
			_, err := NewPortableBundledFilesApplier(nil)
			return err
		},
		"starter Work": func() error {
			_, err := NewFactoryStarterWorkApplier(nil)
			return err
		},
		"document pruning": func() error {
			_, err := NewPortableBundledDocsPruner(nil)
			return err
		},
	}
	for name, construct := range constructors {
		name, construct := name, construct
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if err := construct(); err == nil ||
				!strings.Contains(err.Error(), "portable filesystem is required") {
				t.Fatalf("constructor error = %v, want required filesystem", err)
			}
		})
	}
}

func TestPortableOperationsUseInjectedDirectoryWalker(t *testing.T) {
	t.Parallel()

	walkErr := errors.New("injected walk failed")
	fileSystem := portableOperationFileSystem{walk: func(
		string,
		fs.WalkDirFunc,
	) error {
		return walkErr
	}}

	t.Run("bundled files", func(t *testing.T) {
		factoryDir := filepath.Join(t.TempDir(), "factory")
		mustMakePortableDirectory(t, filepath.Join(factoryDir, "scripts"))
		apply, err := NewPortableBundledFilesApplier(fileSystem)
		if err != nil {
			t.Fatalf("construct operation: %v", err)
		}
		if err := apply(factoryDir, &factorydefinitions.FactoryConfig{}, false, false); !errors.Is(err, walkErr) {
			t.Fatalf("apply error = %v, want injected walk error", err)
		}
	})

	t.Run("starter Work", func(t *testing.T) {
		factoryDir := t.TempDir()
		mustMakePortableDirectory(t, filepath.Join(factoryDir, factorydefinitions.InputsDir))
		apply, err := NewFactoryStarterWorkApplier(fileSystem)
		if err != nil {
			t.Fatalf("construct operation: %v", err)
		}
		if err := apply(factoryDir, &factorydefinitions.FactoryConfig{}); !errors.Is(err, walkErr) {
			t.Fatalf("apply error = %v, want injected walk error", err)
		}
	})

	t.Run("document pruning", func(t *testing.T) {
		factoryDir := filepath.Join(t.TempDir(), "factory")
		mustMakePortableDirectory(t, filepath.Join(factoryDir, "docs"))
		prune, err := NewPortableBundledDocsPruner(fileSystem)
		if err != nil {
			t.Fatalf("construct operation: %v", err)
		}
		if err := prune(factoryDir, &factorydefinitions.FactoryConfig{}); !errors.Is(err, walkErr) {
			t.Fatalf("prune error = %v, want injected walk error", err)
		}
	})
}

func mustMakePortableDirectory(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("create portable directory: %v", err)
	}
}
