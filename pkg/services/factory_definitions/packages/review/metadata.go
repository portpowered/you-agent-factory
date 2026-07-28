// Package review is a transitional shim over the Distribution-owned review package
// asset/metadata implementation.
package review

import distributionreview "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/distribution/review"

const (
	PackagedFactoryName                   = distributionreview.PackagedFactoryName
	PackagedFactoryProject                = distributionreview.PackagedFactoryProject
	PackagedWorkTypeName                  = distributionreview.PackagedWorkTypeName
	PackagedExecuteWorkstationName        = distributionreview.PackagedExecuteWorkstationName
	PackagedReviewWorkstationName         = distributionreview.PackagedReviewWorkstationName
	PackagedInvocationReturnTerminalState = distributionreview.PackagedInvocationReturnTerminalState
)
