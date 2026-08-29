package watch_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	productionLedgerFixtureName        = "production-retry-ledger.replay.json"
	productionLedgerSource             = "factory-session-~default-083224-637a0122-f34d-4c5e-99a0-20e70b5375c2.json"
	productionLedgerWorkID             = "batch-operator-wave2-restoration-20260809-wo-c3-run-parity-long-prompt-input"
	productionLedgerWorkType           = "task"
	productionLedgerTerminalID         = "factory-event/work-state-change/batch-operator-wave2-restoration-20260809-wo-c3-run-parity-long-prompt-input/60"
	productionLedgerObservedResponseID = "factory-event/model-response/2cf2a099-909b-4446-8e8d-1453054e093c/model-request/1"
)

// TestWorkWatchRecordedProductionRetryLedger routes a checked-in, redacted
// recording from productionLedgerSource through the public HTTP stream and
// the same root-built Process used by the CLI entrypoint. The fixture keeps
// the source event order and request-derived model-response identities;
// corrected recorder identity proof remains in recorder tests.
func TestWorkWatchRecordedProductionRetryLedger(t *testing.T) {
	fixture := loadProductionLedgerFixture(t)
	assertProductionLedgerFixture(t, fixture)

	t.Run("CASE-WW-008 finite drains terminal retained history", func(t *testing.T) {
		stream := newProductionLedgerStream(t, fixture.Events)
		process := support.BuildProcess(t, serviceedges.Edges{})
		support.CleanupProcess(t, process)
		stdout := newLedgerOutput()
		stderr := newLedgerOutput()
		inputs := productionLedgerWatchInput(t, stream.URL(), false, stdout, stderr)
		command := support.StartProcessCommand(t, process, inputs)

		waitForLedgerCommand(t, command, stdout, stderr)
		lines := decodeWatchLines(t, stdout.String())
		assertProductionFiniteLines(t, lines)
	})

	t.Run("CASE-WW-017 finite exposes a replayed structured result on its first transition", func(t *testing.T) {
		events := productionLedgerEventsWithStructuredResult(t, fixture.Events)
		stream := newProductionLedgerStream(t, events)
		process := support.BuildProcess(t, serviceedges.Edges{})
		support.CleanupProcess(t, process)
		stdout := newLedgerOutput()
		stderr := newLedgerOutput()
		inputs := productionLedgerWatchInput(t, stream.URL(), false, stdout, stderr)
		command := support.StartProcessCommand(t, process, inputs)

		waitForLedgerCommand(t, command, stdout, stderr)
		lines := decodeWatchLines(t, stdout.String())
		assertProductionFiniteLines(t, lines)
		var result map[string]any
		if err := json.Unmarshal(lines[0].StructuredResult, &result); err != nil {
			t.Fatalf("decode replayed structuredResult: %v (%s)", err, lines[0].StructuredResult)
		}
		if result["decision"] != "accept" || result["attempt"] != float64(1) {
			t.Fatalf("replayed structuredResult = %#v, want decision=accept attempt=1", result)
		}
	})

	t.Run("CASE-WW-017 replayed ledger follow remains attached and consumes later transitions", func(t *testing.T) {
		runProductionLedgerFollowCase(t, fixture.Events)
	})

	t.Run("CASE-WW-005 rejects a same-sequence conflicting retry record", func(t *testing.T) {
		responseIndex := productionLedgerEventIndex(t, fixture.Events, factoryapi.FactoryEventTypeModelResponse)
		conflict := conflictingProductionLedgerEvent(t, fixture.Events[responseIndex], fixture.Events[responseIndex].Context.Sequence)
		history := append([]factoryapi.FactoryEvent(nil), fixture.Events[:responseIndex+1]...)
		history = append(history, conflict)
		stream := newProductionLedgerStream(t, history)
		process := support.BuildProcess(t, serviceedges.Edges{})
		support.CleanupProcess(t, process)
		stdout := newLedgerOutput()
		stderr := newLedgerOutput()
		inputs := productionLedgerWatchInput(t, stream.URL(), false, stdout, stderr)

		err := process.Execute(inputs)
		if err == nil {
			t.Fatal("conflicting production-ledger retry returned nil error")
		}
		if !strings.Contains(err.Error(), "non-increasing canonical sequence") {
			t.Fatalf("conflicting retry error = %v, want same-sequence corruption diagnostic", err)
		}
		if stdout.String() != "" {
			t.Fatalf("conflicting retry emitted Work transitions: %q", stdout.String())
		}
		support.RequireSafeCLIDiagnostic(t, stderr.String())
	})
}

