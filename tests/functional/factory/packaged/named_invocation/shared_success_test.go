package named_invocation

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestNamedInvocationSharedProcess keeps every named-invocation scenario on
// one immutable hosted application. Customer executions own explicit Factory
// Sessions, homes, working directories, Factory copies, and provider routes.
func TestNamedInvocationSharedProcess(t *testing.T) {
	fixture := newNamedInvocationFixture(t)
	t.Run("TestNamedInvocationSharedSuccess", func(t *testing.T) {
		runNamedInvocationSharedSuccess(t, fixture)
	})
	t.Run("TestNamedInvocationSharedPreparationFailures", func(t *testing.T) {
		runNamedInvocationSharedPreparationFailures(t, fixture)
	})
	t.Run("TestRun_EffectiveSchemaPreparationFailuresStopBeforeExecutionSideEffects", func(t *testing.T) {
		runFactoryRootLookupCancellation(t, fixture)
	})
	t.Run("reuse after cancellation", func(t *testing.T) {
		runNamedGoalSuccess(t, fixture)
	})
}

func runNamedInvocationSharedSuccess(t *testing.T, fixture *namedInvocationFixture) {
	t.Helper()
	t.Run("factory builder list and help", func(t *testing.T) {
		runFactoryBuilderListAndHelp(t, fixture)
	})
	t.Run("parallel customer invocations", func(t *testing.T) {
		for _, test := range []struct {
			name string
			run  func(*testing.T, *namedInvocationFixture)
		}{
			{name: "named goal", run: runNamedGoalSuccess},
			{name: "named subagent", run: runNamedSubagentSuccess},
			{name: "no-signature compatibility", run: runNoSignatureCompatibility},
			{name: "effective signature parity", run: runEffectiveSignatureParity},
			{name: "default-only input", run: runDefaultOnlyInput},
		} {
			test := test
			t.Run(test.name, func(t *testing.T) {
				t.Parallel()
				test.run(t, fixture)
			})
		}
	})
}

func runFactoryBuilderListAndHelp(t *testing.T, fixture *namedInvocationFixture) {
	t.Helper()
	scenario := fixture.newScenario(t)
	helpOutput, helpStderr := executeCustomerCommand(
		t, fixture.process, scenario.environment, scenario.workingDirectory,
		[]string{"you", "run", "--named", packagedFactoryBuilderName, "--help"},
	)
	if helpStderr != "" {
		t.Fatalf("Factory Builder help stderr = %q", helpStderr)
	}
	for _, fragment := range []string{
		"Creates and installs one validated graph or JavaScript Factory from a customer request.",
		"--factory-name",
		"--orchestrator",
		"--builder-provider",
		"--builder-model",
	} {
		if !strings.Contains(helpOutput, fragment) {
			t.Fatalf("Factory Builder help missing %q:\n%s", fragment, helpOutput)
		}
	}

	listOutput, listStderr := executeCustomerCommand(
		t, fixture.process, scenario.environment, scenario.workingDirectory,
		[]string{"you", "factory", "list"},
	)
	if listStderr != "" {
		t.Fatalf("factory list stderr = %q", listStderr)
	}
	if !strings.Contains(listOutput, packagedFactoryBuilderName) {
		t.Fatalf("factory list output missing %q:\n%s", packagedFactoryBuilderName, listOutput)
	}
	factoryDir := packagedFactoryPath(scenario.homeDir, packagedFactoryBuilderName)
	if _, err := os.Stat(filepath.Join(factoryDir, "factory.json")); err != nil {
		t.Fatalf("Factory Builder was not materialized for named help: %v", err)
	}
	fixture.capturePackagedFactorySources(t, scenario.homeDir)
}

