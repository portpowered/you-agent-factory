package workerworkattribution

import (
	"context"
	"encoding/json"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// PrepareWorkerWorkAttribution warms derived names from the same committed
// catalog hydrated by owner recovery. It performs no journal scan or writes.
// Optional history failures remain read-time facts, never startup failures or
// negative cache entries. Cancellation still belongs to the activating caller.
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
	_, _ = query.projection(ctx, page, request.FactorySessionID)
}
