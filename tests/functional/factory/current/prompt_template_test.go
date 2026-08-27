package current

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// testSharedPromptTemplateContractAndValidation proves that an explicit Factory
// Session can fetch the workstation prompt-template contract for its Current
// Factory and validate a draft prompt that references its available variables.
func testSharedPromptTemplateContractAndValidation(t *testing.T, fixture *sharedCurrentFactoryAPI) {
	fixture.requireServerRunning(t)
	session := fixture.openSession(t, "alpha-prompt-contract")
	current := getCurrentFactoryForSession(t, session.serverURL, session.id)
	if current.Name != factoryapi.FactoryName("alpha-prompt-contract") {
		t.Fatalf("prompt contract session current factory name = %q, want alpha-prompt-contract", current.Name)
	}
	assertFactoryWorkType(t, current, "alpha-prompt-contract-task", "prompt contract session current factory")
	contract := getPromptTemplateContract(t, session.serverURL, session.id, defaultFunctionalWorkstationName)
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
		session.serverURL,
		session.id,
		defaultFunctionalWorkstationName,
		`you submit --session {{ .Context.SessionID }} --work {{ (index .Inputs 0).Payload }}`,
	)
	if !result.Valid {
		t.Fatalf("validation result valid = false, diagnostics = %#v", result.Diagnostics)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("validation diagnostics = %#v, want none", result.Diagnostics)
	}
	fixture.requireServerRunning(t)
}

// testSharedInvalidPromptTemplate proves that prompt-template
// validation returns typed public diagnostics when a draft references an
// unavailable input index or an unknown variable path on the Current Factory
// workstation contract.
func testSharedInvalidPromptTemplate(t *testing.T, fixture *sharedCurrentFactoryAPI) {
	fixture.requireServerRunning(t)
	session := fixture.openSession(t, "alpha-prompt-invalid")
	current := getCurrentFactoryForSession(t, session.serverURL, session.id)
	if current.Name != factoryapi.FactoryName("alpha-prompt-invalid") {
		t.Fatalf("invalid prompt session current factory name = %q, want alpha-prompt-invalid", current.Name)
	}
	assertFactoryWorkType(t, current, "alpha-prompt-invalid-task", "invalid prompt session current factory")
	contract := getPromptTemplateContract(t, session.serverURL, session.id, defaultFunctionalWorkstationName)
	if contract.InputCount != 1 {
		t.Fatalf("contract input count = %d, want 1", contract.InputCount)
	}

	unavailableResult := validatePromptTemplateForSession(
		t,
		session.serverURL,
		session.id,
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
		session.serverURL,
		session.id,
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
	fixture.requireServerRunning(t)
}

// testSharedTemplateValidationDoesNotMutate proves that prompt-template
// validation leaves the Current Factory definition unchanged within the same
// Factory Session when validating both valid and invalid workstation prompt drafts.
func testSharedTemplateValidationDoesNotMutate(t *testing.T, fixture *sharedCurrentFactoryAPI) {
	fixture.requireServerRunning(t)
	session := fixture.openSession(t, "alpha-prompt-nonmutation")
	before := getCurrentFactoryForSession(t, session.serverURL, session.id)
	if before.Version == nil {
		t.Fatal("current factory version = nil, want version metadata")
	}
	assertFactoryWorkType(t, before, "alpha-prompt-nonmutation-task", "current factory before validation")

	validResult := validatePromptTemplateForSession(
		t,
		session.serverURL,
		session.id,
		defaultFunctionalWorkstationName,
		`you submit --session {{ .Context.SessionID }} --work {{ (index .Inputs 0).Payload }}`,
	)
	if !validResult.Valid {
		t.Fatalf("valid prompt validation valid = false, diagnostics = %#v", validResult.Diagnostics)
	}

	invalidResult := validatePromptTemplateForSession(
		t,
		session.serverURL,
		session.id,
		defaultFunctionalWorkstationName,
		`{{ (index .Inputs 1).Payload }}`,
	)
	if invalidResult.Valid {
		t.Fatalf("invalid prompt validation valid = true, diagnostics = %#v", invalidResult.Diagnostics)
	}

	after := getCurrentFactoryForSession(t, session.serverURL, session.id)
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("current factory after validation = %#v, want unchanged %#v", after, before)
	}
	fixture.requireServerRunning(t)
}

