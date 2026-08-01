package replay_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	factorydefinitionfixtures "github.com/portpowered/infinite-you/internal/testutil/factorydefinitionfixtures"
	"github.com/portpowered/infinite-you/internal/testutil/runtimefixtures"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorydefinitionswire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/wire"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	factorymapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/factorysnapshot"
)

func generatedFactoryFromLoadedConfig(
	capture interfaces.LoadedFactorySnapshotCapturer,
	loaded interfaces.MutableLoadedFactorySource,
	sourceDirectory string,
	metadata map[string]string,
) (factoryapi.Factory, error) {
	snapshot, err := capture(loaded, sourceDirectory, metadata)
	if err != nil {
		return factoryapi.Factory{}, err
	}
	generated, err := factorysnapshot.ToAPI(snapshot)
	if err != nil {
		return factoryapi.Factory{}, err
	}
	return *generated, nil
}

// scriptedLoadedFactorySnapshotCapturer keeps Recordings tests at the Factory
// Definitions root contract. Snapshot capture/merge policy is proved by the
// Factory Definitions owner; these tests consume a canonical serialized value.
func scriptedLoadedFactorySnapshotCapturer(
	factoryDir string,
) interfaces.LoadedFactorySnapshotCapturer {
	return func(
		source interfaces.FactorySnapshotSource,
		sourceDirectory string,
		metadata map[string]string,
	) (*interfaces.FactorySnapshot, error) {
		if source == nil {
			return nil, fmt.Errorf("scripted snapshot source is required")
		}
		if source.FactoryDir() != factoryDir {
			return nil, fmt.Errorf(
				"scripted snapshot source directory = %q, want %q",
				source.FactoryDir(),
				factoryDir,
			)
		}
		generated, err := generatedFactoryFromRootConfig(source.FactoryConfig())
		if err != nil {
			return nil, err
		}
		if sourceDirectory == "" {
			sourceDirectory = factoryDir
		}
		generated.FactoryDirectory = &factoryDir
		generated.SourceDirectory = &sourceDirectory
		snapshotMetadata := factoryapi.StringMap{
			"source_format":       "agent-factory.replay.v1",
			"factory_hash":        "sha256:scripted",
			"workers_hash":        "sha256:scripted",
			"workstations_hash":   "sha256:scripted",
			"runtime_config_hash": "sha256:scripted",
		}
		for key, value := range metadata {
			snapshotMetadata[key] = value
		}
		generated.Metadata = &snapshotMetadata
		return interfaces.NewFactorySnapshot(generated)
	}
}

type factoryConfigMutation func(*interfaces.FactoryConfig)

func loadedFactoryValue(factoryDir string, mutations ...factoryConfigMutation) (interfaces.MutableLoadedFactorySource, error) {
	payload, err := os.ReadFile(filepath.Join(factoryDir, interfaces.FactoryConfigFile))
	if err != nil {
		return nil, err
	}
	config, err := factorymapping.NewFactoryConfigMapper().Expand(payload)
	if err != nil {
		return nil, err
	}
	for _, mutate := range mutations {
		mutate(config)
	}
	return factorydefinitionfixtures.NewLoadedSource(factoryDir, config, nil, nil)
}

func withWorkerDefinition(name string, update func(*interfaces.FactoryWorkerConfig)) factoryConfigMutation {
	return func(config *interfaces.FactoryConfig) {
		for index := range config.Workers {
			if config.Workers[index].Name == name {
				update(&config.Workers[index])
				return
			}
		}
	}
}

func withWorkstationDefinition(name string, update func(*interfaces.FactoryWorkstationConfig)) factoryConfigMutation {
	return func(config *interfaces.FactoryConfig) {
		for index := range config.Workstations {
			if config.Workstations[index].Name == name {
				update(&config.Workstations[index])
				return
			}
		}
	}
}

func generatedFactoryFromRootConfig(
	config *interfaces.FactoryConfig,
) (factoryapi.Factory, error) {
	object, err := factorysnapshot.ObjectFromFactoryConfig(config)
	if err != nil {
		return factoryapi.Factory{}, err
	}
	snapshot, err := interfaces.NewFactorySnapshot(object)
	if err != nil {
		return factoryapi.Factory{}, err
	}
	generated, err := factorysnapshot.ToAPI(snapshot)
	if err != nil {
		return factoryapi.Factory{}, err
	}
	return *generated, nil
}

func runtimeConfigFromGeneratedFactory(generated factoryapi.Factory) (factorydefinitionswire.ReplayRuntimeConfig, error) {
	payload, err := json.Marshal(generated)
	if err != nil {
		return nil, err
	}
	config, err := factorymapping.FactoryConfigFromOpenAPIJSON(payload)
	if err != nil {
		return nil, err
	}
	factoryDir := ""
	if generated.FactoryDirectory != nil {
		factoryDir = *generated.FactoryDirectory
	}
	return runtimefixtures.ReplayRuntimeConfigValue(config, factoryDir), nil
}

func runtimeConfigFromFactorySnapshot(snapshot *interfaces.FactorySnapshot) (factorydefinitionswire.ReplayRuntimeConfig, error) {
	var generated factoryapi.Factory
	if err := snapshot.Decode(&generated); err != nil {
		return nil, err
	}
	return runtimeConfigFromGeneratedFactory(generated)
}

var configTestFactorySnapshotDecoder factorydefinitionswire.FactorySnapshotJSONDecoder = func(
	data []byte,
) (*interfaces.FactorySnapshot, error) {
	generated, err := factorymapping.GeneratedFactoryFromOpenAPIJSON(data)
	if err != nil {
		return nil, err
	}
	return interfaces.NewFactorySnapshot(generated)
}