func runNamedGoalSuccess(t *testing.T, fixture *namedInvocationFixture) {
	t.Helper()
	scenario := fixture.newScenario(t, support.CodexDecisionCommandResult(wantHermeticInvocationPrimaryResult))
	factoryDir := fixture.copyPackagedFactory(t, scenario, packagedGoalFactoryName)
	stdout, stderr := fixture.executeHostedCustomerCommand(
		t, scenario, factoryDir,
		[]string{"you", "run", "--named", packagedGoalFactoryName, "--no-record", "--quiet", "hermetic no-server named goal prompt"},
		"",
	)
	assertSharedInvocationResult(t, stdout, stderr, wantHermeticInvocationPrimaryResult)
	if got := scenario.provider.CallCount(); got != 1 {
		t.Fatalf("named goal provider calls = %d, want 1", got)
	}
}

func runNamedSubagentSuccess(t *testing.T, fixture *namedInvocationFixture) {
	t.Helper()
	requestText := "hermetic no-server named subagent prompt"
	scenario := fixture.newScenario(t, platformprocess.CommandResult{
		Stdout: support.CodexSuccessStdout(wantHermeticInvocationPrimaryResult),
	})
	factoryDir := fixture.copyPackagedFactory(t, scenario, packagedSubagentFactoryName)
	stdout, stderr := fixture.executeHostedCustomerCommand(
		t, scenario, factoryDir,
		[]string{"you", "run", "--named", packagedSubagentFactoryName, "--no-record", "--quiet", requestText},
		"",
	)
	assertSharedInvocationResult(t, stdout, stderr, wantHermeticInvocationPrimaryResult)
	if stdout == requestText {
		t.Fatalf("stdout echoed submitted request text instead of agent response")
	}
	if got := scenario.provider.CallCount(); got != 1 {
		t.Fatalf("named subagent provider calls = %d, want 1", got)
	}
}

func runNoSignatureCompatibility(t *testing.T, fixture *namedInvocationFixture) {
	t.Helper()
	scenario := fixture.newScenario(t,
		support.CodexDecisionCommandResult(wantHermeticInvocationPrimaryResult),
		support.CodexDecisionCommandResult(wantHermeticInvocationPrimaryResult),
		support.CodexDecisionCommandResult(wantHermeticInvocationPrimaryResult),
		support.CodexDecisionCommandResult(wantHermeticInvocationPrimaryResult),
		support.CodexDecisionCommandResult(wantHermeticInvocationPrimaryResult),
		support.CodexDecisionCommandResult(wantHermeticInvocationPrimaryResult),
	)
	factoryDir := fixture.copyPackagedFactory(t, scenario, packagedGoalFactoryName)
	namedFactoryDir := support.CopyFactoryAsNamed(t, factoryDir, scenario.homeDir, customizedNamedGoalFactoryName)
	namedFactoryPath := filepath.Join(namedFactoryDir, "factory.json")
	support.RemoveInvocationSignatureFixture(t, namedFactoryPath)
	base := []string{"--no-record", "--quiet"}
	cases := []struct {
		name  string
		input []string
		stdin string
	}{
		{name: "positional compatibility", input: []string{"legacy positional input"}},
		{name: "stdin compatibility", input: []string{"-"}, stdin: "legacy stdin input\n"},
		{name: "signature-only syntax remains literal text", input: []string{"--mode", "fast"}},
	}
	for _, test := range cases {
		test := test
		t.Run(test.name, func(t *testing.T) {
			namedArgs := append([]string{"you", "run", "--named", customizedNamedGoalFactoryName}, base...)
			namedArgs = append(namedArgs, test.input...)
			namedStdout, namedStderr := fixture.executeHostedCustomerCommand(
				t, scenario, namedFactoryDir, namedArgs, test.stdin,
			)
			fileArgs := append([]string{"you", "run", "--factory", namedFactoryPath}, base...)
			fileArgs = append(fileArgs, test.input...)
			fileStdout, fileStderr := fixture.executeHostedCustomerCommand(
				t, scenario, namedFactoryDir, fileArgs, test.stdin,
			)
			if namedStderr != "" || fileStderr != "" {
				t.Fatalf("invocation stderr: named=%q file=%q", namedStderr, fileStderr)
			}
			if namedStdout != wantHermeticInvocationPrimaryResult || fileStdout != namedStdout {
				t.Fatalf("selection outputs differ: named=%q file=%q", namedStdout, fileStdout)
			}
		})
	}
	if got := scenario.provider.CallCount(); got != len(cases)*2 {
		t.Fatalf("no-signature provider calls = %d, want %d", got, len(cases)*2)
	}
}

