package resume_from_recording_test

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	resumeFromRecordingTimeout = 15 * time.Second
)

// TestKilledFactorySessionResumesOriginalDispatchesAfterRestart proves that a
// real two-stage Work item survives a process boundary through the public
// recording resume surface. The command runner is installed through
// edges.Edges, so the scenario still crosses the real Workers, Factory Runtime,
// Factory Sessions, Work, and Recordings paths.
func TestKilledFactorySessionResumesOriginalDispatchesAfterRestart(t *testing.T) {
	t.Parallel()

	scenario := newResumeFromRecordingScenario(t)
	t.Cleanup(scenario.runner.ReleaseRemainingDispatch)
	first := scenario.startFirstProcess(t)
	firstURL := first.waitForReady(t)
	first.waitFor(t, "first-returned")
	first.waitFor(t, "second-entered")
	submitted := support.SubmitDefaultSessionWork(t, firstURL, factoryapi.SubmitWorkRequest{
		Name:         stringPointer("resume-from-recording-work"),
		Payload:      map[string]any{"subject": "resume-from-recording"},
		WorkTypeName: "task",
	})
	if submitted.Accepted != true || submitted.WorkId == nil || *submitted.WorkId == "" {
		t.Fatalf("public Work submission = %#v, want one accepted Work identity", submitted)
	}
	scenario.workID = *submitted.WorkId
	scenario.requestID = submitted.RequestId
	boundary := scenario.captureAtKillBoundary(t, firstURL)

	// Kill joins the first root-built process before the replacement is
	// constructed. An OS process kill leaves the recording naturally
	// unfinalized; this test never edits or finalizes its persisted state.
	first.killAndJoin(t)
	assertResumeFromRecordingUnfinalizedRecording(t, scenario.recordingPath)

	second := scenario.startSuccessorProcess(t)
	scenario.assertRestored(t, second.URL(), boundary)
	scenario.resumeAndFinish(t, second.URL(), boundary)
}

// TestResumeFromRecordingChildProcess hosts the predecessor in a separate OS
// process so the parent can exercise an actual abrupt process boundary. The
// parent test is the only caller; a normal package run skips this helper.
func TestResumeFromRecordingChildProcess(t *testing.T) {
	if os.Getenv("RSM7_RESUME_CHILD") != "1" {
		t.Skip("predecessor child process helper")
	}
	control, err := net.DialTimeout("tcp", os.Getenv("RSM7_RESUME_CONTROL"), resumeFromRecordingTimeout)
	if err != nil {
		t.Fatalf("connect predecessor control: %v", err)
	}
	defer control.Close()
	childControl := newResumeFromRecordingChildControl(control)
	runner := newResumeFromRecordingChildCommandRunner(childControl)
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                os.Getenv("RSM7_RESUME_FACTORY_DIR"),
		WaitForServiceModeRuntime: true,
		Env: []string{
			"HOME=" + os.Getenv("RSM7_RESUME_HOME"),
			"USERPROFILE=" + os.Getenv("RSM7_RESUME_HOME"),
		},
		Args: []string{
			"--record", os.Getenv("RSM7_RESUME_RECORDING"),
		},
		Edges: serviceedges.Edges{ProviderCommandRunner: runner},
	})
	if err := childControl.send(resumeFromRecordingChildMessage{
		Type: "ready",
		URL:  server.URL(),
	}); err != nil {
		t.Fatalf("announce predecessor readiness: %v", err)
	}
	select {}
}

type resumeFromRecordingScenario struct {
	projectRoot   string
	home          string
	env           []string
	recordingPath string
	successorPath string
	workID        string
	requestID     string
	runner        *resumeFromRecordingCommandRunner
}

type resumeFromRecordingBoundary struct {
	work        factoryapi.Work
	completedID string
	events      []factoryapi.FactoryEvent
	cursor      factoryapi.FactoryEvent
}

