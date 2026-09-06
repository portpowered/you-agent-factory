package wire

import (
	"context"
	"encoding/base64"
	"errors"
	"sort"
	"strings"
	"time"

	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
)

// ObservationServiceCatalog resolves the Worker Sessions services currently
// visible in one process. The catalog is evaluated for every read so a
// Factory Session opened after HTTP binding is still part of the fleet view.
type ObservationServiceCatalog func(context.Context) ([]workersessions.Service, error)

// FleetObservationService merges runtime-owned Worker Session registries into
// one deterministic, bounded identity collection. It intentionally exposes
// only the top-level list capability; lifecycle and Work-scoped operations
// remain bound to their owning runtime/service.
type FleetObservationService struct {
	catalog ObservationServiceCatalog
}

// NewFleetObservationService constructs a process-wide top-level observation
// view from a dynamic service catalog. Construction stays in the owning
// service's wire package so the service root remains a single contract.
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
	req workersessions.ListWorkerSessionObservationsRequest,
) (workersessions.ListWorkerSessionObservationsResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	query, err := s.prepareFleetQuery(ctx, req)
	if err != nil {
		return workersessions.ListWorkerSessionObservationsResult{}, err
	}
	sources, err := s.catalog(ctx)
	if err != nil {
		return workersessions.ListWorkerSessionObservationsResult{}, err
	}
	observations, err := collectFleetObservations(ctx, sources, req, query.limit)
	if err != nil {
		return workersessions.ListWorkerSessionObservationsResult{}, err
	}
	return buildFleetObservationPage(observations, query), nil
}

type fleetObservationQuery struct {
	limit  int
	cursor string
	scope  workersessions.ObservationScope
	states []workersessions.State
}

func (s *FleetObservationService) prepareFleetQuery(
	ctx context.Context,
	req workersessions.ListWorkerSessionObservationsRequest,
) (fleetObservationQuery, error) {
	if s == nil || s.catalog == nil {
		return fleetObservationQuery{}, workersessions.ErrObservationProjectionUnavailable
	}
	if err := req.Validate(); err != nil {
		return fleetObservationQuery{}, err
	}
	if err := ctx.Err(); err != nil {
		return fleetObservationQuery{}, err
	}
	limit := req.MaxResults
	if limit == 0 {
		limit = workersessions.DefaultWorkerSessionObservationListMaxResults
	}
	decoded, err := decodeFleetObservationCursor(req.NextToken)
	if err != nil {
		return fleetObservationQuery{}, err
	}
	return fleetObservationQuery{
		limit:  limit,
		cursor: decoded,
		scope:  req.Scope.Normalized(),
		states: append([]workersessions.State(nil), req.States...),
	}, nil
}

func buildFleetObservationPage(
	observations map[string]workersessions.Observation,
	query fleetObservationQuery,
) workersessions.ListWorkerSessionObservationsResult {
	ids := fleetObservationIDs(observations, query)
	pageIDs := ids
	if len(pageIDs) > query.limit {
		pageIDs = pageIDs[:query.limit]
	}
	page := make([]workersessions.Observation, 0, len(pageIDs))
	for _, id := range pageIDs {
		page = append(page, observations[id])
	}
	nextToken := ""
	if len(ids) > len(pageIDs) && len(pageIDs) > 0 {
		nextToken = base64.StdEncoding.EncodeToString([]byte(pageIDs[len(pageIDs)-1]))
	}
	return workersessions.ListWorkerSessionObservationsResult{
		Observations: page,
		MaxResults:   query.limit,
		NextToken:    nextToken,
	}
}

