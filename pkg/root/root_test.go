package root

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	"github.com/portpowered/infinite-you/pkg/config/defaultpaths"
	factorysessions "github.com/portpowered/infinite-you/pkg/factory/sessions"
	factorysessionexecution "github.com/portpowered/infinite-you/pkg/factory/sessions/execution"
	"github.com/portpowered/infinite-you/pkg/initializer"
	modelscli "github.com/portpowered/infinite-you/pkg/transports/cli/models"
	sessionexecutioncli "github.com/portpowered/infinite-you/pkg/transports/cli/sessionexecution"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	"github.com/portpowered/infinite-you/pkg/wire"
)

func TestNormalizeSnapshotsArgumentsAndEnvironment(t *testing.T) {
	args := []string{"custom-you", "docs", "--", "", "--topic", "--topic"}
	environment := []string{"PRESENT=first", "EMPTY=", "PRESENT=last"}

	input, err := Normalize(Input{Args: args, Env: environment})
	if err != nil {
		t.Fatalf("Normalize() error = %v", err)
	}
	args[1] = "changed"
	environment[0] = "PRESENT=changed"

	if input.Executable() != "custom-you" {
		t.Fatalf("Executable() = %q, want custom-you", input.Executable())
	}
	wantArguments := []string{"docs", "--", "", "--topic", "--topic"}
	if got := strings.Join(input.Arguments(), "\x00"); got != strings.Join(wantArguments, "\x00") {
		t.Fatalf("Arguments() = %q, want %q", input.Arguments(), wantArguments)
	}
	returned := input.Arguments()
	returned[0] = "mutated"
	if input.Arguments()[0] != "docs" {
		t.Fatal("Arguments() exposed mutable normalized state")
	}
	if value, ok := input.LookupEnv("PRESENT"); !ok || value != "last" {
		t.Fatalf("LookupEnv(PRESENT) = %q, %t; want last, true", value, ok)
	}
	if value, ok := input.LookupEnv("EMPTY"); !ok || value != "" {
		t.Fatalf("LookupEnv(EMPTY) = %q, %t; want empty, true", value, ok)
	}
	if _, ok := input.LookupEnv("ABSENT"); ok {
		t.Fatal("LookupEnv(ABSENT) reported an absent value as present")
	}
}

func TestHomeDirRejectsMissingEnvironment(t *testing.T) {
	input, err := Normalize(Input{Args: []string{"you"}})
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if _, err := homeDir(input); err == nil {
		t.Fatal("homeDir succeeded without a home environment variable")
	}
}

func TestProductionInitializerUsesInjectedLifecycle(t *testing.T) {
	t.Parallel()
	graph := &ApplicationGraph{}
	called := false
	runner := productionInitializer{initialize: func(_ context.Context, got *initializer.ProcessGraph) error {
		called = true
		if got != graph {
			t.Fatalf("graph = %p, want %p", got, graph)
		}
		return nil
	}}
	if err := runner.Run(context.Background(), Initialization{Graph: graph}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !called {
		t.Fatal("injected initializer was not called")
	}
	if err := (productionInitializer{}).Run(context.Background(), Initialization{}); err == nil {
		t.Fatal("default initializer accepted a nil application graph")
	}
}

func TestExecuteRoutesHelpAndExplicitCommandsToSuppliedStreams(t *testing.T) {
	t.Parallel()

	var help bytes.Buffer
	err := Execute(Input{
		Args:    []string{"renamed-binary", "--help"},
		Env:     homeEnvironment(t.TempDir()),
		Stdout:  &help,
		Context: context.Background(),
	})
	if err != nil {
		t.Fatalf("Execute(help) error = %v", err)
	}
	if !strings.HasPrefix(help.String(), "Run and manage CPN-based workflow factories") {
		t.Fatalf("help output = %q", help.String())
	}

	var docs bytes.Buffer
	err = Execute(Input{
		Args:    []string{"you", "docs", "agents"},
		Env:     homeEnvironment(t.TempDir()),
		Stdout:  &docs,
		Context: context.Background(),
	})
	if err != nil {
		t.Fatalf("Execute(docs agents) error = %v", err)
	}
	if !strings.Contains(docs.String(), "# Agents") {
		t.Fatalf("docs output does not contain agents topic: %q", docs.String())
	}
	if strings.Contains(help.String(), "# Agents") {
		t.Fatal("sequential execution leaked the second command's output into the first stream")
	}
}

func TestExecuteInvalidArgumentsReturnsDiagnostic(t *testing.T) {
	t.Parallel()

	var stderr bytes.Buffer
	err := Execute(Input{
		Args:    []string{"you", "definitely-not-a-command"},
		Env:     homeEnvironment(t.TempDir()),
		Stderr:  &stderr,
		Context: context.Background(),
	})
	if err == nil {
		t.Fatal("Execute(invalid command) error = nil")
	}
	if !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("Execute(invalid command) error = %q", err)
	}
}

