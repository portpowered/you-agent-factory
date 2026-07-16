package logicaltarget

import (
	factorysessions "github.com/portpowered/infinite-you/pkg/factory/sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// APILogicalTarget maps one normalized logical target reference to the public
// client-safe API shape.
func APILogicalTarget(ref CanonicalReference) factoryapi.FactorySessionLogicalTarget {
	target := factoryapi.FactorySessionLogicalTarget{
		Kind:       apiLogicalTargetKind(ref.Kind),
		FolderPath: ref.FolderPath,
	}
	if ref.Kind == KindNamed {
		namedTarget := ref.NamedTarget
		target.NamedTarget = &namedTarget
	}
	if ref.Kind == KindProvider && ref.Provider != nil {
		target.ProviderBoundary = &factoryapi.FactorySessionLogicalProviderBoundary{
			Provider: ref.Provider.Provider,
			Kind:     ref.Provider.Kind,
			Boundary: ref.Provider.Boundary,
		}
	}
	return target
}

// RuntimeLogicalTarget maps one canonical reference to the Factory
// Session-owned live runtime projection contract.
func RuntimeLogicalTarget(ref CanonicalReference) factorysessions.RuntimeLogicalTarget {
	target := factorysessions.RuntimeLogicalTarget{
		Kind:       string(ref.Kind),
		FolderPath: ref.FolderPath,
	}
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

// APILogicalTargetFromSession derives the public normalized target metadata for
// one live session within backendScopeID.
func APILogicalTargetFromSession(
	backendScopeID string,
	session *factorysessions.LiveSession,
) (*factoryapi.FactorySessionLogicalTarget, error) {
	if session == nil {
		return nil, nil
	}
	ref, err := NormalizeTargetRef(backendScopeID, session.FolderPath, session.Target)
	if err != nil {
		return nil, err
	}
	target := APILogicalTarget(ref)
	return &target, nil
}

func apiLogicalTargetKind(kind Kind) factoryapi.FactorySessionLogicalTargetKind {
	switch kind {
	case KindNamed:
		return factoryapi.FactorySessionLogicalTargetKindNamed
	case KindProvider:
		return factoryapi.FactorySessionLogicalTargetKindProvider
	default:
		return factoryapi.FactorySessionLogicalTargetKindDefault
	}
}
