// Package testlink registers the nested document owner constructor for
// Operator Settings unit tests without creating an import cycle.
package testlink

// RegisterDocumentOwner wires the nested document owner constructor into Operator Settings
// unit tests.
func RegisterDocumentOwner() {
	// Document owners are explicitly constructed by each test's wire helper.
}

// RegisterProvidersRoot wires the Providers root constructor used by transitional
// servicewire composition in tests that do not load pkg/wire.
func RegisterProvidersRoot() {
	// Providers roots are passed directly to Settings wire constructors.
}

// RegisterComposition wires transitional Settings composition hooks for tests.
func RegisterComposition() {
	RegisterDocumentOwner()
	RegisterProvidersRoot()
}
