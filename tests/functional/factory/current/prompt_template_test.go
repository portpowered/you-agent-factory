package current

import (
	"testing"
)

// TestAPIPromptTemplateContractAndValidationRoundTrip proves that a Factory Session
// can fetch the workstation prompt-template contract for the Current Factory and
// validate a draft prompt that references variables exposed by that contract.
func TestAPIPromptTemplateContractAndValidationRoundTrip(t *testing.T) {
	rootDir := t.TempDir()
	seedNamedFactoryRoot(t, rootDir, "alpha", "alpha-task")

	server := startCurrentFactoryServer(t, rootDir)
	defer server.Stop(t)

	const sessionID = "~default"
	contract := getPromptTemplateContract(t, server.URL(), sessionID, defaultFunctionalWorkstationName)
	if contract.InputCount != 1 {
		t.Fatalf("contract input count = %d, want 1", contract.InputCount)
	}
	if len(contract.AvailableVariables) == 0 {
		t.Fatalf("contract available variables = %#v, want populated list", contract.AvailableVariables)
	}
	if !promptTemplateContractHasVariablePath(contract, ".Context.SessionID") {
		t.Fatalf("contract available variables = %#v, want .Context.SessionID", contract.AvailableVariables)
	}
	if !promptTemplateContractHasVariablePath(contract, ".Inputs[0].Payload") {
		t.Fatalf("contract available variables = %#v, want .Inputs[0].Payload", contract.AvailableVariables)
	}

	result := validatePromptTemplateForSession(
		t,
		server.URL(),
		sessionID,
		defaultFunctionalWorkstationName,
		`you submit --session {{ .Context.SessionID }} --work {{ (index .Inputs 0).Payload }}`,
	)
	if !result.Valid {
		t.Fatalf("validation result valid = false, diagnostics = %#v", result.Diagnostics)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("validation diagnostics = %#v, want none", result.Diagnostics)
	}
}
