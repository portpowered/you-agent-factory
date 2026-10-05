package factorysessionexecution

import (
	"context"
	"errors"
	"github.com/portpowered/infinite-you/internal/testutil/checkpointfixtures"
	"github.com/portpowered/infinite-you/internal/testutil/factoryruntimefixtures"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"strings"
	"testing"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
)

func TestDurableLiveChangeRejectsAnotherProjectsRuntime(t *testing.T) {
	const sessionID = "dur-sess-project-one"
	service := &JavaScriptRuntimeService{sessions: map[string]*runtimeSessionState{
		sessionID: {session: SessionReadResult{SessionID: sessionID}, projectRoot: "/project-one"},
	}}
	_, err := service.ApplyLiveChangeWithRuntime(context.Background(), sessionID, factorysessions.LiveChangeRequest{}, nil, "/project-two")
	if !errors.Is(err, factorysessions.ErrSessionNotFound) {
		t.Fatalf("cross-project live change error = %v, want session not found", err)
	}
}

func (s *JavaScriptRuntimeService) childExecutorHooks(mode, sessionID string) factory.JavaScriptRuntimeHooks {
	return s.childExecutorHooksForRequest(mode, sessionID, nil)
}

func (s *JavaScriptRuntimeService) childExecutorHooksForRequest(mode, sessionID string, mockWorkers *workers.MockWorkersConfig) factory.JavaScriptRuntimeHooks {
	return s.childExecutorHooksForStart(mode, sessionID, mockWorkers, nil, nil, nil)
}

// TestDirectChildExecutor_CarriesCanonicalPermissionsToWorkersExecuteRequest
// is the standalone composition regression. Its child has no Factory Runtime
// or Worker Session behind it, so the direct executor must translate the
// canonical child permission into the detached Workers request itself.
func TestDirectChildExecutor_CarriesCanonicalPermissionsToWorkersExecuteRequest(t *testing.T) {
	for _, test := range []struct {
		name       string
		permission factory.JavaScriptChildPermission
		want       bool
	}{
		{name: "DEFAULT", permission: factory.JavaScriptChildPermissionDefault, want: false},
		{name: "SKIP_PERMISSIONS", permission: factory.JavaScriptChildPermissionSkipPermissions, want: true},
		{name: "omitted", want: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			invocation := &recordingWorkerExecution{result: workers.ExecuteResult{
				Outcome: workers.ExecutionOutcomeAccepted,
				Output: workers.ProposedOutput{
					Primary: []work.WorkContentPart{{
						Type: work.WorkContentPartTypeText,
						Text: "child output",
					}},
				},
			}}
			executor := newDirectChildExecutor(
				"direct-sess-1",
				invocation,
				newChildRecordSink(),
				childTestValues{},
				"/project",
				0,
			)
			request := factory.JavaScriptChildExecutionRequest{Prompt: "run", Permissions: test.permission}

			if _, err := executor.Execute(context.Background(), request); err != nil {
				t.Fatalf("Execute: %v", err)
			}
			if invocation.request.Target.Permissions.SkipPermissions != test.want {
				t.Fatalf("Workers skip-permissions = %v, want %v", invocation.request.Target.Permissions.SkipPermissions, test.want)
			}
		})
	}
}

func TestJavaScriptRuntimeService_StandaloneChildUsesInjectedWorkersExecute(t *testing.T) {
	invoker := &recordingWorkerExecution{result: workers.ExecuteResult{
		Outcome: workers.ExecutionOutcomeAccepted,
	}}
	service := &JavaScriptRuntimeService{
		projectRoot: "/project",
		childValues: childTestValues{},
	}
	service.SetDirectWorkerExecution(invoker)

	hooks := service.childExecutorHooks(ChildExecutorModeLive, "standalone-session")
	if hooks.NewChildExecutor == nil {
		t.Fatal("standalone child executor hook = nil")
	}
	sink := newChildRecordSink()
	if _, err := hooks.NewChildExecutor("standalone-session", sink, factory.DefaultJavaScriptPolicy()).Execute(
		context.Background(),
		factory.JavaScriptChildExecutionRequest{Prompt: "run", Preset: "worker-a"},
	); err != nil {
		t.Fatalf("Execute standalone child: %v", err)
	}
	if invoker.request.Correlation.FactorySessionID != "standalone-session" {
		t.Fatalf("standalone child session ID = %q, want standalone-session", invoker.request.Correlation.FactorySessionID)
	}
	if invoker.request.Target.WorkerName != "worker-a" {
		t.Fatalf("standalone child worker name = %q, want worker-a", invoker.request.Target.WorkerName)
	}
}

