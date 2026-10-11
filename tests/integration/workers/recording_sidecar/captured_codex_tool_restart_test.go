package recording_sidecar_test

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/workers"
	api "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// The invoking lane owns compilation. This one real subprocess/restart journey
// uses the production Codex adapter with a private PATH-owned, unpaid shim.
func TestCapturedCodexToolJoinedRestart(t *testing.T) {
	t.Parallel()
	f := newInvokeArtifactFixture(t)
	f.env = capturedCodexRestartEnvironment(f.env)
	raw, err := os.ReadFile(f.document)
	var document map[string]any
	if err != nil || json.Unmarshal(raw, &document) != nil {
		t.Fatal("restart execution unavailable")
	}
	document["execution"].(map[string]any)["envVars"].(map[string]any)["API_KEY"] = "wsv-key-fixture"
	raw, err = json.Marshal(document)
	if err != nil || os.WriteFile(f.document, raw, 0o600) != nil {
		t.Fatal("restart credential fixture unavailable")
	}
	if err := os.WriteFile(filepath.Join(f.project, "provider", "shim.cjs"), []byte(capturedCodexRestartShim), 0o600); err != nil {
		t.Fatal(err)
	}
	first := startHistoryHost(t, f.ctx, f.binary, f.project, f.env)
	f.cli(t, first.url, "invoke", "--execution", f.document)
	before := readCapturedCodexRestart(t, f, first.url)
	first.stop(t, f.ctx, f.binary, f.project, f.env)
	for _, name := range []string{".codex", ".cursor", ".claude"} {
		if err := os.WriteFile(filepath.Join(f.home, name), []byte("provider history unavailable"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// Removing the execution shim ensures replay cannot start a new provider.
	if err := os.Remove(filepath.Join(f.project, "provider", "shim.cjs")); err != nil {
		t.Fatal(err)
	}
	second := startHistoryHost(t, f.ctx, f.binary, f.project, f.env)
	after := readCapturedCodexRestart(t, f, second.url)
	second.stop(t, f.ctx, f.binary, f.project, f.env)
	// Confirmation describes live Runtime association, not recorded facts.
	before.Observation.ConfirmationState = after.Observation.ConfirmationState
	if !reflect.DeepEqual(before, after) {
		t.Fatal("joined restart changed acknowledged tool facts, identities, usage or terminal timing/cause")
	}
}

func capturedCodexRestartEnvironment(environment []string) []string {
	result := make([]string, 0, len(environment))
	for _, entry := range environment {
		key, _, _ := strings.Cut(entry, "=")
		key = strings.ToUpper(key)
		if strings.Contains(key, "TOKEN") || strings.Contains(key, "SECRET") || strings.Contains(key, "PASSWORD") || strings.Contains(key, "API_KEY") || strings.Contains(key, "PROXY") || strings.HasPrefix(key, "CODEX_") || strings.HasPrefix(key, "OPENAI_") || strings.HasPrefix(key, "ANTHROPIC_") || strings.HasPrefix(key, "YOU_WORKER_SESSION_") {
			continue
		}
		result = append(result, entry)
	}
	return result
}

func readCapturedCodexRestart(t *testing.T, f invokeArtifactFixture, server string) capturedSummaryArtifactSnapshot {
	t.Helper()
	const id = "t7-artifact-session"
	var snapshot capturedSummaryArtifactSnapshot
	// Capture commits are asynchronous to command return. Observe the public
	// health/watermark before taking the detached snapshot used for parity.
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		if json.Unmarshal(f.cli(t, server, "show", "--worker-session-id", id), &snapshot.Observation) != nil {
			t.Fatal("restart observation unavailable")
		}
		if snapshot.Observation.RecordingHealth != nil && *snapshot.Observation.RecordingHealth == "COMPLETE" {
			break
		}
		select {
		case <-f.ctx.Done():
			t.Fatal("tool recording did not commit")
		case <-ticker.C:
		}
	}
	if json.Unmarshal(f.cli(t, server, "read", "--worker-session-id", id, "--view", "logs"), &snapshot.Logs) != nil || json.Unmarshal(f.cli(t, server, "read", "--worker-session-id", id, "--view", "transcript"), &snapshot.Transcript) != nil {
		t.Fatal("tool captured views unavailable")
	}
	assertCapturedCodexRestartFacts(t, snapshot)
	assertCapturedCodexRestartHTTP(t, f, server, snapshot)
	assertHistoryMCPViews(t, f.ctx, f.binary, f.project, f.env, server, snapshot.Observation, snapshot.Logs, []string{"READ", "logs", "transcript"}, snapshot.Transcript)
	assertCapturedCodexRestartPages(t, f, server, snapshot.Logs)
	return snapshot
}

func assertCapturedCodexRestartPages(t *testing.T, f invokeArtifactFixture, server string, want api.WorkerSessionLogPage) {
	t.Helper()
	var events []api.WorkerSessionEvent
	token := ""
	for range len(want.Events) + 1 {
		page := historyLogPage(t, f.ctx, f.binary, f.project, f.env, server, want.WorkerSessionId, token)
		events = append(events, page.Events...)
		if page.NextToken == nil {
			if !reflect.DeepEqual(events, want.Events) {
				t.Fatal("restart paging changed acknowledged positions or facts")
			}
			return
		}
		token = *page.NextToken
	}
	t.Fatal("bounded tool recording did not finish paging")
}

func assertCapturedCodexRestartFacts(t *testing.T, snapshot capturedSummaryArtifactSnapshot) {
	t.Helper()
	assertCapturedCodexRestartMetadata(t, snapshot.Observation)
	assertCapturedCodexRestartLogs(t, snapshot.Logs)
	if len(snapshot.Transcript.Entries) != 7 {
		t.Fatalf("restart transcript entry count=%d expected=7", len(snapshot.Transcript.Entries))
	}
	for i, entry := range snapshot.Transcript.Entries[:6] {
		id := []string{"success", "failure", "mcp"}[i/2]
		if entry.CallId == nil || *entry.CallId != id {
			t.Fatal("restart transcript lost first-seen tool order")
		}
	}
	raw, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"private-command", "private-output", "private-denial", "private-token", "private-reasoning", "wsv-key-fixture", "private-key-value"} {
		if strings.Contains(string(raw), secret) {
			t.Fatal("restart leaked sensitive native-shaped facts")
		}
	}
}

func assertCapturedCodexRestartMetadata(t *testing.T, row api.WorkerSessionObservation) {
	t.Helper()
	if row.State != "COMPLETED" || row.Model == nil || *row.Model != "t7-artifact-model" || row.ProviderSession == nil || row.ProviderSession.Id != "restart-tools-thread" || row.StartedAt == nil || row.EndedAt == nil || row.DurationMillis == nil || row.TerminalCause == nil || *row.TerminalCause != "COMPLETED" || row.TokenUsage == nil {
		t.Fatal("tool restart lost recorded identity/model/usage/timing/cause")
	}
}

func assertCapturedCodexRestartLogs(t *testing.T, logs api.WorkerSessionLogPage) {
	t.Helper()
	wantIDs := []string{"success", "success", "failure", "mcp", "mcp"}
	wantStatuses := []string{"in_progress", "completed", "failed", "in_progress", "completed"}
	index := 0
	for i, record := range logs.Events {
		if record.Event.Position != int64(i+1) || record.Event.CapturedAt == nil {
			t.Fatal("restart lost committed order/timestamp")
		}
		if record.Event.Payload["kind"] != "TOOL" {
			continue
		}
		payload := decodeCapturedCodexRestartTool(t, record.Event.Payload)
		if index >= len(wantIDs) || payload.ToolCallID != wantIDs[index] || payload.Status != wantStatuses[index] {
			t.Fatal("restart lost native-shaped tool lifecycle facts")
		}
		name := "command_execution"
		if index >= 3 {
			name = "mcp_tool_call:docs/lookup"
		}
		if payload.ToolName != name {
			t.Fatalf("restart tool name: index=%d id=%q name=%q expected=%q", index, payload.ToolCallID, payload.ToolName, name)
		}
		assertCapturedCodexRestartResult(t, payload, index)
		index++
	}
	if index != len(wantIDs) || logs.CommittedPosition != int64(len(logs.Events)) {
		t.Fatal("restart lost or duplicated captured lifecycle entries")
	}
}

func decodeCapturedCodexRestartTool(t *testing.T, value map[string]interface{}) workers.ToolPayload {
	t.Helper()
	raw, err := json.Marshal(value)
	var draft workers.Draft
	var payload workers.ToolPayload
	if err != nil || json.Unmarshal(raw, &draft) != nil || json.Unmarshal(draft.Payload, &payload) != nil {
		t.Fatal("restart tool payload invalid")
	}
	return payload
}

func assertCapturedCodexRestartResult(t *testing.T, payload workers.ToolPayload, index int) {
	t.Helper()
	if index == 4 {
		var result map[string]any
		if json.Unmarshal(payload.ResultSummary, &result) != nil {
			t.Fatal("restart MCP result invalid")
		}
		want := map[string]any{"status": "completed", "result": map[string]any{"value": "visible", "private_reasoning": "<redacted>", "nested": map[string]any{"safe": float64(0)}}}
		if !reflect.DeepEqual(result, want) {
			t.Fatal("restart MCP result lost safe numeric facts or retained private keys")
		}
		return
	}
	if index != 1 && index != 2 {
		return
	}
	var result map[string]any
	if json.Unmarshal(payload.ResultSummary, &result) != nil || result["exit_code"] != float64(index-1) || result["status"] != payload.Status {
		t.Fatal("restart changed zero/sandbox-failed result")
	}
}

func assertCapturedCodexRestartHTTP(t *testing.T, f invokeArtifactFixture, server string, snapshot capturedSummaryArtifactSnapshot) {
	t.Helper()
	for _, view := range []string{"logs", "transcript"} {
		request, err := http.NewRequestWithContext(f.ctx, http.MethodGet, server+"/worker-sessions/t7-artifact-session/"+view, nil)
		if err != nil {
			t.Fatal(err)
		}
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		var actual, want any
		if view == "logs" {
			actual, want = &api.WorkerSessionLogPage{}, &snapshot.Logs
		} else {
			actual, want = &api.WorkerSessionTranscriptResponse{}, &snapshot.Transcript
		}
		err = json.NewDecoder(response.Body).Decode(actual)
		_ = response.Body.Close()
		if response.StatusCode != http.StatusOK || err != nil || !reflect.DeepEqual(actual, want) {
			t.Fatal("restart CLI/HTTP captured facts disagree")
		}
	}
}

const capturedCodexRestartShim = `
if (process.argv.includes('--version')) { console.log('controlled-codex'); process.exit(0); }
const emit = value => console.log(JSON.stringify(value));
emit({type:'thread.started', thread_id:'restart-tools-thread'});
emit({type:'item.started', item:{id:'success',type:'command_execution',command:'private-command',status:'in_progress'}});
emit({type:'item.completed', item:{id:'success',type:'command_execution',aggregated_output:'private-output',exit_code:0,status:'completed'}});
emit({type:'item.completed', item:{id:'failure',type:'command_execution',aggregated_output:'private-denial',exit_code:1,status:'failed'}});
emit({type:'item.started', item:{id:'mcp',type:'mcp_tool_call',server:'docs',tool:'lookup',arguments:{query:'visible',api_key:'private-token'},status:'in_progress'}});
emit({type:'item.completed', item:{id:'mcp',type:'mcp_tool_call',server:'docs',tool:'lookup',result:{value:'visible',private_reasoning:'private-reasoning','wsv-key-fixture':'private-key-value',nested:{'prefix-wsv-key-fixture':'private-key-value',safe:0}},status:'completed'}});
emit({type:'item.completed', item:{id:'answer',type:'agent_message',text:'restart tools COMPLETE'}});
emit({type:'turn.completed',usage:{input_tokens:5,output_tokens:2}});
`
