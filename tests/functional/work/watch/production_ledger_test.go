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
	productionLedgerFixtureName = "production-retry-ledger.replay.json"
	productionLedgerWorkID      = "work-production-retry"
	productionLedgerWorkType    = "task"
	productionLedgerRetryID     = "factory-event/model-response/2cf2a099-909b-4446-8e8d-1453054e093c/model-request/1"
)

// TestWorkWatchRecordedProductionRetryLedger routes a checked-in, redacted
// recorded event ledger through the public HTTP stream and the same
// root-built Process used by the CLI entrypoint. The fixture deliberately
// includes the observed request-derived model-response identity at a later
// sequence; corrected recorder identity proof remains in recorder tests.
func TestWorkWatchRecordedProductionRetryLedger(t *testing.T) {
	fixture := loadProductionLedgerFixture(t)
	assertProductionLedgerFixture(t, fixture)

	t.Run("finite drains terminal retained history", func(t *testing.T) {
		stream := newProductionLedgerStream(t, fixture.Events)
		process := support.BuildProcess(t, serviceedges.Edges{})
		support.CleanupProcess(t, process)
		stdout := newLedgerOutput()
		stderr := newLedgerOutput()
		inputs := productionLedgerWatchInput(t, stream.URL(), false, stdout, stderr)
		command := support.StartProcessCommand(t, process, inputs)

		waitForLedgerCommand(t, command, stdout, stderr)
		lines := decodeWatchLines(t, stdout.String())
		assertWorkWatchTransitionLines(
			t,
			lines,
			factorysessions.DefaultSessionID,
			productionLedgerWorkID,
			[][2]string{{"init", "processing"}, {"processing", "complete"}},
		)
	})

	t.Run("follow remains attached and consumes later transitions", func(t *testing.T) {
		stream := newProductionLedgerStream(t, fixture.Events)
		process := support.BuildProcess(t, serviceedges.Edges{})
		support.CleanupProcess(t, process)
		stdout := newLedgerOutput()
		stderr := newLedgerOutput()
		inputs := productionLedgerWatchInput(t, stream.URL(), true, stdout, stderr)
		command := support.StartProcessCommand(t, process, inputs)

		waitForLedgerSignal(t, stream.historySent, "retained production ledger")
		waitForLedgerLines(t, stdout, 2, "retained terminal transitions")

		stream.Publish(
			productionLedgerTransition(
				t,
				"factory-event/work-state-change/work-follow-up/processing",
				15,
				"work-follow-up",
				"init",
				"processing",
			),
			productionLedgerTransition(
				t,
				"factory-event/work-state-change/work-follow-up/complete",
				16,
				"work-follow-up",
				"processing",
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
	})

	t.Run("rejects a same-sequence conflicting retry record", func(t *testing.T) {
		conflict := conflictingProductionLedgerEvent(t, fixture.Events[8], fixture.Events[8].Context.Sequence)
		history := append([]factoryapi.FactoryEvent(nil), fixture.Events[:9]...)
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
		if !strings.Contains(stderr.String(), "non-increasing canonical sequence") {
			t.Fatalf("conflicting retry stderr = %q, want corruption diagnostic", stderr.String())
		}
	})
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
	for _, secretMarker := range []string{"OPENAI_API_KEY", "sk-", "BEGIN PRIVATE KEY", "PRODUCTION_SECRET"} {
		if strings.Contains(text, secretMarker) {
			t.Fatalf("production ledger fixture contains secret marker %q", secretMarker)
		}
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
	if fixture.SchemaVersion != "agent-factory.replay.v1" {
		t.Fatalf("production ledger schemaVersion = %q, want agent-factory.replay.v1", fixture.SchemaVersion)
	}
	if fixture.RecordedAt.IsZero() {
		t.Fatal("production ledger recordedAt is zero")
	}
	if len(fixture.Events) < 15 {
		t.Fatalf("production ledger event count = %d, want complete retry history", len(fixture.Events))
	}

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
		factoryapi.FactoryEventTypeRunResponse:                      false,
	}
	ids := make(map[string][]int)
	modelRequestIDs := make(map[string][]int)
	terminalWork := false
	for index, event := range fixture.Events {
		if event.SchemaVersion != factoryapi.AgentFactoryEventV1 {
			t.Fatalf("fixture event %d schemaVersion = %q, want %q", index, event.SchemaVersion, factoryapi.AgentFactoryEventV1)
		}
		if event.Id == "" || event.Context.EventTime.IsZero() {
			t.Fatalf("fixture event %d has incomplete canonical identity/time: %#v", index, event)
		}
		if event.Context.Sequence != index {
			t.Fatalf("fixture event %d sequence = %d, want %d", index, event.Context.Sequence, index)
		}
		_, ok := wantTypes[event.Type]
		if !ok {
			t.Fatalf("fixture event %d has unexpected event family %q", index, event.Type)
		}
		wantTypes[event.Type] = true
		ids[event.Id] = append(ids[event.Id], event.Context.Sequence)

		switch event.Type {
		case factoryapi.FactoryEventTypeWorkRequest:
			payload, err := event.Payload.AsWorkRequestEventPayload()
			if err != nil || payload.Works == nil || len(*payload.Works) != 1 {
				t.Fatalf("decode fixture Work request %q: payload=%#v error=%v", event.Id, payload, err)
			}
			work := (*payload.Works)[0]
			if ledgerString(work.WorkId) != productionLedgerWorkID || ledgerString(work.WorkTypeName) != productionLedgerWorkType {
				t.Fatalf("fixture Work lineage = workId:%q workType:%q, want %q:%q", ledgerString(work.WorkId), ledgerString(work.WorkTypeName), productionLedgerWorkID, productionLedgerWorkType)
			}
		case factoryapi.FactoryEventTypeModelRequest:
			payload, err := event.Payload.AsModelRequestEventPayload()
			if err != nil {
				t.Fatalf("decode fixture MODEL_REQUEST %q: %v", event.Id, err)
			}
			modelRequestIDs[payload.ModelRequestId] = append(modelRequestIDs[payload.ModelRequestId], event.Context.Sequence)
		case factoryapi.FactoryEventTypeModelResponse:
			payload, err := event.Payload.AsModelResponseEventPayload()
			if err != nil {
				t.Fatalf("decode fixture MODEL_RESPONSE %q: %v", event.Id, err)
			}
			if payload.ModelRequestId != "dispatch-retry/model-request/1" {
				t.Fatalf("fixture MODEL_RESPONSE %q correlation = %q, want request ordinal", event.Id, payload.ModelRequestId)
			}
			if event.Id != productionLedgerRetryID {
				t.Fatalf("fixture MODEL_RESPONSE id %q does not retain the observed request-ordinal shape", event.Id)
			}
		case factoryapi.FactoryEventTypeWorkStateChange:
			payload, err := event.Payload.AsWorkStateChangeEventPayload()
			if err != nil {
				t.Fatalf("decode fixture WORK_STATE_CHANGE %q: %v", event.Id, err)
			}
			if payload.WorkId != productionLedgerWorkID || payload.WorkTypeName != productionLedgerWorkType {
				t.Fatalf("fixture Work transition lineage = %q/%q", payload.WorkId, payload.WorkTypeName)
			}
			terminalWork = terminalWork || payload.ToState == "complete"
		}
	}
	for eventType, seen := range wantTypes {
		if !seen {
			t.Fatalf("production ledger omitted event family %q", eventType)
		}
	}
	if got := ids[productionLedgerRetryID]; len(got) != 2 || got[0] >= got[1] {
		t.Fatalf("repeated model-response identity sequences = %#v, want two increasing occurrences", got)
	}
	if got := modelRequestIDs["dispatch-retry/model-request/1"]; len(got) != 2 || got[0] >= got[1] {
		t.Fatalf("refreshed model-request ordinal sequences = %#v, want two increasing occurrences", got)
	}
	if !terminalWork {
		t.Fatal("production ledger has no terminal Work transition")
	}
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

func assertProductionFollowLines(t *testing.T, lines []workWatchLine) {
	t.Helper()
	want := []struct {
		workID string
		from   string
		to     string
		id     string
		seq    int64
		final  bool
	}{
		{productionLedgerWorkID, "init", "processing", "factory-event/work-state-change/work-production-retry/processing", 10, false},
		{productionLedgerWorkID, "processing", "complete", "factory-event/work-state-change/work-production-retry/complete", 13, true},
		{"work-follow-up", "init", "processing", "factory-event/work-state-change/work-follow-up/processing", 15, false},
		{"work-follow-up", "processing", "complete", "factory-event/work-state-change/work-follow-up/complete", 16, true},
	}
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
