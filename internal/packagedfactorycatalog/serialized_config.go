package packagedfactorycatalog

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorymapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig"
	authoredmapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig/authored"
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
	canonicalInput, canonical, err := serializeCanonicalFactory(config)
	if err != nil {
		return nil, err
	}
	canonicalHash := sha256.Sum256(canonicalInput)
	return json.Marshal(factorydefinitions.SerializedFactoryConfig{
		Version:              factorydefinitions.SerializedFactoryConfigVersion,
		PayloadSHA256:        hex.EncodeToString(hash[:]),
		Config:               native,
		CanonicalInputSHA256: hex.EncodeToString(canonicalHash[:]),
		Canonical:            canonical,
	})
}

func serializeCanonicalFactory(config *factorydefinitions.FactoryConfig) ([]byte, []byte, error) {
	authored, err := authoredmapping.AuthoredFactoryConfigForExpandedLayout(config)
	if err != nil {
		return nil, nil, err
	}
	input, err := json.Marshal(authored)
	if err != nil {
		return nil, nil, err
	}
	var decoded *factorydefinitions.FactoryConfig
	if err := json.Unmarshal(input, &decoded); err != nil {
		return nil, nil, err
	}
	if !reflect.DeepEqual(authored, decoded) {
		return nil, nil, fmt.Errorf("canonical Factory input loses authored values")
	}
	canonical, err := factorymapping.NewFactoryConfigMapper().Flatten(authored)
	return input, canonical, err
}
