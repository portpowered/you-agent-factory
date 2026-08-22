package packagedinstallation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	authoringlayoutpersist "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/authoring_layout/persist"
	namedfactorypath "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/catalog/namedpaths"
)

type Service struct {
	persistence      factorydefinitions.PackagedFactoryPersistence
	fileSystem       factorydefinitions.PackagedInstallationFileSystem
	directoryCreator factorydefinitions.PackagedInstallationDirectoryCreator
	ownerProbe       ownerProbe
	logger           logging.Logger
}

const (
	stagingOwnerSuffix       = "owner"
	stagingOwnerMetadataName = ".owner.json"
	stagingAcquireAttempts   = 2
)

type ownerRecord struct {
	PID int `json:"pid"`
}

type ownerLiveness string

const (
	ownerLivenessActive           ownerLiveness = "active"
	ownerLivenessOrphaned         ownerLiveness = "orphaned"
	ownerLivenessIndeterminate    ownerLiveness = "indeterminate"
	ownerLivenessPermissionDenied ownerLiveness = "permission-denied"
	ownerLivenessRacing           ownerLiveness = "racing"
)

type ownerProbe interface {
	Current() (ownerRecord, error)
	Classify(ownerRecord) ownerLiveness
}

type localOwnerProbe struct{}

func (localOwnerProbe) Current() (ownerRecord, error) {
	return ownerRecord{PID: os.Getpid()}, nil
}

func (localOwnerProbe) Classify(owner ownerRecord) ownerLiveness {
	return probeOwnerPID(owner.PID)
}

func New(
	persistence factorydefinitions.PackagedFactoryPersistence,
	fileSystem factorydefinitions.PackagedInstallationFileSystem,
	directoryCreator factorydefinitions.PackagedInstallationDirectoryCreator,
	loggers ...logging.Logger,
) *Service {
	var logger logging.Logger
	if len(loggers) > 0 {
		logger = loggers[0]
	}
	return &Service{
		persistence:      persistence,
		fileSystem:       fileSystem,
		directoryCreator: directoryCreator,
		ownerProbe:       localOwnerProbe{},
		logger:           logging.EnsureLogger(logger),
	}
}

func newWithOwnerProbe(
	persistence factorydefinitions.PackagedFactoryPersistence,
	fileSystem factorydefinitions.PackagedInstallationFileSystem,
	directoryCreator factorydefinitions.PackagedInstallationDirectoryCreator,
	probe ownerProbe,
	loggers ...logging.Logger,
) *Service {
	service := New(persistence, fileSystem, directoryCreator, loggers...)
	if probe != nil {
		service.ownerProbe = probe
	}
	return service
}

func (service *Service) EnsurePackagedFactories(
	ctx context.Context,
	namedFactoriesRoot string,
	backendScopeID string,
	definitions []factorydefinitions.PackagedDefinition,
) ([]factorydefinitions.PackagedFactoryInstallResult, error) {
	results := make([]factorydefinitions.PackagedFactoryInstallResult, 0, len(definitions))
	for _, definition := range definitions {
		result, err := service.ensureManagedPackagedFactory(
			ctx,
			namedFactoriesRoot,
			backendScopeID,
			definition,
		)
		if err != nil {
			results = append(results, result)
			return results, err
		}
		results = append(results, result)
	}
	return results, nil
}

