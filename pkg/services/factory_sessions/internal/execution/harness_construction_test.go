package factorysessionexecution_test

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil/checkpointfixtures"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessionexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution/runtimepersist"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/fileeffects"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

const finalWorkflow = `return { subject: args.subject };`

const childWorkflow = `return (async function () {
  const child = await agent.run({
    prompt: "summarize workflows",
    label: "summarize-findings",
    modelProvider: "codex",
  });
  return { child: child };
})();`

const busyLoopWorkflow = `while (true) {}`

type harnessMode string

const (
	harnessModeFake       harnessMode = "fake"
	harnessModeJavaScript harnessMode = "javascript-runtime"
)

// harnessConfig is deliberately test-only. It keeps these six owner scenarios
// compact without restoring an application composition surface.
type harnessConfig struct {
	Mode                harnessMode
	ProjectRoot         string
	Clock               factory.Clock
	InvocationExecutor  workerexecution.InvocationExecutor
	Persistence         runtimepersist.Store
	ChildExecutorMode   string
	CheckpointSummaries factory.JavaScriptCheckpointSummaries
	Workflows           factory.JavaScriptWorkflows
	RecordingWriter     recordings.PortableRecordingWriter
	FakeScenarios       []factorysessionexecution.FakeScenario
	FakeFixturePath     string
}

func newHarness(config harnessConfig) (factorysessionexecution.Service, error) {
	switch config.Mode {
	case harnessModeFake:
		if hasHarnessRuntimeDependencies(config) {
			return nil, fmt.Errorf("durable execution test harness: fake mode does not accept JavaScript runtime dependencies")
		}
		if config.Clock == nil {
			return nil, fmt.Errorf("durable execution test harness: clock is required for fake mode")
		}
		if strings.TrimSpace(config.FakeFixturePath) != "" {
			if len(config.FakeScenarios) != 0 {
				return nil, fmt.Errorf("durable execution test harness: fake mode accepts fixture path or options, not both")
			}
			service, err := factorysessionexecution.NewFakeServiceFromContractFixtures(
				strings.TrimSpace(config.FakeFixturePath),
				config.Clock,
				fileeffects.ContractFixtureReader(os.ReadFile),
			)
			if err != nil {
				return nil, fmt.Errorf("durable execution test harness: load fake fixtures: %w", err)
			}
			return service, nil
		}
		return factorysessionexecution.NewFakeService(config.Clock, config.FakeScenarios...)
	case harnessModeJavaScript:
		if err := validateHarnessJavaScriptConfig(config); err != nil {
			return nil, err
		}
		return factorysessionexecution.NewJavaScriptRuntimeService(
			strings.TrimSpace(config.ProjectRoot),
			config.ChildExecutorMode,
			config.InvocationExecutor,
			config.Persistence,
			config.Clock,
			harnessSyncWaitScheduler{},
			config.CheckpointSummaries,
			config.Workflows,
			orchestrationJavaScriptFromWorkflows(config.Workflows),
			config.Workflows,
			nil,
			factory.JavaScriptWorkerSettings{},
			config.RecordingWriter,
			func() string { return "00000000-0000-4000-8000-000000000001" },
			nil, nil, nil,
		), nil
	default:
		return nil, fmt.Errorf("durable execution test harness: unsupported mode %q", config.Mode)
	}
}

type harnessSyncWaitScheduler struct{}

func (harnessSyncWaitScheduler) Now() time.Time { return time.Now() }

func (harnessSyncWaitScheduler) After(duration time.Duration) <-chan time.Time {
	return time.After(duration)
}

func hasHarnessRuntimeDependencies(config harnessConfig) bool {
	return strings.TrimSpace(config.ProjectRoot) != "" || config.InvocationExecutor != nil ||
		config.Persistence != nil || config.Workflows != nil || strings.TrimSpace(config.ChildExecutorMode) != ""
}