func runProductionLedgerFollowCase(t *testing.T, events []factoryapi.FactoryEvent) {
	t.Helper()
	stream := newProductionLedgerStream(t, events)
	process := support.BuildProcess(t, serviceedges.Edges{})
	support.CleanupProcess(t, process)
	stdout := newLedgerOutput()
	stderr := newLedgerOutput()
	inputs := productionLedgerWatchInput(t, stream.URL(), true, stdout, stderr)
	command := support.StartProcessCommand(t, process, inputs)

	waitForLedgerSignal(t, stream.historySent, "retained production ledger")
	waitForLedgerLines(t, stdout, 1, "retained terminal transitions")

	stream.Publish(
		productionLedgerTransition(
			t,
			"factory-event/work-state-change/work-follow-up/in-review",
			223,
			"work-follow-up",
			"init",
			"in-review",
		),
		productionLedgerTransition(
			t,
			"factory-event/work-state-change/work-follow-up/complete",
			224,
			"work-follow-up",
			"in-review",
			"complete",
		),
	)
	waitForLedgerLines(t, stdout, 2, "later follow transitions")

	command.Stop(t)
	if err := command.Err(); err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("follow Work watch cancellation error = %v, want cancellation or nil", err)
	}
	if got := strings.TrimSpace(stderr.String()); got != "" && got != "Error: context canceled" {
		t.Fatalf("follow Work watch wrote unexpected diagnostics on test cancellation: %q", stderr.String())
	}

	lines := decodeWatchLines(t, stdout.String())
	assertProductionFollowLines(t, lines)
}

func productionLedgerEventIndex(t *testing.T, events []factoryapi.FactoryEvent, eventType factoryapi.FactoryEventType) int {
	t.Helper()
	for index, event := range events {
		if event.Type == eventType {
			return index
		}
	}
	t.Fatalf("production ledger has no %q event", eventType)
	return -1
}

func productionLedgerEventsWithStructuredResult(t *testing.T, events []factoryapi.FactoryEvent) []factoryapi.FactoryEvent {
	t.Helper()
	cloned := append([]factoryapi.FactoryEvent(nil), events...)
	found := false
	for index, event := range cloned {
		if event.Type != factoryapi.FactoryEventTypeDispatchResponse {
			continue
		}
		payload, err := event.Payload.AsDispatchResponseEventPayload()
		if err != nil || payload.OutputWork == nil || payload.Outcome == factoryapi.WorkOutcomeFailed || payload.Outcome == factoryapi.WorkOutcomeRejected {
			continue
		}
		outputs := append([]factoryapi.Work(nil), (*payload.OutputWork)...)
		for outputIndex, output := range outputs {
			if ledgerString(output.WorkId) != productionLedgerWorkID {
				continue
			}
			outputs[outputIndex].StructuredResult = map[string]any{
				"attempt":  float64(1),
				"decision": "accept",
			}
			payload.OutputWork = &outputs
			if err := event.Payload.FromDispatchResponseEventPayload(payload); err != nil {
				t.Fatalf("encode structured result into replay event %q: %v", event.Id, err)
			}
			cloned[index] = event
			found = true
		}
	}
	if found {
		return cloned
	}
	t.Fatalf("production ledger has no dispatch response output for %q", productionLedgerWorkID)
	return nil
}

