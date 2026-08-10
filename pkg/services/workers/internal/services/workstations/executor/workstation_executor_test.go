// backendsizecheck:ignore-file focused workstation execution tests stay together until the model-binding seam gets a dedicated package-level test split.
// pkgmaintcheck:ignore-file-lines focused workstation execution tests stay together until the model-binding seam gets a dedicated package-level test split.
package executor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
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
				RunnerID: workerexecution.RunnerIDCodex,
				Source:   workerexecution.RunnerSelectionSourceWorkstation,
			}, nil
		},
	}

	selection, err := executor.resolveRunnerSelection("agent", "codex")
	if err != nil {
		t.Fatalf("resolveRunnerSelection() error = %v", err)
	}
	if selection.RunnerID != workerexecution.RunnerIDCodex {
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

func TestWorkstationExecutorPreservesExecutorProviderInDetachedRequest(t *testing.T) {
	t.Parallel()
	runtimeConfig := staticRuntimeConfig{
		Workers: map[string]*interfaces.FactoryWorkerConfig{
			"worker-a": {
				Type:             interfaces.WorkerTypeModel,
				ExecutorProvider: "cursor-acp",
			},
		},
		Workstations: map[string]*interfaces.FactoryWorkstationConfig{
			"standard": {Type: interfaces.WorkstationTypeModel, PromptTemplate: "run"},
		},
	}
	capture := &wsMockExecutor{result: workerexecution.WorkResult{Outcome: workerexecution.OutcomeAccepted}}
	executor := newTestWorkstationExecutor(runtimeConfig, capture)

	_, err := executor.Execute(context.Background(), work.WorkDispatch{
		DispatchID:      "dispatch-acp",
		TransitionID:    "transition-acp",
		WorkerType:      "worker-a",
		WorkstationName: "standard",
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !capture.called {
		t.Fatal("detached workstation executor was not called")
	}
	if got := capture.dispatch.ExecutorProvider; got != "cursor-acp" {
		t.Fatalf("ExecutorProvider = %q, want cursor-acp", got)
	}
}

func TestWorkstationExecutorCarriesCanonicalLegacyProviderThroughInference(t *testing.T) {
	t.Parallel()

	providers := providerRegistryWithExternalFixture(t)

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
	if got != "cursor" {
		t.Fatalf("modelProviderForExecution(cursor) = %q, want cursor", got)
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

type expectedArtifactTestCase struct {
	name          string
	workType      []interfaces.ExpectedArtifactConfig
	workstation   []interfaces.ExpectedArtifactConfig
	wantOutcome   workerexecution.WorkOutcome
	wantReasons   []workerexecution.ExpectedArtifactVerificationReason
	wantPatterns  []string
	wantErrorPart string
	writeEmpty    bool
	unsafeInput   bool
}

func TestWorkstationExecutor_VerifiesExpectedArtifactsAfterWorkerSuccess(t *testing.T) {
	t.Parallel()
	workspace := t.TempDir()
	writeExpectedArtifactFixtures(t, workspace)
	for _, testCase := range expectedArtifactTestCases() {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			runExpectedArtifactTestCase(t, workspace, testCase)
		})
	}
}

func writeExpectedArtifactFixtures(t *testing.T, workspace string) {
	t.Helper()
	if err := os.Mkdir(filepath.Join(workspace, "reports"), 0o755); err != nil {
		t.Fatalf("mkdir reports: %v", err)
	}
	fixtures := map[string]string{
		"literal.txt":                        "done",
		filepath.Join("reports", "one.json"): "{}",
		filepath.Join("reports", "two.json"): "{}",
		"task-report.json":                   "report",
	}
	for relativePath, contents := range fixtures {
		if err := os.WriteFile(filepath.Join(workspace, relativePath), []byte(contents), 0o644); err != nil {
			t.Fatalf("write artifact %q: %v", relativePath, err)
		}
	}
}

func expectedArtifactTestCases() []expectedArtifactTestCase {
	return []expectedArtifactTestCase{
		{name: "no declarations leaves success unchanged", wantOutcome: workerexecution.OutcomeAccepted},
		{
			name:        "literal and glob success",
			workType:    []interfaces.ExpectedArtifactConfig{{Name: "literal", Pattern: "literal.txt", NonEmpty: true}},
			workstation: []interfaces.ExpectedArtifactConfig{{Name: "reports", Pattern: "reports/*.json", NonEmpty: true}},
			wantOutcome: workerexecution.OutcomeAccepted,
		},
		{
			name:        "name template success",
			workstation: []interfaces.ExpectedArtifactConfig{{Name: "named report", Pattern: "{{ (index .Inputs 0).Name }}.json", NonEmpty: true}},
			wantOutcome: workerexecution.OutcomeAccepted,
		},
		{
			name: "multiple missing and empty are stable",
			workType: []interfaces.ExpectedArtifactConfig{
				{Name: "missing-z", Pattern: "missing/z.txt"},
				{Name: "empty-work", Pattern: "empty-work.txt", NonEmpty: true},
			},
			workstation:   []interfaces.ExpectedArtifactConfig{{Name: "missing-a", Pattern: "missing/a.txt"}},
			wantOutcome:   workerexecution.OutcomeFailed,
			wantReasons:   []workerexecution.ExpectedArtifactVerificationReason{workerexecution.ExpectedArtifactVerificationReasonMissing, workerexecution.ExpectedArtifactVerificationReasonEmpty, workerexecution.ExpectedArtifactVerificationReasonMissing},
			wantPatterns:  []string{"missing/z.txt", "empty-work.txt", "missing/a.txt"},
			wantErrorPart: "EXPECTED_ARTIFACTS_UNSATISFIED",
			writeEmpty:    true,
		},
		{
			name:          "unsafe rendered input is redacted",
			workstation:   []interfaces.ExpectedArtifactConfig{{Name: "unsafe", Pattern: "{{ (index .Inputs 0).Name }}.txt"}},
			wantOutcome:   workerexecution.OutcomeFailed,
			wantReasons:   []workerexecution.ExpectedArtifactVerificationReason{workerexecution.ExpectedArtifactVerificationReasonMissing},
			wantPatterns:  []string{"<invalid>"},
			wantErrorPart: "<invalid>",
			unsafeInput:   true,
		},
	}
}

func runExpectedArtifactTestCase(t *testing.T, workspace string, testCase expectedArtifactTestCase) {
	t.Helper()
	if testCase.writeEmpty {
		if err := os.WriteFile(filepath.Join(workspace, "empty-work.txt"), nil, 0o644); err != nil {
			t.Fatalf("write empty artifact: %v", err)
		}
	}
	mock := &wsMockExecutor{result: workerexecution.WorkResult{Outcome: workerexecution.OutcomeAccepted, Output: "worker output"}}
	we := newTestWorkstationExecutor(expectedArtifactRuntimeConfig(workspace, testCase), mock)
	inputName := "task-report"
	if testCase.unsafeInput {
		inputName = filepath.Join(workspace, "secret")
	}
	result, err := we.Execute(context.Background(), expectedArtifactDispatch(testCase.name, inputName))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	assertExpectedArtifactResult(t, result, mock, testCase, workspace)
}

func expectedArtifactRuntimeConfig(workspace string, testCase expectedArtifactTestCase) staticRuntimeConfig {
	return staticRuntimeConfig{
		Factory: &interfaces.FactoryConfig{
			WorkTypes: []interfaces.WorkTypeConfig{{ID: "task", Name: "task", ExpectedArtifacts: testCase.workType}},
		},
		Workers: map[string]*interfaces.FactoryWorkerConfig{
			"worker-a": {Type: interfaces.WorkerTypeModel},
		},
		Workstations: map[string]*interfaces.FactoryWorkstationConfig{
			"standard": {Type: interfaces.WorkstationTypeModel, ExpectedArtifacts: testCase.workstation},
		},
		FactoryPath: workspace,
	}
}

func expectedArtifactDispatch(name, inputName string) work.WorkDispatch {
	return work.WorkDispatch{
		DispatchID:      "artifact-" + strings.ReplaceAll(name, " ", "-"),
		TransitionID:    "transition-artifact",
		WorkerType:      "worker-a",
		WorkstationName: "standard",
		InputTokens: InputTokens(factoryruntime.RuntimeToken{
			ID: "token-artifact",
			Color: factoryruntime.RuntimeTokenColor{
				Name:       inputName,
				WorkID:     "work-artifact",
				WorkTypeID: "task",
				DataType:   factoryruntime.RuntimeTokenDataTypeWork,
			},
		}),
	}
}

func assertExpectedArtifactResult(t *testing.T, result workerexecution.WorkResult, mock *wsMockExecutor, testCase expectedArtifactTestCase, workspace string) {
	t.Helper()
	if result.Outcome != testCase.wantOutcome {
		t.Fatalf("Outcome = %s, want %s; result = %#v", result.Outcome, testCase.wantOutcome, result)
	}
	if !mock.called {
		t.Fatal("worker executor was not called")
	}
	if result.Output != "worker output" {
		t.Fatalf("Output = %q, want preserved worker output", result.Output)
	}
	if testCase.wantOutcome == workerexecution.OutcomeAccepted {
		assertExpectedArtifactSuccess(t, result)
		return
	}
	assertExpectedArtifactFailure(t, result, testCase, workspace)
}

func assertExpectedArtifactSuccess(t *testing.T, result workerexecution.WorkResult) {
	t.Helper()
	if result.ArtifactVerification != nil || result.FailureMetadata != nil {
		t.Fatalf("successful result carries verification failure: %#v", result)
	}
}

func assertExpectedArtifactFailure(t *testing.T, result workerexecution.WorkResult, testCase expectedArtifactTestCase, workspace string) {
	t.Helper()
	verification := result.ArtifactVerification
	if verification == nil || verification.Code != workerexecution.WorkFailureTypeExpectedArtifactsUnsatisfied {
		t.Fatalf("verification = %#v, want stable code", verification)
	}
	if len(verification.Entries) != len(testCase.wantReasons) {
		t.Fatalf("verification entries = %#v, want %d entries", verification.Entries, len(testCase.wantReasons))
	}
	assertExpectedArtifactEntries(t, verification.Entries, testCase, workspace)
	if result.FailureMetadata == nil || result.FailureMetadata.Type != workerexecution.WorkFailureTypeExpectedArtifactsUnsatisfied {
		t.Fatalf("FailureMetadata = %#v, want expected-artifact terminal type", result.FailureMetadata)
	}
	if !strings.Contains(result.Error, testCase.wantErrorPart) || strings.Contains(result.Error, workspace) {
		t.Fatalf("Error = %q, want safe stable summary containing %q", result.Error, testCase.wantErrorPart)
	}
}

func assertExpectedArtifactEntries(t *testing.T, entries []workerexecution.ExpectedArtifactVerificationEntry, testCase expectedArtifactTestCase, workspace string) {
	t.Helper()
	for index, entry := range entries {
		if entry.Reason != testCase.wantReasons[index] || entry.Pattern != testCase.wantPatterns[index] {
			t.Fatalf("verification entry %d = %#v, want reason=%s pattern=%q", index, entry, testCase.wantReasons[index], testCase.wantPatterns[index])
		}
		if strings.Contains(entry.Pattern, workspace) {
			t.Fatalf("verification entry %d leaked workspace %q: %#v", index, workspace, entry)
		}
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

func TestWorkstationExecutor_ModelWorkstation_InterpolatesOmittedInvocationArguments(t *testing.T) {
	mock := &wsMockExecutor{result: workerexecution.WorkResult{Outcome: workerexecution.OutcomeAccepted}}
	we := newTestWorkstationExecutor(
		staticRuntimeConfig{
			Workers: map[string]*interfaces.FactoryWorkerConfig{
				"worker-a": {
					Type: interfaces.WorkerTypeModel, ModelProvider: "${provider}", Model: "${model}",
					RuntimeDefaultModelProvider: "codex", RuntimeDefaultModel: "operator-model",
				},
			},
			Workstations: map[string]*interfaces.FactoryWorkstationConfig{
				"standard": {Type: interfaces.WorkstationTypeModel, PromptTemplate: "Process work"},
			},
		},
		mock,
	)
	workerInterpolated := false
	we.Interpolation = factorydefinitionfixtures.InvocationInterpolation{
		InterpolateWorker: func(
			worker interfaces.FactoryWorkerConfig,
			args *work.InvocationArguments,
			_ interfaces.FileReader,
		) (interfaces.FactoryWorkerConfig, error) {
			if args != nil {
				t.Fatalf("invocation arguments = %#v, want nil for omitted optional arguments", args)
			}
			workerInterpolated = true
			worker.ModelProvider = ""
			worker.Model = ""
			return worker, nil
		},
	}

	result, err := we.Execute(context.Background(), work.WorkDispatch{
		DispatchID: "d-omitted", TransitionID: "t-omitted", WorkerType: "worker-a", WorkstationName: "standard",
		InputTokens: InputTokens(factoryruntime.RuntimeToken{ID: "tok-1", Color: factoryruntime.RuntimeTokenColor{WorkID: "work-1"}}),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !workerInterpolated || !mock.called || result.Outcome != workerexecution.OutcomeAccepted {
		t.Fatalf("interpolated=%t called=%t result=%#v", workerInterpolated, mock.called, result)
	}
	if mock.dispatch.ModelProvider != "codex" || mock.dispatch.Model != "operator-model" {
		t.Fatalf("provider/model = %q/%q, want operator fallback", mock.dispatch.ModelProvider, mock.dispatch.Model)
	}
}

func TestWorkstationExecutor_ScriptWorkerUsesInvocationInterpolatedArguments(t *testing.T) {
	mock := &interpolatedScriptExecutorMock{}
	we := newTestWorkstationExecutor(staticRuntimeConfig{
		Workers: map[string]*interfaces.FactoryWorkerConfig{
			"script-a": {Type: interfaces.WorkerTypeScript, Command: "tool", Args: []string{"${branch}"}},
		},
		Workstations: map[string]*interfaces.FactoryWorkstationConfig{
			"script-run": {Type: interfaces.WorkstationTypeModel, WorkerTypeName: "script-a"},
		},
	}, mock)
	we.Interpolation = factorydefinitionfixtures.InvocationInterpolation{
		InterpolateWorker: func(worker interfaces.FactoryWorkerConfig, _ *work.InvocationArguments, _ interfaces.FileReader) (interfaces.FactoryWorkerConfig, error) {
			worker.Args = []string{"feature/customer-branch"}
			return worker, nil
		},
	}

	result, err := we.Execute(context.Background(), work.WorkDispatch{
		DispatchID: "d-script", TransitionID: "t-script", WorkerType: "script-a", WorkstationName: "script-run",
		InputTokens: InputTokens(factoryruntime.RuntimeToken{ID: "tok-script", Color: factoryruntime.RuntimeTokenColor{
			WorkID: "work-script", InvocationArguments: &work.InvocationArguments{},
		}}),
	})
	if err != nil || result.Outcome != workerexecution.OutcomeAccepted {
		t.Fatalf("Execute() = %#v, %v", result, err)
	}
	if !mock.interpolatedCalled || mock.plainCalled || strings.Join(mock.worker.Args, ",") != "feature/customer-branch" {
		t.Fatalf("script executor = %#v", mock)
	}
}

type interpolatedScriptExecutorMock struct {
	plainCalled        bool
	interpolatedCalled bool
	worker             *interfaces.FactoryWorkerConfig
}

func (mock *interpolatedScriptExecutorMock) Execute(context.Context, workerexecution.WorkstationExecutionRequest) (workerexecution.WorkResult, error) {
	mock.plainCalled = true
	return workerexecution.WorkResult{Outcome: workerexecution.OutcomeAccepted}, nil
}

func (mock *interpolatedScriptExecutorMock) ExecuteWithWorker(_ context.Context, _ workerexecution.WorkstationExecutionRequest, worker *interfaces.FactoryWorkerConfig) (workerexecution.WorkResult, error) {
	mock.interpolatedCalled = true
	clone := *worker
	clone.Args = append([]string(nil), worker.Args...)
	mock.worker = &clone
	return workerexecution.WorkResult{Outcome: workerexecution.OutcomeAccepted}, nil
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

func providerRegistryWithExternalFixture(t *testing.T) *selectionTestCatalog {
	t.Helper()
	return &selectionTestCatalog{}
}

type selectionTestCatalog struct{}

func (*selectionTestCatalog) CanonicalIdentity(identity string) (string, error) {
	normalized := strings.TrimSpace(identity)
	if normalized == "" || normalized != strings.ToLower(normalized) || strings.Contains(normalized, "_") {
		return "", fmt.Errorf("provider %q is invalid", identity)
	}
	aliases := map[string]string{
		"openai": "codex", "anthropic": "claude", "agent": "cursor",
		"kiro-cli": "kiro", "customer": "customer.provider",
	}
	if canonical, ok := aliases[normalized]; ok {
		return canonical, nil
	}
	known := map[string]bool{
		"agy": true, "claude": true, "codex": true, "cursor": true,
		"gemini": true, "kiro": true, "opencode": true, "pi": true,
		"customer.provider": true,
	}
	if !known[normalized] {
		return "", fmt.Errorf("provider %q is unknown", identity)
	}
	return normalized, nil
}

func (catalog *selectionTestCatalog) ResolveRunnerSelection(workstation, factory, model string) (workerexecution.ResolvedRunnerSelection, error) {
	selection := workerexecution.ResolveRunnerSelection(workstation, factory, model)
	if workstation == "" && factory == "" && strings.TrimSpace(model) != "" {
		canonical, err := catalog.CanonicalIdentity(model)
		if err != nil {
			return workerexecution.ResolvedRunnerSelection{}, err
		}
		selection.RunnerID = canonical
		selection.Source = workerexecution.RunnerSelectionSourceLegacyProvider
	}
	return selection, nil
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

func TestWorkstationExecutor_RunReasoningEffortOverrideReachesExecutionRequest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                string
		workerEffort        string
		runEffort           string
		wantReasoningEffort string
	}{
		{name: "omitted preserves authored effort", workerEffort: "medium", wantReasoningEffort: "medium"},
		{name: "run override wins", workerEffort: "low", runEffort: "xhigh", wantReasoningEffort: "xhigh"},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			mock := &dispatchCapturingExecutor{result: workerexecution.WorkResult{Outcome: workerexecution.OutcomeAccepted}}
			we := newTestWorkstationExecutor(staticRuntimeConfig{
				Workers: map[string]*interfaces.FactoryWorkerConfig{
					"worker-a": {Type: interfaces.WorkerTypeModel, Body: "system", ReasoningEffort: tc.workerEffort},
				},
				Workstations: map[string]*interfaces.FactoryWorkstationConfig{
					"standard": {Type: interfaces.WorkstationTypeModel, PromptTemplate: "do work"},
				},
			}, mock)
			we.RunReasoningEffort = tc.runEffort

			result, err := we.Execute(context.Background(), work.WorkDispatch{
				DispatchID: "d-reasoning", TransitionID: "t-reasoning", WorkerType: "worker-a", WorkstationName: "standard",
			})
			if err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			if result.Outcome != workerexecution.OutcomeAccepted {
				t.Fatalf("outcome = %s, want accepted", result.Outcome)
			}
			if mock.dispatch.ReasoningEffort != tc.wantReasoningEffort {
				t.Fatalf("execution request reasoning effort = %q, want %q", mock.dispatch.ReasoningEffort, tc.wantReasoningEffort)
			}
		})
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

func TestWorkstationExecutor_ScriptClassifierUsesFinalStdoutLineAndPreservesDiagnostics(t *testing.T) {
	stdout := "checking payload\r\n\t needs_review \t\r\n\r\n"
	stderr := "script diagnostic\n"
	mock := &wsMockExecutor{result: workerexecution.WorkResult{
		Outcome: workerexecution.OutcomeAccepted,
		// Script runners trim the result content, while command diagnostics
		// retain the exact streams for inspection.
		Output: strings.TrimSpace(stdout),
		Diagnostics: &workerexecution.WorkDiagnostics{Command: &workerexecution.CommandDiagnostic{
			Stdout: stdout,
			Stderr: stderr,
		}},
	}}
	we := newTestWorkstationExecutor(
		staticRuntimeConfig{
			Workers: map[string]*interfaces.FactoryWorkerConfig{
				"script-worker": {Type: interfaces.WorkerTypeScript, Body: "script"},
			},
			Workstations: map[string]*interfaces.FactoryWorkstationConfig{
				"classifier": {Type: interfaces.WorkstationTypeClassify, PromptTemplate: "classify"},
			},
		},
		mock,
	)

	result, err := we.Execute(context.Background(), work.WorkDispatch{
		DispatchID:      "d-script-classifier-line",
		TransitionID:    "t-script-classifier-line",
		WorkerType:      "script-worker",
		WorkstationName: "classifier",
		InputTokens:     InputTokens(factoryruntime.RuntimeToken{ID: "tok-1"}),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Outcome != workerexecution.OutcomeAccepted || result.Output != "needs_review" {
		t.Fatalf("result = %#v, want accepted needs_review label", result)
	}
	if result.SelectedClassificationLabel != "" {
		t.Fatalf("selected classification label = %q, want empty before route matching", result.SelectedClassificationLabel)
	}
	if result.Diagnostics == nil || result.Diagnostics.Command == nil {
		t.Fatalf("diagnostics = %#v, want command diagnostics", result.Diagnostics)
	}
	if result.Diagnostics.Command.Stdout != stdout || result.Diagnostics.Command.Stderr != stderr {
		t.Fatalf("command diagnostics = %#v, want unmodified stdout/stderr", result.Diagnostics.Command)
	}
}

func TestWorkstationExecutor_ScriptClassifierRejectsWhitespaceOnlyStdoutAndPreservesDiagnostics(t *testing.T) {
	stdout := " \r\n\t  \n"
	stderr := "script warning"
	mock := &wsMockExecutor{result: workerexecution.WorkResult{
		Outcome: workerexecution.OutcomeAccepted,
		Output:  "",
		Diagnostics: &workerexecution.WorkDiagnostics{Command: &workerexecution.CommandDiagnostic{
			Stdout: stdout,
			Stderr: stderr,
		}},
	}}
	we := newTestWorkstationExecutor(
		staticRuntimeConfig{
			Workers: map[string]*interfaces.FactoryWorkerConfig{
				"script-worker": {Type: interfaces.WorkerTypeScript, Body: "script"},
			},
			Workstations: map[string]*interfaces.FactoryWorkstationConfig{
				"classifier": {Type: interfaces.WorkstationTypeClassify, PromptTemplate: "classify"},
			},
		},
		mock,
	)

	result, err := we.Execute(context.Background(), work.WorkDispatch{
		DispatchID:      "d-script-classifier-empty",
		TransitionID:    "t-script-classifier-empty",
		WorkerType:      "script-worker",
		WorkstationName: "classifier",
		InputTokens:     InputTokens(factoryruntime.RuntimeToken{ID: "tok-1"}),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Outcome != workerexecution.OutcomeFailed {
		t.Fatalf("Outcome = %s, want %s", result.Outcome, workerexecution.OutcomeFailed)
	}
	if result.Error != "classifier output invalid: empty label" {
		t.Fatalf("Error = %q, want actionable missing-label diagnostic", result.Error)
	}
	if result.SelectedClassificationLabel != "" {
		t.Fatalf("selected classification label = %q, want empty on failure", result.SelectedClassificationLabel)
	}
	if result.Diagnostics == nil || result.Diagnostics.Command == nil ||
		result.Diagnostics.Command.Stdout != stdout || result.Diagnostics.Command.Stderr != stderr {
		t.Fatalf("command diagnostics = %#v, want unmodified stdout/stderr", result.Diagnostics)
	}
}

func TestWorkstationExecutor_InferenceClassifierRetainsWholeOutputInterpretation(t *testing.T) {
	output := "provider diagnostic\nneeds_review"
	mock := &wsMockExecutor{result: workerexecution.WorkResult{Outcome: workerexecution.OutcomeAccepted, Output: output}}
	we := newTestWorkstationExecutor(
		staticRuntimeConfig{
			Workers: map[string]*interfaces.FactoryWorkerConfig{
				"model-worker": {Type: interfaces.WorkerTypeModel, Body: "model"},
			},
			Workstations: map[string]*interfaces.FactoryWorkstationConfig{
				"classifier": {Type: interfaces.WorkstationTypeClassify, PromptTemplate: "classify"},
			},
		},
		mock,
	)

	result, err := we.Execute(context.Background(), work.WorkDispatch{
		DispatchID:      "d-inference-classifier-whole-output",
		TransitionID:    "t-inference-classifier-whole-output",
		WorkerType:      "model-worker",
		WorkstationName: "classifier",
		InputTokens:     InputTokens(factoryruntime.RuntimeToken{ID: "tok-1"}),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Outcome != workerexecution.OutcomeAccepted || result.Output != output {
		t.Fatalf("result = %#v, want accepted whole inference output", result)
	}
}

func TestWorkstationExecutor_NonClassifierScriptRetainsWholeOutputInterpretation(t *testing.T) {
	output := "script diagnostic\nscript result"
	mock := &wsMockExecutor{result: workerexecution.WorkResult{Outcome: workerexecution.OutcomeAccepted, Output: output}}
	we := newTestWorkstationExecutor(
		staticRuntimeConfig{
			Workers: map[string]*interfaces.FactoryWorkerConfig{
				"script-worker": {Type: interfaces.WorkerTypeScript, Body: "script"},
			},
			Workstations: map[string]*interfaces.FactoryWorkstationConfig{
				"standard": {Type: interfaces.WorkstationTypeModel, PromptTemplate: "run"},
			},
		},
		mock,
	)

	result, err := we.Execute(context.Background(), work.WorkDispatch{
		DispatchID:      "d-script-whole-output",
		TransitionID:    "t-script-whole-output",
		WorkerType:      "script-worker",
		WorkstationName: "standard",
		InputTokens:     InputTokens(factoryruntime.RuntimeToken{ID: "tok-1"}),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Outcome != workerexecution.OutcomeAccepted || result.Output != output {
		t.Fatalf("result = %#v, want accepted whole script output", result)
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

type promptReloadExecutor struct {
	calls     []workerexecution.WorkstationExecutionRequest
	onExecute func(workerexecution.WorkstationExecutionRequest)
}

func (e *promptReloadExecutor) Execute(
	_ context.Context,
	request workerexecution.WorkstationExecutionRequest,
) (workerexecution.WorkResult, error) {
	if e.onExecute != nil {
		e.onExecute(request)
	}
	e.calls = append(e.calls, workerexecution.CloneWorkstationExecutionRequest(request))
	return workerexecution.WorkResult{Outcome: workerexecution.OutcomeAccepted}, nil
}

func TestWorkstationExecutorReloadsFileBackedPromptsPerDispatch(t *testing.T) {
	workerPath := "factory/workers/worker-a/AGENTS.md"
	workstationPath := "factory/workstations/review/AGENTS.md"
	files := &workstationFileSystemStub{files: map[string][]byte{
		workerPath:      []byte("---\ntype: MODEL\n---\nworker old"),
		workstationPath: []byte("---\ntype: MODEL\n---\nworkstation old"),
	}}
	runtimeConfig := staticRuntimeConfig{
		Workers: map[string]*interfaces.FactoryWorkerConfig{
			"worker-a": {
				Type:             interfaces.WorkerTypeModel,
				Body:             "cached worker",
				PromptSourcePath: workerPath,
			},
		},
		Workstations: map[string]*interfaces.FactoryWorkstationConfig{
			"review": {
				Type:             interfaces.WorkstationTypeModel,
				PromptTemplate:   "cached workstation",
				PromptSourcePath: workstationPath,
			},
		},
	}
	capture := &promptReloadExecutor{}
	we := newTestWorkstationExecutor(runtimeConfig, capture)
	we.FileSystem = files

	editAfterSnapshot := true
	capture.onExecute = func(request workerexecution.WorkstationExecutionRequest) {
		if !editAfterSnapshot || request.Dispatch.DispatchID != "dispatch-1" {
			return
		}
		files.files[workerPath] = []byte("---\ntype: MODEL\n---\nworker new")
		files.files[workstationPath] = []byte("---\ntype: MODEL\n---\nworkstation new")
		editAfterSnapshot = false
		if request.SystemPrompt != "worker old" || request.UserMessage != "workstation old" {
			t.Errorf("first request changed after source edit: system=%q user=%q", request.SystemPrompt, request.UserMessage)
		}
	}

	for _, dispatchID := range []string{"dispatch-1", "dispatch-2"} {
		result, err := we.Execute(context.Background(), work.WorkDispatch{
			DispatchID:      dispatchID,
			TransitionID:    "transition-" + dispatchID,
			WorkerType:      "worker-a",
			WorkstationName: "review",
		})
		if err != nil {
			t.Fatalf("Execute(%s) error = %v", dispatchID, err)
		}
		if result.Outcome != workerexecution.OutcomeAccepted {
			t.Fatalf("Execute(%s) outcome = %s, want accepted", dispatchID, result.Outcome)
		}
	}

	if len(capture.calls) != 2 {
		t.Fatalf("provider-bound calls = %d, want 2", len(capture.calls))
	}
	if got := capture.calls[0].SystemPrompt; got != "worker old" {
		t.Fatalf("first system prompt = %q, want worker old", got)
	}
	if got := capture.calls[0].UserMessage; got != "workstation old" {
		t.Fatalf("first user message = %q, want workstation old", got)
	}
	if got := capture.calls[1].SystemPrompt; got != "worker new" {
		t.Fatalf("second system prompt = %q, want worker new", got)
	}
	if got := capture.calls[1].UserMessage; got != "workstation new" {
		t.Fatalf("second user message = %q, want workstation new", got)
	}
	if runtimeConfig.Workers["worker-a"].Body != "cached worker" {
		t.Fatalf("runtime worker body mutated to %q", runtimeConfig.Workers["worker-a"].Body)
	}
	if runtimeConfig.Workstations["review"].PromptTemplate != "cached workstation" {
		t.Fatalf("runtime workstation prompt mutated to %q", runtimeConfig.Workstations["review"].PromptTemplate)
	}
}

func TestWorkstationExecutorKeepsInlinePromptsImmutable(t *testing.T) {
	files := &workstationFileSystemStub{files: map[string][]byte{}}
	capture := &promptReloadExecutor{}
	we := newTestWorkstationExecutor(staticRuntimeConfig{
		Workers: map[string]*interfaces.FactoryWorkerConfig{
			"worker-a": {Type: interfaces.WorkerTypeModel, Body: "inline worker"},
		},
		Workstations: map[string]*interfaces.FactoryWorkstationConfig{
			"review": {Type: interfaces.WorkstationTypeModel, PromptTemplate: "inline workstation"},
		},
	}, capture)
	we.FileSystem = files

	result, err := we.Execute(context.Background(), work.WorkDispatch{
		DispatchID:      "dispatch-inline",
		TransitionID:    "transition-inline",
		WorkerType:      "worker-a",
		WorkstationName: "review",
	})
	if err != nil || result.Outcome != workerexecution.OutcomeAccepted {
		t.Fatalf("Execute() = (%#v, %v), want accepted", result, err)
	}
	if len(files.reads) != 0 {
		t.Fatalf("inline prompt reads = %#v, want none", files.reads)
	}
	if len(capture.calls) != 1 || capture.calls[0].SystemPrompt != "inline worker" || capture.calls[0].UserMessage != "inline workstation" {
		t.Fatalf("inline request = %#v", capture.calls)
	}
}

func TestWorkstationExecutorFailsPromptSourceWithoutProviderCallAndRecovers(t *testing.T) {
	tests := []struct {
		name        string
		role        string
		path        string
		worker      *interfaces.FactoryWorkerConfig
		workstation *interfaces.FactoryWorkstationConfig
		repair      string
	}{
		{
			name: "worker",
			role: "worker",
			path: "factory/workers/worker-a/AGENTS.md",
			worker: &interfaces.FactoryWorkerConfig{
				Type:             interfaces.WorkerTypeModel,
				Body:             "stale worker",
				PromptSourcePath: "factory/workers/worker-a/AGENTS.md",
			},
			workstation: &interfaces.FactoryWorkstationConfig{
				Type:           interfaces.WorkstationTypeModel,
				PromptTemplate: "inline workstation",
			},
			repair: "---\ntype: MODEL\n---\nrepaired worker",
		},
		{
			name: "workstation",
			role: "workstation",
			path: "factory/workstations/review/prompt.md",
			worker: &interfaces.FactoryWorkerConfig{
				Type: interfaces.WorkerTypeModel,
				Body: "inline worker",
			},
			workstation: &interfaces.FactoryWorkstationConfig{
				Type:                   interfaces.WorkstationTypeModel,
				PromptTemplate:         "stale workstation",
				PromptSourcePath:       "factory/workstations/review/prompt.md",
				PromptSourceIsTemplate: true,
			},
			repair: "repaired workstation",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			files := &workstationFileSystemStub{files: map[string][]byte{}}
			capture := &promptReloadExecutor{}
			we := newTestWorkstationExecutor(staticRuntimeConfig{
				Workers:      map[string]*interfaces.FactoryWorkerConfig{"worker-a": test.worker},
				Workstations: map[string]*interfaces.FactoryWorkstationConfig{"review": test.workstation},
			}, capture)
			we.FileSystem = files

			result, err := we.Execute(context.Background(), work.WorkDispatch{
				DispatchID:      "dispatch-missing",
				TransitionID:    "transition-missing",
				WorkerType:      "worker-a",
				WorkstationName: "review",
			})
			if err != nil {
				t.Fatalf("missing source Execute() error = %v", err)
			}
			if result.Outcome != workerexecution.OutcomeFailed {
				t.Fatalf("missing source outcome = %s, want failed", result.Outcome)
			}
			if !strings.Contains(result.Error, test.role) || !strings.Contains(result.Error, test.path) {
				t.Fatalf("missing source error = %q, want role and path", result.Error)
			}
			if len(capture.calls) != 0 {
				t.Fatal("provider executor was called after prompt source failure")
			}

			files.files[test.path] = []byte(test.repair)
			result, err = we.Execute(context.Background(), work.WorkDispatch{
				DispatchID:      "dispatch-repaired",
				TransitionID:    "transition-repaired",
				WorkerType:      "worker-a",
				WorkstationName: "review",
			})
			if err != nil || result.Outcome != workerexecution.OutcomeAccepted {
				t.Fatalf("repaired Execute() = (%#v, %v), want accepted", result, err)
			}
			if len(capture.calls) != 1 {
				t.Fatalf("provider calls after repair = %d, want 1", len(capture.calls))
			}
		})
	}
}

var _ workerexecution.WorkstationRequestExecutor = (*promptReloadExecutor)(nil)

const validObservationReport = `## Inspection status
Inspected: yes

## Chronological events
- 00:00.000 — The subject enters frame.
- 00:02.000 — The subject turns toward the light.

## Temporal or transient defects
None observed.

## Audio content and defects
Audio content: noise
None observed.

## Observed speech
None observed.

## Overall recommendation
Recommendation: pass
`

const validClipQAVerdict = `{"action_completed":true,"spec_deviations":[],"temporal_artifacts":[],"audio_content":"noise","unexpected_speech":false,"verdict":"pass","confidence":0.95}`

func TestValidateMarkdownObservationReportAcceptsCompleteReport(t *testing.T) {
	if err := validateOutputContract(validObservationReport, outputContractMarkdownObservationReportV1); err != nil {
		t.Fatalf("validateOutputContract() error = %v, want complete report accepted", err)
	}
}

func TestValidateMarkdownObservationReportRejectsIncompleteAndRefusalOutput(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "prose without required sections",
			content: "The clip contains a woman and a ticking clock. The audio is noise.",
			want:    `missing required section "inspection status"`,
		},
		{
			name:    "provider refusal",
			content: "I could not inspect the file because it does not exist. Recommendation: pass",
			want:    `missing required section "inspection status"`,
		},
		{
			name:    "missing recommendation",
			content: strings.Replace(validObservationReport, "Recommendation: pass", "No recommendation available", 1),
			want:    "exactly one pass or reroll recommendation",
		},
		{
			name:    "duplicate recommendation",
			content: strings.Replace(validObservationReport, "Recommendation: pass", "Recommendation: pass\nRecommendation: reroll", 1),
			want:    "exactly one pass or reroll recommendation",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateOutputContract(test.content, outputContractMarkdownObservationReportV1)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("validateOutputContract() error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func TestValidateStructuredClipQAVerdictEnforcesBoundsAndPassInvariants(t *testing.T) {
	if err := validateOutputContract(validClipQAVerdict, outputContractStructuredClipQAVerdictV1); err != nil {
		t.Fatalf("validateOutputContract() error = %v, want valid verdict accepted", err)
	}

	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "confidence below zero",
			content: strings.Replace(validClipQAVerdict, `"confidence":0.95`, `"confidence":-0.01`, 1),
			want:    "between 0 and 1",
		},
		{
			name:    "confidence above one",
			content: strings.Replace(validClipQAVerdict, `"confidence":0.95`, `"confidence":1.01`, 1),
			want:    "between 0 and 1",
		},
		{
			name:    "pass with incomplete action",
			content: strings.Replace(validClipQAVerdict, `"action_completed":true`, `"action_completed":false`, 1),
			want:    "action_completed=true",
		},
		{
			name:    "pass with specification deviation",
			content: strings.Replace(validClipQAVerdict, `"spec_deviations":[]`, `"spec_deviations":["wrong action"]`, 1),
			want:    "spec_deviations to be empty",
		},
		{
			name:    "pass with temporal artifact",
			content: strings.Replace(validClipQAVerdict, `"temporal_artifacts":[]`, `"temporal_artifacts":["flash"]`, 1),
			want:    "temporal_artifacts to be empty",
		},
		{
			name:    "pass with unexpected speech",
			content: strings.Replace(validClipQAVerdict, `"unexpected_speech":false`, `"unexpected_speech":true`, 1),
			want:    "unexpected_speech=false",
		},
		{
			name:    "reroll without reason",
			content: strings.Replace(validClipQAVerdict, `"verdict":"pass"`, `"verdict":"reroll"`, 1),
			want:    "observed failure reason",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateOutputContract(test.content, outputContractStructuredClipQAVerdictV1)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("validateOutputContract() error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func TestValidateStructuredClipQAVerdictAcceptsInspectedReroll(t *testing.T) {
	content := strings.Replace(validClipQAVerdict, `"action_completed":true`, `"action_completed":false`, 1)
	content = strings.Replace(content, `"verdict":"pass"`, `"verdict":"reroll"`, 1)
	if err := validateOutputContract(content, outputContractStructuredClipQAVerdictV1); err != nil {
		t.Fatalf("validateOutputContract() error = %v, want inspected reroll accepted", err)
	}
}
