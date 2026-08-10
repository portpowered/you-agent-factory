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

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	authoringlayoutpersist "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/authoring_layout/persist"
	namedfactorypath "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/catalog/namedpaths"
)

type Service struct {
	persistence factorydefinitions.Persistence
	fileSystem  factorydefinitions.PackagedInstallationFileSystem
	ownerProbe  ownerProbe
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
	persistence factorydefinitions.Persistence,
	fileSystem factorydefinitions.PackagedInstallationFileSystem,
) *Service {
	return &Service{
		persistence: persistence,
		fileSystem:  fileSystem,
		ownerProbe:  localOwnerProbe{},
	}
}

func newWithOwnerProbe(
	persistence factorydefinitions.Persistence,
	fileSystem factorydefinitions.PackagedInstallationFileSystem,
	probe ownerProbe,
) *Service {
	service := New(persistence, fileSystem)
	if probe != nil {
		service.ownerProbe = probe
	}
	return service
}

func (service *Service) EnsurePackagedFactories(
	ctx context.Context,
	namedFactoriesRoot string,
	definitions []factorydefinitions.PackagedDefinition,
) ([]factorydefinitions.PackagedFactoryInstallResult, error) {
	results := make([]factorydefinitions.PackagedFactoryInstallResult, 0, len(definitions))
	for _, definition := range definitions {
		result, err := service.InstallPackagedFactory(
			ctx,
			factorydefinitions.PackagedFactoryInstallParams{
				NamedFactoriesRoot: namedFactoriesRoot,
				Definition:         definition,
				Format:             factorydefinitions.PackagedFactoryFormatJSON,
			},
		)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	return results, nil
}

func (service *Service) InstallPackagedFactory(
	ctx context.Context,
	params factorydefinitions.PackagedFactoryInstallParams,
) (factorydefinitions.PackagedFactoryInstallResult, error) {
	definition := params.Definition
	format := params.Format
	namedFactoriesRoot := params.NamedFactoriesRoot
	result := factorydefinitions.PackagedFactoryInstallResult{
		Name:   definition.Name,
		Format: format,
	}
	if err := ctx.Err(); err != nil {
		return result, installError(definition.Name, namedFactoriesRoot, err)
	}
	payload, rootFileName, normalizedFormat, err := selectPayload(
		definition,
		format,
	)
	if err != nil {
		return result, installError(definition.Name, namedFactoriesRoot, err)
	}
	result.Format = normalizedFormat
	targetDir, err := namedfactorypath.MapDir(namedFactoriesRoot, definition.Name)
	if err != nil {
		return result, installError(definition.Name, namedFactoriesRoot, err)
	}
	result.FactoryDir = targetDir
	if err := service.requireDependencies(); err != nil {
		return result, installError(definition.Name, namedFactoriesRoot, err)
	}
	if err := service.rejectPreExistingStaging(namedFactoriesRoot, definition.Name); err != nil {
		return result, installError(definition.Name, namedFactoriesRoot, err)
	}
	if _, err := service.fileSystem.Stat(targetDir); err == nil {
		if err := service.persistence.ValidateFactoryLayout(targetDir); err != nil {
			return result, installError(definition.Name, namedFactoriesRoot, fmt.Errorf("existing target %s is invalid: %w", targetDir, err))
		}
		if params.Replace {
			return service.withStagingOwnership(
				ctx,
				namedFactoriesRoot,
				definition.Name,
				result,
				func() (factorydefinitions.PackagedFactoryInstallResult, error) {
					return service.replaceExistingPackagedFactory(
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
		existingFormat, formatErr := authoredRootFormat(targetDir, service.fileSystem)
		if formatErr != nil {
			return result, installError(definition.Name, namedFactoriesRoot, formatErr)
		}
		if existingFormat != normalizedFormat {
			return result, installError(
				definition.Name,
				namedFactoriesRoot,
				fmt.Errorf(
					"%w: factory %q already exists",
					factorydefinitions.ErrNamedFactoryAlreadyExists,
					definition.Name,
				),
			)
		}
		result.Outcome = factorydefinitions.PackagedFactoryInstallSkipped
		return result, nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return result, installError(definition.Name, namedFactoriesRoot, fmt.Errorf("inspect target %s: %w", targetDir, err))
	}
	return service.withStagingOwnership(
		ctx,
		namedFactoriesRoot,
		definition.Name,
		result,
		func() (factorydefinitions.PackagedFactoryInstallResult, error) {
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

func (service *Service) requireDependencies() error {
	if service == nil {
		return fmt.Errorf("packaged Factory installation service is required")
	}
	if service.fileSystem == nil {
		return fmt.Errorf("packaged Factory installation filesystem is required")
	}
	if service.persistence == nil {
		return fmt.Errorf("Factory Definitions persistence service is required")
	}
	if service.ownerProbe == nil {
		return fmt.Errorf("packaged Factory installation owner probe is required")
	}
	return nil
}

func (service *Service) rejectPreExistingStaging(rootDir, name string) error {
	stagingPath, err := findPreExistingStaging(service.fileSystem, rootDir, name)
	if err != nil || stagingPath == "" {
		return err
	}
	contention, retry, inspectErr := service.inspectStagingContention(rootDir, name, stagingPath)
	if inspectErr != nil {
		return inspectErr
	}
	if retry {
		return nil
	}
	return contention
}

func (service *Service) withStagingOwnership(
	ctx context.Context,
	rootDir string,
	name string,
	result factorydefinitions.PackagedFactoryInstallResult,
	operation func() (factorydefinitions.PackagedFactoryInstallResult, error),
) (installResult factorydefinitions.PackagedFactoryInstallResult, installErr error) {
	lease, err := service.acquireStagingOwnership(ctx, rootDir, name)
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
			installErr = installError(name, rootDir, installErr)
		}
	}()
	return operation()
}

type stagingLease struct {
	path  string
	root  string
	name  string
	owner ownerRecord
}

func (service *Service) acquireStagingOwnership(
	ctx context.Context,
	rootDir string,
	name string,
) (*stagingLease, error) {
	for attempt := 0; attempt < stagingAcquireAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		stagingPath, err := findPreExistingStaging(service.fileSystem, rootDir, name)
		if err != nil {
			return nil, err
		}
		if stagingPath != "" {
			contention, retry, inspectErr := service.inspectStagingContention(
				rootDir,
				name,
				stagingPath,
			)
			if inspectErr != nil {
				return nil, inspectErr
			}
			if retry {
				continue
			}
			return nil, contention
		}
		if err := service.fileSystem.MkdirAll(rootDir, 0o755); err != nil {
			return nil, installationFailure(rootDir, name, "", ownerLivenessIndeterminate, err)
		}
		leasePath := stagingOwnershipPath(rootDir, name)
		if err := service.fileSystem.Mkdir(leasePath, 0o755); err != nil {
			if errors.Is(err, fs.ErrExist) {
				continue
			}
			return nil, installationFailure(rootDir, name, leasePath, ownerLivenessIndeterminate, err)
		}
		owner, err := service.ownerProbe.Current()
		if err != nil {
			_ = service.fileSystem.RemoveAll(leasePath)
			return nil, installationFailure(rootDir, name, leasePath, ownerLivenessIndeterminate, err)
		}
		if owner.PID <= 0 {
			_ = service.fileSystem.RemoveAll(leasePath)
			return nil, installationFailure(
				rootDir,
				name,
				leasePath,
				ownerLivenessIndeterminate,
				fmt.Errorf("owner PID is invalid: %d", owner.PID),
			)
		}
		if err := service.publishOwnerRecord(leasePath, owner); err != nil {
			_ = service.fileSystem.RemoveAll(leasePath)
			return nil, installationFailure(rootDir, name, leasePath, ownerLivenessIndeterminate, err)
		}
		return &stagingLease{path: leasePath, root: rootDir, name: name, owner: owner}, nil
	}
	return nil, installationContention(
		rootDir,
		name,
		stagingOwnershipPath(rootDir, name),
		"failed",
		ownerLivenessRacing,
		0,
	)
}

func (service *Service) inspectStagingContention(
	rootDir string,
	name string,
	stagingPath string,
) (error, bool, error) {
	if _, err := service.fileSystem.Stat(stagingPath); errors.Is(err, fs.ErrNotExist) {
		return nil, true, nil
	} else if err != nil {
		return nil, false, fmt.Errorf("inspect packaged Factory staging resource %s: %w", stagingPath, err)
	}
	if stagingPath != stagingOwnershipPath(rootDir, name) {
		return installationContention(
			rootDir,
			name,
			stagingPath,
			"indeterminate-contention",
			ownerLivenessIndeterminate,
			0,
		), false, nil
	}
	owner, liveness, err := service.readOwnerRecord(stagingPath)
	if errors.Is(err, fs.ErrNotExist) {
		if _, statErr := service.fileSystem.Stat(stagingPath); errors.Is(statErr, fs.ErrNotExist) {
			return nil, true, nil
		}
		return installationContention(
			rootDir,
			name,
			stagingPath,
			"indeterminate-contention",
			ownerLivenessIndeterminate,
			0,
		), false, nil
	}
	if err != nil {
		return installationContention(
			rootDir,
			name,
			stagingPath,
			"indeterminate-contention",
			liveness,
			owner.PID,
		), false, nil
	}
	if liveness == ownerLivenessActive {
		return installationContention(
			rootDir,
			name,
			stagingPath,
			"active-contention",
			liveness,
			owner.PID,
		), false, nil
	}
	return installationContention(
		rootDir,
		name,
		stagingPath,
		"indeterminate-contention",
		liveness,
		owner.PID,
	), false, nil
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
		return installationFailure(
			lease.root,
			lease.name,
			lease.path,
			ownerLivenessIndeterminate,
			err,
		)
	}
	if owner != lease.owner {
		return installationFailure(
			lease.root,
			lease.name,
			lease.path,
			ownerLivenessRacing,
			fmt.Errorf("staging owner changed from PID %d to PID %d", lease.owner.PID, owner.PID),
		)
	}
	if err := service.fileSystem.RemoveAll(lease.path); err != nil {
		return installationFailure(
			lease.root,
			lease.name,
			lease.path,
			ownerLivenessActive,
			err,
		)
	}
	return nil
}

func stagingOwnershipPath(rootDir, name string) string {
	return filepath.Join(
		rootDir,
		authoringlayoutpersist.StagingDirectoryPrefix(name)+stagingOwnerSuffix,
	)
}

func installationContention(
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
		"%w: scope=%s resource=%s outcome=%s owner_liveness=%s %s; verify no you process is still installing packaged Factory %q, stop or verify that owner before retrying, then remove only %s and retry",
		factorydefinitions.ErrFactoryInstallationContention,
		rootDir,
		resource,
		outcome,
		liveness,
		ownerEvidence,
		name,
		resource,
	)
}

func installationFailure(
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
		"%w: scope=%s resource=%s outcome=failed owner_liveness=%s; verify no you process is still installing packaged Factory %q, stop or verify that owner before retrying, then remove only %s and retry: %w",
		factorydefinitions.ErrFactoryInstallationContention,
		rootDir,
		resource,
		liveness,
		name,
		resource,
		cause,
	)
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
	prepared, err := service.persistence.PrepareFactoryLayout(ctx, name, payload)
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
	prepared, err := service.persistence.PrepareFactoryLayout(ctx, name, payload)
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
