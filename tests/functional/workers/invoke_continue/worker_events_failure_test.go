package acceptance

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	api "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// The existing durable-writer edge refuses later appends for t7-degraded IDs.
// Restart is sequential because archived reads must have no live Events topic
// or supervisor. Independent private profiles can still execute in parallel.
func TestWorkerEventsDegradedReplayRestart(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	root, dir := t.TempDir(), t.TempDir()
	hostDir, home, err := prepareInvokeContinuePackageRoot(t, root)
	if err != nil {
		t.Fatal(err)
	}
	const id = "t7-degraded-events"
	runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{Stdout: directCodexSessionOutput("degraded-events-thread", "uncaptured later output")})
	route := &invokeContinueStaticCommandRoute{routes: []invokeContinueStaticCommandRouteEntry{{workingDirectory: dir, runner: runner}}}
	first := startContinuationRestartHost(t, root, hostDir, home, route)
	path := filepath.Join(dir, "execution.json")
	writeInvokeContinueExecutionSpec(t, path, invokeContinueExecutionSpec{requestID: id + "-request", workerSessionID: id, dispatchID: id + "-attempt", workingDirectory: dir, userMessage: id})
	summaryRestartCLI(t, first, home, dir, "invoke", "--execution", path)
	before := support.GetJSON[api.WorkerSessionLogPage](t, first.baseURL+"/worker-sessions/"+id+"/logs")
	if before.Health != "DEGRADED" || len(before.Events) != 2 {
		t.Fatalf("controlled append failure lost its committed prefix: %+v", before)
	}
	if err := first.command.stop(); err != nil {
		t.Fatal(err)
	}
	if err := first.process.Close(ctx); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{".codex", ".cursor", ".claude"} {
		if err := os.WriteFile(filepath.Join(home, name), []byte("provider files unavailable"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	fresh := startContinuationRestartHost(t, root, hostDir, home, route)
	endpoint := fresh.baseURL + "/worker-sessions/" + id
	for _, query := range []string{"?replayOnly=true&limit=1", "?limit=1"} {
		frames := readWorkerEventsFinite(t, ctx, endpoint+"/events"+query)
		assertWorkerEventsDegradedPrefix(t, frames, before.Events, id)
	}
	after := support.GetJSON[api.WorkerSessionLogPage](t, endpoint+"/logs")
	if after.Health != before.Health || !reflect.DeepEqual(after.Events, before.Events) {
		t.Fatalf("restart changed committed capture health/content: before=%+v after=%+v", before, after)
	}
	active := summaryRestartCLI(t, fresh, home, dir, "list", "--history", "active")
	var rows api.ListWorkerSessionsResponse
	decodeDirectWorkerSessionResult(t, active, &rows)
	if len(rows.Sessions) != 0 || runner.CallCount() != 1 {
		t.Fatalf("archived reads restored execution authority: %s calls=%d", active, runner.CallCount())
	}
	assertWorkerEventsCorruptSetup(t, ctx, fresh, root, hostDir, home, id, route)
}

func assertWorkerEventsCorruptSetup(t *testing.T, ctx context.Context, fresh invokeContinueStartedProcess, root, hostDir, home, id string, route *invokeContinueStaticCommandRoute) {
	t.Helper()
	// A complete malformed line is a source failure, distinct from a salvageable
	// interrupted tail. Reconstruct through a new joined host to avoid testing
	// whether an already hydrated recording is invalidated by external writes.
	journals, err := filepath.Glob(filepath.Join(hostDir, ".you-agent-factory", "worker-recordings", "*.worker.jsonl"))
	if err != nil || len(journals) != 1 {
		t.Fatalf("private recording fixture: %v %v", journals, err)
	}
	if err := fresh.command.stop(); err != nil {
		t.Fatal(err)
	}
	if err := fresh.process.Close(ctx); err != nil {
		t.Fatal(err)
	}
	journal, err := os.OpenFile(journals[0], os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	_, writeErr := journal.WriteString("private malformed capture line\n")
	closeErr := journal.Close()
	if writeErr != nil || closeErr != nil {
		t.Fatalf("source failure fixture: %v %v", writeErr, closeErr)
	}
	failed := startContinuationRestartHost(t, root, hostDir, home, route)
	assertWorkerEventsDirectDenial(t, ctx, failed.baseURL+"/worker-sessions/"+id+"/events", http.StatusInternalServerError, api.ErrorResponseCodePROJECTIONUNAVAILABLE)
}

func assertWorkerEventsDegradedPrefix(t *testing.T, frames []api.WorkerSessionEvent, want []api.WorkerSessionEvent, id string) {
	t.Helper()
	var records []api.WorkerSessionEventRecord
	var summary *api.WorkerSessionReplaySummary
	for _, frame := range frames {
		if frame.WorkerSessionId != id || frame.FactorySessionId != nil || len(frame.WorkIds) != 0 || frame.Delivery == api.WorkerSessionEventDeliveryTerminalReplay || frame.Delivery == api.WorkerSessionEventDeliverySourceFailure {
			t.Fatalf("degraded replay fabricated identity/completion: %+v", frame)
		}
		if frame.Event.Position > 0 {
			records = append(records, frame.Event)
		}
		if frame.ReplaySummary != nil {
			summary = frame.ReplaySummary
		}
	}
	if !reflect.DeepEqual(records, workerEventsFailureRecords(want)) || summary == nil || summary.Complete || summary.EventsEmitted != int64(len(want)) || summary.Reason != "capture-DEGRADED" {
		t.Fatalf("degraded replay changed prefix or health: records=%+v summary=%+v", records, summary)
	}
	raw, err := json.Marshal(frames)
	if err != nil || strings.Contains(string(raw), "uncaptured later output") || strings.Contains(string(raw), "private-t7-capture-failure") {
		t.Fatalf("degraded replay leaked uncaptured output or private failure: %s (%v)", raw, err)
	}
}

func workerEventsFailureRecords(frames []api.WorkerSessionEvent) []api.WorkerSessionEventRecord {
	var records []api.WorkerSessionEventRecord
	for _, frame := range frames {
		if frame.Event.Position > 0 {
			records = append(records, frame.Event)
		}
	}
	return records
}
