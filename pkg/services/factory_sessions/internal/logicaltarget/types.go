package logicaltarget

import factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"

// Kind identifies the normalized logical session target selector.
type Kind = factorysessions.LogicalTargetKind

const (
	KindDefault  = factorysessions.LogicalTargetKindDefault
	KindNamed    = factorysessions.LogicalTargetKindNamed
	KindProvider = factorysessions.LogicalTargetKindProvider
)

// ProviderBoundary scopes a provider-backed logical session target to a stable
// workspace or account boundary without carrying secrets.
type ProviderBoundary = factorysessions.LogicalTargetProviderBoundary

// CanonicalReference is the stable normalized factory session target reference
// used to derive logical session identity within one backend scope.
type CanonicalReference = factorysessions.CanonicalLogicalTargetReference
