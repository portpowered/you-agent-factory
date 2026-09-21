package http_test

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func assertCopiedLedgerEventsEqual(
	t *testing.T,
	before, after map[string][]factoryapi.WorkerSessionEvent,
) {
	t.Helper()
	if len(before) != len(after) {
		t.Fatalf("retained Worker Session event topic count changed: before=%d after=%d", len(before), len(after))
	}
	for workerSessionID, left := range before {
		right, ok := after[workerSessionID]
		if !ok || len(left) != len(right) {
			t.Fatalf("Worker Session %q retained event count changed: before=%d after=%d existsAfter=%t", workerSessionID, len(left), len(right), ok)
		}
		assertCopiedLedgerEventSliceEqual(t, "copied restart for Worker Session "+workerSessionID, left, right)
	}
}

func assertCopiedLedgerEventSliceEqual(
	t *testing.T,
	label string,
	left, right []factoryapi.WorkerSessionEvent,
) {
	t.Helper()
	if len(left) != len(right) {
		t.Fatalf("%s: retained event count changed: before=%d after=%d", label, len(left), len(right))
	}
	for index := range left {
		leftComparable, rightComparable := left[index], right[index]
		// A new root opens a new process-local Events generation. The
		// generation token may change while each accepted record identity
		// and position must remain exact.
		leftComparable.Event.Cursor.StreamGenerationId = nil
		rightComparable.Event.Cursor.StreamGenerationId = nil
		// Replay summaries describe the requested page, so a tail reconnect
		// reports a different emitted count from the initial full replay.
		leftComparable.ReplaySummary = nil
		rightComparable.ReplaySummary = nil
		if reflect.DeepEqual(leftComparable, rightComparable) {
			continue
		}
		changedPayloadFields := make([]string, 0)
		for key := range left[index].Event.Payload {
			if !reflect.DeepEqual(left[index].Event.Payload[key], right[index].Event.Payload[key]) {
				changedPayloadFields = append(changedPayloadFields, key)
			}
		}
		for key := range right[index].Event.Payload {
			if _, exists := left[index].Event.Payload[key]; !exists {
				changedPayloadFields = append(changedPayloadFields, key)
			}
		}
		t.Fatalf("%s: event[%d] changed: record fields=%v cursor fields=%v payload fields=%v position=%d->%d schema=%q->%q sourceEvent=%q->%q delivery=%q->%q", label, index,
			copiedLedgerChangedFieldNames(left[index].Event, right[index].Event),
			copiedLedgerChangedFieldNames(left[index].Event.Cursor, right[index].Event.Cursor),
			changedPayloadFields, left[index].Event.Position, right[index].Event.Position,
			left[index].Event.SchemaId, right[index].Event.SchemaId,
			left[index].Event.SourceEventId, right[index].Event.SourceEventId,
			left[index].Delivery, right[index].Delivery)
	}
}

func copiedLedgerChangedFieldNames(left, right any) []string {
	leftValue, rightValue := reflect.ValueOf(left), reflect.ValueOf(right)
	if leftValue.Type() != rightValue.Type() || leftValue.Kind() != reflect.Struct {
		return []string{"<type>"}
	}
	changed := make([]string, 0)
	for index := 0; index < leftValue.NumField(); index++ {
		if !reflect.DeepEqual(leftValue.Field(index).Interface(), rightValue.Field(index).Interface()) {
			changed = append(changed, leftValue.Type().Field(index).Name)
		}
	}
	return changed
}