func TestLiveChildWithoutWorkersExecutionFailsWithChildSessionID(t *testing.T) {
	service := &JavaScriptRuntimeService{
		projectRoot: "/project",
		childValues: childTestValues{},
	}
	hooks := service.childExecutorHooks(ChildExecutorModeLive, "parent-session")
	if hooks.NewChildExecutor == nil {
		t.Fatal("live child executor hook = nil")
	}

	_, err := hooks.NewChildExecutor(
		"child-session-42",
		newChildRecordSink(),
		factory.DefaultJavaScriptPolicy(),
	).Execute(context.Background(), factory.JavaScriptChildExecutionRequest{Prompt: "run"})
	if err == nil || !strings.Contains(err.Error(), "child-session-42") || !strings.Contains(err.Error(), "Workers Execute capability is required") {
		t.Fatalf("missing Workers Execute error = %v, want child session and capability", err)
	}
}

func TestDurableChildMockWorkersAreSelectedPerRequest(t *testing.T) {
	service := newProcessChildRuntime(&recordingWorkerExecution{})
	mocked := workers.NewEmptyMockWorkersConfig()
	mockedChild := service.childExecutorHooksForRequest(ChildExecutorModeLive, "mocked", mocked).
		NewChildExecutor("mocked-child", newChildRecordSink(), factory.DefaultJavaScriptPolicy()).(*childWorkerExecutor)
	liveChild := service.childExecutorHooksForRequest(ChildExecutorModeLive, "live", nil).
		NewChildExecutor("live-child", newChildRecordSink(), factory.DefaultJavaScriptPolicy()).(*childWorkerExecutor)
	if mockedChild.mockWorkers == nil || liveChild.mockWorkers != nil {
		t.Fatalf("per-request mock selection: mocked = %v, live = %v", mockedChild.mockWorkers, liveChild.mockWorkers)
	}
	mockedChild.mockWorkers.MockWorkers = append(mockedChild.mockWorkers.MockWorkers, workers.MockWorkerConfig{ID: "changed"})
	if len(mocked.MockWorkers) != 0 {
		t.Fatal("child mutated request mock configuration")
	}
}

func TestDurableChildAttemptStarterIsSelectedPerRequest(t *testing.T) {
	service := newProcessChildRuntime(&recordingWorkerExecution{})
	starter := factorysessions.WorkerAttemptStarter(func(context.Context, workers.ExecuteRequest) (func(context.Context, workers.ExecuteResult, error) error, error) {
		return nil, nil
	})
	selected := service.childExecutorHooksForStart(ChildExecutorModeLive, "selected", nil, starter, nil, nil).
		NewChildExecutor("selected-child", newChildRecordSink(), factory.DefaultJavaScriptPolicy()).(*childWorkerExecutor)
	other := service.childExecutorHooksForStart(ChildExecutorModeLive, "other", nil, nil, nil, nil).
		NewChildExecutor("other-child", newChildRecordSink(), factory.DefaultJavaScriptPolicy()).(*childWorkerExecutor)
	if selected.attemptStarter == nil || other.attemptStarter != nil {
		t.Fatalf("per-request attempt starters: selected = %v, other = %v", selected.attemptStarter != nil, other.attemptStarter != nil)
	}
}

