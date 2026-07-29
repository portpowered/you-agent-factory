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

func TestProvidersACPSerializesConcurrentPromptsOnOneStdioConnection(t *testing.T) {
	t.Setenv(acpHelperEnvironment, "serialize")
	signals := t.TempDir()
	promptStarted := filepath.Join(signals, "prompt-started")
	release := filepath.Join(signals, "release")
	t.Setenv("YOU_TEST_ACP_PROMPT_SIGNAL", promptStarted)
	t.Setenv("YOU_TEST_ACP_RELEASE_SIGNAL", release)

	var starts atomic.Int32
	root, err := providerswire.NewService(
		providerswire.WithCommandFactory(acpHelperCommandFactory(&starts)),
		providerswire.WithExecutableLocator(availableExecutableLocator{}),
	)
	if err != nil {
		t.Fatalf("construct Providers: %v", err)
	}
	lifecycle := root.(providers.Lifecycle)
	t.Cleanup(func() { _ = lifecycle.Close(context.Background()) })

	results := make(chan error, 2)
	execute := func(attempt string) {
		_, executeErr := root.Execute(context.Background(), providers.ExecuteRequest{
			Provider: "cursor-acp", AttemptID: attempt, UserMessage: attempt,
			WorkingDirectory: t.TempDir(), ProcessEnvironment: os.Environ(),
		})
		results <- executeErr
	}
	go execute("first")
	waitForACPTestFile(t, promptStarted)
	go execute("second")
	select {
	case err := <-results:
		t.Fatalf("a prompt completed before the first prompt was released: %v", err)
	case <-time.After(150 * time.Millisecond):
	}
	if err := os.WriteFile(release, []byte("release"), 0o600); err != nil {
		t.Fatalf("release first prompt: %v", err)
	}
	for range 2 {
		select {
		case err := <-results:
			if err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for serialized prompts")
		}
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
