package watch_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
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
	controlledWatchWorkID   = "controlled-work-watch"
	controlledWatchWorkType = "task"
)

func TestWorkWatchControlledLifecycleCases(t *testing.T) {
	t.Run("boundary cases", runControlledWatchBoundaryCases)
	t.Run("cancellation cases", runControlledWatchCancellationCases)
	t.Run("recovery and cleanup cases", runControlledWatchRecoveryAndCleanupCases)
	t.Run("contention cases", runControlledWatchContentionCases)
}

func runControlledWatchBoundaryCases(t *testing.T) {
	t.Run("CASE-WW-002 empty stream cancellation", func(t *testing.T) {
		stream := newControlledWatchStream(t, nil, nil)
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		command, stdout, stderr, connection := startControlledWatch(t, stream, ctx, true)

		if got := stdout.String(); got != "" {
			t.Fatalf("empty canceled watch stdout = %q, want empty", got)
		}
		cancel()
		err := waitControlledWatchCommand(t, command)
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Fatalf("empty canceled watch error = %v, want nil or context.Canceled", err)
		}
		command.AcceptError()
		assertExpectedWatchCancellationDiagnostic(t, stderr.String())
		waitForControlledSignal(t, connection.closed, "empty watch stream close")
	})

	t.Run("CASE-WW-003 retained history precedes live transition", func(t *testing.T) {
		fixture := newControlledWatchFixture(t)
		stream := newControlledWatchStream(t, [][]factoryapi.FactoryEvent{{
			fixture.metadata,
			fixture.request,
			fixture.processing,
		}}, nil)
		command, stdout, stderr, connection := startControlledWatch(t, stream, t.Context(), false)
		waitForControlledSignal(t, connection.historySent, "retained Work watch history")
		waitForLedgerLines(t, stdout, 1, "retained non-terminal transition")

		connection.Publish(fixture.complete)
		waitForControlledSignal(t, connection.published, "live terminal publication")
		waitForControlledSignal(t, connection.delivered, "live terminal delivery")
		waitForLedgerLines(t, stdout, 1, "live terminal transition")
		assertControlledWatchSuccess(t, command, stdout, stderr)
		waitForControlledSignal(t, connection.closed, "successful retained/live stream close")
		assertControlledWatchLines(t, stdout.String(), []controlledWatchExpectedLine{
			{fixture.processing.Id, "init", "processing", false},
			{fixture.complete.Id, "processing", "complete", true},
		})
	})

	t.Run("CASE-WW-004 exact duplicate is idempotent", func(t *testing.T) {
		fixture := newControlledWatchFixture(t)
		stream := newControlledWatchStream(t, [][]factoryapi.FactoryEvent{{
			fixture.metadata,
			fixture.request,
			fixture.processing,
			fixture.processing,
			fixture.complete,
		}}, map[int]bool{0: true})
		command, stdout, stderr, connection := startControlledWatch(t, stream, t.Context(), false)
		assertControlledWatchSuccess(t, command, stdout, stderr)
		waitForControlledSignal(t, connection.closed, "duplicate watch stream close")
		assertControlledWatchLines(t, stdout.String(), []controlledWatchExpectedLine{
			{fixture.processing.Id, "init", "processing", false},
			{fixture.complete.Id, "processing", "complete", true},
		})
	})
}

