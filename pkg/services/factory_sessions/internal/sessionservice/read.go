package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	workflowresult "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	canonicaldurable "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/canonical/durable"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/controlplane"
	factorysessionexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livesession"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/responseevents"
	liveruntime "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/live_runtime"
	responsestreamservice "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/response_stream"
)

// ResolveFactorySession returns the canonical live session entity for
// boundary adapters that need transient response-event or summary state.
func (s *Service) ResolveFactorySession(sessionID string) *livesession.LiveSession {
	if s == nil || s.host == nil {
		return nil
	}
	return s.liveRuntime.Resolve(sessionID)
}

// SubscribeFactoryResponseEvents resolves exactly one live Factory Session and
// delegates cursor, filtering, and retained-then-live policy to its owner.
func (s *Service) SubscribeFactoryResponseEvents(
	ctx context.Context,
	request factorysessions.ResponseEventSubscriptionRequest,
) (*factorysessions.ResponseEventCursor, error) {
	if s == nil || s.host == nil {
		return nil, fmt.Errorf("factory session gateway is required")
	}
	session := s.liveRuntime.Resolve(request.SessionID)
	if session == nil {
		return nil, factorysessions.ErrSessionNotFound
	}
	if session.ResponseEvents == nil {
		return nil, factorysessions.ErrRuntimeNotAvailable
	}
	if s.responseEvents == nil {
		return nil, factorysessions.ErrRuntimeNotAvailable
	}
	cursor, err := s.responseEvents.Subscribe(ctx, session.ResponseEvents, responsestreamservice.SubscriptionRequest{
		AfterSequence: request.AfterSequence,
		DispatchID:    request.DispatchID,
		Kinds:         request.Kinds,
	})
	switch {
	case errors.Is(err, responsestreamservice.ErrInvalidCursor):
		return nil, factorysessions.ErrInvalidResponseEventCursor
	case errors.Is(err, responsestreamservice.ErrInvalidFilter):
		return nil, factorysessions.ErrInvalidResponseEventFilter
	default:
		return cursor, err
	}
}

// SubscribeDurableFactoryResponseEvents opens one durable-session response-event
// cursor through the bound durable execution implementation.
func (s *Service) SubscribeDurableFactoryResponseEvents(
	ctx context.Context,
	request factorysessions.ResponseEventSubscriptionRequest,
) (*factorysessions.ResponseEventCursor, error) {
	if s == nil || s.durable == nil {
		return nil, factorysessions.ErrRuntimeNotAvailable
	}
	subscriber, ok := s.durable.(interface {
		SubscribeResponseEvents(context.Context, string, factorysessions.ResponseEventSubscriptionRequest) (*factorysessions.ResponseEventCursor, error)
	})
	if !ok {
		return nil, factorysessions.ErrRuntimeNotAvailable
	}
	return subscriber.SubscribeResponseEvents(ctx, request.SessionID, request)
}

// ListFactorySessions returns live workspace session summaries through control-plane read policy.
func (s *Service) ListFactorySessions(ctx context.Context) ([]factorysessions.ReadProjection, error) {
	if s == nil || s.host == nil {
		return nil, fmt.Errorf("factory session gateway is required")
	}
	return s.liveRuntime.List(ctx)
}

// GetFactorySession returns one live session detail through control-plane read routing.
func (s *Service) GetFactorySession(ctx context.Context, sessionID string) (factorysessions.SessionProjection, error) {
	if s == nil || s.host == nil {
		return factorysessions.SessionProjection{}, fmt.Errorf("factory session gateway is required")
	}
	return s.liveRuntime.Get(ctx, sessionID)
}

// GetFactorySessionSyncPreflight validates reconnect cursors before live event recovery.
func (s *Service) GetFactorySessionSyncPreflight(
	ctx context.Context,
	sessionID string,
	reconnect *factorydefinitions.FactoryEventReconnectCursor,
	logicalResolve *factorydefinitions.FactorySessionLogicalResolveHint,
) (factorysessions.SyncPreflightResult, error) {
	if s == nil || s.host == nil {
		return factorysessions.SyncPreflightResult{}, fmt.Errorf("factory session gateway is required")
	}
	return controlplane.GetLiveFactorySessionSyncPreflight(
		ctx, s.host, sessionID, reconnect, logicalResolve, s.reconnects,
	)
}

