//go:build functionallong

package workflow

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/testutil"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestMultiChannelFileWatcher_DefaultSubmission(t *testing.T) {
	support.SkipLongFunctional(t, "slow multi-channel file-watcher default submission sweep")

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "filewatcher_flow"))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "default item"}`))

	provider := testutil.NewMockProvider(support.AcceptedProviderResponse())
	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap())

	h.RunUntilComplete(t, 10*time.Second)

	h.Assert().
		HasTokenInPlace("task:complete").
		HasNoTokenInPlace("task:init").
		TokenCount(1)
}

func TestMultiChannelFileWatcher_ExecutionIDSubmission(t *testing.T) {
	support.SkipLongFunctional(t, "slow multi-channel file-watcher execution-id sweep")

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "filewatcher_flow"))

	execDir := filepath.Join(dir, "inputs", "task", "exec-123")
	if err := os.MkdirAll(execDir, 0o755); err != nil {
		t.Fatalf("create exec dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(execDir, "work-1.json"), []byte(`{"title": "executor work"}`), 0o644); err != nil {
		t.Fatalf("write work file: %v", err)
	}

	provider := testutil.NewMockProvider(support.AcceptedProviderResponse())
	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap())

	h.RunUntilComplete(t, 10*time.Second)

	h.Assert().
		HasTokenInPlace("task:complete").
		TokenCount(1)
}

func TestMultiChannelFileWatcher_DynamicExecDir(t *testing.T) {
	support.SkipLongFunctional(t, "slow multi-channel file-watcher dynamic-exec-dir sweep")

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "filewatcher_flow"))

	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "default work"}`))

	execDir := filepath.Join(dir, "inputs", "task", "exec-dynamic")
	if err := os.MkdirAll(execDir, 0o755); err != nil {
		t.Fatalf("create exec dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(execDir, "work.json"), []byte(`{"title": "exec work"}`), 0o644); err != nil {
		t.Fatalf("write work file: %v", err)
	}

	provider := testutil.NewMockProvider(
		support.AcceptedProviderResponse(),
		support.AcceptedProviderResponse(),
	)
	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap())

	h.RunUntilComplete(t, 10*time.Second)

	h.Assert().TokenCount(2)

	snap := h.Marking()
	for _, tok := range snap.Tokens {
		if tok.PlaceID != "task:complete" && tok.PlaceID != "task:failed" {
			t.Errorf("token leak: token %s in non-terminal place %s", tok.ID, tok.PlaceID)
		}
	}
}
