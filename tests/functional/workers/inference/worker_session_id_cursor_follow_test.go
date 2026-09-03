package inference_test

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"github.com/portpowered/infinite-you/tests/internal/functionalevidence"
)

// TestWSRFT011WorkerSessionCursorResumeAcrossRestart proves that the public
// Worker-ID stream resumes exclusively after an acknowledged cursor in a
// fresh replay process and returns typed identity errors for invalid cursors.
//
// WSR-FT-011: exclusive Worker-ID cursor resume across restart and malformed,
// foreign, future, stale, and unavailable cursor outcomes.
func TestWSRFT011WorkerSessionCursorResumeAcrossRestart(t *testing.T) {
	t.Parallel()
	dir := support.ScaffoldSingleStepFactory(t, "wsr-ft-011-worker-id-cursor")
	artifactPath := filepath.Join(t.TempDir(), "wsr-ft-011-worker-id-cursor.replay.json")
	sessionID := uuid.NewString()
	homeDir := t.TempDir()
	env := append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		WaitForServiceModeRuntime: true,
		ServerReadyTimeout:        60 * time.Second,
		Env:                       env,
		Args:                      []string{"--session", sessionID, "--record", artifactPath},
		ProviderOverride:          support.MockInferenceProvider("first completion", "second completion"),
	})

	firstWork := submitWSRFT011Work(t, server.URL(), sessionID, "first")
	secondWork := submitWSRFT011Work(t, server.URL(), sessionID, "second")
	support.WaitForSessionTerminalStatus(t, server.URL(), sessionID, 30*time.Second)
	firstWorkerID := workerIDForWSRFT011Work(t, server.URL(), sessionID, firstWork)
	secondWorkerID := workerIDForWSRFT011Work(t, server.URL(), sessionID, secondWork)
	if firstWorkerID == secondWorkerID {
		t.Fatalf("Worker Session IDs = %q and %q, want distinct attempts", firstWorkerID, secondWorkerID)
	}

	firstHistory := readWSRFT011Events(t, workerEventsWSRFT011URL(server.URL(), sessionID, firstWorkerID, url.Values{
		"replayOnly": []string{"true"},
	}))
	firstRecords := assertWSRFT011Replay(t, firstHistory, firstWorkerID)
	if len(firstRecords) < 2 {
		t.Fatalf("Worker-ID history records = %d, want opening and terminal records", len(firstRecords))
	}
	acknowledged := firstRecords[0].Event.Position

	server.Stop(t)
	replayServer := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                t.TempDir(),
		WaitForServiceModeRuntime: true,
		ServerReadyTimeout:        60 * time.Second,
		Env:                       env,
		Args:                      []string{"--session", sessionID, "--replay", artifactPath, "--no-record"},
	})
	resumedHistory := readWSRFT011Events(t, workerEventsWSRFT011URL(replayServer.URL(), sessionID, firstWorkerID, url.Values{
		"replayOnly":     []string{"true"},
		"after_position": []string{strconv.FormatInt(acknowledged, 10)},
	}))
	replayRecords := assertWSRFT011Replay(t, readWSRFT011Events(t, workerEventsWSRFT011URL(replayServer.URL(), sessionID, firstWorkerID, url.Values{
		"replayOnly": []string{"true"},
	})), firstWorkerID)
	assertWSRFT011SameRecords(t, firstRecords, replayRecords)
	resumedRecords := assertWSRFT011Replay(t, resumedHistory, firstWorkerID)
	if len(resumedRecords) != len(firstRecords)-1 {
		t.Fatalf("resumed records = %d, want %d after exclusive position %d", len(resumedRecords), len(firstRecords)-1, acknowledged)
	}
	for index, event := range resumedRecords {
		if event.Event.Position <= acknowledged {
			t.Fatalf("resumed record[%d] position = %d, want > %d", index, event.Event.Position, acknowledged)
		}
	}
	assertWSRFT011SameRecords(t, resumedRecords, firstRecords[1:])

	assertWSRFT011CursorError(t, workerEventsWSRFT011URL(replayServer.URL(), sessionID, firstWorkerID, url.Values{
		"after_position": []string{"0"},
	}), string(factoryapi.ErrorResponseCodeWORKERSESSIONEVENTCURSORINVALID))
	assertWSRFT011CursorError(t, workerEventsWSRFT011URL(replayServer.URL(), sessionID, firstWorkerID, url.Values{
		"after_position": []string{"9223372036854775807"},
	}), string(factoryapi.ErrorResponseCodeWORKERSESSIONEVENTCURSORFUTURE))
	assertWSRFT011CursorError(t, workerEventsWSRFT011URL(replayServer.URL(), sessionID, secondWorkerID, url.Values{
		"after_position": []string{strconv.FormatInt(acknowledged, 10)},
	}), string(factoryapi.ErrorResponseCodeWORKERSESSIONEVENTCURSORFOREIGN))
	assertWSRFT011CursorError(t, workerEventsWSRFT011URL(replayServer.URL(), sessionID, firstWorkerID, url.Values{
		"after_position":       []string{strconv.FormatInt(acknowledged, 10)},
		"stream_generation_id": []string{"missing-generation"},
	}), string(factoryapi.ErrorResponseCodeWORKERSESSIONEVENTCURSORUNAVAILABLE))
	functionalevidence.Covers(t,
		"rest/streamWorkerSessionEventsByWorkerSessionId",
		"sse/streamWorkerSessionEventsByWorkerSessionId",
	)
}