func newResumeFromRecordingScenario(t *testing.T) *resumeFromRecordingScenario {
	t.Helper()
	home := t.TempDir()
	return &resumeFromRecordingScenario{
		projectRoot:   setupResumeFromRecordingFixture(t),
		home:          home,
		env:           []string{"HOME=" + home, "USERPROFILE=" + home},
		recordingPath: filepath.Join(home, "killed.recording.json"),
		successorPath: filepath.Join(home, "resumed.recording.json"),
		runner:        newResumeFromRecordingCommandRunner(),
	}
}

func (scenario *resumeFromRecordingScenario) startFirstProcess(
	t *testing.T,
) *resumeFromRecordingChildProcess {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen for predecessor control: %v", err)
	}
	process := &resumeFromRecordingChildProcess{
		listener: listener,
		messages: make(chan resumeFromRecordingChildMessage, 16),
		waitDone: make(chan struct{}),
	}
	go process.acceptMessages()
	command := exec.Command(
		os.Args[0],
		"-test.run=^TestResumeFromRecordingChildProcess$",
		"-test.v",
	)
	command.Env = append(os.Environ(),
		"RSM7_RESUME_CHILD=1",
		"RSM7_RESUME_CONTROL="+listener.Addr().String(),
		"RSM7_RESUME_FACTORY_DIR="+scenario.projectRoot,
		"RSM7_RESUME_HOME="+scenario.home,
		"RSM7_RESUME_RECORDING="+scenario.recordingPath,
	)
	command.Dir = scenario.projectRoot
	var stdout, stderr syncBuffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Start(); err != nil {
		_ = listener.Close()
		t.Fatalf("start predecessor process: %v", err)
	}
	process.command = command
	process.stdout = &stdout
	process.stderr = &stderr
	t.Cleanup(func() { process.stop(t) })
	return process
}

func (scenario *resumeFromRecordingScenario) startSuccessorProcess(
	t *testing.T,
) *support.FunctionalAPIServer {
	t.Helper()
	return support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                scenario.projectRoot,
		WaitForServiceModeRuntime: true,
		Env:                       scenario.env,
		Args:                      []string{"--resume", scenario.recordingPath, "--record", scenario.successorPath},
		Edges:                     serviceedges.Edges{ProviderCommandRunner: scenario.runner},
	})
}

type resumeFromRecordingChildMessage struct {
	Type string `json:"type"`
	URL  string `json:"url,omitempty"`
}

type resumeFromRecordingChildControl struct {
	connection net.Conn
	encoder    *json.Encoder
	mu         sync.Mutex
}

func newResumeFromRecordingChildControl(connection net.Conn) *resumeFromRecordingChildControl {
	return &resumeFromRecordingChildControl{
		connection: connection,
		encoder:    json.NewEncoder(connection),
	}
}

func (control *resumeFromRecordingChildControl) send(message resumeFromRecordingChildMessage) error {
	control.mu.Lock()
	defer control.mu.Unlock()
	return control.encoder.Encode(message)
}

type resumeFromRecordingChildProcess struct {
	listener net.Listener
	command  *exec.Cmd
	messages chan resumeFromRecordingChildMessage
	waitDone chan struct{}
	waitOnce sync.Once
	waitErr  error
	stdout   *syncBuffer
	stderr   *syncBuffer
}

func (process *resumeFromRecordingChildProcess) acceptMessages() {
	connection, err := process.listener.Accept()
	if err != nil {
		close(process.messages)
		return
	}
	defer connection.Close()
	decoder := json.NewDecoder(bufio.NewReader(connection))
	for {
		var message resumeFromRecordingChildMessage
		if err := decoder.Decode(&message); err != nil {
			break
		}
		process.messages <- message
	}
	close(process.messages)
}

func (process *resumeFromRecordingChildProcess) waitForReady(t *testing.T) string {
	t.Helper()
	message := process.waitFor(t, "ready")
	if message.URL == "" {
		t.Fatalf("predecessor readiness message has no URL")
	}
	return message.URL
}

