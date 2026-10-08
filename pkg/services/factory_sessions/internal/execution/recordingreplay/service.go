package recordingreplay

import (
	"context"
	"errors"
	"sync"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	fse "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution"
)

// ErrNonLiveReplay reports an operation that would require live execution.
var ErrNonLiveReplay = errors.New("recorded Factory Sessions are historical and do not support live execution")

// Behavior supplies the fixed historical read and explicit handoff operations.
// Selected projections and live handles belong exclusively to acquired scopes.
type Behavior struct{}

func NewBehavior() *Behavior { return &Behavior{} }

// Scope owns one historical opening's projection, live handle and handoff state.
type Scope struct {
	behavior   *Behavior
	projection RecordingReplayProjection
	live       fse.Service
	mu         sync.RWMutex
	handedOff  bool
	handoffMu  sync.Mutex
}

func (b *Behavior) Acquire(projection RecordingReplayProjection, live fse.Service) *Scope {
	return &Scope{behavior: b, projection: copyProjection(projection), live: live}
}

// Inspection returns the complete public read model for the recording. The
// caller receives only the bounded facts already restored by ReplayRecording;
// no live execution or mutable checkpoint state is exposed.
func (b *Behavior) Inspection(s *Scope) factorysessions.HistoricalReplayInspection {
	if s == nil {
		return factorysessions.HistoricalReplayInspection{}
	}
	inspection := factorysessions.HistoricalReplayInspection{
		Session:       copySession(s.projection.Session),
		Events:        fse.EventReadResult{SessionID: s.projection.Events.SessionID, Events: copyEvents(s.projection.Events.Events)},
		Artifacts:     copyArtifacts(s.projection.Artifacts),
		Result:        copyResult(s.projection.Result),
		WorkerHistory: copyWorkerHistory(s.projection.WorkerHistory),
		Redaction: factorysessions.HistoricalReplayRedaction{
			RuntimeStateOmitted:        s.projection.Redaction.RuntimeStateOmitted,
			CheckpointBodiesOmitted:    s.projection.Redaction.CheckpointBodiesOmitted,
			ProviderTranscriptsOmitted: s.projection.Redaction.ProviderTranscriptsOmitted,
			ChildDispatchesOmitted:     s.projection.Redaction.ChildDispatchesOmitted,
			SecretsRedacted:            s.projection.Redaction.SecretsRedacted,
		},
	}
	if s.projection.FactoryProjection != nil {
		inspection.FactoryProjection = factorysessions.HistoricalReplayFactoryProjection{
			Availability: factorysessions.HistoricalReplayFactoryProjectionAvailable,
			State:        copyFactoryProjection(s.projection.FactoryProjection),
		}
	} else {
		inspection.FactoryProjection = factorysessions.HistoricalReplayFactoryProjection{
			Availability: factorysessions.HistoricalReplayFactoryProjectionUnavailable,
			Reason:       factorysessions.HistoricalReplayFactoryProjectionReasonNotRecorded,
		}
	}
	if checkpoint := s.projection.Checkpoint; checkpoint != nil {
		inspection.Checkpoint = &factorysessions.HistoricalReplayCheckpoint{
			ID: checkpoint.ID, Label: checkpoint.Label, Summary: checkpoint.Summary,
			ArtifactID: checkpoint.ArtifactID, Timestamp: checkpoint.Timestamp,
		}
	}
	return inspection
}

// IsNonLiveReplay lets control-plane routing recognize recorded canonical
// session identities that predate the durable-execution ID prefix convention.
// The exception remains true after handoff because this service still owns the
// replay identity and delegates subsequent operations to the live owner.
func (b *Behavior) IsNonLiveReplay(s *Scope) bool {
	return true
}

var _ fse.Service = (*Scope)(nil)

func (s *Scope) session(sessionID string) error {
	if s == nil || sessionID != s.projection.Session.SessionID {
		return fse.ErrSessionNotFound
	}
	return nil
}
func (b *Behavior) StartAsync(s *Scope, ctx context.Context, request fse.StartRequest) (fse.AsyncStartResult, error) {
	owner, handedOff := s.handedOffOwner()
	if !handedOff {
		return fse.AsyncStartResult{}, ErrNonLiveReplay
	}
	return owner.StartAsync(ctx, request)
}
func (b *Behavior) StartSync(s *Scope, ctx context.Context, request fse.StartRequest) (fse.SyncStartResult, error) {
	owner, handedOff := s.handedOffOwner()
	if !handedOff {
		return fse.SyncStartResult{}, ErrNonLiveReplay
	}
	return owner.StartSync(ctx, request)
}
func (b *Behavior) ResumeInterruptedSession(s *Scope,
	ctx context.Context,
	sessionID string,
	request fse.ResumeSessionRequest,
) (fse.AsyncStartResult, error) {
	s.handoffMu.Lock()
	defer s.handoffMu.Unlock()
	owner, err := s.resumeOwnerLocked(ctx, sessionID)
	if err != nil {
		return fse.AsyncStartResult{}, err
	}
	result, err := owner.ResumeInterruptedSession(ctx, sessionID, request)
	if err == nil {
		s.markHandedOff()
	}
	return result, err
}

