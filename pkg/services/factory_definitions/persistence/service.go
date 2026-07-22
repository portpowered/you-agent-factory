// Package persistence implements Factory Definition layout persistence.
package persistence

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	namedfactorypath "github.com/portpowered/infinite-you/pkg/services/factory_definitions/namedpaths"
)

type service struct {
	validator            factorydefinitions.Validator
	validateDefinition   factorydefinitions.DefinitionValidationOperation
	mapInput             factorydefinitions.FactoryLayoutPayloadMapper
	prepare              factorydefinitions.FactoryLayoutPayloadPreparer
	write                func(string, *factorydefinitions.PreparedFactoryLayoutPayload, string) error
	validate             func(string) error
	flatten              factorydefinitions.FactoryLayoutFlattener
	expand               factorydefinitions.FactoryLayoutExpander
	writeCurrent         factorydefinitions.CurrentFactoryPointerWriter
	fileSystem           factorydefinitions.PersistenceFileSystem
	requireDefinitionDir factorydefinitions.DefinitionDirectoryRequirer
	directories          factorydefinitions.DirectoryReplacementStore
}

// New constructs the Factory Definitions persistence implementation from flat
// serialization and filesystem capabilities selected by Wire.
func New(
	validator factorydefinitions.Validator,
	mapInput factorydefinitions.FactoryLayoutPayloadMapper,
	prepare factorydefinitions.FactoryLayoutPayloadPreparer,
	write func(string, *factorydefinitions.PreparedFactoryLayoutPayload, string) error,
	validate func(string) error,
	flatten factorydefinitions.FactoryLayoutFlattener,
	expand factorydefinitions.FactoryLayoutExpander,
	writeCurrent factorydefinitions.CurrentFactoryPointerWriter,
	fileSystem factorydefinitions.PersistenceFileSystem,
	requireDefinitionDir factorydefinitions.DefinitionDirectoryRequirer,
	directories factorydefinitions.DirectoryReplacementStore,
) (factorydefinitions.Persistence, error) {
	if fileSystem == nil {
		return nil, fmt.Errorf("Factory Definitions persistence filesystem is required")
	}
	if requireDefinitionDir == nil {
		return nil, fmt.Errorf("Factory Definition directory validator is required")
	}
	validateDefinition, _ := any(validator).(factorydefinitions.DefinitionValidationOperation)
	return &service{
		validator:            validator,
		validateDefinition:   validateDefinition,
		mapInput:             mapInput,
		prepare:              prepare,
		write:                write,
		validate:             validate,
		flatten:              flatten,
		expand:               expand,
		writeCurrent:         writeCurrent,
		fileSystem:           fileSystem,
		requireDefinitionDir: requireDefinitionDir,
		directories:          directories,
	}, nil
}

func (s *service) PersistNamedFactory(
	ctx context.Context,
	request factorydefinitions.NamedFactoryPersistenceRequest,
) (factorydefinitions.NamedFactoryPersistenceResult, error) {
	name := strings.TrimSpace(request.Name)
	result := factorydefinitions.NamedFactoryPersistenceResult{Name: name}
	if name == "" {
		return result, fmt.Errorf("factory name is required")
	}
	rootDir := strings.TrimSpace(request.RootDir)
	if rootDir == "" {
		return result, fmt.Errorf("factory root is required")
	}
	if err := namedfactorypath.ValidateName(name); err != nil {
		return result, err
	}
	factoryDir, err := namedfactorypath.MapDir(rootDir, name)
	if err != nil {
		return result, err
	}
	result.FactoryDir = factoryDir
	if s == nil {
		return result, fmt.Errorf("Factory Definitions persistence service is required")
	}
	prepared, err := s.PrepareFactoryLayout(ctx, name, request.Payload)
	if err != nil {
		return result, err
	}
	switch request.Mode {
	case factorydefinitions.NamedFactoryPersistenceModeCreate:
		result.FactoryDir, err = s.CreateNamedFactory(rootDir, name, prepared)
	case factorydefinitions.NamedFactoryPersistenceModeReplace:
		result.FactoryDir, err = s.ReplaceNamedFactory(rootDir, name, prepared)
	default:
		return result, fmt.Errorf("unsupported named Factory persistence mode %q", request.Mode)
	}
	if err != nil {
		return result, err
	}
	if request.Mode == factorydefinitions.NamedFactoryPersistenceModeCreate &&
		request.SetCurrent {
		if s.writeCurrent == nil {
			return result, fmt.Errorf("current Factory pointer writer is required")
		}
		if err := s.writeCurrent(rootDir, name); err != nil {
			return result, err
		}
	}
	return result, nil
}