func (process *resumeFromRecordingChildProcess) waitFor(t *testing.T, wantType string) resumeFromRecordingChildMessage {
	t.Helper()
	timer := time.NewTimer(resumeFromRecordingTimeout)
	defer timer.Stop()
	select {
	case message, ok := <-process.messages:
		if !ok {
			t.Fatalf("predecessor exited before control message %q; stdout=%q stderr=%q", wantType, process.stdout.String(), process.stderr.String())
		}
		if message.Type != wantType {
			t.Fatalf("predecessor control message = %q, want %q", message.Type, wantType)
		}
		return message
	case <-timer.C:
		t.Fatalf("predecessor did not send control message %q within %s; stdout=%q stderr=%q", wantType, resumeFromRecordingTimeout, process.stdout.String(), process.stderr.String())
		return resumeFromRecordingChildMessage{}
	}
}

func (process *resumeFromRecordingChildProcess) killAndJoin(t *testing.T) {
	t.Helper()
	if process.command.ProcessState == nil {
		if err := process.command.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
			t.Fatalf("kill predecessor process: %v", err)
		}
	}
	if err := process.join(); err == nil {
		t.Fatal("predecessor process exited cleanly after an explicit kill")
	}
}

func (process *resumeFromRecordingChildProcess) stop(t testing.TB) {
	t.Helper()
	if process == nil || process.command == nil {
		return
	}
	if process.command.ProcessState == nil {
		if err := process.command.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
			t.Errorf("kill predecessor process during cleanup: %v", err)
		}
	}
	_ = process.join()
	_ = process.listener.Close()
}

func (process *resumeFromRecordingChildProcess) join() error {
	process.waitOnce.Do(func() {
		process.waitErr = process.command.Wait()
		close(process.waitDone)
	})
	<-process.waitDone
	return process.waitErr
}

type syncBuffer struct {
	mu   sync.Mutex
	data []byte
}

func (buffer *syncBuffer) Write(data []byte) (int, error) {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	buffer.data = append(buffer.data, data...)
	return len(data), nil
}

func (buffer *syncBuffer) String() string {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	return string(append([]byte(nil), buffer.data...))
}

type resumeFromRecordingChildCommandRunner struct {
	delegate *support.ShapedProviderCommandRunner
	control  *resumeFromRecordingChildControl

	mu    sync.Mutex
	calls int
}

func newResumeFromRecordingChildCommandRunner(
	control *resumeFromRecordingChildControl,
) *resumeFromRecordingChildCommandRunner {
	return &resumeFromRecordingChildCommandRunner{
		control: control,
		delegate: support.NewShapedProviderCommandRunner(
			platformprocess.CommandResult{Stdout: []byte("step-one COMPLETE")},
			platformprocess.CommandResult{Stdout: []byte("step-two interrupted COMPLETE")},
		),
	}
}

func (runner *resumeFromRecordingChildCommandRunner) Run(
	ctx context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	runner.mu.Lock()
	runner.calls++
	call := runner.calls
	runner.mu.Unlock()

	result, err := runner.delegate.Run(ctx, request)
	if err != nil {
		return result, err
	}
	switch call {
	case 1:
		if err := runner.control.send(resumeFromRecordingChildMessage{Type: "first-returned"}); err != nil {
			return platformprocess.CommandResult{}, err
		}
		return result, nil
	case 2:
		if err := runner.control.send(resumeFromRecordingChildMessage{Type: "second-entered"}); err != nil {
			return platformprocess.CommandResult{}, err
		}
		select {
		case <-ctx.Done():
			return platformprocess.CommandResult{}, ctx.Err()
		}
	default:
		return result, nil
	}
}

var _ platformprocess.CommandRunner = (*resumeFromRecordingChildCommandRunner)(nil)

