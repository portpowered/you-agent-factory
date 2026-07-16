// Package classifier owns the @you/classifier packaged-factory identity and topology metadata.
package classifier

import builtinclassifier "github.com/portpowered/infinite-you/pkg/factory/packages/definitions/classifier"

// BuiltInFactoryJSON is the canonical runnable @you/classifier definition.
var BuiltInFactoryJSON = builtinclassifier.BuiltInFactoryJSON

const (
	PackagedFactoryName    = "@you/classifier"
	PackagedFactoryProject = "builtin-classifier"
	ClassifierWorkstation  = "classify-complexity"
)