// TestWSRFT012WorkerSessionFollowAndProviderReferenceParity proves that a
// live Worker-ID follower captures the durable history without duplication or
// loss, and that the Provider-reference compatibility routes delegate to the
// same observation, transcript, cursor, and event behavior.
//
// WSR-FT-012: race-safe durable/live follow and Provider-reference parity.
func TestWSRFT012WorkerSessionFollowAndProviderReferenceParity(t *testing.T) {
	t.Parallel()
	release := make(chan struct{})
	dir := support.ScaffoldSingleStepFactory(t, "wsr-ft-012-worker-id-follow")
	sessionID := uuid.NewString()
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		WaitForServiceModeRuntime: true,
		Args:                      []string{"--session", sessionID},
		ProviderOverride:          support.BlockingInferenceProvider(release),
	})
	workID := submitWSRFT011Work(t, server.URL(), sessionID, "live-follow")
	workerID := waitForWSRFT012WorkerID(t, server.URL(), sessionID, workID)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	started := make(chan error, 1)
	streamDone := make(chan wsrft012StreamResult, 1)
	go func() {
		events, err := readWSRFT012LiveStream(ctx, workerEventsWSRFT011URL(server.URL(), sessionID, workerID, nil), started)
		streamDone <- wsrft012StreamResult{events: events, err: err}
	}()
	if err := <-started; err != nil {
		t.Fatalf("start live Worker-ID follow: %v", err)
	}
	close(release)
	stream := <-streamDone
	if stream.err != nil {
		t.Fatalf("read live Worker-ID follow: %v", stream.err)
	}
	liveRecords := assertWSRFT012Live(t, stream.events, workerID)
	replayedRecords := assertWSRFT011Replay(t, readWSRFT011Events(t, workerEventsWSRFT011URL(server.URL(), sessionID, workerID, url.Values{
		"replayOnly": []string{"true"},
	})), workerID)
	assertWSRFT011SameRecords(t, liveRecords, replayedRecords)
	server.Stop(t)

	providerServer, providerFactorySession, providerWorkerID, providerSession := startWSRFT012ProviderServer(t)
	directObservation := getWSRFT010Observation(t, providerServer.URL(), providerFactorySession, providerWorkerID)
	providerObservation := getWSRFT012ProviderObservation(t, providerServer.URL(), providerFactorySession, providerSession)
	if !reflect.DeepEqual(directObservation, providerObservation) {
		t.Fatalf("Worker-ID/provider observations differ:\ndirect=%#v\nprovider=%#v", directObservation, providerObservation)
	}

	directEvents := assertWSRFT011Replay(t, readWSRFT011Events(t, workerEventsWSRFT011URL(providerServer.URL(), providerFactorySession, providerWorkerID, url.Values{
		"replayOnly": []string{"true"},
	})), providerWorkerID)
	providerEvents := assertWSRFT011Replay(t, readWSRFT011Events(t, providerEventsWSRFT012URL(providerServer.URL(), providerFactorySession, providerSession, url.Values{
		"replayOnly": []string{"true"},
	})), providerWorkerID)
	assertWSRFT011SameRecords(t, directEvents, providerEvents)

	acknowledged := directEvents[0].Event.Position
	directResume := assertWSRFT011Replay(t, readWSRFT011Events(t, workerEventsWSRFT011URL(providerServer.URL(), providerFactorySession, providerWorkerID, url.Values{
		"replayOnly":     []string{"true"},
		"after_position": []string{strconv.FormatInt(acknowledged, 10)},
	})), providerWorkerID)
	providerResume := assertWSRFT011Replay(t, readWSRFT011Events(t, providerEventsWSRFT012URL(providerServer.URL(), providerFactorySession, providerSession, url.Values{
		"replayOnly":     []string{"true"},
		"after_position": []string{strconv.FormatInt(acknowledged, 10)},
	})), providerWorkerID)
	assertWSRFT011SameRecords(t, directResume, providerResume)

	directTranscript := getWSRFT012Transcript(t, workerObservationURL(providerServer.URL(), providerFactorySession, providerWorkerID)+"/transcript")
	providerTranscript := getWSRFT012Transcript(t, providerTranscriptWSRFT012URL(providerServer.URL(), providerFactorySession, providerSession))
	if directTranscript.status != providerTranscript.status || directTranscript.code != providerTranscript.code ||
		!reflect.DeepEqual(directTranscript.transcript, providerTranscript.transcript) {
		t.Fatalf("Worker-ID/provider transcripts differ:\ndirect=%#v\nprovider=%#v", directTranscript, providerTranscript)
	}
	providerServer.Stop(t)

	functionalevidence.Covers(t,
		"rest/getWorkerSessionObservationBySessionId",
		"rest/readWorkerSessionTranscriptBySessionId",
		"rest/streamWorkerSessionEventsBySessionId",
		"sse/streamWorkerSessionEventsBySessionId",
	)
}

