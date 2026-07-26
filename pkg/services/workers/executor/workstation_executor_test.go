// backendsizecheck:ignore-file focused workstation execution tests stay together until the model-binding seam gets a dedicated package-level test split.
// pkgmaintcheck:ignore-file-lines focused workstation execution tests stay together until the model-binding seam gets a dedicated package-level test split.
package executor

import (
	"context"
	"encoding/json"
	"errors"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil/factorydefinitionfixtures"
	modelproviders "github.com/portpowered/infinite-you/packages/model-providers"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	inferencecontract "github.com/portpowered/infinite-you/pkg/services/workers/provider/inferencecontract"
	providerregistry "github.com/portpowered/infinite-you/pkg/services/workers/provider/registry"
)

type workstationRuntimeToken = factoryruntime.RuntimeToken
type workstationRuntimeTokenColor = factoryruntime.RuntimeTokenColor

func TestWorkstationExecutorUsesInjectedProviderSelectionAuthority(t *testing.T) {
	t.Parallel()
	var gotWorkstation, gotFactory, gotWorker string
	executor := &WorkstationExecutor{
		DefaultRunnerID: "factory-provider",
		ResolveRunnerSelection: func(
			workstation string,
			factory string,
			worker string,
		) (workerexecution.ResolvedRunnerSelection, error) {
			gotWorkstation, gotFactory, gotWorker = workstation, factory, worker
			return workerexecution.ResolvedRunnerSelection{
				RunnerID: workerexecution.RunnerIDCursorCLI,
				Source:   workerexecution.RunnerSelectionSourceWorkstation,
			}, nil
		},
	}

	selection, err := executor.resolveRunnerSelection("agent", "codex")
	if err != nil {
		t.Fatalf("resolveRunnerSelection() error = %v", err)
	}
	if selection.RunnerID != workerexecution.RunnerIDCursorCLI {
		t.Fatalf("selection = %#v", selection)
	}
	if gotWorkstation != "agent" || gotFactory != "factory-provider" || gotWorker != "codex" {
		t.Fatalf(
			"resolver inputs = (%q, %q, %q)",
			gotWorkstation,
			gotFactory,
			gotWorker,
		)
	}
}

func TestWorkstationExecutorCarriesCanonicalLegacyProviderThroughInference(t *testing.T) {
	t.Parallel()

	registrations, err := providerregistry.BuiltInRegistrations()
	if err != nil {
		t.Fatalf("BuiltInRegistrations() error = %v", err)
	}
	providers, err := providerregistry.New(registrations...)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	for _, tc := range []struct {
		alias     string
		canonical string
	}{
		{alias: "openai", canonical: "codex"},
		{alias: "anthropic", canonical: "claude"},
	} {
		t.Run(tc.alias, func(t *testing.T) {
			t.Parallel()

			runtimeConfig := staticRuntimeConfig{
				Workers: map[string]*interfaces.FactoryWorkerConfig{
					"worker-a": {
						Type:          interfaces.WorkerTypeModel,
						ModelProvider: tc.alias,
					},
				},
				Workstations: map[string]*interfaces.FactoryWorkstationConfig{
					"standard": {
						Type:           interfaces.WorkstationTypeModel,
						PromptTemplate: "run",
					},
				},
			}
			provider := &agentMockProvider{
				response: workerexecution.InferenceResponse{Content: "done"},
			}
			agent := NewAgentExecutor(
				runtimeConfig,
				provider,
				nil,
				time.Now,
				deterministicRetryRandom,
			)
			workstation := newTestWorkstationExecutor(runtimeConfig, agent)
			workstation.ResolveRunnerSelection = providers.ResolveRunnerSelection

			result, executeErr := workstation.Execute(context.Background(), work.WorkDispatch{
				DispatchID:      "dispatch-" + tc.alias,
				TransitionID:    "transition-" + tc.alias,
				WorkerType:      "worker-a",
				WorkstationName: "standard",
			})
			if executeErr != nil {
				t.Fatalf("Execute() error = %v", executeErr)
			}
			if result.Outcome == workerexecution.OutcomeFailed {
				t.Fatalf("Execute() result = %#v", result)
			}
			if provider.callCount != 1 {
				t.Fatalf("provider calls = %d, want 1", provider.callCount)
			}
			if provider.lastReq.ModelProvider != tc.canonical {
				t.Fatalf(
					"final provider request ModelProvider = %q, want canonical %q",
					provider.lastReq.ModelProvider,
					tc.canonical,
				)
			}
			if provider.lastReq.RunnerID != tc.canonical {
				t.Fatalf(
					"final provider request RunnerID = %q, want canonical %q",
					provider.lastReq.RunnerID,
					tc.canonical,
				)
			}
		})
	}
}

func TestModelProviderForExecutionProjectsCanonicalCursorIdentityToNativeCommand(t *testing.T) {
	t.Parallel()

	got := modelProviderForExecution("cursor", workerexecution.ResolvedRunnerSelection{
		RunnerID: workerexecution.RunnerIDCodex,
		Source:   workerexecution.RunnerSelectionSourceDefault,
	})
	if got != "agent" {
		t.Fatalf("modelProviderForExecution(cursor) = %q, want agent", got)
	}
}

type wsMockExecutor struct {
	dispatch workerexecution.WorkstationExecutionRequest
	called   bool
	result   workerexecution.WorkResult
	err      error
}

type workstationFileSystemStub struct {
	files   map[string][]byte
	reads   []string
	stats   []string
	statErr error
}

func (fileSystem *workstationFileSystemStub) ReadFile(path string) ([]byte, error) {
	fileSystem.reads = append(fileSystem.reads, path)
	content, ok := fileSystem.files[path]
	if !ok {
		return nil, fs.ErrNotExist
	}
	return append([]byte(nil), content...), nil
}

func (fileSystem *workstationFileSystemStub) Stat(path string) (fs.FileInfo, error) {
	fileSystem.stats = append(fileSystem.stats, path)
	return nil, fileSystem.statErr
}

type dispatchCapturingExecutor struct {
	dispatch    workerexecution.WorkstationExecutionRequest
	called      bool
	deadline    time.Time
	hasDeadline bool
	result      workerexecution.WorkResult
	err         error
}

func (m *wsMockExecutor) Execute(_ context.Context, d workerexecution.WorkstationExecutionRequest) (workerexecution.WorkResult, error) {
	m.called = true
	m.dispatch = d
	return m.result, m.err
}

func (m *dispatchCapturingExecutor) Execute(ctx context.Context, d workerexecution.WorkstationExecutionRequest) (workerexecution.WorkResult, error) {
	m.called = true
	m.dispatch = d
	m.deadline, m.hasDeadline = ctx.Deadline()
	return m.result, m.err
}

func newTestWorkstationExecutor(runtimeConfig interfaces.RuntimeConfigLookup, executor WorkstationRequestExecutor) *WorkstationExecutor {
	return &WorkstationExecutor{
		Now:             time.Now,
		RuntimeConfig:   runtimeConfig,
		Executor:        executor,
		Interpolation:   factorydefinitionfixtures.InvocationInterpolation{},
		ExecutionPolicy: scriptedWorkstationExecutionPolicy(),
		Renderer:        &DefaultPromptRenderer{},
		FileSystem:      platformfilesystem.Local{},
	}
}

func scriptedWorkstationExecutionPolicy() interfaces.WorkstationExecutionPolicyService {
	return factorydefinitionfixtures.WorkstationExecutionPolicy{
		Resolve: func(workstation *interfaces.FactoryWorkstationConfig) (time.Duration, error) {
			if workstation == nil {
				return 0, nil
			}
			switch workstation.Limits.MaxExecutionTime {
			case "", "0s":
				return 0, nil
			case "75ms":
				return 75 * time.Millisecond, nil
			default:
				return 0, errors.New("unscripted workstation execution limit")
			}
		},
	}
}

