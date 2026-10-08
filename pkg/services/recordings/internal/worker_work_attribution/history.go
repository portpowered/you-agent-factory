package workerworkattribution

import (
	"context"
	"crypto/sha256"
	"errors"
	"github.com/google/uuid"
	"os"
	"sync"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

// CanonicalHistoryQuery is the existing Recordings decoder and projection.
type CanonicalHistoryQuery interface {
	ReadHistoricalEvents(recordings.HistoricalRecordingQueryRequest) (recordings.HistoricalRecordingQueryResult, error)
	DecodeHistoricalEvents(recordings.HistoricalRecordingQueryRequest, []byte) (recordings.HistoricalRecordingQueryResult, error)
}

// CurrentBoardArtifact selects only the configured current board in one scope.
// It must not discover recordings or select a sibling/default scope's facts.
type CurrentBoardArtifact func(context.Context, string) (recordings.RecordingArtifactReference, error)

// ArtifactHistoryReader binds captured provenance to the existing history owner.
type ArtifactHistoryReader struct {
	query        CanonicalHistoryQuery
	currentBoard CurrentBoardArtifact
	readFile     recordings.RecordingReadFile
	mu           sync.Mutex
	names        map[historyIdentity]cachedNames
	decodes      map[nameDecodeKey]*nameDecode
}

func NewArtifactHistoryReader(query CanonicalHistoryQuery, currentBoard CurrentBoardArtifact, readFile recordings.RecordingReadFile) *ArtifactHistoryReader {
	return &ArtifactHistoryReader{query: query, currentBoard: currentBoard, readFile: readFile, names: make(map[historyIdentity]cachedNames), decodes: make(map[nameDecodeKey]*nameDecode)}
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

// Retain only names and associations, never events, worlds or source bytes.
// A content digest deliberately catches same-size, same-timestamp replacement.
// The bounded cache still reads bytes for freshness; startup-maintained source
// revisions are a separate prerequisite for eliminating that remaining IO.
const maxNameArtifacts = 64

type cachedNames struct {
	digest     [sha256.Size]byte
	projection nameProjection
}

type nameDecodeKey struct {
	identity historyIdentity
	digest   [sha256.Size]byte
}

type nameDecode struct {
	done       chan struct{}
	projection nameProjection
	err        error
}

func (r *ArtifactHistoryReader) readWorkerFactoryNames(ctx context.Context, page recordings.WorkerCapturedActivityPage) (nameProjection, error) {
	if err := ctx.Err(); err != nil {
		return nameProjection{}, err
	}
	artifact := recordings.RecordingArtifactReference(page.Catalog.OriginatingArtifact)
	if artifact == "" {
		var err error
		artifact, err = r.currentBoard(ctx, page.Catalog.FactorySessionID)
		if err != nil {
			return nameProjection{}, err
		}
	}
	identity := recordings.HistoricalRecordingIdentity{RecordingID: recordings.RecordingID(page.Catalog.RecordingID), Artifact: artifact, Scope: recordings.CanonicalEventScope{FactorySessionID: page.Catalog.FactorySessionID}}
	if artifact == "" {
		return nameProjection{}, &recordings.HistoricalRecordingQueryError{Kind: recordings.HistoricalRecordingQueryErrorMissingHistory, RecordingID: identity.RecordingID}
	}
	payload, err := r.readFile(string(artifact))
	if canceled := ctx.Err(); canceled != nil {
		return nameProjection{}, canceled
	}
	if err != nil {
		kind := recordings.HistoricalRecordingQueryErrorUnavailable
		if errors.Is(err, os.ErrNotExist) {
			kind = recordings.HistoricalRecordingQueryErrorMissingHistory
		}
		return nameProjection{}, &recordings.HistoricalRecordingQueryError{Kind: kind, RecordingID: identity.RecordingID, Cause: err}
	}
	key := historyIdentity{page.Catalog.FactorySessionID, page.Catalog.RecordingID, page.Catalog.RecordingGenerationID, string(artifact)}
	digest := sha256.Sum256(payload)
	return r.sharedNames(ctx, nameDecodeKey{key, digest}, identity, payload)
}

// The leading caller owns the synchronous decode; no detached work needs a
// lifecycle join. Waiters can cancel independently. If the leader is canceled,
// a surviving waiter retries with its own context and already-read snapshot.
// Different source digests never share results, even at the same artifact path.
func (r *ArtifactHistoryReader) sharedNames(ctx context.Context, key nameDecodeKey, identity recordings.HistoricalRecordingIdentity, payload []byte) (nameProjection, error) {
	for {
		if err := ctx.Err(); err != nil {
			return nameProjection{}, err
		}
		r.mu.Lock()
		if cached, ok := r.names[key.identity]; ok && cached.digest == key.digest {
			r.mu.Unlock()
			return cached.projection, ctx.Err()
		}
		if pending, ok := r.decodes[key]; ok {
			r.mu.Unlock()
			select {
			case <-ctx.Done():
				return nameProjection{}, ctx.Err()
			case <-pending.done:
				if errors.Is(pending.err, context.Canceled) || errors.Is(pending.err, context.DeadlineExceeded) {
					continue
				}
				if err := ctx.Err(); err != nil {
					return nameProjection{}, err
				}
				return pending.projection, pending.err
			}
		}
		pending := &nameDecode{done: make(chan struct{})}
		r.decodes[key] = pending
		r.mu.Unlock()

		projection, err := r.decodeNames(ctx, identity, payload)
		r.mu.Lock()
		if err == nil {
			if len(r.names) >= maxNameArtifacts {
				clear(r.names)
			}
			r.names[key.identity] = cachedNames{digest: key.digest, projection: projection}
		}
		pending.projection, pending.err = projection, err
		delete(r.decodes, key)
		close(pending.done)
		r.mu.Unlock()
		return projection, err
	}
}

func (r *ArtifactHistoryReader) decodeNames(ctx context.Context, identity recordings.HistoricalRecordingIdentity, payload []byte) (nameProjection, error) {
	history, err := r.query.DecodeHistoricalEvents(recordings.HistoricalRecordingQueryRequest{Recording: identity, InferFactorySessionScope: true}, payload)
	if err != nil {
		return nameProjection{}, err
	}
	projection, err := scopedNames(history, identity.Scope.FactorySessionID)
	if err != nil {
		return nameProjection{}, err
	}
	if err := ctx.Err(); err != nil {
		return nameProjection{}, err
	}
	return projection, nil
}

// The source-native ~default alias is accepted only for a canonical captured
// UUID and the selected artifact. Explicit foreign scopes remain invalid.
func scopedNames(history recordings.HistoricalRecordingQueryResult, factory string) (nameProjection, error) {
	source := history.Recording.Scope.FactorySessionID
	if source != factory {
		if _, err := uuid.Parse(factory); err != nil || source != "~default" {
			return nameProjection{}, recordings.ErrInvalidProjectionScope
		}
	}
	projection, err := projectNames(history, source)
	projection.reportedDefault = source != factory
	return projection, err
}
