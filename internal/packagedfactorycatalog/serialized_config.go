package packagedfactorycatalog

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorymapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig"
)

// serializeFactoryConversion binds a native serialization to the exact public
// bytes produced by this generation pass. Verify round-trip equality so no
// ignored or runtime-only fields are lost at the serialization boundary.
func serializeFactoryConversion(payload []byte) ([]byte, error) {
	config, err := factorymapping.NewFactoryConfigMapper().ExpandStrict(payload)
	if err != nil {
		return nil, fmt.Errorf("compile Factory conversion: %w", err)
	}
	native, err := json.Marshal(config)
	if err != nil {
		return nil, fmt.Errorf("serialize Factory config: %w", err)
	}
	var decoded *factorydefinitions.FactoryConfig
	if err := json.Unmarshal(native, &decoded); err != nil {
		return nil, fmt.Errorf("decode serialized Factory config: %w", err)
	}
	if !reflect.DeepEqual(config, decoded) {
		return nil, fmt.Errorf("serialized Factory config loses mapped values")
	}
	hash := sha256.Sum256(payload)
	return json.Marshal(factorydefinitions.SerializedFactoryConfig{
		Version:       factorydefinitions.SerializedFactoryConfigVersion,
		PayloadSHA256: hex.EncodeToString(hash[:]),
		Config:        native,
	})
}