func TestWorkstationExecutorRequiresInjectedClock(t *testing.T) {
	_, err := (&WorkstationExecutor{}).Execute(context.Background(), work.WorkDispatch{})
	if err == nil || !strings.Contains(err.Error(), "clock is required") {
		t.Fatalf("Execute() error = %v, want missing-clock failure", err)
	}
}

func TestPathExistsUsesInjectedWorkstationFileSystem(t *testing.T) {
	t.Parallel()

	files := &workstationFileSystemStub{}
	if !pathExists(files, "virtual-existing-path") {
		t.Fatal("pathExists() = false, want injected successful Stat to mean present")
	}
	if got := strings.Join(files.stats, ","); got != "virtual-existing-path" {
		t.Fatalf("Stat paths = %q, want virtual-existing-path", got)
	}
	files.statErr = fs.ErrNotExist
	if pathExists(files, "virtual-missing-path") {
		t.Fatal("pathExists() = true, want injected not-exist result to mean absent")
	}
}

func TestWorkstationExecutor_ModelWorkstation_RendersPromptAndDelegates(t *testing.T) {
	mock := &wsMockExecutor{result: workerexecution.WorkResult{Outcome: workerexecution.OutcomeAccepted, Output: "done"}}
	we := newTestWorkstationExecutor(
		staticRuntimeConfig{
			Workers: map[string]*interfaces.FactoryWorkerConfig{
				"worker-a": {Type: interfaces.WorkerTypeModel, Body: "You are a helpful assistant."},
			},
			Workstations: map[string]*interfaces.FactoryWorkstationConfig{
				"standard": {Type: interfaces.WorkstationTypeModel, PromptTemplate: "Process work {{ (index .Inputs 0).WorkID }}"},
			},
		},
		mock,
	)

	result, err := we.Execute(context.Background(), work.WorkDispatch{
		DispatchID:      "d-1",
		TransitionID:    "t-1",
		WorkerType:      "worker-a",
		WorkstationName: "standard",
		InputTokens: InputTokens(factoryruntime.RuntimeToken{
			ID:    "tok-1",
			Color: factoryruntime.RuntimeTokenColor{WorkID: "work-1", WorkTypeID: "code-changes"},
		}),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !mock.called {
		t.Fatal("executor was not called")
	}
	if result.Output != "done" {
		t.Fatalf("Output = %q, want %q", result.Output, "done")
	}
	if mock.dispatch.SystemPrompt != "You are a helpful assistant." {
		t.Fatalf("system prompt not set")
	}
	if mock.dispatch.UserMessage != "Process work work-1" {
		t.Fatalf("user message = %q", mock.dispatch.UserMessage)
	}
}

// pkgmaintcheck:ignore-cyclomatic-complexity service-ownership migration preserves this decision flow; simplify branches and remove this exemption.
// pkgmaintcheck:ignore-function-lines service-ownership migration preserves this orchestration flow; extract focused helpers and remove this exemption.
func TestWorkstationExecutor_ModelWorkstation_InterpolatesInvocationArguments(t *testing.T) {
	mock := &wsMockExecutor{result: workerexecution.WorkResult{Outcome: workerexecution.OutcomeAccepted, Output: "done"}}
	we := newTestWorkstationExecutor(
		staticRuntimeConfig{
			Factory: &interfaces.FactoryConfig{
				InvocationSignature: &interfaces.InvocationSignatureConfig{
					Parameters: []interfaces.InvocationParameterConfig{
						{Name: "input"},
						{Name: "provider"},
						{Name: "model"},
						{Name: "apiKey", Sensitive: true},
					},
				},
			},
			Workers: map[string]*interfaces.FactoryWorkerConfig{
				"worker-a": {
					Type:  interfaces.WorkerTypeModel,
					Body:  "Provider ${provider}",
					Model: "${model}",
				},
			},
			Workstations: map[string]*interfaces.FactoryWorkstationConfig{
				"standard": {
					Type:             interfaces.WorkstationTypeModel,
					PromptTemplate:   "Process ${input}",
					WorkingDirectory: "workspace/${provider}",
				},
			},
		},
		mock,
	)
	files := &workstationFileSystemStub{files: map[string][]byte{
		"worker-provider.txt":   []byte("cursor"),
		"workstation-input.txt": []byte("draft"),
	}}
	we.FileSystem = files
	we.Interpolation = factorydefinitionfixtures.InvocationInterpolation{
		InterpolateWorker: func(
			worker interfaces.FactoryWorkerConfig,
			_ *work.InvocationArguments,
			readFile interfaces.FileReader,
		) (interfaces.FactoryWorkerConfig, error) {
			provider, err := readFile("worker-provider.txt")
			if err != nil {
				return interfaces.FactoryWorkerConfig{}, err
			}
			worker.Body = "Provider " + string(provider)
			worker.Model = "gpt-5.5"
			return worker, nil
		},
		InterpolateWorkstation: func(
			workstation interfaces.FactoryWorkstationConfig,
			_ *work.InvocationArguments,
			readFile interfaces.FileReader,
		) (interfaces.FactoryWorkstationConfig, error) {
			input, err := readFile("workstation-input.txt")
			if err != nil {
				return interfaces.FactoryWorkstationConfig{}, err
			}
			workstation.PromptTemplate = "Process " + string(input)
			workstation.WorkingDirectory = "workspace/cursor"
			return workstation, nil
		},
	}

	result, err := we.Execute(context.Background(), work.WorkDispatch{
		DispatchID:      "d-interpolate",
		TransitionID:    "t-interpolate",
		WorkerType:      "worker-a",
		WorkstationName: "standard",
		InputTokens: InputTokens(factoryruntime.RuntimeToken{
			ID: "tok-1",
			Color: factoryruntime.RuntimeTokenColor{
				WorkID: "work-1",
				InvocationArguments: &work.InvocationArguments{
					Arguments: map[string]work.InvocationArgument{
						"input":    {Values: []string{"draft"}},
						"provider": {Values: []string{"cursor"}},
						"model":    {Values: []string{"gpt-5.5"}},
						"apiKey": {
							Values:    []string{"secret"},
							Sensitive: true,
							Sources:   []work.InvocationArgumentSource{{Kind: "NAMED", Redact: true}},
						},
					},
				},
			},
		}),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Outcome != workerexecution.OutcomeAccepted {
		t.Fatalf("Outcome = %s, want %s", result.Outcome, workerexecution.OutcomeAccepted)
	}
	if mock.dispatch.SystemPrompt != "Provider cursor" {
		t.Fatalf("system prompt = %q, want interpolated worker body", mock.dispatch.SystemPrompt)
	}
	if mock.dispatch.UserMessage != "Process draft" {
		t.Fatalf("user message = %q, want interpolated prompt", mock.dispatch.UserMessage)
	}
	if got, want := strings.Join(files.reads, ","), "workstation-input.txt,worker-provider.txt"; got != want {
		t.Fatalf("filesystem reads = %q, want %q", got, want)
	}
	if !strings.HasSuffix(mock.dispatch.WorkingDirectory, filepath.Join("workspace", "cursor")) {
		t.Fatalf("working directory = %q, want interpolated provider path suffix", mock.dispatch.WorkingDirectory)
	}
	if result.Diagnostics == nil || result.Diagnostics.Invocation == nil {
		t.Fatalf("result diagnostics = %#v, want invocation summary", result.Diagnostics)
	}
	if len(result.Diagnostics.Invocation.Parameters) != 4 {
		t.Fatalf("invocation parameter count = %d, want 4", len(result.Diagnostics.Invocation.Parameters))
	}
	for _, parameter := range result.Diagnostics.Invocation.Parameters {
		if parameter.Name == "model" && parameter.Redacted {
			t.Fatalf("model diagnostic = %#v, want non-redacted summary", parameter)
		}
		if parameter.Name == "apiKey" && !parameter.Redacted {
			t.Fatalf("apiKey diagnostic = %#v, want redacted summary", parameter)
		}
	}
}

func TestWorkstationExecutor_ResolvesInterpolatedProviderThroughRegistryBeforeExecution(t *testing.T) {
	t.Parallel()
	providers := providerRegistryWithExternalFixture(t)

	for _, test := range []struct {
		name      string
		resolved  string
		canonical string
	}{
		{name: "canonical", resolved: "codex", canonical: "codex"},
		{name: "openai legacy alias", resolved: "openai", canonical: "codex"},
		{name: "anthropic legacy alias", resolved: "anthropic", canonical: "claude"},
		{name: "cursor manifest alias", resolved: "agent", canonical: "cursor"},
		{name: "kiro manifest alias", resolved: "kiro-cli", canonical: "kiro"},
		{name: "registered extension alias", resolved: "customer", canonical: "customer.provider"},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			mock := &wsMockExecutor{result: workerexecution.WorkResult{
				Outcome: workerexecution.OutcomeAccepted,
				Output:  "done",
			}}
			executor := interpolatedProviderExecutor(test.resolved, mock)
			executor.ResolveProviderIdentity = providers.CanonicalIdentity
			executor.ResolveRunnerSelection = providers.ResolveRunnerSelection

			result, err := executor.Execute(context.Background(), interpolatedProviderDispatch())
			if err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			if result.Outcome != workerexecution.OutcomeAccepted {
				t.Fatalf("Execute() result = %#v", result)
			}
			if !mock.called {
				t.Fatal("provider executor was not called")
			}
			if mock.dispatch.ModelProvider != test.canonical {
				t.Fatalf(
					"execution modelProvider = %q, want canonical %q",
					mock.dispatch.ModelProvider,
					test.canonical,
				)
			}
		})
	}
}

func TestWorkstationExecutor_RejectsInterpolatedProviderBeforeExecutionIO(t *testing.T) {
	t.Parallel()
	providers := providerRegistryWithExternalFixture(t)

	for _, test := range []struct {
		name     string
		resolved string
		want     string
	}{
		{name: "malformed", resolved: "Not_A_Provider", want: "is invalid"},
		{name: "unknown", resolved: "missing.provider", want: "is unknown"},
		{name: "non-selectable", resolved: "agy", want: "is not selectable"},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			mock := &wsMockExecutor{}
			executor := interpolatedProviderExecutor(test.resolved, mock)
			executor.ResolveProviderIdentity = providers.CanonicalIdentity
			// A concrete workstation runner deliberately wins runner precedence.
			// The provider field must still be validated independently.
			executor.ResolveRunnerSelection = providers.ResolveRunnerSelection

			result, err := executor.Execute(context.Background(), interpolatedProviderDispatch())
			if err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			if result.Outcome != workerexecution.OutcomeFailed ||
				!strings.Contains(result.Error, "invocation modelProvider selection failed") ||
				!strings.Contains(result.Error, test.want) {
				t.Fatalf("Execute() result = %#v, want failure containing %q", result, test.want)
			}
			if mock.called {
				t.Fatal("provider executor was called for a rejected provider")
			}
			if result.Diagnostics == nil || result.Diagnostics.Invocation == nil {
				t.Fatalf("result diagnostics = %#v, want invocation diagnostic", result.Diagnostics)
			}
		})
	}
}