func (scenario *resumeFromRecordingScenario) captureAtKillBoundary(
	t *testing.T,
	baseURL string,
) resumeFromRecordingBoundary {
	t.Helper()
	listed := support.ListDefaultSessionWork(t, baseURL)
	work := requireResumeFromRecordingWork(t, listed, scenario.workID)
	if got := support.WorkItemCustomerLocation(work); got != support.WorkCustomerLocation("task", "processing") {
		t.Fatalf("pre-kill Work location = %q, want task:processing", got)
	}
	events := support.GetFactoryEventsForSessionAt(t, baseURL, factorysessions.DefaultSessionID)
	assertResumeFromRecordingFactoryEvents(t, "before restart", events)
	completedID := requireResumeFromRecordingCompletedDispatchID(t, events)
	return resumeFromRecordingBoundary{
		work:        work,
		completedID: completedID,
		events:      events,
		cursor:      requireResumeFromRecordingFactoryEvent(t, events, factoryapi.FactoryEventTypeDispatchResponse, completedID),
	}
}

func (scenario *resumeFromRecordingScenario) assertRestored(
	t *testing.T,
	baseURL string,
	boundary resumeFromRecordingBoundary,
) {
	t.Helper()
	scenario.runner.wait(t, scenario.runner.thirdEntered, "resumed second provider dispatch")
	listed := support.ListDefaultSessionWork(t, baseURL)
	work := requireResumeFromRecordingWork(t, listed, scenario.workID)
	if work.Name != boundary.work.Name || support.StringPointerValue(work.WorkId) != support.StringPointerValue(boundary.work.WorkId) {
		t.Fatalf("post-restart Work identity changed: %#v -> %#v", boundary.work, work)
	}
	if got := support.WorkItemCustomerLocation(work); got != support.WorkCustomerLocation("task", "processing") {
		t.Fatalf("post-restart Work location = %q, want restored task:processing", got)
	}
	restartedEvents := support.GetFactoryEventsForSessionAt(t, baseURL, factorysessions.DefaultSessionID)
	assertResumeFromRecordingFactoryEvents(t, "after restart", restartedEvents)
	assertResumeFromRecordingFactoryEventIDsPresent(t, boundary.events, restartedEvents)
	support.AssertSingleWorkRequestEvent(t, restartedEvents, scenario.requestID, scenario.workID, "task")
	if got := countResumeFromRecordingDispatchEvents(restartedEvents, factoryapi.FactoryEventTypeDispatchResponse, boundary.completedID); got != 1 {
		t.Fatalf("completed dispatch response events after restart = %d, want 1", got)
	}
}

func (scenario *resumeFromRecordingScenario) resumeAndFinish(
	t *testing.T,
	baseURL string,
	boundary resumeFromRecordingBoundary,
) {
	t.Helper()
	scenario.runner.ReleaseRemainingDispatch()
	finalWork := waitForResumeFromRecordingWorkState(t, baseURL, scenario.workID, "complete")
	if got := support.CountWorkAtCustomerState(finalWork, support.WorkCustomerLocation("task", "complete")); got != 1 {
		t.Fatalf("post-resume completed Work count = %d, want 1; listed=%#v", got, finalWork.Results)
	}
	completedWork := requireResumeFromRecordingWork(t, finalWork, scenario.workID)
	if got := support.WorkItemCustomerLocation(completedWork); got != support.WorkCustomerLocation("task", "complete") {
		t.Fatalf("post-resume Work location = %q, want task:complete", got)
	}
	if got := scenario.runner.CallCount(); got != 3 {
		t.Fatalf("provider command calls after resume = %d, want exactly 3", got)
	}
	finalEvents := support.GetFactoryEventsForSessionAt(t, baseURL, factorysessions.DefaultSessionID)
	assertResumeFromRecordingFactoryEvents(t, "after resume", finalEvents)
	assertResumeFromRecordingFactoryEventIDsPresent(t, boundary.events, finalEvents)
	support.AssertSingleWorkRequestEvent(t, finalEvents, scenario.requestID, scenario.workID, "task")
	if got := countResumeFromRecordingDispatchEvents(finalEvents, factoryapi.FactoryEventTypeDispatchResponse, boundary.completedID); got != 1 {
		t.Fatalf("completed dispatch response events after resume = %d, want 1", got)
	}
	assertResumeFromRecordingFactoryCursorReplay(t, baseURL, scenario, boundary, finalEvents)
}