// GetFactorySessionResult returns the terminal JavaScript session result read shape.
func (s *Service) GetFactorySessionResult(ctx context.Context, sessionID string) (workflowresult.LiveSessionResult, error) {
	if s == nil || s.host == nil {
		return workflowresult.LiveSessionResult{}, fmt.Errorf("factory session gateway is required")
	}
	return controlplane.GetLiveFactorySessionResult(ctx, s.host, s.results, sessionID)
}

// GetFactorySessionPartialResult returns checkpoint-backed partial JavaScript results.
func (s *Service) GetFactorySessionPartialResult(
	ctx context.Context,
	sessionID string,
) (workflowresult.PartialSessionResult, error) {
	if s == nil || s.host == nil {
		return workflowresult.PartialSessionResult{}, fmt.Errorf("factory session gateway is required")
	}
	return controlplane.GetLiveFactorySessionPartialResult(ctx, s.host, sessionID)
}

// Get reads one mode-neutral Factory Session projection directly from its
// selected owner. The returned view contains no live runtime or durable
// execution implementation values.
func (s *Service) Get(
	ctx context.Context,
	request factorysessions.SessionGetRequest,
) (factorysessions.SessionGetResult, error) {
	if err := validateCanonicalSessionID(request.SessionID); err != nil {
		return factorysessions.SessionGetResult{}, err
	}
	sessionID := strings.TrimSpace(request.SessionID)
	switch request.Mode {
	case factorysessions.SessionOperationModeLive:
		live, err := s.canonicalLiveRuntime()
		if err != nil {
			return factorysessions.SessionGetResult{}, err
		}
		projection, err := live.Get(ctx, sessionID)
		if err != nil {
			return factorysessions.SessionGetResult{}, err
		}
		return factorysessions.SessionGetResult{
			Session: canonicalLiveSessionView(projection),
		}, nil
	case factorysessions.SessionOperationModeDurable:
		reads, err := s.canonicalDurableReads()
		if err != nil {
			return factorysessions.SessionGetResult{}, err
		}
		projection, err := reads.GetCanonical(ctx, sessionID)
		if err != nil {
			return factorysessions.SessionGetResult{}, err
		}
		return factorysessions.SessionGetResult{
			Session: canonicalDurableSessionView(projection),
		}, nil
	default:
		return factorysessions.SessionGetResult{}, canonicalRequestError(
			"mode", "mode must be live or durable",
		)
	}
}

// List reads one mode-neutral Factory Session inventory. Combined results
// retain live rows first and durable rows second, matching the established
// detached ordering while keeping each row owner-projected.
func (s *Service) List(
	ctx context.Context,
	request factorysessions.SessionListRequest,
) (factorysessions.SessionListResult, error) {
	filters, err := canonicalSessionListFilters(request.Mode, request.Filters)
	if err != nil {
		return factorysessions.SessionListResult{}, err
	}
	result := factorysessions.SessionListResult{Mode: request.Mode}
	switch request.Mode {
	case factorysessions.SessionOperationModeLive:
		live, err := s.canonicalLiveRuntime()
		if err != nil {
			return factorysessions.SessionListResult{}, err
		}
		projections, err := live.List(ctx)
		if err != nil {
			return factorysessions.SessionListResult{}, err
		}
		result.Sessions = canonicalLiveSessionViews(projections)
		return result, nil
	case factorysessions.SessionOperationModeDurable:
		reads, err := s.canonicalDurableReads()
		if err != nil {
			return factorysessions.SessionListResult{}, err
		}
		projections, err := reads.ListCanonical(ctx, factorysessions.ListSessionsRequest{
			Scope:   factorysessions.SessionListScopePersisted,
			Filters: filters,
		})
		if err != nil {
			return factorysessions.SessionListResult{}, err
		}
		result.Sessions = canonicalDurableSessionViews(projections.DurableSessions)
		return result, nil
	case factorysessions.SessionOperationModeAll:
		live, err := s.canonicalLiveRuntime()
		if err != nil {
			return factorysessions.SessionListResult{}, err
		}
		liveProjections, err := live.List(ctx)
		if err != nil {
			return factorysessions.SessionListResult{}, err
		}
		reads, err := s.canonicalDurableReads()
		if err != nil {
			return factorysessions.SessionListResult{}, err
		}
		durableProjections, err := reads.ListCanonical(ctx, factorysessions.ListSessionsRequest{
			Scope:   factorysessions.SessionListScopePersisted,
			Filters: filters,
		})
		if err != nil {
			return factorysessions.SessionListResult{}, err
		}
		result.Sessions = make([]factorysessions.SessionView, 0, len(liveProjections)+len(durableProjections.DurableSessions))
		result.Sessions = append(result.Sessions, canonicalLiveSessionViews(liveProjections)...)
		result.Sessions = append(result.Sessions, canonicalDurableSessionViews(durableProjections.DurableSessions)...)
		return result, nil
	default:
		return factorysessions.SessionListResult{}, canonicalRequestError(
			"mode", "mode must be live, durable, or all",
		)
	}
}

