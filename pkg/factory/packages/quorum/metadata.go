// Package quorum owns metadata for the built-in @you/quorum factory.
package quorum

import (
	"strings"

	builtinquorum "github.com/portpowered/infinite-you/pkg/factory/packages/definitions/quorum"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

// BuiltInFactoryJSON is the canonical runnable @you/quorum definition.
var BuiltInFactoryJSON = builtinquorum.BuiltInFactoryJSON

const (
	// PackagedFactoryName is the canonical named factory identifier for @you/quorum.
	PackagedFactoryName = "@you/quorum"
	// PackagedFactoryProject is the stable project ID for the built-in quorum factory.
	PackagedFactoryProject = "builtin-quorum"
)

// IsPackagedFactory reports whether cfg identifies the built-in @you/quorum factory.
func IsPackagedFactory(cfg *interfaces.FactoryConfig) bool {
	if cfg == nil {
		return false
	}
	return strings.TrimSpace(cfg.Name) == PackagedFactoryName ||
		strings.TrimSpace(cfg.Project) == PackagedFactoryProject
}
