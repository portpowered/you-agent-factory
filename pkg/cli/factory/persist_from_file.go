package factory

import (
	"errors"
	"fmt"
	"os"
	"strings"

	configload "github.com/portpowered/infinite-you/pkg/config/load"
	configpersist "github.com/portpowered/infinite-you/pkg/config/persist"
)

type persistFromFileMode int

const (
	persistFromFileModeSave persistFromFileMode = iota
	persistFromFileModeUpdate
)

type persistFromFileConfig struct {
	Mode       persistFromFileMode
	Name       string
	From       string
	Dir        string
	SetCurrent bool
}

type persistFromFileResult struct {
	Name       string
	FactoryDir string
}

func persistFromFile(cfg persistFromFileConfig) (persistFromFileResult, error) {
	name := strings.TrimSpace(cfg.Name)
	if name == "" {
		return persistFromFileResult{}, fmt.Errorf("factory name is required")
	}
	from := strings.TrimSpace(cfg.From)
	if from == "" {
		return persistFromFileResult{}, fmt.Errorf("--from is required")
	}
	if strings.TrimSpace(cfg.Dir) == "" {
		return persistFromFileResult{}, fmt.Errorf("factory root is required")
	}

	payload, err := os.ReadFile(from)
	if err != nil {
		return persistFromFileResult{}, fmt.Errorf("read factory config %s: %w", from, err)
	}

	if _, err := configload.LoadFromCanonicalJSON(payload, configload.LoadOptions{}); err != nil {
		return persistFromFileResult{}, err
	}

	factoryDir, err := persistFromFileNamedFactory(cfg, name, payload)
	if err != nil {
		return persistFromFileResult{}, err
	}

	if cfg.Mode == persistFromFileModeSave && cfg.SetCurrent {
		if err := configpersist.WriteCurrentFactoryPointer(cfg.Dir, name); err != nil {
			return persistFromFileResult{}, err
		}
	}

	return persistFromFileResult{
		Name:       name,
		FactoryDir: factoryDir,
	}, nil
}

func persistFromFileNamedFactory(cfg persistFromFileConfig, name string, payload []byte) (string, error) {
	switch cfg.Mode {
	case persistFromFileModeSave:
		return configpersist.PersistNamedFactory(cfg.Dir, name, payload)
	case persistFromFileModeUpdate:
		return configpersist.ReplaceNamedFactory(cfg.Dir, name, payload)
	default:
		return "", fmt.Errorf("unsupported persist-from-file mode")
	}
}

func renderPersistFromFileError(mode persistFromFileMode, err error) error {
	if mode == persistFromFileModeSave && errors.Is(err, configpersist.ErrNamedFactoryAlreadyExists) {
		return fmt.Errorf("factory already exists: %w", err)
	}
	if mode == persistFromFileModeUpdate && errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("factory not found: %w", err)
	}
	if configload.IsInvalidNamedFactory(err) || configpersist.IsInvalidNamedFactory(err) {
		return fmt.Errorf("invalid factory config: %w", err)
	}
	return err
}