func runControlledWatchCancellationCases(t *testing.T) {
	t.Run("CASE-WW-006 follow cancellation detaches", func(t *testing.T) {
		fixture := newControlledWatchFixture(t)
		stream := newControlledWatchStream(t, [][]factoryapi.FactoryEvent{{
			fixture.metadata,
			fixture.request,
			fixture.processing,
		}}, nil)
		ctx, cancel := context.WithCancel(t.Context())
		command, stdout, stderr, connection := startControlledWatch(t, stream, ctx, true)
		waitForControlledSignal(t, connection.historySent, "follow retained history")
		waitForLedgerLines(t, stdout, 1, "follow transition")

		cancel()
		err := waitControlledWatchCommand(t, command)
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Fatalf("follow cancellation error = %v, want nil or context.Canceled", err)
		}
		command.AcceptError()
		assertExpectedWatchCancellationDiagnostic(t, stderr.String())
		waitForControlledSignal(t, connection.closed, "follow cancellation stream close")
		assertControlledWatchLines(t, stdout.String(), []controlledWatchExpectedLine{
			{fixture.processing.Id, "init", "processing", false},
		})
	})

	t.Run("CASE-WW-007 established stream deadline", func(t *testing.T) {
		fixture := newControlledWatchFixture(t)
		stream := newControlledWatchStream(t, [][]factoryapi.FactoryEvent{{
			fixture.metadata,
			fixture.request,
		}}, nil)
		process := support.BuildProcess(t, serviceedges.Edges{})
		support.CleanupProcess(t, process)
		ctx, cancel := context.WithTimeout(t.Context(), 250*time.Millisecond)
		defer cancel()
		stdout := newLedgerOutput()
		stderr := newLedgerOutput()
		inputs := controlledWatchInput(t, ctx, stream.URL(), false, stdout, stderr)
		command := support.StartProcessCommand(t, process, inputs)
		connection := waitForControlledConnection(t, stream)
		waitForControlledSignal(t, connection.attached, "deadline watch stream attachment")

		err := waitControlledWatchCommand(t, command)
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("established watch deadline error = %v, want context.DeadlineExceeded", err)
		}
		command.AcceptError()
		if got := stdout.String(); got != "" {
			t.Fatalf("deadline watch stdout = %q, want no partial line", got)
		}
		support.RequireSafeCLIDiagnostic(t, stderr.String())
		waitForControlledSignal(t, connection.closed, "deadline watch stream close")
	})
}

func runControlledWatchRecoveryAndCleanupCases(t *testing.T) {
	t.Run("CASE-WW-005 conflicting duplicate fails safely", func(t *testing.T) {
		runControlledConflictFailure(t)
	})

	t.Run("CASE-WW-009 disconnect preserves accepted transition", func(t *testing.T) {
		runControlledReconnectCase(t)
	})

	t.Run("CASE-WW-010 reconnect uses accepted cursor", func(t *testing.T) {
		runControlledReconnectCase(t)
	})

	t.Run("CASE-WW-011 success cleanup closes stream", func(t *testing.T) {
		fixture := newControlledWatchFixture(t)
		stream := newControlledWatchStream(t, [][]factoryapi.FactoryEvent{{
			fixture.metadata,
			fixture.request,
			fixture.complete,
		}}, map[int]bool{0: true})
		command, stdout, stderr, connection := startControlledWatch(t, stream, t.Context(), false)
		assertControlledWatchSuccess(t, command, stdout, stderr)
		waitForControlledSignal(t, connection.closed, "success cleanup stream close")
		assertControlledWatchLines(t, stdout.String(), []controlledWatchExpectedLine{
			{fixture.complete.Id, "processing", "complete", true},
		})
	})

	t.Run("CASE-WW-012 failure cleanup preserves primary error", func(t *testing.T) {
		runControlledConflictFailure(t)
	})
}

func runControlledWatchContentionCases(t *testing.T) {
	t.Run("CASE-WW-016 delivery and cancellation use signals", func(t *testing.T) {
		fixture := newControlledWatchFixture(t)
		stream := newControlledWatchStream(t, [][]factoryapi.FactoryEvent{{
			fixture.metadata,
			fixture.request,
		}}, nil)
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		command, stdout, stderr, connection := startControlledWatch(t, stream, ctx, true)
		published := make(chan struct{})
		go func() {
			connection.Publish(fixture.processing)
			close(published)
		}()
		waitForLedgerLines(t, stdout, 1, "contended transition delivery")
		waitForControlledSignal(t, published, "contended publisher completion")
		cancel()
		err := waitControlledWatchCommand(t, command)
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Fatalf("contended cancellation error = %v, want nil or context.Canceled", err)
		}
		command.AcceptError()
		assertExpectedWatchCancellationDiagnostic(t, stderr.String())
		waitForControlledSignal(t, connection.closed, "contended cancellation stream close")
		assertControlledWatchLines(t, stdout.String(), []controlledWatchExpectedLine{
			{fixture.processing.Id, "init", "processing", false},
		})
	})
}

