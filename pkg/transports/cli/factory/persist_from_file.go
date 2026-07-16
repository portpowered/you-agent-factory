package factory

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	configload "github.com/portpowered/infinite-you/pkg/config/load"
	configpersist "github.com/portpowered/infinite-you/pkg/config/persist"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/factory/validation"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/pkg/transports/mapping"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/validationentry"
)

type persistFromFileMode int

const (
	persistFromFileModeCreate persistFromFileMode = iota
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
	if err := apisurface.ValidateWritableNamedFactoryName(factoryapi.FactoryName(name)); err != nil {
		return persistFromFileResult{}, err
	}

	payload, err := os.ReadFile(from)
	if err != nil {
		return persistFromFileResult{}, fmt.Errorf("read factory config %s: %w", from, err)
	}

	if err := validatePersistFromFilePayload(payload); err != nil {
		return persistFromFileResult{}, err
	}

	factoryDir, err := persistFromFileNamedFactory(cfg, name, payload)
	if err != nil {
		factoryPath, pathErr := factoryconfig.NamedFactoryDirPath(cfg.Dir, name)
		if pathErr != nil {
			return persistFromFileResult{}, pathErr
		}
		return persistFromFileResult{}, factoryconfig.MaybeFormatBlockingFactoryLoadOperatorError(err, factoryPath)
	}

	if cfg.Mode == persistFromFileModeCreate && cfg.SetCurrent {
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
	case persistFromFileModeCreate:
		return configpersist.PersistNamedFactory(cfg.Dir, name, payload)
	case persistFromFileModeUpdate:
		return configpersist.ReplaceNamedFactory(cfg.Dir, name, payload)
	default:
		return "", fmt.Errorf("unsupported persist-from-file mode")
	}
}

func validatePersistFromFilePayload(payload []byte) error {
	var factory factoryapi.Factory
	if err := json.Unmarshal(payload, &factory); err != nil {
		return fmt.Errorf("%w: parse factory config: %w", configload.ErrInvalidNamedFactory, err)
	}

	result, err := validationentry.ValidateFactoryAPI(context.Background(), factory, factoryvalidation.Options{
		Profile: factoryvalidation.ProfilePrePersist,
	})
	if err != nil {
		if configload.IsInvalidNamedFactory(err) {
			return err
		}
		return fmt.Errorf("%w: %v", configload.ErrInvalidNamedFactory, err)
	}
	if result.HasBlockingTargets() {
		return persistFromFileValidationTargetsError(result.BlockingTargets())
	}
	return nil
}

func persistFromFileValidationTargetsError(targets []factoryvalidation.Target) error {
	detail := factoryvalidation.DefaultTopologyValidationMessage
	if len(targets) > 0 {
		if msg := strings.TrimSpace(targets[0].Message); msg != "" {
			detail = msg
		} else if code := strings.TrimSpace(targets[0].Code); code != "" {
			detail = code
		}
	}
	if len(targets) > 1 {
		detail = fmt.Sprintf("%s (%d validation issues)", detail, len(targets))
	}
	return fmt.Errorf("%w: %s", configload.ErrInvalidNamedFactory, detail)
}

func renderPersistFromFileError(mode persistFromFileMode, err error) error {
	if mode == persistFromFileModeCreate && errors.Is(err, configpersist.ErrNamedFactoryAlreadyExists) {
		return fmt.Errorf("factory already exists: %w", err)
	}
	if mode == persistFromFileModeUpdate && errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("factory not found: %w", err)
	}
	if errors.Is(err, apisurface.ErrInvalidNamedFactoryName) {
		return err
	}
	if configload.IsInvalidNamedFactory(err) || configpersist.IsInvalidNamedFactory(err) {
		return fmt.Errorf("invalid factory config: %w", err)
	}
	return err
}
