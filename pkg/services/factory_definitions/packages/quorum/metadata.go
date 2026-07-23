// Package quorum owns metadata for the built-in @you/quorum factory.
package quorum

import (
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/quorumpolicy"
)

const (
	PackagedFactoryName    = factorydefinitions.PackagedQuorumFactoryName
	PackagedFactoryProject = factorydefinitions.PackagedQuorumFactoryProject
)

var IsPackagedFactory = quorumpolicy.IsPackagedQuorumFactory
