package workersessions

import (
	"context"
	"encoding/base64"
	"sort"
	"strings"
)

// ObservationServiceCatalog resolves the Worker Sessions services currently
// visible in one process. The catalog is evaluated for every read so a
// Factory Session opened after HTTP binding is still part of the fleet view.
type ObservationServiceCatalog func(context.Context) ([]ObservationService, error)

// FleetObservationService merges runtime-owned Worker Session registries into
// one deterministic, bounded identity collection. It intentionally exposes
// only the top-level list capability; lifecycle and Work-scoped operations
// remain bound to their owning runtime/service.
type FleetObservationService struct {
	catalog ObservationServiceCatalog
}

var _ TopLevelObservationService = (*FleetObservationService)(nil)

// NewFleetObservationService constructs a process-wide top-level observation
// view from a dynamic service catalog.
func NewFleetObservationService(catalog ObservationServiceCatalog) *FleetObservationService {
	if catalog == nil {
		return nil
	}
	return &FleetObservationService{catalog: catalog}
}

// ListWorkerSessionObservations returns one globally ordered page across all
// catalogued Worker Session services. Each source is fully walked with the
// request filters before the fleet-level cursor and bound are applied, so a
// page cannot omit or duplicate an observation merely because it belongs to a
// different Factory Session.
func (s *FleetObservationService) ListWorkerSessionObservations(
	ctx context.Context,
	req ListWorkerSessionObservationsRequest,
) (ListWorkerSessionObservationsResult, error) {
	if s == nil || s.catalog == nil {
		return ListWorkerSessionObservationsResult{}, ErrObservationProjectionUnavailable
	}
	if err := req.Validate(); err != nil {
		return ListWorkerSessionObservationsResult{}, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return ListWorkerSessionObservationsResult{}, err
	}

	limit := req.MaxResults
	if limit == 0 {
		limit = DefaultWorkerSessionObservationListMaxResults
	}
	cursor, err := decodeFleetObservationCursor(req.NextToken)
	if err != nil {
		return ListWorkerSessionObservationsResult{}, err
	}

	sources, err := s.catalog(ctx)
	if err != nil {
		return ListWorkerSessionObservationsResult{}, err
	}
	observations, err := collectFleetObservations(ctx, sources, req, limit)
	if err != nil {
		return ListWorkerSessionObservationsResult{}, err
	}

	ids := make([]string, 0, len(observations))
	for id, observation := range observations {
		if id <= cursor || !fleetObservationMatches(observation, req.Scope.Normalized(), req.States) {
			continue
		}
		ids = append(ids, id)
	}
	sort.Strings(ids)

	pageIDs := ids
	if len(pageIDs) > limit {
		pageIDs = pageIDs[:limit]
	}
	page := make([]Observation, 0, len(pageIDs))
	for _, id := range pageIDs {
		page = append(page, observations[id])
	}
	nextToken := ""
	if len(ids) > len(pageIDs) && len(pageIDs) > 0 {
		nextToken = base64.StdEncoding.EncodeToString([]byte(pageIDs[len(pageIDs)-1]))
	}
	return ListWorkerSessionObservationsResult{
		Observations: page,
		MaxResults:   limit,
		NextToken:    nextToken,
	}, nil
}

func collectFleetObservations(
	ctx context.Context,
	sources []ObservationService,
	req ListWorkerSessionObservationsRequest,
	limit int,
) (map[string]Observation, error) {
	observations := make(map[string]Observation)
	for _, source := range sources {
		if source == nil {
			continue
		}
		nextToken := ""
		seenTokens := make(map[string]struct{})
		for {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			sourceRequest := req
			sourceRequest.MaxResults = limit
			sourceRequest.NextToken = nextToken
			result, err := source.ListWorkerSessionObservations(ctx, sourceRequest)
			if err != nil {
				return nil, err
			}
			for _, observation := range result.Observations {
				id := strings.TrimSpace(observation.WorkerSessionID)
				if id == "" {
					continue
				}
				if _, exists := observations[id]; !exists {
					observation.WorkerSessionID = id
					observations[id] = observation
				}
			}
			if strings.TrimSpace(result.NextToken) == "" {
				break
			}
			if _, exists := seenTokens[result.NextToken]; exists || result.NextToken == nextToken {
				return nil, ErrInvalidObservationPagination
			}
			seenTokens[result.NextToken] = struct{}{}
			nextToken = result.NextToken
		}
	}
	return observations, nil
}

func decodeFleetObservationCursor(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	decoded, err := base64.StdEncoding.DecodeString(value)
	if err != nil || strings.TrimSpace(string(decoded)) == "" {
		return "", ErrInvalidObservationPagination
	}
	return string(decoded), nil
}

func fleetObservationMatches(
	observation Observation,
	scope ObservationScope,
	states []State,
) bool {
	scopeMatches := (scope == ObservationScopeDirect && observation.Direct) ||
		(scope == ObservationScopeFactory && !observation.Direct) ||
		scope == ObservationScopeAll
	if !scopeMatches || len(states) == 0 {
		return scopeMatches
	}
	for _, state := range states {
		if state == observation.State {
			return true
		}
	}
	return false
}