func (s *service) PrepareFactoryLayout(
	ctx context.Context,
	segment string,
	payload []byte,
) (*factorydefinitions.PreparedFactoryLayoutPayload, error) {
	if s == nil || s.prepare == nil {
		return nil, fmt.Errorf("Factory Definitions layout preparer is required")
	}
	if s.validator == nil {
		return nil, fmt.Errorf("Factory Definitions validator is required")
	}
	if s.validateDefinition == nil {
		return nil, fmt.Errorf("Factory Definitions validation operation is required")
	}
	if s.mapInput == nil {
		return nil, fmt.Errorf("Factory Definitions payload mapper is required")
	}
	request, err := s.mapInput(payload)
	if err != nil {
		return nil, err
	}
	request.Profile = factorydefinitions.ValidationProfilePrePersist
	result, err := s.validateDefinition.ValidateDefinition(ctx, request)
	if err != nil {
		if errors.Is(err, factorydefinitions.ErrInvalidNamedFactory) {
			return nil, err
		}
		return nil, fmt.Errorf("%w: %v", factorydefinitions.ErrInvalidNamedFactory, err)
	}
	if targets := result.BlockingTargets(); len(targets) != 0 {
		return nil, factorydefinitions.NewBlockingFactoryLoadError(
			factorydefinitions.ValidationResult{Targets: targets},
		)
	}
	return s.prepare(ctx, segment, payload, s.validator)
}

func (s *service) ValidateFactoryLayout(targetDir string) error {
	if s == nil || s.validate == nil {
		return fmt.Errorf("Factory Definitions layout validator is required")
	}
	return s.validate(targetDir)
}

func (s *service) FlattenFactoryLayout(path string) ([]byte, error) {
	if s == nil || s.flatten == nil {
		return nil, fmt.Errorf("Factory Definitions layout flattener is required")
	}
	return s.flatten(path)
}

func (s *service) ExpandFactoryLayout(
	path string,
) (string, factorydefinitions.LayoutExpansionReport, error) {
	if s == nil || s.expand == nil {
		return "", factorydefinitions.LayoutExpansionReport{},
			fmt.Errorf("Factory Definitions layout expander is required")
	}
	return s.expand(path)
}

func (s *service) CreateNamedFactory(
	rootDir string,
	name string,
	prepared *factorydefinitions.PreparedFactoryLayoutPayload,
) (string, error) {
	return s.persistNamedFactory(rootDir, name, prepared, false)
}

func (s *service) ReplaceNamedFactory(
	rootDir string,
	name string,
	prepared *factorydefinitions.PreparedFactoryLayoutPayload,
) (string, error) {
	return s.persistNamedFactory(rootDir, name, prepared, true)
}

func (s *service) ReplaceFactoryLayout(
	targetDir string,
	prepared *factorydefinitions.PreparedFactoryLayoutPayload,
) (*factorydefinitions.FactorySplitLayoutReplaceResult, error) {
	if s == nil || s.write == nil {
		return nil, fmt.Errorf("Factory Definitions layout writer is required")
	}
	if s.validate == nil {
		return nil, fmt.Errorf("Factory Definitions layout validator is required")
	}
	if prepared == nil {
		return nil, fmt.Errorf("prepared Factory layout payload is required")
	}
	if strings.TrimSpace(targetDir) == "" {
		return nil, fmt.Errorf("factory directory is required")
	}
	if err := s.requireDefinitionDir(targetDir); err != nil {
		return nil, fmt.Errorf("replace factory layout at dir: %w", err)
	}

	segment := filepath.Base(targetDir)
	parentDir := filepath.Dir(targetDir)
	stagingDir, err := s.fileSystem.MkdirTemp(parentDir, "."+segment+".staging-")
	if err != nil {
		return nil, fmt.Errorf(
			"create staging directory for factory %q: %w",
			segment,
			err,
		)
	}
	defer func() {
		_ = s.fileSystem.RemoveAll(stagingDir)
	}()

	sourcePath := filepath.Join(targetDir, factorydefinitions.FactoryConfigFile)
	if err := s.write(stagingDir, prepared, sourcePath); err != nil {
		return nil, fmt.Errorf("%w: %w", factorydefinitions.ErrInvalidNamedFactory, err)
	}
	if err := s.validate(stagingDir); err != nil {
		return nil, fmt.Errorf(
			"%w: validate factory %q config: %w",
			factorydefinitions.ErrInvalidNamedFactory,
			segment,
			err,
		)
	}

	if s.directories == nil {
		return nil, fmt.Errorf("directory replacement store is required")
	}
	backupDir, err := s.directories.Commit(
		parentDir,
		targetDir,
		stagingDir,
	)
	if err != nil {
		return nil, fmt.Errorf("commit factory %q: %w", segment, err)
	}
	return &factorydefinitions.FactorySplitLayoutReplaceResult{
		Restore: func() {
			s.directories.Restore(targetDir, backupDir)
		},
		DiscardBackup: func() {
			_ = s.fileSystem.RemoveAll(backupDir)
		},
	}, nil
}

