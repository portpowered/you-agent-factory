package recording_sidecar_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"

	api "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// Both selected hosts expose the same committed records through the delivered
// CLI and real HTTP boundary. The existing MCP client asserts the same frames.
func assertHistoryReplay(t *testing.T, ctx context.Context, binary, project string, env []string, server string, logs api.WorkerSessionLogPage) {
	t.Helper()
	output := historyCLI(t, ctx, binary, project, env, "--server", server, "--json", "worker-sessions", "stream", "--worker-session-id", logs.WorkerSessionId, "--replay-only")
	var frames []api.WorkerSessionEvent
	scanner := bufio.NewScanner(bytes.NewReader(output))
	for scanner.Scan() {
		var frame api.WorkerSessionEvent
		if err := json.Unmarshal(scanner.Bytes(), &frame); err != nil {
			t.Fatal(err)
		}
		frames = append(frames, frame)
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	// The CLI stream v1 shape omits commit timestamps and cursors.
	// Compare its published source-record fields; HTTP/MCP compare all facts.
	cliLogs := logs
	cliLogs.Events = append([]api.WorkerSessionEvent(nil), logs.Events...)
	for i := range cliLogs.Events {
		cliLogs.Events[i].Event.CapturedAt = nil
		cliLogs.Events[i].Event.Cursor = api.WorkerSessionEventCursor{}
	}
	assertHistoryReplayFrames(t, frames, cliLogs)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server+"/worker-sessions/"+logs.WorkerSessionId+"/events?replayOnly=true", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("HTTP archive replay status=%d", response.StatusCode)
	}
	frames = nil
	scanner = bufio.NewScanner(response.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var frame api.WorkerSessionEvent
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &frame); err != nil {
			t.Fatal(err)
		}
		frames = append(frames, frame)
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	assertHistoryReplayFrames(t, frames, logs)
}

func assertHistoryReplayFrames(t *testing.T, frames []api.WorkerSessionEvent, logs api.WorkerSessionLogPage) {
	t.Helper()
	if len(frames) != len(logs.Events) || len(frames) == 0 {
		t.Fatalf("replay records=%d logs=%d frames=%+v", len(frames), len(logs.Events), frames)
	}
	for i, frame := range frames {
		if frame.WorkerSessionId != logs.WorkerSessionId || !reflect.DeepEqual(frame.Event, logs.Events[i].Event) {
			t.Fatalf("replay record %d differs from committed logs: %+v %+v", i, frame, logs.Events[i])
		}
	}
	last := frames[len(frames)-1]
	if last.Delivery != api.WorkerSessionEventDelivery("TERMINAL_REPLAY") || last.ReplaySummary == nil || !last.ReplaySummary.Complete || last.ReplaySummary.EventsEmitted != int64(len(frames)) {
		t.Fatalf("missing complete finite replay: %+v", last)
	}
}
