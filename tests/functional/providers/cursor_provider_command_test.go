package providers

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	modelprovider "github.com/portpowered/infinite-you/pkg/models/provider"
	"github.com/portpowered/infinite-you/pkg/work"
	"github.com/portpowered/infinite-you/pkg/workers"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestCursorProviderCommand_DispatchesAgentWithRenderedPrompt(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "simple_pipeline"))
	support.WriteAgentConfig(t, dir, "processor", buildCursorModelWorkerConfig("test-cursor-model", false))

	runner := testutil.NewProviderCommandRunner(workers.CommandResult{Stdout: support.CursorProviderSuccessStdout("Done. COMPLETE")})
	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProviderCommandRunner(runner),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)
	submitCursorProviderSmokeWork(t, h)

	h.RunUntilComplete(t, 10*time.Second)

	h.Assert().
		HasTokenInPlace("task:complete").
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:failed")
	if runner.CallCount() != 1 {
		t.Fatalf("provider runner call count = %d, want 1", runner.CallCount())
	}

	req := runner.LastRequest()
	if req.Command != string(modelprovider.Cursor) {
		t.Fatalf("command = %q, want %q", req.Command, modelprovider.Cursor)
	}
	support.AssertArgsContainSequence(t, req.Args, []string{"-p"})
	support.AssertArgsContainSequence(t, req.Args, []string{"--output-format", "stream-json", "--stream-partial-output"})
	support.AssertArgsContainSequence(t, req.Args, []string{"--model", "test-cursor-model"})
	assertProviderArgsPrompt(t, req, cursorMergedPrompt("Process the input task.", "Do the work."))
	assertProviderStdin(t, req, "")
}

func TestCursorProviderCommand_SkipPermissionsPassesForceFlag(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "simple_pipeline"))
	support.WriteAgentConfig(t, dir, "processor", buildCursorModelWorkerConfig("test-cursor-model", true))

	runner := testutil.NewProviderCommandRunner(workers.CommandResult{Stdout: support.CursorProviderSuccessStdout("Done. COMPLETE")})
	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProviderCommandRunner(runner),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)
	submitCursorProviderSmokeWork(t, h)

	h.RunUntilComplete(t, 10*time.Second)

	h.Assert().HasTokenInPlace("task:complete")
	if runner.CallCount() != 1 {
		t.Fatalf("provider runner call count = %d, want 1", runner.CallCount())
	}

	req := runner.LastRequest()
	if req.Command != string(modelprovider.Cursor) {
		t.Fatalf("command = %q, want %q", req.Command, modelprovider.Cursor)
	}
	support.AssertArgsContainSequence(t, req.Args, []string{"-f", "-p"})
	support.AssertArgsContainSequence(t, req.Args, []string{"--output-format", "stream-json", "--stream-partial-output"})
	assertProviderArgsPrompt(t, req, cursorMergedPrompt("Process the input task.", "Do the work."))
}

func TestCursorProviderCommand_PublicModelProviderEnumRoutesToAgent(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "simple_pipeline"))
	support.WriteAgentConfig(t, dir, "processor", `---
type: MODEL_WORKER
model: test-cursor-model
modelProvider: CURSOR
stopToken: COMPLETE
---
Process the input task.
`)

	runner := testutil.NewProviderCommandRunner(workers.CommandResult{Stdout: support.CursorProviderSuccessStdout("Done. COMPLETE")})
	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProviderCommandRunner(runner),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)
	submitCursorProviderSmokeWork(t, h)

	h.RunUntilComplete(t, 10*time.Second)

	h.Assert().HasTokenInPlace("task:complete")
	req := runner.LastRequest()
	if req.Command != string(modelprovider.Cursor) {
		t.Fatalf("command = %q, want %q", req.Command, modelprovider.Cursor)
	}
	support.AssertArgsContainSequence(t, req.Args, []string{"-p", "--model", "test-cursor-model"})
	support.AssertArgsContainSequence(t, req.Args, []string{"--output-format", "stream-json", "--stream-partial-output"})
}

func submitCursorProviderSmokeWork(t *testing.T, h *testutil.ServiceTestHarness) {
	t.Helper()

	h.SubmitWorkRequest(context.Background(), work.WorkRequest{
		RequestID: "request-cursor-provider-smoke",
		Type:      work.WorkRequestTypeFactoryRequestBatch,
		Works: []work.Work{{
			Name:       "cursor-provider-smoke",
			WorkTypeID: "task",
			TraceID:    "trace-cursor-provider-smoke",
			Content: []work.WorkContentPart{
				{Type: work.WorkContentPartTypeText, Text: "cursor provider smoke"},
			},
		}},
	})
}

func buildCursorModelWorkerConfig(model string, skipPermissions bool) string {
	lines := []string{
		"---",
		"type: MODEL_WORKER",
		"model: " + model,
		"modelProvider: " + string(modelprovider.Cursor),
		"stopToken: COMPLETE",
	}
	if skipPermissions {
		lines = append(lines, "skipPermissions: true")
	}
	lines = append(lines, "---", "Process the input task.")
	return strings.Join(lines, "\n") + "\n"
}
