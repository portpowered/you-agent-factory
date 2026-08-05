// Package workflow owns the System Bootstrap initialize and rollback workflow
// behind the sealed CTR-BOOT root contract.
package workflow

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	systeminitialization "github.com/portpowered/infinite-you/pkg/services/system_initialization"
)

// Initializer is the canonical System Bootstrap workflow implementer. It
// satisfies the singular peer-facing Service contract without exposing
// additional Bootstrap authority interfaces to peers.
type Initializer struct {
	operatorSettings OperatorSettings
	packaging        factorydefinitions.Packaging
	inspectPath      InspectPath
	migrationFiles   LegacyFactoryMigrationFileSystem
}

var _ systeminitialization.Service = (*Initializer)(nil)

// New constructs the canonical workflow from already-selected collaborators.
func New(
	operatorSettings OperatorSettings,
	packaging factorydefinitions.Packaging,
	inspectPath InspectPath,
	migrationFiles LegacyFactoryMigrationFileSystem,
) (*Initializer, error) {
	if operatorSettings == nil {
		return nil, fmt.Errorf("construct system initialization: Operator Settings service is required")
	}
	if packaging == nil {
		return nil, fmt.Errorf("construct system initialization: Factory Definitions packaging capability is required")
	}
	if inspectPath == nil {
		return nil, fmt.Errorf("construct system initialization: inspect path edge is required")
	}
	if migrationFiles == nil {
		return nil, fmt.Errorf("construct system initialization: legacy Factory migration filesystem is required")
	}
	return &Initializer{
		operatorSettings: operatorSettings,
		packaging:        packaging,
		inspectPath:      inspectPath,
		migrationFiles:   migrationFiles,
	}, nil
}

// Initialize ensures operator configuration and packaged Factories exist
// without overwriting valid customer-owned files.
// pkgmaintcheck:ignore-cyclomatic-complexity service-ownership migration preserves this decision flow; simplify branches and remove this exemption.
func (initializer *Initializer) Initialize(
	ctx context.Context,
	request systeminitialization.Request,
) (systeminitialization.Result, error) {
	if ctx == nil {
		return systeminitialization.Result{}, fmt.Errorf("initialize system: context is required")
	}
	if err := ctx.Err(); err != nil {
		return systeminitialization.Result{}, fmt.Errorf("initialize system: %w: %w", systeminitialization.ErrInitializeCancelled, err)
	}

	homeDir := strings.TrimSpace(request.HomeDir)
	if homeDir == "" {
		return systeminitialization.Result{}, fmt.Errorf("%w", systeminitialization.ErrMissingHomeDir)
	}
	if initializer == nil {
		return systeminitialization.Result{}, fmt.Errorf("initialize system: service is required")
	}
	if initializer.operatorSettings == nil {
		return systeminitialization.Result{}, fmt.Errorf("initialize system: Operator Settings service is required")
	}
	if initializer.packaging == nil {
		return systeminitialization.Result{}, fmt.Errorf("initialize system: Factory Definitions packaging capability is required")
	}
	if initializer.inspectPath == nil {
		return systeminitialization.Result{}, fmt.Errorf("initialize system: inspect path edge is required")
	}
	if initializer.migrationFiles == nil {
		return systeminitialization.Result{}, fmt.Errorf("initialize system: legacy Factory migration filesystem is required")
	}

	packagedFactoryNames, err := resolvePackagedFactoryNames(ctx, initializer.packaging)
	if err != nil {
		return systeminitialization.Result{}, err
	}

	configPath := operatorsettings.DefaultConfigPath(homeDir)
	namedFactoriesRoot := factorydefinitions.NamedFactoriesRoot(homeDir)
	if err := migrateLegacyNamedFactories(homeDir, namedFactoriesRoot, initializer.migrationFiles); err != nil {
		return systeminitialization.Result{}, err
	}

	if err := ensureSystemConfigParentIsDirectory(configPath, initializer.inspectPath); err != nil {
		return systeminitialization.Result{}, err
	}

	systemConfigOutcome := systeminitialization.SystemConfigCreated
	settings := initializer.operatorSettings
	if _, err := initializer.inspectPath(configPath); err == nil {
		if _, err := settings.LoadFileConfig(configPath); err != nil {
			return systeminitialization.Result{}, partialInitializeFailure(
				"read existing operator config failed",
				rollbackFactsAfterLegacyMigration(
					systeminitialization.RollbackFact{
						Step:    systeminitialization.InitializeStepSystemConfig,
						Outcome: systeminitialization.RollbackStepUnresolved,
					},
				),
				fmt.Errorf("read existing operator config %q: %w", configPath, err),
			)
		}
		systemConfigOutcome = systeminitialization.SystemConfigSkipped
	} else if !errors.Is(err, fs.ErrNotExist) {
		return systeminitialization.Result{}, fmt.Errorf("stat operator config %q: %w", configPath, err)
	} else {
		if _, err := settings.EnsureLocalBackendScope(configPath); err != nil {
			return systeminitialization.Result{}, partialInitializeFailure(
				"create system config failed",
				rollbackFactsAfterLegacyMigration(
					systeminitialization.RollbackFact{
						Step:    systeminitialization.InitializeStepSystemConfig,
						Outcome: systeminitialization.RollbackStepUnresolved,
					},
				),
				fmt.Errorf("create system config at %q: %w", configPath, err),
			)
		}
		if _, err := settings.LoadFileConfig(configPath); err != nil {
			return systeminitialization.Result{}, partialInitializeFailure(
				"validate created operator config failed",
				rollbackFactsAfterLegacyMigration(
					systeminitialization.RollbackFact{
						Step:    systeminitialization.InitializeStepSystemConfig,
						Outcome: systeminitialization.RollbackStepUnresolved,
					},
				),
				fmt.Errorf("validate created operator config %q: %w", configPath, err),
			)
		}
	}

	packagedFactories, err := installPackagedFactories(
		ctx,
		initializer.packaging,
		namedFactoriesRoot,
		packagedFactoryNames,
	)
	if err != nil {
		return systeminitialization.Result{}, partialInitializeFailure(
			"packaged factory install failed",
			rollbackFactsAfterSystemConfig(systemConfigOutcome, systeminitialization.RollbackFact{
				Step:    systeminitialization.InitializeStepPackagedFactories,
				Outcome: systeminitialization.RollbackStepUnresolved,
			}),
			err,
		)
	}

	return systeminitialization.Result{
		HomeDir:             homeDir,
		ConfigPath:          configPath,
		NamedFactoriesRoot:  namedFactoriesRoot,
		SystemConfigOutcome: systemConfigOutcome,
		PackagedFactories:   packagedFactories,
	}, nil
}

