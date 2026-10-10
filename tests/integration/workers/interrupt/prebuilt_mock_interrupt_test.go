package interrupt_test

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/builtcliacceptance"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// One delivered-artifact journey: CLI invoke/interrupt and HTTP replay observe
// the production host. File gates synchronize real processes, so polling public
// state/file arrival is required here rather than an in-process fake clock.
func TestPrebuiltRecordedInterruptPreservesMockExecutionPolicy(t *testing.T) {
	binary := resolvePrebuiltArtifact(t)
	ctx, cancel := context.WithTimeout(t.Context(), interruptIntegrationTimeout)
	defer cancel()
	root := t.TempDir()
	env, marker := mockDeniedEnvironment(t, root)
	dir := filepath.Join(root, "factory")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	writeInterruptFactory(t, dir)
	gate := &workers.MockWorkerGateConfig{
		ArrivedFile: filepath.Join(root, "arrived"), ReleaseFile: filepath.Join(root, "release"), Timeout: "60s",
	}
	tokens := int64(17)
	config := workers.MockWorkersConfig{MockWorkers: []workers.MockWorkerConfig{{
		ID: "unconditional", RunType: workers.MockWorkerRunTypeAccept, GateConfig: gate,
		Usage: &workers.MockWorkerUsageConfig{Provider: "codex", Model: interruptModel, InputTokens: &tokens},
	}}}
	encoded, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(root, "mock-workers.json")
	if err := os.WriteFile(configPath, encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	port, err := builtcliacceptance.ReserveLocalTCPPort()
	if err != nil {
		t.Fatal(err)
	}
	baseURL := "http://127.0.0.1:" + strconv.Itoa(port)
	// Reuse production daemon supervision/cleanup; only the existing mock flag
	// differs. No build or wiring substitution occurs inside this test.
	daemon := startInterruptDaemonWithOptions(t, ctx, binary, dir, baseURL, env, "--with-mock-workers", configPath)
	waitForInterruptStatus(t, ctx, daemon, baseURL)
	waitForMockRuntimeIdle(t, ctx, baseURL)
	invoke := runInterruptBinary(t, ctx, binary, dir, env,
		"--remote", "--server", baseURL, "--json", "worker-sessions", "invoke",
		"--request-id", interruptSourceRequestID, "--worker-session-id", interruptSourceWorkerSessionID,
		"--dispatch-id", interruptSourceDispatchID, "--execution", mockInterruptExecution(dir),
		"--retry-max-attempts", "1", "--async")
	assertInterruptInvokeAccepted(t, invoke)
	waitForInterruptMarker(t, ctx, gate.ArrivedFile)
	waitForInterruptWorkerState(t, ctx, baseURL, root, daemon, interruptSourceWorkerSessionID, "RUNNING")
	if err := os.Remove(gate.ArrivedFile); err != nil {
		t.Fatal(err)
	}
	accepted := mockInterruptCLI(t, ctx, binary, dir, baseURL, env)
	waitForInterruptMarker(t, ctx, gate.ArrivedFile)
	source := waitForInterruptWorkerState(t, ctx, baseURL, root, daemon, interruptSourceWorkerSessionID, "CANCELED")
	successor := waitForInterruptWorkerState(t, ctx, baseURL, root, daemon, interruptSuccessorWorkerSessionID, "RUNNING")
	assertMockInterruptedPair(t, source, successor, accepted)
	assertMockInterruptReplay(t, ctx, baseURL, accepted)
	if err := os.WriteFile(gate.ReleaseFile, []byte("release\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	final := waitForMockInterruptCompletionUsage(t, ctx, baseURL, tokens)
	if final.TokenUsage == nil || final.TokenUsage.InputTokens == nil || int64(*final.TokenUsage.InputTokens) != tokens {
		t.Fatalf("mock completion lost declared usage: %+v", final)
	}
	// The existing usage-enabled command mock emits this synthetic identity on
	// completion. Native launch truth comes from the denying executable observer.
	if final.ProviderSession == nil || final.ProviderSession.Id != "mock-codex-session" {
		t.Fatalf("completion did not retain mock protocol identity: %+v", final.ProviderSession)
	}
	after := waitForInterruptWorkerState(t, ctx, baseURL, root, daemon, interruptSourceWorkerSessionID, "CANCELED")
	if !reflect.DeepEqual(after.TokenUsage, source.TokenUsage) {
		t.Fatalf("successor mutated source usage: before=%+v after=%+v", source.TokenUsage, after.TokenUsage)
	}
	assertNoMockNativeAttempts(t, marker)
	restartPrebuiltInterruptedPair(t, ctx, binary, dir, baseURL, env, daemon, configPath, port, accepted)
	assertNoMockNativeAttempts(t, marker)
	t.Log("post-change prebuilt CLI/host: source canceled before one gated mock successor; declared usage, settings, lineage, exact replay, zero native attempts/starts and joined cleanup")
}

func waitForMockInterruptCompletionUsage(t *testing.T, ctx context.Context, baseURL string, tokens int64) factoryapi.WorkerSessionObservation {
	t.Helper()
	client := &http.Client{Timeout: 2 * time.Second}
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		observation, _, err := getInterruptWorker(ctx, client, baseURL, interruptSuccessorWorkerSessionID)
		if err == nil {
			switch observation.State {
			case "FAILED", "CANCELED", "TERMINATED":
				t.Fatalf("mock successor failed while waiting for captured usage: %+v", observation)
			case "COMPLETED":
				// Live completion may precede captured declared usage.
				if observation.TokenUsage != nil && observation.TokenUsage.InputTokens != nil && int64(*observation.TokenUsage.InputTokens) == tokens {
					return observation
				}
			}
		}
		select {
		case <-ctx.Done():
			t.Fatalf("mock completion/usage deadline: %v observation=%+v error=%v", ctx.Err(), observation, err)
		case <-ticker.C:
		}
	}
}

func mockInterruptExecution(dir string) string {
	var document map[string]any
	if err := json.Unmarshal([]byte(interruptExecutionDocument(dir)), &document); err != nil {
		panic(err)
	}
	document["execution"].(map[string]any)["reasoningEffort"] = "high"
	// This is a direct admission. The script-based fixture's default Factory
	// correlation is unnecessary here and would test separately owned attribution.
	delete(document["execution"].(map[string]any), "factorySessionId")
	encoded, err := json.Marshal(document)
	if err != nil {
		panic(err)
	}
	return string(encoded)
}

func mockInterruptCLI(t *testing.T, ctx context.Context, binary, dir, baseURL string, env []string) factoryapi.WorkerSessionInterruptResponse {
	t.Helper()
	result := runInterruptBinary(t, ctx, binary, dir, env,
		"--remote", "--server", baseURL, "--json", "worker-sessions", "interrupt", interruptSourceWorkerSessionID,
		"--request-id", interruptRequestID, "--successor-worker-session-id", interruptSuccessorWorkerSessionID,
		"--replacement-message", interruptReplacementMessage, "--resume-mode", "recorded", "--async")
	var accepted factoryapi.WorkerSessionInterruptResponse
	if err := json.Unmarshal([]byte(result.stdout), &accepted); result.err != nil || err != nil || !accepted.Accepted {
		t.Fatalf("recorded CLI interrupt: %v decode=%v stdout=%s stderr=%s", result.err, err, result.stdout, result.stderr)
	}
	return accepted
}

func assertMockInterruptedPair(t *testing.T, source, successor factoryapi.WorkerSessionObservation, accepted factoryapi.WorkerSessionInterruptResponse) {
	t.Helper()
	if accepted.Source.State != "CANCELED" || accepted.SuccessorWorkerSessionId != interruptSuccessorWorkerSessionID ||
		source.SuccessorWorkerSessionId == nil || *source.SuccessorWorkerSessionId != interruptSuccessorWorkerSessionID ||
		successor.PredecessorWorkerSessionId == nil || *successor.PredecessorWorkerSessionId != interruptSourceWorkerSessionID {
		t.Fatalf("source stop/reciprocal lineage: accepted=%+v source=%+v successor=%+v", accepted, source, successor)
	}
	if source.ProviderSessionAvailable || successor.ProviderSessionAvailable || successor.Model == nil || *successor.Model != interruptModel ||
		successor.ReasoningEffort == nil || *successor.ReasoningEffort != "high" {
		t.Fatalf("mock policy/settings: source=%+v successor=%+v", source, successor)
	}
}

func assertMockInterruptReplay(t *testing.T, ctx context.Context, baseURL string, accepted factoryapi.WorkerSessionInterruptResponse) {
	t.Helper()
	mode := factoryapi.WorkerSessionInterruptRequestResumeMode("recorded")
	payload, err := json.Marshal(factoryapi.WorkerSessionInterruptRequest{
		RequestId: interruptRequestID, SuccessorWorkerSessionId: interruptSuccessorWorkerSessionID,
		ReplacementMessage: interruptReplacementMessage, ResumeMode: &mode,
	})
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/worker-sessions/"+interruptSourceWorkerSessionID+"/interrupt", strings.NewReader(string(payload)))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var replay factoryapi.WorkerSessionInterruptResponse
	if err := json.NewDecoder(response.Body).Decode(&replay); err != nil || response.StatusCode != http.StatusAccepted || !reflect.DeepEqual(replay, accepted) {
		t.Fatalf("HTTP replay=%+v status=%d error=%v want=%+v", replay, response.StatusCode, err, accepted)
	}
	listed, err := getInterruptJSON[factoryapi.ListWorkerSessionsResponse](ctx, http.DefaultClient, baseURL+"/worker-sessions?scope=direct&history=all")
	if err != nil || len(listed.Sessions) != 2 {
		t.Fatalf("replay admitted extra sessions: %+v error=%v", listed, err)
	}
}

func waitForMockRuntimeIdle(t *testing.T, ctx context.Context, baseURL string) {
	t.Helper()
	// Runtime activation completes asynchronously in the delivered host. Observe
	// its public status before invoking; integration cannot inject that lifecycle.
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		status, err := getInterruptJSON[factoryapi.StatusResponse](ctx, http.DefaultClient, baseURL+"/status")
		if err == nil && status.RuntimeStatus == "IDLE" {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("mock runtime readiness: %v status=%+v error=%v", ctx.Err(), status, err)
		case <-ticker.C:
		}
	}
}

// These two cells and the retained interrupt journey consume a current prebuilt
// artifact. Only declarative mock executions run; each host is stopped and joined
// before its replacement opens the same isolated profile.
func TestPrebuiltControlledTerminalRestart(t *testing.T) {
	binary := resolvePrebuiltArtifact(t)
	for _, action := range []string{"cancel", "terminate"} {
		t.Run(action, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), interruptIntegrationTimeout)
			defer cancel()
			root := t.TempDir()
			env, marker := mockDeniedEnvironment(t, root)
			dir := filepath.Join(root, "factory")
			if err := os.MkdirAll(dir, 0o700); err != nil {
				t.Fatal(err)
			}
			writeInterruptFactory(t, dir)
			configPath, gate := writeTerminalRestartMock(t, root)
			port, err := builtcliacceptance.ReserveLocalTCPPort()
			if err != nil {
				t.Fatal(err)
			}
			baseURL := "http://127.0.0.1:" + strconv.Itoa(port)
			first := startInterruptDaemonWithOptions(t, ctx, binary, dir, baseURL, env, "--with-mock-workers", configPath)
			waitForInterruptStatus(t, ctx, first, baseURL)
			waitForMockRuntimeIdle(t, ctx, baseURL)
			result := runInterruptBinary(t, ctx, binary, dir, env, "--remote", "--server", baseURL, "--json", "worker-sessions", "invoke", "--request-id", interruptSourceRequestID, "--worker-session-id", interruptSourceWorkerSessionID, "--dispatch-id", interruptSourceDispatchID, "--execution", mockInterruptExecution(dir), "--retry-max-attempts", "1", "--async")
			assertInterruptInvokeAccepted(t, result)
			waitForInterruptMarker(t, ctx, gate.ArrivedFile)
			stopped := runInterruptBinary(t, ctx, binary, dir, env, "--remote", "--server", baseURL, "--json", "worker-sessions", action, interruptSourceWorkerSessionID)
			if stopped.err != nil {
				t.Fatalf("joined %s: %v %s %s", action, stopped.err, stopped.stdout, stopped.stderr)
			}
			before := readPrebuiltTerminalSnapshot(t, ctx, binary, dir, baseURL, env, interruptSourceWorkerSessionID)
			assertPrebuiltControlledFacts(t, action, before.observation)
			stopInterruptDaemon(t, binary, dir, baseURL, env, first)
			assertInterruptPortAvailable(t, port)
			fresh := startInterruptDaemonWithOptions(t, ctx, binary, dir, baseURL, env, "--with-mock-workers", configPath)
			waitForInterruptStatus(t, ctx, fresh, baseURL)
			waitForMockRuntimeIdle(t, ctx, baseURL)
			assertPrebuiltTerminalRestart(t, ctx, binary, dir, baseURL, env, interruptSourceWorkerSessionID, before)
			stopInterruptDaemon(t, binary, dir, baseURL, env, fresh)
			assertInterruptPortAvailable(t, port)
			assertNoMockNativeAttempts(t, marker)
		})
	}
}