func (service *Service) InstallPackagedFactory(
	ctx context.Context,
	params factorydefinitions.PackagedFactoryInstallParams,
) (result factorydefinitions.PackagedFactoryInstallResult, installErr error) {
	definition := params.Definition
	format := params.Format
	namedFactoriesRoot := params.NamedFactoriesRoot
	backendScopeID := normalizedBackendScopeID(params.BackendScopeID)
	result = factorydefinitions.PackagedFactoryInstallResult{
		Name:   definition.Name,
		Format: format,
	}
	defer func() {
		if installErr != nil && params.ManagedRefresh {
			result.Outcome = factorydefinitions.PackagedFactoryInstallFailed
		}
	}()
	if err := ctx.Err(); err != nil {
		return result, service.installationError(backendScopeID, namedFactoriesRoot, definition.Name, "", ownerLivenessIndeterminate, err)
	}
	payload, rootFileName, normalizedFormat, err := selectPayload(
		definition,
		format,
	)
	if err != nil {
		return result, service.installationError(backendScopeID, namedFactoriesRoot, definition.Name, "", ownerLivenessIndeterminate, err)
	}
	result.Format = normalizedFormat
	targetDir, err := namedfactorypath.MapDir(namedFactoriesRoot, definition.Name)
	if err != nil {
		return result, service.installationError(backendScopeID, namedFactoriesRoot, definition.Name, "", ownerLivenessIndeterminate, err)
	}
	result.FactoryDir = targetDir
	if err := service.requireDependencies(); err != nil {
		return result, service.installationError(backendScopeID, namedFactoriesRoot, definition.Name, "", ownerLivenessIndeterminate, err)
	}
	recoveredLease, err := service.rejectPreExistingStaging(backendScopeID, namedFactoriesRoot, definition.Name)
	if err != nil {
		return result, service.wrapInstallationError(definition.Name, namedFactoriesRoot, err)
	}
	if _, statErr := service.fileSystem.Stat(targetDir); statErr == nil {
		return service.installExistingPackagedFactory(
			ctx,
			params,
			backendScopeID,
			normalizedFormat,
			targetDir,
			payload,
			rootFileName,
			result,
			recoveredLease,
		)
	} else if !errors.Is(statErr, fs.ErrNotExist) {
		if releaseErr := service.releaseStagingOwnership(recoveredLease); releaseErr != nil {
			statErr = errors.Join(statErr, releaseErr)
		}
		return result, service.installationError(backendScopeID, namedFactoriesRoot, definition.Name, targetDir, ownerLivenessIndeterminate, fmt.Errorf("inspect target %s: %w", targetDir, statErr))
	}
	return service.withStagingOwnership(
		ctx,
		namedFactoriesRoot,
		definition.Name,
		backendScopeID,
		result,
		recoveredLease,
		func() (factorydefinitions.PackagedFactoryInstallResult, error) {
			if params.ManagedRefresh {
				return service.createManagedPackagedFactory(
					ctx,
					namedFactoriesRoot,
					definition.Name,
					payload,
					rootFileName,
					result,
				)
			}
			return service.createPackagedFactory(
				ctx,
				namedFactoriesRoot,
				definition.Name,
				targetDir,
				payload,
				rootFileName,
				result,
			)
		},
	)
}

func (service *Service) installExistingPackagedFactory(
	ctx context.Context,
	params factorydefinitions.PackagedFactoryInstallParams,
	backendScopeID string,
	normalizedFormat factorydefinitions.PackagedFactoryFormat,
	targetDir string,
	payload []byte,
	rootFileName string,
	result factorydefinitions.PackagedFactoryInstallResult,
	recoveredLease *stagingLease,
) (factorydefinitions.PackagedFactoryInstallResult, error) {
	name := params.Definition.Name
	rootDir := params.NamedFactoriesRoot
	if params.ManagedRefresh {
		return service.withStagingOwnership(
			ctx,
			rootDir,
			name,
			backendScopeID,
			result,
			recoveredLease,
			func() (factorydefinitions.PackagedFactoryInstallResult, error) {
				return service.reconcileManagedPackagedFactory(
					ctx,
					rootDir,
					name,
					normalizedFormat,
					targetDir,
					payload,
					rootFileName,
					result,
				)
			},
		)
	}
	if err := service.persistence.ValidateFactoryLayout(targetDir); err != nil {
		if releaseErr := service.releaseStagingOwnership(recoveredLease); releaseErr != nil {
			err = errors.Join(err, releaseErr)
		}
		return result, service.installationError(backendScopeID, rootDir, name, targetDir, ownerLivenessIndeterminate, fmt.Errorf("existing target %s is invalid: %w", targetDir, err))
	}
	if params.Replace {
		return service.withStagingOwnership(
			ctx,
			rootDir,
			name,
			backendScopeID,
			result,
			recoveredLease,
			func() (factorydefinitions.PackagedFactoryInstallResult, error) {
				return service.replaceExistingPackagedFactory(ctx, rootDir, name, targetDir, payload, rootFileName, result)
			},
		)
	}
	if releaseErr := service.releaseStagingOwnership(recoveredLease); releaseErr != nil {
		return result, service.wrapInstallationError(name, rootDir, releaseErr)
	}
	existingFormat, err := authoredRootFormat(targetDir, service.fileSystem)
	if err != nil {
		return result, service.installationError(backendScopeID, rootDir, name, targetDir, ownerLivenessIndeterminate, err)
	}
	if existingFormat != normalizedFormat {
		return result, service.installationError(
			backendScopeID,
			rootDir,
			name,
			targetDir,
			ownerLivenessIndeterminate,
			fmt.Errorf("%w: factory %q already exists", factorydefinitions.ErrNamedFactoryAlreadyExists, name),
		)
	}
	result.Outcome = factorydefinitions.PackagedFactoryInstallSkipped
	return result, nil
}

