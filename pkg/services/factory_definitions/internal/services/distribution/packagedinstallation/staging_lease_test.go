package packagedinstallation

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

const stagingLeaseObservationTimeout = 30 * time.Second

// TestInstallPackagedFactory_PublishedStagingLeaseAlwaysNamesItsOwner parks the
// installer at the two moments a staging lease could become visible without an
// owner record - directory creation during acquisition, and the recursive
// delete during release - and proves a concurrent observer either sees no
// staging resource at all or sees one that reports a concrete owner PID.
//
// The observation is repeated across a release-then-reacquire cycle by the same
// owner, which is the sequence the CLI successor path exercises and the one
// that produced the unattributable owner_pid=unavailable diagnostic.
func TestInstallPackagedFactory_PublishedStagingLeaseAlwaysNamesItsOwner(t *testing.T) {
	root := t.TempDir()
	name := "@test/lease-identity"
	gate := newStagingLeaseGate()
	seam := &yieldingPackagedInstallationFileSystem{gate: gate}
	installer := New(&successfulPackagedInstallationPersistence{}, seam, seam.Mkdir)
	observer := New(&successfulPackagedInstallationPersistence{}, platformfilesystem.Local{}, os.Mkdir)
	params := factorydefinitions.PackagedFactoryInstallParams{
		NamedFactoriesRoot: root,
		BackendScopeID:     "lease-identity-scope",
		Definition: factorydefinitions.PackagedDefinition{
			Name: name,
			JSON: []byte(`{}`),
		},
		Format: factorydefinitions.PackagedFactoryFormatJSON,
	}

	installDone := make(chan error, 1)
	go func() {
		// Two installations in sequence acquire, release, then reacquire the
		// same canonical lease path with the same owning process.
		for attempt := 0; attempt < 2; attempt++ {
			if _, err := installer.InstallPackagedFactory(t.Context(), params); err != nil {
				installDone <- fmt.Errorf("install attempt %d: %w", attempt+1, err)
				return
			}
		}
		installDone <- nil
	}()

	acquisitionYields, releaseYields := 0, 0
	for done := false; !done; {
		select {
		case parked := <-gate.reached:
			if strings.HasPrefix(parked, "mkdir:") {
				acquisitionYields++
			} else {
				releaseYields++
			}
			observeStagingLeaseIdentity(t, observer, root, name, parked)
			gate.release <- struct{}{}
		case err := <-installDone:
			if err != nil {
				t.Fatalf("InstallPackagedFactory() error = %v", err)
			}
			done = true
		case <-time.After(stagingLeaseObservationTimeout):
			t.Fatalf("timed out waiting for staging lease observation; acquisitions=%d releases=%d", acquisitionYields, releaseYields)
		}
	}

	if acquisitionYields < 2 {
		t.Errorf("observed %d lease acquisition windows, want the acquire and reacquire of a release-then-reacquire cycle", acquisitionYields)
	}
	if releaseYields < 2 {
		t.Errorf("observed %d lease release windows, want one per installation", releaseYields)
	}
	if _, err := os.Stat(stagingOwnershipPath(root, name)); !os.IsNotExist(err) {
		t.Errorf("staging lease stat error = %v, want the lease released", err)
	}
	assertNoStagingLeaseResidue(t, root)
}

