package models_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// TestStory001CharacterizesFailedPullServerLifecycle captures the built
// server's response, process identity, health checks, and terminal streams
// around a controlled source failure. Whether the current defect terminates
// the server is intentionally an observation, not a corrected-behavior claim.
func TestStory001CharacterizesFailedPullServerLifecycle(t *testing.T) {
	origin := newCharacterizationOrigin(t, characterizationOriginOptions{failManifest: true})
	binaryPath := buildStory001Binary(t)
	workDir := t.TempDir()
	writeStory001Factory(t, workDir)
	homeDir := t.TempDir()
	cacheDir := t.TempDir()
	environment := story001EnvironmentWithBrowserStub(t, homeDir, cacheDir, origin.URL())
	address := reserveStory001Loopback(t)
	process := startStory001Command(t, context.Background(), binaryPath, workDir, environment, "server", "--listen", address)
	t.Cleanup(process.stop)

	baseURL := "http://" + address
	ready := waitForStory001HTTP200(t, t.Context(), baseURL+"/status")
	if ready.status != http.StatusOK {
		process.stop()
		t.Fatalf("built server did not become healthy: %s", summarizeHTTP(ready))
	}
	pid := process.command.Process.Pid
	pullContext, cancel := context.WithTimeout(t.Context(), story001ServerTimeout)
	pull := callStory001HTTP(t, pullContext, http.MethodPost, baseURL+"/models/embed/pull", strings.NewReader("{}"))
	cancel()
	afterHealth := callStory001HTTP(t, t.Context(), http.MethodGet, baseURL+"/status", nil)
	laterRequest := callStory001HTTP(t, t.Context(), http.MethodGet, baseURL+"/models", nil)
	aliveAfterFailure := afterHealth.status == http.StatusOK && laterRequest.status == http.StatusOK
	samePID := aliveAfterFailure && process.command.Process.Pid == pid
	process.stop()

	t.Logf(
		"STORY-001-EVIDENCE acceptance=server-lifecycle probe=built server failed pull pid=%d samePID=%t aliveAfterFailure=%t ready={%s} pull={%s} afterHealth={%s} laterRequest={%s} stdoutBytes=%d stdoutSHA256=%s stderrBytes=%d stderrSHA256=%s",
		pid, samePID, aliveAfterFailure, summarizeHTTP(ready), summarizeHTTP(pull), summarizeHTTP(afterHealth), summarizeHTTP(laterRequest),
		len(process.stdout.Bytes()), sha256Hex(process.stdout.Bytes()), len(process.stderr.Bytes()), sha256Hex(process.stderr.Bytes()),
	)
	if strings.Contains(baseURL, ":7438") || strings.Contains(origin.URL(), ":7438") {
		t.Fatal("story-001 server probe selected forbidden port 7438")
	}
}

// TestStory003FailedPullReturnsTypedResponseAndServerSurvives proves the
// corrected request boundary against the built Windows server. The origin
// fails before any model asset body is served, so the witness covers the
// customer pull response and the same-process recovery path together.
func TestStory003FailedPullReturnsTypedResponseAndServerSurvives(t *testing.T) {
	origin := newCharacterizationOrigin(t, characterizationOriginOptions{failManifest: true})
	binaryPath := buildStory001Binary(t)
	workDir := t.TempDir()
	writeStory001Factory(t, workDir)
	homeDir := t.TempDir()
	cacheDir := t.TempDir()
	environment := story001EnvironmentWithBrowserStub(t, homeDir, cacheDir, origin.URL())
	address := reserveStory001Loopback(t)
	process := startStory001Command(t, context.Background(), binaryPath, workDir, environment, "server", "--listen", address)
	t.Cleanup(process.stop)

	baseURL := "http://" + address
	ready := waitForStory001HTTP200(t, t.Context(), baseURL+"/status")
	if ready.status != http.StatusOK {
		process.stop()
		t.Fatalf("built server did not become healthy: %s", summarizeHTTP(ready))
	}
	pid := process.command.Process.Pid
	pull := callStory001HTTP(t, t.Context(), http.MethodPost, baseURL+"/models/embed/pull", strings.NewReader("{}"))
	if pull.err != "" {
		t.Fatalf("failed pull returned transport error: %s", summarizeHTTP(pull))
	}
	if pull.status != http.StatusUnprocessableEntity {
		t.Fatalf("failed pull status = %d, want 422: %s", pull.status, summarizeHTTP(pull))
	}
	var failure factoryapi.ModelPullResponse
	if err := json.Unmarshal(pull.body, &failure); err != nil {
		t.Fatalf("decode failed pull response: %v; body=%s", err, pull.body)
	}
	if failure.Outcome != factoryapi.ModelPullOutcomeFAILED ||
		failure.ManagedRuntimePull.PullOutcome != factoryapi.ManagedRuntimePullOutcomeSOURCEFETCHFAILED ||
		failure.ManagedRuntimePull.ReadinessState != factoryapi.ManagedRuntimeReadinessStateFAILED {
		t.Fatalf("failed pull response = %#v, want FAILED/SOURCE_FETCH_FAILED/FAILED", failure)
	}
	afterHealth := callStory001HTTP(t, t.Context(), http.MethodGet, baseURL+"/status", nil)
	laterRequest := callStory001HTTP(t, t.Context(), http.MethodGet, baseURL+"/models", nil)
	if afterHealth.status != http.StatusOK || laterRequest.status != http.StatusOK {
		t.Fatalf("server did not survive failed pull: pull={%s} afterHealth={%s} laterRequest={%s}", summarizeHTTP(pull), summarizeHTTP(afterHealth), summarizeHTTP(laterRequest))
	}
	if process.command.Process.Pid != pid {
		t.Fatalf("server PID changed after failed pull: before=%d after=%d", pid, process.command.Process.Pid)
	}

	t.Logf(
		"STORY-003-EVIDENCE acceptance=typed-failed-pull-and-server-survival probe=built Windows server controlled origin pid=%d samePID=%t pull={%s} afterHealth={%s} laterRequest={%s} stdoutBytes=%d stdoutSHA256=%s stderrBytes=%d stderrSHA256=%s",
		pid, process.command.Process.Pid == pid, summarizeHTTP(pull), summarizeHTTP(afterHealth), summarizeHTTP(laterRequest),
		len(process.stdout.Bytes()), sha256Hex(process.stdout.Bytes()), len(process.stderr.Bytes()), sha256Hex(process.stderr.Bytes()),
	)
}