func (service *Service) requireDependencies() error {
	if service == nil {
		return fmt.Errorf("packaged Factory installation service is required")
	}
	if service.fileSystem == nil {
		return fmt.Errorf("packaged Factory installation filesystem is required")
	}
	if service.directoryCreator == nil {
		return fmt.Errorf("packaged Factory installation directory creator is required")
	}
	if service.persistence == nil {
		return fmt.Errorf("Factory Definitions persistence service is required")
	}
	if service.ownerProbe == nil {
		return fmt.Errorf("packaged Factory installation owner probe is required")
	}
	return nil
}

func (service *Service) rejectPreExistingStaging(backendScopeID, rootDir, name string) (*stagingLease, error) {
	stagingPath, err := findPreExistingStaging(service.fileSystem, rootDir, name)
	if err != nil {
		return nil, service.installationFailure(
			backendScopeID,
			rootDir,
			name,
			stagingOwnershipPath(rootDir, name),
			ownerLivenessIndeterminate,
			err,
		)
	}
	if stagingPath == "" {
		return nil, nil
	}
	contention, retry, recoveredLease, inspectErr := service.inspectStagingContention(backendScopeID, rootDir, name, stagingPath)
	if inspectErr != nil {
		return nil, inspectErr
	}
	if retry {
		return recoveredLease, nil
	}
	return nil, contention
}

func (service *Service) withStagingOwnership(
	ctx context.Context,
	rootDir string,
	name string,
	backendScopeID string,
	result factorydefinitions.PackagedFactoryInstallResult,
	recoveredLease *stagingLease,
	operation func() (factorydefinitions.PackagedFactoryInstallResult, error),
) (installResult factorydefinitions.PackagedFactoryInstallResult, installErr error) {
	lease := recoveredLease
	var err error
	if lease == nil {
		lease, err = service.acquireStagingOwnership(ctx, backendScopeID, rootDir, name)
	}
	if err != nil {
		return result, err
	}
	defer func() {
		if releaseErr := service.releaseStagingOwnership(lease); releaseErr != nil {
			if installErr != nil {
				installErr = errors.Join(installErr, releaseErr)
			} else {
				installErr = releaseErr
			}
			installErr = service.wrapInstallationError(name, rootDir, installErr)
		}
	}()
	installResult, installErr = operation()
	if installErr == nil {
		service.logInstallationOutcome(backendScopeID, installResult, lease)
	} else {
		service.logOutcome(
			backendScopeID,
			name,
			lease.path,
			"failed",
			ownerLivenessActive,
			lease.owner.PID,
		)
	}
	return installResult, installErr
}

type stagingLease struct {
	path           string
	root           string
	name           string
	backendScopeID string
	owner          ownerRecord
}

