package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

// ReplaceDefaultFactoryDefinition atomically replaces the legacy single-file
// factory.json at rootDir with payload. The returned restore function reverts
// the on-disk definition when activation fails after persist.
func ReplaceDefaultFactoryDefinition(rootDir string, payload []byte) (func(), error) {
	path := filepath.Join(rootDir, interfaces.FactoryConfigFile)
	previous, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read default factory definition %s: %w", path, err)
	}
	if err := writeFactoryDefinitionFile(path, payload); err != nil {
		return nil, err
	}
	return func() {
		_ = writeFactoryDefinitionFile(path, previous)
	}, nil
}

func writeFactoryDefinitionFile(path string, payload []byte) error {
	staged, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".staging-")
	if err != nil {
		return fmt.Errorf("stage factory definition %s: %w", path, err)
	}
	stagedPath := staged.Name()
	committed := false
	defer func() {
		if !committed {
			_ = os.Remove(stagedPath)
		}
	}()
	if _, err := staged.Write(payload); err != nil {
		_ = staged.Close()
		return fmt.Errorf("write staged factory definition %s: %w", stagedPath, err)
	}
	if err := staged.Chmod(0o644); err != nil {
		_ = staged.Close()
		return fmt.Errorf("chmod staged factory definition %s: %w", stagedPath, err)
	}
	if err := staged.Close(); err != nil {
		return fmt.Errorf("close staged factory definition %s: %w", stagedPath, err)
	}
	if err := os.Rename(stagedPath, path); err != nil {
		return fmt.Errorf("replace factory definition %s: %w", path, err)
	}
	committed = true
	return nil
}
