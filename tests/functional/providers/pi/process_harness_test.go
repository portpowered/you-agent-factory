package pi

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	piFunctionalSessionID            = "pi-functional-session"
	piFunctionalReplacementSessionID = "pi-functional-replacement"
)

// TestPiStreamingSuccessThroughRootBuildProcess proves ordered Pi progress,
// tool correlation, detached session metadata, and exactly one terminal result
// through the customer process boundary.
func TestPiStreamingSuccessThroughRootBuildProcess(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	support.WriteAgentConfig(t, dir, "worker", support.BuildModelWorkerConfig(
		modelprovider.ProviderPi,
		"anthropic/claude-sonnet-4",
	))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"pi streaming success"}`))

	runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{
		Stdout: piSuccessStream(piFunctionalSessionID, "Pi answer COMPLETE"),
	})

	_, listed, events, responseEvents := support.RunFactoryToCompletionWithEdgesAndResponseEvents(
		t,
		dir,
		serviceedges.Edges{ProviderCommandRunner: runner},
		20*time.Second,
	)

	if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 1 {
		encoded, _ := json.Marshal(events)
		t.Logf("Pi failure events: %s", encoded)
		t.Fatalf("completed work = %d, want 1; listed=%#v", got, listed)
	}
	if runner.CallCount() != 1 {
		t.Fatalf("Pi command runner calls = %d, want 1 through Providers path", runner.CallCount())
	}
	request := runner.LastRequest()
	if request.Command != "pi" {
		t.Fatalf("command = %q, want pi", request.Command)
	}
	assertArg(t, request.Args, "--print")
	assertArgPair(t, request.Args, "--mode", "json")
	assertArg(t, request.Args, "--approve")
	assertArgPair(t, request.Args, "--model", "anthropic/claude-sonnet-4")

	dispatchID := assertPiOrderedCompletion(t, events, "Pi answer COMPLETE")
	assertPiResponseLifecycle(t, responseEvents, dispatchID, "working", "Pi answer COMPLETE")
	assertPiProviderSession(t, events, piFunctionalSessionID)
}

// TestPiResumeContinuityThroughRootBuildProcess proves retry resumes the
// observed session and preserves detached Provider Session metadata.
func TestPiResumeContinuityThroughRootBuildProcess(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	support.WriteAgentConfig(t, dir, "worker", support.BuildModelWorkerConfig(
		modelprovider.ProviderPi,
		"anthropic/claude-sonnet-4",
	))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"pi resume continuity"}`))

	runner := testutil.NewProviderCommandRunner(
		platformprocess.CommandResult{
			ExitCode: 1,
			Stdout: []byte(
				`{"type":"session","id":"` + piFunctionalSessionID + `"}` + "\n" +
					`{"type":"auto_retry_start","attempt":2,"retryDelayMs":2000,"errorStatus":429}` + "\n",
			),
		},
		platformprocess.CommandResult{
			Stdout: piSuccessStream(piFunctionalReplacementSessionID, "Resumed COMPLETE"),
		},
	)

	_, listed, events := support.RunFactoryToCompletionWithEdgesAndObservations(t, dir, serviceedges.Edges{
		ProviderCommandRunner: runner,
	}, 20*time.Second)

	if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 1 {
		t.Fatalf("completed work = %d, want 1; listed=%#v", got, listed)
	}
	requests := runner.Requests()
	if len(requests) != 2 {
		t.Fatalf("Pi command runner calls = %d, want failed attempt plus retry", len(requests))
	}
	assertArgPair(t, requests[1].Args, "--session", piFunctionalSessionID)
	assertPiAttemptSessions(t, events, piFunctionalSessionID, piFunctionalReplacementSessionID)
}