// Control applies one mode-neutral lifecycle or dispatch control to its
// selected owner. Live CLOSE remains destructive; CANCEL and TERMINATE use the
// lifecycle-control owner so stopped sessions remain inspectable.
func (s *Service) Control(
	ctx context.Context,
	request factorysessions.SessionControlRequest,
) (factorysessions.SessionControlResult, error) {
	if err := validateCanonicalControlRequest(request); err != nil {
		return factorysessions.SessionControlResult{}, err
	}
	normalized := cloneCanonicalControlRequest(request)
	normalized.SessionID = strings.TrimSpace(request.SessionID)
	if normalized.Control.RequestID == "" {
		normalized.Control.RequestID = strings.TrimSpace(request.Correlation.RequestID)
	}
	if normalized.Control.TurnID == "" {
		normalized.Control.TurnID = strings.TrimSpace(request.Correlation.TurnID)
	}
	switch normalized.Mode {
	case factorysessions.SessionOperationModeLive:
		return s.controlCanonicalLive(ctx, normalized)
	case factorysessions.SessionOperationModeDurable:
		return s.controlCanonicalDurable(ctx, normalized)
	default:
		return factorysessions.SessionControlResult{}, canonicalRequestError(
			"mode", "mode must be live or durable",
		)
	}
}

// ReadResult reads a durable result or live result projection without routing
// through the compatibility-shaped Factory Sessions methods.
func (s *Service) ReadResult(
	ctx context.Context,
	request factorysessions.SessionResultReadRequest,
) (factorysessions.SessionResultReadResult, error) {
	if err := validateCanonicalSessionID(request.SessionID); err != nil {
		return factorysessions.SessionResultReadResult{}, err
	}
	if request.Mode != factorysessions.SessionOperationModeDurable && request.Mode != factorysessions.SessionOperationModeLive {
		return factorysessions.SessionResultReadResult{}, canonicalRequestError(
			"mode", "mode must be live or durable",
		)
	}
	normalizedRequest, err := factorysessionexecution.NormalizeResultRequest(request.Request)
	if err != nil {
		return factorysessions.SessionResultReadResult{}, err
	}
	sessionID := strings.TrimSpace(request.SessionID)
	switch request.Mode {
	case factorysessions.SessionOperationModeDurable:
		results, err := s.canonicalDurableResults()
		if err != nil {
			return factorysessions.SessionResultReadResult{}, err
		}
		projection, err := results.ReadResultCanonical(ctx, sessionID, normalizedRequest)
		if err != nil {
			return factorysessions.SessionResultReadResult{}, err
		}
		return factorysessions.SessionResultReadResult{
			SessionID: sessionID,
			Mode:      factorysessions.SessionOperationModeDurable,
			Status:    string(projection.ResultStatus),
			Durable:   cloneCanonicalDurableResult(projection),
		}, nil
	case factorysessions.SessionOperationModeLive:
		return s.readCanonicalLiveResult(ctx, sessionID, normalizedRequest)
	}
	return factorysessions.SessionResultReadResult{}, canonicalRequestError(
		"mode", "mode must be live or durable",
	)
}

// QueryDispatches reads the canonical durable dispatch summary projection.
// Petri-bearing dispatch detail remains on the legacy GetDispatch surface.
func (s *Service) queryCanonicalDispatches(
	ctx context.Context,
	request factorysessions.DispatchQueryRequest,
) (factorysessions.ListDispatchesResult, error) {
	if err := validateCanonicalSessionID(request.SessionID); err != nil {
		return factorysessions.ListDispatchesResult{}, err
	}
	filters, err := factorysessionexecution.NormalizeDispatchFilters(request.Filters)
	if err != nil {
		return factorysessions.ListDispatchesResult{}, err
	}
	dispatches, err := s.canonicalDurableDispatches()
	if err != nil {
		return factorysessions.ListDispatchesResult{}, err
	}
	result, err := dispatches.QueryDispatchesCanonical(ctx, factorysessions.DispatchQueryRequest{
		SessionID: strings.TrimSpace(request.SessionID),
		Filters:   filters,
	})
	if err != nil {
		return factorysessions.ListDispatchesResult{}, err
	}
	return cloneCanonicalDispatchesResult(result), nil
}