func resolvePackagedFactoryNames(
	ctx context.Context,
	packaging factorydefinitions.Packaging,
) ([]string, error) {
	listed, err := packaging.ListBuiltInPackagedFactories(
		ctx,
		factorydefinitions.ListBuiltInPackagedFactoriesRequest{},
	)
	if err != nil {
		return nil, fmt.Errorf("list packaged Factories for system initialization: %w", err)
	}
	names := make([]string, 0, len(listed.Entries))
	for _, entry := range listed.Entries {
		// Preserve the existing preflight boundary: every published identity
		// resolves before Bootstrap changes the customer configuration. Retain
		// only the public name; Packaging owns detached package content.
		_, err := packaging.ResolveBuiltInPackagedFactory(
			ctx,
			factorydefinitions.ResolveBuiltInPackagedFactoryRequest{Name: entry.Name},
		)
		if err != nil {
			return nil, fmt.Errorf(
				"resolve packaged Factory %q for system initialization: %w",
				entry.Name,
				err,
			)
		}
		names = append(names, entry.Name)
	}
	return names, nil
}

func installPackagedFactories(
	ctx context.Context,
	packaging factorydefinitions.Packaging,
	namedFactoriesRoot string,
	names []string,
) ([]systeminitialization.PackagedFactoryResult, error) {
	results := make([]systeminitialization.PackagedFactoryResult, 0, len(names))
	for _, name := range names {
		installed, err := packaging.InstallPackagedFactory(ctx, factorydefinitions.InstallPackagedFactoryRequest{
			RootDir: namedFactoriesRoot,
			Name:    name,
			Format:  factorydefinitions.PackagedFactoryFormatJSON,
		})
		if err != nil {
			return nil, fmt.Errorf("install packaged Factory %q for system initialization: %w", name, err)
		}
		results = append(results, systeminitialization.PackagedFactoryResult{
			Name:       installed.Definition.Name,
			FactoryDir: installed.Definition.FactoryDir,
			Outcome:    systeminitialization.PackagedFactoryOutcome(installed.Outcome),
		})
	}
	return results, nil
}

func partialInitializeFailure(
	message string,
	facts []systeminitialization.RollbackFact,
	cause error,
) error {
	return systeminitialization.InitializePartialFailure{
		Message: message,
		Facts:   facts,
		Cause:   cause,
	}
}

func rollbackFactsAfterLegacyMigration(extra ...systeminitialization.RollbackFact) []systeminitialization.RollbackFact {
	facts := []systeminitialization.RollbackFact{{
		Step:    systeminitialization.InitializeStepLegacyMigration,
		Outcome: systeminitialization.RollbackStepCompleted,
	}}
	return append(facts, extra...)
}

func rollbackFactsAfterSystemConfig(
	systemConfigOutcome systeminitialization.SystemConfigOutcome,
	extra ...systeminitialization.RollbackFact,
) []systeminitialization.RollbackFact {
	facts := rollbackFactsAfterLegacyMigration(systeminitialization.RollbackFact{
		Step:    systeminitialization.InitializeStepSystemConfig,
		Outcome: systemConfigRollbackOutcome(systemConfigOutcome),
	})
	return append(facts, extra...)
}

func systemConfigRollbackOutcome(
	systemConfigOutcome systeminitialization.SystemConfigOutcome,
) systeminitialization.RollbackStepOutcome {
	switch systemConfigOutcome {
	case systeminitialization.SystemConfigCreated, systeminitialization.SystemConfigSkipped:
		return systeminitialization.RollbackStepRolledBackOrPreserved
	default:
		return systeminitialization.RollbackStepUnresolved
	}
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
