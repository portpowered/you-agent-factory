package authored

import (
	"fmt"
	"os"
	"path/filepath"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

// These helpers preserve the table fixtures as parser tests. Production
// filesystem ownership lives in Factory Definitions authoredlayout.Reader.
func LoadWorkerConfig(
	dir string,
) (*factorydefinitions.FactoryWorkerConfig, error) {
	path := filepath.Join(dir, factorydefinitions.FactoryAgentsFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("load worker config from %s: %w", dir, err)
	}
	config, err := ParseWorkerConfig(data, path)
	if err != nil {
		return nil, fmt.Errorf("load worker config from %s: %w", dir, err)
	}
	return config, nil
}

func LoadWorkstationConfig(
	dir string,
) (*factorydefinitions.FactoryWorkstationConfig, error) {
	path := filepath.Join(dir, factorydefinitions.FactoryAgentsFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("load workstation config from %s: %w", dir, err)
	}
	config, err := ParseWorkstationConfig(data, path)
	if err != nil {
		return nil, fmt.Errorf("load workstation config from %s: %w", dir, err)
	}
	if config.PromptFile != "" {
		promptPath := filepath.Join(dir, config.PromptFile)
		prompt, readErr := os.ReadFile(promptPath)
		if readErr != nil {
			return nil, fmt.Errorf("load prompt file %s: %w", promptPath, readErr)
		}
		config.PromptTemplate = string(prompt)
	}
	return config, nil
}
