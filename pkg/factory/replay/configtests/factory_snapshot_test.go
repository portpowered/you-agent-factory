package replay_test

import (
	"github.com/portpowered/infinite-you/pkg/config"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	"github.com/portpowered/infinite-you/pkg/factory/replay"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/factorysnapshot"
)

func generatedFactoryFromLoadedConfig(loaded *config.LoadedFactoryConfig, opts ...replay.FactorySnapshotOption) (factoryapi.Factory, error) {
	snapshot, err := replay.FactorySnapshotFromLoadedConfig(loaded, opts...)
	if err != nil {
		return factoryapi.Factory{}, err
	}
	generated, err := factorysnapshot.ToAPI(snapshot)
	if err != nil {
		return factoryapi.Factory{}, err
	}
	return *generated, nil
}

func runtimeConfigFromGeneratedFactory(generated factoryapi.Factory) (*replay.EmbeddedRuntimeConfig, error) {
	snapshot, err := interfaces.NewFactorySnapshot(generated)
	if err != nil {
		return nil, err
	}
	return replay.RuntimeConfigFromFactorySnapshot(snapshot)
}
