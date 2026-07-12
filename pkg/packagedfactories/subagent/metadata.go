package subagent

import (
	"strings"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

const (
	// PackagedFactoryName is the canonical named factory identifier for @you/subagent.
	PackagedFactoryName = "@you/subagent"
	// PackagedFactoryProject is the stable project id for the built-in subagent factory.
	PackagedFactoryProject = "builtin-subagent"
	// PackagedWorkTypeName is the DEFAULT-handled work type for one-pass subagent runs.
	PackagedWorkTypeName = "task"
	// PackagedRunWorkstationName is the single AGENT_RUN workstation name.
	PackagedRunWorkstationName = "run-subagent"
	// PackagedWorkerName is the single AGENT_WORKER name.
	PackagedWorkerName = "subagent-worker"
)

// IsPackagedFactory reports whether cfg identifies the built-in @you/subagent factory.
func IsPackagedFactory(cfg *interfaces.FactoryConfig) bool {
	if cfg == nil {
		return false
	}
	if strings.TrimSpace(cfg.Name) == PackagedFactoryName {
		return true
	}
	return strings.TrimSpace(cfg.Project) == PackagedFactoryProject
}
