package packagedinstallation

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync/atomic"
)

const (
	// stagingLeasePendingPrefix and stagingLeaseRetractedPrefix name the private
	// directories that hold a staging lease before it is published and after it
	// is withdrawn.
	//
	// Neither prefix can be discovered by findPreExistingStaging. A staging
	// directory prefix is always "." + <factory name> + ".staging-", which
	// contains at least two "." characters, while these names contain exactly
	// one. A private lease directory therefore cannot be mistaken for a lease no
	// matter which Factory name is being installed.
	stagingLeasePendingPrefix   = ".you-packaged-lease-pending-"
	stagingLeaseRetractedPrefix = ".you-packaged-lease-retracted-"
)

var stagingLeaseSequence uint64

// privateStagingLeasePath returns a unique path inside rootDir that no staging
// lookup can discover. Keeping it inside rootDir keeps publication a
// same-filesystem rename, which is the atomicity the lease depends on.
func privateStagingLeasePath(rootDir, prefix string) string {
	sequence := atomic.AddUint64(&stagingLeaseSequence, 1)
	return filepath.Join(rootDir, fmt.Sprintf("%s%d-%d", prefix, os.Getpid(), sequence))
}

// identifyStagingOwner resolves the owning process before any directory exists,
// so a failed or invalid probe leaves no residue behind.
func (service *Service) identifyStagingOwner() (ownerRecord, error) {
	owner, err := service.ownerProbe.Current()
	if err != nil {
		return ownerRecord{}, err
	}
	if owner.PID <= 0 {
		return ownerRecord{}, fmt.Errorf("owner PID is invalid: %d", owner.PID)
	}
	return owner, nil
}

// stageStagingLease builds the complete lease - directory plus owner record - at
// a private path. Nothing observable is created here, so the lease can never be
// seen while it is still half built.
func (service *Service) stageStagingLease(rootDir string, owner ownerRecord) (string, error) {
	pendingPath := privateStagingLeasePath(rootDir, stagingLeasePendingPrefix)
	if err := service.directoryCreator(pendingPath, 0o755); err != nil {
		return "", err
	}
	if err := service.publishOwnerRecord(pendingPath, owner); err != nil {
		_ = service.fileSystem.RemoveAll(pendingPath)
		return "", err
	}
	return pendingPath, nil
}

// publishStagingLease moves the fully identified private lease onto the
// canonical staging ownership path. This rename is the single step that makes
// the lease externally observable, so any observer that can see the lease
// directory can also read its owner record.
//
// It reports whether another process published first. That lost race keeps the
// behavior the exclusive directory reservation had: the loser discards its own
// private lease and falls through to the existing staging contention inspection
// and retry. A published lease is a non-empty directory, so a losing rename
// surfaces as fs.ErrExist on some platforms and as a directory-not-empty or
// access error on others; the presence of the destination is the portable
// lost-race signal.
func (service *Service) publishStagingLease(pendingPath, leasePath string) (bool, error) {
	err := service.fileSystem.Rename(pendingPath, leasePath)
	if err == nil {
		return false, nil
	}
	_ = service.fileSystem.RemoveAll(pendingPath)
	if errors.Is(err, fs.ErrExist) || service.stagingLeaseIsPublished(leasePath) {
		return true, nil
	}
	return false, fmt.Errorf("publish staging owner lease: %w", err)
}

func (service *Service) stagingLeaseIsPublished(leasePath string) bool {
	_, err := service.fileSystem.Stat(leasePath)
	return err == nil
}

// retractStagingLease withdraws a published lease. The lease is renamed out of
// the observable path first because a recursive delete unlinks the owner record
// before the directory that holds it, which would expose exactly the
// published-but-unidentified lease that publication is careful never to create.
// When the platform refuses the rename the recursive delete still runs against
// the lease directly, so a release that would otherwise succeed is never turned
// into a failure.
func (service *Service) retractStagingLease(rootDir, leasePath string) error {
	retractedPath := privateStagingLeasePath(rootDir, stagingLeaseRetractedPrefix)
	if err := service.fileSystem.Rename(leasePath, retractedPath); err != nil {
		return service.fileSystem.RemoveAll(leasePath)
	}
	return service.fileSystem.RemoveAll(retractedPath)
}

// publishOwnedStagingLease identifies the owning process first, then builds the
// lease privately and publishes it with one atomic rename. Ordering the owner
// record ahead of publication is what guarantees that a lease visible at the
// canonical staging path always reports a concrete owner PID instead of the
// unattributable owner_pid=unavailable diagnostic a half-built lease produced.
//
// It reports lost=true when another process published first, which routes the
// caller back through the existing staging contention inspection and retry.
func (service *Service) publishOwnedStagingLease(
	backendScopeID string,
	rootDir string,
	name string,
) (*stagingLease, bool, error) {
	if err := service.fileSystem.MkdirAll(rootDir, 0o755); err != nil {
		return nil, false, service.installationFailure(backendScopeID, rootDir, name, "", ownerLivenessIndeterminate, err)
	}
	leasePath := stagingOwnershipPath(rootDir, name)
	owner, err := service.identifyStagingOwner()
	if err != nil {
		return nil, false, service.installationFailure(backendScopeID, rootDir, name, leasePath, ownerLivenessIndeterminate, err)
	}
	pendingPath, err := service.stageStagingLease(rootDir, owner)
	if err != nil {
		return nil, false, service.installationFailure(backendScopeID, rootDir, name, leasePath, ownerLivenessIndeterminate, err)
	}
	lost, err := service.publishStagingLease(pendingPath, leasePath)
	if err != nil {
		return nil, false, service.installationFailure(backendScopeID, rootDir, name, leasePath, ownerLivenessIndeterminate, err)
	}
	if lost {
		return nil, true, nil
	}
	service.logOutcome(
		backendScopeID,
		name,
		leasePath,
		"acquired",
		ownerLivenessActive,
		owner.PID,
	)
	return &stagingLease{
		path:           leasePath,
		root:           rootDir,
		name:           name,
		backendScopeID: backendScopeID,
		owner:          owner,
	}, false, nil
}
