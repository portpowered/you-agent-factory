package recording_summary_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/builtcliacceptance"
	"github.com/portpowered/infinite-you/internal/testutil"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// F03 crosses real capture admission and HTTP composition. Direct Workers have
// no Factory Session. Both identities share one immutable command-runner shape;
// only the rejected identity's storage boundary refuses usage. Restart removes
// live observations so they cannot conceal uncommitted archived facts.
func TestArchivedSummaryExcludesRejectedProviderUsage(t *testing.T) {
	t.Parallel()
	profile, home := t.TempDir(), t.TempDir()
	factory := support.ScaffoldSingleStepFactory(t, "rejected-usage")
	runner := testutil.NewProviderCommandRunner(
		platformprocess.CommandResult{Stdout: support.CodexSuccessStdoutWithUsage("committed COMPLETE", 0, 0)},
		platformprocess.CommandResult{Stdout: support.CodexSuccessStdoutWithUsage("rejected COMPLETE", 999, 777)},
	)
	store := &rejectUsageStore{}
	env := builtcliacceptance.ProcessEnvForIsolatedHome(home)
	host := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: factory, WorkingDirectory: profile, Env: env, ServerReadyTimeout: time.Minute,
		Edges: serviceedges.Edges{
			FactorySessionsWorkingDirectory: platformfilesystem.Local{WorkingDirectory: profile},
			ProviderCommandRunner:           runner,
			WorkerRecordingWriter:           store,
			WorkerRecordingStoreObserver:    func(value recordings.WorkerRecordingStore) { store.WorkerRecordingStore = value },
		},
	})
	for _, id := range []string{"committed-usage", "rejected-usage"} {
		invokeSummaryWorker(t, host, profile, env, id)
	}
	if store.rejected.Load() != 1 || runner.CallCount() != 2 {
		t.Fatalf("fault/provider edge not exercised: refusals=%d calls=%d", store.rejected.Load(), runner.CallCount())
	}
	host.Close(t)
	gate := &readGate{}
	archived := startHost(t, factory, profile, gate)
	gate.denied.Store(true)
	for range 3 {
		committed := readSummary(t, archived.URL()+"/worker-sessions/committed-usage", http.StatusOK)
		if committed.TokenUsage == nil || committed.TokenUsage.InputTokens == nil || *committed.TokenUsage.InputTokens != 0 {
			t.Fatalf("committed explicit zero lost: %+v", committed)
		}
		got := readSummary(t, archived.URL()+"/worker-sessions/rejected-usage", http.StatusOK)
		if got.WorkerSessionId != "rejected-usage" || got.Model == nil || *got.Model != "functional-model" || got.TokenUsage != nil || got.RecordingHealth == nil || string(*got.RecordingHealth) != "DEGRADED" || string(got.State) != "COMPLETED" || got.TerminalCause == nil || string(*got.TerminalCause) != "COMPLETED" {
			t.Fatalf("archived summary advertised rejected usage or lost committed facts: %+v", got)
		}
	}
	if gate.attempts.Load() != 0 {
		t.Fatalf("archived detail attempted %d recording reads", gate.attempts.Load())
	}
}

// The approved durable-writer edge refuses one selected record before commit,
// delegating every other record and the safe failure marker to the real store.
type rejectUsageStore struct {
	recordings.WorkerRecordingStore
	rejected atomic.Int64
}

func (store *rejectUsageStore) PersistWorkerRecord(ctx context.Context, record recordings.WorkerRecordingRecord) error {
	var draft workers.Draft
	if record.WorkerSessionID == "rejected-usage" && json.Unmarshal(record.Record.Payload, &draft) == nil && draft.Kind == workers.KindUsage {
		var usage map[string]json.RawMessage
		if json.Unmarshal(draft.Payload, &usage) == nil && usage["inputTokens"] != nil {
			store.rejected.Add(1)
			return errors.New("controlled usage append refusal")
		}
	}
	return store.WorkerRecordingStore.PersistWorkerRecord(ctx, record)
}

func invokeSummaryWorker(t *testing.T, host *support.FunctionalAPIServer, dir string, env []string, id string) {
	t.Helper()
	document := map[string]any{
		"requestId": id, "workerSessionId": id,
		"execution": map[string]any{
			"workstationName": "direct", "workingDirectory": dir, "workerType": "direct-worker",
			"runnerId": "codex", "executorProvider": "codex", "modelProvider": "codex",
			"model": "functional-model", "userMessage": "summary proof",
			"dispatch": map[string]any{"dispatchId": id + "-attempt", "workstationName": "direct", "workerType": "direct-worker"},
		},
	}
	data, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, id+".json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	input := support.FakeInputs(t.Context(), []string{"you", "--json", "worker-sessions", "invoke", "--execution", path})
	input.Input.Env, input.Input.WorkingDirectory = env, dir
	if err := host.Execute(t, input.Input); err != nil {
		t.Fatalf("invoke %s: %v stdout=%s stderr=%s", id, err, input.Stdout(), input.Stderr())
	}
	var result struct {
		State           string `json:"state"`
		WorkerSessionID string `json:"workerSessionId"`
	}
	if json.Unmarshal([]byte(input.Stdout()), &result) != nil || result.State != "COMPLETED" || result.WorkerSessionID != id {
		t.Fatalf("invoke terminal result: %s", input.Stdout())
	}
}