type wsrft012StreamResult struct {
	events []factoryapi.WorkerSessionEvent
	err    error
}

func submitWSRFT011Work(t *testing.T, baseURL, sessionID, suffix string) string {
	t.Helper()
	name := "wsr-ft-011-" + suffix
	response := support.SubmitSessionWorkAt(t, baseURL, sessionID, factoryapi.SubmitWorkRequest{
		Name:         &name,
		WorkTypeName: "task",
		Payload:      json.RawMessage(fmt.Sprintf(`{"title":"%s"}`, name)),
	})
	if response.WorkId == nil || strings.TrimSpace(*response.WorkId) == "" {
		t.Fatalf("submit %q response = %#v, want Work ID", suffix, response)
	}
	return *response.WorkId
}

func workerIDForWSRFT011Work(t *testing.T, baseURL, sessionID, workID string) string {
	t.Helper()
	list := support.ListSessionWorkerSessions(t, baseURL, sessionID, workID)
	if len(list.Sessions) != 1 || strings.TrimSpace(list.Sessions[0].WorkerSessionId) == "" {
		t.Fatalf("Worker Session list for Work %q = %#v, want one identified attempt", workID, list)
	}
	return list.Sessions[0].WorkerSessionId
}

func waitForWSRFT012WorkerID(t *testing.T, baseURL, sessionID, workID string) string {
	t.Helper()
	deadline := time.NewTimer(15 * time.Second)
	defer deadline.Stop()
	// Worker admission is asynchronous and the public API has no readiness
	// operation for this projection, so use a bounded readiness poll rather
	// than sleeping before opening the live stream.
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		list := support.ListSessionWorkerSessions(t, baseURL, sessionID, workID)
		if len(list.Sessions) > 0 && strings.TrimSpace(list.Sessions[0].WorkerSessionId) != "" {
			return list.Sessions[0].WorkerSessionId
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatalf("timed out waiting for Worker Session admission for Work %q", workID)
		}
	}
}