func runControlledConflictFailure(t *testing.T) {
	t.Helper()
	fixture := newControlledWatchFixture(t)
	conflict := controlledWatchTransition(t, fixture.processing.Id, fixture.processing.Context.Sequence, "processing", "complete")
	stream := newControlledWatchStream(t, [][]factoryapi.FactoryEvent{{
		fixture.metadata,
		fixture.request,
		fixture.processing,
		conflict,
	}}, map[int]bool{0: true})
	command, stdout, stderr, connection := startControlledWatch(t, stream, t.Context(), false)
	err := waitControlledWatchCommand(t, command)
	if err == nil || !strings.Contains(err.Error(), "conflicting canonical data") {
		t.Fatalf("conflicting duplicate error = %v, want canonical conflict", err)
	}
	command.AcceptError()
	support.RequireSafeCLIDiagnostic(t, stderr.String())
	assertControlledWatchLines(t, stdout.String(), []controlledWatchExpectedLine{
		{fixture.processing.Id, "init", "processing", false},
	})
	waitForControlledSignal(t, connection.closed, "conflicting watch stream close")
}

func runControlledReconnectCase(t *testing.T) {
	t.Helper()
	fixture := newControlledWatchFixture(t)
	stream := newControlledWatchStream(t, [][]factoryapi.FactoryEvent{
		{fixture.metadata, fixture.request, fixture.processing},
		nil,
	}, map[int]bool{0: true})
	command, stdout, stderr, first := startControlledWatch(t, stream, t.Context(), false)
	waitForControlledSignal(t, first.historySent, "disconnect retained history")
	waitForLedgerLines(t, stdout, 1, "transition before disconnect")
	second := waitForControlledConnection(t, stream)
	waitForControlledSignal(t, second.attached, "reconnected Work watch stream")
	query := second.query
	if got := query.Get("after_event_id"); got != fixture.processing.Id {
		t.Fatalf("reconnect after_event_id = %q, want %q", got, fixture.processing.Id)
	}
	if got := query.Get("after_sequence"); got != fmt.Sprint(fixture.processing.Context.Sequence) {
		t.Fatalf("reconnect after_sequence = %q, want %d", got, fixture.processing.Context.Sequence)
	}
	second.Publish(fixture.complete)
	waitForControlledSignal(t, second.published, "reconnected terminal publication")
	waitForControlledSignal(t, second.delivered, "reconnected terminal delivery")
	waitForLedgerLines(t, stdout, 1, "transition after reconnect")
	assertControlledWatchSuccess(t, command, stdout, stderr)
	waitForControlledSignal(t, first.closed, "disconnected stream close")
	waitForControlledSignal(t, second.closed, "reconnected stream close")
	assertControlledWatchLines(t, stdout.String(), []controlledWatchExpectedLine{
		{fixture.processing.Id, "init", "processing", false},
		{fixture.complete.Id, "processing", "complete", true},
	})
}

func assertControlledWatchSuccess(t *testing.T, command *support.ProcessCommand, stdout, stderr *ledgerOutput) {
	t.Helper()
	if err := waitControlledWatchCommand(t, command); err != nil {
		t.Fatalf("controlled Work watch error = %v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
	}
	if got := strings.TrimSpace(stderr.String()); got != "" {
		t.Fatalf("controlled Work watch wrote diagnostics without verbose mode: %q", got)
	}
}

func waitControlledWatchCommand(t *testing.T, command *support.ProcessCommand) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), workWatchAttachmentTimeout)
	defer cancel()
	select {
	case <-command.Done():
		return command.Err()
	case <-ctx.Done():
		t.Fatalf("timed out waiting for controlled Work watch command: %v", ctx.Err())
		return nil
	}
}

