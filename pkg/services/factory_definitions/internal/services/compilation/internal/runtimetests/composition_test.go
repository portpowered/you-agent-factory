package runtimetests

import (
	"fmt"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	compilationloadedsource "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/compilation/internal/loadedsource"
)

// LoadedFactoryConfig preserves the concrete nil-receiver coverage in this
// implementation-owned suite. Production consumers use the root
// MutableLoadedFactorySource contract.
type LoadedFactoryConfig = compilationloadedsource.Source

type InlineRuntimeDefinitionOptions struct {
	RequireSplitDefinitions bool
	WorkstationLoader       factorydefinitions.WorkstationLoader
}

func NewLoadedFactoryConfig(
	factoryDir string,
	factoryConfig *factorydefinitions.FactoryConfig,
	runtimeDefinitions factorydefinitions.RuntimeDefinitionLookup,
) (*LoadedFactoryConfig, error) {
	source, err := compilationloadedsource.New(
		factoryDir,
		factoryConfig,
		runtimeDefinitions,
		nil,
	)
	return concreteLoadedSource(source, err)
}

func LoadRuntimeConfig(
	factoryRoot string,
	workstationLoader factorydefinitions.WorkstationLoader,
) (*LoadedFactoryConfig, error) {
	source, err := factorydefinitioncomposition.LoadCurrent(
		factoryRoot,
		workstationLoader,
	)
	return concreteLoadedSource(source, err)
}

func LoadRuntimeConfigFromFactoryDir(
	factoryDir string,
	workstationLoader factorydefinitions.WorkstationLoader,
) (*LoadedFactoryConfig, error) {
	source, err := factorydefinitioncomposition.LoadDirectory(
		factoryDir,
		workstationLoader,
	)
	return concreteLoadedSource(source, err)
}

func InlineRuntimeDefinitions(
	factoryDir string,
	_ *factorydefinitions.FactoryConfig,
	options InlineRuntimeDefinitionOptions,
) (*factorydefinitions.FactoryConfig, error) {
	loaded, err := LoadRuntimeConfigFromFactoryDir(
		factoryDir,
		options.WorkstationLoader,
	)
	if err != nil {
		return nil, err
	}
	return loaded.FactoryConfig(), nil
}

func ExpandFactoryConfigLayout(path string) (string, error) {
	targetDir, _, err := factorydefinitioncomposition.ExpandLayout(path)
	return targetDir, err
}

func concreteLoadedSource(
	source factorydefinitions.MutableLoadedFactorySource,
	err error,
) (*LoadedFactoryConfig, error) {
	if err != nil {
		return nil, err
	}
	loaded, ok := source.(*compilationloadedsource.Source)
	if !ok {
		return nil, fmt.Errorf(
			"Factory Definitions composition returned %T, want *compilationloadedsource.Source",
			source,
		)
	}
	return loaded, nil
}