func workerEventsWSRFT011URL(baseURL, sessionID, workerID string, query url.Values) string {
	endpoint := workerObservationURL(baseURL, sessionID, workerID) + "/events"
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}
	return endpoint
}

func providerEventsWSRFT012URL(baseURL, sessionID, providerSession string, query url.Values) string {
	return providerWorkerSessionWSRFT012URL(baseURL, sessionID, "/events", providerSession, query)
}

func providerTranscriptWSRFT012URL(baseURL, sessionID, providerSession string) string {
	return providerWorkerSessionWSRFT012URL(baseURL, sessionID, "/transcript", providerSession, nil)
}

func providerWorkerSessionWSRFT012URL(baseURL, sessionID, suffix, providerSession string, query url.Values) string {
	values := url.Values{
		"provider": []string{"codex"},
		"kind":     []string{"session_id"},
		"id":       []string{providerSession},
	}
	for key, entries := range query {
		values[key] = append([]string(nil), entries...)
	}
	return fmt.Sprintf(
		"%s/factory-sessions/%s/worker-sessions%s?%s",
		strings.TrimSuffix(baseURL, "/"),
		url.PathEscape(sessionID),
		suffix,
		values.Encode(),
	)
}

func readWSRFT011Events(t *testing.T, endpoint string) []factoryapi.WorkerSessionEvent {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		t.Fatalf("build Worker Session event request: %v", err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("GET Worker Session events: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("GET Worker Session events status = %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	events, err := decodeWSRFT011Events(response.Body)
	if err != nil {
		t.Fatalf("decode Worker Session event stream: %v", err)
	}
	return events
}

func readWSRFT012LiveStream(ctx context.Context, endpoint string, started chan<- error) ([]factoryapi.WorkerSessionEvent, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		started <- fmt.Errorf("build request: %w", err)
		return nil, err
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		started <- fmt.Errorf("GET live Worker Session events: %w", err)
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		err := fmt.Errorf("status = %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
		started <- err
		return nil, err
	}
	started <- nil
	return decodeWSRFT011Events(response.Body)
}

func decodeWSRFT011Events(reader io.Reader) ([]factoryapi.WorkerSessionEvent, error) {
	var events []factoryapi.WorkerSessionEvent
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		var event factoryapi.WorkerSessionEvent
		if err := json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data:"))), &event); err != nil {
			return nil, err
		}
		events = append(events, event)
		if event.ReplaySummary != nil && event.ReplaySummary.Complete {
			return events, nil
		}
	}
	return events, scanner.Err()
}

func assertWSRFT011Replay(t *testing.T, events []factoryapi.WorkerSessionEvent, workerID string) []factoryapi.WorkerSessionEvent {
	t.Helper()
	var records []factoryapi.WorkerSessionEvent
	var summary *factoryapi.WorkerSessionReplaySummary
	for _, event := range events {
		if event.Delivery == factoryapi.WorkerSessionEventDelivery("SOURCE_FAILURE") {
			t.Fatalf("Worker Session replay source failure = %#v", event)
		}
		if event.ReplaySummary != nil {
			summary = event.ReplaySummary
		}
		if event.Event.Position == 0 {
			continue
		}
		if event.Event.Position <= 0 || event.WorkerSessionId != workerID {
			t.Fatalf("Worker Session replay record = %#v, want identified positive-position record", event)
		}
		records = append(records, event)
	}
	if summary == nil || !summary.Complete || summary.EventsEmitted != int64(len(records)) {
		t.Fatalf("Worker Session replay summary = %#v for %d records, want complete matching summary", summary, len(records))
	}
	assertWSRFT011RecordOrder(t, records, workerID)
	return records
}

