package customer_workflows_test

import "testing"

// TestWorkflowPolicyJourneys owns its immutable fixture until all customer scenarios finish.
func TestWorkflowPolicyJourneys(t *testing.T) {
	t.Parallel()
	initializePolicyFixture(t)
	t.Run("TestJavaScriptDeniedChildOperationReturnsStablePolicyDiagnostic", testPolicyJavaScriptDeniedChildOperationReturnsStablePolicyDiagnostic)
	t.Run("TestJavaScriptPolicyFailureDoesNotDispatchExternalWork", testPolicyJavaScriptPolicyFailureDoesNotDispatchExternalWork)
}