func validateHarnessJavaScriptConfig(config harnessConfig) error {
	if strings.TrimSpace(config.ProjectRoot) == "" {
		return fmt.Errorf("durable execution test harness: project root is required for JavaScript mode")
	}
	if config.Clock == nil {
		return fmt.Errorf("durable execution test harness: clock is required for JavaScript mode")
	}
	if config.Persistence == nil {
		return fmt.Errorf("durable execution test harness: persistence is required for JavaScript mode")
	}
	if config.CheckpointSummaries == nil {
		return fmt.Errorf("durable execution test harness: checkpoint summaries are required for JavaScript mode")
	}
	if config.Workflows == nil {
		return fmt.Errorf("durable execution test harness: JavaScript workflows are required for JavaScript mode")
	}
	switch config.ChildExecutorMode {
	case factorysessionexecution.ChildExecutorModeFake:
		if config.InvocationExecutor != nil {
			return fmt.Errorf("durable execution test harness: invocation executor is only valid with live-provider child execution")
		}
	case factorysessionexecution.ChildExecutorModeLive:
		if config.InvocationExecutor == nil {
			return fmt.Errorf("durable execution test harness: invocation executor is required for live-provider child execution")
		}
	default:
		return fmt.Errorf("durable execution test harness: child executor mode must be %q or %q", factorysessionexecution.ChildExecutorModeFake, factorysessionexecution.ChildExecutorModeLive)
	}
	return nil
}

type fixedClock struct{ now time.Time }

func (c fixedClock) Now() time.Time { return c.now }

type memoryStore struct {
	mu        sync.Mutex
	snapshots map[string][]byte
}

func newMemoryStore() *memoryStore { return &memoryStore{snapshots: make(map[string][]byte)} }

func (s *memoryStore) Save(sessionID string, encoded []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.snapshots[sessionID] = append([]byte(nil), encoded...)
	return nil
}

func (s *memoryStore) Load(sessionID string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	encoded, ok := s.snapshots[sessionID]
	if !ok {
		return nil, fs.ErrNotExist
	}
	return append([]byte(nil), encoded...), nil
}

type recordingProvider struct {
	mu    sync.Mutex
	calls int
}

// scriptedHarnessWorkflows implements only the public Factory Runtime contract
// observed by this Factory Session harness. JavaScript parsing and execution
// invariants stay in Factory Runtime's owner-local suite.
type scriptedHarnessWorkflows struct{}

func (scriptedHarnessWorkflows) PreviewWorkflow(
	context.Context,
	factory.WorkflowPreviewInput,
) (factory.WorkflowPreview, error) {
	return factory.WorkflowPreview{}, fmt.Errorf("unexpected PreviewWorkflow call")
}

func (scriptedHarnessWorkflows) BuildPreview(
	factory.WorkflowPreviewRequest,
) factory.WorkflowPreview {
	return factory.WorkflowPreview{}
}

func (scriptedHarnessWorkflows) DefaultSourceContext(
	projectRoot string,
) (factory.WorkflowSourceContext, error) {
	return factory.WorkflowSourceContext{ProjectRoot: projectRoot}, nil
}

func (scriptedHarnessWorkflows) ResolveSource(
	request factory.WorkflowSourceRequest,
	_ factory.WorkflowSourceContext,
) factory.WorkflowSourceResolution {
	content := request.InlineSource
	if request.Kind == factory.WorkflowSourceKindWorkflowName {
		content = finalWorkflow
	}
	digest := sha256.Sum256([]byte(content))
	return factory.WorkflowSourceResolution{
		RequestKind:  request.Kind,
		RequestValue: request.Value,
		ResolvedKind: request.Kind,
		LookupStage:  factory.WorkflowSourceLookupStageExplicitSourceKind,
		SourceRef:    "scripted.workflow.js",
		SourceHash:   fmt.Sprintf("sha256:%x", digest[:]),
		Dialect:      "you-workflow-v1",
		Content:      content,
		ArtifactRoot: factory.WorkflowSourceArtifactRootDecision{Allowed: true},
		Found:        true,
	}
}

func (scriptedHarnessWorkflows) LoadSource(
	request factory.WorkflowValidationLoadRequest,
) (factory.WorkflowValidationLoadedSource, []factory.WorkflowValidationIssue) {
	return factory.WorkflowValidationLoadedSource{
		SourceRef:        request.SourceRef,
		SourceHash:       "sha256:scripted",
		Format:           factory.WorkflowValidationFormatJavaScript,
		AuthoredSource:   request.Content,
		ExecutableSource: request.Content,
	}, nil
}

func (scriptedHarnessWorkflows) ValidateArgs([]byte, map[string]any) error {
	return nil
}

