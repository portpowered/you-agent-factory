package replay

import (
	"encoding/json"

	"github.com/portpowered/infinite-you/internal/testutil/runtimefixtures"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	factorymapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig"
)

var testFactorySnapshotDecoder factorydefinitions.FactorySnapshotJSONDecoder = func(
	data []byte,
) (*factorydefinitions.FactorySnapshot, error) {
	generated, err := factorymapping.GeneratedFactoryFromOpenAPIJSON(data)
	if err != nil {
		return nil, err
	}
	return factorydefinitions.NewFactorySnapshot(generated)
}

func testRuntimeConfigDecoder(
	snapshot *factorydefinitions.FactorySnapshot,
) (factorydefinitions.ReplayRuntimeConfig, error) {
	var generated factoryapi.Factory
	if err := snapshot.Decode(&generated); err != nil {
		return nil, err
	}
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
