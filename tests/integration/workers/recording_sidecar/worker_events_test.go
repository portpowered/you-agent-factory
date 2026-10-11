package recording_sidecar_test

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	api "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// Both cells consume the upstream immutable artifact, never build a binary,
// and use private profiles and a controlled native provider command.
// They run sequentially to keep host, CLI and Windows cmd/node fixture
// processes within the lane's four-process budget.
func TestWorkerEventsFiniteReplayRestart(t *testing.T) {
	f := newInvokeArtifactFixture(t)
	f.env = append(f.env, "T7_SHIM_COMPLETE=1")
	host := startHistoryHost(t, f.ctx, f.binary, f.project, f.env)
	f.cli(t, host.url, "invoke", "--execution", f.document)
	before := readWorkerEventsArtifact(t, f.ctx, host.url, "?replayOnly=true")
	assertWorkerEventsArtifactTerminal(t, before)
	ordinary := readWorkerEventsArtifact(t, f.ctx, host.url, "")
	assertWorkerEventsArtifactTerminal(t, ordinary)
	host.stop(t, f.ctx, f.binary, f.project, f.env)
	for _, name := range []string{".codex", ".cursor", ".claude"} {
		if err := os.WriteFile(filepath.Join(f.home, name), []byte("no provider files"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	restarted := startHistoryHost(t, f.ctx, f.binary, f.project, f.env)
	after := readWorkerEventsArtifact(t, f.ctx, restarted.url, "?replayOnly=true")
	assertWorkerEventsArtifactTerminal(t, after)
	// Delivery kind and summary placement can differ between retained Events
	// and archived capture; compare the exact customer event records.
	if !reflect.DeepEqual(workerEventsArtifactRecords(before), workerEventsArtifactRecords(after)) {
		t.Fatal("restart changed event content, identities, cursors or stored timestamps")
	}
	restarted.stop(t, f.ctx, f.binary, f.project, f.env)
	assertWorkerEventsArtifactDamagedSource(t, f, after)
}

// A stopped-host filesystem fault is a real artifact edge, not a reader fake.
// A torn tail keeps the accepted prefix with degraded completion; completing
// that malformed line instead makes setup fail with the existing typed error.
func assertWorkerEventsArtifactDamagedSource(t *testing.T, f invokeArtifactFixture, want []api.WorkerSessionEvent) {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(f.project, ".you-agent-factory", "worker-recordings", "*.worker.jsonl"))
	if err != nil || len(paths) != 1 {
		t.Fatalf("private capture fixture: %v %v", paths, err)
	}
	appendWorkerEventsArtifactFault(t, paths[0], "private damaged capture")
	degraded := startHistoryHost(t, f.ctx, f.binary, f.project, f.env)
	for _, query := range []string{"?replayOnly=true&limit=1", "?limit=1"} {
		frames := readWorkerEventsArtifact(t, f.ctx, degraded.url, query)
		if !reflect.DeepEqual(workerEventsArtifactRecords(frames), workerEventsArtifactRecords(want)) {
			t.Fatal("damaged tail changed accepted records or stored timestamps")
		}
		last := frames[len(frames)-1]
		if last.ReplaySummary == nil || last.ReplaySummary.Complete || last.ReplaySummary.Reason != "capture-DEGRADED" || last.ReplaySummary.EventsEmitted != int64(len(workerEventsArtifactRecords(want))) {
			t.Fatalf("damaged tail fabricated completion: %+v", last)
		}
		for _, frame := range frames {
			if frame.Delivery == api.WorkerSessionEventDeliveryTerminalReplay || frame.Delivery == api.WorkerSessionEventDeliverySourceFailure {
				t.Fatalf("damaged tail lost truthful incomplete replay: %+v", frame)
			}
		}
	}
	degraded.stop(t, f.ctx, f.binary, f.project, f.env)
	appendWorkerEventsArtifactFault(t, paths[0], "\n")
	failed := startHistoryHost(t, f.ctx, f.binary, f.project, f.env)
	request, err := http.NewRequestWithContext(f.ctx, http.MethodGet, failed.url+"/worker-sessions/t7-artifact-session/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	body, readErr := io.ReadAll(response.Body)
	response.Body.Close()
	var diagnostic api.ErrorResponse
	if readErr != nil || json.Unmarshal(body, &diagnostic) != nil || response.StatusCode != http.StatusInternalServerError || diagnostic.Code != api.ErrorResponseCodePROJECTIONUNAVAILABLE || response.Header.Get("Content-Type") == "text/event-stream" || strings.Contains(string(body), "private damaged capture") {
		t.Fatalf("corrupt source setup concealed failure: status=%d body=%s error=%v", response.StatusCode, body, readErr)
	}
	failed.stop(t, f.ctx, f.binary, f.project, f.env)
}

func appendWorkerEventsArtifactFault(t *testing.T, path, suffix string) {
	t.Helper()
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	_, writeErr := file.WriteString(suffix)
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		t.Fatalf("capture filesystem fixture: %v %v", writeErr, closeErr)
	}
}

func TestWorkerEventsIdleFollowSocketDetach(t *testing.T) {
	f := newInvokeArtifactFixture(t)
	release := filepath.Join(f.project, "release")
	raw, err := os.ReadFile(f.document)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatal(err)
	}
	document["execution"].(map[string]any)["envVars"].(map[string]any)["WSV_SHIM_RELEASE"] = release
	writeRecordingIntegrityJSON(t, f.document, document)
	host := startHistoryHost(t, f.ctx, f.binary, f.project, f.env)
	f.cli(t, host.url, "invoke", "--execution", f.document, "--async")
	f.waitForProgress(t, host.url)
	first := openWorkerEventsArtifact(t, f.ctx, host.url, "")
	defer first.Body.Close()
	second := openWorkerEventsArtifact(t, f.ctx, host.url, "")
	defer second.Body.Close()
	// Headers have arrived at both sockets before any future output is released.
	if err := first.Body.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(release, []byte("release"), 0o600); err != nil {
		t.Fatal(err)
	}
	frames := drainWorkerEventsArtifact(t, second)
	assertWorkerEventsArtifactTerminal(t, frames)
	found := false
	for _, frame := range frames {
		payload, _ := json.Marshal(frame.Event.Payload)
		found = found || strings.Contains(string(payload), "WSV_LATER_PROGRESS")
	}
	if !found {
		t.Fatal("surviving observer missed gated later output")
	}
	host.stop(t, f.ctx, f.binary, f.project, f.env)
}

func openWorkerEventsArtifact(t *testing.T, ctx context.Context, server, query string) *http.Response {
	t.Helper()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, server+"/worker-sessions/t7-artifact-session/events"+query, nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || response.Header.Get("Content-Type") != "text/event-stream" || response.Header.Get("Cache-Control") != "no-cache" || response.Header.Get("X-Accel-Buffering") != "no" {
		response.Body.Close()
		t.Fatalf("unsupported stream response: status=%d headers=%v", response.StatusCode, response.Header)
	}
	return response
}

func readWorkerEventsArtifact(t *testing.T, ctx context.Context, server, query string) []api.WorkerSessionEvent {
	t.Helper()
	response := openWorkerEventsArtifact(t, ctx, server, query)
	defer response.Body.Close()
	return drainWorkerEventsArtifact(t, response)
}

func drainWorkerEventsArtifact(t *testing.T, response *http.Response) []api.WorkerSessionEvent {
	t.Helper()
	var frames []api.WorkerSessionEvent
	scanner := bufio.NewScanner(response.Body)
	for scanner.Scan() {
		if !strings.HasPrefix(scanner.Text(), "data:") {
			continue
		}
		var frame api.WorkerSessionEvent
		if err := json.Unmarshal([]byte(strings.TrimPrefix(scanner.Text(), "data:")), &frame); err != nil {
			t.Fatal(err)
		}
		frames = append(frames, frame)
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("stream failed to reach socket EOF: %v", err)
	}
	return frames
}

func workerEventsArtifactRecords(frames []api.WorkerSessionEvent) []api.WorkerSessionEventRecord {
	var records []api.WorkerSessionEventRecord
	for _, frame := range frames {
		if frame.Event.Position > 0 {
			records = append(records, frame.Event)
		}
	}
	return records
}

func assertWorkerEventsArtifactTerminal(t *testing.T, frames []api.WorkerSessionEvent) {
	t.Helper()
	var position int64
	terminal := false
	for _, frame := range frames {
		if frame.WorkerSessionId != "t7-artifact-session" || frame.Delivery == api.WorkerSessionEventDeliverySourceFailure {
			t.Fatalf("stream identity/failure: %+v", frame)
		}
		if frame.Event.Position > 0 {
			if frame.Event.Position != position+1 || frame.Event.Cursor.Position != frame.Event.Position || frame.Event.Cursor.WorkerSessionId == nil || *frame.Event.Cursor.WorkerSessionId != frame.WorkerSessionId {
				t.Fatalf("stream lost exclusive ordered Worker cursor: %+v", frame.Event)
			}
			position = frame.Event.Position
		}
		terminal = terminal || frame.Delivery == api.WorkerSessionEventDeliveryTerminal || frame.Delivery == api.WorkerSessionEventDeliveryTerminalReplay
	}
	if !terminal || position == 0 {
		t.Fatal("finite stream lacked terminal captured history")
	}
}