func providerRegistryWithExternalFixture(t *testing.T) *providerregistry.Registry {
	t.Helper()
	registrations, err := providerregistry.BuiltInRegistrations()
	if err != nil {
		t.Fatalf("BuiltInRegistrations() error = %v", err)
	}
	var catalog struct {
		Providers []providerregistry.Manifest `json:"providers"`
	}
	if err := json.Unmarshal(modelproviders.CatalogJSON(), &catalog); err != nil {
		t.Fatalf("decode provider catalog: %v", err)
	}
	manifest := catalog.Providers[0]
	manifest.ID = "customer.provider"
	manifest.Aliases = []string{"customer"}
	manifest.ImplementationAvailability = providerregistry.ImplementationExternallySupplied
	manifest.TechnicalSupportLevel = providerregistry.SupportProduction
	manifest.Deprecation = nil
	manifest.MaximumExecutionCapabilities = providerregistry.ExecutionCapabilities{
		PromptSubmission: true,
	}
	manifest.MaximumResponseFidelityCapabilities = providerregistry.ResponseFidelityCapabilities{}
	registrations = append(registrations, providerregistry.ExternalRegistration(
		manifest,
		executorTestIntegration{},
	))
	providers, err := providerregistry.New(registrations...)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return providers
}

type executorTestIntegration struct{}

func (executorTestIntegration) Identity() inferencecontract.Identity {
	return "customer.provider"
}

func (executorTestIntegration) MaximumCapabilities() inferencecontract.CapabilitySet {
	return inferencecontract.NewCapabilitySet(inferencecontract.CapabilityPromptSubmission)
}

func (executorTestIntegration) Discover(context.Context) (inferencecontract.Discovery, error) {
	panic("provider discovery must not run during provider selection")
}

func (executorTestIntegration) Capabilities(
	context.Context,
	inferencecontract.InvocationRequest,
) (inferencecontract.CapabilitySet, error) {
	panic("provider capability I/O must not run during provider selection")
}

func (executorTestIntegration) Invoke(
	context.Context,
	inferencecontract.InvocationRequest,
	inferencecontract.ResponseWriter,
) error {
	panic("provider invocation must not run during provider selection")
}

func interpolatedProviderExecutor(
	resolved string,
	mock WorkstationRequestExecutor,
) *WorkstationExecutor {
	executor := newTestWorkstationExecutor(staticRuntimeConfig{
		Factory: &interfaces.FactoryConfig{
			InvocationSignature: &interfaces.InvocationSignatureConfig{
				Parameters: []interfaces.InvocationParameterConfig{{Name: "provider"}},
			},
		},
		Workers: map[string]*interfaces.FactoryWorkerConfig{
			"worker-a": {
				Type:          interfaces.WorkerTypeModel,
				ModelProvider: "${provider}",
			},
		},
		Workstations: map[string]*interfaces.FactoryWorkstationConfig{
			"standard": {
				Type:           interfaces.WorkstationTypeModel,
				Runner:         "codex",
				PromptTemplate: "run",
			},
		},
	}, mock)
	executor.Interpolation = factorydefinitionfixtures.InvocationInterpolation{
		InterpolateWorker: func(
			worker interfaces.FactoryWorkerConfig,
			_ *work.InvocationArguments,
			_ interfaces.FileReader,
		) (interfaces.FactoryWorkerConfig, error) {
			worker.ModelProvider = resolved
			return worker, nil
		},
	}
	return executor
}

func interpolatedProviderDispatch() work.WorkDispatch {
	return work.WorkDispatch{
		DispatchID:      "dispatch-provider",
		TransitionID:    "transition-provider",
		WorkerType:      "worker-a",
		WorkstationName: "standard",
		InputTokens: InputTokens(factoryruntime.RuntimeToken{
			ID: "token-provider",
			Color: factoryruntime.RuntimeTokenColor{
				InvocationArguments: &work.InvocationArguments{
					Arguments: map[string]work.InvocationArgument{
						"provider": {Values: []string{"unused-by-scripted-interpolator"}},
					},
				},
			},
		}),
	}
}