func assertExpectedWatchCancellationDiagnostic(t *testing.T, stderr string) {
	t.Helper()
	if got := strings.TrimSpace(stderr); got != "" && got != "Error: context canceled" {
		t.Fatalf("watch cancellation wrote unexpected diagnostic: %q", stderr)
	}
}

type controlledWatchExpectedLine struct {
	eventID string
	from    string
	to      string
	final   bool
}

func assertControlledWatchLines(t *testing.T, output string, want []controlledWatchExpectedLine) {
	t.Helper()
	lines := decodeWatchLines(t, output)
	if len(lines) != len(want) {
		t.Fatalf("controlled Work watch line count = %d, want %d: %q", len(lines), len(want), output)
	}
	for index, line := range lines {
		if line.SchemaVersion != "you.work.watch.v1" || line.SessionID != factorysessions.DefaultSessionID {
			t.Fatalf("controlled line %d envelope = %#v", index, line)
		}
		if line.EventID != want[index].eventID || line.FromState != want[index].from ||
			line.ToState != want[index].to || line.Terminal != want[index].final {
			t.Fatalf("controlled line %d = %#v, want %#v", index, line, want[index])
		}
		if index > 0 && line.Sequence <= lines[index-1].Sequence {
			t.Fatalf("controlled sequences regressed at line %d: %#v", index, lines)
		}
	}
	if !strings.HasSuffix(output, "\n") {
		t.Fatalf("controlled Work watch output has partial final line: %q", output)
	}
}

type controlledWatchFixture struct {
	metadata   factoryapi.FactoryEvent
	request    factoryapi.FactoryEvent
	processing factoryapi.FactoryEvent
	complete   factoryapi.FactoryEvent
}

func newControlledWatchFixture(t *testing.T) controlledWatchFixture {
	t.Helper()
	return controlledWatchFixture{
		metadata: controlledWatchEvent(t, factoryapi.FactoryEventTypeInitialStructureRequest, "controlled-factory", 0,
			factoryapi.InitialStructureRequestEventPayload{Factory: factoryapi.Factory{
				WorkTypes: &[]factoryapi.WorkType{{Name: controlledWatchWorkType, States: []factoryapi.WorkState{
					{Name: "init", Type: factoryapi.WorkStateTypeINITIAL},
					{Name: "processing", Type: factoryapi.WorkStateTypePROCESSING},
					{Name: "complete", Type: factoryapi.WorkStateTypeTERMINAL},
					{Name: "failed", Type: factoryapi.WorkStateTypeFAILED},
				}}},
			}}),
		request: controlledWatchEvent(t, factoryapi.FactoryEventTypeWorkRequest, "controlled-request", 1,
			factoryapi.WorkRequestEventPayload{Works: &[]factoryapi.Work{{
				WorkId:       controlledWatchStringPtr(controlledWatchWorkID),
				WorkTypeName: controlledWatchStringPtr(controlledWatchWorkType),
				State:        &factoryapi.WorkState{Name: "init", Type: factoryapi.WorkStateTypeINITIAL},
			}}}),
		processing: controlledWatchTransition(t, "controlled-processing", 2, "init", "processing"),
		complete:   controlledWatchTransition(t, "controlled-complete", 3, "processing", "complete"),
	}
}