func assertCopiedLedgerWorkerRecordingTimings(
	t *testing.T,
	snapshot recordings.WorkerRecordingSnapshot,
	public copiedLedgerPublicSnapshot,
) {
	t.Helper()
	if snapshot.RecordingID == "" || len(snapshot.Sessions) == 0 {
		t.Fatalf("Worker recording sidecar lacks identity or retained sessions: %#v", snapshot)
	}
	startedAtBySession := make(map[string]time.Time, len(snapshot.Sessions))
	for _, session := range snapshot.Sessions {
		if _, duplicate := startedAtBySession[session.WorkerSessionID]; duplicate {
			t.Fatalf("Worker recording sidecar repeats Worker Session %q", session.WorkerSessionID)
		}
		if len(session.Records) == 0 {
			t.Fatalf("Worker recording sidecar has no opening record for %q", session.WorkerSessionID)
		}
		var draft workerexecution.Draft
		if err := json.Unmarshal(session.Records[0].Payload, &draft); err != nil {
			t.Fatalf("decode Worker Session %q opening draft: %v", session.WorkerSessionID, err)
		}
		if draft.Kind != workerexecution.KindSession || draft.Phase != workerexecution.PhaseStarted {
			t.Fatalf("Worker Session %q opening draft = %s/%s, want SESSION/STARTED", session.WorkerSessionID, draft.Kind, draft.Phase)
		}
		var payload workerexecution.SessionPayload
		if err := json.Unmarshal(draft.Payload, &payload); err != nil {
			t.Fatalf("decode Worker Session %q opening payload: %v", session.WorkerSessionID, err)
		}
		if payload.WorkerSessionID != session.WorkerSessionID || payload.StartedAt == nil {
			t.Fatalf("Worker Session %q opening payload lacks its exact identity/timestamp: %#v", session.WorkerSessionID, payload)
		}
		startedAtBySession[session.WorkerSessionID] = payload.StartedAt.UTC()
	}
	seenPublicSessions := make(map[string]struct{}, len(startedAtBySession))
	for workID, list := range public.lists {
		for _, observation := range list.Sessions {
			startedAt, exists := startedAtBySession[observation.WorkerSessionId]
			if !exists {
				t.Fatalf("Work %q Worker Session %q is absent from copied Worker recording", workID, observation.WorkerSessionId)
			}
			if observation.StartedAt == nil || !observation.StartedAt.Equal(startedAt) {
				t.Fatalf("Work %q Worker Session %q StartedAt = %v, want source-native opening time %v", workID, observation.WorkerSessionId, observation.StartedAt, startedAt)
			}
			seenPublicSessions[observation.WorkerSessionId] = struct{}{}
		}
	}
	if len(seenPublicSessions) != len(startedAtBySession) {
		t.Fatalf("copied Worker recording has %d sessions but public Work lists expose %d", len(startedAtBySession), len(seenPublicSessions))
	}
}

func assertCopiedLedgerSessionListsEqual(
	t *testing.T,
	before, after map[string]factoryapi.ListWorkerSessionsResponse,
) {
	t.Helper()
	if len(before) != len(after) {
		t.Fatalf("Work-scoped Worker Session list count changed: before=%d after=%d", len(before), len(after))
	}
	for workID, left := range before {
		right, ok := after[workID]
		if !ok || len(left.Sessions) != len(right.Sessions) {
			t.Fatalf("Work %q Worker Session associations changed: before=%#v after=%#v", workID, left.Sessions, right.Sessions)
		}
		for index := range left.Sessions {
			if reflect.DeepEqual(left.Sessions[index], right.Sessions[index]) {
				continue
			}
			leftValue, rightValue := reflect.ValueOf(left.Sessions[index]), reflect.ValueOf(right.Sessions[index])
			changed := make([]string, 0)
			for field := 0; field < leftValue.NumField(); field++ {
				if !reflect.DeepEqual(leftValue.Field(field).Interface(), rightValue.Field(field).Interface()) {
					changed = append(changed, leftValue.Type().Field(field).Name)
				}
			}
			t.Fatalf("Work %q Worker Session association %q changed fields %v: before=%#v after=%#v", workID, left.Sessions[index].WorkerSessionId, changed, left.Sessions[index], right.Sessions[index])
		}
	}
}

func assertCopiedLedgerWorkInventoryEqual(t *testing.T, before, after factoryapi.ListWorkResponse) {
	t.Helper()
	if len(before.Results) != len(after.Results) {
		t.Fatalf("public Work inventory rows changed after restart: before=%d after=%d", len(before.Results), len(after.Results))
	}
	for index := range before.Results {
		left, right := before.Results[index], after.Results[index]
		if !reflect.DeepEqual(left, right) {
			changed := make([]string, 0)
			typeOf := reflect.TypeOf(left)
			leftValue, rightValue := reflect.ValueOf(left), reflect.ValueOf(right)
			for field := 0; field < leftValue.NumField(); field++ {
				if !reflect.DeepEqual(leftValue.Field(field).Interface(), rightValue.Field(field).Interface()) {
					changed = append(changed, typeOf.Field(field).Name)
				}
			}
			t.Fatalf("public Work %q changed fields after copied-ledger restart: %v; confirmation=%q -> %q failure=%#v -> %#v state=%#v -> %#v previousChainingTraceIds=%#v -> %#v", left.Name, changed, copiedLedgerConfirmation(left), copiedLedgerConfirmation(right), left.FailureDetail, right.FailureDetail, left.State, right.State, left.PreviousChainingTraceIds, right.PreviousChainingTraceIds)
		}
	}
}

