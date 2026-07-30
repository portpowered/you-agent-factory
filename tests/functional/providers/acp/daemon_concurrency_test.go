package acp_test

import (
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestProvidersACPSerializesConcurrentPromptsOnOneStdioConnection(t *testing.T) {
	t.Setenv(acpHelperEnvironment, "serialize")
	signals := t.TempDir()
	promptStarted := filepath.Join(signals, "prompt-started")
	release := filepath.Join(signals, "release")
	t.Setenv("YOU_TEST_ACP_PROMPT_SIGNAL", promptStarted)
	t.Setenv("YOU_TEST_ACP_RELEASE_SIGNAL", release)

	var starts atomic.Int32
	server := startACPDaemonProcess(t, &starts)
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
	waitForACPTestFile(t, promptStarted)
	select {
	case result := <-results:
		t.Fatalf("parallel invocation completed before the first prompt was released: response=%#v error=%v", result.response, result.err)
	case <-time.After(150 * time.Millisecond):
	}
	if err := os.WriteFile(release, []byte("release"), 0o600); err != nil {
		t.Fatalf("release first prompt: %v", err)
	}
	select {
	case result := <-results:
		if result.err != nil || result.response.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
			t.Fatalf("parallel invocation = %#v, error = %v", result.response, result.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for serialized prompts")
	}
	if starts.Load() != 1 {
		t.Fatalf("ACP process starts = %d, want 1", starts.Load())
	}
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