func writeTerminalRestartMock(t *testing.T, root string) (string, *workers.MockWorkerGateConfig) {
	t.Helper()
	gate := &workers.MockWorkerGateConfig{ArrivedFile: filepath.Join(root, "arrived"), ReleaseFile: filepath.Join(root, "release"), Timeout: "60s"}
	config := workers.MockWorkersConfig{MockWorkers: []workers.MockWorkerConfig{{ID: "unconditional", RunType: workers.MockWorkerRunTypeAccept, GateConfig: gate}}}
	raw, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "mock-workers.json")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	return path, gate
}

type prebuiltTerminalSnapshot struct {
	observation factoryapi.WorkerSessionObservation
	logs        factoryapi.WorkerSessionLogPage
}

func readPrebuiltTerminalSnapshot(t *testing.T, ctx context.Context, binary, dir, baseURL string, env []string, id string) prebuiltTerminalSnapshot {
	t.Helper()
	var result prebuiltTerminalSnapshot
	show := runInterruptBinary(t, ctx, binary, dir, env, "--remote", "--server", baseURL, "--json", "worker-sessions", "show", "--worker-session-id", id)
	if show.err != nil || json.Unmarshal([]byte(show.stdout), &result.observation) != nil {
		t.Fatalf("show: %v %s %s", show.err, show.stdout, show.stderr)
	}
	httpRow, err := getInterruptJSON[factoryapi.WorkerSessionObservation](ctx, http.DefaultClient, baseURL+"/worker-sessions/"+id)
	if err != nil || !reflect.DeepEqual(httpRow, result.observation) {
		t.Fatalf("CLI/HTTP terminal mismatch: %+v %+v %v", result.observation, httpRow, err)
	}
	result.logs, err = getInterruptJSON[factoryapi.WorkerSessionLogPage](ctx, http.DefaultClient, baseURL+"/worker-sessions/"+id+"/logs")
	if err != nil || len(result.logs.Events) == 0 || result.logs.NextToken != nil {
		t.Fatalf("bounded complete logs: %+v %v", result.logs, err)
	}
	for index, event := range result.logs.Events {
		if event.Event.CapturedAt == nil || (index > 0 && event.Event.Position <= result.logs.Events[index-1].Event.Position) {
			t.Fatalf("unordered/missing capture time: %+v", result.logs.Events)
		}
	}
	return result
}

