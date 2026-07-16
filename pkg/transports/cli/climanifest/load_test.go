package climanifest_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifest"
)

func TestLoadProduction_ProductionManifest(t *testing.T) {
	path := testutil.MustRepoPath(t, climanifest.ProductionManifestPath)
	manifest, err := climanifest.LoadProduction(path)
	if err != nil {
		t.Fatalf("LoadProduction() error = %v", err)
	}
	if manifest.RootPath != "you" {
		t.Fatalf("RootPath = %q, want you", manifest.RootPath)
	}
	if manifest.FormatVersion == "" {
		t.Fatal("FormatVersion is empty")
	}

	root, err := manifest.CommandByID("you")
	if err != nil {
		t.Fatalf("CommandByID(you) error = %v", err)
	}
	if root.ID != "you" {
		t.Fatalf("root ID = %q, want you", root.ID)
	}
}

func TestLoadProduction_Errors(t *testing.T) {
	t.Run("missing file", func(t *testing.T) {
		_, err := climanifest.LoadProduction(filepath.Join(t.TempDir(), "missing.json"))
		if err == nil {
			t.Fatal("LoadProduction() error = nil, want read failure")
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "commands.json")
		if err := os.WriteFile(path, []byte("{"), 0o644); err != nil {
			t.Fatalf("write manifest: %v", err)
		}
		_, err := climanifest.LoadProduction(path)
		if err == nil {
			t.Fatal("LoadProduction() error = nil, want decode failure")
		}
	})

	t.Run("missing rootPath", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "commands.json")
		if err := os.WriteFile(path, []byte(`{"commands":{"you":{"id":"you"}}}`), 0o644); err != nil {
			t.Fatalf("write manifest: %v", err)
		}
		_, err := climanifest.LoadProduction(path)
		if err == nil {
			t.Fatal("LoadProduction() error = nil, want rootPath failure")
		}
	})

	t.Run("missing commands", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "commands.json")
		if err := os.WriteFile(path, []byte(`{"rootPath":"you"}`), 0o644); err != nil {
			t.Fatalf("write manifest: %v", err)
		}
		_, err := climanifest.LoadProduction(path)
		if err == nil {
			t.Fatal("LoadProduction() error = nil, want commands failure")
		}
	})
}

func TestManifest_CommandByID_Missing(t *testing.T) {
	manifest := climanifest.Manifest{Commands: map[string]climanifest.Command{}}
	_, err := manifest.CommandByID("missing")
	if err == nil {
		t.Fatal("CommandByID() error = nil, want missing command failure")
	}
}