// SubscribeResponses opens a retained-then-live cursor for either a live or
// durable session. Live identity wins when it resolves; no default-session
// fallback is attempted.
func (s *Service) SubscribeResponses(
	ctx context.Context,
	request factorysessions.SessionResponseSubscriptionRequest,
) (factorysessions.SessionResponseSubscriptionResult, error) {
	if err := validateCanonicalSessionID(request.SessionID); err != nil {
		return factorysessions.SessionResponseSubscriptionResult{}, err
	}
	if request.AfterSequence < 0 {
		return factorysessions.SessionResponseSubscriptionResult{}, canonicalRequestError(
			"afterSequence", "sequence must not be negative",
		)
	}
	if err := validateCanonicalResponseKinds(request.Kinds); err != nil {
		return factorysessions.SessionResponseSubscriptionResult{}, err
	}
	normalized := factorysessions.ResponseEventSubscriptionRequest{
		SessionID:     strings.TrimSpace(request.SessionID),
		AfterSequence: request.AfterSequence,
		DispatchID:    strings.TrimSpace(request.DispatchID),
		Kinds:         append([]factorysessions.ResponseEventKind(nil), request.Kinds...),
	}
	if live := s.resolveCanonicalLiveSession(normalized.SessionID); live != nil {
		cursor, err := s.subscribeCanonicalLiveResponses(ctx, live, normalized)
		if err != nil {
			return factorysessions.SessionResponseSubscriptionResult{}, err
		}
		return factorysessions.SessionResponseSubscriptionResult{Cursor: cursor}, nil
	}
	responses, err := s.canonicalDurableResponses()
	if err != nil {
		return factorysessions.SessionResponseSubscriptionResult{}, err
	}
	cursor, err := responses.SubscribeResponsesCanonical(ctx, normalized)
	if err != nil {
		return factorysessions.SessionResponseSubscriptionResult{}, err
	}
	return factorysessions.SessionResponseSubscriptionResult{Cursor: cursor}, nil
}

func (s *Service) canonicalLiveRuntime() (liveruntime.Service, error) {
	if s == nil || s.liveRuntime == nil {
		return nil, fmt.Errorf("%w: live session service is required", factorysessions.ErrRuntimeNotAvailable)
	}
	return s.liveRuntime, nil
}

func (s *Service) canonicalDurableReads() (canonicaldurable.Service, error) {
	execution, err := s.durableExecution()
	if err != nil {
		return nil, err
	}
	reads, ok := execution.(canonicaldurable.Service)
	if !ok || reads == nil {
		return nil, fmt.Errorf("%w: canonical durable read service is required", factorysessions.ErrExecutionServiceNotConfigured)
	}
	return reads, nil
}

func (s *Service) canonicalDurableResults() (canonicaldurable.Service, error) {
	execution, err := s.durableExecution()
	if err != nil {
		return nil, err
	}
	results, ok := execution.(canonicaldurable.Service)
	if !ok || results == nil {
		return nil, fmt.Errorf("%w: canonical durable result service is required", factorysessions.ErrExecutionServiceNotConfigured)
	}
	return results, nil
}

func (s *Service) canonicalDurableDispatches() (canonicaldurable.Service, error) {
	execution, err := s.durableExecution()
	if err != nil {
		return nil, err
	}
	dispatches, ok := execution.(canonicaldurable.Service)
	if !ok || dispatches == nil {
		return nil, fmt.Errorf("%w: canonical durable dispatch service is required", factorysessions.ErrExecutionServiceNotConfigured)
	}
	return dispatches, nil
}

func (s *Service) canonicalDurableResponses() (canonicaldurable.Service, error) {
	execution, err := s.durableExecution()
	if err != nil {
		return nil, err
	}
	responses, ok := execution.(canonicaldurable.Service)
	if !ok || responses == nil {
		return nil, fmt.Errorf("%w: canonical durable response service is required", factorysessions.ErrRuntimeNotAvailable)
	}
	return responses, nil
}

