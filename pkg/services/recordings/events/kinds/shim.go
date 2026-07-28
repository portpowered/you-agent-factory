package factoryeventkinds

import (
	ledgereventskinds "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/canonical_ledger/events/kinds"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
)

var (
	IsPublicEmittableFactoryEventKind = ledgereventskinds.IsPublicEmittableFactoryEventKind
	OpenAPISchemaNameFromRef          = ledgereventskinds.OpenAPISchemaNameFromRef
	ValidateBundledFactoryEventKindParity = ledgereventskinds.ValidateBundledFactoryEventKindParity
	ValidateFactoryEventKindParity    = ledgereventskinds.ValidateFactoryEventKindParity
	ValidateFactoryEventTypePayloadMapping = ledgereventskinds.ValidateFactoryEventTypePayloadMapping

	ContractOnlyFactoryEventKinds        = ledgereventskinds.ContractOnlyFactoryEventKinds
	ExcludedNonPublicFactoryEventKinds   = ledgereventskinds.ExcludedNonPublicFactoryEventKinds
	CompareFactoryEventKindParity        = ledgereventskinds.CompareFactoryEventKindParity
	LoadFactoryEventKindParityInputFromOpenAPIYAML = ledgereventskinds.LoadFactoryEventKindParityInputFromOpenAPIYAML
	ParseFactoryEventTypePayloadMapping  = ledgereventskinds.ParseFactoryEventTypePayloadMapping
	PublicEmittableFactoryEventKinds     = ledgereventskinds.PublicEmittableFactoryEventKinds
)

type (
	ContractOnlyKind                    = ledgereventskinds.ContractOnlyKind
	ExcludedNonPublicKind               = ledgereventskinds.ExcludedNonPublicKind
	FactoryEventKindParityDrift         = ledgereventskinds.FactoryEventKindParityDrift
	FactoryEventKindParityInput         = ledgereventskinds.FactoryEventKindParityInput
	FactoryEventTypePayloadMappingEntry = ledgereventskinds.FactoryEventTypePayloadMappingEntry
	PublicEmittableKind                 = ledgereventskinds.PublicEmittableKind
)

// Preserve recordings root import for vocabulary boundary tests that assert the
// shim still depends on the published Recordings contract.
var _ = recordings.FactoryEventTypeRunRequest
