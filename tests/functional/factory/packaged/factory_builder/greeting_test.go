package factory_builder

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// greetingCommandRunner answers the routing classification with "help" and
// captures the prompt the help workstation then receives.
type greetingCommandRunner struct {
	mu           sync.Mutex
	routingCalls int
	helpPrompts  []string
	buildPrompts []string
}

func (runner *greetingCommandRunner) Run(
	_ context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	prompt := string(request.Stdin)
	runner.mu.Lock()
	defer runner.mu.Unlock()
	switch {
	case isBuilderRoutingPrompt(request):
		runner.routingCalls++
		return platformprocess.CommandResult{Stdout: support.CodexSuccessStdout("help")}, nil
	case strings.Contains(prompt, "You are Factory Builder."):
		runner.buildPrompts = append(runner.buildPrompts, prompt)
		return platformprocess.CommandResult{Stdout: support.CodexSuccessStdout("should not build")}, nil
	default:
		runner.helpPrompts = append(runner.helpPrompts, prompt)
		return platformprocess.CommandResult{
			Stdout: support.CodexSuccessStdout("Factory Builder creates one reusable Factory from a description."),
		}, nil
	}
}

func (runner *greetingCommandRunner) snapshot() (routing int, help, build []string) {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return runner.routingCalls,
		append([]string(nil), runner.helpPrompts...),
		append([]string(nil), runner.buildPrompts...)
}

func TestFactoryBuilder(t *testing.T) {
	fixture := newFactoryBuilderSharedFixture(t)
	// The shared host keeps the production application alive while each child
	// uses its own explicit session. The public CLI has no selector for an
	// already-open live session, so session invocations use the API-owned
	// invocation contract below while Builder-issued customer commands still
	// use Process.Execute.
	fixture.prepareExistingInvalidScenario(t)
	fixture.startServer(t)
	t.Run("TestFactoryBuilderVagueFirstTurnAnswersWithoutBuilding", func(t *testing.T) {
		testFactoryBuilderVagueFirstTurnAnswersWithoutBuilding(t, fixture)
	})
	t.Run("TestFactoryBuilderWithNoRequestGreetsInsteadOfFailing", func(t *testing.T) {
		testFactoryBuilderWithNoRequestGreetsInsteadOfFailing(t, fixture)
	})
	t.Run("TestFactoryBuilderCreatesAndInstallsValidatedGraphFactory", func(t *testing.T) {
		testFactoryBuilderCreatesAndInstallsValidatedGraphFactory(t, fixture)
	})
	t.Run("TestFactoryBuilderCreatesAndInstallsValidatedJavaScriptFactory", func(t *testing.T) {
		testFactoryBuilderCreatesAndInstallsValidatedJavaScriptFactory(t, fixture)
	})
	t.Run("TestFactoryBuilderRejectsInvalidGeneratedCandidateWithoutInstallation", func(t *testing.T) {
		testFactoryBuilderRejectsInvalidGeneratedCandidateWithoutInstallation(t, fixture)
	})
	// Close the explicit sessions before releasing the host. This leaves the
	// reusable process available for the installed-artifact CLI checks without
	// creating a second process or API server.
	fixture.closeAllScenarios(t)
	runtimeArtifacts, isolatedRows := fixture.assertDurableSessionsClean(t)
	fixture.stopServer(t)
	fixture.assertInstalledArtifacts(t)
	fixture.removeAllScenarioRoots(t)
	fixture.lifecycle.assertClean(t, runtimeArtifacts, isolatedRows)
}

func (fixture *factoryBuilderSharedFixture) prepareExistingInvalidScenario(t *testing.T) {
	t.Helper()
	runner := newInvalidCandidateRunner(invalidFactoryExistingName)
	scenario := fixture.newScenario(t, runner)
	runner.process = fixture.process
	runner.environment = scenario.environment
	runner.operatorRoot = scenario.operatorRoot
	installedPath := filepath.Join(scenario.operatorRoot, invalidFactoryExistingName)
	installExistingGraphFactory(t, fixture.process, scenario.environment, scenario.workingDirectory, runner.operatorRoot, invalidFactoryExistingName)
	assertInstalledGraphFactoryRuns(t, scenario, installedPath)
	if got := runner.InstalledFactoryCallCount(); got != 1 {
		t.Fatalf("existing Factory setup provider command call count = %d, want one baseline invocation", got)
	}
	scenario.installedKind = "existing-invalid"
	scenario.installedPath = installedPath
	scenario.existingBefore = readInstalledFactoryConfig(t, installedPath)
	fixture.invalidExistingScenario = scenario
}

