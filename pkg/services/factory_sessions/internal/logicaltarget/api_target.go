package logicaltarget

import (
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
)

// RuntimeLogicalTarget maps one canonical reference to the Factory
// Session-owned live runtime projection contract.
func RuntimeLogicalTarget(ref CanonicalReference) factorysessions.RuntimeLogicalTarget {
	return factorysessions.RuntimeLogicalTargetFromReference(ref)
}