func TestExecuteSequentialHomesControlConfigAndRunPaths(t *testing.T) {
	ambientHome := t.TempDir()
	t.Setenv("HOME", ambientHome)
	t.Setenv("USERPROFILE", ambientHome)

	homes := []string{t.TempDir(), t.TempDir()}
	for _, home := range homes {
		if err := Execute(Input{
			Args: []string{"you", "config", "init"}, Env: homeEnvironment(home), Context: context.Background(),
		}); err != nil {
			t.Fatalf("Execute(config init, home %q) error = %v", home, err)
		}
		if _, err := os.Stat(defaultpaths.OperatorConfigPath(home)); err != nil {
			t.Fatalf("Stat(config for supplied home %q) error = %v", home, err)
		}

		builder := &recordingGraphBuilder{graph: &ApplicationGraph{}}
		initializer := &recordingInitializer{}
		err := ExecuteWithDependencies(Input{
			Args: []string{"you", "run", "--named", "@you/goal", "--no-record", "--quiet", "Plan the sprint"},
			Env:  homeEnvironment(home), Context: context.Background(),
		}, Dependencies{GraphBuilder: builder, Initializer: initializer})
		if err != nil {
			t.Fatalf("ExecuteWithDependencies(run, home %q) error = %v", home, err)
		}
		cfg := builder.request.Startup.RunConfig
		if cfg == nil || cfg.HomeDir != home {
			t.Fatalf("run home = %v, want %q", cfg, home)
		}
		wantGlobalRoot := defaultpaths.NamedFactoriesRoot(home)
		if cfg.NamedFactoryResolution == nil || cfg.NamedFactoryResolution.GlobalRoot != wantGlobalRoot {
			t.Fatalf("named-factory resolution = %+v, want global root %q", cfg.NamedFactoryResolution, wantGlobalRoot)
		}
		if !strings.HasPrefix(filepath.Clean(cfg.Dir), filepath.Clean(wantGlobalRoot)+string(os.PathSeparator)) {
			t.Fatalf("named-factory dir = %q, want below supplied home root %q", cfg.Dir, wantGlobalRoot)
		}
	}
	if _, err := os.Stat(defaultpaths.OperatorConfigPath(ambientHome)); !os.IsNotExist(err) {
		t.Fatalf("ambient config Stat error = %v, want not-exist", err)
	}
}

