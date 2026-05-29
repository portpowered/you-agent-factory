package factoryrun

import (
	"fmt"
	"os"
	"strings"

	"github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

// LoadFactoryConfigFromConfigFile reads and expands a canonical factory.json file.
func LoadFactoryConfigFromConfigFile(configFilePath string) (*interfaces.FactoryConfig, error) {
	trimmed := strings.TrimSpace(configFilePath)
	if trimmed == "" {
		return nil, fmt.Errorf("factory config path is required")
	}

	data, err := os.ReadFile(trimmed)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("factory config file not found: %s", trimmed)
		}
		return nil, fmt.Errorf("read factory config file %s: %w", trimmed, err)
	}

	cfg, err := config.NewFactoryConfigMapper().Expand(data)
	if err != nil {
		return nil, fmt.Errorf("parse factory config %s: %w", trimmed, err)
	}
	return cfg, nil
}

// ValidateFactoryForPromptRun validates a factory config for you run --factory prompt submission.
func ValidateFactoryForPromptRun(cfg *interfaces.FactoryConfig) error {
	if cfg == nil {
		return fmt.Errorf("factory config is required")
	}
	result := config.NewConfigValidator(config.WithRequireDefaultHandlingWorkType()).Validate(cfg)
	if result.HasErrors() {
		return fmt.Errorf("%s", result.Error())
	}
	return nil
}

// DefaultHandlingWorkTypeName returns the single work type that declares handlingBehavior DEFAULT.
func DefaultHandlingWorkTypeName(cfg *interfaces.FactoryConfig) (string, error) {
	if cfg == nil {
		return "", fmt.Errorf("factory config is required")
	}

	var found string
	for _, workType := range cfg.WorkTypes {
		for _, behavior := range workType.HandlingBehavior {
			if interfaces.StrictPublicWorkTypeHandlingBehavior(behavior) != interfaces.WorkTypeHandlingBehaviorDefault {
				continue
			}
			if found != "" {
				return "", fmt.Errorf("expected at most one work type with handlingBehavior DEFAULT, found multiple")
			}
			found = workType.Name
		}
	}
	if found == "" {
		return "", fmt.Errorf("expected exactly one work type with handlingBehavior DEFAULT for simplified prompt runs")
	}
	return found, nil
}
