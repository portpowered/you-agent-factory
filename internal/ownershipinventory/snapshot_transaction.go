package ownershipinventory

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
)

const (
	snapshotTargetMode    fs.FileMode = 0o644
	snapshotStagePattern              = ".ownership-snapshot-stage-*"
	snapshotBackupPattern             = ".ownership-snapshot-backup-*"
)

const ownershipSnapshotWriteCount = 6

// PublishSnapshotGroup validates, serializes, stages, and commits the six
// ownership snapshots as one handled-error transaction.
func PublishSnapshotGroup(
	root string,
	packages []string,
	inventory Inventory,
	freeze PathLeaseFreeze,
	candidates SnapshotCandidates,
) error {
	return publishSnapshotGroup(osFileSystem{}, root, packages, inventory, freeze, candidates)
}

func publishSnapshotGroup(
	files fileSystem,
	root string,
	packages []string,
	inventory Inventory,
	freeze PathLeaseFreeze,
	candidates SnapshotCandidates,
) error {
	writes, err := snapshotGroupWrites(packages, inventory, freeze, candidates)
	if err != nil {
		return fmt.Errorf("prepare snapshot group: %w", err)
	}
	return publishSnapshotWrites(files, root, writes)
}

func snapshotGroupWrites(
	packages []string,
	inventory Inventory,
	freeze PathLeaseFreeze,
	candidates SnapshotCandidates,
) ([]snapshotWrite, error) {
	if report := ValidateInventory(inventory, packages); !report.OK() {
		return nil, fmt.Errorf("validate S-03 ownership inventory: %d violations: %#v", report.ViolationCount(), report)
	}
	if report := ValidatePathLeaseFreeze(freeze); !report.OK() {
		return nil, fmt.Errorf("validate S-04 path-lease freeze: %d violations: %#v", report.ViolationCount(), report)
	}

	inventoryPayload, err := marshalInventory(inventory)
	if err != nil {
		return nil, err
	}
	freezePayload, err := marshalPathLeaseFreeze(freeze)
	if err != nil {
		return nil, err
	}
	candidateWrites, err := snapshotWrites(candidates)
	if err != nil {
		return nil, err
	}
	writes := append([]snapshotWrite{
		{relativePath: InventoryRelativePath, payload: inventoryPayload},
		{relativePath: PathLeaseFreezeRelativePath, payload: freezePayload},
	}, candidateWrites...)
	if err := validateSnapshotWriteSet(writes); err != nil {
		return nil, err
	}
	return writes, nil
}

func marshalInventory(inventory Inventory) ([]byte, error) {
	payload, err := marshalJSON("ownership inventory", inventory)
	if err != nil {
		return nil, err
	}
	return payload, nil
}

func marshalPathLeaseFreeze(freeze PathLeaseFreeze) ([]byte, error) {
	return marshalJSON("path-lease freeze", freeze)
}

func marshalJSON(label string, value any) ([]byte, error) {
	return marshalSnapshot(label, value, func() error { return nil })
}

func validateSnapshotWriteSet(writes []snapshotWrite) error {
	if len(writes) != ownershipSnapshotWriteCount {
		return fmt.Errorf("snapshot write group has %d entries, want %d", len(writes), ownershipSnapshotWriteCount)
	}
	for index, write := range writes {
		wantPath := ownershipSnapshotRelativePath(index)
		if write.relativePath != wantPath {
			return fmt.Errorf("snapshot write group entry %d path %q, want %q", index+1, write.relativePath, wantPath)
		}
		if len(write.payload) == 0 {
			return fmt.Errorf("snapshot write group entry %d path %q has empty payload", index+1, write.relativePath)
		}
	}
	return nil
}

func ownershipSnapshotRelativePath(index int) string {
	switch index {
	case 0:
		return InventoryRelativePath
	case 1:
		return PathLeaseFreezeRelativePath
	case 2:
		return OperatorSettingsRootGoInventoryRelativePath
	case 3:
		return OperatorSettingsTopLevelInventoryRelativePath
	case 4:
		return ProviderSessionsRootGoInventoryRelativePath
	case 5:
		return ProviderSessionsTopLevelInventoryRelativePath
	default:
		return ""
	}
}

type snapshotTarget struct {
	write            snapshotWrite
	path             string
	original         snapshotOriginal
	stagePath        string
	backupPath       string
	backupMoved      bool
	targetTouched    bool
	originalRestored bool
}

type snapshotOriginal struct {
	exists  bool
	payload []byte
	mode    fs.FileMode
}

