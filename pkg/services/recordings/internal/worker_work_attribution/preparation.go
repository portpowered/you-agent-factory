package workerworkattribution

import (
	"context"
	"encoding/json"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// PrepareWorkerWorkAttribution warms derived names from the same committed
// catalog hydrated by owner recovery. It performs no journal scan or writes.
// Optional history failures leave names absent, never startup failures or
// request-time retries. Cancellation belongs to the activating caller.
func (s *Service) PrepareWorkerWorkAttribution(ctx context.Context) error {
	if ctx == nil {
		return recordings.ErrInvalidProjectionInput
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	catalog, ok := s.captures.(interface {
		ListWorkerSessionCaptures(context.Context, recordings.WorkerCapturedCatalogRequest) (recordings.WorkerCapturedCatalogPage, error)
	})
	if !ok {
		return nil // External writers retain their existing read capabilities.
	}
	query := attributionQuery{service: s, projections: make(map[historyIdentity]projectionResult)}
	request := recordings.WorkerCapturedCatalogRequest{Limit: 1000}
	for {
		page, err := catalog.ListWorkerSessionCaptures(ctx, request)
		if err != nil {
			return ctx.Err() // Preparation must not hide or broaden read-time errors.
		}
		for _, item := range page.Items {
			if err := ctx.Err(); err != nil {
				return err
			}
			query.prepareCapturedNames(ctx, item)
		}
		if page.NextToken == "" {
			return ctx.Err()
		}
		request.NextToken = page.NextToken
	}
}

func (query *attributionQuery) prepareCapturedNames(ctx context.Context, item recordings.WorkerCapturedCatalogItem) {
	var draft workers.Draft
	var opening workers.SessionPayload
	if json.Unmarshal(item.Opening.Payload, &draft) != nil || json.Unmarshal(draft.Payload, &opening) != nil || len(opening.WorkIDs) == 0 {
		return
	}
	page := recordings.WorkerCapturedActivityPage{Catalog: item.Catalog, Opening: item.Opening}
	request := recordings.WorkerWorkAttributionRequest{WorkerSessionID: item.Catalog.WorkerSessionID, FactorySessionID: item.Catalog.FactorySessionID, WorkID: opening.WorkIDs[0]}
	if _, err := validateOpening(page, request); err != nil || request.FactorySessionID == "" || request.WorkID == "" {
		return
	}
	// Warm only the exact validated source. Association/conflict policy still
	// runs independently for every row at the public attribution read boundary.
	if page.Catalog.WorkName != "" || query.service.history == nil {
		return
	}
	projection, err := query.projection(ctx, page, request.FactorySessionID)
	if err != nil || ctx.Err() != nil {
		return
	}
	association, exists := projection.associations[request.WorkerSessionID]
	if !exists || association.dispatch != opening.DispatchID || !containsWork(association.workIDs, request.WorkID) {
		return
	}
	query.service.mu.Lock()
	query.service.prepared[captureIdentity(page)] = projection
	query.service.mu.Unlock()
}

func (q *attributionQuery) projection(ctx context.Context, page recordings.WorkerCapturedActivityPage, factory string) (nameProjection, error) {
	if reader, ok := q.service.history.(interface {
		workerFactoryArtifact(context.Context, recordings.WorkerCapturedActivityPage) (recordings.RecordingArtifactReference, error)
	}); ok {
		artifact, err := reader.workerFactoryArtifact(ctx, page)
		if err != nil {
			return nameProjection{}, err
		}
		page.Catalog.OriginatingArtifact = string(artifact)
	}
	key := historyIdentity{page.Catalog.FactorySessionID, page.Catalog.RecordingID, page.Catalog.RecordingGenerationID, page.Catalog.OriginatingArtifact}
	// Capture generations identify individual Worker attempts, not revisions of
	// their shared Factory artifact. Each opening is validated before reaching
	// here; within this preparation the exact scoped artifact supplies one snapshot
	// of names and associations for all those attempts. Legacy candidates are
	// resolved independently above before sharing the selected exact source.
	// The reader's startup decode cache still uses the complete capture identity.
	if key.artifact != "" {
		key.generation = ""
	}
	if result, loaded := q.projections[key]; loaded {
		return result.projection, result.err
	}
	projection, err := q.loadProjection(ctx, page, factory)
	// Unavailability is one source observation for this preparation, just like a
	// successful projection. A later startup preparation retries it; no negative result is
	// retained in the startup decode cache. Every opening is still validated.
	q.projections[key] = projectionResult{projection, err}
	return projection, err
}

func (q *attributionQuery) loadProjection(ctx context.Context, page recordings.WorkerCapturedActivityPage, factory string) (nameProjection, error) {
	if reader, ok := q.service.history.(interface {
		readWorkerFactoryNames(context.Context, recordings.WorkerCapturedActivityPage) (nameProjection, error)
	}); ok {
		projection, err := reader.readWorkerFactoryNames(ctx, page)
		if err != nil {
			return nameProjection{}, err
		}
		return projection, ctx.Err()
	}
	history, err := q.service.history.ReadWorkerFactoryHistory(ctx, page)
	if canceled := ctx.Err(); canceled != nil {
		return nameProjection{}, canceled
	}
	if err != nil {
		return nameProjection{}, err
	}
	projection, err := scopedNames(history, factory)
	if err != nil {
		return nameProjection{}, err
	}
	return projection, nil
}
