package factorysession

import (
	"context"
	"encoding/json"
	"reflect"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessionexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// isAbsentSource reports whether a bridged capability is absent, including the
// typed-nil values compatibility callers routinely pass for "not bound".
func isAbsentSource(value any) bool {
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

// DurableResultInput carries transport-resolved durable Factory Session result
// read fields before shared normalization. Transports that must not name the
// Factory Sessions contract carry this shape instead.
type DurableResultInput struct {
	Mode             string
	IncludeArtifacts bool
}

// DurableEventReconnectInput carries transport-resolved durable Factory Session
// event reconnect fields before shared normalization. Transports that must not
// name the Factory Sessions contract carry this shape instead.
type DurableEventReconnectInput struct {
	AfterEventID  string
	AfterSequence *int
}

// EventReconnectRequestFromInput maps one transport-resolved event reconnect
// request into the shared service contract.
func EventReconnectRequestFromInput(input DurableEventReconnectInput) (factorysessionexecution.EventReconnectRequest, error) {
	return factorysessionexecution.EventReconnectRequest{
		AfterEventID:  input.AfterEventID,
		AfterSequence: input.AfterSequence,
	}, nil
}

// ResultRequestFromInput maps one transport-resolved durable result read into
// the shared service contract.
func ResultRequestFromInput(input DurableResultInput) (factorysessionexecution.ResultRequest, error) {
	req := factorysessionexecution.ResultRequest{
		IncludeArtifacts: input.IncludeArtifacts,
	}
	if input.Mode != "" {
		req.Mode = factorysessionexecution.ResultMode(input.Mode)
	}
	return req, nil
}

// DurableResultInputFromAPI maps one public durable result read request into the
// transport-resolved input shape.
func DurableResultInputFromAPI(params factoryapi.GetFactorySessionResultsParams) (DurableResultInput, error) {
	input := DurableResultInput{}
	if params.Mode != nil {
		input.Mode = string(*params.Mode)
	}
	if params.IncludeArtifacts != nil {
		input.IncludeArtifacts = *params.IncludeArtifacts
	}
	return input, nil
}

// DurableEventReconnectInputFromAPI maps one public event reconnect request into
// the transport-resolved input shape.
func DurableEventReconnectInputFromAPI(params factoryapi.GetEventsBySessionIdParams) (DurableEventReconnectInput, error) {
	input := DurableEventReconnectInput{}
	if params.AfterEventId != nil {
		input.AfterEventID = string(*params.AfterEventId)
	}
	if params.AfterSequence != nil {
		sequence := int(*params.AfterSequence)
		input.AfterSequence = &sequence
	}
	return input, nil
}

// DurableHistorySource is the durable Factory Session history capability this
// package bridges for compatibility transports. It is satisfied by DurableAPI.
type DurableHistorySource interface {
	GetDurableFactorySessionResult(context.Context, string, factorysessionexecution.ResultRequest) (factoryapi.FactorySessionResult, error)
	ReadDurableFactorySessionEvents(context.Context, string, factorysessionexecution.EventReconnectRequest) (*interfaces.FactoryEventStream, error)
	ProbeDurableFactorySessionEvents(context.Context, string, factorysessionexecution.EventReconnectRequest) error
	ListDurableFactorySessionDispatches(context.Context, string, factoryapi.ListFactorySessionDispatchesParams) (factoryapi.ListFactorySessionDispatchesResponse, error)
	GetDurableFactorySessionDispatch(context.Context, string, string) (factoryapi.FactoryDispatch, error)
	ListDurableFactorySessionArtifacts(context.Context, string) (factoryapi.ListFactorySessionArtifactsResponse, error)
	GetDurableFactorySessionArtifact(context.Context, string, string) (factoryapi.FactorySessionArtifactDetail, error)
}

// DurableHistoryBridge restates the durable history capability in
// transport-resolved input vocabulary so a compatibility transport can forward
// a history read without naming the Factory Sessions request contract.
type DurableHistoryBridge struct {
	source DurableHistorySource
}

// NewDurableHistoryBridge wraps one durable history capability for
// compatibility transports. It returns nil when no source is supplied so the
// caller keeps its own "no legacy history bound" behavior.
func NewDurableHistoryBridge(source DurableHistorySource) *DurableHistoryBridge {
	if isAbsentSource(source) {
		return nil
	}
	return &DurableHistoryBridge{source: source}
}

// GetDurableFactorySessionResult forwards one transport-resolved durable result read.
func (bridge *DurableHistoryBridge) GetDurableFactorySessionResult(
	ctx context.Context,
	sessionID string,
	input DurableResultInput,
) (factoryapi.FactorySessionResult, error) {
	request, err := ResultRequestFromInput(input)
	if err != nil {
		return factoryapi.FactorySessionResult{}, err
	}
	return bridge.source.GetDurableFactorySessionResult(ctx, sessionID, request)
}

// ReadDurableFactorySessionEvents forwards one transport-resolved reconnect read.
func (bridge *DurableHistoryBridge) ReadDurableFactorySessionEvents(
	ctx context.Context,
	sessionID string,
	input DurableEventReconnectInput,
) (*interfaces.FactoryEventStream, error) {
	request, err := EventReconnectRequestFromInput(input)
	if err != nil {
		return nil, err
	}
	return bridge.source.ReadDurableFactorySessionEvents(ctx, sessionID, request)
}

// ProbeDurableFactorySessionEvents forwards one transport-resolved reconnect probe.
func (bridge *DurableHistoryBridge) ProbeDurableFactorySessionEvents(
	ctx context.Context,
	sessionID string,
	input DurableEventReconnectInput,
) error {
	request, err := EventReconnectRequestFromInput(input)
	if err != nil {
		return err
	}
	return bridge.source.ProbeDurableFactorySessionEvents(ctx, sessionID, request)
}

// ListDurableFactorySessionDispatches forwards one dispatch listing read.
func (bridge *DurableHistoryBridge) ListDurableFactorySessionDispatches(
	ctx context.Context,
	sessionID string,
	params factoryapi.ListFactorySessionDispatchesParams,
) (factoryapi.ListFactorySessionDispatchesResponse, error) {
	return bridge.source.ListDurableFactorySessionDispatches(ctx, sessionID, params)
}

// GetDurableFactorySessionDispatch forwards one dispatch detail read.
func (bridge *DurableHistoryBridge) GetDurableFactorySessionDispatch(
	ctx context.Context,
	sessionID string,
	dispatchID string,
) (factoryapi.FactoryDispatch, error) {
	return bridge.source.GetDurableFactorySessionDispatch(ctx, sessionID, dispatchID)
}

// ListDurableFactorySessionArtifacts forwards one artifact listing read.
func (bridge *DurableHistoryBridge) ListDurableFactorySessionArtifacts(
	ctx context.Context,
	sessionID string,
) (factoryapi.ListFactorySessionArtifactsResponse, error) {
	return bridge.source.ListDurableFactorySessionArtifacts(ctx, sessionID)
}

// GetDurableFactorySessionArtifact forwards one artifact detail read.
func (bridge *DurableHistoryBridge) GetDurableFactorySessionArtifact(
	ctx context.Context,
	sessionID string,
	artifactID string,
) (factoryapi.FactorySessionArtifactDetail, error) {
	return bridge.source.GetDurableFactorySessionArtifact(ctx, sessionID, artifactID)
}

// DurableArtifactFact is one artifact fact read from durable Factory Session
// execution by a transport that does not own the Factory Sessions contract.
type DurableArtifactFact struct {
	ID          string
	Kind        string
	Visibility  string
	Label       string
	ContentHash string
	SizeBytes   int64
	DispatchID  string
}

// DurableInspectionSource is the durable Factory Session inspection capability
// this package bridges for compatibility transports.
type DurableInspectionSource interface {
	QueryDispatches(context.Context, factorysessionexecution.DispatchQueryRequest) (factorysessionexecution.ListDispatchesResult, error)
	ListArtifacts(context.Context, string) (factorysessionexecution.ListArtifactsResult, error)
	ReadEvents(context.Context, string, factorysessionexecution.EventReconnectRequest) (factorysessionexecution.EventReadResult, error)
}

// DurableInspectionBridge restates durable session inspection in
// transport-resolved vocabulary.
type DurableInspectionBridge struct {
	source DurableInspectionSource
}

// NewDurableInspectionBridge wraps one durable inspection capability when the
// supplied value provides it, and returns nil otherwise so the caller keeps its
// own "no compatibility inspection available" behavior.
func NewDurableInspectionBridge(value any) *DurableInspectionBridge {
	source, ok := value.(DurableInspectionSource)
	if !ok || isAbsentSource(source) {
		return nil
	}
	return &DurableInspectionBridge{source: source}
}

// QueryDispatches reads the detached dispatch facts of one session.
func (bridge *DurableInspectionBridge) QueryDispatches(
	ctx context.Context,
	sessionID string,
) ([]HistoricalDispatchInput, error) {
	result, err := bridge.source.QueryDispatches(ctx, factorysessionexecution.DispatchQueryRequest{
		SessionID: sessionID,
	})
	if err != nil {
		return nil, err
	}
	dispatches := make([]HistoricalDispatchInput, 0, len(result.Dispatches))
	for _, dispatch := range result.Dispatches {
		dispatches = append(dispatches, HistoricalDispatchInput{
			ID:           dispatch.ID,
			Status:       string(dispatch.Status),
			DispatchKind: dispatch.DispatchKind,
		})
	}
	return dispatches, nil
}

// ListArtifacts reads the artifact facts of one session.
func (bridge *DurableInspectionBridge) ListArtifacts(
	ctx context.Context,
	sessionID string,
) ([]DurableArtifactFact, error) {
	result, err := bridge.source.ListArtifacts(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	artifacts := make([]DurableArtifactFact, 0, len(result.Artifacts))
	for _, artifact := range result.Artifacts {
		artifacts = append(artifacts, DurableArtifactFact{
			ID: artifact.ID, Kind: artifact.Kind, Visibility: artifact.Visibility,
			Label: artifact.Label, ContentHash: artifact.ContentHash,
			SizeBytes: artifact.SizeBytes, DispatchID: artifact.DispatchID,
		})
	}
	return artifacts, nil
}

// ReadEvents reads the retained event payloads of one session.
func (bridge *DurableInspectionBridge) ReadEvents(
	ctx context.Context,
	sessionID string,
	input DurableEventReconnectInput,
) ([]json.RawMessage, error) {
	request, err := EventReconnectRequestFromInput(input)
	if err != nil {
		return nil, err
	}
	result, err := bridge.source.ReadEvents(ctx, sessionID, request)
	if err != nil {
		return nil, err
	}
	return result.Events, nil
}