func fleetObservationIDs(
	observations map[string]workersessions.Observation,
	query fleetObservationQuery,
) []string {
	ids := make([]string, 0, len(observations))
	for id, observation := range observations {
		if id <= query.cursor || !fleetObservationMatches(observation, query.scope, query.states) {
			continue
		}
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func collectFleetObservations(
	ctx context.Context,
	sources []workersessions.Service,
	req workersessions.ListWorkerSessionObservationsRequest,
	limit int,
) (map[string]workersessions.Observation, error) {
	observations := make(map[string]workersessions.Observation)
	for _, source := range sources {
		if source == nil {
			continue
		}
		if err := collectFleetSource(ctx, source, req, limit, observations); err != nil {
			return nil, err
		}
	}
	return observations, nil
}

func collectFleetSource(
	ctx context.Context,
	source workersessions.Service,
	req workersessions.ListWorkerSessionObservationsRequest,
	limit int,
	observations map[string]workersessions.Observation,
) error {
	nextToken := ""
	seenTokens := make(map[string]struct{})
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		sourceRequest := req
		sourceRequest.MaxResults = limit
		sourceRequest.NextToken = nextToken
		result, err := source.ListWorkerSessionObservations(ctx, sourceRequest)
		if err != nil && !errors.Is(err, workersessions.ErrObservationProjectionUnavailable) {
			return err
		}
		for _, observation := range result.Observations {
			addFleetObservation(observations, observation)
		}
		if strings.TrimSpace(result.NextToken) == "" {
			return nil
		}
		if _, exists := seenTokens[result.NextToken]; exists || result.NextToken == nextToken {
			return workersessions.ErrInvalidObservationPagination
		}
		seenTokens[result.NextToken] = struct{}{}
		nextToken = result.NextToken
	}
}

func addFleetObservation(
	observations map[string]workersessions.Observation,
	observation workersessions.Observation,
) {
	id := strings.TrimSpace(observation.WorkerSessionID)
	if id == "" {
		return
	}
	observation.WorkerSessionID = id
	current, exists := observations[id]
	if !exists {
		observations[id] = observation.Clone()
		return
	}
	observations[id] = mergeFleetObservation(current, observation)
}

// mergeFleetObservation combines duplicate views of one Worker Session. A
// Factory-scoped decorator may own canonical identity and recording health,
// while the process-local registry still owns live timing or a provider
// association. Keep the first source's authoritative non-empty facts and
// fill only gaps from the other source so catalog order cannot erase useful
// optional projection data.
func mergeFleetObservation(
	current workersessions.Observation,
	candidate workersessions.Observation,
) workersessions.Observation {
	merged := current.Clone()
	if merged.PredecessorWorkerSessionID == "" {
		merged.PredecessorWorkerSessionID = candidate.PredecessorWorkerSessionID
	}
	if merged.SuccessorWorkerSessionID == "" {
		merged.SuccessorWorkerSessionID = candidate.SuccessorWorkerSessionID
	}
	if merged.Model == nil {
		merged.Model = cloneFleetString(candidate.Model)
	}
	if merged.ReasoningEffort == nil {
		merged.ReasoningEffort = cloneFleetString(candidate.ReasoningEffort)
	}
	if merged.FactorySessionID == "" {
		merged.FactorySessionID = candidate.FactorySessionID
	}
	if !merged.ProviderSessionAvailable && candidate.ProviderSessionAvailable {
		merged.ProviderSession = candidate.ProviderSession.Clone()
		merged.ProviderSessionAvailable = true
	}
	if len(merged.WorkIDs) == 0 && len(candidate.WorkIDs) > 0 {
		merged.WorkIDs = append([]string(nil), candidate.WorkIDs...)
	}
	if merged.TurnID == "" {
		merged.TurnID = candidate.TurnID
	}
	if merged.AttemptID == "" {
		merged.AttemptID = candidate.AttemptID
	}
	if merged.StartedAt == nil {
		merged.StartedAt = cloneFleetTime(candidate.StartedAt)
	}
	if merged.EndedAt == nil {
		merged.EndedAt = cloneFleetTime(candidate.EndedAt)
	}
	if merged.Duration == nil {
		merged.Duration = cloneFleetDuration(candidate.Duration)
	}
	if (merged.DurationBasis == "" || merged.DurationBasis == workersessions.DurationBasisUnavailable) && candidate.DurationBasis != workersessions.DurationBasisUnavailable {
		merged.DurationBasis = candidate.DurationBasis
	}
	if merged.TokenUsage == nil && candidate.TokenUsage != nil {
		usage := candidate.TokenUsage.Clone()
		merged.TokenUsage = &usage
	}
	if merged.TurnUsage == nil && candidate.TurnUsage != nil {
		usage := candidate.TurnUsage.Clone()
		merged.TurnUsage = &usage
	}
	if (merged.Transcript == "" || merged.Transcript == workersessions.TranscriptAvailabilityUnavailable) && candidate.Transcript != workersessions.TranscriptAvailabilityUnavailable {
		merged.Transcript = candidate.Transcript
		merged.Parse = candidate.Parse.Clone()
	}
	if merged.RecordingHealth == "" {
		merged.RecordingHealth = candidate.RecordingHealth
		merged.RecordingHealthReason = candidate.RecordingHealthReason
	}
	if merged.Failure == nil && candidate.Failure != nil {
		failure := *candidate.Failure
		merged.Failure = &failure
	}
	if merged.ConfirmationState != workersessions.ConfirmationStateConfirmed && candidate.ConfirmationState == workersessions.ConfirmationStateConfirmed {
		merged.ConfirmationState = candidate.ConfirmationState
	}
	return merged
}

func cloneFleetString(value *string) *string {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}

func cloneFleetTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}

func cloneFleetDuration(value *time.Duration) *time.Duration {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}

func decodeFleetObservationCursor(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	decoded, err := base64.StdEncoding.DecodeString(value)
	if err != nil || strings.TrimSpace(string(decoded)) == "" {
		return "", workersessions.ErrInvalidObservationPagination
	}
	return string(decoded), nil
}

func fleetObservationMatches(
	observation workersessions.Observation,
	scope workersessions.ObservationScope,
	states []workersessions.State,
) bool {
	scopeMatches := (scope == workersessions.ObservationScopeDirect && observation.Direct) ||
		(scope == workersessions.ObservationScopeFactory && !observation.Direct) ||
		scope == workersessions.ObservationScopeAll
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
