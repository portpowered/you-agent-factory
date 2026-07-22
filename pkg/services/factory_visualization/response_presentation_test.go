package factory_visualization

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
)

type gatedPresentationWriter struct {
	gate     chan struct{}
	attempts atomic.Int64
	mu       sync.Mutex
	buffer   bytes.Buffer
}

func newGatedPresentationWriter() *gatedPresentationWriter {
	return &gatedPresentationWriter{gate: make(chan struct{})}
}

func (w *gatedPresentationWriter) Write(payload []byte) (int, error) {
	w.attempts.Add(1)
	<-w.gate
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buffer.Write(payload)
}

func (w *gatedPresentationWriter) release() { close(w.gate) }

func (w *gatedPresentationWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buffer.String()
}

func TestResponsePresentation_BestEffortOutputDoesNotBlockAndBoundsBacklog(t *testing.T) {
	t.Parallel()

	writer := newGatedPresentationWriter()
	output := newBestEffortOutput(writer)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for index := 0; index < defaultProgressQueueCapacity+16; index++ {
			_ = output.Enqueue([]byte("progress"))
		}
	}()

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("best-effort enqueue blocked behind a slow presentation writer")
	}
	if output.Dropped() == 0 {
		t.Fatal("best-effort output did not report bounded-backlog drops")
	}
	writer.release()
	if err := output.CloseAndDrain(); err != nil {
		t.Fatalf("CloseAndDrain: %v", err)
	}
}

func TestResponsePresentation_LosslessOutputPreservesOrderThroughSlowWriter(t *testing.T) {
	t.Parallel()

	writer := newGatedPresentationWriter()
	output := newLosslessOutput(writer)
	for _, record := range []string{"first", "second", "terminal"} {
		if err := output.Enqueue([]byte(record)); err != nil {
			t.Fatalf("Enqueue(%q): %v", record, err)
		}
	}
	done := make(chan error, 1)
	go func() { done <- output.CloseAndDrain() }()
	waitForPresentationWriteAttempt(t, writer)
	select {
	case err := <-done:
		t.Fatalf("lossless drain completed while writer remained blocked: %v", err)
	default:
	}
	writer.release()
	if err := <-done; err != nil {
		t.Fatalf("CloseAndDrain: %v", err)
	}
	if got, want := writer.String(), "first\nsecond\nterminal\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestResponsePresentation_BestEffortDrainAbandonsSlowWriter(t *testing.T) {
	t.Parallel()

	writer := newGatedPresentationWriter()
	output := newBestEffortOutput(writer)
	if err := output.Enqueue([]byte("progress")); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	waitForPresentationWriteAttempt(t, writer)
	started := time.Now()
	if err := output.CloseAndDrain(); err != nil {
		t.Fatalf("CloseAndDrain: %v", err)
	}
	if elapsed := time.Since(started); elapsed > progressDrainTimeout+250*time.Millisecond {
		t.Fatalf("best-effort drain took %s, want bounded completion", elapsed)
	}
	writer.release()
}

func TestResponsePresentation_AttachmentDeliversAndDrainsBeforeDetach(t *testing.T) {
	t.Parallel()

	cursor := newPresentationCursor()
	source := presentationSource{cursor: cursor}
	sink := &recordingPresentationSink{}
	attachment := responsePresentation{}.Attach(
		context.Background(), source, factorysessions.DefaultSessionID, sink,
	)
	if attachment == nil {
		t.Fatal("Attach returned nil")
	}
	cursor.batches <- []factorysessions.FactoryResponseEvent{{Sequence: 1}}
	waitForPresentedEvents(t, sink, 1)
	cursor.batches <- []factorysessions.FactoryResponseEvent{{Sequence: 2}}
	attachment.Stop()
	if got := sink.sequences(); strings.TrimSpace(got) != "1 2" {
		t.Fatalf("presented sequences = %q, want %q", got, "1 2")
	}
	select {
	case <-cursor.detached:
	default:
		t.Fatal("response-event cursor was not detached")
	}
}

