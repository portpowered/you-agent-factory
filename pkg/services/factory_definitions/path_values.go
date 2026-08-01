package factorydefinitions

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	factorycontracts "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/contracts"
)

// Named-factory path policy is implemented by the catalog owner. These value
// contracts preserve compatibility for legacy callers while the implementation
// moves behind the root Service/catalog composition boundary.
var (
	ErrInvalidName       = factorycontracts.ErrInvalidName
	ErrNotFound          = factorycontracts.ErrNamedFactoryPathNotFound
	ErrLayoutNotFound    = errors.New("factory layout not found")
	ValidateName         = validateNamedFactoryName
	PathSegments         = namedFactoryPathSegments
	NameFromPathSegments = namedFactoryNameFromPathSegments
	MapDir               = mapNamedFactoryDir

	LegacyNamedFactoriesRoot = func(homeDir string) string {
		return filepath.Join(homeDir, ".you-agent-factory", "you-agent-factories")
	}
	NamedFactoriesRoot = func(homeDir string) string {
		return filepath.Join(homeDir, ".you-agent-factory", "factories")
	}
	NamedFactoriesRootForHome = func(homeDir string) (string, error) {
		trimmed := strings.TrimSpace(homeDir)
		if trimmed == "" {
			return "", fmt.Errorf("user home directory is required")
		}
		return NamedFactoriesRoot(trimmed), nil
	}
	ProjectFactoriesRoot = func(workingDir string) string {
		return filepath.Join(workingDir, "factory")
	}
	ProjectFactoriesRootForWorkingDir = func(workingDir string) (string, error) {
		trimmed := strings.TrimSpace(workingDir)
		if trimmed == "" {
			return "", fmt.Errorf("working directory is required")
		}
		return ProjectFactoriesRoot(trimmed), nil
	}
	ResolveNamedFactoryRoots = func(homeDir, workingDir string) (NamedFactoryRoots, error) {
		global, err := NamedFactoriesRootForHome(homeDir)
		if err != nil {
			return NamedFactoryRoots{}, err
		}
		project, err := ProjectFactoriesRootForWorkingDir(workingDir)
		if err != nil {
			return NamedFactoryRoots{}, err
		}
		return NamedFactoryRoots{Project: project, Global: global}, nil
	}
)

func validateNamedFactoryName(name string) error {
	segments, err := namedFactoryPathSegments(name)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidName, err)
	}
	canonical, err := namedFactoryNameFromPathSegments(segments)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidName, err)
	}
	if canonical != strings.TrimSpace(name) {
		return fmt.Errorf("%w: factory name %q is not canonical", ErrInvalidName, name)
	}
	return nil
}

func mapNamedFactoryDir(rootDir, name string) (string, error) {
	root := strings.TrimSpace(rootDir)
	if root == "" {
		return "", fmt.Errorf("factory root is required")
	}
	segments, err := namedFactoryPathSegments(name)
	if err != nil {
		return "", err
	}
	parts := append([]string{root}, segments...)
	return filepath.Join(parts...), nil
}

func namedFactoryPathSegments(name string) ([]string, error) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return nil, fmt.Errorf("factory name is required")
	}
	if strings.HasPrefix(trimmed, "@") {
		parts := strings.Split(trimmed, "/")
		if len(parts) != 2 || parts[0] == "@" || parts[1] == "" {
			return nil, fmt.Errorf("factory name %q must be scoped as @scope/name", trimmed)
		}
		segments := make([]string, 0, len(parts))
		for index, part := range parts {
			kind := "factory"
			if index == 0 {
				kind = "factory scope"
			}
			segment, err := safeNamedFactorySegment(kind, strings.TrimPrefix(part, "@"))
			if index == 0 && err == nil {
				segment = "@" + segment
			}
			if err != nil {
				return nil, err
			}
			segments = append(segments, segment)
		}
		return segments, nil
	}
	segment, err := safeNamedFactorySegment("factory", trimmed)
	if err != nil {
		return nil, err
	}
	return []string{segment}, nil
}

func namedFactoryNameFromPathSegments(segments []string) (string, error) {
	if len(segments) == 0 {
		return "", fmt.Errorf("factory path segments are required")
	}
	switch len(segments) {
	case 1:
		if strings.HasPrefix(segments[0], "@") {
			return "", fmt.Errorf("factory path segments %#v are not a valid hierarchical layout", segments)
		}
		if _, err := safeNamedFactorySegment("factory", segments[0]); err != nil {
			return "", err
		}
		return segments[0], nil
	case 2:
		if !strings.HasPrefix(segments[0], "@") {
			return "", fmt.Errorf("factory path segments %#v are not a valid hierarchical layout", segments)
		}
		return segments[0] + "/" + segments[1], validateNamedFactoryNameParts(segments[0] + "/" + segments[1])
	default:
		return "", fmt.Errorf("factory path segments %#v are not a valid hierarchical layout", segments)
	}
}

func validateNamedFactoryNameParts(name string) error {
	parts := strings.Split(name, "/")
	if len(parts) != 2 || parts[0] == "@" || parts[1] == "" {
		return fmt.Errorf("factory name %q must be scoped as @scope/name", name)
	}
	if _, err := safeNamedFactorySegment("factory scope", strings.TrimPrefix(parts[0], "@")); err != nil {
		return err
	}
	_, err := safeNamedFactorySegment("factory", parts[1])
	return err
}

func safeNamedFactorySegment(kind, name string) (string, error) {
	segment := strings.TrimSpace(name)
	if segment == "" {
		return "", fmt.Errorf("%s name is required for factory config layout", kind)
	}
	if filepath.IsAbs(segment) || filepath.VolumeName(segment) != "" || strings.ContainsAny(segment, `/\\`) {
		return "", fmt.Errorf("%s name %q cannot contain path separators", kind, name)
	}
	if segment == "." || segment == ".." {
		return "", fmt.Errorf("%s name %q is not a valid directory name", kind, name)
	}
	return segment, nil
}