func TestDurableChildProgressPublisherIsSelectedPerRequest(t *testing.T) {
	service := newProcessChildRuntime(&recordingWorkerExecution{})
	var selectedFragments []workers.ProgressFragment
	selected := service.childExecutorHooksForStart(ChildExecutorModeLive, "selected", nil, nil, nil, func(fragment workers.ProgressFragment) {
		selectedFragments = append(selectedFragments, fragment)
	}).NewChildExecutor("selected-child", newChildRecordSink(), factory.DefaultJavaScriptPolicy()).(*childWorkerExecutor)
	other := service.childExecutorHooksForStart(ChildExecutorModeLive, "other", nil, nil, nil, nil).
		NewChildExecutor("other-child", newChildRecordSink(), factory.DefaultJavaScriptPolicy()).(*childWorkerExecutor)

	selected.publish("selected-dispatch", workers.ProgressFragment{Kind: workers.ResponseFragmentKind, Payload: "selected text"})
	if len(selectedFragments) != 1 || selectedFragments[0].DispatchID != "selected-dispatch" || selectedFragments[0].Correlation.DispatchID != "selected-dispatch" {
		t.Fatalf("selected progress = %#v", selectedFragments)
	}
	other.publish("other-dispatch", workers.ProgressFragment{Kind: workers.ResponseFragmentKind, Payload: "other text"})
	if len(selectedFragments) != 1 {
		t.Fatalf("other start leaked into selected publisher: %#v", selectedFragments)
	}
}

func newTestChildWorkerExecutor(
	invoke childExecuteService,
	sink *childRecordSink,
	observe workerDispatchObserver,
) *childWorkerExecutor {
	return newChildWorkerExecutor("dur-sess-1", invoke, sink, childTestValues{}, observe, "/project", 0)
}

// recordingWorkerExecution is the narrow Workers Execute seam a child reaches
// through. It intentionally does not embed Factory Runtime or an executor.
type recordingWorkerExecution struct {
	request   workers.ExecuteRequest
	result    workers.ExecuteResult
	err       error
	onInvoke  func()
	onExecute func(workers.ExecuteRequest)
}

func (i *recordingWorkerExecution) Execute(
	_ context.Context,
	req workers.ExecuteRequest,
) (workers.ExecuteResult, error) {
	i.request = req
	if i.onExecute != nil {
		i.onExecute(req)
	}
	if i.onInvoke != nil {
		i.onInvoke()
	}
	return i.result, i.err
}

type childRecordSink struct {
	records  []factory.JavaScriptRuntimeRecord
	statuses []string
	next     int
}

func newChildRecordSink() *childRecordSink { return &childRecordSink{} }

func (s *childRecordSink) Append(record factory.JavaScriptRuntimeRecord) {
	s.records = append(s.records, record)
}

func (s *childRecordSink) AppendChildDispatch(_ factory.JavaScriptChildDispatchRecord, status string) {
	s.statuses = append(s.statuses, status)
}

func (s *childRecordSink) NextChildDispatchIdentity() (string, int) {
	s.next++
	return "dispatch-1", s.next - 1
}

func (s *childRecordSink) NextChildArtifactID() string { return "artifact-1" }

func (s *childRecordSink) terminalChildDispatch(t *testing.T) factory.JavaScriptChildDispatchRecord {
	t.Helper()
	for index := len(s.records) - 1; index >= 0; index-- {
		if record := s.records[index]; record.ChildDispatch != nil {
			return *record.ChildDispatch
		}
	}
	t.Fatal("no terminal child dispatch record was appended")
	return factory.JavaScriptChildDispatchRecord{}
}

type childTestValues struct{}

func (childTestValues) TextDigest(string) string           { return "digest" }
func (childTestValues) SchemaDigest(map[string]any) string { return "schema-digest" }
func (childTestValues) CloneOutputMap(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	clone := make(map[string]any, len(m))
	for key, value := range m {
		clone[key] = value
	}
	return clone
}

// newProcessChildRuntime exercises the complete production constructor rather
// than attaching the execution capability after publication.
func newProcessChildRuntime(worker childExecuteService) *JavaScriptRuntimeService {
	return NewProcessDurableRuntime("/project", ChildExecutorModeLive, nil, nil,
		durableFixedClock{}, testSyncWaitScheduler{}, nil, nil, nil, childTestValues{},
		nil, factory.JavaScriptWorkerSettings{}, nil, testSessionIDGenerator,
		nil, nil, nil, nil, nil, nil, worker, nil, nil)
}

