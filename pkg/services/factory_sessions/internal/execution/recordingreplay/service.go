package recordingreplay

import (
	"context"
	"errors"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	fse "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution"
)

// ErrNonLiveReplay reports an operation that would require live execution.
var ErrNonLiveReplay = errors.New("recorded Factory Sessions are historical and do not support live execution")

// Service exposes one validated recording through the canonical public read contract.
type Service struct{ projection RecordingReplayProjection }

func NewService(projection RecordingReplayProjection) *Service {
	return &Service{projection: projection}
}

// Inspection returns the complete public read model for the recording. The
// caller receives only the bounded facts already restored by ReplayRecording;
// no live execution or mutable checkpoint state is exposed.
func (s *Service) Inspection() factorysessions.HistoricalReplayInspection {
	if s == nil {
		return factorysessions.HistoricalReplayInspection{}
	}
	inspection := factorysessions.HistoricalReplayInspection{
		Session:   s.projection.Session,
		Events:    s.projection.Events,
		Artifacts: s.projection.Artifacts,
		Result:    s.projection.Result,
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
func (*Service) IsNonLiveReplay() bool { return true }

var _ fse.Service = (*Service)(nil)

func (s *Service) session(sessionID string) error {
	if s == nil || sessionID != s.projection.Session.SessionID {
		return fse.ErrSessionNotFound
	}
	return nil
}
func (*Service) StartAsync(context.Context, fse.StartRequest) (fse.AsyncStartResult, error) {
	return fse.AsyncStartResult{}, ErrNonLiveReplay
}
func (*Service) StartSync(context.Context, fse.StartRequest) (fse.SyncStartResult, error) {
	return fse.SyncStartResult{}, ErrNonLiveReplay
}
func (*Service) ResumeInterruptedSession(context.Context, string, fse.ResumeSessionRequest) (fse.AsyncStartResult, error) {
	return fse.AsyncStartResult{}, ErrNonLiveReplay
}
func (s *Service) GetSession(_ context.Context, id string) (fse.SessionReadResult, error) {
	if err := s.session(id); err != nil {
		return fse.SessionReadResult{}, err
	}
	return s.projection.Session, nil
}
func (*Service) Pause(context.Context, string, fse.ControlRequest) (fse.LifecycleControlResult, error) {
	return fse.LifecycleControlResult{}, ErrNonLiveReplay
}
func (*Service) Resume(context.Context, string, fse.ControlRequest) (fse.LifecycleControlResult, error) {
	return fse.LifecycleControlResult{}, ErrNonLiveReplay
}
func (*Service) Cancel(context.Context, string, fse.ControlRequest) (fse.LifecycleControlResult, error) {
	return fse.LifecycleControlResult{}, ErrNonLiveReplay
}
func (*Service) Terminate(context.Context, string, fse.ControlRequest) (fse.LifecycleControlResult, error) {
	return fse.LifecycleControlResult{}, ErrNonLiveReplay
}
func (*Service) Approve(context.Context, string, fse.ApproveRequest) (fse.LifecycleControlResult, error) {
	return fse.LifecycleControlResult{}, ErrNonLiveReplay
}
func (*Service) RetryDispatch(context.Context, string, fse.RetryDispatchRequest) (fse.LifecycleControlResult, error) {
	return fse.LifecycleControlResult{}, ErrNonLiveReplay
}
func (*Service) InterruptDispatch(context.Context, string, fse.InterruptDispatchRequest) (fse.LifecycleControlResult, error) {
	return fse.LifecycleControlResult{}, ErrNonLiveReplay
}
func (s *Service) GetResult(_ context.Context, id string, req fse.ResultRequest) (fse.ResultReadResult, error) {
	if err := s.session(id); err != nil {
		return fse.ResultReadResult{}, err
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
func (s *Service) ListDispatches(_ context.Context, id string) (fse.ListDispatchesResult, error) {
	if err := s.session(id); err != nil {
		return fse.ListDispatchesResult{}, err
	}
	return fse.ListDispatchesResult{SessionID: id}, nil
}

func (s *Service) QueryDispatches(ctx context.Context, request fse.DispatchQueryRequest) (fse.ListDispatchesResult, error) {
	result, err := s.ListDispatches(ctx, request.SessionID)
	if err != nil {
		return fse.ListDispatchesResult{}, err
	}
	return fse.FilterDispatches(result, request.Filters)
}
func (s *Service) GetDispatch(_ context.Context, id, _ string) (fse.DispatchDetail, error) {
	if err := s.session(id); err != nil {
		return fse.DispatchDetail{}, err
	}
	return fse.DispatchDetail{}, fse.ErrDispatchNotFound
}
func (s *Service) ListArtifacts(_ context.Context, id string) (fse.ListArtifactsResult, error) {
	if err := s.session(id); err != nil {
		return fse.ListArtifactsResult{}, err
	}
	return s.projection.Artifacts, nil
}
func (s *Service) GetArtifact(_ context.Context, id, artifactID string) (fse.ArtifactDetail, error) {
	if err := s.session(id); err != nil {
		return fse.ArtifactDetail{}, err
	}
	for _, artifact := range s.projection.Artifacts.Artifacts {
		if artifact.ID == artifactID {
			return fse.ArtifactDetail{ArtifactSummary: artifact, SessionID: id}, nil
		}
	}
	return fse.ArtifactDetail{}, fse.ErrArtifactNotFound
}
func (s *Service) ReadEvents(_ context.Context, id string, req fse.EventReconnectRequest) (fse.EventReadResult, error) {
	if err := s.session(id); err != nil {
		return fse.EventReadResult{}, err
	}
	events, err := fse.FilterEventsAfterReconnect(s.projection.Events.Events, req, id)
	return fse.EventReadResult{SessionID: id, Events: events}, err
}
func (s *Service) ListSessions(context.Context, fse.ListSessionsRequest) (fse.ListSessionsResult, error) {
	session := s.projection.Session
	return fse.ListSessionsResult{DurableSessions: []fse.DurableSessionListSummary{{SessionID: session.SessionID, Status: session.Status, OrchestratorKind: session.OrchestratorKind, ResolvedSource: session.ResolvedSource, SourceHash: session.SourceHash, Policy: session.Policy, ResultSummary: session.ResultSummary, ArtifactCount: session.ArtifactCount, Lifecycle: session.Lifecycle, Links: session.Links}}}, nil
}