// observeStagingLeaseIdentity is the concurrent observer. It runs the package's
// own staging inspection path while the installer is parked, and holds the
// invariant the diagnostic depends on: a discoverable staging resource always
// reports an owner PID a remediation can act on.
func observeStagingLeaseIdentity(t *testing.T, observer *Service, root, name, parked string) {
	t.Helper()
	stagingPath, err := findPreExistingStaging(platformfilesystem.Local{}, root, name)
	if err != nil {
		t.Errorf("findPreExistingStaging() while parked at %s error = %v", parked, err)
		return
	}
	if stagingPath == "" {
		// No lease is observable at all, which is the other legal outcome.
		return
	}
	if stagingPath != stagingOwnershipPath(root, name) {
		t.Errorf(
			"staging lookup while parked at %s discovered %q, want only the canonical lease path %q; a private lease path must never be discoverable",
			parked,
			stagingPath,
			stagingOwnershipPath(root, name),
		)
		return
	}
	contention, retry, recovered, inspectErr := observer.inspectStagingContention(
		"observer-scope",
		root,
		name,
		stagingPath,
	)
	if inspectErr != nil {
		t.Errorf("inspectStagingContention() while parked at %s error = %v", parked, inspectErr)
		return
	}
	if recovered != nil {
		t.Errorf("observer reclaimed the live lease at %s while parked at %s", recovered.path, parked)
		return
	}
	if retry {
		// The lease was released between lookup and inspection, so nothing was
		// observable by the time the owner record was read.
		return
	}
	if contention == nil {
		t.Errorf("inspectStagingContention() while parked at %s returned no diagnostic for a visible lease", parked)
		return
	}
	message := contention.Error()
	if strings.Contains(message, "owner_pid=unavailable") {
		t.Errorf("observer saw a published lease with no readable owner while parked at %s: %s", parked, message)
		return
	}
	if want := fmt.Sprintf("owner_pid=%d", os.Getpid()); !strings.Contains(message, want) {
		t.Errorf("staging diagnostic while parked at %s = %q, want %q", parked, message, want)
	}
}

// assertNoStagingLeaseResidue proves the private paths used to build and
// withdraw a lease are always cleaned up, so a completed installation leaves
// the Factory root exactly as it found it.
func assertNoStagingLeaseResidue(t *testing.T, root string) {
	t.Helper()
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("read Factory root: %v", err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), stagingLeasePendingPrefix) ||
			strings.HasPrefix(entry.Name(), stagingLeaseRetractedPrefix) {
			t.Errorf("private staging lease residue %q was left behind", entry.Name())
		}
	}
}

// TestPrivateStagingLeasePathsAreNotDiscoverableStagingResources proves the
// private build and withdraw paths cannot be mistaken for a lease. They live in
// the same directory the staging lookup scans, so a name that matched the
// staging prefix would reintroduce the unidentified-lease diagnostic through a
// different door.
func TestPrivateStagingLeasePathsAreNotDiscoverableStagingResources(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{
		"@test/private-paths",
		"you-packaged-lease-pending",
		"you-packaged-lease-retracted",
		"",
	} {
		for _, prefix := range []string{stagingLeasePendingPrefix, stagingLeaseRetractedPrefix} {
			privatePath := privateStagingLeasePath(root, prefix)
			if err := os.Mkdir(privatePath, 0o755); err != nil {
				t.Fatalf("create private lease path: %v", err)
			}
			discovered, err := findPreExistingStaging(platformfilesystem.Local{}, root, name)
			if err != nil {
				t.Fatalf("findPreExistingStaging(%q) error = %v", name, err)
			}
			if discovered != "" {
				t.Errorf(
					"findPreExistingStaging(%q) = %q with private path %q present, want no staging resource",
					name,
					discovered,
					filepath.Base(privatePath),
				)
			}
			if err := os.RemoveAll(privatePath); err != nil {
				t.Fatalf("remove private lease path: %v", err)
			}
		}
	}
}

