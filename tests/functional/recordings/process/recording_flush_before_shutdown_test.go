package recordingsprocess_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	platformreplay "github.com/portpowered/infinite-you/pkg/platform/replay"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	recordingShutdownObservationTimeout = 15 * time.Second
)

// TestRecordingFlushBeforeProcessExecuteReturns proves the reusable lifecycle
// behavior through root.BuildProcess and Process.Execute. The provider edge
// holds one live dispatch until cancellation, so the only terminal failure
// event is produced by the orderly-stop path under test.
func TestRecordingFlushBeforeProcessExecuteReturns(t *testing.T) {
	t.Parallel()
	dir := support.ScaffoldSingleStepFactory(t, "recording-flush-before-shutdown")
	support.WriteAgentConfig(t, dir, "processor", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "gpt-5-codex"))
	recordPath := filepath.Join(t.TempDir(), "shutdown.replay.json")
	runner := newRecordingShutdownBlockingRunner()
	writer := newRecordingShutdownWriteProbe()

	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		Args:                      []string{"--record", recordPath},
		WaitForServiceModeRuntime: true,
		Edges: serviceedges.Edges{
			ProviderCommandRunner: runner,
			RecordingWriteFile:    writer.WriteFile,
		},
	})

	name := "recording-flush-before-shutdown-work"
	submitRecordingShutdownWork(t, server, name)
	awaitRecordingShutdownSignal(t, runner.started, "provider dispatch start")

	before, err := waitForStandaloneRecordingEvent(recordPath, "DISPATCH_REQUEST", recordingShutdownObservationTimeout)
	if err != nil {
		t.Fatalf("wait for durable DISPATCH_REQUEST: %v", err)
	}
	beforeInfo, err := os.Stat(recordPath)
	if err != nil {
		t.Fatalf("stat recording before orderly stop: %v", err)
	}
	writesBeforeStop := writer.WriteCount()
	bytesBeforeStop := writer.BytesWritten()

	// Stop waits for Process.Execute to return, which includes the injected
	// initializer orderly-stop hook and its synchronous Recordings flush.
	server.Stop(t)

	after, err := readStandaloneRecording(recordPath)
	if err != nil {
		t.Fatalf("standalone parse after orderly stop: %v", err)
	}
	afterInfo, err := os.Stat(recordPath)
	if err != nil {
		t.Fatalf("stat recording after orderly stop: %v", err)
	}
	failed, err := standaloneFailureEvents(after)
	if err != nil {
		t.Fatalf("inspect standalone failure events: %v", err)
	}
	if len(failed) == 0 {
		t.Fatalf("standalone recording has no failure event after orderly stop: events=%s", standaloneRecordingEventSummaries(after))
	}
	if len(after.Events) <= len(before.Events) {
		t.Fatalf("standalone event count after orderly stop = %d, want > durable baseline %d", len(after.Events), len(before.Events))
	}
	if !afterInfo.ModTime().After(beforeInfo.ModTime()) {
		t.Fatalf("recording mtime after orderly stop = %s, want later than baseline %s", afterInfo.ModTime(), beforeInfo.ModTime())
	}

	writesAfterStop := writer.WriteCount()
	if writesAfterStop != writesBeforeStop+1 {
		t.Fatalf("recording writes after durable dispatch baseline = %d, want exactly one final whole-file write (baseline=%d)", writesAfterStop, writesBeforeStop)
	}
	bytesAfterStop := writer.BytesWritten()
	if bytesAfterStop <= bytesBeforeStop {
		t.Fatalf("recording bytes after orderly stop = %d, want more than steady-state baseline %d", bytesAfterStop, bytesBeforeStop)
	}
	t.Logf(
		"orderly recording durability before_mtime=%s after_mtime=%s before_events=%d after_events=%d before_writes=%d after_writes=%d before_bytes=%d after_bytes=%d final_flush_bytes=%d failure_events=%v",
		beforeInfo.ModTime().UTC().Format(time.RFC3339Nano),
		afterInfo.ModTime().UTC().Format(time.RFC3339Nano),
		len(before.Events),
		len(after.Events),
		writesBeforeStop,
		writesAfterStop,
		bytesBeforeStop,
		bytesAfterStop,
		bytesAfterStop-bytesBeforeStop,
		failed,
	)
}

