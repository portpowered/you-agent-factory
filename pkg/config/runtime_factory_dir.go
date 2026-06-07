package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

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
	name, err := NamedFactoryLayoutSegmentToName(segment)
	if err != nil {
		return "", fmt.Errorf("read current factory pointer %s: %w", path, err)
	}
	return name, nil
}

// WriteCurrentFactoryPointer persists the selected named factory for later
// restart-time resolution.
func WriteCurrentFactoryPointer(rootDir, name string) error {
	if strings.TrimSpace(rootDir) == "" {
		return fmt.Errorf("factory root is required")
	}

	segment, err := NamedFactoryNameToLayoutSegment(name)
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

	canonicalName, err := canonicalNamedFactoryName(name)
	if err != nil {
		return "", err
	}
	segment, err := NamedFactoryNameToLayoutSegment(canonicalName)
	if err != nil {
		return "", err
	}

	factoryDir := filepath.Join(rootDir, segment)
	if err := requireFactoryConfig(factoryDir); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf(
				"resolve named factory %q in root %s: %w",
				canonicalName,
				rootDir,
				newNamedFactoryNotFoundError(canonicalName),
			)
		}
		return "", fmt.Errorf("resolve named factory %q in root %s: %w", canonicalName, rootDir, err)
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
