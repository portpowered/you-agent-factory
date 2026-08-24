package wire

import (
	"context"
	"errors"
	"testing"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factorysessionwire "github.com/portpowered/infinite-you/pkg/services/factory_sessions/wire"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"go.uber.org/zap"
)

type fleetWorkerSessionsServiceStub struct {
	workersessions.Service
}

type fleetFactorySessionsServiceStub struct {
	factorysessions.Service
	projections  []factorysessions.ReadProjection
	err          error
	observations map[string]workersessions.Service
}

func (stub *fleetFactorySessionsServiceStub) ListFactorySessions(context.Context) ([]factorysessions.ReadProjection, error) {
	return stub.projections, stub.err
}

func (stub *fleetFactorySessionsServiceStub) WorkerSessionsObservationForSession(sessionID string) workersessions.Service {
	return stub.observations[sessionID]
}

type fleetFactoryListingServiceStub struct {
	factorysessions.Service
	projections []factorysessions.ReadProjection
}

func (stub *fleetFactoryListingServiceStub) ListFactorySessions(context.Context) ([]factorysessions.ReadProjection, error) {
	return stub.projections, nil
}

type fleetWorkServiceStub struct {
	work.Service
}

func fleetReadProjection(factorySessionID, liveSessionID string) factorysessions.ReadProjection {
	return factorysessions.ReadProjection{
		Context: factorysessions.ProjectionContext{
			FactorySessionID: factorySessionID,
			Session:          &factorysessions.ScopedLiveSessionSummary{ID: liveSessionID},
		},
	}
}

func TestWorkerSessionObservationSourcesMergeLiveFactorySessionsDeterministically(t *testing.T) {
	t.Parallel()

	root := &fleetWorkerSessionsServiceStub{}
	a, b, z := &fleetWorkerSessionsServiceStub{}, &fleetWorkerSessionsServiceStub{}, &fleetWorkerSessionsServiceStub{}
	factoryRoot := &fleetFactorySessionsServiceStub{
		projections: []factorysessions.ReadProjection{
			fleetReadProjection(" z ", "ignored"),
			fleetReadProjection("", " a "),
			fleetReadProjection("z", "duplicate"),
			fleetReadProjection("", ""),
			fleetReadProjection("b", "b-live"),
		},
		observations: map[string]workersessions.Service{"a": a, "b": b, "z": z},
	}

	sources, err := workerSessionObservationSources(context.Background(), factorysessionwire.OpenedApplicationRuntime{
		FactorySessions: factoryRoot,
		WorkerSessions:  root,
	})
	if err != nil {
		t.Fatalf("workerSessionObservationSources() error = %v", err)
	}
	if len(sources) != 4 {
		t.Fatalf("sources = %d, want process root plus three session observations", len(sources))
	}
	if sources[0] != root || sources[1] != a || sources[2] != b || sources[3] != z {
		t.Fatalf("sources = %#v, want root followed by sorted a, b, z observations", sources)
	}
}

func TestWorkerSessionObservationSourcesHandlesUnavailableAndUnadornedRoots(t *testing.T) {
	t.Parallel()

	root := &fleetWorkerSessionsServiceStub{}
	sources, err := workerSessionObservationSources(context.Background(), factorysessionwire.OpenedApplicationRuntime{
		WorkerSessions: root,
	})
	if err != nil || len(sources) != 1 || sources[0] != root {
		t.Fatalf("without Factory Sessions = sources:%#v err:%v, want only process root", sources, err)
	}

	noRootSources, err := workerSessionObservationSources(context.Background(), factorysessionwire.OpenedApplicationRuntime{})
	if err != nil || len(noRootSources) != 0 {
		t.Fatalf("without opened services = sources:%#v err:%v, want empty source set", noRootSources, err)
	}

	listingOnly := &fleetFactoryListingServiceStub{
		projections: []factorysessions.ReadProjection{fleetReadProjection("session-a", "")},
	}
	sources, err = workerSessionObservationSources(context.Background(), factorysessionwire.OpenedApplicationRuntime{
		FactorySessions: listingOnly,
		WorkerSessions:  root,
	})
	if err != nil || len(sources) != 1 || sources[0] != root {
		t.Fatalf("without optional provider = sources:%#v err:%v, want only process root", sources, err)
	}
}

func TestWorkerSessionObservationSourcesPropagatesFactorySessionListingFailure(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("list live sessions failed")
	_, err := workerSessionObservationSources(context.Background(), factorysessionwire.OpenedApplicationRuntime{
		FactorySessions: &fleetFactorySessionsServiceStub{err: wantErr},
		WorkerSessions:  &fleetWorkerSessionsServiceStub{},
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("workerSessionObservationSources() error = %v, want %v", err, wantErr)
	}
}

func TestNewHTTPWorkerSessionsHandlerBindsFleetObservationView(t *testing.T) {
	t.Parallel()

	handler := newHTTPWorkerSessionsHandler(factorysessionwire.OpenedApplicationRuntime{
		WorkerSessions: &fleetWorkerSessionsServiceStub{},
		Work:           &fleetWorkServiceStub{},
		Logger:         zap.NewNop(),
	})
	if handler == nil {
		t.Fatal("newHTTPWorkerSessionsHandler() = nil, want bound Worker Sessions handler")
	}
}
