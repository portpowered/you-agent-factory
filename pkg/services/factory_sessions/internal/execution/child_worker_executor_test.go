package factorysessionexecution

import (
	"context"
	"errors"
	"testing"

	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// TestChildWorkerExecutor_CompletedChildRecordsItsWorkerAndOutput pins the
// record a customer reads for a child that ran: its provider, its provider
// session, its decoded output, and the attempt count the Worker actually took.
func TestChildWorkerExecutor_CompletedChildRecordsItsWorkerAndOutput(t *testing.T) {
	invoker := &recordingWorkerInvoker{result: factory.InvokeWorkerResult{
		Outcome:            factory.InvokeWorkerOutcomeCompleted,
		Output:             `{"text":"child finished"}`,
		Provider:           "codex",
		ProviderSessionRef: "codex-session-1",
		Attempts:           2,
	}}
	sink := newChildRecordSink()
	executor := newTestChildWorkerExecutor(invoker, sink, nil)

	result, err := executor.Execute(context.Background(), factory.JavaScriptChildExecutionRequest{
		Prompt:        "summarize",
		Label:         "summarize-findings",
		ModelProvider: "codex",
		OutputSchema:  map[string]any{"type": "object"},
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Status != factory.JavaScriptChildDispatchStatusCompleted {
		t.Fatalf("child status = %q, want COMPLETED", result.Status)
	}
	if got := result.Output["text"]; got != "child finished" {
		t.Fatalf("child output text = %v, want the provider's decoded content", got)
	}

	terminal := sink.terminalChildDispatch(t)
	if terminal.Provider != "codex" || terminal.ProviderSessionRef != "codex-session-1" {
		t.Fatalf("terminal record provider = %q/%q, want codex/codex-session-1",
			terminal.Provider, terminal.ProviderSessionRef)
	}
	if terminal.Attempt != 2 {
		t.Fatalf("terminal record attempt = %d, want the Worker's own attempt count 2", terminal.Attempt)
	}
	if len(sink.statuses) != 2 ||
		sink.statuses[0] != factory.JavaScriptChildDispatchStatusQueued ||
		sink.statuses[1] != factory.JavaScriptChildDispatchStatusRunning {
		t.Fatalf("dispatch statuses = %v, want QUEUED then RUNNING before the terminal record", sink.statuses)
	}
}

// TestChildWorkerExecutor_FailedChildCarriesItsProviderWithTheSessionReference
// is the regression pin for the defect that turned a crashed provider into an
// internal error.
//
// A provider session reference without its provider is rejected when the
// session's runtime facts are mapped to canonical events, and that failure is
// not scoped to the child -- it fails the whole execution. A crashed ACP peer
// surfaced as HTTP 500 rather than a FAILED session.
func TestChildWorkerExecutor_FailedChildCarriesItsProviderWithTheSessionReference(t *testing.T) {
	retryable := true
	invoker := &recordingWorkerInvoker{result: factory.InvokeWorkerResult{
		Outcome:            factory.InvokeWorkerOutcomeFailed,
		Provider:           "codex",
		ProviderSessionRef: "codex-session-1",
		Diagnostic:         "the provider exited before completing",
		FailureReason:      string(workers.WorkFailureTypeInternalServerError),
		Retryable:          &retryable,
	}}
	sink := newChildRecordSink()
	executor := newTestChildWorkerExecutor(invoker, sink, nil)

	result, err := executor.Execute(context.Background(), factory.JavaScriptChildExecutionRequest{
		Prompt: "summarize",
	})
	if err == nil {
		t.Fatal("Execute error = nil, want the child's failure surfaced to the workflow")
	}
	if result.Status != factory.JavaScriptChildDispatchStatusFailed {
		t.Fatalf("child status = %q, want FAILED", result.Status)
	}

	terminal := sink.terminalChildDispatch(t)
	if terminal.ProviderSessionRef == "" {
		t.Fatal("terminal record provider session ref = empty, want the observed reference")
	}
	if terminal.Provider == "" {
		t.Fatal("terminal record provider = empty; a session reference without its provider " +
			"fails canonical event mapping and takes the whole execution with it")
	}
	if terminal.FailureClassification != workers.WorkFailureTypeInternalServerError {
		t.Fatalf("failure classification = %q, want the Workers-owned classification",
			terminal.FailureClassification)
	}
	if terminal.Retryable == nil || !*terminal.Retryable {
		t.Fatalf("retryable = %#v, want the classification's retry verdict", terminal.Retryable)
	}
	if terminal.FailureDetail == nil || terminal.FailureDetail.Message != "the provider exited before completing" {
		t.Fatalf("failure detail = %#v, want the bounded diagnostic", terminal.FailureDetail)
	}
}

// TestChildWorkerExecutor_InvocationErrorStillRecordsAFailedChild proves a
// Worker that could not be invoked at all is still a failed child rather than
// an unrecorded one: the session already committed QUEUED and RUNNING, and
// leaving those without a terminal would strand the dispatch forever.
func TestChildWorkerExecutor_InvocationErrorStillRecordsAFailedChild(t *testing.T) {
	invoker := &recordingWorkerInvoker{err: errors.New("worker sessions unavailable")}
	sink := newChildRecordSink()
	executor := newTestChildWorkerExecutor(invoker, sink, nil)

	result, err := executor.Execute(context.Background(), factory.JavaScriptChildExecutionRequest{Prompt: "go"})
	if err == nil {
		t.Fatal("Execute error = nil, want the invocation failure surfaced")
	}
	if result.Status != factory.JavaScriptChildDispatchStatusFailed {
		t.Fatalf("child status = %q, want FAILED", result.Status)
	}
	if got := sink.terminalChildDispatch(t).Status; got != factory.JavaScriptChildDispatchStatusFailed {
		t.Fatalf("terminal record status = %q, want FAILED", got)
	}
}

// TestChildWorkerExecutor_ScopesTheWorkersIdentityAndReleasesItAfterTheWorker
// pins both halves of the Workers identity contract: the identity handed to
// Workers is scoped to this session, and the claim that routes the Worker's
// progress back is released once the Worker is terminal.
func TestChildWorkerExecutor_ScopesTheWorkersIdentityAndReleasesItAfterTheWorker(t *testing.T) {
	invoker := &recordingWorkerInvoker{result: factory.InvokeWorkerResult{
		Outcome: factory.InvokeWorkerOutcomeCompleted,
	}}
	sink := newChildRecordSink()
	var observed []string
	var releasedWhileRunning bool
	released := 0
	executor := newTestChildWorkerExecutor(invoker, sink, func(workerDispatchID, sessionID string) func() {
		observed = append(observed, workerDispatchID+"@"+sessionID)
		return func() { released++ }
	})
	invoker.onInvoke = func() { releasedWhileRunning = released > 0 }

	if _, err := executor.Execute(context.Background(), factory.JavaScriptChildExecutionRequest{Prompt: "go"}); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(observed) != 1 || observed[0] != "dur-sess-1/dispatch-1@dur-sess-1" {
		t.Fatalf("observed Worker dispatch = %v, want the session-scoped identity", observed)
	}
	if invoker.request.DispatchID != "dur-sess-1/dispatch-1" {
		t.Fatalf("Workers dispatch ID = %q, want the session-scoped identity", invoker.request.DispatchID)
	}
	if releasedWhileRunning {
		t.Fatal("the Worker's progress claim was released while the Worker was still running")
	}
	if released != 1 {
		t.Fatalf("claim releases = %d, want exactly one once the Worker is terminal", released)
	}
	// The session's own record keeps the identity its customer sees.
	if got := sink.terminalChildDispatch(t).DispatchID; got != "dispatch-1" {
		t.Fatalf("recorded dispatch ID = %q, want the session's own unqualified identity", got)
	}
}

// TestChildWorkerExecutor_CarriesTheAuthoredWorkerNameAndPermissionPolicy
// keeps the two selections a mock-worker configuration and the provider both
// depend on attached to the Worker itself.
func TestChildWorkerExecutor_CarriesTheAuthoredWorkerNameAndPermissionPolicy(t *testing.T) {
	invoker := &recordingWorkerInvoker{result: factory.InvokeWorkerResult{
		Outcome: factory.InvokeWorkerOutcomeCompleted,
	}}
	executor := newTestChildWorkerExecutor(invoker, newChildRecordSink(), nil)

	if _, err := executor.Execute(context.Background(), factory.JavaScriptChildExecutionRequest{
		Prompt:          "go",
		Preset:          "worker-a",
		SkipPermissions: true,
		ModelProvider:   "codex",
	}); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if invoker.request.WorkerName != "worker-a" {
		t.Fatalf("worker name = %q, want the authored preset", invoker.request.WorkerName)
	}
	if !invoker.request.SkipPermissions {
		t.Fatal("skip-permissions = false, want the child's resolved policy")
	}
	if invoker.request.RunnerID != "codex" {
		t.Fatalf("runner = %q, want the runner resolved from the child's model provider", invoker.request.RunnerID)
	}
}

func newTestChildWorkerExecutor(
	invoke factory.Service,
	sink *childRecordSink,
	observe workerDispatchObserver,
) *childWorkerExecutor {
	return newChildWorkerExecutor("dur-sess-1", invoke, sink, childTestValues{}, observe, 0, "/project")
}

// recordingWorkerInvoker is the Factory Runtime seam a child reaches its Worker
// through. Only InvokeWorker carries behavior; the rest of the root contract is
// present because the child holds the whole service, not a narrowed slice.
type recordingWorkerInvoker struct {
	factory.Service

	request  factory.InvokeWorkerRequest
	result   factory.InvokeWorkerResult
	err      error
	onInvoke func()
}

func (i *recordingWorkerInvoker) InvokeWorker(
	_ context.Context,
	req factory.InvokeWorkerRequest,
) (factory.InvokeWorkerResult, error) {
	i.request = req
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