func TestExecuteSetupAndFactoryAuthoringCommandsThroughProductionComposition(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	factoryDir := filepath.Join(t.TempDir(), "authored-factory")
	var initOutput bytes.Buffer
	if err := Execute(Input{
		Args:    []string{"you", "init", "--dir", factoryDir},
		Env:     homeEnvironment(home),
		Stdout:  &initOutput,
		Context: context.Background(),
	}); err != nil {
		t.Fatalf("Execute(init) error = %v", err)
	}
	if !strings.Contains(initOutput.String(), "Initialized default factory directory structure") {
		t.Fatalf("init output = %q", initOutput.String())
	}

	var validateOutput bytes.Buffer
	if err := Execute(Input{
		Args:    []string{"you", "factory", "config", "validate", factoryDir},
		Env:     homeEnvironment(home),
		Stdout:  &validateOutput,
		Context: context.Background(),
	}); err != nil {
		t.Fatalf("Execute(factory config validate) error = %v", err)
	}
	if !strings.Contains(validateOutput.String(), "Factory validation passed") {
		t.Fatalf("factory validation output = %q", validateOutput.String())
	}

	missingPath := filepath.Join(t.TempDir(), "missing-factory")
	if err := Execute(Input{
		Args:    []string{"you", "factory", "config", "validate", missingPath},
		Env:     homeEnvironment(home),
		Context: context.Background(),
	}); err == nil || !strings.Contains(err.Error(), "find factory config") {
		t.Fatalf("Execute(factory config validate missing path) error = %v", err)
	}
	if _, err := os.Stat(missingPath); !os.IsNotExist(err) {
		t.Fatalf("missing factory path Stat error = %v, want not-exist", err)
	}
}

func TestExecuteWorkflowStartsUseInjectedDurableExecutionService(t *testing.T) {
	t.Parallel()
	catalogPath := testutil.MustRepoPath(t, "pkg/transports/http/testdata/durable-session-contract-fixtures.json")
	service, err := factorysessionexecution.NewFakeServiceFromContractFixtures(catalogPath)
	if err != nil {
		t.Fatalf("NewFakeServiceFromContractFixtures() error = %v", err)
	}
	var requests []sessionexecutioncli.ServiceRequest
	commands := []struct {
		name      string
		args      []string
		sessionID string
	}{
		{name: "sync run", args: []string{"workflow", "run", "--request-id", "req-petri-success-001", "--factory", "customer-support-triage"}, sessionID: "dur-sess-petri-success-001"},
		{name: "async start", args: []string{"workflow", "start", "--request-id", "req-js-run-n-001", "--workflow", "release-train"}, sessionID: "dur-sess-js-run-n-001"},
	}
	for _, command := range commands {
		var output bytes.Buffer
		err = ExecuteWithDependencies(Input{
			Args: append([]string{"you", "--json"}, command.args...),
			Env:  homeEnvironment(t.TempDir()), Stdout: &output, Context: context.Background(),
		}, Dependencies{BuildSessionExecution: func(_ context.Context, request sessionexecutioncli.ServiceRequest) (sessionexecutioncli.ServiceOwner, error) {
			requests = append(requests, request)
			return rootTestExecutionOwner{Service: service}, nil
		}})
		if err != nil {
			t.Fatalf("ExecuteWithDependencies(%s) error = %v", command.name, err)
		}
		if !strings.Contains(output.String(), `"sessionId":"`+command.sessionID+`"`) {
			t.Fatalf("%s output = %q, want result from injected fixture service", command.name, output.String())
		}
	}
	if len(requests) != len(commands) {
		t.Fatalf("durable execution requests = %+v, want one per command", requests)
	}
	for _, request := range requests {
		if request.Provider != string(factorysessionexecution.ExecutionProviderFake) {
			t.Fatalf("durable execution request = %+v, want fake provider", request)
		}
	}
}

type rootTestExecutionOwner struct {
	factorysessionexecution.Service
}

func (rootTestExecutionOwner) Close() error { return nil }

