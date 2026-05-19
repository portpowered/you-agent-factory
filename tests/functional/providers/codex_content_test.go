package providers

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil"
	"github.com/portpowered/infinite-you/pkg/workers"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestCodexContentDispatch_MixedContentEmitsOrderedImageArgs(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "simple_pipeline"))
	support.WriteAgentConfig(t, dir, "processor", `---
type: MODEL_WORKER
model: test-model
modelProvider: codex
stopToken: COMPLETE
---
Process the input task.
`)
	firstImage := writeCodexContentFixtureImage(t, dir, "one.png")
	secondImage := writeCodexContentFixtureImage(t, dir, "two.png")

	runner := testutil.NewProviderCommandRunner(workers.CommandResult{Stdout: []byte("Done. COMPLETE")})
	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProviderCommandRunner(runner),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	h.SubmitWorkRequest(context.Background(), interfaces.WorkRequest{
		RequestID: "request-codex-images",
		Type:      interfaces.WorkRequestTypeFactoryRequestBatch,
		Works: []interfaces.Work{{
			Name:       "mixed-image-work",
			WorkTypeID: "task",
			TraceID:    "trace-codex-images",
			Content: []interfaces.WorkContentPart{
				{Type: interfaces.WorkContentPartTypeText, Text: "first caption"},
				{Type: interfaces.WorkContentPartTypeImage, File: firstImage},
				{Type: interfaces.WorkContentPartTypeText, Text: "second caption"},
				{Type: interfaces.WorkContentPartTypeImage, File: secondImage},
			},
		}},
	})

	h.RunUntilComplete(t, 10*time.Second)

	h.Assert().
		HasTokenInPlace("task:complete").
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:failed")
	if runner.CallCount() != 1 {
		t.Fatalf("provider runner call count = %d, want 1", runner.CallCount())
	}

	call := runner.LastRequest()
	wantArgs := []string{"exec", "--model", "test-model", "-i", firstImage, "-i", secondImage, "-"}
	assertCommandArgs(t, call, wantArgs)
	if string(call.Stdin) != "Do the work." {
		t.Fatalf("codex stdin = %q, want rendered prompt", string(call.Stdin))
	}
}

func TestCodexContentDispatch_TextOnlyContentDoesNotEmitImageArgs(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "simple_pipeline"))
	support.WriteAgentConfig(t, dir, "processor", `---
type: MODEL_WORKER
model: test-model
modelProvider: codex
stopToken: COMPLETE
---
Process the input task.
`)

	runner := testutil.NewProviderCommandRunner(workers.CommandResult{Stdout: []byte("Done. COMPLETE")})
	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProviderCommandRunner(runner),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	h.SubmitWorkRequest(context.Background(), interfaces.WorkRequest{
		RequestID: "request-codex-text",
		Type:      interfaces.WorkRequestTypeFactoryRequestBatch,
		Works: []interfaces.Work{{
			Name:       "text-only-work",
			WorkTypeID: "task",
			TraceID:    "trace-codex-text",
			Content: []interfaces.WorkContentPart{
				{Type: interfaces.WorkContentPartTypeText, Text: "text only"},
			},
		}},
	})

	h.RunUntilComplete(t, 10*time.Second)

	h.Assert().
		HasTokenInPlace("task:complete").
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:failed")
	if runner.CallCount() != 1 {
		t.Fatalf("provider runner call count = %d, want 1", runner.CallCount())
	}

	wantArgs := []string{"exec", "--model", "test-model", "-"}
	assertCommandArgs(t, runner.LastRequest(), wantArgs)
}

func writeCodexContentFixtureImage(t *testing.T, dir, name string) string {
	t.Helper()

	path := filepath.Join(dir, "fixtures", name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create image fixture directory: %v", err)
	}
	if err := os.WriteFile(path, []byte("image"), 0o644); err != nil {
		t.Fatalf("write image fixture: %v", err)
	}
	return path
}
