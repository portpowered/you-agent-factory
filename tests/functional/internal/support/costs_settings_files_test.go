package support

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
)

// Component proof of the fault edge only: no root process or customer journey.
func TestCostsSettingsReadFailureIsDocumentScopedAndReversible(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	selected, peer := filepath.Join(dir, "selected.json"), filepath.Join(dir, "peer.json")
	for _, path := range []string{selected, peer} {
		if err := os.WriteFile(path, []byte("readable settings"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	files := NewCostsSettingsFiles(platformfilesystem.Local{})
	cause := errors.New("controlled unavailable settings")
	files.SetReadFailure(selected, cause)
	if data, err := files.ReadFile(selected); !errors.Is(err, cause) || data != nil {
		t.Fatalf("selected read = %q/%v, want injected cause without data", data, err)
	}
	if data, err := files.ReadFile(peer); err != nil || string(data) != "readable settings" {
		t.Fatalf("peer read = %q/%v, want delegated success", data, err)
	}
	files.SetReadFailure(selected, nil)
	if data, err := files.ReadFile(selected); err != nil || string(data) != "readable settings" {
		t.Fatalf("recovered read = %q/%v, want delegated success", data, err)
	}
}
