package packagedinstallation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	namedfactorypath "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/catalog/namedpaths"
	"gopkg.in/yaml.v3"
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
			factorydefinitions.PackagedFactoryInstallParams{
				NamedFactoriesRoot: namedFactoriesRoot,
				Definition:         definition,
				Format:             factorydefinitions.PackagedFactoryFormatJSON,
			},
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
	params factorydefinitions.PackagedFactoryInstallParams,
) (factorydefinitions.PackagedFactoryInstallResult, error) {
	definition := params.Definition
	format := params.Format
	namedFactoriesRoot := params.NamedFactoriesRoot
	result := factorydefinitions.PackagedFactoryInstallResult{
		Name:   definition.Name,
		Format: format,
	}
	if err := ctx.Err(); err != nil {
		return result, installError(definition.Name, namedFactoriesRoot, err)
	}
	payload, rootFileName, normalizedFormat, err := selectPayload(
		definition,
		format,
	)
	if err != nil {
		return result, installError(definition.Name, namedFactoriesRoot, err)
	}
	result.Format = normalizedFormat
	if service == nil || service.fileSystem == nil {
		return result, installError(definition.Name, namedFactoriesRoot, fmt.Errorf("packaged Factory installation filesystem is required"))
	}
	if service == nil || service.persistence == nil {
		return result, installError(definition.Name, namedFactoriesRoot, fmt.Errorf("Factory Definitions persistence service is required"))
	}
	// Prepare validates the complete Factory aggregate, including portable
	// bundled-file paths and contents, before target inspection or mutation.
	prepared, err := service.persistence.PrepareFactoryLayout(ctx, definition.Name, payload)
	if err != nil {
		return result, installError(
			definition.Name,
			namedFactoriesRoot,
			factorydefinitions.NewPackagedFactoryInputError(
				factorydefinitions.PackagedFactoryErrorMalformed,
				definition.Name,
				normalizedFormat,
				"",
				err,
			),
		)
	}
	if prepared == nil {
		return result, installError(
			definition.Name,
			namedFactoriesRoot,
			factorydefinitions.NewPackagedFactoryInputError(
				factorydefinitions.PackagedFactoryErrorMalformed,
				definition.Name,
				normalizedFormat,
				"",
				fmt.Errorf("prepared Factory layout is required"),
			),
		)
	}
	prepared.RootFileName = rootFileName
	targetDir, err := namedfactorypath.MapDir(namedFactoriesRoot, definition.Name)
	if err != nil {
		return result, installError(
			definition.Name,
			namedFactoriesRoot,
			factorydefinitions.NewPackagedFactoryInputError(
				factorydefinitions.PackagedFactoryErrorMalformed,
				definition.Name,
				normalizedFormat,
				"",
				err,
			),
		)
	}
	result.FactoryDir = targetDir
	if _, err := service.fileSystem.Stat(targetDir); err == nil {
		if err := service.persistence.ValidateFactoryLayout(targetDir); err != nil {
			return result, installError(definition.Name, namedFactoriesRoot, fmt.Errorf("existing target %s is invalid: %w", targetDir, err))
		}
		if params.Replace {
			return service.replaceExistingPackagedFactory(
				ctx,
				namedFactoriesRoot,
				definition.Name,
				targetDir,
				prepared,
				result,
			)
		}
		existingFormat, formatErr := authoredRootFormat(targetDir, service.fileSystem)
		if formatErr != nil {
			return result, installError(definition.Name, namedFactoriesRoot, formatErr)
		}
		if existingFormat != normalizedFormat {
			return result, installError(
				definition.Name,
				namedFactoriesRoot,
				fmt.Errorf(
					"%w: factory %q already exists",
					factorydefinitions.ErrNamedFactoryAlreadyExists,
					definition.Name,
				),
			)
		}
		result.Outcome = factorydefinitions.PackagedFactoryInstallSkipped
		return result, nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return result, installError(definition.Name, namedFactoriesRoot, fmt.Errorf("inspect target %s: %w", targetDir, err))
	}
	return service.createPackagedFactory(
		ctx,
		namedFactoriesRoot,
		definition.Name,
		targetDir,
		prepared,
		result,
	)
}

func (service *Service) createPackagedFactory(
	ctx context.Context,
	namedFactoriesRoot string,
	name string,
	targetDir string,
	prepared *factorydefinitions.PreparedFactoryLayoutPayload,
	result factorydefinitions.PackagedFactoryInstallResult,
) (factorydefinitions.PackagedFactoryInstallResult, error) {
	if err := ctx.Err(); err != nil {
		return result, installError(name, namedFactoriesRoot, err)
	}
	factoryDir, err := service.persistence.CreateNamedFactory(namedFactoriesRoot, name, prepared)
	if err != nil {
		return result, installError(name, namedFactoriesRoot, err)
	}
	result.FactoryDir = factoryDir
	result.Outcome = factorydefinitions.PackagedFactoryInstallCreated
	return result, nil
}

func (service *Service) replaceExistingPackagedFactory(
	ctx context.Context,
	namedFactoriesRoot string,
	name string,
	targetDir string,
	prepared *factorydefinitions.PreparedFactoryLayoutPayload,
	result factorydefinitions.PackagedFactoryInstallResult,
) (factorydefinitions.PackagedFactoryInstallResult, error) {
	if err := ctx.Err(); err != nil {
		return result, installError(name, namedFactoriesRoot, err)
	}
	factoryDir, err := service.persistence.ReplaceNamedFactory(namedFactoriesRoot, name, prepared)
	if err != nil {
		return result, installError(name, namedFactoriesRoot, err)
	}
	result.FactoryDir = factoryDir
	result.Outcome = factorydefinitions.PackagedFactoryInstallReplaced
	return result, nil
}