type productionLedgerFixture struct {
	SchemaVersion string                    `json:"schemaVersion"`
	RecordedAt    time.Time                 `json:"recordedAt"`
	Events        []factoryapi.FactoryEvent `json:"events"`
}

func loadProductionLedgerFixture(t *testing.T) productionLedgerFixture {
	t.Helper()
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller could not locate the production ledger test")
	}
	path := filepath.Join(filepath.Dir(sourceFile), "testdata", productionLedgerFixtureName)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read production ledger fixture %s: %v", path, err)
	}
	text := string(data)
	for _, secretMarker := range []string{"OPENAI_API_KEY", "BEGIN PRIVATE KEY", "PRODUCTION_SECRET"} {
		if strings.Contains(text, secretMarker) {
			t.Fatalf("production ledger fixture contains secret marker %q", secretMarker)
		}
	}
	if regexp.MustCompile(`sk-[A-Za-z0-9_-]{20,}`).MatchString(text) {
		t.Fatal("production ledger fixture contains a likely API key")
	}
	if !strings.Contains(text, "redacted") {
		t.Fatal("production ledger fixture has no redacted sensitive values")
	}

	var fixture productionLedgerFixture
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatalf("decode production ledger fixture: %v", err)
	}
	return fixture
}

func assertProductionLedgerFixture(t *testing.T, fixture productionLedgerFixture) {
	t.Helper()
	t.Logf("validated redacted production source %s", productionLedgerSource)
	if fixture.SchemaVersion != "agent-factory.replay.v1" {
		t.Fatalf("production ledger schemaVersion = %q, want agent-factory.replay.v1", fixture.SchemaVersion)
	}
	if fixture.RecordedAt.IsZero() {
		t.Fatal("production ledger recordedAt is zero")
	}
	if len(fixture.Events) < 200 {
		t.Fatalf("production ledger event count = %d, want complete retry history", len(fixture.Events))
	}
	ids := validateProductionLedgerEnvelope(t, fixture.Events)
	if len(ids) != len(fixture.Events) {
		t.Fatalf("production ledger reused canonical event IDs: %d unique IDs for %d events", len(ids), len(fixture.Events))
	}
	if !hasProductionLedgerWorkRequest(t, fixture.Events, productionLedgerWorkID) {
		t.Fatalf("production ledger has no Work request for %q", productionLedgerWorkID)
	}
	modelRequestIDs := productionLedgerModelRequests(t, fixture.Events)
	modelResponseCount, failedModelResponse, succeededModelResponse := productionLedgerModelResponses(t, fixture.Events, modelRequestIDs)
	if modelResponseCount < 2 || !failedModelResponse || !succeededModelResponse {
		t.Fatalf("production ledger model retry outcomes = count:%d failed:%t succeeded:%t", modelResponseCount, failedModelResponse, succeededModelResponse)
	}
	if !hasProductionLedgerEventID(fixture.Events, productionLedgerObservedResponseID) {
		t.Fatalf("production ledger is missing observed model response %q", productionLedgerObservedResponseID)
	}
	if !hasProductionLedgerTerminal(t, fixture.Events, productionLedgerWorkID) {
		t.Fatalf("production ledger has no terminal transition for %q", productionLedgerWorkID)
	}
}

