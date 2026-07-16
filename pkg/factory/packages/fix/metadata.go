// Package fix owns public metadata for the @you/fix packaged factory.
package fix

import builtinfix "github.com/portpowered/infinite-you/pkg/factory/packages/definitions/fix"

// BuiltInFactoryJSON is the canonical runnable @you/fix definition.
var BuiltInFactoryJSON = builtinfix.BuiltInFixFactoryJSON

const (
	PackagedFactoryName              = "@you/fix"
	PackagedFactoryProject           = "builtin-fix"
	PackagedWorkTypeName             = "fix"
	PackagedPlanWorkstationName      = "plan-fix"
	PackagedImplementWorkstationName = "implement-fix"
	PackagedReviewWorkstationName    = "review-fix"
)