func controlledWatchEvent(t *testing.T, eventType factoryapi.FactoryEventType, id string, sequence int, payload any) factoryapi.FactoryEvent {
	t.Helper()
	var union factoryapi.FactoryEvent_Payload
	var err error
	switch typed := payload.(type) {
	case factoryapi.InitialStructureRequestEventPayload:
		err = union.FromInitialStructureRequestEventPayload(typed)
	case factoryapi.WorkRequestEventPayload:
		err = union.FromWorkRequestEventPayload(typed)
	case factoryapi.WorkStateChangeEventPayload:
		err = union.FromWorkStateChangeEventPayload(typed)
	default:
		t.Fatalf("unsupported controlled Work watch payload %T", payload)
	}
	if err != nil {
		t.Fatalf("encode controlled Work watch payload: %v", err)
	}
	sessionID := factorysessions.DefaultSessionID
	return factoryapi.FactoryEvent{
		SchemaVersion: factoryapi.AgentFactoryEventV1,
		Type:          eventType,
		Id:            id,
		Context: factoryapi.FactoryEventContext{
			EventTime: time.Date(2026, time.August, 28, 12, 0, sequence, 0, time.UTC),
			Sequence:  sequence,
			SessionId: &sessionID,
		},
		Payload: union,
	}
}

func controlledWatchTransition(t *testing.T, id string, sequence int, fromState, toState string) factoryapi.FactoryEvent {
	t.Helper()
	return controlledWatchEvent(t, factoryapi.FactoryEventTypeWorkStateChange, id, sequence,
		factoryapi.WorkStateChangeEventPayload{
			WorkId:       controlledWatchWorkID,
			WorkTypeName: controlledWatchWorkType,
			FromState:    fromState,
			ToState:      toState,
			Source:       factoryapi.WorkStateChangeSourceCLI,
			Reason:       controlledWatchStringPtr("controlled lifecycle transition"),
		})
}

func controlledWatchStringPtr(value string) *string { return &value }

type controlledWatchStream struct {
	server            *httptest.Server
	histories         [][]factoryapi.FactoryEvent
	closeAfterHistory map[int]bool
	requests          chan *controlledWatchConnection

	mu          sync.Mutex
	nextRequest int
}

type controlledWatchConnection struct {
	index       int
	query       url.Values
	attached    chan struct{}
	historySent chan struct{}
	closed      chan struct{}
	events      chan factoryapi.FactoryEvent
	published   chan struct{}
	delivered   chan struct{}
	publishOnce sync.Once
	deliverOnce sync.Once
}

func newControlledWatchStream(
	t *testing.T,
	histories [][]factoryapi.FactoryEvent,
	closeAfterHistory map[int]bool,
) *controlledWatchStream {
	t.Helper()
	stream := &controlledWatchStream{
		histories:         cloneControlledWatchHistories(histories),
		closeAfterHistory: appendControlledWatchCloseMap(closeAfterHistory),
		requests:          make(chan *controlledWatchConnection, 8),
	}
	stream.server = httptest.NewServer(http.HandlerFunc(stream.serveHTTP))
	t.Cleanup(stream.server.Close)
	return stream
}

func cloneControlledWatchHistories(histories [][]factoryapi.FactoryEvent) [][]factoryapi.FactoryEvent {
	cloned := make([][]factoryapi.FactoryEvent, len(histories))
	for index, history := range histories {
		cloned[index] = append([]factoryapi.FactoryEvent(nil), history...)
	}
	return cloned
}

func appendControlledWatchCloseMap(values map[int]bool) map[int]bool {
	cloned := make(map[int]bool, len(values))
	for index, closeAfter := range values {
		cloned[index] = closeAfter
	}
	return cloned
}

func (stream *controlledWatchStream) URL() string {
	if stream == nil || stream.server == nil {
		return ""
	}
	return stream.server.URL
}

func (stream *controlledWatchStream) serveHTTP(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet || request.URL.Path != workWatchEventPath {
		http.NotFound(writer, request)
		return
	}
	connection := stream.newConnection(request)
	defer close(connection.closed)
	select {
	case stream.requests <- connection:
	case <-request.Context().Done():
		return
	}
	writer.Header().Set("Content-Type", "text/event-stream")
	writer.Header().Set("Cache-Control", "no-cache")
	writer.Header().Set(factorysessions.SessionEventStreamRetainedCountHeader, fmt.Sprint(len(stream.history(connection.index))))
	flusher, ok := writer.(http.Flusher)
	if !ok {
		return
	}
	flusher.Flush()
	close(connection.attached)
	for _, event := range stream.history(connection.index) {
		if err := writeControlledWatchSSE(writer, flusher, event); err != nil {
			return
		}
	}
	close(connection.historySent)
	if stream.closeAfterHistory[connection.index] {
		return
	}
	for {
		select {
		case event := <-connection.events:
			if err := writeControlledWatchSSE(writer, flusher, event); err != nil {
				return
			}
			connection.deliverOnce.Do(func() { close(connection.delivered) })
		case <-request.Context().Done():
			return
		}
	}
}

