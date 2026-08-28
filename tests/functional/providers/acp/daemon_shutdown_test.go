package acp_test

import (
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

// Isolation: isolated-with-reason - shutdown join; a real blocked ACP peer
// must be canceled and joined when the root process stops.
func TestProvidersShutdownCancelsActivePromptAndJoinsACPProcess(t *testing.T) {
	t.Setenv(acpHelperEnvironment, "block")
	signal := filepath.Join(t.TempDir(), "prompt-started")
	t.Setenv("YOU_TEST_ACP_PROMPT_SIGNAL", signal)
	var starts atomic.Int32
	server := startACPDaemonProcess(t, &starts)
	executionDone := make(chan error, 1)
	go func() {
		_, executeErr := invokeACPDaemonWorkflow(t, server, "shutdown", singleACPAgentWorkflow)
		executionDone <- executeErr
	}()
	waitForACPTestFile(t, signal)
	server.Stop(t)
	select {
	case err := <-executionDone:
		_ = err // Either a failed durable response or a closed HTTP connection proves the join.
	case <-time.After(3 * time.Second):
		t.Fatal("active Execute() did not join during shutdown")
	}
	if starts.Load() != 1 {
		t.Fatalf("ACP process starts = %d, want 1", starts.Load())
	}
}