func assertResumeFromRecordingFactoryEvents(t *testing.T, phase string, events []factoryapi.FactoryEvent) {
	t.Helper()
	if len(events) == 0 {
		t.Fatalf("Factory Events %s = 0, want retained public history", phase)
	}
	seenIDs := make(map[string]struct{}, len(events))
	previousSequence := -1
	previousSessionSequence := -1
	for index, event := range events {
		if event.Id == "" {
			t.Fatalf("Factory Event %s[%d] has an empty identity", phase, index)
		}
		if _, exists := seenIDs[event.Id]; exists {
			t.Fatalf("Factory Event %s[%d] repeats identity %q", phase, index, event.Id)
		}
		seenIDs[event.Id] = struct{}{}
		if event.Context.Sequence <= previousSequence {
			t.Fatalf(
				"Factory Event %s[%d] sequence %d does not strictly follow %d",
				phase,
				index,
				event.Context.Sequence,
				previousSequence,
			)
		}
		previousSequence = event.Context.Sequence
		if event.Context.SessionSequence != nil {
			if *event.Context.SessionSequence <= previousSessionSequence {
				t.Fatalf(
					"Factory Event %s[%d] session sequence %d does not strictly follow %d",
					phase,
					index,
					*event.Context.SessionSequence,
					previousSessionSequence,
				)
			}
			previousSessionSequence = *event.Context.SessionSequence
		}
	}
}

func assertResumeFromRecordingFactoryEventIDsPresent(
	t *testing.T,
	beforeRestart []factoryapi.FactoryEvent,
	afterRestart []factoryapi.FactoryEvent,
) {
	t.Helper()
	afterIDs := make(map[string]struct{}, len(afterRestart))
	for _, event := range afterRestart {
		afterIDs[event.Id] = struct{}{}
	}
	for _, event := range beforeRestart {
		if _, exists := afterIDs[event.Id]; !exists {
			t.Fatalf("Factory Event %q was lost across restart", event.Id)
		}
	}
}

func assertResumeFromRecordingFactoryCursorReplay(
	t *testing.T,
	baseURL string,
	scenario *resumeFromRecordingScenario,
	boundary resumeFromRecordingBoundary,
	finalEvents []factoryapi.FactoryEvent,
) {
	t.Helper()
	cursorIndex := indexResumeFromRecordingFactoryEvent(t, finalEvents, boundary.cursor.Id)
	wantCount := len(finalEvents) - cursorIndex - 1
	if wantCount == 0 {
		t.Fatalf("Factory Event cursor %q has no post-resume events to replay", boundary.cursor.Id)
	}
	replayed := support.GetFactoryEventsAfterAt(
		t,
		baseURL,
		support.FactoryEventReadCursor{AfterEventID: boundary.cursor.Id},
	)
	if len(replayed) != wantCount {
		t.Fatalf("Factory Event cursor replay count = %d, want %d", len(replayed), wantCount)
	}
	seenIDs := make(map[string]struct{}, len(boundary.events)+len(replayed))
	previousSequence := -1
	for _, event := range boundary.events {
		seenIDs[event.Id] = struct{}{}
		previousSequence = event.Context.Sequence
		if event.Id == boundary.cursor.Id {
			break
		}
	}
	for index, event := range replayed {
		if event.Id == boundary.cursor.Id {
			t.Fatalf("Factory Event cursor %q was redelivered", boundary.cursor.Id)
		}
		if _, exists := seenIDs[event.Id]; exists {
			t.Fatalf("Factory Event %q was duplicated across the acknowledged cursor", event.Id)
		}
		if event.Context.Sequence <= previousSequence {
			t.Fatalf(
				"replayed Factory Event %q sequence %d at index %d regressed from %d",
				event.Id,
				event.Context.Sequence,
				index,
				previousSequence,
			)
		}
		want := finalEvents[cursorIndex+index+1]
		if event.Id != want.Id || event.Context.Sequence != want.Context.Sequence {
			t.Fatalf("replayed Factory Event[%d] = %q/%d, want %q/%d", index, event.Id, event.Context.Sequence, want.Id, want.Context.Sequence)
		}
		seenIDs[event.Id] = struct{}{}
		previousSequence = event.Context.Sequence
	}
}

