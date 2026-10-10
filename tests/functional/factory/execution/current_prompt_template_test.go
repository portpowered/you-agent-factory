package execution_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/work"
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

// TestPromptRenderingJourneys observes rendered names, markdown and template
// failures through terminal Work and the external provider command boundary.
func TestPromptRenderingJourneys(t *testing.T) {
	t.Parallel()
	host := newFactorySessionHost(t, serviceedges.Edges{WorkersWorktreeGit: promptJourneyGit{}})
	t.Run("ProcessSubmissionTagsParameterizeWorkerDirectory", func(t *testing.T) { testProcessSubmissionTagsParameterizeWorkerDirectory(t, host) })
	t.Run("ProcessWorkNameMapsIntoPromptTemplate", func(t *testing.T) { testProcessWorkNameMapsIntoPromptTemplate(t, host) })
	t.Run("ProcessMarkdownWorkNameAndPayloadMapIntoPromptTemplate", func(t *testing.T) { testProcessMarkdownWorkNameAndPayloadMapIntoPromptTemplate(t, host) })
	t.Run("ProcessParameterizedTemplateFailureRoutesWorkToFailed", func(t *testing.T) { testProcessParameterizedTemplateFailureRoutesWorkToFailed(t, host) })
	for _, checkout := range []string{"created", "reused", "acquire-denied", "invalid-existing"} {
		t.Run("Worktree/"+checkout, func(t *testing.T) { testPromptWorktree(t, host, checkout) })
	}
}

// W7 uses the group's reusable process and explicit sessions. Only Git is
// controlled; workspace files and terminal Work/event readback remain real.
func testPromptWorktree(t *testing.T, host *factorySessionHost, checkout string) {
	t.Parallel()
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "name_propagation"))
	path := filepath.Join(dir, "factory.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	config["workstations"].([]any)[0].(map[string]any)["worktree"] = checkout
	data, err = json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	support.WriteAgentConfig(t, dir, "executor", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "gpt-5-codex"))
	selected := filepath.Join(dir, ".worktrees", checkout)
	peer := filepath.Join(dir, ".worktrees", "unrelated", "customer.txt")
	writePromptWorkspaceFile(t, peer, "peer uncommitted content")
	if checkout == "reused" || checkout == "invalid-existing" {
		writePromptWorkspaceFile(t, filepath.Join(selected, "customer.txt"), "selected uncommitted content")
	}
	if checkout == "reused" {
		writePromptWorkspaceFile(t, filepath.Join(selected, ".git"), "gitdir: controlled checkout\n")
	}
	// Registered before Run: its session-close cleanup runs first, then these
	// assertions observe retained customer files after the owned lifecycle ends.
	t.Cleanup(func() { assertPromptWorktreeFiles(t, selected, peer, checkout) })
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{Name: "selected-work", WorkTypeID: "task", Payload: []byte("selected workspace request")})
	provider := support.NewRecordingCommandRunner("selected workspace output COMPLETE")
	_, listed, events := host.Run(t, dir, provider, 10*time.Second)
	failed := checkout == "acquire-denied" || checkout == "invalid-existing"
	assertPromptWorktreeOutcome(t, listed, events, provider.CallCount(), checkout)
	if !failed {
		if calls := provider.Requests(); len(calls) != 1 || calls[0].WorkDir != selected {
			t.Fatalf("provider workspace = %#v, want %s", calls, selected)
		}
		assertCompletedWorkName(t, listed, "task", "selected-work")
	}
}

func assertPromptWorktreeFiles(t *testing.T, selected, peer, checkout string) {
	t.Helper()
	assertPromptWorkspaceFile(t, peer, "peer uncommitted content")
	if checkout == "reused" || checkout == "invalid-existing" {
		assertPromptWorkspaceFile(t, filepath.Join(selected, "customer.txt"), "selected uncommitted content")
	}
	if checkout == "created" || checkout == "reused" {
		assertPromptWorkspaceFile(t, filepath.Join(selected, ".git"), "gitdir: controlled checkout\n")
	}
	if checkout == "acquire-denied" {
		if _, err := os.Stat(selected); !os.IsNotExist(err) {
			t.Fatalf("failed acquisition published workspace: %v", err)
		}
	}
}

func assertPromptWorktreeOutcome(t *testing.T, listed factoryapi.ListWorkResponse, events []factoryapi.FactoryEvent, calls int, checkout string) {
	t.Helper()
	failed := checkout == "acquire-denied" || checkout == "invalid-existing"
	complete, failures := 1, 0
	if failed {
		complete, failures = 0, 1
	}
	assertCurrentFactoryWorkCustomerStates(t, listed, map[string]int{
		support.WorkCustomerLocation("task", "complete"): complete,
		support.WorkCustomerLocation("task", "failed"):   failures,
	})
	if calls != complete {
		t.Fatalf("provider calls = %d, want %d", calls, complete)
	}
	raw, err := json.Marshal(events)
	if err != nil {
		t.Fatal(err)
	}
	want := "selected workspace output COMPLETE"
	if failed {
		want = "checkout acquisition denied"
		if checkout == "invalid-existing" {
			want = "exists but is not a valid git worktree"
		}
		if strings.Contains(string(raw), "selected workspace output COMPLETE") {
			t.Fatal("failed acquisition published a successful provider output")
		}
	}
	if !strings.Contains(string(raw), want) {
		t.Fatalf("terminal events lack %q: %s", want, raw)
	}
}

func writePromptWorkspaceFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func assertPromptWorkspaceFile(t *testing.T, path, want string) {
	t.Helper()
	if got, err := os.ReadFile(path); err != nil || string(got) != want {
		t.Fatalf("workspace content %s = %q, %v, want %q", path, got, err, want)
	}
}

