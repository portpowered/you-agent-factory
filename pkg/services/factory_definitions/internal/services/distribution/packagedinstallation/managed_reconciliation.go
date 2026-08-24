package packagedinstallation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

const (
	managedStampVersion  = 1
	managedStampName     = ".you-packaged-installation.json"
	managedStampTemp     = managedStampName + ".tmp"
	managedBackupRoot    = ".you-packaged-backups"
	managedScratchRoot   = ".you-packaged-managed"
	managedPathAttempts  = 100
	managedEnsureRetries = 7
	managedRetryDelay    = 10 * time.Millisecond
)

type managedInstallationStamp struct {
	Version            int    `json:"version"`
	FactoryName        string `json:"factoryName"`
	PublishedContentID string `json:"publishedContentId"`
	InstalledContentID string `json:"installedContentId"`
}

func (service *Service) ensureManagedPackagedFactory(
	ctx context.Context,
	rootDir string,
	backendScopeID string,
	definition factorydefinitions.PackagedDefinition,
) (factorydefinitions.PackagedFactoryInstallResult, error) {
	params := factorydefinitions.PackagedFactoryInstallParams{
		NamedFactoriesRoot: rootDir,
		BackendScopeID:     backendScopeID,
		Definition:         definition,
		Format:             factorydefinitions.PackagedFactoryFormatJSON,
		ManagedRefresh:     true,
	}
	for attempt := 0; ; attempt++ {
		result, err := service.InstallPackagedFactory(ctx, params)
		if err == nil || !errors.Is(err, factorydefinitions.ErrFactoryInstallationContention) || attempt >= managedEnsureRetries {
			return result, err
		}
		delay := managedRetryDelay << attempt
		if err := waitForManagedRetry(ctx, delay); err != nil {
			return result, err
		}
	}
}

