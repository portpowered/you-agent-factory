package operatorsettings

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
}