func assertWSRFT012Live(t *testing.T, events []factoryapi.WorkerSessionEvent, workerID string) []factoryapi.WorkerSessionEvent {
	t.Helper()
	var records []factoryapi.WorkerSessionEvent
	for _, event := range events {
		if event.Delivery == factoryapi.WorkerSessionEventDelivery("SOURCE_FAILURE") {
			t.Fatalf("live Worker Session source failure = %#v", event)
		}
		if event.Event.Position == 0 {
			continue
		}
		if event.Event.Position <= 0 || event.WorkerSessionId != workerID {
			t.Fatalf("live Worker Session record = %#v, want identified positive-position record", event)
		}
		records = append(records, event)
	}
	if len(records) < 2 {
		t.Fatalf("live Worker Session records = %#v, want at least opening and TERMINAL records", records)
	}
	lastDelivery := records[len(records)-1].Delivery
	terminal := lastDelivery == factoryapi.WorkerSessionEventDelivery("TERMINAL") || lastDelivery == factoryapi.WorkerSessionEventDelivery("TERMINAL_REPLAY")
	if !terminal {
		t.Fatalf("live Worker Session records = %#v, want terminal delivery", records)
	}
	assertWSRFT011RecordOrder(t, records, workerID)
	return records
}

func assertWSRFT011RecordOrder(t *testing.T, records []factoryapi.WorkerSessionEvent, workerID string) {
	t.Helper()
	seen := make(map[string]struct{}, len(records))
	generationID := ""
	for index, event := range records {
		if event.WorkerSessionId != workerID {
			t.Fatalf("Worker Session record[%d] worker ID = %q, want %q", index, event.WorkerSessionId, workerID)
		}
		currentGenerationID := wsrft011StringPointer(event.Event.Cursor.StreamGenerationId)
		if strings.TrimSpace(currentGenerationID) == "" {
			t.Fatalf("Worker Session record[%d] cursor generation is empty", index)
		}
		if generationID == "" {
			generationID = currentGenerationID
		} else if currentGenerationID != generationID {
			t.Fatalf("Worker Session cursor generation changed at record[%d]: %q/%q", index, generationID, currentGenerationID)
		}
		if wsrft011StringPointer(event.Event.Cursor.WorkerSessionId) != workerID || event.Event.Cursor.Position != event.Event.Position {
			t.Fatalf("Worker Session record[%d] cursor = %#v, want worker %q and position %d", index, event.Event.Cursor, workerID, event.Event.Position)
		}
		if index > 0 && event.Event.Position <= records[index-1].Event.Position {
			t.Fatalf("Worker Session positions are not strictly increasing: record[%d]=%d previous=%d", index, event.Event.Position, records[index-1].Event.Position)
		}
		if event.Event.SourceSequence != event.Event.Position {
			t.Fatalf("Worker Session record[%d] source sequence = %d, want position %d", index, event.Event.SourceSequence, event.Event.Position)
		}
		key := fmt.Sprintf("%s|%s|%d|%s", event.Event.SourceType, event.Event.SourceId, event.Event.SourceSequence, event.Event.SourceEventId)
		if _, exists := seen[key]; exists {
			t.Fatalf("Worker Session duplicated source event %q", key)
		}
		seen[key] = struct{}{}
	}
}

func assertWSRFT011SameRecords(t *testing.T, left, right []factoryapi.WorkerSessionEvent) {
	t.Helper()
	if len(left) != len(right) {
		t.Fatalf("Worker Session record counts = %d/%d, want equal histories\nleft=%s\nright=%s", len(left), len(right), formatWSRFT011Records(left), formatWSRFT011Records(right))
	}
	for index := range left {
		differences := wsrft011RecordDifferences(left[index], right[index])
		if len(differences) > 0 {
			t.Fatalf("Worker Session first divergence at record[%d] fields=%s:\nleft=%s\nright=%s\nfull-left=%s\nfull-right=%s", index, strings.Join(differences, ","), describeWSRFT011Record(left[index]), describeWSRFT011Record(right[index]), formatWSRFT011Records(left), formatWSRFT011Records(right))
		}
	}
}

