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
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	platformreplay "github.com/portpowered/infinite-you/pkg/platform/replay"
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
	submitted := support.SubmitDefaultSessionWork(t, firstURL, factoryapi.SubmitWorkRequest{
		Name:         stringPointer("resume-from-recording-work"),
		Payload:      map[string]any{"subject": "resume-from-recording"},
		WorkTypeName: "task",
	})
	if submitted.Accepted != true || submitted.WorkId == nil || *submitted.WorkId == "" {
		t.Fatalf("public Work submission = %#v, want one accepted Work identity", submitted)
	}
	scenario.workID = *submitted.WorkId
	first.waitFor(t, "first-returned")
	first.waitFor(t, "second-entered")
	boundary := scenario.captureAtKillBoundary(t, firstURL)
	first.waitForRecordingCheckpoint(t)

	// Kill joins the first root-built process before the replacement is
	// constructed. An OS process kill leaves the recording naturally
	// unfinalized; this test never edits or finalizes its persisted state.
	first.killAndJoin(t)
	assertResumeFromRecordingUnfinalizedRecording(t, scenario.recordingPath)

	second := scenario.startSuccessorProcess(t)
	scenario.assertRestored(t, second.URL(), boundary)
	scenario.resumeAndFinish(t, second.URL(), boundary)
}

// TestSuccessorRecordingReplaysAgainstUnchangedCheckout proves successor recordings replay against an unchanged checkout.
func TestSuccessorRecordingReplaysAgainstUnchangedCheckout(t *testing.T) {
	t.Parallel()

	scenario := newResumeFromRecordingScenario(t)
	t.Cleanup(scenario.runner.ReleaseRemainingDispatch)
	first := scenario.startFirstProcess(t)
	firstURL := first.waitForReady(t)
	submitted := support.SubmitDefaultSessionWork(t, firstURL, factoryapi.SubmitWorkRequest{
		Name:         stringPointer("resume-from-recording-replay-work"),
		Payload:      map[string]any{"subject": "resume-from-recording-replay"},
		WorkTypeName: "task",
	})
	if submitted.WorkId == nil || *submitted.WorkId == "" {
		t.Fatalf("public Work submission = %#v, want one accepted Work identity", submitted)
	}
	scenario.workID = *submitted.WorkId
	first.waitFor(t, "first-returned")
	first.waitFor(t, "second-entered")
	boundary := scenario.captureAtKillBoundary(t, firstURL)
	first.waitForRecordingCheckpoint(t)
	first.killAndJoin(t)

	second := scenario.startSuccessorProcess(t)
	scenario.assertRestored(t, second.URL(), boundary)
	scenario.resumeAndFinish(t, second.URL(), boundary)
	second.Stop(t)
	assertSuccessorRecordingPreservesRunMetadata(t, scenario.recordingPath, scenario.successorPath)

	api := support.NewProcessAPIServer()
	process := support.BuildProcess(t, serviceedges.Edges{APIServerStarter: api.Start})
	support.CleanupProcess(t, process)
	inputs := support.FakeInputs(context.Background(), []string{
		"you", "run", "--dir", scenario.projectRoot, "--server", "http://127.0.0.1:1",
		"--quiet", "--replay", scenario.successorPath, "--no-record",
	})
	inputs.Env = append([]string(nil), scenario.env...)
	inputs.WorkingDirectory = scenario.projectRoot
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf("successor replay failed: %v; stdout=%q stderr=%q", err, inputs.Stdout(), inputs.Stderr())
	}
	if got := inputs.Stdout(); got != "" {
		t.Fatalf("successor replay stdout = %q, want no drift warning", got)
	}
}

func assertSuccessorRecordingPreservesRunMetadata(t *testing.T, sourcePath, successorPath string) {
	t.Helper()
	sourceMetadata := runRequestMetadata(t, sourcePath)
	successorMetadata := runRequestMetadata(t, successorPath)
	for _, key := range []string{
		"factory_hash",
		"runtime_config_hash",
		"workers_hash",
		"workstations_hash",
	} {
		if sourceMetadata[key] != successorMetadata[key] {
			t.Fatalf("successor RUN_REQUEST metadata[%q] = %q, want source value %q", key, successorMetadata[key], sourceMetadata[key])
		}
	}
}

