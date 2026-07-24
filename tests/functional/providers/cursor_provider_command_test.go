package providers

import (
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestCursorProviderCommand_DispatchesAgentWithRenderedPrompt(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "simple_pipeline"))
	support.WriteAgentConfig(t, dir, "processor", buildCursorModelWorkerConfig("test-cursor-model", false))

	runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{Stdout: support.CursorProviderSuccessStdout("Done. COMPLETE")})
	writeCursorProviderSmokeWork(t, dir)
	_, listed := support.RunFactoryToCompletionWithEdgesAndWork(t, dir, serviceedges.Edges{
		ProviderCommandRunner: runner,
	}, 10*time.Second)
	assertCursorProviderCompleted(t, listed)
	if runner.CallCount() != 1 {
		t.Fatalf("provider runner call count = %d, want 1", runner.CallCount())
	}

	req := runner.LastRequest()
	if req.Command != string(modelprovider.ProviderCursor) {
		t.Fatalf("command = %q, want %q", req.Command, modelprovider.ProviderCursor)
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

	runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{Stdout: support.CursorProviderSuccessStdout("Done. COMPLETE")})
	writeCursorProviderSmokeWork(t, dir)
	_, listed := support.RunFactoryToCompletionWithEdgesAndWork(t, dir, serviceedges.Edges{
		ProviderCommandRunner: runner,
	}, 10*time.Second)
	assertCursorProviderCompleted(t, listed)
	if runner.CallCount() != 1 {
		t.Fatalf("provider runner call count = %d, want 1", runner.CallCount())
	}

	req := runner.LastRequest()
	if req.Command != string(modelprovider.ProviderCursor) {
		t.Fatalf("command = %q, want %q", req.Command, modelprovider.ProviderCursor)
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

	runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{Stdout: support.CursorProviderSuccessStdout("Done. COMPLETE")})
	writeCursorProviderSmokeWork(t, dir)
	_, listed := support.RunFactoryToCompletionWithEdgesAndWork(t, dir, serviceedges.Edges{
		ProviderCommandRunner: runner,
	}, 10*time.Second)
	assertCursorProviderCompleted(t, listed)
	req := runner.LastRequest()
	if req.Command != string(modelprovider.ProviderCursor) {
		t.Fatalf("command = %q, want %q", req.Command, modelprovider.ProviderCursor)
	}
	support.AssertArgsContainSequence(t, req.Args, []string{"-p", "--model", "test-cursor-model"})
	support.AssertArgsContainSequence(t, req.Args, []string{"--output-format", "stream-json", "--stream-partial-output"})
}

func writeCursorProviderSmokeWork(t *testing.T, dir string) {
	t.Helper()

	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		Name: "cursor-provider-smoke", WorkTypeID: "task",
		TraceID: "trace-cursor-provider-smoke", Payload: []byte("cursor provider smoke"),
	})
}

func assertCursorProviderCompleted(t *testing.T, listed factoryapi.ListWorkResponse) {
	t.Helper()
	assertSessionPlaces(t, listed, map[string]int{
		"task:complete": 1, "task:init": 0, "task:failed": 0,
	})
}

func assertSessionPlaces(t *testing.T, listed factoryapi.ListWorkResponse, wants map[string]int) {
	t.Helper()
	for placeID, want := range wants {
		if got := support.CountWorkAtCustomerState(listed, placeID); got != want {
			t.Errorf("%s token count = %d, want %d", placeID, got, want)
		}
	}
}

func assertDispatchOutput(t *testing.T, events []factoryapi.FactoryEvent, want string) {
	t.Helper()
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeDispatchResponse {
			continue
		}
		payload, err := event.Payload.AsDispatchResponseEventPayload()
		if err != nil {
			t.Fatalf("decode dispatch response: %v", err)
		}
		if payload.Output == nil || *payload.Output != want {
			t.Fatalf("dispatch output = %#v, want %q", payload.Output, want)
		}
		return
	}
	t.Fatalf("Factory Event history has no dispatch response: %#v", events)
}

func buildCursorModelWorkerConfig(model string, skipPermissions bool) string {
	lines := []string{
		"---",
		"type: MODEL_WORKER",
		"model: " + model,
		"modelProvider: " + string(modelprovider.ProviderCursor),
		"stopToken: COMPLETE",
	}
	if skipPermissions {
		lines = append(lines, "skipPermissions: true")
	}
	lines = append(lines, "---", "Process the input task.")
	return strings.Join(lines, "\n") + "\n"
}