func (service *Service) acquireStagingOwnership(
	ctx context.Context,
	backendScopeID string,
	rootDir string,
	name string,
) (*stagingLease, error) {
	for attempt := 0; attempt < stagingAcquireAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, service.installationFailure(backendScopeID, rootDir, name, stagingOwnershipPath(rootDir, name), ownerLivenessIndeterminate, err)
		}
		stagingPath, err := findPreExistingStaging(service.fileSystem, rootDir, name)
		if err != nil {
			return nil, service.installationFailure(backendScopeID, rootDir, name, stagingOwnershipPath(rootDir, name), ownerLivenessIndeterminate, err)
		}
		if stagingPath != "" {
			contention, retry, recoveredLease, inspectErr := service.inspectStagingContention(
				backendScopeID,
				rootDir,
				name,
				stagingPath,
			)
			if inspectErr != nil {
				return nil, inspectErr
			}
			if retry {
				if recoveredLease != nil {
					return recoveredLease, nil
				}
				continue
			}
			return nil, contention
		}
		lease, lost, err := service.publishOwnedStagingLease(backendScopeID, rootDir, name)
		if err != nil {
			return nil, err
		}
		if lost {
			continue
		}
		return lease, nil
	}
	return nil, service.installationContention(
		backendScopeID,
		rootDir,
		name,
		stagingOwnershipPath(rootDir, name),
		"failed",
		ownerLivenessRacing,
		0,
	)
}

func (service *Service) inspectStagingContention(
	backendScopeID string,
	rootDir string,
	name string,
	stagingPath string,
) (error, bool, *stagingLease, error) {
	if _, err := service.fileSystem.Stat(stagingPath); errors.Is(err, fs.ErrNotExist) {
		return nil, true, nil, nil
	} else if err != nil {
		return nil, false, nil, service.installationFailure(
			backendScopeID,
			rootDir,
			name,
			stagingPath,
			ownerLivenessIndeterminate,
			fmt.Errorf("inspect packaged Factory staging resource %s: %w", stagingPath, err),
		)
	}
	if stagingPath != stagingOwnershipPath(rootDir, name) {
		return service.installationContention(
			backendScopeID,
			rootDir,
			name,
			stagingPath,
			"indeterminate-contention",
			ownerLivenessIndeterminate,
			0,
		), false, nil, nil
	}
	owner, liveness, err := service.readOwnerRecord(stagingPath)
	if errors.Is(err, fs.ErrNotExist) {
		if _, statErr := service.fileSystem.Stat(stagingPath); errors.Is(statErr, fs.ErrNotExist) {
			return nil, true, nil, nil
		}
		return service.installationContention(
			backendScopeID,
			rootDir,
			name,
			stagingPath,
			"indeterminate-contention",
			ownerLivenessIndeterminate,
			0,
		), false, nil, nil
	}
	if err != nil {
		return service.installationContention(
			backendScopeID,
			rootDir,
			name,
			stagingPath,
			"indeterminate-contention",
			liveness,
			owner.PID,
		), false, nil, nil
	}
	if liveness == ownerLivenessActive {
		return service.installationContention(
			backendScopeID,
			rootDir,
			name,
			stagingPath,
			"active-contention",
			liveness,
			owner.PID,
		), false, nil, nil
	}
	if liveness == ownerLivenessOrphaned {
		recoveredLease, reclaimErr := service.reclaimOrphanedStaging(
			backendScopeID,
			rootDir,
			name,
			stagingPath,
			owner,
		)
		if reclaimErr != nil {
			return nil, false, nil, reclaimErr
		}
		if recoveredLease != nil {
			return nil, true, recoveredLease, nil
		}
	}
	return service.installationContention(
		backendScopeID,
		rootDir,
		name,
		stagingPath,
		"indeterminate-contention",
		liveness,
		owner.PID,
	), false, nil, nil
}