func runEffectiveSignatureParity(t *testing.T, fixture *namedInvocationFixture) {
	t.Helper()
	scenario := fixture.newScenario(t,
		support.CodexDecisionCommandResult("canonical provider result"),
		support.CodexDecisionCommandResult("canonical provider result"),
	)
	factoryDir := fixture.copyPackagedFactory(t, scenario, packagedGoalFactoryName)
	namedFactoryDir := support.CopyFactoryAsNamed(t, factoryDir, scenario.homeDir, customizedNamedGoalFactoryName)
	namedFactoryPath := filepath.Join(namedFactoryDir, "factory.json")
	addEffectiveSignatureFixture(t, namedFactoryPath)
	support.ReplaceGoalWorkstationPrompt(t, namedFactoryPath, "input=${input}|format=${format}|count=${count}|document=${document}|stdin=${body}")
	documentPath := filepath.Join(scenario.workingDirectory, "story.md")
	if err := os.WriteFile(documentPath, []byte("factory invocation document"), 0o600); err != nil {
		t.Fatalf("write FILE_CONTENTS fixture: %v", err)
	}
	common := []string{
		"--no-record", "--quiet",
		"equivalent canonical prompt", "one.md", "two.md",
		"--t", "alpha", "--tag", "beta",
		"--count", "2",
		"--file", documentPath,
		"-",
	}
	namedArgs := append([]string{"you", "run", "--named", customizedNamedGoalFactoryName}, common...)
	namedStdout, namedStderr := fixture.executeHostedCustomerCommand(
		t, scenario, namedFactoryDir, namedArgs, "canonical stdin body",
	)
	fileArgs := append([]string{"you", "run", "--factory", namedFactoryPath}, common...)
	fileStdout, fileStderr := fixture.executeHostedCustomerCommand(
		t, scenario, namedFactoryDir, fileArgs, "canonical stdin body",
	)
	if namedStderr != "" || fileStderr != "" {
		t.Fatalf("invocation stderr: named=%q file=%q", namedStderr, fileStderr)
	}
	if namedStdout == "" || fileStdout != namedStdout {
		t.Fatalf("selection outputs differ: named=%q file=%q", namedStdout, fileStdout)
	}

	calls := scenario.provider.Requests()
	if len(calls) != 2 {
		t.Fatalf("provider calls = %d, want named and explicit-file calls", len(calls))
	}
	wantPrompt := "input=equivalent canonical prompt|format=json|count=2|document=factory invocation document|stdin=canonical stdin body"
	placeholders := []string{"${executorProvider}", "${executorModel}", "${input}", "${format}", "${count}", "${document}", "${body}", "${mode}"}
	for index, call := range calls {
		if call.Command != "codex" {
			t.Fatalf("provider call %d command = %q, want codex", index, call.Command)
		}
		if support.RequestContainsInterpolation(call, placeholders...) {
			t.Fatalf("provider call %d contains unresolved interpolation: %#v", index, call)
		}
		if got := string(call.Stdin); !strings.Contains(got, wantPrompt) {
			t.Fatalf("provider call %d prompt = %q, want resolved prompt fragment %q", index, got, wantPrompt)
		}
	}
}

