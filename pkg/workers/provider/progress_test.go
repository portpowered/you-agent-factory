package provider_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/workers/provider"
)

func TestInferenceProgressPublishingCommandRunner_PublishesOrderedFragments(t *testing.T) {
	t.Parallel()

	scriptPath := filepath.Join(t.TempDir(), "stream.sh")
	script := "#!/bin/sh\necho stdout-chunk\necho stderr-chunk 1>&2\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}

	var published []provider.InferenceProgressFragment
	runner := provider.NewInferenceProgressPublishingCommandRunner(func(fragment provider.InferenceProgressFragment) {
		published = append(published, fragment)
	}, nil)

	result, err := runner.Run(context.Background(), provider.CommandRequest{
		Command:    scriptPath,
		DispatchID: "dispatch-stream-1",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !strings.Contains(string(result.Stdout), "stdout-chunk") {
		t.Fatalf("stdout = %q, want stdout-chunk", result.Stdout)
	}
	if len(published) < 2 {
		t.Fatalf("published events = %d, want at least 2", len(published))
	}

	var sawResponse bool
	var sawProgress bool
	for _, fragment := range published {
		if fragment.DispatchID != "dispatch-stream-1" {
			t.Fatalf("dispatch = %q, want dispatch-stream-1", fragment.DispatchID)
		}
		switch fragment.Kind {
		case provider.ResponseFragmentKind:
			sawResponse = true
		case provider.ProgressFragmentKind:
			sawProgress = true
		default:
			t.Fatalf("unexpected kind %q", fragment.Kind)
		}
	}
	if !sawResponse || !sawProgress {
		t.Fatalf("published fragments = %#v, want both response and progress kinds", published)
	}
}
