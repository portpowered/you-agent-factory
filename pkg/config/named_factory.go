package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

// ErrFactoryLayoutNotFound reports that a directory does not contain either a
// legacy single-factory layout or a named-factory current-pointer layout.
var ErrFactoryLayoutNotFound = errors.New("factory layout not found")

// ErrNamedFactoryAlreadyExists reports that the requested named-factory target
// already exists on disk.
var ErrNamedFactoryAlreadyExists = errors.New("named factory already exists")

// ErrInvalidNamedFactory reports that the submitted named-factory payload could
// not be normalized into a runnable named-factory layout.
var ErrInvalidNamedFactory = errors.New("invalid named factory")

// ValidateNamedFactoryName applies the canonical safe directory-segment rules
// used by the named-factory on-disk layout.
func ValidateNamedFactoryName(name string) error {
	_, err := safeFactoryLayoutSegment("factory", name)
	return err
}

// PersistNamedFactory materializes a compact canonical factory payload under a
// named subdirectory rooted at rootDir.
func PersistNamedFactory(rootDir, name string, canonicalFactoryJSON []byte) (string, error) {
	return persistNamedFactory(rootDir, name, canonicalFactoryJSON, namedFactoryPersistHooks{})
}

type namedFactoryPersistHooks struct {
	afterWrite        func(stagingDir string) error
	loadRuntimeConfig func(factoryDir string, workstationLoader WorkstationLoader) (*LoadedFactoryConfig, error)
}

func persistNamedFactory(rootDir, name string, canonicalFactoryJSON []byte, hooks namedFactoryPersistHooks) (string, error) {
	if strings.TrimSpace(rootDir) == "" {
		return "", fmt.Errorf("factory root is required")
	}

	segment, err := safeFactoryLayoutSegment("factory", name)
	if err != nil {
		return "", err
	}

	targetDir := filepath.Join(rootDir, segment)
	if _, err := os.Stat(targetDir); err == nil {
		return "", fmt.Errorf("%w: factory %q already exists", ErrNamedFactoryAlreadyExists, segment)
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("check existing factory %q: %w", segment, err)
	}
	if err := os.MkdirAll(rootDir, 0o755); err != nil {
		return "", fmt.Errorf("create factory root %s: %w", rootDir, err)
	}

	mapper := NewFactoryConfigMapper()
	factoryCfg, err := mapper.Expand(canonicalFactoryJSON)
	if err != nil {
		return "", fmt.Errorf("%w: parse factory %q config: %v", ErrInvalidNamedFactory, segment, err)
	}
	authoredFactoryCfg, err := authoredFactoryConfigForExpandedLayout(factoryCfg)
	if err != nil {
		return "", fmt.Errorf("%w: normalize authored factory %q config: %v", ErrInvalidNamedFactory, segment, err)
	}
	canonical, err := mapper.Flatten(authoredFactoryCfg)
	if err != nil {
		return "", fmt.Errorf("%w: normalize factory %q config: %v", ErrInvalidNamedFactory, segment, err)
	}

	sourcePath := filepath.Join(targetDir, interfaces.FactoryConfigFile)
	stagingDir, err := os.MkdirTemp(rootDir, "."+segment+".staging-")
	if err != nil {
		return "", fmt.Errorf("create staging directory for factory %q: %w", segment, err)
	}
	keepStaging := false
	defer func() {
		if !keepStaging {
			_ = os.RemoveAll(stagingDir)
		}
	}()

	if err := writeNamedFactoryLayout(stagingDir, factoryCfg, canonical, sourcePath); err != nil {
		return "", fmt.Errorf("%w: %v", ErrInvalidNamedFactory, err)
	}
	if hooks.afterWrite != nil {
		if err := hooks.afterWrite(stagingDir); err != nil {
			return "", fmt.Errorf("prepare staged factory %q: %w", segment, err)
		}
	}
	loadRuntimeConfig := hooks.loadRuntimeConfig
	if loadRuntimeConfig == nil {
		loadRuntimeConfig = LoadRuntimeConfig
	}
	if _, err := loadRuntimeConfig(stagingDir, nil); err != nil {
		return "", fmt.Errorf("%w: validate factory %q config: %v", ErrInvalidNamedFactory, segment, err)
	}
	if err := os.Rename(stagingDir, targetDir); err != nil {
		return "", fmt.Errorf("commit factory %q: %w", segment, err)
	}
	keepStaging = true
	return targetDir, nil
}

