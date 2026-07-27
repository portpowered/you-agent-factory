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
		result, err := service.InstallPackagedFactory(
			ctx,
			namedFactoriesRoot,
			definition,
			factorydefinitions.PackagedFactoryFormatJSON,
		)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	return results, nil
}

func (service *Service) InstallPackagedFactory(
	ctx context.Context,
	namedFactoriesRoot string,
	definition factorydefinitions.PackagedDefinition,
	format factorydefinitions.PackagedFactoryFormat,
) (factorydefinitions.PackagedFactoryInstallResult, error) {
	result := factorydefinitions.PackagedFactoryInstallResult{
		Name:   definition.Name,
		Format: format,
	}
	payload, rootFileName, normalizedFormat, err := selectPayload(
		definition,
		format,
	)
	if err != nil {
		return result, installError(definition.Name, namedFactoriesRoot, err)
	}
	result.Format = normalizedFormat
	targetDir, err := namedfactorypath.MapDir(namedFactoriesRoot, definition.Name)
	if err != nil {
		return result, installError(definition.Name, namedFactoriesRoot, err)
	}
	result.FactoryDir = targetDir
	if service == nil || service.fileSystem == nil {
		return result, installError(definition.Name, namedFactoriesRoot, fmt.Errorf("packaged Factory installation filesystem is required"))
	}
	if _, err := service.fileSystem.Stat(targetDir); err == nil {
		if service == nil || service.persistence == nil {
			return result, installError(definition.Name, namedFactoriesRoot, fmt.Errorf("Factory Definitions persistence service is required"))
		}
		if err := service.persistence.ValidateFactoryLayout(targetDir); err != nil {
			return result, installError(definition.Name, namedFactoriesRoot, fmt.Errorf("existing target %s is invalid: %w", targetDir, err))
		}
		result.Outcome = factorydefinitions.PackagedFactoryInstallSkipped
		return result, nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return result, installError(definition.Name, namedFactoriesRoot, fmt.Errorf("inspect target %s: %w", targetDir, err))
	}
	if service == nil || service.persistence == nil {
		return result, installError(definition.Name, namedFactoriesRoot, fmt.Errorf("Factory Definitions persistence service is required"))
	}
	prepared, err := service.persistence.PrepareFactoryLayout(ctx, definition.Name, payload)
	if err != nil {
		return result, installError(definition.Name, namedFactoriesRoot, err)
	}
	if prepared == nil {
		return result, installError(
			definition.Name,
			namedFactoriesRoot,
			fmt.Errorf("prepared Factory layout is required"),
		)
	}
	prepared.RootFileName = rootFileName
	factoryDir, err := service.persistence.CreateNamedFactory(namedFactoriesRoot, definition.Name, prepared)
	if err != nil {
		return result, installError(definition.Name, namedFactoriesRoot, err)
	}
	result.FactoryDir = factoryDir
	result.Outcome = factorydefinitions.PackagedFactoryInstallCreated
	return result, nil
}

func selectPayload(
	definition factorydefinitions.PackagedDefinition,
	format factorydefinitions.PackagedFactoryFormat,
) ([]byte, string, factorydefinitions.PackagedFactoryFormat, error) {
	if format == "" {
		format = factorydefinitions.PackagedFactoryFormatJSON
	}
	switch format {
	case factorydefinitions.PackagedFactoryFormatJSON:
		if len(definition.JSON) == 0 {
			return nil, "", "", fmt.Errorf("packaged Factory does not publish JSON")
		}
		return append([]byte(nil), definition.JSON...),
			factorydefinitions.FactoryConfigFile, format, nil
	case factorydefinitions.PackagedFactoryFormatYAML:
		if len(definition.YAML) == 0 {
			return nil, "", "", fmt.Errorf("packaged Factory does not publish YAML")
		}
		return append([]byte(nil), definition.JSON...),
			"factory.yaml", format, nil
	case factorydefinitions.PackagedFactoryFormatYML:
		if len(definition.YAML) == 0 {
			return nil, "", "", fmt.Errorf("packaged Factory does not publish YAML")
		}
		return append([]byte(nil), definition.JSON...),
			"factory.yml", format, nil
	default:
		return nil, "", "", fmt.Errorf("unsupported packaged Factory format %q", format)
	}
}

func installError(name, namedFactoriesRoot string, err error) error {
	return fmt.Errorf("install packaged factory %q in global root %s: %w", name, namedFactoriesRoot, err)
}