func requireResumeFromRecordingFactoryEvent(
	t *testing.T,
	events []factoryapi.FactoryEvent,
	eventType factoryapi.FactoryEventType,
	dispatchID string,
) factoryapi.FactoryEvent {
	t.Helper()
	for _, event := range events {
		if event.Type == eventType && support.StringPointerValue(event.Context.DispatchId) == dispatchID {
			return event
		}
	}
	t.Fatalf("Factory Event type %q for dispatch %q is missing", eventType, dispatchID)
	return factoryapi.FactoryEvent{}
}

func indexResumeFromRecordingFactoryEvent(
	t *testing.T,
	events []factoryapi.FactoryEvent,
	eventID string,
) int {
	t.Helper()
	for index, event := range events {
		if event.Id == eventID {
			return index
		}
	}
	t.Fatalf("Factory Event cursor %q is missing from retained history", eventID)
	return -1
}

func setupResumeFromRecordingFixture(t *testing.T) string {
	t.Helper()

	projectRoot := support.ScaffoldFactory(t, map[string]any{
		"name": "resume-from-recording",
		"workTypes": []map[string]any{{
			"name": "task",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "processing", "type": "PROCESSING"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]string{
			{"name": "worker-a"},
			{"name": "worker-b"},
		},
		"workstations": []map[string]any{
			{
				"name":      "step-one",
				"worker":    "worker-a",
				"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
				"outputs":   []map[string]string{{"workType": "task", "state": "processing"}},
				"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
			},
			{
				"name":      "step-two",
				"worker":    "worker-b",
				"inputs":    []map[string]string{{"workType": "task", "state": "processing"}},
				"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
				"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
			},
		},
	})
	for _, worker := range []string{"worker-a", "worker-b"} {
		support.WriteAgentConfig(
			t,
			projectRoot,
			worker,
			support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "gpt-5-codex"),
		)
	}
	return projectRoot
}

func assertResumeFromRecordingUnfinalizedRecording(t *testing.T, path string) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read naturally unfinalized recording %q: %v", path, err)
	}
	var artifact struct {
		Events []struct {
			Payload json.RawMessage `json:"payload"`
		} `json:"events"`
	}
	if err := json.Unmarshal(raw, &artifact); err != nil {
		t.Fatalf("decode naturally unfinalized recording %q: %v", path, err)
	}
	for _, event := range artifact.Events {
		var envelope struct {
			WallClock *struct {
				FinishedAt *time.Time `json:"finishedAt,omitempty"`
			} `json:"wallClock,omitempty"`
		}
		if err := json.Unmarshal(event.Payload, &envelope); err != nil {
			continue
		}
		if envelope.WallClock != nil && envelope.WallClock.FinishedAt != nil {
			t.Fatalf("recording %q was finalized before resume at %s", path, envelope.WallClock.FinishedAt)
		}
	}
}

func requireResumeFromRecordingWork(
	t *testing.T,
	listed factoryapi.ListWorkResponse,
	workID string,
) factoryapi.Work {
	t.Helper()
	for _, work := range listed.Results {
		if support.StringPointerValue(work.WorkId) == workID {
			return work
		}
	}
	t.Fatalf("public Work listing is missing %q: %#v", workID, listed.Results)
	return factoryapi.Work{}
}