func validateProductionLedgerEnvelope(t *testing.T, events []factoryapi.FactoryEvent) map[string][]int {
	t.Helper()
	wantTypes := map[factoryapi.FactoryEventType]bool{
		factoryapi.FactoryEventTypeRunRequest:                       false,
		factoryapi.FactoryEventTypeInitialStructureRequest:          false,
		factoryapi.FactoryEventTypeSessionStarted:                   false,
		factoryapi.FactoryEventTypeFactoryStateResponse:             false,
		factoryapi.FactoryEventTypeWorkRequest:                      false,
		factoryapi.FactoryEventTypeDispatchRequest:                  false,
		factoryapi.FactoryEventTypeDispatchWorkerSessionAssociation: false,
		factoryapi.FactoryEventTypeModelRequest:                     false,
		factoryapi.FactoryEventTypeModelResponse:                    false,
		factoryapi.FactoryEventTypeDispatchResponse:                 false,
		factoryapi.FactoryEventTypeWorkStateChange:                  false,
		factoryapi.FactoryEventTypeAgentRunResponse:                 false,
		factoryapi.FactoryEventTypeRelationshipChangeRequest:        false,
		factoryapi.FactoryEventTypeScriptRequest:                    false,
		factoryapi.FactoryEventTypeScriptResponse:                   false,
	}
	ids := make(map[string][]int)
	for index, event := range events {
		if event.SchemaVersion != factoryapi.AgentFactoryEventV1 {
			t.Fatalf("fixture event %d schemaVersion = %q, want %q", index, event.SchemaVersion, factoryapi.AgentFactoryEventV1)
		}
		if event.Id == "" || event.Context.EventTime.IsZero() {
			t.Fatalf("fixture event %d has incomplete canonical identity/time: %#v", index, event)
		}
		if event.Context.Sequence != index {
			t.Fatalf("fixture event %d sequence = %d, want captured sequence %d", index, event.Context.Sequence, index)
		}
		_, ok := wantTypes[event.Type]
		if !ok {
			t.Fatalf("fixture event %d has unexpected event family %q", index, event.Type)
		}
		wantTypes[event.Type] = true
		ids[event.Id] = append(ids[event.Id], event.Context.Sequence)
	}
	for eventType, seen := range wantTypes {
		if !seen {
			t.Fatalf("production ledger omitted event family %q", eventType)
		}
	}
	return ids
}

func hasProductionLedgerEventID(events []factoryapi.FactoryEvent, eventID string) bool {
	for _, event := range events {
		if event.Id == eventID {
			return true
		}
	}
	return false
}

func hasProductionLedgerWorkRequest(t *testing.T, events []factoryapi.FactoryEvent, workID string) bool {
	t.Helper()
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeWorkRequest {
			continue
		}
		payload, err := event.Payload.AsWorkRequestEventPayload()
		if err != nil || payload.Works == nil {
			t.Fatalf("decode fixture Work request %q: payload=%#v error=%v", event.Id, payload, err)
		}
		for _, work := range *payload.Works {
			if ledgerString(work.WorkId) == workID && ledgerString(work.WorkTypeName) == productionLedgerWorkType {
				return true
			}
		}
	}
	return false
}

func productionLedgerModelRequests(t *testing.T, events []factoryapi.FactoryEvent) map[string][]int {
	t.Helper()
	requests := make(map[string][]int)
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeModelRequest {
			continue
		}
		payload, err := event.Payload.AsModelRequestEventPayload()
		if err != nil {
			t.Fatalf("decode fixture MODEL_REQUEST %q: %v", event.Id, err)
		}
		requests[payload.ModelRequestId] = append(requests[payload.ModelRequestId], event.Context.Sequence)
	}
	return requests
}

