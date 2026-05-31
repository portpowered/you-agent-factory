package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

type factorySplitLayoutReplaceHooks struct {
	afterStageWrite func(stagingDir string) error
}

// ReplaceFactorySplitLayout atomically replaces an existing factory directory at
// targetDir with a split-layout materialization of canonicalFactoryJSON. The
// returned restore function reinstates the pre-replace directory tree when
// activation or downstream steps fail after persist.
func ReplaceFactorySplitLayout(targetDir string, canonicalFactoryJSON []byte) (func(), error) {
	return replaceFactorySplitLayout(targetDir, canonicalFactoryJSON, factorySplitLayoutReplaceHooks{})
}

func replaceFactorySplitLayout(targetDir string, canonicalFactoryJSON []byte, hooks factorySplitLayoutReplaceHooks) (func(), error) {
	if strings.TrimSpace(targetDir) == "" {
		return nil, fmt.Errorf("factory directory is required")
	}
	if err := requireFactoryConfig(targetDir); err != nil {
		return nil, fmt.Errorf("replace factory split layout: %w", err)
	}

	segment := filepath.Base(targetDir)
	parentDir := filepath.Dir(targetDir)
	factoryCfg, canonical, err := normalizeNamedFactoryPayload(segment, canonicalFactoryJSON)
	if err != nil {
		return nil, err
	}

	sourcePath := filepath.Join(targetDir, interfaces.FactoryConfigFile)
	stagingDir, err := os.MkdirTemp(parentDir, "."+segment+".staging-")
	if err != nil {
		return nil, fmt.Errorf("create staging directory for factory %q: %w", segment, err)
	}
	keepStaging := false
	defer func() {
		if !keepStaging {
			_ = os.RemoveAll(stagingDir)
		}
	}()

	if _, err := writeNamedFactoryLayout(stagingDir, factoryCfg, canonical, sourcePath); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidNamedFactory, err)
	}
	if hooks.afterStageWrite != nil {
		if err := hooks.afterStageWrite(stagingDir); err != nil {
			return nil, fmt.Errorf("prepare staged factory %q: %w", segment, err)
		}
	}
	if _, err := LoadRuntimeConfig(stagingDir, nil); err != nil {
		return nil, fmt.Errorf("%w: validate factory %q config: %w", ErrInvalidNamedFactory, segment, err)
	}

	backupDir, err := os.MkdirTemp(parentDir, "."+segment+".previous-")
	if err != nil {
		return nil, fmt.Errorf("prepare replacement backup for factory %q: %w", segment, err)
	}
	if err := os.Remove(backupDir); err != nil {
		return nil, fmt.Errorf("prepare replacement backup for factory %q: %w", segment, err)
	}

	if err := os.Rename(targetDir, backupDir); err != nil {
		return nil, fmt.Errorf("backup existing factory %q: %w", segment, err)
	}
	committed := false
	defer func() {
		if !committed {
			if restoreErr := os.Rename(backupDir, targetDir); restoreErr != nil {
				return
			}
			_ = os.RemoveAll(backupDir)
		}
	}()

	if err := os.Rename(stagingDir, targetDir); err != nil {
		return nil, fmt.Errorf("commit factory %q: %w", segment, err)
	}
	keepStaging = true
	committed = true

	restore := func() {
		restoreFactorySplitLayoutReplace(targetDir, backupDir)
	}
	return restore, nil
}

func restoreFactorySplitLayoutReplace(targetDir, backupDir string) {
	if strings.TrimSpace(targetDir) == "" || strings.TrimSpace(backupDir) == "" {
		return
	}
	if _, err := os.Stat(backupDir); err != nil {
		return
	}

	parentDir := filepath.Dir(targetDir)
	segment := filepath.Base(targetDir)
	trashDir, err := os.MkdirTemp(parentDir, "."+segment+".rollback-trash-")
	if err != nil {
		return
	}
	if err := os.Remove(trashDir); err != nil {
		return
	}

	if err := os.Rename(targetDir, trashDir); err != nil {
		_ = os.RemoveAll(trashDir)
		return
	}
	if err := os.Rename(backupDir, targetDir); err != nil {
		_ = os.Rename(trashDir, targetDir)
		return
	}
	_ = os.RemoveAll(trashDir)
}
