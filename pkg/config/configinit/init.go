// Package configinit implements the canonical you config init system bootstrap.
package configinit

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/config/defaultpaths"
	"github.com/portpowered/infinite-you/pkg/config/operatorconfig"
	"github.com/portpowered/infinite-you/pkg/config/systemconfig"
	factorypackages "github.com/portpowered/infinite-you/pkg/factory/packages"
)

// SystemConfigOutcome reports whether init created or skipped operator/system config.
type SystemConfigOutcome string

const (
	SystemConfigCreated SystemConfigOutcome = "created"
	SystemConfigSkipped SystemConfigOutcome = "skipped"
)

// PackagedFactoryOutcome reports whether init created or skipped one packaged
// default factory.
type PackagedFactoryOutcome string

const (
	PackagedFactoryCreated PackagedFactoryOutcome = "created"
	PackagedFactorySkipped PackagedFactoryOutcome = "skipped"
)

// PackagedFactoryResult summarizes one packaged default factory ensure
// operation during you config init.
type PackagedFactoryResult struct {
	Name       string
	FactoryDir string
	Outcome    PackagedFactoryOutcome
}

// Result summarizes a successful you config init run.
type Result struct {
	HomeDir             string
	ConfigPath          string
	NamedFactoriesRoot  string
	SystemConfigOutcome SystemConfigOutcome
	PackagedFactories   []PackagedFactoryResult
}

// Init ensures operator/system config exists under homeDir without overwriting
// an already-present config file.
func Init(homeDir string) (Result, error) {
	homeDir = strings.TrimSpace(homeDir)
	if homeDir == "" {
		return Result{}, fmt.Errorf("home directory is required")
	}

	configPath := defaultpaths.OperatorConfigPath(homeDir)
	namedFactoriesRoot := defaultpaths.NamedFactoriesRoot(homeDir)

	if err := ensureSystemConfigParentIsDirectory(configPath); err != nil {
		return Result{}, err
	}

	systemConfigOutcome := SystemConfigCreated
	if _, err := os.Stat(configPath); err == nil {
		if _, err := operatorconfig.LoadFileConfig(configPath); err != nil {
			return Result{}, fmt.Errorf("read existing operator config %q: %w", configPath, err)
		}
		systemConfigOutcome = SystemConfigSkipped
	} else if !os.IsNotExist(err) {
		return Result{}, fmt.Errorf("stat operator config %q: %w", configPath, err)
	} else {
		if _, err := systemconfig.EnsureLocalBackendScope(configPath); err != nil {
			return Result{}, fmt.Errorf("create system config at %q: %w", configPath, err)
		}
		if _, err := operatorconfig.LoadFileConfig(configPath); err != nil {
			return Result{}, fmt.Errorf("validate created operator config %q: %w", configPath, err)
		}
	}

	if err := migrateLegacyNamedFactories(homeDir, namedFactoriesRoot); err != nil {
		return Result{}, err
	}

	packagedFactories, err := ensurePackagedDefaultFactories(namedFactoriesRoot)
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

type legacyFactoryMigration struct {
	name      string
	sourceDir string
	targetDir string
}

func migrateLegacyNamedFactories(homeDir, namedFactoriesRoot string) error {
	legacyRoot := defaultpaths.LegacyNamedFactoriesRoot(homeDir)
	info, err := os.Stat(legacyRoot)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect legacy global factory root %s: %w", legacyRoot, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("migrate legacy global factory root %s: path is not a directory; rename or remove it before retrying", legacyRoot)
	}

	migrations, err := inventoryLegacyFactoryMigrations(legacyRoot, namedFactoriesRoot)
	if err != nil {
		return fmt.Errorf("list legacy global factories in %s: %w", legacyRoot, err)
	}
	for _, migration := range migrations {
		if _, err := os.Stat(migration.targetDir); err == nil {
			return fmt.Errorf(
				"migrate legacy factory %q: canonical destination %s already exists; preserved legacy factory at %s without overwriting either copy; move or rename one copy and retry",
				migration.name,
				migration.targetDir,
				migration.sourceDir,
			)
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("inspect canonical destination %s for legacy factory %q: %w", migration.targetDir, migration.name, err)
		}
	}

	for _, migration := range migrations {
		if err := os.MkdirAll(filepath.Dir(migration.targetDir), 0o755); err != nil {
			return fmt.Errorf("create canonical parent for legacy factory %q at %s: %w", migration.name, migration.targetDir, err)
		}
	}
	for _, migration := range migrations {
		if err := os.Rename(migration.sourceDir, migration.targetDir); err != nil {
			return fmt.Errorf("migrate legacy factory %q from %s to %s: %w", migration.name, migration.sourceDir, migration.targetDir, err)
		}
	}
	return nil
}