func submitRecordingShutdownWork(t testing.TB, server *support.FunctionalAPIServer, name string) {
	t.Helper()
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "--server", server.URL(), "--json", "submit",
		"--name", name, "--work-type-name", "task", "--payload", "-",
	})
	inputs.Input.Stdin = strings.NewReader(`{"title":"hold the live dispatch"}`)
	stdinIsTTY := false
	inputs.Input.StdinIsTTY = &stdinIsTTY
	if err := server.Execute(t, inputs.Input); err != nil {
		t.Fatalf("Process.Execute(submit) error = %v\nstdout:\n%s\nstderr:\n%s", err, inputs.Stdout(), inputs.Stderr())
	}
}

type recordingShutdownBlockingRunner struct {
	started chan struct{}
	starts  chan struct{}
	once    sync.Once
}

func newRecordingShutdownBlockingRunner() *recordingShutdownBlockingRunner {
	return &recordingShutdownBlockingRunner{
		started: make(chan struct{}),
		starts:  make(chan struct{}, 16),
	}
}

func (runner *recordingShutdownBlockingRunner) Run(ctx context.Context, _ platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	runner.once.Do(func() { close(runner.started) })
	runner.starts <- struct{}{}
	<-ctx.Done()
	// A canceled provider subprocess reports its termination as an execution
	// failure to the runtime; returning context.Canceled here would exercise the
	// caller-cancellation suppression path instead of the failure transition
	// whose durable publication this test is proving.
	return platformprocess.CommandResult{}, errors.New("recording shutdown provider process terminated")
}

func awaitRecordingShutdownStarts(t testing.TB, starts <-chan struct{}, count int, name string) {
	t.Helper()
	timer := time.NewTimer(recordingShutdownObservationTimeout)
	defer timer.Stop()
	for index := 0; index < count; index++ {
		select {
		case <-starts:
		case <-timer.C:
			t.Fatalf("timed out waiting for %s (%d/%d)", name, index, count)
		}
	}
}

type recordingShutdownWriteProbe struct {
	mu           sync.Mutex
	writes       int
	bytesWritten int64
	local        platformreplay.Local
}

func newRecordingShutdownWriteProbe() *recordingShutdownWriteProbe {
	return &recordingShutdownWriteProbe{local: platformreplay.NewLocal(runtime.GOOS)}
}

func (probe *recordingShutdownWriteProbe) WriteFile(path string, data []byte) error {
	probe.mu.Lock()
	probe.writes++
	probe.bytesWritten += int64(len(data))
	probe.mu.Unlock()
	return probe.local.WriteFile(path, data)
}

func (probe *recordingShutdownWriteProbe) WriteCount() int {
	probe.mu.Lock()
	defer probe.mu.Unlock()
	return probe.writes
}

func (probe *recordingShutdownWriteProbe) BytesWritten() int64 {
	probe.mu.Lock()
	defer probe.mu.Unlock()
	return probe.bytesWritten
}

func awaitRecordingShutdownWritesToSettle(t testing.TB, probe *recordingShutdownWriteProbe, quietPeriod time.Duration) {
	t.Helper()
	deadline := time.Now().Add(recordingShutdownObservationTimeout)
	lastWrites := probe.WriteCount()
	unchangedSince := time.Now()
	for time.Now().Before(deadline) {
		time.Sleep(25 * time.Millisecond)
		writes := probe.WriteCount()
		if writes != lastWrites {
			lastWrites = writes
			unchangedSince = time.Now()
			continue
		}
		if time.Since(unchangedSince) >= quietPeriod {
			return
		}
	}
	t.Fatalf("recording writes did not settle: writes=%d", lastWrites)
}

