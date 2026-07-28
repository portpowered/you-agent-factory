// Package workstationexecution owns Workstation execution-limit normalization
// and resolution, owned by nested invocation_policy.
package workstationexecution

import (
	"fmt"
	"strings"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

// Service owns Workstation execution-limit normalization and resolution.
type Service struct{}

var _ factorydefinitions.WorkstationExecutionPolicyService = Service{}

func NewService() factorydefinitions.WorkstationExecutionPolicyService {
	return Service{}
}

// NormalizeExecutionLimit rewrites legacy Workstation timeout authoring into
// the canonical limits.maxExecutionTime field.
func NormalizeExecutionLimit(cfg *factorydefinitions.FactoryWorkstationConfig) {
	if cfg == nil {
		return
	}
	if strings.TrimSpace(cfg.Limits.MaxExecutionTime) == "" &&
		strings.TrimSpace(cfg.Timeout) != "" {
		cfg.Limits.MaxExecutionTime = cfg.Timeout
	}
	cfg.Timeout = ""
}

// ExecutionTimeout resolves the canonical Workstation execution limit.
func (Service) ExecutionTimeout(
	cfg *factorydefinitions.FactoryWorkstationConfig,
) (time.Duration, error) {
	if cfg == nil || strings.TrimSpace(cfg.Limits.MaxExecutionTime) == "" {
		return 0, nil
	}
	timeout, err := time.ParseDuration(cfg.Limits.MaxExecutionTime)
	if err != nil {
		return 0, fmt.Errorf(
			"invalid workstation limits.maxExecutionTime %q: %w",
			cfg.Limits.MaxExecutionTime,
			err,
		)
	}
	if timeout <= 0 {
		return 0, nil
	}
	return timeout, nil
}
