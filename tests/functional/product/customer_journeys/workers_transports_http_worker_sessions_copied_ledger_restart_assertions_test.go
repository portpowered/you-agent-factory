package customer_journeys_test

import (
	"bufio"
	"bytes"
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

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// A public move after recovery must append one new physical attempt without
// changing any committed historical rows, including an unrelated failure.
func assertCopiedLedgerNewAttempt(t *testing.T, server *support.FunctionalAPIServer,
	factoryID, workID, failureWorkID, firstWorkerSessionID string, before copiedLedgerPublicSnapshot) {
	t.Helper()
	body, err := json.Marshal(factoryapi.MoveWorkRequest{StateName: "review"})
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost,
		copiedLedgerWorkURL(server.URL(), factoryID, workID)+"/move", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(response.Body)
		t.Fatalf("move restored Work status=%d body=%s", response.StatusCode, data)
	}
	waitCopiedLedgerWorkState(t, server.URL(), factoryID, workID, "complete")
	endpoint := strings.TrimSuffix(server.URL(), "/") + "/factory-sessions/" + url.PathEscape(factoryID) +
		"/worker-sessions?workId=" + url.QueryEscape(workID)
	// Use the existing public confirmation predicate; execution completion
	// alone is insufficient to prove the capture and canonical flush settled.
	listed, err := support.WaitForObservation(copiedLedgerReplayTimeout, func() (factoryapi.ListWorkerSessionsResponse, error) {
		var list factoryapi.ListWorkerSessionsResponse
		err := readCopiedLedgerJSON(t.Context(), endpoint, &list)
		return list, err
	}, func(list factoryapi.ListWorkerSessionsResponse) bool {
		if len(list.Sessions) != 3 {
			return false
		}
		for _, row := range list.Sessions {
			if !copiedLedgerAttemptConfirmed(row, factoryID, workID, factoryapi.WorkerSessionObservationStateCompleted) || row.TokenUsage == nil {
				return false
			}
		}
		return true
	})
	if err != nil {
		t.Fatalf("new restored attempt did not commit: %v; rows=%#v", err, listed)
	}
	assertCopiedLedgerWorkRows(t, listed, factoryID, workID, 3, factoryapi.WorkerSessionObservationStateCompleted)
	if !reflect.DeepEqual(listed.Sessions[:2], before.lists[workID].Sessions) {
		t.Fatalf("new commit changed ordered historical attempts: before=%#v after=%#v", before.lists[workID], listed)
	}
	if listed.Sessions[2].Model == nil || *listed.Sessions[2].Model != copiedLedgerReplayTargetModel {
		t.Fatalf("new attempt lost model: %#v", listed.Sessions[2])
	}
	failure := support.ListSessionWorkerSessions(t, server.URL(), factoryID, failureWorkID)
	assertCopiedLedgerSessionListsEqual(t, map[string]factoryapi.ListWorkerSessionsResponse{failureWorkID: before.lists[failureWorkID]},
		map[string]factoryapi.ListWorkerSessionsResponse{failureWorkID: failure})
	before.lists[workID] = listed
	assertCopiedLedgerCLIParity(t, server, factoryID, workID, firstWorkerSessionID, before)
}

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

func assertCopiedLedgerLiveHistoryIdentities(t *testing.T, live, history copiedLedgerPublicSnapshot) {
	t.Helper()
	for workID, list := range live.lists {
		other := history.lists[workID]
		if len(list.Sessions) != len(other.Sessions) {
			t.Fatalf("live/history attempt count differs for %s", workID)
		}
		for index, left := range list.Sessions {
			right := other.Sessions[index]
			if left.WorkerSessionId != right.WorkerSessionId || left.AttemptId != right.AttemptId || left.State != right.State ||
				!reflect.DeepEqual(left.FactorySessionId, right.FactorySessionId) || !reflect.DeepEqual(left.WorkIds, right.WorkIds) ||
				!reflect.DeepEqual(left.ProviderSession, right.ProviderSession) || left.Transcript != right.Transcript {
				t.Fatalf("live/history identity changed: live=%#v history=%#v", left, right)
			}
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

// Execution-terminal events do not prove a completed recording flush. There is
// no public durability-settled notification for these combined projections, so
// observe the public condition before capturing each recovery epoch's facts.
func waitCopiedLedgerSnapshotConfirmed(t *testing.T, baseURL, factoryID string, workIDs []string, targetWorkID, failureWorkID string) copiedLedgerPublicSnapshot {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), copiedLedgerReplayTimeout)
	defer cancel()
	last, err := support.WaitForObservation(copiedLedgerReplayTimeout,
		func() (copiedLedgerPublicSnapshot, error) {
			return observeCopiedLedgerSnapshot(ctx, baseURL, factoryID, workIDs)
		}, func(snapshot copiedLedgerPublicSnapshot) bool {
			return copiedLedgerSnapshotConfirmed(snapshot, factoryID, workIDs, targetWorkID, failureWorkID)
		})
	if err != nil {
		// Snapshot fields are private; encode each public response explicitly.
		response, _ := json.Marshal([]any{last.inventory, last.works, last.lists, last.details})
		t.Fatalf("settle copied-ledger session %q Works %v: %v; last public responses=%s", factoryID, workIDs, err, response)
	}
	for _, workID := range workIDs {
		count, state := 1, factoryapi.WorkerSessionObservationStateCompleted
		if workID == targetWorkID {
			count = 2
		}
		if workID == failureWorkID {
			state = factoryapi.WorkerSessionObservationStateFailed
		}
		assertCopiedLedgerWorkRows(t, last.lists[workID], factoryID, workID, count, state)
	}
	return last
}

func observeCopiedLedgerSnapshot(ctx context.Context, baseURL, factoryID string, workIDs []string) (copiedLedgerPublicSnapshot, error) {
	snapshot := copiedLedgerPublicSnapshot{
		works:   make(map[string]factoryapi.Work),
		lists:   make(map[string]factoryapi.ListWorkerSessionsResponse),
		details: make(map[string]factoryapi.WorkerSessionObservation),
	}
	if err := readCopiedLedgerJSON(ctx, copiedLedgerWorkListURL(baseURL, factoryID), &snapshot.inventory); err != nil {
		return snapshot, err
	}
	for _, workID := range workIDs {
		var work factoryapi.Work
		if err := readCopiedLedgerJSON(ctx, copiedLedgerWorkURL(baseURL, factoryID, workID), &work); err != nil {
			return snapshot, err
		}
		snapshot.works[workID] = work
		var list factoryapi.ListWorkerSessionsResponse
		endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + url.PathEscape(factoryID) + "/worker-sessions?workId=" + url.QueryEscape(workID)
		if err := readCopiedLedgerJSON(ctx, endpoint, &list); err != nil {
			return snapshot, err
		}
		snapshot.lists[workID] = list
		for _, observation := range list.Sessions {
			var detail factoryapi.WorkerSessionObservation
			if err := readCopiedLedgerJSON(ctx, copiedLedgerWorkerSessionURL(baseURL, factoryID, observation.WorkerSessionId), &detail); err != nil {
				return snapshot, err
			}
			snapshot.details[observation.WorkerSessionId] = detail
		}
	}
	return snapshot, nil
}

func readCopiedLedgerJSON(ctx context.Context, endpoint string, target any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return fmt.Errorf("GET %s: %w", endpoint, err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return fmt.Errorf("read %s: %w", endpoint, err)
	}
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: status=%d body=%s", endpoint, response.StatusCode, body)
	}
	if err := json.Unmarshal(body, target); err != nil {
		return fmt.Errorf("decode %s: %w; body=%s", endpoint, err, body)
	}
	return nil
}

