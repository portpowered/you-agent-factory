package packagedinstallation

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"sync/atomic"
)

var stagingReclaimSequence uint64

func (service *Service) reclaimOrphanedStaging(
	backendScopeID string,
	rootDir string,
	name string,
	stagingPath string,
	expected ownerRecord,
) (*stagingLease, error) {
	current, liveness, err := service.readOwnerRecord(stagingPath)
	if err != nil {
		return nil, service.installationContention(
			backendScopeID,
			rootDir,
			name,
			stagingPath,
			"indeterminate-contention",
			liveness,
			current.PID,
		)
	}
	if current != expected || liveness != ownerLivenessOrphaned {
		return nil, service.installationContention(
			backendScopeID,
			rootDir,
			name,
			stagingPath,
			"indeterminate-contention",
			ownerLivenessRacing,
			current.PID,
		)
	}
	recoveredLeasePath, err := service.claimOrphanedStaging(
		backendScopeID,
		rootDir,
		name,
		stagingPath,
		expected,
	)
	if err != nil {
		return nil, err
	}
	return service.publishRecoveredStagingLease(
		backendScopeID,
		rootDir,
		name,
		recoveredLeasePath,
		stagingPath,
		expected,
	)
}

func (service *Service) claimOrphanedStaging(
	backendScopeID string,
	rootDir string,
	name string,
	stagingPath string,
	expected ownerRecord,
) (string, error) {
	sequence := atomic.AddUint64(&stagingReclaimSequence, 1)
	// Renaming the exact lease directory is the atomic claim. The winner moves
	// it to a distinct recovery path, so a delayed reclaimer cannot remove a
	// replacement lease at the deterministic path.
	recoveredLeasePath := fmt.Sprintf("%s-recovered-%d-%d", stagingPath, os.Getpid(), sequence)
	if err := service.fileSystem.Rename(stagingPath, recoveredLeasePath); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", service.installationContention(
				backendScopeID,
				rootDir,
				name,
				stagingPath,
				"indeterminate-contention",
				ownerLivenessRacing,
				expected.PID,
			)
		}
		return "", service.installationFailure(
			backendScopeID,
			rootDir,
			name,
			stagingPath,
			ownerLivenessOrphaned,
			err,
		)
	}
	return recoveredLeasePath, nil
}

func (service *Service) publishRecoveredStagingLease(
	backendScopeID string,
	rootDir string,
	name string,
	recoveredLeasePath string,
	stagingPath string,
	expected ownerRecord,
) (*stagingLease, error) {
	owner, err := service.ownerProbe.Current()
	if err != nil {
		return nil, service.installationFailure(
			backendScopeID,
			rootDir,
			name,
			recoveredLeasePath,
			ownerLivenessIndeterminate,
			err,
		)
	}
	if owner.PID <= 0 {
		return nil, service.installationFailure(
			backendScopeID,
			rootDir,
			name,
			recoveredLeasePath,
			ownerLivenessIndeterminate,
			fmt.Errorf("owner PID is invalid: %d", owner.PID),
		)
	}
	if err := service.publishOwnerRecord(recoveredLeasePath, owner); err != nil {
		return nil, service.installationFailure(
			backendScopeID,
			rootDir,
			name,
			recoveredLeasePath,
			ownerLivenessIndeterminate,
			err,
		)
	}
	service.logOutcome(
		backendScopeID,
		name,
		stagingPath,
		"reclaimed-orphan",
		ownerLivenessOrphaned,
		expected.PID,
	)
	service.logOutcome(
		backendScopeID,
		name,
		recoveredLeasePath,
		"acquired",
		ownerLivenessActive,
		owner.PID,
	)
	return &stagingLease{
		path:           recoveredLeasePath,
		root:           rootDir,
		name:           name,
		backendScopeID: backendScopeID,
		owner:          owner,
	}, nil
}