func (fixture *factoryBuilderSharedFixture) assertInstalledArtifacts(t testing.TB) {
	t.Helper()
	for _, scenario := range fixture.scenarios {
		switch scenario.installedKind {
		case "graph":
			assertInstalledGraphFactory(t, fixture.process, scenario.environment, scenario.installedPath, scenario.runner.operatorRoot)
			assertInstalledGraphFactoryRuns(t, scenario, scenario.installedPath)
			if got := scenario.runner.InstalledFactoryCallCount(); got != 1 {
				t.Fatalf("installed Graph Factory provider command call count = %d, want one customer invocation", got)
			}
		case "javascript":
			assertInstalledJavaScriptFactory(t, fixture.process, scenario.environment, scenario.installedPath, scenario.runner.operatorRoot)
			assertInstalledJavaScriptFactoryRuns(t, scenario, scenario.installedPath)
			if got := scenario.runner.InstalledFactoryCallCount(); got != 2 {
				t.Fatalf("installed JavaScript Factory provider command call count = %d, want two intended analysis calls", got)
			}
		case "existing-invalid":
			after := readInstalledFactoryConfig(t, scenario.installedPath)
			if !bytes.Equal(scenario.existingBefore, after) {
				t.Fatalf("installed Factory changed after rejected candidate\nbefore:\n%s\nafter:\n%s", scenario.existingBefore, after)
			}
			assertInstalledGraphFactoryRuns(t, scenario, scenario.installedPath)
		default:
			if scenario.installedKind != "" {
				t.Fatalf("unknown installed Factory assertion kind %q", scenario.installedKind)
			}
		}
	}
}

// TestFactoryBuilderVagueFirstTurnAnswersWithoutBuilding proves problems.md
// 4.1: a customer's first vague message reaches Factory Builder's usage
// guidance instead of immediately attempting to author and install a Factory.
func testFactoryBuilderVagueFirstTurnAnswersWithoutBuilding(t *testing.T, fixture *factoryBuilderSharedFixture) {
	runner := &greetingCommandRunner{}
	scenario := fixture.newScenario(t, runner)
	scenario.open(t)
	response := invokeFactoryBuilder(t, scenario, map[string]any{
		"request":         "what can you do?",
		"builderProvider": "CODEX",
		"builderModel":    "gpt-5",
	})
	if response.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("invocation status = %q, want COMPLETED", response.Status)
	}

	routing, helpPrompts, buildPrompts := runner.snapshot()
	if routing != 1 {
		t.Fatalf("routing classifications = %d, want exactly 1", routing)
	}
	if len(buildPrompts) != 0 {
		t.Fatalf("build workstation ran %d times for a vague first turn, want 0", len(buildPrompts))
	}
	if len(helpPrompts) != 1 {
		t.Fatalf("help workstation ran %d times, want exactly 1", len(helpPrompts))
	}

	// The guidance is authored, not model-invented, so the worker's prompt is
	// where its accuracy is actually assertable.
	for _, fragment := range []string{
		"you run --named @you/factory-builder --to",
		"--orchestrator graph|javascript",
		"you docs authoring-factories",
		"you docs javascript-workflows",
		"Do not read the workspace or run any command.",
	} {
		if !strings.Contains(helpPrompts[0], fragment) {
			t.Fatalf("help prompt is missing %q; got:\n%s", fragment, helpPrompts[0])
		}
	}
}

// TestFactoryBuilderWithNoRequestGreetsInsteadOfFailing proves a bare
// invocation is admitted and answered rather than rejected for a missing
// required input, which is the CLI analogue of an ACP client's first turn.
func testFactoryBuilderWithNoRequestGreetsInsteadOfFailing(t *testing.T, fixture *factoryBuilderSharedFixture) {
	runner := &greetingCommandRunner{}
	scenario := fixture.newScenario(t, runner)
	scenario.open(t)
	response := invokeFactoryBuilder(t, scenario, map[string]any{
		"builderProvider": "CODEX",
		"builderModel":    "gpt-5",
	})
	if response.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("invocation status = %q, want COMPLETED for a bare invocation", response.Status)
	}
	if _, _, buildPrompts := runner.snapshot(); len(buildPrompts) != 0 {
		t.Fatalf("build workstation ran %d times with no request, want 0", len(buildPrompts))
	}
}

func invokeFactoryBuilder(
	t *testing.T,
	scenario *factoryBuilderScenario,
	args map[string]any,
) factoryapi.InvocationResponse {
	t.Helper()
	return invokeFactorySession(t, scenario, args)
}

func invokeFactorySession(
	t *testing.T,
	scenario *factoryBuilderScenario,
	args map[string]any,
) factoryapi.InvocationResponse {
	t.Helper()
	requestID := fmt.Sprintf("factory-builder-session-%d", scenario.fixture.nextRequestID())
	payload, err := json.Marshal(factoryapi.InvocationRequest{RequestId: &requestID, Args: &args})
	if err != nil {
		t.Fatalf("marshal Factory Session invocation: %v", err)
	}
	endpoint := strings.TrimSuffix(scenario.fixture.baseURL, "/") +
		"/factory-sessions/" + url.PathEscape(scenario.sessionID) + "/invocations"
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("build Factory Session invocation request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("POST Factory Session invocation: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("POST Factory Session invocation status = %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	var decoded factoryapi.InvocationResponse
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		t.Fatalf("decode Factory Session invocation: %v", err)
	}
	if err := scenario.fixture.recordInvocationRequestID(decoded.RequestId); err != nil {
		t.Fatalf("Factory Builder invocation request ID: %v", err)
	}
	return decoded
}
