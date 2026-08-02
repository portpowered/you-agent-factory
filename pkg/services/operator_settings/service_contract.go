package operatorsettings

import "context"

// Service is the singular cross-service Operator Settings root authority.
// Peer packages depend on this one named interface for document operations
// and effective resolution rather than nested document/resolution implementation
// packages or construction ports such as filesystem, encoder/decoder, or
// lifecycle collaborators.
type Service interface {
	// LoadDocument loads the operator document at the requested path. Missing
	// documents return EmptyDocument unless RequireExisting is true, which fails
	// with ErrDocumentNotFound. Malformed requests fail with ErrDocumentMalformed.
	LoadDocument(LoadDocumentRequest) (LoadDocumentResult, error)

	// ApplyDocumentUpdate applies a semantic document change and persists the
	// resulting document. Conflicts fail with ErrDocumentConflict, unsupported
	// updates fail with ErrDocumentUnsupported, and malformed requests fail
	// with ErrDocumentMalformed.
	ApplyDocumentUpdate(ApplyDocumentUpdateRequest) (ApplyDocumentUpdateResult, error)

	// ResolveEffective resolves an immutable effective selection from detached
	// document baseline and override facts. Resolution does not mutate the
	// operator document. Invalid inputs fail with ErrResolutionInvalidInput,
	// unsupported overrides fail with ErrResolutionUnsupportedOverride, and
	// baseline conflicts fail with ErrResolutionConflict.
	ResolveEffective(ResolveEffectiveRequest) (ResolveEffectiveResult, error)

	// DefaultConfigPath returns the service-owned operator configuration path
	// for one home directory.
	DefaultConfigPath(string) string

	// LoadFileConfig loads the complete operator configuration from an explicit
	// path. A missing file returns an empty configuration with runtime defaults.
	LoadFileConfig(string) (Config, error)

	// ResolveFromHomeWithEnvironment resolves file, environment, and flag
	// defaults using the service-owned configuration path and loader policy.
	ResolveFromHomeWithEnvironment(string, Defaults, FlagOverrides) (ResolvedDefaults, error)

	// EnsureLocalBackendScope reuses or atomically creates the local backend
	// identity in the service-owned configuration document.
	EnsureLocalBackendScope(string) (ResolvedBackendScope, error)

	// ProjectInputInventory returns the deterministic settings input inventory.
	ProjectInputInventory() InputInventory

	// DeriveProviderBackendScopeID derives a stable provider-backed scope value.
	DeriveProviderBackendScopeID(string, string, string) string

	// IsLocalBackendScopeID reports whether a value has the local scope shape.
	IsLocalBackendScopeID(string) bool

	// ConfigureACPIntegrationAdd persists one additional ACP integration.
	ConfigureACPIntegrationAdd(context.Context, string, ACPIntegration) (Document, error)

	// ConfigureACPIntegrationDelete persists removal of one ACP integration.
	ConfigureACPIntegrationDelete(context.Context, string, string) (Document, error)

	// EnsurePackagedACPIntegrations materializes packaged integrations only when
	// the customer has not supplied the ACP integration list.
	EnsurePackagedACPIntegrations(context.Context, string, []ACPIntegration) (Document, error)

	// ResolveACPAgentProfile resolves an immutable effective ACP agent profile
	// from a detached authored-document fact. A request with no authored
	// profile resolves to BuiltInACPAgentProfile. Resolution does not read or
	// mutate the operator document, and invalid profiles fail with
	// ACPAgentProfileFailure (ErrACPAgentProfileInvalid).
	ResolveACPAgentProfile(ResolveACPAgentProfileRequest) (ResolveACPAgentProfileResult, error)
}
