// Package quorum owns metadata for the built-in @you/quorum factory.
package quorum

import (
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	builtin "github.com/portpowered/infinite-you/pkg/services/factory_definitions/packages/definitions/quorum"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/quorumpolicy"
)

var BuiltInFactoryJSON = builtin.BuiltInFactoryJSON

const (
	PackagedFactoryName    = factorydefinitions.PackagedQuorumFactoryName
	PackagedFactoryProject = factorydefinitions.PackagedQuorumFactoryProject
)

var IsPackagedFactory = quorumpolicy.IsPackagedQuorumFactory