func waitForManagedRetry(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (service *Service) logInstallationOutcome(
	backendScopeID string,
	result factorydefinitions.PackagedFactoryInstallResult,
	lease *stagingLease,
) {
	outcome := string(result.Outcome)
	if outcome == "" {
		outcome = "success"
	}
	logOutcome := outcome
	if result.Outcome == factorydefinitions.PackagedFactoryInstallCreated ||
		result.Outcome == factorydefinitions.PackagedFactoryInstallReplaced ||
		result.Outcome == factorydefinitions.PackagedFactoryInstallSkipped {
		logOutcome = "success"
	}
	fields := []any{
		"backend_scope_id", normalizedBackendScopeID(backendScopeID),
		"factory_name", result.Name,
		"resource", lease.path,
		"outcome", logOutcome,
		"install_outcome", outcome,
		"owner_liveness", ownerLivenessActive,
		"owner_pid", lease.owner.PID,
		"owner_identity", "unverified",
	}
	if result.BackupDir != "" {
		fields = append(fields, "backup_dir", result.BackupDir)
	}
	switch result.Outcome {
	case factorydefinitions.PackagedFactoryInstallCustomerModified:
		service.logger.Warn("factory_definitions.packaged_installation", fields...)
	case factorydefinitions.PackagedFactoryInstallFailed:
		service.logger.Error("factory_definitions.packaged_installation", fields...)
	default:
		service.logger.Info("factory_definitions.packaged_installation", fields...)
	}
}

func (service *Service) createManagedPackagedFactory(
	ctx context.Context,
	rootDir string,
	name string,
	payload []byte,
	rootFileName string,
	result factorydefinitions.PackagedFactoryInstallResult,
) (factorydefinitions.PackagedFactoryInstallResult, error) {
	prepared, err := service.prepareManagedLayout(ctx, name, payload, rootFileName)
	if err != nil {
		return managedInstallFailure(result, name, rootDir, err)
	}
	factoryDir, err := service.persistence.CreateNamedFactory(rootDir, name, prepared)
	if err != nil {
		return managedInstallFailure(result, name, rootDir, err)
	}
	result.FactoryDir = factoryDir
	installedID, err := service.contentIdentity(factoryDir)
	if err != nil {
		return managedInstallFailure(result, name, rootDir, err)
	}
	if err := service.writeManagedStamp(ctx, factoryDir, name, installedID, installedID); err != nil {
		return managedInstallFailure(result, name, rootDir, err)
	}
	result.Outcome = factorydefinitions.PackagedFactoryInstallCreated
	result.PublishedContentID = installedID
	result.InstalledContentID = installedID
	return result, nil
}

func (service *Service) reconcileManagedPackagedFactory(
	ctx context.Context,
	rootDir string,
	name string,
	normalizedFormat factorydefinitions.PackagedFactoryFormat,
	targetDir string,
	payload []byte,
	rootFileName string,
	result factorydefinitions.PackagedFactoryInstallResult,
) (factorydefinitions.PackagedFactoryInstallResult, error) {
	if err := service.persistence.ValidateFactoryLayout(targetDir); err != nil {
		return managedInstallFailure(result, name, rootDir, fmt.Errorf("existing target %s is invalid: %w", targetDir, err))
	}
	existingFormat, err := authoredRootFormat(targetDir, service.fileSystem)
	if err != nil {
		return managedInstallFailure(result, name, rootDir, err)
	}
	if existingFormat != normalizedFormat {
		return managedInstallFailure(
			result,
			name,
			rootDir,
			fmt.Errorf("%w: factory %q already exists", factorydefinitions.ErrNamedFactoryAlreadyExists, name),
		)
	}
	prepared, err := service.prepareManagedLayout(ctx, name, payload, rootFileName)
	if err != nil {
		return managedInstallFailure(result, name, rootDir, err)
	}
	publishedID, err := service.expectedManagedContentID(ctx, rootDir, name, prepared)
	if err != nil {
		return managedInstallFailure(result, name, rootDir, err)
	}
	installedID, err := service.contentIdentity(targetDir)
	if err != nil {
		return managedInstallFailure(result, name, rootDir, err)
	}
	stamp, stamped, err := service.readManagedStamp(targetDir, name)
	if err != nil {
		return managedInstallFailure(result, name, rootDir, err)
	}
	result.PublishedContentID = publishedID
	result.InstalledContentID = installedID
	if installedID == publishedID {
		if !stamped || stamp.PublishedContentID != publishedID || stamp.InstalledContentID != installedID {
			if err := service.writeManagedStamp(ctx, targetDir, name, publishedID, installedID); err != nil {
				return managedInstallFailure(result, name, rootDir, err)
			}
		}
		result.Outcome = factorydefinitions.PackagedFactoryInstallCurrent
		return result, nil
	}
	outcome := factorydefinitions.PackagedFactoryInstallRefreshed
	if stamped && stamp.InstalledContentID != installedID {
		outcome = factorydefinitions.PackagedFactoryInstallCustomerModified
	}
	return service.refreshManagedPackagedFactory(
		ctx,
		rootDir,
		name,
		targetDir,
		prepared,
		publishedID,
		outcome,
		result,
	)
}

func (service *Service) refreshManagedPackagedFactory(
	ctx context.Context,
	rootDir string,
	name string,
	targetDir string,
	prepared *factorydefinitions.PreparedFactoryLayoutPayload,
	publishedID string,
	outcome factorydefinitions.PackagedFactoryInstallOutcome,
	result factorydefinitions.PackagedFactoryInstallResult,
) (factorydefinitions.PackagedFactoryInstallResult, error) {
	backupDir, err := service.snapshotManagedFactory(ctx, rootDir, name, targetDir)
	if err != nil {
		return managedInstallFailure(result, name, rootDir, err)
	}
	result.BackupDir = backupDir
	if err := ctx.Err(); err != nil {
		return service.cleanupManagedReplacementFailure(result, name, rootDir, backupDir, err)
	}
	replacement, err := service.persistence.ReplaceFactoryLayout(targetDir, prepared)
	if err != nil {
		return service.cleanupManagedReplacementFailure(result, name, rootDir, backupDir, err)
	}
	if replacement == nil {
		return service.cleanupManagedReplacementFailure(
			result,
			name,
			rootDir,
			backupDir,
			fmt.Errorf("Factory Definitions replacement result is required"),
		)
	}
	installedID, err := service.contentIdentity(targetDir)
	if err != nil {
		service.discardReplacementBackup(replacement)
		return managedInstallFailure(result, name, rootDir, err)
	}
	if err := service.writeManagedStamp(ctx, targetDir, name, publishedID, installedID); err != nil {
		service.discardReplacementBackup(replacement)
		return managedInstallFailure(result, name, rootDir, err)
	}
	service.discardReplacementBackup(replacement)
	result.Outcome = outcome
	result.PublishedContentID = publishedID
	result.InstalledContentID = installedID
	return result, nil
}

func (service *Service) prepareManagedLayout(
	ctx context.Context,
	name string,
	payload []byte,
	rootFileName string,
) (*factorydefinitions.PreparedFactoryLayoutPayload, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	prepared, err := service.persistence.PreparePackagedFactoryLayout(ctx, name, payload)
	if err != nil {
		return nil, err
	}
	if prepared == nil {
		return nil, fmt.Errorf("prepared Factory layout is required")
	}
	prepared.RootFileName = rootFileName
	return prepared, nil
}

func (service *Service) expectedManagedContentID(
	ctx context.Context,
	rootDir string,
	name string,
	prepared *factorydefinitions.PreparedFactoryLayoutPayload,
) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := service.fileSystem.MkdirAll(rootDir, 0o755); err != nil {
		return "", fmt.Errorf("create packaged Factory staging parent %s: %w", rootDir, err)
	}
	scratchDir, err := service.createManagedDirectory(rootDir, managedScratchRoot, name, 0o755)
	if err != nil {
		return "", fmt.Errorf("create packaged Factory comparison staging: %w", err)
	}
	factoryDir, createErr := service.persistence.CreateNamedFactory(scratchDir, name, prepared)
	identity, identityErr := "", error(nil)
	if createErr == nil {
		identity, identityErr = service.contentIdentity(factoryDir)
	}
	cleanupErr := service.fileSystem.RemoveAll(scratchDir)
	if createErr != nil {
		return "", fmt.Errorf("materialize packaged Factory comparison layout: %w", createErr)
	}
	if identityErr != nil {
		return "", identityErr
	}
	if cleanupErr != nil {
		return "", fmt.Errorf("remove packaged Factory comparison staging %s: %w", scratchDir, cleanupErr)
	}
	return identity, nil
}

