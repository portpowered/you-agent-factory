package current

import (
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
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

// TestAPIInvalidPromptTemplateNamesMissingVariables proves that prompt-template
// validation returns typed public diagnostics when a draft references an
// unavailable input index or an unknown variable path on the Current Factory
// workstation contract.
func TestAPIInvalidPromptTemplateNamesMissingVariables(t *testing.T) {
	rootDir := t.TempDir()
	seedNamedFactoryRoot(t, rootDir, "alpha", "alpha-task")

	server := startCurrentFactoryServer(t, rootDir)
	defer server.Stop(t)

	const sessionID = "~default"
	contract := getPromptTemplateContract(t, server.URL(), sessionID, defaultFunctionalWorkstationName)
	if contract.InputCount != 1 {
		t.Fatalf("contract input count = %d, want 1", contract.InputCount)
	}

	unavailableResult := validatePromptTemplateForSession(
		t,
		server.URL(),
		sessionID,
		defaultFunctionalWorkstationName,
		`{{ (index .Inputs 1).Payload }}`,
	)
	if unavailableResult.Valid {
		t.Fatalf("unavailable input validation valid = true, diagnostics = %#v", unavailableResult.Diagnostics)
	}
	if len(unavailableResult.Diagnostics) == 0 {
		t.Fatalf("unavailable input diagnostics = %#v, want typed public diagnostics", unavailableResult.Diagnostics)
	}
	if !promptTemplateValidationHasDiagnosticKind(unavailableResult, factoryapi.UNAVAILABLEVARIABLE) {
		t.Fatalf(
			"unavailable input diagnostics = %#v, want %s",
			unavailableResult.Diagnostics,
			factoryapi.UNAVAILABLEVARIABLE,
		)
	}

	invalidResult := validatePromptTemplateForSession(
		t,
		server.URL(),
		sessionID,
		defaultFunctionalWorkstationName,
		`{{ (index .Inputs 0).Unknown }}`,
	)
	if invalidResult.Valid {
		t.Fatalf("invalid field validation valid = true, diagnostics = %#v", invalidResult.Diagnostics)
	}
	if len(invalidResult.Diagnostics) == 0 {
		t.Fatalf("invalid field diagnostics = %#v, want typed public diagnostics", invalidResult.Diagnostics)
	}
	if !promptTemplateValidationHasDiagnosticKind(invalidResult, factoryapi.INVALIDVARIABLE) {
		t.Fatalf(
			"invalid field diagnostics = %#v, want %s",
			invalidResult.Diagnostics,
			factoryapi.INVALIDVARIABLE,
		)
	}
}
