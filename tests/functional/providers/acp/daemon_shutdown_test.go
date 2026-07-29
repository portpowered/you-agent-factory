package acp_test

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	providerswire "github.com/portpowered/infinite-you/pkg/services/providers/wire"
)

func TestProvidersShutdownCancelsActivePromptAndJoinsACPProcess(t *testing.T) {
	t.Setenv(acpHelperEnvironment, "block")
	signal := filepath.Join(t.TempDir(), "prompt-started")
	t.Setenv("YOU_TEST_ACP_PROMPT_SIGNAL", signal)
	var starts atomic.Int32
	root, err := providerswire.NewService(
		providerswire.WithCommandFactory(acpHelperCommandFactory(&starts)),
		providerswire.WithExecutableLocator(availableExecutableLocator{}),
	)
	if err != nil {
		t.Fatalf("construct Providers: %v", err)
	}
	executionDone := make(chan error, 1)
	go func() {
		_, executeErr := root.Execute(context.Background(), providers.ExecuteRequest{
			Provider: "cursor-acp", AttemptID: "shutdown", UserMessage: "block",
			WorkingDirectory: t.TempDir(), ProcessEnvironment: os.Environ(),
		})
		executionDone <- executeErr
	}()
	waitForACPTestFile(t, signal)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := root.(providers.Lifecycle).Close(ctx); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	select {
	case err := <-executionDone:
		if err == nil {
			t.Fatal("active Execute() error = nil after shutdown")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("active Execute() did not join during shutdown")
	}
	if starts.Load() != 1 {
		t.Fatalf("ACP process starts = %d, want 1", starts.Load())
	}
}
