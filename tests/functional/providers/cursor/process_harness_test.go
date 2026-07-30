package cursor

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	cursorSessionID            = "cursor-functional-session"
	cursorReplacementSessionID = "cursor-functional-replacement"
)

// TestCursorShortPromptSuccessThroughRootBuildProcess proves the product graph
// selects Cursor and preserves its command, context, lineage, and terminal work.
func TestCursorShortPromptSuccessThroughRootBuildProcess(t *testing.T) {
	dir := cursorFactoryDir(t, "short prompt")
	support.WriteAgentConfig(t, dir, "worker", cursorWorkerConfig(true, ""))
	support.WriteWorkstationConfig(t, dir, "process", `---
type: MODEL_WORKSTATION
workingDirectory: .
env:
  CURSOR_FUNCTIONAL_CONTEXT: configured
---
Test workstation.
`)
	files := &recordingTemporaryFileSystem{}
	runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{
		Stdout: cursorSuccessStream(cursorSessionID, "Cursor answer COMPLETE"),
	})

	_, listed, events, responseEvents := support.RunFactoryToCompletionWithEdgesAndResponseEvents(t, dir, serviceedges.Edges{
		ProviderCommandRunner:              runner,
		WorkersProviderTemporaryFileSystem: files,
	}, 20*time.Second)

	if support.CountWorkAtCustomerState(listed, "task:done") != 1 {
		encoded, _ := json.Marshal(events)
		t.Logf("Cursor failure events: %s", encoded)
	}
	assertWorkOutcome(t, listed, 1, 0)
	if runner.CallCount() != 1 {
		t.Fatalf("Cursor command runner calls = %d, want 1", runner.CallCount())
	}
	request := runner.LastRequest()
	if request.Command != "cursor" {
		t.Fatalf("command = %q, want cursor", request.Command)
	}
	assertArgPair(t, request.Args, "--model", "cursor-test-model")
	assertArg(t, request.Args, "-f")
	assertArgPair(t, request.Args, "--output-format", "stream-json")
	assertArg(t, request.Args, "--stream-partial-output")
	if prompt := request.Args[len(request.Args)-1]; !strings.Contains(prompt, "Process the input task.") ||
		!strings.Contains(prompt, "Test workstation.") {
		t.Fatalf("prompt = %q, want composed system and user content", prompt)
	}
	if !contains(request.Env, "CURSOR_FUNCTIONAL_CONTEXT=configured") {
		t.Fatalf("environment = %#v, want configured Cursor value", request.Env)
	}
	if request.WorkDir == "" {
		t.Fatal("command working directory is empty, want configured workspace")
	}
	assertArgPair(t, request.Args, "--workspace", request.WorkDir)
	if files.CreateCount() != 0 {
		t.Fatalf("short prompt temporary-file creates = %d, want 0", files.CreateCount())
	}
	dispatchID := assertOrderedCorrelatedCompletion(t, events, "Cursor answer COMPLETE")
	assertCursorResponseLifecycle(t, responseEvents, dispatchID, "working", "Cursor answer COMPLETE")
}

// TestCursorWindowsLongPromptThroughRootBuildProcess proves oversized Windows
// prompts use the injected file edge and remove the exact file after execution.
func TestCursorWindowsLongPromptThroughRootBuildProcess(t *testing.T) {
	dir := cursorFactoryDir(t, "long prompt")
	support.WriteAgentConfig(t, dir, "worker", cursorWorkerConfig(false, ""))
	longPrompt := strings.Repeat("🚀", 4_000)
	support.WriteWorkstationConfig(t, dir, "process", "---\ntype: MODEL_WORKSTATION\n---\n"+longPrompt+"\n")
	files := &recordingTemporaryFileSystem{}
	runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{
		Stdout: cursorSuccessStream(cursorSessionID, "Long prompt complete COMPLETE"),
	})

	_, listed, events := support.RunFactoryToCompletionWithEdgesAndObservations(t, dir, serviceedges.Edges{
		ProviderCommandRunner:              runner,
		WorkersOperatingSystem:             "windows",
		WorkersProviderTemporaryFileSystem: files,
	}, 20*time.Second)

	assertWorkOutcome(t, listed, 1, 0)
	request := runner.LastRequest()
	promptArg := request.Args[len(request.Args)-1]
	if !strings.HasPrefix(promptArg, "@") || promptArg != "@"+files.Path() {
		t.Fatalf("prompt arg = %q, want exact @%s reference", promptArg, files.Path())
	}
	content := files.Content()
	if !strings.Contains(content, longPrompt) || !strings.Contains(content, "Process the input task.") {
		t.Fatalf("temporary prompt omitted composed content; length=%d", len(content))
	}
	if files.CreateCount() != 1 || files.CloseCount() != 1 || files.RemoveCount() != 1 {
		t.Fatalf(
			"temporary-file lifecycle = create %d close %d remove %d, want 1/1/1",
			files.CreateCount(), files.CloseCount(), files.RemoveCount(),
		)
	}
	if files.RemovedPath() != files.Path() {
		t.Fatalf("removed path = %q, want exact created path %q", files.RemovedPath(), files.Path())
	}
	assertOrderedCorrelatedCompletion(t, events, "Long prompt complete COMPLETE")
}

