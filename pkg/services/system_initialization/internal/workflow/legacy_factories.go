package workflow

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

type legacyFactoryMigration struct {
	name      string
	sourceDir string
	targetDir string
}

func migrateLegacyNamedFactories(homeDir, canonicalRoot string, files LegacyFactoryMigrationFileSystem) error {
	legacyRoot := factorydefinitions.LegacyNamedFactoriesRoot(homeDir)
	info, err := files.Stat(legacyRoot)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect legacy global Factory root %s: %w", legacyRoot, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("migrate legacy global Factory root %s: path is not a directory; rename or remove it before retrying", legacyRoot)
	}

	migrations, err := inventoryLegacyFactoryMigrations(legacyRoot, canonicalRoot, files)
	if err != nil {
		return fmt.Errorf("list legacy global Factories in %s: %w", legacyRoot, err)
	}
	for _, migration := range migrations {
		if _, err := files.Stat(migration.targetDir); err == nil {
			return fmt.Errorf("migrate legacy Factory %q: canonical destination %s already exists; preserved legacy Factory at %s without overwriting either copy; move or rename one copy and retry", migration.name, migration.targetDir, migration.sourceDir)
		} else if !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("inspect canonical destination %s for legacy Factory %q: %w", migration.targetDir, migration.name, err)
		}
	}
	for _, migration := range migrations {
		if err := files.MkdirAll(filepath.Dir(migration.targetDir), 0o755); err != nil {
			return fmt.Errorf("create canonical parent for legacy Factory %q at %s: %w", migration.name, migration.targetDir, err)
		}
	}
	for _, migration := range migrations {
		if err := files.Rename(migration.sourceDir, migration.targetDir); err != nil {
			return fmt.Errorf("migrate legacy Factory %q from %s to %s: %w", migration.name, migration.sourceDir, migration.targetDir, err)
		}
	}
	return nil
}

func inventoryLegacyFactoryMigrations(legacyRoot, canonicalRoot string, files LegacyFactoryMigrationFileSystem) ([]legacyFactoryMigration, error) {
	pointerPath := filepath.Join(legacyRoot, factorydefinitions.CurrentFactoryPointerFile)
	pointer, err := files.ReadFile(pointerPath)
	if err == nil {
		if err := factorydefinitions.ValidateName(strings.TrimSpace(string(pointer))); err != nil {
			return nil, fmt.Errorf("validate legacy current Factory pointer %s: %w", pointerPath, err)
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("read legacy current Factory pointer %s: %w", pointerPath, err)
	}

	children, err := files.ReadDir(legacyRoot)
	if err != nil {
		return nil, fmt.Errorf("read legacy Factory root %s: %w", legacyRoot, err)
	}
	migrations := make([]legacyFactoryMigration, 0, len(children))
	for _, child := range children {
		if !child.IsDir() || isLegacyMigrationStagingDir(child.Name()) {
			continue
		}
		if !strings.HasPrefix(child.Name(), "@") {
			migration, err := mapLegacyFactoryMigration(legacyRoot, canonicalRoot, []string{child.Name()})
			if err != nil {
				return nil, err
			}
			migrations = append(migrations, migration)
			continue
		}
		scopeDir := filepath.Join(legacyRoot, child.Name())
		scopeChildren, err := files.ReadDir(scopeDir)
		if err != nil {
			return nil, fmt.Errorf("read legacy Factory scope %s: %w", scopeDir, err)
		}
		for _, scopeChild := range scopeChildren {
			if !scopeChild.IsDir() || isLegacyMigrationStagingDir(scopeChild.Name()) {
				continue
			}
			migration, err := mapLegacyFactoryMigration(legacyRoot, canonicalRoot, []string{child.Name(), scopeChild.Name()})
			if err != nil {
				return nil, err
			}
			migrations = append(migrations, migration)
		}
	}
	return migrations, nil
}

func mapLegacyFactoryMigration(legacyRoot, canonicalRoot string, segments []string) (legacyFactoryMigration, error) {
	name, err := factorydefinitions.NameFromPathSegments(segments)
	if err != nil {
		return legacyFactoryMigration{}, fmt.Errorf("map legacy Factory directory %s: %w", filepath.Join(append([]string{legacyRoot}, segments...)...), err)
	}
	targetDir, err := factorydefinitions.MapDir(canonicalRoot, name)
	if err != nil {
		return legacyFactoryMigration{}, fmt.Errorf("map legacy Factory %q from %s: %w", name, legacyRoot, err)
	}
	return legacyFactoryMigration{name: name, sourceDir: filepath.Join(append([]string{legacyRoot}, segments...)...), targetDir: targetDir}, nil
}

func isLegacyMigrationStagingDir(name string) bool {
	return strings.HasPrefix(name, ".") && strings.Contains(name, ".staging-")
}
