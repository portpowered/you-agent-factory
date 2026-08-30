//go:build !windows

package ownershipinventory

import (
	"syscall"
	"testing"
)

func TestOwnershipSnapshotGroupFallbackRestoresExactModeAfterUmask(t *testing.T) {
	previousUmask := syscall.Umask(0o077)
	t.Cleanup(func() { syscall.Umask(previousUmask) })

	fixture := newSnapshotTransactionFixture(t)
	root := t.TempDir()
	initial := seedSnapshotTransactionTargets(t, root, fixture.writes, func(int) bool { return true })
	failedPosition := 1
	files := &controlledSnapshotFileSystem{
		failInstallAt:          failedPosition + 1,
		failRollbackRenameOnce: true,
	}

	if err := fixture.publish(t, files, root); err == nil {
		t.Fatal("publishSnapshotGroup() error = nil, want forced replacement failure")
	}
	assertSnapshotTransactionStates(t, root, fixture.writes, initial)
	assertNoSnapshotTransactionResidue(t, root)
}
