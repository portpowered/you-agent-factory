package packagedinstallation

import (
	"context"
	"errors"
	"fmt"
	"io/fs"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	namedfactorypath "github.com/portpowered/infinite-you/pkg/services/factory_definitions/namedpaths"
)

type Service struct {
	persistence factorydefinitions.Persistence
	fileSystem  factorydefinitions.PackagedInstallationFileSystem
}

func New(
	persistence factorydefinitions.Persistence,
	fileSystem factorydefinitions.PackagedInstallationFileSystem,
) *Service {
	return &Service{persistence: persistence, fileSystem: fileSystem}
}

func (service *Service) EnsurePackagedFactories(
	ctx context.Context,
	namedFactoriesRoot string,
	definitions []factorydefinitions.PackagedDefinition,
) ([]factorydefinitions.PackagedFactoryInstallResult, error) {
	results := make([]factorydefinitions.PackagedFactoryInstallResult, 0, len(definitions))
	for _, definition := range definitions {
		factoryDir, outcome, err := service.ensurePackagedFactory(ctx, namedFactoriesRoot, definition)
		if err != nil {
			return nil, err
		}
		results = append(results, factorydefinitions.PackagedFactoryInstallResult{
			Name: definition.Name, FactoryDir: factoryDir, Outcome: outcome,
		})
	}
	return results, nil
}

func (service *Service) ensurePackagedFactory(
	ctx context.Context,
	namedFactoriesRoot string,
	definition factorydefinitions.PackagedDefinition,
) (string, factorydefinitions.PackagedFactoryInstallOutcome, error) {
	targetDir, err := namedfactorypath.MapDir(namedFactoriesRoot, definition.Name)
	if err != nil {
		return "", "", installError(definition.Name, namedFactoriesRoot, err)
	}
	if service == nil || service.fileSystem == nil {
		return "", "", installError(definition.Name, namedFactoriesRoot, fmt.Errorf("packaged Factory installation filesystem is required"))
	}
	if _, err := service.fileSystem.Stat(targetDir); err == nil {
		if service == nil || service.persistence == nil {
			return "", "", installError(definition.Name, namedFactoriesRoot, fmt.Errorf("Factory Definitions persistence service is required"))
		}
		if err := service.persistence.ValidateFactoryLayout(targetDir); err != nil {
			return "", "", installError(definition.Name, namedFactoriesRoot, fmt.Errorf("existing target %s is invalid: %w", targetDir, err))
		}
		return targetDir, factorydefinitions.PackagedFactoryInstallSkipped, nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return "", "", installError(definition.Name, namedFactoriesRoot, fmt.Errorf("inspect target %s: %w", targetDir, err))
	}
	if service == nil || service.persistence == nil {
		return "", "", installError(definition.Name, namedFactoriesRoot, fmt.Errorf("Factory Definitions persistence service is required"))
	}
	prepared, err := service.persistence.PrepareFactoryLayout(ctx, definition.Name, definition.JSON)
	if err != nil {
		return "", "", installError(definition.Name, namedFactoriesRoot, err)
	}
	factoryDir, err := service.persistence.CreateNamedFactory(namedFactoriesRoot, definition.Name, prepared)
	if err != nil {
		return "", "", installError(definition.Name, namedFactoriesRoot, err)
	}
	return factoryDir, factorydefinitions.PackagedFactoryInstallCreated, nil
}

func installError(name, namedFactoriesRoot string, err error) error {
	return fmt.Errorf("install packaged factory %q in global root %s: %w", name, namedFactoriesRoot, err)
}