func copiedLedgerConfirmation(work factoryapi.Work) string {
	if work.ConfirmationState == nil {
		return ""
	}
	return string(*work.ConfirmationState)
}

func assertCopiedLedgerWorkInventory(t *testing.T, inventory factoryapi.ListWorkResponse, workIDs []string) {
	t.Helper()
	want := make(map[string]struct{}, len(workIDs))
	for _, workID := range workIDs {
		want[workID] = struct{}{}
	}
	got := make(map[string]struct{}, len(inventory.Results))
	for _, work := range inventory.Results {
		workID := support.StringPointerValue(work.WorkId)
		if _, duplicate := got[workID]; duplicate {
			t.Fatalf("public Work inventory repeats stable ID %q", workID)
		}
		got[workID] = struct{}{}
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("public Work inventory contains IDs %v, want exact copied batch IDs %v", got, want)
	}
}

func assertCopiedLedgerRepeatedCursorResume(
	t *testing.T,
	baseURL, factoryID, workerSessionID string,
	completeReplay []factoryapi.WorkerSessionEvent,
) {
	t.Helper()
	records := copiedLedgerEventRecords(completeReplay)
	if len(records) < 2 {
		t.Fatalf("copied Worker Session event history has %d records, want at least two for exclusive resume", len(records))
	}
	cursor := records[0].Event.Cursor
	query := url.Values{"replayOnly": []string{"true"}, "after_position": []string{strconv.FormatInt(cursor.Position, 10)}}
	if cursor.StreamGenerationId != nil {
		query.Set("stream_generation_id", *cursor.StreamGenerationId)
	}
	endpoint := copiedLedgerWorkerSessionURL(baseURL, factoryID, workerSessionID) + "/events?" + query.Encode()
	want := records[1:]
	for reconnect := 0; reconnect < 3; reconnect++ {
		resumed := copiedLedgerEventRecords(readCopiedLedgerEvents(t, endpoint))
		assertCopiedLedgerEventSliceEqual(t, fmt.Sprintf("exclusive Worker Session reconnect %d for %q", reconnect+1, workerSessionID), want, resumed)
	}
}