func copiedLedgerSnapshotConfirmed(snapshot copiedLedgerPublicSnapshot, factoryID string, workIDs []string, targetWorkID, failureWorkID string) bool {
	if len(snapshot.inventory.Results) != len(workIDs) || len(snapshot.works) != len(workIDs) {
		return false
	}
	inventory := make(map[string]factoryapi.Work)
	for _, work := range snapshot.inventory.Results {
		inventory[support.StringPointerValue(work.WorkId)] = work
	}
	if len(inventory) != len(workIDs) {
		return false
	}
	seenSessions, seenAttempts := make(map[string]bool), make(map[string]bool)
	for _, workID := range workIDs {
		state, count, attemptState := "complete", 1, factoryapi.WorkerSessionObservationStateCompleted
		if workID == targetWorkID {
			count = 2
		}
		if workID == failureWorkID {
			state, attemptState = "failed", factoryapi.WorkerSessionObservationStateFailed
		}
		if !copiedLedgerWorkConfirmed(inventory[workID], workID, state) || !copiedLedgerWorkConfirmed(snapshot.works[workID], workID, state) {
			return false
		}
		if !copiedLedgerAttemptsConfirmed(snapshot.lists[workID], snapshot.details, factoryID, workID, count, attemptState, seenSessions, seenAttempts) {
			return false
		}
	}
	return len(snapshot.details) == len(seenSessions)
}

func copiedLedgerWorkConfirmed(work factoryapi.Work, workID, state string) bool {
	return support.StringPointerValue(work.WorkId) == workID && work.State != nil &&
		work.State.Name == state && copiedLedgerConfirmation(work) == "CONFIRMED"
}

func copiedLedgerAttemptConfirmed(observation factoryapi.WorkerSessionObservation, factoryID, workID string, state factoryapi.WorkerSessionObservationState) bool {
	return observation.WorkerSessionId != "" && observation.AttemptId != "" && observation.State == state &&
		observation.ConfirmationState == factoryapi.CONFIRMED && support.StringPointerValue(observation.FactorySessionId) == factoryID &&
		support.StringPointerValue(observation.WorkId) == workID && reflect.DeepEqual(observation.WorkIds, []string{workID}) &&
		(state != factoryapi.WorkerSessionObservationStateFailed || observation.Failure != nil)
}

func copiedLedgerAttemptsConfirmed(list factoryapi.ListWorkerSessionsResponse, details map[string]factoryapi.WorkerSessionObservation,
	factoryID, workID string, count int, state factoryapi.WorkerSessionObservationState, seenSessions, seenAttempts map[string]bool) bool {
	if len(list.Sessions) != count {
		return false
	}
	for _, listed := range list.Sessions {
		if seenSessions[listed.WorkerSessionId] || seenAttempts[listed.AttemptId] {
			return false
		}
		seenSessions[listed.WorkerSessionId], seenAttempts[listed.AttemptId] = true, true
		detail := details[listed.WorkerSessionId]
		if detail.WorkerSessionId != listed.WorkerSessionId || detail.AttemptId != listed.AttemptId ||
			!copiedLedgerAttemptConfirmed(listed, factoryID, workID, state) ||
			!copiedLedgerAttemptConfirmed(detail, factoryID, workID, state) {
			return false
		}
	}
	return true
}