func (s *Service) controlCanonicalLive(
	ctx context.Context,
	request factorysessions.SessionControlRequest,
) (factorysessions.SessionControlResult, error) {
	live, err := s.canonicalLiveRuntime()
	if err != nil {
		return factorysessions.SessionControlResult{}, err
	}
	if request.Operation == factorysessions.SessionControlClose {
		if err := live.Close(ctx, request.SessionID); err != nil {
			return factorysessions.SessionControlResult{}, err
		}
		return factorysessions.SessionControlResult{
			SessionID: request.SessionID,
			Mode:      request.Mode,
			Operation: request.Operation,
			Closed:    true,
		}, nil
	}
	operation := factorysessions.LifecycleControlKind(request.Operation)
	control, err := live.ApplyControl(ctx, request.SessionID, operation, request.Control)
	if err != nil {
		return factorysessions.SessionControlResult{}, err
	}
	return canonicalControlResult(request, control), nil
}

func (s *Service) controlCanonicalDurable(
	ctx context.Context,
	request factorysessions.SessionControlRequest,
) (factorysessions.SessionControlResult, error) {
	controls, err := s.canonicalDurableControls()
	if err != nil {
		return factorysessions.SessionControlResult{}, err
	}
	result, err := controls.ControlCanonical(ctx, request)
	if err != nil {
		return factorysessions.SessionControlResult{}, err
	}
	if result.Recovery != nil {
		return factorysessions.SessionControlResult{
			SessionID: request.SessionID,
			Mode:      request.Mode,
			Operation: request.Operation,
			Status:    factorysessions.LifecycleStatus(result.Recovery.Status),
			Recovery:  cloneCanonicalAsyncStartResult(result.Recovery),
		}, nil
	}
	if result.Lifecycle == nil {
		return factorysessions.SessionControlResult{}, fmt.Errorf("canonical durable control returned no result")
	}
	return canonicalControlResult(request, *result.Lifecycle), nil
}

func (s *Service) canonicalDurableControls() (canonicaldurable.Service, error) {
	execution, err := s.durableExecution()
	if err != nil {
		return nil, err
	}
	controls, ok := execution.(canonicaldurable.Service)
	if !ok || controls == nil {
		return nil, fmt.Errorf("%w: canonical durable control service is required", factorysessions.ErrExecutionServiceNotConfigured)
	}
	return controls, nil
}

func (s *Service) readCanonicalLiveResult(
	ctx context.Context,
	sessionID string,
	request factorysessions.ResultRequest,
) (factorysessions.SessionResultReadResult, error) {
	if s == nil || s.host == nil {
		return factorysessions.SessionResultReadResult{}, fmt.Errorf(
			"%w: live result host is required", factorysessions.ErrRuntimeNotAvailable,
		)
	}
	if request.Mode == factorysessions.ResultModePartial {
		result, err := controlplane.GetLiveFactorySessionPartialResult(ctx, s.host, sessionID)
		if err != nil {
			return factorysessions.SessionResultReadResult{}, err
		}
		return factorysessions.SessionResultReadResult{
			SessionID: sessionID,
			Mode:      factorysessions.SessionOperationModeLive,
			Status:    "PARTIAL",
			Live: &factorysessions.SessionLiveResult{
				SessionID:         result.SessionID,
				Status:            "PARTIAL",
				CheckpointRefs:    cloneCanonicalCheckpointRefs(result.CheckpointRefs),
				ResultArtifactRef: cloneCanonicalArtifactRef(result.PartialResultArtifactRef),
			},
		}, nil
	}
	if s.results == nil {
		return factorysessions.SessionResultReadResult{}, fmt.Errorf(
			"%w: live result projection is required", factorysessions.ErrRuntimeNotAvailable,
		)
	}
	result, err := controlplane.GetLiveFactorySessionResult(ctx, s.host, s.results, sessionID)
	if err != nil {
		return factorysessions.SessionResultReadResult{}, err
	}
	status := fmt.Sprint(result.Status)
	return factorysessions.SessionResultReadResult{
		SessionID: sessionID,
		Mode:      factorysessions.SessionOperationModeLive,
		Status:    status,
		Live: &factorysessions.SessionLiveResult{
			SessionID:         result.SessionID,
			Status:            status,
			CheckpointRefs:    cloneCanonicalCheckpointRefs(result.CheckpointRefs),
			ResultArtifactRef: cloneCanonicalArtifactRef(result.ResultArtifactRef),
		},
	}, nil
}

