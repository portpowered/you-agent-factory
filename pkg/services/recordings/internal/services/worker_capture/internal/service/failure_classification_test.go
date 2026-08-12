package service

import (
	"context"
	"errors"
	"testing"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

type failureClassificationWriter struct {
	failure recordings.WorkerRecordingFailure
}

func (writer *failureClassificationWriter) PersistWorkerRecord(context.Context, recordings.WorkerRecordingRecord) error {
	return nil
}

func (writer *failureClassificationWriter) PersistWorkerRecordingFailure(
	_ context.Context,
	failure recordings.WorkerRecordingFailure,
) error {
	writer.failure = failure
	return nil
}

func TestWorkerRecordingFailurePersistsStableClassification(t *testing.T) {
	tests := []struct {
		name string
		err  error
		code string
	}{
		{name: "opening", err: recordings.ErrWorkerRecordingOpening, code: "OPENING_INVALID"},
		{name: "persistence", err: recordings.ErrWorkerRecordingPersistence, code: "PERSISTENCE_FAILED"},
		{name: "gap", err: recordings.ErrWorkerRecordingGap, code: "RETENTION_GAP"},
		{name: "closed", err: recordings.ErrWorkerRecordingClosed, code: "SOURCE_CLOSED"},
		{name: "canceled", err: recordings.ErrWorkerRecordingCanceled, code: "CANCELED"},
		{name: "backpressure", err: recordings.ErrWorkerRecordingBackpressure, code: "BACKPRESSURE"},
		{name: "terminal", err: recordings.ErrWorkerRecordingTerminal, code: "TERMINAL_INVALID"},
		{name: "incomplete", err: recordings.ErrWorkerRecordingIncomplete, code: "INCOMPLETE"},
		{name: "duplicate", err: recordings.ErrWorkerRecordingDuplicate, code: "DUPLICATE_CONFLICT"},
		{name: "order", err: recordings.ErrWorkerRecordingOrder, code: "ORDER_INVALID"},
		{name: "delivery", err: errors.New("unexpected delivery"), code: "DELIVERY_FAILED"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			writer := &failureClassificationWriter{}
			capture := &capture{
				request: recordings.WorkerSessionRecordingRequest{
					RecordingID:     "recording-classification",
					WorkerSessionID: "worker-classification",
				},
				writer:  writer,
				logger:  logging.NoopLogger{},
				failure: make(chan struct{}),
				stop:    func() {},
			}

			capture.fail(test.err)

			if !errors.Is(capture.failureError(), test.err) {
				t.Fatalf("failure = %v, want %v", capture.failureError(), test.err)
			}
			if writer.failure.Code != test.code {
				t.Fatalf("persisted failure code = %q, want %q", writer.failure.Code, test.code)
			}
			if writer.failure.RecordingID != "recording-classification" || writer.failure.WorkerSessionID != "worker-classification" {
				t.Fatalf("persisted failure identity = %#v, want recording and Worker Session identity", writer.failure)
			}
		})
	}
}