// Git outcomes are immutable and selected by customer-authored checkout names;
// no scenario switches shared state or launches a Git subprocess.
type promptJourneyGit struct{}

func (promptJourneyGit) Run(_ context.Context, dir string, args ...string) (string, string, int, error) {
	if reflect.DeepEqual(args, []string{"rev-parse", "--show-toplevel"}) {
		return dir, "", 0, nil
	}
	if reflect.DeepEqual(args, []string{"rev-parse", "--is-inside-work-tree"}) {
		return "true\n", "", 0, nil
	}
	if len(args) == 4 && args[0] == "worktree" && args[1] == "add" && args[3] == "HEAD" {
		checkout := args[2]
		if filepath.Base(checkout) == "acquire-denied" {
			return "", "checkout acquisition denied", 1, nil
		}
		if err := os.MkdirAll(checkout, 0o700); err != nil {
			return "", "", 1, err
		}
		err := os.WriteFile(filepath.Join(checkout, ".git"), []byte("gitdir: controlled checkout\n"), 0o600)
		return "", "", 0, err
	}
	return "", "", 1, fmt.Errorf("unexpected Git request in %s: %v", dir, args)
}

// TestProcessWorkNameMapsIntoPromptTemplate proves that a submitted Work name is
// rendered into the workstation prompt template before provider invocation.
func testProcessWorkNameMapsIntoPromptTemplate(t *testing.T, host *factorySessionHost) {
	t.Parallel()
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "name_propagation"))
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		Name:       "design-doc-review",
		WorkTypeID: "task",
		Payload:    []byte(`review the design document`),
		TraceID:    "trace-prompt-test",
	})

	provider := support.NewRecordingCommandRunner("Reviewed. COMPLETE")
	_, listed, _ := host.Run(t, dir, provider, 10*time.Second)
	assertCurrentFactoryWorkCustomerStates(t, listed, map[string]int{
		support.WorkCustomerLocation("task", "complete"): 1,
	})

	providerCalls := provider.Requests()
	if len(providerCalls) == 0 {
		t.Fatal("provider calls = 0, want at least 1")
	}
	if userMessage := string(providerCalls[0].Stdin); !strings.Contains(userMessage, "Task Name: design-doc-review") {
		t.Errorf("provider user message = %q, want Task Name: design-doc-review", userMessage)
	}
}

// TestProcessMarkdownWorkNameAndPayloadMapIntoPromptTemplate proves that seeded
// markdown Work name and payload content render into the workstation prompt.
func testProcessMarkdownWorkNameAndPayloadMapIntoPromptTemplate(t *testing.T, host *factorySessionHost) {
	t.Parallel()
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "name_propagation"))
	testutil.WriteSeedMarkdownFile(t, dir, "task", "architecture-review",
		[]byte("# Architecture Review\n\nPlease review the system architecture."))

	provider := support.NewRecordingCommandRunner("Reviewed. COMPLETE")
	_, listed, _ := host.Run(t, dir, provider, 10*time.Second)
	assertCurrentFactoryWorkCustomerStates(t, listed, map[string]int{
		support.WorkCustomerLocation("task", "complete"): 1,
		support.WorkCustomerLocation("task", "init"):     0,
	})

	providerCalls := provider.Requests()
	if len(providerCalls) == 0 {
		t.Fatal("provider calls = 0, want at least 1")
	}
	userMessage := string(providerCalls[0].Stdin)
	if !strings.Contains(userMessage, "Task Name: architecture-review") {
		t.Errorf("provider user message = %q, want Task Name: architecture-review", userMessage)
	}
	if !strings.Contains(userMessage, "# Architecture Review") {
		t.Errorf("provider user message = %q, want markdown payload content", userMessage)
	}
	assertCompletedWorkName(t, listed, "task", "architecture-review")
}

// TestProcessSubmissionTagsParameterizeWorkerDirectory proves that submitted
// tags select the customer-authored directory passed to the provider command.
func testProcessSubmissionTagsParameterizeWorkerDirectory(t *testing.T, host *factorySessionHost) {
	t.Parallel()
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "repeater_workstation"))
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkTypeID: "task",
		Payload:    []byte(`{}`),
		Tags:       map[string]string{"branch": "feature-abc"},
	})

	provider := support.NewRecordingCommandRunner("done COMPLETE")
	_, listed, _ := host.Run(t, dir, provider, 10*time.Second)
	assertCurrentFactoryWorkCustomerStates(t, listed, map[string]int{
		support.WorkCustomerLocation("task", "complete"): 1,
	})

	calls := provider.Requests()
	if len(calls) == 0 {
		t.Fatal("provider command calls = 0")
	}
	if calls[0].WorkDir != filepath.Join(dir, "worktrees", "feature-abc") {
		t.Fatalf("provider working directory = %q, want submitted branch directory", calls[0].WorkDir)
	}
}

// TestProcessParameterizedTemplateFailureRoutesWorkToFailed proves that an
// unresolved workstation prompt template routes Work to failed without invoking
// the provider.
func testProcessParameterizedTemplateFailureRoutesWorkToFailed(t *testing.T, host *factorySessionHost) {
	t.Parallel()
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "parameterized_failure"))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "unresolved template test"}`))

	provider := support.NewRecordingCommandRunner("Should not reach COMPLETE")
	_, listed, _ := host.Run(t, dir, provider, 10*time.Second)
	assertCurrentFactoryWorkCustomerStates(t, listed, map[string]int{
		support.WorkCustomerLocation("task", "failed"):   1,
		support.WorkCustomerLocation("task", "complete"): 0,
	})
	if provider.CallCount() != 0 {
		t.Errorf("provider call count = %d, want 0 before invocation", provider.CallCount())
	}
}
