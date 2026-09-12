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
// catalogued Worker Session services. Each source contributes one bounded
// lookahead page at the fleet cursor before the rows are merged, so a page
// cannot omit an observation merely because it belongs to a different Factory
// Session without exhaustively walking every source history.
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
	observations, err := collectFleetObservations(ctx, sources, req, query)
	if err != nil {
		return workersessions.ListWorkerSessionObservationsResult{}, err
	}
	if err := ctx.Err(); err != nil {
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
	query fleetObservationQuery,
) (map[string]workersessions.Observation, error) {
	observations := make(map[string]workersessions.Observation)
	for _, source := range sources {
		if source == nil {
			continue
		}
		page, err := readFleetSourcePage(ctx, source, req, query)
		if err != nil {
			return nil, err
		}
		for _, observation := range page.observations {
			if observation.WorkerSessionID <= query.cursor ||
				!fleetObservationMatches(observation, query.scope, query.states) {
				return nil, workersessions.ErrInvalidObservationPagination
			}
			addFleetObservation(observations, observation)
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
	}
	return observations, nil
}

type fleetSourcePage struct {
	observations []workersessions.Observation
}

func readFleetSourcePage(
	ctx context.Context,
	source workersessions.Service,
	req workersessions.ListWorkerSessionObservationsRequest,
	query fleetObservationQuery,
) (fleetSourcePage, error) {
	if err := ctx.Err(); err != nil {
		return fleetSourcePage{}, err
	}
	sourceRequest := req
	sourceRequest.MaxResults = fleetLookaheadLimit(query.limit)
	sourceRequest.NextToken = encodeFleetObservationCursor(query.cursor)
	result, err := source.ListWorkerSessionObservations(ctx, sourceRequest)
	if contextErr := ctx.Err(); contextErr != nil {
		return fleetSourcePage{}, contextErr
	}
	if err != nil && !errors.Is(err, workersessions.ErrObservationProjectionUnavailable) {
		return fleetSourcePage{}, err
	}
	if validationErr := validateFleetSourcePage(sourceRequest, result); validationErr != nil {
		return fleetSourcePage{}, validationErr
	}
	if errors.Is(err, workersessions.ErrObservationProjectionUnavailable) && len(result.Observations) == 0 {
		return fleetSourcePage{}, workersessions.ErrObservationProjectionUnavailable
	}
	page := fleetSourcePage{observations: cloneFleetObservations(result.Observations)}
	if err := ctx.Err(); err != nil {
		return fleetSourcePage{}, err
	}
	return page, nil
}

func fleetLookaheadLimit(limit int) int {
	if limit == int(^uint(0)>>1) {
		return limit
	}
	return limit + 1
}

func encodeFleetObservationCursor(value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	return base64.StdEncoding.EncodeToString([]byte(value))
}

func validateFleetSourcePage(
	request workersessions.ListWorkerSessionObservationsRequest,
	result workersessions.ListWorkerSessionObservationsResult,
) error {
	cursor, err := decodeFleetObservationCursor(request.NextToken)
	if err != nil {
		return workersessions.ErrInvalidObservationPagination
	}
	if len(result.Observations) > request.MaxResults {
		return workersessions.ErrInvalidObservationPagination
	}
	previousID := ""
	for _, observation := range result.Observations {
		id := strings.TrimSpace(observation.WorkerSessionID)
		if id == "" || id <= previousID || id <= cursor {
			return workersessions.ErrInvalidObservationPagination
		}
		if !fleetObservationMatches(observation, request.Scope.Normalized(), request.States) {
			return workersessions.ErrInvalidObservationPagination
		}
		previousID = id
	}

	nextToken := strings.TrimSpace(result.NextToken)
	if nextToken == "" {
		return nil
	}
	if len(result.Observations) != request.MaxResults {
		return workersessions.ErrInvalidObservationPagination
	}
	nextID, err := decodeFleetObservationCursor(nextToken)
	if err != nil || len(result.Observations) == 0 || nextID != previousID {
		return workersessions.ErrInvalidObservationPagination
	}
	if nextID <= cursor {
		return workersessions.ErrInvalidObservationPagination
	}
	return nil
}

func cloneFleetObservations(observations []workersessions.Observation) []workersessions.Observation {
	clones := make([]workersessions.Observation, 0, len(observations))
	for _, observation := range observations {
		clone := observation.Clone()
		clone.WorkerSessionID = strings.TrimSpace(clone.WorkerSessionID)
		clones = append(clones, clone)
	}
	return clones
}

func addFleetObservation(
	observations map[string]workersessions.Observation,
	observation workersessions.Observation,
) {
	id := strings.TrimSpace(observation.WorkerSessionID)
	if id == "" {
		return
	}
	observation = observation.Clone()
	observation.WorkerSessionID = id
	existing, exists := observations[id]
	if exists {
		mergeFleetObservation(&existing, observation)
		observations[id] = existing
		return
	}
	observations[id] = observation
}

func mergeFleetObservation(
	existing *workersessions.Observation,
	incoming workersessions.Observation,
) {
	// Catalog order is the deterministic authority for conflicting non-empty
	// facts. Later observations only fill fields the first observation did not
	// project, so a secondary source cannot replace a trusted fact or detail.
	preserveFleetString(&existing.PredecessorWorkerSessionID, incoming.PredecessorWorkerSessionID)
	preserveFleetString(&existing.SuccessorWorkerSessionID, incoming.SuccessorWorkerSessionID)
	preserveFleetString(&existing.FactorySessionID, incoming.FactorySessionID)
	preserveFleetString(&existing.TurnID, incoming.TurnID)
	preserveFleetString(&existing.AttemptID, incoming.AttemptID)
	preserveFleetString(&existing.StreamGenerationID, incoming.StreamGenerationID)
	preserveFleetStringPointer(&existing.Model, incoming.Model)
	preserveFleetStringPointer(&existing.ReasoningEffort, incoming.ReasoningEffort)
	if !existing.ProviderSessionAvailable && incoming.ProviderSessionAvailable {
		existing.ProviderSession = incoming.ProviderSession.Clone()
		existing.ProviderSessionAvailable = true
	} else if existing.ProviderSessionAvailable && incoming.ProviderSessionAvailable {
		if existing.ProviderSession.Provider == "" {
			existing.ProviderSession.Provider = incoming.ProviderSession.Provider
		}
		preserveFleetString(&existing.ProviderSession.Kind, incoming.ProviderSession.Kind)
		preserveFleetString(&existing.ProviderSession.ID, incoming.ProviderSession.ID)
	}
	if len(existing.WorkIDs) == 0 && len(incoming.WorkIDs) > 0 {
		existing.WorkIDs = append([]string(nil), incoming.WorkIDs...)
	}
	if !existing.State.Valid() && incoming.State.Valid() {
		existing.State = incoming.State
	}
	if existing.ConfirmationState == "" {
		existing.ConfirmationState = incoming.ConfirmationState
	}
	mergeFleetTiming(existing, incoming)
	if existing.Transcript == "" {
		existing.Transcript = incoming.Transcript
	}
	if existing.RecordingHealth == "" {
		existing.RecordingHealth = incoming.RecordingHealth
	}
	preserveFleetString(&existing.RecordingHealthReason, incoming.RecordingHealthReason)
	mergeFleetFailure(&existing.Failure, incoming.Failure)
	mergeFleetTokenUsage(&existing.TokenUsage, incoming.TokenUsage)
	mergeFleetTurnUsage(&existing.TurnUsage, incoming.TurnUsage)
	mergeFleetParseDiagnostics(&existing.Parse, incoming.Parse)
	if !existing.StateSequenceKnown && incoming.StateSequenceKnown {
		existing.StateSequence = incoming.StateSequence
		existing.StateSequenceKnown = true
	}
}

func mergeFleetTiming(existing *workersessions.Observation, incoming workersessions.Observation) {
	if existing.StartedAt == nil {
		existing.StartedAt = cloneFleetTime(incoming.StartedAt)
	}
	if existing.EndedAt == nil {
		existing.EndedAt = cloneFleetTime(incoming.EndedAt)
	}
	if existing.DurationBasis == "" {
		existing.DurationBasis = incoming.DurationBasis
	}
	if existing.Duration == nil && existing.DurationBasis != workersessions.DurationBasisUnavailable {
		existing.Duration = cloneFleetDuration(incoming.Duration)
	}
}

func preserveFleetString(current *string, candidate string) {
	if strings.TrimSpace(*current) == "" && strings.TrimSpace(candidate) != "" {
		*current = candidate
	}
}

func preserveFleetStringPointer(current **string, candidate *string) {
	if *current == nil && candidate != nil {
		value := *candidate
		*current = &value
	}
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

func cloneFleetFailure(value *workersessions.FailureCause) *workersessions.FailureCause {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}

func mergeFleetFailure(existing **workersessions.FailureCause, incoming *workersessions.FailureCause) {
	if *existing == nil {
		*existing = cloneFleetFailure(incoming)
		return
	}
	if incoming == nil {
		return
	}
	current := *existing
	if current.Kind == "" {
		current.Kind = incoming.Kind
	}
	preserveFleetString(&current.Detail, incoming.Detail)
	preserveFleetString(&current.AgentRunFailureClass, incoming.AgentRunFailureClass)
	if current.ProviderFailureKind == "" {
		current.ProviderFailureKind = incoming.ProviderFailureKind
	}
	if current.ProviderContinuationFailureKind == "" {
		current.ProviderContinuationFailureKind = incoming.ProviderContinuationFailureKind
	}
	if current.ProviderContinuationOutcome == "" {
		current.ProviderContinuationOutcome = incoming.ProviderContinuationOutcome
	}
}

func mergeFleetTokenUsage(existing **workersessions.TokenUsage, incoming *workersessions.TokenUsage) {
	if *existing == nil {
		if incoming != nil {
			clone := incoming.Clone()
			*existing = &clone
		}
		return
	}
	if incoming == nil {
		return
	}
	current := *existing
	if current.CacheWriteTokens == nil {
		current.CacheWriteTokens = cloneFleetInt(incoming.CacheWriteTokens)
	}
	if current.CachedInputTokens == nil {
		current.CachedInputTokens = cloneFleetInt(incoming.CachedInputTokens)
	}
	if current.InputTokens == nil {
		current.InputTokens = cloneFleetInt(incoming.InputTokens)
	}
	if current.OutputTokens == nil {
		current.OutputTokens = cloneFleetInt(incoming.OutputTokens)
	}
	if current.ReasoningOutputTokens == nil {
		current.ReasoningOutputTokens = cloneFleetInt(incoming.ReasoningOutputTokens)
	}
	if current.TotalTokens == nil {
		current.TotalTokens = cloneFleetInt(incoming.TotalTokens)
	}
}

func cloneFleetInt(value *int) *int {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}

func mergeFleetTurnUsage(existing **workersessions.TurnUsage, incoming *workersessions.TurnUsage) {
	if *existing == nil {
		if incoming != nil {
			clone := incoming.Clone()
			*existing = &clone
		}
		return
	}
	if incoming == nil {
		return
	}
	current := *existing
	if current.TurnCount == 0 {
		current.TurnCount = incoming.TurnCount
	}
	if current.FinalContextTokens == 0 {
		current.FinalContextTokens = incoming.FinalContextTokens
	}
	if current.PeakContextTokens == 0 {
		current.PeakContextTokens = incoming.PeakContextTokens
	}
}

func mergeFleetParseDiagnostics(existing *workersessions.ParseDiagnostics, incoming workersessions.ParseDiagnostics) {
	if existing.EventCount == 0 {
		existing.EventCount = incoming.EventCount
	}
	if existing.MalformedLineCount == 0 {
		existing.MalformedLineCount = incoming.MalformedLineCount
	}
	if existing.UnknownEventCount == 0 {
		existing.UnknownEventCount = incoming.UnknownEventCount
	}
	if len(existing.Errors) == 0 && len(incoming.Errors) > 0 {
		existing.Errors = append([]workersessions.ParseDiagnostic(nil), incoming.Errors...)
	}
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