// pkgmaintcheck:ignore-cyclomatic-complexity this workstation execution contract test keeps canonical runtime field assertions together on the worker seam.
func TestWorkstationExecutor_ModelWorkstationUsesCanonicalWorkstationRuntimeFields(t *testing.T) {
	projectRoot := t.TempDir()

	mock := &dispatchCapturingExecutor{result: workerexecution.WorkResult{Outcome: workerexecution.OutcomeAccepted, Output: "done"}}
	we := newTestWorkstationExecutor(canonicalWorkstationRuntimeConfig(projectRoot), mock)

	start := time.Now()
	result, err := we.Execute(context.Background(), canonicalWorkstationDispatch())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Outcome != workerexecution.OutcomeAccepted {
		t.Fatalf("Outcome = %s, want %s", result.Outcome, workerexecution.OutcomeAccepted)
	}
	if !mock.called {
		t.Fatal("executor was not called")
	}
	if mock.dispatch.WorkerType != "canonical-worker" {
		t.Fatalf("worker type = %q, want canonical worker binding", mock.dispatch.WorkerType)
	}
	if mock.dispatch.ProjectID != "agent-factory" {
		t.Fatalf("project ID = %q, want canonical dispatch project context", mock.dispatch.ProjectID)
	}
	if mock.dispatch.SystemPrompt != "canonical system" {
		t.Fatalf("system prompt = %q, want canonical worker body", mock.dispatch.SystemPrompt)
	}
	if mock.dispatch.UserMessage != "Review work-1 for agent-factory" {
		t.Fatalf("user message = %q", mock.dispatch.UserMessage)
	}
	if mock.dispatch.OutputSchema != `{"type":"object"}` {
		t.Fatalf("output schema = %q", mock.dispatch.OutputSchema)
	}
	if mock.dispatch.WorkingDirectory != filepath.Join(projectRoot, "repo", "feature-runtime") {
		t.Fatalf("working directory = %q", mock.dispatch.WorkingDirectory)
	}
	if mock.dispatch.Worktree != "worktrees/feature-runtime" {
		t.Fatalf("worktree = %q", mock.dispatch.Worktree)
	}
	if mock.dispatch.EnvVars["PROJECT"] != "agent-factory" || mock.dispatch.EnvVars["BRANCH"] != "feature-runtime" {
		t.Fatalf("env vars = %#v", mock.dispatch.EnvVars)
	}
	if !mock.hasDeadline {
		t.Fatal("expected workstation timeout to set executor deadline")
	}
	remaining := mock.deadline.Sub(start)
	if remaining < 30*time.Millisecond || remaining > 250*time.Millisecond {
		t.Fatalf("deadline offset = %v, want workstation timeout range", remaining)
	}
}

func TestResolveModelOperationBindings_UsesInputThenConfigThenDefaultAndRecordsSource(t *testing.T) {
	workstation := &interfaces.FactoryWorkstationConfig{
		Type:      interfaces.WorkstationTypeInvoke,
		Operation: "TTS",
		OperationBindings: []interfaces.ModelOperationBinding{
			{Slot: "text", Selector: &interfaces.ModelOperationBindingSelector{Label: "utterance", Type: interfaces.ModelOperationContentTypeText}},
			{
				Slot:     "voice",
				Selector: &interfaces.ModelOperationBindingSelector{Role: "voice"},
				Config:   []work.WorkContentPart{{Type: work.WorkContentPartTypeJSON, Role: "voice", JSON: []byte(`{"name":"alloy"}`)}},
			},
			{Slot: "style", DefaultContent: []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "neutral", Slot: "style"}}},
		},
	}
	worker := &interfaces.FactoryWorkerConfig{
		Name: "tts-worker",
		Operations: []interfaces.ModelOperation{{
			Name:   "TTS",
			Inputs: []interfaces.ModelOperationSlot{{Name: "text", Required: true}, {Name: "voice"}, {Name: "style"}, {Name: "optional"}},
		}},
	}
	inputs := []factoryruntime.RuntimeToken{{
		ID: "tok-1",
		Color: factoryruntime.RuntimeTokenColor{Content: []work.WorkContentPart{
			{Type: work.WorkContentPartTypeText, Slot: "ignored", Label: "utterance", Text: "first"},
			{Type: work.WorkContentPartTypeText, Slot: "text", Label: "utterance", Text: "second"},
		}},
	}}

	got, err := resolveModelOperationBindings(workstation, worker, inputs)
	if err != nil {
		t.Fatalf("resolveModelOperationBindings: %v", err)
	}
	if len(got) != 4 {
		t.Fatalf("binding count = %d, want 4", len(got))
	}
	if got[0].Source != workerexecution.ModelOperationBindingSourceInput || got[0].Content[0].Text != "first" {
		t.Fatalf("text binding = %#v, want first input match", got[0])
	}
	if got[1].Source != workerexecution.ModelOperationBindingSourceConfig || string(got[1].Content[0].JSON) != `{"name":"alloy"}` {
		t.Fatalf("voice binding = %#v, want config fallback", got[1])
	}
	if got[2].Source != workerexecution.ModelOperationBindingSourceDefault || got[2].Content[0].Text != "neutral" {
		t.Fatalf("style binding = %#v, want default fallback", got[2])
	}
	if got[3].Source != workerexecution.ModelOperationBindingSourceOmitted || len(got[3].Content) != 0 {
		t.Fatalf("optional binding = %#v, want omitted", got[3])
	}
}

func TestResolveModelOperationBindings_ImplicitlyMatchesBySlotAndRejectsMissingRequiredInput(t *testing.T) {
	workstation := &interfaces.FactoryWorkstationConfig{Type: interfaces.WorkstationTypeInvoke, Operation: "TTS"}
	worker := &interfaces.FactoryWorkerConfig{
		Name: "tts-worker",
		Operations: []interfaces.ModelOperation{{
			Name:   "TTS",
			Inputs: []interfaces.ModelOperationSlot{{Name: "text", Required: true}},
		}},
	}

	got, err := resolveModelOperationBindings(workstation, worker, []factoryruntime.RuntimeToken{{
		ID:    "tok-1",
		Color: factoryruntime.RuntimeTokenColor{Content: []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Slot: "text", Text: "hello"}}},
	}})
	if err != nil {
		t.Fatalf("resolveModelOperationBindings implicit slot: %v", err)
	}
	if len(got) != 1 || got[0].Source != workerexecution.ModelOperationBindingSourceInput || got[0].Content[0].Text != "hello" {
		t.Fatalf("implicit slot binding = %#v, want input text", got)
	}

	_, err = resolveModelOperationBindings(workstation, worker, nil)
	if err == nil {
		t.Fatal("expected missing required input slot to fail")
	}
}

func TestInferenceRequestForExecutionRequest_AuthoredWorkingDirectoryRequiresRunnerCapability(t *testing.T) {
	req := testAgentRequest(
		work.WorkDispatch{DispatchID: "d-authored-workdir", TransitionID: "t-authored-workdir", WorkerType: "worker-a", WorkstationName: "review"},
		withAgentPrompts("System prompt", "Review"),
		withAgentWorkingDirectory("/tmp/authored"),
		func(req *workerexecution.WorkstationExecutionRequest) {
			req.WorkingDirectoryAuthored = true
		},
	)
	got := inferenceRequestForExecutionRequest(req, &interfaces.FactoryWorkerConfig{
		Model: "gemini-1.5-pro", ModelProvider: workerexecution.RunnerIDGemini,
	}, nil)
	for _, capability := range got.RequiredOptionalCapabilities {
		if capability == workerexecution.RunnerOptionalCapabilityWorkingDirectory {
			return
		}
	}
	t.Fatalf("capabilities = %#v, want authored working directory capability", got.RequiredOptionalCapabilities)
}

