package systeminitialization

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
)

// Initializer installs operator configuration and the injected packaged
// Factory catalog.
type Initializer struct {
	definitions       []factorydefinitions.PackagedDefinition
	operatorSettings  OperatorSettings
	packagedInstaller factorydefinitions.PackagedFactoryInstaller
	inspectPath       InspectPath
	migrationFiles    LegacyFactoryMigrationFileSystem
}

// New constructs the canonical service from already-selected collaborators.
func New(
	operatorSettings OperatorSettings,
	packagedInstaller factorydefinitions.PackagedFactoryInstaller,
	definitions []factorydefinitions.PackagedDefinition,
	inspectPath InspectPath,
	migrationFiles LegacyFactoryMigrationFileSystem,
) (*Initializer, error) {
	if operatorSettings == nil {
		return nil, fmt.Errorf("construct system initialization: Operator Settings service is required")
	}
	if packagedInstaller == nil {
		return nil, fmt.Errorf("construct system initialization: Factory Definitions packaged installer is required")
	}
	if inspectPath == nil {
		return nil, fmt.Errorf("construct system initialization: inspect path edge is required")
	}
	if migrationFiles == nil {
		return nil, fmt.Errorf("construct system initialization: legacy Factory migration filesystem is required")
	}
	return &Initializer{
		operatorSettings:  operatorSettings,
		packagedInstaller: packagedInstaller,
		definitions:       append([]factorydefinitions.PackagedDefinition(nil), definitions...),
		inspectPath:       inspectPath,
		migrationFiles:    migrationFiles,
	}, nil
}

// Initialize ensures operator configuration and packaged Factories exist
// without overwriting valid customer-owned files.
// pkgmaintcheck:ignore-cyclomatic-complexity service-ownership migration preserves this decision flow; simplify branches and remove this exemption.
func (initializer *Initializer) Initialize(
	ctx context.Context,
	request Request,
) (Result, error) {
	if ctx == nil {
		return Result{}, fmt.Errorf("initialize system: context is required")
	}
	if err := ctx.Err(); err != nil {
		return Result{}, fmt.Errorf("initialize system: %w", err)
	}

	homeDir := strings.TrimSpace(request.HomeDir)
	if homeDir == "" {
		return Result{}, fmt.Errorf("home directory is required")
	}
	if initializer == nil {
		return Result{}, fmt.Errorf("initialize system: service is required")
	}
	if initializer.operatorSettings == nil {
		return Result{}, fmt.Errorf("initialize system: Operator Settings service is required")
	}
	if initializer.packagedInstaller == nil {
		return Result{}, fmt.Errorf("initialize system: Factory Definitions packaged installer is required")
	}
	if initializer.inspectPath == nil {
		return Result{}, fmt.Errorf("initialize system: inspect path edge is required")
	}
	if initializer.migrationFiles == nil {
		return Result{}, fmt.Errorf("initialize system: legacy Factory migration filesystem is required")
	}

	configPath := operatorsettings.DefaultConfigPath(homeDir)
	namedFactoriesRoot := factorydefinitions.NamedFactoriesRoot(homeDir)
	if err := migrateLegacyNamedFactories(homeDir, namedFactoriesRoot, initializer.migrationFiles); err != nil {
		return Result{}, err
	}

	if err := ensureSystemConfigParentIsDirectory(configPath, initializer.inspectPath); err != nil {
		return Result{}, err
	}

	systemConfigOutcome := SystemConfigCreated
	settings := initializer.operatorSettings
	if _, err := initializer.inspectPath(configPath); err == nil {
		if _, err := settings.LoadFileConfig(configPath); err != nil {
			return Result{}, fmt.Errorf("read existing operator config %q: %w", configPath, err)
		}
		systemConfigOutcome = SystemConfigSkipped
	} else if !errors.Is(err, fs.ErrNotExist) {
		return Result{}, fmt.Errorf("stat operator config %q: %w", configPath, err)
	} else {
		if _, err := settings.EnsureLocalBackendScope(configPath); err != nil {
			return Result{}, fmt.Errorf("create system config at %q: %w", configPath, err)
		}
		if _, err := settings.LoadFileConfig(configPath); err != nil {
			return Result{}, fmt.Errorf("validate created operator config %q: %w", configPath, err)
		}
	}

	installed, err := initializer.packagedInstaller.EnsurePackagedFactories(
		ctx,
		namedFactoriesRoot,
		initializer.definitions,
	)
	packagedFactories := projectPackagedFactoryResults(installed)
	if err != nil {
		return Result{}, err
	}

	return Result{
		HomeDir:             homeDir,
		ConfigPath:          configPath,
		NamedFactoriesRoot:  namedFactoriesRoot,
		SystemConfigOutcome: systemConfigOutcome,
		PackagedFactories:   packagedFactories,
	}, nil
}

func projectPackagedFactoryResults(installed []factorydefinitions.PackagedFactoryInstallResult) []PackagedFactoryResult {
	results := make([]PackagedFactoryResult, 0, len(installed))
	for _, result := range installed {
		results = append(results, PackagedFactoryResult{
			Name: result.Name, FactoryDir: result.FactoryDir, Outcome: PackagedFactoryOutcome(result.Outcome),
		})
	}
	return results
}

func ensureSystemConfigParentIsDirectory(configPath string, inspectPath InspectPath) error {
	parentDir := filepath.Dir(configPath)
	info, err := inspectPath(parentDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("create system config at %s: inspect parent path %s: %w", configPath, parentDir, err)
	}
	if info.IsDir() {
		return nil
	}
	return fmt.Errorf(
		"create system config at %s: parent path %s exists but is not a directory; remove or rename it and retry",
		configPath,
		parentDir,
	)
}
