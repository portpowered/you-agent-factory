package factorydefinitions

import contracts "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/contracts"

type PreparedFactoryLayoutPayload = contracts.PreparedFactoryLayoutPayload
type FactorySplitLayoutReplaceResult = contracts.FactorySplitLayoutReplaceResult
type LayoutExpansionReport = contracts.LayoutExpansionReport
type NamedFactoryListEntry = contracts.NamedFactoryListEntry
type NamedFactoryResolution = contracts.NamedFactoryResolution
type NamedFactoryResolutionSource = contracts.NamedFactoryResolutionSource
type NamedFactoryPrecedenceDecision = contracts.NamedFactoryPrecedenceDecision
type Persistence = contracts.Persistence

const (
	NamedFactoryResolutionSourceProjectLocal = contracts.NamedFactoryResolutionSourceProjectLocal
	NamedFactoryResolutionSourceGlobal       = contracts.NamedFactoryResolutionSourceGlobal

	NamedFactoryPrecedenceDecisionNone              = contracts.NamedFactoryPrecedenceDecisionNone
	NamedFactoryPrecedenceDecisionProjectOverGlobal = contracts.NamedFactoryPrecedenceDecisionProjectOverGlobal
)

var IsInvalidNamedFactoryName = contracts.IsInvalidNamedFactoryName
var IsNamedFactoryNotFound = contracts.IsNamedFactoryNotFound
