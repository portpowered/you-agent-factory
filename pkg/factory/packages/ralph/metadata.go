// Package ralph exposes metadata for the @you/ralph packaged factory.
package ralph

import builtinralph "github.com/portpowered/infinite-you/pkg/factory/packages/definitions/ralph"

var BuiltInFactoryJSON = builtinralph.BuiltInRalphFactoryJSON

const (
	PackagedFactoryName            = "@you/ralph"
	PackagedFactoryProject         = "builtin-ralph"
	PackagedWorkTypeName           = "ralph"
	PackagedPlanWorkstationName    = "plan-ralph"
	PackagedExecuteWorkstationName = "execute-ralph"
	PackagedLoopBreakerName        = "execute-ralph-loop-breaker"
)