func (service *Service) publishOwnerRecord(path string, owner ownerRecord) error {
	data, err := json.Marshal(owner)
	if err != nil {
		return fmt.Errorf("encode staging owner metadata: %w", err)
	}
	temporary := filepath.Join(path, stagingOwnerMetadataName+".tmp")
	metadataPath := filepath.Join(path, stagingOwnerMetadataName)
	if err := service.fileSystem.WriteFile(temporary, data, 0o600); err != nil {
		return fmt.Errorf("write staging owner metadata: %w", err)
	}
	if err := service.fileSystem.Rename(temporary, metadataPath); err != nil {
		_ = service.fileSystem.RemoveAll(temporary)
		return fmt.Errorf("publish staging owner metadata: %w", err)
	}
	return nil
}

func (service *Service) readOwnerRecord(path string) (ownerRecord, ownerLiveness, error) {
	metadataPath := filepath.Join(path, stagingOwnerMetadataName)
	data, err := service.fileSystem.ReadFile(metadataPath)
	if err != nil {
		if errors.Is(err, fs.ErrPermission) {
			return ownerRecord{}, ownerLivenessPermissionDenied, err
		}
		return ownerRecord{}, ownerLivenessIndeterminate, err
	}
	var owner ownerRecord
	if err := json.Unmarshal(data, &owner); err != nil || owner.PID <= 0 {
		if err == nil {
			err = fmt.Errorf("owner PID must be positive")
		}
		return ownerRecord{}, ownerLivenessIndeterminate, fmt.Errorf("invalid staging owner metadata: %w", err)
	}
	return owner, service.ownerProbe.Classify(owner), nil
}

func (service *Service) releaseStagingOwnership(lease *stagingLease) error {
	if lease == nil {
		return nil
	}
	owner, _, err := service.readOwnerRecord(lease.path)
	if err != nil {
		return service.installationFailure(
			lease.backendScopeID,
			lease.root,
			lease.name,
			lease.path,
			ownerLivenessIndeterminate,
			err,
		)
	}
	if owner != lease.owner {
		return service.installationFailure(
			lease.backendScopeID,
			lease.root,
			lease.name,
			lease.path,
			ownerLivenessRacing,
			fmt.Errorf("staging owner changed from PID %d to PID %d", lease.owner.PID, owner.PID),
		)
	}
	if err := service.retractStagingLease(lease.root, lease.path); err != nil {
		return service.installationFailure(
			lease.backendScopeID,
			lease.root,
			lease.name,
			lease.path,
			ownerLivenessActive,
			err,
		)
	}
	return nil
}

func (service *Service) logOutcome(
	backendScopeID string,
	name string,
	resource string,
	outcome string,
	liveness ownerLiveness,
	ownerPID int,
) {
	ownerEvidence := any("unavailable")
	if ownerPID > 0 {
		ownerEvidence = ownerPID
	}
	fields := []any{
		"backend_scope_id", normalizedBackendScopeID(backendScopeID),
		"factory_name", name,
		"resource", resource,
		"outcome", outcome,
		"owner_liveness", liveness,
		"owner_pid", ownerEvidence,
		"owner_identity", "unverified",
	}
	switch outcome {
	case "failed":
		service.logger.Error("factory_definitions.packaged_installation", fields...)
	case "active-contention", "indeterminate-contention":
		service.logger.Warn("factory_definitions.packaged_installation", fields...)
	default:
		service.logger.Info("factory_definitions.packaged_installation", fields...)
	}
}

func (service *Service) installationContention(
	backendScopeID string,
	rootDir string,
	name string,
	resource string,
	outcome string,
	liveness ownerLiveness,
	ownerPID int,
) error {
	service.logOutcome(backendScopeID, name, resource, outcome, liveness, ownerPID)
	return installationContention(
		backendScopeID,
		rootDir,
		name,
		resource,
		outcome,
		liveness,
		ownerPID,
	)
}

func (service *Service) installationFailure(
	backendScopeID string,
	rootDir string,
	name string,
	resource string,
	liveness ownerLiveness,
	cause error,
) error {
	service.logOutcome(backendScopeID, name, resource, "failed", liveness, 0)
	return installationFailure(backendScopeID, rootDir, name, resource, liveness, cause)
}