// ReadCurrentFactoryPointer returns the current named factory selected for the
// root directory's named-factory layout.
func ReadCurrentFactoryPointer(rootDir string) (string, error) {
	path := filepath.Join(rootDir, interfaces.CurrentFactoryPointerFile)
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	segment, err := safeFactoryLayoutSegment("factory", string(data))
	if err != nil {
		return "", fmt.Errorf("read current factory pointer %s: %w", path, err)
	}
	return segment, nil
}

// WriteCurrentFactoryPointer persists the selected named factory for later
// restart-time resolution.
func WriteCurrentFactoryPointer(rootDir, name string) error {
	if strings.TrimSpace(rootDir) == "" {
		return fmt.Errorf("factory root is required")
	}

	segment, err := safeFactoryLayoutSegment("factory", name)
	if err != nil {
		return err
	}
	if err := requireFactoryConfig(filepath.Join(rootDir, segment)); err != nil {
		return fmt.Errorf("set current factory %q: %w", segment, err)
	}
	if err := os.MkdirAll(rootDir, 0o755); err != nil {
		return fmt.Errorf("create factory root %s: %w", rootDir, err)
	}

	path := filepath.Join(rootDir, interfaces.CurrentFactoryPointerFile)
	if err := os.WriteFile(path, []byte(segment+"\n"), 0o644); err != nil {
		return fmt.Errorf("write current factory pointer %s: %w", path, err)
	}
	return nil
}

// ResolveNamedFactoryDir returns the canonical on-disk directory for a
// persisted named factory rooted under rootDir.
func ResolveNamedFactoryDir(rootDir, name string) (string, error) {
	if strings.TrimSpace(rootDir) == "" {
		return "", fmt.Errorf("factory root is required")
	}

	segment, err := safeFactoryLayoutSegment("factory", name)
	if err != nil {
		return "", err
	}

	factoryDir := filepath.Join(rootDir, segment)
	if err := requireFactoryConfig(factoryDir); err != nil {
		return "", fmt.Errorf("resolve factory %q: %w", segment, err)
	}
	return factoryDir, nil
}

// ResolveCurrentFactoryDir returns the directory that should be treated as the
// active runtime root. A persisted current-pointer layout takes precedence over
// a legacy single-factory root.
func ResolveCurrentFactoryDir(rootDir string) (string, error) {
	if strings.TrimSpace(rootDir) == "" {
		return "", fmt.Errorf("factory root is required")
	}

	if name, err := ReadCurrentFactoryPointer(rootDir); err == nil {
		return ResolveNamedFactoryDir(rootDir, name)
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}

	if err := requireFactoryConfig(rootDir); err == nil {
		return rootDir, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}

	return "", fmt.Errorf("resolve current factory in %s: %w", rootDir, ErrFactoryLayoutNotFound)
}

func writeNamedFactoryLayout(targetDir string, cfg *interfaces.FactoryConfig, canonical []byte, sourcePath string) error {
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return fmt.Errorf("create factory directory %s: %w", targetDir, err)
	}

	formatted, err := formatCanonicalFactoryJSON(canonical, sourcePath)
	if err != nil {
		return err
	}
	factoryPath := filepath.Join(targetDir, interfaces.FactoryConfigFile)
	if err := os.WriteFile(factoryPath, formatted, 0o644); err != nil {
		return fmt.Errorf("write canonical factory config %s: %w", factoryPath, err)
	}
	if err := writeExpandedWorkerFiles(targetDir, cfg.Workers); err != nil {
		return err
	}
	if err := writeExpandedWorkstationFiles(targetDir, cfg.Workstations); err != nil {
		return err
	}
	inputsDir := filepath.Join(targetDir, interfaces.InputsDir)
	if err := os.MkdirAll(inputsDir, 0o755); err != nil {
		return fmt.Errorf("create inputs directory %s: %w", inputsDir, err)
	}
	return nil
}

func requireFactoryConfig(factoryDir string) error {
	factoryPath := filepath.Join(factoryDir, interfaces.FactoryConfigFile)
	info, err := os.Stat(factoryPath)
	if err != nil {
		return fmt.Errorf("find factory config %s: %w", factoryPath, err)
	}
	if info.IsDir() {
		return fmt.Errorf("factory config %s is a directory", factoryPath)
	}
	return nil
}