// SubscribeResponseEvents keeps the response-event read surface behind the
// replay wall until the explicit resume handoff succeeds. Once handed off,
// the cursor is served by the already-composed durable owner.
func (b *Behavior) SubscribeResponseEvents(s *Scope,
	ctx context.Context,
	sessionID string,
	request factorysessions.ResponseEventSubscriptionRequest,
) (*factorysessions.ResponseEventCursor, error) {
	if err := s.session(sessionID); err != nil {
		return nil, err
	}
	owner, handedOff := s.handedOffOwnerForSession(sessionID)
	if !handedOff {
		return nil, ErrNonLiveReplay
	}
	subscriber, ok := owner.(interface {
		SubscribeResponseEvents(context.Context, string, factorysessions.ResponseEventSubscriptionRequest) (*factorysessions.ResponseEventCursor, error)
	})
	if !ok {
		return nil, factorysessions.ErrRuntimeNotAvailable
	}
	return subscriber.SubscribeResponseEvents(ctx, sessionID, request)
}

func (b *Behavior) GetSession(s *Scope, ctx context.Context, id string) (fse.SessionReadResult, error) {
	if err := s.session(id); err != nil {
		return fse.SessionReadResult{}, err
	}
	if owner, handedOff := s.handedOffOwnerForSession(id); handedOff {
		return owner.GetSession(ctx, id)
	}
	return copySession(s.projection.Session), nil
}
func (b *Behavior) Pause(s *Scope, ctx context.Context, id string, request fse.ControlRequest) (fse.LifecycleControlResult, error) {
	owner, err := s.handedOffOwnerForSessionOperation(id)
	if err != nil {
		return fse.LifecycleControlResult{}, err
	}
	return owner.Pause(ctx, id, request)
}
func (b *Behavior) Resume(s *Scope, ctx context.Context, id string, request fse.ControlRequest) (fse.LifecycleControlResult, error) {
	s.handoffMu.Lock()
	defer s.handoffMu.Unlock()
	owner, err := s.resumeOwnerLocked(ctx, id)
	if err != nil {
		return fse.LifecycleControlResult{}, err
	}
	result, err := owner.Resume(ctx, id, request)
	if err == nil {
		s.markHandedOff()
	}
	return result, err
}
func (b *Behavior) Cancel(s *Scope, ctx context.Context, id string, request fse.ControlRequest) (fse.LifecycleControlResult, error) {
	owner, err := s.handedOffOwnerForSessionOperation(id)
	if err != nil {
		return fse.LifecycleControlResult{}, err
	}
	return owner.Cancel(ctx, id, request)
}
func (b *Behavior) Terminate(s *Scope, ctx context.Context, id string, request fse.ControlRequest) (fse.LifecycleControlResult, error) {
	owner, err := s.handedOffOwnerForSessionOperation(id)
	if err != nil {
		return fse.LifecycleControlResult{}, err
	}
	return owner.Terminate(ctx, id, request)
}
func (b *Behavior) Approve(s *Scope, ctx context.Context, id string, request fse.ApproveRequest) (fse.LifecycleControlResult, error) {
	owner, err := s.handedOffOwnerForSessionOperation(id)
	if err != nil {
		return fse.LifecycleControlResult{}, err
	}
	return owner.Approve(ctx, id, request)
}
func (b *Behavior) RetryDispatch(s *Scope, ctx context.Context, id string, request fse.RetryDispatchRequest) (fse.LifecycleControlResult, error) {
	owner, err := s.handedOffOwnerForSessionOperation(id)
	if err != nil {
		return fse.LifecycleControlResult{}, err
	}
	return owner.RetryDispatch(ctx, id, request)
}
func (b *Behavior) InterruptDispatch(s *Scope, ctx context.Context, id string, request fse.InterruptDispatchRequest) (fse.LifecycleControlResult, error) {
	owner, err := s.handedOffOwnerForSessionOperation(id)
	if err != nil {
		return fse.LifecycleControlResult{}, err
	}
	return owner.InterruptDispatch(ctx, id, request)
}
func (b *Behavior) GetResult(s *Scope, ctx context.Context, id string, req fse.ResultRequest) (fse.ResultReadResult, error) {
	if err := s.session(id); err != nil {
		return fse.ResultReadResult{}, err
	}
	if owner, handedOff := s.handedOffOwnerForSession(id); handedOff {
		return owner.GetResult(ctx, id, req)
	}
	normalized, err := fse.NormalizeResultRequest(req)
	if err != nil {
		return fse.ResultReadResult{}, err
	}
	result := copyResult(s.projection.Result)
	result.Mode = normalized.Mode
	result.IncludeArtifacts = normalized.IncludeArtifacts
	if !normalized.IncludeArtifacts {
		result.ArtifactRefs = nil
	}
	return result, nil
}
func (b *Behavior) ListDispatches(s *Scope, ctx context.Context, id string) (fse.ListDispatchesResult, error) {
	if err := s.session(id); err != nil {
		return fse.ListDispatchesResult{}, err
	}
	if owner, handedOff := s.handedOffOwnerForSession(id); handedOff {
		return owner.ListDispatches(ctx, id)
	}
	return fse.ListDispatchesResult{
		SessionID:  id,
		Dispatches: []fse.DispatchSummary{},
	}, nil
}