func (service *Service) installationError(
	backendScopeID string,
	rootDir string,
	name string,
	resource string,
	liveness ownerLiveness,
	cause error,
) error {
	service.logOutcome(backendScopeID, name, resource, "failed", liveness, 0)
	return installError(name, rootDir, cause)
}

func (service *Service) wrapInstallationError(name, rootDir string, err error) error {
	return installError(name, rootDir, err)
}

func stagingOwnershipPath(rootDir, name string) string {
	return filepath.Join(
		rootDir,
		authoringlayoutpersist.StagingDirectoryPrefix(name)+stagingOwnerSuffix,
	)
}

func installationContention(
	backendScopeID string,
	rootDir string,
	name string,
	resource string,
	outcome string,
	liveness ownerLiveness,
	ownerPID int,
) error {
	ownerEvidence := "owner_pid=unavailable owner_identity=unverified"
	if ownerPID > 0 {
		ownerEvidence = fmt.Sprintf("owner_pid=%d owner_identity=unverified", ownerPID)
	}
	return fmt.Errorf(
		"%w: backend_scope_id=%s resource=%s outcome=%s owner_liveness=%s %s; verify no you process is still installing packaged Factory %q, stop or verify that owner before retrying, then remove only %s and retry",
		factorydefinitions.ErrFactoryInstallationContention,
		normalizedBackendScopeID(backendScopeID),
		resource,
		outcome,
		liveness,
		ownerEvidence,
		name,
		resource,
	)
}

func installationFailure(
	backendScopeID string,
	rootDir string,
	name string,
	resource string,
	liveness ownerLiveness,
	cause error,
) error {
	if resource == "" {
		resource = stagingOwnershipPath(rootDir, name)
	}
	return fmt.Errorf(
		"%w: backend_scope_id=%s resource=%s outcome=failed owner_liveness=%s; verify no you process is still installing packaged Factory %q, stop or verify that owner before retrying, then remove only %s and retry: %w",
		factorydefinitions.ErrFactoryInstallationContention,
		normalizedBackendScopeID(backendScopeID),
		resource,
		liveness,
		name,
		resource,
		cause,
	)
}

func normalizedBackendScopeID(value string) string {
	if trimmed := strings.TrimSpace(value); trimmed != "" {
		return trimmed
	}
	return "unknown"
}

func findPreExistingStaging(
	fileSystem factorydefinitions.PackagedInstallationFileSystem,
	rootDir string,
	name string,
) (string, error) {
	ownershipPath := stagingOwnershipPath(rootDir, name)
	if _, err := fileSystem.Stat(ownershipPath); err == nil {
		return ownershipPath, nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Errorf("inspect packaged Factory staging resource %s: %w", ownershipPath, err)
	}
	entries, err := fileSystem.ReadDir(rootDir)
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("inspect packaged Factory staging directory %s: %w", rootDir, err)
	}
	prefix := authoringlayoutpersist.StagingDirectoryPrefix(name)
	for _, entry := range entries {
		if entry.IsDir() && strings.HasPrefix(entry.Name(), prefix) {
			return filepath.Join(rootDir, entry.Name()), nil
		}
	}
	return "", nil
}

func (service *Service) createPackagedFactory(
	ctx context.Context,
	namedFactoriesRoot string,
	name string,
	targetDir string,
	payload []byte,
	rootFileName string,
	result factorydefinitions.PackagedFactoryInstallResult,
) (factorydefinitions.PackagedFactoryInstallResult, error) {
	if err := ctx.Err(); err != nil {
		return result, installError(name, namedFactoriesRoot, err)
	}
	prepared, err := service.persistence.PreparePackagedFactoryLayout(ctx, name, payload)
	if err != nil {
		return result, installError(name, namedFactoriesRoot, err)
	}
	if prepared == nil {
		return result, installError(
			name,
			namedFactoriesRoot,
			fmt.Errorf("prepared Factory layout is required"),
		)
	}
	prepared.RootFileName = rootFileName
	factoryDir, err := service.persistence.CreateNamedFactory(namedFactoriesRoot, name, prepared)
	if err != nil {
		return result, installError(name, namedFactoriesRoot, err)
	}
	result.FactoryDir = factoryDir
	result.Outcome = factorydefinitions.PackagedFactoryInstallCreated
	return result, nil
}

