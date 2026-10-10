package service

import (
	"context"
	"errors"
	"reflect"
	"testing"

	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
)

func TestFleetWorkerSessionAddressCollisionAndSelectedOwner(t *testing.T) {
	t.Parallel()
	left := workersessions.Observation{WorkerSessionID: "legacy", FactorySessionID: "factory-a", WorkIDs: []string{"work-a"}, State: workersessions.StateCompleted}
	right := workersessions.Observation{WorkerSessionID: "legacy", FactorySessionID: "factory-b", WorkIDs: []string{"work-b"}, State: workersessions.StateRunning}
	service := newLegacyFleetFixture(func(context.Context) ([]workersessions.Service, error) {
		return []workersessions.Service{newFleetObservationSource("live-a", left), newFleetObservationSource("captured-a", left), newFleetObservationSource("live-b", right)}, nil
	})
	ctx := context.Background()
	result, err := service.GetObservationByWorkerSessionID(ctx, workersessions.GetObservationByWorkerSessionIDRequest{WorkerSessionID: "legacy"})
	var ambiguous *workersessions.AmbiguousAddressError
	want := (workersessions.AmbiguousAddressError{Candidates: []workersessions.AddressCandidate{left.AddressCandidate(), right.AddressCandidate()}}).Clone()
	if !errors.Is(err, workersessions.ErrWorkerSessionAmbiguous) || !errors.As(err, &ambiguous) || !reflect.DeepEqual(ambiguous.Candidates, want.Candidates) || result.WorkerSessionID != "" {
		t.Fatalf("fleet collision = %+v, %v, candidates=%+v", result, err, ambiguous)
	}
	for _, selected := range []workersessions.Observation{left, right} {
		result, err = service.GetObservationByWorkerSessionID(ctx, workersessions.GetObservationByWorkerSessionIDRequest{WorkerSessionID: "legacy", FactorySessionID: selected.FactorySessionID})
		if err != nil || !reflect.DeepEqual(result, selected) {
			t.Fatalf("selected owner = %+v, %v; want %+v", result, err, selected)
		}
	}
	for _, req := range []workersessions.GetObservationByWorkerSessionIDRequest{{WorkerSessionID: "unknown"}, {WorkerSessionID: "legacy", FactorySessionID: "foreign"}} {
		if _, err := service.GetObservationByWorkerSessionID(ctx, req); !errors.Is(err, workersessions.ErrObservationSessionNotFound) {
			t.Fatalf("unknown/foreign = %v", err)
		}
	}
}

func TestFleetWorkerSessionAddressDeduplicatesOneOwner(t *testing.T) {
	t.Parallel()
	live := workersessions.Observation{WorkerSessionID: "legacy", FactorySessionID: "factory", State: workersessions.StateRunning}
	captured := live
	captured.State = workersessions.StateCompleted
	service := newLegacyFleetFixture(func(context.Context) ([]workersessions.Service, error) {
		return []workersessions.Service{newFleetObservationSource("live", live), newFleetObservationSource("capture", captured)}, nil
	})
	result, err := service.GetObservationByWorkerSessionID(context.Background(), workersessions.GetObservationByWorkerSessionIDRequest{WorkerSessionID: "legacy"})
	if err != nil || !reflect.DeepEqual(result, live) {
		t.Fatalf("duplicate representations = %+v, %v", result, err)
	}
}

func TestFleetWorkerSessionAddressDoesNotSelectBeforeUnavailableSource(t *testing.T) {
	t.Parallel()
	available := newFleetObservationSource("live", workersessions.Observation{WorkerSessionID: "legacy", FactorySessionID: "factory"})
	unavailable := newFleetObservationSource("unavailable")
	unavailable.err = workersessions.ErrObservationProjectionUnavailable
	service := newLegacyFleetFixture(func(context.Context) ([]workersessions.Service, error) {
		return []workersessions.Service{available, unavailable}, nil
	})
	result, err := service.GetObservationByWorkerSessionID(context.Background(), workersessions.GetObservationByWorkerSessionIDRequest{WorkerSessionID: "legacy"})
	if !errors.Is(err, workersessions.ErrObservationProjectionUnavailable) || result.WorkerSessionID != "" {
		t.Fatalf("unproved uniqueness = %+v, %v", result, err)
	}
}

func TestFleetWorkerSessionAddressCombinesAmbiguousSourceWithPeer(t *testing.T) {
	t.Parallel()
	first := workersessions.Observation{WorkerSessionID: "legacy", FactorySessionID: "factory-a", State: workersessions.StateRunning}
	second := workersessions.Observation{WorkerSessionID: "legacy", FactorySessionID: "factory-b", State: workersessions.StateCompleted}
	third := workersessions.Observation{WorkerSessionID: "legacy", FactorySessionID: "factory-c", State: workersessions.StatePaused}
	ambiguousSource := newFleetObservationSource("colliding-registry")
	ambiguousSource.err = (workersessions.AmbiguousAddressError{Candidates: []workersessions.AddressCandidate{first.AddressCandidate(), second.AddressCandidate()}}).Clone()
	service := newLegacyFleetFixture(func(context.Context) ([]workersessions.Service, error) {
		return []workersessions.Service{ambiguousSource, newFleetObservationSource("duplicate", first), newFleetObservationSource("peer", third)}, nil
	})
	result, err := service.GetObservationByWorkerSessionID(context.Background(), workersessions.GetObservationByWorkerSessionIDRequest{WorkerSessionID: "legacy"})
	var ambiguous *workersessions.AmbiguousAddressError
	want := (workersessions.AmbiguousAddressError{Candidates: []workersessions.AddressCandidate{first.AddressCandidate(), second.AddressCandidate(), third.AddressCandidate()}}).Clone()
	if !errors.As(err, &ambiguous) || !reflect.DeepEqual(ambiguous.Candidates, want.Candidates) || result.WorkerSessionID != "" {
		t.Fatalf("complete fleet candidate set = %+v, %v, candidates=%+v", result, err, ambiguous)
	}
}

func TestFleetObservationServiceRoutesTranscriptAndEventsToOwningSource(t *testing.T) {
	t.Parallel()
	wanted := workersessions.Observation{WorkerSessionID: "worker-fleet-2", FactorySessionID: "factory-2"}
	service := newLegacyFleetFixture(func(context.Context) ([]workersessions.Service, error) {
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