func (scriptedHarnessWorkflows) ValidateLoaded(
	factory.WorkflowValidationLoadedSource,
	factory.WorkflowValidationRequest,
) factory.WorkflowValidationResult {
	return factory.WorkflowValidationResult{}
}

func (scriptedHarnessWorkflows) Validate(
	factory.WorkflowValidationRequest,
) factory.WorkflowValidationResult {
	return factory.WorkflowValidationResult{}
}

func (scriptedHarnessWorkflows) Run(
	ctx context.Context,
	request factory.JavaScriptRuntimeRequest,
	hooks factory.JavaScriptRuntimeHooks,
) (factory.JavaScriptRuntimeOutcome, error) {
	if strings.Contains(request.Source, "while (true)") {
		<-ctx.Done()
		return factory.JavaScriptRuntimeOutcome{
			Failure: factory.JavaScriptRuntimeFailure{
				Code:    factory.JavaScriptRuntimeCodeCanceled,
				Message: ctx.Err().Error(),
			},
		}, nil
	}

	result := map[string]any{"subject": "root"}
	records := newScriptedChildRecordSink()
	if strings.Contains(request.Source, "agent.run") && hooks.NewChildExecutor != nil {
		executor := hooks.NewChildExecutor(request.SessionID, records, request.Policy)
		child, err := executor.Execute(ctx, factory.JavaScriptChildExecutionRequest{
			Prompt:        "summarize workflows",
			Label:         "summarize-findings",
			ModelProvider: "codex",
			ArgsSubject:   "workflows",
			WorkflowName:  "scripted-child",
		})
		if err != nil {
			return factory.JavaScriptRuntimeOutcome{}, err
		}
		result = child.Output
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		return factory.JavaScriptRuntimeOutcome{}, err
	}
	return factory.JavaScriptRuntimeOutcome{
		OK:      true,
		Value:   factory.TypedValue{JSON: encoded},
		Records: records.records,
	}, nil
}

func (scriptedHarnessWorkflows) ResumeContext(
	factory.JavaScriptCompletedCheckpointSummary,
	[]factory.JavaScriptRuntimeRecord,
) factory.JavaScriptResumeContext {
	return factory.JavaScriptResumeContext{}
}

func (scriptedHarnessWorkflows) TextDigest(string) string {
	return "sha256:scripted-prompt"
}

func (scriptedHarnessWorkflows) SchemaDigest(map[string]any) string {
	return "sha256:scripted-schema"
}

func (scriptedHarnessWorkflows) CloneOutputMap(output map[string]any) map[string]any {
	cloned := make(map[string]any, len(output))
	for key, value := range output {
		cloned[key] = value
	}
	return cloned
}

type scriptedChildRecordSink struct {
	records       []factory.JavaScriptRuntimeRecord
	dispatchCount int
	artifactCount int
}

func newScriptedChildRecordSink() *scriptedChildRecordSink {
	return &scriptedChildRecordSink{}
}

func (sink *scriptedChildRecordSink) Append(record factory.JavaScriptRuntimeRecord) {
	record.Sequence = len(sink.records) + 1
	sink.records = append(sink.records, record)
}

func (sink *scriptedChildRecordSink) AppendChildDispatch(
	base factory.JavaScriptChildDispatchRecord,
	status string,
) {
	record := base
	record.Status = status
	sink.Append(factory.JavaScriptRuntimeRecord{
		Kind:          factory.JavaScriptRecordKindChildDispatch,
		ChildDispatch: &record,
	})
}

func (sink *scriptedChildRecordSink) NextChildDispatchIdentity() (string, int) {
	sink.dispatchCount++
	return fmt.Sprintf("dispatch-%d", sink.dispatchCount), sink.dispatchCount
}

func (sink *scriptedChildRecordSink) NextChildArtifactID() string {
	sink.artifactCount++
	return fmt.Sprintf("child-artifact-%d", sink.artifactCount)
}

func (p *recordingProvider) Execute(
	_ context.Context,
	input workerexecution.InvocationInput,
) (workerexecution.InvocationResult, error) {
	p.mu.Lock()
	p.calls++
	p.mu.Unlock()
	response := workerexecution.InferenceResponse{Content: `{"text":"provider result"}`}
	return workerexecution.InvocationResult{Response: response, Attempt: input.Attempt}, nil
}

func (p *recordingProvider) callCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls
}

