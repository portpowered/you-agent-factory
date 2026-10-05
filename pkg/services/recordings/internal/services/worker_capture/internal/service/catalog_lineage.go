package service

import (
	"encoding/json"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

type capturedLineageIdentity struct{ factory, worker string }

type capturedSuccessor struct {
	catalog recordings.WorkerSessionCatalogEntry
	attempt string
}

// indexSuccessorOpening requires catalogMu and the recording append barrier.
// Only the immutable, synchronized admission establishes reverse lineage.
func (writer *FileWriter) indexSuccessorOpening(session *recordingSession, catalog recordings.WorkerSessionCatalogEntry) {
	opening, ok := capturedOpeningPayload(session)
	if !ok || opening.Lineage == nil || opening.Lineage.PredecessorWorkerSessionID == "" ||
		opening.Lineage.PredecessorWorkerSessionID == catalog.WorkerSessionID {
		return
	}
	key := capturedLineageIdentity{catalog.FactorySessionID, opening.Lineage.PredecessorWorkerSessionID}
	if writer.successors == nil {
		writer.successors = make(map[capturedLineageIdentity]map[string]capturedSuccessor)
	}
	if writer.successors[key] == nil {
		writer.successors[key] = make(map[string]capturedSuccessor)
	}
	writer.successors[key][catalog.WorkerSessionID] = capturedSuccessor{catalog, opening.Lineage.PreviousAttemptID}
}

func capturedOpeningPayload(session *recordingSession) (workers.SessionPayload, bool) {
	var draft workers.Draft
	var opening workers.SessionPayload
	if len(session.records) == 0 || json.Unmarshal(session.records[0].Payload, &draft) != nil ||
		draft.Kind != workers.KindSession || draft.Phase != workers.PhaseStarted || json.Unmarshal(draft.Payload, &opening) != nil ||
		opening.WorkerSessionID != session.projection.WorkerSessionID {
		return opening, false
	}
	return opening, true
}

// capturedSuccessor reads the incremental reverse index without scanning
// journals or acquiring another recording's lock. Its caller holds the source
// append barrier; catalogMu never covers filesystem effects.
func (writer *FileWriter) capturedSuccessor(session *recordingSession, catalog recordings.WorkerSessionCatalogEntry) (string, error) {
	opening, ok := capturedOpeningPayload(session)
	if !ok {
		return "", recordings.ErrWorkerRecordingReplay
	}
	writer.catalogMu.Lock()
	defer writer.catalogMu.Unlock()
	result := ""
	for id, candidate := range writer.successors[capturedLineageIdentity{catalog.FactorySessionID, catalog.WorkerSessionID}] {
		if candidate.attempt != "" && candidate.attempt != opening.AttemptID {
			continue
		}
		current, exists := writer.catalog[id]
		if !exists || current.RecordingID != candidate.catalog.RecordingID || current.RecordingGenerationID != candidate.catalog.RecordingGenerationID ||
			current.FactorySessionID != catalog.FactorySessionID || result != "" {
			return "", recordings.ErrWorkerRecordingReplay
		}
		result = id
	}
	return result, nil
}