func inventoryLegacyFactoryMigrations(legacyRoot, namedFactoriesRoot string) ([]legacyFactoryMigration, error) {
	// Keep pointer validation from the normal list path, but do not use that
	// list for migration: it intentionally omits factories with invalid config.
	if _, err := factoryconfig.ListNamedFactories(legacyRoot); err != nil {
		return nil, err
	}
	children, err := os.ReadDir(legacyRoot)
	if err != nil {
		return nil, fmt.Errorf("read legacy factory root %s: %w", legacyRoot, err)
	}

	migrations := make([]legacyFactoryMigration, 0, len(children))
	for _, child := range children {
		if !child.IsDir() || isLegacyMigrationStagingDir(child.Name()) {
			continue
		}
		if !strings.HasPrefix(child.Name(), "@") {
			migration, err := mapLegacyFactoryMigration(legacyRoot, namedFactoriesRoot, []string{child.Name()})
			if err != nil {
				return nil, err
			}
			migrations = append(migrations, migration)
			continue
		}

		scopeDir := filepath.Join(legacyRoot, child.Name())
		scopeChildren, err := os.ReadDir(scopeDir)
		if err != nil {
			return nil, fmt.Errorf("read legacy factory scope %s: %w", scopeDir, err)
		}
		for _, scopeChild := range scopeChildren {
			if !scopeChild.IsDir() || isLegacyMigrationStagingDir(scopeChild.Name()) {
				continue
			}
			migration, err := mapLegacyFactoryMigration(legacyRoot, namedFactoriesRoot, []string{child.Name(), scopeChild.Name()})
			if err != nil {
				return nil, err
			}
			migrations = append(migrations, migration)
		}
	}
	return migrations, nil
}

func mapLegacyFactoryMigration(legacyRoot, namedFactoriesRoot string, segments []string) (legacyFactoryMigration, error) {
	name, err := factoryconfig.NamedFactoryNameFromPathSegments(segments)
	if err != nil {
		return legacyFactoryMigration{}, fmt.Errorf("map legacy factory directory %s: %w", filepath.Join(append([]string{legacyRoot}, segments...)...), err)
	}
	targetDir, err := factoryconfig.MapNamedFactoryDir(namedFactoriesRoot, name)
	if err != nil {
		return legacyFactoryMigration{}, fmt.Errorf("map legacy factory %q from %s: %w", name, legacyRoot, err)
	}
	return legacyFactoryMigration{
		name:      name,
		sourceDir: filepath.Join(append([]string{legacyRoot}, segments...)...),
		targetDir: targetDir,
	}, nil
}

func isLegacyMigrationStagingDir(name string) bool {
	return strings.HasPrefix(name, ".") && strings.Contains(name, ".staging-")
}

func ensureSystemConfigParentIsDirectory(configPath string) error {
	parentDir := filepath.Dir(configPath)
	info, err := os.Stat(parentDir)
	if err != nil {
		if os.IsNotExist(err) {
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

func ensurePackagedDefaultFactories(namedFactoriesRoot string) ([]PackagedFactoryResult, error) {
	definitions, err := packagedFactoryDefinitions()
	if err != nil {
		return nil, err
	}
	return ensurePackagedFactories(namedFactoriesRoot, definitions)
}

func packagedFactoryDefinitions() ([]factorypackages.Definition, error) {
	names := factorypackages.Names()
	definitions := make([]factorypackages.Definition, 0, len(names))
	for _, name := range names {
		definition, ok := factorypackages.Lookup(name)
		if !ok {
			return nil, fmt.Errorf("load packaged factory %q from catalog: definition not found", name)
		}
		definitions = append(definitions, definition)
	}
	return definitions, nil
}

func ensurePackagedFactories(
	namedFactoriesRoot string,
	definitions []factorypackages.Definition,
) ([]PackagedFactoryResult, error) {
	results := make([]PackagedFactoryResult, 0, len(definitions))
	for _, definition := range definitions {
		factoryDir, outcome, err := ensurePackagedFactory(namedFactoriesRoot, definition)
		if err != nil {
			return nil, err
		}
		results = append(results, PackagedFactoryResult{
			Name:       definition.Name,
			FactoryDir: factoryDir,
			Outcome:    outcome,
		})
	}
	return results, nil
}

func ensurePackagedFactory(
	namedFactoriesRoot string,
	definition factorypackages.Definition,
) (string, PackagedFactoryOutcome, error) {
	targetDir, err := factoryconfig.MapNamedFactoryDir(namedFactoriesRoot, definition.Name)
	if err != nil {
		return "", "", packagedFactoryInstallError(definition.Name, namedFactoriesRoot, err)
	}

	if _, err := os.Stat(targetDir); err == nil {
		if err := factoryconfig.ValidateFactoryDirReadOnly(targetDir, nil); err != nil {
			return "", "", packagedFactoryInstallError(
				definition.Name,
				namedFactoriesRoot,
				fmt.Errorf("existing target %s is invalid: %w", targetDir, err),
			)
		}
		return targetDir, PackagedFactorySkipped, nil
	} else if !os.IsNotExist(err) {
		return "", "", packagedFactoryInstallError(
			definition.Name,
			namedFactoriesRoot,
			fmt.Errorf("inspect target %s: %w", targetDir, err),
		)
	}

	factoryDir, err := factoryconfig.PersistNamedFactory(namedFactoriesRoot, definition.Name, definition.JSON)
	if err != nil {
		return "", "", packagedFactoryInstallError(definition.Name, namedFactoriesRoot, err)
	}
	return factoryDir, PackagedFactoryCreated, nil
}

func packagedFactoryInstallError(name, namedFactoriesRoot string, err error) error {
	return fmt.Errorf("install packaged factory %q in global root %s: %w", name, namedFactoriesRoot, err)
}
