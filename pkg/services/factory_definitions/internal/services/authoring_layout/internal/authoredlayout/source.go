package authoredlayout

import (
	"errors"
	"fmt"
	factoryeffects "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/effects"
	"io/fs"
	"path/filepath"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

const (
	factoryJSONRootName = "factory.json"
	factoryYAMLRootName = "factory.yaml"
	factoryYMLRootName  = "factory.yml"
)

// NewFactorySourceLoader resolves a file or one unambiguous Factory directory
// root and returns its JSON-compatible authored representation.
func NewFactorySourceLoader(
	fileSystem factoryeffects.AuthoredLayoutReaderFileSystem,
) factorydefinitions.AuthoredFactorySourceLoader {
	resolveSourcePath := newFactorySourcePathResolver(fileSystem)
	return func(path string) (factorydefinitions.AuthoredFactorySource, error) {
		if fileSystem == nil {
			return factorydefinitions.AuthoredFactorySource{}, fmt.Errorf(
				"Factory Definitions authored-source filesystem is required",
			)
		}
		sourcePath, err := resolveSourcePath(path)
		if err != nil {
			return factorydefinitions.AuthoredFactorySource{}, err
		}
		return loadFactorySourceFile(fileSystem, sourcePath)
	}
}

func newFactorySourcePathResolver(
	fileSystem factoryeffects.AuthoredLayoutReaderFileSystem,
) func(string) (string, error) {
	return func(path string) (string, error) {
		if fileSystem == nil {
			return "", fmt.Errorf("Factory Definitions authored-source filesystem is required")
		}
		info, err := fileSystem.Stat(path)
		if err != nil {
			return "", fmt.Errorf("find factory config source %s: %w", path, err)
		}
		if !info.IsDir() {
			if !info.Mode().IsRegular() {
				return "", fmt.Errorf("Factory Definition source must be a regular file: %s", path)
			}
			return path, nil
		}
		return resolveFactoryDirectoryRoot(fileSystem, path)
	}
}

func loadFactorySourceFile(
	fileSystem factoryeffects.AuthoredLayoutReaderFileSystem,
	sourcePath string,
) (factorydefinitions.AuthoredFactorySource, error) {
	format, err := authoredFactoryFormatForPath(sourcePath)
	if err != nil {
		return factorydefinitions.AuthoredFactorySource{}, fmt.Errorf(
			"select Factory Definition source %s: %w",
			sourcePath,
			err,
		)
	}
	data, err := fileSystem.ReadFile(sourcePath)
	if err != nil {
		return factorydefinitions.AuthoredFactorySource{}, fmt.Errorf(
			"read factory config %s: %w",
			sourcePath,
			err,
		)
	}
	decoded, err := decodeAuthoredFactory(sourcePath, format, data)
	if err != nil {
		return factorydefinitions.AuthoredFactorySource{}, err
	}
	return factorydefinitions.AuthoredFactorySource{
		Path:   sourcePath,
		Format: format,
		Data:   decoded,
	}, nil
}

func authoredFactoryFormatForPath(
	path string,
) (factorydefinitions.AuthoredFactoryFormat, error) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".json":
		return factorydefinitions.AuthoredFactoryFormatJSON, nil
	case ".yaml", ".yml":
		return factorydefinitions.AuthoredFactoryFormatYAML, nil
	default:
		return "", fmt.Errorf(
			"unsupported Factory Definition extension %q; supported extensions are %s",
			filepath.Ext(path),
			factorydefinitions.SupportedAuthoredFactoryExtensions,
		)
	}
}

func resolveFactoryDirectoryRoot(
	fileSystem factoryeffects.AuthoredLayoutReaderFileSystem,
	directory string,
) (string, error) {
	rootNames := [...]string{
		factoryJSONRootName,
		factoryYAMLRootName,
		factoryYMLRootName,
	}
	matches := make([]string, 0, len(rootNames))
	for _, name := range rootNames {
		candidate := filepath.Join(directory, name)
		info, err := fileSystem.Stat(candidate)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return "", fmt.Errorf("inspect Factory Definition root %s: %w", candidate, err)
		}
		if !info.Mode().IsRegular() {
			return "", fmt.Errorf("Factory Definition root must be a regular file: %s", candidate)
		}
		matches = append(matches, candidate)
	}
	switch len(matches) {
	case 0:
		return "", fmt.Errorf(
			"Factory Definition directory %s has no supported root; expected exactly one of %s",
			directory,
			factorydefinitions.SupportedAuthoredFactoryRootFiles,
		)
	case 1:
		return matches[0], nil
	default:
		return "", fmt.Errorf(
			"Factory Definition directory %s has ambiguous roots: %s; keep exactly one of %s",
			directory,
			strings.Join(matches, ", "),
			factorydefinitions.SupportedAuthoredFactoryRootFiles,
		)
	}
}