func assertCopiedLedgerCLIParity(
	t *testing.T,
	server *support.FunctionalAPIServer,
	factoryID, workID, workerSessionID string,
	snapshot copiedLedgerPublicSnapshot,
) {
	t.Helper()
	listInputs := support.FakeInputs(t.Context(), []string{
		"you", "worker-sessions", "list", "--session", factoryID,
		"--work-id", workID, "--server", server.URL(), "--output", "json",
	})
	if err := server.Execute(t, listInputs.Input); err != nil {
		t.Fatalf("CLI Work-scoped Worker Session list after restart: %v\nstderr=%s", err, listInputs.Stderr())
	}
	var listed factoryapi.ListWorkerSessionsResponse
	if err := json.Unmarshal([]byte(strings.TrimSpace(listInputs.Stdout())), &listed); err != nil {
		t.Fatalf("decode CLI Work-scoped list: %v\nstdout=%s", err, listInputs.Stdout())
	}
	if !reflect.DeepEqual(listed, snapshot.lists[workID]) {
		t.Fatalf("CLI Work-scoped list differs from HTTP after restart:\nCLI=%#v\nHTTP=%#v", listed, snapshot.lists[workID])
	}
	readInputs := support.FakeInputs(t.Context(), []string{
		"you", "worker-sessions", "read", "--session", factoryID,
		"--worker-session-id", workerSessionID, "--server", server.URL(), "--json",
	})
	if err := server.Execute(t, readInputs.Input); err != nil {
		t.Fatalf("CLI stable-ID transcript read after restart: %v\nstderr=%s", err, readInputs.Stderr())
	}
	var transcript factoryapi.WorkerSessionTranscriptResponse
	if err := json.Unmarshal([]byte(strings.TrimSpace(readInputs.Stdout())), &transcript); err != nil {
		t.Fatalf("decode CLI stable-ID transcript: %v\nstdout=%s", err, readInputs.Stdout())
	}
	if !reflect.DeepEqual(transcript, snapshot.transcripts[workerSessionID]) {
		t.Fatalf("CLI stable-ID transcript differs from HTTP after restart:\nCLI=%#v\nHTTP=%#v", transcript, snapshot.transcripts[workerSessionID])
	}
	assertWorkerTranscriptRedacted(t, readInputs.Stdout())
	streamInputs := support.FakeInputs(t.Context(), []string{
		"you", "worker-sessions", "stream", "--session", factoryID,
		"--worker-session-id", workerSessionID, "--server", server.URL(), "--replay-only", "--json",
	})
	if err := server.Execute(t, streamInputs.Input); err != nil {
		t.Fatalf("CLI Worker Session replay after restart: %v\nstderr=%s", err, streamInputs.Stderr())
	}
	cliRecords, summary := decodeWorkScopedWorkerSessionCLIStream(t, streamInputs.Stdout())
	if summary == nil || !summary.Complete || summary.EventsEmitted != int64(len(cliRecords)) {
		t.Fatalf("CLI Worker Session replay summary = %#v for %d records, want complete replay", summary, len(cliRecords))
	}
	assertCopiedLedgerCLIEventParity(t, cliRecords, copiedLedgerEventRecords(snapshot.events[workerSessionID]))
}

func assertCopiedLedgerCLIEventParity(
	t *testing.T,
	cliRecords []workScopedCLIStreamFrame,
	httpRecords []factoryapi.WorkerSessionEvent,
) {
	t.Helper()
	if len(cliRecords) != len(httpRecords) {
		t.Fatalf("CLI retained event count=%d, HTTP=%d", len(cliRecords), len(httpRecords))
	}
	for index, cli := range cliRecords {
		want := httpRecords[index]
		if cli.WorkerSessionID != want.WorkerSessionId || cli.FactorySessionID != stringValue(want.FactorySessionId) ||
			!reflect.DeepEqual(cli.WorkIDs, want.WorkIds) || cli.Event == nil || cli.Event.Position != want.Event.Position ||
			cli.Event.SourceType != want.Event.SourceType || cli.Event.SourceID != want.Event.SourceId ||
			cli.Event.SourceSequence != want.Event.SourceSequence || cli.Event.SourceEventID != want.Event.SourceEventId {
			t.Fatalf("CLI/HTTP retained event identity differs at index %d: CLI=%#v HTTP=%#v", index, cli, want)
		}
	}
}

func copiedLedgerEventRecords(frames []factoryapi.WorkerSessionEvent) []factoryapi.WorkerSessionEvent {
	records := make([]factoryapi.WorkerSessionEvent, 0, len(frames))
	for _, frame := range frames {
		if frame.Event.Position > 0 {
			records = append(records, frame)
		}
	}
	return records
}

func readCopiedLedgerEvents(t *testing.T, endpoint string) []factoryapi.WorkerSessionEvent {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), copiedLedgerReplayTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		t.Fatalf("build Worker Session replay request: %v", err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("GET Worker Session replay: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("GET Worker Session replay status=%d body=%s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	frames := make([]factoryapi.WorkerSessionEvent, 0)
	scanner := bufio.NewScanner(response.Body)
	scanner.Buffer(make([]byte, 4096), 1<<20)
	complete := false
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		var frame factoryapi.WorkerSessionEvent
		if err := json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data:"))), &frame); err != nil {
			t.Fatalf("decode Worker Session replay frame: %v", err)
		}
		frames = append(frames, frame)
		if frame.ReplaySummary != nil && frame.ReplaySummary.Complete {
			complete = true
			break
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("read Worker Session replay stream: %v", err)
	}
	if !complete {
		t.Fatalf("Worker Session replay ended without a complete summary: %#v", frames)
	}
	return frames
}