func wsrft011RecordDifferences(left, right factoryapi.WorkerSessionEvent) []string {
	differences := make([]string, 0, 10)
	if left.WorkerSessionId != right.WorkerSessionId {
		differences = append(differences, "worker_session_id")
	}
	if !reflect.DeepEqual(left.WorkIds, right.WorkIds) {
		differences = append(differences, "work_ids")
	}
	if left.Event.Position != right.Event.Position {
		differences = append(differences, "position")
	}
	if left.Event.SourceType != right.Event.SourceType {
		differences = append(differences, "source_type")
	}
	if left.Event.SourceId != right.Event.SourceId {
		differences = append(differences, "source_id")
	}
	if left.Event.SourceSequence != right.Event.SourceSequence {
		differences = append(differences, "source_sequence")
	}
	if left.Event.SourceEventId != right.Event.SourceEventId {
		differences = append(differences, "source_event_id")
	}
	if left.Event.SchemaId != right.Event.SchemaId {
		differences = append(differences, "schema_id")
	}
	if wsrft011StringPointer(left.Event.Cursor.WorkerSessionId) != wsrft011StringPointer(right.Event.Cursor.WorkerSessionId) {
		differences = append(differences, "cursor_worker_session_id")
	}
	if left.Event.Cursor.Position != right.Event.Cursor.Position {
		differences = append(differences, "cursor_position")
	}
	if !reflect.DeepEqual(left.Event.Payload, right.Event.Payload) {
		differences = append(differences, "payload")
	}
	if wsrft011DeliveryClass(left.Delivery) != wsrft011DeliveryClass(right.Delivery) {
		differences = append(differences, "delivery")
	}
	return differences
}

func wsrft011DeliveryClass(delivery factoryapi.WorkerSessionEventDelivery) string {
	switch delivery {
	case factoryapi.WorkerSessionEventDelivery("TERMINAL"), factoryapi.WorkerSessionEventDelivery("TERMINAL_REPLAY"):
		return "TERMINAL"
	default:
		return string(delivery)
	}
}

func formatWSRFT011Records(records []factoryapi.WorkerSessionEvent) string {
	if len(records) == 0 {
		return "[]"
	}
	descriptions := make([]string, len(records))
	for index, record := range records {
		descriptions[index] = fmt.Sprintf("%d:{%s}", index, describeWSRFT011Record(record))
	}
	return "[" + strings.Join(descriptions, "; ") + "]"
}

func describeWSRFT011Record(event factoryapi.WorkerSessionEvent) string {
	return fmt.Sprintf(
		"position=%d cursor={generation=%q worker=%q position=%d} schema=%q source={type=%q id=%q sequence=%d event_id=%q} delivery=%q work_ids=%v payload=%s",
		event.Event.Position,
		wsrft011StringPointer(event.Event.Cursor.StreamGenerationId),
		wsrft011StringPointer(event.Event.Cursor.WorkerSessionId),
		event.Event.Cursor.Position,
		event.Event.SchemaId,
		event.Event.SourceType,
		event.Event.SourceId,
		event.Event.SourceSequence,
		event.Event.SourceEventId,
		event.Delivery,
		event.WorkIds,
		fmt.Sprintf("%#v", event.Event.Payload),
	)
}

func wsrft011StringPointer(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func assertWSRFT011CursorError(t *testing.T, endpoint, wantCode string) {
	t.Helper()
	status, response, err := requestWSRFT011Error(endpoint)
	if err != nil {
		t.Fatalf("GET cursor error %s: %v", endpoint, err)
	}
	if status != http.StatusBadRequest || string(response.Code) != wantCode {
		t.Fatalf("GET cursor error %s = %d/%#v, want 400/%s", endpoint, status, response, wantCode)
	}
}

func requestWSRFT011Error(endpoint string) (int, factoryapi.ErrorResponse, error) {
	response, err := http.Get(endpoint)
	if err != nil {
		return 0, factoryapi.ErrorResponse{}, err
	}
	defer response.Body.Close()
	var payload factoryapi.ErrorResponse
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return response.StatusCode, factoryapi.ErrorResponse{}, err
	}
	return response.StatusCode, payload, nil
}

