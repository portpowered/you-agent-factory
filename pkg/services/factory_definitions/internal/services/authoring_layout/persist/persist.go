// Package persist owns atomic create and replace orchestration for one prepared
// Factory aggregate layout.
package persist

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

// Ports are the durable-write collaborators required to stage and commit one
// named Factory layout.
type Ports struct {
	Write                func(string, *factorydefinitions.PreparedFactoryLayoutPayload, string) error
	Validate             func(string) error
	FileSystem           factorydefinitions.PersistenceFileSystem
	RequireDefinitionDir factorydefinitions.DefinitionDirectoryRequirer
	Directories          factorydefinitions.DirectoryReplacementStore
}

// NamedFactory atomically creates or replaces one named Factory layout under
// rootDir using staging and commit semantics.
func NamedFactory(
	ctx context.Context,
	rootDir string,
	name string,
	prepared *factorydefinitions.PreparedFactoryLayoutPayload,
	replaceExisting bool,
	ports Ports,
) (string, error) {
	if ports.Write == nil {
		return "", fmt.Errorf("Factory Definitions layout writer is required")
	}
	if ports.Validate == nil {
		return "", fmt.Errorf("Factory Definitions layout validator is required")
	}
	if ports.FileSystem == nil {
		return "", fmt.Errorf("Factory Definitions persistence filesystem is required")
	}
	if ports.RequireDefinitionDir == nil {
		return "", fmt.Errorf("Factory Definition directory validator is required")
	}
	if ports.Directories == nil {
		return "", fmt.Errorf("directory replacement store is required")
	}
	if err := ctx.Err(); err != nil {
		return "", err
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
	if err := validateTarget(ports.FileSystem, targetDir, canonicalName, replaceExisting); err != nil {
		return "", err
	}
	if err := ensureParentDirectories(ports.FileSystem, rootDir, targetDir); err != nil {
		return "", err
	}

	stagingDir, err := ports.FileSystem.MkdirTemp(rootDir, stagingPrefix(canonicalName))
	if err != nil {
		return "", fmt.Errorf("create staging directory for factory %q: %w", canonicalName, err)
	}
	keepStaging := false
	defer func() {
		if !keepStaging {
			_ = ports.FileSystem.RemoveAll(stagingDir)
		}
	}()

	sourcePath := filepath.Join(targetDir, factorydefinitions.FactoryConfigFile)
	if err := ports.Write(stagingDir, prepared, sourcePath); err != nil {
		return "", fmt.Errorf("%w: %w", factorydefinitions.ErrInvalidNamedFactory, err)
	}
	if err := ports.Validate(stagingDir); err != nil {
		return "", fmt.Errorf(
			"%w: validate factory %q config: %w",
			factorydefinitions.ErrInvalidNamedFactory,
			canonicalName,
			err,
		)
	}
	if replaceExisting {
		if err := replaceDirectory(ports, stagingDir, targetDir, canonicalName); err != nil {
			return "", err
		}
	} else if err := ports.FileSystem.Rename(stagingDir, targetDir); err != nil {
		return "", fmt.Errorf("commit factory %q: %w", canonicalName, err)
	}
	keepStaging = true
	return targetDir, nil
}

func validateTarget(
	fileSystem factorydefinitions.PersistenceFileSystem,
	targetDir, name string,
	replaceExisting bool,
) error {
	if _, err := fileSystem.Stat(targetDir); err == nil {
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

func ensureParentDirectories(
	fileSystem factorydefinitions.PersistenceFileSystem,
	rootDir, targetDir string,
) error {
	if err := fileSystem.MkdirAll(rootDir, 0o755); err != nil {
		return fmt.Errorf("create factory root %s: %w", rootDir, err)
	}
	parentDir := filepath.Dir(targetDir)
	if parentDir == rootDir {
		return nil
	}
	if err := fileSystem.MkdirAll(parentDir, 0o755); err != nil {
		return fmt.Errorf("create factory parent directory %s: %w", parentDir, err)
	}
	return nil
}

func replaceDirectory(
	ports Ports,
	stagingDir, targetDir, name string,
) error {
	backupDir, err := ports.Directories.Commit(
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
	return ports.FileSystem.RemoveAll(backupDir)
}

func stagingPrefix(name string) string {
	safe := strings.NewReplacer("/", "--", `\`, "--", "@", "").
		Replace(strings.TrimSpace(name))
	if safe == "" {
		safe = "factory"
	}
	return "." + safe + ".staging-"
}
