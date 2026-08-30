//go:build !windows

package ownershipinventory

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOwnershipSnapshotGroupRejectsIntermediateSymlinkBeforeOutsideMutation(t *testing.T) {
	fixture := newSnapshotTransactionFixture(t)
	root := t.TempDir()
	outside := t.TempDir()
	outsideTarget := filepath.Join(outside, "internal", "baselines", "ownership-inventory.json")
	if err := os.MkdirAll(filepath.Dir(outsideTarget), 0o755); err != nil {
		t.Fatalf("mkdir outside target: %v", err)
	}
	sentinel := []byte("outside-sentinel\n")
	if err := os.WriteFile(outsideTarget, sentinel, 0o600); err != nil {
		t.Fatalf("write outside target: %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "docs")); err != nil {
		t.Fatalf("create intermediate symlink: %v", err)
	}

	err := fixture.publish(t, osFileSystem{}, root)
	if err == nil || !strings.Contains(err.Error(), "destination directory component") || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("publishSnapshotGroup() error = %v, want intermediate symlink diagnostic", err)
	}
	got, err := os.ReadFile(outsideTarget)
	if err != nil {
		t.Fatalf("read outside target: %v", err)
	}
	if !bytes.Equal(got, sentinel) {
		t.Fatalf("outside target changed to %q, want %q", got, sentinel)
	}
	assertNoSnapshotTransactionResidue(t, root)
	assertNoSnapshotTransactionResidue(t, outside)
}
