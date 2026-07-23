package authoredlayout

import (
	"fmt"
	"path/filepath"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

// LoadFactorySource resolves a file or Factory directory and reads its
// authored factory.json representation.
func NewFactorySourceLoader(
	fileSystem factorydefinitions.AuthoredLayoutReaderFileSystem,
) factorydefinitions.AuthoredFactorySourceLoader {
	return func(path string) ([]byte, error) {
		if fileSystem == nil {
			return nil, fmt.Errorf("Factory Definitions authored-source filesystem is required")
		}
		return loadFactorySource(fileSystem, path)
	}
}

func loadFactorySource(
	fileSystem factorydefinitions.AuthoredLayoutReaderFileSystem,
	path string,
) ([]byte, error) {
	info, err := fileSystem.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("find factory config source %s: %w", path, err)
	}
	sourcePath := path
	if info.IsDir() {
		sourcePath = filepath.Join(path, factorydefinitions.FactoryConfigFile)
	}
	format, err := factorydefinitions.AuthoredFactoryFormatForPath(sourcePath)
	if err != nil {
		return nil, fmt.Errorf("select Factory Definition source %s: %w", sourcePath, err)
	}
	data, err := fileSystem.ReadFile(sourcePath)
	if err != nil {
		return nil, fmt.Errorf("read factory config %s: %w", sourcePath, err)
	}
	return decodeAuthoredFactory(sourcePath, format, data)
}
