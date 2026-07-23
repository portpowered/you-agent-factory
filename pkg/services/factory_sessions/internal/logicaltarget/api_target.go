package logicaltarget

import (
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
)

// RuntimeLogicalTarget maps one canonical reference to the Factory
// Session-owned live runtime projection contract.
func RuntimeLogicalTarget(ref CanonicalReference) factorysessions.RuntimeLogicalTarget {
	target := factorysessions.RuntimeLogicalTarget{Kind: string(ref.Kind), FolderPath: ref.FolderPath}
	if ref.Kind == KindNamed {
		namedTarget := ref.NamedTarget
		target.NamedTarget = &namedTarget
	}
	if ref.Kind == KindProvider && ref.Provider != nil {
		target.ProviderBoundary = &factorysessions.RuntimeLogicalProviderBoundary{
			Provider: ref.Provider.Provider,
			Kind:     ref.Provider.Kind,
			Boundary: ref.Provider.Boundary,
		}
	}
	return target
}