func publishSnapshotWrites(files fileSystem, root string, writes []snapshotWrite) error {
	if err := validateSnapshotWriteSet(writes); err != nil {
		return fmt.Errorf("validate snapshot write group: %w", err)
	}
	targets, err := preflightSnapshotTargets(files, root, writes)
	if err != nil {
		return err
	}
	if err := stageSnapshotTargets(files, targets); err != nil {
		cleanupErr := cleanupStagedArtifacts(files, targets)
		return errors.Join(err, cleanupErr)
	}
	if err := commitSnapshotTargets(files, targets); err != nil {
		rollbackErr := rollbackSnapshotTargets(files, targets)
		cleanupErr := cleanupFailedTransaction(files, targets)
		return errors.Join(err, rollbackErr, cleanupErr)
	}
	return cleanupCommittedTransaction(files, targets)
}

func preflightSnapshotTargets(files fileSystem, root string, writes []snapshotWrite) ([]snapshotTarget, error) {
	targets := make([]snapshotTarget, 0, len(writes))
	for index, write := range writes {
		path := filepath.Join(root, filepath.FromSlash(write.relativePath))
		if err := files.mkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, fmt.Errorf("preflight position %d (%s): create destination directory %q: %w", index+1, write.relativePath, filepath.Dir(path), err)
		}
		original, err := captureSnapshotOriginal(files, path)
		if err != nil {
			return nil, fmt.Errorf("preflight position %d (%s): %w", index+1, write.relativePath, err)
		}
		targets = append(targets, snapshotTarget{write: write, path: path, original: original})
	}
	return targets, nil
}

func captureSnapshotOriginal(files fileSystem, path string) (snapshotOriginal, error) {
	info, err := files.lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return snapshotOriginal{mode: snapshotTargetMode}, nil
	}
	if err != nil {
		return snapshotOriginal{}, fmt.Errorf("inspect target %q: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return snapshotOriginal{}, fmt.Errorf("target %q is non-regular (%s)", path, snapshotEntryKind(info))
	}
	payload, err := files.readFile(path)
	if err != nil {
		return snapshotOriginal{}, fmt.Errorf("read original target %q: %w", path, err)
	}
	return snapshotOriginal{exists: true, payload: payload, mode: info.Mode().Perm()}, nil
}

func snapshotEntryKind(info fs.FileInfo) string {
	if info.IsDir() {
		return "directory"
	}
	if info.Mode()&fs.ModeSymlink != 0 {
		return "symlink"
	}
	return info.Mode().String()
}

func stageSnapshotTargets(files fileSystem, targets []snapshotTarget) error {
	for index := range targets {
		path, err := stageSnapshotFile(files, targets[index].path, targets[index].write.payload, targets[index].original.mode)
		targets[index].stagePath = path
		if err != nil {
			return fmt.Errorf("stage position %d (%s): %w", index+1, targets[index].write.relativePath, err)
		}
	}
	return nil
}

func stageSnapshotFile(files fileSystem, targetPath string, payload []byte, mode fs.FileMode) (string, error) {
	directory := filepath.Dir(targetPath)
	stage, err := files.createTemp(directory, snapshotStagePattern)
	if err != nil {
		return "", fmt.Errorf("create destination-local stage beside %q: %w", targetPath, err)
	}
	stagePath := stage.Name()
	if stagePath == "" {
		return stagePath, errors.Join(
			fmt.Errorf("create destination-local stage beside %q: empty stage name", targetPath),
			stage.Close(),
		)
	}
	var operationErr error
	written, writeErr := stage.Write(payload)
	if writeErr != nil {
		operationErr = fmt.Errorf("write destination-local stage %q: %w", stagePath, writeErr)
	} else if written != len(payload) {
		operationErr = fmt.Errorf("write destination-local stage %q: %w (wrote %d of %d bytes)", stagePath, fs.ErrInvalid, written, len(payload))
	} else if err := stage.Chmod(mode); err != nil {
		operationErr = fmt.Errorf("set destination-local stage mode %q: %w", stagePath, err)
	}
	closeErr := stage.Close()
	if closeErr != nil {
		operationErr = errors.Join(operationErr, fmt.Errorf("close destination-local stage %q: %w", stagePath, closeErr))
	}
	return stagePath, operationErr
}

func commitSnapshotTargets(files fileSystem, targets []snapshotTarget) error {
	for index := range targets {
		target := &targets[index]
		if target.original.exists {
			backupPath, err := reserveSnapshotBackup(files, target.path)
			target.backupPath = backupPath
			if err != nil {
				return replacementError(index, target, "prepare original backup", err)
			}
			if err := files.rename(target.path, target.backupPath); err != nil {
				return replacementError(index, target, "move original to backup", err)
			}
			target.backupMoved = true
		}
		target.targetTouched = true
		if err := files.rename(target.stagePath, target.path); err != nil {
			return replacementError(index, target, "install staged snapshot", err)
		}
	}
	return nil
}