func TestExecuteModelsInvokeUsesInjectedModelCollaborator(t *testing.T) {
	repoRoot := testutil.MustRepoPath(t, "")
	t.Chdir(repoRoot)
	var requests []modelscli.InvocationRequest
	runner := &sentinelModelInvocationRunner{}
	var output bytes.Buffer
	err := ExecuteWithDependencies(Input{
		Args: []string{"you", "--json", "models", "invoke", "OMNIVOICE_Q4_K_M", "--operation", "TTS", "--text", "hello"},
		Env:  homeEnvironment(t.TempDir()), Stdout: &output, Context: context.Background(),
	}, Dependencies{BuildModelInvocation: func(_ context.Context, request modelscli.InvocationRequest) (modelscli.InvocationRunner, error) {
		requests = append(requests, request)
		return runner, nil
	}})
	if err != nil {
		t.Fatalf("ExecuteWithDependencies(models invoke) error = %v", err)
	}
	if len(requests) != 1 || requests[0].FactoryDir != filepath.Join(repoRoot, "factory") {
		t.Fatalf("model invocation requests = %+v, want one request for repository factory", requests)
	}
	if runner.invocations != 1 || runner.modelName != "OMNIVOICE_Q4_K_M" {
		t.Fatalf("sentinel invocations = %d model = %q, want one injected invocation", runner.invocations, runner.modelName)
	}
	if !strings.Contains(output.String(), `"modelName":"OMNIVOICE_Q4_K_M"`) {
		t.Fatalf("models invoke output = %q, want sentinel result", output.String())
	}
}

func TestExecuteMCPServeUsesInjectedExecutionCollaborator(t *testing.T) {
	t.Parallel()
	injected := factorysessionexecution.NewFakeService()
	var requests []wire.MCPExecutionRequest
	var output bytes.Buffer
	err := ExecuteWithDependencies(Input{
		Args: []string{"you", "mcp", "serve", "--runtime", "--project-root", t.TempDir()},
		Env:  homeEnvironment(t.TempDir()), Stdin: strings.NewReader(""), Stdout: &output,
		Context: context.Background(),
	}, Dependencies{BuildMCPExecution: func(_ context.Context, request wire.MCPExecutionRequest) (factorysessionexecution.Service, error) {
		requests = append(requests, request)
		return injected, nil
	}})
	if err != nil {
		t.Fatalf("ExecuteWithDependencies(mcp serve) error = %v", err)
	}
	if len(requests) != 1 || !requests[0].RuntimeBacked || strings.TrimSpace(requests[0].ProjectRoot) == "" {
		t.Fatalf("MCP execution requests = %+v, want one normalized runtime-backed request", requests)
	}
	if output.Len() != 0 {
		t.Fatalf("MCP output = %q, want no protocol output for closed stdin", output.String())
	}
}

type sentinelModelInvocationRunner struct {
	invocations int
	modelName   string
}

func (r *sentinelModelInvocationRunner) Run(ctx context.Context) error {
	<-ctx.Done()
	return ctx.Err()
}

func (r *sentinelModelInvocationRunner) InvokeModel(_ context.Context, modelName string, request factoryapi.ModelInvocationRequest) (apisurface.ModelInvocationResult, error) {
	r.invocations++
	r.modelName = modelName
	return apisurface.ModelInvocationResult{ModelName: modelName, Worker: "sentinel-model-worker", Operation: request.Operation}, nil
}

func (*sentinelModelInvocationRunner) GetCurrentFactoryForSession(context.Context, string) (factoryapi.Factory, error) {
	return factoryapi.Factory{Name: "sentinel-factory"}, nil
}

func (*sentinelModelInvocationRunner) CloseFactorySession(_ context.Context, sessionID string) error {
	if sessionID != factorysessions.DefaultSessionID {
		return errors.New("unexpected session id")
	}
	return nil
}

func homeEnvironment(home string) []string {
	if runtime.GOOS == "windows" {
		return []string{"USERPROFILE=" + home}
	}
	if runtime.GOOS == "plan9" {
		return []string{"home=" + home}
	}
	return []string{"HOME=" + home}
}

func TestNormalizeRejectsMissingExecutableAndMalformedEnvironment(t *testing.T) {
	t.Parallel()

	if _, err := Normalize(Input{}); err == nil || !strings.Contains(err.Error(), "executable") {
		t.Fatalf("Normalize(missing executable) error = %v", err)
	}
	if _, err := Normalize(Input{Args: []string{"you"}, Env: []string{"MALFORMED"}}); err == nil || !strings.Contains(err.Error(), "environment") {
		t.Fatalf("Normalize(malformed environment) error = %v", err)
	}
}