func runDefaultOnlyInput(t *testing.T, fixture *namedInvocationFixture) {
	t.Helper()
	scenario := fixture.newScenario(t,
		support.CodexDecisionCommandResult("default applied"),
		support.CodexDecisionCommandResult("default applied"),
	)
	factoryDir := fixture.copyPackagedFactory(t, scenario, packagedGoalFactoryName)
	namedFactoryDir := support.CopyFactoryAsNamed(t, factoryDir, scenario.homeDir, customizedNamedGoalFactoryName)
	factoryPath := filepath.Join(namedFactoryDir, "factory.json")
	replaceInvocationSignatureFixture(t, factoryPath, map[string]any{
		"parameters": []any{map[string]any{
			"name": "mode", "defaultValue": "safe",
			"bindings": []any{map[string]any{"kind": "NAMED"}},
		}},
	})
	support.ReplaceGoalWorkerInstructions(t, factoryPath, "mode=${mode}")
	support.ReplaceGoalWorkstationPrompt(t, factoryPath, "mode=${mode}")

	namedStdout, namedStderr := fixture.executeHostedCustomerCommand(
		t, scenario, namedFactoryDir,
		emptyInvocationArguments("named", factoryPath, customizedNamedGoalFactoryName),
		"",
	)
	fileStdout, fileStderr := fixture.executeHostedCustomerCommand(
		t, scenario, namedFactoryDir,
		emptyInvocationArguments("file", factoryPath, customizedNamedGoalFactoryName),
		"",
	)
	if namedStderr != "" || fileStderr != "" {
		t.Fatalf("default-only invocation stderr: named=%q file=%q", namedStderr, fileStderr)
	}
	if namedStdout == "" || fileStdout != namedStdout {
		t.Fatalf("default-only selection outputs differ: named=%q file=%q", namedStdout, fileStdout)
	}

	for index, call := range scenario.provider.Requests() {
		if call.Command != "codex" || support.RequestContainsInterpolation(call, "${mode}") {
			t.Fatalf("default-only provider call %d = %#v, want resolved codex request", index, call)
		}
		if !strings.Contains(string(call.Stdin), "mode=safe") {
			t.Fatalf("default-only provider prompt %d = %q, want resolved default", index, call.Stdin)
		}
	}
}

func assertSharedInvocationResult(t *testing.T, stdout, stderr, want string) {
	t.Helper()
	if stderr != "" {
		t.Fatalf("named invocation stderr = %q, want empty", stderr)
	}
	if stdout != want {
		t.Fatalf("stdout = %q, want primary result %q", stdout, want)
	}
}

type namedInvocationFixture struct {
	process                   support.ApplicationProcess
	provider                  *namedInvocationProviderRouter
	authoredReader            *namedInvocationAuthoredReaderRouter
	api                       *support.ProcessAPIServer
	baseURL                   string
	packagedFactorySources    map[string]string
	packagedFactorySourceHome string
}

type namedInvocationScenario struct {
	rootDir          string
	homeDir          string
	workingDirectory string
	environment      []string
	provider         *testutil.ProviderCommandRunner
}

func newNamedInvocationFixture(t *testing.T) *namedInvocationFixture {
	t.Helper()
	fixture := &namedInvocationFixture{
		provider:                  newNamedInvocationProviderRouter(),
		authoredReader:            newNamedInvocationAuthoredReaderRouter(),
		api:                       support.NewProcessAPIServer(),
		packagedFactorySources:    make(map[string]string),
		packagedFactorySourceHome: t.TempDir(),
	}
	process, err := fixture.buildProcess(context.Background(), fixture.edges())
	if err != nil {
		t.Fatalf("build shared named-invocation root process: %v", err)
	}
	fixture.process = process
	support.CleanupProcess(t, process)
	fixture.startHost(t)
	return fixture
}

func (fixture *namedInvocationFixture) buildProcess(
	ctx context.Context,
	edges serviceedges.Edges,
) (support.ApplicationProcess, error) {
	return support.BuildProcessWithContext(ctx, edges)
}