func (s *service) persistNamedFactory(
	rootDir string,
	name string,
	prepared *factorydefinitions.PreparedFactoryLayoutPayload,
	replaceExisting bool,
) (string, error) {
	if s == nil || s.write == nil {
		return "", fmt.Errorf("Factory Definitions layout writer is required")
	}
	if s.validate == nil {
		return "", fmt.Errorf("Factory Definitions layout validator is required")
	}
	if prepared == nil {
		return "", fmt.Errorf("prepared Factory layout payload is required")
	}
	if strings.TrimSpace(rootDir) == "" {
		return "", fmt.Errorf("factory root is required")
	}
	if err := namedfactorypath.ValidateName(name); err != nil {
		return "", err
	}

	canonicalName := strings.TrimSpace(name)
	targetDir, err := namedfactorypath.MapDir(rootDir, canonicalName)
	if err != nil {
		return "", err
	}
	if err := s.validateTarget(targetDir, canonicalName, replaceExisting); err != nil {
		return "", err
	}
	if err := s.ensureParentDirectories(rootDir, targetDir); err != nil {
		return "", err
	}

	stagingDir, err := s.fileSystem.MkdirTemp(rootDir, stagingPrefix(canonicalName))
	if err != nil {
		return "", fmt.Errorf("create staging directory for factory %q: %w", canonicalName, err)
	}
	keepStaging := false
	defer func() {
		if !keepStaging {
			_ = s.fileSystem.RemoveAll(stagingDir)
		}
	}()

	sourcePath := filepath.Join(targetDir, factorydefinitions.FactoryConfigFile)
	if err := s.write(stagingDir, prepared, sourcePath); err != nil {
		return "", fmt.Errorf("%w: %w", factorydefinitions.ErrInvalidNamedFactory, err)
	}
	if err := s.validate(stagingDir); err != nil {
		return "", fmt.Errorf(
			"%w: validate factory %q config: %w",
			factorydefinitions.ErrInvalidNamedFactory,
			canonicalName,
			err,
		)
	}
	if replaceExisting {
		if err := s.replaceDirectory(stagingDir, targetDir, canonicalName); err != nil {
			return "", err
		}
	} else if err := s.fileSystem.Rename(stagingDir, targetDir); err != nil {
		return "", fmt.Errorf("commit factory %q: %w", canonicalName, err)
	}
	keepStaging = true
	return targetDir, nil
}

func (s *service) validateTarget(targetDir, name string, replaceExisting bool) error {
	if _, err := s.fileSystem.Stat(targetDir); err == nil {
		if replaceExisting {
			return nil
		}
		return fmt.Errorf(
			"%w: factory %q already exists",
			factorydefinitions.ErrNamedFactoryAlreadyExists,
			name,
		)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("check existing factory %q: %w", name, err)
	}
	if replaceExisting {
		return fmt.Errorf("replace factory %q: %w", name, fs.ErrNotExist)
	}
	return nil
}

func (s *service) ensureParentDirectories(rootDir, targetDir string) error {
	if err := s.fileSystem.MkdirAll(rootDir, 0o755); err != nil {
		return fmt.Errorf("create factory root %s: %w", rootDir, err)
	}
	parentDir := filepath.Dir(targetDir)
	if parentDir == rootDir {
		return nil
	}
	if err := s.fileSystem.MkdirAll(parentDir, 0o755); err != nil {
		return fmt.Errorf("create factory parent directory %s: %w", parentDir, err)
	}
	return nil
}

func (s *service) replaceDirectory(stagingDir, targetDir, name string) error {
	if s.directories == nil {
		return fmt.Errorf("directory replacement store is required")
	}
	backupDir, err := s.directories.Commit(
		filepath.Dir(targetDir),
		targetDir,
		stagingDir,
	)
	if err != nil {
		return fmt.Errorf("commit factory %q: %w", name, err)
	}
	if backupDir == "" {
		return nil
	}
	return s.fileSystem.RemoveAll(backupDir)
}

func stagingPrefix(name string) string {
	safe := strings.NewReplacer("/", "--", `\`, "--", "@", "").
		Replace(strings.TrimSpace(name))
	if safe == "" {
		safe = "factory"
	}
	return "." + safe + ".staging-"
}

var _ factorydefinitions.Persistence = (*service)(nil)
