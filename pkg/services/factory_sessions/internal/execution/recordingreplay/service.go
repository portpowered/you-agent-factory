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

// Service exposes one validated recording through the canonical public read
// contract and owns the narrow transition to an already-composed live owner.
type Service struct {
	projection RecordingReplayProjection
	live       fse.Service

	mu        sync.RWMutex
	handedOff bool
	handoffMu sync.Mutex
}

// NewService constructs a historical replay. An optional live owner is used
// only for the explicit resume handoff; the replay projection never executes
// work or restores checkpoint state itself.
func NewService(projection RecordingReplayProjection, liveOwners ...fse.Service) *Service {
	var live fse.Service
	if len(liveOwners) > 0 {
		live = liveOwners[0]
	}
	return &Service{projection: projection, live: live}
}

// Inspection returns the complete public read model for the recording. The
// caller receives only the bounded facts already restored by ReplayRecording;
// no live execution or mutable checkpoint state is exposed.
func (s *Service) Inspection() factorysessions.HistoricalReplayInspection {
	if s == nil {
		return factorysessions.HistoricalReplayInspection{}
	}
	inspection := factorysessions.HistoricalReplayInspection{
		Session:       s.projection.Session,
		Events:        s.projection.Events,
		Artifacts:     s.projection.Artifacts,
		Result:        s.projection.Result,
		WorkerHistory: s.projection.WorkerHistory,
		Redaction: factorysessions.HistoricalReplayRedaction{
			RuntimeStateOmitted:        s.projection.Redaction.RuntimeStateOmitted,
			CheckpointBodiesOmitted:    s.projection.Redaction.CheckpointBodiesOmitted,
			ProviderTranscriptsOmitted: s.projection.Redaction.ProviderTranscriptsOmitted,
			ChildDispatchesOmitted:     s.projection.Redaction.ChildDispatchesOmitted,
			SecretsRedacted:            s.projection.Redaction.SecretsRedacted,
		},
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
func (s *Service) IsNonLiveReplay() bool {
	if s == nil {
		return true
	}
	s.mu.RLock()
	handedOff := s.handedOff
	s.mu.RUnlock()
	return !handedOff
}

var _ fse.Service = (*Service)(nil)

func (s *Service) session(sessionID string) error {
	if s == nil || sessionID != s.projection.Session.SessionID {
		return fse.ErrSessionNotFound
	}
	return nil
}
func (s *Service) StartAsync(ctx context.Context, request fse.StartRequest) (fse.AsyncStartResult, error) {
	owner, handedOff := s.handedOffOwner()
	if !handedOff {
		return fse.AsyncStartResult{}, ErrNonLiveReplay
	}
	return owner.StartAsync(ctx, request)
}
func (s *Service) StartSync(ctx context.Context, request fse.StartRequest) (fse.SyncStartResult, error) {
	owner, handedOff := s.handedOffOwner()
	if !handedOff {
		return fse.SyncStartResult{}, ErrNonLiveReplay
	}
	return owner.StartSync(ctx, request)
}
func (s *Service) ResumeInterruptedSession(
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
func (s *Service) GetSession(ctx context.Context, id string) (fse.SessionReadResult, error) {
	if err := s.session(id); err != nil {
		return fse.SessionReadResult{}, err
	}
	if owner, handedOff := s.handedOffOwnerForSession(id); handedOff {
		return owner.GetSession(ctx, id)
	}
	return s.projection.Session, nil
}
func (s *Service) Pause(ctx context.Context, id string, request fse.ControlRequest) (fse.LifecycleControlResult, error) {
	owner, err := s.handedOffOwnerForSessionOperation(id)
	if err != nil {
		return fse.LifecycleControlResult{}, err
	}
	return owner.Pause(ctx, id, request)
}
func (s *Service) Resume(ctx context.Context, id string, request fse.ControlRequest) (fse.LifecycleControlResult, error) {
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
func (s *Service) Cancel(ctx context.Context, id string, request fse.ControlRequest) (fse.LifecycleControlResult, error) {
	owner, err := s.handedOffOwnerForSessionOperation(id)
	if err != nil {
		return fse.LifecycleControlResult{}, err
	}
	return owner.Cancel(ctx, id, request)
}
func (s *Service) Terminate(ctx context.Context, id string, request fse.ControlRequest) (fse.LifecycleControlResult, error) {
	owner, err := s.handedOffOwnerForSessionOperation(id)
	if err != nil {
		return fse.LifecycleControlResult{}, err
	}
	return owner.Terminate(ctx, id, request)
}
func (s *Service) Approve(ctx context.Context, id string, request fse.ApproveRequest) (fse.LifecycleControlResult, error) {
	owner, err := s.handedOffOwnerForSessionOperation(id)
	if err != nil {
		return fse.LifecycleControlResult{}, err
	}
	return owner.Approve(ctx, id, request)
}
func (s *Service) RetryDispatch(ctx context.Context, id string, request fse.RetryDispatchRequest) (fse.LifecycleControlResult, error) {
	owner, err := s.handedOffOwnerForSessionOperation(id)
	if err != nil {
		return fse.LifecycleControlResult{}, err
	}
	return owner.RetryDispatch(ctx, id, request)
}
func (s *Service) InterruptDispatch(ctx context.Context, id string, request fse.InterruptDispatchRequest) (fse.LifecycleControlResult, error) {
	owner, err := s.handedOffOwnerForSessionOperation(id)
	if err != nil {
		return fse.LifecycleControlResult{}, err
	}
	return owner.InterruptDispatch(ctx, id, request)
}
func (s *Service) GetResult(ctx context.Context, id string, req fse.ResultRequest) (fse.ResultReadResult, error) {
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
	result := s.projection.Result
	result.Mode = normalized.Mode
	result.IncludeArtifacts = normalized.IncludeArtifacts
	if !normalized.IncludeArtifacts {
		result.ArtifactRefs = nil
	}
	return result, nil
}
func (s *Service) ListDispatches(ctx context.Context, id string) (fse.ListDispatchesResult, error) {
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

func (s *Service) QueryDispatches(ctx context.Context, request fse.DispatchQueryRequest) (fse.ListDispatchesResult, error) {
	if err := s.session(request.SessionID); err != nil {
		return fse.ListDispatchesResult{}, err
	}
	if owner, handedOff := s.handedOffOwnerForSession(request.SessionID); handedOff {
		return owner.QueryDispatches(ctx, request)
	}
	result, err := s.ListDispatches(ctx, request.SessionID)
	if err != nil {
		return fse.ListDispatchesResult{}, err
	}
	return fse.FilterDispatches(result, request.Filters)
}
func (s *Service) GetDispatch(ctx context.Context, id, dispatchID string) (fse.DispatchDetail, error) {
	if err := s.session(id); err != nil {
		return fse.DispatchDetail{}, err
	}
	if owner, handedOff := s.handedOffOwnerForSession(id); handedOff {
		return owner.GetDispatch(ctx, id, dispatchID)
	}
	return fse.DispatchDetail{}, fse.ErrDispatchNotFound
}
func (s *Service) ListArtifacts(ctx context.Context, id string) (fse.ListArtifactsResult, error) {
	if err := s.session(id); err != nil {
		return fse.ListArtifactsResult{}, err
	}
	if owner, handedOff := s.handedOffOwnerForSession(id); handedOff {
		return owner.ListArtifacts(ctx, id)
	}
	return s.projection.Artifacts, nil
}
func (s *Service) GetArtifact(ctx context.Context, id, artifactID string) (fse.ArtifactDetail, error) {
	if err := s.session(id); err != nil {
		return fse.ArtifactDetail{}, err
	}
	if owner, handedOff := s.handedOffOwnerForSession(id); handedOff {
		return owner.GetArtifact(ctx, id, artifactID)
	}
	for _, artifact := range s.projection.Artifacts.Artifacts {
		if artifact.ID == artifactID {
			return fse.ArtifactDetail{ArtifactSummary: artifact, SessionID: id}, nil
		}
	}
	return fse.ArtifactDetail{}, fse.ErrArtifactNotFound
}
func (s *Service) ReadEvents(ctx context.Context, id string, req fse.EventReconnectRequest) (fse.EventReadResult, error) {
	if err := s.session(id); err != nil {
		return fse.EventReadResult{}, err
	}
	if owner, handedOff := s.handedOffOwnerForSession(id); handedOff {
		return owner.ReadEvents(ctx, id, req)
	}
	events, err := fse.FilterEventsAfterReconnect(s.projection.Events.Events, req, id)
	return fse.EventReadResult{SessionID: id, Events: events}, err
}
func (s *Service) ListSessions(ctx context.Context, request fse.ListSessionsRequest) (fse.ListSessionsResult, error) {
	if owner, handedOff := s.handedOffOwner(); handedOff {
		return owner.ListSessions(ctx, request)
	}
	session := s.projection.Session
	return fse.ListSessionsResult{DurableSessions: []fse.DurableSessionListSummary{{SessionID: session.SessionID, Status: session.Status, OrchestratorKind: session.OrchestratorKind, ResolvedSource: session.ResolvedSource, SourceHash: session.SourceHash, Policy: session.Policy, ResultSummary: session.ResultSummary, ArtifactCount: session.ArtifactCount, Lifecycle: session.Lifecycle, Links: session.Links}}}, nil
}
