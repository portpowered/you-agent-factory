// Package http owns HTTP adaptation for Recordings operations.
//
// The top-level HTTP transport registers generated routes and composes this
// adapter when PSS fan-in arrives. Request decoding, generated contract mapping,
// Recordings root invocation, error mapping, and streaming policy for
// Recordings-owned HTTP operations remain here with the owning service.
package http

import (
	"context"
	"errors"
	"reflect"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// Adapter maps Recordings service values at the outward HTTP boundary.
type Adapter struct {
	root              recordings.Service
	legacyHistory     LegacyHistory
	legacyPreparation LegacyRequestPreparation
	legacyLiveEvents  LegacyLiveEvents
}

// LegacyHistory is the narrow compatibility seam used only by standalone
// durable-execution hosts while they do not open a process Recordings root.
// Production runtime composition uses NewAdapter and never supplies this
// bridge; the generated route shell still has one Recordings HTTP owner.
type LegacyHistory interface {
	GetDurableFactorySessionResult(context.Context, string, factorysessions.ResultRequest) (factoryapi.FactorySessionResult, error)
	ReadDurableFactorySessionEvents(context.Context, string, factorysessions.EventReconnectRequest) (*interfaces.FactoryEventStream, error)
	ProbeDurableFactorySessionEvents(context.Context, string, factorysessions.EventReconnectRequest) error
	ListDurableFactorySessionDispatches(context.Context, string, factoryapi.ListFactorySessionDispatchesParams) (factoryapi.ListFactorySessionDispatchesResponse, error)
	GetDurableFactorySessionDispatch(context.Context, string, string) (factoryapi.FactoryDispatch, error)
	ListDurableFactorySessionArtifacts(context.Context, string) (factoryapi.ListFactorySessionArtifactsResponse, error)
	GetDurableFactorySessionArtifact(context.Context, string, string) (factoryapi.FactorySessionArtifactDetail, error)
}

// LegacyRequestPreparation preserves the request normalization performed by
// the old standalone durable binding. It is intentionally not part of the
// Recordings Service contract and is only accepted by NewLegacyAdapter.
type LegacyRequestPreparation interface {
	PrepareResult(factorysessions.ResultRequest) (factorysessions.ResultRequest, error)
	PrepareEventReconnect(factorysessions.EventReconnectRequest) (factorysessions.EventReconnectRequest, error)
}

// LegacyLiveEvents is the narrow compatibility seam for older focused HTTP
// fakes that have not yet supplied a process Recordings ledger.
type LegacyLiveEvents interface {
	SubscribeFactoryEventsForSession(context.Context, string, *interfaces.FactoryEventReconnectCursor) (*interfaces.FactoryEventStream, error)
	ProbeFactoryEventsForSession(context.Context, string, *interfaces.FactoryEventReconnectCursor) error
}

// NewAdapter constructs the Recordings HTTP representation adapter.
func NewAdapter(root recordings.Service) *Adapter {
	if root == nil {
		return nil
	}
	return &Adapter{root: root}
}

// NewAdapterWithLegacyFallback binds the finalized Recordings path together
// with a temporary live-session compatibility seam. Recordings remains the
// first read for finalized sessions; the legacy capability is consulted only
// when the owner reports that no portable artifact is available yet.
func NewAdapterWithLegacyFallback(
	root recordings.Service,
	history LegacyHistory,
	preparation LegacyRequestPreparation,
	live LegacyLiveEvents,
) *Adapter {
	if root == nil {
		return NewLegacyAdapterWithLive(history, preparation, live)
	}
	if isNilCompatibilityValue(history) {
		history = nil
	}
	if isNilCompatibilityValue(preparation) {
		preparation = nil
	}
	if isNilCompatibilityValue(live) {
		live = nil
	}
	return &Adapter{
		root: root, legacyHistory: history, legacyPreparation: preparation,
		legacyLiveEvents: live,
	}
}

// NewLegacyAdapter places the standalone durable compatibility path behind the
// Recordings-owned HTTP adapter. It is used by direct JavaScript hosting and
// narrow transport fakes; opened process runtimes use NewAdapter instead.
func NewLegacyAdapter(history LegacyHistory, preparation LegacyRequestPreparation) *Adapter {
	return NewLegacyAdapterWithLive(history, preparation)
}

// NewLegacyAdapterWithLive extends the standalone compatibility bridge with
// the old live-session event capability used by narrow transport fakes.
func NewLegacyAdapterWithLive(
	history LegacyHistory,
	preparation LegacyRequestPreparation,
	live ...LegacyLiveEvents,
) *Adapter {
	var liveEvents LegacyLiveEvents
	if len(live) > 0 {
		liveEvents = live[0]
	}
	if isNilCompatibilityValue(history) {
		history = nil
	}
	if isNilCompatibilityValue(preparation) {
		preparation = nil
	}
	if isNilCompatibilityValue(liveEvents) {
		liveEvents = nil
	}
	return &Adapter{
		legacyHistory: history, legacyPreparation: preparation,
		legacyLiveEvents: liveEvents,
	}
}

func isNilCompatibilityValue(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}

// Root returns the accepted Recordings root consumed by adapter-owned operations.
func (a *Adapter) Root() recordings.Service {
	if a == nil {
		return nil
	}
	return a.root
}

// invokeSubscribeFrom forwards subscribe requests through the accepted Recordings root.
func (a *Adapter) invokeSubscribeFrom(
	ctx context.Context,
	request recordings.SubscribeRequest,
) (recordings.SubscribeResult, error) {
	if a == nil || a.root == nil {
		return recordings.SubscribeResult{}, errors.New("recordings service is required")
	}
	return a.root.SubscribeFrom(ctx, request)
}

func (a *Adapter) invokeQueryRecordingStatus(
	request recordings.RecordingStatusRequest,
) (recordings.RecordingStatusResult, error) {
	if a == nil || a.root == nil {
		return recordings.RecordingStatusResult{}, errors.New("recordings service is required")
	}
	return a.root.QueryRecordingStatus(request)
}

func (a *Adapter) invokeBuildPortableArtifact(
	request recordings.BuildPortableArtifactRequest,
) (recordings.BuildPortableArtifactResult, error) {
	if a == nil || a.root == nil {
		return recordings.BuildPortableArtifactResult{}, errors.New("recordings service is required")
	}
	return a.root.BuildPortableArtifact(request)
}

func (a *Adapter) invokeReconstructWorldState(
	request recordings.ReconstructWorldStateRequest,
) (recordings.ReconstructWorldStateResult, error) {
	if a == nil || a.root == nil {
		return recordings.ReconstructWorldStateResult{}, errors.New("recordings service is required")
	}
	return a.root.ReconstructWorldState(request)
}

func (a *Adapter) invokeQueryHistoricalRecording(
	request recordings.HistoricalRecordingQueryRequest,
) (recordings.HistoricalRecordingQueryResult, error) {
	if a == nil || a.root == nil {
		return recordings.HistoricalRecordingQueryResult{}, errors.New("recordings service is required")
	}
	return a.root.QueryHistoricalRecording(request)
}
