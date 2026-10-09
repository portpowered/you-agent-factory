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
	stopInterruptDaemon(t, binary, dir, baseURL, env, daemon)
	assertInterruptPortAvailable(t, port)
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
	listed, err := getInterruptJSON[factoryapi.ListWorkerSessionsResponse](ctx, http.DefaultClient, baseURL+"/worker-sessions?scope=direct")
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
