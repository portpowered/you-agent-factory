package namedpaths

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	factorycontracts "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/contracts"
)

var (
	// ErrInvalidName classifies a named Factory name that cannot map safely to
	// its canonical hierarchical layout.
	ErrInvalidName = factorycontracts.ErrInvalidName
	// ErrNotFound classifies a canonical named Factory directory that does not
	// contain a Factory definition.
	ErrNotFound = factorycontracts.ErrNamedFactoryPathNotFound
)

// CandidatePaths contains the detached, ordered paths used to diagnose a
// failed cross-root named Factory lookup.
type CandidatePaths = factorycontracts.NamedFactoryCandidatePaths

// ResolveCandidatePaths maps both catalog roots without inspecting the
// filesystem. The returned paths are detached values ordered consistently
// with cross-root catalog lookup precedence.
func (r *Resolver) ResolveCandidatePaths(
	projectRoot, globalRoot, name string,
) (CandidatePaths, error) {
	if r == nil || r.fileSystem == nil {
		return CandidatePaths{}, fmt.Errorf("named Factory path filesystem is required")
	}
	project, err := MapDir(projectRoot, name)
	projectErr := err
	global, globalErr := MapDir(globalRoot, name)
	if projectErr != nil && globalErr != nil {
		return CandidatePaths{}, errors.Join(projectErr, globalErr)
	}
	return CandidatePaths{
		Project: project,
		Global:  global,
	}, nil
}

// ValidateName validates and canonicalizes one named Factory display name.
func ValidateName(name string) error {
	segments, err := PathSegments(name)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidName, err)
	}
	canonical, err := NameFromPathSegments(segments)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidName, err)
	}
	if canonical != strings.TrimSpace(name) {
		return fmt.Errorf("%w: factory name %q is not canonical", ErrInvalidName, name)
	}
	return nil
}

// ResolveExistingDir maps a named Factory and verifies that its canonical
// factory.json exists as a regular file.
func (r *Resolver) ResolveExistingDir(rootDir, name string) (string, error) {
	if r == nil || r.fileSystem == nil {
		return "", fmt.Errorf("named Factory path filesystem is required")
	}
	if strings.TrimSpace(rootDir) == "" {
		return "", fmt.Errorf("factory root is required")
	}
	if err := ValidateName(name); err != nil {
		return "", err
	}
	canonicalName := strings.TrimSpace(name)
	factoryDir, err := MapDir(rootDir, canonicalName)
	if err != nil {
		return "", err
	}
	configPath := filepath.Join(factoryDir, factoryConfigFile)
	info, err := r.fileSystem.Stat(configPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			if dirInfo, dirErr := r.fileSystem.Stat(factoryDir); dirErr == nil && dirInfo.IsDir() {
				return "", fmt.Errorf(
					"resolve named factory %q in root %s: existing target could not be loaded: find factory config %s: %w",
					canonicalName,
					rootDir,
					configPath,
					err,
				)
			} else if dirErr != nil && !errors.Is(dirErr, fs.ErrNotExist) {
				return "", fmt.Errorf(
					"resolve named factory %q in root %s: stat target directory: %w",
					canonicalName,
					rootDir,
					dirErr,
				)
			}
			return "", fmt.Errorf(
				"resolve named factory %q in root %s: %w",
				canonicalName,
				rootDir,
				ErrNotFound,
			)
		}
		return "", fmt.Errorf("resolve named factory %q in root %s: find factory config %s: %w", canonicalName, rootDir, configPath, err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("resolve named factory %q in root %s: factory config %s is not a regular file", canonicalName, rootDir, configPath)
	}
	return factoryDir, nil
}

// RequireDefinitionDir verifies that factoryDir contains a regular
// factory.json definition file.
func (r *Resolver) RequireDefinitionDir(factoryDir string) error {
	if r == nil || r.fileSystem == nil {
		return fmt.Errorf("named Factory path filesystem is required")
	}
	if strings.TrimSpace(factoryDir) == "" {
		return fmt.Errorf("factory directory is required")
	}
	configPath := filepath.Join(factoryDir, factoryConfigFile)
	info, err := r.fileSystem.Stat(configPath)
	if err != nil {
		return fmt.Errorf("find factory config %s: %w", configPath, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("factory config %s is not a regular file", configPath)
	}
	return nil
}
