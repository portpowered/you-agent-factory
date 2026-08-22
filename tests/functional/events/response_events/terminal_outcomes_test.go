package response_events

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

type responseEventFrameReader interface {
	TryNextFrame(time.Duration) (support.FactoryResponseEventFrame, bool)
}

var errResponseEventStreamTerminalTimeout = errors.New(
	"timed out waiting for terminal RUN Response Event",
)

type responseEventTerminalOutcome uint8

const (
	responseEventNotTerminal responseEventTerminalOutcome = iota
	responseEventCompleted
	responseEventFailed
)

func responseEventTerminalOutcomeFor(
	event factoryapi.FactoryResponseEvent,
) responseEventTerminalOutcome {
	if event.Kind == factoryapi.FactoryResponseEventKindRun {
		switch event.Phase {
		case factoryapi.FactoryResponseEventPhaseCompleted:
			return responseEventCompleted
		case factoryapi.FactoryResponseEventPhaseFailed,
			factoryapi.FactoryResponseEventPhaseCanceled:
			return responseEventFailed
		}
	}
	if event.Kind == factoryapi.FactoryResponseEventKindError &&
		(event.Phase == factoryapi.FactoryResponseEventPhaseFailed ||
			event.Phase == factoryapi.FactoryResponseEventPhaseCanceled) {
		return responseEventFailed
	}
	return responseEventNotTerminal
}

func collectResponseEventStreamUntilTerminalRun(
	t *testing.T,
	stream responseEventFrameReader,
	afterSequence int64,
	timeout time.Duration,
) []support.FactoryResponseEventFrame {
	t.Helper()
	collected, err := readResponseEventStreamUntilTerminalRun(
		stream,
		afterSequence,
		timeout,
	)
	if err != nil {
		t.Fatal(err)
	}
	return collected
}

func readResponseEventStreamUntilTerminalRun(
	stream responseEventFrameReader,
	afterSequence int64,
	timeout time.Duration,
) ([]support.FactoryResponseEventFrame, error) {
	// RUN/COMPLETED is the producer-defined terminal boundary for one
	// invocation; an idle interval would only measure scheduler batching.
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	var collected []support.FactoryResponseEventFrame
	for {
		select {
		case <-deadline.C:
			return collected, fmt.Errorf(
				"%w after sequence %d: got %d frames within %s",
				errResponseEventStreamTerminalTimeout,
				afterSequence,
				len(collected),
				timeout,
			)
		default:
			frame, ok := stream.TryNextFrame(50 * time.Millisecond)

			if !ok {
				continue
			}
			if frame.Event.Sequence <= afterSequence {
				return collected, fmt.Errorf(
					"live continuation frame sequence %d is not after acknowledged sequence %d",
					frame.Event.Sequence,
					afterSequence,
				)
			}
			collected = append(collected, frame)
			switch responseEventTerminalOutcomeFor(frame.Event) {
			case responseEventCompleted:
				return collected, nil
			case responseEventFailed:
				return collected, fmt.Errorf(
					"terminal %s Response Event phase = %q after sequence %d, want COMPLETED",
					frame.Event.Kind,
					frame.Event.Phase,
					afterSequence,
				)
			}
		}
	}
}

func TestReadResponseEventStreamUntilTerminalRunOutcomes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		frames            []support.FactoryResponseEventFrame
		wantTerminalError string
		wantTimeout       bool
		wantFrameCount    int
	}{
		{
			name: "completed run",
			frames: []support.FactoryResponseEventFrame{
				responseEventFrame(11, factoryapi.FactoryResponseEventKindProgress, factoryapi.FactoryResponseEventPhaseUpdated),
				responseEventFrame(12, factoryapi.FactoryResponseEventKindRun, factoryapi.FactoryResponseEventPhaseCompleted),
			},
			wantFrameCount: 2,
		},
		{
			name: "failed run",
			frames: []support.FactoryResponseEventFrame{
				responseEventFrame(11, factoryapi.FactoryResponseEventKindRun, factoryapi.FactoryResponseEventPhaseFailed),
			},
			wantTerminalError: "terminal RUN Response Event phase = \"FAILED\"",
			wantFrameCount:    1,
		},
		{
			name: "canceled run",
			frames: []support.FactoryResponseEventFrame{
				responseEventFrame(11, factoryapi.FactoryResponseEventKindRun, factoryapi.FactoryResponseEventPhaseCanceled),
			},
			wantTerminalError: "terminal RUN Response Event phase = \"CANCELED\"",
			wantFrameCount:    1,
		},
		{
			name: "failed error",
			frames: []support.FactoryResponseEventFrame{
				responseEventFrame(11, factoryapi.FactoryResponseEventKindError, factoryapi.FactoryResponseEventPhaseFailed),
			},
			wantTerminalError: "terminal ERROR Response Event phase = \"FAILED\"",
			wantFrameCount:    1,
		},
		{
			name: "canceled error",
			frames: []support.FactoryResponseEventFrame{
				responseEventFrame(11, factoryapi.FactoryResponseEventKindError, factoryapi.FactoryResponseEventPhaseCanceled),
			},
			wantTerminalError: "terminal ERROR Response Event phase = \"CANCELED\"",
			wantFrameCount:    1,
		},
		{
			name: "timeout without terminal run",
			frames: []support.FactoryResponseEventFrame{
				responseEventFrame(11, factoryapi.FactoryResponseEventKindProgress, factoryapi.FactoryResponseEventPhaseUpdated),
			},
			wantTimeout:    true,
			wantFrameCount: 1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reader := newScriptedResponseEventFrameReader(test.frames)
			got, err := readResponseEventStreamUntilTerminalRun(reader, 10, 5*time.Millisecond)
			if len(got) != test.wantFrameCount {
				t.Fatalf("collected frame count = %d, want %d", len(got), test.wantFrameCount)
			}
			switch {
			case test.wantTimeout:
				if !errors.Is(err, errResponseEventStreamTerminalTimeout) {
					t.Fatalf("error = %v, want terminal timeout", err)
				}
			case test.wantTerminalError != "":
				if err == nil || !strings.Contains(err.Error(), test.wantTerminalError) {
					t.Fatalf("error = %v, want substring %q", err, test.wantTerminalError)
				}
			default:
				if err != nil {
					t.Fatalf("read terminal response events: %v", err)
				}
			}
		})
	}
}

func responseEventFrame(
	sequence int64,
	kind factoryapi.FactoryResponseEventKind,
	phase factoryapi.FactoryResponseEventPhase,
) support.FactoryResponseEventFrame {
	return support.FactoryResponseEventFrame{
		SSEID: fmt.Sprint(sequence),
		Event: factoryapi.FactoryResponseEvent{
			EventId:  fmt.Sprintf("response-event-%d", sequence),
			Kind:     kind,
			Phase:    phase,
			Sequence: sequence,
		},
	}
}

type scriptedResponseEventFrameReader struct {
	frames []support.FactoryResponseEventFrame
	index  int
}

func newScriptedResponseEventFrameReader(
	frames []support.FactoryResponseEventFrame,
) *scriptedResponseEventFrameReader {
	return &scriptedResponseEventFrameReader{frames: frames}
}

func (r *scriptedResponseEventFrameReader) TryNextFrame(
	timeout time.Duration,
) (support.FactoryResponseEventFrame, bool) {
	if r.index < len(r.frames) {
		frame := r.frames[r.index]
		r.index++
		return frame, true
	}
	if timeout <= 0 {
		return support.FactoryResponseEventFrame{}, false
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	<-timer.C
	return support.FactoryResponseEventFrame{}, false
}