// TestPiNativeFailureThroughRootBuildProcessIsSafe proves native Pi failures
// remain safe and observable through the customer process boundary.
func TestPiNativeFailureThroughRootBuildProcessIsSafe(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	support.WriteAgentConfig(t, dir, "worker", support.BuildModelWorkerConfig(
		modelprovider.ProviderPi,
		"anthropic/claude-sonnet-4",
	))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"pi native failure"}`))

	const leaked = `C:\private\pi-token.txt`
	runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{
		ExitCode: 1,
		Stdout: append(
			[]byte(`{"type":"session","id":"`+piFunctionalSessionID+`"}`+"\n"),
			piTerminalRecord("token path "+leaked+" leaked")...,
		),
	})
	_, listed, events := support.RunFactoryToCompletionWithEdgesAndObservations(t, dir, serviceedges.Edges{
		ProviderCommandRunner: runner,
	}, 20*time.Second)

	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 1 {
		t.Fatalf("failed work = %d, want 1; listed=%#v", got, listed)
	}
	if runner.CallCount() != 1 {
		t.Fatalf("Pi command runner calls = %d, want 1", runner.CallCount())
	}
	encoded, err := json.Marshal(events)
	if err != nil {
		t.Fatalf("marshal Factory events: %v", err)
	}
	payload := string(encoded)
	if strings.Contains(payload, leaked) || strings.Contains(payload, "pi-token") {
		t.Fatalf("Factory events leaked unsafe Pi failure detail: %s", payload)
	}
}

// TestPiCommandCancellationThroughRootBuildProcessIsCanonical proves
// cancellation returns the canonical outcome through the Providers adapter.
func TestPiCommandCancellationThroughRootBuildProcessIsCanonical(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	support.WriteAgentConfig(t, dir, "worker", support.BuildModelWorkerConfig(
		modelprovider.ProviderPi,
		"anthropic/claude-sonnet-4",
	))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"pi command cancel"}`))

	runner := &commandCancellationRunner{}
	_, listed, events := support.RunFactoryToCompletionWithEdgesAndObservations(t, dir, serviceedges.Edges{
		ProviderCommandRunner: runner,
	}, 20*time.Second)

	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 1 {
		t.Fatalf("failed work = %d, want 1; listed=%#v", got, listed)
	}
	if runner.calls != 1 {
		t.Fatalf("Pi command runner calls = %d, want 1", runner.calls)
	}
	if runner.lastRequest.Command != "pi" {
		t.Fatalf("command = %q, want pi", runner.lastRequest.Command)
	}
	encoded, err := json.Marshal(events)
	if err != nil {
		t.Fatalf("marshal Factory events: %v", err)
	}
	payload := string(encoded)
	if !strings.Contains(payload, "provider invocation was canceled") {
		t.Fatalf("Factory events missing canonical cancellation outcome: %s", payload)
	}
}

func piSuccessStream(sessionID, result string) []byte {
	records := []string{
		`{"type":"session","id":"` + sessionID + `"}`,
		`{"type":"message_start","message":{"id":"msg-1","role":"assistant","content":[]}}`,
		`{"type":"message_update","message":{"id":"msg-1","role":"assistant","content":[]},"assistantMessageEvent":{"type":"text_delta","delta":"working","contentIndex":0}}`,
		`{"type":"tool_execution_start","toolCallId":"call-1","toolName":"read_file"}`,
		`{"type":"tool_execution_end","toolCallId":"call-1","toolName":"read_file","result":{}}`,
		string(piTerminalRecord(result)),
	}
	return []byte(strings.Join(records, "\n") + "\n")
}

func piTerminalRecord(result string) []byte {
	return []byte(
		`{"type":"message_end","message":{"id":"msg-1","role":"assistant","content":[{"type":"text","text":` +
			mustJSON(result) + `}],"stopReason":"stop"}}`,
	)
}

func mustJSON(value string) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return string(encoded)
}

func assertPiOrderedCompletion(t *testing.T, events []factoryapi.FactoryEvent, wantOutput string) string {
	t.Helper()
	requestIndex, responseIndex, dispatchResponseIndex := -1, -1, -1
	dispatchID := ""
	for index, event := range events {
		switch event.Type {
		case factoryapi.FactoryEventTypeDispatchRequest:
			if event.Context.DispatchId != nil {
				dispatchID = *event.Context.DispatchId
			}
		case factoryapi.FactoryEventTypeInferenceRequest:
			if dispatchID != "" && event.Context.DispatchId != nil && *event.Context.DispatchId == dispatchID {
				requestIndex = index
			}
		case factoryapi.FactoryEventTypeInferenceResponse:
			if dispatchID != "" && event.Context.DispatchId != nil && *event.Context.DispatchId == dispatchID {
				responseIndex = index
			}
		case factoryapi.FactoryEventTypeDispatchResponse:
			if dispatchID == "" || event.Context.DispatchId == nil || *event.Context.DispatchId != dispatchID {
				continue
			}
			payload, err := event.Payload.AsDispatchResponseEventPayload()
			if err != nil {
				t.Fatalf("decode dispatch response: %v", err)
			}
			if payload.Output == nil || *payload.Output != wantOutput {
				t.Fatalf("dispatch output = %#v, want %q", payload.Output, wantOutput)
			}
			dispatchResponseIndex = index
		}
	}
	if dispatchID == "" || requestIndex < 0 || responseIndex <= requestIndex ||
		dispatchResponseIndex <= responseIndex {
		t.Fatalf(
			"event order dispatch=%q inference request=%d response=%d dispatch response=%d",
			dispatchID, requestIndex, responseIndex, dispatchResponseIndex,
		)
	}
	return dispatchID
}