// TestCursorResumeContinuityThroughRootBuildProcess proves retry resumes the
// observed session, preserves it without replacement, and accepts a valid replacement.
func TestCursorResumeContinuityThroughRootBuildProcess(t *testing.T) {
	tests := []struct {
		name        string
		second      []byte
		wantSession string
	}{
		{
			name:        "preserves requested session",
			second:      cursorTerminalRecord("", "Resumed COMPLETE"),
			wantSession: cursorSessionID,
		},
		{
			name: "accepts replacement session",
			second: append(
				[]byte(`{"type":"system","subtype":"init","session_id":"`+cursorReplacementSessionID+`"}`+"\n"),
				cursorTerminalRecord("", "Replaced COMPLETE")...,
			),
			wantSession: cursorReplacementSessionID,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := cursorFactoryDir(t, tc.name)
			support.WriteAgentConfig(t, dir, "worker", cursorWorkerConfig(false, ""))
			runner := testutil.NewProviderCommandRunner(
				platformprocess.CommandResult{
					ExitCode: 1,
					Stdout: append(
						[]byte(`{"type":"system","subtype":"init","session_id":"`+cursorSessionID+`"}`+"\n"),
						[]byte(`{"type":"result","subtype":"rate_limit_error","is_error":true,"result":"capacity unavailable","session_id":""}`)...,
					),
				},
				platformprocess.CommandResult{Stdout: tc.second},
			)

			_, listed, events := support.RunFactoryToCompletionWithEdgesAndObservations(t, dir, serviceedges.Edges{
				ProviderCommandRunner: runner,
			}, 20*time.Second)

			assertWorkOutcome(t, listed, 1, 0)
			requests := runner.Requests()
			if len(requests) != 2 {
				t.Fatalf("Cursor command runner calls = %d, want failed attempt plus retry", len(requests))
			}
			assertArgPair(t, requests[1].Args, "--resume", cursorSessionID)
			assertAttemptSessions(t, events, cursorSessionID, tc.wantSession)
		})
	}
}

// TestCursorRejectsUnsupportedCapabilityBeforeExternalIO proves negotiation
// fails before command execution or Windows long-prompt materialization.
func TestCursorRejectsUnsupportedCapabilityBeforeExternalIO(t *testing.T) {
	dir := cursorFactoryDir(t, "unsupported capability")
	support.WriteAgentConfig(t, dir, "worker", cursorWorkerConfig(false, ""))
	support.WriteWorkstationConfig(t, dir, "process", `---
type: MODEL_WORKSTATION
outputSchema: '{}'
---
`+strings.Repeat("🚀", 4_000))
	files := &recordingTemporaryFileSystem{}
	runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{
		Stdout: cursorSuccessStream(cursorSessionID, "must not execute"),
	})

	_, listed, _ := support.RunFactoryToCompletionWithEdgesAndObservations(t, dir, serviceedges.Edges{
		ProviderCommandRunner:              runner,
		WorkersOperatingSystem:             "windows",
		WorkersProviderTemporaryFileSystem: files,
	}, 20*time.Second)

	assertWorkOutcome(t, listed, 0, 1)
	if runner.CallCount() != 0 || files.CreateCount() != 0 {
		t.Fatalf(
			"unsupported capability performed external I/O: command calls=%d temporary creates=%d",
			runner.CallCount(), files.CreateCount(),
		)
	}
}