func (fixture *namedInvocationFixture) startHost(t *testing.T) {
	t.Helper()
	hostDir := support.ScaffoldFactory(t, map[string]any{
		"name": "named-invocation-idle-host",
		"workTypes": []map[string]any{{
			"name": "idle",
			"states": []map[string]string{
				{"name": "ready", "type": "INITIAL"},
				{"name": "done", "type": "TERMINAL"},
			},
		}},
	})
	hostHome := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	inputs := support.FakeInputs(ctx, []string{
		"you", "run", "--dir", hostDir, "--continuously", "--with-server", "--quiet", "--no-record",
	})
	inputs.Input.Env = namedInvocationEnvironment(hostHome)
	inputs.Input.WorkingDirectory = hostDir
	done := make(chan error, 1)
	go func() { done <- fixture.process.Execute(inputs.Input) }()
	baseURL, err := fixture.api.WaitForBaseURL(15 * time.Second)
	if err != nil {
		cancel()
		t.Fatalf("start named-invocation hosted application: %v", err)
	}
	fixture.baseURL = baseURL
	t.Cleanup(func() {
		cancel()
		select {
		case executionErr := <-done:
			if executionErr != nil && !errors.Is(executionErr, context.Canceled) {
				t.Errorf("stop named-invocation hosted application: %v", executionErr)
			}
		case <-time.After(10 * time.Second):
			t.Error("timed out stopping named-invocation hosted application")
		}
	})
}

func (fixture *namedInvocationFixture) executeHostedCustomerCommand(
	t *testing.T,
	scenario *namedInvocationScenario,
	factoryDir string,
	args []string,
	stdin string,
) (string, string) {
	t.Helper()
	opened := support.OpenFactorySessionAt(t, fixture.baseURL, factoryDir)
	sessionID := opened.Session.Id
	if err := fixture.provider.registerScope(sessionID, scenario.provider); err != nil {
		t.Fatalf("register named-invocation provider session route: %v", err)
	}
	t.Cleanup(func() {
		fixture.provider.unregisterScope(sessionID)
		support.CloseFactorySessionAt(t, fixture.baseURL, sessionID)
	})

	remoteArgs := []string{"you", "--remote", "--server", fixture.baseURL}
	for _, arg := range args[1:] {
		remoteArgs = append(remoteArgs, arg)
		if arg == "run" {
			remoteArgs = append(remoteArgs, "--session", sessionID)
		}
	}
	return executeCustomerCommandWithStdin(
		t, fixture.process, scenario.environment, scenario.workingDirectory, remoteArgs, stdin,
	)
}

func (fixture *namedInvocationFixture) capturePackagedFactorySources(
	t *testing.T,
	homeDir string,
) {
	t.Helper()
	for _, name := range []string{
		packagedGoalFactoryName,
		packagedSubagentFactoryName,
		packagedFactoryBuilderName,
	} {
		sourceDir := packagedFactoryPath(homeDir, name)
		if _, err := os.Stat(filepath.Join(sourceDir, "factory.json")); err != nil {
			t.Fatalf("materialized packaged Factory %q missing from source home: %v", name, err)
		}
		fixture.packagedFactorySources[name] = support.CopyFactoryAsNamed(
			t, sourceDir, fixture.packagedFactorySourceHome, name,
		)
	}
}

func (fixture *namedInvocationFixture) copyPackagedFactory(
	t *testing.T,
	scenario *namedInvocationScenario,
	name string,
) string {
	t.Helper()
	sourceDir := fixture.packagedFactorySources[name]
	if sourceDir == "" {
		initializedDir := initializePackagedFactory(
			t, fixture.process, scenario.environment, scenario.workingDirectory,
			scenario.homeDir, name,
		)
		sourceDir = support.CopyFactoryAsNamed(
			t, initializedDir, fixture.packagedFactorySourceHome, name,
		)
		fixture.packagedFactorySources[name] = sourceDir
	}
	return support.CopyFactoryAsNamed(t, sourceDir, scenario.homeDir, name)
}

func packagedFactoryPath(homeDir, name string) string {
	return filepath.Join(
		append([]string{homeDir, ".you-agent-factory", "factories"}, strings.Split(name, "/")...)...,
	)
}

