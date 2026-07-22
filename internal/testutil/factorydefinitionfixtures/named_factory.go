package factorydefinitionfixtures

import (
	"os"
	"path/filepath"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

// SeedNamedFactory authors a detached Factory input at the exact directory
// selected by the caller. It deliberately owns no named-catalog path policy;
// customer-scale tests carry directories returned by public operations.
func SeedNamedFactory(factoryDir string, payload []byte) (string, error) {
	if err := os.MkdirAll(factoryDir, 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(factoryDir, factorydefinitions.FactoryConfigFile), payload, 0o644); err != nil {
		return "", err
	}
	return factoryDir, nil
}

func SeedNamedFactoryUnchecked(factoryDir string, payload []byte) (string, error) {
	return SeedNamedFactory(factoryDir, payload)
}
