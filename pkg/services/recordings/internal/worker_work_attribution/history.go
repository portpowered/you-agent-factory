package workerworkattribution

import (
	"context"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

// CanonicalHistoryQuery is the existing Recordings decoder and projection.
type CanonicalHistoryQuery interface {
	ReadHistoricalEvents(recordings.HistoricalRecordingQueryRequest) (recordings.HistoricalRecordingQueryResult, error)
}

// CurrentBoardArtifact selects only the configured current board in one scope.
// It must not discover recordings or select a sibling/default scope's facts.
type CurrentBoardArtifact func(context.Context, string) (recordings.RecordingArtifactReference, error)

// ArtifactHistoryReader binds captured provenance to the existing history owner.
type ArtifactHistoryReader struct {
	query        CanonicalHistoryQuery
	currentBoard CurrentBoardArtifact
}

func NewArtifactHistoryReader(query CanonicalHistoryQuery, currentBoard CurrentBoardArtifact) *ArtifactHistoryReader {
	return &ArtifactHistoryReader{query: query, currentBoard: currentBoard}
}

func (r *ArtifactHistoryReader) ReadWorkerFactoryHistory(ctx context.Context, page recordings.WorkerCapturedActivityPage) (recordings.HistoricalRecordingQueryResult, error) {
	if err := ctx.Err(); err != nil {
		return recordings.HistoricalRecordingQueryResult{}, err
	}
	artifact := recordings.RecordingArtifactReference(page.Catalog.OriginatingArtifact)
	if artifact == "" {
		var err error
		artifact, err = r.currentBoard(ctx, page.Catalog.FactorySessionID)
		if err != nil {
			return recordings.HistoricalRecordingQueryResult{}, err
		}
	}
	if artifact == "" {
		return recordings.HistoricalRecordingQueryResult{}, &recordings.HistoricalRecordingQueryError{Kind: recordings.HistoricalRecordingQueryErrorMissingHistory, RecordingID: recordings.RecordingID(page.Catalog.RecordingID)}
	}
	result, err := r.query.ReadHistoricalEvents(recordings.HistoricalRecordingQueryRequest{InferFactorySessionScope: true, Recording: recordings.HistoricalRecordingIdentity{
		RecordingID: recordings.RecordingID(page.Catalog.RecordingID), Artifact: artifact,
		Scope: recordings.CanonicalEventScope{FactorySessionID: page.Catalog.FactorySessionID},
	}})
	if canceled := ctx.Err(); canceled != nil {
		return recordings.HistoricalRecordingQueryResult{}, canceled
	}
	return result, err
}