func TestWorkstationExecutor_ModelWorkstation_PreservesDistinctMultiInputCanonicalContent(t *testing.T) {
	mock := &dispatchCapturingExecutor{result: workerexecution.WorkResult{Outcome: workerexecution.OutcomeAccepted, Output: "done"}}
	we := newTestWorkstationExecutor(
		staticRuntimeConfig{
			Workers: map[string]*interfaces.FactoryWorkerConfig{
				"worker-a": {Type: interfaces.WorkerTypeModel, Body: "system"},
			},
			Workstations: map[string]*interfaces.FactoryWorkstationConfig{
				"standard": {
					Type: interfaces.WorkstationTypeModel,
					PromptTemplate: `{{ (index .Inputs 0).WorkID }}:{{ range (index .Inputs 0).Content }} [{{ .Type }}={{ .Text }}{{ .File }}]{{ end }}
{{ (index .Inputs 1).WorkID }}:{{ range (index .Inputs 1).Content }} [{{ .Type }}={{ .Text }}{{ .File }}]{{ end }}`,
				},
			},
		},
		mock,
	)

	result, err := we.Execute(context.Background(), work.WorkDispatch{
		DispatchID:      "d-multi-content",
		TransitionID:    "t-multi-content",
		WorkerType:      "worker-a",
		WorkstationName: "standard",
		InputTokens: InputTokens(
			factoryruntime.RuntimeToken{
				ID: "tok-text",
				Color: factoryruntime.RuntimeTokenColor{
					WorkID: "work-text",
					Content: []work.WorkContentPart{
						{Type: work.WorkContentPartTypeText, Text: "plan"},
					},
					Payload: []byte("plan"),
				},
			},
			factoryruntime.RuntimeToken{
				ID: "tok-mixed",
				Color: factoryruntime.RuntimeTokenColor{
					WorkID: "work-mixed",
					Content: []work.WorkContentPart{
						{Type: work.WorkContentPartTypeText, Text: "caption"},
						{Type: work.WorkContentPartTypeImage, File: "fixtures/mockup.png"},
					},
					Payload: []byte("caption"),
				},
			},
		),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Outcome != workerexecution.OutcomeAccepted {
		t.Fatalf("Outcome = %s, want %s", result.Outcome, workerexecution.OutcomeAccepted)
	}
	if !mock.called {
		t.Fatal("executor was not called")
	}
	if !strings.Contains(mock.dispatch.UserMessage, "work-text: [text=plan]") {
		t.Fatalf("rendered prompt = %q, want first input content preserved", mock.dispatch.UserMessage)
	}
	if !strings.Contains(mock.dispatch.UserMessage, "work-mixed: [text=caption] [image=fixtures/mockup.png]") {
		t.Fatalf("rendered prompt = %q, want second input mixed content preserved", mock.dispatch.UserMessage)
	}

	inputTokens := executionRequestInputTokens(mock.dispatch)
	if len(inputTokens) != 2 {
		t.Fatalf("forwarded input token count = %d, want 2", len(inputTokens))
	}
	if inputTokens[0].Color.WorkID != "work-text" || len(inputTokens[0].Color.Content) != 1 {
		t.Fatalf("first forwarded input = %#v, want text input preserved", inputTokens[0].Color)
	}
	if inputTokens[1].Color.WorkID != "work-mixed" || len(inputTokens[1].Color.Content) != 2 {
		t.Fatalf("second forwarded input = %#v, want mixed-content input preserved", inputTokens[1].Color)
	}
	if inputTokens[1].Color.Content[1].Type != work.WorkContentPartTypeImage || inputTokens[1].Color.Content[1].File != "fixtures/mockup.png" {
		t.Fatalf("second forwarded input content = %#v, want ordered image part", inputTokens[1].Color.Content)
	}
}

func TestWorkstationExecutor_ResolveWorkstationExecutionContext_AppliesResolvedRuntimeFields(t *testing.T) {
	projectRoot := t.TempDir()
	we := newTestWorkstationExecutor(canonicalWorkstationRuntimeConfig(projectRoot), &wsMockExecutor{})
	workstationDef, ok := we.RuntimeConfig.Workstation("review")
	if !ok {
		t.Fatal("expected review workstation")
	}

	resolved, failed := we.resolveWorkstationExecutionContext(
		canonicalWorkstationDispatch(),
		workstationDef,
		time.Now(),
		logging.NoopLogger{},
	)
	if failed != nil {
		t.Fatalf("unexpected failed result: %#v", failed)
	}

	if resolved.ProjectID != "agent-factory" {
		t.Fatalf("project ID = %q", resolved.ProjectID)
	}
	if resolved.WorkingDirectory != filepath.Join(projectRoot, "repo", "feature-runtime") {
		t.Fatalf("working directory = %q", resolved.WorkingDirectory)
	}
	if resolved.Worktree != "worktrees/feature-runtime" {
		t.Fatalf("worktree = %q", resolved.Worktree)
	}
	if resolved.EnvVars["PROJECT"] != "agent-factory" || resolved.EnvVars["BRANCH"] != "feature-runtime" {
		t.Fatalf("env vars = %#v", resolved.EnvVars)
	}
}

func TestWorkstationExecutor_ResolvesRelativeWorkingDirectoryAgainstRuntimeConfigFactoryDirectory(t *testing.T) {
	wantDir := t.TempDir()
	setTestWorkingDirectory(t, t.TempDir())

	mock := &dispatchCapturingExecutor{result: workerexecution.WorkResult{Outcome: workerexecution.OutcomeAccepted, Output: "done"}}
	we := newTestWorkstationExecutor(
		staticRuntimeConfig{
			FactoryPath: wantDir,
			Workers: map[string]*interfaces.FactoryWorkerConfig{
				"worker-a": {Type: interfaces.WorkerTypeModel, Body: "system"},
			},
			Workstations: map[string]*interfaces.FactoryWorkstationConfig{
				"standard": {
					Type:             interfaces.WorkstationTypeModel,
					PromptTemplate:   "Work from {{ .Context.WorkDir }}",
					WorkingDirectory: ".",
				},
			},
		},
		mock,
	)

	result, err := we.Execute(context.Background(), work.WorkDispatch{
		DispatchID:      "d-relative",
		TransitionID:    "t-relative",
		WorkerType:      "worker-a",
		WorkstationName: "standard",
		InputTokens:     InputTokens(workstationRuntimeToken{ID: "tok-1", Color: workstationRuntimeTokenColor{WorkID: "work-1"}}),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Outcome != workerexecution.OutcomeAccepted {
		t.Fatalf("Outcome = %s, want %s", result.Outcome, workerexecution.OutcomeAccepted)
	}
	if mock.dispatch.WorkingDirectory != wantDir {
		t.Fatalf("working directory = %q, want %q", mock.dispatch.WorkingDirectory, wantDir)
	}
	if mock.dispatch.UserMessage != "Work from "+wantDir {
		t.Fatalf("user message = %q", mock.dispatch.UserMessage)
	}
}

func TestWorkstationExecutor_ResolvesRelativeWorkingDirectoryAgainstRuntimeBaseDirectoryOverride(t *testing.T) {
	factoryDir := t.TempDir()
	wantDir := t.TempDir()
	setTestWorkingDirectory(t, t.TempDir())

	mock := &dispatchCapturingExecutor{result: workerexecution.WorkResult{Outcome: workerexecution.OutcomeAccepted, Output: "done"}}
	we := newTestWorkstationExecutor(
		staticRuntimeConfig{
			FactoryPath:     factoryDir,
			RuntimeBasePath: wantDir,
			Workers: map[string]*interfaces.FactoryWorkerConfig{
				"worker-a": {Type: interfaces.WorkerTypeModel, Body: "system"},
			},
			Workstations: map[string]*interfaces.FactoryWorkstationConfig{
				"standard": {
					Type:             interfaces.WorkstationTypeModel,
					PromptTemplate:   "Work from {{ .Context.WorkDir }}",
					WorkingDirectory: ".",
				},
			},
		},
		mock,
	)

	result, err := we.Execute(context.Background(), work.WorkDispatch{
		DispatchID:      "d-relative-runtime-base",
		TransitionID:    "t-relative-runtime-base",
		WorkerType:      "worker-a",
		WorkstationName: "standard",
		InputTokens:     InputTokens(workstationRuntimeToken{ID: "tok-1", Color: workstationRuntimeTokenColor{WorkID: "work-1"}}),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Outcome != workerexecution.OutcomeAccepted {
		t.Fatalf("Outcome = %s, want %s", result.Outcome, workerexecution.OutcomeAccepted)
	}
	if mock.dispatch.WorkingDirectory != wantDir {
		t.Fatalf("working directory = %q, want %q", mock.dispatch.WorkingDirectory, wantDir)
	}
	if mock.dispatch.UserMessage != "Work from "+wantDir {
		t.Fatalf("user message = %q", mock.dispatch.UserMessage)
	}
}

func TestWorkstationExecutor_ResolvesPortableRootedWorkingDirectoryAgainstRuntimeBaseDirectoryOverride(t *testing.T) {
	wantDir := t.TempDir()
	setTestWorkingDirectory(t, t.TempDir())

	mock := &dispatchCapturingExecutor{result: workerexecution.WorkResult{Outcome: workerexecution.OutcomeAccepted, Output: "done"}}
	we := newTestWorkstationExecutor(
		staticRuntimeConfig{
			RuntimeBasePath: wantDir,
			Workers: map[string]*interfaces.FactoryWorkerConfig{
				"worker-a": {Type: interfaces.WorkerTypeModel, Body: "system"},
			},
			Workstations: map[string]*interfaces.FactoryWorkstationConfig{
				"standard": {
					Type:             interfaces.WorkstationTypeModel,
					PromptTemplate:   "Work from {{ .Context.WorkDir }}",
					WorkingDirectory: "/worktrees/feature-abc",
				},
			},
		},
		mock,
	)

	result, err := we.Execute(context.Background(), work.WorkDispatch{
		DispatchID:      "d-portable-rooted",
		TransitionID:    "t-portable-rooted",
		WorkerType:      "worker-a",
		WorkstationName: "standard",
		InputTokens:     InputTokens(factoryruntime.RuntimeToken{ID: "tok-1", Color: factoryruntime.RuntimeTokenColor{WorkID: "work-1"}}),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Outcome != workerexecution.OutcomeAccepted {
		t.Fatalf("Outcome = %s, want %s", result.Outcome, workerexecution.OutcomeAccepted)
	}
	expectedDir := filepath.Join(wantDir, "worktrees", "feature-abc")
	if mock.dispatch.WorkingDirectory != expectedDir {
		t.Fatalf("working directory = %q, want %q", mock.dispatch.WorkingDirectory, expectedDir)
	}
	if mock.dispatch.UserMessage != "Work from "+expectedDir {
		t.Fatalf("user message = %q", mock.dispatch.UserMessage)
	}
}

func TestWorkstationExecutor_PreservesExistingUnixAbsoluteWorkingDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix absolute path semantics do not apply on Windows")
	}
	absoluteDir := t.TempDir()
	setTestWorkingDirectory(t, t.TempDir())

	mock := &dispatchCapturingExecutor{result: workerexecution.WorkResult{Outcome: workerexecution.OutcomeAccepted, Output: "done"}}
	we := newTestWorkstationExecutor(
		staticRuntimeConfig{
			RuntimeBasePath: t.TempDir(),
			Workers: map[string]*interfaces.FactoryWorkerConfig{
				"worker-a": {Type: interfaces.WorkerTypeModel, Body: "system"},
			},
			Workstations: map[string]*interfaces.FactoryWorkstationConfig{
				"standard": {
					Type:             interfaces.WorkstationTypeModel,
					PromptTemplate:   "Work from {{ .Context.WorkDir }}",
					WorkingDirectory: filepath.ToSlash(absoluteDir),
				},
			},
		},
		mock,
	)

	result, err := we.Execute(context.Background(), work.WorkDispatch{
		DispatchID:      "d-unix-absolute",
		TransitionID:    "t-unix-absolute",
		WorkerType:      "worker-a",
		WorkstationName: "standard",
		InputTokens:     InputTokens(factoryruntime.RuntimeToken{ID: "tok-1", Color: factoryruntime.RuntimeTokenColor{WorkID: "work-1"}}),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Outcome != workerexecution.OutcomeAccepted {
		t.Fatalf("Outcome = %s, want %s", result.Outcome, workerexecution.OutcomeAccepted)
	}
	if mock.dispatch.WorkingDirectory != filepath.Clean(absoluteDir) {
		t.Fatalf("working directory = %q, want %q", mock.dispatch.WorkingDirectory, filepath.Clean(absoluteDir))
	}
	if mock.dispatch.UserMessage != "Work from "+filepath.Clean(absoluteDir) {
		t.Fatalf("user message = %q", mock.dispatch.UserMessage)
	}
}

func TestWorkstationExecutor_LoadedRuntimeConfigRuntimeBaseDirOverrideDrivesRelativeExecutionPath(t *testing.T) {
	factoryDir := t.TempDir()
	runtimeBaseDir := t.TempDir()
	setTestWorkingDirectory(t, t.TempDir())
	runtimeCfg, err := factorydefinitionfixtures.NewLoadedSource(
		factoryDir,
		&interfaces.FactoryConfig{
			Name: "agent-factory",
			Workers: []interfaces.FactoryWorkerConfig{{
				Name: "worker-a", Type: interfaces.WorkerTypeModel, Model: "gpt-5.4", Body: "System prompt.",
			}},
			Workstations: []interfaces.FactoryWorkstationConfig{{
				Name: "standard", Type: interfaces.WorkstationTypeModel, WorkerTypeName: "worker-a",
				WorkingDirectory: "workspace", PromptTemplate: "Work from {{ .Context.WorkDir }}",
			}},
		},
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("NewLoadedSource: %v", err)
	}
	runtimeCfg.SetRuntimeBaseDir(runtimeBaseDir)

	mock := &dispatchCapturingExecutor{result: workerexecution.WorkResult{Outcome: workerexecution.OutcomeAccepted, Output: "done"}}
	we := newTestWorkstationExecutor(runtimeCfg, mock)

	result, err := we.Execute(context.Background(), work.WorkDispatch{
		DispatchID:      "d-loaded-runtime-base",
		TransitionID:    "t-loaded-runtime-base",
		WorkerType:      "worker-a",
		WorkstationName: "standard",
		ProjectID:       "agent-factory",
		InputTokens:     InputTokens(factoryruntime.RuntimeToken{ID: "tok-1", Color: factoryruntime.RuntimeTokenColor{WorkID: "work-1"}}),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Outcome != workerexecution.OutcomeAccepted {
		t.Fatalf("Outcome = %s, want %s", result.Outcome, workerexecution.OutcomeAccepted)
	}
	if mock.dispatch.WorkingDirectory != filepath.Join(runtimeBaseDir, "workspace") {
		t.Fatalf("working directory = %q, want %q", mock.dispatch.WorkingDirectory, filepath.Join(runtimeBaseDir, "workspace"))
	}
	if mock.dispatch.UserMessage != "Work from "+filepath.Join(runtimeBaseDir, "workspace") {
		t.Fatalf("user message = %q", mock.dispatch.UserMessage)
	}
}

func setTestWorkingDirectory(t *testing.T, dir string) {
	t.Helper()

	originalWorkingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir(%q): %v", dir, err)
	}
	t.Cleanup(func() {
		if chdirErr := os.Chdir(originalWorkingDirectory); chdirErr != nil {
			t.Fatalf("restore working directory: %v", chdirErr)
		}
	})
}

func writeRuntimeLookupFixture(t *testing.T, factoryDir string) {
	t.Helper()

	writeRuntimeLookupFactoryJSON(t, factoryDir, map[string]any{
		"id": "agent-factory",
		"workTypes": []map[string]any{
			{
				"name": "task",
				"states": []map[string]string{
					{"name": "init", "type": "INITIAL"},
					{"name": "complete", "type": "TERMINAL"},
				},
			},
		},
		"workers": []map[string]any{
			{"name": "worker-a"},
		},
		"workstations": []map[string]any{
			{
				"name":    "standard",
				"worker":  "worker-a",
				"inputs":  []map[string]string{{"workType": "task", "state": "init"}},
				"outputs": []map[string]string{{"workType": "task", "state": "complete"}},
			},
		},
	})
	writeRuntimeLookupAgentsMD(t, filepath.Join(factoryDir, "workers", "worker-a"), `---
type: MODEL_WORKER
model: gpt-5.4
---
System prompt.
`)
	writeRuntimeLookupAgentsMD(t, filepath.Join(factoryDir, "workstations", "standard"), `---
type: MODEL_WORKSTATION
worker: worker-a
workingDirectory: workspace
---
Work from {{ .Context.WorkDir }}
`)
}

func writeRuntimeLookupFactoryJSON(t *testing.T, factoryDir string, cfg map[string]any) {
	t.Helper()
	if _, ok := cfg["name"]; !ok {
		cfg["name"] = filepath.Base(factoryDir)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent: %v", err)
	}
	if err := os.WriteFile(filepath.Join(factoryDir, interfaces.FactoryConfigFile), data, 0o644); err != nil {
		t.Fatalf("write factory.json: %v", err)
	}
}

func writeRuntimeLookupAgentsMD(t *testing.T, dir string, content string) {
	t.Helper()

	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, interfaces.FactoryAgentsFileName), []byte(content), 0o644); err != nil {
		t.Fatalf("write AGENTS.md: %v", err)
	}
}

func canonicalWorkstationRuntimeConfig(factoryDir string) staticRuntimeConfig {
	return staticRuntimeConfig{
		FactoryPath: factoryDir,
		Workers: map[string]*interfaces.FactoryWorkerConfig{
			"canonical-worker": {Type: interfaces.WorkerTypeModel, Body: "canonical system", Timeout: "1h"},
		},
		Workstations: map[string]*interfaces.FactoryWorkstationConfig{
			"review": {
				Type:             interfaces.WorkstationTypeModel,
				WorkerTypeName:   "canonical-worker",
				PromptTemplate:   `Review {{ (index .Inputs 0).WorkID }} for {{ .Context.Project }}`,
				OutputSchema:     `{"type":"object"}`,
				Limits:           interfaces.WorkstationLimits{MaxExecutionTime: "75ms"},
				StopWords:        []string{"DONE"},
				WorkingDirectory: `/repo/{{ index (index .Inputs 0).Tags "branch" }}`,
				Worktree:         `worktrees/{{ index (index .Inputs 0).Tags "branch" }}`,
				Env: map[string]string{
					"PROJECT": "{{ .Context.Project }}",
					"BRANCH":  `{{ index (index .Inputs 0).Tags "branch" }}`,
				},
			},
		},
	}
}

func canonicalWorkstationDispatch() work.WorkDispatch {
	return work.WorkDispatch{
		DispatchID:      "d-canonical",
		TransitionID:    "t-review",
		WorkerType:      "stale-worker",
		WorkstationName: "review",
		ProjectID:       "agent-factory",
		InputTokens: InputTokens(factoryruntime.RuntimeToken{
			ID: "tok-1",
			Color: factoryruntime.RuntimeTokenColor{
				WorkID: "work-1",
				Tags:   map[string]string{"branch": "feature-runtime"},
			},
		}),
	}
}

func TestWorkstationExecutor_LogicalMove_DoesNotCallExecutor(t *testing.T) {
	mock := &wsMockExecutor{}
	we := newTestWorkstationExecutor(
		staticRuntimeConfig{
			Workstations: map[string]*interfaces.FactoryWorkstationConfig{
				"logical": {Type: interfaces.WorkstationTypeLogical},
			},
		},
		mock,
	)

	result, err := we.Execute(context.Background(), work.WorkDispatch{DispatchID: "d-1", TransitionID: "t-logical", WorkstationName: "logical"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mock.called {
		t.Fatal("executor should not be called")
	}
	if result.Outcome != workerexecution.OutcomeAccepted {
		t.Fatalf("Outcome = %s, want %s", result.Outcome, workerexecution.OutcomeAccepted)
	}
}

func TestWorkstationExecutor_ExecutorError_ReturnsFailedResult(t *testing.T) {
	mock := &wsMockExecutor{err: errors.New("connection timeout")}
	we := newTestWorkstationExecutor(
		staticRuntimeConfig{
			Workers: map[string]*interfaces.FactoryWorkerConfig{
				"worker-a": {Body: "system"},
			},
			Workstations: map[string]*interfaces.FactoryWorkstationConfig{
				"standard": {Type: interfaces.WorkstationTypeModel, PromptTemplate: "do work"},
			},
		},
		mock,
	)

	result, err := we.Execute(context.Background(), work.WorkDispatch{
		DispatchID:      "d-1",
		TransitionID:    "t-1",
		WorkerType:      "worker-a",
		WorkstationName: "standard",
		InputTokens:     InputTokens(factoryruntime.RuntimeToken{ID: "tok-1"}),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Outcome != workerexecution.OutcomeFailed {
		t.Fatalf("Outcome = %s, want %s", result.Outcome, workerexecution.OutcomeFailed)
	}
	if result.Error != "executor failed: connection timeout" {
		t.Fatalf("Error = %q", result.Error)
	}
}

func TestWorkstationExecutor_ClassifierTrimsLabelAndIgnoresNonFailureOutcomeKinds(t *testing.T) {
	mock := &wsMockExecutor{result: workerexecution.WorkResult{Outcome: workerexecution.OutcomeRejected, Output: "  approved \n"}}
	we := newTestWorkstationExecutor(
		staticRuntimeConfig{
			Workers: map[string]*interfaces.FactoryWorkerConfig{
				"worker-a": {Body: "system"},
			},
			Workstations: map[string]*interfaces.FactoryWorkstationConfig{
				"classifier": {Type: interfaces.WorkstationTypeClassify, PromptTemplate: "classify"},
			},
		},
		mock,
	)

	result, err := we.Execute(context.Background(), work.WorkDispatch{
		DispatchID:      "d-classifier-trim",
		TransitionID:    "t-classifier-trim",
		WorkerType:      "worker-a",
		WorkstationName: "classifier",
		InputTokens:     InputTokens(factoryruntime.RuntimeToken{ID: "tok-1"}),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Outcome != workerexecution.OutcomeAccepted {
		t.Fatalf("Outcome = %s, want %s", result.Outcome, workerexecution.OutcomeAccepted)
	}
	if result.Output != "approved" {
		t.Fatalf("Output = %q, want approved", result.Output)
	}
}

func TestWorkstationExecutor_ClassifierRejectsJSONStringLabel(t *testing.T) {
	mock := &wsMockExecutor{result: workerexecution.WorkResult{Outcome: workerexecution.OutcomeAccepted, Output: "\"needs_review\""}}
	we := newTestWorkstationExecutor(
		staticRuntimeConfig{
			Workers: map[string]*interfaces.FactoryWorkerConfig{
				"worker-a": {Body: "system"},
			},
			Workstations: map[string]*interfaces.FactoryWorkstationConfig{
				"classifier": {Type: interfaces.WorkstationTypeClassify, PromptTemplate: "classify"},
			},
		},
		mock,
	)

	result, err := we.Execute(context.Background(), work.WorkDispatch{
		DispatchID:      "d-classifier-json-string",
		TransitionID:    "t-classifier-json-string",
		WorkerType:      "worker-a",
		WorkstationName: "classifier",
		InputTokens:     InputTokens(factoryruntime.RuntimeToken{ID: "tok-1"}),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Outcome != workerexecution.OutcomeFailed {
		t.Fatalf("Outcome = %s, want %s", result.Outcome, workerexecution.OutcomeFailed)
	}
	if result.Error != `classifier output invalid: expected plain string label (raw output: "\"needs_review\"")` {
		t.Fatalf("Error = %q", result.Error)
	}
}

func TestWorkstationExecutor_ClassifierRejectsEmptyOrNonStringOutput(t *testing.T) {
	testCases := []struct {
		name   string
		output string
	}{
		{name: "empty", output: " \n\t "},
		{name: "json object", output: `{"label":"approved"}`},
		{name: "json number", output: `123`},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mock := &wsMockExecutor{result: workerexecution.WorkResult{Outcome: workerexecution.OutcomeAccepted, Output: tc.output}}
			we := newTestWorkstationExecutor(
				staticRuntimeConfig{
					Workers: map[string]*interfaces.FactoryWorkerConfig{
						"worker-a": {Body: "system"},
					},
					Workstations: map[string]*interfaces.FactoryWorkstationConfig{
						"classifier": {Type: interfaces.WorkstationTypeClassify, PromptTemplate: "classify"},
					},
				},
				mock,
			)

			result, err := we.Execute(context.Background(), work.WorkDispatch{
				DispatchID:      "d-classifier-invalid",
				TransitionID:    "t-classifier-invalid",
				WorkerType:      "worker-a",
				WorkstationName: "classifier",
				InputTokens:     InputTokens(factoryruntime.RuntimeToken{ID: "tok-1"}),
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.Outcome != workerexecution.OutcomeFailed {
				t.Fatalf("Outcome = %s, want %s", result.Outcome, workerexecution.OutcomeFailed)
			}
			if !strings.HasPrefix(result.Error, "classifier output invalid:") {
				t.Fatalf("Error = %q, want classifier output invalid prefix", result.Error)
			}
			if strings.TrimSpace(tc.output) != "" && !strings.Contains(result.Error, "raw output:") {
				t.Fatalf("Error = %q, want raw output evidence", result.Error)
			}
		})
	}
}

func TestWorkstationExecutor_PromptRenderFailure_ReturnsFailedResult(t *testing.T) {
	mock := &wsMockExecutor{}
	we := newTestWorkstationExecutor(
		staticRuntimeConfig{
			Workers: map[string]*interfaces.FactoryWorkerConfig{
				"worker-a": {Body: "system"},
			},
			Workstations: map[string]*interfaces.FactoryWorkstationConfig{
				"broken": {Type: interfaces.WorkstationTypeModel, PromptTemplate: "{{ .InvalidSyntax"},
			},
		},
		mock,
	)

	result, err := we.Execute(context.Background(), work.WorkDispatch{
		DispatchID:      "d-prompt-failure",
		TransitionID:    "t-prompt-failure",
		WorkerType:      "worker-a",
		WorkstationName: "broken",
		InputTokens:     InputTokens(factoryruntime.RuntimeToken{ID: "tok-1", Color: factoryruntime.RuntimeTokenColor{WorkID: "work-1"}}),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mock.called {
		t.Fatal("executor should not be called when prompt rendering fails")
	}
	if result.Outcome != workerexecution.OutcomeFailed {
		t.Fatalf("Outcome = %s, want %s", result.Outcome, workerexecution.OutcomeFailed)
	}
	if !strings.HasPrefix(result.Error, "prompt render failed:") {
		t.Fatalf("Error = %q, want prompt render failed prefix", result.Error)
	}
}

func TestWorkstationExecutor_ResolvesWorkerAndWorkstationPerDispatch(t *testing.T) {
	mock := &wsMockExecutor{result: workerexecution.WorkResult{Outcome: workerexecution.OutcomeAccepted, Output: "done"}}
	we := &WorkstationExecutor{
		Now: time.Now,
		RuntimeConfig: staticRuntimeConfig{
			Workers: map[string]*interfaces.FactoryWorkerConfig{
				"worker-a": {Body: "system-a"},
				"worker-b": {Body: "system-b"},
			},
			Workstations: map[string]*interfaces.FactoryWorkstationConfig{
				"review-a": {Type: interfaces.WorkstationTypeModel, PromptTemplate: "Review {{ (index .Inputs 0).WorkID }}"},
				"review-b": {Type: interfaces.WorkstationTypeModel, PromptTemplate: "Inspect {{ (index .Inputs 0).WorkID }}"},
			},
		},
		Executor: mock,
		Renderer: &DefaultPromptRenderer{},
	}

	first, err := we.Execute(context.Background(), work.WorkDispatch{
		DispatchID:      "d-1",
		TransitionID:    "t-1",
		WorkerType:      "worker-a",
		WorkstationName: "review-a",
		InputTokens:     InputTokens(factoryruntime.RuntimeToken{ID: "tok-1", Color: factoryruntime.RuntimeTokenColor{WorkID: "work-1"}}),
	})
	if err != nil {
		t.Fatalf("first execute error: %v", err)
	}
	if first.Outcome != workerexecution.OutcomeAccepted {
		t.Fatalf("first outcome = %s, want %s", first.Outcome, workerexecution.OutcomeAccepted)
	}
	if got := mock.dispatch.SystemPrompt; got != "system-a" {
		t.Fatalf("first system prompt = %q", got)
	}
	if got := mock.dispatch.UserMessage; got != "Review work-1" {
		t.Fatalf("first user message = %q", got)
	}

	second, err := we.Execute(context.Background(), work.WorkDispatch{
		DispatchID:      "d-2",
		TransitionID:    "t-2",
		WorkerType:      "worker-b",
		WorkstationName: "review-b",
		InputTokens:     InputTokens(factoryruntime.RuntimeToken{ID: "tok-2", Color: factoryruntime.RuntimeTokenColor{WorkID: "work-2"}}),
	})
	if err != nil {
		t.Fatalf("second execute error: %v", err)
	}
	if second.Outcome != workerexecution.OutcomeAccepted {
		t.Fatalf("second outcome = %s, want %s", second.Outcome, workerexecution.OutcomeAccepted)
	}
	if got := mock.dispatch.SystemPrompt; got != "system-b" {
		t.Fatalf("second system prompt = %q", got)
	}
	if got := mock.dispatch.UserMessage; got != "Inspect work-2" {
		t.Fatalf("second user message = %q", got)
	}
}