// TestCursorAdverseOutcomesThroughRootBuildProcess proves unsafe, malformed,
// incomplete, timeout, and cancellation outcomes remain bounded and clean files.
func TestCursorAdverseOutcomesThroughRootBuildProcess(t *testing.T) {
	const leaked = `C:\private\cursor-token.txt`
	tests := []struct {
		name       string
		result     platformprocess.CommandResult
		commandErr error
		wantReason factoryapi.WorkFailureType
		notReason  factoryapi.WorkFailureType
	}{
		{
			name: "native authentication failure",
			result: platformprocess.CommandResult{
				ExitCode: 1,
				Stdout: []byte(
					`{"type":"result","subtype":"authentication_error","is_error":true,"result":` +
						mustJSON("Please sign in; token path "+leaked+" leaked") +
						`,"session_id":"` + cursorSessionID + `"}`,
				),
			},
			wantReason: factoryapi.WorkFailureTypeAuthFailure,
		},
		{
			name:       "malformed output",
			result:     platformprocess.CommandResult{Stdout: []byte(`{malformed secret ` + leaked)},
			wantReason: factoryapi.WorkFailureTypePermanentBadRequest,
		},
		{name: "incomplete output", result: platformprocess.CommandResult{Stdout: []byte(
			`{"type":"assistant","message":{"content":[{"type":"text","text":"private draft"}]},"session_id":"` +
				cursorSessionID + `"}`,
		)}, wantReason: factoryapi.WorkFailureTypePermanentBadRequest},
		{
			name:       "timeout",
			commandErr: context.DeadlineExceeded,
			wantReason: factoryapi.WorkFailureTypeTimeout,
		},
		{
			name:       "cancellation",
			commandErr: context.Canceled,
			notReason:  factoryapi.WorkFailureTypeTimeout,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := cursorFactoryDir(t, tc.name)
			support.WriteAgentConfig(t, dir, "worker", cursorWorkerConfig(false, ""))
			support.WriteWorkstationConfig(t, dir, "process", "---\ntype: MODEL_WORKSTATION\n---\n"+
				strings.Repeat("🚀", 4_000))
			files := &recordingTemporaryFileSystem{}
			runner := &fixedCommandRunner{result: tc.result, err: tc.commandErr}

			_, listed, events := support.RunFactoryToCompletionWithEdgesAndObservations(t, dir, serviceedges.Edges{
				ProviderCommandRunner:              runner,
				WorkersOperatingSystem:             "windows",
				WorkersProviderTemporaryFileSystem: files,
			}, 20*time.Second)

			assertWorkOutcome(t, listed, 0, 1)
			if files.CreateCount() == 0 || files.CreateCount() != files.CloseCount() ||
				files.CreateCount() != files.RemoveCount() {
				t.Fatalf(
					"temporary-file lifecycle = create %d close %d remove %d, want balanced non-zero cleanup",
					files.CreateCount(), files.CloseCount(), files.RemoveCount(),
				)
			}
			encoded, err := json.Marshal(events)
			if err != nil {
				t.Fatalf("marshal Factory events: %v", err)
			}
			payload := string(encoded)
			if strings.Contains(payload, leaked) || strings.Contains(payload, "cursor-token") {
				t.Fatalf("Factory events leaked unsafe Cursor output: %s", payload)
			}
			reason := terminalFailureReason(t, events)
			if tc.wantReason != "" && reason != tc.wantReason {
				t.Fatalf("failure reason = %q, want %q", reason, tc.wantReason)
			}
			if tc.notReason != "" && reason == tc.notReason {
				t.Fatalf("failure reason = %q, must remain distinct", reason)
			}
		})
	}
}

func cursorFactoryDir(t *testing.T, title string) string {
	t.Helper()
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":`+mustJSON(title)+`}`))
	return dir
}

func cursorWorkerConfig(skipPermissions bool, timeout string) string {
	lines := []string{
		"---",
		"type: MODEL_WORKER",
		"model: cursor-test-model",
		"modelProvider: " + string(modelprovider.ProviderCursor),
	}
	if skipPermissions {
		lines = append(lines, "skipPermissions: true")
	}
	if timeout != "" {
		lines = append(lines, "timeout: "+timeout)
	}
	lines = append(lines, "stopToken: COMPLETE", "---", "Process the input task.")
	return strings.Join(lines, "\n") + "\n"
}

func cursorSuccessStream(sessionID, result string) []byte {
	records := []string{
		`{"type":"system","subtype":"init","session_id":"` + sessionID + `"}`,
		`{"type":"assistant","timestamp_ms":1,"message":{"role":"assistant","content":[{"type":"text","text":"working"}]},"session_id":"` + sessionID + `"}`,
		`{"type":"tool_call","subtype":"started","call_id":"call-1","tool_call":{"readToolCall":{"args":{"path":"README.md"}}},"session_id":"` + sessionID + `"}`,
		`{"type":"tool_call","subtype":"completed","call_id":"call-1","tool_call":{"readToolCall":{"result":{"success":{}}}},"session_id":"` + sessionID + `"}`,
		string(cursorTerminalRecord(sessionID, result)),
	}
	return []byte(strings.Join(records, "\n"))
}

func cursorTerminalRecord(sessionID, result string) []byte {
	return []byte(
		`{"type":"result","subtype":"success","is_error":false,"result":` +
			mustJSON(result) + `,"session_id":` + mustJSON(sessionID) + `}`,
	)
}

func mustJSON(value string) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return string(encoded)
}

func assertWorkOutcome(t *testing.T, listed factoryapi.ListWorkResponse, done, failed int) {
	t.Helper()
	if got := support.CountWorkAtCustomerState(listed, "task:done"); got != done {
		encoded, _ := json.Marshal(listed)
		t.Fatalf("completed work = %d, want %d; listed=%s", got, done, encoded)
	}
	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != failed {
		encoded, _ := json.Marshal(listed)
		t.Fatalf("failed work = %d, want %d; listed=%s", got, failed, encoded)
	}
}

