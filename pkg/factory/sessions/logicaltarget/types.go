package logicaltarget

// Kind identifies the normalized logical session target selector.
type Kind string

const (
	KindDefault  Kind = "default"
	KindNamed    Kind = "named"
	KindProvider Kind = "provider"
)

// ProviderBoundary scopes a provider-backed logical session target to a stable
// workspace or account boundary without carrying secrets.
type ProviderBoundary struct {
	Provider string
	Kind     string
	Boundary string
}

// CanonicalReference is the stable normalized factory session target reference
// used to derive logical session identity within one backend scope.
type CanonicalReference struct {
	BackendScopeID string
	FolderPath     string
	Kind           Kind
	NamedTarget    string
	Provider       *ProviderBoundary
}
