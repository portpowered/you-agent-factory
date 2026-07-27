package service

import (
	"time"

	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
)

func (service *Service) session(
	id recordings.RecordingID,
) (*recordingSession, error) {
	service.mu.Lock()
	defer service.mu.Unlock()
	return service.sessionLocked(id)
}

func (service *Service) startPeriodic(
	id recordings.RecordingID,
	interval time.Duration,
) {
	if service.writer == nil || service.tickers == nil {
		return
	}
	if interval <= 0 {
		interval = recordings.DefaultRecordingFlushInterval
	}
	service.mu.Lock()
	session, err := service.sessionLocked(id)
	if err != nil || session.periodicDone != nil {
		service.mu.Unlock()
		return
	}
	ticker := service.tickers(interval)
	if ticker.Ticks == nil || ticker.Stop == nil {
		service.mu.Unlock()
		return
	}
	session.periodicStop = make(chan struct{})
	session.periodicDone = make(chan struct{})
	stop := session.periodicStop
	done := session.periodicDone
	service.mu.Unlock()

	go func() {
		defer close(done)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case _, open := <-ticker.Ticks:
				if !open {
					return
				}
				_ = service.flush(id, session)
			}
		}
	}()
}

func (service *Service) flush(
	id recordings.RecordingID,
	session *recordingSession,
) error {
	session.flushMu.Lock()
	defer session.flushMu.Unlock()

	service.mu.Lock()
	if session.flushedVersion == session.version {
		service.mu.Unlock()
		return nil
	}
	version := session.version
	snapshot := recordings.RecordingSnapshot{
		Status: recordingStatus(id, session),
		Events: append([]recordings.CanonicalEvent(nil), session.events...),
	}
	target := session.serviceTarget
	var through *recordings.CanonicalEventCursor
	if len(snapshot.Events) > 0 {
		cursor := snapshot.Events[len(snapshot.Events)-1].Cursor
		through = &cursor
	}
	service.mu.Unlock()

	if service.writer != nil {
		if err := service.writer(target, snapshot); err != nil {
			return err
		}
	}

	service.mu.Lock()
	if session.flushedVersion < version {
		session.flushedVersion = version
		session.flushedThrough = through
	}
	service.mu.Unlock()
	return nil
}
