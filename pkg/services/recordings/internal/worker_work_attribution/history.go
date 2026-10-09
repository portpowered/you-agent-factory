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
	revision     func(string) (string, error)
	mu           sync.Mutex
	names        map[historyIdentity]cachedNames
	decodes      map[nameDecodeKey]*nameDecode
	reads        map[historyIdentity]*artifactRead
	nameUse      uint64
}

func NewArtifactHistoryReader(revision func(string) (string, error), query CanonicalHistoryQuery, currentBoard CurrentBoardArtifact, readFile recordings.RecordingReadFile) *ArtifactHistoryReader {
	return &ArtifactHistoryReader{query: query, currentBoard: currentBoard, readFile: readFile, revision: revision, names: make(map[historyIdentity]cachedNames), decodes: make(map[nameDecodeKey]*nameDecode), reads: make(map[historyIdentity]*artifactRead)}
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

// Retain bounded compact names and associations, never events, worlds or source bytes.
// Capacity covers thousands of distinct profile sources without startup/list churn.
// A content digest deliberately catches same-size, same-timestamp replacement.
// A supported filesystem revision avoids rereading unchanged sources. Without
// a reliable revision edge, reads still establish freshness through the digest.
const maxNameArtifacts = 4096

type cachedNames struct {
	digest     [sha256.Size]byte
	projection nameProjection
	lastUse    uint64
	revision   string
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

type artifactRead struct {
	done    chan struct{}
	payload []byte
	digest  [sha256.Size]byte
	err     error
}

func (r *ArtifactHistoryReader) readWorkerFactoryNames(ctx context.Context, page recordings.WorkerCapturedActivityPage) (nameProjection, error) {
	if err := ctx.Err(); err != nil {
		return nameProjection{}, err
	}
	artifact, err := r.workerFactoryArtifact(ctx, page)
	if err != nil {
		return nameProjection{}, err
	}
	identity := recordings.HistoricalRecordingIdentity{RecordingID: recordings.RecordingID(page.Catalog.RecordingID), Artifact: artifact, Scope: recordings.CanonicalEventScope{FactorySessionID: page.Catalog.FactorySessionID}}
	if artifact == "" {
		return nameProjection{}, &recordings.HistoricalRecordingQueryError{Kind: recordings.HistoricalRecordingQueryErrorMissingHistory, RecordingID: identity.RecordingID}
	}
	key := historyIdentity{page.Catalog.FactorySessionID, page.Catalog.RecordingID, page.Catalog.RecordingGenerationID, string(artifact)}
	revision, err := r.sourceRevision(key)
	if err == nil && revision != "" {
		if projection, ok := r.revisionProjection(key, revision); ok {
			return projection, ctx.Err()
		}
	}
	var read *artifactRead
	if err == nil {
		read, err = r.sharedArtifact(ctx, key)
	}
	if err != nil {
		if canceled := ctx.Err(); canceled != nil {
			return nameProjection{}, canceled
		}
		kind := recordings.HistoricalRecordingQueryErrorUnavailable
		if errors.Is(err, os.ErrNotExist) {
			kind = recordings.HistoricalRecordingQueryErrorMissingHistory
		}
		return nameProjection{}, &recordings.HistoricalRecordingQueryError{Kind: kind, RecordingID: identity.RecordingID, Cause: err}
	}
	projection, err := r.sharedNames(ctx, nameDecodeKey{key, read.digest}, identity, read.payload)
	if err == nil && revision != "" {
		r.retainSourceRevision(key, read.digest, revision)
	}
	if canceled := ctx.Err(); canceled != nil {
		return nameProjection{}, canceled
	}
	return projection, err
}

// Resolve legacy selection before request-local sharing. The caller keeps the
// original capture for legacy association validation; this is only the exact
// source selected for this read, never persisted provenance.
func (r *ArtifactHistoryReader) workerFactoryArtifact(ctx context.Context, page recordings.WorkerCapturedActivityPage) (recordings.RecordingArtifactReference, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if artifact := page.Catalog.OriginatingArtifact; artifact != "" {
		return recordings.RecordingArtifactReference(artifact), nil
	}
	artifact, err := r.currentBoard(ctx, page.Catalog.FactorySessionID)
	if err != nil {
		return "", err
	}
	if artifact == "" {
		return "", &recordings.HistoricalRecordingQueryError{Kind: recordings.HistoricalRecordingQueryErrorMissingHistory, RecordingID: recordings.RecordingID(page.Catalog.RecordingID)}
	}
	return artifact, nil
}

// Share only an in-flight source read, not a completed source snapshot. Each
// fresh query still observes replacements and temporary unavailability. Bytes
// are owned by the participating calls and never retained in the name cache.
// The synchronous leader owns the IO; canceled waiters leave independently and
// surviving waiters retry a canceled leader with a new source snapshot.
// Capture generations identify Worker attempts, not Factory source revisions.
// Share IO only for the exact scoped recording/artifact; each caller validates
// its own opening and association before using the resulting projection.
func (r *ArtifactHistoryReader) sharedArtifact(ctx context.Context, key historyIdentity) (*artifactRead, error) {
	key.generation = ""
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		r.mu.Lock()
		if pending, ok := r.reads[key]; ok {
			r.mu.Unlock()
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-pending.done:
				if errors.Is(pending.err, context.Canceled) || errors.Is(pending.err, context.DeadlineExceeded) {
					continue
				}
				if err := ctx.Err(); err != nil {
					return nil, err
				}
				return pending, pending.err
			}
		}
		pending := &artifactRead{done: make(chan struct{})}
		r.reads[key] = pending
		r.mu.Unlock()

		pending.payload, pending.err = r.readFile(key.artifact)
		if pending.err == nil {
			pending.digest = sha256.Sum256(pending.payload)
		}
		if err := ctx.Err(); err != nil {
			pending.payload, pending.err = nil, err
		}
		r.mu.Lock()
		delete(r.reads, key)
		close(pending.done)
		r.mu.Unlock()
		return pending, pending.err
	}
}

