package service

import (
	"errors"
	"strings"
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
			case tick, open := <-ticker.Ticks:
				if !open {
					return
				}
				_ = service.flush(id, session, flushAttempt{
					kind:       flushPeriodic,
					recordedAt: tick.UTC(),
				})
			}
		}
	}()
}

type flushKind string

const (
	flushActive   flushKind = "active"
	flushPeriodic flushKind = "periodic"
	flushFinal    flushKind = "final"

	activeFlushFailureCode      = "active_flush_failed"
	periodicFlushFailureCode    = "periodic_flush_failed"
	finalFlushFailureCode       = "final_flush_failed"
	snapshotEncodingFailureCode = "snapshot_encoding_failed"
	terminalMetadataFailureCode = "terminal_metadata_failed"
)

type flushAttempt struct {
	kind       flushKind
	recordedAt time.Time
}

func (service *Service) flush(
	id recordings.RecordingID,
	session *recordingSession,
	attempt flushAttempt,
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
		Status:             recordingStatus(id, session),
		Events:             append([]recordings.CanonicalEvent(nil), session.events...),
		CanonicalSessionID: strings.TrimSpace(session.selection.CanonicalSessionID),
		SecretProvenance:   cloneSecretProvenance(session.secretProvenance),
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
			service.mu.Lock()
			service.recordFailureLocked(session, recordings.RecordingFailure{
				Code:       flushFailureCode(attempt.kind, err),
				Message:    flushFailureMessage(attempt.kind, target, err),
				RecordedAt: attempt.recordedAt,
			}, err)
			service.mu.Unlock()
			return err
		}
	}

	service.mu.Lock()
	if session.flushedVersion < version {
		session.flushedVersion = version
		session.flushedThrough = through
		if service.writer != nil {
			service.advanceDurableThroughLocked(through)
		}
	}
	service.mu.Unlock()
	return nil
}

func (service *Service) advanceDurableThroughLocked(
	through *recordings.CanonicalEventCursor,
) {
	if through == nil || strings.TrimSpace(through.StreamGenerationID) == "" || through.Sequence < 0 {
		return
	}
	if service.durableThrough == nil {
		service.durableThrough = make(map[string]recordings.CanonicalEventCursor)
	}
	generationID := strings.TrimSpace(through.StreamGenerationID)
	current, exists := service.durableThrough[generationID]
	if !exists || through.Sequence > current.Sequence {
		cursor := *through
		cursor.StreamGenerationID = generationID
		service.durableThrough[generationID] = cursor
	}
}

func flushFailureCode(kind flushKind, err error) string {
	if errors.Is(err, recordings.ErrRecordingSnapshotEncoding) {
		return snapshotEncodingFailureCode
	}
	switch kind {
	case flushPeriodic:
		return periodicFlushFailureCode
	case flushFinal:
		return finalFlushFailureCode
	default:
		return activeFlushFailureCode
	}
}

func flushFailureMessage(kind flushKind, target string, err error) string {
	return string(kind) + " flush recording target " + target + ": " + err.Error()
}
