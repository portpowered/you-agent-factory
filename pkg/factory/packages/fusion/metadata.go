package fusion

import (
	"strings"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	builtinfusion "github.com/portpowered/infinite-you/pkg/factory/packages/definitions/fusion"
)

// BuiltInFactoryJSON is the canonical runnable @you/fusion definition owned
// by the factory packages family.
var BuiltInFactoryJSON = builtinfusion.BuiltInFactoryJSON

const (
	// PackagedFactoryName is the canonical named factory identifier for @you/fusion.
	PackagedFactoryName = "@you/fusion"
	// PackagedFactoryProject is the stable project id for the built-in fusion factory.
	PackagedFactoryProject = "builtin-fusion"
)

// IsPackagedFactory reports whether cfg identifies the built-in @you/fusion factory.
func IsPackagedFactory(cfg *interfaces.FactoryConfig) bool {
	if cfg == nil {
		return false
	}
	if strings.TrimSpace(cfg.Name) == PackagedFactoryName {
		return true
	}
	return strings.TrimSpace(cfg.Project) == PackagedFactoryProject
}
