package wire

import (
	"context"
	"testing"

	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
)

func TestFleetObservationServiceRoutesTranscriptAndEventsToOwningSource(t *testing.T) {
	t.Parallel()
	wanted := workersessions.Observation{WorkerSessionID: "worker-fleet-2", FactorySessionID: "factory-2"}
	service := NewFleetObservationService(func(context.Context) ([]workersessions.Service, error) {
		return []workersessions.Service{
			newFleetObservationSource("first", workersessions.Observation{WorkerSessionID: "worker-fleet-1"}),
			newFleetObservationSource("second", wanted),
		}, nil
	})

	transcript, err := service.ReadTranscriptByWorkerSessionID(context.Background(), workersessions.ReadTranscriptByWorkerSessionIDRequest{WorkerSessionID: wanted.WorkerSessionID})
	if err != nil || transcript.WorkerSessionID != wanted.WorkerSessionID {
		t.Fatalf("ReadTranscriptByWorkerSessionID() = %#v, %v", transcript, err)
	}
	subscription, err := service.StreamObservationsByWorkerSessionID(context.Background(), workersessions.StreamObservationsByWorkerSessionIDRequest{WorkerSessionID: wanted.WorkerSessionID, ReplayOnly: true})
	if err != nil {
		t.Fatalf("StreamObservationsByWorkerSessionID() error = %v", err)
	}
	delivery := subscription.Next(context.Background())
	if delivery.Event.Cursor.WorkerSessionID != wanted.WorkerSessionID {
		t.Fatalf("stream cursor WorkerSessionID = %q, want %q", delivery.Event.Cursor.WorkerSessionID, wanted.WorkerSessionID)
	}
}

func (source *fleetObservationSource) ReadTranscriptByWorkerSessionID(ctx context.Context, request workersessions.ReadTranscriptByWorkerSessionIDRequest) (workersessions.ReadTranscriptResult, error) {
	if err := request.Validate(); err != nil {
		return workersessions.ReadTranscriptResult{}, err
	}
	for _, observation := range source.inventory {
		if observation.WorkerSessionID == request.WorkerSessionID {
			return workersessions.ReadTranscriptResult{WorkerSessionID: request.WorkerSessionID}, nil
		}
	}
	return workersessions.ReadTranscriptResult{}, workersessions.ErrObservationSessionNotFound
}

func (source *fleetObservationSource) StreamObservationsByWorkerSessionID(ctx context.Context, request workersessions.StreamObservationsByWorkerSessionIDRequest) (workersessions.ObservationSubscription, error) {
	if err := request.Validate(); err != nil {
		return workersessions.ObservationSubscription{}, err
	}
	for _, observation := range source.inventory {
		if observation.WorkerSessionID == request.WorkerSessionID {
			return workersessions.ObservationSubscription{NextFunc: func(context.Context) workersessions.ObservationDelivery {
				return workersessions.ObservationDelivery{Kind: workersessions.ObservationDeliveryRecord, Event: workersessions.ObservationEvent{Cursor: workersessions.ObservationCursor{WorkerSessionID: request.WorkerSessionID}}}
			}}, nil
		}
	}
	return workersessions.ObservationSubscription{}, workersessions.ErrObservationSessionNotFound
}