// waitForResumeFromRecordingWorkState waits on the committed public Work
// projection after a provider release. The runner signal proves the provider
// boundary; this bounded read is still needed because Work projection writes
// are committed asynchronously after the runtime event is emitted.
func waitForResumeFromRecordingWorkState(
	t *testing.T,
	baseURL, workID, state string,
) factoryapi.ListWorkResponse {
	t.Helper()
	wantLocation := support.WorkCustomerLocation("task", state)
	listed, err := support.WaitForObservation(
		resumeFromRecordingTimeout,
		func() (factoryapi.ListWorkResponse, error) {
			return support.ListDefaultSessionWork(t, baseURL), nil
		},
		func(listed factoryapi.ListWorkResponse) bool {
			for _, work := range listed.Results {
				if support.StringPointerValue(work.WorkId) == workID && support.WorkItemCustomerLocation(work) == wantLocation {
					return true
				}
			}
			return false
		},
	)
	if err != nil {
		t.Fatalf("wait for Work %q at %s: %v", workID, wantLocation, err)
	}
	return listed
}

func requireResumeFromRecordingCompletedDispatchID(
	t *testing.T,
	events []factoryapi.FactoryEvent,
) string {
	t.Helper()
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeDispatchResponse {
			continue
		}
		if dispatchID := support.StringPointerValue(event.Context.DispatchId); dispatchID != "" {
			return dispatchID
		}
	}
	t.Fatal("public Factory Events contain no completed dispatch response")
	return ""
}

func countResumeFromRecordingDispatchEvents(
	events []factoryapi.FactoryEvent,
	eventType factoryapi.FactoryEventType,
	dispatchID string,
) int {
	count := 0
	for _, event := range events {
		if event.Type == eventType && support.StringPointerValue(event.Context.DispatchId) == dispatchID {
			count++
		}
	}
	return count
}

func stringPointer(value string) *string {
	return &value
}

type resumeFromRecordingCommandRunner struct {
	delegate *support.ShapedProviderCommandRunner

	mu                  sync.Mutex
	calls               int
	thirdEntered        chan struct{}
	releaseRemaining    chan struct{}
	releaseRemainingOne sync.Once
}

func newResumeFromRecordingCommandRunner() *resumeFromRecordingCommandRunner {
	return &resumeFromRecordingCommandRunner{
		delegate: support.NewShapedProviderCommandRunner(
			platformprocess.CommandResult{Stdout: []byte("step-two resumed COMPLETE")},
		),
		calls:            2,
		thirdEntered:     make(chan struct{}),
		releaseRemaining: make(chan struct{}),
	}
}

func (runner *resumeFromRecordingCommandRunner) Run(
	ctx context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	runner.mu.Lock()
	runner.calls++
	call := runner.calls
	runner.mu.Unlock()

	result, err := runner.delegate.Run(ctx, request)
	if err != nil {
		return result, err
	}
	switch call {
	case 3:
		close(runner.thirdEntered)
		select {
		case <-runner.releaseRemaining:
			return result, nil
		case <-ctx.Done():
			return platformprocess.CommandResult{}, ctx.Err()
		}
	default:
		return result, nil
	}
}

func (runner *resumeFromRecordingCommandRunner) CallCount() int {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return runner.calls
}

func (runner *resumeFromRecordingCommandRunner) ReleaseRemainingDispatch() {
	runner.releaseRemainingOne.Do(func() { close(runner.releaseRemaining) })
}

func (runner *resumeFromRecordingCommandRunner) wait(t *testing.T, signal <-chan struct{}, name string) {
	t.Helper()
	timer := time.NewTimer(resumeFromRecordingTimeout)
	defer timer.Stop()
	select {
	case <-signal:
	case <-timer.C:
		t.Fatalf("%s did not reach its deterministic signal within %s", name, resumeFromRecordingTimeout)
	}
}

var _ platformprocess.CommandRunner = (*resumeFromRecordingCommandRunner)(nil)
