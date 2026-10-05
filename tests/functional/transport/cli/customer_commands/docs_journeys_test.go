package customer_commands_test

import "testing"

// TestCLICommandDocumentationJourneys owns its immutable fixture until all customer scenarios finish.
func TestCLICommandDocumentationJourneys(t *testing.T) {
	t.Parallel()
	initializeDocsFixture(t)
	t.Run("TestDocsFailureBoundariesAndReuse", testDocsDocsFailureBoundariesAndReuse)
	t.Run("TestInstalledDocumentationBehaviorThroughPublicProcess", testDocsInstalledDocumentationBehaviorThroughPublicProcess)
	t.Run("TestDocsAliasReturnsCanonicalCustomerDocumentation", testDocsDocsAliasReturnsCanonicalCustomerDocumentation)
}