// TestPublishStagingLeaseTreatsAnOccupiedDestinationAsALostRace proves the
// lost-race signal does not depend on one platform's rename error. A published
// lease is a non-empty directory, so the losing rename reports fs.ErrExist on
// some platforms and a directory-not-empty or access error on others.
func TestPublishStagingLeaseTreatsAnOccupiedDestinationAsALostRace(t *testing.T) {
	root := t.TempDir()
	service := New(&successfulPackagedInstallationPersistence{}, platformfilesystem.Local{}, os.Mkdir)
	leasePath := stagingOwnershipPath(root, "@test/occupied")
	if err := os.MkdirAll(leasePath, 0o755); err != nil {
		t.Fatalf("create winner lease: %v", err)
	}
	if err := os.WriteFile(filepath.Join(leasePath, stagingOwnerMetadataName), []byte(`{"pid":4242}`), 0o600); err != nil {
		t.Fatalf("write winner owner metadata: %v", err)
	}
	pendingPath, err := service.stageStagingLease(root, ownerRecord{PID: os.Getpid()})
	if err != nil {
		t.Fatalf("stageStagingLease() error = %v", err)
	}

	lost, err := service.publishStagingLease(pendingPath, leasePath)
	if err != nil {
		t.Fatalf("publishStagingLease() error = %v, want a clean lost race", err)
	}
	if !lost {
		t.Fatal("publishStagingLease() lost = false, want the occupied destination reported as a lost race")
	}
	if _, statErr := os.Stat(pendingPath); !os.IsNotExist(statErr) {
		t.Errorf("pending lease stat error = %v, want the loser's private lease removed", statErr)
	}
	owner, _, readErr := service.readOwnerRecord(leasePath)
	if readErr != nil {
		t.Fatalf("readOwnerRecord() error = %v, want the winner lease preserved", readErr)
	}
	if owner.PID != 4242 {
		t.Errorf("winner owner PID = %d, want 4242", owner.PID)
	}
}

// TestPublishStagingLeaseReportsAFailureWhenTheDestinationIsAbsent proves a
// rename failure that is not a lost race still surfaces as a bounded failure
// rather than being retried forever.
func TestPublishStagingLeaseReportsAFailureWhenTheDestinationIsAbsent(t *testing.T) {
	root := t.TempDir()
	fileSystem := &failingPackagedInstallationFileSystem{}
	service := New(&successfulPackagedInstallationPersistence{}, fileSystem, fileSystem.Mkdir)
	pendingPath := filepath.Join(root, "pending-lease")
	if err := os.MkdirAll(pendingPath, 0o755); err != nil {
		t.Fatalf("create pending lease: %v", err)
	}
	fileSystem.renameErr = fmt.Errorf("rename refused")

	lost, err := service.publishStagingLease(pendingPath, stagingOwnershipPath(root, "@test/absent"))
	if lost {
		t.Fatal("publishStagingLease() lost = true, want a bounded failure when nothing occupies the destination")
	}
	if err == nil || !strings.Contains(err.Error(), "rename refused") {
		t.Fatalf("publishStagingLease() error = %v, want the rename failure reported", err)
	}
}

type stagingLeaseGate struct {
	reached chan string
	release chan struct{}
}

func newStagingLeaseGate() *stagingLeaseGate {
	return &stagingLeaseGate{reached: make(chan string), release: make(chan struct{})}
}

func (gate *stagingLeaseGate) yield(parked string) {
	gate.reached <- parked
	<-gate.release
}

// yieldingPackagedInstallationFileSystem parks the installer at each moment a
// staging lease could become observable without its owner record.
type yieldingPackagedInstallationFileSystem struct {
	platformfilesystem.Local
	gate *stagingLeaseGate
}

// Mkdir yields after the directory exists, which is the exact window in which a
// lease published before its owner record would be visible to an observer.
func (fileSystem *yieldingPackagedInstallationFileSystem) Mkdir(path string, mode fs.FileMode) error {
	if err := os.Mkdir(path, mode); err != nil {
		return err
	}
	fileSystem.gate.yield("mkdir:" + path)
	return nil
}

// RemoveAll reproduces what a real recursive delete does to a lease directory:
// the owner record is unlinked before the directory that holds it. Yielding in
// between exposes any lease that is removed in place instead of being renamed
// out of the observable path first.
func (fileSystem *yieldingPackagedInstallationFileSystem) RemoveAll(path string) error {
	metadataPath := filepath.Join(path, stagingOwnerMetadataName)
	if _, err := os.Stat(metadataPath); err == nil {
		if err := os.Remove(metadataPath); err != nil {
			return err
		}
		fileSystem.gate.yield("removeall:" + path)
	}
	return fileSystem.Local.RemoveAll(path)
}
