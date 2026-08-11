package factorydefinitions

import (
	"context"

	contracts "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/contracts"
)

type PreparedFactoryLayoutPayload = contracts.PreparedFactoryLayoutPayload
type FactorySplitLayoutReplaceResult = contracts.FactorySplitLayoutReplaceResult
type LayoutExpansionReport = contracts.LayoutExpansionReport
type NamedFactoryListEntry = contracts.NamedFactoryListEntry
type NamedFactoryResolution = contracts.NamedFactoryResolution
type NamedFactoryResolutionSource = contracts.NamedFactoryResolutionSource
type NamedFactoryPrecedenceDecision = contracts.NamedFactoryPrecedenceDecision
type Persistence = contracts.Persistence

// PackagedFactoryPersistence is the explicit persistence capability required
// by first-party packaged Factory installation. The packaged preparation path
// is intentionally separate from ordinary named-Factory persistence so its
// catalog-only lifecycle allowances cannot be discovered through a runtime
// type assertion or applied to customer-authored persistence.
type PackagedFactoryPersistence interface {
	Persistence
	PreparePackagedFactoryLayout(context.Context, string, []byte) (*PreparedFactoryLayoutPayload, error)
}

const (
	NamedFactoryResolutionSourceProjectLocal = contracts.NamedFactoryResolutionSourceProjectLocal
	NamedFactoryResolutionSourceGlobal       = contracts.NamedFactoryResolutionSourceGlobal

	NamedFactoryPrecedenceDecisionNone              = contracts.NamedFactoryPrecedenceDecisionNone
	NamedFactoryPrecedenceDecisionProjectOverGlobal = contracts.NamedFactoryPrecedenceDecisionProjectOverGlobal
)

var IsInvalidNamedFactoryName = contracts.IsInvalidNamedFactoryName
var IsNamedFactoryNotFound = contracts.IsNamedFactoryNotFound
