package acp_test

import (
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestProvidersShutdownCancelsActivePromptAndJoinsACPProcess(t *testing.T) {
	t.Parallel()
	signal := filepath.Join(t.TempDir(), "prompt-started")
	fixture := functionalACPFixture("block")
	fixture.PromptSignalPath = signal
	var starts atomic.Int32
	server := startACPDaemonProcess(t, &starts, fixture)
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
