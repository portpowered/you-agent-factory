package factorysessionexecution

import (
	"context"
	"strings"
	"testing"

	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

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