func assertOrderedCorrelatedCompletion(t *testing.T, events []factoryapi.FactoryEvent, wantOutput string) string {
	t.Helper()
	requestIndex, responseIndex, dispatchResponseIndex := -1, -1, -1
	dispatchID := ""
	for index, event := range events {
		switch event.Type {
		case factoryapi.FactoryEventTypeDispatchRequest:
			if event.Context.DispatchId != nil {
				dispatchID = *event.Context.DispatchId
			}
		case factoryapi.FactoryEventTypeModelRequest:
			if dispatchID != "" && event.Context.DispatchId != nil && *event.Context.DispatchId == dispatchID {
				requestIndex = index
			}
		case factoryapi.FactoryEventTypeModelResponse:
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

func assertCursorResponseLifecycle(
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
		t.Fatalf("Cursor response events = %#v, want exactly %d lifecycle events", events, len(want))
	}
	runID := events[0].RunId
	for index, event := range events {
		if event.Kind != want[index].kind || event.Phase != want[index].phase {
			t.Fatalf("response event[%d] = %s/%s, want %s/%s", index, event.Kind, event.Phase, want[index].kind, want[index].phase)
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
	if err != nil || started.ToolCallId != "call-1" || started.ToolName != "readToolCall" {
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

func assertAttemptSessions(t *testing.T, events []factoryapi.FactoryEvent, failed, succeeded string) {
	t.Helper()
	var failedSession, succeededSession string
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeModelResponse {
			continue
		}
		payload, err := support.AsInferenceResponseObservation(event)
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

func terminalFailureReason(t *testing.T, events []factoryapi.FactoryEvent) factoryapi.WorkFailureType {
	t.Helper()
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeModelResponse {
			continue
		}
		payload, err := support.AsInferenceResponseObservation(event)
		if err != nil {
			t.Fatalf("decode inference response: %v", err)
		}
		if payload.Outcome == factoryapi.InferenceOutcomeFailed && payload.FailureDetail != nil {
			return payload.FailureDetail.Reason
		}
	}
	return ""
}

func assertArg(t *testing.T, args []string, want string) {
	t.Helper()
	if !contains(args, want) {
		t.Fatalf("args = %#v, want %q", args, want)
	}
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

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

type fixedCommandRunner struct {
	mu     sync.Mutex
	result platformprocess.CommandResult
	err    error
	calls  int
}

func (r *fixedCommandRunner) Run(context.Context, platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	return r.result, r.err
}

type recordingTemporaryFileSystem struct {
	mu          sync.Mutex
	sequence    int
	path        string
	content     string
	removedPath string
	creates     int
	closes      int
	removes     int
}

func (f *recordingTemporaryFileSystem) CreateTemp(_ string, _ string) (platformfilesystem.TemporaryFile, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sequence++
	f.creates++
	f.path = "C:\\functional\\cursor-prompt-" + mustJSONNumber(f.sequence) + ".md"
	return &recordingTemporaryFile{files: f, path: f.path}, nil
}

func (f *recordingTemporaryFileSystem) Remove(path string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.removes++
	f.removedPath = path
	return nil
}

func (f *recordingTemporaryFileSystem) CreateCount() int {
	return f.count(func() int { return f.creates })
}
func (f *recordingTemporaryFileSystem) CloseCount() int {
	return f.count(func() int { return f.closes })
}
func (f *recordingTemporaryFileSystem) RemoveCount() int {
	return f.count(func() int { return f.removes })
}

func (f *recordingTemporaryFileSystem) Path() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.path
}

func (f *recordingTemporaryFileSystem) Content() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.content
}

func (f *recordingTemporaryFileSystem) RemovedPath() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.removedPath
}

func (f *recordingTemporaryFileSystem) count(value func() int) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return value()
}

type recordingTemporaryFile struct {
	files *recordingTemporaryFileSystem
	path  string
}

func (f *recordingTemporaryFile) Name() string { return f.path }

func (f *recordingTemporaryFile) WriteString(value string) (int, error) {
	f.files.mu.Lock()
	defer f.files.mu.Unlock()
	f.files.content = value
	return len(value), nil
}

func (f *recordingTemporaryFile) Close() error {
	f.files.mu.Lock()
	defer f.files.mu.Unlock()
	f.files.closes++
	return nil
}

func mustJSONNumber(value int) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return string(encoded)
}

var _ platformprocess.CommandRunner = (*fixedCommandRunner)(nil)
var _ platformfilesystem.TemporaryFileSystem = (*recordingTemporaryFileSystem)(nil)
var _ platformfilesystem.TemporaryFile = (*recordingTemporaryFile)(nil)
