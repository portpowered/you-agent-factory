package service_test

import (
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/testing/recordingsstub"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

func factoryEventsFromCanonical(events []recordings.CanonicalEvent) []factorydefinitions.FactoryEvent {
	mapped := make([]factorydefinitions.FactoryEvent, len(events))
	for index, event := range events {
		mapped[index] = factorydefinitions.FactoryEvent{
			Id: string(event.ID),
			Context: factorydefinitions.FactoryEventContext{
				Sequence: int(event.Sequence),
			},
		}
	}
	return mapped
}

func newProjectionStub() *recordingsstub.Service {
	return &recordingsstub.Service{}
}

func newProjectionStubWithDashboard(data recordings.SimpleDashboardRenderData) *recordingsstub.Service {
	return &recordingsstub.Service{DashboardData: data}
}

func newTrackingProjectionStub(projected chan<- []factorydefinitions.FactoryEvent) *recordingsstub.Service {
	return &recordingsstub.Service{
		ReconstructWorldStateFn: func(request recordings.ReconstructWorldStateRequest) (recordings.ReconstructWorldStateResult, error) {
			events := factoryEventsFromCanonical(request.Events)
			projected <- append([]factorydefinitions.FactoryEvent(nil), events...)
			return recordings.ReconstructWorldStateResult{
				WorldState: recordings.WorldStateView{
					SchemaVersion: recordings.WorldStateViewSchemaV1,
					Payload:       `{"topology":{}}`,
				},
			}, nil
		},
	}
}

func newFailingProjectionStub(err error) *recordingsstub.Service {
	return &recordingsstub.Service{
		ReconstructWorldStateFn: func(recordings.ReconstructWorldStateRequest) (recordings.ReconstructWorldStateResult, error) {
			return recordings.ReconstructWorldStateResult{}, err
		},
	}
}

// cursorContinuityProjection records each reconstruction call so continuity
// tests can assert monotonic event growth without duplicate retained replay.
type cursorContinuityProjection struct {
	*recordingsstub.Service
	reconstructCalls [][]factorydefinitions.FactoryEvent
}

func newCursorContinuityProjection() *cursorContinuityProjection {
	projection := &cursorContinuityProjection{Service: &recordingsstub.Service{}}
	projection.ReconstructWorldStateFn = func(request recordings.ReconstructWorldStateRequest) (recordings.ReconstructWorldStateResult, error) {
		events := factoryEventsFromCanonical(request.Events)
		projection.reconstructCalls = append(
			projection.reconstructCalls,
			append([]factorydefinitions.FactoryEvent(nil), events...),
		)
		return recordings.ReconstructWorldStateResult{
			WorldState: recordings.WorldStateView{
				SchemaVersion: recordings.WorldStateViewSchemaV1,
				Payload:       `{"topology":{}}`,
			},
		}, nil
	}
	return projection
}
