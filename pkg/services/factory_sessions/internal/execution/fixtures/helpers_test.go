// backendsizecheck:ignore-file service-ownership migration preserves this consolidated surface until a dedicated responsibility split removes the exemption.
// pkgmaintcheck:ignore-file-lines service-ownership migration preserves this consolidated file; split responsibilities and remove this exemption.
package fixtures_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil/checkpointfixtures"
	"github.com/portpowered/infinite-you/internal/testutil/factoryruntimefixtures"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	fse "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution/fixtures"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution/runtimepersist"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/fileeffects"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

const runtimeEventSource = "runtime-service"

var fixtureSessionIdentity atomic.Uint64

func fixtureSessionID() string {
	return fmt.Sprintf("00000000-0000-4000-8000-%012d", fixtureSessionIdentity.Add(1))
}

type scriptedChildRecordSink struct {
	records   []factory.JavaScriptRuntimeRecord
	onRecord  func(factory.JavaScriptRuntimeRecord)
	childNext int
	artNext   int
}

func (s *scriptedChildRecordSink) Append(record factory.JavaScriptRuntimeRecord) {
	record.Sequence = len(s.records) + 1
	s.records = append(s.records, record)
	if s.onRecord != nil {
		s.onRecord(record)
	}
}

func (s *scriptedChildRecordSink) AppendChildDispatch(base factory.JavaScriptChildDispatchRecord, status string) {
	base.Status = status
	s.Append(factory.JavaScriptRuntimeRecord{Kind: factory.JavaScriptRecordKindChildDispatch, ChildDispatch: &base})
}

func (s *scriptedChildRecordSink) NextChildDispatchIdentity() (string, int) {
	s.childNext++
	return fmt.Sprintf("dispatch-%d", s.childNext), s.childNext
}

func (s *scriptedChildRecordSink) NextChildArtifactID() string {
	s.artNext++
	return fmt.Sprintf("child-artifact-%d", s.artNext)
}

func scriptedSingleChildWorkflows(
	request factory.JavaScriptChildExecutionRequest,
) factory.JavaScriptWorkflows {
	return factoryruntimefixtures.ScriptedJavaScriptWorkflows{
		RunFunc: func(
			ctx context.Context,
			runtimeRequest factory.JavaScriptRuntimeRequest,
			hooks factory.JavaScriptRuntimeHooks,
		) (factory.JavaScriptRuntimeOutcome, error) {
			sink := &scriptedChildRecordSink{onRecord: hooks.OnRecord}
			var result factory.JavaScriptChildExecutionResult
			var err error
			if hooks.NewChildExecutor != nil {
				result, err = hooks.NewChildExecutor(runtimeRequest.SessionID, sink, runtimeRequest.Policy).Execute(ctx, request)
			} else {
				artifactRef := factory.FormatArtifactURI(runtimeRequest.SessionID, "child-artifact-1")
				for _, status := range []string{
					factory.JavaScriptChildDispatchStatusQueued,
					factory.JavaScriptChildDispatchStatusRunning,
					factory.JavaScriptChildDispatchStatusCompleted,
				} {
					record := factory.JavaScriptChildDispatchRecord{
						DispatchID: "dispatch-1", ChildIndex: 1, Status: status,
						Label: request.Label, Preset: request.Preset,
						ModelProvider: request.ModelProvider, Model: request.Model,
						ReasoningEffort: request.ReasoningEffort,
						ExecutionMode:   factory.JavaScriptChildExecutionModeFake,
						Provider:        "fake", ProviderSessionRef: "fake-provider-session-1",
						ArtifactRef: artifactRef,
					}
					if status == factory.JavaScriptChildDispatchStatusCompleted {
						record.Output = map[string]any{"executionMode": factory.JavaScriptChildExecutionModeFake}
					}
					sink.Append(factory.JavaScriptRuntimeRecord{Kind: factory.JavaScriptRecordKindChildDispatch, ChildDispatch: &record})
				}
				result = factory.JavaScriptChildExecutionResult{
					DispatchID: "dispatch-1", ChildIndex: 1,
					Status:             factory.JavaScriptChildDispatchStatusCompleted,
					ExecutionMode:      factory.JavaScriptChildExecutionModeFake,
					Output:             map[string]any{"executionMode": factory.JavaScriptChildExecutionModeFake},
					ArtifactRef:        artifactRef,
					ProviderSessionRef: "fake-provider-session-1",
				}
			}
			if err != nil {
				code := factory.JavaScriptRuntimeCodeScriptError
				if ctx.Err() != nil {
					code = factory.JavaScriptRuntimeCodeCanceled
					if ctx.Err() == context.DeadlineExceeded {
						code = factory.JavaScriptRuntimeCodeTimeout
					}
				}
				return factory.JavaScriptRuntimeOutcome{
					Failure: factory.JavaScriptRuntimeFailure{Code: code, Message: err.Error()},
					Records: sink.records,
				}, nil
			}
			encoded, marshalErr := json.Marshal(map[string]any{"child": scriptedChildResultValue(result)})
			return factory.JavaScriptRuntimeOutcome{
				OK: true, Value: factory.TypedValue{JSON: encoded}, Records: sink.records,
			}, marshalErr
		},
	}
}

func scriptedChildResultValue(result factory.JavaScriptChildExecutionResult) map[string]any {
	return map[string]any{
		"dispatchId":         result.DispatchID,
		"childIndex":         result.ChildIndex,
		"status":             result.Status,
		"executionMode":      result.ExecutionMode,
		"diagnostic":         result.Diagnostic,
		"output":             result.Output,
		"artifactRef":        result.ArtifactRef,
		"providerSessionRef": result.ProviderSessionRef,
	}
}

