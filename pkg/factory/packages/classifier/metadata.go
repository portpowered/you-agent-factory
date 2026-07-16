// Package classifier owns the @you/classifier packaged-factory identity and topology metadata.
package classifier

import (
	"strings"

	builtinclassifier "github.com/portpowered/infinite-you/pkg/factory/packages/definitions/classifier"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

// BuiltInFactoryJSON is the canonical runnable @you/classifier definition.
var BuiltInFactoryJSON = builtinclassifier.BuiltInFactoryJSON

const (
	PackagedFactoryName    = "@you/classifier"
	PackagedFactoryProject = "builtin-classifier"
	ClassifierWorkstation  = "classify-complexity"
)

// IsPackagedFactory reports whether cfg identifies the built-in classifier factory.
func IsPackagedFactory(cfg *interfaces.FactoryConfig) bool {
	if cfg == nil {
		return false
	}
	return strings.TrimSpace(cfg.Name) == PackagedFactoryName ||
		strings.TrimSpace(cfg.Project) == PackagedFactoryProject
}
