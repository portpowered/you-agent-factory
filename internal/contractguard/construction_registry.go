package contractguard

import "github.com/portpowered/infinite-you/internal/lint/analyzers"

// These aliases keep the legacy scanner operational during its replacement.
// Delete this bridge with the scanner once all provenance rules are migrated.
type ConstructionSymbol = analyzers.ConstructionSymbol
type ConstructionMode = analyzers.ConstructionMode
type ConstructionKind = analyzers.ConstructionKind
type ConstructionCapabilitySet = analyzers.ConstructionCapabilitySet
type ConstructionParameter = analyzers.ConstructionParameter
type ConstructionConstructor = analyzers.ConstructionConstructor
type ConstructionType = analyzers.ConstructionType
type ConstructionAllowance = analyzers.ConstructionAllowance
type ConstructionRegistry = analyzers.ConstructionRegistry

const (
	ConstructionReport   = analyzers.ConstructionReport
	ConstructionEnforce  = analyzers.ConstructionEnforce
	ConstructionBehavior = analyzers.ConstructionBehavior
	ConstructionEffect   = analyzers.ConstructionEffect
	ConstructionState    = analyzers.ConstructionState
	ConstructionResource = analyzers.ConstructionResource
	ConstructionDomain   = analyzers.ConstructionDomain
)

func RepositoryConstructionRegistry() ConstructionRegistry {
	return analyzers.RepositoryConstructionRegistry()
}
