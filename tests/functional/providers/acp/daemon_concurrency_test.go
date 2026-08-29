package acp_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// Isolation: isolated-with-reason - connection serialization; two concurrent
// prompts must contend for one real ACP stdio connection and one peer.
func TestProvidersACPSerializesConcurrentPromptsOnOneStdioConnection(t *testing.T) {
	signals := t.TempDir()
	promptHeld := filepath.Join(signals, "prompt-started")
	release := filepath.Join(signals, "release")
	fixture := functionalACPFixture("serialize")
	fixture.PromptSignalPath = promptHeld
	fixture.PromptReleasePath = release

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
	// The peer writes promptHeld immediately before it waits for release. The
	// test owns release and does not create it until after this assertion, so
	// the marker is the synchronization boundary; no timing pad is needed.
	waitForACPTestFile(t, promptHeld)
	select {
	case result := <-results:
		t.Fatalf("parallel invocation completed before the first prompt was released: response=%s error=%v", formatACPInvocationResponse(result.response), result.err)
	default:
	}
	if err := os.WriteFile(release, []byte("release"), 0o600); err != nil {
		t.Fatalf("release first prompt: %v", err)
	}
	select {
	case result := <-results:
		if result.err != nil || result.response.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
			t.Fatalf("parallel invocation = %s, error = %v", formatACPInvocationResponse(result.response), result.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for serialized prompts")
	}
	if starts.Load() != 1 {
		t.Fatalf("ACP process starts = %d, want 1", starts.Load())
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