func authoredRootFormat(
	targetDir string,
	fileSystem factorydefinitions.PackagedInstallationFileSystem,
) (factorydefinitions.PackagedFactoryFormat, error) {
	for _, candidate := range []struct {
		file   string
		format factorydefinitions.PackagedFactoryFormat
	}{
		{factorydefinitions.FactoryConfigFile, factorydefinitions.PackagedFactoryFormatJSON},
		{"factory.yaml", factorydefinitions.PackagedFactoryFormatYAML},
		{"factory.yml", factorydefinitions.PackagedFactoryFormatYML},
	} {
		_, err := fileSystem.Stat(filepath.Join(targetDir, candidate.file))
		if err == nil {
			return candidate.format, nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return "", fmt.Errorf("inspect authored root %s: %w", candidate.file, err)
		}
	}
	return "", fmt.Errorf("no authored root definition in %s", targetDir)
}

func selectPayload(
	definition factorydefinitions.PackagedDefinition,
	format factorydefinitions.PackagedFactoryFormat,
) ([]byte, string, factorydefinitions.PackagedFactoryFormat, error) {
	if strings.TrimSpace(definition.Name) == "" {
		return nil, "", "", factorydefinitions.NewPackagedFactoryInputError(
			factorydefinitions.PackagedFactoryErrorMalformed,
			definition.Name,
			format,
			"",
			fmt.Errorf("package identity is required"),
		)
	}
	if format == "" {
		format = factorydefinitions.PackagedFactoryFormatJSON
	}
	if !supportsFormat(definition.Formats, format) {
		return nil, "", "", factorydefinitions.NewPackagedFactoryInputError(
			factorydefinitions.PackagedFactoryErrorUnsupported,
			definition.Name,
			format,
			"",
			fmt.Errorf("package does not publish the selected representation"),
		)
	}
	if _, err := factorydefinitions.BuildPackagedFactoryIntegrity(definition.JSON, definition.YAML); err != nil {
		return nil, "", "", factorydefinitions.NewPackagedFactoryInputError(
			factorydefinitions.PackagedFactoryErrorMalformed,
			definition.Name,
			format,
			"",
			err,
		)
	}
	if err := factorydefinitions.VerifyPackagedFactoryIntegrity(definition, format); err != nil {
		return nil, "", "", err
	}
	switch format {
	case factorydefinitions.PackagedFactoryFormatJSON:
		if len(definition.JSON) == 0 {
			return nil, "", "", factorydefinitions.NewPackagedFactoryInputError(
				factorydefinitions.PackagedFactoryErrorUnsupported,
				definition.Name,
				format,
				"",
				fmt.Errorf("package does not publish JSON"),
			)
		}
		return append([]byte(nil), definition.JSON...),
			factorydefinitions.FactoryConfigFile, format, nil
	case factorydefinitions.PackagedFactoryFormatYAML:
		if len(definition.YAML) == 0 {
			return nil, "", "", factorydefinitions.NewPackagedFactoryInputError(
				factorydefinitions.PackagedFactoryErrorUnsupported,
				definition.Name,
				format,
				"",
				fmt.Errorf("package does not publish YAML"),
			)
		}
		payload, err := canonicalYAMLPayload(definition.YAML)
		return payload, "factory.yaml", format, err
	case factorydefinitions.PackagedFactoryFormatYML:
		if len(definition.YAML) == 0 {
			return nil, "", "", factorydefinitions.NewPackagedFactoryInputError(
				factorydefinitions.PackagedFactoryErrorUnsupported,
				definition.Name,
				format,
				"",
				fmt.Errorf("package does not publish YAML"),
			)
		}
		payload, err := canonicalYAMLPayload(definition.YAML)
		return payload, "factory.yml", format, err
	default:
		return nil, "", "", factorydefinitions.NewPackagedFactoryInputError(
			factorydefinitions.PackagedFactoryErrorUnsupported,
			definition.Name,
			format,
			"",
			fmt.Errorf("unsupported packaged Factory format %q", format),
		)
	}
}

func supportsFormat(
	formats []factorydefinitions.PackagedFactoryFormat,
	format factorydefinitions.PackagedFactoryFormat,
) bool {
	if len(formats) == 0 {
		return true
	}
	for _, candidate := range formats {
		if candidate == format ||
			((format == factorydefinitions.PackagedFactoryFormatYAML || format == factorydefinitions.PackagedFactoryFormatYML) &&
				candidate == factorydefinitions.PackagedFactoryFormatYAML) {
			return true
		}
	}
	return false
}

func canonicalYAMLPayload(payload []byte) ([]byte, error) {
	var document any
	if err := yaml.Unmarshal(payload, &document); err != nil {
		return nil, fmt.Errorf("decode packaged Factory YAML: %w", err)
	}
	canonical, err := json.Marshal(document)
	if err != nil {
		return nil, fmt.Errorf("encode packaged Factory YAML as JSON: %w", err)
	}
	if len(canonical) == 0 || canonical[0] != '{' {
		return nil, fmt.Errorf("packaged Factory YAML must be an object")
	}
	return canonical, nil
}

func installError(name, namedFactoriesRoot string, err error) error {
	return fmt.Errorf("install packaged factory %q in global root %s: %w", name, namedFactoriesRoot, err)
}