func awaitRecordingShutdownSignal(t testing.TB, signal <-chan struct{}, name string) {
	t.Helper()
	timer := time.NewTimer(recordingShutdownObservationTimeout)
	defer timer.Stop()
	select {
	case <-signal:
	case <-timer.C:
		t.Fatalf("timed out waiting for %s", name)
	}
}

// The process-boundary tests intentionally poll only the artifact path. The
// recording writer is asynchronous by design, so this bounded observation is
// required to distinguish a durable snapshot from an in-memory event.
func waitForStandaloneRecordingEvent(path, eventType string, timeout time.Duration) (standaloneRecording, error) {
	return waitForStandaloneRecordingEventCount(path, eventType, 1, timeout)
}

func waitForStandaloneRecordingEventCount(
	path string,
	eventType string,
	minimumCount int,
	timeout time.Duration,
) (standaloneRecording, error) {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for {
		recording, err := readStandaloneRecording(path)
		if err == nil {
			lastErr = nil
			count := 0
			for _, event := range recording.Events {
				if event.Type == eventType {
					count++
				}
			}
			if count >= minimumCount {
				return recording, nil
			}
		} else {
			// The platform writer replaces the artifact while this observer
			// reads it. Missing, partially replaced, and Windows sharing errors
			// are all transient until the next complete snapshot is visible.
			lastErr = err
		}
		if time.Now().After(deadline) {
			if lastErr != nil {
				return standaloneRecording{}, fmt.Errorf("timed out waiting for %s in %q: %w", eventType, path, lastErr)
			}
			return standaloneRecording{}, fmt.Errorf("timed out waiting for %s in %q", eventType, path)
		}
		timer := time.NewTimer(25 * time.Millisecond)
		<-timer.C
	}
}

type standaloneRecording struct {
	Events []standaloneRecordingEvent `json:"events"`
}

type standaloneRecordingEvent struct {
	ID      string                          `json:"id"`
	Type    string                          `json:"type"`
	Context standaloneRecordingEventContext `json:"context"`
	Payload json.RawMessage                 `json:"payload"`
}

type standaloneRecordingEventContext struct {
	DispatchID *string `json:"dispatchId,omitempty"`
}

type standaloneFailureEvent struct {
	EventType     string
	EventID       string
	DispatchID    string
	Outcome       string
	Error         string
	FailureDetail string
}

func readStandaloneRecording(path string) (standaloneRecording, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return standaloneRecording{}, err
	}
	var recording standaloneRecording
	if err := json.Unmarshal(data, &recording); err != nil {
		return standaloneRecording{}, fmt.Errorf("decode JSON: %w", err)
	}
	return recording, nil
}

func standaloneFailureEvents(recording standaloneRecording) ([]standaloneFailureEvent, error) {
	failed := make([]standaloneFailureEvent, 0)
	for _, event := range recording.Events {
		if event.Type != "DISPATCH_RESPONSE" && event.Type != "MODEL_RESPONSE" {
			continue
		}
		var payload struct {
			Outcome       string          `json:"outcome"`
			Error         string          `json:"error"`
			FailureDetail json.RawMessage `json:"failureDetail"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			return nil, fmt.Errorf("decode %s payload %q: %w", event.Type, event.ID, err)
		}
		if strings.EqualFold(payload.Outcome, "FAILED") {
			dispatchID := ""
			if event.Context.DispatchID != nil {
				dispatchID = *event.Context.DispatchID
			}
			failed = append(failed, standaloneFailureEvent{
				EventType:     event.Type,
				EventID:       event.ID,
				DispatchID:    dispatchID,
				Outcome:       payload.Outcome,
				Error:         payload.Error,
				FailureDetail: strings.TrimSpace(string(payload.FailureDetail)),
			})
		}
	}
	return failed, nil
}

func standaloneRecordingEventSummaries(recording standaloneRecording) string {
	parts := make([]string, 0, len(recording.Events))
	for _, event := range recording.Events {
		parts = append(parts, fmt.Sprintf("%s:%s", event.Type, strings.TrimSpace(string(event.Payload))))
	}
	return strings.Join(parts, " | ")
}
