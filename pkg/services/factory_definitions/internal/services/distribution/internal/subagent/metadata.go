package subagent

import (
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

const (
	// PackagedFactoryName is the canonical named factory identifier for @you/subagent.
	PackagedFactoryName = factorydefinitions.PackagedSubagentFactoryName
	// PackagedFactoryProject is the stable project id for the built-in subagent factory.
	PackagedFactoryProject = factorydefinitions.PackagedSubagentFactoryProject
	// PackagedWorkTypeName is the DEFAULT-handled work type for one-pass subagent runs.
	PackagedWorkTypeName = factorydefinitions.PackagedSubagentWorkTypeName
	// PackagedRunWorkstationName is the single AGENT_RUN workstation name.
	PackagedRunWorkstationName = factorydefinitions.PackagedSubagentRunWorkstationName
	// PackagedWorkerName is the single AGENT_WORKER name.
	PackagedWorkerName = factorydefinitions.PackagedSubagentWorkerName
)

// IsPackagedFactory reports whether cfg identifies the built-in @you/subagent factory.
func IsPackagedFactory(cfg *factorydefinitions.FactoryConfig) bool {
	if cfg == nil {
		return false
	}
	if strings.TrimSpace(cfg.Name) == PackagedFactoryName {
		return true
	}
	return strings.TrimSpace(cfg.Project) == PackagedFactoryProject
}
