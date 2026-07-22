package review

import builtinreview "github.com/portpowered/infinite-you/pkg/services/factory_definitions/packages/definitions/review"

// BuiltInFactoryJSON is the canonical runnable @you/review definition.
var BuiltInFactoryJSON = builtinreview.BuiltInReviewFactoryJSON

const (
	PackagedFactoryName                   = "@you/review"
	PackagedFactoryProject                = "builtin-review"
	PackagedWorkTypeName                  = "reviewable-work"
	PackagedExecuteWorkstationName        = "execute-review-work"
	PackagedReviewWorkstationName         = "review-review-work"
	PackagedInvocationReturnTerminalState = "approved"
)
