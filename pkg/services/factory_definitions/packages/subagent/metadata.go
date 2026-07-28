// Package subagent is a transitional shim over the Distribution-owned subagent
// package asset/metadata implementation.
package subagent

import (
	distributionsubagent "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/distribution/subagent"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

const (
	PackagedFactoryName      = distributionsubagent.PackagedFactoryName
	PackagedFactoryProject   = distributionsubagent.PackagedFactoryProject
	PackagedWorkTypeName     = distributionsubagent.PackagedWorkTypeName
	PackagedRunWorkstationName = distributionsubagent.PackagedRunWorkstationName
	PackagedWorkerName       = distributionsubagent.PackagedWorkerName
)

// IsPackagedFactory reports whether cfg identifies the built-in @you/subagent factory.
func IsPackagedFactory(cfg *factorydefinitions.FactoryConfig) bool {
	return distributionsubagent.IsPackagedFactory(cfg)
}