func (b *Behavior) QueryDispatches(s *Scope, ctx context.Context, request fse.DispatchQueryRequest) (fse.ListDispatchesResult, error) {
	if err := s.session(request.SessionID); err != nil {
		return fse.ListDispatchesResult{}, err
	}
	if owner, handedOff := s.handedOffOwnerForSession(request.SessionID); handedOff {
		return owner.QueryDispatches(ctx, request)
	}
	result, err := b.ListDispatches(s, ctx, request.SessionID)
	if err != nil {
		return fse.ListDispatchesResult{}, err
	}
	return fse.FilterDispatches(result, request.Filters)
}
func (b *Behavior) GetDispatch(s *Scope, ctx context.Context, id, dispatchID string) (fse.DispatchDetail, error) {
	if err := s.session(id); err != nil {
		return fse.DispatchDetail{}, err
	}
	if owner, handedOff := s.handedOffOwnerForSession(id); handedOff {
		return owner.GetDispatch(ctx, id, dispatchID)
	}
	return fse.DispatchDetail{}, fse.ErrDispatchNotFound
}
func (b *Behavior) ListArtifacts(s *Scope, ctx context.Context, id string) (fse.ListArtifactsResult, error) {
	if err := s.session(id); err != nil {
		return fse.ListArtifactsResult{}, err
	}
	if owner, handedOff := s.handedOffOwnerForSession(id); handedOff {
		return owner.ListArtifacts(ctx, id)
	}
	return copyArtifacts(s.projection.Artifacts), nil
}
func (b *Behavior) GetArtifact(s *Scope, ctx context.Context, id, artifactID string) (fse.ArtifactDetail, error) {
	if err := s.session(id); err != nil {
		return fse.ArtifactDetail{}, err
	}
	if owner, handedOff := s.handedOffOwnerForSession(id); handedOff {
		return owner.GetArtifact(ctx, id, artifactID)
	}
	for _, artifact := range s.projection.Artifacts.Artifacts {
		if artifact.ID == artifactID {
			return fse.ArtifactDetail{ArtifactSummary: copyArtifact(artifact), SessionID: id}, nil
		}
	}
	return fse.ArtifactDetail{}, fse.ErrArtifactNotFound
}
func (b *Behavior) ReadEvents(s *Scope, ctx context.Context, id string, req fse.EventReconnectRequest) (fse.EventReadResult, error) {
	if err := s.session(id); err != nil {
		return fse.EventReadResult{}, err
	}
	if owner, handedOff := s.handedOffOwnerForSession(id); handedOff {
		return owner.ReadEvents(ctx, id, req)
	}
	events, err := fse.FilterEventsAfterReconnect(s.projection.Events.Events, req, id)
	return fse.EventReadResult{SessionID: id, Events: copyEvents(events)}, err
}
func (b *Behavior) ListSessions(s *Scope, ctx context.Context, request fse.ListSessionsRequest) (fse.ListSessionsResult, error) {
	if owner, handedOff := s.handedOffOwner(); handedOff {
		return owner.ListSessions(ctx, request)
	}
	session := copySession(s.projection.Session)
	return fse.ListSessionsResult{DurableSessions: []fse.DurableSessionListSummary{{SessionID: session.SessionID, Status: session.Status, OrchestratorKind: session.OrchestratorKind, ResolvedSource: session.ResolvedSource, SourceHash: session.SourceHash, Policy: session.Policy, ResultSummary: session.ResultSummary, ArtifactCount: session.ArtifactCount, Lifecycle: session.Lifecycle, Links: session.Links}}}, nil
}