func TestProcessDurableRuntimeInjectedWorkerPreservesOutcomeAndCause(t *testing.T) {
	t.Parallel()
	for _, failure := range []error{nil, errors.New("selected worker failed")} {
		worker := &recordingWorkerExecution{err: failure, result: workers.ExecuteResult{
			Outcome: workers.ExecutionOutcomeAccepted, Output: workers.ProposedOutput{Primary: []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "selected output"}}},
		}}
		service := newProcessChildRuntime(worker)
		result, err := service.childExecutorHooks(ChildExecutorModeLive, "parent").NewChildExecutor("selected-child", newChildRecordSink(), factory.DefaultJavaScriptPolicy()).Execute(t.Context(), factory.JavaScriptChildExecutionRequest{Prompt: "run", Preset: "worker-a"})
		if failure != nil {
			if err == nil || err.Error() != failure.Error() || result.Status != factory.JavaScriptChildDispatchStatusFailed {
				t.Fatalf("worker diagnostic = %#v, %v", result, err)
			}
			continue
		}
		if err != nil || result.Status != factory.JavaScriptChildDispatchStatusCompleted || worker.request.Target.WorkerName != "worker-a" || worker.request.Correlation.FactorySessionID != "selected-child" {
			t.Fatalf("child result=%#v request=%#v error=%v", result, worker.request, err)
		}
	}
}

type selectedProcessProvider struct{ providers.Service }

func (selectedProcessProvider) ResolveIdentity(context.Context, providers.ResolveIdentityRequest) (providers.ResolveIdentityResult, error) {
	return providers.ResolveIdentityResult{ID: "selected-provider"}, nil
}

func TestProcessDurableRuntimeForwardsSelectedProviderToWorker(t *testing.T) {
	t.Parallel()
	worker := &recordingWorkerExecution{result: workers.ExecuteResult{Outcome: workers.ExecutionOutcomeAccepted}}
	worker.onExecute = func(request workers.ExecuteRequest) {
		if request.Input.ProviderOverride == nil {
			t.Fatal("selected provider missing")
		}
		identity, err := request.Input.ProviderOverride.ResolveIdentity(t.Context(), providers.ResolveIdentityRequest{Identity: "selected"})
		if err != nil || identity.ID != "selected-provider" {
			t.Fatalf("provider outcome=%#v error=%v", identity, err)
		}
	}
	service := NewProcessDurableRuntime("/project", ChildExecutorModeLive, nil, nil,
		durableFixedClock{}, testSyncWaitScheduler{}, nil, nil, nil, childTestValues{},
		nil, factory.JavaScriptWorkerSettings{}, nil, testSessionIDGenerator,
		nil, nil, nil, nil, nil, nil, worker, selectedProcessProvider{}, nil)
	_, err := service.childExecutorHooks(ChildExecutorModeLive, "parent").NewChildExecutor("selected-child", newChildRecordSink(), factory.DefaultJavaScriptPolicy()).Execute(t.Context(), factory.JavaScriptChildExecutionRequest{Prompt: "run"})
	if err != nil {
		t.Fatal(err)
	}
}

func TestProcessDurableRuntimeResumeUsesInjectedScopeAndPreservesFailure(t *testing.T) {
	t.Parallel()
	const sessionID = "dur-sess-0123456789abcdef0123456789abcdef"
	failure := errors.New("selected resume scope failed")
	store := &runtimeRecordingStore{}
	state := interruptedSessionForAdmissionTest(sessionID)
	state.projectRoot = "/project"
	persistResumeCoverageSnapshot(t, store, sessionID, state)
	workflows := factoryruntimefixtures.ScriptedJavaScriptWorkflows{}
	summaries := checkpointfixtures.CheckpointSummariesFixture{LatestResult: checkpointfixtures.ResumableCheckpointSummaryResult()}
	service := NewProcessDurableRuntime("/project", ChildExecutorModeFake, nil, store,
		durableFixedClock{}, testSyncWaitScheduler{}, summaries, workflows, workflows, workflows,
		nil, factory.JavaScriptWorkerSettings{}, nil, testSessionIDGenerator,
		nil, nil, nil, nil, nil, func(project string) (ResumeRuntimeScope, error) {
			if project != "/project" {
				t.Fatalf("resume project = %q", project)
			}
			return ResumeRuntimeScope{}, failure
		}, nil, nil, nil)
	_, err := service.ResumeInterruptedSession(t.Context(), sessionID, ResumeSessionRequest{RequestID: "selected-resume"})
	if !errors.Is(err, failure) {
		t.Fatalf("resume cause=%v", err)
	}
	read, err := service.GetSession(t.Context(), sessionID)
	if err != nil || read.Status != LifecycleStatusInterrupted {
		t.Fatalf("failed resume state=%#v error=%v", read, err)
	}
}