func runRequestMetadata(t *testing.T, path string) map[string]string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read recording %q: %v", path, err)
	}
	var artifact struct {
		Events []struct {
			Type    string          `json:"type"`
			Payload json.RawMessage `json:"payload"`
		} `json:"events"`
	}
	if err := json.Unmarshal(raw, &artifact); err != nil {
		t.Fatalf("decode recording %q: %v", path, err)
	}
	for _, event := range artifact.Events {
		if event.Type != string(factoryapi.FactoryEventTypeRunRequest) {
			continue
		}
		var payload struct {
			Factory struct {
				Metadata map[string]string `json:"metadata"`
			} `json:"factory"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			t.Fatalf("decode RUN_REQUEST metadata in %q: %v", path, err)
		}
		return payload.Factory.Metadata
	}
	t.Fatalf("recording %q contains no RUN_REQUEST metadata", path)
	return nil
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
	writeRecording := newResumeFromRecordingWriteFile(childControl)
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
		Edges: serviceedges.Edges{
			ProviderCommandRunner: runner,
			RecordingWriteFile:    writeRecording,
		},
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
	runner        *resumeFromRecordingCommandRunner
}

type resumeFromRecordingBoundary struct {
	work                 factoryapi.Work
	completedDispatchID  string
	completedEventID     string
	completedEventCursor support.FactoryEventReadCursor
	events               []factoryapi.FactoryEvent
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
	t.Cleanup(func() {
		process.stop(t)
		if t.Failed() {
			t.Logf("predecessor output after cleanup: stdout=%q stderr=%q", stdout.String(), stderr.String())
		}
	})
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
	Type                string `json:"type"`
	URL                 string `json:"url,omitempty"`
	EventCount          int    `json:"eventCount,omitempty"`
	CompletedDispatches int    `json:"completedDispatches,omitempty"`
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
	for {
		select {
		case message, ok := <-process.messages:
			if !ok {
				t.Fatalf("predecessor exited before control message %q; stdout=%q stderr=%q", wantType, process.stdout.String(), process.stderr.String())
			}
			if message.Type == "recording-flushed" {
				continue
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
}

func (process *resumeFromRecordingChildProcess) waitForRecordingCheckpoint(t *testing.T) {
	t.Helper()
	timer := time.NewTimer(resumeFromRecordingTimeout)
	defer timer.Stop()
	for {
		select {
		case message, ok := <-process.messages:
			if !ok {
				t.Fatalf("predecessor exited before recording checkpoint; stdout=%q stderr=%q", process.stdout.String(), process.stderr.String())
			}
			if message.Type == "recording-flushed" && message.CompletedDispatches > 0 {
				return
			}
		case <-timer.C:
			t.Fatalf("predecessor did not durably flush a completed dispatch within %s; stdout=%q stderr=%q", resumeFromRecordingTimeout, process.stdout.String(), process.stderr.String())
		}
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

func newResumeFromRecordingWriteFile(
	control *resumeFromRecordingChildControl,
) func(string, []byte) error {
	storage := platformreplay.NewLocal(runtime.GOOS)
	return func(path string, data []byte) error {
		if err := storage.WriteFile(path, data); err != nil {
			return err
		}
		var artifact struct {
			Events []struct {
				Type string `json:"type"`
			} `json:"events"`
		}
		if err := json.Unmarshal(data, &artifact); err != nil {
			return err
		}
		completed := 0
		for _, event := range artifact.Events {
			if strings.EqualFold(event.Type, string(factoryapi.FactoryEventTypeDispatchResponse)) {
				completed++
			}
		}
		return control.send(resumeFromRecordingChildMessage{
			Type:                "recording-flushed",
			EventCount:          len(artifact.Events),
			CompletedDispatches: completed,
		})
	}
}

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
	session := support.GetDefaultSession(t, baseURL)
	if session.Runtime.Progress.Categories.Initial != 0 || session.Runtime.Progress.InFlightCount != 1 {
		t.Fatalf("pre-kill board progress = %+v, in-flight=%d; want one in-flight processing Work", session.Runtime.Progress.Categories, session.Runtime.Progress.InFlightCount)
	}
	events := support.GetFactoryEventsForSessionAt(t, baseURL, factorysessions.DefaultSessionID)
	assertResumeFromRecordingFactoryEvents(t, "before restart", events)
	completedEvent := requireResumeFromRecordingCompletedEvent(t, events)
	completedSequence := support.ReconnectSequenceForFactoryEvent(completedEvent)
	return resumeFromRecordingBoundary{
		work:                work,
		completedDispatchID: support.StringPointerValue(completedEvent.Context.DispatchId),
		completedEventID:    completedEvent.Id,
		completedEventCursor: support.FactoryEventReadCursor{
			AfterEventID:  completedEvent.Id,
			AfterSequence: &completedSequence,
		},
		events: events,
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
	session := support.GetDefaultSession(t, baseURL)
	if session.Runtime.Progress.Categories.Initial != 0 || session.Runtime.Progress.InFlightCount != 1 {
		t.Fatalf("post-restart board progress = %+v, in-flight=%d; want one restored in-flight processing Work", session.Runtime.Progress.Categories, session.Runtime.Progress.InFlightCount)
	}
	restartedEvents := support.GetFactoryEventsForSessionAt(t, baseURL, factorysessions.DefaultSessionID)
	assertResumeFromRecordingFactoryEvents(t, "after restart", restartedEvents)
	assertResumeFromRecordingSuccessorEvents(t, boundary.events, restartedEvents, boundary)
	assertResumeFromRecordingCursorSuffix(t, baseURL, boundary, restartedEvents)
	if got := countResumeFromRecordingDispatchEvents(restartedEvents, factoryapi.FactoryEventTypeDispatchResponse, boundary.completedDispatchID); got != 1 {
		t.Fatalf("completed dispatch response events after restart = %d, want the one preserved completion and no re-execution", got)
	}
}

func (scenario *resumeFromRecordingScenario) resumeAndFinish(
	t *testing.T,
	baseURL string,
	boundary resumeFromRecordingBoundary,
) {
	t.Helper()
	scenario.runner.ReleaseRemainingDispatch()
	terminalStatus := support.WaitForSessionTerminalStatus(
		t, baseURL, factorysessions.DefaultSessionID, resumeFromRecordingTimeout,
	)
	if terminalStatus.RuntimeStatus != string(factoryapi.FactorySessionStatusIDLE) &&
		terminalStatus.RuntimeStatus != string(factoryapi.FactorySessionStatusFINISHED) {
		t.Fatalf("post-resume public session runtime status = %q, want IDLE or FINISHED", terminalStatus.RuntimeStatus)
	}
	terminalSession := support.GetDefaultSession(t, baseURL)
	if terminalSession.Runtime.Status != factoryapi.FactorySessionStatusIDLE &&
		terminalSession.Runtime.Status != factoryapi.FactorySessionStatusFINISHED {
		t.Fatalf("post-resume Factory Session runtime = %#v, want terminal IDLE or FINISHED", terminalSession.Runtime)
	}
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
	assertResumeFromRecordingSuccessorEvents(t, boundary.events, finalEvents, boundary)
	assertResumeFromRecordingCursorSuffix(t, baseURL, boundary, finalEvents)
	if got := countResumeFromRecordingDispatchEvents(finalEvents, factoryapi.FactoryEventTypeDispatchResponse, boundary.completedDispatchID); got != 1 {
		t.Fatalf("completed dispatch response events after resume = %d, want the one preserved completion and no re-execution", got)
	}
}

func assertResumeFromRecordingSuccessorEvents(
	t *testing.T,
	predecessor []factoryapi.FactoryEvent,
	successor []factoryapi.FactoryEvent,
	boundary resumeFromRecordingBoundary,
) {
	t.Helper()
	cursorIndex := indexResumeFromRecordingEvent(predecessor, boundary.completedEventID)
	if cursorIndex < 0 {
		t.Fatal("predecessor Factory Event history has no acknowledged completion event")
	}
	successorCursorIndex := indexResumeFromRecordingEvent(successor, boundary.completedEventID)
	if successorCursorIndex < 0 {
		t.Fatalf("successor Factory Event history has no acknowledged event %q", boundary.completedEventID)
	}
	if successorCursorIndex != cursorIndex {
		t.Fatalf("successor acknowledged event index = %d, want predecessor index %d", successorCursorIndex, cursorIndex)
	}
	// The pre-kill snapshot can include events emitted after the durable
	// checkpoint that made the acknowledged completion restartable. Only the
	// prefix through that acknowledged cursor is therefore canonical across
	// the process boundary; the successor suffix is checked from its public
	// cursor below.
	for index, expected := range predecessor[:cursorIndex+1] {
		actual := successor[index]
		if actual.Id != expected.Id || actual.Type != expected.Type || actual.Context.Sequence != expected.Context.Sequence ||
			support.ReconnectSequenceForFactoryEvent(actual) != support.ReconnectSequenceForFactoryEvent(expected) {
			t.Fatalf("successor Factory Event prefix[%d] = %#v, want predecessor %#v", index, actual, expected)
		}
	}
	combined := append([]factoryapi.FactoryEvent(nil), predecessor[:cursorIndex+1]...)
	combined = append(combined, successor[successorCursorIndex+1:]...)
	assertResumeFromRecordingFactoryEvents(t, "across restart", combined)
}

func assertResumeFromRecordingCursorSuffix(
	t *testing.T,
	baseURL string,
	boundary resumeFromRecordingBoundary,
	successor []factoryapi.FactoryEvent,
) {
	t.Helper()
	cursorIndex := indexResumeFromRecordingEvent(successor, boundary.completedEventID)
	if cursorIndex < 0 {
		t.Fatalf("successor Factory Event history has no acknowledged event %q", boundary.completedEventID)
	}
	want := successor[cursorIndex+1:]
	byID := support.GetFactoryEventsAfterForSessionAt(
		t, baseURL, factorysessions.DefaultSessionID,
		support.FactoryEventReadCursor{AfterEventID: boundary.completedEventID},
	)
	assertResumeFromRecordingCursorEvents(t, "after_event_id", boundary.completedEventID, want, byID)
	bySequence := support.GetFactoryEventsAfterForSessionAt(
		t, baseURL, factorysessions.DefaultSessionID,
		support.FactoryEventReadCursor{AfterSequence: boundary.completedEventCursor.AfterSequence},
	)
	assertResumeFromRecordingCursorEvents(t, "after_sequence", boundary.completedEventID, want, bySequence)
}

func assertResumeFromRecordingCursorEvents(
	t *testing.T,
	cursorKind string,
	acknowledgedID string,
	want []factoryapi.FactoryEvent,
	got []factoryapi.FactoryEvent,
) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("Factory Event %s suffix length = %d, want %d", cursorKind, len(got), len(want))
	}
	for index, expected := range want {
		actual := got[index]
		if actual.Id == acknowledgedID {
			t.Fatalf("Factory Event %s suffix redelivered acknowledged event %q", cursorKind, acknowledgedID)
		}
		if actual.Id != expected.Id || actual.Type != expected.Type || actual.Context.Sequence != expected.Context.Sequence ||
			support.ReconnectSequenceForFactoryEvent(actual) != support.ReconnectSequenceForFactoryEvent(expected) {
			t.Fatalf("Factory Event %s suffix[%d] = %#v, want %#v", cursorKind, index, actual, expected)
		}
	}
}

func indexResumeFromRecordingEvent(events []factoryapi.FactoryEvent, eventID string) int {
	for index, event := range events {
		if event.Id == eventID {
			return index
		}
	}
	return -1
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

func requireResumeFromRecordingCompletedEvent(
	t *testing.T,
	events []factoryapi.FactoryEvent,
) factoryapi.FactoryEvent {
	t.Helper()
	for _, event := range events {
		if event.Type == factoryapi.FactoryEventTypeDispatchReconciled {
			payload, err := event.Payload.AsDispatchReconciledEventPayload()
			if err == nil && payload.ReconciledStatus == factoryapi.FactoryDispatchStatusCOMPLETED {
				return event
			}
		}
	}
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeDispatchResponse {
			continue
		}
		if dispatchID := support.StringPointerValue(event.Context.DispatchId); dispatchID != "" {
			return event
		}
	}
	t.Fatal("public Factory Events contain no completed dispatch event")
	return factoryapi.FactoryEvent{}
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