func (stream *controlledWatchStream) newConnection(request *http.Request) *controlledWatchConnection {
	stream.mu.Lock()
	index := stream.nextRequest
	stream.nextRequest++
	stream.mu.Unlock()
	query := make(map[string][]string, len(request.URL.Query()))
	for key, values := range request.URL.Query() {
		query[key] = append([]string(nil), values...)
	}
	return &controlledWatchConnection{
		index:       index,
		query:       url.Values(query),
		attached:    make(chan struct{}),
		historySent: make(chan struct{}),
		closed:      make(chan struct{}),
		events:      make(chan factoryapi.FactoryEvent, 8),
		published:   make(chan struct{}),
		delivered:   make(chan struct{}),
	}
}

func (stream *controlledWatchStream) history(index int) []factoryapi.FactoryEvent {
	if index < 0 || index >= len(stream.histories) {
		return nil
	}
	return stream.histories[index]
}

func (connection *controlledWatchConnection) Publish(events ...factoryapi.FactoryEvent) {
	for _, event := range events {
		select {
		case connection.events <- event:
			connection.publishOnce.Do(func() { close(connection.published) })
		case <-connection.closed:
			return
		}
	}
}

func writeControlledWatchSSE(writer http.ResponseWriter, flusher http.Flusher, event factoryapi.FactoryEvent) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(writer, "data: %s\n\n", payload); err != nil {
		return err
	}
	flusher.Flush()
	return nil
}

func startControlledWatch(
	t *testing.T,
	stream *controlledWatchStream,
	ctx context.Context,
	follow bool,
) (*support.ProcessCommand, *ledgerOutput, *ledgerOutput, *controlledWatchConnection) {
	t.Helper()
	process := support.BuildProcess(t, serviceedges.Edges{})
	support.CleanupProcess(t, process)
	stdout := newLedgerOutput()
	stderr := newLedgerOutput()
	inputs := controlledWatchInput(t, ctx, stream.URL(), follow, stdout, stderr)
	command := support.StartProcessCommand(t, process, inputs)
	connection := waitForControlledConnection(t, stream)
	waitForControlledSignal(t, connection.attached, "controlled Work watch stream attachment")
	return command, stdout, stderr, connection
}

func controlledWatchInput(
	t *testing.T,
	ctx context.Context,
	serverURL string,
	follow bool,
	stdout,
	stderr *ledgerOutput,
) root.Input {
	t.Helper()
	if ctx == nil {
		ctx = t.Context()
	}
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
		Context:          ctx,
		WorkingDirectory: home,
		StdinIsTTY:       &falseValue,
		StdoutIsTTY:      &falseValue,
		StderrIsTTY:      &falseValue,
	}
}

func waitForControlledConnection(t *testing.T, stream *controlledWatchStream) *controlledWatchConnection {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), workWatchAttachmentTimeout)
	defer cancel()
	select {
	case connection := <-stream.requests:
		return connection
	case <-ctx.Done():
		t.Fatalf("timed out waiting for controlled Work watch request: %v", ctx.Err())
		return nil
	}
}

func waitForControlledSignal(t *testing.T, signal <-chan struct{}, description string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), workWatchAttachmentTimeout)
	defer cancel()
	select {
	case <-signal:
	case <-ctx.Done():
		t.Fatalf("timed out waiting for %s: %v", description, ctx.Err())
	}
}
