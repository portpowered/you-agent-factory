package workersessions_test

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// F-02 crosses real Work admission, provider supervision and synced capture.
// Two explicit Factory Sessions share one root-built host and one immutable
// command edge; this fleet-wide observation owns an isolated profile. Provider
// release is signal-driven. The public terminal-status observer waits for the
// runtime's committed result, since command return precedes capture completion.
func runArchivedWorkAttributionCompleted(t *testing.T, process support.Process) {
	t.Parallel()
	dir := support.ScaffoldSingleStepFactory(t, "completed-attribution")
	support.WriteAgentConfig(t, dir, "processor", support.BuildModelWorkerConfig(models.ProviderCodex, "test-model"))
	release := make(chan struct{})
	unblock := sync.OnceFunc(func() { close(release) })
	t.Cleanup(unblock)
	runner := completingAttributionRunner{controlHostRunner{started: make(chan (<-chan struct{}), 2)}, release}
	var sessions support.FactorySessionStarter
	host := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: dir, WaitForServiceModeRuntime: true,
		Edges: serviceedges.Edges{ProviderCommandRunner: runner, FactorySessionsWorkingDirectory: historyWorkingDirectory(dir)},
		BeforeStart: func(_ testing.TB, p support.Process, _ root.Input) {
			sessions = p.(support.ApplicationProcess).FactorySessions()
		},
	})
	session, ctx := startMCP(t, process, host.URL())
	var scopes []string
	for _, name := range []string{"completed-alpha", "completed-beta"} {
		scopes = append(scopes, admitRecordedAttributionWork(t, ctx, sessions, host, runner.controlHostRunner, dir, name))
	}
	live := historyParityPage(t, ctx, session, host, "active", "factory", "")["sessions"].([]any)
	if len(live) != 2 {
		t.Fatalf("gated live membership: %v", live)
	}
	if archived := historyParityPage(t, ctx, session, host, "archived", "factory", ""); len(archived["sessions"].([]any)) != 0 {
		t.Fatalf("live owners leaked into archive: %v", archived)
	}
	first := callWorker(t, ctx, session, "list", map[string]any{"history": "all", "scope": "factory", "limit": 1})["result"].(map[string]any)
	token := first["paginationContext"].(map[string]any)["nextToken"].(string)
	if len(first["sessions"].([]any)) != 1 || token == "" {
		t.Fatalf("all view did not create bounded snapshot: %v", first)
	}
	frozen := historyParityPage(t, ctx, session, host, "all", "factory", token)
	if len(frozen["sessions"].([]any)) != 1 {
		t.Fatalf("all continuation membership: %v", frozen)
	}
	unblock()
	for _, scope := range scopes {
		status := support.WaitForSessionTerminalStatus(t, host.URL(), scope, 30*time.Second)
		if status.Categories.Terminal != 1 || status.Categories.Failed != 0 {
			t.Fatalf("provider completion did not finish named Work: %+v", status)
		}
	}
	for _, view := range []string{"archived", "all"} {
		page := historyParityPage(t, ctx, session, host, view, "factory", "")
		assertCompletedAttributionRows(t, page, live)
	}
	assertJSONEqual(t, frozen, historyParityPage(t, ctx, session, host, "all", "factory", token))
	if active := historyParityPage(t, ctx, session, host, "active", "factory", ""); len(active["sessions"].([]any)) != 0 {
		t.Fatalf("completed attempts remained active: %v", active)
	}
}

type completingAttributionRunner struct {
	controlHostRunner
	release <-chan struct{}
}

func (runner completingAttributionRunner) RunStreaming(ctx context.Context, request platformprocess.CommandRequest, observer platformprocess.OutputChunkObserver) (platformprocess.CommandResult, error) {
	if observer != nil {
		observer(platformprocess.OutputStreamStdout, []byte(`{"type":"thread.started","thread_id":"completed-attribution-provider"}`+"\n"))
	}
	result, err := runner.Run(ctx, request)
	if observer != nil && err == nil {
		observer(platformprocess.OutputStreamStdout, result.Stdout)
	}
	return result, err
}

func (runner completingAttributionRunner) Run(ctx context.Context, _ platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	done := make(chan struct{})
	defer close(done)
	select {
	case runner.started <- done:
	case <-ctx.Done():
		return platformprocess.CommandResult{}, ctx.Err()
	}
	select {
	case <-ctx.Done():
		return platformprocess.CommandResult{}, ctx.Err()
	case <-runner.release:
	}
	output := strings.Join([]string{
		`{"type":"thread.started","thread_id":"completed-attribution-provider"}`,
		`{"type":"turn.started"}`,
		`{"type":"item.completed","item":{"id":"completed-result","type":"agent_message","text":"COMPLETE"}}`,
		`{"type":"turn.completed","usage":{"input_tokens":4,"output_tokens":3}}`,
	}, "\n") + "\n"
	return platformprocess.CommandResult{Stdout: []byte(output)}, nil
}

func assertCompletedAttributionRows(t *testing.T, page map[string]any, live []any) {
	t.Helper()
	rows := page["sessions"].([]any)
	if len(rows) != len(live) {
		t.Fatalf("completed membership: %v", page)
	}
	expected := make(map[string]map[string]any, len(live))
	for _, value := range live {
		row := value.(map[string]any)
		expected[row["workerSessionId"].(string)] = row
	}
	for _, value := range rows {
		row := value.(map[string]any)
		id := row["workerSessionId"].(string)
		before, exists := expected[id]
		if !exists {
			t.Fatalf("duplicate or foreign completed attempt: %v", row)
		}
		delete(expected, id)
		for _, field := range []string{"workId", "workName", "factorySessionId", "provider"} {
			if row[field] != before[field] {
				t.Fatalf("completion changed %s: before=%v after=%v", field, before, row)
			}
		}
		if row["state"] != "COMPLETED" || row["recordingHealth"] != "COMPLETE" || row["terminalCause"] != "COMPLETED" {
			t.Fatalf("completion lost terminal capture facts: %v", row)
		}
		if row["provider"] != "codex" || row["confirmationState"] != "CONFIRMED" {
			t.Fatalf("completion lost provider attribution: %v", row)
		}
		usage, ok := row["tokenUsage"].(map[string]any)
		if !ok || usage["inputTokens"] != float64(4) || usage["outputTokens"] != float64(3) {
			t.Fatalf("completion lost captured provider usage: %v", row)
		}
	}
}
