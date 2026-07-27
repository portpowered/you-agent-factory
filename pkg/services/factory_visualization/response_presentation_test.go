package factory_visualization

import (
	"bytes"
	"io"
	"runtime"
	"testing"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func TestNewResponsePresentationIsInert(t *testing.T) {
	t.Parallel()

	before := runtime.NumGoroutine()
	presentation := NewResponsePresentation()
	if presentation == nil {
		t.Fatal("NewResponsePresentation() returned nil")
	}
	time.Sleep(25 * time.Millisecond)
	if after := runtime.NumGoroutine(); after > before+2 {
		t.Fatalf("presentation construction started goroutines: before=%d after=%d", before, after)
	}
}

func TestResponsePresentation_FacadeOpensUsableOutputs(t *testing.T) {
	t.Parallel()

	presentation := NewResponsePresentation()
	for _, mode := range []struct {
		name string
		open func(ResponsePresentation, io.Writer) Output
	}{
		{name: "best-effort", open: func(p ResponsePresentation, writer io.Writer) Output {
			return p.OpenBestEffortOutput(writer)
		}},
		{name: "lossless", open: func(p ResponsePresentation, writer io.Writer) Output {
			return p.OpenLosslessOutput(writer)
		}},
	} {
		t.Run(mode.name, func(t *testing.T) {
			var output bytes.Buffer
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
		})
	}
}

func TestResponsePresentation_FacadeOpensUsableFactoryEventStreams(t *testing.T) {
	t.Parallel()

	presentation := NewResponsePresentation()
	var output bytes.Buffer
	stream := presentation.OpenLosslessFactoryEventStream(&output, func(event factorydefinitions.FactoryEvent) ([]byte, bool) {
		return []byte(event.Id), true
	})
	stream.PresentFactoryEvents([]factorydefinitions.FactoryEvent{{Id: "event"}})
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
	if got, want := output.String(), "event\nterminal\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
	if finalizedAgain, err := stream.Finalize(nil); err != nil || finalizedAgain {
		t.Fatalf("second Finalize() = (%v, %v), want (false, nil)", finalizedAgain, err)
	}
	stream.PresentFactoryEvents([]factorydefinitions.FactoryEvent{{Id: "late"}})
	if err := stream.CloseAndDrain(); err != nil {
		t.Fatalf("CloseAndDrain after late Present: %v", err)
	}
	if got, want := output.String(), "event\nterminal\n"; got != want {
		t.Fatalf("late Present after Finalize changed output = %q, want %q", got, want)
	}
}