func (service *Service) replaceExistingPackagedFactory(
	ctx context.Context,
	namedFactoriesRoot string,
	name string,
	targetDir string,
	payload []byte,
	rootFileName string,
	result factorydefinitions.PackagedFactoryInstallResult,
) (factorydefinitions.PackagedFactoryInstallResult, error) {
	if err := ctx.Err(); err != nil {
		return result, installError(name, namedFactoriesRoot, err)
	}
	prepared, err := service.persistence.PreparePackagedFactoryLayout(ctx, name, payload)
	if err != nil {
		return result, installError(name, namedFactoriesRoot, err)
	}
	if prepared == nil {
		return result, installError(
			name,
			namedFactoriesRoot,
			fmt.Errorf("prepared Factory layout is required"),
		)
	}
	prepared.RootFileName = rootFileName
	factoryDir, err := service.persistence.ReplaceNamedFactory(namedFactoriesRoot, name, prepared)
	if err != nil {
		return result, installError(name, namedFactoriesRoot, err)
	}
	result.FactoryDir = factoryDir
	result.Outcome = factorydefinitions.PackagedFactoryInstallReplaced
	return result, nil
}

func authoredRootFormat(
	targetDir string,
	fileSystem factorydefinitions.PackagedInstallationFileSystem,
) (factorydefinitions.PackagedFactoryFormat, error) {
	for _, candidate := range []struct {
		file   string
		format factorydefinitions.PackagedFactoryFormat
	}{
		{factorydefinitions.FactoryConfigFile, factorydefinitions.PackagedFactoryFormatJSON},
		{"factory.yaml", factorydefinitions.PackagedFactoryFormatYAML},
		{"factory.yml", factorydefinitions.PackagedFactoryFormatYML},
	} {
		_, err := fileSystem.Stat(filepath.Join(targetDir, candidate.file))
		if err == nil {
			return candidate.format, nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return "", fmt.Errorf("inspect authored root %s: %w", candidate.file, err)
		}
	}
	return "", fmt.Errorf("no authored root definition in %s", targetDir)
}

func selectPayload(
	definition factorydefinitions.PackagedDefinition,
	format factorydefinitions.PackagedFactoryFormat,
) ([]byte, string, factorydefinitions.PackagedFactoryFormat, error) {
	if format == "" {
		format = factorydefinitions.PackagedFactoryFormatJSON
	}
	switch format {
	case factorydefinitions.PackagedFactoryFormatJSON:
		if len(definition.JSON) == 0 {
			return nil, "", "", fmt.Errorf("packaged Factory does not publish JSON")
		}
		return append([]byte(nil), definition.JSON...),
			factorydefinitions.FactoryConfigFile, format, nil
	case factorydefinitions.PackagedFactoryFormatYAML:
		if len(definition.YAML) == 0 {
			return nil, "", "", fmt.Errorf("packaged Factory does not publish YAML")
		}
		return append([]byte(nil), definition.JSON...),
			"factory.yaml", format, nil
	case factorydefinitions.PackagedFactoryFormatYML:
		if len(definition.YAML) == 0 {
			return nil, "", "", fmt.Errorf("packaged Factory does not publish YAML")
		}
		return append([]byte(nil), definition.JSON...),
			"factory.yml", format, nil
	default:
		return nil, "", "", fmt.Errorf("unsupported packaged Factory format %q", format)
	}
}

func installError(name, namedFactoriesRoot string, err error) error {
	return fmt.Errorf("install packaged factory %q in global root %s: %w", name, namedFactoriesRoot, err)
}
