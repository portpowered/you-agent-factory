package service

import (
	"context"

	chatsessions "github.com/portpowered/infinite-you/pkg/services/chat_sessions"
	"github.com/portpowered/infinite-you/pkg/services/events"
)

// initialSessionVersion is the Version a newly created Session carries. A
// caller's first mutation attempt therefore supplies this exact value as
// ExpectedVersion.
const initialSessionVersion uint64 = 1

// initialTargetEpisodeNumber is the TargetEpisode number a newly created
// Session's first, still-open episode carries.
const initialTargetEpisodeNumber uint64 = 1

// CreateSession validates req in full before any state is touched, so an
// invalid RequestID, WorkingRoot, or InitialTarget creates no observable session.
// newID and now are only ever called while s.mu is held (see the write-lock
// below), so CreateSession is safe under concurrent calls even when the
// injected IDGenerator/Clock are not themselves safe for concurrent use --
// Store does not require its dependencies to be concurrency-safe by
// contract, it serializes access to them itself.
func (s *Store) CreateSession(_ context.Context, req chatsessions.CreateSessionRequest) (result chatsessions.CreateSessionResult, err error) {
	s.logStart("CreateSession", "")
	defer func() {
		s.logOutcome("CreateSession", result.Session.ID, err, "version", result.Session.Version, "target_episode", result.Session.TargetEpisode)
	}()
	if err := req.RequestID.Validate(); err != nil {
		return chatsessions.CreateSessionResult{}, err
	}
	if req.WorkingRoot == "" {
		return chatsessions.CreateSessionResult{}, &chatsessions.ValidationError{
			Value: "CreateSessionRequest", Field: "WorkingRoot", Err: chatsessions.ErrRequiredValue,
		}
	}
	if err := req.InitialTarget.Validate(); err != nil {
		return chatsessions.CreateSessionResult{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.now()
	session := chatsessions.Session{
		ID:             s.newID(),
		State:          chatsessions.SessionStateCreated,
		WorkingRoot:    req.WorkingRoot,
		SelectedTarget: req.InitialTarget,
		TargetEpisode:  initialTargetEpisodeNumber,
		Version:        initialSessionVersion,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := session.Validate(); err != nil {
		return chatsessions.CreateSessionResult{}, err
	}
	episode := chatsessions.TargetEpisode{
		Number:    initialTargetEpisodeNumber,
		State:     chatsessions.TargetEpisodeStateOpen,
		Target:    req.InitialTarget,
		StartedAt: now,
	}
	if err := episode.Validate(); err != nil {
		return chatsessions.CreateSessionResult{}, err
	}

	s.sessions[session.ID] = sessionRecord{
		session:            session,
		episodes:           []chatsessions.TargetEpisode{episode},
		turns:              make(map[string]chatsessions.Turn),
		turnsByRequest:     make(map[chatsessions.RequestIdentity]string),
		attachments:        make(map[string]chatsessions.Attachment),
		controls:           make(map[chatsessions.RequestIdentity]chatsessions.ControlIntent),
		sequencedItemIDs:   make(map[string]struct{}),
		sequencedPositions: make(map[events.AggregateSequence]sequencedSourceIdentity),
		sequencedBySource:  make(map[sequencedSourceIdentity]sequencedRecord),
	}
	return chatsessions.CreateSessionResult{Session: session}, nil
}

// GetSession returns a detached copy of the current Session state. It
// reports *NotFoundError when SessionID does not identify an existing
// session and creates no placeholder history for an unknown ID.
func (s *Store) GetSession(_ context.Context, req chatsessions.GetSessionRequest) (result chatsessions.GetSessionResult, err error) {
	s.logStart("GetSession", req.SessionID)
	defer func() {
		s.logOutcome("GetSession", req.SessionID, err, "version", result.Session.Version, "target_episode", result.Session.TargetEpisode)
	}()
	s.mu.RLock()
	defer s.mu.RUnlock()
	record, ok := s.sessions[req.SessionID]
	if !ok {
		return chatsessions.GetSessionResult{}, &chatsessions.NotFoundError{Value: "Session", ID: req.SessionID}
	}
	return chatsessions.GetSessionResult{
		Session:          record.session,
		Episode:          record.episodes[len(record.episodes)-1],
		MostRecentTurnID: record.lastTurnID,
	}, nil
}
