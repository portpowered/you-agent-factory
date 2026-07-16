package dashboard

import (
	"context"
	"time"

	factoryeventprojection "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryeventprojection"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	"github.com/portpowered/infinite-you/pkg/factory/projections"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/transports/cli/dashboardrender"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// RuntimeDashboardReads is the bounded read surface needed to project a
// dashboard view from the active runtime.
type RuntimeDashboardReads interface {
	GetEngineStateSnapshot(context.Context) (*interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], error)
	GetFactoryEvents(context.Context) ([]factoryapi.FactoryEvent, error)
}

// NewRuntimeDashboardReader adapts runtime reads without exposing mutation or
// lifecycle operations to the dashboard sidecar.
func NewRuntimeDashboardReader(reads RuntimeDashboardReads) DashboardReader {
	return runtimeDashboardReader{reads: reads}
}

type runtimeDashboardReader struct {
	reads RuntimeDashboardReads
}

func (r runtimeDashboardReader) ReadDashboard(ctx context.Context, now time.Time) (DashboardRenderInput, error) {
	es, err := r.reads.GetEngineStateSnapshot(ctx)
	if err != nil {
		return DashboardRenderInput{}, err
	}
	events, err := r.reads.GetFactoryEvents(ctx)
	if err != nil {
		return DashboardRenderInput{}, err
	}
	worldState, err := factoryeventprojection.ReconstructFactoryWorldState(events, es.TickCount)
	if err != nil {
		return DashboardRenderInput{}, err
	}
	renderData := dashboardrender.SimpleDashboardRenderDataFromWorldState(worldState)
	renderData.ActiveThrottlePauses = projections.ProjectActiveThrottlePauses(worldState.Topology, es.ActiveThrottlePauses)
	return DashboardRenderInput{EngineState: *es, RenderData: renderData, Now: now}, nil
}

// DashboardRendererFunc adapts an existing render callback to the bounded
// renderer dependency.
type DashboardRendererFunc func(DashboardRenderInput)

func (f DashboardRendererFunc) RenderDashboard(input DashboardRenderInput) { f(input) }

// ClockTiming preserves the configured runtime clock while using real
// cancellable tickers for process scheduling.
type ClockTiming struct{ Clock interface{ Now() time.Time } }

func (t ClockTiming) Now() time.Time { return t.Clock.Now() }

func (ClockTiming) NewTicker(interval time.Duration) DashboardTicker {
	return realDashboardTicker{Ticker: time.NewTicker(interval)}
}