func (s *Service) resolveCanonicalLiveSession(sessionID string) *livesession.LiveSession {
	if s == nil || s.liveRuntime == nil {
		return nil
	}
	return s.liveRuntime.Resolve(sessionID)
}

func (s *Service) subscribeCanonicalLiveResponses(
	ctx context.Context,
	session *livesession.LiveSession,
	request factorysessions.ResponseEventSubscriptionRequest,
) (*factorysessions.ResponseEventCursor, error) {
	if session == nil || session.ResponseEvents == nil || s == nil || s.responseEvents == nil {
		return nil, factorysessions.ErrRuntimeNotAvailable
	}
	cursor, err := s.responseEvents.Subscribe(ctx, session.ResponseEvents, responsestreamservice.SubscriptionRequest{
		AfterSequence: request.AfterSequence,
		DispatchID:    request.DispatchID,
		Kinds:         request.Kinds,
	})
	switch {
	case errors.Is(err, responsestreamservice.ErrInvalidCursor):
		return nil, factorysessions.ErrInvalidResponseEventCursor
	case errors.Is(err, responsestreamservice.ErrInvalidFilter):
		return nil, factorysessions.ErrInvalidResponseEventFilter
	default:
		return cursor, err
	}
}

func validateCanonicalSessionID(sessionID string) error {
	if strings.TrimSpace(sessionID) == "" {
		return canonicalRequestError("sessionId", "session id is required")
	}
	return nil
}

func validateCanonicalControlRequest(request factorysessions.SessionControlRequest) error {
	if err := validateCanonicalSessionID(request.SessionID); err != nil {
		return err
	}
	switch request.Mode {
	case factorysessions.SessionOperationModeLive:
		switch request.Operation {
		case factorysessions.SessionControlPause, factorysessions.SessionControlResume,
			factorysessions.SessionControlClose, factorysessions.SessionControlCancel,
			factorysessions.SessionControlTerminate:
			return nil
		default:
			return canonicalRequestError("operation", "unsupported live control operation")
		}
	case factorysessions.SessionOperationModeDurable:
		switch request.Operation {
		case factorysessions.SessionControlPause, factorysessions.SessionControlResume,
			factorysessions.SessionControlCancel, factorysessions.SessionControlTerminate,
			factorysessions.SessionControlRecover, factorysessions.SessionControlApprove,
			factorysessions.SessionControlRetryDispatch, factorysessions.SessionControlInterruptDispatch:
			return nil
		default:
			return canonicalRequestError("operation", "unsupported durable control operation")
		}
	default:
		return canonicalRequestError("mode", "mode must be live or durable")
	}
}

func canonicalSessionListFilters(
	mode factorysessions.SessionOperationMode,
	filters factorysessions.SessionListFilters,
) (factorysessions.SessionListFilters, error) {
	var scope factorysessions.SessionListScope
	switch mode {
	case factorysessions.SessionOperationModeLive:
		scope = factorysessions.SessionListScopeLive
	case factorysessions.SessionOperationModeDurable:
		scope = factorysessions.SessionListScopePersisted
	case factorysessions.SessionOperationModeAll:
		scope = factorysessions.SessionListScopeAll
	default:
		return factorysessions.SessionListFilters{}, canonicalRequestError(
			"mode", "mode must be live, durable, or all",
		)
	}
	normalized, err := factorysessionexecution.NormalizeListSessionsRequest(factorysessions.ListSessionsRequest{
		Scope:   scope,
		Filters: filters,
	})
	if err != nil {
		return factorysessions.SessionListFilters{}, err
	}
	return cloneCanonicalSessionListFilters(normalized.Filters), nil
}

func validateCanonicalResponseKinds(kinds []factorysessions.ResponseEventKind) error {
	for index, kind := range kinds {
		if canonicalResponseEventKindValid(kind) {
			continue
		}
		return fmt.Errorf("%w: kinds[%d] %q is unsupported", factorysessions.ErrInvalidResponseEventFilter, index, kind)
	}
	return nil
}

func canonicalResponseEventKindValid(kind factorysessions.ResponseEventKind) bool {
	switch responseevents.Kind(kind) {
	case responseevents.KindSession, responseevents.KindRun, responseevents.KindTurn,
		responseevents.KindMessage, responseevents.KindReasoning, responseevents.KindTool,
		responseevents.KindFileChange, responseevents.KindPlan, responseevents.KindProgress,
		responseevents.KindUsage, responseevents.KindError, responseevents.KindStreamGap:
		return true
	default:
		return false
	}
}

