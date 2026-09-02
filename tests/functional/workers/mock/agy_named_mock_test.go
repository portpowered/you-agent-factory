package mock

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const namedAgyMockModel = "gemini-3.6-flash-high"

// testNamedAgyMockPreservesDispatchMetadataAndCompletionLog proves the
// Workers-owned mock feature through the canonical root process. The named
// mock intercepts an Antigravity attempt, retains its configured exit code in
// the command completion log, and keeps the source Work correlation without
// invoking the replaceable live ProviderCommandRunner edge.
func testNamedAgyMockPreservesDispatchMetadataAndCompletionLog(
	t *testing.T,
	fixture *sharedWorkersMockFixture,
) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	support.WriteAgentConfig(t, dir, "worker", support.BuildModelWorkerConfig(
		modelprovider.ProviderAntigravity,
		namedAgyMockModel,
	))
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		RequestID:  "agy-mock-request",
		WorkID:     "agy-mock-work",
		Name:       "agy-mock-work",
		WorkTypeID: "task",
		TraceID:    "agy-mock-trace",
		Payload:    []byte(`{"title":"named Agy mock"}`),
	})

	liveRunner := support.NewRecordingCommandRunner("live Agy edge must not run")
	fixture.useCommandRunnersFor(
		t,
		dir,
		liveRunner,
		support.NewRecordingCommandRunner("live script edge must not run"),
	)
	session := fixture.openSession(t, dir)
	listed, events := session.terminalObservations(t, 20*time.Second)
	defer session.closeAndAssertGone(t)

	if liveRunner.CallCount() != 0 {
		t.Fatalf("live Agy edge calls = %d, want zero for named mock dispatch", liveRunner.CallCount())
	}
	if got := support.CountWorkAtCustomerState(listed, support.WorkCustomerLocation("task", "failed")); got != 1 {
		t.Fatalf("Agy mock failed Work count = %d, want 1", got)
	}
	dispatches := support.ObserveDispatchEvents(t, events)
	if len(dispatches) != 1 || dispatches[0].Response == nil || dispatches[0].Response.Outcome != factoryapi.WorkOutcomeFailed {
		t.Fatalf("Agy mock dispatches = %#v, want one failed dispatch", dispatches)
	}
	if !support.DispatchObservationIncludesWork(dispatches[0], "agy-mock-work") {
		t.Fatalf("Agy mock dispatch = %#v, want work correlation", dispatches[0])
	}
	record := requireSharedWorkersMockRuntimeLogRecord(
		t,
		fixture.runtimeLogDir,
		"command_runner.completed",
		"agy-mock-request",
	)
	if record["exit_code"] != float64(7) {
		t.Fatalf("logged Agy exit_code = %#v, want 7", record["exit_code"])
	}
	for key, want := range map[string]any{
		"request_id": "agy-mock-request",
		"trace_id":   "agy-mock-trace",
		"work_id":    "agy-mock-work",
	} {
		if record[key] != want {
			t.Fatalf("logged Agy %s = %#v, want %q", key, record[key], want)
		}
	}
}
