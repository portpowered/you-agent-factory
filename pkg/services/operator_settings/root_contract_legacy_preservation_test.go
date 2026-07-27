package operatorsettings

import "testing"

// TestImplementationConstructionPortsRemainOwnerLocal proves existing Operator
// Settings implementation and construction ports remain in place for later
// IMP-SET-* absorption while the published Service seam stays peer-facing.
func TestImplementationConstructionPortsRemainOwnerLocal(t *testing.T) {
	t.Parallel()

	// Construction ports remain owner-local; peer Service method signatures do
	// not accept them as caller parameters.
	var (
		_ FileSystem
		_ CreateTemporaryFile
		_ ConfigDecoder
		_ ConfigEncoder
		_ ConfigLoader
		_ IDGenerator
		_ ProviderCatalog
		_ Config
		_ ConfigDocument
	)
}
