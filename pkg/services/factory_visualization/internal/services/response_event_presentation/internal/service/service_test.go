package service_test

import (
	"bytes"
	"io"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/contracts"
	presentationservice "github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/services/response_event_presentation/internal/service"
	responseeventpresentationwire "github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/services/response_event_presentation/wire"
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

func TestNewServiceIsInert(t *testing.T) {
	t.Parallel()

	before := runtime.NumGoroutine()
	service := responseeventpresentationwire.NewService()
	if service == nil {
		t.Fatal("NewService() returned nil")
	}
	time.Sleep(25 * time.Millisecond)
	if after := runtime.NumGoroutine(); after > before+2 {
		t.Fatalf("service construction started goroutines: before=%d after=%d", before, after)
	}
}

func TestResponsePresentation_BestEffortOutputDoesNotBlockAndBoundsBacklog(t *testing.T) {
	t.Parallel()

	service := presentationservice.New()
	writer := newGatedPresentationWriter()
	output := service.OpenBestEffortOutput(writer)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for index := 0; index < contracts.DefaultProgressQueueCapacity+16; index++ {
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

	service := presentationservice.New()
	writer := newGatedPresentationWriter()
	output := service.OpenLosslessOutput(writer)
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

	service := presentationservice.New()
	writer := newGatedPresentationWriter()
	output := service.OpenBestEffortOutput(writer)
	if err := output.Enqueue([]byte("progress")); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	waitForPresentationWriteAttempt(t, writer)
	started := time.Now()
	if err := output.CloseAndDrain(); err != nil {
		t.Fatalf("CloseAndDrain: %v", err)
	}
	if elapsed := time.Since(started); elapsed > 500*time.Millisecond {
		t.Fatalf("best-effort drain took %s, want bounded completion", elapsed)
	}
	writer.release()
}

func TestResponsePresentation_FactoryEventStreamsDrainEventsBeforeTerminal(t *testing.T) {
	t.Parallel()

	service := presentationservice.New()
	for _, mode := range []struct {
		name string
		open func(io.Writer, contracts.FactoryEventEncoder) contracts.FactoryEventStream
	}{
		{
			name: "best-effort",
			open: service.OpenBestEffortFactoryEventStream,
		},
		{
			name: "lossless",
			open: service.OpenLosslessFactoryEventStream,
		},
	} {
		t.Run(mode.name, func(t *testing.T) {
			var output bytes.Buffer
			stream := mode.open(&output, func(event factorydefinitions.FactoryEvent) ([]byte, bool) {
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

func TestResponsePresentation_OutputsProvideExclusiveTerminalWrite(t *testing.T) {
	t.Parallel()

	service := presentationservice.New()
	for _, mode := range []struct {
		name string
		open func(io.Writer) contracts.Output
	}{
		{name: "best-effort", open: service.OpenBestEffortOutput},
		{name: "lossless", open: service.OpenLosslessOutput},
	} {
		t.Run(mode.name, func(t *testing.T) {
			var output bytes.Buffer
			presentationOutput := mode.open(&output)
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