func assertPrebuiltControlledFacts(t *testing.T, action string, row factoryapi.WorkerSessionObservation) {
	t.Helper()
	state, cause, terminal := "CANCELED", "OPERATOR_CANCELED", "OPERATOR_CANCEL"
	if action == "terminate" {
		state, cause, terminal = "TERMINATED", "OPERATOR_TERMINATED", "OPERATOR_TERMINATE"
	}
	if string(row.State) != state || row.Failure == nil || string(row.Failure.Kind) != cause || row.Failure.Detail == "" || row.StartedAt == nil || row.EndedAt == nil || row.DurationMillis == nil || row.TerminalCause == nil || string(*row.TerminalCause) != terminal {
		t.Fatalf("%s controlled facts: %+v", action, row)
	}
}

func assertPrebuiltTerminalRestart(t *testing.T, ctx context.Context, binary, dir, baseURL string, env []string, id string, want prebuiltTerminalSnapshot) {
	t.Helper()
	got := readPrebuiltTerminalSnapshot(t, ctx, binary, dir, baseURL, env, id)
	want.observation.ConfirmationState = got.observation.ConfirmationState
	want.observation.Revivable = got.observation.Revivable
	if !reflect.DeepEqual(want, got) {
		before, _ := json.Marshal(want.observation)
		after, _ := json.Marshal(got.observation)
		t.Fatalf("%s restart changed terminal/capture: before=%s after=%s logsEqual=%v", id, before, after, reflect.DeepEqual(want.logs, got.logs))
	}
	rows, err := getInterruptJSON[factoryapi.ListWorkerSessionsResponse](ctx, http.DefaultClient, baseURL+"/worker-sessions?history=all")
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows.Sessions {
		if row.WorkerSessionId == id {
			row.ConfirmationState = got.observation.ConfirmationState
			if !reflect.DeepEqual(row, got.observation) {
				t.Fatalf("archive list/show differs: %+v %+v", row, got.observation)
			}
			return
		}
	}
	t.Fatalf("archived list omitted %s", id)
}