func scriptedAgentRunChildWorkflows() factory.JavaScriptWorkflows {
	return scriptedSingleChildWorkflows(factory.JavaScriptChildExecutionRequest{
		Prompt: "summarize workflows", Label: "summarize-findings",
		Preset: "careful-review", ModelProvider: "CODEX", Model: "gpt-test", ReasoningEffort: "medium",
		WorkflowName: "agent-run-fake-child", ArgsSubject: "workflows",
	})
}

func scriptedFakeChildrenWorkflows(count int) factory.JavaScriptWorkflows {
	return factoryruntimefixtures.ScriptedJavaScriptWorkflows{
		RunFunc: func(
			_ context.Context,
			request factory.JavaScriptRuntimeRequest,
			hooks factory.JavaScriptRuntimeHooks,
		) (factory.JavaScriptRuntimeOutcome, error) {
			sink := &scriptedChildRecordSink{onRecord: hooks.OnRecord}
			results := make([]any, 0, count)
			for index := 1; index <= count; index++ {
				artifactRef := factory.FormatArtifactURI(request.SessionID, fmt.Sprintf("child-artifact-%d", index))
				record := factory.JavaScriptChildDispatchRecord{
					DispatchID: fmt.Sprintf("dispatch-%d", index), ChildIndex: index,
					Status:        factory.JavaScriptChildDispatchStatusCompleted,
					Label:         fmt.Sprintf("child-%d", index),
					ExecutionMode: factory.JavaScriptChildExecutionModeFake,
					Provider:      "fake", ProviderSessionRef: fmt.Sprintf("fake-provider-session-%d", index),
					ArtifactRef: artifactRef,
					Output:      map[string]any{"executionMode": factory.JavaScriptChildExecutionModeFake},
				}
				sink.Append(factory.JavaScriptRuntimeRecord{Kind: factory.JavaScriptRecordKindChildDispatch, ChildDispatch: &record})
				results = append(results, map[string]any{"executionMode": factory.JavaScriptChildExecutionModeFake})
			}
			encoded, err := json.Marshal(map[string]any{"results": results})
			return factory.JavaScriptRuntimeOutcome{OK: true, Value: factory.TypedValue{JSON: encoded}, Records: sink.records}, err
		},
	}
}

func scriptedLiveChildrenWorkflows(
	requests []factory.JavaScriptChildExecutionRequest,
) factory.JavaScriptWorkflows {
	return factoryruntimefixtures.ScriptedJavaScriptWorkflows{
		RunFunc: func(
			ctx context.Context,
			runtimeRequest factory.JavaScriptRuntimeRequest,
			hooks factory.JavaScriptRuntimeHooks,
		) (factory.JavaScriptRuntimeOutcome, error) {
			sink := &scriptedChildRecordSink{onRecord: hooks.OnRecord}
			executor := hooks.NewChildExecutor(runtimeRequest.SessionID, sink, runtimeRequest.Policy)
			results := make([]any, 0, len(requests))
			for _, request := range requests {
				result, err := executor.Execute(ctx, request)
				if err != nil {
					results = append(results, map[string]any{
						"dispatchId": result.DispatchID,
						"status":     factory.JavaScriptChildDispatchStatusFailed,
					})
					continue
				}
				results = append(results, scriptedChildResultValue(result))
			}
			encoded, err := json.Marshal(map[string]any{"results": results})
			return factory.JavaScriptRuntimeOutcome{
				OK: true, Value: factory.TypedValue{JSON: encoded}, Records: sink.records,
			}, err
		},
	}
}

type runtimeServiceConfig struct {
	ProjectRoot       string
	ChildExecutorMode string
	ProviderExecutor  workers.InvocationExecutor
	Persistence       runtimepersist.Store
	Clock             factory.Clock
	CheckpointSummary *factory.JavaScriptCheckpointSummary
	WorkerPresetIDs   map[string]struct{}
	WorkerSettings    factory.JavaScriptWorkerSettings
	Workflows         factory.JavaScriptWorkflows
}

type executionServiceConfig struct {
	ProjectRoot       string
	ChildExecutorMode string
	ProviderExecutor  workers.InvocationExecutor
	FakeScenarios     []fse.FakeScenario
	Persistence       fse.PersistenceChoice
	Clock             factory.Clock
	WorkerPresetIDs   map[string]struct{}
	WorkerSettings    factory.JavaScriptWorkerSettings
	Workflows         factory.JavaScriptWorkflows
}