func canonicalControlResult(
	request factorysessions.SessionControlRequest,
	result factorysessions.LifecycleControlResult,
) factorysessions.SessionControlResult {
	return factorysessions.SessionControlResult{
		SessionID:         request.SessionID,
		Mode:              request.Mode,
		Operation:         request.Operation,
		Outcome:           result.Outcome,
		Status:            result.Status,
		Detail:            result.Detail,
		ApprovalPreviewID: result.ApprovalPreviewID,
		DispatchID:        result.DispatchID,
		RetryDispatchID:   result.RetryDispatchID,
		Links:             result.Links,
	}
}

func canonicalLiveSessionView(projection factorysessions.SessionProjection) factorysessions.SessionView {
	runtimeAvailable := projection.Context.Session != nil && projection.Context.Session.Runtime != nil
	view := canonicalLiveView(projection.Context, projection.Runtime, runtimeAvailable)
	return view
}

func canonicalLiveSessionViews(projections []factorysessions.ReadProjection) []factorysessions.SessionView {
	if len(projections) == 0 {
		return nil
	}
	views := make([]factorysessions.SessionView, 0, len(projections))
	for _, projection := range projections {
		views = append(views, canonicalLiveView(projection.Context, projection.Runtime, projection.RuntimeAvailable))
	}
	return views
}

func canonicalLiveView(
	projection factorysessions.ProjectionContext,
	runtime factorysessions.RuntimeProjection,
	runtimeAvailable bool,
) factorysessions.SessionView {
	view := factorysessions.SessionView{
		Mode:             factorysessions.SessionOperationModeLive,
		Status:           runtime.Status,
		RuntimeAvailable: runtimeAvailable,
	}
	if projection.Session != nil {
		view.SessionID = projection.Session.ID
		view.FactoryDir = projection.Session.FactoryDir
		view.FolderPath = projection.Session.FolderPath
		view.Project = projection.Session.Project
		view.IsDefault = projection.Session.IsDefault
		view.Target = projection.Session.Target
	}
	if view.SessionID == "" {
		view.SessionID = projection.FactorySessionID
	}
	return view
}

func canonicalDurableSessionView(projection factorysessions.SessionReadResult) factorysessions.SessionView {
	view := factorysessions.SessionView{
		SessionID:        projection.SessionID,
		Mode:             factorysessions.SessionOperationModeDurable,
		Status:           string(projection.Status),
		OrchestratorKind: projection.OrchestratorKind,
		SourceRef:        projection.ResolvedSource.SourceRef,
		SourceHash:       projection.SourceHash,
	}
	if projection.ResultSummary != nil {
		view.ResultStatus = projection.ResultSummary.ResultStatus
	}
	return view
}

func canonicalDurableSessionViews(
	projections []factorysessions.DurableSessionListSummary,
) []factorysessions.SessionView {
	if len(projections) == 0 {
		return nil
	}
	views := make([]factorysessions.SessionView, 0, len(projections))
	for _, projection := range projections {
		view := factorysessions.SessionView{
			SessionID:        projection.SessionID,
			Mode:             factorysessions.SessionOperationModeDurable,
			Status:           string(projection.Status),
			OrchestratorKind: projection.OrchestratorKind,
			SourceRef:        projection.ResolvedSource.SourceRef,
			SourceHash:       projection.SourceHash,
		}
		if projection.ResultSummary != nil {
			view.ResultStatus = projection.ResultSummary.ResultStatus
		}
		views = append(views, view)
	}
	return views
}

func cloneCanonicalControlRequest(
	request factorysessions.SessionControlRequest,
) factorysessions.SessionControlRequest {
	cloned := request
	if request.Recover != nil {
		recoverRequest := *request.Recover
		cloned.Recover = &recoverRequest
	}
	if request.Approve != nil {
		approve := *request.Approve
		approve.ApprovedPolicy = cloneCanonicalAnyMap(request.Approve.ApprovedPolicy)
		cloned.Approve = &approve
	}
	if request.Retry != nil {
		retry := *request.Retry
		cloned.Retry = &retry
	}
	if request.Interrupt != nil {
		interrupt := *request.Interrupt
		cloned.Interrupt = &interrupt
	}
	return cloned
}