func productionLedgerModelResponses(
	t *testing.T,
	events []factoryapi.FactoryEvent,
	requests map[string][]int,
) (int, bool, bool) {
	t.Helper()
	count := 0
	failed := false
	succeeded := false
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeModelResponse {
			continue
		}
		payload, err := event.Payload.AsModelResponseEventPayload()
		if err != nil {
			t.Fatalf("decode fixture MODEL_RESPONSE %q: %v", event.Id, err)
		}
		count++
		if !strings.HasPrefix(event.Id, "factory-event/model-response/") || !strings.HasSuffix(event.Id, "/model-request/1") {
			t.Fatalf("fixture MODEL_RESPONSE %q does not retain request-ordinal identity", event.Id)
		}
		if event.Id != "factory-event/model-response/"+payload.ModelRequestId {
			t.Fatalf("fixture MODEL_RESPONSE %q does not correlate to modelRequestId %q", event.Id, payload.ModelRequestId)
		}
		requestSequences := requests[payload.ModelRequestId]
		if len(requestSequences) == 0 || requestSequences[0] >= event.Context.Sequence {
			t.Fatalf("fixture MODEL_RESPONSE %q has no earlier matching MODEL_REQUEST: %#v", event.Id, requestSequences)
		}
		if payload.OutputPreview == nil || *payload.OutputPreview != "redacted" {
			t.Fatalf("fixture MODEL_RESPONSE %q output was not redacted", event.Id)
		}
		switch string(payload.Outcome) {
		case "FAILED":
			failed = true
		case "SUCCEEDED":
			succeeded = true
		}
	}
	return count, failed, succeeded
}

func hasProductionLedgerTerminal(t *testing.T, events []factoryapi.FactoryEvent, workID string) bool {
	t.Helper()
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeWorkStateChange {
			continue
		}
		payload, err := event.Payload.AsWorkStateChangeEventPayload()
		if err != nil {
			t.Fatalf("decode fixture WORK_STATE_CHANGE %q: %v", event.Id, err)
		}
		if payload.WorkTypeName != productionLedgerWorkType {
			t.Fatalf("fixture Work transition type = %q, want %q", payload.WorkTypeName, productionLedgerWorkType)
		}
		if payload.WorkId == workID && payload.ToState == "complete" {
			return true
		}
	}
	return false
}

type productionLedgerStream struct {
	server      *httptest.Server
	history     []factoryapi.FactoryEvent
	later       chan factoryapi.FactoryEvent
	historySent chan struct{}
	historyOnce sync.Once
}

func newProductionLedgerStream(t *testing.T, events []factoryapi.FactoryEvent) *productionLedgerStream {
	t.Helper()
	stream := &productionLedgerStream{
		history:     append([]factoryapi.FactoryEvent(nil), events...),
		later:       make(chan factoryapi.FactoryEvent, 8),
		historySent: make(chan struct{}),
	}
	stream.server = httptest.NewServer(http.HandlerFunc(stream.serveHTTP))
	t.Cleanup(stream.server.Close)
	return stream
}

func (stream *productionLedgerStream) URL() string {
	if stream == nil || stream.server == nil {
		return ""
	}
	return stream.server.URL
}

func (stream *productionLedgerStream) Publish(events ...factoryapi.FactoryEvent) {
	for _, event := range events {
		stream.later <- event
	}
}

func (stream *productionLedgerStream) serveHTTP(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet || request.URL.Path != "/factory-sessions/~default/events" {
		http.NotFound(writer, request)
		return
	}
	writer.Header().Set("Content-Type", "text/event-stream")
	writer.Header().Set("Cache-Control", "no-cache")
	writer.Header().Set(factorysessions.SessionEventStreamRetainedCountHeader, strconv.Itoa(len(stream.history)))
	flusher, ok := writer.(http.Flusher)
	if !ok {
		http.Error(writer, "production ledger stream does not support flushing", http.StatusInternalServerError)
		return
	}
	for _, event := range stream.history {
		if !writeProductionLedgerSSE(writer, flusher, event) {
			return
		}
	}
	stream.historyOnce.Do(func() { close(stream.historySent) })

	for {
		select {
		case event := <-stream.later:
			if !writeProductionLedgerSSE(writer, flusher, event) {
				return
			}
		case <-request.Context().Done():
			return
		}
	}
}

func writeProductionLedgerSSE(writer http.ResponseWriter, flusher http.Flusher, event factoryapi.FactoryEvent) bool {
	payload, err := json.Marshal(event)
	if err != nil {
		return false
	}
	if _, err := fmt.Fprintf(writer, "data: %s\n\n", payload); err != nil {
		return false
	}
	flusher.Flush()
	return true
}