func assertPiResponseLifecycle(
	t *testing.T,
	events []factoryapi.FactoryResponseEvent,
	dispatchID, wantDelta, wantFinal string,
) {
	t.Helper()
	want := []struct {
		kind  factoryapi.FactoryResponseEventKind
		phase factoryapi.FactoryResponseEventPhase
	}{
		{factoryapi.FactoryResponseEventKindRun, factoryapi.FactoryResponseEventPhaseStarted},
		{factoryapi.FactoryResponseEventKindMessage, factoryapi.FactoryResponseEventPhaseStarted},
		{factoryapi.FactoryResponseEventKindMessage, factoryapi.FactoryResponseEventPhaseDelta},
		{factoryapi.FactoryResponseEventKindTool, factoryapi.FactoryResponseEventPhaseStarted},
		{factoryapi.FactoryResponseEventKindTool, factoryapi.FactoryResponseEventPhaseCompleted},
		{factoryapi.FactoryResponseEventKindMessage, factoryapi.FactoryResponseEventPhaseCompleted},
		{factoryapi.FactoryResponseEventKindRun, factoryapi.FactoryResponseEventPhaseCompleted},
	}
	if len(events) != len(want) {
		t.Fatalf("Pi response events = %#v, want exactly %d lifecycle events", events, len(want))
	}
	runID := events[0].RunId
	for index, spec := range want {
		event := events[index]
		if event.Kind != spec.kind || event.Phase != spec.phase {
			t.Fatalf("response event[%d] = %s/%s, want %s/%s", index, event.Kind, event.Phase, spec.kind, spec.phase)
		}
		if event.DispatchId == nil || *event.DispatchId != dispatchID ||
			event.RunId != runID || event.Sequence != int64(index+1) {
			t.Fatalf("response event[%d] correlation = %#v, want dispatch %q run %q sequence %d", index, event, dispatchID, runID, index+1)
		}
	}
	delta, err := events[2].Payload.AsFactoryResponseEventMessageDeltaPayload()
	if err != nil || delta.TextDelta == nil || *delta.TextDelta != wantDelta {
		t.Fatalf("assistant delta = %#v, %v; want %q", delta, err, wantDelta)
	}
	started, err := events[3].Payload.AsFactoryResponseEventToolPayload()
	if err != nil || started.ToolCallId != "call-1" || started.ToolName != "read_file" {
		t.Fatalf("started tool = %#v, %v", started, err)
	}
	completed, err := events[4].Payload.AsFactoryResponseEventToolPayload()
	if err != nil || completed.ToolCallId != started.ToolCallId {
		t.Fatalf("completed tool = %#v, %v; want call %q", completed, err, started.ToolCallId)
	}
	message, err := events[5].Payload.AsFactoryResponseEventMessagePayload()
	if err != nil || len(message.ContentBlocks) != 1 {
		t.Fatalf("terminal message = %#v, %v", message, err)
	}
	text, err := message.ContentBlocks[0].AsFactoryResponseEventTextContentBlock()
	if err != nil || text.Text != wantFinal {
		t.Fatalf("terminal message text = %#v, %v; want %q", text, err, wantFinal)
	}
}

func assertPiProviderSession(t *testing.T, events []factoryapi.FactoryEvent, wantSessionID string) {
	t.Helper()
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeInferenceResponse {
			continue
		}
		payload, err := event.Payload.AsInferenceResponseEventPayload()
		if err != nil {
			t.Fatalf("decode inference response: %v", err)
		}
		if payload.Outcome != factoryapi.InferenceOutcomeSucceeded {
			continue
		}
		if payload.ProviderSession == nil || payload.ProviderSession.Id == nil {
			t.Fatal("inference response missing Provider Session identity")
		}
		if got := *payload.ProviderSession.Id; got != wantSessionID {
			t.Fatalf("Provider Session id = %q, want %q", got, wantSessionID)
		}
		return
	}
	t.Fatal("missing succeeded INFERENCE_RESPONSE with Provider Session")
}

func assertPiAttemptSessions(t *testing.T, events []factoryapi.FactoryEvent, failed, succeeded string) {
	t.Helper()
	var failedSession, succeededSession string
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeInferenceResponse {
			continue
		}
		payload, err := event.Payload.AsInferenceResponseEventPayload()
		if err != nil {
			t.Fatalf("decode inference response: %v", err)
		}
		if payload.ProviderSession == nil || payload.ProviderSession.Id == nil {
			continue
		}
		switch payload.Outcome {
		case factoryapi.InferenceOutcomeFailed:
			failedSession = *payload.ProviderSession.Id
		case factoryapi.InferenceOutcomeSucceeded:
			succeededSession = *payload.ProviderSession.Id
		}
	}
	if failedSession != failed || succeededSession != succeeded {
		t.Fatalf(
			"attempt sessions = failed %q succeeded %q, want %q/%q",
			failedSession, succeededSession, failed, succeeded,
		)
	}
}

type commandCancellationRunner struct {
	calls       int
	lastRequest platformprocess.CommandRequest
}

func (r *commandCancellationRunner) Run(_ context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	r.calls++
	r.lastRequest = request
	return platformprocess.CommandResult{}, context.Canceled
}

func assertArg(t *testing.T, args []string, want string) {
	t.Helper()
	for _, arg := range args {
		if arg == want {
			return
		}
	}
	t.Fatalf("args = %#v, want %q", args, want)
}

func assertArgPair(t *testing.T, args []string, flag, value string) {
	t.Helper()
	for index := 0; index+1 < len(args); index++ {
		if args[index] == flag && args[index+1] == value {
			return
		}
	}
	t.Fatalf("args = %#v, want %q %q", args, flag, value)
}