func startWSRFT012ProviderServer(t *testing.T) (*support.FunctionalAPIServer, string, string, string) {
	t.Helper()
	dir := support.ScaffoldSingleStepFactory(t, "wsr-ft-012-provider-reference")
	support.WriteAgentConfig(t, dir, "processor", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "fixture-model"))
	providerSession := "session_fixture_codex_success"
	factorySession := uuid.NewString()
	homeDir := t.TempDir()
	writeWSRFT012CodexRollout(t, homeDir, providerSession)
	providerOutput := readWSRFT012ProviderFixture(t, "stdout.jsonl")
	runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{Stdout: providerOutput})
	env := append([]string(nil), os.Environ()...)
	env = append(env, "HOME="+homeDir, "USERPROFILE="+homeDir)
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		WaitForServiceModeRuntime: true,
		Args:                      []string{"--session", factorySession, "--record", filepath.Join(t.TempDir(), "wsr-ft-012-provider.replay.json")},
		Env:                       env,
		Edges: serviceedges.Edges{
			ProviderCommandRunner: runner,
			ProviderSessionResolveHomeDirectory: func() (string, error) {
				return homeDir, nil
			},
		},
	})
	workID := submitWSRFT011Work(t, server.URL(), factorySession, "provider-reference")
	support.WaitForSessionTerminalStatus(t, server.URL(), factorySession, 30*time.Second)
	workerID := workerIDForWSRFT011Work(t, server.URL(), factorySession, workID)
	observation := getWSRFT010Observation(t, server.URL(), factorySession, workerID)
	if observation.ProviderSession == nil || observation.ProviderSession.Provider != string(modelprovider.ProviderCodex) || observation.ProviderSession.Kind != "session_id" || observation.ProviderSession.Id != providerSession {
		t.Fatalf("Codex Worker Session provider association = %#v, want provider/codex session_id identity", observation.ProviderSession)
	}
	return server, factorySession, workerID, providerSession
}

func readWSRFT012ProviderFixture(t *testing.T, fileName string) []byte {
	t.Helper()
	path := filepath.Join(testutil.MustRepoRoot(t), filepath.FromSlash(support.ProviderSessionFixturePath("codex", "success", fileName)))
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read provider fixture %s: %v", path, err)
	}
	return contents
}

func writeWSRFT012CodexRollout(t *testing.T, homeDir, sessionID string) {
	t.Helper()
	directory := filepath.Join(homeDir, ".codex", "sessions", "2026", "07", "27")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatalf("create Codex rollout directory: %v", err)
	}
	path := filepath.Join(directory, "rollout-"+sessionID+".jsonl")
	if err := os.WriteFile(path, readWSRFT012ProviderFixture(t, "rollout.jsonl"), 0o600); err != nil {
		t.Fatalf("write Codex rollout fixture: %v", err)
	}
}

func getWSRFT012ProviderObservation(t *testing.T, baseURL, factorySession, providerSession string) factoryapi.WorkerSessionObservation {
	t.Helper()
	endpoint := providerWorkerSessionWSRFT012URL(baseURL, factorySession, "/detail", providerSession, nil)
	response, err := http.Get(endpoint)
	if err != nil {
		t.Fatalf("GET Provider-reference Worker Session observation: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("GET Provider-reference Worker Session observation status = %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	var observation factoryapi.WorkerSessionObservation
	if err := json.NewDecoder(response.Body).Decode(&observation); err != nil {
		t.Fatalf("decode Provider-reference Worker Session observation: %v", err)
	}
	return observation
}

type wsrft012TranscriptResult struct {
	status     int
	code       string
	transcript *factoryapi.WorkerSessionTranscriptResponse
}

func getWSRFT012Transcript(t *testing.T, endpoint string) wsrft012TranscriptResult {
	t.Helper()
	response, err := http.Get(endpoint)
	if err != nil {
		t.Fatalf("GET Worker Session transcript: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusOK {
		var transcript factoryapi.WorkerSessionTranscriptResponse
		if err := json.NewDecoder(response.Body).Decode(&transcript); err != nil {
			t.Fatalf("decode Worker Session transcript: %v", err)
		}
		return wsrft012TranscriptResult{status: response.StatusCode, transcript: &transcript}
	}
	var payload factoryapi.ErrorResponse
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatalf("decode Worker Session transcript error: %v", err)
	}
	return wsrft012TranscriptResult{status: response.StatusCode, code: string(payload.Code)}
}