func newExecutionService(provider fse.ExecutionProvider, config executionServiceConfig) (fse.Service, error) {
	switch provider {
	case fse.ExecutionProviderFake:
		clock := config.Clock
		if clock == nil {
			clock = fixtureClock{now: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
		}
		return fse.NewFakeService(clock, config.FakeScenarios...)
	case fse.ExecutionProviderJavaScriptRuntime:
		workflows := config.Workflows
		if workflows == nil {
			workflows = factoryruntimefixtures.ScriptedJavaScriptWorkflows{}
		}
		return fse.NewJavaScriptExecutionService(
			config.ProjectRoot,
			config.ChildExecutorMode,
			config.ProviderExecutor,
			config.Persistence,
			config.Clock,
			fixtureSyncWaitScheduler{},
			checkpointfixtures.CheckpointSummariesFixture{
				BuildResult:  checkpointfixtures.ResumableCheckpointSummaryResult(),
				LatestResult: checkpointfixtures.ResumableCheckpointSummaryResult(),
			},
			workflows,
			workflows,
			workflows,
			config.WorkerPresetIDs,
			config.WorkerSettings,
			fixtureRecordingWriter(),
			fixtureSessionID,
			nil, nil, nil,
		)
	default:
		return nil, fse.NewValidationError("provider", "unsupported execution provider")
	}
}

func newConfiguredJavaScriptRuntimeService(config runtimeServiceConfig) *fse.JavaScriptRuntimeService {
	workflows := config.Workflows
	if workflows == nil {
		workflows = factoryruntimefixtures.ScriptedJavaScriptWorkflows{}
	}
	checkpointSummary := config.CheckpointSummary
	if checkpointSummary == nil {
		checkpointSummary = checkpointfixtures.ResumableCheckpointSummaryResult()
	}
	clock := config.Clock
	if clock == nil {
		clock = fixtureClock{now: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	}
	return fse.NewJavaScriptRuntimeService(
		config.ProjectRoot, config.ChildExecutorMode,
		config.ProviderExecutor, config.Persistence, clock, fixtureSyncWaitScheduler{},
		checkpointfixtures.CheckpointSummariesFixture{
			BuildResult:  checkpointSummary,
			LatestResult: checkpointSummary,
		},
		workflows, workflows, workflows,
		config.WorkerPresetIDs, config.WorkerSettings, fixtureRecordingWriter(),
		fixtureSessionID,
		nil, nil, nil,
	)
}

type fixtureSyncWaitScheduler struct{}

func (fixtureSyncWaitScheduler) Now() time.Time { return time.Now() }

func (fixtureSyncWaitScheduler) After(duration time.Duration) <-chan time.Time {
	return time.After(duration)
}

func fixtureRecordingWriter() recordings.PortableRecordingWriter {
	return portableRecordingTestWriter{}
}

type portableRecordingTestWriter struct{}

func (portableRecordingTestWriter) Write(path string, value recordings.PortableRecording) error {
	if err := recordings.ValidatePortableRecording(value); err != nil {
		return err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o600)
}

type acceptedPetriExecutor struct{}

func (*acceptedPetriExecutor) Execute(_ context.Context, dispatch work.WorkDispatch) (workerexecution.WorkResult, error) {
	return workerexecution.WorkResult{DispatchID: dispatch.DispatchID, TransitionID: dispatch.TransitionID, Outcome: workerexecution.OutcomeAccepted, Output: "done"}, nil
}

func petriRecordingNet() *factory.Net {
	workType := &factory.WorkType{ID: "task", Name: "Task", States: []factory.StateDefinition{
		{Value: "init", Category: factory.StateCategoryInitial}, {Value: "done", Category: factory.StateCategoryTerminal},
	}}
	places := make(map[string]*factory.PetriPlace)
	for _, place := range workType.GeneratePlaces() {
		places[place.ID] = place
	}
	transition := &factory.PetriTransition{
		ID: "process", Name: "Process", Type: factory.PetriTransitionNormal, WorkerType: "mock",
		InputArcs:  []factory.PetriArc{{ID: "input", PlaceID: "task:init", Direction: factory.PetriArcInput, Cardinality: factory.PetriArcCardinality{Mode: factory.PetriCardinalityOne}}},
		OutputArcs: []factory.PetriArc{{ID: "output", PlaceID: "task:done", Direction: factory.PetriArcOutput, Cardinality: factory.PetriArcCardinality{Mode: factory.PetriCardinalityOne}}},
	}
	return &factory.Net{ID: "petri-recording", Places: places, Transitions: map[string]*factory.PetriTransition{transition.ID: transition}, WorkTypes: map[string]*factory.WorkType{workType.ID: workType}, Resources: map[string]*factory.ResourceDef{}}
}

func assertPersistedPetriMutationAndCanonicalProjection(t *testing.T, store runtimepersist.Store, sessionID string) {
	t.Helper()
	encoded, err := store.Load(sessionID)
	if err != nil {
		t.Fatalf("Load persisted owner snapshot: %v", err)
	}
	var snapshot fse.PersistedRuntimeSessionState
	if err := json.Unmarshal(encoded, &snapshot); err != nil {
		t.Fatalf("decode persisted owner snapshot: %v", err)
	}
	var mutation *interfaces.TokenMutationRecord
	var canonicalCount int
	for i := range snapshot.Records {
		record := snapshot.Records[i]
		if record.Kind == fse.DurableRecordKindCanonicalFactoryEvent {
			canonicalCount++
		}
		if record.Kind == fse.DurableRecordKindPetriTokenMutation {
			mutation = record.PetriMutation
		}
	}
	if canonicalCount == 0 {
		t.Fatal("reloaded owner snapshot lost canonical Factory Events")
	}
	if mutation == nil || mutation.Type != interfaces.MutationCreate || mutation.TransitionID != "process" || mutation.ToPlace != "task:done" {
		t.Fatalf("reloaded Petri mutation = %#v, want typed process mutation into task:done", mutation)
	}
}

var _ workers.WorkerExecutor = (*acceptedPetriExecutor)(nil)

func runtimePersistence(projectRoot string) runtimepersist.Store {
	store, err := runtimepersist.NewDirectoryStore(
		runtimepersist.DirForProjectRoot(projectRoot),
		platformfilesystem.Local{},
	)
	if err != nil {
		panic(err)
	}
	return store
}

func contractFixtureCatalogPath(t *testing.T) string {
	t.Helper()
	return filepath.Join("..", "..", "..", "..", "..", "transports", "http", "testdata", "durable-session-contract-fixtures.json")
}

func assertStringSliceEqual(t *testing.T, label string, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s = %#v, want %#v", label, got, want)
	}
	for index := range got {
		if got[index] != want[index] {
			t.Fatalf("%s[%d] = %q, want %q", label, index, got[index], want[index])
		}
	}
}
func publishedScenarioByPurpose(t *testing.T, purpose fixtures.FixtureScenarioPurpose) fixtures.PublishedFixtureScenario {
	t.Helper()
	for _, row := range fixtures.PublishedFixtureScenarios {
		if row.Purpose == purpose {
			return row
		}
	}
	t.Fatalf("published scenario missing for purpose %q", purpose)
	return fixtures.PublishedFixtureScenario{}
}

func startRequestForPublished(row fixtures.PublishedFixtureScenario) fse.StartRequest {
	switch row.RequestID {
	case "req-js-timeout-001":
		return fse.StartRequest{
			RequestID: row.RequestID,
			Source: fse.Source{
				Kind:         factory.WorkflowSourceKindWorkflowName,
				WorkflowName: "long-running-audit",
			},
			Wait: &fse.WaitOptions{TimeoutMillis: int64Ptr(30000)},
		}
	case "req-idempotent-replay-001":
		return fse.StartRequest{
			RequestID: row.RequestID,
			Source: fse.Source{
				Kind:         factory.WorkflowSourceKindWorkflowFile,
				WorkflowFile: ".claude/workflows/idempotent.yaml",
			},
			Args: map[string]any{"task": "replay"},
			RequestedPolicy: map[string]any{
				"policyHash": "req-policy-idempotent",
			},
		}
	default:
		return fse.StartRequest{
			RequestID: row.RequestID,
			Source: fse.Source{
				Kind:      factory.WorkflowSourceKindFactoryID,
				FactoryID: "customer-support-triage",
			},
		}
	}
}

func containsLiveSessionID(sessions []fse.LiveSessionSummary, sessionID string) bool {
	for _, session := range sessions {
		if session.ID == sessionID {
			return true
		}
	}
	return false
}

func containsDurableSessionID(sessions []fse.DurableSessionListSummary, sessionID string) bool {
	for _, session := range sessions {
		if session.SessionID == sessionID {
			return true
		}
	}
	return false
}

func startPublishedScenario(t *testing.T, service *fse.FakeService, row fixtures.PublishedFixtureScenario) {
	t.Helper()
	req := startRequestForPublished(row)
	if row.Purpose == fixtures.FixturePurposeSyncSuccess || row.Purpose == fixtures.FixturePurposeSyncTimeout {
		if _, err := service.StartSync(context.Background(), req); err != nil {
			t.Fatalf("fse.StartSync(%s): %v", row.Purpose, err)
		}
		return
	}
	if _, err := service.StartAsync(context.Background(), req); err != nil {
		t.Fatalf("fse.StartAsync(%s): %v", row.Purpose, err)
	}
}

func startAwaitingApprovalSession(t *testing.T, service *fse.FakeService) {
	t.Helper()
	_, err := service.StartAsync(context.Background(), fse.StartRequest{
		RequestID: "req-js-awaiting-001",
		Source: fse.Source{
			Kind:         factory.WorkflowSourceKindWorkflowFile,
			WorkflowFile: ".claude/workflows/approval-gate.yaml",
		},
	})
	if err != nil {
		t.Fatalf("fse.StartAsync awaiting approval: %v", err)
	}
}

func startFailedPartialSession(t *testing.T, service *fse.FakeService) {
	t.Helper()
	startAsyncByRequestID(t, service, "req-js-failed-partial-001")
}

func liveSessionCount(t *testing.T, service *fse.FakeService) int {
	t.Helper()
	result, err := service.ListSessions(context.Background(), fse.ListSessionsRequest{
		Scope: fse.SessionListScopeLive,
	})
	if err != nil {
		t.Fatalf("fse.ListSessions live: %v", err)
	}
	return len(result.LiveSessions)
}

func assertTypedFailureHash(t *testing.T, err error, wantHash string) fixtures.TypedFailureIdentity {
	t.Helper()
	if err == nil {
		t.Fatal("error = nil, want typed failure")
	}
	identity, ok := fixtures.TypedFailureIdentityFromError(err)
	if !ok {
		t.Fatalf("error = %v, want mappable typed failure identity", err)
	}
	hash, err := fixtures.TypedFailureHash(identity)
	if err != nil {
		t.Fatalf("fixtures.TypedFailureHash: %v", err)
	}
	if hash != wantHash {
		t.Fatalf("typed failure hash = %q, want %q (identity=%#v)", hash, wantHash, identity)
	}
	return identity
}

func newContractFakeService(t *testing.T) *fse.FakeService {
	t.Helper()
	service, err := fse.NewFakeServiceFromContractFixtures(
		contractFixtureCatalogPath(t),
		fixtureClock{now: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
		fileeffects.ContractFixtureReader(os.ReadFile),
	)
	if err != nil {
		t.Fatalf("NewFakeServiceFromContractFixtures: %v", err)
	}
	return service
}

func startAsyncByRequestID(t *testing.T, service *fse.FakeService, requestID string) fse.AsyncStartResult {
	t.Helper()
	result, err := service.StartAsync(context.Background(), fse.StartRequest{
		RequestID: requestID,
		Source:    fse.Source{Kind: factory.WorkflowSourceKindFactoryID, FactoryID: "customer-support-triage"},
	})
	if err != nil {
		t.Fatalf("StartAsync(%q): %v", requestID, err)
	}
	return result
}

func int64Ptr(value int64) *int64 {
	return &value
}

func startPublishedScenarioWithSync(t *testing.T, service *fse.FakeService, row fixtures.PublishedFixtureScenario, sync bool) {
	t.Helper()
	req := startRequestForPublished(row)
	if sync {
		if _, err := service.StartSync(context.Background(), req); err != nil {
			t.Fatalf("StartSync: %v", err)
		}
		return
	}
	if _, err := service.StartAsync(context.Background(), req); err != nil {
		t.Fatalf("StartAsync: %v", err)
	}
}

func assertDispatchListStableSummaries(
	t *testing.T,
	service *fse.FakeService,
	row fixtures.PublishedFixtureScenario,
	wantIDs []string,
	wantHash string,
) {
	t.Helper()
	listed, err := service.ListDispatches(context.Background(), row.SessionID)
	if err != nil {
		t.Fatalf("ListDispatches: %v", err)
	}
	if listed.SessionID != row.SessionID {
		t.Fatalf("sessionId = %q, want %q", listed.SessionID, row.SessionID)
	}
	if len(listed.Dispatches) != len(wantIDs) {
		t.Fatalf("dispatches = %#v, want %d rows", listed.Dispatches, len(wantIDs))
	}
	for index, wantID := range wantIDs {
		got := listed.Dispatches[index]
		if got.ID != wantID {
			t.Fatalf("dispatch[%d].id = %q, want %q", index, got.ID, wantID)
		}
		if got.Status == "" || got.DispatchKind == "" {
			t.Fatalf("dispatch[%d] missing status/kind: %#v", index, got)
		}
	}
	read, err := service.GetSession(context.Background(), row.SessionID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if err := fse.ValidateDispatchListMatchesSessionProgress(read, listed.Dispatches); err != nil {
		t.Fatalf("ValidateDispatchListMatchesSessionProgress: %v", err)
	}
	hash, err := fixtures.ListDispatchesResultHash(listed)
	if err != nil {
		t.Fatalf("ListDispatchesResultHash: %v", err)
	}
	if hash != wantHash {
		t.Fatalf("dispatch list hash = %q, want %q", hash, wantHash)
	}
}

func assertArtifactListStableSummaries(
	t *testing.T,
	service *fse.FakeService,
	row fixtures.PublishedFixtureScenario,
	wantIDs []string,
	wantHash string,
) {
	t.Helper()
	listed, err := service.ListArtifacts(context.Background(), row.SessionID)
	if err != nil {
		t.Fatalf("ListArtifacts: %v", err)
	}
	if listed.SessionID != row.SessionID {
		t.Fatalf("sessionId = %q, want %q", listed.SessionID, row.SessionID)
	}
	if len(listed.Artifacts) != len(wantIDs) {
		t.Fatalf("artifacts = %#v, want %d rows", listed.Artifacts, len(wantIDs))
	}
	for index, wantID := range wantIDs {
		got := listed.Artifacts[index]
		if got.ID != wantID {
			t.Fatalf("artifact[%d].id = %q, want %q", index, got.ID, wantID)
		}
		if got.Kind == "" || got.ContentHash == "" {
			t.Fatalf("artifact[%d] missing kind/contentHash: %#v", index, got)
		}
		if got.RetrievalRef == nil || got.RetrievalRef.Href == "" {
			t.Fatalf("artifact[%d] missing retrieval ref: %#v", index, got)
		}
		wantHref := "/factory-sessions/" + row.SessionID + "/artifacts/" + wantID
		if got.RetrievalRef.Href != wantHref {
			t.Fatalf("retrieval href = %q, want %q", got.RetrievalRef.Href, wantHref)
		}
	}
	hash, err := fixtures.ListArtifactsResultHash(listed)
	if err != nil {
		t.Fatalf("ListArtifactsResultHash: %v", err)
	}
	if hash != wantHash {
		t.Fatalf("artifact list hash = %q, want %q", hash, wantHash)
	}
}

func assertCanonicalEventEnvelope(t *testing.T, raw json.RawMessage, eventType, id string) {
	t.Helper()
	const schemaVersion = "agent-factory.event.v1"
	var envelope struct {
		SchemaVersion string `json:"schemaVersion"`
		ID            string `json:"id"`
		Type          string `json:"type"`
		Context       struct {
			Sequence int `json:"sequence"`
		} `json:"context"`
		Payload json.RawMessage `json:"payload"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("Unmarshal event: %v", err)
	}
	if envelope.SchemaVersion != schemaVersion {
		t.Fatalf("schemaVersion = %q, want %q", envelope.SchemaVersion, schemaVersion)
	}
	if id != "" && envelope.ID != id {
		t.Fatalf("id = %q, want %q", envelope.ID, id)
	}
	if eventType != "" && envelope.Type != eventType {
		t.Fatalf("type = %q, want %q", envelope.Type, eventType)
	}
	if envelope.Context.Sequence <= 0 {
		t.Fatalf("sequence = %d, want positive", envelope.Context.Sequence)
	}
	if len(envelope.Payload) == 0 {
		t.Fatal("payload missing")
	}
}

func readRuntimeSessionEvents(
	t *testing.T,
	service fse.Service,
	sessionID string,
) (fse.SessionReadResult, fse.ResultReadResult, fse.EventReadResult) {
	t.Helper()
	liveSession, err := service.GetSession(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	liveResult, err := service.GetResult(context.Background(), sessionID, fse.ResultRequest{
		Mode: fse.ResultModeFinal,
	})
	if err != nil {
		t.Fatalf("GetResult: %v", err)
	}
	events, err := service.ReadEvents(context.Background(), sessionID, fse.EventReconnectRequest{})
	if err != nil {
		t.Fatalf("ReadEvents: %v", err)
	}
	if len(events.Events) < 2 {
		t.Fatalf("events = %d, want at least start and result-updated", len(events.Events))
	}
	return liveSession, liveResult, events
}

func assertRuntimeEventSource(t *testing.T, events []json.RawMessage) {
	t.Helper()
	for index, raw := range events {
		var envelope struct {
			Context struct {
				Source *string `json:"source"`
			} `json:"context"`
		}
		if err := json.Unmarshal(raw, &envelope); err != nil {
			t.Fatalf("Unmarshal event[%d]: %v", index, err)
		}
		if envelope.Context.Source == nil || *envelope.Context.Source != runtimeEventSource {
			got := "<nil>"
			if envelope.Context.Source != nil {
				got = *envelope.Context.Source
			}
			t.Fatalf("event[%d].context.source = %q, want %q", index, got, runtimeEventSource)
		}
	}
}

func assertReplayedSessionMatchesLive(t *testing.T, live, replayed fse.SessionReadResult) {
	t.Helper()
	if replayed.SessionID != live.SessionID {
		t.Fatalf("sessionId = %q, want %q", replayed.SessionID, live.SessionID)
	}
	if replayed.Status != live.Status {
		t.Fatalf("status = %q, want %q", replayed.Status, live.Status)
	}
	if replayed.SourceHash != live.SourceHash {
		t.Fatalf("sourceHash = %q, want %q", replayed.SourceHash, live.SourceHash)
	}
	if replayed.Policy.EffectiveHash != live.Policy.EffectiveHash {
		t.Fatalf("policyHash = %q, want %q", replayed.Policy.EffectiveHash, live.Policy.EffectiveHash)
	}
	if !reflect.DeepEqual(replayed.PhaseSummaries, live.PhaseSummaries) {
		t.Fatalf("phase summaries = %#v, want %#v", replayed.PhaseSummaries, live.PhaseSummaries)
	}
	if !reflect.DeepEqual(replayed.LatestCheckpoint, live.LatestCheckpoint) {
		t.Fatalf("latest checkpoint = %#v, want %#v", replayed.LatestCheckpoint, live.LatestCheckpoint)
	}
	if replayed.Links.Session != live.Links.Session {
		t.Fatalf("session link = %q, want %q", replayed.Links.Session, live.Links.Session)
	}
	if replayed.Links.Results != live.Links.Results {
		t.Fatalf("results link = %q, want %q", replayed.Links.Results, live.Links.Results)
	}
	if replayed.Links.Events != live.Links.Events {
		t.Fatalf("events link = %q, want %q", replayed.Links.Events, live.Links.Events)
	}
	if live.ResultSummary != nil {
		if replayed.ResultSummary == nil {
			t.Fatal("replayed resultSummary missing")
		}
		if replayed.ResultSummary.ResultStatus != live.ResultSummary.ResultStatus {
			t.Fatalf("resultSummary.status = %q, want %q", replayed.ResultSummary.ResultStatus, live.ResultSummary.ResultStatus)
		}
	}
}

func assertReplayedResultStatusMatchesLive(t *testing.T, live, replayed fse.ResultReadResult) {
	t.Helper()
	if replayed.ResultStatus != live.ResultStatus {
		t.Fatalf("resultStatus = %q, want %q", replayed.ResultStatus, live.ResultStatus)
	}
	if replayed.SessionStatus != live.SessionStatus {
		t.Fatalf("sessionStatus = %q, want %q", replayed.SessionStatus, live.SessionStatus)
	}
	if live.Availability == nil {
		if replayed.Availability != nil {
			t.Fatalf("replayed availability = %#v, want nil", replayed.Availability)
		}
		return
	}
	if replayed.Availability == nil {
		t.Fatalf("replayed availability missing, want %#v", live.Availability)
	}
	if replayed.Availability.Reason != live.Availability.Reason {
		t.Fatalf("availability.reason = %q, want %q", replayed.Availability.Reason, live.Availability.Reason)
	}
	if replayed.Availability.Message != live.Availability.Message {
		t.Fatalf("availability.message = %q, want %q", replayed.Availability.Message, live.Availability.Message)
	}
	if replayed.Availability.Retryable != live.Availability.Retryable {
		t.Fatalf("availability.retryable = %v, want %v", replayed.Availability.Retryable, live.Availability.Retryable)
	}
}

func assertReplayedResultMatchesEventProjection(t *testing.T, replayed fse.ResultReadResult, events []json.RawMessage) {
	t.Helper()
	if err := fse.ValidateResultMatchesEventProjection(replayed, events); err != nil {
		t.Fatalf("ValidateResultMatchesEventProjection: %v", err)
	}
}

func assertReplayedResultMatchesSessionRead(t *testing.T, session fse.SessionReadResult, result fse.ResultReadResult) {
	t.Helper()
	if err := fse.ValidateResultMatchesSessionRead(session, result); err != nil {
		t.Fatalf("ValidateResultMatchesSessionRead: %v", err)
	}
}

func assertReplayProjectionStable(
	t *testing.T,
	firstSession, secondSession fse.SessionReadResult,
	firstResult, secondResult fse.ResultReadResult,
) {
	t.Helper()
	if firstSession.Status != secondSession.Status {
		t.Fatalf("status drift = %q vs %q", firstSession.Status, secondSession.Status)
	}
	if firstSession.SourceHash != secondSession.SourceHash {
		t.Fatalf("sourceHash drift = %q vs %q", firstSession.SourceHash, secondSession.SourceHash)
	}
	if firstSession.Policy.EffectiveHash != secondSession.Policy.EffectiveHash {
		t.Fatalf("policyHash drift = %q vs %q", firstSession.Policy.EffectiveHash, secondSession.Policy.EffectiveHash)
	}
	if firstSession.Links.Session != secondSession.Links.Session {
		t.Fatalf("links drift = %q vs %q", firstSession.Links.Session, secondSession.Links.Session)
	}
	if len(firstSession.ArtifactRefs) != len(secondSession.ArtifactRefs) {
		t.Fatalf("artifact stub drift = %d vs %d", len(firstSession.ArtifactRefs), len(secondSession.ArtifactRefs))
	}
	if firstResult.ResultStatus != secondResult.ResultStatus {
		t.Fatalf("resultStatus drift = %q vs %q", firstResult.ResultStatus, secondResult.ResultStatus)
	}
}

type interruptedResumableHarness struct {
	projectRoot string
	provider    *sequentialBlockingProvider
	initial     fse.Service
	sessionID   string
	interrupted fse.SessionReadResult
	summary     *factory.JavaScriptCheckpointSummary
}

func startInterruptedResumableSession(t *testing.T, requestID string) interruptedResumableHarness {
	t.Helper()
	return startInterruptedResumableSessionForWorkflow(
		t,
		requestID,
		"resumable-two-step-fake-children.workflow.js",
		"resumable-two-step-fake-children",
	)
}

func startInterruptedCheckpointStateBranchSession(t *testing.T, requestID string) interruptedResumableHarness {
	t.Helper()
	return startInterruptedResumableSessionForWorkflow(
		t,
		requestID,
		"resumable-checkpoint-state-branch.workflow.js",
		"resumable-checkpoint-state-branch",
	)
}

func startInterruptedResumableSessionForWorkflow(
	t *testing.T,
	requestID string,
	workflowFile string,
	workflowName string,
) interruptedResumableHarness {
	t.Helper()

	provider := newSequentialBlockingProviderForWorkflow(workflowName)
	projectRoot := setupRuntimeWorkflowFixture(t, workflowFile, workflowName)
	checkpointSummary := checkpointfixtures.ResumableCheckpointSummaryResult()
	if workflowName == "resumable-live-child-output" {
		checkpointSummary = checkpointfixtures.ReplayFirstChildCheckpointSummaryResult()
	}
	initial := newConfiguredJavaScriptRuntimeService(runtimeServiceConfig{
		ProjectRoot:       projectRoot,
		ChildExecutorMode: fse.ChildExecutorModeLive,
		ProviderExecutor:  provider,
		Persistence:       runtimePersistence(projectRoot),
		CheckpointSummary: checkpointSummary,
	})

	started, err := initial.StartAsync(context.Background(), fse.StartRequest{
		RequestID: requestID,
		Source: fse.Source{
			Kind:         factory.WorkflowSourceKindWorkflowName,
			WorkflowName: workflowName,
		},
		Args: map[string]any{
			"subject": "workflows",
		},
		Runtime: &fse.RuntimeOptions{
			ChildExecutorMode: fse.ChildExecutorModeLive,
		},
	})
	if err != nil {
		t.Fatalf("StartAsync: %v", err)
	}

	waitForDispatchStatus(t, initial, started.SessionID, "dispatch-1", fse.DispatchStatusCompleted, 10*time.Second)
	waitForDispatchStatus(t, initial, started.SessionID, "dispatch-2", fse.DispatchStatusRunning, 10*time.Second)
	provider.waitUntilBlockedOnInfer(t, 10*time.Second)

	interruptResult, err := initial.InterruptDispatch(context.Background(), started.SessionID, fse.InterruptDispatchRequest{
		ControlRequest: fse.ControlRequest{Reason: "process restart simulation"},
		DispatchID:     "dispatch-2",
	})
	if err != nil {
		t.Fatalf("InterruptDispatch: %v", err)
	}
	if interruptResult.Outcome != fse.LifecycleControlOutcomeAccepted {
		t.Fatalf("interrupt outcome = %q, want ACCEPTED", interruptResult.Outcome)
	}

	provider.waitForCanceledInfer(t, 10*time.Second)
	interrupted := waitUntilSessionStatus(t, initial, started.SessionID, fse.LifecycleStatusInterrupted, 5*time.Second)
	if interrupted.Status != fse.LifecycleStatusInterrupted {
		t.Fatalf("session status = %q, want INTERRUPTED", interrupted.Status)
	}

	return interruptedResumableHarness{
		projectRoot: projectRoot,
		provider:    provider,
		initial:     initial,
		sessionID:   started.SessionID,
		interrupted: interrupted,
		summary:     checkpointSummary,
	}
}

func newResumedRuntimeService(harness interruptedResumableHarness) *fse.JavaScriptRuntimeService {
	return newConfiguredJavaScriptRuntimeService(runtimeServiceConfig{
		ProjectRoot:       harness.projectRoot,
		ChildExecutorMode: fse.ChildExecutorModeLive,
		ProviderExecutor:  harness.provider,
		Persistence:       runtimePersistence(harness.projectRoot),
		CheckpointSummary: harness.summary,
	})
}

func resumeInterruptedHarness(t *testing.T, harness interruptedResumableHarness, requestID string) *fse.JavaScriptRuntimeService {
	t.Helper()
	resumedService := newResumedRuntimeService(harness)
	resumed, err := resumedService.ResumeInterruptedSession(context.Background(), harness.sessionID, fse.ResumeSessionRequest{
		RequestID: requestID,
	})
	if err != nil {
		t.Fatalf("ResumeInterruptedSession: %v", err)
	}
	if resumed.SessionID != harness.sessionID {
		t.Fatalf("resumed sessionId = %q, want %q", resumed.SessionID, harness.sessionID)
	}
	if resumed.Status != string(fse.LifecycleStatusResuming) && resumed.Status != string(fse.LifecycleStatusSucceeded) {
		t.Fatalf("resumed start status = %q, want RESUMING or SUCCEEDED", resumed.Status)
	}
	return resumedService
}

func assertInterruptedLifecycleHasTimestamp(t *testing.T, lifecycle *fse.LifecycleTimestamps) {
	t.Helper()
	if lifecycle == nil || lifecycle.InterruptedAt == nil {
		t.Fatalf("interrupted lifecycle = %#v, want interruptedAt", lifecycle)
	}
}

func assertResumedSessionReadSurfaces(t *testing.T, success fse.SessionReadResult) {
	t.Helper()
	if success.Lifecycle == nil || success.Lifecycle.InterruptedAt == nil || success.Lifecycle.ResumedAt == nil {
		t.Fatalf("resumed lifecycle = %#v, want interruptedAt and resumedAt", success.Lifecycle)
	}
	if success.Lifecycle.FinishedAt == nil {
		t.Fatal("expected finishedAt on succeeded resumed session")
	}
	if success.Progress == nil || success.Progress.CompletedDispatches != 2 {
		t.Fatalf("progress = %#v, want 2 completed dispatches", success.Progress)
	}
	if success.ResultSummary == nil || success.ResultSummary.ResultStatus != string(fse.ResultStatusFinal) {
		t.Fatalf("resultSummary = %#v, want FINAL", success.ResultSummary)
	}
}

func assertResumedResultAndDispatches(t *testing.T, service *fse.JavaScriptRuntimeService, sessionID string) {
	t.Helper()
	result, err := service.GetResult(context.Background(), sessionID, fse.ResultRequest{
		Mode: fse.ResultModeFinal,
	})
	if err != nil {
		t.Fatalf("GetResult: %v", err)
	}
	if result.ResultStatus != fse.ResultStatusFinal || result.SessionStatus != fse.LifecycleStatusSucceeded {
		t.Fatalf("result = %#v, want FINAL/SUCCEEDED", result)
	}

	dispatches, err := service.ListDispatches(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("ListDispatches: %v", err)
	}
	if len(dispatches.Dispatches) != 2 {
		t.Fatalf("dispatch count = %d, want 2", len(dispatches.Dispatches))
	}
	for _, dispatch := range dispatches.Dispatches {
		if dispatch.Status != fse.DispatchStatusCompleted {
			t.Fatalf("dispatch %s = %#v, want COMPLETED", dispatch.ID, dispatch)
		}
	}
}

func assertResumedReplayProjection(t *testing.T, events []json.RawMessage) {
	t.Helper()
	replayedSession, replayedResult, err := fse.ReplaySessionProjection(events)
	if err != nil {
		t.Fatalf("ReplaySessionProjection: %v", err)
	}
	if replayedSession.Status != fse.LifecycleStatusSucceeded {
		t.Fatalf("replayed status = %q, want SUCCEEDED", replayedSession.Status)
	}
	if replayedResult.ResultStatus != fse.ResultStatusFinal {
		t.Fatalf("replayed result = %q, want FINAL", replayedResult.ResultStatus)
	}
	if replayedSession.Lifecycle == nil || replayedSession.Lifecycle.ResumedAt == nil {
		t.Fatalf("replayed lifecycle = %#v, want resumedAt", replayedSession.Lifecycle)
	}
}

func assertResumedReconnectEvents(t *testing.T, service *fse.JavaScriptRuntimeService, sessionID string, events []json.RawMessage) {
	t.Helper()
	reconnect, err := service.ReadEvents(context.Background(), sessionID, reconnectAfterFirstEvent(t, events))
	if err != nil {
		t.Fatalf("ReadEvents reconnect: %v", err)
	}
	if len(reconnect.Events) == 0 {
		t.Fatal("expected reconnect-filtered events after first event id")
	}
}

type sequentialBlockingProvider struct {
	mu              sync.Mutex
	calls           int
	blockedOnce     bool
	contextCanceled int
	workflowName    string
	blockedOnCtx    chan struct{}
	blockSignal     sync.Once
}

func newSequentialBlockingProvider() *sequentialBlockingProvider {
	return newSequentialBlockingProviderForWorkflow("resumable-two-step-fake-children")
}

func newSequentialBlockingProviderForWorkflow(workflowName string) *sequentialBlockingProvider {
	return &sequentialBlockingProvider{
		workflowName: workflowName,
		blockedOnCtx: make(chan struct{}),
	}
}

func (p *sequentialBlockingProvider) Execute(
	ctx context.Context,
	input workerexecution.InvocationInput,
) (workerexecution.InvocationResult, error) {
	p.mu.Lock()
	p.calls++
	call := p.calls
	alreadyBlocked := p.blockedOnce
	p.mu.Unlock()

	if call == 1 {
		response := workerexecution.InferenceResponse{
			Content: fmt.Sprintf(`{"text":"live:%s:step-one:step-one:workflows","label":"step-one"}`, p.workflowName),
			ProviderSession: &workerexecution.ProviderSessionMetadata{
				Provider: "mock",
				Kind:     "session_id",
				ID:       "live-provider-session-1",
			},
		}
		return workerexecution.InvocationResult{
			Response: response, Attempt: input.Attempt,
			ProviderSession: workerexecution.CloneProviderSessionMetadata(response.ProviderSession),
		}, nil
	}

	if !alreadyBlocked {
		p.mu.Lock()
		p.blockedOnce = true
		p.mu.Unlock()

		p.blockSignal.Do(func() { close(p.blockedOnCtx) })
		<-ctx.Done()
		p.mu.Lock()
		p.contextCanceled++
		p.mu.Unlock()
		return workerexecution.InvocationResult{Attempt: input.Attempt}, ctx.Err()
	}

	response := workerexecution.InferenceResponse{
		Content: fmt.Sprintf(`{"text":"live:%s:step-two:step-two:workflows","label":"step-two"}`, p.workflowName),
		ProviderSession: &workerexecution.ProviderSessionMetadata{
			Provider: "mock",
			Kind:     "session_id",
			ID:       "live-provider-session-2",
		},
	}
	return workerexecution.InvocationResult{
		Response: response, Attempt: input.Attempt,
		ProviderSession: workerexecution.CloneProviderSessionMetadata(response.ProviderSession),
	}, nil
}

func (p *sequentialBlockingProvider) CallCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls
}

var _ workers.InvocationExecutor = (*sequentialBlockingProvider)(nil)

func (p *sequentialBlockingProvider) waitUntilBlockedOnInfer(t *testing.T, timeout time.Duration) {
	t.Helper()
	select {
	case <-p.blockedOnCtx:
		return
	case <-time.After(timeout):
		t.Fatal("provider Infer did not block on workflow context")
	}
}

func (p *sequentialBlockingProvider) waitForCanceledInfer(t *testing.T, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		p.mu.Lock()
		canceled := p.contextCanceled
		p.mu.Unlock()
		if canceled > 0 {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("provider Infer did not observe canceled workflow context")
}

func waitForDispatchStatus(
	t *testing.T,
	service fse.Service,
	sessionID string,
	dispatchID string,
	want fse.DispatchStatus,
	timeout time.Duration,
) fse.DispatchSummary {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		listed, err := service.ListDispatches(context.Background(), sessionID)
		if err != nil {
			t.Fatalf("ListDispatches: %v", err)
		}
		for _, dispatch := range listed.Dispatches {
			if dispatch.ID != dispatchID {
				continue
			}
			if dispatch.Status == want {
				return dispatch
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("dispatch %q did not reach status %q within %s", dispatchID, want, timeout)
	return fse.DispatchSummary{}
}