func cloneCanonicalSessionListFilters(
	filters factorysessions.SessionListFilters,
) factorysessions.SessionListFilters {
	cloned := filters
	cloned.Statuses = append([]factorysessions.LifecycleStatus(nil), filters.Statuses...)
	cloned.OrchestratorKinds = append([]string(nil), filters.OrchestratorKinds...)
	cloned.CreatedAfter = cloneCanonicalTime(filters.CreatedAfter)
	cloned.CreatedBefore = cloneCanonicalTime(filters.CreatedBefore)
	cloned.UpdatedAfter = cloneCanonicalTime(filters.UpdatedAfter)
	cloned.UpdatedBefore = cloneCanonicalTime(filters.UpdatedBefore)
	return cloned
}

func cloneCanonicalTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneCanonicalDurableResult(
	result factorysessions.ResultReadResult,
) *factorysessions.SessionDurableResult {
	cloned := &factorysessions.SessionDurableResult{
		SessionID:        result.SessionID,
		Status:           result.ResultStatus,
		SessionStatus:    result.SessionStatus,
		Mode:             result.Mode,
		IncludeArtifacts: result.IncludeArtifacts,
		PrimaryResult:    append([]byte(nil), result.PrimaryResult...),
		ArtifactIDs:      append([]string(nil), result.ArtifactIDs...),
		ArtifactRefs:     append([]factorysessions.ArtifactRefSummary(nil), result.ArtifactRefs...),
	}
	if result.Failure != nil {
		failure := *result.Failure
		cloned.Failure = &failure
	}
	if result.Availability != nil {
		availability := *result.Availability
		cloned.Availability = &availability
	}
	return cloned
}

func cloneCanonicalCheckpointRefs(
	refs []factorydefinitions.FactorySessionJavaScriptCheckpointEventRef,
) []factorydefinitions.FactorySessionJavaScriptCheckpointEventRef {
	if len(refs) == 0 {
		return nil
	}
	cloned := make([]factorydefinitions.FactorySessionJavaScriptCheckpointEventRef, len(refs))
	for index, ref := range refs {
		cloned[index] = ref
		cloned[index].ArtifactRef = cloneCanonicalArtifactRef(ref.ArtifactRef)
		cloned[index].Label = cloneCanonicalString(ref.Label)
		cloned[index].Summary = cloneCanonicalString(ref.Summary)
		cloned[index].Timestamp = cloneCanonicalTime(ref.Timestamp)
	}
	return cloned
}

func cloneCanonicalArtifactRef(
	ref *factorydefinitions.FactoryArtifactRef,
) *factorydefinitions.FactoryArtifactRef {
	if ref == nil {
		return nil
	}
	cloned := *ref
	if ref.ContentHash != nil {
		contentHash := *ref.ContentHash
		cloned.ContentHash = &contentHash
	}
	if ref.SizeBytes != nil {
		sizeBytes := *ref.SizeBytes
		cloned.SizeBytes = &sizeBytes
	}
	return &cloned
}

func cloneCanonicalString(value *string) *string {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneCanonicalDispatchesResult(
	result factorysessions.ListDispatchesResult,
) factorysessions.ListDispatchesResult {
	cloned := factorysessions.ListDispatchesResult{SessionID: result.SessionID}
	if len(result.Dispatches) == 0 {
		return cloned
	}
	cloned.Dispatches = make([]factorysessions.DispatchSummary, len(result.Dispatches))
	for index, dispatch := range result.Dispatches {
		cloned.Dispatches[index] = cloneCanonicalDispatch(dispatch)
	}
	return cloned
}

func cloneCanonicalDispatch(
	dispatch factorysessions.DispatchSummary,
) factorysessions.DispatchSummary {
	cloned := dispatch
	cloned.ProviderSessionRefs = append([]factorysessions.ProviderSessionRef(nil), dispatch.ProviderSessionRefs...)
	cloned.OutputArtifactIDs = append([]string(nil), dispatch.OutputArtifactIDs...)
	if dispatch.Retryable != nil {
		retryable := *dispatch.Retryable
		cloned.Retryable = &retryable
	}
	if dispatch.Usage != nil {
		usage := *dispatch.Usage
		cloned.Usage = &usage
	}
	cloned.Warnings = append([]factorysessions.DispatchWarning(nil), dispatch.Warnings...)
	if dispatch.FailureDetail != nil {
		failure := *dispatch.FailureDetail
		cloned.FailureDetail = &failure
	}
	if dispatch.JavaScript != nil {
		javaScript := *dispatch.JavaScript
		cloned.JavaScript = &javaScript
	}
	return cloned
}

var _ factorysessions.Service = (*Service)(nil)