func (service *Service) readManagedStamp(
	targetDir string,
	name string,
) (managedInstallationStamp, bool, error) {
	data, err := service.fileSystem.ReadFile(filepath.Join(targetDir, managedStampName))
	if errors.Is(err, fs.ErrNotExist) {
		return managedInstallationStamp{}, false, nil
	}
	if err != nil {
		return managedInstallationStamp{}, false, fmt.Errorf("read packaged Factory management evidence %s: %w", targetDir, err)
	}
	var stamp managedInstallationStamp
	if err := json.Unmarshal(data, &stamp); err != nil {
		return managedInstallationStamp{}, false, nil
	}
	if stamp.Version != managedStampVersion ||
		stamp.FactoryName != name ||
		strings.TrimSpace(stamp.PublishedContentID) == "" ||
		strings.TrimSpace(stamp.InstalledContentID) == "" {
		return managedInstallationStamp{}, false, nil
	}
	return stamp, true, nil
}

func (service *Service) writeManagedStamp(
	ctx context.Context,
	targetDir string,
	name string,
	publishedID string,
	installedID string,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	data, err := json.Marshal(managedInstallationStamp{
		Version:            managedStampVersion,
		FactoryName:        name,
		PublishedContentID: publishedID,
		InstalledContentID: installedID,
	})
	if err != nil {
		return fmt.Errorf("encode packaged Factory management evidence: %w", err)
	}
	temporary := filepath.Join(targetDir, managedStampTemp)
	metadata := filepath.Join(targetDir, managedStampName)
	if err := service.fileSystem.WriteFile(temporary, data, 0o600); err != nil {
		_ = service.fileSystem.RemoveAll(temporary)
		return fmt.Errorf("write packaged Factory management evidence: %w", err)
	}
	if err := ctx.Err(); err != nil {
		_ = service.fileSystem.RemoveAll(temporary)
		return err
	}
	if err := service.fileSystem.Rename(temporary, metadata); err != nil {
		_ = service.fileSystem.RemoveAll(temporary)
		return fmt.Errorf("publish packaged Factory management evidence: %w", err)
	}
	return nil
}

