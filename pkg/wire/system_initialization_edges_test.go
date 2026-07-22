package wire

import (
	"errors"
	"io/fs"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
)

func TestSystemInitializationInspectPathPreservesOverrideAndSelectsProcessDefault(t *testing.T) {
	t.Parallel()

	path := t.TempDir()
	info, err := provideSystemInitializationInspectPath(serviceedges.Edges{})(path)
	if err != nil {
		t.Fatalf("default inspect path: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("default inspect path IsDir() = false for %q", path)
	}

	inspected := ""
	override := func(path string) (fs.FileInfo, error) {
		inspected = path
		return nil, fs.ErrPermission
	}
	_, err = provideSystemInitializationInspectPath(serviceedges.Edges{
		SystemInitializationInspectPath: override,
	})("customer-path")
	if !errors.Is(err, fs.ErrPermission) || inspected != "customer-path" {
		t.Fatalf("override inspect path = (%q, %v), want customer-path and permission error", inspected, err)
	}
}
