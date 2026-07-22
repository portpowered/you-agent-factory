package factory

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/transports/cli/factoryload"
)

type persistFromFileMode int

const (
	persistFromFileModeCreate persistFromFileMode = iota
	persistFromFileModeUpdate
)

type persistFromFileConfig struct {
	Context    context.Context
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

func persistFromFile(
	cfg persistFromFileConfig,
	persist factorydefinitions.NamedFactoryPersistenceOperation,
	loadSource factorydefinitions.AuthoredFactorySourceLoader,
) (persistFromFileResult, error) {
	from := strings.TrimSpace(cfg.From)
	if from == "" {
		return persistFromFileResult{}, fmt.Errorf("--from is required")
	}
	if loadSource == nil {
		return persistFromFileResult{}, fmt.Errorf("Factory Definitions authored source loader is required")
	}
	payload, err := loadSource(from)
	if err != nil {
		return persistFromFileResult{}, fmt.Errorf("read factory config %s: %w", from, err)
	}
	if cfg.Context == nil {
		return persistFromFileResult{}, fmt.Errorf("factory persistence context is required")
	}
	if persist == nil {
		return persistFromFileResult{}, fmt.Errorf("Factory Definitions named persistence operation is required")
	}
	mode, err := namedFactoryPersistenceMode(cfg.Mode)
	if err != nil {
		return persistFromFileResult{}, err
	}
	result, err := persist(
		cfg.Context,
		factorydefinitions.NamedFactoryPersistenceRequest{
			Mode:       mode,
			RootDir:    cfg.Dir,
			Name:       cfg.Name,
			Payload:    payload,
			SetCurrent: cfg.SetCurrent,
		},
	)
	if err != nil {
		if result.FactoryDir != "" {
			err = factoryload.MaybeFormatOperatorError(err, result.FactoryDir)
		}
		return persistFromFileResult{}, err
	}
	return persistFromFileResult{
		Name:       result.Name,
		FactoryDir: result.FactoryDir,
	}, nil
}

func namedFactoryPersistenceMode(
	mode persistFromFileMode,
) (factorydefinitions.NamedFactoryPersistenceMode, error) {
	switch mode {
	case persistFromFileModeCreate:
		return factorydefinitions.NamedFactoryPersistenceModeCreate, nil
	case persistFromFileModeUpdate:
		return factorydefinitions.NamedFactoryPersistenceModeReplace, nil
	default:
		return "", fmt.Errorf("unsupported persist-from-file mode")
	}
}

func renderPersistFromFileError(mode persistFromFileMode, err error) error {
	if mode == persistFromFileModeCreate && errors.Is(err, factorydefinitions.ErrNamedFactoryAlreadyExists) {
		return fmt.Errorf("factory already exists: %w", err)
	}
	if mode == persistFromFileModeUpdate && errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("factory not found: %w", err)
	}
	if errors.Is(err, factorydefinitions.ErrInvalidNamedFactoryName) {
		return err
	}
	if errors.Is(err, factorydefinitions.ErrInvalidNamedFactory) {
		return fmt.Errorf("invalid factory config: %w", err)
	}
	return err
}