func (service *Service) contentIdentity(root string) (string, error) {
	if _, err := service.fileSystem.Stat(root); err != nil {
		return "", fmt.Errorf("inspect packaged Factory content %s: %w", root, err)
	}
	hasher := sha256.New()
	if err := service.hashManagedDirectory(hasher, root, ""); err != nil {
		return "", err
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func (service *Service) hashManagedDirectory(hasher interface{ Write([]byte) (int, error) }, root string, relative string) error {
	entries, err := service.fileSystem.ReadDir(root)
	if err != nil {
		return fmt.Errorf("read packaged Factory content %s: %w", root, err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, entry := range entries {
		childRelative := entry.Name()
		if relative != "" {
			childRelative = filepath.Join(relative, entry.Name())
		}
		if childRelative == managedStampName || childRelative == managedStampTemp {
			continue
		}
		childPath := filepath.Join(root, entry.Name())
		if entry.IsDir() {
			if _, err := fmt.Fprintf(hasher, "d:%d:%s\n", len(childRelative), filepath.ToSlash(childRelative)); err != nil {
				return err
			}
			if err := service.hashManagedDirectory(hasher, childPath, childRelative); err != nil {
				return err
			}
			continue
		}
		data, err := service.fileSystem.ReadFile(childPath)
		if err != nil {
			return fmt.Errorf("read packaged Factory content %s: %w", childPath, err)
		}
		if _, err := fmt.Fprintf(
			hasher,
			"f:%d:%s:%d:",
			len(childRelative),
			filepath.ToSlash(childRelative),
			len(data),
		); err != nil {
			return err
		}
		if _, err := hasher.Write(data); err != nil {
			return err
		}
		if _, err := hasher.Write([]byte{'\n'}); err != nil {
			return err
		}
	}
	return nil
}

func (service *Service) snapshotManagedFactory(
	ctx context.Context,
	rootDir string,
	name string,
	targetDir string,
) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	before, err := service.contentIdentity(targetDir)
	if err != nil {
		return "", err
	}
	backupDir, err := service.createManagedDirectory(
		rootDir,
		managedBackupRoot,
		name,
		0o700,
	)
	if err != nil {
		return "", fmt.Errorf("reserve packaged Factory backup: %w", err)
	}
	if err := service.copyManagedDirectory(ctx, targetDir, backupDir); err != nil {
		cleanupErr := service.fileSystem.RemoveAll(backupDir)
		if cleanupErr != nil {
			err = errors.Join(err, fmt.Errorf("remove incomplete packaged Factory backup %s: %w", backupDir, cleanupErr))
		}
		return "", fmt.Errorf("preserve packaged Factory %q at %s: %w", name, backupDir, err)
	}
	after, err := service.contentIdentity(targetDir)
	if err != nil {
		cleanupErr := service.fileSystem.RemoveAll(backupDir)
		if cleanupErr != nil {
			err = errors.Join(err, fmt.Errorf("remove packaged Factory backup %s: %w", backupDir, cleanupErr))
		}
		return "", fmt.Errorf("verify packaged Factory backup %s: %w", backupDir, err)
	}
	if before != after {
		if cleanupErr := service.fileSystem.RemoveAll(backupDir); cleanupErr != nil {
			return "", fmt.Errorf("packaged Factory %q changed while preserving backup %s and cleanup failed: %w", name, backupDir, cleanupErr)
		}
		return "", fmt.Errorf("packaged Factory %q changed while preserving backup %s; retry initialization", name, backupDir)
	}
	return backupDir, nil
}

func (service *Service) copyManagedDirectory(ctx context.Context, sourceDir, targetDir string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	entries, err := service.fileSystem.ReadDir(sourceDir)
	if err != nil {
		return err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		sourcePath := filepath.Join(sourceDir, entry.Name())
		targetPath := filepath.Join(targetDir, entry.Name())
		if entry.IsDir() {
			if err := service.fileSystem.MkdirAll(targetPath, info.Mode().Perm()); err != nil {
				return err
			}
			if err := service.copyManagedDirectory(ctx, sourcePath, targetPath); err != nil {
				return err
			}
			continue
		}
		data, err := service.fileSystem.ReadFile(sourcePath)
		if err != nil {
			return err
		}
		if err := service.fileSystem.WriteFile(targetPath, data, info.Mode().Perm()); err != nil {
			return err
		}
	}
	return nil
}

func (service *Service) createManagedDirectory(
	rootDir string,
	parentName string,
	name string,
	mode fs.FileMode,
) (string, error) {
	parentDir := filepath.Join(rootDir, parentName)
	if err := service.fileSystem.MkdirAll(parentDir, 0o700); err != nil {
		return "", err
	}
	safeName := strings.NewReplacer("/", "--", "\\", "--", "@", "").Replace(name)
	for attempt := 0; attempt < managedPathAttempts; attempt++ {
		candidate := filepath.Join(
			parentDir,
			fmt.Sprintf("%s-%s-%d-%d", parentName, safeName, os.Getpid(), attempt),
		)
		if err := service.directoryCreator(candidate, mode); err == nil {
			return candidate, nil
		} else if !errors.Is(err, fs.ErrExist) && !os.IsExist(err) {
			return "", err
		}
	}
	return "", fmt.Errorf("could not reserve a unique packaged Factory path below %s", parentDir)
}

func (service *Service) cleanupManagedReplacementFailure(
	result factorydefinitions.PackagedFactoryInstallResult,
	name string,
	rootDir string,
	backupDir string,
	cause error,
) (factorydefinitions.PackagedFactoryInstallResult, error) {
	if cleanupErr := service.fileSystem.RemoveAll(backupDir); cleanupErr != nil {
		cause = errors.Join(cause, fmt.Errorf("remove packaged Factory backup %s: %w", backupDir, cleanupErr))
	} else {
		result.BackupDir = ""
	}
	return managedInstallFailure(result, name, rootDir, cause)
}

func (service *Service) discardReplacementBackup(replacement *factorydefinitions.FactorySplitLayoutReplaceResult) {
	if replacement != nil && replacement.DiscardBackup != nil {
		replacement.DiscardBackup()
	}
}

func managedInstallFailure(
	result factorydefinitions.PackagedFactoryInstallResult,
	name string,
	rootDir string,
	cause error,
) (factorydefinitions.PackagedFactoryInstallResult, error) {
	result.Outcome = factorydefinitions.PackagedFactoryInstallFailed
	return result, installError(name, rootDir, cause)
}
