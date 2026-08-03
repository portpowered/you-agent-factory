package service

import (
	"context"

	chatsessions "github.com/portpowered/infinite-you/pkg/services/chat_sessions"
)

// initialSessionVersion is the Version a newly created Session carries. A
// caller's first mutation attempt therefore supplies this exact value as
// ExpectedVersion.
const initialSessionVersion uint64 = 1

// initialTargetEpisodeNumber is the TargetEpisode number a newly created
// Session's first, still-open episode carries.
const initialTargetEpisodeNumber uint64 = 1

// CreateSession validates req in full before any state is touched, so an
// invalid RequestID, Cwd, or InitialTarget creates no observable session.
func (s *Store) CreateSession(_ context.Context, req chatsessions.CreateSessionRequest) (chatsessions.CreateSessionResult, error) {
	if err := req.RequestID.Validate(); err != nil {
		return chatsessions.CreateSessionResult{}, err
	}
	if req.Cwd == "" {
		return chatsessions.CreateSessionResult{}, &chatsessions.ValidationError{
			Value: "CreateSessionRequest", Field: "Cwd", Err: chatsessions.ErrRequiredValue,
		}
	}
	if err := req.InitialTarget.Validate(); err != nil {
		return chatsessions.CreateSessionResult{}, err
	}

	now := s.now()
	session := chatsessions.Session{
		ID:             s.newID(),
		State:          chatsessions.SessionStateCreated,
		Cwd:            req.Cwd,
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

	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[session.ID] = sessionRecord{
		session:     session,
		episodes:    []chatsessions.TargetEpisode{episode},
		turns:       make(map[string]chatsessions.Turn),
		attachments: make(map[string]chatsessions.Attachment),
	}
	return chatsessions.CreateSessionResult{Session: session}, nil
}

// GetSession returns a detached copy of the current Session state. It
// reports *NotFoundError when SessionID does not identify an existing
// session and creates no placeholder history for an unknown ID.
func (s *Store) GetSession(_ context.Context, req chatsessions.GetSessionRequest) (chatsessions.GetSessionResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	record, ok := s.sessions[req.SessionID]
	if !ok {
		return chatsessions.GetSessionResult{}, &chatsessions.NotFoundError{Value: "Session", ID: req.SessionID}
	}
	return chatsessions.GetSessionResult{Session: record.session}, nil
}
