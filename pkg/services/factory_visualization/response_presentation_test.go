package factory_visualization

import (
	"bytes"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
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

func TestResponsePresentation_FactoryEventStreamsDrainEventsBeforeTerminal(t *testing.T) {
	t.Parallel()

	for _, mode := range []struct {
		name string
		open func(ResponsePresentation, io.Writer, FactoryEventEncoder) interface {
			PresentFactoryEvents([]factorydefinitions.FactoryEvent)
			Finalize(FinalResponseWriter) (bool, error)
			CloseAndDrain() error
		}
	}{
		{name: "best-effort", open: func(presentation ResponsePresentation, writer io.Writer, encode FactoryEventEncoder) interface {
			PresentFactoryEvents([]factorydefinitions.FactoryEvent)
			Finalize(FinalResponseWriter) (bool, error)
			CloseAndDrain() error
		} {
			return presentation.OpenBestEffortFactoryEventStream(writer, encode)
		}},
		{name: "lossless", open: func(presentation ResponsePresentation, writer io.Writer, encode FactoryEventEncoder) interface {
			PresentFactoryEvents([]factorydefinitions.FactoryEvent)
			Finalize(FinalResponseWriter) (bool, error)
			CloseAndDrain() error
		} {
			return presentation.OpenLosslessFactoryEventStream(writer, encode)
		}},
	} {
		t.Run(mode.name, func(t *testing.T) {
			var output bytes.Buffer
			presentation := NewResponsePresentation()
			stream := mode.open(presentation, &output, func(event factorydefinitions.FactoryEvent) ([]byte, bool) {
				if event.Id == "omit" {
					return nil, false
				}
				return []byte(event.Id), true
			})
			stream.PresentFactoryEvents([]factorydefinitions.FactoryEvent{
				{Id: "first"}, {Id: "omit"}, {Id: "second"},
			})
			finalized, err := stream.Finalize(func(writer io.Writer, progressSeen bool) error {
				if !progressSeen {
					t.Fatal("terminal writer did not observe accepted Factory Events")
				}
				_, writeErr := io.WriteString(writer, "terminal\n")
				return writeErr
			})
			if err != nil || !finalized {
				t.Fatalf("Finalize() = (%v, %v), want (true, nil)", finalized, err)
			}
			if got, want := output.String(), "first\nsecond\nterminal\n"; got != want {
				t.Fatalf("output = %q, want %q", got, want)
			}
			if finalizedAgain, err := stream.Finalize(nil); err != nil || finalizedAgain {
				t.Fatalf("second Finalize() = (%v, %v), want (false, nil)", finalizedAgain, err)
			}
			stream.PresentFactoryEvents([]factorydefinitions.FactoryEvent{{Id: "late"}})
			if err := stream.CloseAndDrain(); err != nil {
				t.Fatalf("CloseAndDrain: %v", err)
			}
		})
	}
}

func TestResponsePresentation_PublicOutputsProvideExclusiveTerminalWrite(t *testing.T) {
	t.Parallel()

	for _, mode := range []struct {
		name string
		open func(ResponsePresentation, io.Writer) Output
	}{
		{name: "best-effort", open: func(presentation ResponsePresentation, writer io.Writer) Output {
			return presentation.OpenBestEffortOutput(writer)
		}},
		{name: "lossless", open: func(presentation ResponsePresentation, writer io.Writer) Output {
			return presentation.OpenLosslessOutput(writer)
		}},
	} {
		t.Run(mode.name, func(t *testing.T) {
			var output bytes.Buffer
			presentation := NewResponsePresentation()
			presentationOutput := mode.open(presentation, &output)
			if err := presentationOutput.Enqueue([]byte("event")); err != nil {
				t.Fatalf("Enqueue: %v", err)
			}
			if err := presentationOutput.CloseAndDrain(); err != nil {
				t.Fatalf("CloseAndDrain: %v", err)
			}
			if err := presentationOutput.WithWriterExclusive(func(writer io.Writer) error {
				_, writeErr := io.WriteString(writer, "terminal\n")
				return writeErr
			}); err != nil {
				t.Fatalf("WithWriterExclusive: %v", err)
			}
			if got, want := output.String(), "event\nterminal\n"; got != want {
				t.Fatalf("output = %q, want %q", got, want)
			}
			if got := presentationOutput.Dropped(); got != 0 {
				t.Fatalf("Dropped = %d, want 0", got)
			}
		})
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