var _ workerexecution.InvocationExecutor = (*recordingProvider)(nil)

func TestNew_SelectsFakeAndKeepsInstancesIsolated(t *testing.T) {
	t.Parallel()
	clock := fixedClock{now: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	scenario := factorysessionexecution.FakeScenario{
		RequestID: "request-fake",
		Session: factorysessionexecution.SessionReadResult{
			SessionID: "dur-sess-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			Status:    factorysessionexecution.LifecycleStatusSucceeded,
		},
	}
	first, err := newHarness(harnessConfig{
		Mode:          harnessModeFake,
		Clock:         clock,
		FakeScenarios: []factorysessionexecution.FakeScenario{scenario},
	})
	if err != nil {
		t.Fatalf("New(first fake): %v", err)
	}
	second, err := newHarness(harnessConfig{Mode: harnessModeFake, Clock: clock})
	if err != nil {
		t.Fatalf("New(second fake): %v", err)
	}

	started, err := first.StartSync(context.Background(), startRequest("request-fake", finalWorkflow))
	if err != nil {
		t.Fatalf("StartSync: %v", err)
	}
	if _, err := second.GetSession(context.Background(), started.SessionID); err == nil {
		t.Fatal("second harness observed first harness session")
	}
}

func TestNewJavaScript_ForwardsClockProjectRootAndPersistence(t *testing.T) {
	t.Parallel()
	projectRoot := t.TempDir()
	writeNamedWorkflow(t, projectRoot, "from-project", finalWorkflow)
	store := newMemoryStore()
	wantTime := time.Date(2035, time.January, 2, 3, 4, 5, 0, time.FixedZone("test", -8*60*60))
	config := javascriptConfig(projectRoot, fixedClock{now: wantTime}, store)
	service, err := newHarness(config)
	if err != nil {
		t.Fatalf("New(JavaScript): %v", err)
	}

	started, err := service.StartSync(context.Background(), factorysessionexecution.StartRequest{
		RequestID: "request-project-root",
		Source: factorysessionexecution.Source{
			Kind:         factory.WorkflowSourceKindWorkflowName,
			WorkflowName: "from-project",
		},
		Args: map[string]any{"subject": "root"},
	})
	if err != nil {
		t.Fatalf("StartSync: %v", err)
	}
	read, err := service.GetSession(context.Background(), started.SessionID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if read.Lifecycle == nil || read.Lifecycle.StartedAt == nil || !read.Lifecycle.StartedAt.Equal(wantTime.UTC()) {
		t.Fatalf("startedAt = %#v, want %s", read.Lifecycle, wantTime.UTC())
	}

	reloaded, err := newHarness(config)
	if err != nil {
		t.Fatalf("New(reloaded JavaScript): %v", err)
	}
	if _, err := reloaded.GetSession(context.Background(), started.SessionID); err != nil {
		t.Fatalf("GetSession from reused persistence: %v", err)
	}

	isolated, err := newHarness(javascriptConfig(projectRoot, fixedClock{now: wantTime}, newMemoryStore()))
	if err != nil {
		t.Fatalf("New(isolated JavaScript): %v", err)
	}
	if _, err := isolated.GetSession(context.Background(), started.SessionID); err == nil {
		t.Fatal("independent persistence unexpectedly shared session state")
	}
}

func TestNewJavaScript_ForwardsLiveChildProviderAndMode(t *testing.T) {
	t.Parallel()
	provider := &recordingProvider{}
	config := javascriptConfig(t.TempDir(), fixedClock{now: time.Now()}, newMemoryStore())
	config.ChildExecutorMode = factorysessionexecution.ChildExecutorModeLive
	config.InvocationExecutor = provider
	service, err := newHarness(config)
	if err != nil {
		t.Fatalf("New(JavaScript live): %v", err)
	}

	started, err := service.StartSync(context.Background(), startRequest("request-live-child", childWorkflow))
	if err != nil {
		t.Fatalf("StartSync: %v", err)
	}
	if started.Status != string(factorysessionexecution.LifecycleStatusSucceeded) {
		t.Fatalf("status = %q, want SUCCEEDED", started.Status)
	}
	if provider.callCount() != 1 {
		t.Fatalf("provider calls = %d, want 1", provider.callCount())
	}
}

func TestNewJavaScript_AsyncRunningSupportsStatusNotReadyAndCancellation(t *testing.T) {
	t.Parallel()
	service, err := newHarness(javascriptConfig(
		t.TempDir(),
		fixedClock{now: time.Now()},
		newMemoryStore(),
	))
	if err != nil {
		t.Fatalf("New(JavaScript): %v", err)
	}

	started, err := service.StartAsync(
		context.Background(),
		startRequest("request-async-running", busyLoopWorkflow),
	)
	if err != nil {
		t.Fatalf("StartAsync: %v", err)
	}
	if started.Status != string(factorysessionexecution.LifecycleStatusRunning) {
		t.Fatalf("status = %q, want RUNNING", started.Status)
	}
	read, err := service.GetSession(context.Background(), started.SessionID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if read.Status != factorysessionexecution.LifecycleStatusRunning {
		t.Fatalf("read status = %q, want RUNNING", read.Status)
	}
	result, err := service.GetResult(context.Background(), started.SessionID, factorysessionexecution.ResultRequest{
		Mode: factorysessionexecution.ResultModeFinal,
	})
	if err != nil {
		t.Fatalf("GetResult: %v", err)
	}
	if result.ResultStatus != factorysessionexecution.ResultStatusNotReady ||
		result.Availability == nil ||
		!result.Availability.Retryable {
		t.Fatalf("result = %#v, want retryable NOT_READY", result)
	}
	cancelled, err := service.Cancel(
		context.Background(),
		started.SessionID,
		factorysessionexecution.ControlRequest{},
	)
	if err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if cancelled.Outcome != factorysessionexecution.LifecycleControlOutcomeAccepted {
		t.Fatalf("cancel outcome = %q, want ACCEPTED", cancelled.Outcome)
	}
	waitForHarnessStatus(t, service, started.SessionID, factorysessionexecution.LifecycleStatusCanceled)
}

func TestNewJavaScript_AsyncCompletionPublishesTerminalResult(t *testing.T) {
	t.Parallel()
	service, err := newHarness(javascriptConfig(
		t.TempDir(),
		fixedClock{now: time.Now()},
		newMemoryStore(),
	))
	if err != nil {
		t.Fatalf("New(JavaScript): %v", err)
	}

	started, err := service.StartAsync(
		context.Background(),
		startRequest("request-async-final", finalWorkflow),
	)
	if err != nil {
		t.Fatalf("StartAsync: %v", err)
	}
	read := waitForHarnessStatus(
		t,
		service,
		started.SessionID,
		factorysessionexecution.LifecycleStatusSucceeded,
	)
	if read.ResultSummary == nil ||
		read.ResultSummary.ResultStatus != string(factorysessionexecution.ResultStatusFinal) {
		t.Fatalf("result summary = %#v, want FINAL", read.ResultSummary)
	}
	result, err := service.GetResult(context.Background(), started.SessionID, factorysessionexecution.ResultRequest{
		Mode: factorysessionexecution.ResultModeFinal,
	})
	if err != nil {
		t.Fatalf("GetResult: %v", err)
	}
	if result.ResultStatus != factorysessionexecution.ResultStatusFinal ||
		len(result.PrimaryResult) == 0 {
		t.Fatalf("result = %#v, want terminal FINAL primary result", result)
	}
}

func TestNew_RejectsUnsupportedAndIncompleteConfiguration(t *testing.T) {
	t.Parallel()
	valid := javascriptConfig(t.TempDir(), fixedClock{now: time.Now()}, newMemoryStore())
	fakeClock := fixedClock{now: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	tests := []struct {
		name   string
		config harnessConfig
		want   string
	}{
		{name: "missing mode", config: harnessConfig{}, want: "unsupported mode"},
		{name: "fake runtime dependency", config: harnessConfig{Mode: harnessModeFake, Clock: fakeClock, ProjectRoot: t.TempDir()}, want: "does not accept"},
		{name: "missing fake clock", config: harnessConfig{Mode: harnessModeFake}, want: "clock is required for fake mode"},
		{name: "fake fixture and scenarios", config: harnessConfig{Mode: harnessModeFake, Clock: fakeClock, FakeFixturePath: "fixtures.json", FakeScenarios: []factorysessionexecution.FakeScenario{{RequestID: "scenario"}}}, want: "not both"},
		{name: "missing fake fixture", config: harnessConfig{Mode: harnessModeFake, Clock: fakeClock, FakeFixturePath: filepath.Join(t.TempDir(), "missing.json")}, want: "load fake fixtures"},
		{name: "missing root", config: withConfig(valid, func(c *harnessConfig) { c.ProjectRoot = "" }), want: "project root is required"},
		{name: "missing clock", config: withConfig(valid, func(c *harnessConfig) { c.Clock = nil }), want: "clock is required"},
		{name: "missing persistence", config: withConfig(valid, func(c *harnessConfig) { c.Persistence = nil }), want: "persistence is required"},
		{name: "missing child mode", config: withConfig(valid, func(c *harnessConfig) { c.ChildExecutorMode = "" }), want: "child executor mode"},
		{name: "live without executor", config: withConfig(valid, func(c *harnessConfig) { c.ChildExecutorMode = factorysessionexecution.ChildExecutorModeLive }), want: "invocation executor is required"},
		{name: "fake with executor", config: withConfig(valid, func(c *harnessConfig) {
			c.InvocationExecutor = &recordingProvider{}
		}), want: "invocation executor is only valid"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := newHarness(tt.config)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("New() error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func javascriptConfig(projectRoot string, clock fixedClock, store *memoryStore) harnessConfig {
	return harnessConfig{
		Mode:        harnessModeJavaScript,
		ProjectRoot: projectRoot,
		Clock:       clock,
		CheckpointSummaries: checkpointfixtures.CheckpointSummariesFixture{
			BuildResult:  checkpointfixtures.ResumableCheckpointSummaryResult(),
			LatestResult: checkpointfixtures.ResumableCheckpointSummaryResult(),
		},
		Workflows:         scriptedHarnessWorkflows{},
		Persistence:       store,
		ChildExecutorMode: factorysessionexecution.ChildExecutorModeFake,
	}
}

func waitForHarnessStatus(
	t *testing.T,
	service factorysessionexecution.Service,
	sessionID string,
	want factorysessionexecution.LifecycleStatus,
) factorysessionexecution.SessionReadResult {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		read, err := service.GetSession(context.Background(), sessionID)
		if err != nil {
			t.Fatalf("GetSession: %v", err)
		}
		if read.Status == want {
			return read
		}
		if factorysessionexecution.IsTerminalLifecycleStatus(read.Status) && read.Status != want {
			t.Fatalf("session reached %q before %q", read.Status, want)
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("session did not reach %q", want)
	return factorysessionexecution.SessionReadResult{}
}

func withConfig(config harnessConfig, change func(*harnessConfig)) harnessConfig {
	change(&config)
	return config
}

func startRequest(requestID, source string) factorysessionexecution.StartRequest {
	return factorysessionexecution.StartRequest{
		RequestID: requestID,
		Source: factorysessionexecution.Source{
			Kind: factory.WorkflowSourceKindInlineWorkflow,
			InlineWorkflow: &factorysessionexecution.InlineWorkflowSource{
				Dialect:      "you-workflow-v1",
				InlineSource: source,
			},
		},
		Args: map[string]any{"subject": "test"},
	}
}

func writeNamedWorkflow(t *testing.T, projectRoot, name, source string) {
	t.Helper()
	dir := filepath.Join(projectRoot, factory.WorkflowSourceProjectClaudeWorkflowsDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, name+".js"), []byte(source), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

type orchestrationJavaScriptAdapter struct {
	factory.JavaScriptWorkflowRuntime
}

func orchestrationJavaScriptFromWorkflows(workflows factory.JavaScriptWorkflows) factory.OrchestrationJavaScriptExecution {
	if workflows == nil {
		return nil
	}
	return orchestrationJavaScriptAdapter{workflows}
}

func (a orchestrationJavaScriptAdapter) RunJavaScript(
	ctx context.Context,
	req factory.JavaScriptRuntimeRequest,
	hooks factory.JavaScriptRuntimeHooks,
) (factory.JavaScriptRuntimeOutcome, error) {
	return a.Run(ctx, req, hooks)
}

func (a orchestrationJavaScriptAdapter) ResumeJavaScript(
	summary factory.JavaScriptCompletedCheckpointSummary,
	records []factory.JavaScriptRuntimeRecord,
) factory.JavaScriptResumeContext {
	return a.ResumeContext(summary, records)
}
