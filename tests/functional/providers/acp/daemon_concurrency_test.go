package acp_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// Both prompt-specific entry barriers must be reached before either release.
// Isolation: isolated-with-reason - independently owned prompt barriers.
func TestProvidersACPRunsConcurrentPromptsOnIndependentRequestOwnedPeers(t *testing.T) {
	t.Parallel()
	barriers := t.TempDir()
	entered := filepath.Join(barriers, "entered")
	released := filepath.Join(barriers, "released")
	if err := os.MkdirAll(entered, 0o700); err != nil {
		t.Fatalf("create ACP prompt entry directory: %v", err)
	}
	if err := os.MkdirAll(released, 0o700); err != nil {
		t.Fatalf("create ACP prompt release directory: %v", err)
	}
	fixture := functionalACPFixture("concurrent")
	fixture.PromptSignalDirectory = entered
	fixture.PromptReleaseDirectory = released

	var starts atomic.Int32
	server := startACPDaemonProcess(t, &starts, fixture)
	defer server.Stop(t)
	results := make(chan struct {
		response factoryapi.FactorySessionSyncExecutionResponse
		err      error
	}, 1)
	go func() {
		response, err := invokeACPDaemonWorkflow(t, server, "acp-daemon-concurrency", parallelACPAgentWorkflow)
		results <- struct {
			response factoryapi.FactorySessionSyncExecutionResponse
			err      error
		}{response: response, err: err}
	}()
	// Each peer writes its own "<role>.entered" marker immediately before it
	// waits for its own "<role>.released" file. The test owns both releases and
	// creates neither until both entry markers exist, so those markers are the
	// synchronization boundary: each one proves that prompt reached its own
	// turn while the other prompt was still waiting. No timing pad is needed.
	for _, role := range []string{"first", "second"} {
		waitForACPTestFile(t, filepath.Join(entered, role+".entered"))
	}
	select {
	case result := <-results:
		t.Fatalf("concurrent invocation completed before either prompt was released: response=%s error=%v", formatACPInvocationResponse(result.response), result.err)
	default:
	}
	for _, role := range []string{"first", "second"} {
		if err := os.WriteFile(filepath.Join(released, role+".released"), []byte("release"), 0o600); err != nil {
			t.Fatalf("release ACP prompt %s: %v", role, err)
		}
	}
	var response factoryapi.FactorySessionSyncExecutionResponse
	select {
	case result := <-results:
		if result.err != nil || result.response.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
			t.Fatalf("concurrent invocation = %s, error = %v", formatACPInvocationResponse(result.response), result.err)
		}
		response = result.response
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the released concurrent prompts")
	}
	assertConcurrentPromptOutputs(t, response)
	// One request-owned process per admitted prompt, and no shared peer that
	// could have carried both prompts.
	if starts.Load() != 2 {
		t.Fatalf("ACP process starts = %d, want 2 request-owned stdio peers for two concurrent prompts", starts.Load())
	}
}

// Each prompt must return its own answer in the public primary result.
func assertConcurrentPromptOutputs(t *testing.T, response factoryapi.FactorySessionSyncExecutionResponse) {
	t.Helper()
	if response.Result == nil || response.Result.PrimaryResult == nil || len(*response.Result.PrimaryResult) == 0 {
		t.Fatalf("concurrent invocation returned no primary result: %s", formatACPInvocationResponse(response))
	}
	part, err := (*response.Result.PrimaryResult)[0].AsWorkTextContentPart()
	if err != nil {
		t.Fatalf("concurrent invocation primary result is not a text part: %v", err)
	}
	for _, want := range []string{"first independent answer", "second independent answer"} {
		if !strings.Contains(part.Text, want) {
			t.Fatalf("concurrent invocation primary result = %q, want an independent %q result", part.Text, want)
		}
	}
}

func formatACPInvocationResponse(response factoryapi.FactorySessionSyncExecutionResponse) string {
	payload, err := json.Marshal(response)
	if err != nil {
		return "<unable to marshal response: " + err.Error() + ">"
	}
	return string(payload)
}

func waitForACPTestFile(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(path); err == nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", path)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
