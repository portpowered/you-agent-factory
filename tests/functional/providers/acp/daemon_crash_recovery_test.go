package acp_test

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	providerswire "github.com/portpowered/infinite-you/pkg/services/providers/wire"
)

func TestProvidersACPRestartsAfterCrashWithoutReplayingUncertainPrompt(t *testing.T) {
	t.Setenv(acpHelperEnvironment, "crash-once")
	marker := filepath.Join(t.TempDir(), "crashed")
	t.Setenv("YOU_TEST_ACP_CRASH_MARKER", marker)
	var starts atomic.Int32
	root, err := providerswire.NewService(
		providerswire.WithCommandFactory(acpHelperCommandFactory(&starts)),
		providerswire.WithExecutableLocator(availableExecutableLocator{}),
	)
	if err != nil {
		t.Fatalf("construct Providers: %v", err)
	}
	t.Cleanup(func() { _ = root.(providers.Lifecycle).Close(context.Background()) })
	request := providers.ExecuteRequest{
		Provider: "cursor-acp", AttemptID: "crash", UserMessage: "do not replay",
		WorkingDirectory: t.TempDir(), ProcessEnvironment: os.Environ(),
	}
	if _, err := root.Execute(context.Background(), request); err == nil {
		t.Fatal("first Execute() error = nil, want peer crash")
	}
	request.AttemptID = "after-crash"
	result, err := root.Execute(context.Background(), request)
	if err != nil {
		t.Fatalf("second Execute() error = %v", err)
	}
	if result.Content == "" {
		t.Fatal("second Execute() returned empty content")
	}
	if starts.Load() != 2 {
		t.Fatalf("ACP process starts = %d, want one crash plus one replacement", starts.Load())
	}
}