// The leading caller owns the synchronous decode; no detached work needs a
// lifecycle join. Waiters can cancel independently. If the leader is canceled,
// a surviving waiter retries with its own context and already-read snapshot.
// Different source digests never share results, even at the same artifact path.
func (r *ArtifactHistoryReader) sharedNames(ctx context.Context, key nameDecodeKey, identity recordings.HistoricalRecordingIdentity, payload []byte) (nameProjection, error) {
	// Attempts sharing one exact scoped Factory snapshot decode the same facts.
	// Source reads coalesce across attempts only while in flight; this reduction
	// also shares completed facts by digest. Every caller validates its opening
	// and association separately before attributing a name.
	key.identity.generation = ""
	for {
		if err := ctx.Err(); err != nil {
			return nameProjection{}, err
		}
		r.mu.Lock()
		if projection, ok := r.cachedProjection(key); ok {
			r.mu.Unlock()
			return projection, ctx.Err()
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
			r.retainProjection(key, projection)
		}
		pending.projection, pending.err = projection, err
		delete(r.decodes, key)
		close(pending.done)
		r.mu.Unlock()
		return projection, err
	}
}

// Caller holds mu. The decoder consumes Factory scope, recording, artifact and
// bytes, not the capture generation. Reuse only after a fresh source read for
// the exact scoped artifact and an equal digest. Each caller validates its own
// opening and association in attributionQuery; no capture facts are borrowed.
// Keeping one entry per source avoids eviction churn from many Worker attempts.
func (r *ArtifactHistoryReader) cachedProjection(key nameDecodeKey) (nameProjection, bool) {
	cached, ok := r.names[key.identity]
	if ok && cached.digest == key.digest {
		r.nameUse++
		cached.lastUse = r.nameUse
		r.names[key.identity] = cached
		return cached.projection, true
	}
	return nameProjection{}, false
}

// Caller holds mu. Replace revisions of the same exact source, and evict only
// the least recently used source when full, preserving other warm histories.
func (r *ArtifactHistoryReader) retainProjection(key nameDecodeKey, projection nameProjection) {
	if _, exists := r.names[key.identity]; !exists && len(r.names) >= maxNameArtifacts {
		var oldest historyIdentity
		var oldestUse uint64
		for identity, cached := range r.names {
			if oldestUse == 0 || cached.lastUse < oldestUse {
				oldest, oldestUse = identity, cached.lastUse
			}
		}
		delete(r.names, oldest)
	}
	r.nameUse++
	r.names[key.identity] = cachedNames{digest: key.digest, projection: projection, lastUse: r.nameUse}
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