func reserveSnapshotBackup(files fileSystem, targetPath string) (string, error) {
	backup, err := files.createTemp(filepath.Dir(targetPath), snapshotBackupPattern)
	if err != nil {
		return "", fmt.Errorf("create destination-local backup beside %q: %w", targetPath, err)
	}
	backupPath := backup.Name()
	if backupPath == "" {
		return backupPath, errors.Join(
			fmt.Errorf("create destination-local backup beside %q: empty backup name", targetPath),
			backup.Close(),
		)
	}
	closeErr := backup.Close()
	removeErr := removeIfAbsent(files, backupPath)
	if closeErr != nil || removeErr != nil {
		return backupPath, errors.Join(
			wrapOptionalError("close destination-local backup", backupPath, closeErr),
			wrapOptionalError("remove backup placeholder", backupPath, removeErr),
		)
	}
	return backupPath, nil
}

func replacementError(index int, target *snapshotTarget, operation string, err error) error {
	return fmt.Errorf("replacement position %d (%s) %s %q: %w", index+1, target.write.relativePath, operation, target.path, err)
}

func rollbackSnapshotTargets(files fileSystem, targets []snapshotTarget) error {
	var rollbackErr error
	for index := len(targets) - 1; index >= 0; index-- {
		target := &targets[index]
		var err error
		if target.original.exists && target.backupMoved {
			err = rollbackExistingTarget(files, target)
		} else if !target.original.exists && target.targetTouched {
			err = removeCreatedTarget(files, target)
		}
		if err != nil {
			rollbackErr = errors.Join(rollbackErr, fmt.Errorf("rollback position %d (%s): %w", index+1, target.write.relativePath, err))
		}
	}
	return rollbackErr
}

func rollbackExistingTarget(files fileSystem, target *snapshotTarget) error {
	removeErr := removeIfAbsent(files, target.path)
	if removeErr == nil {
		if err := files.rename(target.backupPath, target.path); err == nil {
			target.backupMoved = false
			target.originalRestored = true
			return nil
		} else {
			removeErr = fmt.Errorf("restore original from backup %q: %w", target.backupPath, err)
		}
	}
	fallbackErr := files.writeFile(target.path, target.original.payload, target.original.mode)
	if fallbackErr == nil {
		target.originalRestored = true
		return fmt.Errorf("backup restoration failed; retained-byte fallback restored %q: %w", target.path, removeErr)
	}
	return errors.Join(
		removeErr,
		fmt.Errorf("retained-byte fallback for %q failed; original backup retained at %q: %w", target.path, target.backupPath, fallbackErr),
	)
}

func removeCreatedTarget(files fileSystem, target *snapshotTarget) error {
	if err := removeIfAbsent(files, target.path); err != nil {
		return fmt.Errorf("remove newly created target %q: %w", target.path, err)
	}
	target.originalRestored = true
	return nil
}

func cleanupFailedTransaction(files fileSystem, targets []snapshotTarget) error {
	cleanupErr := cleanupStagedArtifacts(files, targets)
	for index := range targets {
		target := &targets[index]
		if target.backupPath == "" || (target.backupMoved && !target.originalRestored) {
			continue
		}
		if err := removeIfAbsent(files, target.backupPath); err != nil {
			cleanupErr = errors.Join(cleanupErr, fmt.Errorf("cleanup position %d (%s) backup %q: %w", index+1, target.write.relativePath, target.backupPath, err))
		}
	}
	return cleanupErr
}

func cleanupCommittedTransaction(files fileSystem, targets []snapshotTarget) error {
	var cleanupErr error
	for index := range targets {
		target := &targets[index]
		if target.backupPath == "" {
			continue
		}
		if err := removeIfAbsent(files, target.backupPath); err != nil {
			cleanupErr = errors.Join(cleanupErr, fmt.Errorf("cleanup position %d (%s) backup %q: %w", index+1, target.write.relativePath, target.backupPath, err))
		}
	}
	return cleanupErr
}

func cleanupStagedArtifacts(files fileSystem, targets []snapshotTarget) error {
	var cleanupErr error
	for index := range targets {
		if targets[index].stagePath == "" {
			continue
		}
		if err := removeIfAbsent(files, targets[index].stagePath); err != nil {
			cleanupErr = errors.Join(cleanupErr, fmt.Errorf("cleanup position %d (%s) stage %q: %w", index+1, targets[index].write.relativePath, targets[index].stagePath, err))
		}
	}
	return cleanupErr
}

func removeIfAbsent(files fileSystem, path string) error {
	if path == "" {
		return nil
	}
	err := files.remove(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

func wrapOptionalError(operation, path string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s %q: %w", operation, path, err)
}