func (fixture *namedInvocationFixture) newScenario(
	t *testing.T,
	results ...platformprocess.CommandResult,
) *namedInvocationScenario {
	t.Helper()
	rootDir := t.TempDir()
	homeDir := filepath.Join(rootDir, "home")
	workingDirectory := filepath.Join(rootDir, "work")
	for label, path := range map[string]string{"home": homeDir, "working directory": workingDirectory} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatalf("create named-invocation %s: %v", label, err)
		}
	}
	scenario := &namedInvocationScenario{
		rootDir:          rootDir,
		homeDir:          homeDir,
		workingDirectory: workingDirectory,
		environment:      namedInvocationEnvironment(homeDir),
	}
	if len(results) == 0 {
		return scenario
	}
	scenario.provider = testutil.NewProviderCommandRunner(results...)
	if err := fixture.provider.register(workingDirectory, scenario.provider); err != nil {
		t.Fatalf("register named-invocation provider route: %v", err)
	}
	t.Cleanup(func() { fixture.provider.unregister(workingDirectory) })
	return scenario
}

func namedInvocationEnvironment(homeDir string) []string {
	environment := make([]string, 0, len(os.Environ())+2)
	for _, entry := range os.Environ() {
		name := strings.SplitN(entry, "=", 2)[0]
		if strings.EqualFold(name, "HOME") || strings.EqualFold(name, "USERPROFILE") {
			continue
		}
		environment = append(environment, entry)
	}
	return append(environment, "HOME="+homeDir, "USERPROFILE="+homeDir)
}

type namedInvocationProviderRouter struct {
	mu          sync.RWMutex
	routes      map[string]platformprocess.CommandRunner
	scopeRoutes map[string]platformprocess.CommandRunner
}

func newNamedInvocationProviderRouter() *namedInvocationProviderRouter {
	return &namedInvocationProviderRouter{
		routes:      make(map[string]platformprocess.CommandRunner),
		scopeRoutes: make(map[string]platformprocess.CommandRunner),
	}
}

func (router *namedInvocationProviderRouter) registerScope(
	scopeID string,
	runner platformprocess.CommandRunner,
) error {
	scopeID = strings.TrimSpace(scopeID)
	if scopeID == "" || runner == nil {
		return errors.New("named-invocation provider scope requires an identity and runner")
	}
	router.mu.Lock()
	defer router.mu.Unlock()
	if _, exists := router.scopeRoutes[scopeID]; exists {
		return errors.New("named-invocation provider scope is already registered")
	}
	router.scopeRoutes[scopeID] = runner
	return nil
}

func (router *namedInvocationProviderRouter) unregisterScope(scopeID string) {
	router.mu.Lock()
	delete(router.scopeRoutes, strings.TrimSpace(scopeID))
	router.mu.Unlock()
}

func (router *namedInvocationProviderRouter) register(path string, runner platformprocess.CommandRunner) error {
	path = normalizeNamedInvocationWorkDir(path)
	if path == "" || runner == nil {
		return errors.New("named-invocation provider route requires a directory and runner")
	}
	router.mu.Lock()
	defer router.mu.Unlock()
	if _, exists := router.routes[path]; exists {
		return errors.New("named-invocation provider route is already registered")
	}
	router.routes[path] = runner
	return nil
}

func (router *namedInvocationProviderRouter) unregister(path string) {
	router.mu.Lock()
	delete(router.routes, normalizeNamedInvocationWorkDir(path))
	router.mu.Unlock()
}

func (router *namedInvocationProviderRouter) Run(
	ctx context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	router.mu.RLock()
	runner := router.scopeRoutes[strings.TrimSpace(request.ExecutionScopeID)]
	if runner == nil {
		runner = router.routes[normalizeNamedInvocationWorkDir(request.WorkDir)]
	}
	router.mu.RUnlock()
	if runner == nil {
		return platformprocess.CommandResult{}, errors.New("named-invocation provider route unavailable")
	}
	return runner.Run(ctx, request)
}

func normalizeNamedInvocationWorkDir(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	cleaned, err := filepath.Abs(filepath.Clean(path))
	if err == nil {
		return cleaned
	}
	return filepath.Clean(path)
}