func restartPrebuiltInterruptedPair(t *testing.T, ctx context.Context, binary, dir, baseURL string, env []string, daemon *interruptDaemon, configPath string, port int, accepted factoryapi.WorkerSessionInterruptResponse) {
	t.Helper()
	sourceSnapshot := readPrebuiltTerminalSnapshot(t, ctx, binary, dir, baseURL, env, interruptSourceWorkerSessionID)
	successorSnapshot := readPrebuiltTerminalSnapshot(t, ctx, binary, dir, baseURL, env, interruptSuccessorWorkerSessionID)
	assertPrebuiltControlledFacts(t, "interrupt", sourceSnapshot.observation)
	stopInterruptDaemon(t, binary, dir, baseURL, env, daemon)
	assertInterruptPortAvailable(t, port)
	fresh := startInterruptDaemonWithOptions(t, ctx, binary, dir, baseURL, env, "--with-mock-workers", configPath)
	waitForInterruptStatus(t, ctx, fresh, baseURL)
	waitForMockRuntimeIdle(t, ctx, baseURL)
	assertPrebuiltTerminalRestart(t, ctx, binary, dir, baseURL, env, interruptSourceWorkerSessionID, sourceSnapshot)
	assertPrebuiltTerminalRestart(t, ctx, binary, dir, baseURL, env, interruptSuccessorWorkerSessionID, successorSnapshot)
	assertMockInterruptReplay(t, ctx, baseURL, accepted)
	stopInterruptDaemon(t, binary, dir, baseURL, env, fresh)
	assertInterruptPortAvailable(t, port)
}