// TestProcessWorkNameMapsIntoPromptTemplate proves that a submitted Work name is
// rendered into the workstation prompt template before provider invocation.
func TestProcessWorkNameMapsIntoPromptTemplate(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "name_propagation"))
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		Name:       "design-doc-review",
		WorkTypeID: "task",
		Payload:    []byte(`review the design document`),
		TraceID:    "trace-prompt-test",
	})

	provider := testutil.NewMockProvider(
		workerexecution.InferenceResponse{Content: "Reviewed. COMPLETE"},
	)
	_, listed := support.RunFactoryToCompletionWithEdgesAndWork(
		t,
		dir,
		serviceedges.Edges{ProviderOverride: provider},
		10*time.Second,
	)
	assertCurrentFactoryWorkCustomerStates(t, listed, map[string]int{
		support.WorkCustomerLocation("task", "complete"): 1,
	})

	providerCalls := provider.Calls()
	if len(providerCalls) == 0 {
		t.Fatal("provider calls = 0, want at least 1")
	}
	if userMessage := providerCalls[0].UserMessage; !strings.Contains(userMessage, "Task Name: design-doc-review") {
		t.Errorf("provider user message = %q, want Task Name: design-doc-review", userMessage)
	}
}

// TestProcessMarkdownWorkNameAndPayloadMapIntoPromptTemplate proves that seeded
// markdown Work name and payload content render into the workstation prompt.
func TestProcessMarkdownWorkNameAndPayloadMapIntoPromptTemplate(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "name_propagation"))
	testutil.WriteSeedMarkdownFile(t, dir, "task", "architecture-review",
		[]byte("# Architecture Review\n\nPlease review the system architecture."))

	provider := testutil.NewMockProvider(
		workerexecution.InferenceResponse{Content: "Reviewed. COMPLETE"},
	)
	_, listed := support.RunFactoryToCompletionWithEdgesAndWork(
		t,
		dir,
		serviceedges.Edges{ProviderOverride: provider},
		10*time.Second,
	)
	assertCurrentFactoryWorkCustomerStates(t, listed, map[string]int{
		support.WorkCustomerLocation("task", "complete"): 1,
		support.WorkCustomerLocation("task", "init"):     0,
	})

	providerCalls := provider.Calls()
	if len(providerCalls) == 0 {
		t.Fatal("provider calls = 0, want at least 1")
	}
	userMessage := providerCalls[0].UserMessage
	if !strings.Contains(userMessage, "Task Name: architecture-review") {
		t.Errorf("provider user message = %q, want Task Name: architecture-review", userMessage)
	}
	if !strings.Contains(userMessage, "# Architecture Review") {
		t.Errorf("provider user message = %q, want markdown payload content", userMessage)
	}
	assertCompletedWorkName(t, listed, "task", "architecture-review")
}

// TestProcessSubmissionTagsReachDispatchInputTokens proves that submission tags
// remain available on dispatch input tokens for parameterized workstation fields.
func TestProcessSubmissionTagsReachDispatchInputTokens(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "repeater_workstation"))
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkTypeID: "task",
		Payload:    []byte(`{}`),
		Tags:       map[string]string{"branch": "feature-abc"},
	})

	provider := testutil.NewMockWorkerMapProvider(map[string][]workerexecution.InferenceResponse{
		"exec-worker":   {{Content: "done COMPLETE"}},
		"finish-worker": {{Content: "done COMPLETE"}},
	})
	_, listed := support.RunFactoryToCompletionWithEdgesAndWork(
		t,
		dir,
		serviceedges.Edges{ProviderOverride: provider},
		10*time.Second,
	)
	assertCurrentFactoryWorkCustomerStates(t, listed, map[string]int{
		support.WorkCustomerLocation("task", "complete"): 1,
	})

	calls := provider.Calls("exec-worker")
	if len(calls) == 0 {
		t.Fatal("exec-worker provider calls = 0, want at least 1")
	}
	call := calls[0]
	if call.Dispatch.WorkstationName == "" {
		t.Error("dispatch workstation name is empty, want populated workstation")
	}
	if len(call.Dispatch.InputTokens) == 0 {
		t.Fatal("dispatch input tokens = 0, want at least 1")
	}
	tags := firstDispatchInputToken(call.Dispatch.InputTokens).Color.Tags
	if tags["branch"] != "feature-abc" {
		t.Errorf("dispatch input tag branch = %q, want feature-abc", tags["branch"])
	}
}

// TestProcessParameterizedTemplateFailureRoutesWorkToFailed proves that an
// unresolved workstation prompt template routes Work to failed without invoking
// the provider.
func TestProcessParameterizedTemplateFailureRoutesWorkToFailed(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "parameterized_failure"))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "unresolved template test"}`))

	provider := testutil.NewMockProvider(
		workerexecution.InferenceResponse{Content: "Should not reach COMPLETE"},
	)
	_, listed := support.RunFactoryToCompletionWithEdgesAndWork(
		t,
		dir,
		serviceedges.Edges{ProviderOverride: provider},
		10*time.Second,
	)
	assertCurrentFactoryWorkCustomerStates(t, listed, map[string]int{
		support.WorkCustomerLocation("task", "failed"):   1,
		support.WorkCustomerLocation("task", "complete"): 0,
	})
	if provider.CallCount() != 0 {
		t.Errorf("provider call count = %d, want 0 before invocation", provider.CallCount())
	}
}