func TestResponsePresentation_ResponseStreamOwnsOrderingDeduplicationAndTerminalWrite(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	stream := NewResponsePresentation().OpenLosslessResponseStream(
		&output,
		func(event factorysessions.FactoryResponseEvent) ([]byte, bool) {
			return []byte(event.EventID), true
		},
	)
	stream.PresentResponseEvents([]factorysessions.FactoryResponseEvent{
		{Sequence: 1, EventID: "first"},
		{Sequence: 1, EventID: "duplicate"},
		{Sequence: 2, EventID: "second"},
	})

	first, err := stream.Finalize(func(writer io.Writer, progressSeen bool) error {
		if !progressSeen {
			t.Fatal("Finalize progressSeen = false, want true")
		}
		_, writeErr := io.WriteString(writer, "terminal")
		return writeErr
	})
	if err != nil {
		t.Fatalf("Finalize: %v", err)
	}
	if !first {
		t.Fatal("Finalize first = false, want true")
	}
	stream.PresentResponseEvents([]factorysessions.FactoryResponseEvent{{Sequence: 3, EventID: "late"}})
	if got, want := output.String(), "first\nsecond\nterminal"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestResponsePresentation_ResponseStreamWritesTerminalOnceConcurrently(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	stream := NewResponsePresentation().OpenLosslessResponseStream(
		&output,
		func(event factorysessions.FactoryResponseEvent) ([]byte, bool) {
			return []byte(event.EventID), true
		},
	)
	stream.PresentResponseEvents([]factorysessions.FactoryResponseEvent{{Sequence: 1, EventID: "progress"}})

	const callers = 16
	var terminalWrites atomic.Int64
	results := make(chan bool, callers)
	errors := make(chan error, callers)
	var group sync.WaitGroup
	group.Add(callers)
	for range callers {
		go func() {
			defer group.Done()
			first, err := stream.Finalize(func(writer io.Writer, _ bool) error {
				terminalWrites.Add(1)
				_, writeErr := io.WriteString(writer, "terminal")
				return writeErr
			})
			results <- first
			errors <- err
		}()
	}
	group.Wait()
	close(results)
	close(errors)

	firstCount := 0
	for first := range results {
		if first {
			firstCount++
		}
	}
	for err := range errors {
		if err != nil {
			t.Fatalf("Finalize: %v", err)
		}
	}
	if firstCount != 1 {
		t.Fatalf("first Finalize count = %d, want 1", firstCount)
	}
	if got := terminalWrites.Load(); got != 1 {
		t.Fatalf("terminal writes = %d, want 1", got)
	}
	if got, want := output.String(), "progress\nterminal"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func waitForPresentationWriteAttempt(t *testing.T, writer *gatedPresentationWriter) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if writer.attempts.Load() > 0 {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("timed out waiting for presentation write")
}

type presentationSource struct {
	cursor factorysessions.ResponseEventCursor
}

func (s presentationSource) SubscribeSessionResponseEventsFromLatest(
	string,
) (factorysessions.ResponseEventCursor, error) {
	return s.cursor, nil
}

type presentationCursor struct {
	batches  chan []factorysessions.FactoryResponseEvent
	detached chan struct{}
	once     sync.Once
}

func newPresentationCursor() *presentationCursor {
	return &presentationCursor{
		batches:  make(chan []factorysessions.FactoryResponseEvent, 4),
		detached: make(chan struct{}),
	}
}

func (c *presentationCursor) Next(ctx context.Context) ([]factorysessions.FactoryResponseEvent, error) {
	select {
	case batch := <-c.batches:
		return batch, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (c *presentationCursor) Drain() ([]factorysessions.FactoryResponseEvent, error) {
	var events []factorysessions.FactoryResponseEvent
	for {
		select {
		case batch := <-c.batches:
			events = append(events, batch...)
		default:
			return events, nil
		}
	}
}

func (c *presentationCursor) Detach() {
	c.once.Do(func() { close(c.detached) })
}

type recordingPresentationSink struct {
	mu     sync.Mutex
	events []factorysessions.FactoryResponseEvent
}

func (s *recordingPresentationSink) PresentResponseEvents(
	events []factorysessions.FactoryResponseEvent,
) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, events...)
}

func (s *recordingPresentationSink) sequences() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var output strings.Builder
	for index, event := range s.events {
		if index > 0 {
			output.WriteByte(' ')
		}
		_, _ = io.WriteString(&output, string(rune('0'+event.Sequence)))
	}
	return output.String()
}

func waitForPresentedEvents(t *testing.T, sink *recordingPresentationSink, count int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		sink.mu.Lock()
		got := len(sink.events)
		sink.mu.Unlock()
		if got >= count {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal(errors.New("timed out waiting for presented response events"))
}
