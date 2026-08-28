package models_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
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
