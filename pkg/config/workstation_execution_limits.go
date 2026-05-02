package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

// NormalizeWorkstationExecutionLimit rewrites legacy workstation timeout
// authoring into the canonical limits.maxExecutionTime field and clears the
// retired top-level timeout field.
func NormalizeWorkstationExecutionLimit(cfg *interfaces.FactoryWorkstationConfig) {
	if cfg == nil {
		return
	}
	if strings.TrimSpace(cfg.Limits.MaxExecutionTime) == "" && strings.TrimSpace(cfg.Timeout) != "" {
		cfg.Limits.MaxExecutionTime = cfg.Timeout
	}
	cfg.Timeout = ""
}

// WorkstationExecutionTimeout resolves the configured workstation execution
// timeout from the canonical execution limits field with a direct-struct legacy
// timeout fallback for older in-memory fixtures.
func WorkstationExecutionTimeout(cfg *interfaces.FactoryWorkstationConfig) (time.Duration, error) {
	if cfg == nil {
		return 0, nil
	}

	if strings.TrimSpace(cfg.Limits.MaxExecutionTime) != "" {
		timeout, err := time.ParseDuration(cfg.Limits.MaxExecutionTime)
		if err != nil {
			return 0, fmt.Errorf("invalid workstation limits.maxExecutionTime %q: %v", cfg.Limits.MaxExecutionTime, err)
		}
		if timeout > 0 {
			return timeout, nil
		}
	}

	if strings.TrimSpace(cfg.Timeout) != "" {
		timeout, err := time.ParseDuration(cfg.Timeout)
		if err != nil {
			return 0, fmt.Errorf("invalid legacy workstation timeout %q: %v", cfg.Timeout, err)
		}
		if timeout > 0 {
			return timeout, nil
		}
	}

	return 0, nil
}