func productionLedgerWatchInput(
	t *testing.T,
	serverURL string,
	follow bool,
	stdout io.Writer,
	stderr io.Writer,
) root.Input {
	t.Helper()
	home := t.TempDir()
	args := []string{
		"you",
		"--server",
		serverURL,
		"work",
		"watch",
		"--session",
		factorysessions.DefaultSessionID,
	}
	if follow {
		args = append(args, "--follow")
	}
	falseValue := false
	return root.Input{
		Args:             args,
		Env:              append(os.Environ(), "HOME="+home, "USERPROFILE="+home),
		Stdin:            strings.NewReader(""),
		Stdout:           stdout,
		Stderr:           stderr,
		Context:          t.Context(),
		WorkingDirectory: home,
		StdinIsTTY:       &falseValue,
		StdoutIsTTY:      &falseValue,
		StderrIsTTY:      &falseValue,
	}
}

type ledgerOutput struct {
	mu    sync.Mutex
	data  bytes.Buffer
	lines chan struct{}
}

func newLedgerOutput() *ledgerOutput {
	return &ledgerOutput{lines: make(chan struct{}, 128)}
}

func (output *ledgerOutput) Write(data []byte) (int, error) {
	output.mu.Lock()
	_, _ = output.data.Write(data)
	lineCount := bytes.Count(data, []byte{'\n'})
	output.mu.Unlock()
	for index := 0; index < lineCount; index++ {
		select {
		case output.lines <- struct{}{}:
		default:
		}
	}
	return len(data), nil
}

func (output *ledgerOutput) String() string {
	output.mu.Lock()
	defer output.mu.Unlock()
	return output.data.String()
}

func waitForLedgerSignal(t *testing.T, signal <-chan struct{}, description string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	select {
	case <-signal:
	case <-ctx.Done():
		t.Fatalf("timed out waiting for %s: %v", description, ctx.Err())
	}
}

func waitForLedgerLines(t *testing.T, output *ledgerOutput, count int, description string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	for index := 0; index < count; index++ {
		select {
		case <-output.lines:
		case <-ctx.Done():
			t.Fatalf("timed out waiting for %s line %d/%d: %v", description, index+1, count, ctx.Err())
		}
	}
}

