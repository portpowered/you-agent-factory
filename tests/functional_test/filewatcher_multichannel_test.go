package functional_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/testutil"
)

// TestMultiChannelFileWatcher_DefaultSubmission confirms that a seed file
// in inputs/task/default/ is picked up by preseed and processed to completion
// through the full service pipeline.
func TestMultiChannelFileWatcher_DefaultSubmission(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "filewatcher_flow"))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "default item"}`))

	provider := testutil.NewMockProvider(acceptedProviderResponse())
	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap())

	h.RunUntilComplete(t, 10*time.Second)

	h.Assert().
		HasTokenInPlace("task:complete").
		HasNoTokenInPlace("task:init").
		TokenCount(1)
}

// TestMultiChannelFileWatcher_ExecutionIDSubmission confirms that a seed file
// in a non-default channel directory (inputs/task/<exec-id>/) is picked up
// by preseed and processed to completion through the service pipeline.
func TestMultiChannelFileWatcher_ExecutionIDSubmission(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "filewatcher_flow"))

	// Write seed file to a named channel directory (non-default).
	execDir := filepath.Join(dir, "inputs", "task", "exec-123")
	if err := os.MkdirAll(execDir, 0o755); err != nil {
		t.Fatalf("create exec dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(execDir, "work-1.json"), []byte(`{"title": "executor work"}`), 0o644); err != nil {
		t.Fatalf("write work file: %v", err)
	}

	provider := testutil.NewMockProvider(acceptedProviderResponse())
	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap())

	h.RunUntilComplete(t, 10*time.Second)

	h.Assert().
		HasTokenInPlace("task:complete").
		TokenCount(1)
}

// TestMultiChannelFileWatcher_DynamicExecDir confirms that seed files across
// multiple channel directories (default and named) are all picked up by
// preseed and processed to completion through the service pipeline.
func TestMultiChannelFileWatcher_DynamicExecDir(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "filewatcher_flow"))

	// Seed file in the default channel.
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "default work"}`))

	// Seed file in a named channel directory.
	execDir := filepath.Join(dir, "inputs", "task", "exec-dynamic")
	if err := os.MkdirAll(execDir, 0o755); err != nil {
		t.Fatalf("create exec dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(execDir, "work.json"), []byte(`{"title": "exec work"}`), 0o644); err != nil {
		t.Fatalf("write work file: %v", err)
	}

	provider := testutil.NewMockProvider(
		acceptedProviderResponse(),
		acceptedProviderResponse(),
	)
	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap())

	h.RunUntilComplete(t, 10*time.Second)

	h.Assert().TokenCount(2)

	// Verify no tokens in non-terminal places.
	snap := h.Marking()
	for _, tok := range snap.Tokens {
		if tok.PlaceID != "task:complete" && tok.PlaceID != "task:failed" {
			t.Errorf("token leak: token %s in non-terminal place %s", tok.ID, tok.PlaceID)
		}
	}
}
