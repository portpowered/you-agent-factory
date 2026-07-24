package providers

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/work"
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

	runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{Stdout: []byte("Done. COMPLETE")})
	testutil.WriteSeedBatchFile(t, dir, work.WorkRequest{
		RequestID: "request-codex-images",
		Type:      work.WorkRequestTypeFactoryRequestBatch,
		Works: []work.Work{{
			Name:       "mixed-image-work",
			WorkTypeID: "task",
			TraceID:    "trace-codex-images",
			Content: []work.WorkContentPart{
				{Type: work.WorkContentPartTypeText, Text: "first caption"},
				{Type: work.WorkContentPartTypeImage, File: firstImage},
				{Type: work.WorkContentPartTypeText, Text: "second caption"},
				{Type: work.WorkContentPartTypeImage, File: secondImage},
			},
		}},
	})
	_, listed := support.RunFactoryToCompletionWithEdgesAndWork(t, dir, serviceedges.Edges{
		ProviderCommandRunner: runner,
	}, 10*time.Second)
	assertCursorProviderCompleted(t, listed)
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

	runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{Stdout: []byte("Done. COMPLETE")})
	testutil.WriteSeedBatchFile(t, dir, work.WorkRequest{
		RequestID: "request-codex-text",
		Type:      work.WorkRequestTypeFactoryRequestBatch,
		Works: []work.Work{{
			Name:       "text-only-work",
			WorkTypeID: "task",
			TraceID:    "trace-codex-text",
			Content: []work.WorkContentPart{
				{Type: work.WorkContentPartTypeText, Text: "text only"},
			},
		}},
	})
	_, listed := support.RunFactoryToCompletionWithEdgesAndWork(t, dir, serviceedges.Edges{
		ProviderCommandRunner: runner,
	}, 10*time.Second)
	assertCursorProviderCompleted(t, listed)
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