func waitForLedgerCommand(t *testing.T, command *support.ProcessCommand, stdout, stderr *ledgerOutput) {
	t.Helper()
	select {
	case <-command.Done():
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for finite Work watch; stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
	if err := command.Err(); err != nil {
		t.Fatalf("finite Work watch error = %v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
	}
	if strings.TrimSpace(stderr.String()) != "" {
		t.Fatalf("finite Work watch wrote diagnostics without verbose mode: %q", stderr.String())
	}
}

type productionLedgerExpectedLine struct {
	workID string
	from   string
	to     string
	id     string
	seq    int64
	final  bool
}

func assertProductionFiniteLines(t *testing.T, lines []workWatchLine) {
	t.Helper()
	assertProductionLines(t, lines, []productionLedgerExpectedLine{
		{
			productionLedgerWorkID,
			"to-complete",
			"complete",
			productionLedgerTerminalID,
			106,
			true,
		},
	})
}

func assertProductionFollowLines(t *testing.T, lines []workWatchLine) {
	t.Helper()
	want := []productionLedgerExpectedLine{
		{
			productionLedgerWorkID,
			"to-complete",
			"complete",
			productionLedgerTerminalID,
			106,
			true,
		},
		{"work-follow-up", "init", "in-review", "factory-event/work-state-change/work-follow-up/in-review", 223, false},
		{"work-follow-up", "in-review", "complete", "factory-event/work-state-change/work-follow-up/complete", 224, true},
	}
	assertProductionLines(t, lines, want)
}

func assertProductionLines(t *testing.T, lines []workWatchLine, want []productionLedgerExpectedLine) {
	t.Helper()
	if len(lines) != len(want) {
		t.Fatalf("follow Work watch transition count = %d, want %d: %#v", len(lines), len(want), lines)
	}
	seenIDs := make(map[string]bool, len(lines))
	for index, line := range lines {
		if line.SchemaVersion != "you.work.watch.v1" || line.SessionID != factorysessions.DefaultSessionID {
			t.Fatalf("follow line %d envelope = %#v", index, line)
		}
		if line.EventID != want[index].id || line.Sequence != want[index].seq || line.WorkID != want[index].workID || line.FromState != want[index].from || line.ToState != want[index].to || line.Terminal != want[index].final {
			t.Fatalf("follow line %d = %#v, want %#v", index, line, want[index])
		}
		if seenIDs[line.EventID] {
			t.Fatalf("follow Work watch emitted duplicate transition %q", line.EventID)
		}
		seenIDs[line.EventID] = true
	}
}

func productionLedgerTransition(t *testing.T, id string, sequence int, workID, fromState, toState string) factoryapi.FactoryEvent {
	t.Helper()
	var payload factoryapi.FactoryEvent_Payload
	if err := payload.FromWorkStateChangeEventPayload(factoryapi.WorkStateChangeEventPayload{
		FromPlaceId:  "task:" + fromState,
		FromState:    fromState,
		Reason:       ledgerStringPtr("follow-up transition"),
		Source:       factoryapi.WorkStateChangeSourceCLI,
		ToPlaceId:    "task:" + toState,
		ToState:      toState,
		WorkId:       workID,
		WorkTypeName: productionLedgerWorkType,
	}); err != nil {
		t.Fatalf("encode later Work transition %q: %v", id, err)
	}
	sessionID := factorysessions.DefaultSessionID
	return factoryapi.FactoryEvent{
		Context: factoryapi.FactoryEventContext{
			EventTime: time.Date(2026, time.August, 9, 13, 0, sequence, 0, time.UTC),
			Sequence:  sequence,
			SessionId: &sessionID,
		},
		Id:            id,
		Payload:       payload,
		SchemaVersion: factoryapi.AgentFactoryEventV1,
		Type:          factoryapi.FactoryEventTypeWorkStateChange,
	}
}

func conflictingProductionLedgerEvent(t *testing.T, event factoryapi.FactoryEvent, sequence int) factoryapi.FactoryEvent {
	t.Helper()
	encoded, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal source conflict event %q: %v", event.Id, err)
	}
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &envelope); err != nil {
		t.Fatalf("decode source conflict envelope %q: %v", event.Id, err)
	}
	var contextPayload map[string]any
	if err := json.Unmarshal(envelope["context"], &contextPayload); err != nil {
		t.Fatalf("decode source conflict context %q: %v", event.Id, err)
	}
	contextPayload["sequence"] = sequence
	envelope["context"], err = json.Marshal(contextPayload)
	if err != nil {
		t.Fatalf("encode source conflict context %q: %v", event.Id, err)
	}
	var responsePayload map[string]any
	if err := json.Unmarshal(envelope["payload"], &responsePayload); err != nil {
		t.Fatalf("decode source conflict payload %q: %v", event.Id, err)
	}
	responsePayload["outputPreview"] = "conflicting same-sequence retry"
	envelope["payload"], err = json.Marshal(responsePayload)
	if err != nil {
		t.Fatalf("encode source conflict payload %q: %v", event.Id, err)
	}
	encoded, err = json.Marshal(envelope)
	if err != nil {
		t.Fatalf("marshal conflict event %q: %v", event.Id, err)
	}
	var conflict factoryapi.FactoryEvent
	if err := json.Unmarshal(encoded, &conflict); err != nil {
		t.Fatalf("decode conflict event %q: %v", event.Id, err)
	}
	return conflict
}

func ledgerString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func ledgerStringPtr(value string) *string {
	return &value
}
